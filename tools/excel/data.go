package excel

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ========== 数据操作 ==========

// GetRowsRequest 获取所有行请求
type GetRowsRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	MaxRows   int    `json:"max_rows,omitempty" jsonschema:"description=最大返回行数(默认100)"`
}

// GetRowsResponse 获取所有行响应
type GetRowsResponse struct {
	Rows         [][]string `json:"rows,omitempty" jsonschema:"description=行数据"`
	RowCount     int        `json:"row_count" jsonschema:"description=实际返回的行数"`
	Message      string     `json:"message,omitempty" jsonschema:"description=操作结果消息"`
	ErrorMessage string     `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetRows 获取工作表的所有行
func (et *ExcelTools) GetRows(ctx context.Context, req *GetRowsRequest) (*GetRowsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &GetRowsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &GetRowsResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &GetRowsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	rows, err := f.GetRows(req.SheetName)
	if err != nil {
		return &GetRowsResponse{
			ErrorMessage: fmt.Sprintf("获取行数据失败: %v", err),
		}, nil
	}

	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = 100
	}

	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}

	return &GetRowsResponse{
		Rows:     rows,
		RowCount: len(rows),
		Message:  fmt.Sprintf("成功获取 %d 行数据", len(rows)),
	}, nil
}

// GetRowRequest 获取单行请求
type GetRowRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Row       int    `json:"row" jsonschema:"description=行号(从1开始),required"`
}

// GetRowResponse 获取单行响应
type GetRowResponse struct {
	Row          []string `json:"row,omitempty" jsonschema:"description=行数据"`
	Message      string   `json:"message,omitempty" jsonschema:"description=操作结果消息"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetRow 获取指定行的数据
func (et *ExcelTools) GetRow(ctx context.Context, req *GetRowRequest) (*GetRowResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &GetRowResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Row <= 0 {
		return &GetRowResponse{
			ErrorMessage: "工作表名称不能为空，行号必须大于0",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &GetRowResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	rows, err := f.GetRows(req.SheetName)
	if err != nil {
		return &GetRowResponse{
			ErrorMessage: fmt.Sprintf("获取行数据失败: %v", err),
		}, nil
	}

	if req.Row > len(rows) {
		return &GetRowResponse{
			ErrorMessage: fmt.Sprintf("行号 %d 超出范围，工作表只有 %d 行", req.Row, len(rows)),
		}, nil
	}

	return &GetRowResponse{
		Row:     rows[req.Row-1],
		Message: fmt.Sprintf("成功获取第 %d 行数据", req.Row),
	}, nil
}

// GetColRequest 获取单列请求
type GetColRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Col       string `json:"col" jsonschema:"description=列名(如A),required"`
	MaxRows   int    `json:"max_rows,omitempty" jsonschema:"description=最大返回行数(默认100)"`
}

// GetColResponse 获取单列响应
type GetColResponse struct {
	Col          []string `json:"col,omitempty" jsonschema:"description=列数据"`
	Message      string   `json:"message,omitempty" jsonschema:"description=操作结果消息"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetCol 获取指定列的数据
func (et *ExcelTools) GetCol(ctx context.Context, req *GetColRequest) (*GetColResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &GetColResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Col == "" {
		return &GetColResponse{
			ErrorMessage: "工作表名称和列名不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &GetColResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	cols, err := f.GetCols(req.SheetName)
	if err != nil {
		return &GetColResponse{
			ErrorMessage: fmt.Sprintf("获取列数据失败: %v", err),
		}, nil
	}

	// 将列名转换为索引
	colIndex, err := excelize.ColumnNameToNumber(req.Col)
	if err != nil {
		return &GetColResponse{
			ErrorMessage: fmt.Sprintf("列名无效: %v", err),
		}, nil
	}

	if colIndex > len(cols) {
		return &GetColResponse{
			ErrorMessage: fmt.Sprintf("列 %s 不存在", req.Col),
		}, nil
	}

	colData := cols[colIndex-1]

	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = 100
	}

	if len(colData) > maxRows {
		colData = colData[:maxRows]
	}

	return &GetColResponse{
		Col:     colData,
		Message: fmt.Sprintf("成功获取列 %s 的数据，共 %d 个单元格", req.Col, len(colData)),
	}, nil
}

// GetSheetDataRequest 获取工作表所有数据请求
type GetSheetDataRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	MaxRows   int    `json:"max_rows,omitempty" jsonschema:"description=最大返回行数(默认1000)"`
	MaxCols   int    `json:"max_cols,omitempty" jsonschema:"description=最大返回列数(默认50)"`
}

// GetSheetDataResponse 获取工作表所有数据响应
type GetSheetDataResponse struct {
	Data         [][]string `json:"data,omitempty" jsonschema:"description=工作表数据"`
	RowCount     int        `json:"row_count" jsonschema:"description=实际返回的行数"`
	ColCount     int        `json:"col_count" jsonschema:"description=实际返回的列数"`
	Message      string     `json:"message,omitempty" jsonschema:"description=操作结果消息"`
	ErrorMessage string     `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetSheetData 获取指定工作表的所有数据
func (et *ExcelTools) GetSheetData(ctx context.Context, req *GetSheetDataRequest) (*GetSheetDataResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &GetSheetDataResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" {
		return &GetSheetDataResponse{
			ErrorMessage: "工作表名称不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &GetSheetDataResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	rows, err := f.GetRows(req.SheetName)
	if err != nil {
		return &GetSheetDataResponse{
			ErrorMessage: fmt.Sprintf("获取工作表数据失败: %v", err),
		}, nil
	}

	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = 1000
	}
	maxCols := req.MaxCols
	if maxCols <= 0 {
		maxCols = 50
	}

	// 限制行数
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}

	// 限制列数
	for i := range rows {
		if len(rows[i]) > maxCols {
			rows[i] = rows[i][:maxCols]
		}
	}

	colCount := 0
	for _, row := range rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}

	return &GetSheetDataResponse{
		Data:     rows,
		RowCount: len(rows),
		ColCount: colCount,
		Message:  fmt.Sprintf("成功获取工作表数据，%d 行 × %d 列", len(rows), colCount),
	}, nil
}

// GetAllSheetsDataRequest 获取所有工作表数据请求
type GetAllSheetsDataRequest struct {
	Path    string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	MaxRows int    `json:"max_rows,omitempty" jsonschema:"description=每个工作表最大返回行数(默认100)"`
	MaxCols int    `json:"max_cols,omitempty" jsonschema:"description=每个工作表最大返回列数(默认20)"`
}

// SheetData 工作表数据
type SheetData struct {
	SheetName string     `json:"sheet_name" jsonschema:"description=工作表名称"`
	Data      [][]string `json:"data" jsonschema:"description=工作表数据"`
	RowCount  int        `json:"row_count" jsonschema:"description=行数"`
	ColCount  int        `json:"col_count" jsonschema:"description=列数"`
}

// GetAllSheetsDataResponse 获取所有工作表数据响应
type GetAllSheetsDataResponse struct {
	Sheets       []SheetData `json:"sheets,omitempty" jsonschema:"description=所有工作表数据"`
	SheetCount   int         `json:"sheet_count" jsonschema:"description=工作表数量"`
	Message      string      `json:"message,omitempty" jsonschema:"description=操作结果消息"`
	ErrorMessage string      `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// GetAllSheetsData 获取所有工作表的数据
func (et *ExcelTools) GetAllSheetsData(ctx context.Context, req *GetAllSheetsDataRequest) (*GetAllSheetsDataResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &GetAllSheetsDataResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &GetAllSheetsDataResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	sheetList := f.GetSheetList()

	maxRows := req.MaxRows
	if maxRows <= 0 {
		maxRows = 100
	}
	maxCols := req.MaxCols
	if maxCols <= 0 {
		maxCols = 20
	}

	var sheets []SheetData
	for _, sheetName := range sheetList {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			continue // 跳过无法读取的工作表
		}

		// 限制行数
		if len(rows) > maxRows {
			rows = rows[:maxRows]
		}

		// 限制列数
		for i := range rows {
			if len(rows[i]) > maxCols {
				rows[i] = rows[i][:maxCols]
			}
		}

		colCount := 0
		for _, row := range rows {
			if len(row) > colCount {
				colCount = len(row)
			}
		}

		sheets = append(sheets, SheetData{
			SheetName: sheetName,
			Data:      rows,
			RowCount:  len(rows),
			ColCount:  colCount,
		})
	}

	return &GetAllSheetsDataResponse{
		Sheets:     sheets,
		SheetCount: len(sheets),
		Message:    fmt.Sprintf("成功获取 %d 个工作表的数据", len(sheets)),
	}, nil
}

// SetRowHeightRequest 设置行高请求
type SetRowHeightRequest struct {
	Path      string  `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string  `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Row       int     `json:"row" jsonschema:"description=行号(从1开始),required"`
	Height    float64 `json:"height" jsonschema:"description=行高,required"`
}

// SetRowHeightResponse 设置行高响应
type SetRowHeightResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetRowHeight 设置行高
func (et *ExcelTools) SetRowHeight(ctx context.Context, req *SetRowHeightRequest) (*SetRowHeightResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetRowHeightResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Row <= 0 {
		return &SetRowHeightResponse{
			ErrorMessage: "工作表名称和行号不能为空，行号必须大于0",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetRowHeightResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.SetRowHeight(req.SheetName, req.Row, req.Height); err != nil {
		return &SetRowHeightResponse{
			ErrorMessage: fmt.Sprintf("设置行高失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &SetRowHeightResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &SetRowHeightResponse{
		Message: fmt.Sprintf("成功设置第 %d 行的行高为 %.2f", req.Row, req.Height),
	}, nil
}

// SetColWidthRequest 设置列宽请求
type SetColWidthRequest struct {
	Path      string  `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string  `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	StartCol  string  `json:"start_col" jsonschema:"description=起始列(如A),required"`
	EndCol    string  `json:"end_col" jsonschema:"description=结束列(如C),required"`
	Width     float64 `json:"width" jsonschema:"description=列宽,required"`
}

// SetColWidthResponse 设置列宽响应
type SetColWidthResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetColWidth 设置列宽
func (et *ExcelTools) SetColWidth(ctx context.Context, req *SetColWidthRequest) (*SetColWidthResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetColWidthResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.StartCol == "" || req.EndCol == "" {
		return &SetColWidthResponse{
			ErrorMessage: "工作表名称、起始列和结束列不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetColWidthResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	if err := f.SetColWidth(req.SheetName, req.StartCol, req.EndCol, req.Width); err != nil {
		return &SetColWidthResponse{
			ErrorMessage: fmt.Sprintf("设置列宽失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &SetColWidthResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &SetColWidthResponse{
		Message: fmt.Sprintf("成功设置列 %s:%s 的列宽为 %.2f", req.StartCol, req.EndCol, req.Width),
	}, nil
}

// InsertRowRequest 插入行请求
type InsertRowRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Row       int    `json:"row" jsonschema:"description=插入位置行号(从1开始),required"`
	Count     int    `json:"count,omitempty" jsonschema:"description=插入行数(默认1)"`
}

// InsertRowResponse 插入行响应
type InsertRowResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// InsertRow 插入行
func (et *ExcelTools) InsertRow(ctx context.Context, req *InsertRowRequest) (*InsertRowResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &InsertRowResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Row <= 0 {
		return &InsertRowResponse{
			ErrorMessage: "工作表名称不能为空，行号必须大于0",
		}, nil
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &InsertRowResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	for i := 0; i < count; i++ {
		if err := f.InsertRows(req.SheetName, req.Row, 1); err != nil {
			return &InsertRowResponse{
				ErrorMessage: fmt.Sprintf("插入行失败: %v", err),
			}, nil
		}
	}

	if err := f.Save(); err != nil {
		return &InsertRowResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &InsertRowResponse{
		Message: fmt.Sprintf("成功在第 %d 行插入 %d 行", req.Row, count),
	}, nil
}

// RemoveRowRequest 删除行请求
type RemoveRowRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Row       int    `json:"row" jsonschema:"description=删除起始行号(从1开始),required"`
	Count     int    `json:"count,omitempty" jsonschema:"description=删除行数(默认1)"`
}

// RemoveRowResponse 删除行响应
type RemoveRowResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// RemoveRow 删除行
func (et *ExcelTools) RemoveRow(ctx context.Context, req *RemoveRowRequest) (*RemoveRowResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &RemoveRowResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Row <= 0 {
		return &RemoveRowResponse{
			ErrorMessage: "工作表名称不能为空，行号必须大于0",
		}, nil
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &RemoveRowResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	// RemoveRow 一次只能删除一行，需要循环删除
	for i := 0; i < count; i++ {
		if err := f.RemoveRow(req.SheetName, req.Row); err != nil {
			return &RemoveRowResponse{
				ErrorMessage: fmt.Sprintf("删除行失败: %v", err),
			}, nil
		}
	}

	if err := f.Save(); err != nil {
		return &RemoveRowResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &RemoveRowResponse{
		Message: fmt.Sprintf("成功删除第 %d 行开始的 %d 行", req.Row, count),
	}, nil
}

// InsertColRequest 插入列请求
type InsertColRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Col       string `json:"col" jsonschema:"description=插入位置列名(如A),required"`
	Count     int    `json:"count,omitempty" jsonschema:"description=插入列数(默认1)"`
}

// InsertColResponse 插入列响应
type InsertColResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// InsertCol 插入列
func (et *ExcelTools) InsertCol(ctx context.Context, req *InsertColRequest) (*InsertColResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &InsertColResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Col == "" {
		return &InsertColResponse{
			ErrorMessage: "工作表名称和列名不能为空",
		}, nil
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &InsertColResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	for i := 0; i < count; i++ {
		if err := f.InsertCols(req.SheetName, req.Col, 1); err != nil {
			return &InsertColResponse{
				ErrorMessage: fmt.Sprintf("插入列失败: %v", err),
			}, nil
		}
	}

	if err := f.Save(); err != nil {
		return &InsertColResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &InsertColResponse{
		Message: fmt.Sprintf("成功在列 %s 插入 %d 列", req.Col, count),
	}, nil
}

// RemoveColRequest 删除列请求
type RemoveColRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Col       string `json:"col" jsonschema:"description=删除起始列名(如A),required"`
	Count     int    `json:"count,omitempty" jsonschema:"description=删除列数(默认1)"`
}

// RemoveColResponse 删除列响应
type RemoveColResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// RemoveCol 删除列
func (et *ExcelTools) RemoveCol(ctx context.Context, req *RemoveColRequest) (*RemoveColResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &RemoveColResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Col == "" {
		return &RemoveColResponse{
			ErrorMessage: "工作表名称和列名不能为空",
		}, nil
	}

	count := req.Count
	if count <= 0 {
		count = 1
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &RemoveColResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	// RemoveCol 一次只能删除一列，需要循环删除
	for i := 0; i < count; i++ {
		if err := f.RemoveCol(req.SheetName, req.Col); err != nil {
			return &RemoveColResponse{
				ErrorMessage: fmt.Sprintf("删除列失败: %v", err),
			}, nil
		}
	}

	if err := f.Save(); err != nil {
		return &RemoveColResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &RemoveColResponse{
		Message: fmt.Sprintf("成功删除列 %s 开始的 %d 列", req.Col, count),
	}, nil
}
