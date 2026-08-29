package analysis

import "encoding/json"

// scanRequiredFields answers, in one pass over an already validated JSON
// object, which of names is absent or explicitly null.
//
// It exists because the reference implementation decodes the payload a second
// time into map[string]json.RawMessage purely to test field presence, which
// allocates a map and one copy of every value in the record. This scanner
// allocates nothing and copies nothing.
//
// It replicates the reference semantics exactly for the object case: a
// duplicate key keeps the last occurrence, a value of null counts as absent,
// and the first name in required order decides the reported failure. Anything
// it cannot decide with certainty — a non-object, or an escaped key, whose
// unescaped spelling this scanner deliberately does not interpret — returns
// decided=false so the caller falls back to the reference decode and its exact
// error text.
//
// raw must already have been accepted by encoding/json, which validates the
// whole document before decoding; this scanner therefore assumes syntactic
// validity and only re-derives structure.
func scanRequiredFields(raw json.RawMessage, names []string) (missing string, decided bool) {
	index := skipJSONSpace(raw, 0)
	if index >= len(raw) || raw[index] != '{' {
		return "", false
	}
	index++

	// found[i] tracks the last value seen for names[i]: whether the key was
	// present at all, and whether that final value was null.
	var (
		presentSmall [8]bool
		nullSmall    [8]bool
	)
	present := presentSmall[:0]
	isNull := nullSmall[:0]
	for range names {
		present = append(present, false)
		isNull = append(isNull, false)
	}

	for {
		index = skipJSONSpace(raw, index)
		if index >= len(raw) {
			return "", false
		}
		if raw[index] == '}' {
			break
		}
		if raw[index] != '"' {
			return "", false
		}
		keyStart := index + 1
		keyEnd, escaped, ok := scanJSONString(raw, index)
		if !ok {
			return "", false
		}
		if escaped {
			// An escaped key may unescape to a required name. Deciding that
			// here would duplicate the decoder's unquoting rules, so defer.
			return "", false
		}
		key := raw[keyStart : keyEnd-1]
		index = skipJSONSpace(raw, keyEnd)
		if index >= len(raw) || raw[index] != ':' {
			return "", false
		}
		index = skipJSONSpace(raw, index+1)
		valueStart := index
		valueEnd, ok := skipJSONValue(raw, index)
		if !ok {
			return "", false
		}
		for i, name := range names {
			if len(key) == len(name) && string(key) == name {
				present[i] = true
				isNull[i] = isJSONNull(raw[valueStart:valueEnd])
			}
		}
		index = skipJSONSpace(raw, valueEnd)
		if index >= len(raw) {
			return "", false
		}
		switch raw[index] {
		case ',':
			index++
		case '}':
		default:
			return "", false
		}
	}

	for i, name := range names {
		if !present[i] || isNull[i] {
			return name, true
		}
	}
	return "", true
}

// isJSONNull matches the reference test, which trims surrounding space from the
// raw value and compares it with the null literal.
func isJSONNull(value []byte) bool {
	start := skipJSONSpace(value, 0)
	end := len(value)
	for end > start && isJSONSpace(value[end-1]) {
		end--
	}
	return string(value[start:end]) == "null"
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func skipJSONSpace(data []byte, index int) int {
	for index < len(data) && isJSONSpace(data[index]) {
		index++
	}
	return index
}

// scanJSONString consumes the string starting at the opening quote in
// data[index] and returns the index just past its closing quote, plus whether
// the string contained any backslash escape.
func scanJSONString(data []byte, index int) (end int, escaped bool, ok bool) {
	index++
	for index < len(data) {
		switch data[index] {
		case '\\':
			escaped = true
			index += 2
			continue
		case '"':
			return index + 1, escaped, true
		}
		index++
	}
	return 0, escaped, false
}

// skipJSONValue consumes one JSON value starting at index and returns the index
// just past it. It relies on the document already being valid.
func skipJSONValue(data []byte, index int) (end int, ok bool) {
	if index >= len(data) {
		return 0, false
	}
	switch data[index] {
	case '"':
		end, _, ok = scanJSONString(data, index)
		return end, ok
	case '{', '[':
		return skipJSONContainer(data, index)
	default:
		start := index
		for index < len(data) && !isJSONSpace(data[index]) && data[index] != ',' && data[index] != '}' && data[index] != ']' {
			index++
		}
		if index == start {
			return 0, false
		}
		return index, true
	}
}

func skipJSONContainer(data []byte, index int) (end int, ok bool) {
	depth := 0
	for index < len(data) {
		switch data[index] {
		case '"':
			next, _, stringOK := scanJSONString(data, index)
			if !stringOK {
				return 0, false
			}
			index = next
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return index + 1, true
			}
		}
		index++
	}
	return 0, false
}
