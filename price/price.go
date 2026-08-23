package price

import (
	"fmt"

	ebook "exchange_sim/book"
	etypes "exchange_sim/types"
)

type MarkPriceCalculator interface {
	Calculate(book *ebook.OrderBook) (int64, error)
}

// MidPriceProvider is an optional upgrade over BookProvider: a provider that
// mutates its books concurrently must compute the mid under its own lock,
// because a *OrderBook handed back by GetBook is read after that lock is
// released and its fields can change mid-read.
type MidPriceProvider interface {
	MidPrice(symbol string) (int64, error)
}

// LockedMidPriceProvider is implemented by a provider that can safely answer
// while its own book lock is already held by the caller. It exists solely for
// exchange-internal index calculation: calling MidPrice in that situation can
// deadlock an RWMutex once a writer is queued. External callers must use the
// ordinary MidPrice path.
type LockedMidPriceProvider interface {
	MidPriceLocked(symbol string) (int64, error)
}

type BookProvider interface {
	GetBook(symbol string) *ebook.OrderBook
}

func sourcePrice(source etypes.PriceSource, symbol string) (int64, error) {
	if source == nil {
		return 0, fmt.Errorf("price source for %s: %w", symbol, etypes.ErrNoPrice)
	}
	price, err := source.Price(symbol)
	if err != nil {
		return 0, fmt.Errorf("price source for %s: %w", symbol, err)
	}
	if price <= 0 {
		return 0, fmt.Errorf("price source for %s returned non-positive price: %w", symbol, etypes.ErrNoPrice)
	}
	return price, nil
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
