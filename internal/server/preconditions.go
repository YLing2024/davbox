package server

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/webdav"

	"github.com/YLing2024/davbox/internal/account"
)

// preconditionMode 是 WEBDAV_PRECONDITIONS 的取值。
type preconditionMode int

const (
	modeEnforce preconditionMode = iota // 默认：真正拒绝
	modeLog                             // 只评估并记日志，不拒绝
	modeOff                             // 完全跳过评估
)

// parsePreconditionMode 解析开关；第二返回值表示取值是否合法。
func parsePreconditionMode(raw string) (preconditionMode, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "enforce":
		return modeEnforce, true
	case "log":
		return modeLog, true
	case "off":
		return modeOff, true
	default:
		return modeEnforce, false
	}
}

// methodUsesIfMatch 列出需要评估 If-Match 的方法。
func methodUsesIfMatch(method string) bool {
	switch method {
	case http.MethodPut, http.MethodDelete, "MOVE", "COPY", "PROPPATCH":
		return true
	}
	return false
}

// methodUsesIfNoneMatch 列出需要评估 If-None-Match 的方法。
func methodUsesIfNoneMatch(method string) bool {
	switch method {
	case http.MethodPut, "MKCOL":
		return true
	}
	return false
}

// guardPreconditions 在库处理写请求之前评估条件头并持锁。
//
// 返回值 release 由调用方在 h.ServeHTTP 返回后调用；ok 为 false 表示已写出 412。
// off 模式直接放行；log 模式只评估记日志、不拒绝也不持锁。
func (s *Server) guardPreconditions(w http.ResponseWriter, r *http.Request, acct account.Account, user string) (release func(), ok bool) {
	if s.precond == modeOff {
		return func() {}, true
	}
	imApplicable := methodUsesIfMatch(r.Method) && r.Header.Get("If-Match") != ""
	inmApplicable := methodUsesIfNoneMatch(r.Method) && r.Header.Get("If-None-Match") != ""
	if !imApplicable && !inmApplicable {
		return func() {}, true
	}

	rel, fsPath := preconditionTarget(acct, user, r.URL.Path)

	if s.precond == modeLog {
		if reject, reason := s.evaluatePreconditions(r, fsPath); reject {
			s.logf("precondition(dry-run) 412 %s %s: %s", r.Method, r.URL.Path, reason)
		}
		return func() {}, true
	}

	// enforce：读取当前 ETag、评估、执行写入必须在同一把路径锁内完成。
	keys := []string{lockKey(user, rel)}
	if r.Method == "MOVE" || r.Method == "COPY" {
		if dstRel, ok := destinationRel(r, user); ok {
			keys = append(keys, lockKey(user, dstRel))
		}
	}
	release = s.pathLocks.lockAll(keys)
	if reject, reason := s.evaluatePreconditions(r, fsPath); reject {
		s.logf("webdav 412 %s %s: %s", r.Method, r.URL.Path, reason)
		writePreconditionFailed(w)
		release()
		return func() {}, false
	}
	return release, true
}

// preconditionTarget 把请求路径换算成相对账号 root 的路径与真实文件系统路径。
func preconditionTarget(acct account.Account, user, urlPath string) (rel, fsPath string) {
	rel = strings.TrimPrefix(urlPath, "/"+user+"/")
	fsPath = filepath.Join(acct.Root, filepath.FromSlash(rel))
	return rel, fsPath
}

// destinationRel 解析 MOVE/COPY 的 Destination，返回相对同账号 root 的路径。
func destinationRel(r *http.Request, user string) (string, bool) {
	hdr := r.Header.Get("Destination")
	if hdr == "" {
		return "", false
	}
	u, err := url.Parse(hdr)
	if err != nil {
		return "", false
	}
	prefix := "/" + user + "/"
	if !strings.HasPrefix(u.Path, prefix) {
		return "", false
	}
	return strings.TrimPrefix(u.Path, prefix), true
}

// evaluatePreconditions 按 RFC 9110 评估 If-Match / If-None-Match。
//
// 两个头同时出现时先评估 If-Match，失败即返回；语法非法的头被忽略并记一条提示。
func (s *Server) evaluatePreconditions(r *http.Request, fsPath string) (reject bool, reason string) {
	var invalid []string

	if methodUsesIfMatch(r.Method) {
		if raw := r.Header.Get("If-Match"); raw != "" {
			tags, wildcard, ok := parseETagList(raw)
			if !ok {
				invalid = append(invalid, "If-Match")
			} else {
				cur, exists := statETag(fsPath)
				matched := wildcard && exists
				if !wildcard {
					matched = exists && anyStrongMatch(cur, tags)
				}
				if !matched {
					s.logInvalid(invalid)
					return true, fmt.Sprintf("If-Match=%s 当前=%s", raw, displayETag(cur, exists))
				}
			}
		}
	}

	if methodUsesIfNoneMatch(r.Method) {
		if raw := r.Header.Get("If-None-Match"); raw != "" {
			tags, wildcard, ok := parseETagList(raw)
			if !ok {
				invalid = append(invalid, "If-None-Match")
			} else {
				cur, exists := statETag(fsPath)
				failed := wildcard && exists
				if !wildcard {
					failed = exists && anyWeakMatch(cur, tags)
				}
				if failed {
					s.logInvalid(invalid)
					return true, fmt.Sprintf("If-None-Match=%s 当前=%s", raw, displayETag(cur, exists))
				}
			}
		}
	}

	s.logInvalid(invalid)
	return false, ""
}

// statETag 返回目标路径当前的强 ETag 与是否存在。
func statETag(fsPath string) (string, bool) {
	fi, err := os.Stat(fsPath)
	if err != nil {
		return "", false
	}
	return strongETag(fi), true
}

func displayETag(etag string, exists bool) string {
	if !exists {
		return "(无)"
	}
	return etag
}

// anyStrongMatch 强比较：弱标签永不匹配，且只比较不透明标签。
func anyStrongMatch(cur string, tags []string) bool {
	for _, t := range tags {
		if isWeakTag(t) {
			continue
		}
		if etagOpaque(t) == etagOpaque(cur) {
			return true
		}
	}
	return false
}

// anyWeakMatch 弱比较：忽略 W/ 前缀后比较不透明标签。
func anyWeakMatch(cur string, tags []string) bool {
	for _, t := range tags {
		if etagOpaque(t) == etagOpaque(cur) {
			return true
		}
	}
	return false
}

func isWeakTag(tag string) bool { return strings.HasPrefix(tag, "W/") }

func etagOpaque(tag string) string {
	if isWeakTag(tag) {
		return tag[2:]
	}
	return tag
}

// parseETagList 解析实体标签列表。返回 (标签, 是否为 `*`, 语法是否合法)。
// 非法（空列表、缺引号、只有逗号、未闭合等）时第三返回值为 false。
func parseETagList(raw string) ([]string, bool, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, false, false
	}
	if s == "*" {
		return nil, true, true
	}
	var tags []string
	i, n := 0, len(s)
	for i < n {
		for i < n && (s[i] == ' ' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= n {
			break
		}
		start := i
		if strings.HasPrefix(s[i:], "W/") {
			i += 2
		}
		if i >= n || s[i] != '"' {
			return nil, false, false
		}
		i++ // 开引号
		for i < n && s[i] != '"' {
			if s[i] < 0x21 || s[i] > 0x7e { // etagc 不允许空格/控制符/DQUOTE
				return nil, false, false
			}
			i++
		}
		if i >= n {
			return nil, false, false // 未闭合
		}
		i++ // 闭引号
		tags = append(tags, s[start:i])
		if i < n && s[i] != ',' && s[i] != ' ' && s[i] != '\t' {
			return nil, false, false
		}
	}
	if len(tags) == 0 {
		return nil, false, false
	}
	return tags, false, true
}

// logInvalid 对语法非法而被忽略的头记一条提示。
func (s *Server) logInvalid(headers []string) {
	for _, h := range headers {
		s.logf("webdav %s 头语法非法，已忽略", h)
	}
}

func writePreconditionFailed(w http.ResponseWriter) {
	w.WriteHeader(http.StatusPreconditionFailed)
	_, _ = w.Write([]byte(webdav.StatusText(http.StatusPreconditionFailed)))
}

// keyedMutex 是按 key 串行化的互斥锁，用于把「读 ETag → 评估 → 写入」串起来。
// 引用计数保证锁表不会随访问过的路径无限增长。
type keyedMutex struct {
	mu sync.Mutex
	m  map[string]*keyedEntry
}

type keyedEntry struct {
	mu   sync.Mutex
	refs int
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{m: map[string]*keyedEntry{}}
}

// lockAll 按字典序依次获取所有 key 的锁，返回一次性释放函数（逆序释放）。
func (k *keyedMutex) lockAll(keys []string) func() {
	uniq := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		uniq = append(uniq, key)
	}
	sort.Strings(uniq)

	heldKeys := make([]string, 0, len(uniq))
	held := make([]*keyedEntry, 0, len(uniq))
	for _, key := range uniq {
		heldKeys = append(heldKeys, key)
		held = append(held, k.acquire(key))
	}
	return func() {
		for i := len(held) - 1; i >= 0; i-- {
			k.release(heldKeys[i], held[i])
		}
	}
}

func (k *keyedMutex) acquire(key string) *keyedEntry {
	k.mu.Lock()
	e := k.m[key]
	if e == nil {
		e = &keyedEntry{}
		k.m[key] = e
	}
	e.refs++
	k.mu.Unlock()
	e.mu.Lock()
	return e
}

func (k *keyedMutex) release(key string, e *keyedEntry) {
	e.mu.Unlock()
	k.mu.Lock()
	e.refs--
	if e.refs == 0 {
		delete(k.m, key)
	}
	k.mu.Unlock()
}

// lockKey 生成账号内某资源路径的锁 key。
func lockKey(user, rel string) string {
	return user + "\x00" + rel
}
