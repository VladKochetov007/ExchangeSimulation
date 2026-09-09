package exchange

import (
	"errors"
	"fmt"
	"math"
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

type expiringPosition struct {
	clientID uint64
	pos      Position
}

type optionExpiryPositionPlan struct {
	clientID uint64
	pos      Position
	cash     int64
	fee      int64
	netCash  int64
}

type optionExpiryPlan struct {
	positions          []optionExpiryPositionPlan
	netSize            int64
	grossCashFlow      int64
	expectedCashFlow   int64
	roundingResidual   int64
	venueRoundingDelta int64
	deliveryFeeTotal   int64
}

type expirySettlementNotice struct {
	symbol          string
	now             int64
	inst            Instrument
	settlementPrice int64
	listedAt        int64
	hasListedAt     bool
	pending         *expirySettlementPending
	unavailable     error
}

// safeExpiryCashFlow adapts the legacy Expirable arithmetic contract to the
// exchange's fail-closed lifecycle. Existing custom instruments expose an
// unchecked method, so a malformed or unrepresentable calculation must become
// a settlement deferral rather than a process-wide panic.
func safeExpiryCashFlow(exp Expirable, size, entryPrice, settlementPrice, basePrecision int64) (cash int64, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("expiry cash-flow calculation panicked: %v", recovered)
			cash = 0
		}
	}()
	return exp.ExpiryCashFlow(size, entryPrice, settlementPrice, basePrecision), nil
}

// safeDeliveryFee applies the non-negative fee contract while containing
// unchecked legacy implementations. A negative fee would create an implicit
// venue subsidy and is therefore an invalid settlement input.
func safeDeliveryFee(exp Expirable, size, settlementPrice, basePrecision int64) (fee int64, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("delivery-fee calculation panicked: %v", recovered)
			fee = 0
		}
	}()
	fee = exp.DeliveryFee(size, settlementPrice, basePrecision)
	if fee < 0 {
		return 0, fmt.Errorf("delivery fee is negative: %d", fee)
	}
	return fee, nil
}

// deferExpiredSettlementLocked is the common fail-closed lifecycle transition
// for unavailable or economically unresolved expiry settlement. It must be
// called with e.mu held. Reporting happens only after the caller releases the
// exchange lock, which lets a same-expiry cohort publish a deterministic batch
// of lifecycle notices without exposing an intermediate economic state.
func (e *DefaultExchange) deferExpiredSettlementLocked(symbol string, now int64, inst Instrument, book *OrderBook, reason string, unavailable error) expirySettlementNotice {
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
			client := e.Clients[clientID]
			if e.clientHasOpenPositionOnSymbolLocked(clientID, symbol) {
				e.cancelClientOrdersAcrossBooksLocked(client)
				continue
			}
			// Orders on the expiring book are invalid for every client, even
			// when that client has no retained position in the contract.
			e.cancelClientOrdersOnBook(client, book, inst)
		}
	}
	pending.Attempts++
	if reason == "" && unavailable != nil {
		reason = unavailable.Error()
	}
	if reason == "" {
		reason = "settlement unresolved"
	}
	pending.LastReason = reason
	e.settlementPending[symbol] = pending
	if perp := marginCore(inst); perp != nil {
		perp.ClearMarkReferences()
	}
	if opt, ok := inst.(*einstrument.EuropeanOption); ok {
		opt.ClearMarks()
	}
	pendingCopy := pending
	return expirySettlementNotice{
		symbol: symbol, now: now, pending: &pendingCopy, unavailable: unavailable,
	}
}

func (e *DefaultExchange) publishExpiryNotice(notice expirySettlementNotice) {
	if notice.pending != nil {
		if notice.unavailable != nil {
			e.reportPriceUnavailable(notice.now, notice.symbol, "expiry_settlement", notice.unavailable)
		}
		if log := e.getLogger(notice.symbol); log != nil {
			log.LogEvent(notice.now, 0, "expiry_settlement_pending", ExpirySettlementPendingEvent{
				Timestamp: notice.now, Symbol: notice.symbol, State: string(expiryStateSettlementPending),
				Policy: notice.pending.Policy, Attempts: notice.pending.Attempts,
				ExpiryReachedAt: notice.pending.ExpiryReachedAt, Reason: notice.pending.LastReason,
			})
		}
		return
	}
	if notice.inst == nil {
		return
	}
	var listedAtEvidence *int64
	if notice.hasListedAt {
		listedAtEvidence = &notice.listedAt
	}
	ann := describeInstrument(notice.inst, "settled", notice.now, listedAtEvidence)
	ann.SettlementPrice = &notice.settlementPrice
	e.MDPublisher.Publish(etypes.InstrumentFeedSymbol, MDInstrument, ann, notice.now)
	if glog := e.getLogger("_global"); glog != nil {
		glog.LogEvent(notice.now, 0, "instrument_settled", ann)
	}
}

// previewOptionExpiryLocked validates every option expiry input and resulting
// cash balance before the book, positions, or venue ledger are mutated. Options
// are one net contract in the closed simulation; a nonzero residual position is
// therefore unresolved rather than an implicit external counterparty.
func (e *DefaultExchange) previewOptionExpiryLocked(option *einstrument.EuropeanOption, settlementPrice int64, positions []expiringPosition, enforceBalance bool) (optionExpiryPlan, error) {
	plan := optionExpiryPlan{positions: make([]optionExpiryPositionPlan, 0, len(positions))}
	quote := option.QuoteAsset()
	precision := option.BasePrecision()
	simulatedBalances := make(map[uint64]int64)
	for _, expiring := range positions {
		if expiring.pos.Size == 0 {
			continue
		}
		client := e.Clients[expiring.clientID]
		if client == nil {
			return optionExpiryPlan{}, fmt.Errorf("option %s settlement recipient %d is unavailable", option.Symbol(), expiring.clientID)
		}
		if expiring.pos.Size == math.MinInt64 {
			return optionExpiryPlan{}, fmt.Errorf("option %s position size is not representable", option.Symbol())
		}
		cash, ok := option.TryExpiryCashFlow(expiring.pos.Size, expiring.pos.EntryPrice, settlementPrice, precision)
		if !ok {
			return optionExpiryPlan{}, fmt.Errorf("option %s expiry cash flow overflows", option.Symbol())
		}
		fee, ok := option.TryDeliveryFee(expiring.pos.Size, settlementPrice, precision)
		if !ok {
			return optionExpiryPlan{}, fmt.Errorf("option %s delivery fee overflows", option.Symbol())
		}
		netCash, ok := etypes.TrySub(cash, fee)
		if !ok {
			return optionExpiryPlan{}, fmt.Errorf("option %s net settlement cash overflows", option.Symbol())
		}
		plan.netSize, ok = etypes.TryAdd(plan.netSize, expiring.pos.Size)
		if !ok {
			return optionExpiryPlan{}, fmt.Errorf("option %s aggregate position size overflows", option.Symbol())
		}
		plan.grossCashFlow, ok = etypes.TryAdd(plan.grossCashFlow, cash)
		if !ok {
			return optionExpiryPlan{}, fmt.Errorf("option %s aggregate cash flow overflows", option.Symbol())
		}
		plan.deliveryFeeTotal, ok = etypes.TryAdd(plan.deliveryFeeTotal, fee)
		if !ok {
			return optionExpiryPlan{}, fmt.Errorf("option %s delivery fees overflow", option.Symbol())
		}
		simulatedBalance, seen := simulatedBalances[expiring.clientID]
		if !seen {
			simulatedBalance = client.PerpBalances[quote]
		}
		simulatedBalance, ok = etypes.TryAdd(simulatedBalance, netCash)
		if !ok {
			return optionExpiryPlan{}, fmt.Errorf("option %s client %d balance overflows", option.Symbol(), expiring.clientID)
		}
		simulatedBalances[expiring.clientID] = simulatedBalance
		plan.positions = append(plan.positions, optionExpiryPositionPlan{
			clientID: expiring.clientID, pos: expiring.pos, cash: cash, fee: fee, netCash: netCash,
		})
	}
	if plan.netSize != 0 {
		return optionExpiryPlan{}, fmt.Errorf("option %s has unmatched net position size %d", option.Symbol(), plan.netSize)
	}
	clientIDs := make([]uint64, 0, len(simulatedBalances))
	for clientID := range simulatedBalances {
		clientIDs = append(clientIDs, clientID)
	}
	slices.Sort(clientIDs)
	if enforceBalance {
		for _, clientID := range clientIDs {
			if simulatedBalances[clientID] < 0 {
				return optionExpiryPlan{}, fmt.Errorf("option %s client %d expiry settlement would create negative perp balance %d", option.Symbol(), clientID, simulatedBalances[clientID])
			}
		}
	}
	var ok bool
	plan.expectedCashFlow, ok = option.TryExpiryCashFlow(plan.netSize, 0, settlementPrice, precision)
	if !ok {
		return optionExpiryPlan{}, fmt.Errorf("option %s net expiry cash flow overflows", option.Symbol())
	}
	plan.roundingResidual, ok = etypes.TrySub(plan.grossCashFlow, plan.expectedCashFlow)
	if !ok {
		return optionExpiryPlan{}, fmt.Errorf("option %s expiry rounding residual overflows", option.Symbol())
	}
	plan.venueRoundingDelta, ok = etypes.TrySub(0, plan.roundingResidual)
	if !ok {
		return optionExpiryPlan{}, fmt.Errorf("option %s expiry rounding ledger delta overflows", option.Symbol())
	}
	simulatedRevenue := e.ExchangeBalance.FeeRevenue[quote]
	if simulatedRevenue, ok = etypes.TryAdd(simulatedRevenue, plan.venueRoundingDelta); !ok {
		return optionExpiryPlan{}, fmt.Errorf("option %s expiry rounding ledger overflows", option.Symbol())
	}
	if _, ok = etypes.TryAdd(simulatedRevenue, plan.deliveryFeeTotal); !ok {
		return optionExpiryPlan{}, fmt.Errorf("option %s delivery fee ledger overflows", option.Symbol())
	}
	return plan, nil
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
// seller margin formula. It returns the completed option-mark epoch, or zero
// when no option received a usable mark in this pass.
func (e *DefaultExchange) UpdateDerivativeMarks() uint64 {
	e.markPassMu.Lock()
	defer e.markPassMu.Unlock()

	return e.updateDerivativeMarksLockedByPass()
}

// updateDerivativeMarksLockedByPass is the derivative lifecycle body. The
// caller must hold markPassMu so its optional complete mark refresh cannot
// interleave with another mark producer.
func (e *DefaultExchange) updateDerivativeMarksLockedByPass() uint64 {
	now := e.Clock.NowUnixNano()

	type expirableData struct {
		symbol string
		inst   Instrument
		book   *OrderBook
	}
	e.mu.RLock()
	lastMarkPassTimestamp := e.lastMarkPassTimestamp
	lastMarkPassEpoch := e.lastMarkPassEpoch
	expirables := make([]expirableData, 0)
	for symbol, inst := range e.Instruments {
		if _, ok := inst.(Expirable); ok {
			expirables = append(expirables, expirableData{symbol: symbol, inst: inst, book: e.Books[symbol]})
		}
	}
	e.mu.RUnlock()
	slices.SortFunc(expirables, func(a, b expirableData) int {
		return strings.Compare(a.inst.Symbol(), b.inst.Symbol())
	})
	markedOptionSymbols := make([]string, 0, len(expirables))

	for _, data := range expirables {
		inst := data.inst
		underlyingPrice, err := e.derivativeUnderlyingPrice(inst)
		if err != nil {
			// No valid underlying reference: defer settlement sampling and option
			// marks rather than inventing a zero price.
			e.mu.Lock()
			if liveBook := e.Books[data.symbol]; liveBook == data.book && e.Instruments[data.symbol] == inst {
				if opt, ok := inst.(*einstrument.EuropeanOption); ok {
					opt.ClearMarks()
				}
				delete(e.markEpochBySymbol, data.symbol)
				delete(e.riskMarkSnapshots, data.symbol)
			}
			e.mu.Unlock()
			e.reportPriceUnavailable(now, inst.Symbol(), "derivative_mark", err)
			continue
		}
		// Revalidate and commit the observation under the exchange lock. Expiry
		// can remove an instrument after the unlocked price lookup; without this
		// identity check a stale snapshot could publish a mark into a contract that
		// has already entered settlement or been delisted.
		e.mu.Lock()
		liveBook := e.Books[data.symbol]
		if liveBook == nil || liveBook != data.book {
			e.mu.Unlock()
			continue
		}
		inst.(Expirable).ObserveSettlement(underlyingPrice, now)
		if _, pending := e.settlementPending[data.symbol]; !pending {
			if opt, ok := inst.(*einstrument.EuropeanOption); ok {
				yearsLeft := float64(opt.ExpiryNano()-now) / float64(365*24*time.Hour)
				mark := eprice.Black76Premium(underlyingPrice, opt.Strike, opt.IV, yearsLeft, opt.IsCall)
				currentSnapshot, current := e.riskMarkSnapshots[data.symbol]
				// Same-timestamp reuse is valid only when the complete set of
				// risk inputs is unchanged. A timestamp is not a source version:
				// a book or injected option parameter may change without the
				// simulation clock advancing. If an input changed, install the
				// candidate and request a fresh complete mark epoch below.
				sameMarkInputs := e.lastMarkPassTimestamp == now && e.lastMarkPassEpoch != 0 && current &&
					currentSnapshot.epoch == e.lastMarkPassEpoch && currentSnapshot.timestamp == now &&
					currentSnapshot.underlying == underlyingPrice && currentSnapshot.mark == mark &&
					currentSnapshot.maintenanceBps == opt.Margin.MMBps
				if sameMarkInputs {
					e.mu.Unlock()
					continue
				}
				opt.SetMarks(underlyingPrice, mark)
				markedOptionSymbols = append(markedOptionSymbols, data.symbol)
			}
		}
		e.mu.Unlock()
	}
	var completedMarkEpoch uint64
	if lastMarkPassTimestamp == now && lastMarkPassEpoch != 0 {
		completedMarkEpoch = lastMarkPassEpoch
	}
	needsCompleteMarginRefresh := false
	if len(markedOptionSymbols) > 0 {
		e.mu.Lock()
		reuseMarkPass := e.lastMarkPassTimestamp == now && e.lastMarkPassEpoch != 0
		if reuseMarkPass {
			completedMarkEpoch = e.lastMarkPassEpoch
		} else {
			completedMarkEpoch = e.markEpoch + 1
			e.markEpoch = completedMarkEpoch
		}
		for _, symbol := range markedOptionSymbols {
			if _, pending := e.settlementPending[symbol]; pending {
				delete(e.riskMarkSnapshots, symbol)
				continue
			}
			if _, live := e.Books[symbol]; live {
				e.markEpochBySymbol[symbol] = completedMarkEpoch
				option, ok := e.Instruments[symbol].(*einstrument.EuropeanOption)
				if !ok {
					continue
				}
				mark, err := option.PositionMark()
				if err != nil {
					delete(e.markEpochBySymbol, symbol)
					continue
				}
				underlying, err := option.UnderlyingMark()
				if err != nil {
					delete(e.markEpochBySymbol, symbol)
					continue
				}
				e.riskMarkSnapshots[symbol] = riskMarkSnapshot{
					book: e.Books[symbol], mark: mark, underlying: underlying,
					maintenanceBps: option.Margin.MMBps,
					epoch:          completedMarkEpoch, timestamp: now,
				}
			}
		}
		// An option lifecycle pass can otherwise create an epoch containing only
		// options. If an account also holds a futures-style position, the next
		// strict profile would combine a fresh option mark with an older sibling
		// mark and fail or depend on which automation callback ran first. Request
		// one complete mark pass at this timestamp whenever the option inputs
		// changed. This is required even without mixed exposure: a timestamp is
		// not a source-generation token, and a changed input must not share an
		// old epoch with untouched sibling marks. Unavailable siblings remain
		// unavailable and therefore fail closed.
		needsCompleteMarginRefresh = reuseMarkPass || e.hasMixedMarginExposureLocked(now)
		e.mu.Unlock()
	}
	if needsCompleteMarginRefresh {
		e.updateAllPerpPricesLockedByPass()
		e.mu.RLock()
		completedMarkEpoch = e.markEpoch
		e.mu.RUnlock()
	}
	e.publishIndexFeeds(now)
	if e.postDerivativeMarkHook != nil {
		// The hook sees a complete fresh mark set and precedes any same-timestamp
		// expiry. It must remain read-only because it runs outside e.mu.
		e.postDerivativeMarkHook()
	}
	return completedMarkEpoch
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
	type cohortKey struct {
		expiry int64
		quote  string
	}
	type expiryCohort struct {
		expiry  int64
		quote   string
		symbols []string
	}
	expiredByCohort := make(map[cohortKey]*expiryCohort)
	firstExpiry := false
	for symbol, inst := range e.Instruments {
		if exp, ok := inst.(Expirable); ok && now >= exp.ExpiryNano() {
			key := cohortKey{expiry: exp.ExpiryNano(), quote: inst.QuoteAsset()}
			cohort := expiredByCohort[key]
			if cohort == nil {
				cohort = &expiryCohort{expiry: key.expiry, quote: key.quote}
				expiredByCohort[key] = cohort
			}
			cohort.symbols = append(cohort.symbols, symbol)
			if _, pending := e.settlementPending[symbol]; !pending {
				firstExpiry = true
			}
		}
	}
	e.mu.RUnlock()

	// Settlement cancels orders and emits events, so map iteration here would
	// make same-timestamp expiries observably nondeterministic. Cohorts are
	// ordered by contractual expiry and quote wallet; symbols within a cohort
	// are canonical only for evidence ordering, never for solvency.
	cohorts := make([]*expiryCohort, 0, len(expiredByCohort))
	for _, cohort := range expiredByCohort {
		slices.Sort(cohort.symbols)
		cohorts = append(cohorts, cohort)
	}
	sort.Slice(cohorts, func(i, j int) bool {
		if cohorts[i].expiry != cohorts[j].expiry {
			return cohorts[i].expiry < cohorts[j].expiry
		}
		return cohorts[i].quote < cohorts[j].quote
	})
	if firstExpiry && e.preExpiryHook != nil {
		// This is deliberately outside e.mu: a strict account snapshot acquires
		// the read lock and must observe the fully marked, still-listed board.
		// The hook contract is read-only, so no exchange state changes between
		// the expiry set being identified and contractual settlement below.
		e.preExpiryHook()
	}
	for _, cohort := range cohorts {
		e.settleExpiredCohort(cohort.symbols, now)
	}
}

// preflightExpiryCohortLocked validates the aggregate cash and venue-ledger
// transition for one (expiry, quote asset) cohort. Individual contracts are
// not allowed to reject on an intermediate wallet balance when another
// same-wallet contract supplies the offsetting cash flow.
func (e *DefaultExchange) preflightExpiryCohortLocked(symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}
	firstBook := e.Books[symbols[0]]
	if firstBook == nil || e.ExchangeBalance == nil {
		return fmt.Errorf("expiry cohort settlement state is unavailable")
	}
	firstExp, firstExpirable := firstBook.Instrument.(Expirable)
	if !firstExpirable {
		return fmt.Errorf("expiry cohort first instrument is not expirable")
	}
	quote := firstBook.Instrument.QuoteAsset()
	expectedExpiry := firstExp.ExpiryNano()
	balances := make(map[uint64]int64)
	venueRevenue := e.ExchangeBalance.FeeRevenue[quote]
	checkBalances := false
	addBalance := func(clientID uint64, delta int64) error {
		client := e.Clients[clientID]
		if client == nil {
			return fmt.Errorf("expiry cohort settlement recipient %d is unavailable", clientID)
		}
		balance, seen := balances[clientID]
		if !seen {
			balance = client.PerpBalances[quote]
		}
		var ok bool
		balance, ok = etypes.TryAdd(balance, delta)
		if !ok {
			return fmt.Errorf("expiry cohort client %d balance overflows", clientID)
		}
		balances[clientID] = balance
		return nil
	}
	addVenueDelta := func(delta int64) error {
		var ok bool
		venueRevenue, ok = etypes.TryAdd(venueRevenue, delta)
		if !ok {
			return fmt.Errorf("expiry cohort venue fee ledger overflows")
		}
		return nil
	}

	for _, symbol := range symbols {
		book := e.Books[symbol]
		if book == nil {
			return fmt.Errorf("expiry cohort book %s is unavailable", symbol)
		}
		inst := book.Instrument
		exp, ok := inst.(Expirable)
		if !ok {
			return fmt.Errorf("expiry cohort instrument %s is not expirable", symbol)
		}
		if inst.QuoteAsset() != quote || exp.ExpiryNano() != expectedExpiry {
			return fmt.Errorf("expiry cohort %s has inconsistent expiry or quote", symbol)
		}
		settlementPrice, err := exp.SettlementPrice()
		if err != nil {
			return fmt.Errorf("expiry cohort %s settlement: %w", symbol, err)
		}
		var positions []expiringPosition
		e.Positions.PositionsForFunding(symbol, func(clientID uint64, pos Position) {
			positions = append(positions, expiringPosition{clientID: clientID, pos: pos})
		})
		sort.Slice(positions, func(i, j int) bool {
			if positions[i].clientID != positions[j].clientID {
				return positions[i].clientID < positions[j].clientID
			}
			return positions[i].pos.PositionSide < positions[j].pos.PositionSide
		})

		if option, isOption := inst.(*einstrument.EuropeanOption); isOption {
			plan, err := e.previewOptionExpiryLocked(option, settlementPrice, positions, false)
			if err != nil {
				return fmt.Errorf("expiry cohort %s option preview: %w", symbol, err)
			}
			checkBalances = true
			for _, position := range plan.positions {
				if err := addBalance(position.clientID, position.netCash); err != nil {
					return err
				}
			}
			venueDelta, ok := etypes.TryAdd(plan.venueRoundingDelta, plan.deliveryFeeTotal)
			if !ok {
				return fmt.Errorf("expiry cohort %s venue delta overflows", symbol)
			}
			if err := addVenueDelta(venueDelta); err != nil {
				return err
			}
			continue
		}

		_, isMargined := inst.(Margined)
		exactStore, hasExactAccounting := e.Positions.(etypes.ExactLinearPositionStore)
		if e.requireExactLinearAccounting && isMargined && !hasExactAccounting {
			return fmt.Errorf("expiry cohort %s requires exact linear accounting", symbol)
		}
		useExactAccounting := isMargined && hasExactAccounting
		var expectedRounding []PositionAccountingRounding
		if useExactAccounting {
			var valid bool
			expectedRounding, valid = exactStore.PreviewPositionAccountingTerminalization(symbol, settlementPrice, inst.BasePrecision())
			if !valid {
				return fmt.Errorf("expiry cohort %s exact terminalization is unavailable", symbol)
			}
			checkBalances = checkBalances || e.requireExactLinearAccounting
		}
		var feeTotal int64
		for _, position := range positions {
			client := e.Clients[position.clientID]
			if client == nil {
				return fmt.Errorf("expiry cohort settlement recipient %d is unavailable", position.clientID)
			}
			var cash int64
			if useExactAccounting {
				var valid bool
				cash, valid = exactStore.PositionUnrealizedPnL(position.pos, settlementPrice, inst.BasePrecision())
				if !valid || !exactStore.CanSettlePositionAtPrice(position.pos, settlementPrice, inst.BasePrecision()) {
					return fmt.Errorf("expiry cohort %s exact position transition is unavailable", symbol)
				}
			} else {
				var cashErr error
				cash, cashErr = safeExpiryCashFlow(exp, position.pos.Size, position.pos.EntryPrice, settlementPrice, inst.BasePrecision())
				if cashErr != nil {
					return fmt.Errorf("expiry cohort %s cash flow: %w", symbol, cashErr)
				}
			}
			fee, feeErr := safeDeliveryFee(exp, position.pos.Size, settlementPrice, inst.BasePrecision())
			if feeErr != nil {
				return fmt.Errorf("expiry cohort %s delivery fee: %w", symbol, feeErr)
			}
			netCash, valid := etypes.TrySub(cash, fee)
			if !valid {
				return fmt.Errorf("expiry cohort %s net cash overflows", symbol)
			}
			if err := addBalance(position.clientID, netCash); err != nil {
				return err
			}
			feeTotal, valid = etypes.TryAdd(feeTotal, fee)
			if !valid {
				return fmt.Errorf("expiry cohort %s delivery fees overflow", symbol)
			}
		}
		for _, adjustment := range expectedRounding {
			if err := addBalance(adjustment.ClientID, adjustment.Amount); err != nil {
				return err
			}
			venueAdjustment, ok := etypes.TrySub(0, adjustment.Amount)
			if !ok {
				return fmt.Errorf("expiry cohort %s rounding venue delta overflows", symbol)
			}
			if err := addVenueDelta(venueAdjustment); err != nil {
				return err
			}
		}
		if err := addVenueDelta(feeTotal); err != nil {
			return err
		}
	}

	if checkBalances {
		clientIDs := make([]uint64, 0, len(balances))
		for clientID := range balances {
			clientIDs = append(clientIDs, clientID)
		}
		slices.Sort(clientIDs)
		for _, clientID := range clientIDs {
			if balances[clientID] < 0 {
				return fmt.Errorf("expiry cohort client %d settlement would create negative perp balance %d", clientID, balances[clientID])
			}
		}
	}
	return nil
}

func (e *DefaultExchange) deferExpiredSettlementCohortLocked(symbols []string, now int64, reason error) []expirySettlementNotice {
	notices := make([]expirySettlementNotice, 0, len(symbols))
	var unavailable error
	if isPriceUnavailable(reason) {
		// Preserve the per-contract price-unavailable diagnostic when the
		// cohort preflight failed because a declared settlement observation is
		// absent. Arithmetic and contract-shape failures remain lifecycle
		// deferrals without being mislabeled as missing market data.
		unavailable = reason
	}
	for _, symbol := range symbols {
		book := e.Books[symbol]
		if book == nil {
			continue
		}
		inst := book.Instrument
		notice := e.deferExpiredSettlementLocked(symbol, now, inst, book,
			fmt.Sprintf("cohort settlement deferred: %v", reason), unavailable)
		notices = append(notices, notice)
	}
	return notices
}

func (e *DefaultExchange) settleExpiredCohort(symbols []string, now int64) {
	e.mu.Lock()
	notices := make([]expirySettlementNotice, 0, len(symbols))
	if err := e.preflightExpiryCohortLocked(symbols); err != nil {
		notices = e.deferExpiredSettlementCohortLocked(symbols, now, err)
	} else {
		for _, symbol := range symbols {
			notice := e.settleExpiredInstrumentLocked(symbol, now, false)
			if notice.pending != nil {
				panic(fmt.Sprintf("expiry cohort %s changed after preflight", symbol))
			}
			notices = append(notices, notice)
		}
	}
	e.mu.Unlock()
	for _, notice := range notices {
		e.publishExpiryNotice(notice)
	}
}

// ExpirySettlementEvent is logged per position at expiry.
type ExpirySettlementEvent struct {
	Timestamp       int64  `json:"timestamp"`
	ClientID        uint64 `json:"client_id"`
	Symbol          string `json:"symbol"`
	QuoteAsset      string `json:"quote_asset"`
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
	notice := e.settleExpiredInstrumentLocked(symbol, now, true)
	e.mu.Unlock()
	e.publishExpiryNotice(notice)
}

// settleExpiredInstrumentLocked is the mutation half of settlement. The
// caller holds e.mu for the whole operation; cohort settlement uses this to
// commit several same-expiry instruments as one indivisible state transition.
func (e *DefaultExchange) settleExpiredInstrumentLocked(symbol string, now int64, enforceBalance bool) expirySettlementNotice {
	book := e.Books[symbol]
	if book == nil {
		return expirySettlementNotice{}
	}
	inst := book.Instrument
	exp := inst.(Expirable)
	settlementPrice, err := exp.SettlementPrice()
	if err != nil {
		// Contractual expiry blocks new risk immediately. Cancel any resting
		// orders once and retain positions until a declared settlement source
		// becomes available; allowing a post-expiry fill would be worse than a
		// visible lifecycle deferral.
		return e.deferExpiredSettlementLocked(symbol, now, inst, book, err.Error(), fmt.Errorf("expiry settlement: %w", err))
	}
	quote := inst.QuoteAsset()
	precision := inst.BasePrecision()
	margined, isMargined := inst.(Margined)
	exactStore, hasExactAccounting := e.Positions.(etypes.ExactLinearPositionStore)
	if e.requireExactLinearAccounting && isMargined && !hasExactAccounting {
		panic("exchange: exact linear position store required for expiry")
	}

	var positions []expiringPosition
	e.Positions.PositionsForFunding(symbol, func(clientID uint64, pos Position) {
		positions = append(positions, expiringPosition{clientID: clientID, pos: pos})
	})
	sort.Slice(positions, func(i, j int) bool {
		if positions[i].clientID != positions[j].clientID {
			return positions[i].clientID < positions[j].clientID
		}
		return positions[i].pos.PositionSide < positions[j].pos.PositionSide
	})
	var optionPlan *optionExpiryPlan
	if option, ok := inst.(*einstrument.EuropeanOption); ok {
		plan, previewErr := e.previewOptionExpiryLocked(option, settlementPrice, positions, enforceBalance)
		if previewErr != nil {
			return e.deferExpiredSettlementLocked(symbol, now, inst, book, previewErr.Error(), nil)
		}
		optionPlan = &plan
	}

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
			cash, cashOK := exactStore.PositionUnrealizedPnL(ep.pos, settlementPrice, precision)
			if !cashOK || !exactStore.CanSettlePositionAtPrice(ep.pos, settlementPrice, precision) {
				panic("exchange: exact linear expiry transition unavailable")
			}
			fee, feeErr := safeDeliveryFee(exp, ep.pos.Size, settlementPrice, precision)
			if feeErr != nil {
				return e.deferExpiredSettlementLocked(symbol, now, inst, book, feeErr.Error(), nil)
			}
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
			simulatedBalances[adjustment.ClientID] = simulatedBalance
			if simulatedFeeRevenue, adjustmentOK = etypes.TrySub(simulatedFeeRevenue, adjustment.Amount); !adjustmentOK {
				panic("exchange: expiry rounding ledger overflows venue balance")
			}
		}
		if _, ok := etypes.TryAdd(simulatedFeeRevenue, simulatedFeeTotal); !ok {
			panic("exchange: expiry delivery fees overflow venue balance")
		}
		if enforceBalance {
			clientIDs := make([]uint64, 0, len(simulatedBalances))
			for clientID := range simulatedBalances {
				clientIDs = append(clientIDs, clientID)
			}
			slices.Sort(clientIDs)
			for _, clientID := range clientIDs {
				if simulatedBalances[clientID] < 0 {
					return e.deferExpiredSettlementLocked(symbol, now, inst, book,
						fmt.Sprintf("client %d expiry settlement would create negative perp balance %d", clientID, simulatedBalances[clientID]), nil)
				}
			}
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
	_, isCashSettledOption := inst.(*einstrument.EuropeanOption)

	var feeTotal int64
	optionPlanIndex := 0
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

		usedExactAccounting := false
		var cash int64
		if isMargined && hasExactAccounting {
			var valid bool
			cash, valid = exactStore.SettlePositionAtPrice(pos, settlementPrice, precision)
			if !valid {
				if e.requireExactLinearAccounting {
					panic("exchange: exact linear expiry settlement unavailable")
				}
				cash, err = safeExpiryCashFlow(exp, pos.Size, pos.EntryPrice, settlementPrice, precision)
				if err != nil {
					return e.deferExpiredSettlementLocked(symbol, now, inst, book, err.Error(), nil)
				}
			} else {
				usedExactAccounting = true
			}
		} else {
			cash, err = safeExpiryCashFlow(exp, pos.Size, pos.EntryPrice, settlementPrice, precision)
			if err != nil {
				return e.deferExpiredSettlementLocked(symbol, now, inst, book, err.Error(), nil)
			}
		}
		fee, err := safeDeliveryFee(exp, pos.Size, settlementPrice, precision)
		if err != nil {
			return e.deferExpiredSettlementLocked(symbol, now, inst, book, err.Error(), nil)
		}
		if optionPlan != nil {
			planned := optionPlan.positions[optionPlanIndex]
			cash = planned.cash
			fee = planned.fee
			optionPlanIndex++
		}
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

		settlementChanges := []BalanceDelta{{Asset: quote, Wallet: "perp", OldBalance: oldBal, NewBalance: newBal, Delta: netCash}}
		e.conservation.record(settlementChanges)

		if log != nil {
			log.LogEvent(now, ep.clientID, "expiry_settlement", ExpirySettlementEvent{
				Timestamp: now, ClientID: ep.clientID, Symbol: symbol,
				QuoteAsset:    quote,
				PositionSide:  pos.PositionSide.String(),
				BasePrecision: precision,
				Size:          pos.Size, EntryPrice: pos.EntryPrice,
				SettlementPrice: settlementPrice, CashFlow: cash, DeliveryFee: fee,
			})
			log.LogEvent(now, ep.clientID, "balance_change", BalanceChangeEvent{
				Timestamp: now, ClientID: ep.clientID, Symbol: symbol,
				PositionSide: pos.PositionSide.String(), Reason: "expiry_settlement",
				Changes: settlementChanges,
			})
		}
	}

	if isCashSettledOption {
		// Each position's cash flow is intentionally truncated to quote units,
		// but the economic contract is one option book. Route only the
		// difference between the sum of those posted integers and the integer
		// cash flow for the net position. A nonzero net position therefore
		// remains visible as an unmatched contract instead of being relabelled
		// as rounding.
		if optionPlan.roundingResidual != 0 {
			e.moveVenueBalance(VenueFeeRevenue, quote, optionPlan.venueRoundingDelta, now, symbol, "option_expiry_rounding")
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
	if optionPlan != nil && log != nil {
		log.LogEvent(now, 0, "option_expiry_accounting", OptionExpiryAccountingEvent{
			Timestamp: now, Symbol: symbol, QuoteAsset: quote,
			BasePrecision: precision, SettlementPrice: settlementPrice,
			PositionCount: len(optionPlan.positions), NetPositionSize: optionPlan.netSize,
			GrossCashFlow: optionPlan.grossCashFlow, ExpectedCashFlow: optionPlan.expectedCashFlow,
			RoundingResidual: optionPlan.roundingResidual, VenueRoundingDelta: optionPlan.venueRoundingDelta,
			DeliveryFeeTotal: optionPlan.deliveryFeeTotal,
		})
	}
	if isMargined {
		if releaser, ok := e.Positions.(etypes.PositionPrecisionReleaser); ok {
			releaser.ClearPositionPrecision(symbol)
		}
	}

	listedAt, hasListedAt := e.instrumentListedAt[symbol]
	delete(e.Books, symbol)
	delete(e.Instruments, symbol)
	delete(e.markEpochBySymbol, symbol)
	delete(e.riskMarkSnapshots, symbol)
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
	return expirySettlementNotice{
		symbol: symbol, now: now, inst: inst, settlementPrice: settlementPrice,
		listedAt: listedAt, hasListedAt: hasListedAt,
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
