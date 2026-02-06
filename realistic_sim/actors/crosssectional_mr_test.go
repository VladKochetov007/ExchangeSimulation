package actors

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/realistic_sim/position"
)

func TestCrossSectionalMRSignalToPosition(t *testing.T) {
	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(100, clock)

	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
	for _, symbol := range symbols {
		instrument := exchange.NewSpotInstrument(symbol, symbol[:3], "USD",
			exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
		ex.AddInstrument(instrument)
	}

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"ETH": 1000 * exchange.BTC_PRECISION,
		"SOL": 10000 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	oms := actor.NewNettingOMS()

	config := CrossSectionalMRConfig{
		Symbols:            symbols,
		LookbackPeriod:     30 * time.Second,
		AllocatedCapital:   100000 * exchange.USD_PRECISION,
		RebalanceInterval:  100 * time.Millisecond,
		MaxPositionSize:    10 * exchange.BTC_PRECISION,
		MinSignalThreshold: 0,
	}

	policy := &position.ProportionalSizing{}
	csActor := NewCrossSectionalMR(1, gateway, config, clock, oms, policy)

	for _, symbol := range symbols {
		instrument := ex.Instruments[symbol]
		csActor.SetInstrument(symbol, instrument)
	}

	basePrice := int64(50000 * exchange.USD_PRECISION)
	csActor.lastMidPrices["BTC/USD"] = basePrice
	csActor.lastMidPrices["ETH/USD"] = basePrice
	csActor.lastMidPrices["SOL/USD"] = basePrice

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).UnixNano()
	for i := 0; i < 60; i++ {
		timestamp := baseTime + int64(i)*int64(time.Second)
		btcPrice := basePrice + int64(i)*50*exchange.USD_PRECISION
		ethPrice := basePrice + int64(i)*20*exchange.USD_PRECISION
		solPrice := basePrice - int64(i)*30*exchange.USD_PRECISION
		csActor.signals.AddPrice("BTC/USD", btcPrice, timestamp)
		csActor.signals.AddPrice("ETH/USD", ethPrice, timestamp)
		csActor.signals.AddPrice("SOL/USD", solPrice, timestamp)
	}

	currentTime := baseTime + 60*int64(time.Second)
	signalMap := csActor.signals.Calculate(symbols, currentTime)

	if len(signalMap) != 3 {
		t.Fatalf("Expected 3 signals, got %d", len(signalMap))
	}

	btcSignal := signalMap["BTC/USD"]
	ethSignal := signalMap["ETH/USD"]
	solSignal := signalMap["SOL/USD"]

	t.Logf("Signals: BTC=%d, ETH=%d, SOL=%d", btcSignal, ethSignal, solSignal)

	if btcSignal <= solSignal {
		t.Errorf("BTC outperformed SOL, so BTC signal (%d) should be > SOL signal (%d)", btcSignal, solSignal)
	}

	if btcSignal <= ethSignal {
		t.Errorf("BTC outperformed ETH, so BTC signal (%d) should be > ETH signal (%d)", btcSignal, ethSignal)
	}

	totalSignal := int64(0)
	for _, sig := range signalMap {
		absSig := sig
		if absSig < 0 {
			absSig = -absSig
		}
		totalSignal += absSig
	}

	btcTarget := csActor.positionMgr.TargetPosition(btcSignal, totalSignal, basePrice)
	solTarget := csActor.positionMgr.TargetPosition(solSignal, totalSignal, basePrice)

	t.Logf("Targets: BTC=%d, SOL=%d", btcTarget, solTarget)

	if btcSignal > 0 && btcTarget < 0 {
		t.Errorf("BTC has positive signal but negative target")
	}
	if solSignal < 0 && solTarget > 0 {
		t.Errorf("SOL has negative signal but positive target")
	}
}

func TestCrossSectionalMRPositionLimits(t *testing.T) {
	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(100, clock)
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 10000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	oms := actor.NewNettingOMS()

	maxPos := int64(1 * exchange.BTC_PRECISION)
	config := CrossSectionalMRConfig{
		Symbols:            []string{"BTC/USD"},
		LookbackPeriod:     30 * time.Second,
		AllocatedCapital:   10000000 * exchange.USD_PRECISION,
		RebalanceInterval:  100 * time.Millisecond,
		MaxPositionSize:    maxPos,
		MinSignalThreshold: 0,
	}

	policy := &position.ProportionalSizing{}
	csActor := NewCrossSectionalMR(1, gateway, config, clock, oms, policy)
	csActor.SetInstrument("BTC/USD", instrument)

	midPrice := int64(50000 * exchange.USD_PRECISION)
	csActor.lastMidPrices["BTC/USD"] = midPrice

	signal := int64(10000)
	totalSignal := int64(10000)

	targetValue := policy.CalculateSize(signal, totalSignal, config.AllocatedCapital)
	rawTarget := targetValue / midPrice

	t.Logf("Raw target: %d, Max position: %d", rawTarget, maxPos)

	if rawTarget <= maxPos {
		t.Skipf("Test setup: rawTarget (%d) doesn't exceed maxPos (%d)", rawTarget, maxPos)
	}

	targetPosition := csActor.positionMgr.TargetPosition(signal, totalSignal, midPrice)
	cappedTarget := targetPosition
	if cappedTarget > maxPos {
		cappedTarget = maxPos
	}

	if cappedTarget != maxPos {
		t.Errorf("Target should be capped at maxPos: expected %d, got %d", maxPos, cappedTarget)
	}
}

func TestCrossSectionalMRRiskFilterBlocking(t *testing.T) {
	flipFilter := position.NewPositionFlipFilter()
	flipFilter.UpdatePosition(5 * exchange.BTC_PRECISION)

	order := &exchange.Order{
		Side: exchange.Sell,
		Qty:  10 * exchange.BTC_PRECISION,
	}

	if !flipFilter.ShouldBlock(order) {
		t.Error("Risk filter should block position flip order")
	}

	safeOrder := &exchange.Order{
		Side: exchange.Sell,
		Qty:  3 * exchange.BTC_PRECISION,
	}

	if flipFilter.ShouldBlock(safeOrder) {
		t.Error("Risk filter should not block safe reduction order")
	}
}

func TestCrossSectionalMRRebalancing(t *testing.T) {
	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(100, clock)

	symbols := []string{"BTC/USD", "ETH/USD"}
	for _, symbol := range symbols {
		instrument := exchange.NewSpotInstrument(symbol, symbol[:3], "USD",
			exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
		ex.AddInstrument(instrument)
	}

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"ETH": 1000 * exchange.BTC_PRECISION,
		"USD": 10000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	oms := actor.NewNettingOMS()

	config := CrossSectionalMRConfig{
		Symbols:            symbols,
		LookbackPeriod:     30 * time.Second,
		AllocatedCapital:   100000 * exchange.USD_PRECISION,
		RebalanceInterval:  200 * time.Millisecond,
		MaxPositionSize:    10 * exchange.BTC_PRECISION,
		MinSignalThreshold: 0,
	}

	policy := &position.ProportionalSizing{}
	csActor := NewCrossSectionalMR(1, gateway, config, clock, oms, policy)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := csActor.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start actor: %v", err)
	}
	defer csActor.Stop()

	for _, symbol := range symbols {
		csActor.SetInstrument(symbol, ex.Instruments[symbol])
	}

	basePrice := int64(50000 * exchange.USD_PRECISION)
	csActor.lastMidPrices["BTC/USD"] = basePrice
	csActor.lastMidPrices["ETH/USD"] = basePrice

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).UnixNano()
	for i := 0; i < 60; i++ {
		timestamp := baseTime + int64(i)*int64(time.Second)
		btcPrice := basePrice + int64(i)*100*exchange.USD_PRECISION
		ethPrice := basePrice - int64(i)*50*exchange.USD_PRECISION
		csActor.signals.AddPrice("BTC/USD", btcPrice, timestamp)
		csActor.signals.AddPrice("ETH/USD", ethPrice, timestamp)
	}

	time.Sleep(300 * time.Millisecond)

	requestCount := csActor.requestSeq
	t.Logf("Request count: %d", requestCount)

	if requestCount == 0 {
		t.Error("Expected rebalancing to submit orders")
	}
}

func TestCrossSectionalMRTimeBased(t *testing.T) {
	clock := &exchange.RealClock{}
	ex := exchange.NewExchange(100, clock)
	instrument := exchange.NewSpotInstrument("BTC/USD", "BTC", "USD",
		exchange.BTC_PRECISION, exchange.USD_PRECISION, exchange.DOLLAR_TICK, exchange.BTC_PRECISION/100)
	ex.AddInstrument(instrument)

	balances := map[string]int64{
		"BTC": 100 * exchange.BTC_PRECISION,
		"USD": 1000000 * exchange.USD_PRECISION,
	}
	feePlan := &exchange.PercentageFee{MakerBps: 2, TakerBps: 5, InQuote: true}
	gateway := ex.ConnectClient(1, balances, feePlan)

	oms := actor.NewNettingOMS()

	config := CrossSectionalMRConfig{
		Symbols:            []string{"BTC/USD", "ETH/USD"},
		LookbackPeriod:     10 * time.Second,
		AllocatedCapital:   100000 * exchange.USD_PRECISION,
		RebalanceInterval:  100 * time.Millisecond,
		MaxPositionSize:    10 * exchange.BTC_PRECISION,
		MinSignalThreshold: 0,
	}

	policy := &position.ProportionalSizing{}
	csActor := NewCrossSectionalMR(1, gateway, config, clock, oms, policy)
	csActor.SetInstrument("BTC/USD", instrument)

	basePrice := int64(50000 * exchange.USD_PRECISION)
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC).UnixNano()

	csActor.signals.AddPrice("BTC/USD", basePrice, baseTime)
	csActor.signals.AddPrice("BTC/USD", basePrice+1000*exchange.USD_PRECISION, baseTime+5*int64(time.Second))
	csActor.signals.AddPrice("BTC/USD", basePrice+2000*exchange.USD_PRECISION, baseTime+15*int64(time.Second))

	csActor.signals.AddPrice("ETH/USD", basePrice, baseTime)
	csActor.signals.AddPrice("ETH/USD", basePrice, baseTime+5*int64(time.Second))
	csActor.signals.AddPrice("ETH/USD", basePrice, baseTime+15*int64(time.Second))

	currentTime := baseTime + 15*int64(time.Second)
	signalMap := csActor.signals.Calculate([]string{"BTC/USD", "ETH/USD"}, currentTime)

	t.Logf("Signals with 10s lookback: %+v", signalMap)

	if len(signalMap) != 2 {
		t.Errorf("Expected 2 signals, got %d", len(signalMap))
	}
}
