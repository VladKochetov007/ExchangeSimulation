package exchange

import (
	"errors"
	"fmt"
	"math"
	"reflect"

	"exchange_sim/instrument"
	"exchange_sim/types"
)

var ErrFundingBookUnavailable = errors.New("funding book price unavailable")

type FundingBookPairRequest struct {
	VenueID       string
	SpotSymbol    string
	PerpSymbol    string
	TimestampNano int64
}

type FundingVisibleQuote struct {
	Price                            int64
	VisibleQty                       int64
	OldestVisibleOrderAcceptedAtNano int64
	OldestVisibleOrderAgeNano        int64
}

type FundingBookTop struct {
	Symbol   string
	Bid      FundingVisibleQuote
	Ask      FundingVisibleQuote
	MidPrice int64
}

// FundingBookPair is one venue-locked spot/perp observation. Oldest order age
// means time since admission of a currently displayed order, not time since an
// iceberg tranche refresh. Source event ordering still requires canonical
// capture evidence; this value alone does not attest the runner's phase.
type FundingBookPair struct {
	VenueID        string
	TimestampNano  int64
	BaseAsset      string
	QuoteAsset     string
	BasePrecision  int64
	QuotePrecision int64
	Spot           FundingBookTop
	Perp           FundingBookTop
}

func (pair FundingBookPair) RateWindowSample() instrument.FundingWindowSample {
	return instrument.FundingWindowSample{
		TimestampNano: pair.TimestampNano,
		IndexPrice:    pair.Spot.MidPrice,
		MarkPrice:     pair.Perp.MidPrice,
	}
}

// CaptureFundingBookPair reads both live books under one exchange lock. A
// caller must invoke it at a verified pre-ingress phase and retain the full
// source pair for evidence; using only RateWindowSample loses provenance.
func (e *DefaultExchange) CaptureFundingBookPair(request FundingBookPairRequest) (FundingBookPair, error) {
	if e == nil || e.Clock == nil || request.VenueID == "" || request.SpotSymbol == "" ||
		request.PerpSymbol == "" || request.SpotSymbol == request.PerpSymbol || request.TimestampNano < 0 {
		return FundingBookPair{}, fmt.Errorf("funding book pair: invalid venue, symbols or timestamp")
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.ID != request.VenueID || e.Clock.NowUnixNano() != request.TimestampNano {
		return FundingBookPair{}, fmt.Errorf("funding book pair: venue or live clock does not match requested source")
	}
	spotInstrument := e.Instruments[request.SpotSymbol]
	perpInstrument := e.Instruments[request.PerpSymbol]
	spotBook := e.Books[request.SpotSymbol]
	perpBook := e.Books[request.PerpSymbol]
	if !fundingInstrumentBindingMatches(spotInstrument, spotBook) ||
		!fundingInstrumentBindingMatches(perpInstrument, perpBook) ||
		spotInstrument.InstrumentType() != "SPOT" ||
		perpInstrument.InstrumentType() != "PERP" || !perpInstrument.IsPerp() ||
		spotInstrument.Symbol() != request.SpotSymbol || perpInstrument.Symbol() != request.PerpSymbol ||
		spotInstrument.BaseAsset() == "" || spotInstrument.QuoteAsset() == "" ||
		spotInstrument.BaseAsset() != perpInstrument.BaseAsset() ||
		spotInstrument.QuoteAsset() != perpInstrument.QuoteAsset() ||
		spotInstrument.BasePrecision() <= 0 || spotInstrument.QuotePrecision() <= 0 ||
		spotInstrument.BasePrecision() != perpInstrument.BasePrecision() ||
		spotInstrument.QuotePrecision() != perpInstrument.QuotePrecision() {
		return FundingBookPair{}, fmt.Errorf("funding book pair: incompatible spot/perp instrument identity or precision")
	}
	if spotBook.Symbol != request.SpotSymbol ||
		perpBook.Symbol != request.PerpSymbol {
		return FundingBookPair{}, fmt.Errorf("funding book pair: configured instrument has no matching live book")
	}
	spot, err := captureFundingBookTop(spotBook, request.TimestampNano)
	if err != nil {
		return FundingBookPair{}, fmt.Errorf("funding book pair %s: %w", request.SpotSymbol, err)
	}
	perp, err := captureFundingBookTop(perpBook, request.TimestampNano)
	if err != nil {
		return FundingBookPair{}, fmt.Errorf("funding book pair %s: %w", request.PerpSymbol, err)
	}
	if !fundingTopPricesAdmitted(spotInstrument, spot) || !fundingTopPricesAdmitted(perpInstrument, perp) {
		return FundingBookPair{}, fmt.Errorf("funding book pair: live quote violates configured instrument price domain")
	}
	return FundingBookPair{
		VenueID: request.VenueID, TimestampNano: request.TimestampNano,
		BaseAsset: spotInstrument.BaseAsset(), QuoteAsset: spotInstrument.QuoteAsset(),
		BasePrecision: spotInstrument.BasePrecision(), QuotePrecision: spotInstrument.QuotePrecision(),
		Spot: spot, Perp: perp,
	}, nil
}

func fundingInstrumentBindingMatches(configured Instrument, book *OrderBook) bool {
	if configured == nil || book == nil || book.Instrument == nil {
		return false
	}
	configuredValue := reflect.ValueOf(configured)
	boundValue := reflect.ValueOf(book.Instrument)
	if configuredValue.Type() != boundValue.Type() {
		return false
	}
	if configuredValue.Kind() == reflect.Pointer {
		return !configuredValue.IsNil() && !boundValue.IsNil() && configuredValue.Pointer() == boundValue.Pointer()
	}
	return reflect.DeepEqual(configured, book.Instrument)
}

func fundingTopPricesAdmitted(inst Instrument, top FundingBookTop) bool {
	return inst.ValidatePrice(top.Bid.Price) && inst.ValidatePrice(top.Ask.Price)
}

func captureFundingBookTop(book *OrderBook, atNano int64) (FundingBookTop, error) {
	if book.Bids == nil || book.Asks == nil {
		return FundingBookTop{}, ErrFundingBookUnavailable
	}
	bestBid := bestFundingDisplayedLevel(book.Bids)
	bestAsk := bestFundingDisplayedLevel(book.Asks)
	if bestBid == nil || bestAsk == nil {
		return FundingBookTop{}, ErrFundingBookUnavailable
	}
	if bestBid.Price <= 0 || bestAsk.Price <= 0 || bestBid.Price > bestAsk.Price {
		return FundingBookTop{}, fmt.Errorf("%w: non-positive or crossed best prices", ErrFundingBookUnavailable)
	}
	bid, err := captureFundingVisibleQuote(bestBid, atNano)
	if err != nil {
		return FundingBookTop{}, err
	}
	ask, err := captureFundingVisibleQuote(bestAsk, atNano)
	if err != nil {
		return FundingBookTop{}, err
	}
	return FundingBookTop{Symbol: book.Symbol, Bid: bid, Ask: ask,
		MidPrice: types.Midpoint(bid.Price, ask.Price)}, nil
}

func bestFundingDisplayedLevel(side *Book) *Limit {
	for level := side.ActiveHead; level != nil; level = level.Next {
		if visibleQty(level) > 0 {
			return level
		}
	}
	return nil
}

func captureFundingVisibleQuote(limit *Limit, atNano int64) (FundingVisibleQuote, error) {
	visible := visibleQty(limit)
	if visible <= 0 {
		return FundingVisibleQuote{}, fmt.Errorf("%w: no displayed best-level depth", ErrFundingBookUnavailable)
	}
	oldest := int64(math.MaxInt64)
	for order := limit.Head; order != nil; order = order.Next {
		remaining, ok := types.TrySub(order.Qty, order.FilledQty)
		if !ok || remaining <= 0 {
			continue
		}
		if order.Visibility == Normal ||
			order.Visibility == Iceberg && order.DisplayRemaining > 0 {
			if order.Timestamp < 0 || order.Timestamp >= atNano {
				return FundingVisibleQuote{}, fmt.Errorf("funding book pair: best quote timestamp violates pre-instant source")
			}
			if order.Timestamp < oldest {
				oldest = order.Timestamp
			}
		}
	}
	if oldest == math.MaxInt64 {
		return FundingVisibleQuote{}, fmt.Errorf("%w: displayed depth has no visible source order", ErrFundingBookUnavailable)
	}
	return FundingVisibleQuote{Price: limit.Price, VisibleQty: visible,
		OldestVisibleOrderAcceptedAtNano: oldest, OldestVisibleOrderAgeNano: atNano - oldest}, nil
}
