package analysis

import (
	"bytes"
	"fmt"
	"os"
	"strings"
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

// retainedCorpusLines returns the lines of the corpus named by
// MVANALYZE_BENCH_CORPUS, or ok=false when no corpus is configured. Tests that
// need real evidence records share it.
func retainedCorpusLines(t *testing.T) ([][]byte, bool) {
	t.Helper()
	path := os.Getenv("MVANALYZE_BENCH_CORPUS")
	if path == "" {
		t.Skip("set MVANALYZE_BENCH_CORPUS to a JSONL evidence file")
		return nil, false
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(blob, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines, true
}

// TestNeedleSetMatchesContainsAny is the equivalence requirement: the automaton
// must admit exactly the lines the per-needle search admits, because the
// prefilter decides which malformed records a metric attempts to decode and so
// which failures it reports.
func TestNeedleSetMatchesContainsAny(t *testing.T) {
	needleGroups := [][]string{
		{"OrderAccepted"},
		{"OrderAccepted", "OrderFill"},
		benchNeedleNames,
		{"a", "aa", "aaa"},
		{"abc", "bcd", "cde"},
		{"Trade", "Trade"},
		{"x"},
	}
	lines := []string{
		"", "x", `{"event":"OrderAccepted"}`, `{"event":"Trade"}`,
		`{"event":"Unrelated","note":"OrderFill"}`, "aaaa", "aab", "abcde",
		`{"a":1}`, "OrderAccepted", `"OrderAccepted"`, "Order", "Accepted",
		"\x00\xff\xfe binary", strings.Repeat("z", 500) + "OrderFill",
	}
	for _, names := range needleGroups {
		needles := make([][]byte, 0, len(names))
		for _, name := range names {
			needles = append(needles, []byte(`"`+name+`"`))
		}
		set := newNeedleSet(needles)
		for _, line := range lines {
			want := containsAny([]byte(line), needles)
			if got := set.matches([]byte(line)); got != want {
				t.Fatalf("needles %v line %q: automaton %v, per-needle search %v",
					names, line, got, want)
			}
		}
	}
}

// TestNeedleSetMatchesContainsAnyOverRetainedEvidence holds the automaton to the
// per-needle search over every line of a real evidence corpus.
func TestNeedleSetMatchesContainsAnyOverRetainedEvidence(t *testing.T) {
	lines, ok := retainedCorpusLines(t)
	if !ok {
		return
	}
	needles := benchNeedles()
	set := newNeedleSet(needles)
	for index, line := range lines {
		if set.matches(line) != containsAny(line, needles) {
			t.Fatalf("line %d: automaton disagrees with the per-needle search", index+1)
		}
	}
}

func BenchmarkPrefilterNeedleSet(b *testing.B) {
	lines, size := benchCorpus(b)
	set := newNeedleSet(benchNeedles())
	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hits := 0
		for _, line := range lines {
			if set.matches(line) {
				hits++
			}
		}
		if hits == 0 {
			b.Fatal("no lines matched")
		}
	}
}

// BenchmarkPrefilterFiveNeedles reflects a single metric's scan, where the
// needle count is small and the standard library's assembly search is strongest.
func BenchmarkPrefilterFiveNeedles(b *testing.B) {
	lines, size := benchCorpus(b)
	names := benchNeedleNames[:5]
	needles := make([][]byte, 0, len(names))
	for _, name := range names {
		needles = append(needles, []byte(`"`+name+`"`))
	}
	b.Run("perNeedle", func(b *testing.B) {
		b.SetBytes(int64(size))
		for i := 0; i < b.N; i++ {
			for _, line := range lines {
				_ = containsAny(line, needles)
			}
		}
	})
	set := newNeedleSet(needles)
	b.Run("needleSet", func(b *testing.B) {
		b.SetBytes(int64(size))
		for i := 0; i < b.N; i++ {
			for _, line := range lines {
				_ = set.matches(line)
			}
		}
	})
}

// BenchmarkPrefilterByNeedleCount locates the crossover between the standard
// library's per-needle assembly search and the single-pass automaton. The
// per-needle cost grows with the needle count while the automaton's does not,
// so which one wins is a property of the scan, not a constant.
func BenchmarkPrefilterByNeedleCount(b *testing.B) {
	lines, size := benchCorpus(b)
	for _, count := range []int{4, 6, 7, 8, 10, 12} {
		names := benchNeedleNames[:count]
		needles := make([][]byte, 0, count)
		for _, name := range names {
			needles = append(needles, []byte(`"`+name+`"`))
		}
		set := newNeedleSet(needles)
		b.Run(fmt.Sprintf("n%d/perNeedle", count), func(b *testing.B) {
			b.SetBytes(int64(size))
			for i := 0; i < b.N; i++ {
				for _, line := range lines {
					_ = containsAny(line, needles)
				}
			}
		})
		b.Run(fmt.Sprintf("n%d/needleSet", count), func(b *testing.B) {
			b.SetBytes(int64(size))
			for i := 0; i < b.N; i++ {
				for _, line := range lines {
					_ = set.matches(line)
				}
			}
		})
	}
}
