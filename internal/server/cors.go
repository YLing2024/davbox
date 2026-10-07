package server

import (
	"net/http"
	"strings"

	"github.com/YLing2024/davbox/internal/settings"
)

// CORS 固定回给浏览器的响应头取值。命中白名单时才设置。
const (
	corsAllowMethods  = "OPTIONS, GET, HEAD, PUT, DELETE, MKCOL, PROPFIND, PROPPATCH, MOVE, COPY"
	corsAllowHeaders  = "Authorization, Content-Type, Depth, Destination, Overwrite, If, If-Match, If-None-Match, Range"
	corsExposeHeaders = "DAV, ETag, Content-Length, Content-Type, Last-Modified"
	corsMaxAge        = "600"
)

// CORS 是一份精确来源白名单。零值或 nil 表示完全关闭（一个头都不回）。
type CORS struct {
	origins map[string]struct{}
}

// ParseCORSOrigins 解析逗号分隔的来源白名单。空白项与非法项忽略；
// 匹配忽略来源末尾的斜杠，scheme/host 大小写不敏感，端口精确。
// 结果为空时返回 nil，表示关闭。
func ParseCORSOrigins(v string) *CORS {
	c := &CORS{origins: map[string]struct{}{}}
	for _, part := range strings.Split(v, ",") {
		if norm, ok := settings.NormalizeOrigin(part); ok {
			c.origins[norm] = struct{}{}
		}
	}
	if len(c.origins) == 0 {
		return nil
	}
	return c
}

// CORSFromOrigins 用一组已规整的来源构造白名单；为空时返回 nil 表示关闭。
func CORSFromOrigins(origins []string) *CORS {
	c := &CORS{origins: map[string]struct{}{}}
	for _, o := range origins {
		if norm, ok := settings.NormalizeOrigin(o); ok {
			c.origins[norm] = struct{}{}
		}
	}
	if len(c.origins) == 0 {
		return nil
	}
	return c
}

// enabled 报告是否配置了至少一个来源。
func (c *CORS) enabled() bool { return c != nil && len(c.origins) > 0 }

// allowed 报告请求的 Origin 是否在白名单内（精确匹配）。
func (c *CORS) allowed(origin string) bool {
	if !c.enabled() || origin == "" {
		return false
	}
	norm, ok := settings.NormalizeOrigin(origin)
	if !ok {
		return false
	}
	_, hit := c.origins[norm]
	return hit
}

// apply 在 CORS 已启用时补上 Vary: Origin；来源命中白名单时再补全 CORS 响应头。
// 返回来源是否命中。未启用时不改动任何响应头。
func (c *CORS) apply(w http.ResponseWriter, r *http.Request) bool {
	if !c.enabled() {
		return false
	}
	// 命中与否都要有 Vary: Origin，避免共享缓存把某个来源的响应串给另一个来源。
	w.Header().Add("Vary", "Origin")

	origin := r.Header.Get("Origin")
	if !c.allowed(origin) {
		return false
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	h.Set("Access-Control-Allow-Methods", corsAllowMethods)
	h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
	h.Set("Access-Control-Expose-Headers", corsExposeHeaders)
	h.Set("Access-Control-Max-Age", corsMaxAge)
	return true
}

// isPreflight 判断是否为 CORS 预检：OPTIONS + Origin + Access-Control-Request-Method。
func isPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions &&
		r.Header.Get("Origin") != "" &&
		r.Header.Get("Access-Control-Request-Method") != ""
}
