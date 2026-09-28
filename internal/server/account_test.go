package server

import (
	"net/http"
	"testing"
)

func TestDAVUnknownAccountChallenge(t *testing.T) {
	s, _, _ := newTestServer(t)

	// 未带任何凭据的未知账号，与“账号存在但未认证”一致：401 + WWW-Authenticate。
	rec := request(s, "PROPFIND", "/ghost/", "", "", nil, map[string]string{"Depth": "0"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未知账号应为 401，实际 %d", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != `Basic realm="davbox"` {
		t.Fatalf("未知账号应带 WWW-Authenticate，实际 %q", got)
	}

	// 带任意凭据结果一样，不泄漏账号是否存在。
	rec = request(s, "PROPFIND", "/ghost/", "ghost", "whatever", nil, map[string]string{"Depth": "0"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未知账号（带凭据）应为 401，实际 %d", rec.Code)
	}
}
