package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestDAVReadonlyLockForbidden(t *testing.T) {
	s, store, _ := newTestServer(t)
	_, pass, _ := store.Create("ro", true, "")

	body := `<?xml version="1.0"?><D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>me</D:owner></D:lockinfo>`
	rec := request(s, "LOCK", "/ro/lockme.txt", "ro", pass, strings.NewReader(body), map[string]string{"Content-Type": "application/xml"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("只读账号 LOCK 应为 403，实际 %d", rec.Code)
	}
	rec = request(s, "UNLOCK", "/ro/lockme.txt", "ro", pass, nil, map[string]string{"Lock-Token": "<opaquelocktoken:x>"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("只读账号 UNLOCK 应为 403，实际 %d", rec.Code)
	}
}
