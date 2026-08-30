package analysis

import (
	"math"
	"strconv"
	"strings"
)

// expiryNanoFromLabel accepts both the legacy Unix-seconds spelling and the
// explicit nanosecond spelling used when a calendar contains sub-second
// boundaries. The parser is deliberately token-based: canonical symbols may
// carry identity components after the expiry without changing its meaning.
func expiryNanoFromLabel(label string) (int64, bool) {
	if strings.HasSuffix(label, "ns") {
		nanos, err := strconv.ParseInt(strings.TrimSuffix(label, "ns"), 10, 64)
		return nanos, err == nil && nanos > 0
	}
	seconds, err := strconv.ParseInt(label, 10, 64)
	if err != nil || seconds <= 0 || seconds > math.MaxInt64/1_000_000_000 {
		return 0, false
	}
	return seconds * 1_000_000_000, true
}

// optionStrikeFromLabel preserves the legacy currency-unit strike spelling
// while decoding canonical K/R labels as exact quote-precision integer units.
func optionStrikeFromLabel(label string, quotePrecision int64) (float64, bool) {
	if len(label) > 1 && (label[0] == 'K' || label[0] == 'R') {
		if quotePrecision <= 0 {
			return 0, false
		}
		raw, err := strconv.ParseInt(label[1:], 10, 64)
		if err != nil || raw <= 0 {
			return 0, false
		}
		return float64(raw) / float64(quotePrecision), true
	}
	strike, err := strconv.ParseFloat(label, 64)
	return strike, err == nil && strike > 0
}
