package actor

import (
	"context"
	"os"
	"testing"
	"time"

	"exchange_sim/exchange"
)

func TestRecorderNewRecorderError(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "/invalid/path/trades.csv",
		SnapshotsPath: "testdata/snapshots.csv",
	}

	_, err := NewRecorder(1, gateway, config)
	if err == nil {
		t.Error("Expected error for invalid trades path")
	}
}

func TestRecorderNewRecorderSnapshotsError(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		SnapshotsPath: "/invalid/path/snapshots.csv",
	}

	_, err := NewRecorder(1, gateway, config)
	if err == nil {
		t.Error("Expected error for invalid snapshots path")
	}
}

func TestRecorderEventLoopContextDone(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		SnapshotsPath: "testdata/snapshots.csv",
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	recorder.Start(ctx)

	time.Sleep(10 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	if recorder.running.Load() {
		t.Error("Recorder should have stopped after context cancellation")
	}

	recorder.Stop()
}

func TestRecorderDrainWriteBufferEmpty(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		SnapshotsPath: "testdata/snapshots.csv",
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recorder.Start(ctx)

	time.Sleep(10 * time.Millisecond)
	recorder.Stop()

	if len(recorder.writeCh) != 0 {
		t.Errorf("Write channel should be empty after drain, got %d items", len(recorder.writeCh))
	}
}

func TestRecorderPlaceQuotesWithZeroMid(t *testing.T) {
	gateway := exchange.NewClientGateway(1)
	config := MarketMakerConfig{
		Symbol:        "BTCUSD",
		SpreadBps:     20,
		QuoteQty:      100000000,
		RefreshOnFill: false,
	}

	mm := NewMarketMaker(1, gateway, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mm.Start(ctx)

	<-gateway.RequestCh

	mm.placeQuotes()

	select {
	case <-gateway.RequestCh:
		t.Error("Should not place quotes when midPrice is 0")
	case <-time.After(50 * time.Millisecond):
	}

	mm.Stop()
}

func TestRecorderOnEventIgnoredTypes(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		SnapshotsPath: "testdata/snapshots.csv",
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	recorder.Start(ctx)

	ignoredEvent := &Event{
		Type: EventOrderAccepted,
		Data: OrderAcceptedEvent{
			OrderID:   123,
			RequestID: 1,
		},
	}

	recorder.OnEvent(ignoredEvent)

	time.Sleep(20 * time.Millisecond)

	if len(recorder.writeCh) != 0 {
		t.Error("Recorder should ignore non-trade/snapshot events")
	}

	recorder.Stop()
}
