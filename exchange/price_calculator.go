package exchange

// MarkPriceCalculator calculates the mark price from an order book
type MarkPriceCalculator interface {
	Calculate(book *OrderBook) int64
}

// LastPriceCalculator uses the last trade price as mark price
// Simplest but can be manipulated by wash trading
type LastPriceCalculator struct{}

func NewLastPriceCalculator() *LastPriceCalculator {
	return &LastPriceCalculator{}
}

func (c *LastPriceCalculator) Calculate(book *OrderBook) int64 {
	return book.GetLastPrice()
}

// MidPriceCalculator uses the mid price between best bid and ask
// Standard approach, more resistant to manipulation
type MidPriceCalculator struct{}

func NewMidPriceCalculator() *MidPriceCalculator {
	return &MidPriceCalculator{}
}

func (c *MidPriceCalculator) Calculate(book *OrderBook) int64 {
	return book.GetMidPrice()
}

// WeightedMidPriceCalculator uses quantity-weighted mid price
// Weights by available quantity at best levels
type WeightedMidPriceCalculator struct{}

func NewWeightedMidPriceCalculator() *WeightedMidPriceCalculator {
	return &WeightedMidPriceCalculator{}
}

func (c *WeightedMidPriceCalculator) Calculate(book *OrderBook) int64 {
	if book.Bids.Best == nil || book.Asks.Best == nil {
		return book.GetLastPrice()
	}

	bidQty := book.Bids.Best.TotalQty
	askQty := book.Asks.Best.TotalQty
	bidPrice := book.Bids.Best.Price
	askPrice := book.Asks.Best.Price

	if bidQty == 0 && askQty == 0 {
		return bidPrice + (askPrice-bidPrice)/2
	}

	if bidQty == 0 {
		return askPrice
	}

	if askQty == 0 {
		return bidPrice
	}

	// Weighted by inverse of quantity (more weight to thicker side)
	totalWeight := bidQty + askQty
	return (bidPrice*askQty + askPrice*bidQty) / totalWeight
}
