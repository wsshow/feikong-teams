package cli

import (
	"errors"

	"github.com/charmbracelet/huh"
)

// ErrInterrupted 用户中断输入（Ctrl+C）
var ErrInterrupted = errors.New("user interrupted")

// ReadInput 使用 huh 读取单行用户输入
// prompt 为行首提示符，suggestFn 提供自动补全候选项（可为 nil）
func ReadInput(prompt string, suggestFn func() []string) (string, error) {
	var input string

	field := huh.NewInput().
		Prompt(prompt).
		Value(&input)

	if suggestFn != nil {
		field = field.SuggestionsFunc(suggestFn, nil)
	}

	if err := field.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrInterrupted
		}
		return "", err
	}

	return input, nil
}
