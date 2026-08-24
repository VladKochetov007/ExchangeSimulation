package analysis

import "sync"

// Sweep describes how far individual taker orders reach into the book.
//
// The walkable fraction says what an order of a given size could consume; this
// says what submitted orders actually did. The two differ whenever orders are
// clamped to the visible touch before submission, and only this one bears on
// whether depth consumption is a live price channel in a given run.
type Sweep struct {
	// Orders is the number of taker orders that produced at least one fill.
	Orders int `json:"orders"`
	// MultiPrice is how many of them filled at more than one price.
	MultiPrice int `json:"multi_price"`
	// PricesPerOrder is the count of distinct fill prices per taker order.
	PricesPerOrder Distribution `json:"prices_per_order"`
	// SweepBps is the span between an order's best and worst fill price, in
	// basis points of its best. Zero for every order that never left the touch.
	SweepBps Distribution `json:"sweep_bps"`
	// SweepBpsWhenMulti restricts the span to orders that did move, so a
	// population of mostly-clamped orders does not hide the size of the ones
	// that were not.
	SweepBpsWhenMulti Distribution `json:"sweep_bps_when_multi"`
	// FillsPerOrder counts executions, not prices: several fills at one price
	// mean the order crossed several makers without moving. It is also the
	// conversion between an order-based rate and a trade-based horizon.
	FillsPerOrder Distribution `json:"fills_per_order"`
	// SweepTicksWhenMulti is the same span in ticks. A multi-price span below
	// one tick would mean the measurement is broken, which basis points hide.
	SweepTicksWhenMulti Distribution `json:"sweep_ticks_when_multi"`
	// ElapsedSecondsWhenMulti is the time between a multi-price order's first
	// and last fill. A genuine sweep is one crossing and takes no time; a large
	// elapsed value means the identifier was reused across separate crossings,
	// which would count a later repricing as if it were a walk down the book.
	ElapsedSecondsWhenMulti Distribution `json:"elapsed_seconds_when_multi"`
	// UndefinedBpsOrders counts orders whose signed price span cannot be
	// expressed as a percentage because it touches or crosses zero. Their
	// price-level and tick-span observations remain included.
	UndefinedBpsOrders int `json:"undefined_bps_orders"`
}

// MeanSpanBps is the expected span across all orders, including the zeros.
//
// This is the quantity to multiply when asking what sweeping contributes on
// average. The product of the multi-price rate and the conditional MEDIAN is
// not an estimator of anything: the conditional distribution is right-skewed,
// so its median understates its mean severalfold.
func (s *Sweep) MeanSpanBps() float64 { return s.SweepBps.Mean }

// MeasureSweep groups a book's executions by the order that crossed.
func (r *Run) MeasureSweep(opts BookShapeOptions) (*Sweep, error) {
	type orderFills struct {
		best, worst         int64
		firstSeen, lastSeen int64
		prices              map[int64]struct{}
		fills               int
	}
	// Order identifiers are allocated per venue and every venue starts at one,
	// so pooling books across venues would merge unrelated orders into
	// fictitious sweeps. Keying on the venue costs nothing and removes a trap
	// that a mistyped selection would otherwise spring silently.
	type orderKey struct {
		venue   string
		orderID uint64
	}
	var mu sync.Mutex
	byOrder := map[orderKey]*orderFills{}

	type payload struct {
		Price        int64  `json:"price"`
		TakerOrderID uint64 `json:"taker_order_id"`
	}
	err := r.Scan(ScanOptions{Events: []string{"Trade"}, Files: opts.Files, FilesSelected: true}, func(event Event) {
		var decoded payload
		if event.Decode(&decoded) != nil || decoded.TakerOrderID == 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		key := orderKey{venue: event.VenueID, orderID: decoded.TakerOrderID}
		entry := byOrder[key]
		if entry == nil {
			entry = &orderFills{
				best: decoded.Price, worst: decoded.Price,
				firstSeen: event.SimTS, lastSeen: event.SimTS,
				prices: map[int64]struct{}{},
			}
			byOrder[key] = entry
		}
		entry.prices[decoded.Price] = struct{}{}
		entry.fills++
		if decoded.Price < entry.best {
			entry.best = decoded.Price
		}
		if decoded.Price > entry.worst {
			entry.worst = decoded.Price
		}
		if event.SimTS < entry.firstSeen {
			entry.firstSeen = event.SimTS
		}
		if event.SimTS > entry.lastSeen {
			entry.lastSeen = event.SimTS
		}
	})
	if err != nil {
		return nil, err
	}

	sweep := &Sweep{Orders: len(byOrder)}
	var pricesPer, spans, spansWhenMulti, ticksWhenMulti, elapsedWhenMulti, fillsPer []float64
	for _, entry := range byOrder {
		pricesPer = append(pricesPer, float64(len(entry.prices)))
		fillsPer = append(fillsPer, float64(entry.fills))
		// The span is unsigned: an order's side is not needed to know how far
		// its fills were spread across prices.
		span := 0.0
		bpsDefined := entry.best > 0 && entry.worst > 0
		if bpsDefined {
			span = 10_000 * (float64(entry.worst) - float64(entry.best)) / float64(entry.best)
			spans = append(spans, span)
		} else {
			sweep.UndefinedBpsOrders++
		}
		if len(entry.prices) > 1 {
			sweep.MultiPrice++
			if bpsDefined {
				spansWhenMulti = append(spansWhenMulti, span)
			}
			if opts.TickSize > 0 {
				ticksWhenMulti = append(ticksWhenMulti, (float64(entry.worst)-float64(entry.best))/float64(opts.TickSize))
			}
			elapsedWhenMulti = append(elapsedWhenMulti, float64(entry.lastSeen-entry.firstSeen)/1e9)
		}
	}
	sweep.PricesPerOrder = Describe(pricesPer)
	sweep.SweepBps = Describe(spans)
	sweep.SweepBpsWhenMulti = Describe(spansWhenMulti)
	sweep.FillsPerOrder = Describe(fillsPer)
	sweep.SweepTicksWhenMulti = Describe(ticksWhenMulti)
	sweep.ElapsedSecondsWhenMulti = Describe(elapsedWhenMulti)
	return sweep, nil
}

// MultiPriceFraction is the share of taker orders that consumed more than one
// price level.
func (s *Sweep) MultiPriceFraction() float64 {
	if s.Orders == 0 {
		return 0
	}
	return float64(s.MultiPrice) / float64(s.Orders)
}
