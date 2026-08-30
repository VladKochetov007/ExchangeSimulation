package exsim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"exchange_sim/evstream"
	"exchange_sim/evstream/codecs"
	etypes "exchange_sim/types"
)

// benchmarkPopulation builds events shaped like the ones the integrated run
// actually produces: a handful of repeated symbols, assets and reasons, one or
// two deltas each, large fixed-point balances. Drawing from a small string set
// is not a convenience — it is the property that makes dictionary interning
// worth having, and a benchmark on random strings would measure a workload this
// simulator never runs.
func benchmarkPopulation(n int) []etypes.BalanceChangeEvent {
	random := rand.New(rand.NewSource(20260902))
	symbols := []string{"ABC/USD", "CDF/USD", "ABC/CDF", "ABC-PERP", "ABC-FUT-1735696801"}
	assets := []string{"USD", "ABC", "CDF"}
	wallets := []string{"perp", "spot"}
	reasons := []string{"fill", "funding", "fee", "liquidation_repay", "interest_charge"}

	out := make([]etypes.BalanceChangeEvent, n)
	for i := range out {
		count := 1 + random.Intn(2)
		changes := make([]etypes.BalanceDelta, count)
		for j := range changes {
			old := random.Int63n(1 << 50)
			delta := random.Int63n(1<<30) - (1 << 29)
			changes[j] = etypes.BalanceDelta{
				Asset:      assets[random.Intn(len(assets))],
				Wallet:     wallets[random.Intn(len(wallets))],
				OldBalance: old,
				NewBalance: old + delta,
				Delta:      delta,
			}
		}
		out[i] = etypes.BalanceChangeEvent{
			Timestamp: 1735689600000000000 + int64(i)*1_000_000,
			ClientID:  uint64(random.Intn(79)),
			Symbol:    symbols[random.Intn(len(symbols))],
			Reason:    reasons[random.Intn(len(reasons))],
			Changes:   changes,
		}
		if random.Intn(3) == 0 {
			out[i].PositionSide = "BOTH"
		}
	}
	return out
}

func typedPopulation(source []etypes.BalanceChangeEvent) []BalanceChange {
	out := make([]BalanceChange, len(source))
	for i, event := range source {
		out[i] = fromJSONEvent(event)
	}
	return out
}

const benchEvents = 20000

// BenchmarkEncodeJSON is the current hot path: encoding/json over the event
// struct, which is what feeds the ordered execution-stream digest today.
func BenchmarkEncodeJSON(b *testing.B) {
	source := benchmarkPopulation(benchEvents)
	b.ReportAllocs()
	b.ResetTimer()

	var bytesOut int64
	for b.Loop() {
		for i := range source {
			encoded, err := json.Marshal(source[i])
			if err != nil {
				b.Fatal(err)
			}
			bytesOut += int64(len(encoded))
		}
	}
	reportPerEvent(b, bytesOut)
}

// BenchmarkEncodeBinary is the same events through the canonical binary format,
// including framing, interning and the SHA-256 execution hash — everything the
// JSON path also pays for, so the comparison is like for like.
func BenchmarkEncodeBinary(b *testing.B) {
	events := typedPopulation(benchmarkPopulation(benchEvents))
	b.ReportAllocs()
	b.ResetTimer()

	var bytesOut int64
	for b.Loop() {
		counter := &countingWriter{}
		writer := evstream.NewWriter(counter, evstream.WriterOptions{})
		var encoded EncodedBalanceChange
		for i := range events {
			if err := InternBalanceChange(writer, events[i], &encoded); err != nil {
				b.Fatal(err)
			}
			if err := writer.Append(events[i].Timestamp, events[i].ClientID, 0, &encoded); err != nil {
				b.Fatal(err)
			}
		}
		if err := writer.Flush(); err != nil {
			b.Fatal(err)
		}
		bytesOut += counter.n
	}
	reportPerEvent(b, bytesOut)
}

func benchmarkEncodeCompressed(b *testing.B, compressor evstream.BlockCompressor) {
	events := typedPopulation(benchmarkPopulation(benchEvents))
	b.ReportAllocs()
	b.ResetTimer()

	var bytesOut int64
	for b.Loop() {
		counter := &countingWriter{}
		writer := evstream.NewWriter(counter, evstream.WriterOptions{Compressor: compressor})
		var encoded EncodedBalanceChange
		for i := range events {
			if err := InternBalanceChange(writer, events[i], &encoded); err != nil {
				b.Fatal(err)
			}
			if err := writer.Append(events[i].Timestamp, events[i].ClientID, 0, &encoded); err != nil {
				b.Fatal(err)
			}
		}
		if err := writer.Flush(); err != nil {
			b.Fatal(err)
		}
		bytesOut += counter.n
	}
	reportPerEvent(b, bytesOut)
}

func BenchmarkEncodeBinaryLZ4(b *testing.B) { benchmarkEncodeCompressed(b, codecs.NewLZ4()) }
func BenchmarkEncodeBinaryS2(b *testing.B)  { benchmarkEncodeCompressed(b, codecs.NewS2()) }

func BenchmarkEncodeBinaryZstd(b *testing.B) {
	zstd, err := codecs.NewZstdFastest()
	if err != nil {
		b.Fatal(err)
	}
	defer zstd.Close()
	benchmarkEncodeCompressed(b, zstd)
}

// BenchmarkDecodeJSON reads the events back the way an analyzer does today.
func BenchmarkDecodeJSON(b *testing.B) {
	source := benchmarkPopulation(benchEvents)
	lines := make([][]byte, len(source))
	for i := range source {
		encoded, err := json.Marshal(source[i])
		if err != nil {
			b.Fatal(err)
		}
		lines[i] = encoded
	}
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var event etypes.BalanceChangeEvent
		for i := range lines {
			if err := json.Unmarshal(lines[i], &event); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkDecodeBinary reads the same events from a canonical stream, with
// full structural verification: block CRCs, frame counts and gap-free
// sequence. The JSON path has no equivalent check at all, so this is if
// anything the harder comparison.
func BenchmarkDecodeBinary(b *testing.B) {
	events := typedPopulation(benchmarkPopulation(benchEvents))
	var buf bytes.Buffer
	writer := evstream.NewWriter(&buf, evstream.WriterOptions{})
	var encoded EncodedBalanceChange
	for i := range events {
		if err := InternBalanceChange(writer, events[i], &encoded); err != nil {
			b.Fatal(err)
		}
		if err := writer.Append(events[i].Timestamp, events[i].ClientID, 0, &encoded); err != nil {
			b.Fatal(err)
		}
	}
	if err := writer.Flush(); err != nil {
		b.Fatal(err)
	}
	stream := buf.Bytes()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		reader, err := evstream.NewReader(bytes.NewReader(stream), evstream.ReaderOptions{})
		if err != nil {
			b.Fatal(err)
		}
		var into BalanceChange
		if err := reader.Range(func(frame evstream.Frame) error {
			return DecodeBalanceChange(frame, reader, &into)
		}); err != nil {
			b.Fatal(err)
		}
	}
}

// reportPerEvent turns the loop total into the number that matters for
// capacity planning: bytes on disk per event.
func reportPerEvent(b *testing.B, bytesOut int64) {
	if b.N == 0 {
		return
	}
	perEvent := float64(bytesOut) / float64(b.N) / float64(benchEvents)
	b.ReportMetric(perEvent, "bytes/event")
}

type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

var _ io.Writer = (*countingWriter)(nil)

// TestReportSizes prints the size comparison as a table, which is easier to
// read than benchmark output and is the number the storage decision turns on.
func TestReportSizes(t *testing.T) {
	source := benchmarkPopulation(benchEvents)
	events := typedPopulation(source)

	jsonBytes := 0
	for i := range source {
		encoded, err := json.Marshal(source[i])
		if err != nil {
			t.Fatal(err)
		}
		// One newline per record, as JSONL evidence is actually stored.
		jsonBytes += len(encoded) + 1
	}

	zstd, err := codecs.NewZstdFastest()
	if err != nil {
		t.Fatal(err)
	}
	defer zstd.Close()

	sizes := []struct {
		name       string
		compressor evstream.BlockCompressor
	}{
		{"binary", nil},
		{"binary+lz4", codecs.NewLZ4()},
		{"binary+s2", codecs.NewS2()},
		{"binary+zstd", zstd},
	}

	t.Logf("%-12s %12s %10s %14s", "format", "bytes", "b/event", "vs JSONL")
	t.Logf("%-12s %12d %10.1f %13s", "jsonl", jsonBytes, float64(jsonBytes)/benchEvents, "1.00x")
	for _, size := range sizes {
		counter := &countingWriter{}
		writer := evstream.NewWriter(counter, evstream.WriterOptions{Compressor: size.compressor})
		var encoded EncodedBalanceChange
		for i := range events {
			if err := InternBalanceChange(writer, events[i], &encoded); err != nil {
				t.Fatal(err)
			}
			if err := writer.Append(events[i].Timestamp, events[i].ClientID, 0, &encoded); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Flush(); err != nil {
			t.Fatal(err)
		}
		t.Logf("%-12s %12d %10.1f %12s", size.name, counter.n, float64(counter.n)/benchEvents,
			fmt.Sprintf("%.2fx", float64(jsonBytes)/float64(counter.n)))
	}
}
