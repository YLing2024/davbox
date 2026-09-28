package auth

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"

	"github.com/YLing2024/davbox/internal/randstr"
)

type adminFile struct {
	Hash string `json:"hash"`
}

// LoadOrCreateSecret 读取或首次生成 32 字节 cookie 签名密钥。
func LoadOrCreateSecret(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, "secret.key")
	raw, err := os.ReadFile(path)
	if err == nil && len(raw) >= 32 {
		return raw, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// LoadOrCreateAdmin 读取或首次创建管理员口令。首次创建时生成 24 位口令，
// 写入 bcrypt 哈希，明文只落到 admin-password.txt 并在 stdout 打印一次。
func LoadOrCreateAdmin(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, "admin.json")
	raw, err := os.ReadFile(path)
	if err == nil {
		var af adminFile
		if err := json.Unmarshal(raw, &af); err != nil {
			return nil, fmt.Errorf("解析 admin.json: %w", err)
		}
		if af.Hash == "" {
			return nil, errors.New("admin.json 缺少 hash 字段")
		}
		return []byte(af.Hash), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	pass, err := randstr.Generate(24)
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	buf, err := json.MarshalIndent(adminFile{Hash: string(hash)}, "", "  ")
	if err != nil {
		return nil, err
	}
	buf = append(buf, '\n')
	if err := writeFile0600(path, buf); err != nil {
		return nil, err
	}
	if err := writeFile0600(filepath.Join(dataDir, "admin-password.txt"), []byte(pass+"\n")); err != nil {
		return nil, err
	}
	fmt.Printf("管理员初始口令（仅本次打印，请自行保存）：%s\n", pass)
	return hash, nil
}

func writeFile0600(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Chmod(0o600)
}
