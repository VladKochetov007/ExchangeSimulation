package feesim

import (
	"context"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// TriArbConfig configures a fee-aware triangular arbitrage actor.
//
// Triangle: Q/USD ↔ Q/ABC ↔ ABC/USD
//
//	Direction A (buy Q cheap via ABC): buy ABC (ABC/USD) → buy Q (Q/ABC) → sell Q (Q/USD)
//	Direction B (sell Q expensive via ABC): buy Q (Q/USD) → sell Q for ABC (Q/ABC) → sell ABC (ABC/USD)
type TriArbConfig struct {
	QUSDSymbol     string // "Q/USD"
	ABCUSDSymbol   string // "ABC/USD"
	QABCSymbol     string // "Q/ABC"
	TakerFeeBps    int64  // taker fee per leg (spot)
	TargetNotional int64  // target USD notional per arb
	QPrecision     int64  // Q base precision (ETH_PRECISION)
	ABCPrecision   int64  // ABC base precision (BTC_PRECISION)
	CheckInterval  time.Duration
}

type bookTop struct {
	bid int64
	ask int64
}

// FeeAwareTriArb executes triangular arbitrage across Q/USD, ABC/USD, Q/ABC.
// Fee-aware: only executes when profit > 3 × TakerFeeBps.
// Sequential 3-leg state machine with fill-before-accept buffering.
type FeeAwareTriArb struct {
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
	bufferedFillID uint64
}

func NewFeeAwareTriArb(id uint64, gw actor.Gateway, cfg TriArbConfig) *FeeAwareTriArb {
	a := &FeeAwareTriArb{
		BaseActor: actor.NewBaseActor(id, gw),
		cfg:       cfg,
		books:     make(map[string]bookTop),
	}
	a.SetHandler(a)
	a.AddTicker(cfg.CheckInterval, a.onTick)
	return a
}

func (a *FeeAwareTriArb) Start(ctx context.Context) error {
	a.Subscribe(a.cfg.QUSDSymbol, exchange.MDSnapshot)
	a.Subscribe(a.cfg.ABCUSDSymbol, exchange.MDSnapshot)
	a.Subscribe(a.cfg.QABCSymbol, exchange.MDSnapshot)
	return a.BaseActor.Start(ctx)
}

func (a *FeeAwareTriArb) HandleEvent(_ context.Context, evt *actor.Event) {
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

func (a *FeeAwareTriArb) onSnapshot(e actor.BookSnapshotEvent) {
	bid, ask := int64(0), int64(0)
	if len(e.Snapshot.Bids) > 0 {
		bid = e.Snapshot.Bids[0].Price
	}
	if len(e.Snapshot.Asks) > 0 {
		ask = e.Snapshot.Asks[0].Price
	}
	a.books[e.Symbol] = bookTop{bid: bid, ask: ask}
}

func (a *FeeAwareTriArb) onAccepted(e actor.OrderAcceptedEvent) {
	if e.RequestID != a.pendingReqID {
		return
	}
	a.pendingOrderID = e.OrderID
	if a.bufferedFillID == e.OrderID {
		a.bufferedFillID = 0
		a.advanceLeg()
	}
}

func (a *FeeAwareTriArb) onFilled(e actor.OrderFillEvent) {
	if a.pendingOrderID == 0 {
		a.bufferedFillID = e.OrderID
		return
	}
	if e.OrderID != a.pendingOrderID {
		return
	}
	a.advanceLeg()
}

func (a *FeeAwareTriArb) onRejected(e actor.OrderRejectedEvent) {
	if e.RequestID == a.pendingReqID {
		a.resetToIdle()
	}
}

func (a *FeeAwareTriArb) advanceLeg() {
	a.legIndex++
	if a.legIndex >= 3 {
		a.resetToIdle()
		return
	}
	a.executingTicks = 0
	a.submitCurrentLeg()
}

func (a *FeeAwareTriArb) submitCurrentLeg() {
	a.pendingReqID = a.SubmitOrder(
		a.legSyms[a.legIndex],
		a.legSides[a.legIndex],
		exchange.Market, 0,
		a.legQtys[a.legIndex],
	)
	a.pendingOrderID = 0
}

func (a *FeeAwareTriArb) resetToIdle() {
	a.executing = false
	a.executingTicks = 0
	a.legIndex = 0
	a.pendingReqID = 0
	a.pendingOrderID = 0
	a.bufferedFillID = 0
}

func (a *FeeAwareTriArb) onTick(_ time.Time) {
	if a.executing {
		a.executingTicks++
		if a.executingTicks > 10 {
			a.resetToIdle()
		}
		return
	}
	a.checkArb()
}

func (a *FeeAwareTriArb) checkArb() {
	qusd := a.books[a.cfg.QUSDSymbol]
	abcusd := a.books[a.cfg.ABCUSDSymbol]
	qabc := a.books[a.cfg.QABCSymbol]

	if qusd.bid == 0 || qusd.ask == 0 || abcusd.bid == 0 || abcusd.ask == 0 || qabc.bid == 0 || qabc.ask == 0 {
		return
	}

	abcPrec := a.cfg.ABCPrecision

	// Calculate lot sizes based on target notional.
	qMid := (qusd.bid + qusd.ask) / 2
	lotQ := a.cfg.TargetNotional * a.cfg.QPrecision / qMid
	if lotQ == 0 {
		return
	}
	lotABC := lotQ * qabc.ask / abcPrec // ABC needed to buy lotQ via cross
	if lotABC == 0 {
		lotABC = lotQ * qabc.bid / abcPrec
	}
	if lotABC == 0 {
		return
	}

	// Total fee cost in bps across 3 legs.
	totalFeeBps := 3 * a.cfg.TakerFeeBps

	// Direction A: buy ABC (ABC/USD) → buy Q with ABC (Q/ABC) → sell Q (Q/USD)
	// Cost: abcusd.ask × qabc.ask / ABCPrecision = implied Q/USD cost via triangle
	// Revenue: qusd.bid
	impliedCostA := abcusd.ask * qabc.ask / abcPrec
	feeAmountA := totalFeeBps * impliedCostA / 10000
	if qusd.bid > impliedCostA+feeAmountA {
		lotABCForA := lotQ * qabc.ask / abcPrec
		if lotABCForA > 0 {
			a.startExecution(false, lotQ, lotABCForA)
			return
		}
	}

	// Direction B: buy Q (Q/USD) → sell Q for ABC (Q/ABC) → sell ABC (ABC/USD)
	// Revenue: qabc.bid × abcusd.bid / ABCPrecision = implied Q/USD revenue via triangle
	// Cost: qusd.ask
	impliedRevenueB := qabc.bid * abcusd.bid / abcPrec
	feeAmountB := totalFeeBps * qusd.ask / 10000
	if impliedRevenueB > qusd.ask+feeAmountB {
		lotABCForB := lotQ * qabc.bid / abcPrec
		if lotABCForB > 0 {
			a.startExecution(true, lotQ, lotABCForB)
		}
	}
}

// startExecution sets up a 3-leg arb sequence.
//
// Direction A (dirB=false): buy ABC (ABC/USD) → buy Q (Q/ABC) → sell Q (Q/USD)
// Direction B (dirB=true):  buy Q (Q/USD) → sell Q (Q/ABC) → sell ABC (ABC/USD)
func (a *FeeAwareTriArb) startExecution(dirB bool, lotQ, lotABC int64) {
	if dirB {
		a.legSyms = [3]string{a.cfg.QUSDSymbol, a.cfg.QABCSymbol, a.cfg.ABCUSDSymbol}
		a.legSides = [3]exchange.Side{exchange.Buy, exchange.Sell, exchange.Sell}
		a.legQtys = [3]int64{lotQ, lotQ, lotABC}
	} else {
		a.legSyms = [3]string{a.cfg.ABCUSDSymbol, a.cfg.QABCSymbol, a.cfg.QUSDSymbol}
		a.legSides = [3]exchange.Side{exchange.Buy, exchange.Buy, exchange.Sell}
		a.legQtys = [3]int64{lotABC, lotQ, lotQ}
	}
	a.executing = true
	a.executingTicks = 0
	a.legIndex = 0
	a.submitCurrentLeg()
}
