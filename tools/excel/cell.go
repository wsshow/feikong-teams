package excel

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ========== 单元格操作 ==========

// SetCellValueRequest 设置单元格值请求
type SetCellValueRequest struct {
	Path      string      `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string      `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Cell      string      `json:"cell" jsonschema:"description=单元格坐标(如A1),required"`
	Value     interface{} `json:"value" jsonschema:"description=单元格值,required"`
}

// SetCellValueResponse 设置单元格值响应
type SetCellValueResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetCellValue 设置单元格值
func (et *ExcelTools) SetCellValue(ctx context.Context, req *SetCellValueRequest) (*SetCellValueResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetCellValueResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Cell == "" {
		return &SetCellValueResponse{
			ErrorMessage: "工作表名称和单元格坐标不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetCellValueResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.SetCellValue(req.SheetName, req.Cell, req.Value); err != nil {
		return &SetCellValueResponse{
			ErrorMessage: fmt.Sprintf("设置单元格值失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &SetCellValueResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &SetCellValueResponse{
		Message: fmt.Sprintf("成功设置单元格 %s 的值", req.Cell),
	}, nil
}

// GetCellValueRequest 获取单元格值请求
type GetCellValueRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Cell      string `json:"cell" jsonschema:"description=单元格坐标(如A1),required"`
}

// GetCellValueResponse 获取单元格值响应
type GetCellValueResponse struct {
	Value        string `json:"value,omitempty" jsonschema:"description=单元格值"`
	Message      string `json:"message,omitempty" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetCellValue 获取单元格值
func (et *ExcelTools) GetCellValue(ctx context.Context, req *GetCellValueRequest) (*GetCellValueResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &GetCellValueResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Cell == "" {
		return &GetCellValueResponse{
			ErrorMessage: "工作表名称和单元格坐标不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &GetCellValueResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	value, err := f.GetCellValue(req.SheetName, req.Cell)
	if err != nil {
		return &GetCellValueResponse{
			ErrorMessage: fmt.Sprintf("获取单元格值失败: %v", err),
		}, nil
	}

	return &GetCellValueResponse{
		Value:   value,
		Message: fmt.Sprintf("成功获取单元格 %s 的值", req.Cell),
	}, nil
}

// SetCellFormulaRequest 设置单元格公式请求
type SetCellFormulaRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Cell      string `json:"cell" jsonschema:"description=单元格坐标(如A1),required"`
	Formula   string `json:"formula" jsonschema:"description=公式(如SUM(A1:A10)),required"`
}

// SetCellFormulaResponse 设置单元格公式响应
type SetCellFormulaResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetCellFormula 设置单元格公式
func (et *ExcelTools) SetCellFormula(ctx context.Context, req *SetCellFormulaRequest) (*SetCellFormulaResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetCellFormulaResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Cell == "" || req.Formula == "" {
		return &SetCellFormulaResponse{
			ErrorMessage: "工作表名称、单元格坐标和公式不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetCellFormulaResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.SetCellFormula(req.SheetName, req.Cell, req.Formula); err != nil {
		return &SetCellFormulaResponse{
			ErrorMessage: fmt.Sprintf("设置公式失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &SetCellFormulaResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &SetCellFormulaResponse{
		Message: fmt.Sprintf("成功设置单元格 %s 的公式", req.Cell),
	}, nil
}

// MergeCellsRequest 合并单元格请求
type MergeCellsRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	StartCell string `json:"start_cell" jsonschema:"description=起始单元格(如A1),required"`
	EndCell   string `json:"end_cell" jsonschema:"description=结束单元格(如B2),required"`
}

// MergeCellsResponse 合并单元格响应
type MergeCellsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// MergeCells 合并单元格
func (et *ExcelTools) MergeCells(ctx context.Context, req *MergeCellsRequest) (*MergeCellsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &MergeCellsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.StartCell == "" || req.EndCell == "" {
		return &MergeCellsResponse{
			ErrorMessage: "工作表名称、起始单元格和结束单元格不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &MergeCellsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.MergeCell(req.SheetName, req.StartCell, req.EndCell); err != nil {
		return &MergeCellsResponse{
			ErrorMessage: fmt.Sprintf("合并单元格失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &MergeCellsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &MergeCellsResponse{
		Message: fmt.Sprintf("成功合并单元格 %s:%s", req.StartCell, req.EndCell),
	}, nil
}

// UnmergeCellsRequest 取消合并单元格请求
type UnmergeCellsRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	StartCell string `json:"start_cell" jsonschema:"description=起始单元格(如A1),required"`
	EndCell   string `json:"end_cell" jsonschema:"description=结束单元格(如B2),required"`
}

// UnmergeCellsResponse 取消合并单元格响应
type UnmergeCellsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// UnmergeCells 取消合并单元格
func (et *ExcelTools) UnmergeCells(ctx context.Context, req *UnmergeCellsRequest) (*UnmergeCellsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &UnmergeCellsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.StartCell == "" || req.EndCell == "" {
		return &UnmergeCellsResponse{
			ErrorMessage: "工作表名称、起始单元格和结束单元格不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &UnmergeCellsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.UnmergeCell(req.SheetName, req.StartCell, req.EndCell); err != nil {
		return &UnmergeCellsResponse{
			ErrorMessage: fmt.Sprintf("取消合并失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &UnmergeCellsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &UnmergeCellsResponse{
		Message: fmt.Sprintf("成功取消合并单元格 %s:%s", req.StartCell, req.EndCell),
	}, nil
}

// SetCellStyleRequest 设置单元格样式请求
type SetCellStyleRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	StartCell string `json:"start_cell" jsonschema:"description=起始单元格(如A1),required"`
	EndCell   string `json:"end_cell" jsonschema:"description=结束单元格(如B2),required"`
	StyleID   int    `json:"style_id" jsonschema:"description=样式ID(通过CreateStyle获得),required"`
}

// SetCellStyleResponse 设置单元格样式响应
type SetCellStyleResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetCellStyle 设置单元格样式
func (et *ExcelTools) SetCellStyle(ctx context.Context, req *SetCellStyleRequest) (*SetCellStyleResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetCellStyleResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.StartCell == "" || req.EndCell == "" {
		return &SetCellStyleResponse{
			ErrorMessage: "工作表名称、起始单元格和结束单元格不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetCellStyleResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.SetCellStyle(req.SheetName, req.StartCell, req.EndCell, req.StyleID); err != nil {
		return &SetCellStyleResponse{
			ErrorMessage: fmt.Sprintf("设置样式失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &SetCellStyleResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &SetCellStyleResponse{
		Message: fmt.Sprintf("成功设置单元格样式 %s:%s", req.StartCell, req.EndCell),
	}, nil
}
