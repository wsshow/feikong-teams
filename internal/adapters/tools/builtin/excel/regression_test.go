package excel

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"fkteams/internal/runtime/resources"

	"github.com/xuri/excelize/v2"
)

func TestExcelRejectsOutsideSymlinkAndReportsFailedWrites(t *testing.T) {
	et, base := newTestExcelTools(t)
	outside := t.TempDir()
	ctx := context.Background()
	if err := os.Symlink(outside, filepath.Join(base, "link")); err != nil {
		t.Skip(err)
	}
	resp, err := et.CreateWorkbook(ctx, &CreateWorkbookRequest{Path: "link/outside.xlsx"})
	if err != nil || resp.ErrorMessage == "" {
		t.Fatalf("outside write accepted: %#v %v", resp, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "outside.xlsx")); !os.IsNotExist(err) {
		t.Fatal("outside file was created")
	}
	if resp, err := et.CreateWorkbook(ctx, &CreateWorkbookRequest{Path: "book.xlsx"}); err != nil || resp.ErrorMessage != "" {
		t.Fatal(resp, err)
	}
	fill, err := et.BatchFillRows(ctx, &BatchFillRowsRequest{Path: "book.xlsx", SheetName: "missing", StartRow: 1, Data: [][]string{{"lost"}}})
	if err != nil || fill.ErrorMessage == "" || fill.RowsFilled != 0 {
		t.Fatalf("failed fill reported success: %#v %v", fill, err)
	}
	cols, err := et.BatchFillCols(ctx, &BatchFillColsRequest{Path: "book.xlsx", SheetName: "Sheet1", StartRow: 1, Columns: map[string][]string{"BAD!": {"lost"}}})
	if err != nil || cols.ErrorMessage == "" {
		t.Fatalf("invalid column reported success: %#v %v", cols, err)
	}
	deleted, err := et.DeleteSheet(ctx, &DeleteSheetRequest{Path: "book.xlsx", SheetName: "missing"})
	if err != nil || deleted.ErrorMessage == "" {
		t.Fatalf("missing sheet delete reported success: %#v %v", deleted, err)
	}
}
func TestExcelSharedLocksPreserveConcurrentUpdates(t *testing.T) {
	base := t.TempDir()
	locks := resources.NewKeyedLocks()
	a, err := NewExcelTools(base, locks)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewExcelTools(base, locks)
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := a.CreateWorkbook(context.Background(), &CreateWorkbookRequest{Path: "book.xlsx"}); err != nil || resp.ErrorMessage != "" {
		t.Fatal(resp, err)
	}
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tool := a
			if i%2 == 1 {
				tool = b
			}
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			resp, err := tool.SetCellValue(context.Background(), &SetCellValueRequest{Path: "book.xlsx", SheetName: "Sheet1", Cell: cell, Value: i + 1})
			if err != nil || resp.ErrorMessage != "" {
				t.Errorf("write: %#v %v", resp, err)
			}
		}(i)
	}
	wg.Wait()
	resp, err := a.GetRow(context.Background(), &GetRowRequest{Path: "book.xlsx", SheetName: "Sheet1", Row: 1})
	if err != nil || resp.ErrorMessage != "" || len(resp.Row) != 16 {
		t.Fatalf("lost workbook updates: %#v %v", resp, err)
	}
}
func TestExcelRowsPaginationStopsAtBudget(t *testing.T) {
	et, _ := newTestExcelTools(t)
	ctx := context.Background()
	et.CreateWorkbook(ctx, &CreateWorkbookRequest{Path: "book.xlsx"})
	fill, err := et.BatchFillRows(ctx, &BatchFillRowsRequest{Path: "book.xlsx", SheetName: "Sheet1", StartRow: 1, Data: [][]string{{"one"}, {"two"}, {"three"}}})
	if err != nil || fill.ErrorMessage != "" {
		t.Fatal(fill, err)
	}
	first, err := et.GetRows(ctx, &GetRowsRequest{Path: "book.xlsx", SheetName: "Sheet1", MaxRows: 2})
	if err != nil || !first.Truncated || first.NextRow != 3 || len(first.Rows) != 2 {
		t.Fatalf("first page: %#v %v", first, err)
	}
	last, err := et.GetRows(ctx, &GetRowsRequest{Path: "book.xlsx", SheetName: "Sheet1", StartRow: first.NextRow, MaxRows: 2})
	if err != nil || last.Truncated || len(last.Rows) != 1 || last.Rows[0][0] != "three" {
		t.Fatalf("next page: %#v %v", last, err)
	}
}
