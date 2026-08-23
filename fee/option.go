package fee

import (
	"fmt"

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

func (f *OptionFee) CalculateFee(ctx etypes.FillContext) (etypes.Fee, error) {
	bps := f.TakerUnderlyingBps
	if ctx.IsMaker {
		bps = f.MakerUnderlyingBps
	}
	premiumNotional := etypes.MulDiv(ctx.Exec.Qty, ctx.Exec.Price, ctx.Precision)

	var amount int64
	if f.Source != nil && f.SymbolMap != nil {
		underlyingSymbol := f.SymbolMap(ctx.BaseAsset, ctx.QuoteAsset)
		underlying, err := f.Source.Price(underlyingSymbol)
		if err != nil {
			// A configured underlying-notional fee cannot silently switch to a
			// premium fee because its price source disappeared. The exchange
			// preflights this error before matching and returns it to the action
			// boundary instead of inventing a zero fee.
			return etypes.Fee{}, fmt.Errorf("option fee underlying %s: %w", underlyingSymbol, err)
		}
		amount = etypes.MulDiv(ctx.Exec.Qty, underlying, ctx.Precision) * bps / 10000
	} else {
		// This is the declared premium-notional schedule when no underlying
		// source policy was configured. Do not use amount==0 as a proxy for an
		// absent reference: zero is a legitimate rounded fee under the selected
		// underlying-notional schedule.
		amount = premiumNotional * bps / 10000
	}
	if f.CapPremiumBps > 0 {
		if cap := premiumNotional * f.CapPremiumBps / 10000; amount > cap {
			amount = cap
		}
	}
	return etypes.Fee{Amount: amount, Asset: ctx.QuoteAsset}, nil
}

var _ etypes.FeeModel = (*OptionFee)(nil)
