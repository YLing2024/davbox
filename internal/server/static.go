package server

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/YLing2024/davbox/web"
)

func distFS() (fs.FS, error) {
	return fs.Sub(web.Dist, "dist")
}

// serveIndex 返回 SPA 入口，/ 与 /admin 共用同一份 index.html。
func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	sub, err := distFS()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "前端资源不可用")
		return
	}
	data, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "前端资源不可用")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// serveStatic 返回 /assets/ 下的构建产物。
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		return
	}
	sub, err := distFS()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "前端资源不可用")
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || strings.Contains(name, "..") {
		writeError(w, http.StatusNotFound, "未找到")
		return
	}
	f, err := sub.Open(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "未找到")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "未找到")
		return
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, info.Name(), info.ModTime(), rs)
		return
	}
	_, _ = io.Copy(w, f)
}
