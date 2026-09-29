package auth

import "testing"

func TestParseMode(t *testing.T) {
	cases := []struct {
		in   string
		want Mode
		bad  bool
	}{
		{"", ModeBuiltin, false},
		{"builtin", ModeBuiltin, false},
		{" Builtin ", ModeBuiltin, false},
		{"sso", ModeSSO, false},
		{"SSO", ModeSSO, false},
		{"oidc", "", true},
	}
	for _, c := range cases {
		got, err := ParseMode(c.in)
		if c.bad {
			if err == nil {
				t.Fatalf("ParseMode(%q) 应报错", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseMode(%q) 意外错误: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("ParseMode(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestModeNormalize(t *testing.T) {
	if got := Mode("").Normalize(); got != ModeBuiltin {
		t.Fatalf("零值应归一为 builtin，实际 %q", got)
	}
	if got := Mode("whatever").Normalize(); got != ModeBuiltin {
		t.Fatalf("非法值应归一为 builtin，实际 %q", got)
	}
	if got := ModeSSO.Normalize(); got != ModeSSO {
		t.Fatalf("sso 应保持不变，实际 %q", got)
	}
}
