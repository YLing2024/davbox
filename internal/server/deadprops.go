package server

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/net/webdav"
)

// deadPropFS 在 webdav.Dir 之上实现死属性（dead properties）持久化。
//
// 存储位置的选择：sidecar 放在账号 root **之外**的
// `<数据目录>/davprops/<user>/`，而不是 root 内的 `.davprops/`。
// 理由：
//  1. root 内任何隐藏目录都仍处在 WebDAV/PROPFIND 和 client 目录列表可见的
//     命名空间里，必须额外过滤每一处遍历（Readdir、client list、用量统计），
//     漏一处就会污染用户目录；放到 root 外则从结构上杜绝这种污染。
//  2. 用量统计（usage.Dir）与 client API 直接走 os 层，无需感知 sidecar。
//
// 资源 key 用 webdav.Handler 剥离 Prefix 后的相对路径，统一 path.Clean；
// 每个资源对应 `<propDir>/<key 各段作为目录>/__props__.json`，root 对应
// `<propDir>/__root__.json`。这样同一资源的文件/目录共享一个 key，
// 且删除/重命名可以直接搬移子树。
type deadPropFS struct {
	root    string
	propDir string
}

var _ webdav.FileSystem = (*deadPropFS)(nil)

func newDeadPropFS(root, propDir string) *deadPropFS {
	return &deadPropFS{root: root, propDir: propDir}
}

// cleanKey 与 x/net/webdav 内部的 slashClean 语义一致：
// 保证以 "/" 开头且已 clean。
func cleanKey(name string) string {
	if name == "" {
		return "/"
	}
	if name[0] != '/' {
		name = "/" + name
	}
	return path.Clean(name)
}

func (d *deadPropFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return webdav.Dir(d.root).Mkdir(ctx, name, perm)
}

func (d *deadPropFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	fi, err := webdav.Dir(d.root).Stat(ctx, name)
	if err != nil {
		return nil, err
	}
	return wrapETag(fi), nil
}

func (d *deadPropFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	f, err := webdav.Dir(d.root).OpenFile(ctx, name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &deadPropFile{File: f, fs: d, key: cleanKey(name)}, nil
}

func (d *deadPropFS) RemoveAll(ctx context.Context, name string) error {
	if err := webdav.Dir(d.root).RemoveAll(ctx, name); err != nil {
		return err
	}
	d.removeProps(cleanKey(name))
	return nil
}

func (d *deadPropFS) Rename(ctx context.Context, oldName, newName string) error {
	if err := webdav.Dir(d.root).Rename(ctx, oldName, newName); err != nil {
		return err
	}
	d.renameProps(cleanKey(oldName), cleanKey(newName))
	return nil
}

// sidecar 返回某资源死属性文件的绝对路径。
func (d *deadPropFS) sidecar(key string) string {
	rel := strings.TrimPrefix(key, "/")
	if rel == "" {
		return filepath.Join(d.propDir, "__root__.json")
	}
	return filepath.Join(d.propDir, filepath.FromSlash(rel), "__props__.json")
}

// resourceDir 返回某资源 sidecar 所在的目录（用于整体删除/搬移）。
func (d *deadPropFS) resourceDir(key string) string {
	return filepath.Join(d.propDir, filepath.FromSlash(strings.TrimPrefix(key, "/")))
}

func (d *deadPropFS) loadProps(key string) (map[xml.Name]webdav.Property, error) {
	b, err := os.ReadFile(d.sidecar(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stored []storedProp
	if err := json.Unmarshal(b, &stored); err != nil {
		return nil, err
	}
	m := make(map[xml.Name]webdav.Property, len(stored))
	for _, s := range stored {
		name := xml.Name{Space: s.Space, Local: s.Local}
		m[name] = webdav.Property{XMLName: name, InnerXML: s.InnerXML}
	}
	return m, nil
}

func (d *deadPropFS) saveProps(key string, m map[xml.Name]webdav.Property) error {
	sc := d.sidecar(key)
	if len(m) == 0 {
		if err := os.Remove(sc); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		pruneEmptyDirs(filepath.Dir(sc), d.propDir)
		return nil
	}
	list := make([]storedProp, 0, len(m))
	for _, p := range m {
		list = append(list, storedProp{Space: p.XMLName.Space, Local: p.XMLName.Local, InnerXML: p.InnerXML})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Space != list[j].Space {
			return list[i].Space < list[j].Space
		}
		return list[i].Local < list[j].Local
	})
	buf, err := json.Marshal(list)
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	dir := filepath.Dir(sc)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".props-*")
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
	return os.Rename(tmpName, sc)
}

func (d *deadPropFS) removeProps(key string) {
	if key == "/" {
		os.RemoveAll(d.propDir)
		return
	}
	os.RemoveAll(d.resourceDir(key))
}

func (d *deadPropFS) renameProps(oldKey, newKey string) {
	if oldKey == "/" || newKey == "/" || oldKey == newKey {
		return
	}
	oldDir := d.resourceDir(oldKey)
	if _, err := os.Stat(oldDir); err != nil {
		return
	}
	newDir := d.resourceDir(newKey)
	if err := os.MkdirAll(filepath.Dir(newDir), 0o700); err != nil {
		return
	}
	os.RemoveAll(newDir)
	_ = os.Rename(oldDir, newDir)
}

// pruneEmptyDirs 自底向上删除空目录，止于（含）stop。
func pruneEmptyDirs(dir, stop string) {
	stop = filepath.Clean(stop)
	for {
		dir = filepath.Clean(dir)
		if dir == stop || !strings.HasPrefix(dir, stop+string(os.PathSeparator)) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

type storedProp struct {
	Space    string `json:"space"`
	Local    string `json:"local"`
	InnerXML []byte `json:"innerXml,omitempty"`
}

// deadPropFile 让 webdav.File 同时满足 webdav.DeadPropsHolder。
type deadPropFile struct {
	webdav.File
	fs  *deadPropFS
	key string
}

var _ webdav.DeadPropsHolder = (*deadPropFile)(nil)

// Stat 包一层 ETager，使 GET/HEAD 与 PROPFIND 取到的 ETag 与条件比较同源。
func (f *deadPropFile) Stat() (os.FileInfo, error) {
	fi, err := f.File.Stat()
	if err != nil {
		return nil, err
	}
	return wrapETag(fi), nil
}

func (f *deadPropFile) DeadProps() (map[xml.Name]webdav.Property, error) {
	return f.fs.loadProps(f.key)
}

func (f *deadPropFile) Patch(patches []webdav.Proppatch) ([]webdav.Propstat, error) {
	m, err := f.fs.loadProps(f.key)
	if err != nil {
		return nil, err
	}
	if m == nil {
		m = map[xml.Name]webdav.Property{}
	}
	pstat := webdav.Propstat{Status: http.StatusOK}
	for _, patch := range patches {
		for _, p := range patch.Props {
			pstat.Props = append(pstat.Props, webdav.Property{XMLName: p.XMLName})
			if patch.Remove {
				delete(m, p.XMLName)
				continue
			}
			m[p.XMLName] = p
		}
	}
	if err := f.fs.saveProps(f.key, m); err != nil {
		return nil, err
	}
	return []webdav.Propstat{pstat}, nil
}
