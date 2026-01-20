package analyst

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/excel"
	"fkteams/tools/file"
	"fkteams/tools/script/uv"
	"fkteams/tools/todo"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

func NewAgent() adk.Agent {
	ctx := context.Background()

	analystSafeDir := "./script"
	analystDirEnv := os.Getenv("FEIKONG_UV_TOOL_DIR")
	if analystDirEnv != "" {
		analystSafeDir = analystDirEnv
	}

	// 初始化 Todo 工具
	todoToolsInstance, err := todo.NewTodoTools(analystSafeDir)
	if err != nil {
		log.Fatalf("初始化Todo工具失败: %v", err)
	}
	todoTools, err := todoToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建 Todo 工具失败:", err)
	}

	// 初始化 Excel 工具
	excelToolsInstance, err := excel.NewExcelTools(analystSafeDir)
	if err != nil {
		log.Fatalf("初始化Excel工具失败: %v", err)
	}
	excelTools, err := excelToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建 Excel 工具失败:", err)
	}

	// 初始化文件工具
	fileToolsInstance, err := file.NewFileTools(analystSafeDir)
	if err != nil {
		log.Fatalf("初始化文件工具失败: %v", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建文件工具失败:", err)
	}

	// 初始化 uv 工具
	uvToolsInstance, err := uv.NewUVTools(analystSafeDir)
	if err != nil {
		log.Fatal("初始化uv工具失败:", err)
	}
	uvTools, err := uvToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建uv工具失败:", err)
	}

	var toolList []tool.BaseTool
	toolList = append(toolList, todoTools...)
	toolList = append(toolList, excelTools...)
	toolList = append(toolList, fileTools...)
	toolList = append(toolList, uvTools...)

	systemMessages, err := AnalystPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"os":           runtime.GOOS,
		"data_dir":     analystSafeDir,
		"script_dir":   analystSafeDir,
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小析",
		Description:   "数据分析专家，擅长使用 Excel 和 Python 脚本从复杂数据中提取有价值的信息。",
		Instruction:   instruction,
		Model:         common.NewChatModel(),
		MaxIterations: common.MaxIterations,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	return a
}
