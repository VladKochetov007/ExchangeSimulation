package fee

import etypes "exchange_sim/types"

type PercentageFee struct {
	MakerBps int64
	TakerBps int64
	InQuote  bool
}

func (f *PercentageFee) CalculateFee(ctx etypes.FillContext) etypes.Fee {
	bps := f.TakerBps
	if ctx.IsMaker {
		bps = f.MakerBps
	}

	var amount int64
	var asset string

	if f.InQuote {
		tradeValue := (ctx.Exec.Price * ctx.Exec.Qty) / ctx.Precision
		amount = (tradeValue * bps) / BPS
		asset = ctx.QuoteAsset
	} else {
		amount = (ctx.Exec.Qty * bps) / BPS
		asset = ctx.BaseAsset
	}

	return etypes.Fee{Asset: asset, Amount: amount}
}
