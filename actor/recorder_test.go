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

	btcusd := exchange.NewPerpFutures("BTCUSD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	instruments := map[string]exchange.Instrument{
		"BTCUSD": btcusd,
	}

	config := RecorderConfig{
		OutputDir:          "testdata",
		Symbols:            []string{"BTCUSD"},
		FlushInterval:      100 * time.Millisecond,
		RecordTrades:       true,
		RecordOrderbook:    true,
		RecordOpenInterest: true,
		RecordFunding:      true,
	}

	recorder, err := NewRecorder(1, gateway, config, instruments)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Stop()

	if _, err := os.Stat("testdata/BTCUSD_PERP_trades.csv"); os.IsNotExist(err) {
		t.Error("BTCUSD_PERP_trades.csv not created")
	}
	if _, err := os.Stat("testdata/BTCUSD_PERP_orderbook.csv"); os.IsNotExist(err) {
		t.Error("BTCUSD_PERP_orderbook.csv not created")
	}
	if _, err := os.Stat("testdata/BTCUSD_PERP_openinterest.csv"); os.IsNotExist(err) {
		t.Error("BTCUSD_PERP_openinterest.csv not created")
	}
	if _, err := os.Stat("testdata/BTCUSD_PERP_funding.csv"); os.IsNotExist(err) {
		t.Error("BTCUSD_PERP_funding.csv not created")
	}
}

func TestRecorderWritesTrade(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	instruments := map[string]exchange.Instrument{
		"BTCUSD": btcusd,
	}

	config := RecorderConfig{
		OutputDir:       "testdata",
		Symbols:         []string{"BTCUSD"},
		FlushInterval:   10 * time.Millisecond,
		RecordTrades:    true,
		RecordOrderbook: false,
	}

	recorder, err := NewRecorder(1, gateway, config, instruments)
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

	file, err := os.Open("testdata/BTCUSD_SPOT_trades.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	foundTrade := false

	for scanner.Scan() {
		line := scanner.Text()
		lines++
		if strings.Contains(line, "1234567890000000000") && strings.Contains(line, "123") {
			foundTrade = true
		}
	}

	if lines < 2 {
		t.Errorf("expected at least 2 lines (header + trade), got %d", lines)
	}

	if !foundTrade {
		t.Error("trade not found in output")
	}
}

func TestRecorderWritesSnapshot(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)

	ethusd := exchange.NewSpotInstrument("ETHUSD", "ETH", "USD", exchange.ETH_PRECISION, exchange.USD_PRECISION, exchange.ETH_PRECISION/100, exchange.ETH_PRECISION/1000)
	instruments := map[string]exchange.Instrument{
		"ETHUSD": ethusd,
	}

	config := RecorderConfig{
		OutputDir:       "testdata",
		Symbols:         []string{"ETHUSD"},
		FlushInterval:   10 * time.Millisecond,
		RecordTrades:    false,
		RecordOrderbook: true,
	}

	recorder, err := NewRecorder(1, gateway, config, instruments)
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
			SeqNum:    42,
		},
	}

	recorder.OnEvent(event)

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()

	file, err := os.Open("testdata/ETHUSD_SPOT_orderbook.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	foundSnapshot := false

	for scanner.Scan() {
		line := scanner.Text()
		lines++
		if strings.Contains(line, "snapshot") && strings.Contains(line, "3000000000000") {
			foundSnapshot = true
		}
	}

	if lines < 2 {
		t.Errorf("expected at least 2 lines, got %d", lines)
	}

	if !foundSnapshot {
		t.Error("snapshot not found in output")
	}
}

func TestRecorderSeparateHiddenFiles(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	instruments := map[string]exchange.Instrument{
		"BTCUSD": btcusd,
	}

	config := RecorderConfig{
		OutputDir:           "testdata",
		Symbols:             []string{"BTCUSD"},
		FlushInterval:       10 * time.Millisecond,
		RecordOrderbook:     true,
		SeparateHiddenFiles: true,
	}

	recorder, err := NewRecorder(1, gateway, config, instruments)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Stop()

	if _, err := os.Stat("testdata/BTCUSD_SPOT_orderbook.csv"); os.IsNotExist(err) {
		t.Error("visible orderbook file not created")
	}
	if _, err := os.Stat("testdata/BTCUSD_SPOT_orderbook_hidden.csv"); os.IsNotExist(err) {
		t.Error("hidden orderbook file not created")
	}
}

func TestRecorderWritesDelta(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)

	btcusd := exchange.NewSpotInstrument("BTCUSD", "BTC", "USD", 100000000, 1000000, exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	instruments := map[string]exchange.Instrument{
		"BTCUSD": btcusd,
	}

	config := RecorderConfig{
		OutputDir:       "testdata",
		Symbols:         []string{"BTCUSD"},
		FlushInterval:   10 * time.Millisecond,
		RecordOrderbook: true,
	}

	recorder, err := NewRecorder(1, gateway, config, instruments)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	delta := &exchange.BookDelta{
		Side:       exchange.Buy,
		Price:      5000000000000,
		VisibleQty: 100000000,
	}

	event := &Event{
		Type: EventBookDelta,
		Data: BookDeltaEvent{
			Symbol:    "BTCUSD",
			Delta:     delta,
			Timestamp: 1234567890000000000,
			SeqNum:    42,
		},
	}

	recorder.OnEvent(event)

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()

	file, err := os.Open("testdata/BTCUSD_SPOT_orderbook.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	foundDelta := false

	for scanner.Scan() {
		line := scanner.Text()
		lines++
		if strings.Contains(line, "delta") && strings.Contains(line, "5000000000000") {
			foundDelta = true
		}
	}

	if lines < 2 {
		t.Errorf("expected at least 2 lines, got %d", lines)
	}

	if !foundDelta {
		t.Error("delta not found in output")
	}
}

func TestRecorderWritesOpenInterest(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)

	btcusd := exchange.NewPerpFutures("BTCUSD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	instruments := map[string]exchange.Instrument{
		"BTCUSD": btcusd,
	}

	config := RecorderConfig{
		OutputDir:          "testdata",
		Symbols:            []string{"BTCUSD"},
		FlushInterval:      10 * time.Millisecond,
		RecordOpenInterest: true,
	}

	recorder, err := NewRecorder(1, gateway, config, instruments)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	event := &Event{
		Type: EventOpenInterest,
		Data: OpenInterestEvent{
			Symbol: "BTCUSD",
			OpenInterest: &exchange.OpenInterest{
				Symbol:         "BTCUSD",
				TotalContracts: 123456789,
				Timestamp:      1234567890000000000,
			},
			Timestamp: 1234567890000000000,
		},
	}

	recorder.OnEvent(event)

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()

	file, err := os.Open("testdata/BTCUSD_PERP_openinterest.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	foundOI := false

	for scanner.Scan() {
		line := scanner.Text()
		lines++
		if strings.Contains(line, "1234567890000000000") && strings.Contains(line, "123456789") {
			foundOI = true
		}
	}

	if lines < 2 {
		t.Errorf("expected at least 2 lines, got %d", lines)
	}

	if !foundOI {
		t.Error("open interest not found in output")
	}
}

func TestRecorderWritesFunding(t *testing.T) {
	os.MkdirAll("testdata", 0755)
	defer os.RemoveAll("testdata")

	gateway := exchange.NewClientGateway(1)

	btcusd := exchange.NewPerpFutures("BTCUSD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION,
		exchange.DOLLAR_TICK, exchange.SATOSHI/1000)
	instruments := map[string]exchange.Instrument{
		"BTCUSD": btcusd,
	}

	config := RecorderConfig{
		OutputDir:     "testdata",
		Symbols:       []string{"BTCUSD"},
		FlushInterval: 10 * time.Millisecond,
		RecordFunding: true,
	}

	recorder, err := NewRecorder(1, gateway, config, instruments)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := recorder.Start(ctx); err != nil {
		t.Fatal(err)
	}

	event := &Event{
		Type: EventFundingUpdate,
		Data: FundingUpdateEvent{
			Symbol: "BTCUSD",
			FundingRate: &exchange.FundingRate{
				Symbol:      "BTCUSD",
				Rate:        12345,
				NextFunding: 1234567890000000000 + 28800,
				Interval:    28800,
				MarkPrice:   5000000000000,
			},
			Timestamp: 1234567890000000000,
		},
	}

	recorder.OnEvent(event)

	time.Sleep(50 * time.Millisecond)
	recorder.Stop()

	file, err := os.Open("testdata/BTCUSD_PERP_funding.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	foundFunding := false

	for scanner.Scan() {
		line := scanner.Text()
		lines++
		if strings.Contains(line, "1234567890000000000") && strings.Contains(line, "12345") {
			foundFunding = true
		}
	}

	if lines < 2 {
		t.Errorf("expected at least 2 lines, got %d", lines)
	}

	if !foundFunding {
		t.Error("funding rate not found in output")
	}
}
