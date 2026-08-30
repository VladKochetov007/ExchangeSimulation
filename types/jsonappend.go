package types

import (
	"encoding/json"
	"strconv"
)

// JSONAppender is an optional fast path for payloads on the hot evidence path.
// A type implementing it appends its own JSON, byte-for-byte identical to what
// encoding/json.Marshal would produce for the same value.
//
// Byte identity is the whole point, not an incidental property. Every execution
// event is marshalled and SHA-256'd into the ordered execution-stream digest,
// and that digest is the campaign's reproducibility attestation: an encoder
// that produced equivalent-but-different JSON would invalidate every published
// hash while changing nothing about the simulation. Implementations are
// therefore held to encoding/json by a differential test, not by inspection.
//
// Callers must treat this as optional. A payload that does not implement it
// still marshals through encoding/json, so adding a type costs nothing
// elsewhere and no registry has to be edited.
type JSONAppender interface {
	// AppendJSON appends this value's canonical JSON encoding to dst and
	// returns the extended slice.
	AppendJSON(dst []byte) []byte
}

// AppendJSONString appends a JSON string literal identical to what
// encoding/json produces, including its HTML escaping.
//
// The fast path handles the printable ASCII that every symbol, asset, wallet
// and reason in this simulator actually uses. Anything else — a quote, a
// backslash, an HTML-significant byte, a control character, or any non-ASCII —
// falls back to encoding/json for that one string. The fallback is what makes
// byte identity a guarantee rather than an assumption about the data.
func AppendJSONString(dst []byte, s string) []byte {
	if simpleJSONString(s) {
		dst = append(dst, '"')
		dst = append(dst, s...)
		return append(dst, '"')
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		// json.Marshal does not fail on a string; if that ever changes, an
		// empty literal is still valid JSON and the differential test will
		// catch the divergence rather than letting it reach a digest.
		return append(dst, '"', '"')
	}
	return append(dst, encoded...)
}

// simpleJSONString reports whether s can be emitted verbatim between quotes.
// encoding/json escapes <, > and & by default for HTML safety, so those are
// excluded along with the quote, the backslash, controls and all non-ASCII.
func simpleJSONString(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7e || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			return false
		}
	}
	return true
}

// AppendJSONInt appends a signed integer the way encoding/json does.
func AppendJSONInt(dst []byte, v int64) []byte {
	return strconv.AppendInt(dst, v, 10)
}

// AppendJSONUint appends an unsigned integer the way encoding/json does.
func AppendJSONUint(dst []byte, v uint64) []byte {
	return strconv.AppendUint(dst, v, 10)
}

// AppendJSON writes a BalanceChangeEvent. Field order follows the struct
// declaration because encoding/json emits struct fields in declaration order,
// and position_side carries omitempty.
func (e BalanceChangeEvent) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"timestamp":`...)
	dst = AppendJSONInt(dst, e.Timestamp)
	dst = append(dst, `,"client_id":`...)
	dst = AppendJSONUint(dst, e.ClientID)
	dst = append(dst, `,"symbol":`...)
	dst = AppendJSONString(dst, e.Symbol)
	if e.PositionSide != "" {
		dst = append(dst, `,"position_side":`...)
		dst = AppendJSONString(dst, e.PositionSide)
	}
	dst = append(dst, `,"reason":`...)
	dst = AppendJSONString(dst, e.Reason)
	dst = append(dst, `,"changes":`...)
	dst = appendBalanceDeltas(dst, e.Changes)
	return append(dst, '}')
}

// appendBalanceDeltas encodes the slice. A nil slice is JSON null and an empty
// non-nil slice is [], which encoding/json distinguishes and a digest notices.
func appendBalanceDeltas(dst []byte, deltas []BalanceDelta) []byte {
	if deltas == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range deltas {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = deltas[i].AppendJSON(dst)
	}
	return append(dst, ']')
}

// AppendJSON writes a BalanceDelta.
func (d BalanceDelta) AppendJSON(dst []byte) []byte {
	dst = append(dst, `{"asset":`...)
	dst = AppendJSONString(dst, d.Asset)
	dst = append(dst, `,"wallet":`...)
	dst = AppendJSONString(dst, d.Wallet)
	dst = append(dst, `,"old_balance":`...)
	dst = AppendJSONInt(dst, d.OldBalance)
	dst = append(dst, `,"new_balance":`...)
	dst = AppendJSONInt(dst, d.NewBalance)
	dst = append(dst, `,"delta":`...)
	dst = AppendJSONInt(dst, d.Delta)
	return append(dst, '}')
}

var (
	_ JSONAppender = BalanceChangeEvent{}
	_ JSONAppender = BalanceDelta{}
)
