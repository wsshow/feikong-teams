package bootstrap

import (
	"fkteams/config"

	"github.com/pterm/pterm"
)

// Initializer 初始化器接口，所有初始化操作都需要实现此接口
type Initializer interface {
	// Name 返回初始化器名称，用于日志输出
	Name() string
	// Run 执行初始化操作
	Run() error
}

// MirrorConfigurer 镜像源配置接口，初始化器可选实现
type MirrorConfigurer interface {
	// ConfigureMirror 配置镜像源，mirror 为 true 时强制配置
	ConfigureMirror(mirror bool)
}

// 注册所有初始化器（有序列表，确保选项顺序一致）
var initializers = []Initializer{
	&uvInitializer{},
	&bunInitializer{},
}

// Names 返回所有可用的初始化器名称
func Names() []string {
	names := make([]string, 0, len(initializers))
	for _, init := range initializers {
		names = append(names, init.Name())
	}
	return names
}

// Run 让用户选择需要初始化的环境并执行
func Run(mirror bool) {
	options := Names()

	// 交互式多选
	selectedOptions, _ := pterm.DefaultInteractiveMultiselect.
		WithOptions(options).
		WithDefaultOptions(options).
		Show("请选择需要初始化的环境（ENTER选择/取消，TAB确认）")

	if len(selectedOptions) == 0 {
		pterm.Warning.Println("未选择任何环境，跳过初始化")
		return
	}

	runSelected(selectedOptions, mirror)
}

// RunWith 直接初始化指定的环境（非交互模式）
// 若 names 为空则初始化全部环境
func RunWith(names []string, mirror bool) {
	if len(names) == 0 {
		names = Names()
	}
	runSelected(names, mirror)
}

func runSelected(selectedOptions []string, mirror bool) {
	// 尝试加载配置文件以获取代理等设置，配置文件不存在时使用默认值
	if err := config.Init(); err != nil {
		pterm.Warning.Printfln("加载配置文件失败（将使用默认设置）: %v", err)
	}

	pterm.Info.Printfln("即将初始化: %v", selectedOptions)

	// 构建 name->initializer 映射用于查找
	initMap := make(map[string]Initializer, len(initializers))
	for _, init := range initializers {
		initMap[init.Name()] = init
	}

	for _, name := range selectedOptions {
		init, ok := initMap[name]
		if !ok {
			pterm.Error.Printfln("未知的环境: %s（可选: %v）", name, Names())
			continue
		}
		pterm.Info.Printfln("[%s] 开始检测...", init.Name())
		if err := init.Run(); err != nil {
			pterm.Error.Printfln("[%s] 初始化失败: %v", init.Name(), err)
		} else {
			pterm.Success.Printfln("[%s] 初始化完成", init.Name())
			if mc, ok := init.(MirrorConfigurer); ok {
				mc.ConfigureMirror(mirror)
			}
		}
	}

	pterm.Success.Println("环境初始化完成")
}

// appendProxyEnv 如果设置了 FEIKONG_PROXY_URL，注入 HTTP_PROXY/HTTPS_PROXY 环境变量
func appendProxyEnv(env []string) []string {
	proxyURL := config.Get().ProxyURL()
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
