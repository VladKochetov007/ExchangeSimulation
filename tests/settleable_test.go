package exchange_test

import (
	"sync/atomic"
	"testing"

	. "exchange_sim/exchange"
)

// customSettleable wraps a SpotInstrument and overrides settlement with custom logic.
// Proves that the exchange dispatches to Settle without a type assertion on the instrument.
type customSettleable struct {
	*SpotInstrument
	settleCallCount atomic.Int64
}

func (c *customSettleable) Settle(ctx SettlementContext) SettlementResult {
	c.settleCallCount.Add(1)
	exec := ctx.Exec
	base, quote := c.BaseAsset(), c.QuoteAsset()
	notional := (exec.Price * exec.Qty) / c.BasePrecision()
	if ctx.TakerOrder.Side == Buy {
		ctx.MutatePerpBalance(exec.TakerClientID, base, exec.Qty)
		ctx.MutatePerpBalance(exec.TakerClientID, quote, -notional)
		ctx.MutatePerpBalance(exec.MakerClientID, base, -exec.Qty)
		ctx.MutatePerpBalance(exec.MakerClientID, quote, notional)
	} else {
		ctx.MutatePerpBalance(exec.TakerClientID, base, -exec.Qty)
		ctx.MutatePerpBalance(exec.TakerClientID, quote, notional)
		ctx.MutatePerpBalance(exec.MakerClientID, base, exec.Qty)
		ctx.MutatePerpBalance(exec.MakerClientID, quote, -notional)
	}
	return SettlementResult{}
}

func TestCustomSettleableInstrument(t *testing.T) {
	ex := NewExchange(10, &RealClock{})

	inst := &customSettleable{
		SpotInstrument: NewSpotInstrument(
			"CUSTOM", "BASE", "QUOTE",
			BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000,
		),
	}
	ex.AddInstrument(inst)

	balances1 := map[string]int64{"BASE": 10 * BTC_PRECISION, "QUOTE": 1_000_000 * USD_PRECISION}
	balances2 := map[string]int64{"BASE": 10 * BTC_PRECISION, "QUOTE": 1_000_000 * USD_PRECISION}
	ex.ConnectClient(1, balances1, &PercentageFee{MakerBps: 0, TakerBps: 0, InQuote: true})
	ex.ConnectClient(2, balances2, &PercentageFee{MakerBps: 0, TakerBps: 0, InQuote: true})

	go ex.HandleClientRequests(ex.Gateways[1])
	go ex.HandleClientRequests(ex.Gateways[2])

	const price = 50_000 * DOLLAR_TICK
	const qty = BTC_PRECISION

	_, rej := InjectLimitOrder(ex, 1, "CUSTOM", Sell, price, qty)
	if rej != "" {
		t.Fatalf("limit order rejected: %v", rej)
	}
	_, rej = InjectMarketOrder(ex, 2, "CUSTOM", Buy, qty)
	if rej != "" {
		t.Fatalf("market order rejected: %v", rej)
	}

	if got := inst.settleCallCount.Load(); got != 1 {
		t.Errorf("Settle called %d times, want 1", got)
	}
}
