package fee

import (
	"math"

	etypes "exchange_sim/types"
)

type PercentageFee struct {
	MakerBps int64
	TakerBps int64
	InQuote  bool
}

func (f *PercentageFee) CalculateFee(ctx etypes.FillContext) (etypes.Fee, error) {
	bps := f.TakerBps
	if ctx.IsMaker {
		bps = f.MakerBps
	}

	var amount int64
	var asset string

	if f.InQuote {
		tradeValue := etypes.MulDiv(ctx.Exec.Qty, ctx.Exec.Price, ctx.Precision)
		var ok bool
		amount, ok = etypes.TryMulBps(tradeValue, bps)
		if !ok {
			amount = math.MaxInt64
		}
		asset = ctx.QuoteAsset
	} else {
		var ok bool
		amount, ok = etypes.TryMulBps(ctx.Exec.Qty, bps)
		if !ok {
			amount = math.MaxInt64
		}
		asset = ctx.BaseAsset
	}

	return etypes.Fee{Asset: asset, Amount: amount}, nil
}
