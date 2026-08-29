package analysis

import "encoding/json"

// splitDataLayer extracts a data layer's fields structurally, without decoding
// the value a second time.
//
// json.Unmarshal validates its whole input before decoding anything, and
// checkValid is about 40% of an Unmarshal call and 26% of analyzer CPU. The data
// layer and the nested payload are sub-values of a line that json.Unmarshal has
// already validated, so validating them again establishes nothing.
//
// This is not a general JSON decoder and does not try to be. It decides only the
// narrow shape the evidence envelope actually uses — an object whose venue_id
// and symbol are plain ASCII strings — and returns decided=false for everything
// else, in which case the caller performs the original decode and inherits its
// exact result and error text. Declining is cheap and always correct; the two
// cases it must never get wrong are duplicate keys, where the last occurrence
// wins, and any input the reference decode would have rejected.
//
// The returned payload aliases raw. That is safe where raw is itself the heap
// copy encoding/json produced for the enclosing field, which is how both call
// sites use it, and it removes a copy the reference path performs.
func splitDataLayer(raw json.RawMessage) (venueID, symbol string, payload json.RawMessage, decided bool) {
	index := skipJSONSpace(raw, 0)
	if index >= len(raw) || raw[index] != '{' {
		return "", "", nil, false
	}
	index++
	for {
		index = skipJSONSpace(raw, index)
		if index >= len(raw) {
			return "", "", nil, false
		}
		if raw[index] == '}' {
			break
		}
		if raw[index] != '"' {
			return "", "", nil, false
		}
		keyStart := index + 1
		keyEnd, escaped, ok := scanJSONString(raw, index)
		if !ok || escaped {
			return "", "", nil, false
		}
		key := raw[keyStart : keyEnd-1]
		index = skipJSONSpace(raw, keyEnd)
		if index >= len(raw) || raw[index] != ':' {
			return "", "", nil, false
		}
		index = skipJSONSpace(raw, index+1)
		valueStart := index
		valueEnd, ok := skipJSONValue(raw, index)
		if !ok {
			return "", "", nil, false
		}
		value := raw[valueStart:valueEnd]
		switch string(key) {
		case "venue_id":
			text, assign, ok := plainJSONString(value)
			if !ok {
				return "", "", nil, false
			}
			if assign {
				venueID = text
			}
		case "symbol":
			text, assign, ok := plainJSONString(value)
			if !ok {
				return "", "", nil, false
			}
			if assign {
				symbol = text
			}
		case "payload":
			// A raw field takes the value bytes verbatim, including any
			// interior whitespace, which is what RawMessage decoding does.
			payload = json.RawMessage(value)
		}
		index = skipJSONSpace(raw, valueEnd)
		if index >= len(raw) {
			return "", "", nil, false
		}
		switch raw[index] {
		case ',':
			index++
		case '}':
		default:
			return "", "", nil, false
		}
	}
	return venueID, symbol, payload, true
}

// plainJSONString reports the text of a JSON string value that needs no
// unquoting work.
//
// assign is false for a null value. That distinction is load-bearing and is not
// the same as an empty string: unmarshalling null into a Go string is a no-op,
// so with duplicate keys a trailing null leaves the earlier value in place. A
// randomized differential against the reference decode caught exactly that case.
//
// It declines on any escape and on any byte outside ASCII, because
// encoding/json's unquoting resolves escapes and substitutes U+FFFD for invalid
// UTF-8; reproducing either here would duplicate decoder rules rather than avoid
// redundant work.
func plainJSONString(value []byte) (text string, assign, ok bool) {
	if string(value) == "null" {
		return "", false, true
	}
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", false, false
	}
	inner := value[1 : len(value)-1]
	for _, c := range inner {
		if c == '\\' || c >= 0x80 {
			return "", false, false
		}
	}
	return string(inner), true, true
}

// decodeEventLayers builds the event's venue, symbol and innermost payload from
// an already validated record. It is shared by the per-metric and fused scans so
// they cannot diverge.
func decodeEventLayers(data json.RawMessage) (venueID, symbol string, payload json.RawMessage, err error) {
	venueID, symbol, payload, decided := splitDataLayer(data)
	if !decided {
		var outer dataLayer
		if err := json.Unmarshal(data, &outer); err != nil {
			return "", "", nil, err
		}
		venueID, symbol, payload = outer.VenueID, outer.Symbol, outer.Payload
	}
	// Unwrap the derivative nesting: an inner payload means the fields sit one
	// level down and the symbol travels with them.
	if mayNestPayload(payload) {
		innerSymbol, innerPayload, ok := splitNestedPayload(payload)
		if !ok {
			var inner dataLayer
			if json.Unmarshal(payload, &inner) == nil {
				innerSymbol, innerPayload = inner.Symbol, inner.Payload
			}
		}
		if len(innerPayload) > 0 {
			if innerSymbol != "" {
				symbol = innerSymbol
			}
			payload = innerPayload
		}
	}
	return venueID, symbol, payload, nil
}

// splitNestedPayload answers the unwrap question structurally. It reports
// ok=false when it declines, so the caller repeats the reference decode.
func splitNestedPayload(payload json.RawMessage) (symbol string, inner json.RawMessage, ok bool) {
	_, symbol, inner, decided := splitDataLayer(payload)
	if !decided {
		return "", nil, false
	}
	return symbol, inner, true
}
