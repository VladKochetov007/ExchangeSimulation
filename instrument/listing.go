package instrument

import (
	"fmt"

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

// DatedFuturesLister maintains a ladder of dated futures on one underlying:
// whenever fewer than len(TenorsNano) unexpired contracts exist, it lists a
// new contract with exactly the configured tenor remaining. A fresh generation
// is created only after the preceding one reaches expiry, so TenorsNano is a
// contract lifetime rather than an epoch-dependent calendar approximation.
type DatedFuturesLister struct {
	Underlying string // spot symbol, e.g. "ABC/USD"
	Spec       ContractSpec
	// TenorsNano lists contract lifetimes to keep on the board (sorted ascending).
	TenorsNano []int64
	// DeliveryFeeBps is stamped on every listed contract.
	DeliveryFeeBps int64
	// ObservationWindowNano overrides the settlement TWAP window when > 0.
	ObservationWindowNano int64

	listed     map[int64]bool // expiry -> listed
	nextExpiry map[int64]int64
}

func (l *DatedFuturesLister) PendingListings(nowNano int64, _ etypes.PriceSource) []etypes.Instrument {
	if l.listed == nil {
		l.listed = make(map[int64]bool)
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
		symbol := fmt.Sprintf("%s-FUT-%d", l.Spec.Base, expiry/1e9)
		f := NewExpiringFutures(symbol, l.Spec.Base, l.Spec.Quote,
			l.Spec.BasePrecision, l.Spec.QuotePrecision, l.Spec.TickSize, l.Spec.MinOrderSize, expiry)
		f.Underlying = l.Underlying
		f.DeliveryFeeBps = l.DeliveryFeeBps
		if l.ObservationWindowNano > 0 {
			f.SetObservationWindow(l.ObservationWindowNano)
		}
		out = append(out, f)
	}
	return out
}

var _ etypes.ListingPolicy = (*DatedFuturesLister)(nil)

// OptionChainLister lists European option chains: for each tenor it creates
// calls and puts at StrikesPerSide strikes either side of the underlying
// price (rounded to StrikeStep), the venue pattern for keeping a chain
// centered while spot moves.
type OptionChainLister struct {
	Underlying string
	Spec       ContractSpec
	TenorsNano []int64
	// StrikeStep is the strike grid spacing in quote precision units.
	StrikeStep int64
	// StrikesPerSide counts strikes above and below the central strike.
	StrikesPerSide int
	// IV seeds the flat mark volatility for margin/mark purposes.
	IV float64
	// Margin overrides DefaultOptionMarginParams when non-zero.
	Margin OptionMarginParams
	// DeliveryFeeBps is stamped on every listed contract.
	DeliveryFeeBps int64
	// ObservationWindowNano overrides the settlement TWAP window when > 0.
	ObservationWindowNano int64

	listed     map[string]bool
	nextExpiry map[int64]int64
}

func (l *OptionChainLister) PendingListings(nowNano int64, prices etypes.PriceSource) []etypes.Instrument {
	if l.listed == nil {
		l.listed = make(map[string]bool)
	}
	if l.nextExpiry == nil {
		l.nextExpiry = make(map[int64]int64)
	}
	if l.StrikeStep <= 0 || l.StrikesPerSide < 0 {
		return nil
	}
	spot := prices.Price(l.Underlying)
	if spot <= 0 {
		return nil
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
			if strike <= 0 {
				continue
			}
			for _, isCall := range []bool{true, false} {
				out = l.appendOption(out, strike, expiry, isCall)
			}
		}
	}
	return out
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
	symbol := fmt.Sprintf("%s-%d-%d-%s", l.Spec.Base, expiry/1e9, strikeLabel, cp)
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

var _ etypes.ListingPolicy = (*OptionChainLister)(nil)
