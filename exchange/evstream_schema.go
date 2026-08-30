package exchange

import (
	"encoding/json"

	"exchange_sim/census"

	"exchange_sim/evstream"
	etypes "exchange_sim/types"
)

// Binary evidence schemas for the event families this package owns, plus the
// per-symbol wrapper and the opaque fallback that together give the stream full
// coverage from the first day.
//
// The fallback matters more than it looks. Without it a binary sink would have
// to carry typed schemas for every family before it could be used at all, and a
// partially converted stream would be half binary and half JSON with a hash
// spanning both. With it the stream is uniformly binary immediately: rare
// families ride as an opaque JSON payload inside a proper frame, keeping
// ordering, framing, indexing and the digest uniform, and each one can be
// promoted to a typed schema later without changing anything around it.
const (
	SchemaFillEvidence uint16 = etypes.SchemaFirstExchange + iota
	SchemaBookDelta
	SchemaBookSnapshot
	SchemaVenueBalance
	SchemaInstrumentLog
)

// --- fillEvidence ---

func (e fillEvidence) SchemaID() uint16      { return SchemaFillEvidence }
func (e fillEvidence) SchemaVersion() uint16 { return 1 }

func (e fillEvidence) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendInt64(dst, e.FeeAmount)
	dst = evstream.AppendInt64(dst, e.FilledQty)
	dst = evstream.AppendBool(dst, e.IsFull)
	dst = evstream.AppendInt64(dst, e.NewEntryPrice)
	dst = evstream.AppendInt64(dst, e.NewSize)
	dst = evstream.AppendUint64(dst, e.OrderID)
	dst = evstream.AppendInt64(dst, e.Price)
	dst = evstream.AppendInt64(dst, e.Qty)
	dst = evstream.AppendInt64(dst, e.RealizedPnL)
	dst = evstream.AppendInt64(dst, e.RemainingQty)
	dst = evstream.AppendUint64(dst, e.TradeID)
	// The five strings all come from small closed sets — assets, sides, roles,
	// position sides and symbols — so interning turns each into four bytes.
	for _, value := range [...]string{e.FeeAsset, e.PositionSide, e.Role, e.Side, e.Symbol} {
		ref, err := in.Intern(value)
		if err != nil {
			return nil, err
		}
		dst = evstream.AppendUint32(dst, ref)
	}
	return dst, nil
}

// DecodeFillEvidence reads the payload back.
func DecodeFillEvidence(payload []byte, resolve evstream.Resolver, into *fillEvidence) error {
	cursor := evstream.NewCursor(payload)
	into.FeeAmount = cursor.Int64()
	into.FilledQty = cursor.Int64()
	into.IsFull = cursor.Bool()
	into.NewEntryPrice = cursor.Int64()
	into.NewSize = cursor.Int64()
	into.OrderID = cursor.Uint64()
	into.Price = cursor.Int64()
	into.Qty = cursor.Int64()
	into.RealizedPnL = cursor.Int64()
	into.RemainingQty = cursor.Int64()
	into.TradeID = cursor.Uint64()
	refs := [5]uint32{}
	for i := range refs {
		refs[i] = cursor.Uint32()
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	targets := [...]*string{&into.FeeAsset, &into.PositionSide, &into.Role, &into.Side, &into.Symbol}
	for i, target := range targets {
		value, ok := resolve.Lookup(refs[i])
		if !ok {
			return evstream.ErrCorrupt
		}
		*target = value
	}
	return finishCursor(cursor)
}

// --- bookDeltaEvidence ---

func (d bookDeltaEvidence) SchemaID() uint16      { return SchemaBookDelta }
func (d bookDeltaEvidence) SchemaVersion() uint16 { return 1 }

func (d bookDeltaEvidence) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendInt64(dst, d.HiddenQty)
	dst = evstream.AppendInt64(dst, d.Price)
	dst = evstream.AppendInt64(dst, d.TotalQty)
	dst = evstream.AppendInt64(dst, d.VisibleQty)
	ref, err := in.Intern(d.Side)
	if err != nil {
		return nil, err
	}
	return evstream.AppendUint32(dst, ref), nil
}

// DecodeBookDelta reads the payload back.
func DecodeBookDelta(payload []byte, resolve evstream.Resolver, into *bookDeltaEvidence) error {
	cursor := evstream.NewCursor(payload)
	into.HiddenQty = cursor.Int64()
	into.Price = cursor.Int64()
	into.TotalQty = cursor.Int64()
	into.VisibleQty = cursor.Int64()
	ref := cursor.Uint32()
	if err := cursor.Err(); err != nil {
		return err
	}
	value, ok := resolve.Lookup(ref)
	if !ok {
		return evstream.ErrCorrupt
	}
	into.Side = value
	return finishCursor(cursor)
}

// --- bookSnapshotEvidence ---

const (
	snapshotOptionalFields = 2
	snapshotAsksBit        = 0
	snapshotBidsBit        = 1
)

func (b bookSnapshotEvidence) SchemaID() uint16      { return SchemaBookSnapshot }
func (b bookSnapshotEvidence) SchemaVersion() uint16 { return 1 }

// AppendPayloadInterning writes both sides. Each carries a presence bit, so a
// nil side and an empty one stay distinguishable exactly as JSON's null and []
// are.
func (b bookSnapshotEvidence) AppendPayloadInterning(dst []byte, _ evstream.Interner) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, evstream.PresenceBits(snapshotOptionalFields))...)
	if b.Asks != nil {
		evstream.SetPresence(dst[start:], snapshotAsksBit)
	}
	if b.Bids != nil {
		evstream.SetPresence(dst[start:], snapshotBidsBit)
	}
	dst = appendLevels(dst, b.Asks)
	return appendLevels(dst, b.Bids), nil
}

func appendLevels(dst []byte, levels []PriceLevel) []byte {
	if levels == nil {
		return dst
	}
	dst = evstream.AppendUint32(dst, uint32(len(levels)))
	for _, level := range levels {
		dst = evstream.AppendInt64(dst, level.Price)
		dst = evstream.AppendInt64(dst, level.VisibleQty)
		dst = evstream.AppendInt64(dst, level.HiddenQty)
	}
	return dst
}

// DecodeBookSnapshot reads the payload back.
func DecodeBookSnapshot(payload []byte, into *bookSnapshotEvidence) error {
	cursor := evstream.NewCursor(payload)
	presence := cursor.Presence(snapshotOptionalFields)
	into.Asks = readLevels(cursor, presence.Has(snapshotAsksBit), into.Asks)
	into.Bids = readLevels(cursor, presence.Has(snapshotBidsBit), into.Bids)
	return finishCursor(cursor)
}

func readLevels(cursor *evstream.Cursor, present bool, reuse []PriceLevel) []PriceLevel {
	if !present {
		return nil
	}
	count := int(cursor.Uint32())
	if cursor.Err() != nil {
		return nil
	}
	out := reuse
	if out == nil || cap(out) < count {
		out = make([]PriceLevel, count)
	} else {
		out = out[:count]
	}
	for i := range out {
		out[i].Price = cursor.Int64()
		out[i].VisibleQty = cursor.Int64()
		out[i].HiddenQty = cursor.Int64()
	}
	return out
}

// --- VenueBalanceEvent ---

const (
	venueBalanceOptionalFields = 1
	venueBalanceSymbolBit      = 0
)

func (e VenueBalanceEvent) SchemaID() uint16      { return SchemaVenueBalance }
func (e VenueBalanceEvent) SchemaVersion() uint16 { return 1 }

func (e VenueBalanceEvent) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, evstream.PresenceBits(venueBalanceOptionalFields))...)
	if e.Symbol != "" {
		evstream.SetPresence(dst[start:], venueBalanceSymbolBit)
	}
	dst = evstream.AppendInt64(dst, e.Timestamp)
	dst = evstream.AppendInt64(dst, e.OldBalance)
	dst = evstream.AppendInt64(dst, e.NewBalance)
	dst = evstream.AppendInt64(dst, e.Delta)
	values := []string{string(e.Bucket), e.Asset, e.Reason}
	if e.Symbol != "" {
		values = append(values, e.Symbol)
	}
	for _, value := range values {
		ref, err := in.Intern(value)
		if err != nil {
			return nil, err
		}
		dst = evstream.AppendUint32(dst, ref)
	}
	return dst, nil
}

// DecodeVenueBalance reads the payload back.
func DecodeVenueBalance(payload []byte, resolve evstream.Resolver, into *VenueBalanceEvent) error {
	cursor := evstream.NewCursor(payload)
	presence := cursor.Presence(venueBalanceOptionalFields)
	into.Timestamp = cursor.Int64()
	into.OldBalance = cursor.Int64()
	into.NewBalance = cursor.Int64()
	into.Delta = cursor.Int64()
	bucketRef, assetRef, reasonRef := cursor.Uint32(), cursor.Uint32(), cursor.Uint32()
	hasSymbol := presence.Has(venueBalanceSymbolBit)
	symbolRef := uint32(0)
	if hasSymbol {
		symbolRef = cursor.Uint32()
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	bucket, ok := resolve.Lookup(bucketRef)
	if !ok {
		return evstream.ErrCorrupt
	}
	into.Bucket = VenueBucket(bucket)
	if into.Asset, ok = resolve.Lookup(assetRef); !ok {
		return evstream.ErrCorrupt
	}
	if into.Reason, ok = resolve.Lookup(reasonRef); !ok {
		return evstream.ErrCorrupt
	}
	into.Symbol = ""
	if hasSymbol {
		if into.Symbol, ok = resolve.Lookup(symbolRef); !ok {
			return evstream.ErrCorrupt
		}
	}
	return finishCursor(cursor)
}

// --- instrumentLogEvent, the per-symbol wrapper ---

func (e instrumentLogEvent) SchemaID() uint16      { return SchemaInstrumentLog }
func (e instrumentLogEvent) SchemaVersion() uint16 { return 1 }

// AppendPayloadInterning writes the wrapper and delegates to the inner payload.
//
// A census of concrete payload types found this wrapper behind nine event
// names, because every per-symbol logger wraps what it is given. The inner
// schema id and version are recorded in the payload so a reader knows what
// follows without a side table, and an inner family that has no typed schema
// yet rides as opaque JSON rather than blocking the wrapper.
func (e instrumentLogEvent) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	symbolRef, err := in.Intern(e.Symbol)
	if err != nil {
		return nil, err
	}
	dst = evstream.AppendUint32(dst, symbolRef)

	if inner, ok := e.Payload.(evstream.InterningAppender); ok {
		dst = evstream.AppendUint16(dst, inner.SchemaID())
		dst = evstream.AppendUint16(dst, inner.SchemaVersion())
		return inner.AppendPayloadInterning(dst, in)
	}
	// Counted so the remaining gap between the measured speedup and the
	// ceiling can be attributed to specific inner families rather than
	// guessed at. The outer wrapper is always typed, so a census taken at the
	// sink cannot see these.
	census.CountFor("binary.wrapped-opaque", "inner payload has no typed schema", true, 0)
	dst = evstream.AppendUint16(dst, evstream.SchemaOpaqueJSON)
	dst = evstream.AppendUint16(dst, 1)
	encoded, err := json.Marshal(e.Payload)
	if err != nil {
		return nil, err
	}
	return evstream.AppendBytes(dst, encoded), nil
}

// OpaqueJSON wraps a payload with no typed schema yet, so it still travels as a
// proper frame inside the canonical stream.
type OpaqueJSON struct{ Value any }

func (o OpaqueJSON) SchemaID() uint16      { return evstream.SchemaOpaqueJSON }
func (o OpaqueJSON) SchemaVersion() uint16 { return 1 }

func (o OpaqueJSON) AppendPayloadInterning(dst []byte, _ evstream.Interner) ([]byte, error) {
	encoded, err := json.Marshal(o.Value)
	if err != nil {
		return nil, err
	}
	return evstream.AppendBytes(dst, encoded), nil
}

func finishCursor(cursor *evstream.Cursor) error {
	if err := cursor.Err(); err != nil {
		return err
	}
	if cursor.Remaining() != 0 {
		return evstream.ErrCorrupt
	}
	return nil
}

var (
	_ evstream.InterningAppender = fillEvidence{}
	_ evstream.InterningAppender = bookDeltaEvidence{}
	_ evstream.InterningAppender = bookSnapshotEvidence{}
	_ evstream.InterningAppender = VenueBalanceEvent{}
	_ evstream.InterningAppender = instrumentLogEvent{}
	_ evstream.InterningAppender = OpaqueJSON{}
)

// --- order lifecycle: accepted, cancelled, expired ---

const (
	SchemaAcceptedOrder uint16 = etypes.SchemaFirstExchange + 16 + iota
	SchemaCancelledOrder
	SchemaExpiredOrder
	SchemaForcedCancel
)

// appendOrder writes the wire fields of an order. The six enums are all
// uint8-backed, so one byte each is exact rather than a truncation waiting to
// happen; the fields tagged json:"-" are deliberately absent, because they are
// live engine state and were never part of the evidence.
func appendOrder(dst []byte, order *Order) []byte {
	dst = evstream.AppendUint64(dst, order.ID)
	dst = evstream.AppendUint64(dst, order.ClientID)
	dst = append(dst, byte(order.Side), byte(order.PositionSide), byte(order.Type),
		byte(order.TimeInForce), byte(order.Visibility), byte(order.Status))
	dst = evstream.AppendBool(dst, order.PostOnly)
	dst = evstream.AppendInt64(dst, order.Price)
	dst = evstream.AppendInt64(dst, order.Qty)
	dst = evstream.AppendInt64(dst, order.FilledQty)
	dst = evstream.AppendInt64(dst, order.IcebergQty)
	return evstream.AppendInt64(dst, order.Timestamp)
}

func readOrder(cursor *evstream.Cursor, into *Order) {
	into.ID = cursor.Uint64()
	into.ClientID = cursor.Uint64()
	into.Side = Side(cursor.Uint8())
	into.PositionSide = PositionSide(cursor.Uint8())
	into.Type = OrderType(cursor.Uint8())
	into.TimeInForce = TimeInForce(cursor.Uint8())
	into.Visibility = Visibility(cursor.Uint8())
	into.Status = OrderStatus(cursor.Uint8())
	into.PostOnly = cursor.Bool()
	into.Price = cursor.Int64()
	into.Qty = cursor.Int64()
	into.FilledQty = cursor.Int64()
	into.IcebergQty = cursor.Int64()
	into.Timestamp = cursor.Int64()
}

func (e acceptedOrderEvidence) SchemaID() uint16      { return SchemaAcceptedOrder }
func (e acceptedOrderEvidence) SchemaVersion() uint16 { return 1 }

func (e acceptedOrderEvidence) AppendPayloadInterning(dst []byte, _ evstream.Interner) ([]byte, error) {
	if e.Order == nil {
		return nil, evstream.ErrCorrupt
	}
	dst = appendOrder(dst, e.Order)
	return evstream.AppendUint64(dst, e.RequestID), nil
}

// DecodeAcceptedOrder reads the payload back into a caller-owned order.
func DecodeAcceptedOrder(payload []byte, order *Order, into *acceptedOrderEvidence) error {
	cursor := evstream.NewCursor(payload)
	readOrder(cursor, order)
	into.Order = order
	into.RequestID = cursor.Uint64()
	return finishCursor(cursor)
}

func (e cancelledOrderEvidence) SchemaID() uint16      { return SchemaCancelledOrder }
func (e cancelledOrderEvidence) SchemaVersion() uint16 { return 1 }

func (e cancelledOrderEvidence) AppendPayloadInterning(dst []byte, _ evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint64(dst, e.OrderID)
	dst = evstream.AppendInt64(dst, e.RemainingQty)
	return evstream.AppendUint64(dst, e.RequestID), nil
}

// DecodeCancelledOrder reads the payload back.
func DecodeCancelledOrder(payload []byte, into *cancelledOrderEvidence) error {
	cursor := evstream.NewCursor(payload)
	into.OrderID = cursor.Uint64()
	into.RemainingQty = cursor.Int64()
	into.RequestID = cursor.Uint64()
	return finishCursor(cursor)
}

func (e expiredOrderEvidence) SchemaID() uint16      { return SchemaExpiredOrder }
func (e expiredOrderEvidence) SchemaVersion() uint16 { return 1 }

func (e expiredOrderEvidence) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint64(dst, e.OrderID)
	dst = evstream.AppendInt64(dst, e.RemainingQty)
	dst = evstream.AppendUint64(dst, e.RequestID)
	ref, err := in.Intern(e.Reason)
	if err != nil {
		return nil, err
	}
	return evstream.AppendUint32(dst, ref), nil
}

// DecodeExpiredOrder reads the payload back.
func DecodeExpiredOrder(payload []byte, resolve evstream.Resolver, into *expiredOrderEvidence) error {
	cursor := evstream.NewCursor(payload)
	into.OrderID = cursor.Uint64()
	into.RemainingQty = cursor.Int64()
	into.RequestID = cursor.Uint64()
	ref := cursor.Uint32()
	if err := cursor.Err(); err != nil {
		return err
	}
	value, ok := resolve.Lookup(ref)
	if !ok {
		return evstream.ErrCorrupt
	}
	into.Reason = value
	return finishCursor(cursor)
}

func (e forcedCancelEvidence) SchemaID() uint16      { return SchemaForcedCancel }
func (e forcedCancelEvidence) SchemaVersion() uint16 { return 1 }

func (e forcedCancelEvidence) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint64(dst, e.OrderID)
	dst = evstream.AppendInt64(dst, e.RemainingQty)
	ref, err := in.Intern(e.Reason)
	if err != nil {
		return nil, err
	}
	return evstream.AppendUint32(dst, ref), nil
}

// DecodeForcedCancel reads the payload back.
func DecodeForcedCancel(payload []byte, resolve evstream.Resolver, into *forcedCancelEvidence) error {
	cursor := evstream.NewCursor(payload)
	into.OrderID = cursor.Uint64()
	into.RemainingQty = cursor.Int64()
	ref := cursor.Uint32()
	if err := cursor.Err(); err != nil {
		return err
	}
	value, ok := resolve.Lookup(ref)
	if !ok {
		return evstream.ErrCorrupt
	}
	into.Reason = value
	return finishCursor(cursor)
}

var (
	_ evstream.InterningAppender = acceptedOrderEvidence{}
	_ evstream.InterningAppender = cancelledOrderEvidence{}
	_ evstream.InterningAppender = expiredOrderEvidence{}
	_ evstream.InterningAppender = forcedCancelEvidence{}
)
