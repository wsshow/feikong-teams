package executil

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidateOperands 拒绝把工具参数中的普通操作数解释为额外命令行选项。
func ValidateOperands(values []string) error {
	if len(values) == 0 || len(values) > 256 {
		return fmt.Errorf("operand count must be between 1 and 256")
	}
	for _, value := range values {
		if value == "" || len(value) > 4096 || strings.TrimSpace(value) != value || strings.HasPrefix(value, "-") {
			return fmt.Errorf("invalid command operand")
		}
		if strings.ContainsFunc(value, unicode.IsControl) {
			return fmt.Errorf("command operand contains control characters")
		}
	}
	return nil
}
