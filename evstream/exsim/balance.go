// Package exsim defines this simulator's event schemas for the evstream
// format.
//
// It is deliberately a separate package from evstream. The format knows nothing
// about balances, orders or books; the schemas know nothing about framing,
// hashing or compression. A new event family is added here — or in any other
// package — without touching the format, and without an entry in a registry
// that everyone has to edit.
package exsim

import (
	"strconv"

	"exchange_sim/evstream"
)

// Schema ids for this simulator. They are dense and stable: an id is never
// reused for a different meaning, because old streams keep referring to it.
const (
	SchemaBalanceChange uint16 = evstream.FirstUserSchema + iota
	SchemaBookDelta
)

// Interner assigns dictionary ids to repeated strings. The writer implements
// it; taking it as an interface keeps the schemas testable without a stream.
type Interner interface {
	Intern(string) (uint32, error)
}

// Resolver turns dictionary ids back into strings when decoding.
type Resolver interface {
	Lookup(uint32) (string, bool)
}

// BalanceDelta is one asset movement inside a balance change.
//
// Every quantity is a signed int64 in the asset's fixed-point precision. There
// are no floats anywhere in this format: a float has platform-dependent
// formatting and would make canonical bytes impossible to guarantee.
type BalanceDelta struct {
	Asset      string
	Wallet     string
	OldBalance int64
	NewBalance int64
	Delta      int64
}

// BalanceChange is the largest event family in the stream, at roughly 20 % of
// all hashed bytes.
//
// PositionSide is genuinely optional — it carries omitempty in the JSON form —
// so it is encoded behind a presence bit rather than as an empty string. A
// decoder can then tell "absent" from "present and empty", which JSON's
// omitempty cannot.
type BalanceChange struct {
	Timestamp    int64
	ClientID     uint64
	Symbol       string
	PositionSide string
	HasSide      bool
	Reason       string
	Changes      []BalanceDelta
}

// balanceChangeOptionalFields is the width of the presence bitmap. Adding an
// optional field means a new schema version, not a silent widening: the bitmap
// size is part of the layout.
//
// Two optional fields, not one. The changes slice needs a presence bit of its
// own because a JSON null and an empty array are different values that a plain
// count cannot distinguish — a length of zero would render both as null and
// silently lose the difference. The round-trip test caught exactly that on the
// first run.
const balanceChangeOptionalFields = 2

const (
	balanceChangePositionSideBit = 0
	balanceChangeChangesBit      = 1
)

// EncodedBalanceChange is a BalanceChange with its strings already interned,
// ready to append. Interning is separated from appending because interning can
// emit dictionary frames and therefore fail, while appending cannot.
type EncodedBalanceChange struct {
	value BalanceChange

	symbolRef  uint32
	sideRef    uint32
	reasonRef  uint32
	assetRefs  []uint32
	walletRefs []uint32
}

// InternBalanceChange resolves every string in the event to a dictionary id.
func InternBalanceChange(in Interner, value BalanceChange, into *EncodedBalanceChange) error {
	into.value = value
	var err error
	if into.symbolRef, err = in.Intern(value.Symbol); err != nil {
		return err
	}
	if value.HasSide {
		if into.sideRef, err = in.Intern(value.PositionSide); err != nil {
			return err
		}
	}
	if into.reasonRef, err = in.Intern(value.Reason); err != nil {
		return err
	}
	into.assetRefs = into.assetRefs[:0]
	into.walletRefs = into.walletRefs[:0]
	for _, change := range value.Changes {
		asset, err := in.Intern(change.Asset)
		if err != nil {
			return err
		}
		wallet, err := in.Intern(change.Wallet)
		if err != nil {
			return err
		}
		into.assetRefs = append(into.assetRefs, asset)
		into.walletRefs = append(into.walletRefs, wallet)
	}
	return nil
}

func (e *EncodedBalanceChange) SchemaID() uint16      { return SchemaBalanceChange }
func (e *EncodedBalanceChange) SchemaVersion() uint16 { return 1 }

// AppendPayload writes the canonical payload.
//
// Layout, version 1:
//
//	presence  [1]byte   bit 0: position_side present
//	symbol    uint32    dictionary id
//	reason    uint32    dictionary id
//	side      uint32    dictionary id, only when the presence bit is set
//	count     uint32    number of deltas
//	deltas    count x { asset uint32, wallet uint32,
//	                    old int64, new int64, delta int64 }
//
// Timestamp and client id live in the frame header and are not repeated here.
func (e *EncodedBalanceChange) AppendPayload(dst []byte) []byte {
	start := len(dst)
	dst = append(dst, make([]byte, evstream.PresenceBits(balanceChangeOptionalFields))...)
	if e.value.HasSide {
		evstream.SetPresence(dst[start:], balanceChangePositionSideBit)
	}
	if e.value.Changes != nil {
		evstream.SetPresence(dst[start:], balanceChangeChangesBit)
	}
	dst = evstream.AppendUint32(dst, e.symbolRef)
	dst = evstream.AppendUint32(dst, e.reasonRef)
	if e.value.HasSide {
		dst = evstream.AppendUint32(dst, e.sideRef)
	}
	if e.value.Changes == nil {
		return dst
	}
	dst = evstream.AppendUint32(dst, uint32(len(e.value.Changes)))
	for i, change := range e.value.Changes {
		dst = evstream.AppendUint32(dst, e.assetRefs[i])
		dst = evstream.AppendUint32(dst, e.walletRefs[i])
		dst = evstream.AppendInt64(dst, change.OldBalance)
		dst = evstream.AppendInt64(dst, change.NewBalance)
		dst = evstream.AppendInt64(dst, change.Delta)
	}
	return dst
}

// DecodeBalanceChange reads a payload back into a typed value.
//
// into.Changes is reused when it has capacity, so a full-stream scan decodes
// without allocating per event.
func DecodeBalanceChange(frame evstream.Frame, resolve Resolver, into *BalanceChange) error {
	cursor := evstream.NewCursor(frame.Payload)
	presence := cursor.Presence(balanceChangeOptionalFields)
	symbolRef := cursor.Uint32()
	reasonRef := cursor.Uint32()
	into.HasSide = presence.Has(balanceChangePositionSideBit)
	sideRef := uint32(0)
	if into.HasSide {
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

	into.Timestamp = frame.Header.SimTS
	into.ClientID = frame.Header.ClientID
	var ok bool
	if into.Symbol, ok = resolve.Lookup(symbolRef); !ok {
		return evstream.ErrCorrupt
	}
	if into.Reason, ok = resolve.Lookup(reasonRef); !ok {
		return evstream.ErrCorrupt
	}
	if into.HasSide {
		if into.PositionSide, ok = resolve.Lookup(sideRef); !ok {
			return evstream.ErrCorrupt
		}
	} else {
		into.PositionSide = ""
	}

	switch {
	case !hasChanges:
		into.Changes = nil
	case into.Changes != nil && cap(into.Changes) >= int(count):
		// Reuse the caller's buffer. The nil check matters: slicing a nil slice
		// to [:0] yields nil, which would turn a present-but-empty list back
		// into an absent one and undo the presence bit above.
		into.Changes = into.Changes[:count]
	default:
		into.Changes = make([]BalanceDelta, count)
	}
	for i := range into.Changes {
		assetRef := cursor.Uint32()
		walletRef := cursor.Uint32()
		into.Changes[i].OldBalance = cursor.Int64()
		into.Changes[i].NewBalance = cursor.Int64()
		into.Changes[i].Delta = cursor.Int64()
		if err := cursor.Err(); err != nil {
			return err
		}
		if into.Changes[i].Asset, ok = resolve.Lookup(assetRef); !ok {
			return evstream.ErrCorrupt
		}
		if into.Changes[i].Wallet, ok = resolve.Lookup(walletRef); !ok {
			return evstream.ErrCorrupt
		}
	}
	if err := cursor.Err(); err != nil {
		return err
	}
	// Trailing bytes mean the payload does not match the version it claims.
	// Accepting them would let a schema change pass unnoticed.
	if cursor.Remaining() != 0 {
		return evstream.ErrCorrupt
	}
	return nil
}

// AppendJSON renders the event in the exact JSON the simulator emits today.
//
// This is the bridge that makes the format auditable rather than opaque: a
// binary stream can be rendered back to the canonical JSON and compared against
// evidence produced by the current writer, so "the binary form preserves every
// field" is a checked claim rather than a design intention.
func (b BalanceChange) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"timestamp":`...)
	dst = strconv.AppendInt(dst, b.Timestamp, 10)
	dst = append(dst, `,"client_id":`...)
	dst = strconv.AppendUint(dst, b.ClientID, 10)
	dst = append(dst, `,"symbol":`...)
	dst = appendJSONString(dst, b.Symbol)
	if b.HasSide {
		dst = append(dst, `,"position_side":`...)
		dst = appendJSONString(dst, b.PositionSide)
	}
	dst = append(dst, `,"reason":`...)
	dst = appendJSONString(dst, b.Reason)
	dst = append(dst, `,"changes":`...)
	if b.Changes == nil {
		dst = append(dst, `null`...)
	} else {
		dst = append(dst, '[')
		for i, change := range b.Changes {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, `{"asset":`...)
			dst = appendJSONString(dst, change.Asset)
			dst = append(dst, `,"wallet":`...)
			dst = appendJSONString(dst, change.Wallet)
			dst = append(dst, `,"old_balance":`...)
			dst = strconv.AppendInt(dst, change.OldBalance, 10)
			dst = append(dst, `,"new_balance":`...)
			dst = strconv.AppendInt(dst, change.NewBalance, 10)
			dst = append(dst, `,"delta":`...)
			dst = strconv.AppendInt(dst, change.Delta, 10)
			dst = append(dst, '}')
		}
		dst = append(dst, ']')
	}
	return append(dst, '}')
}

// appendJSONString writes a JSON string literal matching encoding/json,
// including its HTML escaping, falling back for anything the fast path cannot
// reproduce verbatim.
func appendJSONString(dst []byte, s string) []byte {
	simple := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			simple = false
			break
		}
	}
	if simple {
		dst = append(dst, '"')
		dst = append(dst, s...)
		return append(dst, '"')
	}
	encoded, err := jsonMarshalString(s)
	if err != nil {
		return append(dst, '"', '"')
	}
	return append(dst, encoded...)
}
