package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

// RunnerCreator 创建 Runner 的函数类型
type RunnerCreator func(ctx context.Context) *adk.Runner

// BackgroundExecutor 后台任务执行器
type BackgroundExecutor struct {
	createRunner RunnerCreator
	outputDir    string
}

// NewBackgroundExecutor 创建后台执行器
func NewBackgroundExecutor(createRunner RunnerCreator, outputDir string) *BackgroundExecutor {
	_ = os.MkdirAll(outputDir, 0755)
	return &BackgroundExecutor{
		createRunner: createRunner,
		outputDir:    outputDir,
	}
}

// Execute 执行任务，完全静默，结果写入文件
func (e *BackgroundExecutor) Execute(task string) (string, error) {
	ctx := context.Background()
	r := e.createRunner(ctx)
	if r == nil {
		return "", fmt.Errorf("failed to create runner")
	}

	inputMessages := []adk.Message{schema.UserMessage(task)}
	iter := r.Run(ctx, inputMessages, adk.WithCheckPointID("fkteams_scheduler"))

	var result strings.Builder
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			errMsg := fmt.Sprintf("执行出错: %v", event.Err)
			e.writeResult(task, errMsg)
			return "", event.Err
		}
		// 收集文本输出
		if event.Output != nil && event.Output.MessageOutput != nil {
			if msg := event.Output.MessageOutput.Message; msg != nil {
				result.WriteString(msg.Content)
			}
			if stream := event.Output.MessageOutput.MessageStream; stream != nil {
				for {
					chunk, err := stream.Recv()
					if err != nil {
						break
					}
					result.WriteString(chunk.Content)
				}
			}
		}
	}

	output := result.String()
	e.writeResult(task, output)
	return output, nil
}

// writeResult 将任务结果写入文件
func (e *BackgroundExecutor) writeResult(task string, result string) {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("task_%s.md", timestamp)
	filePath := filepath.Join(e.outputDir, filename)

	content := fmt.Sprintf("# 定时任务执行结果\n\n**时间**: %s\n\n**任务**: %s\n\n## 结果\n\n%s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		task,
		result,
	)

	_ = os.WriteFile(filePath, []byte(content), 0644)
}
