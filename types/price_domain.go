package types

// PriceSignPolicy declares which signed numeric prices an instrument admits.
// It constrains an instrument's economic contract; it never represents price
// availability. A numeric price is available only when the API carrying it
// returned no error.
type PriceSignPolicy uint8

const (
	// PositivePrice requires price > 0. Spot and the current crypto perpetual
	// contracts retain this policy.
	PositivePrice PriceSignPolicy = iota
	// NonNegativePrice permits a zero price but rejects negative prices. Option
	// premiums use this policy: an out-of-the-money contract can trade at zero.
	NonNegativePrice
	// SignedPrice permits negative, zero, and positive prices. It is intended
	// for contracts such as commodity dated futures whose economic specification
	// permits settlement below zero.
	SignedPrice
)

// PriceDomain is an explicit instrument-level price admission contract. Tick
// alignment is evaluated after the sign policy. Invalid tick sizes admit no
// prices, rather than risking a modulo-by-zero panic during request handling.
//
// PriceDomain deliberately does not encode absence. A zero price can be valid
// under NonNegativePrice and SignedPrice; unavailable prices are reported by
// the producing API with ErrNoPrice.
type PriceDomain struct {
	signPolicy PriceSignPolicy
	tickSize   int64
}

// PositivePriceDomain constructs a strictly-positive tick-aligned domain.
func PositivePriceDomain(tickSize int64) PriceDomain {
	return PriceDomain{signPolicy: PositivePrice, tickSize: tickSize}
}

// NonNegativePriceDomain constructs a zero-or-positive tick-aligned domain.
func NonNegativePriceDomain(tickSize int64) PriceDomain {
	return PriceDomain{signPolicy: NonNegativePrice, tickSize: tickSize}
}

// SignedPriceDomain constructs a tick-aligned domain spanning signed prices.
func SignedPriceDomain(tickSize int64) PriceDomain {
	return PriceDomain{signPolicy: SignedPrice, tickSize: tickSize}
}

// Validate reports whether price is admissible for this instrument domain.
func (d PriceDomain) Validate(price int64) bool {
	if d.tickSize <= 0 || price%d.tickSize != 0 {
		return false
	}
	switch d.signPolicy {
	case PositivePrice:
		return price > 0
	case NonNegativePrice:
		return price >= 0
	case SignedPrice:
		return true
	default:
		return false
	}
}

// TickSize returns the domain's required price increment.
func (d PriceDomain) TickSize() int64 { return d.tickSize }

// SignPolicy returns the domain's signed-value policy.
func (d PriceDomain) SignPolicy() PriceSignPolicy { return d.signPolicy }

// Midpoint returns (a+b)/2 with Go integer-division rounding semantics
// (truncation toward zero), without overflowing for any pair of int64 inputs.
//
// The identity (a&b)+((a^b)>>1) is the floor of the mathematical mean under
// two's-complement arithmetic and cannot overflow. For a negative odd sum,
// Go division truncates upward toward zero, so the result is adjusted by one.
// The odd-sum predicate is exactly the low bit of a^b; adding one is safe
// because a negative odd mean cannot already equal MaxInt64.
func Midpoint(a, b int64) int64 {
	floorMean := (a & b) + ((a ^ b) >> 1)
	if floorMean < 0 && (a^b)&1 != 0 {
		return floorMean + 1
	}
	return floorMean
}
