package main

import "testing"

// A symbol is not a path. filepath.Match refuses to let "*" cross a separator,
// so the catch-all pattern silently failed to match every spot book and those
// books were judged against whichever thresholds the flags happened to carry.
func TestMatchSymbolCrossesTheSlashInABookName(t *testing.T) {
	cases := []struct {
		pattern, symbol string
		want            bool
	}{
		{"*", "ABC/USD", true},
		{"*", "ABC-1735696803-48000-C", true},
		{"*-C", "ABC-1735696803-48000-C", true},
		{"*-C", "ABC-1735696803-48000-P", false},
		{"ABC-FUT-*", "ABC-FUT-1735696801", true},
		{"ABC-FUT-*", "ABC-PERP", false},
		{"ABC-PERP", "ABC-PERP", true},
		{"ABC-PERP", "ABC/USD", false},
		{"ABC/*", "ABC/USD", true},
		{"*/USD", "CDF/USD", true},
		{"*/USD", "ABC/CDF", false},
		{"A*C*D", "ABCD", true},
		{"A*C*D", "ABDC", false},
	}
	for _, c := range cases {
		if got := matchSymbol(c.pattern, c.symbol); got != c.want {
			t.Errorf("matchSymbol(%q, %q) = %v, want %v", c.pattern, c.symbol, got, c.want)
		}
	}
}
