package fee

import (
	etypes "exchange_sim/types"
)

// OptionFee mirrors the crypto-venue option fee schedule: the fee is
// proportional to the UNDERLYING notional (not the premium) but capped at a
// fraction of the premium so deep-OTM contracts stay tradeable — the cap is
// the binding constraint for cheap options.
//
// Underlying pricing is injected: Source resolves the underlying symbol's
// price; SymbolMap maps an option symbol to its underlying (both optional —
// without them the fee falls back to premium-notional bps).
type OptionFee struct {
	TakerUnderlyingBps int64
	MakerUnderlyingBps int64
	// CapPremiumBps caps the fee at this fraction of premium notional
	// (e.g. 1250 = 12.5%). 0 = no cap.
	CapPremiumBps int64

	Source    etypes.PriceSource
	SymbolMap func(baseAsset, quoteAsset string) string
}

func (f *OptionFee) CalculateFee(ctx etypes.FillContext) etypes.Fee {
	bps := f.TakerUnderlyingBps
	if ctx.IsMaker {
		bps = f.MakerUnderlyingBps
	}
	premiumNotional := etypes.MulDiv(ctx.Exec.Qty, ctx.Exec.Price, ctx.Precision)

	var amount int64
	if f.Source != nil && f.SymbolMap != nil {
		if underlying := f.Source.Price(f.SymbolMap(ctx.BaseAsset, ctx.QuoteAsset)); underlying > 0 {
			amount = etypes.MulDiv(ctx.Exec.Qty, underlying, ctx.Precision) * bps / 10000
		}
	}
	if amount == 0 {
		amount = premiumNotional * bps / 10000
	}
	if f.CapPremiumBps > 0 {
		if cap := premiumNotional * f.CapPremiumBps / 10000; amount > cap {
			amount = cap
		}
	}
	return etypes.Fee{Amount: amount, Asset: ctx.QuoteAsset}
}

var _ etypes.FeeModel = (*OptionFee)(nil)
