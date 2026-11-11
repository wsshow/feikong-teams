package excel

import (
	runtimeport "fkteams/internal/ports/runtime"
)

// GetTools 返回所有Excel相关工具
func (et *ExcelTools) GetTools() ([]runtimeport.Tool, error) {
	// 工作簿操作

	return runtimeport.NewTools([]runtimeport.ToolDefinition{
		{Name: "excel_create_workbook", Description: "创建新的Excel工作簿", Handler: et.CreateWorkbook},
		{Name: "excel_open_workbook", Description: "打开Excel工作簿并获取信息", Handler: et.OpenWorkbook},
		{Name: "excel_save_workbook", Description: "保存Excel工作簿", Handler: et.SaveWorkbook},
		{Name: "excel_set_workbook_props", Description: "设置Excel工作簿属性", Handler: et.SetWorkbookProps},
		{Name: "excel_create_sheet", Description: "创建新工作表", Handler: et.CreateSheet},
		{Name: "excel_delete_sheet", Description: "删除工作表", Handler: et.DeleteSheet},
		{Name: "excel_rename_sheet", Description: "重命名工作表", Handler: et.RenameSheet},
		{Name: "excel_copy_sheet", Description: "复制工作表", Handler: et.CopySheet},
		{Name: "excel_set_active_sheet", Description: "设置活动工作表", Handler: et.SetActiveSheet},
		{Name: "excel_set_sheet_visible", Description: "设置工作表可见性", Handler: et.SetSheetVisible},
		{Name: "excel_set_cell_value", Description: "设置单元格值", Handler: et.SetCellValue},
		{Name: "excel_get_cell_value", Description: "获取单元格值", Handler: et.GetCellValue},
		{Name: "excel_set_cell_formula", Description: "设置单元格公式", Handler: et.SetCellFormula},
		{Name: "excel_merge_cells", Description: "合并单元格", Handler: et.MergeCells},
		{Name: "excel_unmerge_cells", Description: "取消合并单元格", Handler: et.UnmergeCells},
		{Name: "excel_set_cell_style", Description: "设置单元格样式", Handler: et.SetCellStyle},
		{Name: "excel_get_rows", Description: "获取工作表所有行数据", Handler: et.GetRows},
		{Name: "excel_get_row", Description: "获取指定行的数据", Handler: et.GetRow},
		{Name: "excel_get_col", Description: "获取指定列的数据", Handler: et.GetCol},
		{Name: "excel_get_sheet_data", Description: "获取指定工作表的所有数据", Handler: et.GetSheetData},
		{Name: "excel_get_all_sheets_data", Description: "获取所有工作表的数据", Handler: et.GetAllSheetsData},
		{Name: "excel_set_row_height", Description: "设置行高", Handler: et.SetRowHeight},
		{Name: "excel_set_col_width", Description: "设置列宽", Handler: et.SetColWidth},
		{Name: "excel_insert_row", Description: "插入行", Handler: et.InsertRow},
		{Name: "excel_remove_row", Description: "删除行", Handler: et.RemoveRow},
		{Name: "excel_insert_col", Description: "插入列", Handler: et.InsertCol},
		{Name: "excel_remove_col", Description: "删除列", Handler: et.RemoveCol},
		{Name: "excel_create_style", Description: "创建单元格样式", Handler: et.CreateStyle},
		{Name: "excel_set_conditional_format", Description: "设置条件格式", Handler: et.SetConditionalFormat},
		{Name: "excel_add_picture", Description: "添加图片到工作表", Handler: et.AddPicture},
		{Name: "excel_add_chart", Description: "添加图表到工作表", Handler: et.AddChart},
		{Name: "excel_batch_create_sheets", Description: "批量创建工作表", Handler: et.BatchCreateSheets},
		{Name: "excel_batch_delete_sheets", Description: "批量删除工作表", Handler: et.BatchDeleteSheets},
		{Name: "excel_batch_set_cell_values", Description: "批量设置单元格值", Handler: et.BatchSetCellValues},
		{Name: "excel_batch_fill_rows", Description: "批量填充行数据", Handler: et.BatchFillRows},
		{Name: "excel_batch_fill_cols", Description: "批量填充列数据", Handler: et.BatchFillCols},
		{Name: "excel_batch_insert_rows", Description: "批量插入行", Handler: et.BatchInsertRows},
		{Name: "excel_batch_remove_rows", Description: "批量删除行", Handler: et.BatchRemoveRows},
		{Name: "excel_batch_insert_cols", Description: "批量插入列", Handler: et.BatchInsertCols},
		{Name: "excel_batch_remove_cols", Description: "批量删除列", Handler: et.BatchRemoveCols},
		{Name: "excel_batch_set_cell_styles", Description: "批量设置单元格样式", Handler: et.BatchSetCellStyles},
	})
}
