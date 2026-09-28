package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeadPropsPersistence(t *testing.T) {
	s, store, dir := newTestServer(t)
	_, pass, _ := store.Create("app1", false, "")

	propDir := filepath.Join(dir, "davprops", "app1")

	// 先放一个文件。
	rec := request(s, http.MethodPut, "/app1/a.txt", "app1", pass, strings.NewReader("x"), nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT 应为 201，实际 %d", rec.Code)
	}

	setBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<D:propertyupdate xmlns:D="DAV:" xmlns:Z="urn:test:">` +
		`<D:set><D:prop><Z:author>alice</Z:author></D:prop></D:set>` +
		`</D:propertyupdate>`
	rec = request(s, "PROPPATCH", "/app1/a.txt", "app1", pass, strings.NewReader(setBody), map[string]string{"Content-Type": "application/xml"})
	if rec.Code != 207 {
		t.Fatalf("PROPPATCH 应为 207，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "HTTP/1.1 200 OK") {
		t.Fatalf("PROPPATCH 应返回 200 propstat，实际 %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(propDir, "a.txt", "__props__.json")); err != nil {
		t.Fatalf("应写出 sidecar: %v", err)
	}
	// sidecar 不在账号 root 内。
	if _, err := os.Stat(filepath.Join(dir, "data", "app1", "davprops")); !os.IsNotExist(err) {
		t.Fatal("账号 root 内不应出现 davprops 目录")
	}

	findBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<D:propfind xmlns:D="DAV:" xmlns:Z="urn:test:"><D:prop><Z:author/></D:prop></D:propfind>`
	rec = request(s, "PROPFIND", "/app1/a.txt", "app1", pass, strings.NewReader(findBody), map[string]string{"Depth": "0"})
	if rec.Code != 207 || !strings.Contains(rec.Body.String(), "alice") {
		t.Fatalf("PROPFIND 应读回死属性，实际 %d %s", rec.Code, rec.Body.String())
	}

	// COPY 跟随死属性。
	rec = request(s, "COPY", "/app1/a.txt", "app1", pass, nil, map[string]string{
		"Destination": "http://example.com/app1/c.txt",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("COPY 应为 201，实际 %d", rec.Code)
	}
	rec = request(s, "PROPFIND", "/app1/c.txt", "app1", pass, strings.NewReader(findBody), map[string]string{"Depth": "0"})
	if rec.Code != 207 || !strings.Contains(rec.Body.String(), "alice") {
		t.Fatalf("COPY 后死属性应跟随，实际 %d %s", rec.Code, rec.Body.String())
	}

	// MOVE 跟随死属性。
	rec = request(s, "MOVE", "/app1/a.txt", "app1", pass, nil, map[string]string{
		"Destination": "http://example.com/app1/m.txt",
	})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("MOVE 应为 201/204，实际 %d", rec.Code)
	}
	rec = request(s, "PROPFIND", "/app1/m.txt", "app1", pass, strings.NewReader(findBody), map[string]string{"Depth": "0"})
	if rec.Code != 207 || !strings.Contains(rec.Body.String(), "alice") {
		t.Fatalf("MOVE 后死属性应跟随，实际 %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(propDir, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("MOVE 后旧 sidecar 应消失")
	}

	// 目录列表（PROPFIND Depth:1）不得出现 sidecar。
	rec = request(s, "PROPFIND", "/app1/", "app1", pass, nil, map[string]string{"Depth": "1"})
	if rec.Code != 207 {
		t.Fatalf("PROPFIND Depth:1 应为 207，实际 %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "davprops") || strings.Contains(rec.Body.String(), "__props__") {
		t.Fatalf("目录列表不应暴露 sidecar: %s", rec.Body.String())
	}

	// PROPPATCH remove 后读不到，sidecar 清理。
	removeBody := `<?xml version="1.0" encoding="utf-8"?>` +
		`<D:propertyupdate xmlns:D="DAV:" xmlns:Z="urn:test:">` +
		`<D:remove><D:prop><Z:author/></D:prop></D:remove>` +
		`</D:propertyupdate>`
	rec = request(s, "PROPPATCH", "/app1/c.txt", "app1", pass, strings.NewReader(removeBody), map[string]string{"Content-Type": "application/xml"})
	if rec.Code != 207 {
		t.Fatalf("PROPPATCH remove 应为 207，实际 %d", rec.Code)
	}
	rec = request(s, "PROPFIND", "/app1/c.txt", "app1", pass, strings.NewReader(findBody), map[string]string{"Depth": "0"})
	if strings.Contains(rec.Body.String(), "alice") {
		t.Fatalf("remove 后不应再读到死属性: %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(propDir, "c.txt")); !os.IsNotExist(err) {
		t.Fatal("remove 后 sidecar 应清理")
	}

	// DELETE 清理死属性。
	rec = request(s, http.MethodDelete, "/app1/m.txt", "app1", pass, nil, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE 应为 204，实际 %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(propDir, "m.txt")); !os.IsNotExist(err) {
		t.Fatal("DELETE 后 sidecar 应清理")
	}
}
