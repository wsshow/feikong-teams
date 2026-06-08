package cli

import (
	cliruntime "fkteams/cli/runtime"
	"io"
	"os"
	"strings"
)

// ReadPipeInput 检测 stdin 是否为管道并读取内容
// 返回管道内容和是否检测到管道（即使内容为空，isPipe 也可能为 true）
func ReadPipeInput() (content string, isPipe bool) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", false
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", false // 终端设备，不是管道
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", true
	}
	return strings.TrimSpace(string(data)), true
}

type WorkMode = cliruntime.WorkMode

const (
	ModeTeam   = cliruntime.ModeTeam
	ModeDeep   = cliruntime.ModeDeep
	ModeGroup  = cliruntime.ModeGroup
	ModeCustom = cliruntime.ModeCustom
)

func ExtractAgentMention(input string) (agentName string, query string) {
	return cliruntime.ExtractAgentMention(input)
}

func ParseWorkMode(mode string) WorkMode {
	return cliruntime.ParseWorkMode(mode)
}
