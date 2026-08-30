package exchange

import (
	"errors"
	"exchange_sim/census"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	einstrument "exchange_sim/instrument"
	eprice "exchange_sim/price"
	etypes "exchange_sim/types"
)

// Expirable and related aliases live in exchange/types.go.

// priceSourceFunc adapts a closure to the error-aware PriceSource interface.
type priceSourceFunc func(symbol string) (int64, error)

func (f priceSourceFunc) Price(symbol string) (int64, error) { return f(symbol) }

type listingPriceSourceFunc func(symbol string) (int64, error)

func (f listingPriceSourceFunc) Price(symbol string) (int64, error) { return f(symbol) }

// ErrNoBookPrice means no contemporaneous, two-sided executable midpoint is
// available. It deliberately includes unknown, empty, one-sided, and crossed
// books: none establishes a usable midpoint.
var ErrNoBookPrice = etypes.ErrNoPrice

// isPriceUnavailable classifies a failed consumer price boundary. ErrNoPrice
// means no observation arrived; ErrPriceDomain means a numeric observation
// arrived but cannot be used by this operation's declared model. Both must be
// surfaced to a client or periodic diagnostic, never converted to price zero.
func isPriceUnavailable(err error) bool {
	return errors.Is(err, etypes.ErrNoPrice) || errors.Is(err, etypes.ErrPriceDomain)
}

// expiryLifecycleState names the contractual lifecycle rather than deriving it
// from whether a price happened to be available on one particular automation
// tick. ACTIVE is a live instrument before expiry; EXPIRY_REACHED immediately
// disables trading; SETTLEMENT_PENDING is a permanently halted contract with
// no declared settlement price; SETTLED is emitted as the terminal lifecycle
// announcement when the instrument is delisted.
//
// The default terminal-unavailable policy is intentionally RETRY_FOREVER:
// leave collateral and positions intact, keep trading/funding/marks stopped,
// and retry only the declared source at ordinary expiry checks. There is no
// automatic last-trade or zero-price fallback. A future terminal fallback
// would need its own explicit instrument policy rather than changing this
// state machine implicitly.
type expiryLifecycleState string

const (
	expiryStateActive             expiryLifecycleState = "ACTIVE"
	expiryStateExpiryReached      expiryLifecycleState = "EXPIRY_REACHED"
	expiryStateSettlementPending  expiryLifecycleState = "SETTLEMENT_PENDING"
	expiryStateSettled            expiryLifecycleState = "SETTLED"
	expiryUnavailableRetryForever                      = "RETRY_FOREVER"
)

// expirySettlementPending is retained under DefaultExchange.mu for as long
// as an expired contract lacks a declared reference. Attempts is evidence of
// retry behavior, not an economic counter: no settlement ledger work happens
// until SettlementPrice succeeds.
type expirySettlementPending struct {
	State           expiryLifecycleState
	ExpiryReachedAt int64
	Attempts        uint64
	LastReason      string
	Policy          string
}

// reportPriceUnavailable makes an intentional periodic deferral observable.
// It only serializes already-computed state: no scheduler work, actor-visible
// state, or random draw is introduced by this diagnostic.
func (e *DefaultExchange) reportPriceUnavailable(now int64, symbol, operation string, err error) {
	if err == nil {
		return
	}
	log := e.getLogger(symbol)
	if log == nil && symbol != "_global" {
		log = e.getLogger("_global")
	}
	if log != nil {
		log.LogEvent(now, 0, "price_unavailable", PriceUnavailableEvent{
			Timestamp: now,
			Symbol:    symbol,
			Operation: operation,
			Reason:    err.Error(),
		})
	}
}

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
	if bid > ask {
		return 0, fmt.Errorf("%w: %s has invalid best prices bid=%d ask=%d", ErrNoBookPrice, symbol, bid, ask)
	}
	return etypes.Midpoint(bid, ask), nil
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
	if book.Bids.Best != nil {
		return book.Bids.Best.Price, nil
	}
	if book.Asks.Best != nil {
		return book.Asks.Best.Price, nil
	}
	return 0, fmt.Errorf("%w: %s book is empty or has an invalid best price", ErrNoBookPrice, symbol)
}

// configuredIndexPrice uses the declared external reference when its source
// reports a value. The consuming instrument validates the numeric domain;
// availability is represented solely by the returned error.
func (e *DefaultExchange) configuredIndexPrice(symbol string) (int64, error) {
	if e.indexProvider == nil {
		return 0, fmt.Errorf("%w: no configured index for %s", ErrNoBookPrice, symbol)
	}
	price, err := e.indexProvider.Price(symbol)
	if err != nil {
		return 0, fmt.Errorf("configured index for %s: %w", symbol, err)
	}
	return price, nil
}

// configuredIndexPriceLocked is configuredIndexPrice for indexPriceLocked.
// MidPriceOracle normally takes the provider's read lock, which is correct for
// public callers but re-enters e.mu here. Once a writer queues, nested RLock
// would deadlock; its explicit lock-held path instead reads the already-locked
// book. Other providers retain their normal Price contract.
func (e *DefaultExchange) configuredIndexPriceLocked(symbol string) (int64, error) {
	if e.indexProvider == nil {
		return 0, fmt.Errorf("%w: no configured index for %s", ErrNoBookPrice, symbol)
	}
	type lockedPriceSource interface {
		PriceWithProviderLockHeld(symbol string) (int64, error)
	}
	source := e.indexProvider
	var (
		price int64
		err   error
	)
	if locked, ok := source.(lockedPriceSource); ok {
		price, err = locked.PriceWithProviderLockHeld(symbol)
	} else {
		price, err = source.Price(symbol)
	}
	if err != nil {
		return 0, fmt.Errorf("configured index for %s: %w", symbol, err)
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
	listed := 0
	if census.Enabled {
		defer func() { censusListings.Call(listed == 0) }()
	}
	prices := listingPriceSourceFunc(e.bookMidPrice)
	for _, policy := range e.listingPolicies {
		pending, err := policy.PendingListings(now, prices)
		if err != nil {
			if isPriceUnavailable(err) {
				// No valid underlying midpoint: defer this automatic listing.
				e.reportPriceUnavailable(now, etypes.InstrumentFeedSymbol, "listing", err)
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
			listed++
			ann := describeInstrument(inst, "listed", now, &now)
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

	type expirableData struct {
		inst    Instrument
		pending bool
	}
	e.mu.RLock()
	expirables := make([]expirableData, 0)
	for symbol, inst := range e.Instruments {
		if _, ok := inst.(Expirable); ok {
			_, pending := e.settlementPending[symbol]
			expirables = append(expirables, expirableData{inst: inst, pending: pending})
		}
	}
	e.mu.RUnlock()
	slices.SortFunc(expirables, func(a, b expirableData) int {
		return strings.Compare(a.inst.Symbol(), b.inst.Symbol())
	})

	if census.Enabled {
		censusDerivMarks.Quantity(len(expirables))
	}
	for _, data := range expirables {
		inst := data.inst
		underlyingPrice, err := e.derivativeUnderlyingPrice(inst)
		if err != nil {
			// No valid underlying reference: defer settlement sampling and option
			// marks rather than inventing a zero price.
			e.reportPriceUnavailable(now, inst.Symbol(), "derivative_mark", err)
			continue
		}
		inst.(Expirable).ObserveSettlement(underlyingPrice, now)
		if data.pending {
			// A pending contract must keep sampling its declared settlement
			// source so that a later recovery can settle it. It is nevertheless
			// permanently halted: no post-expiry option mark is published.
			continue
		}

		if census.Enabled {
			censusDerivMarks.Call(census.Repeated(
				fmt.Sprintf("uline/%p/%s", e, inst.Symbol()),
				uint64(census.NewFNV1a().Add(underlyingPrice))))
		}
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
		price, err := e.indexFeedProvider.Price(symbol)
		if err != nil {
			e.reportPriceUnavailable(now, symbol, "index_feed", fmt.Errorf("index feed: %w", err))
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
	var firstExpiry []string
	for symbol, inst := range e.Instruments {
		if exp, ok := inst.(Expirable); ok && now >= exp.ExpiryNano() {
			expired = append(expired, symbol)
			if _, pending := e.settlementPending[symbol]; !pending {
				firstExpiry = append(firstExpiry, symbol)
			}
		}
	}
	e.mu.RUnlock()
	if census.Enabled {
		censusExpiries.Call(len(expired) == 0)
		censusExpiries.Quantity(len(e.Instruments))
	}

	// Settlement cancels orders and emits events, so map iteration here would
	// make same-timestamp expiries observably nondeterministic.
	slices.Sort(expired)
	if len(firstExpiry) > 0 && e.preExpiryHook != nil {
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
	PositionSide    string `json:"position_side,omitempty"`
	BasePrecision   int64  `json:"base_precision,omitempty"`
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
	settlementPrice, err := exp.SettlementPrice()
	if err != nil {
		// Contractual expiry blocks new risk immediately. Cancel any resting
		// orders once and retain positions until a declared settlement source
		// becomes available; allowing a post-expiry fill would be worse than a
		// visible lifecycle deferral.
		pending, alreadyPending := e.settlementPending[symbol]
		if !alreadyPending {
			pending = expirySettlementPending{
				State:           expiryStateSettlementPending,
				ExpiryReachedAt: now,
				Policy:          expiryUnavailableRetryForever,
			}
			clientIDs := make([]uint64, 0, len(e.Clients))
			for clientID := range e.Clients {
				clientIDs = append(clientIDs, clientID)
			}
			slices.Sort(clientIDs)
			for _, clientID := range clientIDs {
				e.cancelClientOrdersOnBook(e.Clients[clientID], book, inst)
			}
		}
		pending.Attempts++
		pending.LastReason = err.Error()
		e.settlementPending[symbol] = pending
		log := e.getLogger(symbol)
		e.mu.Unlock()
		e.reportPriceUnavailable(now, symbol, "expiry_settlement", fmt.Errorf("expiry settlement: %w", err))
		if log != nil {
			log.LogEvent(now, 0, "expiry_settlement_pending", ExpirySettlementPendingEvent{
				Timestamp: now, Symbol: symbol, State: string(expiryStateSettlementPending),
				Policy: pending.Policy, Attempts: pending.Attempts,
				ExpiryReachedAt: pending.ExpiryReachedAt, Reason: pending.LastReason,
			})
		}
		return
	}
	quote := inst.QuoteAsset()
	precision := inst.BasePrecision()
	margined, isMargined := inst.(Margined)
	exactStore, hasExactAccounting := e.Positions.(etypes.ExactLinearPositionStore)
	if e.requireExactLinearAccounting && isMargined && !hasExactAccounting {
		panic("exchange: exact linear position store required for expiry")
	}

	type expiringPos struct {
		clientID uint64
		pos      Position
	}
	var positions []expiringPos
	e.Positions.PositionsForFunding(symbol, func(clientID uint64, pos Position) {
		positions = append(positions, expiringPos{clientID: clientID, pos: pos})
	})
	_, hasMarginLedger := e.Positions.(etypes.MarginLedger)

	var expectedRounding []PositionAccountingRounding
	if e.requireExactLinearAccounting && isMargined {
		var valid bool
		expectedRounding, valid = exactStore.PreviewPositionAccountingTerminalization(symbol, settlementPrice, precision)
		if !valid {
			panic("exchange: exact linear expiry terminalization unavailable")
		}
		simulatedBalances := make(map[uint64]int64)
		simulatedFeeRevenue := e.ExchangeBalance.FeeRevenue[quote]
		var simulatedFeeTotal int64
		for _, ep := range positions {
			client := e.Clients[ep.clientID]
			if client == nil {
				panic("exchange: expiry settlement recipient is unavailable")
			}
			simulatedBalance, seen := simulatedBalances[ep.clientID]
			if !seen {
				simulatedBalance = client.PerpBalances[quote]
			}
			release := ep.pos.Margin
			if !hasMarginLedger {
				var releaseErr error
				release, releaseErr = margined.MarginRequired(abs(ep.pos.Size), ep.pos.EntryPrice, precision)
				if releaseErr != nil {
					panic(fmt.Sprintf("expiry margin release %s: %v", symbol, releaseErr))
				}
			}
			if release > 0 {
				var ok bool
				simulatedBalance, ok = etypes.TryAdd(simulatedBalance, release)
				if !ok {
					panic("exchange: expiry margin release overflows balance")
				}
			}
			cash, cashOK := exactStore.PositionUnrealizedPnL(ep.pos, settlementPrice, precision)
			if !cashOK || !exactStore.CanSettlePositionAtPrice(ep.pos, settlementPrice, precision) {
				panic("exchange: exact linear expiry transition unavailable")
			}
			fee := exp.DeliveryFee(ep.pos.Size, settlementPrice, precision)
			var feeOK bool
			if simulatedFeeTotal, feeOK = etypes.TryAdd(simulatedFeeTotal, fee); !feeOK {
				panic("exchange: expiry delivery fees overflow")
			}
			if simulatedBalance, cashOK = etypes.TryAdd(simulatedBalance, cash); !cashOK {
				panic("exchange: expiry settlement cash overflows balance")
			}
			if simulatedBalance, cashOK = etypes.TrySub(simulatedBalance, fee); !cashOK {
				panic("exchange: expiry delivery fee overflows balance")
			}
			simulatedBalances[ep.clientID] = simulatedBalance
		}
		for _, adjustment := range expectedRounding {
			client := e.Clients[adjustment.ClientID]
			if client == nil {
				panic("exchange: expiry rounding recipient is unavailable")
			}
			simulatedBalance, seen := simulatedBalances[adjustment.ClientID]
			if !seen {
				simulatedBalance = client.PerpBalances[quote]
			}
			var adjustmentOK bool
			if simulatedBalance, adjustmentOK = etypes.TryAdd(simulatedBalance, adjustment.Amount); !adjustmentOK {
				panic("exchange: expiry rounding adjustment overflows balance")
			}
			if simulatedFeeRevenue, adjustmentOK = etypes.TrySub(simulatedFeeRevenue, adjustment.Amount); !adjustmentOK {
				panic("exchange: expiry rounding ledger overflows venue balance")
			}
		}
		if _, ok := etypes.TryAdd(simulatedFeeRevenue, simulatedFeeTotal); !ok {
			panic("exchange: expiry delivery fees overflow venue balance")
		}
	}

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

	ledger, hasLedger := e.Positions.(etypes.MarginLedger)
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
			var marginErr error
			release, marginErr = margined.MarginRequired(absSize, pos.EntryPrice, precision)
			if marginErr != nil {
				panic(fmt.Sprintf("expiry margin release %s: %v", symbol, marginErr))
			}
		}
		if release > 0 {
			client.ReleasePerp(quote, release)
		}

		cash := exp.ExpiryCashFlow(pos.Size, pos.EntryPrice, settlementPrice, precision)
		usedExactAccounting := false
		if isMargined && hasExactAccounting {
			var valid bool
			cash, valid = exactStore.SettlePositionAtPrice(pos, settlementPrice, precision)
			if !valid {
				if e.requireExactLinearAccounting {
					panic("exchange: exact linear expiry settlement unavailable")
				}
				cash = exp.ExpiryCashFlow(pos.Size, pos.EntryPrice, settlementPrice, precision)
			} else {
				usedExactAccounting = true
			}
		}
		fee := exp.DeliveryFee(pos.Size, settlementPrice, precision)
		oldBal := client.PerpBalances[quote]
		netCash, ok := etypes.TrySub(cash, fee)
		if !ok {
			panic("expiry settlement cash overflows balance")
		}
		newBal := etypes.AddAmount(oldBal, netCash)
		client.PerpBalances[quote] = newBal
		feeTotal = etypes.AddAmount(feeTotal, fee)

		closeSide := Sell
		if pos.Size < 0 {
			closeSide = Buy
		}
		if !usedExactAccounting {
			e.Positions.UpdatePosition(ep.clientID, symbol, absSize, settlementPrice, closeSide, pos.PositionSide)
		}

		if log != nil {
			log.LogEvent(now, ep.clientID, "expiry_settlement", ExpirySettlementEvent{
				Timestamp: now, ClientID: ep.clientID, Symbol: symbol,
				PositionSide:  pos.PositionSide.String(),
				BasePrecision: precision,
				Size:          pos.Size, EntryPrice: pos.EntryPrice,
				SettlementPrice: settlementPrice, CashFlow: cash, DeliveryFee: fee,
			})
			log.LogEvent(now, ep.clientID, "balance_change", BalanceChangeEvent{
				Timestamp: now, ClientID: ep.clientID, Symbol: symbol,
				PositionSide: pos.PositionSide.String(), Reason: "expiry_settlement",
				Changes: []BalanceDelta{{Asset: quote, Wallet: "perp", OldBalance: oldBal, NewBalance: newBal, Delta: netCash}},
			})
		}
	}

	if isMargined && hasExactAccounting {
		var rounding []PositionAccountingRounding
		var valid bool
		if e.requireExactLinearAccounting {
			rounding, valid = exactStore.CommitPositionAccountingCarry(symbol, precision, expectedRounding)
		} else {
			rounding, valid = exactStore.DrainPositionAccountingCarry(symbol, precision)
		}
		if !valid {
			if e.requireExactLinearAccounting {
				panic("exchange: exact linear expiry rounding drain unavailable")
			}
		} else {
			for _, adjustment := range rounding {
				client := e.Clients[adjustment.ClientID]
				if client == nil {
					if e.requireExactLinearAccounting {
						panic("exchange: expiry rounding recipient is unavailable")
					}
					continue
				}
				if adjustment.Amount != 0 {
					oldBalance := client.PerpBalances[quote]
					newBalance := etypes.AddAmount(oldBalance, adjustment.Amount)
					client.PerpBalances[quote] = newBalance
					logBalanceChange(e, now, adjustment.ClientID, symbol, "position_rounding", []BalanceDelta{
						{Asset: quote, Wallet: "perp", OldBalance: oldBalance, NewBalance: newBalance, Delta: adjustment.Amount},
					})
					e.moveVenueBalance(VenueFeeRevenue, quote, -adjustment.Amount, now, symbol, "position_rounding")
				}
				if log != nil {
					log.LogEvent(now, adjustment.ClientID, "position_rounding", PositionRoundingEvent{
						Timestamp: now, ClientID: adjustment.ClientID, Symbol: symbol,
						Asset:          quote,
						CashAdjustment: adjustment.Amount, RemainderNumerator: adjustment.RemainderNumerator,
						Precision: precision,
					})
				}
			}
		}
	}

	if feeTotal > 0 {
		e.recordFeeRevenue(quote, Fee{Amount: feeTotal, Asset: quote}, Fee{}, book, now)
	}
	if isMargined {
		if releaser, ok := e.Positions.(etypes.PositionPrecisionReleaser); ok {
			releaser.ClearPositionPrecision(symbol)
		}
	}

	listedAt, hasListedAt := e.instrumentListedAt[symbol]
	delete(e.Books, symbol)
	e.bookGeneration++
	delete(e.Instruments, symbol)
	delete(e.instrumentListedAt, symbol)
	delete(e.settlementPending, symbol)
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

	var listedAtEvidence *int64
	if hasListedAt {
		listedAtEvidence = &listedAt
	}
	ann := describeInstrument(inst, "settled", now, listedAtEvidence)
	ann.SettlementPrice = &settlementPrice
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
		listedAt, hasListedAt := e.instrumentListedAt[symbol]
		var listedAtEvidence *int64
		if hasListedAt {
			listedAtEvidence = &listedAt
		}
		message := &etypes.MarketDataMsg{
			Type:      MDInstrument,
			Symbol:    etypes.InstrumentFeedSymbol,
			Timestamp: now,
			Data:      describeInstrument(instrument, "listed", now, listedAtEvidence),
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

func describeInstrument(inst Instrument, action string, now int64, listedAt *int64) *etypes.InstrumentAnnouncement {
	ann := &etypes.InstrumentAnnouncement{
		Action:         action,
		Symbol:         inst.Symbol(),
		InstrumentType: inst.InstrumentType(),
		QuoteAsset:     inst.QuoteAsset(),
		BasePrecision:  inst.BasePrecision(),
		TickSize:       inst.TickSize(),
		MinOrderSize:   inst.MinOrderSize(),
		Timestamp:      now,
	}
	if ref, ok := inst.(etypes.UnderlyingRef); ok {
		ann.Underlying = ref.UnderlyingSymbol()
	}
	if exp, ok := inst.(Expirable); ok {
		ann.ExpiryNano = exp.ExpiryNano()
		ann.ListedNano = listedAt
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
		listedAt, hasListedAt := e.instrumentListedAt[inst.Symbol()]
		var listedAtEvidence *int64
		if hasListedAt {
			listedAtEvidence = &listedAt
		}
		instruments = append(instruments, describeInstrument(inst, "listed", now, listedAtEvidence))
	}
	reqID := uint64(0)
	if req != nil {
		reqID = req.RequestID
	}
	return Response{RequestID: reqID, Success: true, Data: instruments}
}
