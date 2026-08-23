package exchange

import (
	"errors"
	"fmt"
	"slices"
	"sort"
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

type listingPriceSourceFunc func(symbol string) (int64, error)

func (f listingPriceSourceFunc) Price(symbol string) (int64, error) { return f(symbol) }

// ErrNoBookPrice means no contemporaneous, two-sided executable midpoint is
// available. It deliberately includes unknown, empty, one-sided, and crossed
// books: none establishes a usable midpoint.
var ErrNoBookPrice = errors.New("no usable book price")

// bookMidPrice returns a contemporaneous midpoint of both executable sides.
// The error makes absence explicit; callers must defer, reject, or use an
// already-declared independent source rather than treat zero as a price.
func (e *DefaultExchange) bookMidPrice(symbol string) (int64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.bookMidPriceLocked(symbol)
}

// bookMidPriceLocked is bookMidPrice for callers already holding e.mu (either
// mode) — RWMutex read locks must not nest, a queued writer between them
// deadlocks.
func (e *DefaultExchange) bookMidPriceLocked(symbol string) (int64, error) {
	book := e.Books[symbol]
	if book == nil {
		return 0, fmt.Errorf("%w: %s book missing", ErrNoBookPrice, symbol)
	}
	if book.Bids.Best == nil || book.Asks.Best == nil {
		return 0, fmt.Errorf("%w: %s book is one-sided or empty", ErrNoBookPrice, symbol)
	}
	bid, ask := book.Bids.Best.Price, book.Asks.Best.Price
	if bid <= 0 || ask <= 0 || bid > ask {
		return 0, fmt.Errorf("%w: %s has invalid best prices bid=%d ask=%d", ErrNoBookPrice, symbol, bid, ask)
	}
	// Resting limit prices are positive and an uncrossed live book has
	// bid <= ask, so ask-bid is in [0, MaxInt64-1] and cannot overflow.
	// This form avoids the otherwise possible overflow in bid+ask.
	return bid + (ask-bid)/2, nil
}

// bookReferencePrice returns the declared derivative/index reference policy:
// a true midpoint when both sides exist, otherwise the sole displayed best
// price. It is intentionally distinct from bookMidPrice so consumers cannot
// mistake a one-sided reference for a mathematical midpoint.
func (e *DefaultExchange) bookReferencePrice(symbol string) (int64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.bookReferencePriceLocked(symbol)
}

func (e *DefaultExchange) bookReferencePriceLocked(symbol string) (int64, error) {
	book := e.Books[symbol]
	if book == nil {
		return 0, fmt.Errorf("%w: %s book missing", ErrNoBookPrice, symbol)
	}
	if book.Bids.Best != nil && book.Asks.Best != nil {
		return e.bookMidPriceLocked(symbol)
	}
	if book.Bids.Best != nil && book.Bids.Best.Price > 0 {
		return book.Bids.Best.Price, nil
	}
	if book.Asks.Best != nil && book.Asks.Best.Price > 0 {
		return book.Asks.Best.Price, nil
	}
	return 0, fmt.Errorf("%w: %s book is empty or has an invalid best price", ErrNoBookPrice, symbol)
}

// configuredIndexPrice uses the declared external reference only when it
// supplies a positive price. The PriceSource interface predates explicit
// absence errors, so adapt it once at this boundary rather than propagate its
// zero sentinel into a mark or settlement calculation.
func (e *DefaultExchange) configuredIndexPrice(symbol string) (int64, error) {
	if e.indexProvider == nil {
		return 0, fmt.Errorf("%w: no configured index for %s", ErrNoBookPrice, symbol)
	}
	price := e.indexProvider.Price(symbol)
	if price <= 0 {
		return 0, fmt.Errorf("%w: configured index unavailable for %s", ErrNoBookPrice, symbol)
	}
	return price, nil
}

// derivativeUnderlyingPrice resolves a derivative's declared underlying from
// its explicit top-of-book reference policy, then from its pre-existing
// configured index fallback.
func (e *DefaultExchange) derivativeUnderlyingPrice(inst Instrument) (int64, error) {
	if ref, ok := inst.(etypes.UnderlyingRef); ok && ref.UnderlyingSymbol() != "" {
		price, err := e.bookReferencePrice(ref.UnderlyingSymbol())
		if err == nil {
			return price, nil
		}
		fallback, fallbackErr := e.configuredIndexPrice(inst.Symbol())
		if fallbackErr == nil {
			return fallback, nil
		}
		return 0, fmt.Errorf("derivative %s underlying %s: %w", inst.Symbol(), ref.UnderlyingSymbol(), err)
	}
	price, err := e.configuredIndexPrice(inst.Symbol())
	if err != nil {
		return 0, fmt.Errorf("derivative %s: %w", inst.Symbol(), err)
	}
	return price, nil
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
	prices := listingPriceSourceFunc(e.bookMidPrice)
	for _, policy := range e.listingPolicies {
		pending, err := policy.PendingListings(now, prices)
		if err != nil {
			if errors.Is(err, ErrNoBookPrice) {
				// No valid underlying midpoint: defer this automatic listing.
				continue
			}
			panic(fmt.Sprintf("exchange: listing policy: %v", err))
		}
		for _, inst := range pending {
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
		underlyingPrice, err := e.derivativeUnderlyingPrice(inst)
		if err != nil {
			// No valid underlying reference: defer settlement sampling and option
			// marks rather than inventing a zero price.
			continue
		}
		inst.(Expirable).ObserveSettlement(underlyingPrice, now)

		if opt, ok := inst.(*einstrument.EuropeanOption); ok {
			yearsLeft := float64(opt.ExpiryNano()-now) / float64(365*24*time.Hour)
			mark := eprice.Black76Premium(underlyingPrice, opt.Strike, opt.IV, yearsLeft, opt.IsCall)
			opt.SetMarks(underlyingPrice, mark)
		}
	}
	e.publishIndexFeeds(now)
	if e.postDerivativeMarkHook != nil {
		// The hook sees a complete fresh mark set and precedes any same-timestamp
		// expiry. It must remain read-only because it runs outside e.mu.
		e.postDerivativeMarkHook()
	}
}

// publishIndexFeeds publishes the venue's reference price for each configured
// symbol. A venue publishing an index is not a convenience: it is the only
// public reference a participant has that is not derived from the book it is
// quoting into, so without it every price-setter can only observe itself.
func (e *DefaultExchange) publishIndexFeeds(now int64) {
	if e.indexFeedProvider == nil || len(e.indexFeedSymbols) == 0 {
		return
	}
	for _, symbol := range e.indexFeedSymbols {
		price := e.indexFeedProvider.Price(symbol)
		if price <= 0 {
			continue
		}
		e.MDPublisher.Publish(symbol, MDIndex, &IndexPrice{Symbol: symbol, Price: price, Timestamp: now}, now)
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
	if len(expired) > 0 && e.preExpiryHook != nil {
		// This is deliberately outside e.mu: a strict account snapshot acquires
		// the read lock and must observe the fully marked, still-listed board.
		// The hook contract is read-only, so no exchange state changes between
		// the expiry set being identified and contractual settlement below.
		e.preExpiryHook()
	}
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
// replayListedInstruments sends one "listed" announcement per live derivative
// to a single new subscriber.
//
// It is addressed to that subscriber rather than published, because every
// other participant already knows: a broadcast would tell the whole population
// that the entire chain had just been listed again, which a maker reacts to by
// requoting everything.
func (e *DefaultExchange) replayListedInstruments(clientID uint64, gateway *ClientGateway) {
	if gateway == nil {
		return
	}
	now := e.Clock.NowUnixNano()
	symbols := make([]string, 0, len(e.Instruments))
	for symbol := range e.Instruments {
		symbols = append(symbols, symbol)
	}
	// Map order is randomised per process, and a participant that learns about
	// contracts in a different order every run is a different participant.
	sort.Strings(symbols)
	for _, symbol := range symbols {
		instrument := e.Instruments[symbol]
		if _, dated := instrument.(etypes.Expirable); !dated {
			continue
		}
		if !gateway.IsRunning() {
			return
		}
		message := &etypes.MarketDataMsg{
			Type:      MDInstrument,
			Symbol:    etypes.InstrumentFeedSymbol,
			Timestamp: now,
			Data:      describeInstrument(instrument, "listed", now),
		}
		select {
		case gateway.MarketDataChan() <- message:
		default:
			// The subscriber's inbox is full at the instant it subscribed. It
			// will learn about later listings; there is nothing useful to do
			// here beyond not blocking the caller, which holds the book lock.
			return
		}
	}
}

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
