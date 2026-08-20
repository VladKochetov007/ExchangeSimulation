package derivsim

import "testing"

// A dealer that hedges each trade once is neutral at inception and carries the
// gamma from then on. It must not react to the book's drifted delta, which is
// exactly what separates it from a rebalancing desk.
func TestStaticDeltaHedgeCoversTheTradeAndIgnoresDrift(t *testing.T) {
	policy := StaticDeltaHedge{}
	if got := policy.Hedge(HedgeState{TradedDelta: 40, NetDelta: 40}); got != -40 {
		t.Errorf("hedge = %d, want -40", got)
	}
	drifted := HedgeState{TradedDelta: 0, NetDelta: 250, HedgePosition: -40}
	if got := policy.Hedge(drifted); got != 0 {
		t.Errorf("a static hedge chased the drift: %d, want 0", got)
	}
}

// The band exists so a hedge does not churn against every tick, and it has to
// apply on both sides of neutral.
func TestBandedDeltaHedgeTradesOnlyOutsideTheBand(t *testing.T) {
	policy := BandedDeltaHedge{}
	inside := HedgeState{NetDelta: 30, HedgePosition: -25, BandQty: 10}
	if got := policy.Hedge(inside); got != 0 {
		t.Errorf("hedged inside the band: %d, want 0", got)
	}
	outside := HedgeState{NetDelta: 100, HedgePosition: -25, BandQty: 10}
	if got := policy.Hedge(outside); got != -75 {
		t.Errorf("hedge = %d, want -75", got)
	}
	short := HedgeState{NetDelta: -100, HedgePosition: 25, BandQty: 10}
	if got := policy.Hedge(short); got != 75 {
		t.Errorf("hedge = %d, want 75", got)
	}
}

// In-flight quantity has to count against the target, or a delayed
// acknowledgement makes the dealer hedge the same delta repeatedly.
func TestBandedDeltaHedgeCountsInFlightQuantity(t *testing.T) {
	policy := BandedDeltaHedge{}
	state := HedgeState{NetDelta: 100, HedgePosition: 0, HedgePending: -100, BandQty: 10}
	if got := policy.Hedge(state); got != 0 {
		t.Errorf("hedged over an in-flight order: %d, want 0", got)
	}
}

// A desk with a hedging window rebalances on its schedule, however far the
// delta has drifted in between.
func TestTimedDeltaHedgeWaitsForItsSchedule(t *testing.T) {
	policy := TimedDeltaHedge{IntervalNanos: 1_000_000_000}
	early := HedgeState{NetDelta: 500, LastNano: 1_000_000_000, Nano: 1_500_000_000}
	if got := policy.Hedge(early); got != 0 {
		t.Errorf("rebalanced early: %d, want 0", got)
	}
	due := HedgeState{NetDelta: 500, LastNano: 1_000_000_000, Nano: 2_000_000_000}
	if got := policy.Hedge(due); got != -500 {
		t.Errorf("hedge = %d, want -500", got)
	}
}

func TestNoHedgeHoldsWhateverTheFlowLeaves(t *testing.T) {
	if got := (NoHedge{}).Hedge(HedgeState{NetDelta: 10_000, BandQty: 1}); got != 0 {
		t.Errorf("hedge = %d, want 0", got)
	}
}
