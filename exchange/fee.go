package exchange

const (
	SATOSHI = 100000000
	BPS     = 10000
)

type FeeModel interface {
	CalculateFee(exec *Execution, side Side, isMaker bool, baseAsset, quoteAsset string) Fee
}

type PercentageFee struct {
	MakerBps int64
	TakerBps int64
	InQuote  bool
}

func (f *PercentageFee) CalculateFee(exec *Execution, side Side, isMaker bool, baseAsset, quoteAsset string) Fee {
	bps := f.TakerBps
	if isMaker {
		bps = f.MakerBps
	}

	var amount int64
	var asset string

	if f.InQuote {
		tradeValue := (exec.Price * exec.Qty) / SATOSHI
		amount = (tradeValue * bps) / BPS
		asset = quoteAsset
	} else {
		amount = (exec.Qty * bps) / BPS
		asset = baseAsset
	}

	return Fee{Asset: asset, Amount: amount}
}

type FixedFee struct {
	MakerFee Fee
	TakerFee Fee
}

func (f *FixedFee) CalculateFee(exec *Execution, side Side, isMaker bool, baseAsset, quoteAsset string) Fee {
	if isMaker {
		return f.MakerFee
	}
	return f.TakerFee
}
