package analysis

import (
	"sort"
	"sync"
)

// BookShapeOptions selects the books to measure.
type BookShapeOptions struct {
	Files []string
	// TickSize converts the spread to ticks. Zero leaves SpreadTicks empty.
	TickSize int64
}

// BookShape describes the standing liquidity a taker actually faces.
//
// It exists to answer a structural question the return statistics cannot: an
// order can only consume depth if there is depth beyond the touch to consume.
// If essentially all resting size sits at the best price, no order walks the
// book however large it is, and per-trade impact is unreachable by
// construction rather than by parameter choice.
type BookShape struct {
	Snapshots int `json:"snapshots"`
	// OneSideEmpty counts snapshots with no bid or no ask, where a taker on
	// that side has nothing to trade against at any price.
	OneSideEmpty int `json:"one_side_empty"`
	// BothSidesEmpty counts snapshots with no liquidity at all.
	BothSidesEmpty int `json:"both_sides_empty"`

	// Levels is the number of distinct prices resting on one side.
	BidLevels Distribution `json:"bid_levels"`
	AskLevels Distribution `json:"ask_levels"`

	// TouchShare is the fraction of a side's visible depth resting at its best
	// price. A median of 1.0 means the book is one level deep.
	TouchShare Distribution `json:"touch_share"`
	// TouchDepth and BeyondTouchDepth are in base units, so they can be
	// compared against the order sizes the population actually submits.
	TouchDepth       Distribution `json:"touch_depth"`
	BeyondTouchDepth Distribution `json:"beyond_touch_depth"`

	// SpreadTicks is the distance between best bid and best ask.
	SpreadTicks Distribution `json:"spread_ticks"`
	// HiddenShare is the fraction of total depth that is not displayed.
	HiddenShare Distribution `json:"hidden_share"`
	// TradesPerSnapshot is how many executions fall between consecutive book
	// publications. Any metric referencing a trade to the last published mid
	// is stale by this many trades, and those trades are same-signed on
	// average, so the reference is biased toward the trade's own direction.
	TradesPerSnapshot float64 `json:"trades_per_snapshot"`
}

// isPeriodicSnapshot keeps the exchange's own book publication and drops the
// one sent to a client on subscription. The latter fires at whatever moment a
// participant connects, which clusters at the start of a run when the book is
// still degenerate, and describes one subscriber's view rather than the
// standing book.
func isPeriodicSnapshot(event Event) bool { return event.ClientID == 0 }

type bookLevel struct {
	Price      int64 `json:"price"`
	VisibleQty int64 `json:"visible_qty"`
	HiddenQty  int64 `json:"hidden_qty"`
}

type bookSnapshot struct {
	Bids []bookLevel `json:"bids"`
	Asks []bookLevel `json:"asks"`
}

type bookSnapshotEnvelope struct {
	Payload  *bookSnapshot `json:"payload"`
	Snapshot *bookSnapshot `json:"snapshot"`
	Bids     []bookLevel   `json:"bids"`
	Asks     []bookLevel   `json:"asks"`
}

// levels resolves the snapshot from whichever envelope the venue wrote.
func (e bookSnapshotEnvelope) levels() ([]bookLevel, []bookLevel) {
	if e.Payload != nil {
		return e.Payload.Bids, e.Payload.Asks
	}
	if e.Snapshot != nil {
		return e.Snapshot.Bids, e.Snapshot.Asks
	}
	return e.Bids, e.Asks
}

// sideShape reduces one side of a snapshot to the quantities BookShape tracks.
// Levels sharing a price are merged, since a side's depth is per price and not
// per resting order.
func sideShape(levels []bookLevel, best func([]bookLevel) int64) (levelCount int, touch, total, hidden int64) {
	if len(levels) == 0 {
		return 0, 0, 0, 0
	}
	byPrice := make(map[int64]int64, len(levels))
	for _, level := range levels {
		// Hidden quantity is depth a taker consumes at the same price, so it
		// belongs in the level's size. Excluding it understates the touch and
		// makes MeasureWalkable overstate how often an order must walk.
		byPrice[level.Price] += level.VisibleQty + level.HiddenQty
		total += level.VisibleQty + level.HiddenQty
		hidden += level.HiddenQty
	}
	touch = byPrice[best(levels)]
	return len(byPrice), touch, total, hidden
}

// MeasureBookShape summarises the standing liquidity across a run's snapshots.
func (r *Run) MeasureBookShape(opts BookShapeOptions) (*BookShape, error) {
	var mu sync.Mutex
	shape := &BookShape{}
	var bidLevels, askLevels, touchShare, touchDepth, beyondDepth, spreadTicks, hiddenShare []float64

	err := r.Scan(ScanOptions{Events: []string{"BookSnapshot"}, Files: opts.Files, FilesSelected: true}, func(event Event) {
		if !isPeriodicSnapshot(event) {
			return
		}
		var envelope bookSnapshotEnvelope
		if event.Decode(&envelope) != nil {
			return
		}
		bids, asks := envelope.levels()

		bidCount, bidTouch, bidTotal, bidHidden := sideShape(bids, bestBid)
		askCount, askTouch, askTotal, askHidden := sideShape(asks, bestAsk)

		mu.Lock()
		defer mu.Unlock()
		shape.Snapshots++
		switch {
		case bidCount == 0 && askCount == 0:
			shape.BothSidesEmpty++
			return
		case bidCount == 0 || askCount == 0:
			shape.OneSideEmpty++
		}

		record := func(touch, total, hidden int64) {
			if total <= 0 {
				return
			}
			touchShare = append(touchShare, float64(touch)/float64(total))
			touchDepth = append(touchDepth, float64(touch))
			beyondDepth = append(beyondDepth, float64(total-touch))
			hiddenShare = append(hiddenShare, float64(hidden)/float64(total))
		}
		if bidCount > 0 {
			bidLevels = append(bidLevels, float64(bidCount))
			record(bidTouch, bidTotal, bidHidden)
		}
		if askCount > 0 {
			askLevels = append(askLevels, float64(askCount))
			record(askTouch, askTotal, askHidden)
		}
		if bidCount > 0 && askCount > 0 && opts.TickSize > 0 {
			spread := bestAsk(asks) - bestBid(bids)
			if spread > 0 {
				spreadTicks = append(spreadTicks, float64(spread)/float64(opts.TickSize))
			}
		}
	})
	if err != nil {
		return nil, err
	}

	trades := 0
	if err := r.Scan(ScanOptions{Events: []string{"Trade"}, Files: opts.Files, FilesSelected: true}, func(Event) {
		mu.Lock()
		trades++
		mu.Unlock()
	}); err != nil {
		return nil, err
	}
	if shape.Snapshots > 0 {
		shape.TradesPerSnapshot = float64(trades) / float64(shape.Snapshots)
	}

	shape.BidLevels = Describe(bidLevels)
	shape.AskLevels = Describe(askLevels)
	shape.TouchShare = Describe(touchShare)
	shape.TouchDepth = Describe(touchDepth)
	shape.BeyondTouchDepth = Describe(beyondDepth)
	shape.SpreadTicks = Describe(spreadTicks)
	shape.HiddenShare = Describe(hiddenShare)
	return shape, nil
}

func bestBid(levels []bookLevel) int64 {
	best := levels[0].Price
	for _, level := range levels {
		if level.Price > best {
			best = level.Price
		}
	}
	return best
}

func bestAsk(levels []bookLevel) int64 {
	best := levels[0].Price
	for _, level := range levels {
		if level.Price < best {
			best = level.Price
		}
	}
	return best
}

// WalkableFraction reports how often an order of each given size would have to
// consume more than the best price to fill completely.
//
// This is the quantity that decides whether depth consumption can be a price
// channel at all: an order that never exceeds the touch cannot move the price
// no matter how the matcher behaves.
type WalkableFraction struct {
	SizeBase int64 `json:"size_base"`
	// ExceedsTouch is the share of observed sides where the size is larger than
	// the depth resting at the best price.
	ExceedsTouch float64 `json:"exceeds_touch"`
	// ExceedsBook is the share where it exceeds the entire visible side, so the
	// order cannot fill from displayed liquidity at any price.
	ExceedsBook float64 `json:"exceeds_book"`
	Sides       int     `json:"sides"`
}

// MeasureWalkable evaluates the given order sizes against every observed side.
func (r *Run) MeasureWalkable(opts BookShapeOptions, sizes []int64) ([]WalkableFraction, error) {
	var mu sync.Mutex
	var touches, totals []int64

	err := r.Scan(ScanOptions{Events: []string{"BookSnapshot"}, Files: opts.Files, FilesSelected: true}, func(event Event) {
		if !isPeriodicSnapshot(event) {
			return
		}
		var envelope bookSnapshotEnvelope
		if event.Decode(&envelope) != nil {
			return
		}
		bids, asks := envelope.levels()
		mu.Lock()
		defer mu.Unlock()
		for _, side := range []struct {
			levels []bookLevel
			best   func([]bookLevel) int64
		}{{bids, bestBid}, {asks, bestAsk}} {
			if _, touch, total, _ := sideShape(side.levels, side.best); total > 0 {
				touches = append(touches, touch)
				totals = append(totals, total)
			}
		}
	})
	if err != nil {
		return nil, err
	}

	sorted := append([]int64(nil), sizes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	out := make([]WalkableFraction, 0, len(sorted))
	for _, size := range sorted {
		exceedsTouch, exceedsBook := 0, 0
		for i := range touches {
			if size > touches[i] {
				exceedsTouch++
			}
			if size > totals[i] {
				exceedsBook++
			}
		}
		fraction := WalkableFraction{SizeBase: size, Sides: len(touches)}
		if len(touches) > 0 {
			fraction.ExceedsTouch = float64(exceedsTouch) / float64(len(touches))
			fraction.ExceedsBook = float64(exceedsBook) / float64(len(totals))
		}
		out = append(out, fraction)
	}
	return out, nil
}
