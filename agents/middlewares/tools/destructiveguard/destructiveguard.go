// Package destructiveguard 提供工具调用调度优化：只读工具保持并行，破坏性工具串行化避免竞态。
package destructiveguard

import (
	"context"
	"sync"

	"fkteams/tools"

	"github.com/cloudwego/eino/compose"
)

// New 创建 ToolMiddleware，破坏性工具通过互斥锁串行执行
func New() compose.ToolMiddleware {
	var mu sync.Mutex

	return compose.ToolMiddleware{
		Invokable: func(next compose.InvokableToolEndpoint) compose.InvokableToolEndpoint {
			return func(ctx context.Context, input *compose.ToolInput) (*compose.ToolOutput, error) {
				if tools.IsDestructiveTool(input.Name) {
					mu.Lock()
					defer mu.Unlock()
				}
				return next(ctx, input)
			}
		},
	}
}
