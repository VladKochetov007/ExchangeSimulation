package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// newSpotPair builds an exchange with one BTC/USD spot instrument and two
// funded clients using the given fee plan.
func newSpotPair(t *testing.T, fee FeeModel) *Exchange {
	t.Helper()
	ex := NewExchange(4, nil)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/100))
	balances := map[string]int64{
		"BTC": 100 * BTC_PRECISION,
		"USD": 10_000_000 * USD_PRECISION,
	}
	ex.ConnectNewClient(1, balances, fee)
	ex.ConnectNewClient(2, balances, fee)
	return ex
}

func totalUSD(ex *Exchange) int64 {
	sum := ex.ExchangeBalance.FeeRevenue["USD"]
	for _, c := range ex.Clients {
		sum += c.Balances["USD"]
	}
	return sum
}

func totalBTC(ex *Exchange) int64 {
	sum := ex.ExchangeBalance.FeeRevenue["BTC"]
	for _, c := range ex.Clients {
		sum += c.Balances["BTC"]
	}
	return sum
}

// Money conservation: quote leaves buyers, reaches sellers, fees reach the
// exchange — nothing minted, nothing burned.
func TestSpotConservationWithFees(t *testing.T) {
	fee := &PercentageFee{MakerBps: 2, TakerBps: 8, InQuote: true}
	ex := newSpotPair(t, fee)

	usdBefore := totalUSD(ex)
	btcBefore := totalBTC(ex)

	price := int64(50_000) * USD_PRECISION
	if _, rej := InjectLimitOrder(ex, 1, "BTC/USD", Sell, price, BTC_PRECISION); rej != "" {
		t.Fatalf("maker rejected: %s", rej)
	}
	if _, rej := InjectMarketOrder(ex, 2, "BTC/USD", Buy, BTC_PRECISION); rej != "" {
		t.Fatalf("taker rejected: %s", rej)
	}

	if got := totalUSD(ex); got != usdBefore {
		t.Errorf("USD not conserved: before %d after %d (delta %d)", usdBefore, got, got-usdBefore)
	}
	if got := totalBTC(ex); got != btcBefore {
		t.Errorf("BTC not conserved: before %d after %d (delta %d)", btcBefore, got, got-btcBefore)
	}
	if fees := ex.ExchangeBalance.FeeRevenue["USD"]; fees <= 0 {
		t.Errorf("expected positive fee revenue, got %d", fees)
	}
}

// A market order reserves nothing, so its settlement must not release funds
// reserved for the client's OTHER resting orders.
func TestMarketOrderDoesNotEatOtherReservations(t *testing.T) {
	ex := newSpotPair(t, &PercentageFee{})

	price := int64(50_000) * USD_PRECISION
	// Client 2 rests a limit buy far below market: reservation must survive.
	if _, rej := InjectLimitOrder(ex, 2, "BTC/USD", Buy, 40_000*USD_PRECISION, BTC_PRECISION); rej != "" {
		t.Fatalf("resting buy rejected: %s", rej)
	}
	restingReserved := ex.Clients[2].Reserved["USD"]
	if restingReserved != 40_000*USD_PRECISION {
		t.Fatalf("unexpected resting reservation: %d", restingReserved)
	}

	// Client 1 offers, client 2 lifts it with a market buy.
	if _, rej := InjectLimitOrder(ex, 1, "BTC/USD", Sell, price, BTC_PRECISION); rej != "" {
		t.Fatalf("maker rejected: %s", rej)
	}
	if _, rej := InjectMarketOrder(ex, 2, "BTC/USD", Buy, BTC_PRECISION); rej != "" {
		t.Fatalf("market buy rejected: %s", rej)
	}

	if got := ex.Clients[2].Reserved["USD"]; got != restingReserved {
		t.Errorf("market fill consumed resting order's reservation: reserved %d, want %d", got, restingReserved)
	}
}

// Taker limit buy crossing a lower ask fills at the maker's price; the
// reservation delta (limit vs fill price) must be released, not leaked.
func TestPriceImprovementReleasesFullReservation(t *testing.T) {
	ex := newSpotPair(t, &PercentageFee{})

	askPrice := int64(49_000) * USD_PRECISION
	limitPrice := int64(50_000) * USD_PRECISION
	if _, rej := InjectLimitOrder(ex, 1, "BTC/USD", Sell, askPrice, BTC_PRECISION); rej != "" {
		t.Fatalf("maker rejected: %s", rej)
	}
	// Crosses immediately, fully filled at 49k though reserved at 50k.
	if _, rej := InjectLimitOrder(ex, 2, "BTC/USD", Buy, limitPrice, BTC_PRECISION); rej != "" {
		t.Fatalf("taker rejected: %s", rej)
	}

	if got := ex.Clients[2].Reserved["USD"]; got != 0 {
		t.Errorf("price-improvement leak: %d USD units still reserved after full fill", got)
	}
	wantSpend := int64(49_000 * USD_PRECISION)
	gotBalance := ex.Clients[2].Balances["USD"]
	if gotBalance != 10_000_000*USD_PRECISION-wantSpend {
		t.Errorf("buyer balance %d, want %d", gotBalance, 10_000_000*USD_PRECISION-wantSpend)
	}
}

// Cancel after partial fill must release exactly the remainder (with fee
// headroom), leaving zero reservation dust.
func TestCancelAfterPartialFillLeavesNoDust(t *testing.T) {
	fee := &PercentageFee{MakerBps: 2, TakerBps: 8, InQuote: true}
	ex := newSpotPair(t, fee)

	price := int64(50_000) * USD_PRECISION
	orderID, rej := InjectLimitOrder(ex, 2, "BTC/USD", Buy, price, 2*BTC_PRECISION)
	if rej != "" {
		t.Fatalf("buy rejected: %s", rej)
	}
	// Partial fill: 0.5 BTC sold into the 2 BTC bid.
	if _, rej := InjectMarketOrder(ex, 1, "BTC/USD", Sell, BTC_PRECISION/2); rej != "" {
		t.Fatalf("sell rejected: %s", rej)
	}

	resp := ex.CancelOrder(2, &CancelRequest{RequestID: 99, OrderID: orderID})
	if !resp.Success {
		t.Fatalf("cancel failed: %s", resp.Error)
	}

	if got := ex.Clients[2].Reserved["USD"]; got != 0 {
		t.Errorf("reservation dust after cancel: %d USD units", got)
	}
}

// FOK that cannot fill must leave the book untouched — no consumed maker
// quantity, no phantom fills.
func TestFOKRejectLeavesBookIntact(t *testing.T) {
	ex := newSpotPair(t, &PercentageFee{})

	price := int64(50_000) * USD_PRECISION
	if _, rej := InjectLimitOrder(ex, 1, "BTC/USD", Sell, price, BTC_PRECISION); rej != "" {
		t.Fatalf("maker rejected: %s", rej)
	}

	// FOK buy for 2 BTC when only 1 rests: must reject without touching the book.
	gw := ex.Gateways[2]
	gw.RequestCh <- Request{
		Type: ReqPlaceOrder,
		OrderReq: &OrderRequest{
			RequestID: 777, Side: Buy, Type: LimitOrder, Price: price,
			Qty: 2 * BTC_PRECISION, Symbol: "BTC/USD", TimeInForce: FOK,
		},
	}
	resp := <-gw.ResponseCh
	if resp.Success || resp.Error != RejectFOKNotFilled {
		t.Fatalf("expected FOK reject, got success=%v err=%s", resp.Success, resp.Error)
	}

	book := ex.GetBook("BTC/USD")
	if book.Asks.Best == nil || book.Asks.Best.TotalQty != BTC_PRECISION {
		t.Errorf("FOK reject mutated the book: ask qty = %v", book.Asks.Best)
	}
	if got := ex.Clients[2].Reserved["USD"]; got != 0 {
		t.Errorf("FOK reject leaked reservation: %d", got)
	}
}

// An order must be able to trade through a level occupied only by the same
// client's resting order and reach deeper liquidity from other clients.
func TestSelfOrderDoesNotBlockDeeperLiquidity(t *testing.T) {
	ex := newSpotPair(t, &PercentageFee{})

	// Client 2's own ask at 50k, client 1's ask behind it at 51k.
	if _, rej := InjectLimitOrder(ex, 2, "BTC/USD", Sell, 50_000*USD_PRECISION, BTC_PRECISION); rej != "" {
		t.Fatalf("own ask rejected: %s", rej)
	}
	if _, rej := InjectLimitOrder(ex, 1, "BTC/USD", Sell, 51_000*USD_PRECISION, BTC_PRECISION); rej != "" {
		t.Fatalf("deep ask rejected: %s", rej)
	}

	// Client 2 market buys 1 BTC: must skip its own 50k ask and hit 51k.
	if _, rej := InjectMarketOrder(ex, 2, "BTC/USD", Buy, BTC_PRECISION); rej != "" {
		t.Fatalf("market buy rejected: %s", rej)
	}

	if got := ex.Clients[2].Balances["BTC"]; got != 101*BTC_PRECISION {
		t.Errorf("self-order blockade: buyer BTC = %d, want %d", got, 101*BTC_PRECISION)
	}
	book := ex.GetBook("BTC/USD")
	if book.Asks.Best == nil || book.Asks.Best.Price != 50_000*USD_PRECISION {
		t.Errorf("own resting ask should survive at 50k")
	}
}

// Fee headroom: with taker fees in quote, a buyer at full balance utilization
// must be rejected rather than driven negative by the fee.
func TestFeeHeadroomPreventsNegativeBalance(t *testing.T) {
	fee := &PercentageFee{MakerBps: 0, TakerBps: 100, InQuote: true} // 1% taker
	ex := NewExchange(4, nil)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, BTC_PRECISION/100))
	// Buyer has exactly the notional, NOT the fee.
	ex.ConnectNewClient(1, map[string]int64{"BTC": 10 * BTC_PRECISION}, fee)
	ex.ConnectNewClient(2, map[string]int64{"USD": 50_000 * USD_PRECISION}, fee)

	price := int64(50_000) * USD_PRECISION
	if _, rej := InjectLimitOrder(ex, 1, "BTC/USD", Sell, price, BTC_PRECISION); rej != "" {
		t.Fatalf("maker rejected: %s", rej)
	}
	// Limit buy at 50k × 1 BTC = full balance; 1% fee cannot be covered.
	if _, rej := InjectLimitOrder(ex, 2, "BTC/USD", Buy, price, BTC_PRECISION); rej != RejectInsufficientBalance {
		t.Fatalf("expected insufficient-balance reject, got %q", rej)
	}
	if got := ex.Clients[2].Balances["USD"]; got < 0 {
		t.Errorf("buyer driven negative: %d", got)
	}
}
