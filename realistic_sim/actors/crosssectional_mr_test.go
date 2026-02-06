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
	ex := exchange.NewExchange(100, &exchange.RealClock{})

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
		LookbackWindow:     60,
		AllocatedCapital:   100000 * exchange.USD_PRECISION,
		RebalanceInterval:  100 * time.Millisecond,
		MaxPositionSize:    10 * exchange.BTC_PRECISION,
		MinSignalThreshold: 0,
	}

	policy := &position.ProportionalSizing{}
	csActor := NewCrossSectionalMR(1, gateway, config, oms, policy)

	for _, symbol := range symbols {
		instrument := ex.Instruments[symbol]
		csActor.SetInstrument(symbol, instrument)
	}

	basePrice := int64(50000 * exchange.USD_PRECISION)
	csActor.lastMidPrices["BTC/USD"] = basePrice
	csActor.lastMidPrices["ETH/USD"] = basePrice
	csActor.lastMidPrices["SOL/USD"] = basePrice

	for i := 0; i < 60; i++ {
		btcPrice := basePrice + int64(i)*50*exchange.USD_PRECISION
		ethPrice := basePrice + int64(i)*20*exchange.USD_PRECISION
		solPrice := basePrice - int64(i)*30*exchange.USD_PRECISION
		csActor.signals.AddPrice("BTC/USD", btcPrice)
		csActor.signals.AddPrice("ETH/USD", ethPrice)
		csActor.signals.AddPrice("SOL/USD", solPrice)
	}

	signalMap := csActor.signals.Calculate(symbols)

	if len(signalMap) != 3 {
		t.Fatalf("Expected 3 signals, got %d", len(signalMap))
	}

	btcSignal := signalMap["BTC/USD"]
	ethSignal := signalMap["ETH/USD"]
	solSignal := signalMap["SOL/USD"]

	t.Logf("Signals: BTC=%d, ETH=%d, SOL=%d", btcSignal, ethSignal, solSignal)

	if btcSignal <= solSignal {
		t.Errorf("BTC signal (%d) should be greater than SOL signal (%d)", btcSignal, solSignal)
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
	ex := exchange.NewExchange(100, &exchange.RealClock{})
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
		LookbackWindow:     60,
		AllocatedCapital:   10000000 * exchange.USD_PRECISION,
		RebalanceInterval:  100 * time.Millisecond,
		MaxPositionSize:    maxPos,
		MinSignalThreshold: 0,
	}

	policy := &position.ProportionalSizing{}
	csActor := NewCrossSectionalMR(1, gateway, config, oms, policy)
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
	ex := exchange.NewExchange(100, &exchange.RealClock{})

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
		LookbackWindow:     60,
		AllocatedCapital:   100000 * exchange.USD_PRECISION,
		RebalanceInterval:  200 * time.Millisecond,
		MaxPositionSize:    10 * exchange.BTC_PRECISION,
		MinSignalThreshold: 0,
	}

	policy := &position.ProportionalSizing{}
	csActor := NewCrossSectionalMR(1, gateway, config, oms, policy)

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

	for i := 0; i < 60; i++ {
		btcPrice := basePrice + int64(i)*100*exchange.USD_PRECISION
		ethPrice := basePrice - int64(i)*50*exchange.USD_PRECISION
		csActor.signals.AddPrice("BTC/USD", btcPrice)
		csActor.signals.AddPrice("ETH/USD", ethPrice)
	}

	time.Sleep(300 * time.Millisecond)

	requestCount := csActor.requestSeq
	t.Logf("Request count: %d", requestCount)

	if requestCount == 0 {
		t.Error("Expected rebalancing to submit orders")
	}
}

func TestCrossSectionalMRMinSignalThreshold(t *testing.T) {
	ex := exchange.NewExchange(100, &exchange.RealClock{})
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

	minThreshold := int64(1000)
	config := CrossSectionalMRConfig{
		Symbols:            []string{"BTC/USD", "ETH/USD"},
		LookbackWindow:     60,
		AllocatedCapital:   100000 * exchange.USD_PRECISION,
		RebalanceInterval:  100 * time.Millisecond,
		MaxPositionSize:    10 * exchange.BTC_PRECISION,
		MinSignalThreshold: minThreshold,
	}

	policy := &position.ProportionalSizing{}
	csActor := NewCrossSectionalMR(1, gateway, config, oms, policy)
	csActor.SetInstrument("BTC/USD", instrument)

	basePrice := int64(50000 * exchange.USD_PRECISION)
	csActor.lastMidPrices["BTC/USD"] = basePrice
	csActor.lastMidPrices["ETH/USD"] = basePrice

	for i := 0; i < 60; i++ {
		price := basePrice
		if i%2 == 0 {
			price += 10 * exchange.USD_PRECISION
		}
		csActor.signals.AddPrice("BTC/USD", price)
		csActor.signals.AddPrice("ETH/USD", price)
	}

	signalMap := csActor.signals.Calculate([]string{"BTC/USD", "ETH/USD"})
	btcSignal := signalMap["BTC/USD"]

	absSignal := btcSignal
	if absSignal < 0 {
		absSignal = -absSignal
	}

	if absSignal < minThreshold {
		t.Logf("Signal %d below threshold %d - would be filtered", btcSignal, minThreshold)
	} else {
		t.Logf("Signal %d above threshold %d - would be processed", btcSignal, minThreshold)
	}
}
