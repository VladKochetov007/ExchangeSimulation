package exsim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"exchange_sim/evstream"
)

// The architecture question is whether a canonical binary stream plus block
// indexes can serve selective analytics without a columnar layer. These
// benchmarks answer it per query class, because a blended average would hide
// the one class where a row-major format is structurally weak.
//
//	A  selective metric      one symbol, narrow time window, one family
//	B  broad aggregate       all symbols, one family, whole run
//	C  causal replay         every event in order
//	D  cross-family study    both families in a time window
//
// The expectation going in is that the index carries A and D, that C is a pure
// sequential decode where the binary format should win outright, and that B is
// the hard case: families interleave in a globally ordered stream, so a family
// bitmap skips almost nothing and the reader pays for whole records.

const (
	queryEvents  = 120000
	querySymbols = 5
	queryStartTS = 1735689600000000000
	queryStepTS  = 1_000_000 // 1 ms between events
)

// queryCorpus is one mixed-family stream plus the JSONL equivalent, so both
// representations describe exactly the same events.
type queryCorpus struct {
	stream     []byte
	index      *evstream.Index
	dict       *evstream.Dictionary
	jsonLines  [][]byte
	symbolRefs []uint32
	symbols    []string
	endTS      int64
}

func buildQueryCorpus(tb testing.TB) *queryCorpus {
	tb.Helper()
	random := rand.New(rand.NewSource(20260903))
	symbols := make([]string, querySymbols)
	for i := range symbols {
		symbols[i] = fmt.Sprintf("SYM%02d/USD", i)
	}

	var buf bytes.Buffer
	writer := evstream.NewWriter(&buf, evstream.WriterOptions{})
	var (
		balance  EncodedBalanceChange
		delta    EncodedBookDelta
		lines    = make([][]byte, 0, queryEvents)
		refs     = make([]uint32, querySymbols)
		lastTS   int64
		jsonline []byte
	)
	for i := range symbols {
		ref, err := writer.Intern(symbols[i])
		if err != nil {
			tb.Fatalf("intern: %v", err)
		}
		refs[i] = ref
	}

	for i := range queryEvents {
		ts := int64(queryStartTS + int64(i)*queryStepTS)
		lastTS = ts
		symbolIndex := random.Intn(querySymbols)
		symbol := symbols[symbolIndex]

		// Two families out of three events, matching the real mix where book
		// deltas dominate by count and balance changes by bytes.
		if i%3 == 0 {
			event := BalanceChange{
				Timestamp: ts, ClientID: uint64(random.Intn(79)), Symbol: symbol,
				Reason: "fill",
				Changes: []BalanceDelta{{
					Asset: "USD", Wallet: "perp",
					OldBalance: random.Int63n(1 << 50), NewBalance: random.Int63n(1 << 50),
					Delta: random.Int63n(1 << 30),
				}},
			}
			if err := InternBalanceChange(writer, event, &balance); err != nil {
				tb.Fatalf("intern balance: %v", err)
			}
			if err := writer.Append(ts, event.ClientID, refs[symbolIndex], &balance); err != nil {
				tb.Fatalf("append balance: %v", err)
			}
			jsonline, _ = json.Marshal(map[string]any{
				"event": "balance_change", "sim_ts": ts, "symbol": symbol,
				"client_id": event.ClientID, "reason": event.Reason,
				"old_balance": event.Changes[0].OldBalance,
				"new_balance": event.Changes[0].NewBalance,
				"delta":       event.Changes[0].Delta,
			})
		} else {
			event := BookDelta{
				Timestamp: ts, Symbol: symbol, Side: uint8(i % 2),
				Price: random.Int63n(1 << 40), VisibleQty: random.Int63n(1 << 30),
				HiddenQty: 0, TotalQty: random.Int63n(1 << 30),
			}
			if err := InternBookDelta(writer, event, &delta); err != nil {
				tb.Fatalf("intern delta: %v", err)
			}
			if err := writer.Append(ts, 0, refs[symbolIndex], &delta); err != nil {
				tb.Fatalf("append delta: %v", err)
			}
			jsonline, _ = json.Marshal(map[string]any{
				"event": "book_delta", "sim_ts": ts, "symbol": symbol,
				"side": event.Side, "price": event.Price,
				"visible_qty": event.VisibleQty, "hidden_qty": event.HiddenQty,
				"total_qty": event.TotalQty,
			})
		}
		lines = append(lines, jsonline)
	}
	if err := writer.Flush(); err != nil {
		tb.Fatalf("flush: %v", err)
	}

	return &queryCorpus{
		stream: buf.Bytes(), index: writer.Index(), dict: writer.Dictionary(),
		jsonLines: lines, symbolRefs: refs, symbols: symbols, endTS: lastTS,
	}
}

// jsonScan is the analyzer's current shape: decode every record, filter in Go.
func jsonScan(tb testing.TB, corpus *queryCorpus, keep func(map[string]any) bool) int {
	matched := 0
	for _, line := range corpus.jsonLines {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			tb.Fatalf("unmarshal: %v", err)
		}
		if keep(record) {
			matched++
		}
	}
	return matched
}

// binaryFullScan reads every frame sequentially, with full verification.
func binaryFullScan(tb testing.TB, corpus *queryCorpus, q evstream.Query) int {
	reader, err := evstream.NewReader(bytes.NewReader(corpus.stream), evstream.ReaderOptions{})
	if err != nil {
		tb.Fatalf("reader: %v", err)
	}
	matched := 0
	if err := reader.Range(func(frame evstream.Frame) error {
		if q.Matches(frame.Header) {
			matched++
		}
		return nil
	}); err != nil {
		tb.Fatalf("range: %v", err)
	}
	return matched
}

// binaryIndexedScan reads only the blocks the index selects.
func binaryIndexedScan(tb testing.TB, corpus *queryCorpus, q evstream.Query) (int, int) {
	blocks, skipped := corpus.index.Select(q)
	reader := evstream.NewIndexedReader(bytes.NewReader(corpus.stream),
		evstream.CodecNone, corpus.dict, nil)
	matched := 0
	if err := reader.RangeSelected(blocks, q, func(frame evstream.Frame) error {
		matched++
		return nil
	}); err != nil {
		tb.Fatalf("selected range: %v", err)
	}
	return matched, skipped
}

// TestQueryClassesAgree is the correctness gate for the benchmarks below: an
// indexed scan that skipped a block holding a match would look wonderfully fast
// and be wrong, so every class is first checked to return the same count three
// ways.
func TestQueryClassesAgree(t *testing.T) {
	corpus := buildQueryCorpus(t)
	windowStart := queryStartTS + int64(queryEvents/2)*queryStepTS
	windowEnd := windowStart + int64(queryEvents/100)*queryStepTS

	cases := []struct {
		name  string
		query evstream.Query
		keep  func(map[string]any) bool
	}{
		{
			name: "A selective",
			query: evstream.Query{
				FromTS: windowStart, ToTS: windowEnd,
				Families:   []uint16{SchemaBalanceChange},
				SymbolRefs: []uint32{corpus.symbolRefs[2]},
			},
			keep: func(r map[string]any) bool {
				ts := int64(r["sim_ts"].(float64))
				return r["event"] == "balance_change" && r["symbol"] == corpus.symbols[2] &&
					ts >= windowStart && ts <= windowEnd
			},
		},
		{
			name:  "B broad aggregate",
			query: evstream.Query{Families: []uint16{SchemaBookDelta}},
			keep:  func(r map[string]any) bool { return r["event"] == "book_delta" },
		},
		{
			name:  "C causal replay",
			query: evstream.Query{},
			keep:  func(map[string]any) bool { return true },
		},
		{
			name:  "D cross-family window",
			query: evstream.Query{FromTS: windowStart, ToTS: windowEnd},
			keep: func(r map[string]any) bool {
				ts := int64(r["sim_ts"].(float64))
				return ts >= windowStart && ts <= windowEnd
			},
		},
	}

	for _, tc := range cases {
		wantJSON := jsonScan(t, corpus, tc.keep)
		gotFull := binaryFullScan(t, corpus, tc.query)
		gotIndexed, skipped := binaryIndexedScan(t, corpus, tc.query)
		if gotFull != wantJSON {
			t.Fatalf("%s: binary full scan matched %d, JSON scan %d", tc.name, gotFull, wantJSON)
		}
		if gotIndexed != wantJSON {
			t.Fatalf("%s: indexed scan matched %d, JSON scan %d — the index dropped events",
				tc.name, gotIndexed, wantJSON)
		}
		t.Logf("%-24s matched %7d   blocks skipped %d of %d",
			tc.name, wantJSON, skipped, len(corpus.index.Blocks))
	}
}

// The timing counterpart to TestQueryClassesAgree. Each class is reported
// separately; there is deliberately no combined figure, because the whole point
// is that the classes behave differently.

func benchQuery(b *testing.B, build func(*queryCorpus) (evstream.Query, func(map[string]any) bool)) {
	corpus := buildQueryCorpus(b)
	query, keep := build(corpus)

	b.Run("json-scan", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			jsonScan(b, corpus, keep)
		}
	})
	b.Run("binary-full", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			binaryFullScan(b, corpus, query)
		}
	})
	b.Run("binary-indexed", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			binaryIndexedScan(b, corpus, query)
		}
	})
}

func BenchmarkQueryASelective(b *testing.B) {
	benchQuery(b, func(c *queryCorpus) (evstream.Query, func(map[string]any) bool) {
		start := int64(queryStartTS + int64(queryEvents/2)*queryStepTS)
		end := start + int64(queryEvents/100)*queryStepTS
		return evstream.Query{
				FromTS: start, ToTS: end,
				Families:   []uint16{SchemaBalanceChange},
				SymbolRefs: []uint32{c.symbolRefs[2]},
			}, func(r map[string]any) bool {
				ts := int64(r["sim_ts"].(float64))
				return r["event"] == "balance_change" && r["symbol"] == c.symbols[2] &&
					ts >= start && ts <= end
			}
	})
}

func BenchmarkQueryBBroadAggregate(b *testing.B) {
	benchQuery(b, func(c *queryCorpus) (evstream.Query, func(map[string]any) bool) {
		return evstream.Query{Families: []uint16{SchemaBookDelta}},
			func(r map[string]any) bool { return r["event"] == "book_delta" }
	})
}

func BenchmarkQueryCCausalReplay(b *testing.B) {
	benchQuery(b, func(c *queryCorpus) (evstream.Query, func(map[string]any) bool) {
		return evstream.Query{}, func(map[string]any) bool { return true }
	})
}

func BenchmarkQueryDCrossFamilyWindow(b *testing.B) {
	benchQuery(b, func(c *queryCorpus) (evstream.Query, func(map[string]any) bool) {
		start := int64(queryStartTS + int64(queryEvents/2)*queryStepTS)
		end := start + int64(queryEvents/100)*queryStepTS
		return evstream.Query{FromTS: start, ToTS: end},
			func(r map[string]any) bool {
				ts := int64(r["sim_ts"].(float64))
				return ts >= start && ts <= end
			}
	})
}
