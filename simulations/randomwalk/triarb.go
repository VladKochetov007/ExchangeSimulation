package randomwalk

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// TriArbConfig configures a triangular arbitrage actor.
//
// Triangle: QuoteUSD ↔ Cross ↔ BaseUSD
//   Direction A (buy cheap via quote asset): buy QuoteUSD → buy Cross → sell BaseUSD
//   Direction B (sell expensive via quote asset): buy BaseUSD → sell Cross → sell QuoteUSD
type TriArbConfig struct {
	CrossSymbol    string
	BaseUSDSymbol  string
	QuoteUSDSymbol string
	TargetNotional int64         // target USD notional per arb execution
	MinProfitBps   int64         // minimum bps profit to trigger (after spread)
	BasePrecision  int64         // precision of base and quote assets (BTC_PRECISION)
	CheckInterval  time.Duration
}

type bookTop struct {
	bid int64
	ask int64
}

// TriArbActor executes triangular arbitrage across three spot markets.
// It uses a sequential state machine: idle → leg1 → leg2 → leg3 → idle.
// Each leg is a market order; the next leg starts only after a full fill.
// A timeout (10 ticks) resets the machine if a leg never fills.
//
// Fill-before-accepted ordering: the exchange sends fill notifications inside
// PlaceOrder (under lock) before returning the accepted response. So fills can
// arrive in ResponseCh before the accepted event. We buffer the fill orderID
// and apply it when the accepted event arrives.
type TriArbActor struct {
	*actor.BaseActor
	cfg            TriArbConfig
	books          map[string]bookTop
	executing      bool
	executingTicks int
	legIndex       int
	legSyms        [3]string
	legSides       [3]exchange.Side
	legQtys        [3]int64
	pendingReqID   uint64
	pendingOrderID uint64
	bufferedFillID uint64 // orderID from fill that arrived before accepted
}

func NewTriArbActor(id uint64, gw actor.Gateway, cfg TriArbConfig) *TriArbActor {
	a := &TriArbActor{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		books:     make(map[string]bookTop),
	}
	a.SetHandler(a)
	a.AddTicker(cfg.CheckInterval, a.onTick)
	return a
}

func (a *TriArbActor) Start(ctx context.Context) error {
	a.Subscribe(a.cfg.CrossSymbol, exchange.MDSnapshot)
	a.Subscribe(a.cfg.BaseUSDSymbol, exchange.MDSnapshot)
	a.Subscribe(a.cfg.QuoteUSDSymbol, exchange.MDSnapshot)
	return a.BaseActor.Start(ctx)
}

func (a *TriArbActor) HandleEvent(_ context.Context, evt *actor.Event) {
	switch evt.Type {
	case actor.EventBookSnapshot:
		a.onSnapshot(evt.Data.(actor.BookSnapshotEvent))
	case actor.EventOrderAccepted:
		a.onAccepted(evt.Data.(actor.OrderAcceptedEvent))
	case actor.EventOrderFilled:
		a.onFilled(evt.Data.(actor.OrderFillEvent))
	case actor.EventOrderRejected:
		a.onRejected(evt.Data.(actor.OrderRejectedEvent))
	}
}

func (a *TriArbActor) onSnapshot(e actor.BookSnapshotEvent) {
	bid, ask := int64(0), int64(0)
	if len(e.Snapshot.Bids) > 0 {
		bid = e.Snapshot.Bids[0].Price
	}
	if len(e.Snapshot.Asks) > 0 {
		ask = e.Snapshot.Asks[0].Price
	}
	a.books[e.Symbol] = bookTop{bid: bid, ask: ask}
}

func (a *TriArbActor) onAccepted(e actor.OrderAcceptedEvent) {
	if e.RequestID != a.pendingReqID {
		return
	}
	a.pendingOrderID = e.OrderID
	// Fill may have arrived before accepted due to exchange ordering.
	if a.bufferedFillID == e.OrderID {
		a.bufferedFillID = 0
		a.advanceLeg()
	}
}

func (a *TriArbActor) onFilled(e actor.OrderFillEvent) {
	if a.pendingOrderID == 0 {
		// Accepted not yet received — buffer the fill orderID.
		a.bufferedFillID = e.OrderID
		return
	}
	if e.OrderID != a.pendingOrderID {
		return
	}
	a.advanceLeg()
}

func (a *TriArbActor) onRejected(e actor.OrderRejectedEvent) {
	if e.RequestID == a.pendingReqID {
		a.resetToIdle()
	}
}

func (a *TriArbActor) advanceLeg() {
	a.legIndex++
	if a.legIndex >= 3 {
		a.resetToIdle()
		return
	}
	a.executingTicks = 0
	a.submitCurrentLeg()
}

func (a *TriArbActor) submitCurrentLeg() {
	a.pendingReqID = a.SubmitOrder(
		a.legSyms[a.legIndex],
		a.legSides[a.legIndex],
		exchange.Market, 0,
		a.legQtys[a.legIndex],
	)
	a.pendingOrderID = 0
}

func (a *TriArbActor) resetToIdle() {
	a.executing = false
	a.executingTicks = 0
	a.legIndex = 0
	a.pendingReqID = 0
	a.pendingOrderID = 0
	a.bufferedFillID = 0
}

func (a *TriArbActor) onTick(_ time.Time) {
	if a.executing {
		a.executingTicks++
		if a.executingTicks > 10 {
			// Leg stuck (e.g. book was empty when market order submitted) — abort.
			a.resetToIdle()
		}
		return
	}
	a.checkArb()
}

func (a *TriArbActor) checkArb() {
	bp := a.cfg.BasePrecision
	cross := a.books[a.cfg.CrossSymbol]
	base := a.books[a.cfg.BaseUSDSymbol]
	quote := a.books[a.cfg.QuoteUSDSymbol]

	if cross.bid == 0 || cross.ask == 0 || base.bid == 0 || base.ask == 0 || quote.bid == 0 || quote.ask == 0 {
		return
	}

	baseUSDMid := (base.bid + base.ask) / 2
	lotBase := a.cfg.TargetNotional * bp / baseUSDMid
	if lotBase == 0 {
		return
	}

	minProfit := a.cfg.MinProfitBps * base.ask / 10000

	// Direction A: buy quoteAsset (ABC) → buy baseAsset (DEF) on cross → sell baseAsset on baseUSD
	// Implied cost of 1 DEF via triangle = askQuote(USD/ABC) * askCross(ABC/DEF) / BasePrecision
	impliedA := quote.ask * cross.ask / bp
	if base.bid > impliedA+minProfit {
		lotQuote := lotBase * cross.ask / bp
		if lotQuote > 0 {
			a.startExecution(false, lotBase, lotQuote)
			return
		}
	}

	// Direction B: buy baseAsset (DEF) on baseUSD → sell baseAsset on cross → sell quoteAsset (ABC) on quoteUSD
	// Revenue from 1 DEF sold via triangle = bidCross(ABC/DEF) * bidQuote(USD/ABC) / BasePrecision
	revenueB := cross.bid * quote.bid / bp
	if revenueB > base.ask+minProfit {
		lotQuote := lotBase * cross.bid / bp
		if lotQuote > 0 {
			a.startExecution(true, lotBase, lotQuote)
		}
	}
}

// startExecution sets up and submits the first leg of a 3-leg arb sequence.
//
// Direction A (dirB=false): USD → quoteUSD (buy ABC) → cross (buy DEF) → baseUSD (sell DEF)
// Direction B (dirB=true):  USD → baseUSD (buy DEF) → cross (sell DEF) → quoteUSD (sell ABC)
func (a *TriArbActor) startExecution(dirB bool, lotBase, lotQuote int64) {
	if dirB {
		a.legSyms = [3]string{a.cfg.BaseUSDSymbol, a.cfg.CrossSymbol, a.cfg.QuoteUSDSymbol}
		a.legSides = [3]exchange.Side{exchange.Buy, exchange.Sell, exchange.Sell}
		a.legQtys = [3]int64{lotBase, lotBase, lotQuote}
	} else {
		a.legSyms = [3]string{a.cfg.QuoteUSDSymbol, a.cfg.CrossSymbol, a.cfg.BaseUSDSymbol}
		a.legSides = [3]exchange.Side{exchange.Buy, exchange.Buy, exchange.Sell}
		a.legQtys = [3]int64{lotQuote, lotBase, lotBase}
	}
	a.executing = true
	a.executingTicks = 0
	a.legIndex = 0
	a.submitCurrentLeg()
}
