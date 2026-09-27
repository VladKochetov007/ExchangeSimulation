package repeatedspot

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	worldspot "exchange_sim/simulations/repeatedspot"
	"exchange_sim/types"
)

func TestSignalSourceOwnershipUsesThePublishedNotCurrentRestingBook(t *testing.T) {
	book := &restingBook{orders: map[uint64]*restingOrder{
		1: {clientID: 2, price: 100, side: "BUY", qty: 3},
		2: {clientID: 4, price: 100, side: "BUY", qty: 2},
		3: {clientID: 2, price: 102, side: "SELL", qty: 4},
		4: {clientID: 5, price: 101, side: "BUY", qty: 1},
	}}
	bids := []types.PriceLevel{{Price: 101, VisibleQty: 1}, {Price: 100, VisibleQty: 5}}
	asks := []types.PriceLevel{{Price: 102, VisibleQty: 4}}
	owned, err := book.ownedBest(bids, asks)
	if err != nil {
		t.Fatal(err)
	}
	delete(book.orders, 3) // Source-time attribution survives a later cancellation.
	audit := newME016SignalAudit(MeasurementWindow{StartAt: 0, EndAt: 10}, 0)
	observation := worldspot.MakerObservation{SourceSequence: 7, SourceAt: 2, BestBid: 101, BestAsk: 102}
	source := publicSnapshot{timestamp: 2, bids: bids, asks: asks, ownBest: owned}
	if err := audit.makerSnapshot(Event{ClientID: 2, Timestamp: 4}, observation, source); err != nil {
		t.Fatal(err)
	}
	got := audit.Snapshots[0]
	if got.OwnBidQty != 0 || got.OwnAskQty != 4 || got.OwnOnlyBid || !got.OwnOnlyAsk ||
		got.SourceAt != 2 || got.DeliveredAt != 4 {
		t.Fatalf("wrong source-time ownership: %+v", got)
	}
	if _, err := book.ownedBest([]types.PriceLevel{{Price: 101, VisibleQty: 2}}, asks); err == nil {
		t.Fatal("public best depth with no matching source orders accepted")
	}
}

func TestSignalShadowGainUsesSameDelayedStateForBothActualGains(t *testing.T) {
	for _, gain := range []int64{0, 2} {
		world, err := BuildME016World(ME016Cell{Composition: "M1", SignalGainBps: gain, Seed: 18_101})
		if err != nil {
			t.Fatal(err)
		}
		var contract replayContract
		if err := json.Unmarshal(world.ContractJSON(), &contract); err != nil {
			t.Fatal(err)
		}
		world.Close()
		parameters, err := parseMakerParameters(contract.Participants[2])
		if err != nil {
			t.Fatal(err)
		}
		account := &accountState{clientID: contract.Participants[2].ClientID, maker: parameters,
			envelope: newMakerEnvelopeReplay(0, MeasurementWindow{StartAt: 0, EndAt: int64(20 * time.Second)}, parameters.workingLimit)}
		bidDepth, askDepth := int64(3*e0BasePrecision), int64(e0BasePrecision)
		latest := worldspot.MakerObservation{ActorID: contract.Participants[2].ActorID,
			SourceAt: int64(4 * time.Second), BestBid: 49_990 * e0QuotePrecision,
			BestAsk: 50_010 * e0QuotePrecision, TopBidVisibleQty: &bidDepth, TopAskVisibleQty: &askDepth}
		state := &replayState{latestMakerSnapshot: map[uint64]worldspot.MakerObservation{account.clientID: latest},
			lastTwoSidedMaker: map[uint64]worldspot.MakerObservation{account.clientID: latest}}
		decision := worldspot.MakerDecision{DecisionAt: int64(5 * time.Second),
			LatestBookSourceAt: latest.SourceAt, BestBid: latest.BestBid, BestAsk: latest.BestAsk,
			TopBidVisibleQty: &bidDepth, TopAskVisibleQty: &askDepth,
			LogVariancePerSecond: parameters.initialVariance, Action: "evaluate_placements"}
		mid := latest.BestBid + (latest.BestAsk-latest.BestBid)/2
		decision.TargetBid, decision.TargetAsk, _ = expectedMakerQuote(parameters, decision, mid)
		audit := newME016SignalAudit(MeasurementWindow{StartAt: 0, EndAt: int64(20 * time.Second)}, 0)
		if err := audit.makerDecision(state, account, decision); err != nil {
			t.Fatal(err)
		}
		funnel := audit.funnel(account.clientID)
		if funnel.ShadowGain2Changes != 1 || funnel.BaseQuoteUsable != 1 ||
			funnel.ActualShiftedTargets != gain/2 {
			t.Fatalf("gain %d shadow and actual target mixed: %+v", gain, funnel)
		}
		decision.DecisionAt = int64(10 * time.Second)
		decision.Action = "await_response"
		decision.TargetBid, decision.TargetAsk = 0, 0
		if err := audit.makerDecision(state, account, decision); err != nil {
			t.Fatal(err)
		}
		if funnel.ShadowGain2Changes != 2 || funnel.PendingResponse != 1 ||
			funnel.ActualShiftedTargets != gain/2 {
			t.Fatalf("pending decision lost shadow opportunity or invented target: %+v", funnel)
		}
	}
}

func TestSignalWindowBestDepthAndSpreadAreTimeWeightedAndCrossChecked(t *testing.T) {
	second := int64(time.Second)
	audit := newME016SignalAudit(MeasurementWindow{StartAt: 5 * second, EndAt: 15 * second}, 0)
	audit.clientCategory[2], audit.clientCategory[3] = "maker", "seed"
	book := newPublicBookSeries(0, 1)
	orders := &restingBook{orders: make(map[uint64]*restingOrder)}
	if err := audit.accrueBook(5*second, book); err != nil {
		t.Fatal(err)
	}
	if err := audit.accrueResting(5*second, orders); err != nil {
		t.Fatal(err)
	}
	book.bids[100], book.asks[102] = 3, 2
	orders.orders[1] = &restingOrder{clientID: 2, price: 100, side: "BUY", qty: 3}
	orders.orders[2] = &restingOrder{clientID: 3, price: 102, side: "SELL", qty: 2}
	if err := audit.accrueBook(10*second, book); err != nil {
		t.Fatal(err)
	}
	if err := audit.accrueResting(10*second, orders); err != nil {
		t.Fatal(err)
	}
	delete(book.asks, 102)
	delete(orders.orders, 2)
	resting := RestingDepthSummary{BidPresentNanos: 10 * second, AskPresentNanos: 5 * second,
		TwoSidedNanos: 5 * second, BidDepthBaseUnitNanos: big.NewInt(30 * second).String(),
		AskDepthBaseUnitNanos: big.NewInt(10 * second).String()}
	if err := audit.finish(15*second, book, orders, resting); err != nil {
		t.Fatal(err)
	}
	if audit.Window.TwoSidedNanos != 5*second || audit.Window.BestBidBaseUnitNanos != "30000000000" ||
		audit.Window.BestAskBaseUnitNanos != "10000000000" ||
		audit.Window.SpreadPriceUnitNanos != "10000000000" ||
		audit.Window.MakerBestBidBaseUnitNanos != "30000000000" ||
		audit.Window.SeedBestAskBaseUnitNanos != "10000000000" ||
		audit.Window.SeedBestBidBaseUnitNanos != "0" ||
		audit.Window.MakerBestAskBaseUnitNanos != "0" {
		t.Fatalf("wrong best-level window integral: %+v", audit.Window)
	}
	invalid := newME016SignalAudit(MeasurementWindow{StartAt: 5 * second, EndAt: 15 * second}, 0)
	book = newPublicBookSeries(0, 1)
	book.bids[100], book.asks[102] = 3, 2
	if err := invalid.finish(15*second, book, orders, resting); err == nil {
		t.Fatal("contradictory public-best and resting-order durations accepted")
	}
}

func TestSignalWindowRejectsIntrastepBestPriceOrDepthMutationWithSameSidedness(t *testing.T) {
	for _, wrong := range []struct {
		name     string
		bidPrice int64
		bidQty   int64
	}{
		{"price", 99, 3}, {"depth", 100, 4},
	} {
		t.Run(wrong.name, func(t *testing.T) {
			second := int64(time.Second)
			audit := newME016SignalAudit(MeasurementWindow{StartAt: 0, EndAt: 10 * second}, 0)
			public := newPublicBookSeries(0, 1)
			public.bids[wrong.bidPrice], public.asks[102] = wrong.bidQty, 2
			orders := &restingBook{orders: map[uint64]*restingOrder{
				1: {clientID: 2, price: 100, side: "BUY", qty: 3},
				2: {clientID: 3, price: 102, side: "SELL", qty: 2},
			}}
			resting := RestingDepthSummary{BidPresentNanos: 10 * second,
				AskPresentNanos: 10 * second, TwoSidedNanos: 10 * second,
				BidDepthBaseUnitNanos: big.NewInt(30 * second).String(),
				AskDepthBaseUnitNanos: big.NewInt(20 * second).String()}
			if err := audit.finish(10*second, public, orders, resting); err == nil {
				t.Fatal("intrastep best-quote mutation with unchanged sidedness passed")
			}
		})
	}
}
