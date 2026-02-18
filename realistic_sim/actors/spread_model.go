package actors

import "exchange_sim/exchange"

// SpreadModel computes a half-spread (in price units) given current conditions.
// Inject into market maker actors to decouple quoting logic from spread logic.
type SpreadModel interface {
	HalfSpread(instrument exchange.Instrument, inventory int64) int64
}

// FixedHalfSpread returns a constant half-spread in bps of the mid price.
type FixedHalfSpread struct {
	Bps int64
}

func (f *FixedHalfSpread) HalfSpread(inst exchange.Instrument, _ int64) int64 {
	_ = inst
	return f.Bps
}

// OFISpreadModel widens the half-spread when order flow is imbalanced.
// OFI (order flow imbalance) = signed rolling volume: buys counted positive,
// sells negative. A large positive OFI means buy pressure is dominant — the
// book is being hit on the ask side, which is adverse selection for the MM.
//
// half_spread = base_bps + toxicity_factor * |OFI| / window_volume * max_extra_bps
//
// This is a simplified version of the Avellaneda-Stoikov adverse-selection
// term, calibrated empirically rather than derived from the full model.
type OFISpreadModel struct {
	BaseBps      int64 // minimum half-spread in bps
	MaxExtraBps  int64 // additional spread when OFI is maximally imbalanced
	WindowVolume int64 // rolling volume denominator (normalises OFI to [−1, 1])
	// DecayFactor is the per-trade OFI decay multiplier × 1000.
	// Default (0): 900 = retain 90% per trade; half-life ≈ 6.6 trades.
	// Calibrating by desired half-life H (in trades): DecayFactor = 1000 * exp(-ln2/H).
	//   H=3  → 794   fast, reacts within a few trades
	//   H=7  → 906   moderate (≈ default 900)
	//   H=20 → 966   slow, persistent memory
	DecayFactor  int64

	signedVolume int64
	totalVolume  int64
}

// OnTrade updates the OFI state. Call from the actor's trade event handler.
func (o *OFISpreadModel) OnTrade(side exchange.Side, qty int64) {
	if side == exchange.Buy {
		o.signedVolume += qty
	} else {
		o.signedVolume -= qty
	}
	o.totalVolume += qty

	decay := o.DecayFactor
	if decay == 0 {
		decay = 900
	}
	o.signedVolume = o.signedVolume * decay / 1000
	o.totalVolume = o.totalVolume * decay / 1000
	o.totalVolume = max(o.totalVolume, 1)
}

func (o *OFISpreadModel) HalfSpread(inst exchange.Instrument, _ int64) int64 {
	imbalance := o.signedVolume
	if imbalance < 0 {
		imbalance = -imbalance
	}
	// normalise imbalance to [0, WindowVolume], then scale extra spread
	denom := max(o.WindowVolume, o.totalVolume)
	extra := min(o.MaxExtraBps*imbalance/denom, o.MaxExtraBps)
	return o.BaseBps + extra
}
