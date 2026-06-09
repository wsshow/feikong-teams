package excel

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestBatchSheetAndDataOperations(t *testing.T) {
	tools, base := newTestExcelTools(t)
	ctx := context.Background()
	book := "batch.xlsx"

	if resp, err := tools.CreateWorkbook(ctx, &CreateWorkbookRequest{Path: book}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("CreateWorkbook resp=%#v err=%v", resp, err)
	}

	createSheets, err := tools.BatchCreateSheets(ctx, &BatchCreateSheetsRequest{
		Path:       book,
		SheetNames: []string{"Data", "", "Archive"},
	})
	if err != nil {
		t.Fatalf("BatchCreateSheets: %v", err)
	}
	if len(createSheets.Success) != 2 || len(createSheets.Failed) != 1 {
		t.Fatalf("create sheets = %#v", createSheets)
	}

	setValues, err := tools.BatchSetCellValues(ctx, &BatchSetCellValuesRequest{
		Path:      book,
		SheetName: "Data",
		Values: []CellValue{
			{Cell: "A1", Value: "Name"},
			{Cell: "B1", Value: "Score"},
			{Cell: "", Value: "bad"},
		},
	})
	if err != nil {
		t.Fatalf("BatchSetCellValues: %v", err)
	}
	if len(setValues.Success) != 2 || len(setValues.Failed) != 1 {
		t.Fatalf("set values = %#v", setValues)
	}

	fillRows, err := tools.BatchFillRows(ctx, &BatchFillRowsRequest{
		Path:      book,
		SheetName: "Data",
		StartRow:  2,
		Data: [][]string{
			{"Ada", "99", "extra"},
			{"Linus", "88", "extra"},
		},
	})
	if err != nil || fillRows.ErrorMessage != "" || fillRows.RowsFilled != 2 {
		t.Fatalf("BatchFillRows resp=%#v err=%v", fillRows, err)
	}

	fillCols, err := tools.BatchFillCols(ctx, &BatchFillColsRequest{
		Path:      book,
		SheetName: "Data",
		StartRow:  2,
		Columns: map[string][]string{
			"C": {"A", "B"},
		},
	})
	if err != nil || fillCols.ErrorMessage != "" || fillCols.ColsFilled != 1 {
		t.Fatalf("BatchFillCols resp=%#v err=%v", fillCols, err)
	}

	sheetData, err := tools.GetSheetData(ctx, &GetSheetDataRequest{Path: book, SheetName: "Data", MaxRows: 2, MaxCols: 2})
	if err != nil {
		t.Fatalf("GetSheetData: %v", err)
	}
	if sheetData.ErrorMessage != "" || sheetData.RowCount != 2 || sheetData.ColCount != 2 {
		t.Fatalf("sheet data = %#v", sheetData)
	}
	if sheetData.Data[1][0] != "Ada" || sheetData.Data[1][1] != "99" {
		t.Fatalf("limited sheet data = %#v", sheetData.Data)
	}

	allData, err := tools.GetAllSheetsData(ctx, &GetAllSheetsDataRequest{Path: book, MaxRows: 1, MaxCols: 1})
	if err != nil {
		t.Fatalf("GetAllSheetsData: %v", err)
	}
	if allData.ErrorMessage != "" || allData.SheetCount < 2 {
		t.Fatalf("all sheets data = %#v", allData)
	}

	copyResp, err := tools.CopySheet(ctx, &CopySheetRequest{Path: book, FromSheet: "Data", ToSheet: "Copy"})
	if err != nil || copyResp.ErrorMessage != "" {
		t.Fatalf("CopySheet resp=%#v err=%v", copyResp, err)
	}
	activeResp, err := tools.SetActiveSheet(ctx, &SetActiveSheetRequest{Path: book, SheetName: "Copy"})
	if err != nil || activeResp.ErrorMessage != "" {
		t.Fatalf("SetActiveSheet resp=%#v err=%v", activeResp, err)
	}
	deleteResp, err := tools.BatchDeleteSheets(ctx, &BatchDeleteSheetsRequest{Path: book, SheetNames: []string{"Archive", ""}})
	if err != nil {
		t.Fatalf("BatchDeleteSheets: %v", err)
	}
	if len(deleteResp.Success) != 1 || len(deleteResp.Failed) != 1 {
		t.Fatalf("delete sheets = %#v", deleteResp)
	}

	f, err := excelize.OpenFile(filepath.Join(base, book))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()
	if sheetExists(f.GetSheetList(), "Archive") {
		t.Fatal("Archive sheet should be deleted")
	}
	if !sheetExists(f.GetSheetList(), "Copy") {
		t.Fatal("Copy sheet should exist")
	}
}

func TestCellStyleAndLayoutOperations(t *testing.T) {
	tools, _ := newTestExcelTools(t)
	ctx := context.Background()
	book := "style.xlsx"

	if resp, err := tools.CreateWorkbook(ctx, &CreateWorkbookRequest{Path: book}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("CreateWorkbook resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.SetCellValue(ctx, &SetCellValueRequest{Path: book, SheetName: "Sheet1", Cell: "A1", Value: 10}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetCellValue A1 resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.SetCellValue(ctx, &SetCellValueRequest{Path: book, SheetName: "Sheet1", Cell: "A2", Value: 20}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetCellValue A2 resp=%#v err=%v", resp, err)
	}

	formula, err := tools.SetCellFormula(ctx, &SetCellFormulaRequest{Path: book, SheetName: "Sheet1", Cell: "A3", Formula: "SUM(A1:A2)"})
	if err != nil || formula.ErrorMessage != "" {
		t.Fatalf("SetCellFormula resp=%#v err=%v", formula, err)
	}
	merge, err := tools.MergeCells(ctx, &MergeCellsRequest{Path: book, SheetName: "Sheet1", StartCell: "B1", EndCell: "C1"})
	if err != nil || merge.ErrorMessage != "" {
		t.Fatalf("MergeCells resp=%#v err=%v", merge, err)
	}
	unmerge, err := tools.UnmergeCells(ctx, &UnmergeCellsRequest{Path: book, SheetName: "Sheet1", StartCell: "B1", EndCell: "C1"})
	if err != nil || unmerge.ErrorMessage != "" {
		t.Fatalf("UnmergeCells resp=%#v err=%v", unmerge, err)
	}

	style, err := tools.CreateStyle(ctx, &CreateStyleRequest{
		Path:            book,
		FontBold:        true,
		FontColor:       "FF0000",
		HorizontalAlign: "center",
		BorderStyle:     "thin",
	})
	if err != nil || style.ErrorMessage != "" || style.StyleID == 0 {
		t.Fatalf("CreateStyle resp=%#v err=%v", style, err)
	}
	if resp, err := tools.SetCellStyle(ctx, &SetCellStyleRequest{Path: book, SheetName: "Sheet1", StartCell: "A1", EndCell: "A3", StyleID: style.StyleID}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetCellStyle resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.SetRowHeight(ctx, &SetRowHeightRequest{Path: book, SheetName: "Sheet1", Row: 1, Height: 24}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetRowHeight resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.SetColWidth(ctx, &SetColWidthRequest{Path: book, SheetName: "Sheet1", StartCol: "A", EndCol: "C", Width: 18}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetColWidth resp=%#v err=%v", resp, err)
	}
	if getBorderStyle("unknown") != 1 || getBorderStyle("double") != 6 {
		t.Fatal("border style mapping should use known values and default thin")
	}
}

func sheetExists(sheets []string, name string) bool {
	for _, sheet := range sheets {
		if sheet == name {
			return true
		}
	}
	return false
}

func TestGetToolsIncludesExcelToolNames(t *testing.T) {
	tools, _ := newTestExcelTools(t)
	list, err := tools.GetTools()
	if err != nil {
		t.Fatalf("GetTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range list {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatalf("tool info: %v", err)
		}
		names[info.Name] = true
	}
	for _, name := range []string{"excel_create_workbook", "excel_batch_set_cell_values", "excel_get_sheet_data", "excel_create_style", "excel_add_chart"} {
		if !names[name] {
			t.Fatalf("tool %s missing from %#v", name, names)
		}
	}
}
