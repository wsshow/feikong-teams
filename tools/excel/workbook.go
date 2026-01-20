package excel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xuri/excelize/v2"
)

// ========== 工作簿操作 ==========

// CreateWorkbookRequest 创建工作簿请求
type CreateWorkbookRequest struct {
	Path string `json:"path" jsonschema:"description=工作簿文件路径,required"`
}

// CreateWorkbookResponse 创建工作簿响应
type CreateWorkbookResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	Path         string `json:"path,omitempty" jsonschema:"description=工作簿路径"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// CreateWorkbook 创建新的工作簿
func (et *ExcelTools) CreateWorkbook(ctx context.Context, req *CreateWorkbookRequest) (*CreateWorkbookResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &CreateWorkbookResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &CreateWorkbookResponse{
			ErrorMessage: fmt.Sprintf("无法创建目录: %v", err),
		}, nil
	}

	f := excelize.NewFile()
	defer f.Close()

	// 默认创建 Sheet1
	index, err := f.NewSheet("Sheet1")
	if err != nil {
		return &CreateWorkbookResponse{
			ErrorMessage: fmt.Sprintf("创建工作表失败: %v", err),
		}, nil
	}
	f.SetActiveSheet(index)

	if err := f.SaveAs(path); err != nil {
		return &CreateWorkbookResponse{
			ErrorMessage: fmt.Sprintf("保存工作簿失败: %v", err),
		}, nil
	}

	return &CreateWorkbookResponse{
		Message: fmt.Sprintf("成功创建工作簿: %s", path),
		Path:    path,
	}, nil
}

// OpenWorkbookRequest 打开工作簿请求
type OpenWorkbookRequest struct {
	Path string `json:"path" jsonschema:"description=工作簿文件路径,required"`
}

// WorkbookInfo 工作簿信息
type WorkbookInfo struct {
	Path        string   `json:"path" jsonschema:"description=工作簿路径"`
	SheetCount  int      `json:"sheet_count" jsonschema:"description=工作表数量"`
	SheetNames  []string `json:"sheet_names" jsonschema:"description=工作表名称列表"`
	ActiveSheet string   `json:"active_sheet" jsonschema:"description=活动工作表名称"`
}

// OpenWorkbookResponse 打开工作簿响应
type OpenWorkbookResponse struct {
	Info         *WorkbookInfo `json:"info,omitempty" jsonschema:"description=工作簿信息"`
	Message      string        `json:"message,omitempty" jsonschema:"description=操作结果消息"`
	ErrorMessage string        `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// OpenWorkbook 打开工作簿并获取信息
func (et *ExcelTools) OpenWorkbook(ctx context.Context, req *OpenWorkbookRequest) (*OpenWorkbookResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &OpenWorkbookResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &OpenWorkbookResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	sheetList := f.GetSheetList()
	activeSheetIndex := f.GetActiveSheetIndex()
	var activeSheetName string
	if activeSheetIndex >= 0 && activeSheetIndex < len(sheetList) {
		activeSheetName = sheetList[activeSheetIndex]
	}

	info := &WorkbookInfo{
		Path:        path,
		SheetCount:  len(sheetList),
		SheetNames:  sheetList,
		ActiveSheet: activeSheetName,
	}

	return &OpenWorkbookResponse{
		Info:    info,
		Message: fmt.Sprintf("成功打开工作簿，共 %d 个工作表", len(sheetList)),
	}, nil
}

// SaveWorkbookRequest 保存工作簿请求
type SaveWorkbookRequest struct {
	Path   string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SaveAs string `json:"save_as,omitempty" jsonschema:"description=另存为路径(可选)"`
}

// SaveWorkbookResponse 保存工作簿响应
type SaveWorkbookResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	Path         string `json:"path,omitempty" jsonschema:"description=保存路径"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SaveWorkbook 保存工作簿
func (et *ExcelTools) SaveWorkbook(ctx context.Context, req *SaveWorkbookRequest) (*SaveWorkbookResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SaveWorkbookResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SaveWorkbookResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	var savePath string
	if req.SaveAs != "" {
		savePath, err = et.validatePath(req.SaveAs)
		if err != nil {
			return &SaveWorkbookResponse{
				ErrorMessage: err.Error(),
			}, nil
		}
		// 确保目录存在
		dir := filepath.Dir(savePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &SaveWorkbookResponse{
				ErrorMessage: fmt.Sprintf("无法创建目录: %v", err),
			}, nil
		}
		if err := f.SaveAs(savePath); err != nil {
			return &SaveWorkbookResponse{
				ErrorMessage: fmt.Sprintf("另存为失败: %v", err),
			}, nil
		}
	} else {
		savePath = path
		if err := f.Save(); err != nil {
			return &SaveWorkbookResponse{
				ErrorMessage: fmt.Sprintf("保存失败: %v", err),
			}, nil
		}
	}

	return &SaveWorkbookResponse{
		Message: fmt.Sprintf("成功保存工作簿: %s", savePath),
		Path:    savePath,
	}, nil
}

// SetWorkbookPropsRequest 设置工作簿属性请求
type SetWorkbookPropsRequest struct {
	Path        string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	Title       string `json:"title,omitempty" jsonschema:"description=标题"`
	Subject     string `json:"subject,omitempty" jsonschema:"description=主题"`
	Creator     string `json:"creator,omitempty" jsonschema:"description=创建者"`
	Keywords    string `json:"keywords,omitempty" jsonschema:"description=关键词"`
	Description string `json:"description,omitempty" jsonschema:"description=描述"`
	Category    string `json:"category,omitempty" jsonschema:"description=类别"`
}

// SetWorkbookPropsResponse 设置工作簿属性响应
type SetWorkbookPropsResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetWorkbookProps 设置工作簿属性
func (et *ExcelTools) SetWorkbookProps(ctx context.Context, req *SetWorkbookPropsRequest) (*SetWorkbookPropsResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetWorkbookPropsResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetWorkbookPropsResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	props := &excelize.DocProperties{}
	if req.Title != "" {
		props.Title = req.Title
	}
	if req.Subject != "" {
		props.Subject = req.Subject
	}
	if req.Creator != "" {
		props.Creator = req.Creator
	}
	if req.Keywords != "" {
		props.Keywords = req.Keywords
	}
	if req.Description != "" {
		props.Description = req.Description
	}
	if req.Category != "" {
		props.Category = req.Category
	}

	if err := f.SetDocProps(props); err != nil {
		return &SetWorkbookPropsResponse{
			ErrorMessage: fmt.Sprintf("设置属性失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &SetWorkbookPropsResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &SetWorkbookPropsResponse{
		Message: "成功设置工作簿属性",
	}, nil
}
