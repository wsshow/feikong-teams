package excel

import (
	"context"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// ========== 样式操作 ==========

// CreateStyleRequest 创建样式请求
type CreateStyleRequest struct {
	Path string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	// 字体
	FontFamily string  `json:"font_family,omitempty" jsonschema:"description=字体名称"`
	FontSize   float64 `json:"font_size,omitempty" jsonschema:"description=字体大小"`
	FontBold   bool    `json:"font_bold,omitempty" jsonschema:"description=是否粗体"`
	FontItalic bool    `json:"font_italic,omitempty" jsonschema:"description=是否斜体"`
	FontColor  string  `json:"font_color,omitempty" jsonschema:"description=字体颜色(十六进制,如FF0000)"`
	// 对齐
	HorizontalAlign string `json:"horizontal_align,omitempty" jsonschema:"description=水平对齐(left/center/right)"`
	VerticalAlign   string `json:"vertical_align,omitempty" jsonschema:"description=垂直对齐(top/center/bottom)"`
	// 填充
	FillType    string `json:"fill_type,omitempty" jsonschema:"description=填充类型(pattern/gradient)"`
	FillPattern int    `json:"fill_pattern,omitempty" jsonschema:"description=填充图案(1-18)"`
	FillColor   string `json:"fill_color,omitempty" jsonschema:"description=填充颜色(十六进制)"`
	// 边框
	BorderStyle string `json:"border_style,omitempty" jsonschema:"description=边框样式(thin/medium/thick/double等)"`
	BorderColor string `json:"border_color,omitempty" jsonschema:"description=边框颜色(十六进制)"`
}

// CreateStyleResponse 创建样式响应
type CreateStyleResponse struct {
	StyleID      int    `json:"style_id" jsonschema:"description=样式ID"`
	Message      string `json:"message,omitempty" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// CreateStyle 创建样式
func (et *ExcelTools) CreateStyle(ctx context.Context, req *CreateStyleRequest) (*CreateStyleResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &CreateStyleResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &CreateStyleResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	style := &excelize.Style{}

	// 设置字体
	if req.FontFamily != "" || req.FontSize > 0 || req.FontBold || req.FontItalic || req.FontColor != "" {
		style.Font = &excelize.Font{
			Family: req.FontFamily,
			Size:   req.FontSize,
			Bold:   req.FontBold,
			Italic: req.FontItalic,
			Color:  req.FontColor,
		}
	}

	// 设置对齐
	if req.HorizontalAlign != "" || req.VerticalAlign != "" {
		style.Alignment = &excelize.Alignment{
			Horizontal: req.HorizontalAlign,
			Vertical:   req.VerticalAlign,
		}
	}

	// 设置填充
	if req.FillType != "" || req.FillPattern > 0 || req.FillColor != "" {
		style.Fill = excelize.Fill{
			Type:    req.FillType,
			Pattern: req.FillPattern,
			Color:   []string{req.FillColor},
		}
	}

	// 设置边框
	if req.BorderStyle != "" || req.BorderColor != "" {
		borderStyle := []excelize.Border{
			{Type: "left", Color: req.BorderColor, Style: getBorderStyle(req.BorderStyle)},
			{Type: "top", Color: req.BorderColor, Style: getBorderStyle(req.BorderStyle)},
			{Type: "bottom", Color: req.BorderColor, Style: getBorderStyle(req.BorderStyle)},
			{Type: "right", Color: req.BorderColor, Style: getBorderStyle(req.BorderStyle)},
		}
		style.Border = borderStyle
	}

	styleID, err := f.NewStyle(style)
	if err != nil {
		return &CreateStyleResponse{
			ErrorMessage: fmt.Sprintf("创建样式失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &CreateStyleResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &CreateStyleResponse{
		StyleID: styleID,
		Message: fmt.Sprintf("成功创建样式，ID: %d", styleID),
	}, nil
}

// getBorderStyle 将字符串转换为边框样式
func getBorderStyle(style string) int {
	borderStyles := map[string]int{
		"none":             0,
		"thin":             1,
		"medium":           2,
		"dashed":           3,
		"dotted":           4,
		"thick":            5,
		"double":           6,
		"hair":             7,
		"mediumDashed":     8,
		"dashDot":          9,
		"mediumDashDot":    10,
		"dashDotDot":       11,
		"mediumDashDotDot": 12,
		"slantDashDot":     13,
	}
	if val, ok := borderStyles[style]; ok {
		return val
	}
	return 1 // 默认 thin
}

// SetConditionalFormatRequest 设置条件格式请求
type SetConditionalFormatRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Range     string `json:"range" jsonschema:"description=单元格范围(如A1:A10),required"`
	Type      string `json:"type" jsonschema:"description=条件类型(cell/data_bar/color_scale等),required"`
	Criteria  string `json:"criteria,omitempty" jsonschema:"description=条件标准(如>,<,=等)"`
	Value     string `json:"value,omitempty" jsonschema:"description=比较值"`
	Format    int    `json:"format,omitempty" jsonschema:"description=应用的样式ID"`
}

// SetConditionalFormatResponse 设置条件格式响应
type SetConditionalFormatResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// SetConditionalFormat 设置条件格式
func (et *ExcelTools) SetConditionalFormat(ctx context.Context, req *SetConditionalFormatRequest) (*SetConditionalFormatResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &SetConditionalFormatResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Range == "" || req.Type == "" {
		return &SetConditionalFormatResponse{
			ErrorMessage: "工作表名称、范围和类型不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &SetConditionalFormatResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	formatSet := []excelize.ConditionalFormatOptions{
		{
			Type:     req.Type,
			Criteria: req.Criteria,
			Value:    req.Value,
			Format:   &req.Format,
		},
	}

	if err := f.SetConditionalFormat(req.SheetName, req.Range, formatSet); err != nil {
		return &SetConditionalFormatResponse{
			ErrorMessage: fmt.Sprintf("设置条件格式失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &SetConditionalFormatResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &SetConditionalFormatResponse{
		Message: fmt.Sprintf("成功设置条件格式: %s", req.Range),
	}, nil
}
