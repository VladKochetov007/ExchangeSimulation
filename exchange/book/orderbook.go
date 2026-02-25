package book

import etypes "exchange_sim/exchange/types"

// OrderBook is the full two-sided order book for a symbol.
type OrderBook struct {
	Symbol     string
	Instrument etypes.Instrument
	Bids       *Book
	Asks       *Book
	LastTrade  *etypes.Trade
	SeqNum     uint64
}

func (ob *OrderBook) GetLastPrice() int64 {
	if ob.LastTrade != nil {
		return ob.LastTrade.Price
	}
	return 0
}

func (ob *OrderBook) GetBestBid() int64 {
	if ob.Bids.Best != nil {
		return ob.Bids.Best.Price
	}
	return 0
}

func (ob *OrderBook) GetBestAsk() int64 {
	if ob.Asks.Best != nil {
		return ob.Asks.Best.Price
	}
	return 0
}

// GetMidPrice returns the mid price between best bid and ask,
// falling back to last price if the book is empty.
func (ob *OrderBook) GetMidPrice() int64 {
	bestBid := ob.GetBestBid()
	bestAsk := ob.GetBestAsk()
	if bestBid > 0 && bestAsk > 0 {
		return bestBid + (bestAsk-bestBid)/2
	}
	return ob.GetLastPrice()
}

// FindOrder searches both sides for an order by ID.
func (ob *OrderBook) FindOrder(orderID uint64) *etypes.Order {
	if o := ob.Bids.Orders[orderID]; o != nil {
		return o
	}
	return ob.Asks.Orders[orderID]
}
