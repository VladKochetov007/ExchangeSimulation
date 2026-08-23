package price

import (
	"fmt"

	etypes "exchange_sim/types"
)

// MidPriceOracle derives the price for a symbol from the mid price of a mapped book.
type MidPriceOracle struct {
	provider  BookProvider
	symbolMap map[string]string
}

func NewMidPriceOracle(provider BookProvider) *MidPriceOracle {
	return &MidPriceOracle{
		provider:  provider,
		symbolMap: make(map[string]string),
	}
}

func (o *MidPriceOracle) MapSymbol(from, to string) {
	o.symbolMap[from] = to
}

func (o *MidPriceOracle) Price(symbol string) (int64, error) {
	mapped, err := o.mappedSymbol(symbol)
	if err != nil {
		return 0, err
	}
	// Prefer the lock-safe path: reading a live book returned by GetBook
	// races every order that mutates it.
	if mp, ok := o.provider.(MidPriceProvider); ok {
		price, err := mp.MidPrice(mapped)
		if err != nil {
			return 0, fmt.Errorf("mid-price source %s for %s: %w", mapped, symbol, err)
		}
		return price, nil
	}
	return o.priceFromBook(mapped, symbol)
}

// PriceWithProviderLockHeld returns a price while the provider's own book
// lock is held by the caller. It is deliberately separate from Price: only
// the exchange's locked index path may use it, so ordinary calls retain the
// lock-safe MidPrice behavior.
func (o *MidPriceOracle) PriceWithProviderLockHeld(symbol string) (int64, error) {
	mapped, err := o.mappedSymbol(symbol)
	if err != nil {
		return 0, err
	}
	if mp, ok := o.provider.(LockedMidPriceProvider); ok {
		price, err := mp.MidPriceLocked(mapped)
		if err != nil {
			return 0, fmt.Errorf("locked mid-price source %s for %s: %w", mapped, symbol, err)
		}
		return price, nil
	}
	// A provider without an explicit lock-held API cannot be safely assumed to
	// share the caller's lock, so preserve the normal lock-safe route.
	return o.Price(symbol)
}

func (o *MidPriceOracle) mappedSymbol(symbol string) (string, error) {
	mapped := o.symbolMap[symbol]
	if mapped == "" {
		return "", fmt.Errorf("mid-price mapping for %s: %w", symbol, etypes.ErrNoPrice)
	}
	return mapped, nil
}

func (o *MidPriceOracle) priceFromBook(mapped, symbol string) (int64, error) {
	book := o.provider.GetBook(mapped)
	if book == nil {
		return 0, fmt.Errorf("mid-price book %s for %s: %w", mapped, symbol, etypes.ErrNoPrice)
	}
	price, err := book.GetMidPrice()
	if err != nil {
		return 0, fmt.Errorf("mid-price book %s for %s: %w", mapped, symbol, err)
	}
	return price, nil
}

type StaticPriceOracle struct {
	prices map[string]int64
}

func NewStaticPriceOracle(prices map[string]int64) *StaticPriceOracle {
	return &StaticPriceOracle{prices: prices}
}

func (o *StaticPriceOracle) Price(symbol string) (int64, error) {
	price, ok := o.prices[symbol]
	if !ok || price <= 0 {
		return 0, fmt.Errorf("static price for %s: %w", symbol, etypes.ErrNoPrice)
	}
	return price, nil
}
