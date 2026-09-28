// Package randstr 生成不依赖第三方库的随机字符串。
package randstr

import (
	"crypto/rand"
	"math/big"
)

// 去掉了容易混淆的 0/O/1/l/I。
const charset = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// Generate 返回 n 个字符的随机口令。
func Generate(n int) (string, error) {
	buf := make([]byte, n)
	max := big.NewInt(int64(len(charset)))
	for i := range buf {
		// 每个字符独立取一个均匀随机的下标，避免取模偏置。
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		buf[i] = charset[idx.Int64()]
	}
	return string(buf), nil
}
