package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/YLing2024/davbox/internal/account"
	"github.com/YLing2024/davbox/internal/auth"
	"github.com/YLing2024/davbox/internal/usage"
)

const (
	cookieAdmin  = "davbox_admin"
	cookieClient = "davbox_client"
	sessionTTL   = 12 * time.Hour
)

func setSessionCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) adminSession(r *http.Request) (auth.Session, bool) {
	c, err := r.Cookie(cookieAdmin)
	if err != nil {
		return auth.Session{}, false
	}
	sess, ok := s.signer.Verify(c.Value)
	if !ok || sess.Kind != "admin" {
		return auth.Session{}, false
	}
	return sess, true
}

// adminAuth 按当前认证模式判断管理端身份。
// builtin：校验会话 cookie，用户标识为空串。
// sso：只认网关注入的 X-Auth-User，缺失或空白即未认证（绝不回退到 cookie）。
func (s *Server) adminAuth(r *http.Request) (string, bool) {
	if s.authMode.IsSSO() {
		user := strings.TrimSpace(r.Header.Get(auth.HeaderAuthUser))
		if user == "" {
			return "", false
		}
		return user, true
	}
	sess, ok := s.adminSession(r)
	if !ok {
		return "", false
	}
	return sess.User, true
}

type accountView struct {
	User      string `json:"user"`
	Root      string `json:"root"`
	Readonly  bool   `json:"readonly"`
	Disabled  bool   `json:"disabled"`
	Note      string `json:"note"`
	UsedBytes int64  `json:"usedBytes"`
	UsedFiles int64  `json:"usedFiles"`
}

func (s *Server) viewOf(a account.Account) accountView {
	b, f := s.usage.stat(a.User, a.Root, usage.Dir)
	return accountView{
		User: a.User, Root: a.Root, Readonly: a.Readonly, Disabled: a.Disabled,
		Note: a.Note, UsedBytes: b, UsedFiles: f,
	}
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if xf := r.Header.Get("X-Forwarded-Proto"); xf != "" {
		if i := strings.IndexByte(xf, ','); i >= 0 {
			xf = xf[:i]
		}
		xf = strings.TrimSpace(xf)
		if xf == "https" || xf == "http" {
			scheme = xf
		}
	}
	return scheme + "://" + r.Host
}

func connURL(r *http.Request, user string) string {
	return requestBaseURL(r) + "/" + user
}

// handleAdmin 处理 /api/admin 下的全部请求。
func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin")
	rest = strings.TrimPrefix(rest, "/")
	var parts []string
	if rest != "" {
		parts = strings.Split(rest, "/")
	}

	// 认证模式查询：前端在未登录时也要知道当前模式，故不鉴权。
	if len(parts) == 1 && parts[0] == "mode" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"mode": string(s.authMode)})
		return
	}

	// SSO 模式不提供自带口令的登录/退出，交由网关的 /_auth/* 处理。
	if s.authMode.IsSSO() && len(parts) == 1 && (parts[0] == "login" || parts[0] == "logout") {
		writeError(w, http.StatusNotFound, "未找到")
		return
	}

	if len(parts) == 1 && parts[0] == "login" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		s.adminLogin(w, r)
		return
	}

	if _, ok := s.adminAuth(r); !ok {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}

	switch {
	case len(parts) == 1 && parts[0] == "logout":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		clearSessionCookie(w, cookieAdmin)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

	case len(parts) == 1 && parts[0] == "accounts":
		switch r.Method {
		case http.MethodGet:
			s.adminListAccounts(w, r)
		case http.MethodPost:
			s.adminCreateAccount(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		}

	case len(parts) == 2 && parts[0] == "accounts":
		user := parts[1]
		switch r.Method {
		case http.MethodPatch:
			s.adminPatchAccount(w, r, user)
		case http.MethodDelete:
			s.adminDeleteAccount(w, r, user)
		default:
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
		}

	case len(parts) == 3 && parts[0] == "accounts" && parts[2] == "conn":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		s.adminConn(w, r, parts[1])

	case len(parts) == 3 && parts[0] == "accounts" && parts[2] == "rotate":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "方法不允许")
			return
		}
		s.adminRotate(w, r, parts[1])

	default:
		writeError(w, http.StatusNotFound, "未找到")
	}
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if bcrypt.CompareHashAndPassword(s.adminHash, []byte(body.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "口令错误")
		return
	}
	setSessionCookie(w, cookieAdmin, s.signer.Issue("admin", "", sessionTTL))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) adminListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts := s.store.List()
	out := make([]accountView, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, s.viewOf(a))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) adminCreateAccount(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User     string `json:"user"`
		Readonly bool   `json:"readonly"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	a, pass, err := s.store.Create(strings.TrimSpace(body.User), body.Readonly, body.Note)
	switch {
	case errors.Is(err, account.ErrInvalidName):
		writeError(w, http.StatusBadRequest, "账号名只能用小写字母、数字、下划线或连字符，且不超过 32 位")
		return
	case errors.Is(err, account.ErrDuplicate):
		writeError(w, http.StatusConflict, "账号已存在")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "创建失败")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user": a.User,
		"pass": pass,
		"url":  connURL(r, a.User),
	})
}

func (s *Server) adminConn(w http.ResponseWriter, r *http.Request, user string) {
	a, ok := s.store.Get(user)
	if !ok {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"url":  connURL(r, a.User),
		"user": a.User,
		"pass": a.Pass,
	})
}

func (s *Server) adminRotate(w http.ResponseWriter, r *http.Request, user string) {
	pass, err := s.store.Rotate(user)
	if errors.Is(err, account.ErrNotFound) {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "换口令失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"user": user,
		"pass": pass,
		"url":  connURL(r, user),
	})
}

func (s *Server) adminPatchAccount(w http.ResponseWriter, r *http.Request, user string) {
	var body struct {
		Readonly *bool   `json:"readonly"`
		Disabled *bool   `json:"disabled"`
		Note     *string `json:"note"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	a, err := s.store.Update(user, account.Patch{
		Readonly: body.Readonly,
		Disabled: body.Disabled,
		Note:     body.Note,
	})
	if errors.Is(err, account.ErrNotFound) {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "更新失败")
		return
	}
	writeJSON(w, http.StatusOK, s.viewOf(a))
}

func (s *Server) adminDeleteAccount(w http.ResponseWriter, r *http.Request, user string) {
	err := s.store.Delete(user)
	if errors.Is(err, account.ErrNotFound) {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "删除失败")
		return
	}
	s.forgetLocks(user)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "账号已删除，磁盘目录保留",
	})
}
