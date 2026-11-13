package web

import (
	"embed"
	"io/fs"
)

// FS 嵌入前端生产构建产物。
// all: 保留以下划线或点开头的文件，避免遗漏 Vite 生成的共享模块。
//
//go:embed all:dist
var FS embed.FS

// GetFS 返回前端生产构建目录。
func GetFS() fs.FS {
	sub, err := fs.Sub(FS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
