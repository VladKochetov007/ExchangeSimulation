package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-015. The fee charged on one economic exposure must not depend on how the
// counterparty's liquidity happened to be sliced.
//
// The campaign charges takers with PercentageFee{InQuote: true}, which computes
// trunc(trunc(qty*price/basePrecision) * bps / 10000) per execution. Integer
// truncation is subadditive, so the same total quantity taken as N executions
// cannot cost more than the same quantity taken as one, and generally costs
// less. The shortfall is not paid by anyone: it never reaches
// ExchangeBalance.FeeRevenue.
//
// Why this is an actor-fairness question rather than a rounding nit: the taker
// does not choose the partition. The book does. Two takers submitting the
// identical order at the identical price pay different fees depending on
// whether the resting side was one large order or many small ones — and a taker
// that wants the cheaper side can arrange it by taking against, or itself
// posting, minimum-size clips.
//
// These tests state the boundary as a measurement, not as a policy: the note
// records the direction and the magnitude, and the disposition (aggregate the
// fee over a match, round half-up, or accept the drift) is the owner's.
const (
	auditFeeBps      = 5 // the campaign's taker_fee_bps
	auditFeePrice    = 100 * USD_PRECISION
	auditFeeMinOrder = BTC_PRECISION / 1_000 // the ABC venue minimum
)

// The campaign's own price levels, so a magnitude measured here is a magnitude
// that means something. ABC bootstraps at 50_000 and CDF at 3_000.
const auditFeeCampaignPrice = 50_000 * USD_PRECISION

func newFeeFixture(t *testing.T) *DefaultExchange {
	t.Helper()
	ex := NewExchange(3, &RealClock{})
	ex.AddInstrument(NewSpotInstrument(
		"ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, USD_PRECISION, auditFeeMinOrder))
	taker := &PercentageFee{MakerBps: 0, TakerBps: auditFeeBps, InQuote: true}
	ex.ConnectNewClient(1, map[string]int64{"USD": 1_000_000 * USD_PRECISION}, taker)
	ex.ConnectNewClient(2, map[string]int64{"ABC": 1_000 * BTC_PRECISION}, taker)
	return ex
}

// takeAgainst rests the given sell clips for client 2, then has client 1 buy
// the whole posted quantity in one order, and returns the fee revenue the
// exchange collected.
func takeAgainst(t *testing.T, price int64, clips []int64) int64 {
	t.Helper()
	ex := newFeeFixture(t)
	total := int64(0)
	for _, clip := range clips {
		if _, reject := InjectLimitOrder(ex, 2, "ABC/USD", Sell, price, clip); reject != "" {
			t.Fatalf("resting clip %d rejected: %s", clip, reject)
		}
		total += clip
	}
	if _, reject := InjectLimitOrder(ex, 1, "ABC/USD", Buy, price, total); reject != "" {
		t.Fatalf("taker order rejected: %s", reject)
	}
	if got := ex.Clients[2].Balances["ABC"]; got != 1_000*BTC_PRECISION-total {
		t.Fatalf("the book did not fill completely: maker still holds %d ABC", got)
	}
	return ex.ExchangeBalance.FeeRevenue["USD"]
}

func TestAuditTakerFeeDoesNotDependOnHowTheBookWasSliced(t *testing.T) {
	// This test does not demand a fee policy. Fee semantics are the owner's to
	// decide, and aggregating a fee across a match is a change to scientific
	// economics. What it pins is the shape of the effect, so that a change in
	// its direction or its bound is visible.
	measure := func(t *testing.T, price, clip int64, clips int) (one, many int64) {
		t.Helper()
		sliced := make([]int64, clips)
		whole := int64(0)
		for index := range sliced {
			sliced[index] = clip
			whole += clip
		}
		return takeAgainst(t, price, []int64{whole}), takeAgainst(t, price, sliced)
	}

	t.Run("the effect is directional and bounded by one quote unit per execution", func(t *testing.T) {
		// 150_001 base units at 100 USD is 15_000.1 quote units before the fee
		// rate, so every clip truncates in MulDiv and again in the bps step.
		const clips = 10
		one, many := measure(t, auditFeePrice, 150_001, clips)
		t.Logf("100 USD, %d clips: one execution charges %d quote units, %d executions charge %d",
			clips, one, clips, many)

		// Direction: truncation is subadditive, so slicing can only ever cost
		// the taker less, never more. A reversal would mean the fee had become
		// superadditive, which no rounding rule produces by accident.
		if many > one {
			t.Errorf("slicing made the taker pay MORE (%d vs %d): the fee is no longer subadditive under partition",
				many, one)
		}
		// Bound: each execution can lose at most one smallest quote unit to
		// truncation, twice over, so the shortfall cannot reach the execution
		// count doubled.
		if shortfall := one - many; shortfall >= 2*clips {
			t.Errorf("shortfall %d over %d executions exceeds two quote units per execution: truncation is not the only cause",
				shortfall, clips)
		}
	})

	t.Run("at the campaign's price level the effect is negligible but still one-directional", func(t *testing.T) {
		const clips = 10
		one, many := measure(t, auditFeeCampaignPrice, auditFeeMinOrder, clips)
		t.Logf("50_000 USD (the ABC bootstrap), %d minimum clips: one execution charges %d quote units, %d executions charge %d, shortfall %d (%.4f%%)",
			clips, one, clips, many, one-many, 100*float64(one-many)/float64(one))
		if many > one {
			t.Errorf("slicing made the taker pay MORE at campaign scale (%d vs %d)", many, one)
		}
		if one-many != 0 {
			t.Errorf("a round minimum clip at a round price should truncate nowhere, shortfall %d", one-many)
		}
	})

	// The arm above is the campaign's *best* case: at 50_000 the fee on a
	// minimum clip divides exactly, so nothing is lost. Actors do not trade
	// round clips. At this price the charge reduces to qty/40 quote units, so
	// the worst partition loss for a ten-way slice is trunc(39/4) = 9 units.
	// This arm measures that worst case, so the campaign-scale magnitude is on
	// record as a number rather than as an extrapolation from the small fixture.
	t.Run("the campaign-scale worst case is nine quote units in thirty-seven thousand", func(t *testing.T) {
		const clips = 10
		one, many := measure(t, auditFeeCampaignPrice, auditFeeMinOrder+39, clips)
		shortfall := one - many
		t.Logf("50_000 USD, %d clips chosen to maximise truncation: one execution charges %d, %d executions charge %d, shortfall %d (%.4f%%)",
			clips, one, clips, many, shortfall, 100*float64(shortfall)/float64(one))
		if shortfall < 0 || shortfall > 9 {
			t.Errorf("worst-case shortfall %d is outside the derived bound of 9 quote units", shortfall)
		}
	})
}

// The sharp form: at what trade value does the fee truncate to nothing? Below
// that boundary a fill is free, and whether the boundary is reachable is a
// property of the instrument's minimum order size and its price, not of the
// fee model.
func TestAuditFeeVanishesBelowAThresholdTradeValue(t *testing.T) {
	plan := &PercentageFee{MakerBps: 0, TakerBps: auditFeeBps, InQuote: true}

	feeFor := func(qty, price int64) int64 {
		exec := &Execution{Price: price, Qty: qty}
		charged, err := plan.CalculateFee(FillContext{
			Exec: exec, IsMaker: false, BaseAsset: "ABC", QuoteAsset: "USD", Precision: BTC_PRECISION,
		})
		if err != nil {
			t.Fatalf("fee: %v", err)
		}
		return charged.Amount
	}

	// A minimum-size clip at 100 USD is charged.
	if got := feeFor(auditFeeMinOrder, auditFeePrice); got == 0 {
		t.Fatalf("a minimum clip at 100 USD paid nothing, so the fixture proves nothing")
	}

	// The same minimum-size clip on a one-dollar instrument is not.
	if got := feeFor(auditFeeMinOrder, 1*USD_PRECISION); got != 0 {
		t.Logf("a minimum clip at 1 USD pays %d quote units", got)
	} else {
		t.Logf("FREE: a minimum-size clip at 1 USD pays no fee at all")
	}

	// Report the exact boundary rather than asserting a policy: the largest
	// price at which a minimum-size clip still escapes the fee.
	lowest := int64(0)
	for price := int64(1) * USD_PRECISION; price <= 100*USD_PRECISION; price += USD_PRECISION {
		if feeFor(auditFeeMinOrder, price) == 0 {
			lowest = price
		}
	}
	t.Logf("a minimum-size clip (%d base units) pays no fee at any price up to %d quote units (%.2f USD)",
		auditFeeMinOrder, lowest, float64(lowest)/float64(USD_PRECISION))
}
