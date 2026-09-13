package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves embedded Chat UI at /ui/ with SPA fallback.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/ui", &spaFileServer{fs: sub})
}

type spaFileServer struct {
	fs fs.FS
}

func (s *spaFileServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	if _, err := fs.Stat(s.fs, name); err != nil {
		name = "index.html"
	}
	http.ServeFileFS(w, r, s.fs, name)
}
