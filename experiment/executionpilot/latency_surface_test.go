package executionpilot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLatencyCellsAreExactlyTheProspectiveMatrix(t *testing.T) {
	cells := LatencyCells()
	if len(cells) != 24 {
		t.Fatalf("matrix has %d cells", len(cells))
	}
	seen := map[string]bool{}
	for _, cell := range cells {
		id, err := latencyCellID(cell)
		if err != nil || seen[id] {
			t.Fatalf("invalid or duplicate cell %s: %v", id, err)
		}
		seen[id] = true
	}
	if first, _ := latencyCellID(cells[0]); first != "F-F-q0p5-s12001" {
		t.Fatalf("wrong first cell %s", first)
	}
	if last, _ := latencyCellID(cells[len(cells)-1]); last != "S-S-q5-s12017" {
		t.Fatalf("wrong last cell %s", last)
	}
}

func TestLatencyResourceStampFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource.txt")
	for _, test := range []struct {
		raw   string
		valid bool
	}{
		{"wall_seconds=0.34 peak_rss_kib=33024 exit_status=0\n", true},
		{"wall_seconds=0.34 peak_rss_kib=33024 exit_status=1\n", false},
		{"wall_seconds=31 peak_rss_kib=33024 exit_status=0\n", false},
		{"wall_seconds=0.34 peak_rss_kib=99999999 exit_status=0\n", false},
		{"wall_seconds=NaN peak_rss_kib=33024 exit_status=0\n", false},
	} {
		if err := os.WriteFile(path, []byte(test.raw), 0600); err != nil {
			t.Fatal(err)
		}
		_, _, err := readResourceStamp(path)
		if (err == nil) != test.valid {
			t.Fatalf("resource stamp %q validity=%v err=%v", test.raw, test.valid, err)
		}
	}
}
