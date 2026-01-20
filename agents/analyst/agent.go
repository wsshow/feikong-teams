package analyst

import (
	"context"
	"fkteams/agents/common"
	"fkteams/tools/excel"
	"fkteams/tools/file"
	"fkteams/tools/script/uv"
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
	systemMessages, err := AnalystPromptTemplate.Format(ctx, map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"os":           runtime.GOOS,
	})
	if err != nil {
		log.Fatal(err)
	}
	instruction := systemMessages[0].Content

	// 初始化 Excel 工具
	excelDir := "./data"
	excelDirEnv := os.Getenv("FEIKONG_EXCEL_TOOL_DIR")
	if excelDirEnv != "" {
		excelDir = excelDirEnv
	}
	excelToolsInstance, err := excel.NewExcelTools(excelDir)
	if err != nil {
		log.Fatalf("初始化Excel工具失败: %v", err)
	}
	excelTools, err := excelToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建 Excel 工具失败:", err)
	}

	// 初始化文件工具
	safeDir := "./script"
	codeDirEnv := os.Getenv("FEIKONG_UV_TOOL_DIR")
	if codeDirEnv != "" {
		safeDir = codeDirEnv
	}
	fileToolsInstance, err := file.NewFileTools(safeDir)
	if err != nil {
		log.Fatalf("初始化文件工具失败: %v", err)
	}
	fileTools, err := fileToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建文件工具失败:", err)
	}

	// 初始化 uv 工具
	uvDir := "./script"
	uvDirEnv := os.Getenv("FEIKONG_UV_TOOL_DIR")
	if uvDirEnv != "" {
		uvDir = uvDirEnv
	}
	uvToolsInstance, err := uv.NewUVTools(uvDir)
	if err != nil {
		log.Fatal("初始化uv工具失败:", err)
	}
	uvTools, err := uvToolsInstance.GetTools()
	if err != nil {
		log.Fatal("创建uv工具失败:", err)
	}

	var toolList []tool.BaseTool
	toolList = append(toolList, excelTools...)
	toolList = append(toolList, fileTools...)
	toolList = append(toolList, uvTools...)

	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "小析",
		Description:   "数据分析专家，擅长操作 excel 并使用脚本从数据中提取有价值的信息。",
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
