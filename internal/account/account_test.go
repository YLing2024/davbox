package account

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAuthenticateCorrectAndWrong(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Create("app1", false, "测试"); err != nil {
		t.Fatal(err)
	}
	a, ok := s.Get("app1")
	if !ok {
		t.Fatal("账号应存在")
	}

	if _, ok := s.Authenticate("app1", a.Pass); !ok {
		t.Fatal("正确口令应当通过")
	}
	if _, ok := s.Authenticate("app1", "wrong-password"); ok {
		t.Fatal("错误口令不应通过")
	}
	if _, ok := s.Authenticate("nobody", a.Pass); ok {
		t.Fatal("不存在的账号不应通过")
	}
}

func TestDisabledAccountRejected(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_, _, _ = s.Create("app1", false, "")
	a, _ := s.Get("app1")

	disabled := true
	if _, err := s.Update("app1", Patch{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Authenticate("app1", a.Pass); ok {
		t.Fatal("停用账号不应通过认证")
	}
}

func TestCreateRotateDelete(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)

	if _, _, err := s.Create("app1", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Create("app1", false, ""); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("重名应返回 ErrDuplicate，实际 %v", err)
	}
	if _, _, err := s.Create("Bad_Name", false, ""); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("非法名应返回 ErrInvalidName，实际 %v", err)
	}
	if _, _, err := s.Create("admin", false, ""); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("保留名应返回 ErrInvalidName，实际 %v", err)
	}

	old, _ := s.Get("app1")
	newPass, err := s.Rotate("app1")
	if err != nil {
		t.Fatal(err)
	}
	if newPass == "" || newPass == old.Pass {
		t.Fatal("换口令应生成新口令")
	}
	if _, ok := s.Authenticate("app1", old.Pass); ok {
		t.Fatal("旧口令应失效")
	}
	if _, ok := s.Authenticate("app1", newPass); !ok {
		t.Fatal("新口令应可用")
	}

	if err := s.Delete("app1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("app1"); ok {
		t.Fatal("删除后不应还能查到")
	}
	// 目录保留
	if _, err := os.Stat(filepath.Join(dir, "data", "app1")); err != nil {
		t.Fatalf("删除账号后目录应保留: %v", err)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_, pass, _ := s.Create("app1", true, "备注")

	// 重新打开，确认落盘并能读回。
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := reopened.Get("app1")
	if !ok {
		t.Fatal("重新加载后账号应存在")
	}
	if a.Pass != pass || !a.Readonly || a.Note != "备注" {
		t.Fatalf("字段不一致: %+v", a)
	}

	// 落盘文件权限应为 0600。
	info, err := os.Stat(filepath.Join(dir, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("accounts.json 权限应为 0600，实际 %v", info.Mode().Perm())
	}
}

func TestOpenCreatesEmptyArray(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]\n" {
		t.Fatalf("首次启动应写入空数组，实际 %q", string(raw))
	}
}
