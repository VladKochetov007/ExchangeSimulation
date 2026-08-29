package analysis

import (
	"bytes"
	"os"
	"testing"
)

// The prefilter decides, per evidence line, whether any registered event name
// occurs in it. It runs on every line of every pass, so its ceiling is worth
// knowing exactly: these benchmarks compare the current hand-rolled search with
// the standard library's assembly-backed one over a real evidence corpus.
//
// Set MVANALYZE_BENCH_CORPUS to a JSONL file to run them.

var benchNeedleNames = []string{
	"OrderAccepted", "OrderFill", "OrderCancelled", "OrderRejected",
	"Trade", "BookDelta", "balance_change", "position_update",
	"realized_pnl", "mark_price_update", "funding_rate_update", "open_interest",
}

func benchCorpus(b *testing.B) ([][]byte, int) {
	b.Helper()
	path := os.Getenv("MVANALYZE_BENCH_CORPUS")
	if path == "" {
		b.Skip("set MVANALYZE_BENCH_CORPUS to a JSONL evidence file")
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	lines := bytes.Split(blob, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines, len(blob)
}

func benchNeedles() [][]byte {
	needles := make([][]byte, 0, len(benchNeedleNames))
	for _, name := range benchNeedleNames {
		needles = append(needles, []byte(`"`+name+`"`))
	}
	return needles
}

// containsAnyHandRolled is the search the analyzer used before adopting the
// standard library, kept so the two can be held to identical selection. Both
// compute exact-match substring containment, so they must accept exactly the
// same lines.
func containsAnyHandRolled(line []byte, needles [][]byte) bool {
	for _, needle := range needles {
		if handRolledContains(line, needle) {
			return true
		}
	}
	return false
}

func handRolledContains(haystack, needle []byte) bool {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return false
	}
	first := needle[0]
	limit := len(haystack) - len(needle)
	for i := 0; i <= limit; i++ {
		if haystack[i] != first {
			continue
		}
		match := true
		for j := 1; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func BenchmarkPrefilterHandRolled(b *testing.B) {
	lines, size := benchCorpus(b)
	needles := benchNeedles()
	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hits := 0
		for _, line := range lines {
			if containsAnyHandRolled(line, needles) {
				hits++
			}
		}
		if hits == 0 {
			b.Fatal("no lines matched; corpus or needles are wrong")
		}
	}
}

func BenchmarkPrefilterCurrent(b *testing.B) {
	lines, size := benchCorpus(b)
	needles := benchNeedles()
	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hits := 0
		for _, line := range lines {
			if containsAny(line, needles) {
				hits++
			}
		}
		if hits == 0 {
			b.Fatal("no lines matched; corpus or needles are wrong")
		}
	}
}

// TestPrefilterSelectionIdentical is the equivalence check the benchmark rests
// on: both searches must accept exactly the same lines.
func TestPrefilterSelectionIdentical(t *testing.T) {
	path := os.Getenv("MVANALYZE_BENCH_CORPUS")
	if path == "" {
		t.Skip("set MVANALYZE_BENCH_CORPUS to a JSONL evidence file")
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	needles := benchNeedles()
	for index, line := range bytes.Split(blob, []byte("\n")) {
		if containsAny(line, needles) != containsAnyHandRolled(line, needles) {
			t.Fatalf("line %d: searches disagree", index+1)
		}
	}
}
