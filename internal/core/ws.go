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
	"time"
)

const (
	wsGUID           = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	wsMaxMessageSize = 16 * 1024 * 1024
	wsWriteTimeout   = 5 * time.Second
)

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
	return c.writeWithDeadline(0x1, payload)
}

func (c *wsClient) WritePong(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeWithDeadline(0xA, payload)
}

func (c *wsClient) writeWithDeadline(opcode byte, payload []byte) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
		return err
	}
	defer c.conn.SetWriteDeadline(time.Time{})
	return writeFrame(c.conn, opcode, payload)
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

type wsFrame struct {
	fin     bool
	opcode  byte
	payload []byte
}

func readFrame(r *bufio.Reader) (wsFrame, error) {
	b1, err := r.ReadByte()
	if err != nil {
		return wsFrame{}, err
	}
	b2, err := r.ReadByte()
	if err != nil {
		return wsFrame{}, err
	}
	fin := b1&0x80 != 0
	opcode := b1 & 0x0f
	masked := b2&0x80 != 0
	ln := uint64(b2 & 0x7f)
	if ln == 126 {
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return wsFrame{}, err
		}
		ln = uint64(binary.BigEndian.Uint16(b[:]))
	} else if ln == 127 {
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return wsFrame{}, err
		}
		ln = binary.BigEndian.Uint64(b[:])
		if ln>>63 != 0 {
			return wsFrame{}, errors.New("invalid websocket frame length")
		}
	}
	if opcode >= 0x8 {
		if !fin {
			return wsFrame{}, errors.New("fragmented websocket control frame")
		}
		if ln > 125 {
			return wsFrame{}, errors.New("websocket control frame too large")
		}
	}
	if ln > wsMaxMessageSize {
		return wsFrame{}, errors.New("websocket frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return wsFrame{}, err
		}
	}
	payload := make([]byte, int(ln))
	if _, err := io.ReadFull(r, payload); err != nil {
		return wsFrame{}, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return wsFrame{fin: fin, opcode: opcode, payload: payload}, nil
}

// ReadMessage reassembles RFC 6455 continuation frames into one logical
// message. Control frames can appear between fragments; ping frames are
// answered immediately without abandoning the in-progress message.
func (c *wsClient) ReadMessage() (opcode byte, payload []byte, err error) {
	var messageOpcode byte
	var message []byte
	fragmented := false

	for {
		frame, readErr := readFrame(c.reader)
		if readErr != nil {
			return 0, nil, readErr
		}

		switch frame.opcode {
		case 0x8: // close
			return frame.opcode, frame.payload, nil
		case 0x9: // ping
			if fragmented {
				if err := c.WritePong(frame.payload); err != nil {
					return 0, nil, err
				}
				continue
			}
			return frame.opcode, frame.payload, nil
		case 0xA: // pong
			if fragmented {
				continue
			}
			return frame.opcode, frame.payload, nil
		case 0x0: // continuation
			if !fragmented {
				return 0, nil, errors.New("unexpected websocket continuation frame")
			}
			if len(message)+len(frame.payload) > wsMaxMessageSize {
				return 0, nil, errors.New("websocket message too large")
			}
			message = append(message, frame.payload...)
			if frame.fin {
				return messageOpcode, message, nil
			}
		case 0x1, 0x2: // text / binary
			if fragmented {
				return 0, nil, errors.New("new websocket data frame before fragmented message completed")
			}
			if frame.fin {
				return frame.opcode, frame.payload, nil
			}
			fragmented = true
			messageOpcode = frame.opcode
			message = append(message[:0], frame.payload...)
		default:
			return 0, nil, fmt.Errorf("unsupported websocket opcode 0x%x", frame.opcode)
		}
	}
}
