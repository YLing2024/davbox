package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestClientMe(t *testing.T) {
	s, store, _ := newTestServer(t)
	_, pass, _ := store.Create("app1", false, "")

	// 未登录 -> 401
	rec := request(s, http.MethodGet, "/api/client/me", "", "", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录 /me 应为 401，实际 %d", rec.Code)
	}

	rec = request(s, http.MethodPost, "/api/client/login", "", "", strings.NewReader(`{"user":"app1","pass":"`+pass+`"}`), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("登录应为 200，实际 %d", rec.Code)
	}
	cookies := rec.Result().Cookies()

	rec = doWithCookie(s, http.MethodGet, "/api/client/me", cookies, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/me 应为 200，实际 %d", rec.Code)
	}
	var me struct {
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.User != "app1" {
		t.Fatalf("/me 应返回账号名，实际 %q", me.User)
	}
	if me.Pass != "" {
		t.Fatal("/me 不应返回口令")
	}
}
