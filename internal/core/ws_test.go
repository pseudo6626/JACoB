package core

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func testWSFrame(fin bool, opcode byte, payload []byte) []byte {
	first := opcode
	if fin {
		first |= 0x80
	}
	out := []byte{first}
	n := len(payload)
	switch {
	case n < 126:
		out = append(out, byte(n))
	case n <= 65535:
		out = append(out, 126, byte(n>>8), byte(n))
	default:
		out = append(out, 127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		out = append(out, b[:]...)
	}
	return append(out, payload...)
}

func TestReadMessageReassemblesFragmentedText(t *testing.T) {
	wire := append(testWSFrame(false, 0x1, []byte("hello ")), testWSFrame(true, 0x0, []byte("world"))...)
	c := &wsClient{reader: bufio.NewReader(bytes.NewReader(wire))}
	op, payload, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if op != 0x1 || string(payload) != "hello world" {
		t.Fatalf("opcode=%x payload=%q", op, payload)
	}
}

func TestReadMessageRejectsOversizeFragmentedMessage(t *testing.T) {
	first := testWSFrame(false, 0x1, []byte(strings.Repeat("a", wsMaxMessageSize/2+1)))
	second := testWSFrame(true, 0x0, []byte(strings.Repeat("b", wsMaxMessageSize/2+1)))
	wire := append(first, second...)
	c := &wsClient{reader: bufio.NewReader(bytes.NewReader(wire))}
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("expected oversize fragmented message error")
	}
}
