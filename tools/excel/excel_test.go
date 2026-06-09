package excel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func newTestExcelTools(t *testing.T) (*ExcelTools, string) {
	t.Helper()
	base := t.TempDir()
	tools, err := NewExcelTools(base)
	if err != nil {
		t.Fatalf("NewExcelTools failed: %v", err)
	}
	return tools, base
}

func TestValidatePathRejectsSiblingPrefix(t *testing.T) {
	tools, base := newTestExcelTools(t)
	sibling := base + "_sibling"
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatalf("create sibling dir: %v", err)
	}

	if _, err := tools.validatePath(filepath.Join(sibling, "book.xlsx")); err == nil || !strings.Contains(err.Error(), "访问被拒绝") {
		t.Fatalf("validatePath sibling error = %v, want access denied", err)
	}

	got, err := tools.validatePath(filepath.Join("nested", "book.xlsx"))
	if err != nil {
		t.Fatalf("validatePath relative returned error: %v", err)
	}
	if !strings.HasPrefix(got, base+string(os.PathSeparator)) {
		t.Fatalf("resolved path = %q, want inside %q", got, base)
	}
}

func TestWorkbookSheetAndCellFlow(t *testing.T) {
	tools, base := newTestExcelTools(t)
	ctx := context.Background()
	book := filepath.Join("reports", "book.xlsx")

	createResp, err := tools.CreateWorkbook(ctx, &CreateWorkbookRequest{Path: book})
	if err != nil {
		t.Fatalf("CreateWorkbook returned error: %v", err)
	}
	if createResp.ErrorMessage != "" {
		t.Fatalf("CreateWorkbook failed: %#v", createResp)
	}
	if _, err := os.Stat(filepath.Join(base, book)); err != nil {
		t.Fatalf("workbook not created: %v", err)
	}

	openResp, err := tools.OpenWorkbook(ctx, &OpenWorkbookRequest{Path: book})
	if err != nil {
		t.Fatalf("OpenWorkbook returned error: %v", err)
	}
	if openResp.ErrorMessage != "" || openResp.Info == nil || openResp.Info.SheetCount == 0 {
		t.Fatalf("OpenWorkbook = %#v, want workbook info", openResp)
	}

	sheetResp, err := tools.CreateSheet(ctx, &CreateSheetRequest{Path: book, SheetName: "Data"})
	if err != nil {
		t.Fatalf("CreateSheet returned error: %v", err)
	}
	if sheetResp.ErrorMessage != "" {
		t.Fatalf("CreateSheet failed: %#v", sheetResp)
	}

	if resp, err := tools.SetCellValue(ctx, &SetCellValueRequest{Path: book, SheetName: "Data", Cell: "A1", Value: "Name"}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetCellValue A1 resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.SetCellValue(ctx, &SetCellValueRequest{Path: book, SheetName: "Data", Cell: "B1", Value: "Score"}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetCellValue B1 resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.SetCellValue(ctx, &SetCellValueRequest{Path: book, SheetName: "Data", Cell: "A2", Value: "Ada"}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetCellValue A2 resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.SetCellValue(ctx, &SetCellValueRequest{Path: book, SheetName: "Data", Cell: "B2", Value: 99}); err != nil || resp.ErrorMessage != "" {
		t.Fatalf("SetCellValue B2 resp=%#v err=%v", resp, err)
	}

	cellResp, err := tools.GetCellValue(ctx, &GetCellValueRequest{Path: book, SheetName: "Data", Cell: "B2"})
	if err != nil {
		t.Fatalf("GetCellValue returned error: %v", err)
	}
	if cellResp.ErrorMessage != "" || cellResp.Value != "99" {
		t.Fatalf("GetCellValue = %#v, want 99", cellResp)
	}

	rowsResp, err := tools.GetRows(ctx, &GetRowsRequest{Path: book, SheetName: "Data"})
	if err != nil {
		t.Fatalf("GetRows returned error: %v", err)
	}
	if rowsResp.ErrorMessage != "" || rowsResp.RowCount != 2 || rowsResp.Rows[1][0] != "Ada" {
		t.Fatalf("GetRows = %#v, want two rows with Ada", rowsResp)
	}

	colResp, err := tools.GetCol(ctx, &GetColRequest{Path: book, SheetName: "Data", Col: "B"})
	if err != nil {
		t.Fatalf("GetCol returned error: %v", err)
	}
	if colResp.ErrorMessage != "" || len(colResp.Col) != 2 || colResp.Col[1] != "99" {
		t.Fatalf("GetCol = %#v, want score column", colResp)
	}

	renameResp, err := tools.RenameSheet(ctx, &RenameSheetRequest{Path: book, OldName: "Data", NewName: "Scores"})
	if err != nil {
		t.Fatalf("RenameSheet returned error: %v", err)
	}
	if renameResp.ErrorMessage != "" {
		t.Fatalf("RenameSheet failed: %#v", renameResp)
	}

	f, err := excelize.OpenFile(filepath.Join(base, book))
	if err != nil {
		t.Fatalf("open workbook directly: %v", err)
	}
	defer f.Close()
	if _, err := f.GetSheetIndex("Scores"); err != nil {
		t.Fatalf("Scores sheet missing: %v", err)
	}
}

func TestExcelValidationErrors(t *testing.T) {
	tools, _ := newTestExcelTools(t)
	ctx := context.Background()

	if resp, err := tools.CreateSheet(ctx, &CreateSheetRequest{Path: "missing.xlsx"}); err != nil || !strings.Contains(resp.ErrorMessage, "工作表名称不能为空") {
		t.Fatalf("CreateSheet validation resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.GetCellValue(ctx, &GetCellValueRequest{Path: "missing.xlsx", SheetName: "Sheet1"}); err != nil || !strings.Contains(resp.ErrorMessage, "工作表名称和单元格坐标不能为空") {
		t.Fatalf("GetCellValue validation resp=%#v err=%v", resp, err)
	}
	if resp, err := tools.GetRows(ctx, &GetRowsRequest{Path: "missing.xlsx"}); err != nil || !strings.Contains(resp.ErrorMessage, "工作表名称不能为空") {
		t.Fatalf("GetRows validation resp=%#v err=%v", resp, err)
	}
}
