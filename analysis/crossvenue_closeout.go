package analysis

import (
	"fmt"
	"math"
	"slices"

	etypes "exchange_sim/types"
)

// VenueLocalCloseoutInput describes one independently funded account's change
// from its starting balances and the displayed terminal book on that venue.
// Fees are quote-asset taker fees, rounded once per consumed price level.
type VenueLocalCloseoutInput struct {
	BaseDelta     int64
	QuoteDelta    int64
	BasePrecision int64
	TakerFeeBps   int64
	Bids          []etypes.PriceLevel
	Asks          []etypes.PriceLevel
}

// VenueLocalCloseout is a hypothetical liquidation value, not an executed
// transfer or trade. Value is defined only when the entire local position can
// be covered by the displayed side of the book.
type VenueLocalCloseout struct {
	Available     bool
	Reason        string
	RequiredQty   int64
	CoveredQty    int64
	GrossCashflow int64
	QuoteFees     int64
	Value         int64
}

// ValueVenueLocalInventory walks the relevant terminal side of a venue's
// displayed book. It does not assume hidden depth, cross-venue netting, or a
// midpoint. Callers must establish that the book is the registered terminal
// state and that no unaccounted borrowing or transfers occurred.
func ValueVenueLocalInventory(input VenueLocalCloseoutInput) VenueLocalCloseout {
	result := VenueLocalCloseout{}
	if input.BasePrecision <= 0 || input.TakerFeeBps < 0 {
		result.Reason = "INVALID_CONVENTION"
		return result
	}
	if input.BaseDelta == math.MinInt64 {
		result.Reason = "QUANTITY_OVERFLOW"
		return result
	}
	if input.BaseDelta == 0 {
		result.Available, result.Value = true, input.QuoteDelta
		return result
	}
	levels := input.Bids
	selling := input.BaseDelta > 0
	result.RequiredQty = input.BaseDelta
	if !selling {
		result.RequiredQty = -input.BaseDelta
		levels = input.Asks
	}
	if len(levels) == 0 {
		result.Reason = "NO_EXECUTABLE_SIDE"
		return result
	}
	ordered := slices.Clone(levels)
	slices.SortFunc(ordered, func(left, right etypes.PriceLevel) int {
		if selling {
			return compareInt64(right.Price, left.Price)
		}
		return compareInt64(left.Price, right.Price)
	})
	var priorPrice int64
	for index, level := range ordered {
		if level.Price <= 0 || level.VisibleQty < 0 || level.HiddenQty < 0 || index > 0 && level.Price == priorPrice {
			result.Reason = "INVALID_BOOK"
			return result
		}
		priorPrice = level.Price
		if level.VisibleQty == 0 || result.CoveredQty == result.RequiredQty {
			continue
		}
		remaining := result.RequiredQty - result.CoveredQty
		quantity := min(remaining, level.VisibleQty)
		notional, ok := etypes.TryMulDiv(quantity, level.Price, input.BasePrecision)
		if !ok {
			result.Reason = "ARITHMETIC_OVERFLOW"
			return result
		}
		fee, ok := etypes.TryMulBps(notional, input.TakerFeeBps)
		if !ok {
			result.Reason = "ARITHMETIC_OVERFLOW"
			return result
		}
		gross := notional
		if !selling {
			gross = -notional
		}
		result.GrossCashflow, ok = etypes.TryAdd(result.GrossCashflow, gross)
		if !ok {
			result.Reason = "ARITHMETIC_OVERFLOW"
			return result
		}
		result.QuoteFees, ok = etypes.TryAdd(result.QuoteFees, fee)
		if !ok {
			result.Reason = "ARITHMETIC_OVERFLOW"
			return result
		}
		result.CoveredQty += quantity
	}
	if result.CoveredQty != result.RequiredQty {
		result.Reason = "INSUFFICIENT_VISIBLE_DEPTH"
		return result
	}
	cashflow, ok := etypes.TrySub(result.GrossCashflow, result.QuoteFees)
	if !ok {
		result.Reason = "ARITHMETIC_OVERFLOW"
		return result
	}
	result.Value, ok = etypes.TryAdd(input.QuoteDelta, cashflow)
	if !ok {
		result.Reason = "ARITHMETIC_OVERFLOW"
		return result
	}
	result.Available = true
	return result
}

func compareInt64(left, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

// SumVenueLocalCloseouts refuses to treat one unavailable venue as zero. The
// inputs must be one account per venue, with no unaccounted financing flows.
func SumVenueLocalCloseouts(values map[string]VenueLocalCloseout) (int64, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("cross-venue closeout: no venue accounts")
	}
	venues := make([]string, 0, len(values))
	for venue := range values {
		venues = append(venues, venue)
	}
	slices.Sort(venues)
	var total int64
	for _, venue := range venues {
		value := values[venue]
		if !value.Available {
			return 0, fmt.Errorf("cross-venue closeout: %s unavailable: %s", venue, value.Reason)
		}
		var ok bool
		total, ok = etypes.TryAdd(total, value.Value)
		if !ok {
			return 0, fmt.Errorf("cross-venue closeout: portfolio value overflows")
		}
	}
	return total, nil
}
