package types

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"exchange_sim/evstream"
)

type permissiveSchemaResolver struct {
	delegate evstream.Resolver
}

func (r permissiveSchemaResolver) Lookup(ref uint32) (string, bool) {
	if ref == 0 {
		return "", true
	}
	return r.delegate.Lookup(ref)
}

func TestRequiredTypeSchemasRejectEmptyDictionaryValues(t *testing.T) {
	for _, payload := range []evstream.InterningAppender{BalanceChangeEvent{}, FeeRevenueEvent{}} {
		var output bytes.Buffer
		writer := evstream.NewWriter(&output, evstream.WriterOptions{})
		if err := writer.AppendInterning(1, 1, 0, payload); !errors.Is(err, evstream.ErrEmptyDictionaryValue) {
			t.Fatalf("empty required type field error = %v, want ErrEmptyDictionaryValue", err)
		}
	}
}

func TestRequiredTypeSchemasRejectZeroReference(t *testing.T) {
	t.Run("balance change symbol", func(t *testing.T) {
		frame, reader := writeTypeFrame(t, BalanceChangeEvent{Symbol: "ABC/USD", Reason: "fill"})
		cursor := evstream.NewCursor(frame.Payload)
		cursor.Presence(3)
		cursor.Int64()
		cursor.Uint64()
		if err := cursor.Err(); err != nil {
			t.Fatal(err)
		}
		binary.LittleEndian.PutUint32(frame.Payload[cursor.Offset():], 0)
		var decoded BalanceChangeEvent
		if err := DecodeBalanceChange(frame.Payload, permissiveSchemaResolver{delegate: reader}, &decoded); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("zero balance-change reference error = %v, want ErrCorrupt", err)
		}
	})

	t.Run("fee revenue symbol", func(t *testing.T) {
		frame, reader := writeTypeFrame(t, FeeRevenueEvent{Symbol: "ABC/USD", Asset: "USD"})
		cursor := evstream.NewCursor(frame.Payload)
		for range 4 {
			cursor.Int64()
		}
		if err := cursor.Err(); err != nil {
			t.Fatal(err)
		}
		binary.LittleEndian.PutUint32(frame.Payload[cursor.Offset():], 0)
		var decoded FeeRevenueEvent
		if err := DecodeFeeRevenue(frame.Payload, permissiveSchemaResolver{delegate: reader}, &decoded); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("zero fee-revenue reference error = %v, want ErrCorrupt", err)
		}
	})
}

func writeTypeFrame(t *testing.T, payload evstream.InterningAppender) (evstream.Frame, *evstream.Reader) {
	t.Helper()
	var output bytes.Buffer
	writer := evstream.NewWriter(&output, evstream.WriterOptions{})
	if err := writer.AppendInterning(1, 1, 0, payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := evstream.NewReader(bytes.NewReader(output.Bytes()), evstream.ReaderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var frame evstream.Frame
	if err := reader.Range(func(candidate evstream.Frame) error {
		frame.Header = candidate.Header
		frame.Payload = append(frame.Payload[:0], candidate.Payload...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return frame, reader
}
