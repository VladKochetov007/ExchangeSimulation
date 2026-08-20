package derivsim

// HedgeState is what a hedging policy decides from: the dealer's option risk,
// what it currently holds in the underlying, and the market it would trade in.
type HedgeState struct {
	// NetDelta is the option book's delta in underlying base units, before the
	// existing hedge.
	NetDelta float64
	// HedgePosition is the underlying inventory already held, and
	// HedgePending is signed quantity submitted and not yet resolved.
	HedgePosition int64
	HedgePending  int64
	// TradedDelta is the delta the dealer took on since it last hedged. A
	// policy that hedges each trade and never revisits it reads this and
	// ignores NetDelta.
	TradedDelta float64
	// SpotMid is the underlying's mid, and Nano the decision time.
	SpotMid  int64
	Nano     int64
	BandQty  int64
	LastNano int64
}

// HedgePolicy returns the signed quantity of the underlying to trade now.
// Zero means hold. It is an interface because how a dealer hedges is a
// strategy, not a property of quoting: the same book run under a static hedge
// and a rebalanced one is two different participants, and the difference
// between them is a research question rather than a setting to tune.
type HedgePolicy interface {
	Hedge(state HedgeState) int64
}

// NoHedge leaves the option book unhedged. It is the control arm: whatever
// delta the flow leaves the dealer with, it keeps.
type NoHedge struct{}

// Hedge implements HedgePolicy.
func (NoHedge) Hedge(HedgeState) int64 { return 0 }

// StaticDeltaHedge trades the delta of each option trade once, at the moment
// it happens, and never revisits it.
//
// This is the textbook static hedge and it is deliberately wrong as the
// underlying moves: the position is delta neutral at inception and drifts with
// gamma from then on. A dealer running it and one running BandedDeltaHedge on
// the same flow differ by exactly the gamma the second one pays to shed.
type StaticDeltaHedge struct{}

// Hedge implements HedgePolicy.
func (StaticDeltaHedge) Hedge(state HedgeState) int64 {
	target := -int64(state.TradedDelta)
	if target == 0 {
		return 0
	}
	return target
}

// BandedDeltaHedge holds the whole option book delta neutral, trading whenever
// the gap exceeds a band. The band is what stops the hedge from churning
// against every tick and paying the spread for nothing.
type BandedDeltaHedge struct{}

// Hedge implements HedgePolicy.
func (BandedDeltaHedge) Hedge(state HedgeState) int64 {
	target := -int64(state.NetDelta)
	gap := target - (state.HedgePosition + state.HedgePending)
	if gap > state.BandQty || gap < -state.BandQty {
		return gap
	}
	return 0
}

// TimedDeltaHedge rebalances to neutral on a fixed schedule regardless of how
// far the delta has drifted, which is the discipline a desk with a hedging
// window has rather than one watching a band continuously.
type TimedDeltaHedge struct {
	// IntervalNanos is the spacing between rebalances. Zero rebalances at
	// every opportunity, which is the band policy with a band of zero.
	IntervalNanos int64
}

// Hedge implements HedgePolicy.
func (h TimedDeltaHedge) Hedge(state HedgeState) int64 {
	if h.IntervalNanos > 0 && state.LastNano > 0 && state.Nano-state.LastNano < h.IntervalNanos {
		return 0
	}
	target := -int64(state.NetDelta)
	return target - (state.HedgePosition + state.HedgePending)
}
