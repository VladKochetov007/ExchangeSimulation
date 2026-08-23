package multivenue

import "exchange_sim/actor"

// LocalBookCache is one participant's read-only view of one declared public
// book feed. It deliberately stores copied top-of-book fields rather than an
// exchange or book pointer: a cache may advance only when its owner receives a
// public-feed event through its own gateway.
//
// This first V2-1 slice is single-source. A future composite must retain one
// cache per source and record a vector frontier before it may be used for a
// cross-venue information claim.
type LocalBookCache struct {
	sourceVenue string
	symbol      string

	bid, ask       int64
	bidQty, askQty int64
	sequence       uint64
	publishedAt    int64
	updates        uint64
	rejectedStale  uint64
}

// LocalBookView is a value copy of the locally received top of book.
type LocalBookView struct {
	SourceVenue string
	Symbol      string
	Bid, Ask    int64
	BidQty      int64
	AskQty      int64
	Sequence    uint64
	PublishedAt int64
	Updates     uint64
}

func NewLocalBookCache(sourceVenue, symbol string) *LocalBookCache {
	return &LocalBookCache{sourceVenue: sourceVenue, symbol: symbol}
}

// ObserveSnapshot admits a snapshot only if it is a two-sided observation of
// this cache's declared feed and does not move backwards in its source order.
// It performs no exchange read and has no clock of its own.
func (c *LocalBookCache) ObserveSnapshot(event actor.BookSnapshotEvent) bool {
	if c == nil || event.Symbol != c.symbol || event.Snapshot == nil ||
		len(event.Snapshot.Bids) == 0 || len(event.Snapshot.Asks) == 0 {
		return false
	}
	bid := event.Snapshot.Bids[0]
	ask := event.Snapshot.Asks[0]
	if bid.Price <= 0 || ask.Price <= bid.Price || bid.VisibleQty <= 0 || ask.VisibleQty <= 0 {
		return false
	}
	if (event.SeqNum != 0 && c.sequence != 0 && event.SeqNum <= c.sequence) ||
		(event.SeqNum == 0 && event.Timestamp < c.publishedAt) {
		c.rejectedStale++
		return false
	}
	c.bid, c.ask = bid.Price, ask.Price
	c.bidQty, c.askQty = bid.VisibleQty, ask.VisibleQty
	c.sequence, c.publishedAt = event.SeqNum, event.Timestamp
	c.updates++
	return true
}

func (c *LocalBookCache) View() (LocalBookView, bool) {
	if c == nil || c.bid <= 0 || c.ask <= c.bid {
		return LocalBookView{}, false
	}
	return LocalBookView{
		SourceVenue: c.sourceVenue,
		Symbol:      c.symbol,
		Bid:         c.bid, Ask: c.ask, BidQty: c.bidQty, AskQty: c.askQty,
		Sequence: c.sequence, PublishedAt: c.publishedAt, Updates: c.updates,
	}, true
}

func (c *LocalBookCache) Mid() (int64, bool) {
	view, ok := c.View()
	if !ok {
		return 0, false
	}
	return view.Bid + (view.Ask-view.Bid)/2, true
}

func (c *LocalBookCache) RejectedStale() uint64 {
	if c == nil {
		return 0
	}
	return c.rejectedStale
}
