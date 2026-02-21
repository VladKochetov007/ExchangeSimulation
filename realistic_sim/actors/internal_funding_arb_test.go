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
		ActorID:               9,
		SpotSymbol:            "BTC/USD",
		PerpSymbol:            "BTC-PERP",
		SpotInstrument:        spotInst,
		MinFundingRate:        minRate,
		ExitFundingRate:       exitRate,
		BasisThresholdBps:     10,
		ExitBasisThresholdBps: 2,
		MaxPositionSize:       maxPos,
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

func TestInternalFundingArb_EntersOnHighFunding(t *testing.T) {
	// MinFundingRate=10; rate=20 >= 10 → enter: buy spot + sell perp
	ifa := newFundingArb(10, 0, exchange.BTC_PRECISION)
	ifa.spotMid = 50_000 * exchange.USD_PRECISION
	ifa.perpMid = 50_000 * exchange.USD_PRECISION // No basis
	ifa.fundingRate = 20

	ctx := actor.NewSharedContext()
	ctx.InitializeBalances(map[string]int64{"BTC": 0}, 1_000_000*exchange.USD_PRECISION)
	submit, orders := newCapture()
	ifa.evaluateStrategy(ctx, submit)

	if ifa.currentMode != ModeContango {
		t.Errorf("currentMode: want ModeContango, got %v", ifa.currentMode)
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

func TestInternalFundingArb_EntersOnPositiveBasis(t *testing.T) {
	// BasisThresholdBps=10
	// Spot=50k, Perp=50.1k -> basis = 100/50000 * 10000 = 20 bps
	ifa := newFundingArb(100, 0, exchange.BTC_PRECISION) // High funding threshold to ignore it
	ifa.spotMid = 50_000 * exchange.USD_PRECISION
	ifa.perpMid = 50_100 * exchange.USD_PRECISION
	ifa.fundingRate = 0

	ctx := actor.NewSharedContext()
	ctx.InitializeBalances(map[string]int64{"BTC": 0}, 1_000_000*exchange.USD_PRECISION)
	submit, orders := newCapture()
	ifa.evaluateStrategy(ctx, submit)

	if ifa.currentMode != ModeContango {
		t.Errorf("currentMode: want ModeContango, got %v", ifa.currentMode)
	}
	if len(*orders) != 2 {
		t.Fatalf("want 2 orders, got %d", len(*orders))
	}
}

func TestInternalFundingArb_NoReentryWhenActive(t *testing.T) {
	ifa := newFundingArb(10, 0, exchange.BTC_PRECISION)
	ifa.spotMid = 50_000 * exchange.USD_PRECISION
	ifa.perpMid = 50_200 * exchange.USD_PRECISION
	ifa.fundingRate = 20
	ifa.currentMode = ModeContango

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

func TestInternalFundingArb_ExitsOnLowFundingAndBasis(t *testing.T) {
	// ExitFundingRate=5, ExitBasisThresholdBps=2
	ifa := newFundingArb(10, 5, exchange.BTC_PRECISION)
	ifa.spotMid = 50_000 * exchange.USD_PRECISION
	ifa.perpMid = 50_000 * exchange.USD_PRECISION // 0 basis
	ifa.fundingRate = 2 // < 5
	ifa.currentMode = ModeContango

	// Give the OMS positions
	ifa.spotOMS.OnFill("BTC/USD", actor.OrderFillEvent{
		Side:  exchange.Buy,
		Qty:   exchange.BTC_PRECISION,
		Price: 50_000 * exchange.USD_PRECISION,
	}, exchange.BTC_PRECISION)
	ifa.perpOMS.OnFill("BTC-PERP", actor.OrderFillEvent{
		Side:  exchange.Sell,
		Qty:   exchange.BTC_PRECISION,
		Price: 50_000 * exchange.USD_PRECISION,
	}, exchange.BTC_PRECISION)

	ctx := actor.NewSharedContext()
	submit, orders := newCapture()
	ifa.evaluateStrategy(ctx, submit)

	// Exit orders must be submitted.
	if len(*orders) != 2 {
		t.Fatalf("want 2 exit orders, got %d", len(*orders))
	}
	if !ifa.pendingExit {
		t.Error("pendingExit must be true after exit orders submitted")
	}

	// Simulate fills arriving: reset OMS to flat.
	ifa.spotOMS.OnFill("BTC/USD", actor.OrderFillEvent{
		Side:  exchange.Sell,
		Qty:   exchange.BTC_PRECISION,
		Price: 50_000 * exchange.USD_PRECISION,
		IsFull: true,
	}, exchange.BTC_PRECISION)
	ifa.perpOMS.OnFill("BTC-PERP", actor.OrderFillEvent{
		Side:  exchange.Buy,
		Qty:   exchange.BTC_PRECISION,
		Price: 50_000 * exchange.USD_PRECISION,
		IsFull: true,
	}, exchange.BTC_PRECISION)

	// evaluateStrategy now sees flat OMS and clears currentMode.
	ifa.evaluateStrategy(ctx, submit)
	if ifa.currentMode != ModeNone {
		t.Error("currentMode must be ModeNone after OMS confirms flat position")
	}
}
