package exchange

import (
	"encoding/json"
	"fmt"

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
		value, err := evstream.ResolveRequired(resolve, refs[i])
		if err != nil {
			return err
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
	value, err := evstream.ResolveRequired(resolve, ref)
	if err != nil {
		return err
	}
	into.Side = value
	return finishCursor(cursor)
}

// --- bookSnapshotEvidence ---

const (
	snapshotV1OptionalFields = 2
	snapshotAsksBit          = 0
	snapshotBidsBit          = 1
	snapshotV3OptionalFields = 4
	snapshotPublicAsksBit    = 2
	snapshotPublicBidsBit    = 3
)

func (b bookSnapshotEvidence) SchemaID() uint16      { return SchemaBookSnapshot }
func (b bookSnapshotEvidence) SchemaVersion() uint16 { return 3 }

// AppendPayloadInterning writes both sides. Each carries a presence bit, so a
// nil side and an empty one stay distinguishable exactly as JSON's null and []
// are.
func (b bookSnapshotEvidence) AppendPayloadInterning(dst []byte, _ evstream.Interner) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, evstream.PresenceBits(snapshotV3OptionalFields))...)
	if b.Asks != nil {
		evstream.SetPresence(dst[start:], snapshotAsksBit)
	}
	if b.Bids != nil {
		evstream.SetPresence(dst[start:], snapshotBidsBit)
	}
	if b.PublicAsks != nil {
		evstream.SetPresence(dst[start:], snapshotPublicAsksBit)
	}
	if b.PublicBids != nil {
		evstream.SetPresence(dst[start:], snapshotPublicBidsBit)
	}
	dst = evstream.AppendUint64(dst, b.SourceSequence)
	dst = appendLevels(dst, b.Asks)
	dst = appendLevels(dst, b.Bids)
	dst = appendLevels(dst, b.PublicAsks)
	return appendLevels(dst, b.PublicBids), nil
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
	return DecodeBookSnapshotVersioned(payload, 3, into)
}

// DecodeBookSnapshotVersioned retains the historical v1 layout and the v2
// source-sequence extension while v3 additionally carries the exact public
// projection needed to join delayed observations without guessing the venue's
// public level boundary.
func DecodeBookSnapshotVersioned(payload []byte, schemaVersion uint16, into *bookSnapshotEvidence) error {
	return DecodeBookSnapshotVersionedWithMaxLevels(payload, schemaVersion, into, DefaultMaxBookSnapshotLevels)
}

// DefaultMaxBookSnapshotLevels bounds allocations made while decoding an
// untrusted evidence payload. The explicit override is available to callers
// whose instrument contract permits deeper books; the default is deliberately
// much larger than the exchange's public depth while remaining bounded.
const DefaultMaxBookSnapshotLevels = 1 << 20

// DecodeBookSnapshotVersionedWithMaxLevels decodes a snapshot with an explicit
// per-side level limit. A count must also fit in the remaining payload before
// any backing array is allocated, so malformed frames fail closed without
// turning their advertised count into a memory request.
func DecodeBookSnapshotVersionedWithMaxLevels(payload []byte, schemaVersion uint16, into *bookSnapshotEvidence, maxLevels int) error {
	if maxLevels <= 0 {
		return fmt.Errorf("%w: maximum snapshot level count must be positive", evstream.ErrCorrupt)
	}
	cursor := evstream.NewCursor(payload)
	optionalFields := snapshotV1OptionalFields
	if schemaVersion == 3 {
		optionalFields = snapshotV3OptionalFields
	}
	presence := cursor.Presence(optionalFields)
	into.SourceSequence = 0
	into.PublicAsks = nil
	into.PublicBids = nil
	if schemaVersion == 2 || schemaVersion == 3 {
		into.SourceSequence = cursor.Uint64()
	} else if schemaVersion != 1 {
		return unsupportedSchemaVersion(SchemaBookSnapshot, schemaVersion)
	}
	var err error
	into.Asks, err = readLevels(cursor, presence.Has(snapshotAsksBit), into.Asks, maxLevels)
	if err != nil {
		return err
	}
	into.Bids, err = readLevels(cursor, presence.Has(snapshotBidsBit), into.Bids, maxLevels)
	if err != nil {
		return err
	}
	if schemaVersion == 3 {
		into.PublicAsks, err = readLevels(cursor, presence.Has(snapshotPublicAsksBit), into.PublicAsks, maxLevels)
		if err != nil {
			return err
		}
		into.PublicBids, err = readLevels(cursor, presence.Has(snapshotPublicBidsBit), into.PublicBids, maxLevels)
		if err != nil {
			return err
		}
	}
	return finishCursor(cursor)
}

const encodedPriceLevelBytes = 3 * 8

func readLevels(cursor *evstream.Cursor, present bool, reuse []PriceLevel, maxLevels int) ([]PriceLevel, error) {
	if !present {
		return nil, nil
	}
	count := cursor.Uint32()
	if cursor.Err() != nil {
		return nil, cursor.Err()
	}
	if uint64(count) > uint64(maxLevels) {
		return nil, fmt.Errorf("%w: snapshot level count %d exceeds maximum %d", evstream.ErrCorrupt, count, maxLevels)
	}
	if uint64(count) > uint64(cursor.Remaining()/encodedPriceLevelBytes) {
		return nil, fmt.Errorf("%w: snapshot level count %d exceeds payload capacity", evstream.ErrCorrupt, count)
	}
	levelCount := int(count)
	out := reuse
	if out == nil || cap(out) < levelCount {
		out = make([]PriceLevel, levelCount)
	} else {
		out = out[:levelCount]
	}
	for i := range out {
		out[i].Price = cursor.Int64()
		out[i].VisibleQty = cursor.Int64()
		out[i].HiddenQty = cursor.Int64()
	}
	return out, nil
}

// --- VenueBalanceEvent ---

const (
	venueBalanceOptionalFields = 1
	venueBalanceSymbolBit      = 0
)

func (e VenueBalanceEvent) SchemaID() uint16      { return SchemaVenueBalance }
func (e VenueBalanceEvent) SchemaVersion() uint16 { return 2 }

func (e VenueBalanceEvent) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, evstream.PresenceBits(venueBalanceOptionalFields))...)
	if e.Symbol != "" {
		evstream.SetPresence(dst[start:], venueBalanceSymbolBit)
	}
	dst = evstream.AppendInt64(dst, e.Timestamp)
	dst = evstream.AppendUint64(dst, e.Sequence)
	dst = evstream.AppendUint64(dst, e.TradeID)
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
	return DecodeVenueBalanceVersioned(payload, resolve, 2, into)
}

// DecodeVenueBalanceVersioned preserves compatibility with the initial
// prototype while making sequence and trade identity loss impossible in the
// promoted schema. Version 1 had neither field on the wire; those values are
// therefore explicitly zero when an old stream is read.
func DecodeVenueBalanceVersioned(payload []byte, resolve evstream.Resolver, version uint16, into *VenueBalanceEvent) error {
	if version != 1 && version != 2 {
		return evstream.ErrCorrupt
	}
	cursor := evstream.NewCursor(payload)
	presence := cursor.Presence(venueBalanceOptionalFields)
	into.Timestamp = cursor.Int64()
	into.Sequence = 0
	into.TradeID = 0
	if version >= 2 {
		into.Sequence = cursor.Uint64()
		into.TradeID = cursor.Uint64()
	}
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
	bucket, err := evstream.ResolveRequired(resolve, bucketRef)
	if err != nil {
		return err
	}
	into.Bucket = VenueBucket(bucket)
	if into.Asset, err = evstream.ResolveRequired(resolve, assetRef); err != nil {
		return err
	}
	if into.Reason, err = evstream.ResolveRequired(resolve, reasonRef); err != nil {
		return err
	}
	into.Symbol = ""
	if hasSymbol {
		if into.Symbol, err = evstream.ResolveRequired(resolve, symbolRef); err != nil {
			return err
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

// RenderPayloadJSON reconstructs the canonical JSON payload represented by a
// binary schema. The versioned form is used by the file-layout renderer; the
// short form is a convenience for callers rendering the current schema set.
func RenderPayloadJSON(schemaID uint16, payload []byte, resolve evstream.Resolver) ([]byte, error) {
	return RenderPayloadJSONVersioned(schemaID, currentSchemaVersion(schemaID), payload, resolve)
}

// RenderPayloadJSONVersioned is deliberately fail-closed on unknown schema
// versions. Rendering an empty or guessed payload would turn an evidence loss
// into a plausible JSON record.
func RenderPayloadJSONVersioned(schemaID, schemaVersion uint16, payload []byte, resolve evstream.Resolver) ([]byte, error) {
	switch schemaID {
	case evstream.SchemaOpaqueJSON:
		if schemaVersion != 1 {
			return nil, unsupportedSchemaVersion(schemaID, schemaVersion)
		}
		cursor := evstream.NewCursor(payload)
		body := cursor.Bytes()
		if err := finishCursor(cursor); err != nil {
			return nil, err
		}
		return body, nil

	case SchemaInstrumentLog:
		if schemaVersion != 1 {
			return nil, unsupportedSchemaVersion(schemaID, schemaVersion)
		}
		cursor := evstream.NewCursor(payload)
		symbolRef := cursor.Uint32()
		innerID := cursor.Uint16()
		innerVersion := cursor.Uint16()
		if err := cursor.Err(); err != nil {
			return nil, err
		}
		symbol, err := evstream.ResolveRequired(resolve, symbolRef)
		if err != nil {
			return nil, err
		}
		inner, err := RenderPayloadJSONVersioned(innerID, innerVersion, payload[cursor.Offset():], resolve)
		if err != nil {
			return nil, err
		}
		out := append([]byte(`{"symbol":`), mustMarshalJSON(symbol)...)
		out = append(out, `,"payload":`...)
		out = append(out, inner...)
		return append(out, '}'), nil

	case SchemaFillEvidence:
		if schemaVersion != 1 {
			return nil, unsupportedSchemaVersion(schemaID, schemaVersion)
		}
		var value fillEvidence
		if err := DecodeFillEvidence(payload, resolve, &value); err != nil {
			return nil, err
		}
		return json.Marshal(value)
	case SchemaBookDelta:
		if schemaVersion != 1 {
			return nil, unsupportedSchemaVersion(schemaID, schemaVersion)
		}
		var value bookDeltaEvidence
		if err := DecodeBookDelta(payload, resolve, &value); err != nil {
			return nil, err
		}
		return json.Marshal(value)
	case SchemaBookSnapshot:
		if schemaVersion != 1 && schemaVersion != 2 && schemaVersion != 3 {
			return nil, unsupportedSchemaVersion(schemaID, schemaVersion)
		}
		var value bookSnapshotEvidence
		if err := DecodeBookSnapshotVersioned(payload, schemaVersion, &value); err != nil {
			return nil, err
		}
		if schemaVersion < 3 {
			return json.Marshal(struct {
				Asks           []PriceLevel `json:"asks"`
				Bids           []PriceLevel `json:"bids"`
				SourceSequence uint64       `json:"source_sequence,omitempty"`
			}{Asks: value.Asks, Bids: value.Bids, SourceSequence: value.SourceSequence})
		}
		// Do not invoke the historical compatibility marshaler here. A v3
		// binary payload must render every presence-bearing field, including a
		// zero source sequence and nil public sides, or the binary-to-JSON map
		// is no longer injective.
		return json.Marshal(struct {
			Asks           []PriceLevel `json:"asks"`
			Bids           []PriceLevel `json:"bids"`
			SourceSequence uint64       `json:"source_sequence"`
			PublicAsks     []PriceLevel `json:"public_asks"`
			PublicBids     []PriceLevel `json:"public_bids"`
		}{
			Asks: value.Asks, Bids: value.Bids, SourceSequence: value.SourceSequence,
			PublicAsks: value.PublicAsks, PublicBids: value.PublicBids,
		})
	case SchemaVenueBalance:
		var value VenueBalanceEvent
		if err := DecodeVenueBalanceVersioned(payload, resolve, schemaVersion, &value); err != nil {
			return nil, err
		}
		return json.Marshal(value)
	case etypes.SchemaBalanceChange:
		if schemaVersion != 1 && schemaVersion != 2 {
			return nil, unsupportedSchemaVersion(schemaID, schemaVersion)
		}
		var value etypes.BalanceChangeEvent
		if err := etypes.DecodeBalanceChangeVersioned(payload, resolve, schemaVersion, &value); err != nil {
			return nil, err
		}
		return json.Marshal(value)
	case etypes.SchemaFeeRevenue:
		if schemaVersion != 1 {
			return nil, unsupportedSchemaVersion(schemaID, schemaVersion)
		}
		var value etypes.FeeRevenueEvent
		if err := etypes.DecodeFeeRevenue(payload, resolve, &value); err != nil {
			return nil, err
		}
		return json.Marshal(value)
	case etypes.SchemaTrade:
		if schemaVersion != 1 {
			return nil, unsupportedSchemaVersion(schemaID, schemaVersion)
		}
		var value etypes.Trade
		if err := etypes.DecodeTrade(payload, &value); err != nil {
			return nil, err
		}
		return json.Marshal(value)
	default:
		return nil, fmt.Errorf("%w: no renderer for schema %d", evstream.ErrCorrupt, schemaID)
	}
}

func currentSchemaVersion(schemaID uint16) uint16 {
	if schemaID == SchemaBookSnapshot {
		return 3
	}
	if schemaID == SchemaVenueBalance {
		return 2
	}
	if schemaID == etypes.SchemaBalanceChange {
		return 2
	}
	return 1
}

func unsupportedSchemaVersion(schemaID, version uint16) error {
	return fmt.Errorf("%w: unsupported schema %d version %d", evstream.ErrCorrupt, schemaID, version)
}

func mustMarshalJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte(`""`)
	}
	return encoded
}
