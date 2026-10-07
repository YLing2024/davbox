// Package settings 持久化运行期可改的配置。
//
// 当前只有跨域来源白名单一项，落在 <dataDir>/settings.json。写入采用
// 「临时文件 + rename」保证原子，读取时对损坏文件容错（保留为 .bak 并回退默认值）。
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Settings 是可持久化的设置集合。
type Settings struct {
	CORSOrigins []string `json:"corsOrigins"`
}

// Store 持有 settings.json 的路径与内存副本，读写都在锁内进行。
type Store struct {
	mu   sync.Mutex
	path string
	cur  Settings
}

// Open 读取 <dataDir>/settings.json。
//
//   - 文件不存在：使用 envOrigins（逗号分隔）作为初始默认值；
//   - 文件存在但没有 corsOrigins 键：同样使用 envOrigins；
//   - 文件存在且有该键：以文件为准（哪怕是空数组，也覆盖环境变量）；
//   - 文件损坏：回退到 envOrigins，并把原文件保留为 settings.json.bak。
func Open(dataDir, envOrigins string) (*Store, error) {
	st := &Store{path: filepath.Join(dataDir, "settings.json")}

	raw, err := os.ReadFile(st.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		st.cur = Settings{CORSOrigins: parseEnv(envOrigins)}
	case err != nil:
		return nil, err
	default:
		var doc struct {
			CORSOrigins *[]string `json:"corsOrigins"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			if bakErr := os.Rename(st.path, st.path+".bak"); bakErr != nil {
				return nil, fmt.Errorf("settings.json 损坏且无法备份: %w（原错误：%v）", bakErr, err)
			}
			st.cur = Settings{CORSOrigins: parseEnv(envOrigins)}
			break
		}
		if doc.CORSOrigins == nil {
			// 文件里没有该键（或为 null），环境变量作为初始默认。
			st.cur = Settings{CORSOrigins: parseEnv(envOrigins)}
			break
		}
		st.cur = Settings{CORSOrigins: normalizeList(*doc.CORSOrigins)}
	}
	return st, nil
}

// Current 返回当前设置的副本。
func (s *Store) Current() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Settings{CORSOrigins: append([]string{}, s.cur.CORSOrigins...)}
}

// Save 原子写入设置并更新内存副本。保存前应先完成校验。
func (s *Store) Save(next Settings) error {
	if next.CORSOrigins == nil {
		next.CORSOrigins = []string{}
	}
	buf, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*.tmp")
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
	if err := os.Rename(tmpName, s.path); err != nil {
		return err
	}
	s.cur = Settings{CORSOrigins: append([]string{}, next.CORSOrigins...)}
	return nil
}

// parseEnv 解析逗号分隔的环境变量默认值：规整、去重、丢弃非法项。
func parseEnv(v string) []string {
	return normalizeList(strings.Split(v, ","))
}

// normalizeList 规整一组来源，去重并保持首次出现的顺序。
// 非法项被丢弃（调用方若需报错请改用 ValidateOrigins）。
func normalizeList(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		norm, ok := NormalizeOrigin(item)
		if !ok {
			continue
		}
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	return out
}

// NormalizeOrigin 校验并规整一个来源，接受 scheme://host 或 scheme://host:port，
// scheme 限 http/https。允许一个可选的末尾斜杠；路径、查询、片段、用户信息一律拒绝。
// 返回形如 scheme://host[:port]（scheme/host 小写，显式端口保留）。
func NormalizeOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", false
	}
	if u.User != nil || u.Host == "" {
		return "", false
	}
	if u.Path != "" && u.Path != "/" {
		return "", false
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", false
	}
	if port := u.Port(); port != "" {
		return scheme + "://" + host + ":" + port, true
	}
	return scheme + "://" + host, true
}

// ValidateOrigins 逐项校验来源。空白项跳过；任一非空项不合法时返回错误，
// 错误信息指出是「第 N 行」（N 从 1 开始）。返回规整、去重后的列表。
func ValidateOrigins(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for i, item := range raw {
		if strings.TrimSpace(item) == "" {
			continue
		}
		norm, ok := NormalizeOrigin(item)
		if !ok {
			return nil, fmt.Errorf(
				"第 %d 行不合法：「%s」。应为 scheme://host 或 scheme://host:port，scheme 限 http/https",
				i+1, strings.TrimSpace(item),
			)
		}
		if _, dup := seen[norm]; dup {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	return out, nil
}
