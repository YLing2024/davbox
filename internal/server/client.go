package server

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/YLing2024/davbox/internal/account"
)

var errBadPath = errors.New("路径不合法")

func (s *Server) clientSession(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookieClient)
	if err != nil {
		return "", false
	}
	sess, ok := s.signer.Verify(c.Value)
	if !ok || sess.Kind != "client" || sess.User == "" {
		return "", false
	}
	return sess.User, true
}

func (s *Server) requireClient(w http.ResponseWriter, r *http.Request) (account.Account, bool) {
	user, ok := s.clientSession(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "未登录")
		return account.Account{}, false
	}
	a, ok := s.store.Get(user)
	if !ok || a.Disabled {
		clearSessionCookie(w, cookieClient)
		writeError(w, http.StatusUnauthorized, "账号不可用")
		return account.Account{}, false
	}
	return a, true
}

// clientPath 把客户端路径解析到账号 root 内；任何逃逸都返回 errBadPath。
func (s *Server) clientPath(acct account.Account, p string) (string, error) {
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "", errBadPath
	}
	if strings.Contains(p, "..") || strings.Contains(p, `\`) || strings.ContainsRune(p, 0) {
		return "", errBadPath
	}
	clean := path.Clean(p)
	full := filepath.Join(acct.Root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(acct.Root, full)
	if err != nil {
		return "", errBadPath
	}
	if rel != "." && (rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
		return "", errBadPath
	}
	return full, nil
}

// handleClient 处理 /api/client 下的全部请求。
func (s *Server) handleClient(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/client")
	rest = strings.TrimPrefix(rest, "/")
	var parts []string
	if rest != "" {
		parts = strings.Split(rest, "/")
	}

	if len(parts) == 1 && parts[0] == "login" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		s.clientLogin(w, r)
		return
	}

	acct, ok := s.requireClient(w, r)
	if !ok {
		return
	}

	if r.URL.Path == "/api/client/logout" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		clearSessionCookie(w, cookieClient)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	switch {
	case r.URL.Path == "/api/client/list":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		s.clientList(w, r, acct)

	case r.URL.Path == "/api/client/raw" || strings.HasPrefix(r.URL.Path, "/api/client/raw/"):
		p := strings.TrimPrefix(r.URL.Path, "/api/client/raw")
		switch r.Method {
		case http.MethodGet, http.MethodHead:
			s.clientDownload(w, r, acct, p)
		case http.MethodPut:
			s.clientUpload(w, r, acct, p)
		default:
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		}

	case r.URL.Path == "/api/client/mkdir":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		s.clientMkdir(w, r, acct)

	case r.URL.Path == "/api/client/rename":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		s.clientRename(w, r, acct)

	case r.URL.Path == "/api/client/entry":
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		s.clientDelete(w, r, acct)

	default:
		writeError(w, http.StatusNotFound, "未找到")
	}
}

func (s *Server) clientLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	a, ok := s.store.Authenticate(strings.TrimSpace(body.User), body.Pass)
	if !ok {
		writeError(w, http.StatusUnauthorized, "账号或口令错误")
		return
	}
	setSessionCookie(w, cookieClient, s.signer.Issue("client", a.User, sessionTTL))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": a.User})
}

type entryView struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
}

func (s *Server) clientList(w http.ResponseWriter, r *http.Request, acct account.Account) {
	full, err := s.clientPath(acct, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "路径不合法")
		return
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		writeError(w, http.StatusNotFound, "目录不存在")
		return
	}
	out := make([]entryView, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		v := entryView{Name: e.Name(), IsDir: e.IsDir(), Mtime: info.ModTime().UnixMilli()}
		if !e.IsDir() {
			v.Size = info.Size()
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		li, lj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if li != lj {
			return li < lj
		}
		return out[i].Name < out[j].Name
	})
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) clientDownload(w http.ResponseWriter, r *http.Request, acct account.Account, p string) {
	full, err := s.clientPath(acct, p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "路径不合法")
		return
	}
	f, err := os.Open(full)
	if err != nil {
		writeError(w, http.StatusNotFound, "文件不存在")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusBadRequest, "不能下载目录")
		return
	}
	ct := mime.TypeByExtension(filepath.Ext(info.Name()))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(info.Name()))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (s *Server) clientUpload(w http.ResponseWriter, r *http.Request, acct account.Account, p string) {
	if acct.Readonly {
		writeError(w, http.StatusForbidden, "只读账号")
		return
	}
	full, err := s.clientPath(acct, p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "路径不合法")
		return
	}
	parent := filepath.Dir(full)
	if info, statErr := os.Stat(parent); statErr != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "父目录不存在")
		return
	}
	tmp, err := os.CreateTemp(parent, ".davbox-upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "写入失败")
		return
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, r.Body); err != nil {
		tmp.Close()
		writeError(w, http.StatusInternalServerError, "写入失败")
		return
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		writeError(w, http.StatusInternalServerError, "写入失败")
		return
	}
	if err := tmp.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "写入失败")
		return
	}
	if err := os.Rename(tmpName, full); err != nil {
		writeError(w, http.StatusInternalServerError, "写入失败")
		return
	}
	ok = true
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) clientMkdir(w http.ResponseWriter, r *http.Request, acct account.Account) {
	if acct.Readonly {
		writeError(w, http.StatusForbidden, "只读账号")
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	full, err := s.clientPath(acct, body.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "路径不合法")
		return
	}
	if err := os.Mkdir(full, 0o755); err != nil {
		switch {
		case os.IsExist(err):
			writeError(w, http.StatusConflict, "已存在")
		case os.IsNotExist(err):
			writeError(w, http.StatusBadRequest, "父目录不存在")
		default:
			writeError(w, http.StatusBadRequest, "创建失败")
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Server) clientRename(w http.ResponseWriter, r *http.Request, acct account.Account) {
	if acct.Readonly {
		writeError(w, http.StatusForbidden, "只读账号")
		return
	}
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	from, err := s.clientPath(acct, body.From)
	if err != nil || body.From == "/" {
		writeError(w, http.StatusBadRequest, "路径不合法")
		return
	}
	to, err := s.clientPath(acct, body.To)
	if err != nil || body.To == "/" {
		writeError(w, http.StatusBadRequest, "路径不合法")
		return
	}
	if _, err := os.Stat(from); err != nil {
		writeError(w, http.StatusNotFound, "来源不存在")
		return
	}
	if info, statErr := os.Stat(filepath.Dir(to)); statErr != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "目标父目录不存在")
		return
	}
	if _, err := os.Stat(to); err == nil {
		writeError(w, http.StatusConflict, "目标已存在")
		return
	}
	if err := os.Rename(from, to); err != nil {
		writeError(w, http.StatusInternalServerError, "重命名失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) clientDelete(w http.ResponseWriter, r *http.Request, acct account.Account) {
	if acct.Readonly {
		writeError(w, http.StatusForbidden, "只读账号")
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" || p == "/" {
		writeError(w, http.StatusBadRequest, "不能删除根目录")
		return
	}
	full, err := s.clientPath(acct, p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "路径不合法")
		return
	}
	if err := os.Remove(full); err != nil {
		switch {
		case os.IsNotExist(err):
			writeError(w, http.StatusNotFound, "条目不存在")
		case errors.Is(err, syscall.ENOTEMPTY), errors.Is(err, syscall.EEXIST):
			writeError(w, http.StatusConflict, "目录非空")
		default:
			writeError(w, http.StatusBadRequest, "删除失败")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
