package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	testDAVUser = "app1"
	testDAVPath = "/app1/c.txt"
)

// newDAVTest 建一个默认（enforce）服务器并创建一个可写账号。
func newDAVTest(t *testing.T) (*Server, string) {
	t.Helper()
	s, store, _ := newTestServer(t)
	_, pass, _ := store.Create(testDAVUser, false, "")
	return s, pass
}

func davPut(s *Server, pass, target, content string, headers map[string]string) *httptest.ResponseRecorder {
	return request(s, http.MethodPut, target, testDAVUser, pass, strings.NewReader(content), headers)
}

func mustPut(t *testing.T, s *Server, pass, target, content string) {
	t.Helper()
	if rec := davPut(s, pass, target, content, nil); rec.Code != http.StatusCreated {
		t.Fatalf("预置 PUT %s 应为 201，实际 %d", target, rec.Code)
	}
}

func getBody(t *testing.T, s *Server, pass, target string) (string, string) {
	t.Helper()
	rec := request(s, http.MethodGet, target, testDAVUser, pass, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s 应为 200，实际 %d", target, rec.Code)
	}
	return rec.Body.String(), rec.Header().Get("ETag")
}

var getetagRe = regexp.MustCompile(`getetag[^>]*>([^<]*)<`)

func propfindETag(t *testing.T, s *Server, pass, target string) string {
	t.Helper()
	body := `<?xml version="1.0" encoding="utf-8"?>` +
		`<D:propfind xmlns:D="DAV:"><D:prop><D:getetag/></D:prop></D:propfind>`
	rec := request(s, "PROPFIND", target, testDAVUser, pass, strings.NewReader(body),
		map[string]string{"Depth": "0", "Content-Type": "application/xml"})
	if rec.Code != 207 {
		t.Fatalf("PROPFIND %s 应为 207，实际 %d", target, rec.Code)
	}
	m := getetagRe.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("PROPFIND 响应缺少 getetag: %s", rec.Body.String())
	}
	return m[1]
}

func captureLogs(s *Server) *bytes.Buffer {
	buf := &bytes.Buffer{}
	var mu sync.Mutex
	s.logf = func(format string, v ...any) {
		mu.Lock()
		fmt.Fprintf(buf, format+"\n", v...)
		mu.Unlock()
	}
	return buf
}

func TestPreconditionParseMode(t *testing.T) {
	cases := []struct {
		in   string
		want preconditionMode
		ok   bool
	}{
		{"", modeEnforce, true},
		{"enforce", modeEnforce, true},
		{"log", modeLog, true},
		{"off", modeOff, true},
		{"bogus", modeEnforce, false},
		{"ENFORCE", modeEnforce, true},
	}
	for _, c := range cases {
		got, ok := parsePreconditionMode(c.in)
		if got != c.want || ok != c.ok {
			t.Fatalf("parsePreconditionMode(%q) = (%v,%v)，期望 (%v,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// 用例 1：If-Match 命中当前 ETag → 写入成功且内容更新。
func TestIfMatchHit(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "v1")
	_, etag := getBody(t, s, pass, testDAVPath)

	rec := davPut(s, pass, testDAVPath, "v2", map[string]string{"If-Match": etag})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("If-Match 命中应为 201/204，实际 %d", rec.Code)
	}
	body, _ := getBody(t, s, pass, testDAVPath)
	if body != "v2" {
		t.Fatalf("内容应更新为 v2，实际 %q", body)
	}
}

// 用例 2：If-Match 用过期的 ETag → 412 且文件字节未变。
func TestIfMatchStale(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "v1")
	_, stale := getBody(t, s, pass, testDAVPath)
	mustPut(t, s, pass, testDAVPath, "v2-longer") // 让 stale 过期

	rec := davPut(s, pass, testDAVPath, "v3", map[string]string{"If-Match": stale})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("过期 If-Match 应为 412，实际 %d", rec.Code)
	}
	body, _ := getBody(t, s, pass, testDAVPath)
	if body != "v2-longer" {
		t.Fatalf("412 后文件不应变化，实际 %q", body)
	}
}

// 用例 3：If-Match:* 对不存在资源 412，对存在资源成功。
func TestIfMatchWildcard(t *testing.T) {
	s, pass := newDAVTest(t)

	rec := davPut(s, pass, testDAVPath, "new", map[string]string{"If-Match": "*"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("If-Match:* 对不存在资源应为 412，实际 %d", rec.Code)
	}
	if got := request(s, http.MethodGet, testDAVPath, testDAVUser, pass, nil, nil); got.Code != http.StatusNotFound {
		t.Fatalf("412 不应创建文件，GET 应为 404，实际 %d", got.Code)
	}

	mustPut(t, s, pass, testDAVPath, "v1")
	rec = davPut(s, pass, testDAVPath, "v2", map[string]string{"If-Match": "*"})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("If-Match:* 对存在资源应成功，实际 %d", rec.Code)
	}
}

// 用例 4：If-None-Match:* 对已存在资源 412（文件未变）；对不存在资源 201。
func TestIfNoneMatchWildcard(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "keep")

	rec := davPut(s, pass, testDAVPath, "clobber", map[string]string{"If-None-Match": "*"})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("If-None-Match:* 对已存在资源应为 412，实际 %d", rec.Code)
	}
	body, _ := getBody(t, s, pass, testDAVPath)
	if body != "keep" {
		t.Fatalf("412 后文件不应变化，实际 %q", body)
	}

	rec = davPut(s, pass, "/app1/new.txt", "fresh", map[string]string{"If-None-Match": "*"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("If-None-Match:* 对不存在资源应为 201，实际 %d", rec.Code)
	}

	// MKCOL 同样评估 If-None-Match：已存在集合 412，新集合 201。
	if rec := request(s, "MKCOL", "/app1/dir", testDAVUser, pass, nil, map[string]string{"If-None-Match": "*"}); rec.Code != http.StatusCreated {
		t.Fatalf("MKCOL 新集合应为 201，实际 %d", rec.Code)
	}
	if rec := request(s, "MKCOL", "/app1/dir", testDAVUser, pass, nil, map[string]string{"If-None-Match": "*"}); rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("MKCOL 已存在集合应为 412，实际 %d", rec.Code)
	}
}

// 用例 5：If-None-Match 列表命中当前 ETag → 412；不命中 → 成功。
func TestIfNoneMatchList(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "v1")
	_, etag := getBody(t, s, pass, testDAVPath)

	rec := davPut(s, pass, testDAVPath, "v2", map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("If-None-Match 命中当前 ETag 应为 412，实际 %d", rec.Code)
	}

	rec = davPut(s, pass, testDAVPath, "v2", map[string]string{"If-None-Match": `"other"`})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("If-None-Match 不命中应成功，实际 %d", rec.Code)
	}
}

// 用例 6：弱标签 W/"..." 用作 If-Match 永不匹配。
func TestIfMatchWeakTagNeverMatches(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "v1")
	_, etag := getBody(t, s, pass, testDAVPath)

	rec := davPut(s, pass, testDAVPath, "v2", map[string]string{"If-Match": "W/" + etag})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("弱标签 If-Match 应为 412，实际 %d", rec.Code)
	}
	body, _ := getBody(t, s, pass, testDAVPath)
	if body != "v1" {
		t.Fatalf("弱标签不应写入，实际 %q", body)
	}
}

// 用例 7：两个头同时出现且 If-Match 失败 → 412（顺序正确）。
func TestIfMatchEvaluatedBeforeIfNoneMatch(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "v1")

	rec := davPut(s, pass, testDAVPath, "v2", map[string]string{
		"If-Match":      `"stale"`,
		"If-None-Match": `"stale"`, // 单独看会通过
	})
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("If-Match 失败应先返回 412，实际 %d", rec.Code)
	}
}

// 用例 8：语法非法的头被忽略，请求按原行为成功。
func TestInvalidHeaderIgnored(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "v1")

	bad := []map[string]string{
		{"If-Match": ",,,"},
		{"If-Match": "abc"},           // 缺引号
		{"If-Match": `"unterminated`}, // 未闭合
		{"If-None-Match": ""},
		{"If-None-Match": `, "a"`},
	}
	for i, headers := range bad {
		rec := davPut(s, pass, testDAVPath, fmt.Sprintf("ok%d", i), headers)
		if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
			t.Fatalf("非法头 %v 应被忽略并成功，实际 %d", headers, rec.Code)
		}
	}
}

// 用例 9：无条件头的 PUT/DELETE 行为与改造前一致。
func TestUnconditionalUnchanged(t *testing.T) {
	s, pass := newDAVTest(t)

	rec := davPut(s, pass, testDAVPath, "plain", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("无条件 PUT 应为 201，实际 %d", rec.Code)
	}
	body, _ := getBody(t, s, pass, testDAVPath)
	if body != "plain" {
		t.Fatalf("无条件 PUT 内容应为 plain，实际 %q", body)
	}

	rec = request(s, http.MethodDelete, testDAVPath, testDAVUser, pass, nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("无条件 DELETE 应为 204，实际 %d", rec.Code)
	}
	if got := request(s, http.MethodGet, testDAVPath, testDAVUser, pass, nil, nil); got.Code != http.StatusNotFound {
		t.Fatalf("DELETE 后 GET 应为 404，实际 %d", got.Code)
	}
}

// 用例 10：契约测试——GET 头 ETag == PROPFIND getetag == 条件检查使用的值。
func TestETagContractAcrossSources(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "contract")

	_, getETag := getBody(t, s, pass, testDAVPath)
	pfETag := propfindETag(t, s, pass, testDAVPath)
	if getETag == "" {
		t.Fatal("GET 响应应带 ETag")
	}
	if getETag != pfETag {
		t.Fatalf("GET ETag %q 与 PROPFIND getetag %q 不一致", getETag, pfETag)
	}

	// 条件检查使用的值：用同一 ETag 做 If-Match 必须成功。
	rec := davPut(s, pass, testDAVPath, "updated", map[string]string{"If-Match": getETag})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("If-Match 使用同源 ETag 应成功，实际 %d", rec.Code)
	}
}

// 用例 11：并发 CAS 无丢更新。
func TestConcurrentIfMatchNoLostUpdate(t *testing.T) {
	s, pass := newDAVTest(t)
	mustPut(t, s, pass, testDAVPath, "")

	const n = 20
	var (
		wg        sync.WaitGroup
		successes int64
	)
	errs := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < 10000; attempt++ {
				rec := request(s, http.MethodGet, testDAVPath, testDAVUser, pass, nil, nil)
				if rec.Code != http.StatusOK {
					errs <- fmt.Sprintf("GET 返回 %d", rec.Code)
					return
				}
				etag := rec.Header().Get("ETag")
				next := rec.Body.String() + "x"
				putRec := davPut(s, pass, testDAVPath, next, map[string]string{"If-Match": etag})
				switch putRec.Code {
				case http.StatusCreated, http.StatusNoContent:
					atomic.AddInt64(&successes, 1)
					return
				case http.StatusPreconditionFailed:
					continue // 别人先写了，重读重试
				default:
					errs <- fmt.Sprintf("PUT 返回 %d", putRec.Code)
					return
				}
			}
			errs <- "重试次数超限"
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}

	body, _ := getBody(t, s, pass, testDAVPath)
	got := atomic.LoadInt64(&successes)
	if got != n {
		t.Fatalf("成功写入次数 %d != %d", got, n)
	}
	if int64(len(body)) != got {
		t.Fatalf("最终内容长度 %d != 成功写入次数 %d（存在丢更新）", len(body), got)
	}
}

// 用例 12：log 模式——过期 If-Match 仍写入成功，但产生 dry-run 日志。
func TestLogModeDoesNotReject(t *testing.T) {
	s, pass := newDAVTest(t)
	s.precond = modeLog
	logs := captureLogs(s)

	mustPut(t, s, pass, testDAVPath, "v1")
	_, stale := getBody(t, s, pass, testDAVPath)
	mustPut(t, s, pass, testDAVPath, "v2-longer")

	rec := davPut(s, pass, testDAVPath, "v3-longer", map[string]string{"If-Match": stale})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("log 模式不应拒绝，实际 %d", rec.Code)
	}
	body, _ := getBody(t, s, pass, testDAVPath)
	if body != "v3-longer" {
		t.Fatalf("log 模式应写入 v3-longer，实际 %q", body)
	}
	if !strings.Contains(logs.String(), "precondition(dry-run) 412") {
		t.Fatalf("log 模式应产生 dry-run 日志，实际 %q", logs.String())
	}
}

// 用例 13：off 模式——不拒绝，且不产生前置评估日志。
func TestOffModeSkipsEvaluation(t *testing.T) {
	s, pass := newDAVTest(t)
	s.precond = modeOff
	logs := captureLogs(s)

	mustPut(t, s, pass, testDAVPath, "v1")
	_, stale := getBody(t, s, pass, testDAVPath)
	mustPut(t, s, pass, testDAVPath, "v2-longer")

	rec := davPut(s, pass, testDAVPath, "v3-longer", map[string]string{"If-Match": stale})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("off 模式不应拒绝，实际 %d", rec.Code)
	}
	if logs.Len() != 0 {
		t.Fatalf("off 模式不应产生前置评估日志，实际 %q", logs.String())
	}
}
