package fee

import etypes "exchange_sim/types"

type FixedFee struct {
	MakerFee etypes.Fee
	TakerFee etypes.Fee
}

func (f *FixedFee) CalculateFee(ctx etypes.FillContext) (etypes.Fee, error) {
	if ctx.IsMaker {
		return f.MakerFee, nil
	}
	return f.TakerFee, nil
}
