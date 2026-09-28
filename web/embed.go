package web

import "embed"

// Dist 是 Vite 构建产物，由 go:embed 打进二进制。
//
//go:embed all:dist
var Dist embed.FS
