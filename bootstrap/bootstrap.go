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
var initializers = map[string]Initializer{
	"uv":  &uvInitializer{},
	"bun": &bunInitializer{},
}

// Run 让用户选择需要初始化的环境并执行
func Run() {
	// 构建选项列表
	options := make([]string, 0, len(initializers))
	for name := range initializers {
		options = append(options, name)
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

	for _, name := range selectedOptions {
		init := initializers[name]
		pterm.Info.Printfln("[%s] 开始检测...", init.Name())
		if err := init.Run(); err != nil {
			pterm.Error.Printfln("[%s] 初始化失败: %v", init.Name(), err)
		} else {
			pterm.Success.Printfln("[%s] 初始化完成", init.Name())
		}
	}

	pterm.Success.Println("环境初始化完成")
}
