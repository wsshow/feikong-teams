package excel

import (
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// GetTools 返回所有Excel相关工具
func (et *ExcelTools) GetTools() ([]tool.BaseTool, error) {
	// 工作簿操作
	createWorkbookTool, err := utils.InferTool(
		"excel_create_workbook",
		"创建新的Excel工作簿",
		et.CreateWorkbook,
	)
	if err != nil {
		return nil, err
	}

	openWorkbookTool, err := utils.InferTool(
		"excel_open_workbook",
		"打开Excel工作簿并获取信息",
		et.OpenWorkbook,
	)
	if err != nil {
		return nil, err
	}

	saveWorkbookTool, err := utils.InferTool(
		"excel_save_workbook",
		"保存Excel工作簿",
		et.SaveWorkbook,
	)
	if err != nil {
		return nil, err
	}

	setWorkbookPropsTool, err := utils.InferTool(
		"excel_set_workbook_props",
		"设置Excel工作簿属性",
		et.SetWorkbookProps,
	)
	if err != nil {
		return nil, err
	}

	// 工作表操作
	createSheetTool, err := utils.InferTool(
		"excel_create_sheet",
		"创建新工作表",
		et.CreateSheet,
	)
	if err != nil {
		return nil, err
	}

	deleteSheetTool, err := utils.InferTool(
		"excel_delete_sheet",
		"删除工作表",
		et.DeleteSheet,
	)
	if err != nil {
		return nil, err
	}

	renameSheetTool, err := utils.InferTool(
		"excel_rename_sheet",
		"重命名工作表",
		et.RenameSheet,
	)
	if err != nil {
		return nil, err
	}

	copySheetTool, err := utils.InferTool(
		"excel_copy_sheet",
		"复制工作表",
		et.CopySheet,
	)
	if err != nil {
		return nil, err
	}

	setActiveSheetTool, err := utils.InferTool(
		"excel_set_active_sheet",
		"设置活动工作表",
		et.SetActiveSheet,
	)
	if err != nil {
		return nil, err
	}

	setSheetVisibleTool, err := utils.InferTool(
		"excel_set_sheet_visible",
		"设置工作表可见性",
		et.SetSheetVisible,
	)
	if err != nil {
		return nil, err
	}

	// 单元格操作
	setCellValueTool, err := utils.InferTool(
		"excel_set_cell_value",
		"设置单元格值",
		et.SetCellValue,
	)
	if err != nil {
		return nil, err
	}

	getCellValueTool, err := utils.InferTool(
		"excel_get_cell_value",
		"获取单元格值",
		et.GetCellValue,
	)
	if err != nil {
		return nil, err
	}

	setCellFormulaTool, err := utils.InferTool(
		"excel_set_cell_formula",
		"设置单元格公式",
		et.SetCellFormula,
	)
	if err != nil {
		return nil, err
	}

	mergeCellsTool, err := utils.InferTool(
		"excel_merge_cells",
		"合并单元格",
		et.MergeCells,
	)
	if err != nil {
		return nil, err
	}

	unmergeCellsTool, err := utils.InferTool(
		"excel_unmerge_cells",
		"取消合并单元格",
		et.UnmergeCells,
	)
	if err != nil {
		return nil, err
	}

	setCellStyleTool, err := utils.InferTool(
		"excel_set_cell_style",
		"设置单元格样式",
		et.SetCellStyle,
	)
	if err != nil {
		return nil, err
	}

	// 数据操作
	getRowsTool, err := utils.InferTool(
		"excel_get_rows",
		"获取工作表所有行数据",
		et.GetRows,
	)
	if err != nil {
		return nil, err
	}

	getRowTool, err := utils.InferTool(
		"excel_get_row",
		"获取指定行的数据",
		et.GetRow,
	)
	if err != nil {
		return nil, err
	}

	getColTool, err := utils.InferTool(
		"excel_get_col",
		"获取指定列的数据",
		et.GetCol,
	)
	if err != nil {
		return nil, err
	}

	getSheetDataTool, err := utils.InferTool(
		"excel_get_sheet_data",
		"获取指定工作表的所有数据",
		et.GetSheetData,
	)
	if err != nil {
		return nil, err
	}

	getAllSheetsDataTool, err := utils.InferTool(
		"excel_get_all_sheets_data",
		"获取所有工作表的数据",
		et.GetAllSheetsData,
	)
	if err != nil {
		return nil, err
	}

	setRowHeightTool, err := utils.InferTool(
		"excel_set_row_height",
		"设置行高",
		et.SetRowHeight,
	)
	if err != nil {
		return nil, err
	}

	setColWidthTool, err := utils.InferTool(
		"excel_set_col_width",
		"设置列宽",
		et.SetColWidth,
	)
	if err != nil {
		return nil, err
	}

	insertRowTool, err := utils.InferTool(
		"excel_insert_row",
		"插入行",
		et.InsertRow,
	)
	if err != nil {
		return nil, err
	}

	removeRowTool, err := utils.InferTool(
		"excel_remove_row",
		"删除行",
		et.RemoveRow,
	)
	if err != nil {
		return nil, err
	}

	insertColTool, err := utils.InferTool(
		"excel_insert_col",
		"插入列",
		et.InsertCol,
	)
	if err != nil {
		return nil, err
	}

	removeColTool, err := utils.InferTool(
		"excel_remove_col",
		"删除列",
		et.RemoveCol,
	)
	if err != nil {
		return nil, err
	}

	// 样式操作
	createStyleTool, err := utils.InferTool(
		"excel_create_style",
		"创建单元格样式",
		et.CreateStyle,
	)
	if err != nil {
		return nil, err
	}

	setConditionalFormatTool, err := utils.InferTool(
		"excel_set_conditional_format",
		"设置条件格式",
		et.SetConditionalFormat,
	)
	if err != nil {
		return nil, err
	}

	// 图片和图表操作
	addPictureTool, err := utils.InferTool(
		"excel_add_picture",
		"添加图片到工作表",
		et.AddPicture,
	)
	if err != nil {
		return nil, err
	}

	addChartTool, err := utils.InferTool(
		"excel_add_chart",
		"添加图表到工作表",
		et.AddChart,
	)
	if err != nil {
		return nil, err
	}

	// 批量操作
	batchCreateSheetsTool, err := utils.InferTool(
		"excel_batch_create_sheets",
		"批量创建工作表",
		et.BatchCreateSheets,
	)
	if err != nil {
		return nil, err
	}

	batchDeleteSheetsTool, err := utils.InferTool(
		"excel_batch_delete_sheets",
		"批量删除工作表",
		et.BatchDeleteSheets,
	)
	if err != nil {
		return nil, err
	}

	batchSetCellValuesTool, err := utils.InferTool(
		"excel_batch_set_cell_values",
		"批量设置单元格值",
		et.BatchSetCellValues,
	)
	if err != nil {
		return nil, err
	}

	batchFillRowsTool, err := utils.InferTool(
		"excel_batch_fill_rows",
		"批量填充行数据",
		et.BatchFillRows,
	)
	if err != nil {
		return nil, err
	}

	batchFillColsTool, err := utils.InferTool(
		"excel_batch_fill_cols",
		"批量填充列数据",
		et.BatchFillCols,
	)
	if err != nil {
		return nil, err
	}

	batchInsertRowsTool, err := utils.InferTool(
		"excel_batch_insert_rows",
		"批量插入行",
		et.BatchInsertRows,
	)
	if err != nil {
		return nil, err
	}

	batchRemoveRowsTool, err := utils.InferTool(
		"excel_batch_remove_rows",
		"批量删除行",
		et.BatchRemoveRows,
	)
	if err != nil {
		return nil, err
	}

	batchInsertColsTool, err := utils.InferTool(
		"excel_batch_insert_cols",
		"批量插入列",
		et.BatchInsertCols,
	)
	if err != nil {
		return nil, err
	}

	batchRemoveColsTool, err := utils.InferTool(
		"excel_batch_remove_cols",
		"批量删除列",
		et.BatchRemoveCols,
	)
	if err != nil {
		return nil, err
	}

	batchSetCellStylesTool, err := utils.InferTool(
		"excel_batch_set_cell_styles",
		"批量设置单元格样式",
		et.BatchSetCellStyles,
	)
	if err != nil {
		return nil, err
	}

	return []tool.BaseTool{
		// 工作簿
		createWorkbookTool,
		openWorkbookTool,
		saveWorkbookTool,
		setWorkbookPropsTool,
		// 工作表
		createSheetTool,
		deleteSheetTool,
		renameSheetTool,
		copySheetTool,
		setActiveSheetTool,
		setSheetVisibleTool,
		// 单元格
		setCellValueTool,
		getCellValueTool,
		setCellFormulaTool,
		mergeCellsTool,
		unmergeCellsTool,
		setCellStyleTool,
		// 数据
		getRowsTool,
		getRowTool,
		getColTool,
		getSheetDataTool,
		getAllSheetsDataTool,
		setRowHeightTool,
		setColWidthTool,
		insertRowTool,
		removeRowTool,
		insertColTool,
		removeColTool,
		// 样式
		createStyleTool,
		setConditionalFormatTool,
		// 图片和图表
		addPictureTool,
		addChartTool,
		// 批量操作
		batchCreateSheetsTool,
		batchDeleteSheetsTool,
		batchSetCellValuesTool,
		batchFillRowsTool,
		batchFillColsTool,
		batchInsertRowsTool,
		batchRemoveRowsTool,
		batchInsertColsTool,
		batchRemoveColsTool,
		batchSetCellStylesTool,
	}, nil
}
