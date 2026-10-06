package core

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
)

const (
	wsGUID           = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	wsMaxMessageSize = 8 * 1024 * 1024
)

type wsClient struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
	local  bool

	fragmentOpcode  byte
	fragmentPayload []byte
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsClient, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("missing websocket upgrade")
	}
	connectionUpgrade := false
	for _, part := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
			connectionUpgrade = true
			break
		}
	}
	if !connectionUpgrade {
		return nil, errors.New("missing Connection: Upgrade")
	}
	if v := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Version")); v != "13" {
		return nil, errors.New("unsupported websocket version")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}
	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("server does not support hijacking")
	}
	conn, rw, err := h.Hijack()
	if err != nil {
		return nil, err
	}
	acceptRaw := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(acceptRaw[:])
	_, err = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &wsClient{conn: conn, reader: rw.Reader}, nil
}

func (c *wsClient) Close() error { return c.conn.Close() }

func (c *wsClient) WriteText(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeFrame(c.conn, 0x1, payload)
}

func (c *wsClient) WritePong(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeFrame(c.conn, 0xA, payload)
}

func writeFrame(w io.Writer, opcode byte, payload []byte) error {
	header := []byte{0x80 | opcode}
	n := len(payload)
	switch {
	case n < 126:
		header = append(header, byte(n))
	case n <= 65535:
		header = append(header, 126, byte(n>>8), byte(n))
	default:
		header = append(header, 127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		header = append(header, b[:]...)
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func (c *wsClient) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	r := c.reader
	b1, err := r.ReadByte()
	if err != nil {
		return false, 0, nil, err
	}
	b2, err := r.ReadByte()
	if err != nil {
		return false, 0, nil, err
	}
	if b1&0x70 != 0 {
		return false, 0, nil, errors.New("websocket extensions are not supported")
	}
	fin = b1&0x80 != 0
	opcode = b1 & 0x0f
	masked := b2&0x80 != 0
	if !masked {
		return false, 0, nil, errors.New("client websocket frames must be masked")
	}

	ln := uint64(b2 & 0x7f)
	switch ln {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return false, 0, nil, err
		}
		ln = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return false, 0, nil, err
		}
		if b[0]&0x80 != 0 {
			return false, 0, nil, errors.New("invalid websocket payload length")
		}
		ln = binary.BigEndian.Uint64(b[:])
	}
	if ln > wsMaxMessageSize {
		return false, 0, nil, errors.New("websocket message too large")
	}

	var mask [4]byte
	if _, err = io.ReadFull(r, mask[:]); err != nil {
		return false, 0, nil, err
	}
	payload = make([]byte, int(ln))
	if _, err = io.ReadFull(r, payload); err != nil {
		return false, 0, nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return fin, opcode, payload, nil
}

func (c *wsClient) ReadMessage() (opcode byte, payload []byte, err error) {
	for {
		fin, op, framePayload, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}

		if op >= 0x8 {
			if !fin {
				return 0, nil, errors.New("fragmented websocket control frame")
			}
			if len(framePayload) > 125 {
				return 0, nil, errors.New("websocket control frame too large")
			}
			switch op {
			case 0x8, 0x9, 0xA:
				return op, framePayload, nil
			default:
				return 0, nil, fmt.Errorf("unsupported websocket control opcode 0x%x", op)
			}
		}

		switch op {
		case 0x0:
			if c.fragmentOpcode == 0 {
				return 0, nil, errors.New("unexpected websocket continuation frame")
			}
			if len(c.fragmentPayload)+len(framePayload) > wsMaxMessageSize {
				c.fragmentOpcode = 0
				c.fragmentPayload = nil
				return 0, nil, errors.New("websocket message too large")
			}
			c.fragmentPayload = append(c.fragmentPayload, framePayload...)
			if !fin {
				continue
			}
			opcode = c.fragmentOpcode
			payload = c.fragmentPayload
			c.fragmentOpcode = 0
			c.fragmentPayload = nil
			return opcode, payload, nil

		case 0x1, 0x2:
			if c.fragmentOpcode != 0 {
				return 0, nil, errors.New("new websocket data frame before fragmented message completed")
			}
			if fin {
				return op, framePayload, nil
			}
			c.fragmentOpcode = op
			c.fragmentPayload = append(c.fragmentPayload[:0], framePayload...)

		default:
			return 0, nil, fmt.Errorf("unsupported websocket opcode 0x%x", op)
		}
	}
}
