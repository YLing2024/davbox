package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOpenEnvDefaultWhenFileAbsent(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir, "https://a.example.com, https://b.example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://a.example.com", "https://b.example.com"}
	if got := st.Current().CORSOrigins; !reflect.DeepEqual(got, want) {
		t.Fatalf("环境变量默认值应为 %v，实际 %v", want, got)
	}
}

func TestOpenEnvDefaultNormalizesAndDedupes(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir, "HTTPS://A.example.com/,https://a.example.com,ftp://bad.example.com, ,https://b.example.com:8443")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://a.example.com", "https://b.example.com:8443"}
	if got := st.Current().CORSOrigins; !reflect.DeepEqual(got, want) {
		t.Fatalf("环境变量默认值应规整去重为 %v，实际 %v", want, got)
	}
}

func TestOpenFileOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "settings.json"), `{"corsOrigins":["https://file.example.com"]}`)
	st, err := Open(dir, "https://env.example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://file.example.com"}
	if got := st.Current().CORSOrigins; !reflect.DeepEqual(got, want) {
		t.Fatalf("文件有值时不应使用环境变量，期望 %v，实际 %v", want, got)
	}
}

func TestOpenEmptyFileValueOverridesEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "settings.json"), `{"corsOrigins":[]}`)
	st, err := Open(dir, "https://env.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Current().CORSOrigins; len(got) != 0 {
		t.Fatalf("文件里的空数组应覆盖环境变量（关闭跨域），实际 %v", got)
	}
}

func TestOpenMissingKeyUsesEnv(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "settings.json"), `{}`)
	st, err := Open(dir, "https://env.example.com")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://env.example.com"}
	if got := st.Current().CORSOrigins; !reflect.DeepEqual(got, want) {
		t.Fatalf("文件无该键时应回退环境变量，期望 %v，实际 %v", want, got)
	}
}

func TestOpenCorruptedFileBacksUpAndUsesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	const garbage = "{ not json"
	writeFile(t, path, garbage)

	st, err := Open(dir, "https://env.example.com")
	if err != nil {
		t.Fatalf("损坏文件应容错，实际报错 %v", err)
	}
	want := []string{"https://env.example.com"}
	if got := st.Current().CORSOrigins; !reflect.DeepEqual(got, want) {
		t.Fatalf("损坏文件应回退默认值 %v，实际 %v", want, got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("损坏文件应被移走，实际仍存在: %v", err)
	}
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("应保留原文件为 .bak: %v", err)
	}
	if string(bak) != garbage {
		t.Fatalf(".bak 内容应为原文件，实际 %q", string(bak))
	}
}

func TestSaveAtomicAndReload(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	next := Settings{CORSOrigins: []string{"https://app.example.com", "http://127.0.0.1:18900"}}
	if err := st.Save(next); err != nil {
		t.Fatal(err)
	}
	if got := st.Current().CORSOrigins; !reflect.DeepEqual(got, next.CORSOrigins) {
		t.Fatalf("保存后内存应更新，实际 %v", got)
	}

	// 文件内容合法且可被重新读取。
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "https://app.example.com") {
		t.Fatalf("文件应写入来源，实际 %s", string(raw))
	}
	reopened, err := Open(dir, "https://env.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Current().CORSOrigins; !reflect.DeepEqual(got, next.CORSOrigins) {
		t.Fatalf("重开应读到保存值，实际 %v", got)
	}

	// 不应残留临时文件。
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".settings-") {
			t.Fatalf("不应残留临时文件: %s", e.Name())
		}
	}
}

func TestSaveEmptyListWritesKey(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir, "https://env.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(Settings{}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir, "https://env.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Current().CORSOrigins; len(got) != 0 {
		t.Fatalf("保存空列表后应覆盖环境变量为关闭，实际 %v", got)
	}
}

func TestValidateOriginsErrorLine(t *testing.T) {
	_, err := ValidateOrigins([]string{"https://ok.example.com", "ftp://bad.example.com"})
	if err == nil {
		t.Fatal("非法项应返回错误")
	}
	if !strings.Contains(err.Error(), "第 2 行") {
		t.Fatalf("错误应指出第 2 行，实际 %q", err.Error())
	}
}

func TestValidateOriginsSkipsBlankKeepsLineNumber(t *testing.T) {
	// 第一行为空（跳过），第二行非法 -> 报第 2 行。
	_, err := ValidateOrigins([]string{"  ", "not-an-origin"})
	if err == nil || !strings.Contains(err.Error(), "第 2 行") {
		t.Fatalf("空行应跳过、非法项应报第 2 行，实际 %v", err)
	}
}

func TestValidateOriginsAcceptsAndNormalizes(t *testing.T) {
	got, err := ValidateOrigins([]string{"HTTPS://App.Example.com/", "https://app.example.com", "http://127.0.0.1:18900"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://app.example.com", "http://127.0.0.1:18900"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("期望规整去重为 %v，实际 %v", want, got)
	}
}

func TestValidateOriginsRejectsInvalidForms(t *testing.T) {
	cases := []string{
		"ftp://a.example.com",        // scheme 非 http/https
		"app.example.com",            // 缺 scheme
		"https://",                   // 缺 host
		"https://a.example.com/foo",  // 带路径
		"https://a.example.com?x=1",  // 带查询
		"https://user@a.example.com", // 带用户信息
		"*",                          // 通配
		"https://a.example.com:abc",  // 端口非数字
	}
	for _, raw := range cases {
		if _, err := ValidateOrigins([]string{raw}); err == nil {
			t.Fatalf("应拒绝非法来源 %q", raw)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
