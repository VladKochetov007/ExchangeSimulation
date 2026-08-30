package exsim

import "exchange_sim/evstream"

// BookDelta is the highest-frequency family in the stream at roughly 870,000
// events per simulated hour. It is all fixed-width integers plus one enum, so
// its payload is 25 bytes against about 110 for the JSON form.
type BookDelta struct {
	Timestamp  int64
	Symbol     string
	Side       uint8 // 0 buy, 1 sell — an enum, not a string, on the wire
	Price      int64
	VisibleQty int64
	HiddenQty  int64
	TotalQty   int64
}

// EncodedBookDelta carries the interned symbol reference.
type EncodedBookDelta struct {
	value     BookDelta
	symbolRef uint32
}

// InternBookDelta resolves the symbol to a dictionary id.
func InternBookDelta(in Interner, value BookDelta, into *EncodedBookDelta) error {
	into.value = value
	ref, err := in.Intern(value.Symbol)
	if err != nil {
		return err
	}
	into.symbolRef = ref
	return nil
}

func (e *EncodedBookDelta) SchemaID() uint16      { return SchemaBookDelta }
func (e *EncodedBookDelta) SchemaVersion() uint16 { return 1 }

// AppendPayload writes the canonical payload. No optional fields, so no
// presence bitmap: every field is always present, and saying so costs nothing.
//
//	symbol  uint32
//	side    uint8
//	price, visible, hidden, total   int64 x4
func (e *EncodedBookDelta) AppendPayload(dst []byte) []byte {
	dst = evstream.AppendUint32(dst, e.symbolRef)
	dst = append(dst, e.value.Side)
	dst = evstream.AppendInt64(dst, e.value.Price)
	dst = evstream.AppendInt64(dst, e.value.VisibleQty)
	dst = evstream.AppendInt64(dst, e.value.HiddenQty)
	return evstream.AppendInt64(dst, e.value.TotalQty)
}

// DecodeBookDelta reads a payload back into a typed value.
func DecodeBookDelta(frame evstream.Frame, resolve Resolver, into *BookDelta) error {
	cursor := evstream.NewCursor(frame.Payload)
	symbolRef := cursor.Uint32()
	side := cursor.Uint8()
	into.Price = cursor.Int64()
	into.VisibleQty = cursor.Int64()
	into.HiddenQty = cursor.Int64()
	into.TotalQty = cursor.Int64()
	if err := cursor.Err(); err != nil {
		return err
	}
	into.Side = side
	into.Timestamp = frame.Header.SimTS
	var ok bool
	if into.Symbol, ok = resolve.Lookup(symbolRef); !ok {
		return evstream.ErrCorrupt
	}
	if cursor.Remaining() != 0 {
		return evstream.ErrCorrupt
	}
	return nil
}
