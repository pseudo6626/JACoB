package core

import (
	"errors"

	"jacob/internal/clipboard"
)

type clipboardWriter interface {
	Info() clipboard.Info
	WriteText(string) (clipboard.Result, error)
}

func (s *Server) handleClipboard(c *wsClient, req envelope) {
	if req.Method == "clipboard.writeText" && !s.requirePhysicalHost(c, req.ID, "clipboard writing") {
		return
	}
	if s.clipboard == nil {
		_ = s.sendError(c, req.ID, "CLIPBOARD_UNAVAILABLE", "host clipboard service is unavailable")
		return
	}
	if req.Method == "clipboard.info" {
		s.sendResult(c, req.ID, s.clipboard.Info())
		return
	}
	text, ok := req.Params["text"].(string)
	if !ok {
		_ = s.sendError(c, req.ID, "BAD_PARAMS", "params.text must be a string")
		return
	}
	result, err := s.clipboard.WriteText(text)
	if err != nil {
		code := "CLIPBOARD_FAILED"
		var ce *clipboard.Error
		if errors.As(err, &ce) {
			code = ce.Code
		}
		_ = s.sendError(c, req.ID, code, err.Error())
		return
	}
	s.sendResult(c, req.ID, result)
}
