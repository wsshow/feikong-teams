package excel

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ========== 工作表操作 ==========

// CreateSheetRequest 创建工作表请求
type CreateSheetRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
}

// CreateSheetResponse 创建工作表响应
type CreateSheetResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	SheetIndex   int    `json:"sheet_index,omitempty" jsonschema:"description=工作表索引"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// CreateSheet 创建新工作表
func (et *ExcelTools) CreateSheet(ctx context.Context, req *CreateSheetRequest) (*CreateSheetResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &CreateSheetResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &CreateSheetResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &CreateSheetResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	index, err := f.NewSheet(req.SheetName)
	if err != nil {
		return &CreateSheetResponse{
			ErrorMessage: fmt.Sprintf("创建工作表失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &CreateSheetResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &CreateSheetResponse{
		Message:    fmt.Sprintf("成功创建工作表: %s", req.SheetName),
		SheetIndex: index,
	}, nil
}

// DeleteSheetRequest 删除工作表请求
type DeleteSheetRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
}

// DeleteSheetResponse 删除工作表响应
type DeleteSheetResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// DeleteSheet 删除工作表
func (et *ExcelTools) DeleteSheet(ctx context.Context, req *DeleteSheetRequest) (*DeleteSheetResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &DeleteSheetResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &DeleteSheetResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &DeleteSheetResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	f.DeleteSheet(req.SheetName)

	if err := f.Save(); err != nil {
		return &DeleteSheetResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &DeleteSheetResponse{
		Message: fmt.Sprintf("成功删除工作表: %s", req.SheetName),
	}, nil
}

// RenameSheetRequest 重命名工作表请求
type RenameSheetRequest struct {
	Path    string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	OldName string `json:"old_name" jsonschema:"description=原工作表名称,required"`
	NewName string `json:"new_name" jsonschema:"description=新工作表名称,required"`
}

// RenameSheetResponse 重命名工作表响应
type RenameSheetResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// RenameSheet 重命名工作表
func (et *ExcelTools) RenameSheet(ctx context.Context, req *RenameSheetRequest) (*RenameSheetResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &RenameSheetResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.OldName == "" || req.NewName == "" {
		return &RenameSheetResponse{
			ErrorMessage: "原名称和新名称不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &RenameSheetResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.SetSheetName(req.OldName, req.NewName); err != nil {
		return &RenameSheetResponse{
			ErrorMessage: fmt.Sprintf("重命名失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &RenameSheetResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &RenameSheetResponse{
		Message: fmt.Sprintf("成功将工作表 %s 重命名为 %s", req.OldName, req.NewName),
	}, nil
}

// CopySheetRequest 复制工作表请求
type CopySheetRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	FromSheet string `json:"from_sheet" jsonschema:"description=源工作表名称,required"`
	ToSheet   string `json:"to_sheet" jsonschema:"description=目标工作表名称,required"`
}

// CopySheetResponse 复制工作表响应
type CopySheetResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// CopySheet 复制工作表
func (et *ExcelTools) CopySheet(ctx context.Context, req *CopySheetRequest) (*CopySheetResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &CopySheetResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.FromSheet == "" || req.ToSheet == "" {
		return &CopySheetResponse{
			ErrorMessage: "源工作表和目标工作表名称不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &CopySheetResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	fromIndex, err := f.GetSheetIndex(req.FromSheet)
	if err != nil {
		return &CopySheetResponse{
			ErrorMessage: fmt.Sprintf("获取源工作表索引失败: %v", err),
		}, nil
	}

	toIndex, err := f.NewSheet(req.ToSheet)
	if err != nil {
		return &CopySheetResponse{
			ErrorMessage: fmt.Sprintf("创建目标工作表失败: %v", err),
		}, nil
	}

	if err := f.CopySheet(fromIndex, toIndex); err != nil {
		return &CopySheetResponse{
			ErrorMessage: fmt.Sprintf("复制工作表失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &CopySheetResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &CopySheetResponse{
		Message: fmt.Sprintf("成功将工作表 %s 复制为 %s", req.FromSheet, req.ToSheet),
	}, nil
}

// SetActiveSheetRequest 设置活动工作表请求
type SetActiveSheetRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
}

// SetActiveSheetResponse 设置活动工作表响应
type SetActiveSheetResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetActiveSheet 设置活动工作表
func (et *ExcelTools) SetActiveSheet(ctx context.Context, req *SetActiveSheetRequest) (*SetActiveSheetResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetActiveSheetResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &SetActiveSheetResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetActiveSheetResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	index, err := f.GetSheetIndex(req.SheetName)
	if err != nil {
		return &SetActiveSheetResponse{
			ErrorMessage: fmt.Sprintf("获取工作表索引失败: %v", err),
		}, nil
	}

	f.SetActiveSheet(index)

	if err := f.Save(); err != nil {
		return &SetActiveSheetResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &SetActiveSheetResponse{
		Message: fmt.Sprintf("成功设置活动工作表: %s", req.SheetName),
	}, nil
}

// SetSheetVisibleRequest 设置工作表可见性请求
type SetSheetVisibleRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Visible   bool   `json:"visible" jsonschema:"description=是否可见,required"`
}

// SetSheetVisibleResponse 设置工作表可见性响应
type SetSheetVisibleResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetSheetVisible 设置工作表可见性
func (et *ExcelTools) SetSheetVisible(ctx context.Context, req *SetSheetVisibleRequest) (*SetSheetVisibleResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetSheetVisibleResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &SetSheetVisibleResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetSheetVisibleResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.SetSheetVisible(req.SheetName, req.Visible); err != nil {
		return &SetSheetVisibleResponse{
			ErrorMessage: fmt.Sprintf("设置可见性失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &SetSheetVisibleResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	visibleStatus := "隐藏"
	if req.Visible {
		visibleStatus = "显示"
	}

	return &SetSheetVisibleResponse{
		Message: fmt.Sprintf("成功设置工作表 %s 为%s", req.SheetName, visibleStatus),
	}, nil
}
