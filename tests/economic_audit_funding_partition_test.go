package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-021. Funding is the purest actor-versus-actor flow in the model: longs pay
// shorts every interval. settleFunding computes each position's payment as
// TryMulDiv(positionValue, rate, 10000) per position, and integer truncation is
// subadditive, so the same exposure spread over more accounts is charged less.
//
// The code already recognises that the two sides need not net: netExchangeFlow
// is accumulated explicitly, validated, and routed to exchange revenue with the
// comment that on a real venue this is the insurance fund's residual. So no
// value is lost — the residual is booked. What these tests establish is that the
// residual is not partition-neutral: which way it points is decided by which
// side is more fragmented.
//
// Magnitude first, so the structure is not mistaken for an exploit: the effect
// is bounded by roughly one quote unit per extra account per settlement. At the
// prices used here that is 1 unit in 8801, or 0.011%. It is recorded because it
// is the same mechanism family as RT-008 in fees — every per-item integer charge
// in this system is partition-dependent — not because it moves money.
const (
	fundingSymbol = "ABC-PERP"
	// Equal to BTC_PRECISION, so a position's value is its raw size and the
	// bps step is the only place truncation can happen. That isolates the
	// mechanism instead of mixing it with the price conversion.
	fundingMark = 1000 * USD_PRECISION
	// Chosen from a scan: at this size the split and whole arms disagree.
	fundingLeg  = BTC_PRECISION/100 + 117
	fundingLegs = 8
)

// settleOneInterval settles a single funding interval where one side is held in
// `split` equal accounts and the other in a single account, and reports how much
// cash the split side moved and what the exchange residual became.
func settleOneInterval(t *testing.T, split int, splitSideIsLong bool) (splitSideFlow, residual int64) {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	perp := NewPerpFutures(fundingSymbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)

	// A mark above the index gives a positive rate: longs pay, shorts receive.
	if err := perp.UpdateFundingRate(fundingMark-USD_PRECISION, fundingMark); err != nil {
		t.Fatalf("funding rate: %v", err)
	}
	if rate := perp.GetFundingRate(); rate.Rate <= 0 {
		t.Fatalf("funding rate is %d; this test needs longs to be the payers", rate.Rate)
	}

	total := int64(fundingLeg) * fundingLegs
	perLeg := total / int64(split)
	if perLeg*int64(split) != total {
		t.Fatalf("fixture error: %d does not divide %d evenly", split, total)
	}
	sign := int64(1)
	if !splitSideIsLong {
		sign = -1
	}

	pm := ex.Positions.(*PositionManager)
	before := int64(0)
	for index := 0; index < split; index++ {
		id := uint64(index + 1)
		ex.ConnectNewClient(id, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(id, "USD", USDAmount(1_000_000))
		before += ex.Clients[id].PerpBalances["USD"]
		pm.Lock()
		pm.InjectPosition(id, fundingSymbol, &Position{
			ClientID: id, Symbol: fundingSymbol, PositionSide: PositionBoth,
			Size: sign * perLeg, EntryPrice: fundingMark,
		})
		pm.Unlock()
	}

	// The other side is always one account, so only the split side's partition
	// differs between arms.
	otherID := uint64(split + 1)
	ex.ConnectNewClient(otherID, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(otherID, "USD", USDAmount(1_000_000))
	pm.Lock()
	pm.InjectPosition(otherID, fundingSymbol, &Position{
		ClientID: otherID, Symbol: fundingSymbol, PositionSide: PositionBoth,
		Size: -sign * total, EntryPrice: fundingMark,
	})
	pm.Unlock()

	if err := ex.SettleFunding(perp); err != nil {
		t.Fatalf("settle funding: %v", err)
	}

	after := int64(0)
	for index := 0; index < split; index++ {
		after += ex.Clients[uint64(index+1)].PerpBalances["USD"]
	}
	// Positive means the split side moved that much cash, whichever direction
	// it was moving it in.
	flow := before - after
	if !splitSideIsLong {
		flow = after - before
	}
	return flow, ex.ExchangeBalance.FeeRevenue["USD"]
}

func TestAuditFundingIsNotInvariantUnderAccountPartition(t *testing.T) {
	t.Run("a payer split across accounts pays less", func(t *testing.T) {
		whole, wholeResidual := settleOneInterval(t, 1, true)
		split, splitResidual := settleOneInterval(t, fundingLegs, true)
		t.Logf("one account pays %d (residual %d); %d accounts pay %d (residual %d)",
			whole, wholeResidual, fundingLegs, split, splitResidual)

		if split > whole {
			t.Fatalf("splitting made the payer pay MORE (%d vs %d): funding is superadditive under partition",
				split, whole)
		}
		if split == whole {
			t.Fatalf("no partition effect at this fixture, so the test no longer discriminates")
		}
		// The counterparty is a single account in both arms and receives the
		// same either way, so the difference is not a transfer between traders:
		// the exchange residual makes it up.
		if splitResidual >= wholeResidual {
			t.Errorf("the payer paid %d less but the exchange residual did not absorb it: %d then %d",
				whole-split, wholeResidual, splitResidual)
		}
	})

	t.Run("a receiver split across accounts receives less", func(t *testing.T) {
		whole, wholeResidual := settleOneInterval(t, 1, false)
		split, splitResidual := settleOneInterval(t, fundingLegs, false)
		t.Logf("one account receives %d (residual %d); %d accounts receive %d (residual %d)",
			whole, wholeResidual, fundingLegs, split, splitResidual)

		if split > whole {
			t.Fatalf("splitting made the receiver receive MORE (%d vs %d)", split, whole)
		}
		if split == whole {
			t.Fatalf("no partition effect at this fixture, so the test no longer discriminates")
		}
		if splitResidual <= wholeResidual {
			t.Errorf("the receiver received %d less but the exchange residual did not gain it: %d then %d",
				whole-split, wholeResidual, splitResidual)
		}
	})

	// The bound. Fragmentation moves the rounding away from the fragmented
	// side's cash flow in both directions, so it is not an unconditional
	// advantage: it helps a payer and hurts a receiver, by at most about one
	// quote unit per extra account per settlement.
	t.Run("the effect is bounded by one quote unit per extra account", func(t *testing.T) {
		payerWhole, _ := settleOneInterval(t, 1, true)
		payerSplit, _ := settleOneInterval(t, fundingLegs, true)
		if gap := payerWhole - payerSplit; gap < 0 || gap > fundingLegs-1 {
			t.Errorf("payer gap %d is outside the derived bound of %d quote units", gap, fundingLegs-1)
		}
	})
}
