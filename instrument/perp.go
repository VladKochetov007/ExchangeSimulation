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
			// 1 bp per interval matches the venue-standard interest-rate
			// component (Binance/OKX/Bybit); the old 10 bp default overstated
			// baseline carry 10x and distorted basis-arb economics.
			BaseRate: 1,
			Damping:  100,
			MaxRate:  75,
		},
		MarginRate:            1000,
		MaintenanceMarginRate: 500,
		WarningMarginRate:     750,
	}
}

func (p *PerpFutures) IsPerp() bool           { return true }
func (p *PerpFutures) InstrumentType() string { return "PERP" }

func (p *PerpFutures) MarginRequired(qty, price, precision int64) int64 {
	if qty < 0 || price < 0 || p.MarginRate < 0 {
		return -1
	}
	notional, ok := etypes.TryMulDiv(qty, price, precision)
	if !ok {
		return -1
	}
	margin, ok := etypes.TryMulDiv(notional, p.MarginRate, 10000)
	if !ok {
		return -1
	}
	return margin
}

func (p *PerpFutures) MarginForMarket(qty, refPrice, precision int64) int64 {
	if refPrice == 0 {
		return 0
	}
	return p.MarginRequired(qty, refPrice, precision)
}

func (p *PerpFutures) MarginOnCancel(remainingQty, orderPrice, precision int64) int64 {
	return p.MarginRequired(remainingQty, orderPrice, precision)
}

var _ etypes.Margined = (*PerpFutures)(nil)

func (p *PerpFutures) Settle(ctx etypes.SettlementContext) etypes.SettlementResult {
	exec := ctx.Exec
	quote := p.QuoteAsset()

	takerDelta := ctx.Positions.UpdatePosition(exec.TakerClientID, ctx.BookSymbol, exec.Qty, exec.Price, ctx.TakerOrder.Side, ctx.TakerOrder.PositionSide)
	makerDelta := ctx.Positions.UpdatePosition(exec.MakerClientID, ctx.BookSymbol, exec.Qty, exec.Price, exec.MakerSide, ctx.MakerPosSide)

	if ctx.Log != nil {
		ctx.Log.LogEvent(ctx.Timestamp, exec.TakerClientID, "position_update", etypes.PositionUpdateEvent{
			Timestamp: ctx.Timestamp, ClientID: exec.TakerClientID, Symbol: ctx.BookSymbol,
			OldSize: takerDelta.OldSize, OldEntryPrice: takerDelta.OldEntryPrice,
			NewSize: takerDelta.NewSize, NewEntryPrice: takerDelta.NewEntryPrice,
			TradeQty: exec.Qty, TradePrice: exec.Price, TradeSide: ctx.TakerOrder.Side.String(), Reason: "trade",
		})
		ctx.Log.LogEvent(ctx.Timestamp, exec.MakerClientID, "position_update", etypes.PositionUpdateEvent{
			Timestamp: ctx.Timestamp, ClientID: exec.MakerClientID, Symbol: ctx.BookSymbol,
			OldSize: makerDelta.OldSize, OldEntryPrice: makerDelta.OldEntryPrice,
			NewSize: makerDelta.NewSize, NewEntryPrice: makerDelta.NewEntryPrice,
			TradeQty: exec.Qty, TradePrice: exec.Price, TradeSide: exec.MakerSide.String(), Reason: "trade",
		})
		ctx.Log.LogEvent(ctx.Timestamp, 0, "open_interest", etypes.OpenInterestEvent{
			Timestamp: ctx.Timestamp, Symbol: ctx.BookSymbol,
			OpenInterest: ctx.Positions.CalculateOpenInterest(ctx.BookSymbol),
		})
	}

	takerClosedQty := calcClosedQty(takerDelta.OldSize, exec.Qty, ctx.TakerOrder.Side)
	makerClosedQty := calcClosedQty(makerDelta.OldSize, exec.Qty, exec.MakerSide)
	precision := ctx.BasePrecision

	// Order margin: convert the filled portion's reservation into position
	// margin. Released as a delta against the order's ledger so the amount is
	// exact regardless of price improvement or partial-fill rounding.
	p.releaseOrderMargin(ctx, ctx.TakerOrder, exec.TakerClientID, quote, precision)
	p.releaseOrderMargin(ctx, ctx.MakerOrder, exec.MakerClientID, quote, precision)

	// Position margin: release the closing share first (a flip both closes and
	// opens), then reserve for the opened quantity at the fill price.
	p.settlePositionMargin(ctx, exec.TakerClientID, ctx.TakerOrder.PositionSide, takerDelta, takerClosedQty, quote, precision)
	p.settlePositionMargin(ctx, exec.MakerClientID, ctx.MakerPosSide, makerDelta, makerClosedQty, quote, precision)

	takerPnL := p.settleSide(ctx, exec.TakerClientID, ctx.TakerOrder.Side, takerDelta, takerClosedQty, ctx.TakerFee, quote)
	makerPnL := p.settleSide(ctx, exec.MakerClientID, exec.MakerSide, makerDelta, makerClosedQty, ctx.MakerFee, quote)
	recordSettlementFees(ctx, quote)

	return etypes.SettlementResult{TakerDelta: takerDelta, MakerDelta: makerDelta, TakerPnL: takerPnL, MakerPnL: makerPnL}
}

// releaseOrderMargin unlocks the difference between the order's reserved
// margin and what its unfilled remainder still requires at the order price.
// Market orders reserve no order margin and release nothing.
func (p *PerpFutures) releaseOrderMargin(ctx etypes.SettlementContext, order *etypes.Order, clientID uint64, quote string, precision int64) {
	if order == nil || order.Type == etypes.Market {
		return
	}
	stillNeeded := p.MarginRequired(order.Qty-order.FilledQty, order.Price, precision)
	if release := order.Reserved - stillNeeded; release > 0 {
		ctx.ReleasePerp(clientID, quote, release)
		order.Reserved = stillNeeded
	}
}

// settlePositionMargin maintains position margin across the fill: the closed
// share is released (exactly, via the store's MarginLedger when available)
// and the opened quantity is margined at the fill price.
func (p *PerpFutures) settlePositionMargin(ctx etypes.SettlementContext, clientID uint64, posSide etypes.PositionSide, delta etypes.PositionDelta, closedQty int64, quote string, precision int64) {
	ledger, hasLedger := ctx.Positions.(etypes.MarginLedger)
	if closedQty > 0 && delta.OldSize != 0 {
		var release int64
		if hasLedger {
			release = ledger.ReleasePositionMargin(clientID, ctx.BookSymbol, posSide, closedQty, delta.OldSize)
		} else {
			release = p.MarginRequired(closedQty, delta.OldEntryPrice, precision)
		}
		ctx.ReleasePerp(clientID, quote, release)
	}
	// Opened quantity comes from the actual position delta, not exec.Qty −
	// closedQty: hedge-mode reduces clamp at zero and discard overshoot, so the
	// naive difference would margin quantity that never opened.
	if openedQty := absInt(delta.NewSize) - absInt(delta.OldSize) + closedQty; openedQty > 0 {
		needed := p.MarginRequired(openedQty, ctx.Exec.Price, precision)
		ctx.ReservePerp(clientID, quote, needed)
		if hasLedger {
			ledger.AddPositionMargin(clientID, ctx.BookSymbol, posSide, needed)
		}
	}
}

func (p *PerpFutures) settleSide(ctx etypes.SettlementContext, clientID uint64, side etypes.Side, delta etypes.PositionDelta, closedQty int64, fee etypes.Fee, quote string) int64 {
	pnl := calcPerpPnL(delta.OldSize, delta.OldEntryPrice, ctx.Exec.Qty, ctx.Exec.Price, side, ctx.BasePrecision)
	if pnl != 0 && ctx.Log != nil {
		ctx.Log.LogEvent(ctx.Timestamp, clientID, "realized_pnl", etypes.RealizedPnLEvent{
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
	deltas := []etypes.BalanceDelta{
		{Asset: quote, Wallet: "perp", OldBalance: oldBal, NewBalance: oldBal + pnl, Delta: pnl},
	}
	if fee.Amount != 0 {
		oldFeeBal := ctx.PerpBalance(clientID, fee.Asset)
		ctx.MutatePerpBalance(clientID, fee.Asset, -fee.Amount)
		deltas = append(deltas, etypes.BalanceDelta{
			Asset: fee.Asset, Wallet: "perp",
			OldBalance: oldFeeBal, NewBalance: oldFeeBal - fee.Amount, Delta: -fee.Amount,
		})
	}
	ctx.LogBalanceChange(clientID, ctx.BookSymbol, "trade_settlement", deltas)
	return pnl
}

// recordSettlementFees routes each fee to its own asset bucket: clients are
// debited in Fee.Asset, so revenue booked under any other asset breaks
// per-asset conservation. Fees with an unset asset default to quote.
func recordSettlementFees(ctx etypes.SettlementContext, quote string) {
	takerAsset, makerAsset := ctx.TakerFee.Asset, ctx.MakerFee.Asset
	if takerAsset == "" {
		takerAsset = quote
	}
	if makerAsset == "" {
		makerAsset = quote
	}
	if takerAsset == makerAsset {
		ctx.RecordFeeRevenue(takerAsset, ctx.TakerFee.Amount, ctx.MakerFee.Amount)
		return
	}
	ctx.RecordFeeRevenue(takerAsset, ctx.TakerFee.Amount, 0)
	ctx.RecordFeeRevenue(makerAsset, 0, ctx.MakerFee.Amount)
}

func absInt(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
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
	return sign * etypes.MulDiv(closedQty, tradePrice-oldEntryPrice, basePrecision)
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
