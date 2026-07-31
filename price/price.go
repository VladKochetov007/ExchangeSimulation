package price

import ebook "exchange_sim/book"

type MarkPriceCalculator interface {
	Calculate(book *ebook.OrderBook) int64
}

// MidPriceProvider is an optional upgrade over BookProvider: a provider that
// mutates its books concurrently must compute the mid under its own lock,
// because a *OrderBook handed back by GetBook is read after that lock is
// released and its fields can change mid-read.
type MidPriceProvider interface {
	MidPrice(symbol string) int64
}

type BookProvider interface {
	GetBook(symbol string) *ebook.OrderBook
}

// median3 returns the median of three int64 values.
func median3(a, b, d int64) int64 {
	if a > b {
		a, b = b, a
	}
	if b > d {
		b, d = d, b
	}
	if a > b {
		b = a
	}
	_ = d
	return b
}
