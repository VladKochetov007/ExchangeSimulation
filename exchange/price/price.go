package price

import ebook "exchange_sim/exchange/book"

// MarkPriceCalculator calculates the mark price from an order book.
type MarkPriceCalculator interface {
	Calculate(book *ebook.OrderBook) int64
}

// BookProvider provides read access to order books by symbol.
// *exchange.Exchange satisfies this interface via GetBook.
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
