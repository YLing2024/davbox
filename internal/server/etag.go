package server

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/net/webdav"
)

// strongETag 复刻 golang.org/x/net/webdav 的默认 ETag 算法：
// `"` + 十六进制(mtime 纳秒) + 十六进制(文件大小) + `"`。
//
// 对外发出的 ETag（GET/HEAD 响应头、PROPFIND 的 DAV:getetag）与条件请求
// 比较时读取的当前 ETag 必须逐字一致，所以这里只允许一份实现。
func strongETag(fi os.FileInfo) string {
	return fmt.Sprintf(`"%x%x"`, fi.ModTime().UnixNano(), fi.Size())
}

// etagFileInfo 让库的 findETag 优先走 strongETag，而不是自己再算一遍。
type etagFileInfo struct {
	os.FileInfo
}

var _ webdav.ETager = etagFileInfo{}

func (e etagFileInfo) ETag(context.Context) (string, error) {
	return strongETag(e.FileInfo), nil
}

// wrapETag 把 FileInfo 包成实现 webdav.ETager 的包装；已是 ETager 的原样返回。
func wrapETag(fi os.FileInfo) os.FileInfo {
	if fi == nil {
		return nil
	}
	if _, ok := fi.(webdav.ETager); ok {
		return fi
	}
	return etagFileInfo{FileInfo: fi}
}
