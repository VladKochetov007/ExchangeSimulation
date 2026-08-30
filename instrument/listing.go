package instrument

import (
	"fmt"
	"strconv"

	etypes "exchange_sim/types"
)

// ContractSpec carries the shared parameters both listers stamp onto every
// contract they create.
type ContractSpec struct {
	Base           string
	Quote          string
	BasePrecision  int64
	QuotePrecision int64
	TickSize       int64
	MinOrderSize   int64
}

// DatedFuturesLister lists dated futures on one underlying. Calendar mode is
// selected when Calendar is non-nil; TenorsNano remains an explicit legacy
// rolling-ladder adapter for historical configurations.
type DatedFuturesLister struct {
	Underlying string // spot symbol, e.g. "ABC/USD"
	Spec       ContractSpec
	// Calendar generates fixed expiry timestamps from deterministic listing
	// schedules. Schedule family names are not part of contract identity.
	Calendar          *ExpiryCalendar
	CalendarEpochNano int64
	// TenorsNano is the compatibility path for historical rolling-ladder runs.
	TenorsNano []int64
	// DeliveryFeeBps is stamped on every listed contract.
	DeliveryFeeBps int64
	// ObservationWindowNano overrides the settlement TWAP window when > 0.
	ObservationWindowNano int64

	listed     map[int64]bool // expiry -> listed
	nextExpiry map[int64]int64
	calendar   calendarCursor
}

func (l *DatedFuturesLister) PendingListings(nowNano int64, _ etypes.ListingPriceSource) ([]etypes.Instrument, error) {
	if l.listed == nil {
		l.listed = make(map[int64]bool)
	}
	if l.Calendar != nil {
		return l.pendingCalendarListings(nowNano)
	}
	if l.nextExpiry == nil {
		l.nextExpiry = make(map[int64]int64)
	}
	var out []etypes.Instrument
	for _, tenor := range l.TenorsNano {
		if tenor <= 0 {
			continue
		}
		expiry, ok := l.expiryForTenor(nowNano, tenor)
		if !ok || l.listed[expiry] {
			continue
		}
		l.listed[expiry] = true
		out = append(out, l.newFuture(expiry))
	}
	return out, nil
}

func (l *DatedFuturesLister) pendingCalendarListings(nowNano int64) ([]etypes.Instrument, error) {
	requests, nextIndex, err := l.calendar.due(l.Calendar, l.CalendarEpochNano, nowNano)
	if err != nil {
		return nil, fmt.Errorf("dated futures calendar: %w", err)
	}
	seenExpiries := make(map[int64]struct{}, len(requests))
	var out []etypes.Instrument
	for _, request := range requests {
		if l.listed[request.expiryNano] {
			continue
		}
		if _, exists := seenExpiries[request.expiryNano]; exists {
			continue
		}
		seenExpiries[request.expiryNano] = struct{}{}
		l.listed[request.expiryNano] = true
		out = append(out, l.newFuture(request.expiryNano))
	}
	// Futures have no price-dependent construction, so every due family cursor
	// advances even when another family requested the same expiry.
	l.calendar.nextIndex = nextIndex
	return out, nil
}

func (l *DatedFuturesLister) newFuture(expiryNano int64) etypes.Instrument {
	symbol := fmt.Sprintf("%s-FUT-%s", l.Spec.Base, expirySymbolLabel(expiryNano))
	future := NewExpiringFutures(symbol, l.Spec.Base, l.Spec.Quote,
		l.Spec.BasePrecision, l.Spec.QuotePrecision, l.Spec.TickSize, l.Spec.MinOrderSize, expiryNano)
	future.Underlying = l.Underlying
	future.DeliveryFeeBps = l.DeliveryFeeBps
	if l.ObservationWindowNano > 0 {
		future.SetObservationWindow(l.ObservationWindowNano)
	}
	return future
}

var _ etypes.ListingPolicy = (*DatedFuturesLister)(nil)

// OptionChainLister lists European option chains. Calendar mode creates one
// fixed chain per expiry; TenorsNano is retained for historical rolling-ladder
// configurations where the chain was allowed to grow as spot moved.
type OptionChainLister struct {
	Underlying        string
	Spec              ContractSpec
	Calendar          *ExpiryCalendar
	CalendarEpochNano int64
	TenorsNano        []int64
	// StrikeStep is the strike grid spacing in quote precision units.
	StrikeStep int64
	// StrikesPerSide counts strikes above and below the central strike.
	StrikesPerSide int
	// MaxStrikesPerExpiry bounds distinct strikes that can remain listed for
	// one expiry. Zero preserves the open-ended listing policy; production
	// long-run scenarios should set a finite cap to avoid unbounded book and
	// risk-sweep growth as a drifting underlying crosses new strike grids.
	MaxStrikesPerExpiry int
	// IV seeds the flat mark volatility for margin/mark purposes.
	IV float64
	// Margin overrides DefaultOptionMarginParams when non-zero.
	Margin OptionMarginParams
	// DeliveryFeeBps is stamped on every listed contract.
	DeliveryFeeBps int64
	// ObservationWindowNano overrides the settlement TWAP window when > 0.
	ObservationWindowNano int64

	listed         map[string]bool
	nextExpiry     map[int64]int64
	strikes        map[int64]map[int64]struct{}
	calendar       calendarCursor
	calendarListed map[int64]bool
}

func (l *OptionChainLister) PendingListings(nowNano int64, prices etypes.ListingPriceSource) ([]etypes.Instrument, error) {
	if l.listed == nil {
		l.listed = make(map[string]bool)
	}
	if l.StrikeStep <= 0 || l.StrikesPerSide < 0 {
		return nil, nil
	}
	if l.Calendar != nil {
		return l.pendingCalendarChains(nowNano, prices)
	}
	if l.nextExpiry == nil {
		l.nextExpiry = make(map[int64]int64)
	}
	if l.strikes == nil {
		l.strikes = make(map[int64]map[int64]struct{})
	}
	spot, err := prices.Price(l.Underlying)
	if err != nil {
		return nil, fmt.Errorf("option-chain listing underlying %s: %w", l.Underlying, err)
	}
	if spot <= 0 {
		// Current chains use strictly-positive strikes and Black-76 marks.
		// A numeric non-positive underlying is therefore present but outside
		// this lister's model domain, not an implicit absence sentinel.
		return nil, fmt.Errorf("option-chain listing underlying %s: %w", l.Underlying, etypes.ErrPriceDomain)
	}
	center := ((spot + l.StrikeStep/2) / l.StrikeStep) * l.StrikeStep

	var out []etypes.Instrument
	for _, tenor := range l.TenorsNano {
		if tenor <= 0 {
			continue
		}
		expiry, ok := l.expiryForTenor(nowNano, tenor)
		if !ok {
			continue
		}
		for i := -l.StrikesPerSide; i <= l.StrikesPerSide; i++ {
			strike := center + int64(i)*l.StrikeStep
			if strike <= 0 || !l.allowStrike(expiry, strike) {
				continue
			}
			for _, isCall := range []bool{true, false} {
				out = l.appendOption(out, strike, expiry, isCall)
			}
		}
	}
	return out, nil
}

func (l *OptionChainLister) pendingCalendarChains(nowNano int64, prices etypes.ListingPriceSource) ([]etypes.Instrument, error) {
	requests, nextIndex, err := l.calendar.due(l.Calendar, l.CalendarEpochNano, nowNano)
	if err != nil {
		return nil, fmt.Errorf("option chain calendar: %w", err)
	}
	if len(requests) == 0 {
		l.calendar.nextIndex = nextIndex
		return nil, nil
	}
	if prices == nil {
		return nil, fmt.Errorf("option-chain listing underlying %s: price source is nil", l.Underlying)
	}
	spot, err := prices.Price(l.Underlying)
	if err != nil {
		return nil, fmt.Errorf("option-chain listing underlying %s: %w", l.Underlying, err)
	}
	if spot <= 0 {
		return nil, fmt.Errorf("option-chain listing underlying %s: %w", l.Underlying, etypes.ErrPriceDomain)
	}
	center := ((spot + l.StrikeStep/2) / l.StrikeStep) * l.StrikeStep
	if l.strikes == nil {
		l.strikes = make(map[int64]map[int64]struct{})
	}
	if l.calendarListed == nil {
		l.calendarListed = make(map[int64]bool)
	}

	seenExpiries := make(map[int64]struct{}, len(requests))
	var out []etypes.Instrument
	var expiries []int64
	for _, request := range requests {
		if l.calendarListed[request.expiryNano] {
			continue
		}
		if _, exists := seenExpiries[request.expiryNano]; exists {
			continue
		}
		seenExpiries[request.expiryNano] = struct{}{}
		expiries = append(expiries, request.expiryNano)
	}
	for _, expiryNano := range expiries {
		for i := -l.StrikesPerSide; i <= l.StrikesPerSide; i++ {
			strike := center + int64(i)*l.StrikeStep
			if strike <= 0 || !l.allowStrike(expiryNano, strike) {
				continue
			}
			for _, isCall := range []bool{true, false} {
				// appendOption is deterministic and idempotent by the full option
				// identity, while calendarListed prevents re-centering this expiry.
				out = l.appendOption(out, strike, expiryNano, isCall)
			}
		}
		l.calendarListed[expiryNano] = true
	}
	// A successfully priced batch commits every schedule cursor, including
	// colliding requests that produced no second instrument.
	l.calendar.nextIndex = nextIndex
	return out, nil
}

func (l *OptionChainLister) allowStrike(expiry, strike int64) bool {
	if l.MaxStrikesPerExpiry <= 0 {
		return true
	}
	byStrike := l.strikes[expiry]
	if byStrike == nil {
		byStrike = make(map[int64]struct{})
		l.strikes[expiry] = byStrike
	}
	if _, exists := byStrike[strike]; exists {
		return true
	}
	if len(byStrike) >= l.MaxStrikesPerExpiry {
		return false
	}
	byStrike[strike] = struct{}{}
	return true
}

// expiryForTenor returns the current rolling expiry for one configured tenor.
// It creates the successor only once the previous generation has expired. A
// duration setting therefore remains the actual contract lifetime even when
// the simulation starts at an arbitrary Unix timestamp.
func (l *DatedFuturesLister) expiryForTenor(nowNano, tenor int64) (int64, bool) {
	if expiry := l.nextExpiry[tenor]; expiry > nowNano {
		return expiry, true
	}
	expiry, ok := addTenor(nowNano, tenor)
	if ok {
		l.nextExpiry[tenor] = expiry
	}
	return expiry, ok
}

func (l *OptionChainLister) expiryForTenor(nowNano, tenor int64) (int64, bool) {
	if expiry := l.nextExpiry[tenor]; expiry > nowNano {
		return expiry, true
	}
	expiry, ok := addTenor(nowNano, tenor)
	if ok {
		l.nextExpiry[tenor] = expiry
	}
	return expiry, ok
}

// addTenor rejects unrepresentable timestamps rather than wrapping a malformed
// listing into the past.
func addTenor(nowNano, tenor int64) (int64, bool) {
	if tenor <= 0 {
		return 0, false
	}
	expiry, ok := etypes.TryAdd(nowNano, tenor)
	return expiry, ok && expiry > nowNano
}

func (l *OptionChainLister) appendOption(out []etypes.Instrument, strike, expiry int64, isCall bool) []etypes.Instrument {
	cp := "P"
	if isCall {
		cp = "C"
	}
	// Whole-quote strikes keep the compact human-readable symbol; strikes off
	// the quote-precision grid use raw units — integer division would collide
	// distinct strikes (e.g. step < precision) into one symbol and silently
	// skip listings.
	strikeLabel := strike / l.Spec.QuotePrecision
	if strike%l.Spec.QuotePrecision != 0 {
		strikeLabel = strike
	}
	symbol := fmt.Sprintf("%s-%s-%d-%s", l.Spec.Base, expirySymbolLabel(expiry), strikeLabel, cp)
	if l.listed[symbol] {
		return out
	}
	l.listed[symbol] = true
	opt := NewEuropeanOption(symbol, l.Spec.Base, l.Spec.Quote, l.Underlying,
		l.Spec.BasePrecision, l.Spec.QuotePrecision, l.Spec.TickSize, l.Spec.MinOrderSize,
		strike, expiry, isCall)
	if l.IV > 0 {
		opt.IV = l.IV
	}
	if l.Margin != (OptionMarginParams{}) {
		opt.Margin = l.Margin
	}
	opt.DeliveryFeeBps = l.DeliveryFeeBps
	if l.ObservationWindowNano > 0 {
		opt.SetObservationWindow(l.ObservationWindowNano)
	}
	return append(out, opt)
}

func expirySymbolLabel(expiryNano int64) string {
	if expiryNano%1_000_000_000 == 0 {
		return strconv.FormatInt(expiryNano/1_000_000_000, 10)
	}
	return strconv.FormatInt(expiryNano, 10) + "ns"
}

var _ etypes.ListingPolicy = (*OptionChainLister)(nil)
