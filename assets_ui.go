//go:build !headless

package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:frontend/dist
var assets embed.FS

const hasUI = true

func staticHandler() http.Handler {
	root, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(root))
}
