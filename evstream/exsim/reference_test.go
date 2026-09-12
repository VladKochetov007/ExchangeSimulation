package exsim

import (
	"encoding/binary"
	"errors"
	"testing"

	"exchange_sim/evstream"
)

type permissiveExsimResolver struct {
	delegate evstream.Resolver
}

func (r permissiveExsimResolver) Lookup(ref uint32) (string, bool) {
	if ref == 0 {
		return "", true
	}
	return r.delegate.Lookup(ref)
}

func TestRequiredExsimSchemasRejectEmptyDictionaryValues(t *testing.T) {
	interner := newTestInterner()
	var encoded EncodedBalanceChange
	if err := InternBalanceChange(interner, BalanceChange{}, &encoded); !errors.Is(err, evstream.ErrEmptyDictionaryValue) {
		t.Fatalf("empty balance-change error = %v, want ErrEmptyDictionaryValue", err)
	}
	if err := InternBookDelta(interner, BookDelta{}, new(EncodedBookDelta)); !errors.Is(err, evstream.ErrEmptyDictionaryValue) {
		t.Fatalf("empty book-delta error = %v, want ErrEmptyDictionaryValue", err)
	}
}

func TestRequiredExsimSchemasRejectZeroReference(t *testing.T) {
	t.Run("balance change symbol", func(t *testing.T) {
		interner := newTestInterner()
		var encoded EncodedBalanceChange
		if err := InternBalanceChange(interner, BalanceChange{Symbol: "ABC/USD", Reason: "fill"}, &encoded); err != nil {
			t.Fatal(err)
		}
		payload := append([]byte(nil), encoded.AppendPayload(nil)...)
		cursor := evstream.NewCursor(payload)
		cursor.Presence(balanceChangeOptionalFields)
		cursor.Uint32()
		if err := cursor.Err(); err != nil {
			t.Fatal(err)
		}
		binary.LittleEndian.PutUint32(payload[cursor.Offset():], 0)
		frame := evstream.Frame{Payload: payload}
		var decoded BalanceChange
		if err := DecodeBalanceChange(frame, permissiveExsimResolver{delegate: interner}, &decoded); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("zero balance-change reference error = %v, want ErrCorrupt", err)
		}
	})

	t.Run("book delta symbol", func(t *testing.T) {
		interner := newTestInterner()
		var encoded EncodedBookDelta
		if err := InternBookDelta(interner, BookDelta{Symbol: "ABC/USD"}, &encoded); err != nil {
			t.Fatal(err)
		}
		payload := append([]byte(nil), encoded.AppendPayload(nil)...)
		binary.LittleEndian.PutUint32(payload, 0)
		frame := evstream.Frame{Payload: payload}
		var decoded BookDelta
		if err := DecodeBookDelta(frame, permissiveExsimResolver{delegate: interner}, &decoded); !errors.Is(err, evstream.ErrCorrupt) {
			t.Fatalf("zero book-delta reference error = %v, want ErrCorrupt", err)
		}
	})
}
