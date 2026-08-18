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

	// The exact identity. The revision term is computed per order as the
	// realised response minus the mechanical one, never as a regression
	// residual: a regression hands the whole shared covariance to the
	// mechanical term and calls only the orthogonal remainder repricing, which
	// biases the split toward the answer the measurement exists to test.
	VarActual     float64 `json:"var_actual"`
	VarMechanical float64 `json:"var_mechanical"`
	VarRevision   float64 `json:"var_revision"`
	Covariance    float64 `json:"covariance"`

	// Mean absolute contributions. A squared statistic is not usable here:
	// most orders have a mechanical move of exactly zero, so the mechanical
	// variance is tiny while the realised variance carries the repricing of
	// every one of those orders, and an R2 built from them reports the size of
	// the zero mass rather than a market fact.
	MeanAbsMechanicalBps float64 `json:"mean_abs_mechanical_bps"`
	MeanAbsRevisionBps   float64 `json:"mean_abs_revision_bps"`
	MeanAbsActualBps     float64 `json:"mean_abs_actual_bps"`
	AbsMechanicalShare   float64 `json:"abs_mechanical_share"`

	// ZeroSubsampleMeanAbsBps is the realised response of orders that consumed
	// nothing past the touch. Their mechanical component is exactly zero, so
	// this is repricing measured with no subtraction at all.
	ZeroSubsampleMeanAbsBps float64 `json:"zero_subsample_mean_abs_bps"`

	// Slope is a replenishment estimator, not a test: if makers restore a
	// fraction of the consumed depth by the horizon the slope is one minus
	// that fraction, and above one means they skew further in the direction
	// they were swept.
	MovedOrders            int     `json:"moved_orders"`
	Slope                  float64 `json:"slope"`
	Intercept              float64 `json:"intercept"`
	MovedMeanMechanicalBps float64 `json:"moved_mean_mechanical_bps"`
	MovedMeanActualBps     float64 `json:"moved_mean_actual_bps"`

	// WalkAgreement is the share of touch-moving orders whose counterfactual
	// best price equals the book published right after the order. The exchange
	// holds its lock across matching and publication so no maker can
	// intervene, and the two must agree except where the walk cannot see what
	// the matcher did: self-trade prevention skips the aggressor's own resting
	// size, which the aggregate book does not distinguish.
	WalkAgreement float64 `json:"walk_agreement"`

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
	// response is read. Used only when HorizonNanos is zero.
	HorizonTrades int
	// HorizonNanos measures the horizon in simulated time instead of trades.
	//
	// This is the setting that matters. Makers here requote on a timer, not on
	// order flow, so how much of a response is repricing depends on how many
	// requote ticks the horizon spans. A horizon shorter than the quote
	// interval cannot contain any repricing at all, and reporting one number
	// from one horizon reads out that choice rather than a property of the
	// market. Sweep this across the quote interval instead.
	HorizonNanos int64
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
	var tradeTimestamps []int64
	// Best prices as they stood before each trade. The state before trade i+1
	// is also the state after trade i's whole delta block, because the
	// exchange holds its lock from matching through publication.
	var preBestBid, preBestAsk []int64
	var orders []*orderReplay
	byID := map[uint64]*orderReplay{}
	tradeIndex := 0

	drift, err := ReplayFile(path, func(ts int64, trade tradePayload, book *ReplayedBook) {
		midBeforeTrade = append(midBeforeTrade, book.Mid())
		tradeTimestamps = append(tradeTimestamps, ts)
		preBestBid = append(preBestBid, book.BestBid())
		preBestAsk = append(preBestAsk, book.BestAsk())
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
	var allMechanical, allActual, allRevision []float64
	var zeroSubsampleAbs []float64
	walkChecked, walkAgreed := 0, 0

	for _, order := range orders {
		result.Orders++
		terminal, ok := terminalIndex(order, horizon, opts.HorizonNanos, tradeTimestamps)
		if !ok || order.preMid <= 0 || midBeforeTrade[terminal] <= 0 {
			result.UnmeasurableOrders++
			continue
		}
		counterfactualBest := order.bookAtStart.ConsumeCounterfactual(order.buys, order.qty)
		if counterfactualBest <= 0 {
			// The order would have exhausted the visible side, so there is no
			// counterfactual price. Excluded rather than clamped.
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
		revisionBps := actualBps - mechanicalBps

		allMechanical = append(allMechanical, mechanicalBps)
		allActual = append(allActual, actualBps)
		allRevision = append(allRevision, revisionBps)

		if counterfactualMid == order.preMid {
			result.ZeroMechanical++
			zeroSubsampleAbs = append(zeroSubsampleAbs, math.Abs(actualBps))
		} else {
			mechanical = append(mechanical, mechanicalBps)
			actual = append(actual, actualBps)
			// The book published right after the order is the matcher's own
			// answer to the same counterfactual, so the walk is checked
			// against it rather than trusted. They can differ only where the
			// walk cannot see what the matcher did: self-trade prevention
			// skips the aggressor's own resting size, which an aggregate book
			// does not distinguish, and the aggressor's unfilled remainder
			// rests after publication.
			if next := order.lastTrade + 1; next < len(preBestAsk) {
				published := preBestAsk[next]
				if !order.buys {
					published = preBestBid[next]
				}
				if published > 0 {
					walkChecked++
					if published == counterfactualBest {
						walkAgreed++
					}
				}
			}
		}
	}

	if len(allActual) > 0 {
		result.MeanMechanicalBps = meanOf(allMechanical)
		result.MeanActualBps = meanOf(allActual)
		if result.MeanActualBps != 0 {
			result.MechanicalShare = result.MeanMechanicalBps / result.MeanActualBps
		}
		result.VarActual = varianceOf(allActual)
		result.VarMechanical = varianceOf(allMechanical)
		result.VarRevision = varianceOf(allRevision)
		result.Covariance = covarianceOf(allMechanical, allRevision)

		result.MeanAbsMechanicalBps = meanOf(Abs(allMechanical))
		result.MeanAbsRevisionBps = meanOf(Abs(allRevision))
		result.MeanAbsActualBps = meanOf(Abs(allActual))
		if denominator := result.MeanAbsMechanicalBps + result.MeanAbsRevisionBps; denominator > 0 {
			result.AbsMechanicalShare = result.MeanAbsMechanicalBps / denominator
		}
	}
	result.ZeroSubsampleMeanAbsBps = meanOf(zeroSubsampleAbs)
	if walkChecked > 0 {
		result.WalkAgreement = float64(walkAgreed) / float64(walkChecked)
	}

	result.MovedOrders = len(mechanical)
	if len(mechanical) > 0 {
		result.MovedMeanMechanicalBps = meanOf(mechanical)
		result.MovedMeanActualBps = meanOf(actual)
	}
	if len(mechanical) >= minRegressionSample {
		result.Slope, result.Intercept, _ = fitLineWithIntercept(mechanical, actual)
	}
	return result, nil
}

// terminalIndex finds the trade at which the realised response is read, either
// a fixed number of trades or a span of simulated time after the order's last
// fill.
func terminalIndex(order *orderReplay, horizonTrades int, horizonNanos int64, timestamps []int64) (int, bool) {
	if horizonNanos <= 0 {
		terminal := order.lastTrade + horizonTrades
		return terminal, terminal < len(timestamps)
	}
	deadline := timestamps[order.lastTrade] + horizonNanos
	for index := order.lastTrade + 1; index < len(timestamps); index++ {
		if timestamps[index] >= deadline {
			return index, true
		}
	}
	// The log ends before the horizon elapses, so this order cannot be
	// measured at this horizon.
	return 0, false
}

func varianceOf(sample []float64) float64 {
	if len(sample) < 2 {
		return 0
	}
	mean := meanOf(sample)
	total := 0.0
	for _, value := range sample {
		total += (value - mean) * (value - mean)
	}
	return total / float64(len(sample)-1)
}

func covarianceOf(a, b []float64) float64 {
	if len(a) != len(b) || len(a) < 2 {
		return 0
	}
	meanA, meanB := meanOf(a), meanOf(b)
	total := 0.0
	for i := range a {
		total += (a[i] - meanA) * (b[i] - meanB)
	}
	return total / float64(len(a)-1)
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
