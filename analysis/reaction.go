package analysis

import (
	"math"
	"sort"
	"sync"
)

// Delivered latency and maker adverse selection.
//
// The latency arm's preregistration asks whether opportunities last longer and
// whether makers are picked off more, and the scoring rule asks that realized
// latency be measured rather than assumed from the configuration multiplier.
//
// The logs do not carry a send timestamp beside a receive timestamp, so
// end-to-end latency cannot be read off them directly. What can be measured is
// the delivered consequence of latency: how long after a book changes the
// first reacting order arrives. That is a behavioural measure and it is
// labelled as one -- it bounds delivered latency from above and moves with it,
// but it also contains each actor's own decision interval, so it is not a
// clean latency estimate and is never reported as one.

// ReactionOptions selects inputs.
type ReactionOptions struct {
	Files         []string
	FilesSelected bool
	// HorizonSeconds is how far ahead of a maker's fill the underlying is
	// followed when measuring adverse selection.
	HorizonSeconds float64
	// MaxReactionSeconds bounds the reaction search; a book with no order
	// within it contributes no observation rather than a huge one.
	MaxReactionSeconds float64
}

// ReactionStats is the delivered reaction lag on one book.
type ReactionStats struct {
	VenueID      string  `json:"venue_id"`
	Symbol       string  `json:"symbol"`
	Observations int     `json:"observations"`
	MeanSeconds  float64 `json:"mean_seconds"`
	P50Seconds   float64 `json:"p50_seconds"`
	P90Seconds   float64 `json:"p90_seconds"`
	MinSeconds   float64 `json:"min_seconds"`
}

// AdverseSelection is how a maker's fills look after the fact.
type AdverseSelection struct {
	VenueID string `json:"venue_id"`
	Role    string `json:"role"`
	Fills   int    `json:"fills"`
	// MeanMarkoutBps is the signed move of the trade price against the maker
	// over the horizon, in basis points: positive means the maker was picked
	// off, negative means the maker earned the spread and more.
	MeanMarkoutBps float64 `json:"mean_markout_bps"`
	// PickedOffShare is the fraction of fills with a positive markout.
	PickedOffShare float64 `json:"picked_off_share"`
}

// Reaction is the whole measurement.
type Reaction struct {
	Books []ReactionStats `json:"books"`
	// PooledP50Seconds and PooledMeanSeconds pool every book's observations.
	PooledMeanSeconds float64            `json:"pooled_mean_seconds"`
	PooledP50Seconds  float64            `json:"pooled_p50_seconds"`
	PooledMinSeconds  float64            `json:"pooled_min_seconds"`
	Observations      int                `json:"observations"`
	Adverse           []AdverseSelection `json:"adverse_selection"`
	PooledMarkoutBps  float64            `json:"pooled_markout_bps"`
	// UndefinedMarkouts counts otherwise valid maker-fill horizons whose
	// relative-return denominator or terminal price is non-positive. The
	// signed prices remain in the tape; this percentage statistic is simply
	// undefined across zero and is never abs-normalized.
	UndefinedMarkouts int `json:"undefined_markouts"`
}

// MeasureReaction computes delivered reaction lag and maker markouts.
func (r *Run) MeasureReaction(opts ReactionOptions) (*Reaction, error) {
	horizon := opts.HorizonSeconds
	if horizon <= 0 {
		horizon = 60
	}
	maxReaction := opts.MaxReactionSeconds
	if maxReaction <= 0 {
		maxReaction = 30
	}

	type bookEvent struct {
		at      int64
		isOrder bool
		client  uint64
	}
	type tradeEvent struct {
		at       int64
		price    int64
		makerID  uint64
		takerBuy bool
	}
	var mu sync.Mutex
	books := make(map[markKey][]bookEvent)
	trades := make(map[markKey][]tradeEvent)
	// Maker side per fill, recovered from the fill stream rather than guessed
	// from the trade's aggressor flag.
	type makerFill struct {
		at     int64
		price  int64
		buy    bool
		client uint64
	}
	makerFills := make(map[markKey][]makerFill)

	type tradePayload struct {
		Price int64  `json:"price"`
		Qty   int64  `json:"qty"`
		Side  string `json:"side"`
	}
	type fillPayload struct {
		Price int64  `json:"price"`
		Qty   int64  `json:"qty"`
		Side  string `json:"side"`
		Role  string `json:"role"`
	}

	scan := ScanOptions{
		Events:        []string{"BookDelta", "OrderAccepted", "Trade", "OrderFill"},
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		key := markKey{event.VenueID, event.Symbol}
		switch event.Name {
		case "BookDelta":
			mu.Lock()
			books[key] = append(books[key], bookEvent{at: event.SimTS})
			mu.Unlock()
		case "OrderAccepted":
			mu.Lock()
			books[key] = append(books[key], bookEvent{at: event.SimTS, isOrder: true, client: event.ClientID})
			mu.Unlock()
		case "Trade":
			var payload tradePayload
			if event.Decode(&payload) != nil {
				return
			}
			mu.Lock()
			trades[key] = append(trades[key], tradeEvent{at: event.SimTS, price: payload.Price, takerBuy: payload.Side == "BUY"})
			mu.Unlock()
		case "OrderFill":
			var payload fillPayload
			if event.Decode(&payload) != nil || payload.Role != "maker" {
				return
			}
			mu.Lock()
			makerFills[key] = append(makerFills[key], makerFill{
				at: event.SimTS, price: payload.Price, buy: payload.Side == "BUY", client: event.ClientID,
			})
			mu.Unlock()
		}
	}); err != nil {
		return nil, err
	}

	result := &Reaction{}
	keys := make([]markKey, 0, len(books))
	for key := range books {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].venue != keys[j].venue {
			return keys[i].venue < keys[j].venue
		}
		return keys[i].symbol < keys[j].symbol
	})

	var pooled []float64
	for _, key := range keys {
		events := books[key]
		sort.Slice(events, func(i, j int) bool {
			if events[i].at != events[j].at {
				return events[i].at < events[j].at
			}
			// A change is observed before anything can react to it, so at an
			// identical instant the change sorts first.
			return !events[i].isOrder && events[j].isOrder
		})
		var lags []float64
		for i, event := range events {
			if event.isOrder {
				continue
			}
			for j := i + 1; j < len(events); j++ {
				if !events[j].isOrder {
					continue
				}
				lag := float64(events[j].at-event.at) / 1e9
				if lag <= maxReaction {
					lags = append(lags, lag)
				}
				break
			}
		}
		if len(lags) == 0 {
			continue
		}
		sort.Float64s(lags)
		stats := ReactionStats{
			VenueID: key.venue, Symbol: key.symbol, Observations: len(lags),
			P50Seconds: lags[len(lags)/2],
			P90Seconds: lags[int(float64(len(lags))*0.9)],
			MinSeconds: lags[0],
		}
		sum := 0.0
		for _, lag := range lags {
			sum += lag
		}
		stats.MeanSeconds = sum / float64(len(lags))
		result.Books = append(result.Books, stats)
		pooled = append(pooled, lags...)
	}
	if len(pooled) > 0 {
		sort.Float64s(pooled)
		sum := 0.0
		for _, lag := range pooled {
			sum += lag
		}
		result.Observations = len(pooled)
		result.PooledMeanSeconds = sum / float64(len(pooled))
		result.PooledP50Seconds = pooled[len(pooled)/2]
		result.PooledMinSeconds = pooled[0]
	}

	// Adverse selection: follow the trade price on the same book for the
	// horizon after each maker fill, and sign the move against the maker.
	for key := range trades {
		series := trades[key]
		sort.Slice(series, func(i, j int) bool { return series[i].at < series[j].at })
		trades[key] = series
	}
	type adverseKey struct {
		venue string
		role  string
	}
	type adverseAcc struct {
		fills     int
		sumBps    float64
		pickedOff int
	}
	accumulators := make(map[adverseKey]*adverseAcc)
	for key, fills := range makerFills {
		series := trades[key]
		if len(series) == 0 {
			continue
		}
		for _, fill := range fills {
			target := fill.at + int64(horizon*1e9)
			index := sort.Search(len(series), func(i int) bool { return series[i].at >= target })
			if index >= len(series) {
				continue
			}
			future := float64(series[index].price)
			if fill.price <= 0 || series[index].price <= 0 {
				result.UndefinedMarkouts++
				continue
			}
			move := (future - float64(fill.price)) / float64(fill.price) * 10000
			// A maker who bought loses when the price falls; a maker who sold
			// loses when it rises. Markout is stated as a loss to the maker.
			if fill.buy {
				move = -move
			}
			role := r.Role(key.venue, fill.client)
			accKey := adverseKey{key.venue, role}
			acc := accumulators[accKey]
			if acc == nil {
				acc = &adverseAcc{}
				accumulators[accKey] = acc
			}
			acc.fills++
			acc.sumBps += move
			if move > 0 {
				acc.pickedOff++
			}
		}
	}
	adverseKeys := make([]adverseKey, 0, len(accumulators))
	for key := range accumulators {
		adverseKeys = append(adverseKeys, key)
	}
	sort.Slice(adverseKeys, func(i, j int) bool {
		if adverseKeys[i].venue != adverseKeys[j].venue {
			return adverseKeys[i].venue < adverseKeys[j].venue
		}
		return adverseKeys[i].role < adverseKeys[j].role
	})
	var totalFills int
	var totalBps float64
	for _, key := range adverseKeys {
		acc := accumulators[key]
		row := AdverseSelection{
			VenueID: key.venue, Role: key.role, Fills: acc.fills,
			MeanMarkoutBps: acc.sumBps / float64(acc.fills),
			PickedOffShare: float64(acc.pickedOff) / float64(acc.fills),
		}
		result.Adverse = append(result.Adverse, row)
		totalFills += acc.fills
		totalBps += acc.sumBps
	}
	if totalFills > 0 {
		result.PooledMarkoutBps = totalBps / float64(totalFills)
	}
	if math.IsNaN(result.PooledMarkoutBps) {
		result.PooledMarkoutBps = 0
	}
	return result, nil
}
