package bootstrap

import (
	"github.com/pterm/pterm"
)

// Initializer 初始化器接口，所有初始化操作都需要实现此接口
type Initializer interface {
	// Name 返回初始化器名称，用于日志输出
	Name() string
	// Run 执行初始化操作
	Run() error
}

// 注册所有初始化器（后续新增只需在此追加）
var initializers = []Initializer{
	&uvInitializer{},
}

// Run 执行所有已注册的初始化操作
func Run() {
	pterm.Info.Println("开始初始化运行环境...")

	for _, init := range initializers {
		pterm.Info.Printfln("[%s] 开始检测...", init.Name())
		if err := init.Run(); err != nil {
			pterm.Error.Printfln("[%s] 初始化失败: %v", init.Name(), err)
		} else {
			pterm.Success.Printfln("[%s] 初始化完成", init.Name())
		}
	}

	pterm.Success.Println("环境初始化完成")
}
