package ssh

import (
	"log"
	"regexp"
	"strings"
)

// DangerousCommandChecker 定义安全校验器
type DangerousCommandChecker struct {
	// 关键词黑名单
	Blacklist []string
	// 禁止修改的敏感路径
	RestrictedPaths []string
}

func NewChecker() *DangerousCommandChecker {
	return &DangerousCommandChecker{
		Blacklist: []string{
			"rm ", "mkfs", "dd ", "shutdown", "reboot",
			"chmod 777", "chown ", "passwd", ":(){:|:&};:",
		},
		RestrictedPaths: []string{
			"/etc/", "/boot/", "/root/", "/sys/", "/proc/",
		},
	}
}

func (c *DangerousCommandChecker) IsDangerous(cmd string) (bool, string) {
	// 1. 标准化处理：转小写并去除首尾空格
	trimmedCmd := strings.ToLower(strings.TrimSpace(cmd))

	// 2. 检查黑名单关键词
	for _, word := range c.Blacklist {
		if strings.Contains(trimmedCmd, word) {
			return true, "包含禁用关键词: " + word
		}
	}

	// 3. 检查对敏感目录的写操作或删除操作
	// 匹配类似 > /etc/passwd 或 rm /etc/config 的行为
	for _, path := range c.RestrictedPaths {
		// 检查重定向符号
		if strings.Contains(trimmedCmd, ">") && strings.Contains(trimmedCmd, path) {
			return true, "禁止通过重定向修改系统目录: " + path
		}
		// 检查移动/复制到敏感目录
		if (strings.Contains(trimmedCmd, "mv ") || strings.Contains(trimmedCmd, "cp ")) && strings.Contains(trimmedCmd, path) {
			return true, "禁止移动/复制文件到系统目录: " + path
		}
	}

	// 4. 正则检查：防止复杂的管道符绕过
	// 比如攻击者可能尝试使用 base64 编码绕过检测，或者使用 curl | sh
	remoteFetchRegex := regexp.MustCompile(`(curl|wget).+?\|\s*(bash|sh|python|php)`)
	if remoteFetchRegex.MatchString(trimmedCmd) {
		return true, "检测到远程脚本直接执行风险 (curl/wget | sh)"
	}

	return false, ""
}

var checker = NewChecker()

// isDangerous 对外暴露的安全校验函数
func isDangerous(cmd string) bool {
	dangerous, reason := checker.IsDangerous(cmd)
	if dangerous {
		log.Printf("Detected dangerous command: %s, reason: %s", cmd, reason)
	}
	return dangerous
}
