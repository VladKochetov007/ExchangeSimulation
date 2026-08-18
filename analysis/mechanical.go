package analysis

import "math"

// MechanicalImpact decomposes the price response of an aggressive order into
// the part the order itself removed from the book and the part the makers
// added by requoting.
//
// It is built to avoid the two traps the earlier attempts fell into. Comparing
// arms that differ in the maker's inventory limit varies the very quantity
// under test. Conditioning on whether an order swept several levels selects on
// the causal path from inventory to price, because an order sweeps exactly
// when the touch is thin and the touch is thin when makers have skewed away.
// Here nothing endogenous is conditioned on: every order enters, and an order
// that never exhausted the touch simply contributes a mechanical move of zero.
type MechanicalImpact struct {
	Orders int `json:"orders"`
	// ZeroMechanical is how many orders left the touch where they found it.
	ZeroMechanical int         `json:"zero_mechanical"`
	Drift          ReplayDrift `json:"drift"`

	// MeanMechanicalBps and MeanActualBps are averages over all orders,
	// signed by the aggressor. Their ratio is the decomposition, and unlike a
	// regression it does not depend on the shape of a distribution that is
	// mostly a point mass at zero.
	MeanMechanicalBps float64 `json:"mean_mechanical_bps"`
	MeanActualBps     float64 `json:"mean_actual_bps"`
	MechanicalShare   float64 `json:"mechanical_share"`

	// Slope and R2 regress the realised response on the mechanical one over
	// the orders that moved the touch. Pure depth consumption with no
	// replenishment predicts a slope of one; replenishment pulls it below one
	// and requoting adds variance the mechanical term cannot explain.
	MovedOrders            int     `json:"moved_orders"`
	Slope                  float64 `json:"slope"`
	Intercept              float64 `json:"intercept"`
	R2                     float64 `json:"r2"`
	MovedMeanMechanicalBps float64 `json:"moved_mean_mechanical_bps"`
	MovedMeanActualBps     float64 `json:"moved_mean_actual_bps"`

	// UnmeasurableOrders were dropped: the horizon ran past the log, a side was
	// empty, or the order exhausted the visible side so the counterfactual has
	// no price. Reported rather than silently excluded.
	UnmeasurableOrders int `json:"unmeasurable_orders"`
}

// minRegressionSample is the smallest number of touch-moving orders from which
// a slope is worth quoting.
const minRegressionSample = 30

// MechanicalOptions configures the decomposition.
type MechanicalOptions struct {
	// HorizonTrades is how many trades after an order's last fill the realised
	// response is read. It must be long enough for makers to requote, which in
	// this simulator happens on a quote interval measured in seconds.
	HorizonTrades int
}

// orderReplay accumulates one aggressive order during the walk.
type orderReplay struct {
	firstTrade int
	lastTrade  int
	preMid     int64
	buys       bool
	qty        int64
	// counterfactualBest is the best price on the consumed side after removing
	// the order's quantity, computed against the book as it stood before the
	// order began.
	preBid, preAsk int64
	bookAtStart    *ReplayedBook
}

// MeasureMechanicalImpact replays one book and decomposes each order's impact.
func MeasureMechanicalImpact(path string, opts MechanicalOptions) (*MechanicalImpact, error) {
	horizon := opts.HorizonTrades
	if horizon < 1 {
		horizon = 10
	}

	var midBeforeTrade []int64
	var orders []*orderReplay
	byID := map[uint64]*orderReplay{}
	tradeIndex := 0

	drift, err := ReplayFile(path, func(_ int64, trade tradePayload, book *ReplayedBook) {
		midBeforeTrade = append(midBeforeTrade, book.Mid())
		existing := byID[trade.TakerOrderID]
		if existing == nil {
			existing = &orderReplay{
				firstTrade: tradeIndex,
				preMid:     book.Mid(),
				buys:       trade.Side == "BUY",
				preBid:     book.BestBid(),
				preAsk:     book.BestAsk(),
				bookAtStart: &ReplayedBook{
					bids: copyLevels(book.bids),
					asks: copyLevels(book.asks),
				},
			}
			byID[trade.TakerOrderID] = existing
			orders = append(orders, existing)
		}
		existing.lastTrade = tradeIndex
		existing.qty += trade.Qty
		tradeIndex++
	})
	if err != nil {
		return nil, err
	}

	result := &MechanicalImpact{Drift: *drift}
	var mechanical, actual []float64
	var mechanicalSum, actualSum float64
	for _, order := range orders {
		result.Orders++
		terminal := order.lastTrade + horizon
		if terminal >= len(midBeforeTrade) || order.preMid <= 0 || midBeforeTrade[terminal] <= 0 {
			result.UnmeasurableOrders++
			continue
		}
		counterfactualBest := order.bookAtStart.ConsumeCounterfactual(order.buys, order.qty)
		if counterfactualBest <= 0 {
			// The order would have exhausted the visible side, so there is no
			// counterfactual price. Rare, and excluded rather than clamped.
			result.UnmeasurableOrders++
			continue
		}
		counterfactualMid := counterfactualMidpoint(order, counterfactualBest)
		if counterfactualMid <= 0 {
			result.UnmeasurableOrders++
			continue
		}

		sign := 1.0
		if !order.buys {
			sign = -1.0
		}
		mechanicalBps := sign * 1e4 * math.Log(float64(counterfactualMid)/float64(order.preMid))
		actualBps := sign * 1e4 * math.Log(float64(midBeforeTrade[terminal])/float64(order.preMid))

		if counterfactualMid == order.preMid {
			result.ZeroMechanical++
		} else {
			mechanical = append(mechanical, mechanicalBps)
			actual = append(actual, actualBps)
		}
		mechanicalSum += mechanicalBps
		actualSum += actualBps
	}

	measured := result.Orders - result.UnmeasurableOrders
	if measured > 0 {
		result.MeanMechanicalBps = mechanicalSum / float64(measured)
		result.MeanActualBps = actualSum / float64(measured)
		if result.MeanActualBps != 0 {
			result.MechanicalShare = result.MeanMechanicalBps / result.MeanActualBps
		}
	}
	result.MovedOrders = len(mechanical)
	if len(mechanical) > 0 {
		result.MovedMeanMechanicalBps = meanOf(mechanical)
		result.MovedMeanActualBps = meanOf(actual)
	}
	// The regression needs a sample; the averages above do not, and gating
	// them together once made a single-order test read zero.
	if len(mechanical) >= minRegressionSample {
		result.Slope, result.Intercept, result.R2 = fitLineWithIntercept(mechanical, actual)
	}
	return result, nil
}

// counterfactualMidpoint rebuilds the midpoint from the untouched side and the
// consumed side's new best price.
func counterfactualMidpoint(order *orderReplay, consumedBest int64) int64 {
	if order.buys {
		// A buyer consumed asks; the bid is where it was.
		if order.preBid <= 0 {
			return 0
		}
		return (order.preBid + consumedBest) / 2
	}
	if order.preAsk <= 0 {
		return 0
	}
	return (consumedBest + order.preAsk) / 2
}

func copyLevels(source map[int64]int64) map[int64]int64 {
	out := make(map[int64]int64, len(source))
	for price, qty := range source {
		out[price] = qty
	}
	return out
}

func meanOf(sample []float64) float64 {
	if len(sample) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range sample {
		total += value
	}
	return total / float64(len(sample))
}

func fitLineWithIntercept(x, y []float64) (slope, intercept, r2 float64) {
	n := float64(len(x))
	var sx, sy, sxx, sxy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	denominator := n*sxx - sx*sx
	if denominator == 0 {
		return math.NaN(), math.NaN(), 0
	}
	slope = (n*sxy - sx*sy) / denominator
	intercept = (sy - slope*sx) / n
	meanY := sy / n
	var ssTotal, ssResidual float64
	for i := range x {
		ssTotal += (y[i] - meanY) * (y[i] - meanY)
		residual := y[i] - (intercept + slope*x[i])
		ssResidual += residual * residual
	}
	if ssTotal == 0 {
		return slope, intercept, 0
	}
	return slope, intercept, 1 - ssResidual/ssTotal
}
