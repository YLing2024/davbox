package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/YLing2024/davbox/internal/account"
	"github.com/YLing2024/davbox/internal/auth"
	"github.com/YLing2024/davbox/internal/settings"
)

// newSettingsTestServer 构造一个带 settings 存储的服务器，环境变量作为初始默认。
func newSettingsTestServer(t *testing.T, envOrigins string) (*Server, *account.Store, *settings.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := account.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(testAdminPass), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	sett, err := settings.Open(dir, envOrigins)
	if err != nil {
		t.Fatal(err)
	}
	s := New(Config{
		DataDir:   dir,
		Store:     store,
		AdminHash: hash,
		Signer:    auth.NewSigner([]byte("0123456789abcdef0123456789abcdef")),
		Settings:  sett,
	})
	return s, store, sett, dir
}

func adminCookies(t *testing.T, s *Server) []*http.Cookie {
	t.Helper()
	rec := request(s, http.MethodPost, "/api/admin/login", "", "", strings.NewReader(`{"password":"`+testAdminPass+`"}`), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员登录应为 200，实际 %d", rec.Code)
	}
	return rec.Result().Cookies()
}

func TestAdminSettingsGetPutRoundTrip(t *testing.T) {
	s, _, _, dir := newSettingsTestServer(t, "")
	cookies := adminCookies(t, s)

	rec := doWithCookie(s, http.MethodGet, "/api/admin/settings", cookies, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET settings 应为 200，实际 %d", rec.Code)
	}
	var initial settings.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.CORSOrigins == nil || len(initial.CORSOrigins) != 0 {
		t.Fatalf("初始应为空数组，实际 %v", initial.CORSOrigins)
	}

	rec = doWithCookie(s, http.MethodPut, "/api/admin/settings", cookies,
		strings.NewReader(`{"corsOrigins":["HTTPS://App.Example.com/","http://127.0.0.1:18900"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings 应为 200，实际 %d（%s）", rec.Code, rec.Body.String())
	}
	var saved settings.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	want := []string{"https://app.example.com", "http://127.0.0.1:18900"}
	if strings.Join(saved.CORSOrigins, ",") != strings.Join(want, ",") {
		t.Fatalf("保存响应应为 %v，实际 %v", want, saved.CORSOrigins)
	}

	// 重新 GET 可见（保存后刷新）。
	rec = doWithCookie(s, http.MethodGet, "/api/admin/settings", cookies, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if strings.Join(saved.CORSOrigins, ",") != strings.Join(want, ",") {
		t.Fatalf("GET 应返回保存值 %v，实际 %v", want, saved.CORSOrigins)
	}

	// 落盘到 dataDir/settings.json。
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json 应存在: %v", err)
	}
	if !strings.Contains(string(raw), "https://app.example.com") {
		t.Fatalf("settings.json 应写入来源，实际 %s", string(raw))
	}
}

func TestAdminSettingsAliasPath(t *testing.T) {
	s, _, _, _ := newSettingsTestServer(t, "")
	cookies := adminCookies(t, s)

	// 需求书里的 /admin/api/settings 作为别名同样可用。
	rec := doWithCookie(s, http.MethodGet, "/admin/api/settings", cookies, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("别名 GET 应为 200，实际 %d", rec.Code)
	}
	rec = doWithCookie(s, http.MethodPut, "/admin/api/settings", cookies,
		strings.NewReader(`{"corsOrigins":["https://alias.example.com"]}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("别名 PUT 应为 200，实际 %d", rec.Code)
	}
	var got settings.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.CORSOrigins) != 1 || got.CORSOrigins[0] != "https://alias.example.com" {
		t.Fatalf("别名保存值不对: %v", got.CORSOrigins)
	}
}

func TestAdminSettingsInvalidValueReturns400WithLine(t *testing.T) {
	s, _, _, dir := newSettingsTestServer(t, "")
	cookies := adminCookies(t, s)

	rec := doWithCookie(s, http.MethodPut, "/api/admin/settings", cookies,
		strings.NewReader(`{"corsOrigins":["https://ok.example.com","ftp://bad.example.com"]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法值应为 400，实际 %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["error"], "第 2 行") {
		t.Fatalf("错误应指出第 2 行，实际 %q", body["error"])
	}
	// 非法保存不得落盘。
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("校验失败不应写入 settings.json: %v", err)
	}
}

func TestAdminSettingsBadRequests(t *testing.T) {
	s, _, _, _ := newSettingsTestServer(t, "")
	cookies := adminCookies(t, s)

	// 缺少字段。
	rec := doWithCookie(s, http.MethodPut, "/api/admin/settings", cookies, strings.NewReader(`{}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺少字段应为 400，实际 %d", rec.Code)
	}
	// 请求体不是 JSON。
	rec = doWithCookie(s, http.MethodPut, "/api/admin/settings", cookies, strings.NewReader(`not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应为 400，实际 %d", rec.Code)
	}
	// 方法不允许。
	rec = doWithCookie(s, http.MethodDelete, "/api/admin/settings", cookies, nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE 应为 405，实际 %d", rec.Code)
	}
}

func TestAdminSettingsRequiresAuth(t *testing.T) {
	s, store, _, dir := newSettingsTestServer(t, "")
	_, pass, _ := store.Create("app1", false, "")

	// 无管理端身份：GET/PUT 都 401，且不落盘。
	rec := request(s, http.MethodGet, "/api/admin/settings", "", "", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录 GET 应为 401，实际 %d", rec.Code)
	}
	rec = request(s, http.MethodPut, "/api/admin/settings", "", "", strings.NewReader(`{"corsOrigins":["https://x.example.com"]}`), nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录 PUT 应为 401，实际 %d", rec.Code)
	}
	// 即使带应用账号 Basic 也不能写管理设置。
	rec = request(s, http.MethodPut, "/api/admin/settings", "app1", pass, strings.NewReader(`{"corsOrigins":["https://x.example.com"]}`), nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("WebDAV 凭据不应能写设置，实际 %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("未认证请求不应创建 settings.json: %v", err)
	}
}

func TestSettingsUnderSSO(t *testing.T) {
	dir := t.TempDir()
	store, err := account.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	sett, err := settings.Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	s := New(Config{
		DataDir:  dir,
		Store:    store,
		Signer:   auth.NewSigner([]byte("0123456789abcdef0123456789abcdef")),
		AuthMode: auth.ModeSSO,
		Settings: sett,
	})
	h := map[string]string{"X-Auth-User": "linden"}

	rec := request(s, http.MethodPut, "/api/admin/settings", "", "", strings.NewReader(`{"corsOrigins":["https://sso.example.com"]}`), h)
	if rec.Code != http.StatusOK {
		t.Fatalf("SSO 下 PUT 应为 200，实际 %d", rec.Code)
	}
	rec = request(s, http.MethodGet, "/api/admin/settings", "", "", nil, h)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sso.example.com") {
		t.Fatalf("SSO 下 GET 应返回保存值，实际 %d %q", rec.Code, rec.Body.String())
	}
	rec = request(s, http.MethodGet, "/api/admin/settings", "", "", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("SSO 下缺 header 应为 401，实际 %d", rec.Code)
	}
}

func TestCORSRuntimeHotUpdate(t *testing.T) {
	s, store, _, _ := newSettingsTestServer(t, "")
	_, _, _ = store.Create("app1", false, "")
	cookies := adminCookies(t, s)

	const a = "https://a.example.com"
	const b = "https://b.example.com"

	// 初始关闭：预检 401，无 CORS 头。
	rec := corsPreflight(s, "/app1/", a)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("初始关闭时预检应为 401，实际 %d", rec.Code)
	}

	// 配 A。
	if r := doWithCookie(s, http.MethodPut, "/api/admin/settings", cookies,
		strings.NewReader(`{"corsOrigins":["`+a+`"]}`)); r.Code != http.StatusOK {
		t.Fatalf("配 A 应 200，实际 %d", r.Code)
	}
	if rec := corsPreflight(s, "/app1/", a); rec.Code != http.StatusNoContent {
		t.Fatalf("配 A 后预检 A 应为 204，实际 %d", rec.Code)
	}

	// 不重启改成 B。
	if r := doWithCookie(s, http.MethodPut, "/api/admin/settings", cookies,
		strings.NewReader(`{"corsOrigins":["`+b+`"]}`)); r.Code != http.StatusOK {
		t.Fatalf("改 B 应 200，实际 %d", r.Code)
	}
	rec = corsPreflight(s, "/app1/", a)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("改成 B 后预检 A 应为 401，实际 %d", rec.Code)
	}
	if got := hasAccessControlHeader(rec.Header()); got != "" {
		t.Fatalf("改成 B 后预检 A 不应有 CORS 头，实际有 %s", got)
	}
	if rec := corsPreflight(s, "/app1/", b); rec.Code != http.StatusNoContent {
		t.Fatalf("改成 B 后预检 B 应为 204，实际 %d", rec.Code)
	}
}

func TestSettingsEnvDefaultUntilSaved(t *testing.T) {
	const envOrigin = "https://env.example.com"
	s, store, _, _ := newSettingsTestServer(t, envOrigin)
	_, _, _ = store.Create("app1", false, "")
	cookies := adminCookies(t, s)

	// settings.json 缺失：环境变量作为初始默认，直接生效。
	rec := corsPreflight(s, "/app1/", envOrigin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("环境变量默认来源应命中预检 204，实际 %d", rec.Code)
	}
	rec = doWithCookie(s, http.MethodGet, "/api/admin/settings", cookies, nil)
	var got settings.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.CORSOrigins) != 1 || got.CORSOrigins[0] != envOrigin {
		t.Fatalf("GET 应显示环境变量默认值，实际 %v", got.CORSOrigins)
	}

	// 保存覆盖后，环境变量不再生效。
	const fileOrigin = "https://file.example.com"
	if r := doWithCookie(s, http.MethodPut, "/api/admin/settings", cookies,
		strings.NewReader(`{"corsOrigins":["`+fileOrigin+`"]}`)); r.Code != http.StatusOK {
		t.Fatalf("保存文件值应 200，实际 %d", r.Code)
	}
	if rec := corsPreflight(s, "/app1/", envOrigin); rec.Code != http.StatusUnauthorized {
		t.Fatalf("保存覆盖后环境变量来源应为 401，实际 %d", rec.Code)
	}
	if rec := corsPreflight(s, "/app1/", fileOrigin); rec.Code != http.StatusNoContent {
		t.Fatalf("保存后文件来源应为 204，实际 %d", rec.Code)
	}

	// 保存空列表即关闭跨域（仍无需重启）。
	if r := doWithCookie(s, http.MethodPut, "/api/admin/settings", cookies,
		strings.NewReader(`{"corsOrigins":[]}`)); r.Code != http.StatusOK {
		t.Fatalf("保存空列表应 200，实际 %d", r.Code)
	}
	if rec := corsPreflight(s, "/app1/", fileOrigin); rec.Code != http.StatusUnauthorized {
		t.Fatalf("清空后应为 401，实际 %d", rec.Code)
	}
}
