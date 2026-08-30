package types

import "exchange_sim/evstream"

// Binary evidence schemas for the event families this package owns.
//
// Encoding lives on the value rather than in a registry, so a new family is
// added by giving its type these three methods and nothing central changes.
// Every payload here is fixed-width little-endian integers plus dictionary
// references: no floats, no maps, and optionality carried by an explicit
// presence bit rather than by a zero value that cannot be told from an absent
// one.
//
// Schema ids are permanent. An id is never reused for a different meaning,
// because streams written today keep referring to it.
const (
	SchemaBalanceChange uint16 = evstream.FirstUserSchema + iota
	SchemaFeeRevenue
	SchemaTrade
	SchemaPositionUpdate
	SchemaMarkPriceUpdate

	// SchemaFirstExchange is where the exchange package's ids begin, leaving
	// room for this package to add families without renumbering anything.
	SchemaFirstExchange uint16 = evstream.FirstUserSchema + 32
)

// --- BalanceChangeEvent ---

const (
	balanceChangeOptionalFields = 2
	balanceChangeSideBit        = 0
	balanceChangeChangesBit     = 1
)

func (e BalanceChangeEvent) SchemaID() uint16      { return SchemaBalanceChange }
func (e BalanceChangeEvent) SchemaVersion() uint16 { return 1 }

// AppendPayloadInterning writes the canonical payload.
//
// Changes carries a presence bit of its own because a JSON null and an empty
// array are different values that a length alone cannot distinguish — a zero
// count would render both as null and silently lose the difference.
func (e BalanceChangeEvent) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	start := len(dst)
	dst = append(dst, make([]byte, evstream.PresenceBits(balanceChangeOptionalFields))...)
	if e.PositionSide != "" {
		evstream.SetPresence(dst[start:], balanceChangeSideBit)
	}
	if e.Changes != nil {
		evstream.SetPresence(dst[start:], balanceChangeChangesBit)
	}

	dst = evstream.AppendInt64(dst, e.Timestamp)
	dst = evstream.AppendUint64(dst, e.ClientID)

	var err error
	if dst, err = appendRef(dst, in, e.Symbol); err != nil {
		return nil, err
	}
	if dst, err = appendRef(dst, in, e.Reason); err != nil {
		return nil, err
	}
	if e.PositionSide != "" {
		if dst, err = appendRef(dst, in, e.PositionSide); err != nil {
			return nil, err
		}
	}
	if e.Changes == nil {
		return dst, nil
	}
	dst = evstream.AppendUint32(dst, uint32(len(e.Changes)))
	for _, change := range e.Changes {
		if dst, err = appendRef(dst, in, change.Asset); err != nil {
			return nil, err
		}
		if dst, err = appendRef(dst, in, change.Wallet); err != nil {
			return nil, err
		}
		dst = evstream.AppendInt64(dst, change.OldBalance)
		dst = evstream.AppendInt64(dst, change.NewBalance)
		dst = evstream.AppendInt64(dst, change.Delta)
	}
	return dst, nil
}

// DecodeBalanceChange reads the payload back, reusing the caller's slice.
func DecodeBalanceChange(payload []byte, resolve evstream.Resolver, into *BalanceChangeEvent) error {
	cursor := evstream.NewCursor(payload)
	presence := cursor.Presence(balanceChangeOptionalFields)
	into.Timestamp = cursor.Int64()
	into.ClientID = cursor.Uint64()
	symbolRef, reasonRef := cursor.Uint32(), cursor.Uint32()
	hasSide := presence.Has(balanceChangeSideBit)
	sideRef := uint32(0)
	if hasSide {
		sideRef = cursor.Uint32()
	}
	hasChanges := presence.Has(balanceChangeChangesBit)
	count := uint32(0)
	if hasChanges {
		count = cursor.Uint32()
	}
	if err := cursor.Err(); err != nil {
		return err
	}

	var err error
	if into.Symbol, err = resolveRef(resolve, symbolRef); err != nil {
		return err
	}
	if into.Reason, err = resolveRef(resolve, reasonRef); err != nil {
		return err
	}
	into.PositionSide = ""
	if hasSide {
		if into.PositionSide, err = resolveRef(resolve, sideRef); err != nil {
			return err
		}
	}

	switch {
	case !hasChanges:
		into.Changes = nil
	case into.Changes != nil && cap(into.Changes) >= int(count):
		// The nil check matters: slicing a nil slice to [:0] yields nil, which
		// would turn a present-but-empty list back into an absent one.
		into.Changes = into.Changes[:count]
	default:
		into.Changes = make([]BalanceDelta, count)
	}
	for i := range into.Changes {
		assetRef, walletRef := cursor.Uint32(), cursor.Uint32()
		into.Changes[i].OldBalance = cursor.Int64()
		into.Changes[i].NewBalance = cursor.Int64()
		into.Changes[i].Delta = cursor.Int64()
		if err := cursor.Err(); err != nil {
			return err
		}
		if into.Changes[i].Asset, err = resolveRef(resolve, assetRef); err != nil {
			return err
		}
		if into.Changes[i].Wallet, err = resolveRef(resolve, walletRef); err != nil {
			return err
		}
	}
	return finish(cursor)
}

// --- FeeRevenueEvent ---

func (e FeeRevenueEvent) SchemaID() uint16      { return SchemaFeeRevenue }
func (e FeeRevenueEvent) SchemaVersion() uint16 { return 1 }

func (e FeeRevenueEvent) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendInt64(dst, e.Timestamp)
	dst = evstream.AppendUint64(dst, e.TradeID)
	dst = evstream.AppendInt64(dst, e.TakerFee)
	dst = evstream.AppendInt64(dst, e.MakerFee)
	var err error
	if dst, err = appendRef(dst, in, e.Symbol); err != nil {
		return nil, err
	}
	return appendRef(dst, in, e.Asset)
}

// DecodeFeeRevenue reads the payload back.
func DecodeFeeRevenue(payload []byte, resolve evstream.Resolver, into *FeeRevenueEvent) error {
	cursor := evstream.NewCursor(payload)
	into.Timestamp = cursor.Int64()
	into.TradeID = cursor.Uint64()
	into.TakerFee = cursor.Int64()
	into.MakerFee = cursor.Int64()
	symbolRef, assetRef := cursor.Uint32(), cursor.Uint32()
	if err := cursor.Err(); err != nil {
		return err
	}
	var err error
	if into.Symbol, err = resolveRef(resolve, symbolRef); err != nil {
		return err
	}
	if into.Asset, err = resolveRef(resolve, assetRef); err != nil {
		return err
	}
	return finish(cursor)
}

// --- Trade ---

func (t *Trade) SchemaID() uint16      { return SchemaTrade }
func (t *Trade) SchemaVersion() uint16 { return 1 }

// AppendPayloadInterning writes a trade. Side is an enum on the wire rather
// than the string its JSON form carries: the set is closed and a byte is exact.
func (t *Trade) AppendPayloadInterning(dst []byte, _ evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint64(dst, t.TradeID)
	dst = evstream.AppendInt64(dst, t.Price)
	dst = evstream.AppendInt64(dst, t.Qty)
	dst = append(dst, byte(t.Side))
	dst = evstream.AppendUint64(dst, t.TakerOrderID)
	return evstream.AppendUint64(dst, t.MakerOrderID), nil
}

// DecodeTrade reads the payload back.
func DecodeTrade(payload []byte, into *Trade) error {
	cursor := evstream.NewCursor(payload)
	into.TradeID = cursor.Uint64()
	into.Price = cursor.Int64()
	into.Qty = cursor.Int64()
	into.Side = Side(cursor.Uint8())
	into.TakerOrderID = cursor.Uint64()
	into.MakerOrderID = cursor.Uint64()
	return finish(cursor)
}

// --- shared helpers ---

// appendRef interns a string and appends its dictionary id.
func appendRef(dst []byte, in evstream.Interner, value string) ([]byte, error) {
	ref, err := in.Intern(value)
	if err != nil {
		return nil, err
	}
	return evstream.AppendUint32(dst, ref), nil
}

// resolveRef turns a dictionary id back into its string, treating an id the
// stream never defined as corruption rather than as an empty string.
func resolveRef(resolve evstream.Resolver, ref uint32) (string, error) {
	value, ok := resolve.Lookup(ref)
	if !ok {
		return "", evstream.ErrCorrupt
	}
	return value, nil
}

// finish checks a decode consumed exactly its payload. Trailing bytes mean the
// data does not match the schema version it claims, and accepting them would
// let a layout change pass unnoticed.
func finish(cursor *evstream.Cursor) error {
	if err := cursor.Err(); err != nil {
		return err
	}
	if cursor.Remaining() != 0 {
		return evstream.ErrCorrupt
	}
	return nil
}

var (
	_ evstream.InterningAppender = BalanceChangeEvent{}
	_ evstream.InterningAppender = FeeRevenueEvent{}
	_ evstream.InterningAppender = (*Trade)(nil)
)
