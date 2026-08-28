package main

import "testing"

func TestEffectiveHedgeSymbolDefaultsToUnderlying(t *testing.T) {
	if got := effectiveHedgeSymbol("ABC-USD", "ABC/USD", false, false); got != "ABC/USD" {
		t.Fatalf("default hedging symbol = %q", got)
	}
}

func TestEffectiveHedgeSymbolAcceptsExplicitNewFlag(t *testing.T) {
	if got := effectiveHedgeSymbol("ABC-USD", "XYZ/USD", false, true); got != "XYZ/USD" {
		t.Fatalf("explicit hedging symbol = %q", got)
	}
}

func TestEffectiveHedgeSymbolPreservesLegacyBaseAlias(t *testing.T) {
	if got := effectiveHedgeSymbol("XYZ/USD", "ABC/USD", true, false); got != "XYZ/USD" {
		t.Fatalf("legacy hedging symbol = %q", got)
	}
}

func TestEffectiveHedgeSymbolNewFlagWinsOverLegacyAlias(t *testing.T) {
	if got := effectiveHedgeSymbol("LEGACY", "EXPLICIT", true, true); got != "EXPLICIT" {
		t.Fatalf("hedging symbol precedence = %q", got)
	}
}
