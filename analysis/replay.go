package analysis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// ReplayedBook is a book reconstructed by sequential replay of a venue's log.
//
// It exists because the periodic snapshot is published once a second while
// several trades occur between publications, so any measurement referencing a
// trade to the last published book is stale by that many trades — and those
// trades are same-signed on average, so the staleness biases toward the
// trade's own direction. Replay gives the exact book at every event instead.
type ReplayedBook struct {
	bids map[int64]int64
	asks map[int64]int64
}

func NewReplayedBook() *ReplayedBook {
	return &ReplayedBook{bids: map[int64]int64{}, asks: map[int64]int64{}}
}

// Apply records a level's new total size. A delta carries the absolute size
// for a price, not a increment, and a size of zero removes the level.
func (b *ReplayedBook) Apply(side string, price, totalQty int64) {
	levels := b.asks
	if side == "BUY" {
		levels = b.bids
	}
	if totalQty <= 0 {
		delete(levels, price)
		return
	}
	levels[price] = totalQty
}

// Reset replaces the whole book, which is what a snapshot does.
func (b *ReplayedBook) Reset(bids, asks []bookLevel) {
	b.bids = make(map[int64]int64, len(bids))
	b.asks = make(map[int64]int64, len(asks))
	for _, level := range bids {
		b.bids[level.Price] += level.VisibleQty + level.HiddenQty
	}
	for _, level := range asks {
		b.asks[level.Price] += level.VisibleQty + level.HiddenQty
	}
}

func (b *ReplayedBook) BestBid() int64 { return extremePrice(b.bids, true) }
func (b *ReplayedBook) BestAsk() int64 { return extremePrice(b.asks, false) }

func extremePrice(levels map[int64]int64, highest bool) int64 {
	best := int64(0)
	for price, qty := range levels {
		if qty <= 0 {
			continue
		}
		if best == 0 || (highest && price > best) || (!highest && price < best) {
			best = price
		}
	}
	return best
}

// Mid is the midpoint, or zero when either side is empty.
func (b *ReplayedBook) Mid() int64 {
	bid, ask := b.BestBid(), b.BestAsk()
	if bid <= 0 || ask <= 0 {
		return 0
	}
	return (bid + ask) / 2
}

// sortedLevels returns one side's prices in the order a taker consumes them.
func (b *ReplayedBook) sortedLevels(buySide bool) []int64 {
	levels := b.asks
	if buySide {
		levels = b.bids
	}
	prices := make([]int64, 0, len(levels))
	for price, qty := range levels {
		if qty > 0 {
			prices = append(prices, price)
		}
	}
	// A buyer takes the lowest ask first; a seller takes the highest bid first.
	sort.Slice(prices, func(i, j int) bool {
		if buySide {
			return prices[i] > prices[j]
		}
		return prices[i] < prices[j]
	})
	return prices
}

// ConsumeCounterfactual reports where the best price on the consumed side would
// stand if qty were removed from it and no maker reacted.
//
// This is the mechanical part of impact, isolated: it is what the order alone
// does to the touch, before anybody requotes. It returns zero when the side is
// exhausted, which the caller must treat as unmeasurable rather than as a move
// to price zero.
func (b *ReplayedBook) ConsumeCounterfactual(takerBuys bool, qty int64) int64 {
	// A buying taker consumes asks; the levels it walks are the ask side.
	prices := b.sortedLevels(!takerBuys)
	levels := b.asks
	if !takerBuys {
		levels = b.bids
	}
	remaining := qty
	for _, price := range prices {
		if remaining < levels[price] {
			return price
		}
		remaining -= levels[price]
	}
	return 0
}

// replayEvent is the subset of a log record the replay needs, decoded once.
type replayEvent struct {
	SimTS int64  `json:"sim_ts"`
	Event string `json:"event"`
	Data  struct {
		Payload json.RawMessage `json:"payload"`
	} `json:"data"`
	ClientID uint64 `json:"client_id"`
}

type deltaPayload struct {
	Price      int64  `json:"price"`
	Side       string `json:"side"`
	TotalQty   int64  `json:"total_qty"`
	VisibleQty int64  `json:"visible_qty"`
	HiddenQty  int64  `json:"hidden_qty"`
}

// bestOf is the best price on one side of a published snapshot.
func bestOf(levels []bookLevel, highest bool) int64 {
	best := int64(0)
	for _, level := range levels {
		if level.VisibleQty+level.HiddenQty <= 0 {
			continue
		}
		if best == 0 || (highest && level.Price > best) || (!highest && level.Price < best) {
			best = level.Price
		}
	}
	return best
}

type tradePayload struct {
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	Side         string `json:"side"`
	TakerOrderID uint64 `json:"taker_order_id"`
}

// ReplayVisitor is called for each trade, with the book as it stood before it.
type ReplayVisitor func(ts int64, trade tradePayload, book *ReplayedBook)

// AcceptVisitor is called for each accepted order, with the book as it stood
// when the order arrived, which is what its price must be measured against.
type AcceptVisitor func(ts int64, accepted acceptedPayload, book *ReplayedBook)

type acceptedPayload struct {
	OrderID  uint64 `json:"order_id"`
	ClientID uint64 `json:"client_id"`
	Side     string `json:"side"`
	Price    int64  `json:"price"`
	Qty      int64  `json:"qty"`
	Type     string `json:"type"`
}

// ReplayDrift counts how often the reconstructed book disagreed with a
// published snapshot, which is the check that the replay is faithful.
type ReplayDrift struct {
	Checks     int `json:"checks"`
	Mismatches int `json:"mismatches"`
}

// ReplayFile walks one book log in order, maintaining the book and calling
// visit before each trade is applied.
//
// The file is read sequentially and never concurrently: the whole method
// depends on event order, and the log's order is the matcher's order.
func ReplayFile(path string, visit ReplayVisitor) (*ReplayDrift, error) {
	return ReplayFileWith(path, visit, nil)
}

// ReplayFileWith is ReplayFile with an additional visitor for accepted orders.
func ReplayFileWith(path string, visit ReplayVisitor, onAccept AcceptVisitor) (*ReplayDrift, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	book := NewReplayedBook()
	drift := &ReplayDrift{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<26)
	for scanner.Scan() {
		var event replayEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		switch event.Event {
		case "BookDelta":
			var delta deltaPayload
			if json.Unmarshal(event.Data.Payload, &delta) != nil {
				continue
			}
			// A hidden order changes a level without publishing a delta, so a
			// book containing any would drift silently. No actor in this
			// simulation places one; the replay refuses to guess rather than
			// relying on that staying true.
			if delta.HiddenQty != 0 || (delta.TotalQty != 0 && delta.VisibleQty != delta.TotalQty) {
				return nil, fmt.Errorf("replay %s: hidden depth at price %d (total %d, visible %d); "+
					"book mutations from hidden orders are not published and the reconstruction would drift",
					path, delta.Price, delta.TotalQty, delta.VisibleQty)
			}
			book.Apply(delta.Side, delta.Price, delta.TotalQty)
		case "Trade":
			var trade tradePayload
			if json.Unmarshal(event.Data.Payload, &trade) == nil && visit != nil {
				visit(event.SimTS, trade, book)
			}
		case "OrderAccepted":
			if onAccept == nil {
				continue
			}
			var accepted acceptedPayload
			if json.Unmarshal(event.Data.Payload, &accepted) == nil {
				onAccept(event.SimTS, accepted, book)
			}
		case "BookSnapshot":
			if event.ClientID != 0 {
				continue
			}
			var snapshot bookSnapshot
			if json.Unmarshal(event.Data.Payload, &snapshot) != nil {
				continue
			}
			// The snapshot is the engine's own view. Comparing the replayed
			// touch against it is the only way to learn that some book mutation
			// does not emit a delta, which would let the reconstruction drift
			// silently for the rest of the run.
			// Compare, never resynchronise. Resetting from the snapshot would
			// repair any divergence before the next check could see it, so the
			// check would pass however broken the replay was; it would also
			// truncate the book to the snapshot's depth limit permanently.
			drift.Checks++
			if book.BestBid() != bestOf(snapshot.Bids, true) || book.BestAsk() != bestOf(snapshot.Asks, false) {
				drift.Mismatches++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return drift, nil
}

// MismatchRate is the share of snapshots at which the replayed touch differed
// from the published one.
func (d *ReplayDrift) MismatchRate() float64 {
	if d.Checks == 0 {
		return 0
	}
	return float64(d.Mismatches) / float64(d.Checks)
}
