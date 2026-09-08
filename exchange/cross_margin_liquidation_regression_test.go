package exchange

import (
	"reflect"
	"testing"
)

type crossMarginLiquidationOutcome struct {
	insurance int64
	balance   int64
	positions map[string]int64
}

func seedCrossMarginLiquidationCase(t *testing.T) (*DefaultExchange, *PerpFutures, *PerpFutures) {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	a := NewPerpFutures("A-PERP", "A", "USD", 1, 1, 1, 1)
	b := NewPerpFutures("B-PERP", "B", "USD", 1, 1, 1, 1)
	ex.AddInstrument(a)
	ex.AddInstrument(b)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.ConnectNewClient(2, nil, &FixedFee{})
	ex.AddPerpBalance(2, "USD", 1_000_000)

	if delta := ex.Positions.UpdatePosition(1, a.Symbol(), 10, 100, Buy, PositionBoth); delta.NewSize != 10 {
		t.Fatalf("A position delta = %#v", delta)
	}
	if delta := ex.Positions.UpdatePosition(1, b.Symbol(), 10, 100, Buy, PositionBoth); delta.NewSize != 10 {
		t.Fatalf("B position delta = %#v", delta)
	}
	if err := a.UpdateFundingRate(50, 50); err != nil {
		t.Fatalf("A mark: %v", err)
	}
	if err := b.UpdateFundingRate(150, 150); err != nil {
		t.Fatalf("B mark: %v", err)
	}
	for _, liquidity := range []struct {
		requestID uint64
		symbol    string
		price     int64
	}{
		{requestID: 1, symbol: a.Symbol(), price: 50},
		{requestID: 2, symbol: b.Symbol(), price: 150},
	} {
		response := ex.PlaceOrder(2, &OrderRequest{
			RequestID: liquidity.requestID, Side: Buy, Type: LimitOrder, Price: liquidity.price,
			Qty: 10, Symbol: liquidity.symbol, TimeInForce: GTC, PositionSide: PositionBoth,
		})
		if !response.Success {
			t.Fatalf("seed %s liquidity rejected: %s", liquidity.symbol, response.Error)
		}
	}
	return ex, a, b
}

func runCrossMarginLiquidationCase(t *testing.T, triggerFirst string) crossMarginLiquidationOutcome {
	t.Helper()
	ex, a, b := seedCrossMarginLiquidationCase(t)
	defer ex.Shutdown()
	if triggerFirst == a.Symbol() {
		ex.CheckLiquidations(a.Symbol(), a, 50)
		ex.CheckLiquidations(b.Symbol(), b, 150)
	} else {
		ex.CheckLiquidations(b.Symbol(), b, 150)
		ex.CheckLiquidations(a.Symbol(), a, 50)
	}
	outcome := crossMarginLiquidationOutcome{
		insurance: ex.ExchangeBalance.InsuranceFund["USD"],
		balance:   ex.Clients[1].PerpBalances["USD"],
		positions: map[string]int64{},
	}
	for _, symbol := range []string{a.Symbol(), b.Symbol()} {
		if position := ex.Positions.GetPosition(1, symbol); position != nil {
			outcome.positions[symbol] = position.Size
		}
	}
	return outcome
}

func TestCrossMarginLiquidationIsInvariantToTriggerSymbol(t *testing.T) {
	aFirst := runCrossMarginLiquidationCase(t, "A-PERP")
	bFirst := runCrossMarginLiquidationCase(t, "B-PERP")
	if !reflect.DeepEqual(aFirst, bFirst) {
		t.Fatalf("trigger order changed cross-margin outcome: A-first=%#v B-first=%#v", aFirst, bFirst)
	}
	if aFirst.insurance != 0 || aFirst.balance != 0 || aFirst.positions["A-PERP"] != 0 || aFirst.positions["B-PERP"] != 0 {
		t.Fatalf("fully priceable portfolio was not closed as one account: %#v", aFirst)
	}
}

func TestCrossMarginLiquidationDefersDeficitDuringPartialPortfolioClose(t *testing.T) {
	ex, a, b := seedCrossMarginLiquidationCase(t)
	defer ex.Shutdown()

	// Leave only half of A's closing liquidity and remove B's liquidity. The
	// account has made a partial attempt, not reached a terminal deficit.
	for orderID := range ex.Books[a.Symbol()].Bids.Orders {
		ex.Books[a.Symbol()].Bids.CancelOrder(orderID)
	}
	for orderID := range ex.Books[b.Symbol()].Bids.Orders {
		ex.Books[b.Symbol()].Bids.CancelOrder(orderID)
	}
	response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 3, Side: Buy, Type: LimitOrder, Price: 50, Qty: 5,
		Symbol: a.Symbol(), TimeInForce: GTC, PositionSide: PositionBoth,
	})
	if !response.Success {
		t.Fatalf("partial A liquidity rejected: %s", response.Error)
	}

	ex.CheckLiquidations(a.Symbol(), a, 50)
	if got := ex.ExchangeBalance.InsuranceFund["USD"]; got != 0 {
		t.Fatalf("partial close socialized an interim deficit: %d", got)
	}
	if position := ex.Positions.GetPosition(1, a.Symbol()); position == nil || position.Size != 5 {
		t.Fatalf("partial A position = %#v, want residual size 5; bids=%d last=%#v balance=%d", position, len(ex.Books[a.Symbol()].Bids.Orders), ex.Books[a.Symbol()].LastTrade, ex.Clients[1].PerpBalances["USD"])
	}
	if position := ex.Positions.GetPosition(1, b.Symbol()); position == nil || position.Size != 10 {
		t.Fatalf("B sibling = %#v, want live size 10", position)
	}
}
