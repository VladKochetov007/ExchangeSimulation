package fee

import etypes "exchange_sim/types"

type FixedFee struct {
	MakerFee etypes.Fee
	TakerFee etypes.Fee
}

func (f *FixedFee) CalculateFee(exec *etypes.Execution, side etypes.Side, isMaker bool, baseAsset, quoteAsset string, precision int64) etypes.Fee {
	if isMaker {
		return f.MakerFee
	}
	return f.TakerFee
}
