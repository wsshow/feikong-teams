package web

import (
"embed"
"io/fs"
)

// FS 嵌入整个 web 目录
//
//go:embed css js index.html
var FS embed.FS

// GetFS 返回 web 目录的文件系统
func GetFS() fs.FS {
	return FS
}
