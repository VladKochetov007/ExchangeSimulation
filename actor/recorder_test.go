package actor

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"exchange_sim/exchange"
)

func TestRecorderCreatesFiles(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		ObservedPath:  "testdata/book_observed.csv",
		HiddenPath:    "testdata/book_hidden.csv",
		FlushInterval: 100 * time.Millisecond,
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Stop()

	if _, err := os.Stat("testdata/trades.csv"); os.IsNotExist(err) {
		t.Error("trades.csv not created")
	}
	if _, err := os.Stat("testdata/book_observed.csv"); os.IsNotExist(err) {
		t.Error("book_observed.csv not created")
	}
	if _, err := os.Stat("testdata/book_hidden.csv"); os.IsNotExist(err) {
		t.Error("book_hidden.csv not created")
	}
}

func TestRecorderWritesTrade(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		ObservedPath:  "testdata/book_observed.csv",
		HiddenPath:    "testdata/book_hidden.csv",
		FlushInterval: 10 * time.Millisecond,
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	trade := &exchange.Trade{
		TradeID: 123,
		Price:   5000000000000,
		Qty:     100000000,
		Side:    exchange.Buy,
	}

	event := &Event{
		Type: EventTrade,
		Data: TradeEvent{
			Symbol:    "BTCUSD",
			Trade:     trade,
			Timestamp: 1234567890000000000,
		},
	}

	recorder.OnEvent(event)

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()

	file, err := os.Open("testdata/trades.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	foundTrade := false

	for scanner.Scan() {
		lines++
		line := scanner.Text()
		if strings.Contains(line, "123") && strings.Contains(line, "BTCUSD") {
			foundTrade = true
		}
	}

	if lines < 2 {
		t.Errorf("Expected at least 2 lines (header + trade), got %d", lines)
	}
	if !foundTrade {
		t.Error("Trade not found in output file")
	}
}

func TestRecorderWritesSnapshot(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"ETHUSD"},
		TradesPath:    "testdata/trades.csv",
		ObservedPath:  "testdata/book_observed.csv",
		HiddenPath:    "testdata/book_hidden.csv",
		FlushInterval: 10 * time.Millisecond,
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	snapshot := &exchange.BookSnapshot{
		Bids: []exchange.PriceLevel{
			{Price: 3000000000000, VisibleQty: 500000000},
			{Price: 2999000000000, VisibleQty: 300000000},
		},
		Asks: []exchange.PriceLevel{
			{Price: 3001000000000, VisibleQty: 400000000},
		},
	}

	event := &Event{
		Type: EventBookSnapshot,
		Data: BookSnapshotEvent{
			Symbol:    "ETHUSD",
			Snapshot:  snapshot,
			Timestamp: 1234567890000000000,
		},
	}

	recorder.OnEvent(event)

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()

	file, err := os.Open("testdata/book_observed.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	bidLines := 0
	askLines := 0

	for scanner.Scan() {
		lines++
		line := scanner.Text()
		if strings.Contains(line, "ETHUSD") {
			if strings.Contains(line, "bid") {
				bidLines++
			}
			if strings.Contains(line, "ask") {
				askLines++
			}
		}
	}

	if bidLines != 2 {
		t.Errorf("Expected 2 bid levels, got %d", bidLines)
	}
	if askLines != 1 {
		t.Errorf("Expected 1 ask level, got %d", askLines)
	}
}

func TestRecorderMultipleTrades(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		ObservedPath:  "testdata/book_observed.csv",
		HiddenPath:    "testdata/book_hidden.csv",
		FlushInterval: 10 * time.Millisecond,
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	numTrades := 100
	for i := 0; i < numTrades; i++ {
		trade := &exchange.Trade{
			TradeID: uint64(i),
			Price:   5000000000000 + int64(i)*1000000,
			Qty:     100000000,
			Side:    exchange.Buy,
		}

		event := &Event{
			Type: EventTrade,
			Data: TradeEvent{
				Symbol:    "BTCUSD",
				Trade:     trade,
				Timestamp: int64(1234567890000000000 + i*1000000),
			},
		}

		recorder.OnEvent(event)
	}

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()

	file, err := os.Open("testdata/trades.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0

	for scanner.Scan() {
		lines++
	}

	if lines != numTrades+1 {
		t.Errorf("Expected %d lines (header + %d trades), got %d", numTrades+1, numTrades, lines)
	}
}

func TestRecorderNonBlockingWrite(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		ObservedPath:  "testdata/book_observed.csv",
		HiddenPath:    "testdata/book_hidden.csv",
		FlushInterval: 1 * time.Second,
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	done := make(chan bool)
	go func() {
		for i := 0; i < 20000; i++ {
			trade := &exchange.Trade{
				TradeID: uint64(i),
				Price:   5000000000000,
				Qty:     100000000,
				Side:    exchange.Buy,
			}

			event := &Event{
				Type: EventTrade,
				Data: TradeEvent{
					Symbol:    "BTCUSD",
					Trade:     trade,
					Timestamp: 1234567890000000000,
				},
			}

			recorder.OnEvent(event)
		}
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Recorder blocking on writes")
	}

	recorder.Stop()
}

func TestRecorderGracefulShutdown(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		ObservedPath:  "testdata/book_observed.csv",
		HiddenPath:    "testdata/book_hidden.csv",
		FlushInterval: 10 * time.Millisecond,
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 50; i++ {
		trade := &exchange.Trade{
			TradeID: uint64(i),
			Price:   5000000000000,
			Qty:     100000000,
			Side:    exchange.Buy,
		}

		event := &Event{
			Type: EventTrade,
			Data: TradeEvent{
				Symbol:    "BTCUSD",
				Trade:     trade,
				Timestamp: 1234567890000000000,
			},
		}

		recorder.OnEvent(event)
	}

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()
	time.Sleep(50 * time.Millisecond)

	file, err := os.Open("testdata/trades.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0

	for scanner.Scan() {
		lines++
	}

	if lines != 51 {
		t.Errorf("Expected 51 lines after graceful shutdown, got %d", lines)
	}
}

func TestRecorderSideFormatting(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)
	config := RecorderConfig{
		Symbols:       []string{"BTCUSD"},
		TradesPath:    "testdata/trades.csv",
		ObservedPath:  "testdata/book_observed.csv",
		HiddenPath:    "testdata/book_hidden.csv",
		FlushInterval: 10 * time.Millisecond,
	}

	recorder, err := NewRecorder(1, gateway, config)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	buyTrade := &exchange.Trade{
		TradeID: 1,
		Price:   5000000000000,
		Qty:     100000000,
		Side:    exchange.Buy,
	}

	sellTrade := &exchange.Trade{
		TradeID: 2,
		Price:   5000000000000,
		Qty:     100000000,
		Side:    exchange.Sell,
	}

	recorder.OnEvent(&Event{
		Type: EventTrade,
		Data: TradeEvent{Symbol: "BTCUSD", Trade: buyTrade, Timestamp: 1000},
	})

	recorder.OnEvent(&Event{
		Type: EventTrade,
		Data: TradeEvent{Symbol: "BTCUSD", Trade: sellTrade, Timestamp: 2000},
	})

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()

	file, err := os.Open("testdata/trades.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	foundBuy := false
	foundSell := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, ",1,") && strings.Contains(line, ",buy,") {
			foundBuy = true
		}
		if strings.Contains(line, ",2,") && strings.Contains(line, ",sell,") {
			foundSell = true
		}
	}

	if !foundBuy {
		t.Error("Buy side not correctly formatted")
	}
	if !foundSell {
		t.Error("Sell side not correctly formatted")
	}
}
