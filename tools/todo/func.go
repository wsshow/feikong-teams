package todo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Todo 待办事项结构
type Todo struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`             // pending, in_progress, completed, cancelled
	Priority    string     `json:"priority,omitempty"` // low, medium, high, urgent
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TodoList 待办事项列表
type TodoList struct {
	Todos []Todo `json:"todos"`
}

// TodoTools Todo工具实例，每个agent可以有独立的实例
type TodoTools struct {
	filePath string
}

// NewTodoTools 创建一个新的Todo工具实例
func NewTodoTools(baseDir string) (*TodoTools, error) {
	// 转换为绝对路径
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("无法获取绝对路径: %w", err)
	}

	// 检查目录是否存在，如果不存在则创建
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return nil, fmt.Errorf("无法创建目录 %s: %w", absPath, err)
		}
	}

	filePath := filepath.Join(absPath, "todos.json")

	// 如果文件不存在，创建一个空的待办列表
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		emptyList := TodoList{Todos: []Todo{}}
		data, err := json.MarshalIndent(emptyList, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("无法序列化待办列表: %w", err)
		}
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return nil, fmt.Errorf("无法创建待办列表文件: %w", err)
		}
	}

	return &TodoTools{
		filePath: filePath,
	}, nil
}

// loadTodoList 加载待办列表
func (tt *TodoTools) loadTodoList() (*TodoList, error) {
	data, err := os.ReadFile(tt.filePath)
	if err != nil {
		return nil, fmt.Errorf("无法读取待办列表: %w", err)
	}

	var list TodoList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("无法解析待办列表: %w", err)
	}

	return &list, nil
}

// saveTodoList 保存待办列表
func (tt *TodoTools) saveTodoList(list *TodoList) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("无法序列化待办列表: %w", err)
	}

	if err := os.WriteFile(tt.filePath, data, 0644); err != nil {
		return fmt.Errorf("无法保存待办列表: %w", err)
	}

	return nil
}

// generateID 生成唯一的 ID
func generateID() string {
	return fmt.Sprintf("todo_%d", time.Now().UnixNano())
}

// TodoAddRequest 添加待办事项请求
type TodoAddRequest struct {
	Title       string `json:"title" jsonschema:"description=待办事项标题,required"`
	Description string `json:"description,omitempty" jsonschema:"description=待办事项详细描述"`
	Priority    string `json:"priority,omitempty" jsonschema:"description=优先级: low(低), medium(中), high(高), urgent(紧急)"`
}

// TodoAddResponse 添加待办事项响应
type TodoAddResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	Todo         *Todo  `json:"todo,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// TodoAdd 添加待办事项
func (tt *TodoTools) TodoAdd(ctx context.Context, req *TodoAddRequest) (*TodoAddResponse, error) {
	if req.Title == "" {
		return &TodoAddResponse{
			Success:      false,
			ErrorMessage: "待办事项标题不能为空",
		}, nil
	}

	// 验证优先级
	if req.Priority != "" && req.Priority != "low" && req.Priority != "medium" && req.Priority != "high" && req.Priority != "urgent" {
		return &TodoAddResponse{
			Success:      false,
			ErrorMessage: "优先级必须是 low, medium, high 或 urgent 之一",
		}, nil
	}

	list, err := tt.loadTodoList()
	if err != nil {
		return &TodoAddResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("加载待办列表失败: %v", err),
		}, nil
	}

	now := time.Now()
	todo := Todo{
		ID:          generateID(),
		Title:       req.Title,
		Description: req.Description,
		Status:      "pending",
		Priority:    req.Priority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	list.Todos = append(list.Todos, todo)

	if err := tt.saveTodoList(list); err != nil {
		return &TodoAddResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("保存待办列表失败: %v", err),
		}, nil
	}

	return &TodoAddResponse{
		Success: true,
		Message: "待办事项已添加",
		Todo:    &todo,
	}, nil
}

// TodoListRequest 列出待办事项请求
type TodoListRequest struct {
	Status   string `json:"status,omitempty" jsonschema:"description=按状态过滤: pending(待处理), in_progress(进行中), completed(已完成), cancelled(已取消)"`
	Priority string `json:"priority,omitempty" jsonschema:"description=按优先级过滤: low, medium, high, urgent"`
}

// TodoListResponse 列出待办事项响应
type TodoListResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	Todos        []Todo `json:"todos,omitempty"`
	TotalCount   int    `json:"total_count"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// TodoListFunc 列出待办事项
func (tt *TodoTools) TodoListFunc(ctx context.Context, req *TodoListRequest) (*TodoListResponse, error) {
	list, err := tt.loadTodoList()
	if err != nil {
		return &TodoListResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("加载待办列表失败: %v", err),
		}, nil
	}

	// 过滤待办事项
	var filteredTodos []Todo
	for _, todo := range list.Todos {
		// 按状态过滤
		if req.Status != "" && todo.Status != req.Status {
			continue
		}
		// 按优先级过滤
		if req.Priority != "" && todo.Priority != req.Priority {
			continue
		}
		filteredTodos = append(filteredTodos, todo)
	}

	message := fmt.Sprintf("共有 %d 个待办事项", len(filteredTodos))
	if req.Status != "" {
		message += fmt.Sprintf("（状态: %s）", req.Status)
	}
	if req.Priority != "" {
		message += fmt.Sprintf("（优先级: %s）", req.Priority)
	}

	return &TodoListResponse{
		Success:    true,
		Message:    message,
		Todos:      filteredTodos,
		TotalCount: len(filteredTodos),
	}, nil
}

// TodoUpdateRequest 更新待办事项请求
type TodoUpdateRequest struct {
	ID          string  `json:"id" jsonschema:"description=待办事项ID,required"`
	Title       *string `json:"title,omitempty" jsonschema:"description=新的标题"`
	Description *string `json:"description,omitempty" jsonschema:"description=新的描述"`
	Status      *string `json:"status,omitempty" jsonschema:"description=新的状态: pending, in_progress, completed, cancelled"`
	Priority    *string `json:"priority,omitempty" jsonschema:"description=新的优先级: low, medium, high, urgent"`
}

// TodoUpdateResponse 更新待办事项响应
type TodoUpdateResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	Todo         *Todo  `json:"todo,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// TodoUpdate 更新待办事项
func (tt *TodoTools) TodoUpdate(ctx context.Context, req *TodoUpdateRequest) (*TodoUpdateResponse, error) {
	if req.ID == "" {
		return &TodoUpdateResponse{
			Success:      false,
			ErrorMessage: "待办事项 ID 不能为空",
		}, nil
	}

	// 验证状态
	if req.Status != nil {
		status := *req.Status
		if status != "pending" && status != "in_progress" && status != "completed" && status != "cancelled" {
			return &TodoUpdateResponse{
				Success:      false,
				ErrorMessage: "状态必须是 pending, in_progress, completed 或 cancelled 之一",
			}, nil
		}
	}

	// 验证优先级
	if req.Priority != nil {
		priority := *req.Priority
		if priority != "low" && priority != "medium" && priority != "high" && priority != "urgent" {
			return &TodoUpdateResponse{
				Success:      false,
				ErrorMessage: "优先级必须是 low, medium, high 或 urgent 之一",
			}, nil
		}
	}

	list, err := tt.loadTodoList()
	if err != nil {
		return &TodoUpdateResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("加载待办列表失败: %v", err),
		}, nil
	}

	// 查找并更新待办事项
	var found bool
	var updatedTodo *Todo
	for i := range list.Todos {
		if list.Todos[i].ID == req.ID {
			found = true
			if req.Title != nil {
				list.Todos[i].Title = *req.Title
			}
			if req.Description != nil {
				list.Todos[i].Description = *req.Description
			}
			if req.Status != nil {
				list.Todos[i].Status = *req.Status
				// 如果状态变为已完成，设置完成时间
				if *req.Status == "completed" && list.Todos[i].CompletedAt == nil {
					now := time.Now()
					list.Todos[i].CompletedAt = &now
				}
			}
			if req.Priority != nil {
				list.Todos[i].Priority = *req.Priority
			}
			list.Todos[i].UpdatedAt = time.Now()
			updatedTodo = &list.Todos[i]
			break
		}
	}

	if !found {
		return &TodoUpdateResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("未找到 ID 为 %s 的待办事项", req.ID),
		}, nil
	}

	if err := tt.saveTodoList(list); err != nil {
		return &TodoUpdateResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("保存待办列表失败: %v", err),
		}, nil
	}

	return &TodoUpdateResponse{
		Success: true,
		Message: "待办事项已更新",
		Todo:    updatedTodo,
	}, nil
}

// TodoDeleteRequest 删除待办事项请求
type TodoDeleteRequest struct {
	ID string `json:"id" jsonschema:"description=待办事项ID,required"`
}

// TodoDeleteResponse 删除待办事项响应
type TodoDeleteResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// TodoDelete 删除待办事项
func (tt *TodoTools) TodoDelete(ctx context.Context, req *TodoDeleteRequest) (*TodoDeleteResponse, error) {
	if req.ID == "" {
		return &TodoDeleteResponse{
			Success:      false,
			ErrorMessage: "待办事项 ID 不能为空",
		}, nil
	}

	list, err := tt.loadTodoList()
	if err != nil {
		return &TodoDeleteResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("加载待办列表失败: %v", err),
		}, nil
	}

	// 查找并删除待办事项
	var found bool
	var newTodos []Todo
	for _, todo := range list.Todos {
		if todo.ID == req.ID {
			found = true
			continue
		}
		newTodos = append(newTodos, todo)
	}

	if !found {
		return &TodoDeleteResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("未找到 ID 为 %s 的待办事项", req.ID),
		}, nil
	}

	list.Todos = newTodos

	if err := tt.saveTodoList(list); err != nil {
		return &TodoDeleteResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("保存待办列表失败: %v", err),
		}, nil
	}

	return &TodoDeleteResponse{
		Success: true,
		Message: "待办事项已删除",
	}, nil
}

// TodoBatchAddRequest 批量添加待办事项请求
type TodoBatchAddRequest struct {
	Todos []struct {
		Title       string `json:"title" jsonschema:"description=待办事项标题,required"`
		Description string `json:"description,omitempty" jsonschema:"description=待办事项详细描述"`
		Priority    string `json:"priority,omitempty" jsonschema:"description=优先级: low(低), medium(中), high(高), urgent(紧急)"`
	} `json:"todos" jsonschema:"description=待办事项列表,required"`
}

// TodoBatchAddResponse 批量添加待办事项响应
type TodoBatchAddResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	AddedTodos   []Todo `json:"added_todos,omitempty"`
	AddedCount   int    `json:"added_count"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// TodoBatchAdd 批量添加待办事项
func (tt *TodoTools) TodoBatchAdd(ctx context.Context, req *TodoBatchAddRequest) (*TodoBatchAddResponse, error) {
	if len(req.Todos) == 0 {
		return &TodoBatchAddResponse{
			Success:      false,
			ErrorMessage: "待办事项列表不能为空",
		}, nil
	}

	// 验证所有待办事项
	for i, todoReq := range req.Todos {
		if todoReq.Title == "" {
			return &TodoBatchAddResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("第 %d 个待办事项标题不能为空", i+1),
			}, nil
		}
		if todoReq.Priority != "" && todoReq.Priority != "low" && todoReq.Priority != "medium" && todoReq.Priority != "high" && todoReq.Priority != "urgent" {
			return &TodoBatchAddResponse{
				Success:      false,
				ErrorMessage: fmt.Sprintf("第 %d 个待办事项优先级必须是 low, medium, high 或 urgent 之一", i+1),
			}, nil
		}
	}

	list, err := tt.loadTodoList()
	if err != nil {
		return &TodoBatchAddResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("加载待办列表失败: %v", err),
		}, nil
	}

	now := time.Now()
	var addedTodos []Todo

	for _, todoReq := range req.Todos {
		todo := Todo{
			ID:          generateID(),
			Title:       todoReq.Title,
			Description: todoReq.Description,
			Status:      "pending",
			Priority:    todoReq.Priority,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		list.Todos = append(list.Todos, todo)
		addedTodos = append(addedTodos, todo)

		// 添加微小延迟以确保 ID 唯一
		time.Sleep(time.Microsecond)
	}

	if err := tt.saveTodoList(list); err != nil {
		return &TodoBatchAddResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("保存待办列表失败: %v", err),
		}, nil
	}

	return &TodoBatchAddResponse{
		Success:    true,
		Message:    fmt.Sprintf("成功添加 %d 个待办事项", len(addedTodos)),
		AddedTodos: addedTodos,
		AddedCount: len(addedTodos),
	}, nil
}

// TodoBatchDeleteRequest 批量删除待办事项请求
type TodoBatchDeleteRequest struct {
	IDs []string `json:"ids" jsonschema:"description=待办事项ID列表,required"`
}

// TodoBatchDeleteResponse 批量删除待办事项响应
type TodoBatchDeleteResponse struct {
	Success      bool     `json:"success"`
	Message      string   `json:"message"`
	DeletedCount int      `json:"deleted_count"`
	NotFoundIDs  []string `json:"not_found_ids,omitempty"`
	ErrorMessage string   `json:"error_message,omitempty"`
}

// TodoBatchDelete 批量删除待办事项
func (tt *TodoTools) TodoBatchDelete(ctx context.Context, req *TodoBatchDeleteRequest) (*TodoBatchDeleteResponse, error) {
	if len(req.IDs) == 0 {
		return &TodoBatchDeleteResponse{
			Success:      false,
			ErrorMessage: "ID 列表不能为空",
		}, nil
	}

	list, err := tt.loadTodoList()
	if err != nil {
		return &TodoBatchDeleteResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("加载待办列表失败: %v", err),
		}, nil
	}

	// 创建 ID 集合用于快速查找
	idSet := make(map[string]bool)
	for _, id := range req.IDs {
		idSet[id] = true
	}

	// 过滤掉要删除的待办事项
	var newTodos []Todo
	var notFoundIDs []string
	deletedCount := 0

	for _, todo := range list.Todos {
		if idSet[todo.ID] {
			deletedCount++
			delete(idSet, todo.ID)
		} else {
			newTodos = append(newTodos, todo)
		}
	}

	// 记录未找到的 ID
	for id := range idSet {
		notFoundIDs = append(notFoundIDs, id)
	}

	list.Todos = newTodos

	if err := tt.saveTodoList(list); err != nil {
		return &TodoBatchDeleteResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("保存待办列表失败: %v", err),
		}, nil
	}

	message := fmt.Sprintf("成功删除 %d 个待办事项", deletedCount)
	if len(notFoundIDs) > 0 {
		message += fmt.Sprintf("，%d 个 ID 未找到", len(notFoundIDs))
	}

	return &TodoBatchDeleteResponse{
		Success:      true,
		Message:      message,
		DeletedCount: deletedCount,
		NotFoundIDs:  notFoundIDs,
	}, nil
}

// TodoClearRequest 清空待办事项请求
type TodoClearRequest struct {
	Status string `json:"status,omitempty" jsonschema:"description=仅清空指定状态的待办事项: pending, in_progress, completed, cancelled。不指定则清空所有"`
}

// TodoClearResponse 清空待办事项响应
type TodoClearResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ClearedCount int    `json:"cleared_count"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// TodoClear 清空待办事项
func (tt *TodoTools) TodoClear(ctx context.Context, req *TodoClearRequest) (*TodoClearResponse, error) {
	// 验证状态
	if req.Status != "" && req.Status != "pending" && req.Status != "in_progress" && req.Status != "completed" && req.Status != "cancelled" {
		return &TodoClearResponse{
			Success:      false,
			ErrorMessage: "状态必须是 pending, in_progress, completed 或 cancelled 之一",
		}, nil
	}

	list, err := tt.loadTodoList()
	if err != nil {
		return &TodoClearResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("加载待办列表失败: %v", err),
		}, nil
	}

	originalCount := len(list.Todos)

	// 如果指定了状态，只清空该状态的待办事项
	if req.Status != "" {
		var remainingTodos []Todo
		for _, todo := range list.Todos {
			if todo.Status != req.Status {
				remainingTodos = append(remainingTodos, todo)
			}
		}
		list.Todos = remainingTodos
	} else {
		// 清空所有待办事项
		list.Todos = []Todo{}
	}

	clearedCount := originalCount - len(list.Todos)

	if err := tt.saveTodoList(list); err != nil {
		return &TodoClearResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("保存待办列表失败: %v", err),
		}, nil
	}

	message := fmt.Sprintf("成功清空 %d 个待办事项", clearedCount)
	if req.Status != "" {
		message = fmt.Sprintf("成功清空 %d 个状态为 %s 的待办事项", clearedCount, req.Status)
	}

	return &TodoClearResponse{
		Success:      true,
		Message:      message,
		ClearedCount: clearedCount,
	}, nil
}
