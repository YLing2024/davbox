package auth

import (
	"fmt"
	"strings"
)

// HeaderAuthUser 是 SSO 模式下网关注入的已认证用户标识头。
// 只在服务仅监听回环、且由网关经反向代理暴露时才能信任。
const HeaderAuthUser = "X-Auth-User"

// Mode 是管理端的认证模式。
type Mode string

const (
	// ModeBuiltin 用 davbox 自带的管理员口令（默认，开箱即用）。
	ModeBuiltin Mode = "builtin"
	// ModeSSO 关闭自带口令，只信任网关注入的 X-Auth-User。
	ModeSSO Mode = "sso"
)

// ParseMode 解析配置值：空串按默认 builtin 处理，其余非法值返回错误。
func ParseMode(v string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", string(ModeBuiltin):
		return ModeBuiltin, nil
	case string(ModeSSO):
		return ModeSSO, nil
	default:
		return "", fmt.Errorf("AUTH_MODE 取值非法: %q（只支持 builtin 或 sso）", v)
	}
}

// Normalize 把零值或非法值收敛为默认的 builtin。
func (m Mode) Normalize() Mode {
	if m == ModeBuiltin || m == ModeSSO {
		return m
	}
	return ModeBuiltin
}

// IsSSO 报告当前是否为 SSO 模式。
func (m Mode) IsSSO() bool { return m == ModeSSO }
