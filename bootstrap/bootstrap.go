package bootstrap

import (
	"os"

	"github.com/pterm/pterm"
)

// Initializer 初始化器接口，所有初始化操作都需要实现此接口
type Initializer interface {
	// Name 返回初始化器名称，用于日志输出
	Name() string
	// Run 执行初始化操作
	Run() error
}

// 注册所有初始化器（有序列表，确保选项顺序一致）
var initializers = []Initializer{
	&uvInitializer{},
	&bunInitializer{},
}

// Run 让用户选择需要初始化的环境并执行
func Run() {
	// 构建选项列表
	options := make([]string, 0, len(initializers))
	for _, init := range initializers {
		options = append(options, init.Name())
	}

	// 交互式多选
	selectedOptions, _ := pterm.DefaultInteractiveMultiselect.
		WithOptions(options).
		WithDefaultOptions(options).
		Show("请选择需要初始化的环境（ENTER选择/取消，TAB确认）")

	if len(selectedOptions) == 0 {
		pterm.Warning.Println("未选择任何环境，跳过初始化")
		return
	}

	pterm.Info.Printfln("即将初始化: %v", selectedOptions)

	// 构建 name->initializer 映射用于查找
	initMap := make(map[string]Initializer, len(initializers))
	for _, init := range initializers {
		initMap[init.Name()] = init
	}

	for _, name := range selectedOptions {
		init := initMap[name]
		pterm.Info.Printfln("[%s] 开始检测...", init.Name())
		if err := init.Run(); err != nil {
			pterm.Error.Printfln("[%s] 初始化失败: %v", init.Name(), err)
		} else {
			pterm.Success.Printfln("[%s] 初始化完成", init.Name())
		}
	}

	pterm.Success.Println("环境初始化完成")
}

// appendProxyEnv 如果设置了 FEIKONG_PROXY_URL，注入 HTTP_PROXY/HTTPS_PROXY 环境变量
func appendProxyEnv(env []string) []string {
	proxyURL := os.Getenv("FEIKONG_PROXY_URL")
	if proxyURL == "" {
		return env
	}
	pterm.Info.Printfln("使用代理: %s", proxyURL)
	env = append(env,
		"HTTP_PROXY="+proxyURL,
		"HTTPS_PROXY="+proxyURL,
		"http_proxy="+proxyURL,
		"https_proxy="+proxyURL,
	)
	return env
}
