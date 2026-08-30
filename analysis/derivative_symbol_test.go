package analysis

import "testing"

func TestExpiryFromSymbolAcceptsCanonicalAndLegacyFutureSymbols(t *testing.T) {
	tests := []struct {
		name   string
		symbol string
		want   int64
	}{
		{name: "legacy", symbol: "ABC-FUT-1735689600", want: 1735689600 * 1e9},
		{name: "canonical", symbol: "ABC-FUT-1735689600-U4142432f555344", want: 1735689600 * 1e9},
		{name: "subsecond", symbol: "ABC-FUT-1735689600000000001ns-U4142432f555344", want: 1735689600000000001},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := expiryFromSymbol(test.symbol)
			if !ok || got != test.want {
				t.Fatalf("expiryFromSymbol(%q) = (%d, %t), want (%d, true)", test.symbol, got, ok, test.want)
			}
		})
	}
}

func TestOptionTermsAcceptsCanonicalRawStrikeAndLegacyCurrencyStrike(t *testing.T) {
	tests := []struct {
		name       string
		symbol     string
		precision  int64
		wantExp    int64
		wantStrike float64
		wantCall   bool
	}{
		{name: "legacy", symbol: "ABC-1735689600-50000-C", precision: 100000, wantExp: 1735689600 * 1e9, wantStrike: 50000, wantCall: true},
		{name: "canonical", symbol: "ABC-OPT-U4142432f555344-1735689600-K5000000000-P", precision: 100000, wantExp: 1735689600 * 1e9, wantStrike: 50000, wantCall: false},
		{name: "subsecond", symbol: "ABC-OPT-U4142432f555344-1735689600000000001ns-K5000000000-C", precision: 100000, wantExp: 1735689600000000001, wantStrike: 50000, wantCall: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expiry, strike, isCall, ok := optionTerms(test.symbol, test.precision)
			if !ok || expiry != test.wantExp || strike != test.wantStrike || isCall != test.wantCall {
				t.Fatalf("optionTerms(%q) = (%d, %g, %t, %t), want (%d, %g, %t, true)", test.symbol, expiry, strike, isCall, ok, test.wantExp, test.wantStrike, test.wantCall)
			}
		})
	}
}
