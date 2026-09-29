package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/YLing2024/davbox/internal/account"
	"github.com/YLing2024/davbox/internal/auth"
)

// newSSOTestServer 构造一个 SSO 模式的测试服务器。
func newSSOTestServer(t *testing.T) (*Server, *account.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := account.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	signer := auth.NewSigner([]byte("0123456789abcdef0123456789abcdef"))
	return New(Config{
		DataDir:  dir,
		Store:    store,
		Signer:   signer,
		AuthMode: auth.ModeSSO,
	}), store, dir
}

func TestAdminModeEndpoint(t *testing.T) {
	// builtin 默认
	s, _, _ := newTestServer(t)
	rec := request(s, http.MethodGet, "/api/admin/mode", "", "", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("mode 应为 200，实际 %d", rec.Code)
	}
	var got struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "builtin" {
		t.Fatalf("默认模式应为 builtin，实际 %q", got.Mode)
	}

	// sso
	s2, _, _ := newSSOTestServer(t)
	rec = request(s2, http.MethodGet, "/api/admin/mode", "", "", nil, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "sso" {
		t.Fatalf("sso 模式应为 sso，实际 %q", got.Mode)
	}
}

func TestAdminSSOHeaderAccepted(t *testing.T) {
	s, _, _ := newSSOTestServer(t)
	rec := request(s, http.MethodGet, "/api/admin/accounts", "", "", nil, map[string]string{"X-Auth-User": "linden"})
	if rec.Code != http.StatusOK {
		t.Fatalf("带 X-Auth-User 应为 200，实际 %d", rec.Code)
	}
}

func TestAdminSSOMissingHeaderIs401JSON(t *testing.T) {
	s, _, _ := newSSOTestServer(t)
	rec := request(s, http.MethodGet, "/api/admin/accounts", "", "", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("缺 header 应为 401，实际 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("401 应为 JSON，实际 Content-Type=%q", ct)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("401 不应重定向，实际 Location=%q", loc)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应体不是 JSON: %v (%q)", err, rec.Body.String())
	}
	if body["error"] == "" {
		t.Fatalf("JSON 应含 error 字段，实际 %q", rec.Body.String())
	}
}

func TestAdminSSOEmptyHeaderIs401(t *testing.T) {
	s, _, _ := newSSOTestServer(t)
	rec := request(s, http.MethodGet, "/api/admin/accounts", "", "", nil, map[string]string{"X-Auth-User": "   "})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("空白 X-Auth-User 应为 401，实际 %d", rec.Code)
	}
}

func TestAdminBuiltinIgnoresAuthUserHeader(t *testing.T) {
	// builtin 模式下伪造 X-Auth-User 不能绕过口令认证。
	s, _, _ := newTestServer(t)
	rec := request(s, http.MethodGet, "/api/admin/accounts", "", "", nil, map[string]string{"X-Auth-User": "linden"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("builtin 模式下伪造 header 应为 401，实际 %d", rec.Code)
	}
}

func TestAdminSSOLoginLogoutNotFound(t *testing.T) {
	s, _, _ := newSSOTestServer(t)
	rec := request(s, http.MethodPost, "/api/admin/login", "", "", strings.NewReader(`{"password":"x"}`), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sso 模式 login 应为 404，实际 %d", rec.Code)
	}
	rec = request(s, http.MethodPost, "/api/admin/logout", "", "", nil, map[string]string{"X-Auth-User": "linden"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("sso 模式 logout 应为 404，实际 %d", rec.Code)
	}
}

func TestAdminSSOAccountCRUD(t *testing.T) {
	s, _, _ := newSSOTestServer(t)
	h := map[string]string{"X-Auth-User": "linden"}

	rec := request(s, http.MethodPost, "/api/admin/accounts", "", "", strings.NewReader(`{"user":"ssoapp","readonly":false}`), h)
	if rec.Code != http.StatusCreated {
		t.Fatalf("sso 建账号应为 201，实际 %d", rec.Code)
	}
	rec = request(s, http.MethodGet, "/api/admin/accounts", "", "", nil, h)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ssoapp") {
		t.Fatalf("sso 列表应含新账号，实际 %d %q", rec.Code, rec.Body.String())
	}
	rec = request(s, http.MethodDelete, "/api/admin/accounts/ssoapp", "", "", nil, h)
	if rec.Code != http.StatusOK {
		t.Fatalf("sso 删除应为 200，实际 %d", rec.Code)
	}
}

// SSO 模式不得影响 WebDAV 数据面与 client 接口。
func TestSSOModeDoesNotAffectDataPlane(t *testing.T) {
	s, store, _ := newSSOTestServer(t)
	_, pass, _ := store.Create("app1", false, "")

	rec := request(s, "PROPFIND", "/app1/", "app1", pass, nil, map[string]string{"Depth": "0"})
	if rec.Code != 207 {
		t.Fatalf("SSO 下 WebDAV 认证后应为 207，实际 %d", rec.Code)
	}
	rec = request(s, "PROPFIND", "/app1/", "", "", nil, map[string]string{"Depth": "0"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("SSO 下 WebDAV 匿名应为 401，实际 %d", rec.Code)
	}

	rec = request(s, http.MethodPost, "/api/client/login", "", "", strings.NewReader(`{"user":"app1","pass":"`+pass+`"}`), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("SSO 下 client 登录应为 200，实际 %d", rec.Code)
	}
}

// 静态资源在 SSO 模式下仍可访问，不受管理端鉴权影响。
func TestSSOModeStaticUnaffected(t *testing.T) {
	s, _, _ := newSSOTestServer(t)
	rec := request(s, http.MethodGet, "/admin", "", "", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/admin 页面应为 200，实际 %d", rec.Code)
	}
}

// 保证 builtin 回归：现有显式流程仍可用（含 cookie 登录）。
func TestBuiltinModeStillWorks(t *testing.T) {
	s, _, _ := newTestServer(t)
	rec := request(s, http.MethodPost, "/api/admin/login", "", "", strings.NewReader(`{"password":"`+testAdminPass+`"}`), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("builtin 登录应为 200，实际 %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	rec = doWithCookie(s, http.MethodGet, "/api/admin/accounts", cookies, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("builtin 带 cookie 列表应为 200，实际 %d", rec.Code)
	}
}
