package excel

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ========== 批量操作 ==========

// BatchCreateSheetsRequest 批量创建工作表请求
type BatchCreateSheetsRequest struct {
	Path       string   `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetNames []string `json:"sheet_names" jsonschema:"description=工作表名称列表,required"`
}

// BatchCreateSheetsResponse 批量创建工作表响应
type BatchCreateSheetsResponse struct {
	Message      string         `json:"message" jsonschema:"description=操作结果消息"`
	Success      []SheetResult  `json:"success,omitempty" jsonschema:"description=成功创建的工作表"`
	Failed       []FailedResult `json:"failed,omitempty" jsonschema:"description=创建失败的工作表"`
	ErrorMessage string         `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SheetResult 工作表操作结果
type SheetResult struct {
	SheetName  string `json:"sheet_name" jsonschema:"description=工作表名称"`
	SheetIndex int    `json:"sheet_index,omitempty" jsonschema:"description=工作表索引"`
}

// FailedResult 失败结果
type FailedResult struct {
	Item  string `json:"item" jsonschema:"description=失败的项目"`
	Error string `json:"error" jsonschema:"description=错误原因"`
}

// BatchCreateSheets 批量创建工作表
func (et *ExcelTools) BatchCreateSheets(ctx context.Context, req *BatchCreateSheetsRequest) (*BatchCreateSheetsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchCreateSheetsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if len(req.SheetNames) == 0 {
		return &BatchCreateSheetsResponse{
			ErrorMessage: "工作表名称列表不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchCreateSheetsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	var success []SheetResult
	var failed []FailedResult

	for _, sheetName := range req.SheetNames {
		if sheetName == "" {
			failed = append(failed, FailedResult{
				Item:  sheetName,
				Error: "工作表名称不能为空",
			})
			continue
		}

		index, err := f.NewSheet(sheetName)
		if err != nil {
			failed = append(failed, FailedResult{
				Item:  sheetName,
				Error: err.Error(),
			})
			continue
		}

		success = append(success, SheetResult{
			SheetName:  sheetName,
			SheetIndex: index,
		})
	}

	if err := f.Save(); err != nil {
		return &BatchCreateSheetsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchCreateSheetsResponse{
		Message: fmt.Sprintf("批量创建工作表完成：成功 %d 个，失败 %d 个", len(success), len(failed)),
		Success: success,
		Failed:  failed,
	}, nil
}

// BatchDeleteSheetsRequest 批量删除工作表请求
type BatchDeleteSheetsRequest struct {
	Path       string   `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetNames []string `json:"sheet_names" jsonschema:"description=要删除的工作表名称列表,required"`
}

// BatchDeleteSheetsResponse 批量删除工作表响应
type BatchDeleteSheetsResponse struct {
	Message      string         `json:"message" jsonschema:"description=操作结果消息"`
	Success      []string       `json:"success,omitempty" jsonschema:"description=成功删除的工作表"`
	Failed       []FailedResult `json:"failed,omitempty" jsonschema:"description=删除失败的工作表"`
	ErrorMessage string         `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchDeleteSheets 批量删除工作表
func (et *ExcelTools) BatchDeleteSheets(ctx context.Context, req *BatchDeleteSheetsRequest) (*BatchDeleteSheetsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchDeleteSheetsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if len(req.SheetNames) == 0 {
		return &BatchDeleteSheetsResponse{
			ErrorMessage: "工作表名称列表不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchDeleteSheetsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	var success []string
	var failed []FailedResult

	for _, sheetName := range req.SheetNames {
		if sheetName == "" {
			failed = append(failed, FailedResult{
				Item:  sheetName,
				Error: "工作表名称不能为空",
			})
			continue
		}

		if err := f.DeleteSheet(sheetName); err != nil {
			failed = append(failed, FailedResult{
				Item:  sheetName,
				Error: err.Error(),
			})
			continue
		}

		success = append(success, sheetName)
	}

	if err := f.Save(); err != nil {
		return &BatchDeleteSheetsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchDeleteSheetsResponse{
		Message: fmt.Sprintf("批量删除工作表完成：成功 %d 个，失败 %d 个", len(success), len(failed)),
		Success: success,
		Failed:  failed,
	}, nil
}

// CellValue 单元格值定义
type CellValue struct {
	Cell  string      `json:"cell" jsonschema:"description=单元格坐标(如A1),required"`
	Value interface{} `json:"value" jsonschema:"description=单元格值,required"`
}

// BatchSetCellValuesRequest 批量设置单元格值请求
type BatchSetCellValuesRequest struct {
	Path      string      `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string      `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Values    []CellValue `json:"values" jsonschema:"description=单元格值列表,required"`
}

// BatchSetCellValuesResponse 批量设置单元格值响应
type BatchSetCellValuesResponse struct {
	Message      string         `json:"message" jsonschema:"description=操作结果消息"`
	Success      []string       `json:"success,omitempty" jsonschema:"description=成功设置的单元格"`
	Failed       []FailedResult `json:"failed,omitempty" jsonschema:"description=设置失败的单元格"`
	ErrorMessage string         `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchSetCellValues 批量设置单元格值
func (et *ExcelTools) BatchSetCellValues(ctx context.Context, req *BatchSetCellValuesRequest) (*BatchSetCellValuesResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchSetCellValuesResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &BatchSetCellValuesResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	if len(req.Values) == 0 {
		return &BatchSetCellValuesResponse{
			ErrorMessage: "单元格值列表不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchSetCellValuesResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	var success []string
	var failed []FailedResult

	for _, cv := range req.Values {
		if cv.Cell == "" {
			failed = append(failed, FailedResult{
				Item:  cv.Cell,
				Error: "单元格坐标不能为空",
			})
			continue
		}

		if err := f.SetCellValue(req.SheetName, cv.Cell, cv.Value); err != nil {
			failed = append(failed, FailedResult{
				Item:  cv.Cell,
				Error: err.Error(),
			})
			continue
		}

		success = append(success, cv.Cell)
	}

	if err := f.Save(); err != nil {
		return &BatchSetCellValuesResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchSetCellValuesResponse{
		Message: fmt.Sprintf("批量设置单元格完成：成功 %d 个，失败 %d 个", len(success), len(failed)),
		Success: success,
		Failed:  failed,
	}, nil
}

// BatchFillRowsRequest 批量填充行请求
type BatchFillRowsRequest struct {
	Path      string     `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string     `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	StartRow  int        `json:"start_row" jsonschema:"description=起始行号(从1开始),required"`
	Data      [][]string `json:"data" jsonschema:"description=二维数组数据，每个元素是一行数据,required"`
}

// BatchFillRowsResponse 批量填充行响应
type BatchFillRowsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	RowsFilled   int    `json:"rows_filled" jsonschema:"description=填充的行数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchFillRows 批量填充行
func (et *ExcelTools) BatchFillRows(ctx context.Context, req *BatchFillRowsRequest) (*BatchFillRowsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchFillRowsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &BatchFillRowsResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	if req.StartRow < 1 {
		return &BatchFillRowsResponse{
			ErrorMessage: "起始行号必须大于0",
		}, nil
	}

	if len(req.Data) == 0 {
		return &BatchFillRowsResponse{
			ErrorMessage: "数据不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchFillRowsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	rowsFilled := 0
	for i, rowData := range req.Data {
		currentRow := req.StartRow + i
		for j, value := range rowData {
			// 将列索引转换为列名（A, B, C...）
			colName, err := excelize.ColumnNumberToName(j + 1)
			if err != nil {
				continue
			}

			cell := fmt.Sprintf("%s%d", colName, currentRow)
			if err := f.SetCellValue(req.SheetName, cell, value); err != nil {
				continue
			}
		}
		rowsFilled++
	}

	if err := f.Save(); err != nil {
		return &BatchFillRowsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchFillRowsResponse{
		Message:    fmt.Sprintf("成功填充 %d 行数据", rowsFilled),
		RowsFilled: rowsFilled,
	}, nil
}

// BatchFillColsRequest 批量填充列请求
type BatchFillColsRequest struct {
	Path      string              `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string              `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	StartRow  int                 `json:"start_row" jsonschema:"description=起始行号(从1开始),required"`
	Columns   map[string][]string `json:"columns" jsonschema:"description=列数据，key为列名(如A,B,C)，value为该列的数据数组,required"`
}

// BatchFillColsResponse 批量填充列响应
type BatchFillColsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ColsFilled   int    `json:"cols_filled" jsonschema:"description=填充的列数"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchFillCols 批量填充列
func (et *ExcelTools) BatchFillCols(ctx context.Context, req *BatchFillColsRequest) (*BatchFillColsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchFillColsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &BatchFillColsResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	if req.StartRow < 1 {
		return &BatchFillColsResponse{
			ErrorMessage: "起始行号必须大于0",
		}, nil
	}

	if len(req.Columns) == 0 {
		return &BatchFillColsResponse{
			ErrorMessage: "列数据不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchFillColsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	colsFilled := 0
	for colName, colData := range req.Columns {
		for i, value := range colData {
			cell := fmt.Sprintf("%s%d", colName, req.StartRow+i)
			if err := f.SetCellValue(req.SheetName, cell, value); err != nil {
				continue
			}
		}
		colsFilled++
	}

	if err := f.Save(); err != nil {
		return &BatchFillColsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchFillColsResponse{
		Message:    fmt.Sprintf("成功填充 %d 列数据", colsFilled),
		ColsFilled: colsFilled,
	}, nil
}

// BatchInsertRowsRequest 批量插入行请求
type BatchInsertRowsRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	StartRow  int    `json:"start_row" jsonschema:"description=开始插入的行号(从1开始),required"`
	Count     int    `json:"count" jsonschema:"description=要插入的行数,required"`
}

// BatchInsertRowsResponse 批量插入行响应
type BatchInsertRowsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchInsertRows 批量插入行
func (et *ExcelTools) BatchInsertRows(ctx context.Context, req *BatchInsertRowsRequest) (*BatchInsertRowsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchInsertRowsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &BatchInsertRowsResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	if req.StartRow < 1 {
		return &BatchInsertRowsResponse{
			ErrorMessage: "起始行号必须大于0",
		}, nil
	}

	if req.Count < 1 {
		return &BatchInsertRowsResponse{
			ErrorMessage: "插入行数必须大于0",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchInsertRowsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.InsertRows(req.SheetName, req.StartRow, req.Count); err != nil {
		return &BatchInsertRowsResponse{
			ErrorMessage: fmt.Sprintf("插入行失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &BatchInsertRowsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchInsertRowsResponse{
		Message: fmt.Sprintf("成功在第 %d 行插入 %d 行", req.StartRow, req.Count),
	}, nil
}

// BatchRemoveRowsRequest 批量删除行请求
type BatchRemoveRowsRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	StartRow  int    `json:"start_row" jsonschema:"description=开始删除的行号(从1开始),required"`
	Count     int    `json:"count" jsonschema:"description=要删除的行数,required"`
}

// BatchRemoveRowsResponse 批量删除行响应
type BatchRemoveRowsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchRemoveRows 批量删除行
func (et *ExcelTools) BatchRemoveRows(ctx context.Context, req *BatchRemoveRowsRequest) (*BatchRemoveRowsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchRemoveRowsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &BatchRemoveRowsResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	if req.StartRow < 1 {
		return &BatchRemoveRowsResponse{
			ErrorMessage: "起始行号必须大于0",
		}, nil
	}

	if req.Count < 1 {
		return &BatchRemoveRowsResponse{
			ErrorMessage: "删除行数必须大于0",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchRemoveRowsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	// RemoveRow 每次只能删除一行，需要循环删除
	for i := 0; i < req.Count; i++ {
		// 每次都删除 StartRow，因为删除后后面的行会自动上移
		if err := f.RemoveRow(req.SheetName, req.StartRow); err != nil {
			return &BatchRemoveRowsResponse{
				ErrorMessage: fmt.Sprintf("删除行失败: %v", err),
			}, nil
		}
	}

	if err := f.Save(); err != nil {
		return &BatchRemoveRowsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchRemoveRowsResponse{
		Message: fmt.Sprintf("成功从第 %d 行删除 %d 行", req.StartRow, req.Count),
	}, nil
}

// BatchInsertColsRequest 批量插入列请求
type BatchInsertColsRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Col       string `json:"col" jsonschema:"description=开始插入的列名(如A),required"`
	Count     int    `json:"count" jsonschema:"description=要插入的列数,required"`
}

// BatchInsertColsResponse 批量插入列响应
type BatchInsertColsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchInsertCols 批量插入列
func (et *ExcelTools) BatchInsertCols(ctx context.Context, req *BatchInsertColsRequest) (*BatchInsertColsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchInsertColsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Col == "" {
		return &BatchInsertColsResponse{
			ErrorMessage: "工作表名称和列名不能为空",
		}, nil
	}

	if req.Count < 1 {
		return &BatchInsertColsResponse{
			ErrorMessage: "插入列数必须大于0",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchInsertColsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.InsertCols(req.SheetName, req.Col, req.Count); err != nil {
		return &BatchInsertColsResponse{
			ErrorMessage: fmt.Sprintf("插入列失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &BatchInsertColsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchInsertColsResponse{
		Message: fmt.Sprintf("成功在列 %s 插入 %d 列", req.Col, req.Count),
	}, nil
}

// BatchRemoveColsRequest 批量删除列请求
type BatchRemoveColsRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Col       string `json:"col" jsonschema:"description=开始删除的列名(如A),required"`
	Count     int    `json:"count" jsonschema:"description=要删除的列数,required"`
}

// BatchRemoveColsResponse 批量删除列响应
type BatchRemoveColsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchRemoveCols 批量删除列
func (et *ExcelTools) BatchRemoveCols(ctx context.Context, req *BatchRemoveColsRequest) (*BatchRemoveColsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchRemoveColsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Col == "" {
		return &BatchRemoveColsResponse{
			ErrorMessage: "工作表名称和列名不能为空",
		}, nil
	}

	if req.Count < 1 {
		return &BatchRemoveColsResponse{
			ErrorMessage: "删除列数必须大于0",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchRemoveColsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	// RemoveCol 每次只能删除一列，需要循环删除
	for i := 0; i < req.Count; i++ {
		// 每次都删除同一列名，因为删除后后面的列会自动左移
		if err := f.RemoveCol(req.SheetName, req.Col); err != nil {
			return &BatchRemoveColsResponse{
				ErrorMessage: fmt.Sprintf("删除列失败: %v", err),
			}, nil
		}
	}

	if err := f.Save(); err != nil {
		return &BatchRemoveColsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchRemoveColsResponse{
		Message: fmt.Sprintf("成功从列 %s 删除 %d 列", req.Col, req.Count),
	}, nil
}

// CellStyle 单元格样式定义
type CellStyle struct {
	Cell    string `json:"cell" jsonschema:"description=单元格坐标或范围(如A1或A1:B2),required"`
	StyleID int    `json:"style_id" jsonschema:"description=样式ID(使用excel_create_style创建),required"`
}

// BatchSetCellStylesRequest 批量设置单元格样式请求
type BatchSetCellStylesRequest struct {
	Path      string      `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string      `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Styles    []CellStyle `json:"styles" jsonschema:"description=单元格样式列表,required"`
}

// BatchSetCellStylesResponse 批量设置单元格样式响应
type BatchSetCellStylesResponse struct {
	Message      string         `json:"message" jsonschema:"description=操作结果消息"`
	Success      []string       `json:"success,omitempty" jsonschema:"description=成功设置的单元格"`
	Failed       []FailedResult `json:"failed,omitempty" jsonschema:"description=设置失败的单元格"`
	ErrorMessage string         `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// BatchSetCellStyles 批量设置单元格样式
func (et *ExcelTools) BatchSetCellStyles(ctx context.Context, req *BatchSetCellStylesRequest) (*BatchSetCellStylesResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &BatchSetCellStylesResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &BatchSetCellStylesResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	if len(req.Styles) == 0 {
		return &BatchSetCellStylesResponse{
			ErrorMessage: "样式列表不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &BatchSetCellStylesResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	var success []string
	var failed []FailedResult

	for _, cs := range req.Styles {
		if cs.Cell == "" {
			failed = append(failed, FailedResult{
				Item:  cs.Cell,
				Error: "单元格坐标不能为空",
			})
			continue
		}

		if err := f.SetCellStyle(req.SheetName, cs.Cell, cs.Cell, cs.StyleID); err != nil {
			failed = append(failed, FailedResult{
				Item:  cs.Cell,
				Error: err.Error(),
			})
			continue
		}

		success = append(success, cs.Cell)
	}

	if err := f.Save(); err != nil {
		return &BatchSetCellStylesResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &BatchSetCellStylesResponse{
		Message: fmt.Sprintf("批量设置单元格样式完成：成功 %d 个，失败 %d 个", len(success), len(failed)),
		Success: success,
		Failed:  failed,
	}, nil
}
