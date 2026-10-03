package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/* assets/docs/* assets/docs/schemas/*
var content embed.FS

func Handler() http.Handler {
	sub, _ := fs.Sub(content, "assets")
	return http.FileServer(http.FS(sub))
}
