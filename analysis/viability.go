package analysis

import (
	"sort"
	"sync"
)

// ViabilityOptions configures the corridor measurement.
//
// A market that is merely executing trades is not necessarily alive. Volume
// above zero is satisfied by two participants trading with each other in an
// otherwise empty book, by a single class that has accumulated everyone else's
// capital, and by a book whose spread has widened until nothing crosses it
// except by accident. The corridor is the multidimensional alternative: a
// window is viable when it passes every rule the caller supplies, and a run is
// alive when its windows are.
type ViabilityOptions struct {
	// Files selects the books to measure. Empty measures every indexed log.
	Files []string
	// FilesSelected marks that the caller performed a selection, so an empty
	// Files means "no matches" rather than "not specified".
	FilesSelected bool
	// WindowNanos is the length of one measurement window in simulated time.
	// Zero measures the whole run as a single window, which answers whether a
	// market ever traded and not whether it stayed alive.
	WindowNanos int64
	// TickSize converts spreads to ticks. Zero leaves the spread in quote
	// units, which is not comparable across instruments.
	TickSize int64
	// Rules decide whether a window is viable. They are supplied rather than
	// built in because what counts as a living market depends on the
	// instrument, the venue and the question: an option chain that trades once
	// an hour is healthy and a spot book that does is dead. An empty rule set
	// measures without judging.
	Rules []ViabilityRule
}

// ViabilityRule is one condition a window must satisfy. Breached reports a
// violation, so a rule reads as the thing that goes wrong.
type ViabilityRule struct {
	Name     string
	Breached func(MarketWindow) bool
}

// MarketWindow is what one book did over one window of simulated time.
type MarketWindow struct {
	VenueID string
	Symbol  string
	Start   int64 `json:"start"`
	End     int64 `json:"end"`

	Trades int   `json:"trades"`
	Volume int64 `json:"volume"`
	// TakerRoles and MakerRoles count the distinct participant classes that
	// took and provided liquidity. A market with one of either is a market
	// with a single point of failure, whatever its volume.
	TakerRoles int `json:"taker_roles"`
	MakerRoles int `json:"maker_roles"`
	// TopRoleVolumeShare is the largest share of traded volume attributable to
	// one participant class, taker side. Concentration is how a market dies
	// while still printing trades.
	TopRoleVolumeShare float64 `json:"top_role_volume_share"`

	Snapshots int `json:"snapshots"`
	// EmptySideSnapshots counts publications with no bid or no ask. A book that
	// is one-sided cannot be traded on that side at any price.
	EmptySideSnapshots int          `json:"empty_side_snapshots"`
	SpreadTicks        Distribution `json:"spread_ticks"`
	TouchDepth         Distribution `json:"touch_depth"`

	// Breaches names the rules this window failed, in the order supplied.
	Breaches []string `json:"breaches,omitempty"`
}

// Viable reports whether the window broke no rule.
func (w MarketWindow) Viable() bool { return len(w.Breaches) == 0 }

// Viability is the corridor over a whole run.
type Viability struct {
	Windows []MarketWindow `json:"windows"`
	// Books is how many distinct venue-and-symbol books were measured.
	Books int `json:"books"`
	// ViableWindows and BreachedWindows partition Windows.
	ViableWindows   int `json:"viable_windows"`
	BreachedWindows int `json:"breached_windows"`
	// BreachesByRule counts how many windows each rule rejected, which is what
	// says how a market died rather than that it did.
	BreachesByRule map[string]int `json:"breaches_by_rule"`
	// DeadBooks lists the books that breached in every window they appeared in.
	DeadBooks []string `json:"dead_books,omitempty"`
}

type windowKey struct {
	venue  string
	symbol string
	index  int64
}

type windowAccumulator struct {
	trades          int
	volume          int64
	takerRoleVolume map[string]int64
	makerRoles      map[string]struct{}
	snapshots       int
	emptySide       int
	spread          []float64
	touchDepth      []float64
}

func newWindowAccumulator() *windowAccumulator {
	return &windowAccumulator{
		takerRoleVolume: make(map[string]int64),
		makerRoles:      make(map[string]struct{}),
	}
}

// MeasureViability walks a run and reports the corridor window by window.
func (r *Run) MeasureViability(opts ViabilityOptions) (*Viability, error) {
	var mu sync.Mutex
	windows := make(map[windowKey]*windowAccumulator)
	at := func(key windowKey) *windowAccumulator {
		accumulator, exists := windows[key]
		if !exists {
			accumulator = newWindowAccumulator()
			windows[key] = accumulator
		}
		return accumulator
	}
	// A derivative record carries its symbol beside the payload while a spot
	// record carries it inside, so the book a window belongs to has to be taken
	// from whichever level wrote it. Reading only the outer level puts every
	// spot book in the same nameless bucket.
	keyFor := func(event Event, payloadSymbol string) windowKey {
		index := int64(0)
		if opts.WindowNanos > 0 {
			index = event.SimTS / opts.WindowNanos
		}
		symbol := event.Symbol
		if symbol == "" {
			symbol = payloadSymbol
		}
		return windowKey{venue: event.VenueID, symbol: symbol, index: index}
	}

	// Role here is the fill's liquidity role, taker or maker, which the venue
	// writes beside the quantity. The participant class is looked up from the
	// run's account report instead, since the fill does not carry it.
	type fillPayload struct {
		Qty    int64  `json:"qty"`
		Role   string `json:"role"`
		Symbol string `json:"symbol"`
	}
	scan := ScanOptions{Events: []string{"OrderFill"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	if err := r.Scan(scan, func(event Event) {
		var fill fillPayload
		if event.Decode(&fill) != nil || fill.Qty <= 0 {
			return
		}
		role := r.Role(event.VenueID, event.ClientID)
		taker := fill.Role == "taker"
		key := keyFor(event, fill.Symbol)
		mu.Lock()
		accumulator := at(key)
		if taker {
			accumulator.trades++
			accumulator.volume += fill.Qty
			accumulator.takerRoleVolume[role] += fill.Qty
		} else {
			accumulator.makerRoles[role] = struct{}{}
		}
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	snapshotScan := ScanOptions{Events: []string{"BookSnapshot"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	if err := r.Scan(snapshotScan, func(event Event) {
		if !isPeriodicSnapshot(event) {
			return
		}
		var envelope bookSnapshotEnvelope
		if event.Decode(&envelope) != nil {
			return
		}
		bids, asks := envelope.levels()
		key := keyFor(event, envelope.Symbol)
		mu.Lock()
		accumulator := at(key)
		accumulator.snapshots++
		if len(bids) == 0 || len(asks) == 0 {
			accumulator.emptySide++
			mu.Unlock()
			return
		}
		bestBid, bidDepth := bestWithDepth(bids, true)
		bestAsk, askDepth := bestWithDepth(asks, false)
		spread := float64(bestAsk - bestBid)
		if opts.TickSize > 0 {
			spread /= float64(opts.TickSize)
		}
		accumulator.spread = append(accumulator.spread, spread)
		accumulator.touchDepth = append(accumulator.touchDepth, float64(bidDepth+askDepth)/2)
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	return summariseViability(windows, opts), nil
}

// bestWithDepth returns the best price on a side and the depth resting at it.
func bestWithDepth(levels []bookLevel, buySide bool) (int64, int64) {
	best := int64(0)
	depth := int64(0)
	for _, level := range levels {
		if level.Price <= 0 {
			continue
		}
		if best == 0 || (buySide && level.Price > best) || (!buySide && level.Price < best) {
			best = level.Price
			depth = 0
		}
		if level.Price == best {
			depth += level.VisibleQty + level.HiddenQty
		}
	}
	return best, depth
}

func summariseViability(windows map[windowKey]*windowAccumulator, opts ViabilityOptions) *Viability {
	result := &Viability{BreachesByRule: make(map[string]int)}
	keys := make([]windowKey, 0, len(windows))
	books := make(map[string]struct{})
	for key := range windows {
		keys = append(keys, key)
		books[key.venue+" "+key.symbol] = struct{}{}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].venue != keys[j].venue {
			return keys[i].venue < keys[j].venue
		}
		if keys[i].symbol != keys[j].symbol {
			return keys[i].symbol < keys[j].symbol
		}
		return keys[i].index < keys[j].index
	})
	result.Books = len(books)

	breachedBooks := make(map[string]int)
	windowsPerBook := make(map[string]int)
	for _, key := range keys {
		accumulator := windows[key]
		window := MarketWindow{
			VenueID:    key.venue,
			Symbol:     key.symbol,
			Start:      key.index * opts.WindowNanos,
			End:        (key.index + 1) * opts.WindowNanos,
			Trades:     accumulator.trades,
			Volume:     accumulator.volume,
			TakerRoles: len(accumulator.takerRoleVolume),
			MakerRoles: len(accumulator.makerRoles),
			Snapshots:  accumulator.snapshots,

			EmptySideSnapshots: accumulator.emptySide,
			SpreadTicks:        Describe(accumulator.spread),
			TouchDepth:         Describe(accumulator.touchDepth),
		}
		if accumulator.volume > 0 {
			top := int64(0)
			for _, volume := range accumulator.takerRoleVolume {
				if volume > top {
					top = volume
				}
			}
			window.TopRoleVolumeShare = float64(top) / float64(accumulator.volume)
		}
		for _, rule := range opts.Rules {
			if rule.Breached != nil && rule.Breached(window) {
				window.Breaches = append(window.Breaches, rule.Name)
				result.BreachesByRule[rule.Name]++
			}
		}
		book := key.venue + " " + key.symbol
		windowsPerBook[book]++
		if window.Viable() {
			result.ViableWindows++
		} else {
			result.BreachedWindows++
			breachedBooks[book]++
		}
		result.Windows = append(result.Windows, window)
	}
	for book, breached := range breachedBooks {
		if breached == windowsPerBook[book] {
			result.DeadBooks = append(result.DeadBooks, book)
		}
	}
	sort.Strings(result.DeadBooks)
	return result
}
