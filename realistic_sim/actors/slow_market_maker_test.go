package actors

import (
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

func newSlowMM(symbol string, levels int, ema float64) *SlowMarketMakerActor {
	inst := exchange.NewSpotInstrument(
		symbol, "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.SATOSHI/100,
	)
	config := SlowMarketMakerConfig{
		Symbol:     symbol,
		Instrument: inst,
		SpreadBps:  50,
		QuoteSize:  exchange.BTC_PRECISION,
		EMADecay:   ema,
		Levels:     levels,
	}
	gw := exchange.NewClientGateway(1)
	return NewSlowMarketMaker(1, gw, config)
}

func TestSlowMarketMaker_Defaults(t *testing.T) {
	gw := exchange.NewClientGateway(1)
	smm := NewSlowMarketMaker(1, gw, SlowMarketMakerConfig{SpreadBps: 50})
	if smm.config.RequoteInterval != time.Second {
		t.Errorf("RequoteInterval: want 1s, got %v", smm.config.RequoteInterval)
	}
	if smm.config.Levels != 1 {
		t.Errorf("Levels: want 1, got %d", smm.config.Levels)
	}
	if smm.config.LevelSpacingBps != 50 {
		t.Errorf("LevelSpacingBps: want 50 (= SpreadBps), got %d", smm.config.LevelSpacingBps)
	}
}

func TestSlowMarketMaker_TradeBootstrap(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.1)
	smm.onTrade(actor.TradeEvent{
		Symbol: "BTC/USD",
		Trade:  &exchange.Trade{Price: 50_000},
	})
	if smm.lastMidPrice != 50_000 {
		t.Errorf("want 50000, got %d", smm.lastMidPrice)
	}
}

func TestSlowMarketMaker_TradeEMA(t *testing.T) {
	// EMA update: alpha*new + (1-alpha)*old  →  0.5*200 + 0.5*100 = 150
	smm := newSlowMM("BTC/USD", 1, 0.5)
	smm.lastMidPrice = 100
	smm.onTrade(actor.TradeEvent{
		Symbol: "BTC/USD",
		Trade:  &exchange.Trade{Price: 200},
	})
	if smm.lastMidPrice != 150 {
		t.Errorf("want 150, got %d", smm.lastMidPrice)
	}
}

func TestSlowMarketMaker_WrongSymbolIgnored(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.5)
	smm.lastMidPrice = 100
	smm.onTrade(actor.TradeEvent{
		Symbol: "ETH/USD",
		Trade:  &exchange.Trade{Price: 9999},
	})
	if smm.lastMidPrice != 100 {
		t.Errorf("trade on wrong symbol must not change mid, got %d", smm.lastMidPrice)
	}
}

func TestSlowMarketMaker_SnapshotBootstrap(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.1)
	smm.onBookSnapshot(actor.BookSnapshotEvent{
		Symbol: "BTC/USD",
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 99_000}},
			Asks: []exchange.PriceLevel{{Price: 101_000}},
		},
	})
	if smm.lastMidPrice != 100_000 {
		t.Errorf("want 100000, got %d", smm.lastMidPrice)
	}
}

func TestSlowMarketMaker_SnapshotIgnoredAfterEMA(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.1)
	smm.lastMidPrice = 200_000
	smm.onBookSnapshot(actor.BookSnapshotEvent{
		Symbol: "BTC/USD",
		Snapshot: &exchange.BookSnapshot{
			Bids: []exchange.PriceLevel{{Price: 99_000}},
			Asks: []exchange.PriceLevel{{Price: 101_000}},
		},
	})
	if smm.lastMidPrice != 200_000 {
		t.Errorf("snapshot must not overwrite existing mid, got %d", smm.lastMidPrice)
	}
}

func TestSlowMarketMaker_OrderAcceptedTracksBid(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.1)
	smm.lastBidReqIDs[0] = 42
	smm.onOrderAccepted(actor.OrderAcceptedEvent{OrderID: 99, RequestID: 42})
	if smm.activeBidIDs[0] != 99 {
		t.Errorf("want activeBidIDs[0]=99, got %d", smm.activeBidIDs[0])
	}
}

func TestSlowMarketMaker_OrderAcceptedTracksAsk(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.1)
	smm.lastAskReqIDs[0] = 43
	smm.onOrderAccepted(actor.OrderAcceptedEvent{OrderID: 88, RequestID: 43})
	if smm.activeAskIDs[0] != 88 {
		t.Errorf("want activeAskIDs[0]=88, got %d", smm.activeAskIDs[0])
	}
}

func TestSlowMarketMaker_FullFillClearsIDAndUpdatesInventory(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.1)
	smm.activeBidIDs[0] = 99
	smm.onOrderFilled(actor.OrderFillEvent{
		OrderID: 99,
		Side:    exchange.Buy,
		Qty:     100,
		IsFull:  true,
	})
	if smm.activeBidIDs[0] != 0 {
		t.Error("full fill must clear activeBidIDs[0]")
	}
	if smm.inventory != 100 {
		t.Errorf("inventory: want 100, got %d", smm.inventory)
	}
}

func TestSlowMarketMaker_PartialFillKeepsID(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.1)
	smm.activeBidIDs[0] = 99
	smm.onOrderFilled(actor.OrderFillEvent{
		OrderID: 99,
		Side:    exchange.Buy,
		Qty:     50,
		IsFull:  false,
	})
	if smm.activeBidIDs[0] != 99 {
		t.Error("partial fill must not clear activeBidIDs[0]")
	}
	if smm.inventory != 50 {
		t.Errorf("inventory: want 50, got %d", smm.inventory)
	}
}

func TestSlowMarketMaker_CancelClearsAskID(t *testing.T) {
	smm := newSlowMM("BTC/USD", 1, 0.1)
	smm.activeAskIDs[0] = 77
	smm.onOrderCancelled(actor.OrderCancelledEvent{OrderID: 77})
	if smm.activeAskIDs[0] != 0 {
		t.Error("cancel must clear activeAskIDs[0]")
	}
}
