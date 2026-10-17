package core

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"testing"
	"time"

	"jacob/internal/clipboard"
)

type clipboardReplyConn struct {
	net.Conn
	data bytes.Buffer
}

func (c *clipboardReplyConn) Write(b []byte) (int, error)      { return c.data.Write(b) }
func (c *clipboardReplyConn) SetWriteDeadline(time.Time) error { return nil }

type fakeClipboard struct {
	writes []string
	err    error
}

func (f *fakeClipboard) Info() clipboard.Info {
	return clipboard.Info{Available: true, Backend: "test", WriteOnly: true, HostOnly: true}
}
func (f *fakeClipboard) WriteText(s string) (clipboard.Result, error) {
	f.writes = append(f.writes, s)
	if f.err != nil {
		return clipboard.Result{}, f.err
	}
	return clipboard.Result{Written: true, Backend: "test", Bytes: len(s)}, nil
}

func clipboardRequest(t *testing.T, s *Server, local bool, method string, params map[string]any) envelope {
	t.Helper()
	c := &clipboardReplyConn{}
	s.handleRequest(&wsClient{conn: c, local: local}, envelope{Type: "request", ID: "test", Method: method, Params: params})
	b := c.data.Bytes()
	if len(b) < 2 {
		t.Fatal("no websocket reply")
	}
	offset := 2
	if b[1]&127 == 126 {
		offset = 4
	} else if b[1]&127 == 127 {
		offset = 10
	}
	if offset == 4 && int(binary.BigEndian.Uint16(b[2:4])) != len(b)-offset {
		t.Fatal("bad frame length")
	}
	var reply envelope
	if err := json.Unmarshal(b[offset:], &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

func TestClipboardRPCValidationAndLocalOnly(t *testing.T) {
	f := &fakeClipboard{}
	s := &Server{clipboard: f}
	for _, tc := range []struct {
		local  bool
		params map[string]any
		code   string
	}{
		{false, map[string]any{"text": "target"}, "HOST_ONLY"},
		{true, nil, "BAD_PARAMS"},
		{true, map[string]any{"text": 12}, "BAD_PARAMS"},
	} {
		r := clipboardRequest(t, s, tc.local, "clipboard.writeText", tc.params)
		if r.Error == nil || r.Error.Code != tc.code {
			t.Fatalf("unexpected reply %+v", r)
		}
	}
	if len(f.writes) != 0 {
		t.Fatal("rejected request wrote clipboard")
	}
	for _, text := range []string{"target 🌟", ""} {
		r := clipboardRequest(t, s, true, "clipboard.writeText", map[string]any{"text": text})
		if r.OK == nil || !*r.OK || r.Result.(map[string]any)["written"] != true {
			t.Fatalf("unexpected reply %+v", r)
		}
		if f.writes[len(f.writes)-1] != text {
			t.Fatal("text changed")
		}
	}
}

func TestClipboardRPCInfoAndFailure(t *testing.T) {
	f := &fakeClipboard{}
	s := &Server{clipboard: f}
	r := clipboardRequest(t, s, false, "clipboard.info", nil)
	if r.OK == nil || !*r.OK || len(f.writes) != 0 {
		t.Fatal("info must work without writes")
	}
	for _, err := range []error{&clipboard.Error{Code: "CLIPBOARD_BUSY", Message: "busy"}, errors.New("failure")} {
		f.err = err
		r = clipboardRequest(t, s, true, "clipboard.writeText", map[string]any{"text": "target"})
		if r.Error == nil || r.OK == nil || *r.OK {
			t.Fatalf("failure claimed success: %+v", r)
		}
	}
	r = clipboardRequest(t, &Server{}, true, "clipboard.writeText", map[string]any{"text": "target"})
	if r.Error == nil || r.Error.Code != "CLIPBOARD_UNAVAILABLE" {
		t.Fatal("missing service should be explicit")
	}
}
