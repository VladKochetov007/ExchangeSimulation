package instrument

import (
	"sync/atomic"

	etypes "exchange_sim/types"
)

// OptionMarginParams configures the short-side margin formula, Deribit-style:
// initial margin per unit = max(IMBaseBps − OTM%, IMFloorBps) × underlying
// mark + option mark premium. All rates in bps of the underlying price.
type OptionMarginParams struct {
	IMBaseBps  int64 // e.g. 1500 = 15%
	IMFloorBps int64 // e.g. 1000 = 10%
	MMBps      int64 // maintenance, e.g. 750 = 7.5% (reserved for MTM sweeps)
}

func DefaultOptionMarginParams() OptionMarginParams {
	return OptionMarginParams{IMBaseBps: 1500, IMFloorBps: 1000, MMBps: 750}
}

// EuropeanOption is a cash-settled European option priced in premium per base
// unit (quote precision). Buyers pay the full premium from the perp wallet;
// sellers post initial margin there. At expiry every position auto-exercises
// against the settlement TWAP of the underlying: intrinsic value is exchanged
// in the quote asset and short margin is released.
type EuropeanOption struct {
	SpotInstrument
	Strike     int64 // quote precision units
	IsCall     bool
	Underlying string // spot symbol used for index observation, e.g. "ABC/USD"
	// IV is the flat implied volatility used for the Black-76 mark premium.
	IV     float64
	Margin OptionMarginParams
	// DeliveryFeeBps is charged on the UNDERLYING settlement notional at
	// expiry, capped at DeliveryFeeCapBps of the exercise value (venue
	// pattern: Deribit charges 0.015% of underlying capped at 12.5% of the
	// option value, so worthless options settle free). Cap defaults to 1250.
	DeliveryFeeBps    int64
	DeliveryFeeCapBps int64

	expiryNano     int64
	observer       settlementObserver
	underlyingMark atomic.Int64
	markPremium    atomic.Int64
}

func NewEuropeanOption(symbol, base, quote, underlying string, basePrecision, quotePrecision, tickSize, minOrderSize, strike, expiryNano int64, isCall bool) *EuropeanOption {
	return &EuropeanOption{
		SpotInstrument: SpotInstrument{
			symbol:         symbol,
			base:           base,
			quote:          quote,
			basePrecision:  basePrecision,
			quotePrecision: quotePrecision,
			tickSize:       tickSize,
			minOrderSize:   minOrderSize,
		},
		Strike:     strike,
		IsCall:     isCall,
		Underlying: underlying,
		IV:         0.8,
		Margin:     DefaultOptionMarginParams(),
		expiryNano: expiryNano,
		observer:   settlementObserver{windowNano: 60 * 1e9},
	}
}

func (o *EuropeanOption) InstrumentType() string { return "OPTION" }

// Premiums can legitimately rest at a single tick, so unlike spot the price
// only needs tick alignment.
func (o *EuropeanOption) ValidatePrice(price int64) bool {
	return price > 0 && price%o.tickSize == 0
}

// SetObservationWindow overrides the settlement TWAP lookback (default 60s).
func (o *EuropeanOption) SetObservationWindow(windowNano int64) {
	o.observer.windowNano = windowNano
}

// SetMarks caches the underlying mark and the option mark premium (both
// updated by the exchange price loop) for the seller margin formula.
func (o *EuropeanOption) SetMarks(underlyingMark, markPremium int64) {
	o.underlyingMark.Store(underlyingMark)
	o.markPremium.Store(markPremium)
}

func (o *EuropeanOption) MarkPremium() int64 { return o.markPremium.Load() }

// --- PositionMarginer ---

// PositionMark marks open positions at the current premium mark for
// cross-margin mark-to-market.
func (o *EuropeanOption) PositionMark() int64 { return o.markPremium.Load() }

// MaintenanceForPosition returns the quote maintenance requirement for a
// signed option position. Longs owe nothing (premium was fully paid at
// entry); shorts owe MMBps of the underlying mark plus the current premium
// per unit, Deribit-style — the premium term guarantees maintenance always
// covers buying the position back at the mark.
func (o *EuropeanOption) MaintenanceForPosition(size, precision int64) int64 {
	if size >= 0 {
		return 0
	}
	short := -size
	mm := etypes.MulDiv(short, o.underlyingMark.Load(), precision) * o.Margin.MMBps / 10000
	return mm + etypes.MulDiv(short, o.markPremium.Load(), precision)
}

// --- Expirable ---

func (o *EuropeanOption) ExpiryNano() int64        { return o.expiryNano }
func (o *EuropeanOption) UnderlyingSymbol() string { return o.Underlying }

func (o *EuropeanOption) ObserveSettlement(price, tsNano int64) {
	o.observer.observe(price, tsNano)
}

func (o *EuropeanOption) SettlementPrice() int64 {
	return o.observer.settlementPrice(o.underlyingMark.Load())
}

// intrinsicValue is the exercise value per base unit at the given underlying
// settlement price, in quote precision units.
func (o *EuropeanOption) intrinsicValue(settlementPrice int64) int64 {
	var v int64
	if o.IsCall {
		v = settlementPrice - o.Strike
	} else {
		v = o.Strike - settlementPrice
	}
	if v < 0 {
		return 0
	}
	return v
}

// ExpiryCashFlow auto-exercises: longs receive intrinsic × size, shorts pay.
// The premium was exchanged at trade time, so intrinsic settlement completes
// the P&L without double counting.
func (o *EuropeanOption) ExpiryCashFlow(size, entryPrice, settlementPrice, basePrecision int64) int64 {
	return etypes.MulDiv(size, o.intrinsicValue(settlementPrice), basePrecision)
}

func (o *EuropeanOption) DeliveryFee(size, settlementPrice, basePrecision int64) int64 {
	if o.DeliveryFeeBps <= 0 {
		return 0
	}
	if size < 0 {
		size = -size
	}
	fee := etypes.MulDiv(size, settlementPrice, basePrecision) * o.DeliveryFeeBps / 10000
	capBps := o.DeliveryFeeCapBps
	if capBps == 0 {
		capBps = 1250
	}
	if cap := etypes.MulDiv(size, o.intrinsicValue(settlementPrice), basePrecision) * capBps / 10000; fee > cap {
		fee = cap
	}
	return fee
}

var _ etypes.Expirable = (*EuropeanOption)(nil)

// --- OrderMarginer (side-aware reservation from the perp wallet) ---

// sellerIMPerUnit implements max(IMBase − OTM%, IMFloor) × S + mark premium.
// Before the first mark update (no underlying price cached) it falls back to
// 2× the order premium, which over-margins rather than under-margins.
func (o *EuropeanOption) sellerIMPerUnit(refPremium int64) int64 {
	s := o.underlyingMark.Load()
	mark := o.markPremium.Load()
	if mark == 0 {
		mark = refPremium
	}
	if s <= 0 {
		return 2 * refPremium
	}
	var otmBps int64
	if o.IsCall && o.Strike > s {
		otmBps = (o.Strike - s) * 10000 / s
	} else if !o.IsCall && s > o.Strike {
		otmBps = (s - o.Strike) * 10000 / s
	}
	rate := o.Margin.IMBaseBps - otmBps
	if rate < o.Margin.IMFloorBps {
		rate = o.Margin.IMFloorBps
	}
	return s*rate/10000 + mark
}

func (o *EuropeanOption) MarginForOrder(side etypes.Side, qty, price, precision int64) int64 {
	if side == etypes.Buy {
		return etypes.MulDiv(qty, price, precision)
	}
	return etypes.MulDiv(qty, o.sellerIMPerUnit(price), precision)
}

func (o *EuropeanOption) MarginForMarketOrder(side etypes.Side, qty, refPrice, precision int64) int64 {
	if refPrice == 0 {
		return 0
	}
	return o.MarginForOrder(side, qty, refPrice, precision)
}

var _ etypes.OrderMarginer = (*EuropeanOption)(nil)

// --- Settleable ---

// Settle exchanges the premium in the quote asset (perp wallet), tracks the
// position at the premium price, and maintains short-side margin through the
// MarginLedger. Realized PnL on closes is implicit in the premium cash flows
// and is logged without an extra balance credit.
func (o *EuropeanOption) Settle(ctx etypes.SettlementContext) etypes.SettlementResult {
	exec := ctx.Exec
	quote := o.QuoteAsset()
	precision := ctx.BasePrecision

	takerDelta := ctx.Positions.UpdatePosition(exec.TakerClientID, ctx.BookSymbol, exec.Qty, exec.Price, ctx.TakerOrder.Side, ctx.TakerOrder.PositionSide)
	makerDelta := ctx.Positions.UpdatePosition(exec.MakerClientID, ctx.BookSymbol, exec.Qty, exec.Price, exec.MakerSide, ctx.MakerPosSide)

	o.releaseOptionOrderMargin(ctx, ctx.TakerOrder, exec.TakerClientID, quote, precision)
	o.releaseOptionOrderMargin(ctx, ctx.MakerOrder, exec.MakerClientID, quote, precision)

	takerClosed := calcClosedQty(takerDelta.OldSize, exec.Qty, ctx.TakerOrder.Side)
	makerClosed := calcClosedQty(makerDelta.OldSize, exec.Qty, exec.MakerSide)

	o.settleShortMargin(ctx, exec.TakerClientID, ctx.TakerOrder.PositionSide, ctx.TakerOrder.Side, takerDelta, takerClosed, quote, precision)
	o.settleShortMargin(ctx, exec.MakerClientID, ctx.MakerPosSide, exec.MakerSide, makerDelta, makerClosed, quote, precision)

	takerPnL := o.settlePremium(ctx, exec.TakerClientID, ctx.TakerOrder.Side, takerDelta, takerClosed, ctx.TakerFee, quote, precision)
	makerPnL := o.settlePremium(ctx, exec.MakerClientID, exec.MakerSide, makerDelta, makerClosed, ctx.MakerFee, quote, precision)
	recordSettlementFees(ctx, quote)

	if ctx.Log != nil {
		ctx.Log.LogEvent(ctx.Timestamp, 0, "open_interest", etypes.OpenInterestEvent{
			Timestamp: ctx.Timestamp, Symbol: ctx.BookSymbol,
			OpenInterest: ctx.Positions.CalculateOpenInterest(ctx.BookSymbol),
		})
	}

	return etypes.SettlementResult{TakerDelta: takerDelta, MakerDelta: makerDelta, TakerPnL: takerPnL, MakerPnL: makerPnL}
}

// releaseOptionOrderMargin releases the filled share of the order's side-aware
// reservation, keeping what the unfilled remainder still requires.
func (o *EuropeanOption) releaseOptionOrderMargin(ctx etypes.SettlementContext, order *etypes.Order, clientID uint64, quote string, precision int64) {
	if order == nil || order.Type == etypes.Market {
		return
	}
	stillNeeded := o.MarginForOrder(order.Side, order.Qty-order.FilledQty, order.Price, precision)
	if release := order.Reserved - stillNeeded; release > 0 {
		ctx.ReleasePerp(clientID, quote, release)
		order.Reserved = stillNeeded
	}
}

// settleShortMargin maintains position margin for the short side only: a fill
// that opens short quantity reserves IM at the current formula; a fill that
// buys back short quantity releases the ledger share.
func (o *EuropeanOption) settleShortMargin(ctx etypes.SettlementContext, clientID uint64, posSide etypes.PositionSide, side etypes.Side, delta etypes.PositionDelta, closedQty int64, quote string, precision int64) {
	ledger, hasLedger := ctx.Positions.(etypes.MarginLedger)

	// Buying back an existing short releases its margin share.
	if side == etypes.Buy && delta.OldSize < 0 && closedQty > 0 {
		var release int64
		if hasLedger {
			release = ledger.ReleasePositionMargin(clientID, ctx.BookSymbol, posSide, closedQty, delta.OldSize)
		} else {
			release = etypes.MulDiv(closedQty, o.sellerIMPerUnit(delta.OldEntryPrice), precision)
		}
		ctx.ReleasePerp(clientID, quote, release)
	}

	// Selling beyond any long inventory opens short quantity. Derived from the
	// actual position delta so hedge-mode clamped overshoot is never margined.
	if side == etypes.Sell {
		if openedShort := absInt(delta.NewSize) - absInt(delta.OldSize) + closedQty; openedShort > 0 {
			needed := etypes.MulDiv(openedShort, o.sellerIMPerUnit(ctx.Exec.Price), precision)
			ctx.ReservePerp(clientID, quote, needed)
			if hasLedger {
				ledger.AddPositionMargin(clientID, ctx.BookSymbol, posSide, needed)
			}
		}
	}
}

// settlePremium moves the premium notional (buyer pays, seller receives) and
// debits the fee, all in the perp wallet.
func (o *EuropeanOption) settlePremium(ctx etypes.SettlementContext, clientID uint64, side etypes.Side, delta etypes.PositionDelta, closedQty int64, fee etypes.Fee, quote string, precision int64) int64 {
	premium := etypes.MulDiv(ctx.Exec.Qty, ctx.Exec.Price, precision)
	cash := premium
	if side == etypes.Buy {
		cash = -premium
	}

	pnl := calcPerpPnL(delta.OldSize, delta.OldEntryPrice, ctx.Exec.Qty, ctx.Exec.Price, side, precision)
	if pnl != 0 && ctx.Log != nil {
		ctx.Log.LogEvent(ctx.Timestamp, clientID, "realized_pnl", etypes.RealizedPnLEvent{
			Timestamp: ctx.Timestamp, ClientID: clientID, Symbol: ctx.BookSymbol,
			TradeID: ctx.BookSeqNum, ClosedQty: closedQty,
			EntryPrice: delta.OldEntryPrice, ExitPrice: ctx.Exec.Price,
			PnL: pnl, Side: side.String(),
		})
	}

	oldBal := ctx.PerpBalance(clientID, quote)
	ctx.MutatePerpBalance(clientID, quote, cash)
	deltas := []etypes.BalanceDelta{
		{Asset: quote, Wallet: "perp", OldBalance: oldBal, NewBalance: oldBal + cash, Delta: cash},
	}
	if fee.Amount != 0 {
		oldFeeBal := ctx.PerpBalance(clientID, fee.Asset)
		ctx.MutatePerpBalance(clientID, fee.Asset, -fee.Amount)
		deltas = append(deltas, etypes.BalanceDelta{
			Asset: fee.Asset, Wallet: "perp",
			OldBalance: oldFeeBal, NewBalance: oldFeeBal - fee.Amount, Delta: -fee.Amount,
		})
	}
	ctx.LogBalanceChange(clientID, ctx.BookSymbol, "option_premium", deltas)
	return pnl
}

var _ etypes.Settleable = (*EuropeanOption)(nil)
