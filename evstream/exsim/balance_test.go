package exsim

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"testing"

	"exchange_sim/evstream"
	"exchange_sim/evstream/codecs"
	etypes "exchange_sim/types"
)

// The question this format has to answer is not "is it faster" but "does it
// lose anything". These tests run the full round trip the brief specifies —
// canonical JSON, typed semantic event, binary frame, typed event, canonical
// JSON — and require the two JSON renderings to be byte-identical. A format
// that is faster and lossy is worthless; one that is faster and provably
// lossless can replace JSON on the hot path.

// fromJSONEvent converts the simulator's current event struct into the typed
// semantic form, exactly as a migration would.
func fromJSONEvent(e etypes.BalanceChangeEvent) BalanceChange {
	out := BalanceChange{
		Timestamp:    e.Timestamp,
		ClientID:     e.ClientID,
		Symbol:       e.Symbol,
		PositionSide: e.PositionSide,
		HasSide:      e.PositionSide != "",
		Reason:       e.Reason,
	}
	if e.Changes != nil {
		out.Changes = make([]BalanceDelta, len(e.Changes))
		for i, c := range e.Changes {
			out.Changes[i] = BalanceDelta{
				Asset: c.Asset, Wallet: c.Wallet,
				OldBalance: c.OldBalance, NewBalance: c.NewBalance, Delta: c.Delta,
			}
		}
	}
	return out
}

// roundTrip writes events to a stream and reads them back, returning the
// decoded values plus the writer and reader hashes.
func roundTrip(t *testing.T, events []BalanceChange, compressor evstream.BlockCompressor,
	decompressor evstream.BlockDecompressor) ([]BalanceChange, [32]byte, [32]byte, int) {
	t.Helper()

	var buf bytes.Buffer
	writer := evstream.NewWriter(&buf, evstream.WriterOptions{
		Compressor: compressor,
		BlockBytes: 4096, // small, so the tests exercise many block boundaries
	})
	var encoded EncodedBalanceChange
	for _, event := range events {
		if err := InternBalanceChange(writer, event, &encoded); err != nil {
			t.Fatalf("intern: %v", err)
		}
		if err := writer.Append(event.Timestamp, event.ClientID, 0, &encoded); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	size := buf.Len()

	reader, err := evstream.NewReader(bytes.NewReader(buf.Bytes()), evstream.ReaderOptions{
		Decompressor: decompressor,
		VerifyHash:   true,
	})
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	var got []BalanceChange
	var scratch BalanceChange
	if err := reader.Range(func(frame evstream.Frame) error {
		if err := DecodeBalanceChange(frame, reader, &scratch); err != nil {
			return err
		}
		clone := scratch
		// Preserve the nil-versus-empty distinction in the copy as well. An
		// append onto a nil slice flattens an empty list to nil, which would
		// hide the very difference this test exists to check.
		if scratch.Changes == nil {
			clone.Changes = nil
		} else {
			clone.Changes = append(make([]BalanceDelta, 0, len(scratch.Changes)), scratch.Changes...)
		}
		got = append(got, clone)
		return nil
	}); err != nil {
		t.Fatalf("range: %v", err)
	}
	return got, writer.ExecutionHash(), reader.ExecutionHash(), size
}

// TestJSONToBinaryToJSONIsLossless is the differential the brief asks for.
func TestJSONToBinaryToJSONIsLossless(t *testing.T) {
	source := []etypes.BalanceChangeEvent{
		{Timestamp: 1, ClientID: 2, Symbol: "ABC/USD", Reason: "fill",
			Changes: []etypes.BalanceDelta{{Asset: "USD", Wallet: "perp", OldBalance: 1, NewBalance: 2, Delta: 1}}},
		{Timestamp: 0, ClientID: 0, Symbol: "", Reason: "", Changes: nil},
		{Timestamp: -1, ClientID: math.MaxUint32, Symbol: "ABC-PERP", PositionSide: "BOTH",
			Reason: "funding", Changes: []etypes.BalanceDelta{}},
		{Timestamp: math.MaxInt64, ClientID: 7, Symbol: "ABC-FUT-1735696801",
			PositionSide: "LONG", Reason: "liquidation_repay",
			Changes: []etypes.BalanceDelta{
				{Asset: "USD", Wallet: "perp", OldBalance: math.MinInt64, NewBalance: math.MaxInt64, Delta: -1},
				{Asset: "CDF", Wallet: "spot"},
			}},
	}

	typed := make([]BalanceChange, len(source))
	for i, event := range source {
		typed[i] = fromJSONEvent(event)
	}

	decoded, writerHash, readerHash, _ := roundTrip(t, typed, nil, nil)
	if writerHash != readerHash {
		t.Fatalf("reader hash %x does not match writer hash %x", readerHash, writerHash)
	}
	if len(decoded) != len(source) {
		t.Fatalf("decoded %d events, wrote %d", len(decoded), len(source))
	}

	for i, event := range source {
		want, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal source: %v", err)
		}
		got := decoded[i].AppendJSON(nil)
		if !bytes.Equal(got, want) {
			t.Fatalf("event %d lost information through the round trip\n  binary->json: %s\n  original    : %s",
				i, got, want)
		}
	}
}

// TestRoundTripRandomised is the broad sweep, including the string shapes that
// break naive encoders and the integer extremes that break naive layouts.
func TestRoundTripRandomised(t *testing.T) {
	random := rand.New(rand.NewSource(20260902))
	symbols := []string{"", "ABC/USD", "ABC-PERP", `has"quote`, "<tag>&", "é", "🙂", "ctl\x01"}
	assets := []string{"USD", "ABC", "CDF", ""}
	ints := []int64{0, 1, -1, 987654321, math.MaxInt64, math.MinInt64}

	source := make([]etypes.BalanceChangeEvent, 0, 400)
	for range 400 {
		event := etypes.BalanceChangeEvent{
			Timestamp: ints[random.Intn(len(ints))],
			ClientID:  uint64(random.Intn(1 << 20)),
			Symbol:    symbols[random.Intn(len(symbols))],
			Reason:    symbols[random.Intn(len(symbols))],
		}
		if random.Intn(2) == 0 {
			event.PositionSide = []string{"BOTH", "LONG", "SHORT"}[random.Intn(3)]
		}
		switch random.Intn(3) {
		case 0:
			event.Changes = nil
		case 1:
			event.Changes = []etypes.BalanceDelta{}
		default:
			event.Changes = make([]etypes.BalanceDelta, 1+random.Intn(3))
			for i := range event.Changes {
				event.Changes[i] = etypes.BalanceDelta{
					Asset: assets[random.Intn(len(assets))], Wallet: assets[random.Intn(len(assets))],
					OldBalance: ints[random.Intn(len(ints))],
					NewBalance: ints[random.Intn(len(ints))],
					Delta:      ints[random.Intn(len(ints))],
				}
			}
		}
		source = append(source, event)
	}

	typed := make([]BalanceChange, len(source))
	for i, event := range source {
		typed[i] = fromJSONEvent(event)
	}
	decoded, writerHash, readerHash, _ := roundTrip(t, typed, nil, nil)
	if writerHash != readerHash {
		t.Fatal("reader hash diverged from writer hash")
	}
	for i, event := range source {
		want, _ := json.Marshal(event)
		if got := decoded[i].AppendJSON(nil); !bytes.Equal(got, want) {
			t.Fatalf("event %d: binary->json %s, original %s", i, got, want)
		}
	}
}

// TestHashIsIndependentOfCodec is the property that separates trajectory
// identity from storage. If this ever fails, choosing a compressor would change
// a scientific result, which is precisely what the design forbids.
func TestHashIsIndependentOfCodec(t *testing.T) {
	random := rand.New(rand.NewSource(7))
	events := make([]BalanceChange, 500)
	for i := range events {
		events[i] = BalanceChange{
			Timestamp: int64(i) * 1_000_000, ClientID: uint64(i % 79),
			Symbol: []string{"ABC/USD", "ABC-PERP", "CDF/USD"}[i%3],
			Reason: []string{"fill", "funding", "fee"}[i%3],
			Changes: []BalanceDelta{{
				Asset: "USD", Wallet: "perp",
				OldBalance: random.Int63(), NewBalance: random.Int63(), Delta: random.Int63(),
			}},
		}
	}

	zstd, err := codecs.NewZstdFastest()
	if err != nil {
		t.Fatalf("zstd: %v", err)
	}
	defer zstd.Close()

	cases := []struct {
		name         string
		compressor   evstream.BlockCompressor
		decompressor evstream.BlockDecompressor
	}{
		{"none", nil, nil},
		{"lz4", codecs.NewLZ4(), codecs.NewLZ4()},
		{"s2", codecs.NewS2(), codecs.NewS2()},
		{"zstd", zstd, zstd},
	}

	var reference [32]byte
	for i, tc := range cases {
		decoded, writerHash, readerHash, size := roundTrip(t, events, tc.compressor, tc.decompressor)
		if writerHash != readerHash {
			t.Fatalf("%s: reader hash diverged from writer hash", tc.name)
		}
		if len(decoded) != len(events) {
			t.Fatalf("%s: decoded %d of %d events", tc.name, len(decoded), len(events))
		}
		if i == 0 {
			reference = writerHash
			t.Logf("%-5s stored %d bytes (reference)", tc.name, size)
			continue
		}
		if writerHash != reference {
			t.Fatalf("%s changed the execution hash: %x vs %x — compression must be a storage concern only",
				tc.name, writerHash, reference)
		}
		t.Logf("%-5s stored %d bytes", tc.name, size)
	}
}

// TestDecodeRejectsTrailingBytes covers schema-version drift: a payload longer
// than its version describes must fail rather than decode the prefix and
// silently discard the rest.
func TestDecodeRejectsTrailingBytes(t *testing.T) {
	var encoded EncodedBalanceChange
	dict := newTestInterner()
	if err := InternBalanceChange(dict, BalanceChange{Symbol: "ABC/USD", Reason: "fill"}, &encoded); err != nil {
		t.Fatalf("intern: %v", err)
	}
	payload := append(encoded.AppendPayload(nil), 0xff)
	frame := evstream.Frame{Payload: payload}
	var into BalanceChange
	if err := DecodeBalanceChange(frame, dict, &into); err == nil {
		t.Fatal("decoded a payload with trailing bytes; a schema change would pass unnoticed")
	}
}

// testInterner is a dictionary without a stream behind it, so schema encoding
// can be tested in isolation from framing.
type testInterner struct{ dict *evstream.Dictionary }

func newTestInterner() *testInterner { return &testInterner{dict: evstream.NewDictionary()} }

func (t *testInterner) Intern(s string) (uint32, error) {
	if id, ok := t.dict.Lookup(s); ok {
		return id, nil
	}
	return t.dict.Assign(s), nil
}

func (t *testInterner) Lookup(id uint32) (string, bool) { return t.dict.Value(id) }
