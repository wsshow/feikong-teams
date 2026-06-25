package command

import "io"

// limitedWriter 限制写入大小并追踪截断状态
type limitedWriter struct {
	w         io.Writer
	limit     int64
	written   int64
	truncated bool
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	origLen := len(p)
	remaining := lw.limit - lw.written
	if remaining <= 0 {
		lw.truncated = true
		return origLen, nil
	}
	if int64(origLen) > remaining {
		p = p[:remaining]
		lw.truncated = true
	}
	n, err := lw.w.Write(p)
	lw.written += int64(n)
	return origLen, err
}
