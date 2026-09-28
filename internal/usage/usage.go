// Package usage 统计账号根目录的磁盘占用。
package usage

import (
	"io/fs"
	"path/filepath"
)

// Dir 遍历 root，返回常规文件的总字节数与文件数。读不到的条目直接跳过。
func Dir(root string) (bytes int64, files int64, err error) {
	err = filepath.WalkDir(root, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// 权限或竞态导致的读取失败不影响整体统计。
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		bytes += info.Size()
		files++
		return nil
	})
	return bytes, files, err
}
