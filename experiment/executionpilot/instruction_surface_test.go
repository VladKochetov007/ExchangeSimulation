package executionpilot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstructionCellsAreExactlyLockedMatrix(t *testing.T) {
	cells := InstructionCells()
	if len(cells) != 12 {
		t.Fatalf("ME-003 matrix has %d cells", len(cells))
	}
	seen := map[string]bool{}
	for _, cell := range cells {
		id, err := instructionCellID(cell)
		if err != nil || seen[id] {
			t.Fatalf("invalid or duplicate cell %s: %v", id, err)
		}
		seen[id] = true
	}
	if first, _ := instructionCellID(cells[0]); first != "dev-ioc-50000000-14001" {
		t.Fatalf("unexpected first cell %s", first)
	}
	if last, _ := instructionCellID(cells[len(cells)-1]); last != "dev-fok-500000000-14017" {
		t.Fatalf("unexpected last cell %s", last)
	}
}

func TestInstructionResourceMeasurementFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource.txt")
	for _, test := range []struct {
		name  string
		wall  string
		rss   string
		exit  string
		valid bool
	}{
		{"valid", "0:00.40", "41344", "0", true},
		{"one_hour", "1:00:00.40", "41344", "0", false},
		{"too_slow", "0:31.00", "41344", "0", false},
		{"too_large", "0:00.40", "4194305", "0", false},
		{"nan", "0:NaN", "41344", "0", false},
		{"failed_exit", "0:00.40", "41344", "1", false},
		{"missing_rss", "0:00.40", "", "0", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			content := "Elapsed (wall clock) time (h:mm:ss or m:ss): " + test.wall + "\n" +
				"Maximum resident set size (kbytes): " + test.rss + "\n" +
				"Exit status: " + test.exit + "\n"
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			_, _, err := readInstructionResource(path)
			if (err == nil) != test.valid {
				t.Fatalf("resource validity=%t err=%v", test.valid, err)
			}
		})
	}
}
