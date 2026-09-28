// Package auth 提供会话 cookie 的签名/校验，以及管理员凭据的初始化。
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// Session 是 cookie 里承载的最小信息。
type Session struct {
	Kind string `json:"k"`
	User string `json:"u,omitempty"`
	Exp  int64  `json:"e"`
}

// Signer 用 HMAC-SHA256 对会话载荷签名。
type Signer struct {
	key []byte
}

// NewSigner 用 secret.key 构造签名器。
func NewSigner(key []byte) *Signer {
	return &Signer{key: key}
}

// Issue 生成 kind/user 对应的 cookie 值，ttl 之后过期。
func (s *Signer) Issue(kind, user string, ttl time.Duration) string {
	sess := Session{Kind: kind, User: user, Exp: time.Now().Add(ttl).Unix()}
	payload, _ := json.Marshal(sess)
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + s.sign(body)
}

func (s *Signer) sign(body string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Verify 校验签名与有效期，返回会话内容。
func (s *Signer) Verify(token string) (Session, bool) {
	i := strings.IndexByte(token, '.')
	if i < 0 {
		return Session{}, false
	}
	body, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(s.sign(body))) {
		return Session{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return Session{}, false
	}
	var sess Session
	if err := json.Unmarshal(payload, &sess); err != nil {
		return Session{}, false
	}
	if sess.Exp < time.Now().Unix() {
		return Session{}, false
	}
	return sess, true
}
