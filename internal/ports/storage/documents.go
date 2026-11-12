package storage

// Documents 保存应用生成的结构化文档；路径必须位于显式绑定的数据根目录。
// Read 对不存在的记录返回可由 errors.Is(err, fs.ErrNotExist) 判断的错误。
type Documents interface {
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	Append(path string, data []byte) error
	Remove(path string) error
	// RemoveTree 原子移除目录下的文档，保留显式指定的完整文档路径。
	RemoveTree(directory string, preserve ...string) error
	List(directory string) ([]string, error)
}
