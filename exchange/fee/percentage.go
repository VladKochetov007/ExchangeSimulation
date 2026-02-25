package fee

import etypes "exchange_sim/exchange/types"

type PercentageFee struct {
	MakerBps int64
	TakerBps int64
	InQuote  bool
}

func (f *PercentageFee) CalculateFee(exec *etypes.Execution, side etypes.Side, isMaker bool, baseAsset, quoteAsset string, precision int64) etypes.Fee {
	bps := f.TakerBps
	if isMaker {
		bps = f.MakerBps
	}

	var amount int64
	var asset string

	if f.InQuote {
		tradeValue := (exec.Price * exec.Qty) / precision
		amount = (tradeValue * bps) / BPS
		asset = quoteAsset
	} else {
		amount = (exec.Qty * bps) / BPS
		asset = baseAsset
	}

	return etypes.Fee{Asset: asset, Amount: amount}
}
