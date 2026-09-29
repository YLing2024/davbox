// Package server 负责路由装配：WebDAV、admin API、client API 与静态资源。
package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/webdav"

	"github.com/YLing2024/davbox/internal/account"
	"github.com/YLing2024/davbox/internal/auth"
)

// Config 是装配服务器所需的依赖。
type Config struct {
	DataDir   string
	Store     *account.Store
	AdminHash []byte
	Signer    *auth.Signer
	// AuthMode 是管理端认证模式；零值按 builtin 处理。
	AuthMode auth.Mode
}

// Server 持有全部运行期状态。
type Server struct {
	dataDir   string
	store     *account.Store
	adminHash []byte
	signer    *auth.Signer
	authMode  auth.Mode

	davMu sync.Mutex
	locks map[string]webdav.LockSystem

	usage *usageCache
}

// New 构造服务器。
func New(cfg Config) *Server {
	return &Server{
		dataDir:   cfg.DataDir,
		store:     cfg.Store,
		adminHash: cfg.AdminHash,
		signer:    cfg.Signer,
		authMode:  cfg.AuthMode.Normalize(),
		locks:     map[string]webdav.LockSystem{},
		usage:     newUsageCache(30 * time.Second),
	}
}

// Handler 返回顶层 http.Handler。
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.route)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	decoded := r.URL.Path
	if unsafePath(r.URL.EscapedPath(), decoded) {
		writeError(w, http.StatusBadRequest, "请求路径不合法")
		return
	}

	switch {
	case decoded == "/api/admin" || strings.HasPrefix(decoded, "/api/admin/"):
		s.handleAdmin(w, r)
	case decoded == "/api/client" || strings.HasPrefix(decoded, "/api/client/"):
		s.handleClient(w, r)
	case decoded == "/" || decoded == "/admin" || decoded == "/admin/":
		s.serveIndex(w, r)
	case strings.HasPrefix(decoded, "/assets/"):
		s.serveStatic(w, r)
	default:
		if seg := firstSegment(decoded); seg != "" {
			if _, ok := s.store.Get(seg); ok {
				s.handleDAV(w, r, seg)
				return
			}
			// 账号不存在时返回与未认证一致的 401，避免泄漏账号是否存在。
			writeAuthChallenge(w)
			return
		}
		writeError(w, http.StatusNotFound, "未找到")
	}
}

func firstSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

// unsafePath 拒绝含 `..`、编码后的 `%2e`、反斜杠、空字节的路径。
func unsafePath(escaped, decoded string) bool {
	low := strings.ToLower(escaped)
	for _, bad := range []string{"%2e", "%5c", "%00"} {
		if strings.Contains(low, bad) {
			return true
		}
	}
	if strings.Contains(decoded, "..") || strings.Contains(decoded, `\`) || strings.ContainsRune(decoded, 0) {
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(v)
}

type usageEntry struct {
	bytes int64
	files int64
	exp   time.Time
}

type usageCache struct {
	mu  sync.Mutex
	ttl time.Duration
	m   map[string]usageEntry
}

func newUsageCache(ttl time.Duration) *usageCache {
	return &usageCache{ttl: ttl, m: map[string]usageEntry{}}
}

func (c *usageCache) stat(key, root string, compute func(string) (int64, int64, error)) (int64, int64) {
	c.mu.Lock()
	if e, ok := c.m[key]; ok && time.Now().Before(e.exp) {
		c.mu.Unlock()
		return e.bytes, e.files
	}
	c.mu.Unlock()

	bytes, files, _ := compute(root)

	c.mu.Lock()
	c.m[key] = usageEntry{bytes: bytes, files: files, exp: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return bytes, files
}
