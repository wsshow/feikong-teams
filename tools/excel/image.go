package excel

import (
	"context"
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

// ========== 图片操作 ==========

// AddPictureRequest 添加图片请求
type AddPictureRequest struct {
	Path        string  `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName   string  `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Cell        string  `json:"cell" jsonschema:"description=插入位置单元格(如A1),required"`
	PicturePath string  `json:"picture_path" jsonschema:"description=图片文件路径,required"`
	OffsetX     int     `json:"offset_x,omitempty" jsonschema:"description=X轴偏移量"`
	OffsetY     int     `json:"offset_y,omitempty" jsonschema:"description=Y轴偏移量"`
	ScaleX      float64 `json:"scale_x,omitempty" jsonschema:"description=X轴缩放比例(默认1.0)"`
	ScaleY      float64 `json:"scale_y,omitempty" jsonschema:"description=Y轴缩放比例(默认1.0)"`
}

// AddPictureResponse 添加图片响应
type AddPictureResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// AddPicture 添加图片到工作表
func (et *ExcelTools) AddPicture(ctx context.Context, req *AddPictureRequest) (*AddPictureResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &AddPictureResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Cell == "" || req.PicturePath == "" {
		return &AddPictureResponse{
			ErrorMessage: "工作表名称、单元格位置和图片路径不能为空",
		}, nil
	}

	// 验证图片路径
	picturePath, err := et.validatePath(req.PicturePath)
	if err != nil {
		return &AddPictureResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	// 检查图片文件是否存在
	if _, err := os.Stat(picturePath); os.IsNotExist(err) {
		return &AddPictureResponse{
			ErrorMessage: fmt.Sprintf("图片文件不存在: %s", picturePath),
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &AddPictureResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	scaleX := req.ScaleX
	if scaleX == 0 {
		scaleX = 1.0
	}
	scaleY := req.ScaleY
	if scaleY == 0 {
		scaleY = 1.0
	}

	opts := &excelize.GraphicOptions{
		OffsetX: req.OffsetX,
		OffsetY: req.OffsetY,
		ScaleX:  scaleX,
		ScaleY:  scaleY,
	}

	if err := f.AddPicture(req.SheetName, req.Cell, picturePath, opts); err != nil {
		return &AddPictureResponse{
			ErrorMessage: fmt.Sprintf("添加图片失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &AddPictureResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &AddPictureResponse{
		Message: fmt.Sprintf("成功在单元格 %s 添加图片", req.Cell),
	}, nil
}

// AddChartRequest 添加图表请求
type AddChartRequest struct {
	Path      string `json:"path" jsonschema:"description=工作簿文件路径,required"`
	SheetName string `json:"sheet_name" jsonschema:"description=工作表名称,required"`
	Cell      string `json:"cell" jsonschema:"description=插入位置单元格(如A1),required"`
	Type      string `json:"type" jsonschema:"description=图表类型(col/bar/line/pie/scatter等),required"`
	// 图表数据系列
	SeriesName string `json:"series_name,omitempty" jsonschema:"description=系列名称"`
	Categories string `json:"categories,omitempty" jsonschema:"description=分类范围(如Sheet1!$A$1:$A$5)"`
	Values     string `json:"values,omitempty" jsonschema:"description=值范围(如Sheet1!$B$1:$B$5)"`
	// 图表设置
	Title  string `json:"title,omitempty" jsonschema:"description=图表标题"`
	Width  int    `json:"width,omitempty" jsonschema:"description=图表宽度"`
	Height int    `json:"height,omitempty" jsonschema:"description=图表高度"`
}

// AddChartResponse 添加图表响应
type AddChartResponse struct {
	Message      string `json:"message" jsonschema:"description=操作结果消息"`
	ErrorMessage string `json:"error_message,omitempty" jsonschema:"description=错误信息"`
}

// AddChart 添加图表到工作表
func (et *ExcelTools) AddChart(ctx context.Context, req *AddChartRequest) (*AddChartResponse, error) {
	path, err := et.validatePath(req.Path)
	if err != nil {
		return &AddChartResponse{
			ErrorMessage: err.Error(),
		}, nil
	}

	if req.SheetName == "" || req.Cell == "" || req.Type == "" {
		return &AddChartResponse{
			ErrorMessage: "工作表名称、单元格位置和图表类型不能为空",
		}, nil
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		return &AddChartResponse{
			ErrorMessage: fmt.Sprintf("打开工作簿失败: %v", err),
		}, nil
	}
	defer f.Close()

	series := []excelize.ChartSeries{}
	if req.Categories != "" && req.Values != "" {
		series = append(series, excelize.ChartSeries{
			Name:       req.SeriesName,
			Categories: req.Categories,
			Values:     req.Values,
		})
	}

	// 图表类型映射
	chartTypeMap := map[string]excelize.ChartType{
		"col":      excelize.Col,
		"col3D":    excelize.Col3D,
		"bar":      excelize.Bar,
		"line":     excelize.Line,
		"pie":      excelize.Pie,
		"pie3D":    excelize.Pie3D,
		"doughnut": excelize.Doughnut,
		"scatter":  excelize.Scatter,
		"area":     excelize.Area,
		"radar":    excelize.Radar,
	}

	chartType, ok := chartTypeMap[req.Type]
	if !ok {
		// 默认使用柱状图
		chartType = excelize.Col
	}

	chart := &excelize.Chart{
		Type:   chartType,
		Series: series,
		Title: []excelize.RichTextRun{
			{
				Text: req.Title,
			},
		},
	}

	// 设置图表尺寸
	if req.Width > 0 {
		chart.Dimension.Width = uint(req.Width)
	}
	if req.Height > 0 {
		chart.Dimension.Height = uint(req.Height)
	}

	if err := f.AddChart(req.SheetName, req.Cell, chart); err != nil {
		return &AddChartResponse{
			ErrorMessage: fmt.Sprintf("添加图表失败: %v", err),
		}, nil
	}

	if err := f.Save(); err != nil {
		return &AddChartResponse{
			ErrorMessage: fmt.Sprintf("保存失败: %v", err),
		}, nil
	}

	return &AddChartResponse{
		Message: fmt.Sprintf("成功在单元格 %s 添加%s图表", req.Cell, req.Type),
	}, nil
}
