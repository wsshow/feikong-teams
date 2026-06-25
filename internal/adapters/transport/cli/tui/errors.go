package tui

import "errors"

// ErrInterrupted 表示用户取消了当前 TUI 操作。
var ErrInterrupted = errors.New("user interrupted")
