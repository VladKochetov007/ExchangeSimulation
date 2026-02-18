package actors

import (
	"testing"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func newFundingArb(minRate, exitRate, maxPos int64) *InternalFundingArb {
	spotInst := exchange.NewSpotInstrument(
		"BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.SATOSHI/100,
	)
	return NewInternalFundingArb(InternalFundingArbConfig{
		ActorID:         9,
		SpotSymbol:      "BTC/USD",
		PerpSymbol:      "BTC-PERP",
		SpotInstrument:  spotInst,
		MinFundingRate:  minRate,
		ExitFundingRate: exitRate,
		MaxPositionSize: maxPos,
	})
}

type capturedOrder struct {
	symbol string
	side   exchange.Side
}

func newCapture() (actor.OrderSubmitter, *[]capturedOrder) {
	var orders []capturedOrder
	fn := func(sym string, side exchange.Side, _ exchange.OrderType, _, _ int64) uint64 {
		orders = append(orders, capturedOrder{sym, side})
		return 0
	}
	return fn, &orders
}

func TestInternalFundingArb_Identity(t *testing.T) {
	ifa := newFundingArb(10, 0, exchange.BTC_PRECISION)
	if ifa.GetID() != 9 {
		t.Errorf("GetID: want 9, got %d", ifa.GetID())
	}
	syms := ifa.GetSymbols()
	if len(syms) != 2 {
		t.Fatalf("GetSymbols: want 2, got %d", len(syms))
	}
}

func TestInternalFundingArb_SnapshotUpdatesSpot(t *testing.T) {
	ifa := newFundingArb(10, 0, exchange.BTC_PRECISION)
	ctx := actor.NewSharedContext()
	submit, _ := newCapture()
	ifa.OnEvent(&actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol: "BTC/USD",
			Snapshot: &exchange.BookSnapshot{
				Bids: []exchange.PriceLevel{{Price: 49_900 * exchange.USD_PRECISION}},
				Asks: []exchange.PriceLevel{{Price: 50_100 * exchange.USD_PRECISION}},
			},
		},
	}, ctx, submit)
	want := int64(50_000 * exchange.USD_PRECISION)
	if ifa.spotMid != want {
		t.Errorf("spotMid: want %d, got %d", want, ifa.spotMid)
	}
}

func TestInternalFundingArb_SnapshotUpdatesPerp(t *testing.T) {
	ifa := newFundingArb(10, 0, exchange.BTC_PRECISION)
	ctx := actor.NewSharedContext()
	submit, _ := newCapture()
	ifa.OnEvent(&actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol: "BTC-PERP",
			Snapshot: &exchange.BookSnapshot{
				Bids: []exchange.PriceLevel{{Price: 50_100 * exchange.USD_PRECISION}},
				Asks: []exchange.PriceLevel{{Price: 50_300 * exchange.USD_PRECISION}},
			},
		},
	}, ctx, submit)
	want := int64(50_200 * exchange.USD_PRECISION)
	if ifa.perpMid != want {
		t.Errorf("perpMid: want %d, got %d", want, ifa.perpMid)
	}
}

func TestInternalFundingArb_FundingUpdateStoresRate(t *testing.T) {
	ifa := newFundingArb(10, 0, exchange.BTC_PRECISION)
	ctx := actor.NewSharedContext()
	submit, _ := newCapture()
	ifa.OnEvent(&actor.Event{
		Type: actor.EventFundingUpdate,
		Data: actor.FundingUpdateEvent{
			Symbol:      "BTC-PERP",
			FundingRate: &exchange.FundingRate{Symbol: "BTC-PERP", Rate: 25},
		},
	}, ctx, submit)
	if ifa.fundingRate != 25 {
		t.Errorf("fundingRate: want 25, got %d", ifa.fundingRate)
	}
}

func TestInternalFundingArb_EntersOnHighFunding(t *testing.T) {
	// MinFundingRate=10; rate=20 >= 10 → enter: buy spot + sell perp
	ifa := newFundingArb(10, 0, exchange.BTC_PRECISION)
	ifa.spotMid = 50_000 * exchange.USD_PRECISION
	ifa.perpMid = 50_200 * exchange.USD_PRECISION
	ifa.fundingRate = 20

	ctx := actor.NewSharedContext()
	submit, orders := newCapture()
	ifa.evaluateStrategy(ctx, submit)

	if !ifa.isActive {
		t.Error("isActive must be true after entry")
	}
	if len(*orders) != 2 {
		t.Fatalf("want 2 orders, got %d", len(*orders))
	}
	if (*orders)[0].symbol != "BTC/USD" || (*orders)[0].side != exchange.Buy {
		t.Errorf("order[0]: want Buy BTC/USD, got %v %v", (*orders)[0].side, (*orders)[0].symbol)
	}
	if (*orders)[1].symbol != "BTC-PERP" || (*orders)[1].side != exchange.Sell {
		t.Errorf("order[1]: want Sell BTC-PERP, got %v %v", (*orders)[1].side, (*orders)[1].symbol)
	}
}

func TestInternalFundingArb_NoReentryWhenActive(t *testing.T) {
	ifa := newFundingArb(10, 0, exchange.BTC_PRECISION)
	ifa.spotMid = 50_000 * exchange.USD_PRECISION
	ifa.perpMid = 50_200 * exchange.USD_PRECISION
	ifa.fundingRate = 20
	ifa.isActive = true

	// Simulate existing spot position so enterPosition returns early.
	ifa.spotOMS.OnFill("BTC/USD", actor.OrderFillEvent{
		Side: exchange.Buy,
		Qty:  exchange.BTC_PRECISION,
		Price: 50_000 * exchange.USD_PRECISION,
	}, exchange.BTC_PRECISION)

	ctx := actor.NewSharedContext()
	submit, orders := newCapture()
	ifa.evaluateStrategy(ctx, submit)

	if len(*orders) != 0 {
		t.Errorf("must not re-enter with active position, got %d orders", len(*orders))
	}
}

func TestInternalFundingArb_ExitsOnLowFunding(t *testing.T) {
	ifa := newFundingArb(10, 5, exchange.BTC_PRECISION)
	ifa.spotMid = 50_000 * exchange.USD_PRECISION
	ifa.perpMid = 50_200 * exchange.USD_PRECISION
	ifa.fundingRate = 2
	ifa.isActive = true

	// Give the OMS a net spot position so exitPosition submits a sell.
	ifa.spotOMS.OnFill("BTC/USD", actor.OrderFillEvent{
		Side:  exchange.Buy,
		Qty:   exchange.BTC_PRECISION,
		Price: 50_000 * exchange.USD_PRECISION,
	}, exchange.BTC_PRECISION)
	// Give the OMS a net perp short so exitPosition submits a buy.
	ifa.perpOMS.OnFill("BTC-PERP", actor.OrderFillEvent{
		Side:  exchange.Sell,
		Qty:   exchange.BTC_PRECISION,
		Price: 50_200 * exchange.USD_PRECISION,
	}, exchange.BTC_PRECISION)

	ctx := actor.NewSharedContext()
	submit, orders := newCapture()
	ifa.evaluateStrategy(ctx, submit)

	if ifa.isActive {
		t.Error("isActive must be false after exit")
	}
	if len(*orders) != 2 {
		t.Fatalf("want 2 exit orders, got %d", len(*orders))
	}
	// spot sell + perp buy
	spotExit := (*orders)[0]
	perpExit := (*orders)[1]
	if spotExit.symbol != "BTC/USD" || spotExit.side != exchange.Sell {
		t.Errorf("exit[0]: want Sell BTC/USD, got %v %v", spotExit.side, spotExit.symbol)
	}
	if perpExit.symbol != "BTC-PERP" || perpExit.side != exchange.Buy {
		t.Errorf("exit[1]: want Buy BTC-PERP, got %v %v", perpExit.side, perpExit.symbol)
	}
}
