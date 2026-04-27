package command

import "strings"

// SecurityLevel 安全等级
type SecurityLevel int

const (
	LevelSafe      SecurityLevel = iota
	LevelModerate                // 中等风险，需要警告
	LevelDangerous               // 危险，需要审批或拒绝
)

// SecurityEvaluation 安全评估结果
type SecurityEvaluation struct {
	Level       SecurityLevel
	Description string
	Risks       []string
}

// 危险命令黑名单
// dangerousPatterns 精确匹配模式，按优先级排列。
// 模式末尾带 / 或空白表示仅当该位置后无更多字符时才触发（避免 rm -rf / 误匹配 rm -rf /tmp）
var dangerousPatterns = []struct {
	Pattern     string
	MatchFn     func(cmd string) bool
	Description string
	Risks       []string
}{
	{Pattern: "rm -rf /", Description: "删除根目录", Risks: []string{"会导致系统完全崩溃"},
		MatchFn: func(cmd string) bool {
			// 仅匹配 rm -rf / (无后续路径) 或 rm -rf /* (根下通配)
			return strings.Contains(cmd, "rm -rf /") &&
				(strings.HasSuffix(strings.TrimSpace(cmd), "rm -rf /") ||
					strings.Contains(cmd, "rm -rf /*") ||
					strings.Contains(cmd, "rm -rf / "))
		}},
	{Pattern: "mkfs", Description: "格式化文件系统", Risks: []string{"会清除磁盘上的所有数据"}},
	{Pattern: "dd if=/dev/zero", Description: "覆盖设备", Risks: []string{"可能永久性擦除数据"}},
	{Pattern: ":(){ :|:& };:", Description: "fork 炸弹", Risks: []string{"会耗尽系统资源"}},
	{Pattern: "chmod -r 777 /", Description: "递归修改根目录权限", Risks: []string{"严重的安全风险"}},
	{Pattern: "chown -r", Description: "递归修改所有者", Risks: []string{"可能破坏系统权限"}},
	{Pattern: "mv /", Description: "移动根目录", Risks: []string{"会破坏系统结构"},
		MatchFn: func(cmd string) bool {
			return strings.Contains(cmd, "mv /") &&
				(strings.HasSuffix(strings.TrimSpace(cmd), "mv /") || strings.Contains(cmd, "mv / "))
		}},
	{Pattern: "kill -9 -1", Description: "杀死所有进程", Risks: []string{"会导致系统崩溃"}},
	{Pattern: "remove-item -recurse -force c:\\", Description: "递归删除系统盘", Risks: []string{"会导致系统完全崩溃"},
		MatchFn: func(cmd string) bool {
			return strings.Contains(cmd, "remove-item -recurse -force c:\\")
		}},
	{Pattern: "format-volume", Description: "格式化卷", Risks: []string{"会清除磁盘数据"}},
	{Pattern: "clear-disk", Description: "清除磁盘", Risks: []string{"会永久擦除数据"}},
	{Pattern: "stop-process -id 0", Description: "终止系统关键进程", Risks: []string{"会导致系统崩溃"}},
	{Pattern: "stop-computer", Description: "关闭计算机", Risks: []string{"会立即关机"}},
	{Pattern: "restart-computer", Description: "重启计算机", Risks: []string{"会立即重启"}},
	{Pattern: "set-executionpolicy unrestricted", Description: "解除脚本执行限制", Risks: []string{"严重安全风险"}},
}

// 需要特别审查的命令模式
var riskyPatterns = []struct {
	Pattern     string
	Level       SecurityLevel
	Description string
	Risk        string
}{
	// Unix/Linux
	{"rm -rf", LevelModerate, "强制递归删除", "可能意外删除重要文件"},
	{"rm -r", LevelModerate, "递归删除", "可能意外删除文件"},
	{"dd of=", LevelDangerous, "dd 磁盘写入命令", "可能覆盖重要数据"},
	{"dd if=/dev/", LevelDangerous, "dd 读取设备文件", "可能读取敏感设备"},
	{"> /etc/", LevelDangerous, "重定向到系统配置", "可能破坏系统配置"},
	{"> /", LevelDangerous, "重定向到系统目录", "可能破坏系统文件"},
	{"chmod 777", LevelModerate, "设置全局可写权限", "安全风险"},
	{"chmod -r", LevelModerate, "递归修改权限", "可能影响多个文件"},
	{"chown -r", LevelModerate, "递归修改所有者", "可能破坏权限结构"},
	{"kill -9", LevelModerate, "强制终止进程", "可能导致数据丢失"},
	{"killall", LevelModerate, "终止进程组", "可能导致服务中断"},
	{"pkill", LevelModerate, "终止进程", "可能导致服务中断"},
	{"sudo ", LevelModerate, "以管理员权限执行", "高权限操作"},
	{"pip install", LevelModerate, "安装 Python 包", "可能引入不安全的依赖"},
	{"npm install -g", LevelModerate, "全局安装 npm 包", "可能影响系统环境"},
	{"wget", LevelModerate, "下载文件", "可能下载恶意内容"},
	{"curl", LevelModerate, "下载/上传数据", "可能泄露数据"},
	// Windows/PowerShell
	{"remove-item -recurse -force", LevelDangerous, "强制递归删除", "可能意外删除重要文件"},
	{"remove-item -recurse", LevelModerate, "递归删除", "可能意外删除文件"},
	{"stop-process", LevelModerate, "终止进程", "可能导致服务中断"},
	{"invoke-webrequest", LevelModerate, "下载文件", "可能下载恶意内容"},
	{"invoke-restmethod", LevelModerate, "调用远程接口", "可能泄露数据或下载恶意内容"},
	{"new-psdrive", LevelModerate, "映射网络驱动器", "可能连接不可信网络资源"},
}

func evaluateSecurity(command string) SecurityEvaluation {
	segments := splitShellCommands(command)
	if len(segments) > 1 {
		// 复合命令：取所有子命令中最高的安全等级
		var maxEval SecurityEvaluation
		for _, seg := range segments {
			eval := evaluateSingleCommand(seg)
			if eval.Level > maxEval.Level {
				maxEval = eval
				if maxEval.Level == LevelDangerous {
					return maxEval
				}
			}
		}
		return maxEval
	}
	return evaluateSingleCommand(command)
}

func evaluateSingleCommand(cmdLower string) SecurityEvaluation {
	cmdLower = strings.ToLower(strings.TrimSpace(cmdLower))

	// 末尾裸 & 导致进程脱离 Setpgid 管控，超时/取消时 kill 不到
	if cmdLower != "" && cmdLower[len(cmdLower)-1] == '&' && !strings.HasSuffix(cmdLower, "&&") {
		return SecurityEvaluation{
			Level:       LevelDangerous,
			Description: "后台符号 & 会导致进程脱离管控",
			Risks:       []string{"进程逃逸后无法被超时或取消终止，请移除末尾的 & 符号，改用 background=true 参数"},
		}
	}

	for _, p := range dangerousPatterns {
		matched := strings.Contains(cmdLower, p.Pattern)
		if matched && p.MatchFn != nil {
			matched = p.MatchFn(cmdLower)
		}
		if matched {
			return SecurityEvaluation{
				Level:       LevelDangerous,
				Description: p.Description,
				Risks:       p.Risks,
			}
		}
	}

	for _, p := range riskyPatterns {
		if strings.Contains(cmdLower, p.Pattern) {
			return SecurityEvaluation{
				Level:       p.Level,
				Description: p.Description,
				Risks:       []string{p.Risk},
			}
		}
	}

	return SecurityEvaluation{Level: LevelSafe, Description: "常规命令"}
}

// splitShellCommands 按 &&、;、| 拆分为独立子命令，关注引号
// 注：& 在 evaluateSingleCommand 中单独检测，不在此拆分
func splitShellCommands(command string) []string {
	var segments []string
	var current strings.Builder
	inSingle, inDouble := false, false

	for i := 0; i < len(command); i++ {
		ch := command[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
			current.WriteByte(ch)
		case ch == '"' && !inSingle:
			inDouble = !inDouble
			current.WriteByte(ch)
		case !inSingle && !inDouble && ch == '&' && i+1 < len(command) && command[i+1] == '&':
			segments = append(segments, strings.TrimSpace(current.String()))
			current.Reset()
			i++ // skip second &
		case !inSingle && !inDouble && ch == ';':
			segments = append(segments, strings.TrimSpace(current.String()))
			current.Reset()
		case !inSingle && !inDouble && ch == '|' && (i == 0 || command[i-1] != '|'):
			// split on | but preserve
			segments = append(segments, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		segments = append(segments, strings.TrimSpace(current.String()))
	}
	return segments
}

func securityLevelName(level SecurityLevel) string {
	switch level {
	case LevelSafe:
		return "安全"
	case LevelModerate:
		return "中等"
	case LevelDangerous:
		return "危险"
	default:
		return "未知"
	}
}
