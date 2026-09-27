package repeatedspot

import (
	"errors"
	"math/big"
	"slices"

	"exchange_sim/types"
)

// MarketSummary weights public book states by simulated elapsed time, not by
// the number of subscription or periodic snapshot messages.
type MarketSummary struct {
	HorizonNanos            int64  `json:"horizon_ns"`
	TwoSidedNanos           int64  `json:"two_sided_ns"`
	BidOnlyNanos            int64  `json:"bid_only_ns"`
	AskOnlyNanos            int64  `json:"ask_only_ns"`
	EmptyNanos              int64  `json:"empty_ns"`
	SpreadPriceUnitNanos    string `json:"spread_price_unit_ns"`
	BidDepthBaseUnitNanos   string `json:"bid_depth_base_unit_ns"`
	AskDepthBaseUnitNanos   string `json:"ask_depth_base_unit_ns"`
	PublicSnapshotMessages  int64  `json:"public_snapshot_messages"`
	TradeVolumeBaseUnits    string `json:"trade_volume_base_units"`
	TradeNotionalQuoteUnits string `json:"trade_notional_quote_units"`
}

type publicBookSeries struct {
	startAt      int64
	lastAt       int64
	tickSize     int64
	bids         map[int64]int64
	asks         map[int64]int64
	twoSided     int64
	bidOnly      int64
	askOnly      int64
	empty        int64
	spreadNanos  big.Int
	bidDepthNano big.Int
	askDepthNano big.Int
	snapshots    int64
}

func newPublicBookSeries(startAt, tickSize int64) *publicBookSeries {
	return &publicBookSeries{startAt: startAt, lastAt: startAt, tickSize: tickSize,
		bids: make(map[int64]int64), asks: make(map[int64]int64)}
}

func (series *publicBookSeries) accrue(until int64) error {
	if until < series.lastAt {
		return errors.New("repeated spot: public book time regressed")
	}
	duration := until - series.lastAt
	if duration == 0 {
		return nil
	}
	bid, bidDepth := bestVisible(series.bids, true)
	ask, askDepth := bestVisible(series.asks, false)
	switch {
	case bid > 0 && ask > 0:
		if bid >= ask {
			return errors.New("repeated spot: crossed public book")
		}
		series.twoSided += duration
		series.spreadNanos.Add(&series.spreadNanos,
			new(big.Int).Mul(big.NewInt(ask-bid), big.NewInt(duration)))
	case bid > 0:
		series.bidOnly += duration
	case ask > 0:
		series.askOnly += duration
	default:
		series.empty += duration
	}
	series.bidDepthNano.Add(&series.bidDepthNano, new(big.Int).Mul(big.NewInt(bidDepth), big.NewInt(duration)))
	series.askDepthNano.Add(&series.askDepthNano, new(big.Int).Mul(big.NewInt(askDepth), big.NewInt(duration)))
	series.lastAt = until
	return nil
}

func bestVisible(levels map[int64]int64, bidSide bool) (int64, int64) {
	var best, depth int64
	for price, quantity := range levels {
		if quantity <= 0 {
			continue
		}
		if best == 0 || bidSide && price > best || !bidSide && price < best {
			best, depth = price, quantity
		}
	}
	return best, depth
}

func (series *publicBookSeries) delta(at int64, side string, price, visible, hidden, total int64) error {
	if price <= 0 || price%series.tickSize != 0 || visible < 0 || hidden < 0 || total < 0 ||
		visible > total || hidden != total-visible {
		return errors.New("repeated spot: malformed public book delta")
	}
	if err := series.accrue(at); err != nil {
		return err
	}
	var levels map[int64]int64
	switch side {
	case "BUY":
		levels = series.bids
	case "SELL":
		levels = series.asks
	default:
		return errors.New("repeated spot: public book delta has invalid side")
	}
	if visible == 0 {
		delete(levels, price)
	} else {
		levels[price] = visible
	}
	return nil
}

func (series *publicBookSeries) verifySnapshot(at int64, bids, asks []types.PriceLevel) error {
	if err := series.accrue(at); err != nil {
		return err
	}
	if !slices.Equal(bids, displayedLevels(series.bids, true)) ||
		!slices.Equal(asks, displayedLevels(series.asks, false)) {
		return errors.New("repeated spot: published public snapshot differs from ordered book deltas")
	}
	series.snapshots++
	return nil
}

func (series *publicBookSeries) verifyTerminal(at int64, bids, asks []types.PriceLevel) error {
	if err := series.accrue(at); err != nil {
		return err
	}
	if !slices.Equal(bids, displayedLevels(series.bids, true)) ||
		!slices.Equal(asks, displayedLevels(series.asks, false)) {
		return errors.New("repeated spot: terminal public book differs from ordered book deltas")
	}
	return nil
}

func displayedLevels(levels map[int64]int64, bidSide bool) []types.PriceLevel {
	prices := make([]int64, 0, len(levels))
	for price := range levels {
		prices = append(prices, price)
	}
	slices.Sort(prices)
	if bidSide {
		slices.Reverse(prices)
	}
	if len(prices) > 20 {
		prices = prices[:20]
	}
	result := make([]types.PriceLevel, 0, len(prices))
	for _, price := range prices {
		result = append(result, types.PriceLevel{Price: price, VisibleQty: levels[price]})
	}
	return result
}

func (series *publicBookSeries) summary() MarketSummary {
	return MarketSummary{HorizonNanos: series.lastAt - series.startAt,
		TwoSidedNanos: series.twoSided, BidOnlyNanos: series.bidOnly,
		AskOnlyNanos: series.askOnly, EmptyNanos: series.empty,
		SpreadPriceUnitNanos:   series.spreadNanos.String(),
		BidDepthBaseUnitNanos:  series.bidDepthNano.String(),
		AskDepthBaseUnitNanos:  series.askDepthNano.String(),
		PublicSnapshotMessages: series.snapshots}
}
