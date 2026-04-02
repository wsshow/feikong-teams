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
var dangerousCommands = map[string]SecurityEvaluation{
	// Unix/Linux
	"rm -rf /":        {Level: LevelDangerous, Description: "删除根目录", Risks: []string{"会导致系统完全崩溃"}},
	"rm -rf /*":       {Level: LevelDangerous, Description: "删除根目录下所有内容", Risks: []string{"会导致系统完全崩溃"}},
	"mkfs":            {Level: LevelDangerous, Description: "格式化文件系统", Risks: []string{"会清除磁盘上的所有数据"}},
	"dd if=/dev/zero": {Level: LevelDangerous, Description: "覆盖设备", Risks: []string{"可能永久性擦除数据"}},
	":(){ :|:& };:":   {Level: LevelDangerous, Description: "fork 炸弹", Risks: []string{"会耗尽系统资源"}},
	"chmod -r 777 /":  {Level: LevelDangerous, Description: "递归修改根目录权限", Risks: []string{"严重的安全风险"}},
	"chown -r":        {Level: LevelDangerous, Description: "递归修改所有者", Risks: []string{"可能破坏系统权限"}},
	"mv /":            {Level: LevelDangerous, Description: "移动根目录", Risks: []string{"会破坏系统结构"}},
	"kill -9 -1":      {Level: LevelDangerous, Description: "杀死所有进程", Risks: []string{"会导致系统崩溃"}},
	// Windows/PowerShell
	"remove-item -recurse -force c:\\": {Level: LevelDangerous, Description: "递归删除系统盘", Risks: []string{"会导致系统完全崩溃"}},
	"format-volume":                    {Level: LevelDangerous, Description: "格式化卷", Risks: []string{"会清除磁盘数据"}},
	"clear-disk":                       {Level: LevelDangerous, Description: "清除磁盘", Risks: []string{"会永久擦除数据"}},
	"stop-process -id 0":               {Level: LevelDangerous, Description: "终止系统关键进程", Risks: []string{"会导致系统崩溃"}},
	"stop-computer":                    {Level: LevelDangerous, Description: "关闭计算机", Risks: []string{"会立即关机"}},
	"restart-computer":                 {Level: LevelDangerous, Description: "重启计算机", Risks: []string{"会立即重启"}},
	"set-executionpolicy unrestricted": {Level: LevelDangerous, Description: "解除脚本执行限制", Risks: []string{"严重安全风险"}},
}

// 需要特别审查的命令模式
var riskyPatterns = []struct {
	Pattern     string
	Level       SecurityLevel
	Description string
	Risk        string
}{
	// Unix/Linux
	{"rm -rf", LevelDangerous, "强制递归删除", "可能意外删除重要文件"},
	{"rm -r", LevelModerate, "递归删除", "可能意外删除文件"},
	{"dd if=", LevelDangerous, "dd 磁盘写入命令", "可能覆盖重要数据"},
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
	cmdLower := strings.ToLower(strings.TrimSpace(command))

	for pattern, eval := range dangerousCommands {
		if strings.Contains(cmdLower, pattern) {
			return eval
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
