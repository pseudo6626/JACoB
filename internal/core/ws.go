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

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type wsClient struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
	local  bool
}

func upgradeWebSocket(w http.ResponseWriter, r *http.Request) (*wsClient, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("missing websocket upgrade")
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

func (c *wsClient) ReadMessage() (opcode byte, payload []byte, err error) {
	r := c.reader
	for {
		b1, readErr := r.ReadByte()
		if readErr != nil {
			return 0, nil, readErr
		}
		b2, readErr := r.ReadByte()
		if readErr != nil {
			return 0, nil, readErr
		}
		opcode = b1 & 0x0f
		masked := b2&0x80 != 0
		ln := uint64(b2 & 0x7f)
		if ln == 126 {
			var b [2]byte
			if _, err = io.ReadFull(r, b[:]); err != nil {
				return
			}
			ln = uint64(binary.BigEndian.Uint16(b[:]))
		} else if ln == 127 {
			var b [8]byte
			if _, err = io.ReadFull(r, b[:]); err != nil {
				return
			}
			ln = binary.BigEndian.Uint64(b[:])
		}
		if ln > 8*1024*1024 {
			return 0, nil, errors.New("websocket message too large")
		}
		var mask [4]byte
		if masked {
			if _, err = io.ReadFull(r, mask[:]); err != nil {
				return
			}
		}
		payload = make([]byte, int(ln))
		if _, err = io.ReadFull(r, payload); err != nil {
			return
		}
		if masked {
			for i := range payload {
				payload[i] ^= mask[i%4]
			}
		}
		return opcode, payload, nil
	}
}
