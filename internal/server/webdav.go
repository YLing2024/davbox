package server

import (
	"bufio"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/webdav"

	"github.com/YLing2024/davbox/internal/account"
)

// isWriteMethod 列出只读账号需要拒绝的动词。
// LOCK/UNLOCK 也会改变服务端状态（并可能创建资源），一并阻断。
func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPut, http.MethodDelete, "MKCOL", "MOVE", "COPY", "PROPPATCH", "LOCK", "UNLOCK":
		return true
	}
	return false
}

// davFSFor 为账号构造带死属性持久化的文件系统。
// sidecar 放在账号 root 之外，保证不会出现在客户端可见目录里。
func (s *Server) davFSFor(acct account.Account) webdav.FileSystem {
	return newDeadPropFS(acct.Root, filepath.Join(s.dataDir, "davprops", acct.User))
}

func writeAuthChallenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="davbox"`)
	writeError(w, http.StatusUnauthorized, "未认证")
}

// lockSystem 为每个账号复用同一个内存锁系统。
func (s *Server) lockSystem(user string) webdav.LockSystem {
	s.davMu.Lock()
	defer s.davMu.Unlock()
	ls := s.locks[user]
	if ls == nil {
		ls = webdav.NewMemLS()
		s.locks[user] = ls
	}
	return ls
}

// forgetLocks 在账号被删除后释放其锁系统。
func (s *Server) forgetLocks(user string) {
	s.davMu.Lock()
	defer s.davMu.Unlock()
	delete(s.locks, user)
}

// accessLogEnabled 由 DAVBOX_ACCESS_LOG 控制（1/on 开，默认关）。打开时
// 已认证的 WebDAV 数据面请求全量落 access log（journal），成功失败都记。
// 与 Logger 不同，这里只记方法、路径、返回码、来源 IP 与 UA，绝不触碰
// Authorization。平时默认关，无 journal 噪音；排查客户端兼容性时打开。
func accessLogEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DAVBOX_ACCESS_LOG"))) {
	case "1", "on", "true", "yes":
		return true
	}
	return false
}

// accessWriter 包一层 ResponseWriter 以拿到最终状态码；Flush/Hijack 如实透传。
type accessWriter struct {
	http.ResponseWriter
	status int
}

func (a *accessWriter) WriteHeader(code int) {
	a.status = code
	a.ResponseWriter.WriteHeader(code)
}

func (a *accessWriter) Flush() {
	if f, ok := a.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (a *accessWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := a.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// handleDAV 完成认证、只读检查后交给官方 webdav.Handler。
// 兼容性：一木记账等 App 用不带尾斜杠的 `/<账号>` 甚至 `/<账号>/<目录>`
// 做 PROPFIND 探测，而官方 webdav.Handler 的 Prefix 要求带斜杠，否则
// prefix mismatch → 404。这里在进入 Handler 前只对“恰好等于 /<账号>”
// 的路径补上尾斜杠（账号根集合），子路径一律不动，避免改变文件语义。
func (s *Server) handleDAV(w http.ResponseWriter, r *http.Request, user string) {
	// 兼容性重写：`PROPFIND /<账号>`（无尾斜杠）补成 `/<账号>/`。
	// 仅当路径恰好是账号根时才改写；子路径一律不动。
	if r.URL.Path == "/"+user {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + user + "/"
		r = r2
	}
	authUser, pass, ok := r.BasicAuth()
	if !ok || authUser != user {
		writeAuthChallenge(w)
		return
	}
	acct, ok := s.store.Authenticate(user, pass)
	if !ok {
		writeAuthChallenge(w)
		return
	}
	if acct.Readonly && isWriteMethod(r.Method) {
		writeError(w, http.StatusForbidden, "只读账号")
		return
	}

	if accessLogEnabled() {
		aw := &accessWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			log.Printf("access %s %s -> %d, from %s, ua %q",
				r.Method, r.URL.EscapedPath(), aw.status, r.RemoteAddr, r.UserAgent())
		}()
		w = aw
	}

	h := &webdav.Handler{
		Prefix:     "/" + user + "/",
		FileSystem: s.davFSFor(acct),
		LockSystem: s.lockSystem(user),
		Logger: func(req *http.Request, err error) {
			if err != nil {
				// 只记录方法、路径与错误，绝不触碰 Authorization 头。
				log.Printf("webdav %s %s: %v", req.Method, req.URL.Path, err)
			}
		},
	}

	// 条件请求评估与写入在同一把按 (账号, 路径) 的锁内完成，防止丢更新。
	release, ok := s.guardPreconditions(w, r, acct, user)
	if !ok {
		return
	}
	defer release()

	h.ServeHTTP(w, r)
}

// davHandlerFor 供测试直接拿到某账号的处理器。
func (s *Server) davHandlerFor(acct account.Account) http.Handler {
	return &webdav.Handler{
		Prefix:     "/" + acct.User + "/",
		FileSystem: s.davFSFor(acct),
		LockSystem: s.lockSystem(acct.User),
	}
}
