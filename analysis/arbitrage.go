package analysis

import (
	"math"
	"sort"
	"sync"
)

// ArbitrageOptions configures the omniscient arbitrage auditor.
//
// The auditor is deliberately not a participant. It sees every venue's touch
// at once, pays the taker fee on every leg, and never has to queue, so an edge
// it cannot find does not exist for anyone. An edge it does find is not
// automatically a bug: a real market has transient dislocations that trading
// removes. What distinguishes a bug is an edge that never closes.
type ArbitrageOptions struct {
	Files         []string
	FilesSelected bool
	// TakerFeeBps is charged on every leg of a cycle.
	TakerFeeBps float64
	// StalenessNanos is how old a quote may be and still count as executable.
	// Quotes from different books never arrive together, and treating a stale
	// one as live invents arbitrage out of timing.
	StalenessNanos int64
	// Triangle names the three spot books of a currency triangle, in file form
	// (ABC-USD, CDF-USD, ABC-CDF), and CrossPrecision scales the implied rate.
	BaseSymbol     string
	QuoteSymbol    string
	CrossSymbol    string
	CrossPrecision int64
	// CrossVenueSymbol is the book compared across venues for the same asset.
	CrossVenueSymbol string
	// PerpSymbol and SpotSymbol name the perpetual and its underlying, for the
	// carry cycle: buy the cheaper of the two and sell the dearer, holding the
	// pair until they converge. Unlike the spot cycles this one is not
	// instantaneous — it is closed by funding rather than by a trade — so its
	// edge is reported as a basis and not as a guaranteed profit.
	PerpSymbol string
	SpotSymbol string
	// ParityUnderlying enables the put-call parity cycle on option books.
	ParityUnderlying string
}

// CycleResult is one arbitrage cycle's audit.
type CycleResult struct {
	Cycle string `json:"cycle"`
	// Observations is how many instants had every leg fresh enough to trade.
	Observations int `json:"observations"`
	// UndefinedDomainObservations is how many instants had all required book
	// sides present and fresh but could not be expressed by this audit's
	// current positive-cashflow BPS statistic. It is deliberately separate
	// from a missing or stale quote: a signed or zero price remains evidence,
	// but the ratio-based cycle is not mathematically/economically defined for
	// the current spot/Black-76 policy.
	UndefinedDomainObservations int `json:"undefined_domain_observations"`
	// Profitable is how many of those had a positive edge after fees.
	Profitable int `json:"profitable"`
	// MeanEdgeBps is over profitable observations only, MaxEdgeBps the best
	// single instant, and MeanAllBps over every observation including the
	// unprofitable ones, which is what says whether the market is on average
	// arbitrage-free rather than merely arbitrage-free most of the time.
	MeanEdgeBps float64 `json:"mean_edge_bps"`
	MaxEdgeBps  float64 `json:"max_edge_bps"`
	MeanAllBps  float64 `json:"mean_all_bps"`
	// LongestRunNanos is the longest unbroken stretch during which the edge
	// stayed positive. A permanent structural arbitrage shows up here, and a
	// transient dislocation does not.
	LongestRunNanos int64 `json:"longest_run_nanos"`
	// ProfitableShare is Profitable over Observations.
	ProfitableShare float64 `json:"profitable_share"`
}

// ArbitrageAudit is the set of cycles searched.
type ArbitrageAudit struct {
	Cycles []CycleResult `json:"cycles"`
	// FeeBps and StalenessNanos are echoed so a result cannot be read without
	// the assumptions it was computed under.
	FeeBps         float64 `json:"fee_bps"`
	StalenessNanos int64   `json:"staleness_nanos"`
}

type quote struct {
	bid, ask int64
	at       int64
}

// cycleAccumulator collects one edge per instant and summarises afterwards.
//
// The summary cannot be computed during the scan: files are read concurrently,
// so observations do not arrive in time order, and a run length measured from
// arrival order is meaningless. Keeping the best edge per instant also stops
// one instant being counted several times when several books publish at it.
type cycleAccumulator struct {
	edges     map[int64]float64
	undefined map[int64]struct{}
}

func newCycleAccumulator() *cycleAccumulator {
	return &cycleAccumulator{
		edges:     make(map[int64]float64),
		undefined: make(map[int64]struct{}),
	}
}

func (c *cycleAccumulator) observe(at int64, edgeBps float64) {
	delete(c.undefined, at)
	if existing, seen := c.edges[at]; seen && existing >= edgeBps {
		return
	}
	c.edges[at] = edgeBps
}

// observeUndefined records an instant with every required leg present but
// outside this metric's declared domain. Do not add a numeric zero edge: that
// would falsely claim that an unpriceable signed cycle was arbitrage-free.
func (c *cycleAccumulator) observeUndefined(at int64) {
	if _, measured := c.edges[at]; measured {
		return
	}
	c.undefined[at] = struct{}{}
}

func (c *cycleAccumulator) result(name string) CycleResult {
	out := CycleResult{Cycle: name, UndefinedDomainObservations: len(c.undefined)}
	if len(c.edges) == 0 {
		return out
	}
	instants := make([]int64, 0, len(c.edges))
	for at := range c.edges {
		instants = append(instants, at)
	}
	sort.Slice(instants, func(i, j int) bool { return instants[i] < instants[j] })

	sumAll, sumPositive := 0.0, 0.0
	runStart, runLast := int64(0), int64(0)
	inRun := false
	for _, at := range instants {
		edge := c.edges[at]
		out.Observations++
		sumAll += edge
		if edge <= 0 {
			inRun = false
			continue
		}
		out.Profitable++
		sumPositive += edge
		if edge > out.MaxEdgeBps {
			out.MaxEdgeBps = edge
		}
		if !inRun {
			runStart, inRun = at, true
		}
		runLast = at
		if span := runLast - runStart; span > out.LongestRunNanos {
			out.LongestRunNanos = span
		}
	}
	if out.Profitable > 0 {
		out.MeanEdgeBps = sumPositive / float64(out.Profitable)
	}
	out.MeanAllBps = sumAll / float64(out.Observations)
	out.ProfitableShare = float64(out.Profitable) / float64(out.Observations)
	return out
}

// MeasureArbitrage searches the frozen population's tradable cycles.
//
// The scan collects each book's quote series, and the cycles are evaluated in
// a second pass in time order. Evaluating during the scan is what an earlier
// version did and it was wrong twice over: files are read concurrently, so a
// quote from later in the run can be present while an earlier instant is being
// priced, and a run length measured in arrival order is not a run length at
// all.
func (r *Run) MeasureArbitrage(opts ArbitrageOptions) (*ArbitrageAudit, error) {
	staleness := opts.StalenessNanos
	if staleness <= 0 {
		staleness = int64(2e9)
	}
	feeFactor := 1 - opts.TakerFeeBps/1e4

	var mu sync.Mutex
	series := make(map[markKey][]quote)

	scan := ScanOptions{Events: []string{"BookSnapshot"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	if err := r.Scan(scan, func(event Event) {
		if !isPeriodicSnapshot(event) {
			return
		}
		var envelope bookSnapshotEnvelope
		if event.Decode(&envelope) != nil {
			return
		}
		bids, asks := envelope.levels()
		if len(bids) == 0 || len(asks) == 0 {
			return
		}
		bestBid, _, bidOK := bestWithDepth(bids, true)
		bestAsk, _, askOK := bestWithDepth(asks, false)
		if !bidOK || !askOK {
			return
		}
		// A derivative record names its book beside the payload; a spot record
		// names it nowhere and only the file it was written to identifies it.
		symbol := event.Symbol
		if symbol == "" {
			symbol = symbolFromPath(event.File)
		}
		key := markKey{event.VenueID, symbol}
		mu.Lock()
		series[key] = append(series[key], quote{bid: bestBid, ask: bestAsk, at: event.SimTS})
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	instants := make(map[int64]struct{})
	for key := range series {
		sort.Slice(series[key], func(i, j int) bool { return series[key][i].at < series[key][j].at })
		for _, q := range series[key] {
			instants[q.at] = struct{}{}
		}
	}
	ordered := make([]int64, 0, len(instants))
	for at := range instants {
		ordered = append(ordered, at)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })

	cursor := make(map[markKey]int)
	live := make(map[markKey]quote)
	quoteAt := func(key markKey, at int64) (quote, bool) {
		points := series[key]
		for cursor[key] < len(points) && points[cursor[key]].at <= at {
			live[key] = points[cursor[key]]
			cursor[key]++
		}
		current, seen := live[key]
		if !seen || at-current.at > staleness {
			return quote{}, false
		}
		return current, true
	}

	venues := make(map[string]struct{})
	for key := range series {
		venues[key.venue] = struct{}{}
	}
	venueNames := make([]string, 0, len(venues))
	for venue := range venues {
		venueNames = append(venueNames, venue)
	}
	sort.Strings(venueNames)

	triangles := make(map[string]*cycleAccumulator)
	crossVenue := make(map[string]*cycleAccumulator)
	for _, at := range ordered {
		if opts.BaseSymbol != "" && opts.CrossPrecision > 0 {
			for _, venue := range venueNames {
				base, baseOK := quoteAt(markKey{venue, opts.BaseSymbol}, at)
				quotePair, quoteOK := quoteAt(markKey{venue, opts.QuoteSymbol}, at)
				cross, crossOK := quoteAt(markKey{venue, opts.CrossSymbol}, at)
				if !baseOK || !quoteOK || !crossOK {
					continue
				}
				if triangles[venue] == nil {
					triangles[venue] = newCycleAccumulator()
				}
				if !positiveCashflowQuotes(base, quotePair, cross) {
					triangles[venue].observeUndefined(at)
					continue
				}
				triangles[venue].observe(at, triangleEdgeBps(&base, &quotePair, &cross, opts.CrossPrecision, feeFactor))
			}
		}
		if opts.CrossVenueSymbol == "" {
			continue
		}
		for _, buyVenue := range venueNames {
			for _, sellVenue := range venueNames {
				if buyVenue == sellVenue {
					continue
				}
				offered, buyOK := quoteAt(markKey{buyVenue, opts.CrossVenueSymbol}, at)
				bid, sellOK := quoteAt(markKey{sellVenue, opts.CrossVenueSymbol}, at)
				if !buyOK || !sellOK {
					continue
				}
				name := buyVenue + "->" + sellVenue
				if crossVenue[name] == nil {
					crossVenue[name] = newCycleAccumulator()
				}
				if !positiveCashflowQuotes(offered, bid) {
					crossVenue[name].observeUndefined(at)
					continue
				}
				edge := float64(bid.bid)*feeFactor/(float64(offered.ask)/feeFactor) - 1
				crossVenue[name].observe(at, 1e4*edge)
			}
		}
	}

	parity := make(map[string]*cycleAccumulator)
	if opts.ParityUnderlying != "" {
		terms, err := r.optionTerms(opts)
		if err != nil {
			return nil, err
		}
		for _, at := range ordered {
			for _, venue := range venueNames {
				for _, pair := range terms[venue] {
					call, callOK := quoteAt(markKey{venue, pair.call}, at)
					put, putOK := quoteAt(markKey{venue, pair.put}, at)
					forward, forwardOK := quoteAt(markKey{venue, opts.ParityUnderlying}, at)
					if !callOK || !putOK || !forwardOK {
						continue
					}
					if parity[venue] == nil {
						parity[venue] = newCycleAccumulator()
					}
					if !positiveCashflowQuotes(call, put, forward) {
						parity[venue].observeUndefined(at)
						continue
					}
					parity[venue].observe(at, parityEdgeBps(call, put, forward, pair.strike, feeFactor))
				}
			}
		}
	}

	basis := make(map[string]*cycleAccumulator)
	if opts.PerpSymbol != "" && opts.SpotSymbol != "" {
		for _, at := range ordered {
			for _, venue := range venueNames {
				perp, perpOK := quoteAt(markKey{venue, opts.PerpSymbol}, at)
				spot, spotOK := quoteAt(markKey{venue, opts.SpotSymbol}, at)
				if !perpOK || !spotOK {
					continue
				}
				if basis[venue] == nil {
					basis[venue] = newCycleAccumulator()
				}
				if !positiveCashflowQuotes(perp, spot) {
					basis[venue].observeUndefined(at)
					continue
				}
				// The edge of buying spot and selling the perpetual, after
				// paying both spreads and both fees. It is a carry, not an
				// arbitrage: holding it costs or earns funding.
				edge := float64(perp.bid)*feeFactor/(float64(spot.ask)/feeFactor) - 1
				basis[venue].observe(at, 1e4*edge)
			}
		}
	}

	audit := &ArbitrageAudit{FeeBps: opts.TakerFeeBps, StalenessNanos: staleness}
	names := make([]string, 0, len(triangles))
	for venue := range triangles {
		names = append(names, venue)
	}
	sort.Strings(names)
	for _, venue := range names {
		audit.Cycles = append(audit.Cycles, triangles[venue].result("triangular "+venue))
	}
	names = names[:0]
	for venue := range parity {
		names = append(names, venue)
	}
	sort.Strings(names)
	for _, venue := range names {
		audit.Cycles = append(audit.Cycles, parity[venue].result("put_call_parity "+venue))
	}
	names = names[:0]
	for venue := range basis {
		names = append(names, venue)
	}
	sort.Strings(names)
	for _, venue := range names {
		audit.Cycles = append(audit.Cycles, basis[venue].result("perp_carry "+venue))
	}
	names = names[:0]
	for name := range crossVenue {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		audit.Cycles = append(audit.Cycles, crossVenue[name].result("cross_venue "+name))
	}
	return audit, nil
}

// positiveCashflowQuotes declares the current domain of the fee-aware
// arbitrage and carry statistics. A book can contain signed valid levels, but
// each present bid/ask must be strictly positive before a cash-return ratio is
// meaningful. This is a statistic/model rule, never a book-availability test.
func positiveCashflowQuotes(quotes ...quote) bool {
	for _, quote := range quotes {
		if quote.bid <= 0 || quote.ask <= 0 {
			return false
		}
	}
	return true
}

// triangleEdgeBps prices the round trip USD to base to cross-quote and back.
//
// Both directions are priced and the better one reported, because an
// arbitrage that exists only one way round is still an arbitrage, and taking
// the maximum is the adversarial choice.
func triangleEdgeBps(base, quotePair, cross *quote, crossPrecision int64, feeFactor float64) float64 {
	scale := float64(crossPrecision)
	// Forward: buy the base for USD, sell it for the cross quote, sell that
	// for USD.
	forward := (1 / (float64(base.ask) / feeFactor)) * (float64(cross.bid) * feeFactor / scale) * (float64(quotePair.bid) * feeFactor)
	// Reverse: buy the cross quote for USD, buy the base with it, sell for USD.
	reverse := (1 / (float64(quotePair.ask) / feeFactor)) * (scale / (float64(cross.ask) / feeFactor)) * (float64(base.bid) * feeFactor)
	return 1e4 * (math.Max(forward, reverse) - 1)
}

// optionPair is a call and a put on the same strike and expiry.
type optionPair struct {
	call, put string
	strike    int64
}

// optionTerms finds every call and put sharing a strike and expiry, from the
// listings the venues announced.
func (r *Run) optionTerms(opts ArbitrageOptions) (map[string][]optionPair, error) {
	type listing struct {
		Symbol     string `json:"symbol"`
		Type       string `json:"instrument_type"`
		Strike     int64  `json:"strike"`
		IsCall     bool   `json:"is_call"`
		ExpiryNano int64  `json:"expiry_nano"`
	}
	type termKey struct {
		venue  string
		strike int64
		expiry int64
	}
	var mu sync.Mutex
	calls := make(map[termKey]string)
	puts := make(map[termKey]string)
	scan := ScanOptions{Events: []string{"instrument_listed"}, Files: opts.Files, FilesSelected: opts.FilesSelected}
	if err := r.Scan(scan, func(event Event) {
		var payload listing
		if event.Decode(&payload) != nil || payload.Type != "OPTION" || payload.Strike <= 0 {
			return
		}
		key := termKey{event.VenueID, payload.Strike, payload.ExpiryNano}
		mu.Lock()
		if payload.IsCall {
			calls[key] = payload.Symbol
		} else {
			puts[key] = payload.Symbol
		}
		mu.Unlock()
	}); err != nil {
		return nil, err
	}
	pairs := make(map[string][]optionPair)
	for key, call := range calls {
		put, ok := puts[key]
		if !ok {
			continue
		}
		pairs[key.venue] = append(pairs[key.venue], optionPair{call: call, put: put, strike: key.strike})
	}
	for venue := range pairs {
		sort.Slice(pairs[venue], func(i, j int) bool { return pairs[venue][i].call < pairs[venue][j].call })
	}
	return pairs, nil
}

// parityEdgeBps prices both directions of the put-call parity trade.
//
// A call less a put with the same strike and expiry is a forward at that
// strike. Buying the synthetic and selling the underlying, or the reverse,
// must not pay after fees. The underlying's spot is used as the forward, which
// ignores the cost of carrying the position to expiry, so a small persistent
// edge here is a financing cost rather than an arbitrage; a large one is not.
func parityEdgeBps(call, put, forward quote, strike int64, feeFactor float64) float64 {
	// Buy the synthetic long: pay the call's offer, receive the put's bid.
	syntheticCost := float64(call.ask)/feeFactor - float64(put.bid)*feeFactor + float64(strike)
	// Sell the underlying at its bid.
	sellUnderlying := float64(forward.bid) * feeFactor
	buySynthetic := sellUnderlying - syntheticCost

	// The reverse: sell the synthetic, buy the underlying.
	syntheticProceeds := float64(call.bid)*feeFactor - float64(put.ask)/feeFactor + float64(strike)
	buyUnderlying := float64(forward.ask) / feeFactor
	sellSynthetic := syntheticProceeds - buyUnderlying

	best := buySynthetic
	if sellSynthetic > best {
		best = sellSynthetic
	}
	return 1e4 * best / float64(forward.ask)
}
