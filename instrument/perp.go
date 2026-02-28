package instrument

import etypes "exchange_sim/types"

type PerpFutures struct {
	SpotInstrument
	fundingRate *etypes.FundingRate
	fundingCalc FundingCalculator
	// MarginRate is initial margin in bps (e.g. 1000 = 10% = 10x leverage)
	MarginRate int64
	// MaintenanceMarginRate is the minimum margin ratio in bps before liquidation
	MaintenanceMarginRate int64
	// WarningMarginRate triggers a margin call warning before liquidation
	WarningMarginRate int64
}

func NewPerpFutures(symbol, base, quote string, basePrecision, quotePrecision, tickSize, minOrderSize int64) *PerpFutures {
	return &PerpFutures{
		SpotInstrument: SpotInstrument{
			symbol:         symbol,
			base:           base,
			quote:          quote,
			basePrecision:  basePrecision,
			quotePrecision: quotePrecision,
			tickSize:       tickSize,
			minOrderSize:   minOrderSize,
		},
		fundingRate: &etypes.FundingRate{
			Symbol:      symbol,
			Rate:        0,
			NextFunding: 0,
			Interval:    28800,
			MarkPrice:   0,
			IndexPrice:  0,
		},
		fundingCalc: &SimpleFundingCalc{
			BaseRate: 10,
			Damping:  100,
			MaxRate:  75,
		},
		MarginRate:            1000,
		MaintenanceMarginRate: 500,
		WarningMarginRate:     750,
	}
}

func (p *PerpFutures) IsPerp() bool          { return true }
func (p *PerpFutures) InstrumentType() string { return "PERP" }

func (p *PerpFutures) MarginRequired(qty, price, precision int64) int64 {
	return (qty * price / precision) * p.MarginRate / 10000
}

func (p *PerpFutures) MarginForMarket(qty, refPrice, precision int64) int64 {
	if refPrice == 0 {
		return 0
	}
	return (qty * refPrice / precision) * p.MarginRate / 10000
}

func (p *PerpFutures) MarginOnCancel(remainingQty, orderPrice, precision int64) int64 {
	return (remainingQty * orderPrice / precision) * p.MarginRate / 10000
}

var _ etypes.Margined = (*PerpFutures)(nil)

func (p *PerpFutures) Settle(ctx etypes.SettlementContext) etypes.SettlementResult {
	exec := ctx.Exec
	quote := p.QuoteAsset()

	takerDelta := ctx.Positions.UpdatePosition(exec.TakerClientID, ctx.BookSymbol, exec.Qty, exec.Price, ctx.TakerOrder.Side, ctx.TakerOrder.PositionSide)
	makerDelta := ctx.Positions.UpdatePosition(exec.MakerClientID, ctx.BookSymbol, exec.Qty, exec.Price, exec.MakerSide, ctx.MakerPosSide)

	if ctx.GlobalLog != nil {
		ctx.GlobalLog.LogEvent(ctx.Timestamp, exec.TakerClientID, "position_update", etypes.PositionUpdateEvent{
			Timestamp: ctx.Timestamp, ClientID: exec.TakerClientID, Symbol: ctx.BookSymbol,
			OldSize: takerDelta.OldSize, OldEntryPrice: takerDelta.OldEntryPrice,
			NewSize: takerDelta.NewSize, NewEntryPrice: takerDelta.NewEntryPrice,
			TradeQty: exec.Qty, TradePrice: exec.Price, TradeSide: ctx.TakerOrder.Side.String(), Reason: "trade",
		})
		ctx.GlobalLog.LogEvent(ctx.Timestamp, exec.MakerClientID, "position_update", etypes.PositionUpdateEvent{
			Timestamp: ctx.Timestamp, ClientID: exec.MakerClientID, Symbol: ctx.BookSymbol,
			OldSize: makerDelta.OldSize, OldEntryPrice: makerDelta.OldEntryPrice,
			NewSize: makerDelta.NewSize, NewEntryPrice: makerDelta.NewEntryPrice,
			TradeQty: exec.Qty, TradePrice: exec.Price, TradeSide: exec.MakerSide.String(), Reason: "trade",
		})
		ctx.GlobalLog.LogEvent(ctx.Timestamp, 0, "open_interest", etypes.OpenInterestEvent{
			Timestamp: ctx.Timestamp, Symbol: ctx.BookSymbol,
			OpenInterest: ctx.Positions.CalculateOpenInterest(ctx.BookSymbol),
		})
	}

	takerClosedQty := calcClosedQty(takerDelta.OldSize, exec.Qty, ctx.TakerOrder.Side)
	makerClosedQty := calcClosedQty(makerDelta.OldSize, exec.Qty, exec.MakerSide)
	precision := ctx.BasePrecision

	// Taker margin: market orders reserve opened qty; limit orders release closed qty.
	if ctx.TakerOrder.Type == etypes.Market {
		if openedQty := exec.Qty - takerClosedQty; openedQty > 0 {
			ctx.ReservePerp(exec.TakerClientID, quote, p.MarginRequired(openedQty, exec.Price, precision))
		}
	} else if takerClosedQty > 0 {
		ctx.ReleasePerp(exec.TakerClientID, quote, p.MarginRequired(takerClosedQty, ctx.TakerOrder.Price, precision))
	}
	if takerClosedQty > 0 && takerDelta.OldSize != 0 {
		ctx.ReleasePerp(exec.TakerClientID, quote, p.MarginRequired(takerClosedQty, takerDelta.OldEntryPrice, precision))
	}

	// Maker margin: always limit; use exec.Price since maker order may be gone after full fill.
	if makerClosedQty > 0 {
		ctx.ReleasePerp(exec.MakerClientID, quote, p.MarginRequired(makerClosedQty, exec.Price, precision))
	}
	if makerClosedQty > 0 && makerDelta.OldSize != 0 {
		ctx.ReleasePerp(exec.MakerClientID, quote, p.MarginRequired(makerClosedQty, makerDelta.OldEntryPrice, precision))
	}

	takerPnL := p.settleSide(ctx, exec.TakerClientID, ctx.TakerOrder.Side, takerDelta, takerClosedQty, ctx.TakerFee, quote)
	makerPnL := p.settleSide(ctx, exec.MakerClientID, exec.MakerSide, makerDelta, makerClosedQty, ctx.MakerFee, quote)
	ctx.RecordFeeRevenue(quote, ctx.TakerFee.Amount, ctx.MakerFee.Amount)

	return etypes.SettlementResult{TakerDelta: takerDelta, MakerDelta: makerDelta, TakerPnL: takerPnL, MakerPnL: makerPnL}
}

func (p *PerpFutures) settleSide(ctx etypes.SettlementContext, clientID uint64, side etypes.Side, delta etypes.PositionDelta, closedQty int64, fee etypes.Fee, quote string) int64 {
	pnl := calcPerpPnL(delta.OldSize, delta.OldEntryPrice, ctx.Exec.Qty, ctx.Exec.Price, side, ctx.BasePrecision)
	if pnl != 0 && ctx.GlobalLog != nil {
		ctx.GlobalLog.LogEvent(ctx.Timestamp, clientID, "realized_pnl", etypes.RealizedPnLEvent{
			Timestamp:  ctx.Timestamp,
			ClientID:   clientID,
			Symbol:     ctx.BookSymbol,
			TradeID:    ctx.BookSeqNum,
			ClosedQty:  closedQty,
			EntryPrice: delta.OldEntryPrice,
			ExitPrice:  ctx.Exec.Price,
			PnL:        pnl,
			Side:       side.String(),
		})
	}
	oldBal := ctx.PerpBalance(clientID, quote)
	ctx.MutatePerpBalance(clientID, quote, pnl)
	ctx.MutatePerpBalance(clientID, fee.Asset, -fee.Amount)
	ctx.LogBalanceChange(clientID, ctx.BookSymbol, "trade_settlement", []etypes.BalanceDelta{
		{Asset: quote, Wallet: "perp", OldBalance: oldBal, NewBalance: oldBal + pnl, Delta: pnl},
	})
	return pnl
}

// calcClosedQty returns the portion of tradeQty that reduces an existing position.
func calcClosedQty(oldSize, tradeQty int64, side etypes.Side) int64 {
	if oldSize == 0 {
		return 0
	}
	delta := tradeQty
	if side == etypes.Sell {
		delta = -tradeQty
	}
	if (oldSize > 0 && delta >= 0) || (oldSize < 0 && delta <= 0) {
		return 0
	}
	absOld, absDelta := oldSize, delta
	if absOld < 0 {
		absOld = -absOld
	}
	if absDelta < 0 {
		absDelta = -absDelta
	}
	if absDelta > absOld {
		return absOld
	}
	return absDelta
}

// calcPerpPnL calculates realized PnL for a perp fill.
// Only non-zero when the fill reduces or closes an existing position.
func calcPerpPnL(oldSize, oldEntryPrice, tradeQty, tradePrice int64, tradeSide etypes.Side, basePrecision int64) int64 {
	if oldSize == 0 {
		return 0
	}
	delta := tradeQty
	if tradeSide == etypes.Sell {
		delta = -tradeQty
	}
	if (oldSize > 0 && delta >= 0) || (oldSize < 0 && delta <= 0) {
		return 0
	}
	absOld, absDelta := oldSize, delta
	if absOld < 0 {
		absOld = -absOld
	}
	if absDelta < 0 {
		absDelta = -absDelta
	}
	closedQty := absDelta
	if absDelta > absOld {
		closedQty = absOld
	}
	sign := int64(1)
	if oldSize < 0 {
		sign = -1
	}
	return (closedQty * sign * (tradePrice - oldEntryPrice)) / basePrecision
}

var _ etypes.Settleable = (*PerpFutures)(nil)

func (p *PerpFutures) GetFundingRate() *etypes.FundingRate { return p.fundingRate }

func (p *PerpFutures) SetFundingCalculator(calc FundingCalculator) {
	p.fundingCalc = calc
}

func (p *PerpFutures) UpdateFundingRate(indexPrice int64, markPrice int64) {
	p.fundingRate.IndexPrice = indexPrice
	p.fundingRate.MarkPrice = markPrice
	p.fundingRate.Rate = p.fundingCalc.Calculate(indexPrice, markPrice)
}
