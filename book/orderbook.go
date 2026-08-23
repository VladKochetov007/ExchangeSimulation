package book

import etypes "exchange_sim/types"

// OrderBook is the full two-sided order book for a symbol.
type OrderBook struct {
	Symbol     string
	Instrument etypes.Instrument
	Bids       *Book
	Asks       *Book
	LastTrade  *etypes.Trade
	SeqNum     uint64
}

func (ob *OrderBook) GetLastPrice() (int64, error) {
	if ob != nil && ob.LastTrade != nil && ob.LastTrade.Price > 0 {
		return ob.LastTrade.Price, nil
	}
	return 0, etypes.ErrNoPrice
}

func (ob *OrderBook) GetBestBid() (int64, error) {
	if ob != nil && ob.Bids != nil && ob.Bids.Best != nil && ob.Bids.Best.Price > 0 {
		return ob.Bids.Best.Price, nil
	}
	return 0, etypes.ErrNoPrice
}

func (ob *OrderBook) GetBestAsk() (int64, error) {
	if ob != nil && ob.Asks != nil && ob.Asks.Best != nil && ob.Asks.Best.Price > 0 {
		return ob.Asks.Best.Price, nil
	}
	return 0, etypes.ErrNoPrice
}

// GetMidPrice returns the true midpoint between the current executable best
// bid and ask. It deliberately does not fall back to a last trade or a
// one-sided quote: callers requiring either must select that policy by name.
func (ob *OrderBook) GetMidPrice() (int64, error) {
	bestBid, err := ob.GetBestBid()
	if err != nil {
		return 0, err
	}
	bestAsk, err := ob.GetBestAsk()
	if err != nil {
		return 0, err
	}
	if bestBid > bestAsk {
		return 0, etypes.ErrNoPrice
	}
	return bestBid + (bestAsk-bestBid)/2, nil
}

// FindOrder searches both sides for an order by ID.
func (ob *OrderBook) FindOrder(orderID uint64) *etypes.Order {
	if o := ob.Bids.Orders[orderID]; o != nil {
		return o
	}
	return ob.Asks.Orders[orderID]
}
