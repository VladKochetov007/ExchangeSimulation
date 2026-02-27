package fee

import etypes "exchange_sim/types"

type FixedFee struct {
	MakerFee etypes.Fee
	TakerFee etypes.Fee
}

func (f *FixedFee) CalculateFee(ctx etypes.FillContext) etypes.Fee {
	if ctx.IsMaker {
		return f.MakerFee
	}
	return f.TakerFee
}
