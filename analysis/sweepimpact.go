package analysis

import (
	"math"
	"sort"
)

// SweptOrders identifies the aggressive orders whose fills spanned more than
// one price, which is the observable signature of an order consuming depth
// past the touch.
func (t *TradeTape) SweptOrders() map[uint64]bool {
	seen := map[uint64]map[int64]struct{}{}
	for i, orderID := range t.TakerOrderIDs {
		if orderID == 0 || i >= len(t.Prices) {
			continue
		}
		prices := seen[orderID]
		if prices == nil {
			prices = map[int64]struct{}{}
			seen[orderID] = prices
		}
		prices[t.Prices[i]] = struct{}{}
	}
	swept := make(map[uint64]bool, len(seen))
	for orderID, prices := range seen {
		if len(prices) > 1 {
			swept[orderID] = true
		}
	}
	return swept
}

// SweepImpact compares the price response of orders that consumed more than
// one price level against those that did not, within matched size buckets.
//
// This is the discriminating measurement between two accounts of impact. If
// price moves because trades consume depth, an order that walked the book must
// show a larger response than an equally sized order that did not. If price
// moves because makers reprice around their inventory, sweeping is incidental
// and the two should agree once size is held fixed.
//
// Size is controlled by construction rather than by regression: buckets are
// formed over all orders by size quantile, and the two classes are compared
// only within a bucket. Swept orders are systematically larger, so any pooled
// comparison measures the size distribution instead.
type SweepImpact struct {
	Buckets []SweepImpactBucket `json:"buckets"`
	// SweptN and SingleN are the totals across buckets.
	SweptN  int `json:"swept_n"`
	SingleN int `json:"single_n"`
	// MeanGapBps is the size-weighted mean of the within-bucket differences,
	// counting only buckets where both classes are populated.
	MeanGapBps float64 `json:"mean_gap_bps"`
	// BucketsFavouringSwept counts buckets where swept orders responded more,
	// which says whether the gap is consistent or an average over sign changes.
	BucketsFavouringSwept int `json:"buckets_favouring_swept"`
	BucketsCompared       int `json:"buckets_compared"`
}

// SweepImpactBucket is one size group's response, split by sweep status.
type SweepImpactBucket struct {
	MeanSize      float64 `json:"mean_size"`
	SweptResponse float64 `json:"swept_response"`
	SweptN        int     `json:"swept_n"`
	SingleResp    float64 `json:"single_response"`
	SingleN       int     `json:"single_n"`
	GapBps        float64 `json:"gap_bps"`
}

// MeasureSweepImpact runs the comparison over a tape.
//
// The unit of observation is the aggressive order, not the individual fill. A
// swept order produces several fills, and counting each one separately would
// both double-weight exactly the class under test and assign it a partial size
// in place of the size actually submitted, biasing the comparison downward.
func (t *TradeTape) MeasureSweepImpact(opts ImpactOptions) SweepImpact {
	horizon := opts.HorizonTrades
	if horizon < 1 {
		horizon = 10
	}
	bucketCount := opts.Buckets
	if bucketCount < 2 {
		bucketCount = 10
	}

	type observation struct {
		size, response float64
		swept          bool
	}
	observations := make([]observation, 0, len(t.Prices))

	// Fills are grouped by the order that crossed rather than by adjacency.
	// A book's tape is assembled from several files scanned concurrently and
	// then sorted by timestamp, and dozens of trades share one timestamp, so
	// an order's fills are usually adjacent but are not guaranteed to be.
	type group struct{ first, last int }
	groups := map[uint64]*group{}
	var orderSequence []uint64
	for i := 0; i < len(t.Prices); i++ {
		orderID := uint64(0)
		if i < len(t.TakerOrderIDs) {
			orderID = t.TakerOrderIDs[i]
		}
		if orderID == 0 {
			continue
		}
		existing := groups[orderID]
		if existing == nil {
			groups[orderID] = &group{first: i, last: i}
			orderSequence = append(orderSequence, orderID)
			continue
		}
		if i < existing.first {
			existing.first = i
		}
		if i > existing.last {
			existing.last = i
		}
	}
	for _, orderID := range orderSequence {
		bounds := groups[orderID]
		if observed, ok := t.orderObservation(orderID, bounds.first, bounds.last, horizon, opts.Role); ok {
			observations = append(observations, observed)
		}
	}

	result := SweepImpact{}
	if len(observations) < bucketCount*10 {
		return result
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].size < observations[j].size })

	per := len(observations) / bucketCount
	var gapSum, gapWeight float64
	for b := 0; b < bucketCount; b++ {
		lo, hi := b*per, (b+1)*per
		if b == bucketCount-1 {
			hi = len(observations)
		}
		var sumLogSize, sweptSum, singleSum float64
		sweptCount, singleCount := 0, 0
		for _, obs := range observations[lo:hi] {
			sumLogSize += math.Log(obs.size)
			if obs.swept {
				sweptSum += obs.response
				sweptCount++
			} else {
				singleSum += obs.response
				singleCount++
			}
		}
		bucket := SweepImpactBucket{
			MeanSize: math.Exp(sumLogSize / float64(hi-lo)),
			SweptN:   sweptCount,
			SingleN:  singleCount,
		}
		if sweptCount > 0 {
			bucket.SweptResponse = sweptSum / float64(sweptCount)
		}
		if singleCount > 0 {
			bucket.SingleResp = singleSum / float64(singleCount)
		}
		// A bucket needs both classes to say anything, and enough of the
		// smaller one that its mean is not a handful of orders.
		if sweptCount >= minClassPerBucket && singleCount >= minClassPerBucket {
			bucket.GapBps = bucket.SweptResponse - bucket.SingleResp
			weight := float64(sweptCount)
			if singleCount < sweptCount {
				weight = float64(singleCount)
			}
			gapSum += bucket.GapBps * weight
			gapWeight += weight
			result.BucketsCompared++
			if bucket.GapBps > 0 {
				result.BucketsFavouringSwept++
			}
		}
		result.SweptN += sweptCount
		result.SingleN += singleCount
		result.Buckets = append(result.Buckets, bucket)
	}
	if gapWeight > 0 {
		result.MeanGapBps = gapSum / gapWeight
	}
	return result
}

// minClassPerBucket is the smallest class size a bucket may contribute from.
// Below it the bucket mean is noise and the weighted gap inherits it.
const minClassPerBucket = 20

// orderObservation reduces one aggressive order's contiguous fills to a single
// size and response. The response runs from the mid published before the order
// began to the price a horizon of trades after its last fill, so a sweep's own
// later fills are not counted as part of its own response.
func (t *TradeTape) orderObservation(orderID uint64, first, last, horizon int, role string) (struct {
	size, response float64
	swept          bool
}, bool) {
	var zero struct {
		size, response float64
		swept          bool
	}
	if first == 0 || last+horizon >= len(t.Prices) {
		return zero, false
	}
	if role != "" && (first >= len(t.Roles) || t.Roles[first] != role) {
		return zero, false
	}
	size := int64(0)
	prices := map[int64]struct{}{}
	for i := first; i <= last; i++ {
		if t.Qtys[i] <= 0 || i >= len(t.TakerOrderIDs) || t.TakerOrderIDs[i] != orderID {
			continue
		}
		size += t.Qtys[i]
		prices[t.Prices[i]] = struct{}{}
	}
	if size <= 0 {
		return zero, false
	}
	reference := int64(0)
	if first < len(t.PreMid) {
		reference = t.PreMid[first]
	}
	if reference <= 0 {
		reference = t.Prices[first-1]
	}
	if reference <= 0 {
		return zero, false
	}
	terminal := t.terminalMid(last + horizon)
	if terminal <= 0 {
		return zero, false
	}
	response := 1e4 * math.Log(float64(terminal)/float64(reference))
	zero.size = float64(size)
	zero.response = float64(t.Signs[first]) * response
	zero.swept = len(prices) > 1
	return zero, true
}
