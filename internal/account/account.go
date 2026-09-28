// Package account 定义账号模型，负责 accounts.json 的读写与口令校验。
package account

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	"github.com/YLing2024/davbox/internal/randstr"
)

var (
	// ErrNotFound 表示账号不存在。
	ErrNotFound = errors.New("账号不存在")
	// ErrDuplicate 表示账号名已被占用。
	ErrDuplicate = errors.New("账号已存在")
	// ErrInvalidName 表示账号名不合法或属于保留字。
	ErrInvalidName = errors.New("账号名不合法")
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// 这些前缀与路由、静态资源冲突，不允许作为账号名。
var reserved = map[string]bool{
	"admin":  true,
	"api":    true,
	"assets": true,
}

// Account 是一条账号记录。口令明文保存，见 docs/SPEC.md §2 的说明。
type Account struct {
	User     string `json:"user"`
	Pass     string `json:"pass"`
	Root     string `json:"root"`
	Readonly bool   `json:"readonly"`
	Disabled bool   `json:"disabled"`
	Note     string `json:"note"`
}

// Patch 描述一次局部更新，nil 字段表示不改动。
type Patch struct {
	Readonly *bool
	Disabled *bool
	Note     *string
}

// Store 是内存中的账号表，写入时以临时文件加 Rename 原子替换。
type Store struct {
	mu       sync.Mutex
	path     string
	dataDir  string
	accounts []*Account
}

// Open 读取（必要时创建）dataDir 下的 accounts.json。
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataDir, "data"), 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		path:     filepath.Join(dataDir, "accounts.json"),
		dataDir:  dataDir,
		accounts: []*Account{},
	}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &s.accounts); err != nil {
			return nil, fmt.Errorf("解析 accounts.json: %w", err)
		}
	}
	for _, a := range s.accounts {
		if a.Root == "" {
			a.Root = s.defaultRoot(a.User)
		}
	}
	return s, nil
}

func (s *Store) defaultRoot(user string) string {
	return filepath.Join(s.dataDir, "data", user)
}

// DefaultRoot 返回账号未显式配置 root 时的默认目录。
func (s *Store) DefaultRoot(user string) string {
	return s.defaultRoot(user)
}

func (s *Store) findLocked(user string) (*Account, int) {
	for i, a := range s.accounts {
		if a.User == user {
			return a, i
		}
	}
	return nil, -1
}

// List 返回账号表的副本，按用户名排序。
func (s *Store) List() []Account {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Account, 0, len(s.accounts))
	for _, a := range s.accounts {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].User < out[j].User })
	return out
}

// Get 返回单个账号的副本。
func (s *Store) Get(user string) (Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, _ := s.findLocked(user)
	if a == nil {
		return Account{}, false
	}
	return *a, true
}

// Authenticate 用常量时间比较校验口令；停用账号一律失败。
func (s *Store) Authenticate(user, pass string) (Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, _ := s.findLocked(user)
	if a == nil || a.Disabled {
		return Account{}, false
	}
	if subtle.ConstantTimeCompare([]byte(pass), []byte(a.Pass)) != 1 {
		return Account{}, false
	}
	return *a, true
}

// Create 新建账号：校验名称、生成 20 位口令、创建根目录。
func (s *Store) Create(user string, readonly bool, note string) (Account, string, error) {
	if !nameRe.MatchString(user) || reserved[user] {
		return Account{}, "", ErrInvalidName
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, i := s.findLocked(user); i >= 0 {
		return Account{}, "", ErrDuplicate
	}
	pass, err := randstr.Generate(20)
	if err != nil {
		return Account{}, "", err
	}
	root := s.defaultRoot(user)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Account{}, "", err
	}
	a := &Account{User: user, Pass: pass, Root: root, Readonly: readonly, Note: note}
	s.accounts = append(s.accounts, a)
	if err := s.saveLocked(); err != nil {
		return Account{}, "", err
	}
	return *a, pass, nil
}

// Rotate 重新生成口令并落盘。
func (s *Store) Rotate(user string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, i := s.findLocked(user)
	if i < 0 {
		return "", ErrNotFound
	}
	pass, err := randstr.Generate(20)
	if err != nil {
		return "", err
	}
	a.Pass = pass
	if err := s.saveLocked(); err != nil {
		return "", err
	}
	return pass, nil
}

// Update 局部更新账号。
func (s *Store) Update(user string, p Patch) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, i := s.findLocked(user)
	if i < 0 {
		return Account{}, ErrNotFound
	}
	if p.Readonly != nil {
		a.Readonly = *p.Readonly
	}
	if p.Disabled != nil {
		a.Disabled = *p.Disabled
	}
	if p.Note != nil {
		a.Note = *p.Note
	}
	if err := s.saveLocked(); err != nil {
		return Account{}, err
	}
	return *a, nil
}

// Delete 从账号表移除条目，磁盘目录保持不动。
func (s *Store) Delete(user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, i := s.findLocked(user)
	if i < 0 {
		return ErrNotFound
	}
	s.accounts = append(s.accounts[:i], s.accounts[i+1:]...)
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.accounts == nil {
		s.accounts = []*Account{}
	}
	buf, err := json.MarshalIndent(s.accounts, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".accounts-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}
