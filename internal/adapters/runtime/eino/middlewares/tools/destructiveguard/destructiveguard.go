// Package destructiveguard 提供工具调用调度优化：只读工具保持并行，破坏性工具串行化避免竞态。
package destructiveguard

import (
	"context"
	"sync"

	"fkteams/agentcore"
	einoruntime "fkteams/internal/adapters/runtime/eino"
	"fkteams/tools"

	"github.com/cloudwego/eino/compose"
)

// New 创建工具中间件，破坏性工具通过互斥锁串行执行
func New() agentcore.ToolMiddleware {
	var mu sync.Mutex

	return einoruntime.WrapToolMiddleware("destructive_guard", compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if tools.ShouldSerializeTool(input.Name) {
					mu.Lock()
					defer mu.Unlock()
				}
				return next(ctx, input)
			}
		},
	})
}
