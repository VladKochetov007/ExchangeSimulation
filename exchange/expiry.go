package exchange

import (
	"slices"
	"time"

	einstrument "exchange_sim/instrument"
	eprice "exchange_sim/price"
	etypes "exchange_sim/types"
)

// Expirable and related aliases live in exchange/types.go.

// priceSourceFunc adapts a closure to the PriceSource interface for
// ListingPolicy calls.
type priceSourceFunc func(symbol string) int64

func (f priceSourceFunc) Price(symbol string) int64 { return f(symbol) }

// bookMidPrice returns the mid of a book's best bid/ask (one-sided best when
// only one side is quoted, 0 when the book is empty or unknown).
func (e *DefaultExchange) bookMidPrice(symbol string) int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.bookMidPriceLocked(symbol)
}

// bookMidPriceLocked is bookMidPrice for callers already holding e.mu (either
// mode) — RWMutex read locks must not nest, a queued writer between them
// deadlocks.
func (e *DefaultExchange) bookMidPriceLocked(symbol string) int64 {
	book := e.Books[symbol]
	if book == nil {
		return 0
	}
	hasBid := book.Bids.Best != nil
	hasAsk := book.Asks.Best != nil
	switch {
	case hasBid && hasAsk:
		return (book.Bids.Best.Price + book.Asks.Best.Price) / 2
	case hasBid:
		return book.Bids.Best.Price
	case hasAsk:
		return book.Asks.Best.Price
	}
	return 0
}

// expiryLoop drives listings, derivative mark updates, and expiry settlement.
func (e *DefaultExchange) expiryLoop(ticker Ticker) {
	defer e.automWg.Done()
	defer ticker.Stop()

	for {
		select {
		case <-e.automCtx.Done():
			return
		case <-ticker.C():
			e.automInFlight.Add(1)
			e.CheckListings()
			e.UpdateDerivativeMarks()
			// Expiry is contractual settlement, not a liquidation trigger. Settle
			// and delist contracts first so an at-expiry option cannot be
			// force-traded (and charged a clearance fee) just before cash exercise.
			e.CheckExpiries()
			// After marks refresh: option books never enter the perp mark
			// loop, so this sweep is the only liquidation path for accounts
			// whose exposure is options-only.
			e.CheckPositionMarginerLiquidations()
			e.automInFlight.Add(-1)
			acknowledgeTicker(ticker)
		}
	}
}

// CheckListings polls configured listing policies and lists whatever they
// return, announcing each new instrument on the reference-data feed.
func (e *DefaultExchange) CheckListings() {
	if len(e.listingPolicies) == 0 {
		return
	}
	now := e.Clock.NowUnixNano()
	prices := priceSourceFunc(e.bookMidPrice)
	for _, policy := range e.listingPolicies {
		for _, inst := range policy.PendingListings(now, prices) {
			symbol := inst.Symbol()
			e.mu.RLock()
			_, exists := e.Instruments[symbol]
			e.mu.RUnlock()
			if exists {
				continue
			}
			e.AddInstrument(inst)
			ann := describeInstrument(inst, "listed", now)
			e.MDPublisher.Publish(etypes.InstrumentFeedSymbol, MDInstrument, ann, now)
			if log := e.getLogger("_global"); log != nil {
				log.LogEvent(now, 0, "instrument_listed", ann)
			}
		}
	}
}

// UpdateDerivativeMarks feeds settlement observations to every live Expirable
// and refreshes option marks (underlying mid + Black-76 premium) used by the
// seller margin formula.
func (e *DefaultExchange) UpdateDerivativeMarks() {
	now := e.Clock.NowUnixNano()

	e.mu.RLock()
	expirables := make([]Instrument, 0)
	for _, inst := range e.Instruments {
		if _, ok := inst.(Expirable); ok {
			expirables = append(expirables, inst)
		}
	}
	e.mu.RUnlock()

	for _, inst := range expirables {
		underlying := ""
		if ref, ok := inst.(etypes.UnderlyingRef); ok {
			underlying = ref.UnderlyingSymbol()
		}
		underlyingPrice := int64(0)
		if underlying != "" {
			underlyingPrice = e.bookMidPrice(underlying)
		}
		if underlyingPrice == 0 && e.indexProvider != nil {
			underlyingPrice = e.indexProvider.Price(inst.Symbol())
		}
		if underlyingPrice == 0 {
			continue
		}
		inst.(Expirable).ObserveSettlement(underlyingPrice, now)

		if opt, ok := inst.(*einstrument.EuropeanOption); ok {
			yearsLeft := float64(opt.ExpiryNano()-now) / float64(365*24*time.Hour)
			mark := eprice.Black76Premium(underlyingPrice, opt.Strike, opt.IV, yearsLeft, opt.IsCall)
			opt.SetMarks(underlyingPrice, mark)
		}
	}
}

// CheckExpiries settles and delists every instrument past its expiry.
func (e *DefaultExchange) CheckExpiries() {
	now := e.Clock.NowUnixNano()

	e.mu.RLock()
	var expired []string
	for symbol, inst := range e.Instruments {
		if exp, ok := inst.(Expirable); ok && now >= exp.ExpiryNano() {
			expired = append(expired, symbol)
		}
	}
	e.mu.RUnlock()

	// Settlement cancels orders and emits events, so map iteration here would
	// make same-timestamp expiries observably nondeterministic.
	slices.Sort(expired)
	for _, symbol := range expired {
		e.settleExpiredInstrument(symbol, now)
	}
}

// ExpirySettlementEvent is logged per position at expiry.
type ExpirySettlementEvent struct {
	Timestamp       int64  `json:"timestamp"`
	ClientID        uint64 `json:"client_id"`
	Symbol          string `json:"symbol"`
	Size            int64  `json:"size"`
	EntryPrice      int64  `json:"entry_price"`
	SettlementPrice int64  `json:"settlement_price"`
	CashFlow        int64  `json:"cash_flow"`
	DeliveryFee     int64  `json:"delivery_fee"`
}

// settleExpiredInstrument halts the book, cancels all resting orders (exact
// ledger releases + forced-cancel notifications), cash-settles every position
// at the settlement price, releases position margin, and delists.
func (e *DefaultExchange) settleExpiredInstrument(symbol string, now int64) {
	e.mu.Lock()

	book := e.Books[symbol]
	if book == nil {
		e.mu.Unlock()
		return
	}
	inst := book.Instrument
	exp := inst.(Expirable)
	settlementPrice := exp.SettlementPrice()
	quote := inst.QuoteAsset()
	precision := inst.BasePrecision()

	// Cancel in client-ID order: each cancel republishes the book, so map
	// order would produce a different delta sequence every run.
	expiryClientIDs := make([]uint64, 0, len(e.Clients))
	for clientID := range e.Clients {
		expiryClientIDs = append(expiryClientIDs, clientID)
	}
	slices.Sort(expiryClientIDs)
	for _, clientID := range expiryClientIDs {
		e.cancelClientOrdersOnBook(e.Clients[clientID], book, inst)
	}

	type expiringPos struct {
		clientID uint64
		pos      Position
	}
	var positions []expiringPos
	e.Positions.PositionsForFunding(symbol, func(clientID uint64, pos Position) {
		positions = append(positions, expiringPos{clientID: clientID, pos: pos})
	})

	ledger, hasLedger := e.Positions.(etypes.MarginLedger)
	margined, isMargined := inst.(Margined)
	log := e.getLogger(symbol)

	var feeTotal int64
	for _, ep := range positions {
		pos := ep.pos
		client := e.Clients[ep.clientID]
		if client == nil || pos.Size == 0 {
			continue
		}
		absSize := pos.Size
		if absSize < 0 {
			absSize = -absSize
		}

		// Release position margin exactly via the ledger; fall back to the
		// entry-price recomputation for margined instruments without one.
		var release int64
		if hasLedger {
			release = ledger.ReleasePositionMargin(ep.clientID, symbol, pos.PositionSide, absSize, pos.Size)
		} else if isMargined {
			release = margined.MarginRequired(absSize, pos.EntryPrice, precision)
		}
		if release > 0 {
			client.ReleasePerp(quote, release)
		}

		// A contract whose underlying never printed has no settlement price;
		// closing flat (zero cash, zero fee) beats settling futures at 0 —
		// which would debit longs their entire entry notional — or paying
		// puts the full strike.
		var cash, fee int64
		if settlementPrice > 0 {
			cash = exp.ExpiryCashFlow(pos.Size, pos.EntryPrice, settlementPrice, precision)
			fee = exp.DeliveryFee(pos.Size, settlementPrice, precision)
		} else if log != nil {
			log.LogEvent(now, ep.clientID, "settlement_price_unavailable", map[string]any{
				"symbol": symbol, "size": pos.Size, "entry_price": pos.EntryPrice,
			})
		}
		oldBal := client.PerpBalances[quote]
		client.PerpBalances[quote] += cash - fee
		feeTotal += fee

		closeSide := Sell
		if pos.Size < 0 {
			closeSide = Buy
		}
		e.Positions.UpdatePosition(ep.clientID, symbol, absSize, settlementPrice, closeSide, pos.PositionSide)

		if log != nil {
			log.LogEvent(now, ep.clientID, "expiry_settlement", ExpirySettlementEvent{
				Timestamp: now, ClientID: ep.clientID, Symbol: symbol,
				Size: pos.Size, EntryPrice: pos.EntryPrice,
				SettlementPrice: settlementPrice, CashFlow: cash, DeliveryFee: fee,
			})
			log.LogEvent(now, ep.clientID, "balance_change", BalanceChangeEvent{
				Timestamp: now, ClientID: ep.clientID, Symbol: symbol, Reason: "expiry_settlement",
				Changes: []BalanceDelta{{Asset: quote, Wallet: "perp", OldBalance: oldBal, NewBalance: oldBal + cash - fee, Delta: cash - fee}},
			})
		}
	}

	if feeTotal > 0 {
		e.recordFeeRevenue(quote, Fee{Amount: feeTotal, Asset: quote}, Fee{}, book, now)
	}

	delete(e.Books, symbol)
	delete(e.Instruments, symbol)
	// The AUTO-anchored mark calculator dies with the instrument: the map is
	// keyed by symbol, and a relisting under the same symbol must seed a
	// FRESH basis EMA — inheriting the dead contract's seeded state marks
	// the new book off a basis it never had. User-injected calculators stay:
	// dropping explicit configuration on lifecycle events is not ours to do.
	if e.autoAnchoredSymbols[symbol] {
		delete(e.markPriceCalcs, symbol)
		delete(e.autoAnchoredSymbols, symbol)
	}
	e.mu.Unlock()

	ann := describeInstrument(inst, "settled", now)
	ann.SettlementPrice = settlementPrice
	e.MDPublisher.Publish(etypes.InstrumentFeedSymbol, MDInstrument, ann, now)
	if glog := e.getLogger("_global"); glog != nil {
		glog.LogEvent(now, 0, "instrument_settled", ann)
	}
}

// describeInstrument builds the lifecycle announcement / descriptor.
func describeInstrument(inst Instrument, action string, now int64) *etypes.InstrumentAnnouncement {
	ann := &etypes.InstrumentAnnouncement{
		Action:         action,
		Symbol:         inst.Symbol(),
		InstrumentType: inst.InstrumentType(),
		TickSize:       inst.TickSize(),
		MinOrderSize:   inst.MinOrderSize(),
		Timestamp:      now,
	}
	if ref, ok := inst.(etypes.UnderlyingRef); ok {
		ann.Underlying = ref.UnderlyingSymbol()
	}
	if exp, ok := inst.(Expirable); ok {
		ann.ExpiryNano = exp.ExpiryNano()
	}
	if opt, ok := inst.(*einstrument.EuropeanOption); ok {
		ann.Strike = opt.Strike
		ann.IsCall = opt.IsCall
	}
	return ann
}

// QueryInstruments returns descriptors for all currently listed instruments.
func (e *DefaultExchange) QueryInstruments(clientID uint64, req *QueryRequest) Response {
	now := e.Clock.NowUnixNano()
	e.mu.RLock()
	defer e.mu.RUnlock()
	instruments := make([]*etypes.InstrumentAnnouncement, 0, len(e.Instruments))
	for _, inst := range e.Instruments {
		instruments = append(instruments, describeInstrument(inst, "listed", now))
	}
	reqID := uint64(0)
	if req != nil {
		reqID = req.RequestID
	}
	return Response{RequestID: reqID, Success: true, Data: instruments}
}
