package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// corsPreflight 构造一次 CORS 预检请求（OPTIONS + Origin + ACRM）。
func corsPreflight(s *Server, target, origin string) *httptest.ResponseRecorder {
	req := request(s, http.MethodOptions, target, "", "", nil, map[string]string{
		"Origin":                        origin,
		"Access-Control-Request-Method": "PROPFIND",
	})
	return req
}

func hasAccessControlHeader(h http.Header) string {
	for k := range h {
		if strings.HasPrefix(k, "Access-Control-") {
			return k
		}
	}
	return ""
}

func TestCORSPreflightDisabled(t *testing.T) {
	s, store, _ := newTestServer(t)
	_, _, _ = store.Create("app1", false, "")
	// 未配置 CORS_ORIGINS（cors 为 nil）。
	if s.cors != nil {
		t.Fatal("测试前置：默认应关闭 CORS")
	}

	rec := corsPreflight(s, "/app1/", "https://app.example.com")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未配置时 OPTIONS 应为 401，实际 %d", rec.Code)
	}
	if got := hasAccessControlHeader(rec.Header()); got != "" {
		t.Fatalf("未配置时不应回任何 CORS 头，实际有 %s", got)
	}
	if rec.Header().Get("Vary") != "" {
		t.Fatalf("未配置时不应回 Vary，实际 %q", rec.Header().Get("Vary"))
	}
}

func TestCORSPreflightAllowed(t *testing.T) {
	s, store, _ := newTestServer(t)
	_, _, _ = store.Create("app1", false, "")
	s.cors = ParseCORSOrigins("https://app.example.com")

	const origin = "https://app.example.com"
	rec := corsPreflight(s, "/app1/", origin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("命中的预检应为 204，实际 %d", rec.Code)
	}
	h := rec.Header()
	if got := h.Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("Allow-Origin 应回显来源 %q，实际 %q", origin, got)
	}
	if got := h.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials 应为 true，实际 %q", got)
	}
	if got := h.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PROPFIND") {
		t.Fatalf("Allow-Methods 应含 PROPFIND，实际 %q", got)
	}
	if got := h.Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") || !strings.Contains(got, "Depth") {
		t.Fatalf("Allow-Headers 应含 Authorization/Depth，实际 %q", got)
	}
	if got := h.Get("Access-Control-Expose-Headers"); !strings.Contains(got, "ETag") {
		t.Fatalf("Expose-Headers 应含 ETag，实际 %q", got)
	}
	if got := h.Get("Access-Control-Max-Age"); got != "600" {
		t.Fatalf("Max-Age 应为 600，实际 %q", got)
	}
	if got := h.Get("Vary"); !strings.Contains(got, "Origin") {
		t.Fatalf("应带 Vary: Origin，实际 %q", got)
	}
	if h.Get("WWW-Authenticate") != "" {
		t.Fatalf("命中的预检不应要认证，实际带了 WWW-Authenticate")
	}
}

func TestCORSPreflightOriginDenied(t *testing.T) {
	s, store, _ := newTestServer(t)
	_, _, _ = store.Create("app1", false, "")
	s.cors = ParseCORSOrigins("https://app.example.com")

	rec := corsPreflight(s, "/app1/", "https://evil.example.com")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("来源未命中应仍为 401，实际 %d", rec.Code)
	}
	if got := hasAccessControlHeader(rec.Header()); got != "" {
		t.Fatalf("来源未命中不应回 CORS 头，实际有 %s", got)
	}
	if rec.Header().Get("Vary") != "Origin" {
		t.Fatalf("启用 CORS 时未命中也应带 Vary: Origin，实际 %q", rec.Header().Get("Vary"))
	}
}

func TestCORSActualRequests(t *testing.T) {
	s, store, _ := newTestServer(t)
	_, pass, _ := store.Create("app1", false, "")
	s.cors = ParseCORSOrigins("https://app.example.com")
	const origin = "https://app.example.com"

	steps := []struct {
		name   string
		method string
		target string
		body   string
		want   int
	}{
		{"MKCOL", "MKCOL", "/app1/notes", "", http.StatusCreated},
		{"PUT", http.MethodPut, "/app1/notes/a.txt", "hello-cors", http.StatusCreated},
		{"PROPFIND", "PROPFIND", "/app1/", "", 207},
		{"GET", http.MethodGet, "/app1/notes/a.txt", "", http.StatusOK},
		{"DELETE", http.MethodDelete, "/app1/notes/a.txt", "", http.StatusNoContent},
	}
	for _, st := range steps {
		var body io.Reader
		if st.body != "" {
			body = strings.NewReader(st.body)
		}
		rec := request(s, st.method, st.target, "app1", pass, body, map[string]string{
			"Origin": origin,
			"Depth":  "0",
		})
		if rec.Code != st.want {
			t.Fatalf("%s 真实请求应为 %d，实际 %d", st.name, st.want, rec.Code)
		}
		h := rec.Header()
		if got := h.Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("%s 应回 Access-Control-Allow-Origin=%q，实际 %q", st.name, origin, got)
		}
		if got := h.Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Fatalf("%s 应回 Allow-Credentials，实际 %q", st.name, got)
		}
		if got := h.Get("Access-Control-Expose-Headers"); !strings.Contains(got, "ETag") {
			t.Fatalf("%s 应回 Expose-Headers，实际 %q", st.name, got)
		}
		if got := h.Get("Vary"); !strings.Contains(got, "Origin") {
			t.Fatalf("%s 应带 Vary: Origin，实际 %q", st.name, got)
		}
	}
}

func TestCORSOriginMatching(t *testing.T) {
	// 配置里故意带末尾斜杠与大小写差异。
	cfg := "https://app.example.com/, HTTPS://Second.Example.com:8443,http://127.0.0.1:18900"
	s, store, _ := newTestServer(t)
	_, _, _ = store.Create("app1", false, "")
	s.cors = ParseCORSOrigins(cfg)

	cases := []struct {
		origin string
		allow  bool
	}{
		{"https://app.example.com", true},
		{"https://app.example.com/", true},        // 末尾斜杠
		{"HTTPS://APP.EXAMPLE.COM", true},         // scheme/host 大小写
		{"https://second.example.com:8443", true}, // host 大小写 + 端口一致
		{"https://app.example.com:443", false},    // 显式端口视为不同来源
		{"https://second.example.com", false},     // 缺端口，不匹配
		{"http://127.0.0.1:18900", true},          // 显式端口一致
		{"https://evil.example.com", false},       // 不在白名单
		{"", false},                               // 无来源
	}
	for _, tc := range cases {
		rec := corsPreflight(s, "/app1/", tc.origin)
		gotAllow := rec.Code == http.StatusNoContent
		if gotAllow != tc.allow {
			t.Fatalf("来源 %q 期望 allow=%v，实际 code=%d", tc.origin, tc.allow, rec.Code)
		}
		if tc.allow {
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.origin {
				t.Fatalf("来源 %q 应原样回显，实际 %q", tc.origin, got)
			}
		}
	}
}
