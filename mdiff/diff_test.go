package mdiff

import "testing"

// ============================================================
// Diff 算法测试 (Myers)
// ============================================================

func TestDiffEmpty(t *testing.T) {
	edits := Diff(nil, nil)
	if len(edits) != 0 {
		t.Errorf("expected 0 edits, got %d", len(edits))
	}
}

func TestDiffAllInsert(t *testing.T) {
	edits := Diff(nil, []string{"a", "b", "c"})
	if len(edits) != 3 {
		t.Fatalf("expected 3 edits, got %d", len(edits))
	}
	for _, e := range edits {
		if e.Kind != OpInsert {
			t.Errorf("expected OpInsert, got %d", e.Kind)
		}
	}
}

func TestDiffAllDelete(t *testing.T) {
	edits := Diff([]string{"a", "b", "c"}, nil)
	if len(edits) != 3 {
		t.Fatalf("expected 3 edits, got %d", len(edits))
	}
	for _, e := range edits {
		if e.Kind != OpDelete {
			t.Errorf("expected OpDelete, got %d", e.Kind)
		}
	}
}

func TestDiffNoChange(t *testing.T) {
	lines := []string{"a", "b", "c"}
	edits := Diff(lines, lines)
	for _, e := range edits {
		if e.Kind != OpEqual {
			t.Errorf("expected all OpEqual, got %d for %q", e.Kind, e.Text)
		}
	}
}

func TestDiffSimpleChange(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"a", "x", "c"}
	edits := Diff(oldLines, newLines)

	hasDelete := false
	hasInsert := false
	for _, e := range edits {
		if e.Kind == OpDelete && e.Text == "b" {
			hasDelete = true
		}
		if e.Kind == OpInsert && e.Text == "x" {
			hasInsert = true
		}
	}
	if !hasDelete {
		t.Error("expected delete of b")
	}
	if !hasInsert {
		t.Error("expected insert of x")
	}
}

func TestDiffSingleLine(t *testing.T) {
	edits := Diff([]string{"old"}, []string{"new"})

	hasDelete := false
	hasInsert := false
	for _, e := range edits {
		if e.Kind == OpDelete && e.Text == "old" {
			hasDelete = true
		}
		if e.Kind == OpInsert && e.Text == "new" {
			hasInsert = true
		}
	}
	if !hasDelete || !hasInsert {
		t.Error("expected delete of 'old' and insert of 'new'")
	}
}

func TestDiffInsertAtBeginning(t *testing.T) {
	oldLines := []string{"b", "c"}
	newLines := []string{"a", "b", "c"}
	edits := Diff(oldLines, newLines)

	if edits[0].Kind != OpInsert || edits[0].Text != "a" {
		t.Error("expected insert 'a' at beginning")
	}
}

func TestDiffInsertAtEnd(t *testing.T) {
	oldLines := []string{"a", "b"}
	newLines := []string{"a", "b", "c"}
	edits := Diff(oldLines, newLines)

	lastEdit := edits[len(edits)-1]
	if lastEdit.Kind != OpInsert || lastEdit.Text != "c" {
		t.Error("expected insert 'c' at end")
	}
}

func TestDiffDeleteAtBeginning(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"b", "c"}
	edits := Diff(oldLines, newLines)

	if edits[0].Kind != OpDelete || edits[0].Text != "a" {
		t.Error("expected delete 'a' at beginning")
	}
}

func TestDiffDeleteAtEnd(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"a", "b"}
	edits := Diff(oldLines, newLines)

	lastNonEqual := edits[len(edits)-1]
	for i := len(edits) - 1; i >= 0; i-- {
		if edits[i].Kind != OpEqual {
			lastNonEqual = edits[i]
			break
		}
	}
	if lastNonEqual.Kind != OpDelete || lastNonEqual.Text != "c" {
		t.Error("expected delete 'c' at end")
	}
}

func TestDiffCompleteReplacement(t *testing.T) {
	oldLines := []string{"a", "b", "c"}
	newLines := []string{"x", "y", "z"}
	edits := Diff(oldLines, newLines)

	deletes := 0
	inserts := 0
	for _, e := range edits {
		switch e.Kind {
		case OpDelete:
			deletes++
		case OpInsert:
			inserts++
		}
	}
	if deletes != 3 || inserts != 3 {
		t.Errorf("expected 3 deletes and 3 inserts, got %d/%d", deletes, inserts)
	}
}

func TestDiffMultipleChanges(t *testing.T) {
	oldLines := []string{"a", "b", "c", "d", "e", "f", "g"}
	newLines := []string{"a", "B", "c", "d", "E", "f", "g"}
	edits := Diff(oldLines, newLines)

	changes := 0
	for _, e := range edits {
		if e.Kind != OpEqual {
			changes++
		}
	}
	if changes != 4 {
		t.Errorf("expected 4 non-equal edits, got %d", changes)
	}
}

func TestDiffWithEmptyLines(t *testing.T) {
	oldLines := []string{"a", "", "b"}
	newLines := []string{"a", "", "c"}
	edits := Diff(oldLines, newLines)

	hasDelete := false
	hasInsert := false
	for _, e := range edits {
		if e.Kind == OpDelete && e.Text == "b" {
			hasDelete = true
		}
		if e.Kind == OpInsert && e.Text == "c" {
			hasInsert = true
		}
	}
	if !hasDelete || !hasInsert {
		t.Error("expected change from 'b' to 'c'")
	}
}

func TestDiffIdenticalSingleLine(t *testing.T) {
	edits := Diff([]string{"same"}, []string{"same"})
	if len(edits) != 1 || edits[0].Kind != OpEqual {
		t.Error("expected single OpEqual edit")
	}
}
