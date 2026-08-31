package types

import "strconv"

// CanonicalJSONAppender is implemented by a market-data payload that can render
// the exact JSON `encoding/json` would produce for it, without reflection.
//
// It is optional by design. MarketDataFingerprint falls back to reflection for
// any payload that does not implement it, so a user's own payload type works
// unchanged and can opt into the fast path later without the library knowing
// the type exists.
type CanonicalJSONAppender interface {
	// AppendCanonicalJSON must append bytes identical to json.Marshal of the
	// same value. "Almost identical" is a silent fingerprint change.
	AppendCanonicalJSON(dst []byte) []byte
}

// appendFingerprintJSON renders the fingerprint envelope without reflection,
// or reports false when anything about the value would make a hand-written
// encoding risky: an unknown payload type, or a symbol needing JSON escaping.
//
// The fingerprint feeds delivery evidence and decision attestations, so the
// bytes are an identity rather than an implementation detail. Refusing the fast
// path is always safe; producing subtly different bytes is not.
func appendFingerprintJSON(dst []byte, msg *MarketDataMsg) ([]byte, bool) {
	payload, ok := msg.Data.(CanonicalJSONAppender)
	if !ok || !plainJSONString(msg.Symbol) {
		return dst, false
	}
	dst = append(dst, `{"type":`...)
	dst = strconv.AppendUint(dst, uint64(msg.Type), 10)
	dst = append(dst, `,"symbol":"`...)
	dst = append(dst, msg.Symbol...)
	dst = append(dst, `","sequence":`...)
	dst = strconv.AppendUint(dst, msg.SeqNum, 10)
	dst = append(dst, `,"timestamp":`...)
	dst = strconv.AppendInt(dst, msg.Timestamp, 10)
	dst = append(dst, `,"data":`...)
	dst = payload.AppendCanonicalJSON(dst)
	return append(dst, '}'), true
}

// plainJSONString reports whether s survives JSON encoding unchanged. Go's
// encoder escapes <, > and & as well as quotes, backslashes and control
// characters, so anything outside printable ASCII minus those takes the
// reflection path rather than risking a different escaping decision.
func plainJSONString(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7e || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			return false
		}
	}
	return true
}

func appendPriceLevels(dst []byte, levels []PriceLevel) []byte {
	if levels == nil {
		return append(dst, "null"...)
	}
	dst = append(dst, '[')
	for i, level := range levels {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = append(dst, `{"price":`...)
		dst = strconv.AppendInt(dst, level.Price, 10)
		dst = append(dst, `,"visible_qty":`...)
		dst = strconv.AppendInt(dst, level.VisibleQty, 10)
		dst = append(dst, `,"hidden_qty":`...)
		dst = strconv.AppendInt(dst, level.HiddenQty, 10)
		dst = append(dst, '}')
	}
	return append(dst, ']')
}

func (b BookSnapshot) AppendCanonicalJSON(dst []byte) []byte {
	dst = append(dst, `{"bids":`...)
	dst = appendPriceLevels(dst, b.Bids)
	dst = append(dst, `,"asks":`...)
	dst = appendPriceLevels(dst, b.Asks)
	return append(dst, '}')
}

func (d BookDelta) AppendCanonicalJSON(dst []byte) []byte {
	dst = append(dst, `{"side":"`...)
	dst = append(dst, d.Side.String()...)
	dst = append(dst, `","price":`...)
	dst = strconv.AppendInt(dst, d.Price, 10)
	dst = append(dst, `,"visible_qty":`...)
	dst = strconv.AppendInt(dst, d.VisibleQty, 10)
	dst = append(dst, `,"hidden_qty":`...)
	dst = strconv.AppendInt(dst, d.HiddenQty, 10)
	return append(dst, '}')
}

func (t Trade) AppendCanonicalJSON(dst []byte) []byte {
	dst = append(dst, `{"trade_id":`...)
	dst = strconv.AppendUint(dst, t.TradeID, 10)
	dst = append(dst, `,"price":`...)
	dst = strconv.AppendInt(dst, t.Price, 10)
	dst = append(dst, `,"qty":`...)
	dst = strconv.AppendInt(dst, t.Qty, 10)
	dst = append(dst, `,"side":"`...)
	dst = append(dst, t.Side.String()...)
	dst = append(dst, `","taker_order_id":`...)
	dst = strconv.AppendUint(dst, t.TakerOrderID, 10)
	dst = append(dst, `,"maker_order_id":`...)
	dst = strconv.AppendUint(dst, t.MakerOrderID, 10)
	return append(dst, '}')
}

var (
	_ CanonicalJSONAppender = BookSnapshot{}
	_ CanonicalJSONAppender = BookDelta{}
	_ CanonicalJSONAppender = Trade{}
)
