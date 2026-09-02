package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
)

//go:embed dist/*
var files embed.FS

func Handler() http.Handler {
	root, _ := fs.Sub(files, "dist")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := path.Clean(r.URL.Path)
		if name == "." || name == "/" {
			name = "index.html"
		} else {
			name = name[1:]
		}
		if _, e := root.Open(name); e != nil {
			if path.Ext(name) == "" {
				name = "index.html"
			} else {
				http.NotFound(w, r)
				return
			}
		}
		if path.Base(name) != "index.html" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
		http.FileServer(http.FS(root)).ServeHTTP(w, r)
	})
}
