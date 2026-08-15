package exchange

import (
	"math"
	"sync"
	"testing"
)

type autoBorrowRollbackLogger struct {
	mu               sync.Mutex
	borrows          []BorrowEvent
	repays           []RepayEvent
	rollbackBalances []BalanceChangeEvent
}

func (l *autoBorrowRollbackLogger) LogEvent(_ int64, _ uint64, eventName string, event any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch eventName {
	case "borrow":
		if borrow, ok := event.(BorrowEvent); ok {
			l.borrows = append(l.borrows, borrow)
		}
	case "repay":
		if repay, ok := event.(RepayEvent); ok {
			l.repays = append(l.repays, repay)
		}
	case "balance_change":
		if change, ok := event.(BalanceChangeEvent); ok && change.Reason == "auto_borrow_rollback" {
			l.rollbackBalances = append(l.rollbackBalances, change)
		}
	}
}

func TestMarketSpotCostOverflowIsRejectedBeforeMatching(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": math.MaxInt64}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})
	ex.ConnectNewClient(3, map[string]int64{"X": 1}, &FixedFee{})

	firstPrice := int64(math.MaxInt64 / 2)
	for clientID, price := range map[uint64]int64{2: firstPrice, 3: firstPrice + 2} {
		response := ex.PlaceOrder(clientID, &OrderRequest{
			RequestID: clientID, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
			Price: price, Qty: 1, TimeInForce: GTC, Visibility: Normal,
		})
		if !response.Success {
			t.Fatalf("seed ask %d rejected: %s", clientID, response.Error)
		}
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 10, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 2, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectInsufficientBalance {
		t.Fatalf("overflowing sweep response = %+v, want insufficient-balance rejection", response)
	}
	if got := ex.Clients[1].Balances["USD"]; got != math.MaxInt64 {
		t.Fatalf("rejected sweep changed buyer balance: got %d want %d", got, int64(math.MaxInt64))
	}
	if got := ex.Books["X/USD"].Asks.Best.TotalQty; got != 1 {
		t.Fatalf("rejected sweep consumed best depth: got %d want 1", got)
	}
}

func TestRestingLevelAggregateOverflowIsRejectedBeforeReservation(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": math.MaxInt64}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"USD": 1}, &FixedFee{})

	first := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 1, Qty: math.MaxInt64, TimeInForce: GTC, Visibility: Normal,
	})
	if !first.Success {
		t.Fatalf("max-size resting bid rejected: %s", first.Error)
	}

	second := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if second.Success || second.Error != RejectInvalidQty {
		t.Fatalf("overflowing resting bid = %+v, want invalid-qty rejection", second)
	}
	if got := ex.Books["X/USD"].Bids.Best.TotalQty; got != math.MaxInt64 {
		t.Fatalf("rejected bid changed level aggregate: got %d", got)
	}
	if got := ex.Clients[2].Reserved["USD"]; got != 0 {
		t.Fatalf("rejected bid leaked reservation: got %d", got)
	}
}

func TestMarketForeignFeeAggregationOverflowIsRejectedBeforeMatching(t *testing.T) {
	const orderQty = int64(5)
	feeAmount := int64(math.MaxInt64 / 2)

	ex := NewExchange(6, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": orderQty, "BNB": math.MaxInt64}, &FixedFee{
		TakerFee: Fee{Asset: "BNB", Amount: feeAmount},
	})
	for clientID := uint64(2); clientID < uint64(2+orderQty); clientID++ {
		ex.ConnectNewClient(clientID, map[string]int64{"X": 1}, &FixedFee{})
		response := ex.PlaceOrder(clientID, &OrderRequest{
			RequestID: clientID, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
			Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
		})
		if !response.Success {
			t.Fatalf("seed ask %d rejected: %s", clientID, response.Error)
		}
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 10, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: orderQty, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectInsufficientBalance {
		t.Fatalf("overflowing foreign-fee sweep response = %+v, want insufficient-balance rejection", response)
	}
	if got := ex.Clients[1].Balances["BNB"]; got != math.MaxInt64 {
		t.Fatalf("rejected sweep changed foreign-fee balance: got %d want %d", got, int64(math.MaxInt64))
	}
	if got := len(ex.Books["X/USD"].Asks.Orders); got != int(orderQty) {
		t.Fatalf("rejected sweep consumed ask depth: got %d orders want %d", got, orderQty)
	}
}

func TestMarketForeignFeeUsesConfiguredMatcherAllocations(t *testing.T) {
	ex := NewExchange(4, &RealClock{})
	defer ex.Shutdown()
	ex.Matcher = NewProRataMatcher()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": 3, "BNB": 2}, &FixedFee{
		TakerFee: Fee{Asset: "BNB", Amount: 1},
	})
	for clientID := uint64(2); clientID <= 4; clientID++ {
		ex.ConnectNewClient(clientID, map[string]int64{"X": 2}, &FixedFee{})
		response := ex.PlaceOrder(clientID, &OrderRequest{
			RequestID: clientID, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
			Price: 1, Qty: 2, TimeInForce: GTC, Visibility: Normal,
		})
		if !response.Success {
			t.Fatalf("seed ask %d rejected: %s", clientID, response.Error)
		}
	}
	probe := &Order{ClientID: 1, Side: Buy, Type: Market, Qty: 3}
	executions, ok := ex.previewMarketExecutions(ex.Books["X/USD"], probe)
	if !ok {
		t.Fatal("pro-rata preview failed")
	}
	if got := len(executions); got != 3 {
		releasePreviewExecutions(executions)
		t.Fatalf("pro-rata preview execution count = %d, want 3", got)
	}
	releasePreviewExecutions(executions)

	// Pro-rata splits this three-unit market order across three makers, so its
	// fixed BNB fee is charged three times. A price-time fee walk would see two
	// fills and allow the final settlement debit below zero.
	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 10, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 3, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectInsufficientBalance {
		t.Fatalf("pro-rata foreign-fee sweep response = %+v, want insufficient-balance rejection", response)
	}
	if got := ex.Clients[1].Balances["BNB"]; got != 2 {
		t.Fatalf("rejected pro-rata sweep changed BNB balance: got %d want 2", got)
	}
	if got := len(ex.Books["X/USD"].Asks.Orders); got != 3 {
		t.Fatalf("rejected pro-rata sweep consumed ask depth: got %d orders want 3", got)
	}
}

func TestMarketFeeInReceivedSpotAssetReservesShortfall(t *testing.T) {
	newExchange := func(t *testing.T, buyerBase int64) *DefaultExchange {
		t.Helper()
		ex := NewExchange(2, &RealClock{})
		ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
		ex.ConnectNewClient(1, map[string]int64{"USD": 1, "X": buyerBase}, &FixedFee{
			TakerFee: Fee{Asset: "X", Amount: 2},
		})
		ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})
		response := ex.PlaceOrder(2, &OrderRequest{
			RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
			Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
		})
		if !response.Success {
			t.Fatalf("seed ask rejected: %s", response.Error)
		}
		return ex
	}

	t.Run("unfunded shortfall is rejected", func(t *testing.T) {
		ex := newExchange(t, 0)
		defer ex.Shutdown()
		response := ex.PlaceOrder(1, &OrderRequest{
			RequestID: 2, Symbol: "X/USD", Side: Buy, Type: Market,
			Price: 0, Qty: 1, TimeInForce: GTC, Visibility: Normal,
		})
		if response.Success || response.Error != RejectInsufficientBalance {
			t.Fatalf("received-asset fee response = %+v, want insufficient-balance rejection", response)
		}
		if got := ex.Clients[1].Balances["X"]; got != 0 {
			t.Fatalf("rejected order changed base balance: got %d want 0", got)
		}
	})

	t.Run("exact shortfall funding settles to zero", func(t *testing.T) {
		ex := newExchange(t, 1)
		defer ex.Shutdown()
		response := ex.PlaceOrder(1, &OrderRequest{
			RequestID: 2, Symbol: "X/USD", Side: Buy, Type: Market,
			Price: 0, Qty: 1, TimeInForce: GTC, Visibility: Normal,
		})
		if !response.Success {
			t.Fatalf("exactly funded received-asset fee rejected: %s", response.Error)
		}
		if got := ex.Clients[1].Balances["X"]; got != 0 {
			t.Fatalf("base balance after fill = %d, want 0", got)
		}
		if got := ex.Clients[1].Reserved["X"]; got != 0 {
			t.Fatalf("base fee reservation after fill = %d, want 0", got)
		}
	})
}

func TestRestingSpotOrderReservesMaximumMakerFeeAndRestoresRemainder(t *testing.T) {
	fees := &FixedFee{MakerFee: Fee{Asset: "USD", Amount: 2}}
	ex := NewExchange(3, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": 204}, fees)
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})
	ex.ConnectNewClient(3, map[string]int64{"X": 1}, &FixedFee{})

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 100, Qty: 2, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("resting bid rejected: %s", response.Error)
	}
	orderID := response.Data.(uint64)
	if got := ex.Clients[1].Reserved["USD"]; got != 202 {
		t.Fatalf("initial reservation = %d, want principal 200 + maker fee 2", got)
	}

	for _, clientID := range []uint64{2, 3} {
		response = ex.PlaceOrder(clientID, &OrderRequest{
			RequestID: 10 + clientID, Symbol: "X/USD", Side: Sell, Type: Market,
			Qty: 1, TimeInForce: GTC, Visibility: Normal,
		})
		if !response.Success {
			t.Fatalf("seller %d market order rejected: %s", clientID, response.Error)
		}
		if clientID == 2 {
			order := ex.Books["X/USD"].FindOrder(orderID)
			if order == nil || order.Reserved != 102 {
				t.Fatalf("partial remainder reservation = %#v, want 102", order)
			}
			if available := ex.Clients[1].GetAvailable("USD"); available != 0 {
				t.Fatalf("partial remainder available = %d, want 0", available)
			}
		}
	}

	if balance := ex.Clients[1].Balances["USD"]; balance != 0 {
		t.Fatalf("buyer balance after two maker-fee fills = %d, want 0", balance)
	}
	if reserved := ex.Clients[1].Reserved["USD"]; reserved != 0 {
		t.Fatalf("buyer reservation after full fill = %d, want 0", reserved)
	}
}

func TestRestingSpotSellReservesMakerBaseFee(t *testing.T) {
	fees := &FixedFee{MakerFee: Fee{Asset: "X", Amount: 1}}
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"X": 2}, fees)
	ex.ConnectNewClient(2, map[string]int64{"USD": 100}, &FixedFee{})

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 100, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("resting ask rejected: %s", response.Error)
	}
	if got := ex.Clients[1].Reserved["X"]; got != 2 {
		t.Fatalf("seller reservation = %d, want base 1 + maker fee 1", got)
	}

	response = ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("buyer market order rejected: %s", response.Error)
	}
	if balance := ex.Clients[1].Balances["X"]; balance != 0 {
		t.Fatalf("seller base balance after maker fee = %d, want 0", balance)
	}
}

func TestPerpMarketAdmissionUsesExecutableDepthAndFees(t *testing.T) {
	const (
		shallowRequirement = int64(11_000 * USD_PRECISION) // 10% margin + 1% fee at 50k x 2.
		deepRequirement    = int64(16_500 * USD_PRECISION) // actual 50k + 100k sweep.
	)
	newExchange := func(t *testing.T, takerBalance int64) *DefaultExchange {
		t.Helper()
		ex := NewExchange(3, &RealClock{})
		ex.AddInstrument(NewPerpFutures("X-PERP", "X", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
		fees := &PercentageFee{MakerBps: 100, TakerBps: 100, InQuote: true}
		for _, clientID := range []uint64{1, 2, 3} {
			ex.ConnectNewClient(clientID, nil, fees)
			ex.AddPerpBalance(clientID, "USD", 100_000*USD_PRECISION)
		}
		ex.Clients[1].PerpBalances["USD"] = takerBalance
		for clientID, price := range map[uint64]int64{2: 50_000 * USD_PRECISION, 3: 100_000 * USD_PRECISION} {
			response := ex.PlaceOrder(clientID, &OrderRequest{
				RequestID: clientID, Symbol: "X-PERP", Side: Sell, Type: LimitOrder,
				Price: price, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
			})
			if !response.Success {
				t.Fatalf("seed perp ask %d rejected: %s", clientID, response.Error)
			}
		}
		return ex
	}

	t.Run("shallow reference funding rejects deep sweep", func(t *testing.T) {
		ex := newExchange(t, shallowRequirement)
		defer ex.Shutdown()
		response := ex.PlaceOrder(1, &OrderRequest{RequestID: 10, Symbol: "X-PERP", Side: Buy, Type: Market, Qty: 2 * BTC_PRECISION, TimeInForce: GTC, Visibility: Normal})
		if response.Success || response.Error != RejectInsufficientBalance {
			t.Fatalf("deep sweep response = %+v, want insufficient-balance rejection", response)
		}
		if available := ex.Clients[1].PerpAvailable("USD"); available != shallowRequirement {
			t.Fatalf("rejected sweep changed available margin: got %d want %d", available, shallowRequirement)
		}
	})

	t.Run("exact executable requirement fills without negative availability", func(t *testing.T) {
		ex := newExchange(t, deepRequirement)
		defer ex.Shutdown()
		response := ex.PlaceOrder(1, &OrderRequest{RequestID: 10, Symbol: "X-PERP", Side: Buy, Type: Market, Qty: 2 * BTC_PRECISION, TimeInForce: GTC, Visibility: Normal})
		if !response.Success {
			t.Fatalf("exactly funded deep sweep rejected: %s", response.Error)
		}
		if available := ex.Clients[1].PerpAvailable("USD"); available != 0 {
			t.Fatalf("available margin after exactly funded sweep = %d, want 0", available)
		}
	})
}

func TestOptionMarketAdmissionUsesExecutableDepthAndFees(t *testing.T) {
	const (
		shallowRequirement = int64(6_060 * USD_PRECISION) // 3k premium x 2 + 1% fee.
		deepRequirement    = int64(9_090 * USD_PRECISION) // actual 3k + 6k sweep + fees.
	)
	newExchange := func(t *testing.T, takerBalance int64) *DefaultExchange {
		t.Helper()
		ex := NewExchange(3, &RealClock{})
		ex.AddInstrument(NewEuropeanOption("X-1-C", "X", "USD", "X/USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, 50_000*USD_PRECISION, math.MaxInt64, true))
		fees := &PercentageFee{MakerBps: 100, TakerBps: 100, InQuote: true}
		for _, clientID := range []uint64{1, 2, 3} {
			ex.ConnectNewClient(clientID, nil, fees)
			ex.AddPerpBalance(clientID, "USD", 100_000*USD_PRECISION)
		}
		ex.Clients[1].PerpBalances["USD"] = takerBalance
		for clientID, price := range map[uint64]int64{2: 3_000 * USD_PRECISION, 3: 6_000 * USD_PRECISION} {
			response := ex.PlaceOrder(clientID, &OrderRequest{
				RequestID: clientID, Symbol: "X-1-C", Side: Sell, Type: LimitOrder,
				Price: price, Qty: BTC_PRECISION, TimeInForce: GTC, Visibility: Normal,
			})
			if !response.Success {
				t.Fatalf("seed option ask %d rejected: %s", clientID, response.Error)
			}
		}
		return ex
	}

	t.Run("shallow reference funding rejects deep sweep", func(t *testing.T) {
		ex := newExchange(t, shallowRequirement)
		defer ex.Shutdown()
		response := ex.PlaceOrder(1, &OrderRequest{RequestID: 10, Symbol: "X-1-C", Side: Buy, Type: Market, Qty: 2 * BTC_PRECISION, TimeInForce: GTC, Visibility: Normal})
		if response.Success || response.Error != RejectInsufficientBalance {
			t.Fatalf("deep option sweep response = %+v, want insufficient-balance rejection", response)
		}
	})

	t.Run("exact executable requirement fills without negative availability", func(t *testing.T) {
		ex := newExchange(t, deepRequirement)
		defer ex.Shutdown()
		response := ex.PlaceOrder(1, &OrderRequest{RequestID: 10, Symbol: "X-1-C", Side: Buy, Type: Market, Qty: 2 * BTC_PRECISION, TimeInForce: GTC, Visibility: Normal})
		if !response.Success {
			t.Fatalf("exactly funded deep option sweep rejected: %s", response.Error)
		}
		if available := ex.Clients[1].PerpAvailable("USD"); available != 0 {
			t.Fatalf("available option wallet after exactly funded sweep = %d, want 0", available)
		}
	})
}

func TestSpotPlanRejectsCrossingLimitWithPerFillReceivedFee(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": 2, "X": 1}, &FixedFee{TakerFee: Fee{Asset: "X", Amount: 2}})
	for _, clientID := range []uint64{2, 3} {
		ex.ConnectNewClient(clientID, map[string]int64{"X": 1}, &FixedFee{})
		response := ex.PlaceOrder(clientID, &OrderRequest{
			RequestID: clientID, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
			Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
		})
		if !response.Success {
			t.Fatalf("seed ask %d rejected: %s", clientID, response.Error)
		}
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 10, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 1, Qty: 2, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectInsufficientBalance {
		t.Fatalf("crossing limit response = %+v, want insufficient-balance rejection", response)
	}
	if got := ex.Clients[1].Balances["X"]; got != 1 {
		t.Fatalf("rejected crossing limit changed received asset: got %d want 1", got)
	}
	if got := len(ex.Books["X/USD"].Asks.Orders); got != 2 {
		t.Fatalf("rejected crossing limit consumed asks: got %d want 2", got)
	}
}

func TestSpotPlanCancelsUnfundedReceivedFeeMakerBeforeMatch(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	fees := &FixedFee{MakerFee: Fee{Asset: "X", Amount: 2}}
	ex.ConnectNewClient(1, map[string]int64{"USD": 2}, fees)
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})

	quote := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 1, Qty: 2, TimeInForce: GTC, Visibility: Normal,
	})
	if !quote.Success {
		t.Fatalf("unfunded quote should rest until it faces a fill: %s", quote.Error)
	}
	response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Sell, Type: Market,
		Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("market sell after stale-maker cancellation rejected: %s", response.Error)
	}
	if order := ex.Books["X/USD"].FindOrder(quote.Data.(uint64)); order != nil {
		t.Fatalf("unfunded maker remained live: %#v", order)
	}
	if got := ex.Clients[1].Balances["X"]; got != 0 {
		t.Fatalf("unfunded maker created base debt: got %d want 0", got)
	}
	if got := ex.Clients[2].Balances["X"]; got != 1 {
		t.Fatalf("cancelled maker consumed taker inventory: got %d want 1", got)
	}
}

func TestSpotPlanBlocksMultiTrancheIcebergFeeOverdraft(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"X": 3, "BNB": 2}, &FixedFee{MakerFee: Fee{Asset: "BNB", Amount: 1}})
	ex.ConnectNewClient(2, map[string]int64{"USD": 3}, &FixedFee{})

	quote := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 1, Qty: 3, TimeInForce: GTC, Visibility: Iceberg, IcebergQty: 1,
	})
	if !quote.Success {
		t.Fatalf("iceberg quote rejected: %s", quote.Error)
	}
	response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 3, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("market order after stale-maker cancellation rejected: %s", response.Error)
	}
	if order := ex.Books["X/USD"].FindOrder(quote.Data.(uint64)); order != nil {
		t.Fatalf("unfunded iceberg remained live: %#v", order)
	}
	if got := ex.Clients[1].Balances["BNB"]; got != 2 {
		t.Fatalf("iceberg fee balance changed before match: got %d want 2", got)
	}
	if got := ex.Clients[1].Balances["X"]; got != 3 {
		t.Fatalf("iceberg inventory changed before match: got %d want 3", got)
	}
	if got := ex.Clients[2].Balances["X"]; got != 0 {
		t.Fatalf("buyer received inventory from unfunded iceberg: got %d want 0", got)
	}
}

func TestSpotPlanAllowsExactlyFundedMultiTrancheIceberg(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"X": 3, "BNB": 3}, &FixedFee{MakerFee: Fee{Asset: "BNB", Amount: 1}})
	ex.ConnectNewClient(2, map[string]int64{"USD": 3}, &FixedFee{})

	quote := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 1, Qty: 3, TimeInForce: GTC, Visibility: Iceberg, IcebergQty: 1,
	})
	if !quote.Success {
		t.Fatalf("iceberg quote rejected: %s", quote.Error)
	}
	response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 3, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("exactly funded iceberg sweep rejected: %s", response.Error)
	}
	if got := ex.Clients[1].Balances["BNB"]; got != 0 {
		t.Fatalf("maker BNB after three fees = %d, want 0", got)
	}
	if got := ex.Clients[2].Balances["X"]; got != 3 {
		t.Fatalf("buyer received X = %d, want 3", got)
	}
}

func TestSpotPlanCancelsUnfundedMakerAndReplansDeeperLiquidity(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"X": 1}, &FixedFee{MakerFee: Fee{Asset: "USD", Amount: 2}})
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})
	ex.ConnectNewClient(3, map[string]int64{"USD": 2}, &FixedFee{})

	bad := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if !bad.Success {
		t.Fatalf("unfunded maker should rest until it faces a fill: %s", bad.Error)
	}
	good := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 2, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if !good.Success {
		t.Fatalf("deeper ask rejected: %s", good.Error)
	}

	response := ex.PlaceOrder(3, &OrderRequest{
		RequestID: 3, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("market buyer rejected: %s", response.Error)
	}
	if order := ex.Books["X/USD"].FindOrder(bad.Data.(uint64)); order != nil {
		t.Fatalf("unfunded best maker remained live: %#v", order)
	}
	if got := ex.Clients[3].Balances["USD"]; got != 0 {
		t.Fatalf("buyer did not reach deeper liquidity: USD=%d want 0", got)
	}
	if got := ex.Clients[3].Balances["X"]; got != 1 {
		t.Fatalf("buyer did not receive deeper liquidity: X=%d want 1", got)
	}
}

func TestSpotPlanProtectsResidualMakerReservationAfterPartialFill(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"USD": 203}, &FixedFee{MakerFee: Fee{Asset: "USD", Amount: 2}})
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})

	quote := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 100, Qty: 2, TimeInForce: GTC, Visibility: Normal,
	})
	if !quote.Success {
		t.Fatalf("quote rejected: %s", quote.Error)
	}
	if got := ex.Clients[1].Reserved["USD"]; got != 202 {
		t.Fatalf("initial reservation = %d, want 202", got)
	}

	response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Sell, Type: Market,
		Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("market sell after stale-maker cancellation rejected: %s", response.Error)
	}
	if order := ex.Books["X/USD"].FindOrder(quote.Data.(uint64)); order != nil {
		t.Fatalf("maker with unfundable remainder remained live: %#v", order)
	}
	if got := ex.Clients[1].Balances["USD"]; got != 203 {
		t.Fatalf("cancelled maker changed cash: got %d want 203", got)
	}
	if got := ex.Clients[1].Reserved["USD"]; got != 0 {
		t.Fatalf("cancelled maker leaked reservation: got %d", got)
	}
}

func TestSpotPlanRejectsCreditedAssetOverflowBehindReservation(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 2, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"X": math.MaxInt64, "USD": 1}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})

	locked := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 2, Qty: math.MaxInt64, TimeInForce: GTC, Visibility: Normal,
	})
	if !locked.Success {
		t.Fatalf("max inventory quote rejected: %s", locked.Error)
	}
	if got := ex.Clients[1].Reserved["X"]; got != math.MaxInt64 {
		t.Fatalf("max inventory reservation = %d, want %d", got, int64(math.MaxInt64))
	}
	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("counterparty ask rejected: %s", response.Error)
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 3, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectInsufficientBalance {
		t.Fatalf("overflowing received-base order = %+v, want insufficient-balance rejection", response)
	}
	if got := ex.Clients[1].Balances["X"]; got != math.MaxInt64 {
		t.Fatalf("rejected overflow changed X balance: got %d want %d", got, int64(math.MaxInt64))
	}
	if got := ex.Clients[2].Balances["X"]; got != 1 {
		t.Fatalf("rejected overflow consumed counterparty inventory: got %d want 1", got)
	}
}

func TestSpotPlanRejectsFOKWithoutCancellingUnfundedMaker(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"X": 3}, &FixedFee{MakerFee: Fee{Asset: "X", Amount: 1}})
	ex.ConnectNewClient(2, map[string]int64{"USD": 2}, &FixedFee{})

	quote := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 1, Qty: 2, TimeInForce: GTC, Visibility: Iceberg, IcebergQty: 1,
	})
	if !quote.Success {
		t.Fatalf("iceberg quote rejected: %s", quote.Error)
	}
	response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 1, Qty: 2, TimeInForce: FOK, Visibility: Normal,
	})
	if response.Success || response.Error != RejectFOKNotFilled {
		t.Fatalf("FOK after fee-plan pruning = %+v, want FOK rejection", response)
	}
	if order := ex.Books["X/USD"].FindOrder(quote.Data.(uint64)); order == nil {
		t.Fatal("rejected FOK cancelled an unrelated resting iceberg")
	}
	if got := ex.Clients[2].Balances["USD"]; got != 2 {
		t.Fatalf("FOK rejection changed buyer balance: got %d want 2", got)
	}
}

func TestSpotPlanRejectDoesNotCancelMakerDuringVirtualReplan(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"X": 1}, &FixedFee{MakerFee: Fee{Asset: "USD", Amount: 2}})
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})
	ex.ConnectNewClient(3, map[string]int64{"USD": 1}, &FixedFee{})

	bad := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	good := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 2, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if !bad.Success || !good.Success {
		t.Fatalf("seed asks accepted = %v/%v", bad.Success, good.Success)
	}

	response := ex.PlaceOrder(3, &OrderRequest{
		RequestID: 3, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectInsufficientBalance {
		t.Fatalf("unfunded replan response = %+v, want insufficient balance", response)
	}
	if order := ex.Books["X/USD"].FindOrder(bad.Data.(uint64)); order == nil {
		t.Fatal("rejected taker cancelled the unfunded maker")
	}
	if order := ex.Books["X/USD"].FindOrder(good.Data.(uint64)); order == nil {
		t.Fatal("rejected taker changed deeper liquidity")
	}
	if got := ex.Clients[3].Balances["USD"]; got != 1 {
		t.Fatalf("rejected taker changed balance: got %d want 1", got)
	}
}

func TestSpotPlanDefersMarketFeeEscrowReleaseUntilTerminal(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	ex.ConnectNewClient(1, map[string]int64{"BNB": 2, "USD": 2}, &FixedFee{TakerFee: Fee{Asset: "BNB", Amount: 1}})
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})
	ex.ConnectNewClient(3, map[string]int64{"X": 1}, &FixedFee{})
	for clientID := uint64(2); clientID <= 3; clientID++ {
		response := ex.PlaceOrder(clientID, &OrderRequest{
			RequestID: clientID, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
			Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
		})
		if !response.Success {
			t.Fatalf("seed ask %d rejected: %s", clientID, response.Error)
		}
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 4, Symbol: "X/USD", Side: Buy, Type: Market,
		Qty: 2, TimeInForce: GTC, Visibility: Normal,
	})
	if !response.Success {
		t.Fatalf("market order with exact fee escrow rejected: %s", response.Error)
	}
	if got := ex.Clients[1].Balances["BNB"]; got != 0 {
		t.Fatalf("buyer BNB after two fees = %d, want 0", got)
	}
	if got := ex.Clients[1].Reserved["BNB"]; got != 0 {
		t.Fatalf("terminal market order retained BNB escrow: %d", got)
	}
	if got := ex.Clients[1].Balances["X"]; got != 2 {
		t.Fatalf("buyer X after two fills = %d, want 2", got)
	}
}

func TestSpotPlanRejectRollsBackAutoBorrow(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		CollateralFactors: map[string]float64{"USD": 1},
		AssetPrecisions:   map[string]int64{"USD": 1},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": 1}),
	}); err != nil {
		t.Fatalf("EnableBorrowing: %v", err)
	}
	ex.ConnectNewClient(1, nil, &FixedFee{TakerFee: Fee{Asset: "X", Amount: 2}})
	ex.AddPerpBalance(1, "USD", 10)
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})
	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("seed ask rejected: %s", response.Error)
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectInsufficientBalance {
		t.Fatalf("received-fee order = %+v, want insufficient-balance rejection", response)
	}
	client := ex.Clients[1]
	if got := client.Balances["USD"]; got != 0 {
		t.Fatalf("rejected order retained borrowed spot cash: got %d", got)
	}
	if got := client.Borrowed["USD"]; got != 0 {
		t.Fatalf("rejected order retained debt: got %d", got)
	}
	if got := client.BorrowedSpot["USD"]; got != 0 {
		t.Fatalf("rejected order retained spot debt attribution: got %d", got)
	}
	if got := client.Reserved["USD"]; got != 0 {
		t.Fatalf("rejected order retained spot reservation: got %d", got)
	}
}

func TestSpotPlanRejectLogsCompensatingRepayForAutoBorrow(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
	log := &autoBorrowRollbackLogger{}
	ex.SetLogger("_global", log)
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:           true,
		AutoBorrowSpot:    true,
		CollateralFactors: map[string]float64{"USD": 1},
		AssetPrecisions:   map[string]int64{"USD": 1},
		PriceSource:       NewStaticPriceOracle(map[string]int64{"USD": 1}),
	}); err != nil {
		t.Fatalf("EnableBorrowing: %v", err)
	}
	ex.ConnectNewClient(1, nil, &FixedFee{TakerFee: Fee{Asset: "X", Amount: 2}})
	ex.AddPerpBalance(1, "USD", 10)
	ex.ConnectNewClient(2, map[string]int64{"X": 1}, &FixedFee{})
	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: "X/USD", Side: Sell, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("seed ask rejected: %s", response.Error)
	}

	response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 2, Symbol: "X/USD", Side: Buy, Type: LimitOrder,
		Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal,
	})
	if response.Success || response.Error != RejectInsufficientBalance {
		t.Fatalf("received-fee order = %+v, want insufficient-balance rejection", response)
	}

	log.mu.Lock()
	defer log.mu.Unlock()
	if len(log.borrows) != 1 {
		t.Fatalf("auto-borrow events = %d, want 1", len(log.borrows))
	}
	if log.borrows[0].Asset != "USD" || log.borrows[0].Amount != 1 || log.borrows[0].Reason != "auto_spot" {
		t.Fatalf("auto-borrow event = %+v, want USD principal 1 auto_spot", log.borrows[0])
	}
	if len(log.repays) != 1 {
		t.Fatalf("compensating repay events = %d, want 1", len(log.repays))
	}
	repay := log.repays[0]
	if repay.Asset != "USD" || repay.Principal != log.borrows[0].Amount || repay.RemainingDebt != 0 || repay.Reason != "order_admission_rollback" {
		t.Fatalf("compensating repay = %+v, want USD principal 1 debt 0 admission rollback", repay)
	}
	if len(log.rollbackBalances) != 1 {
		t.Fatalf("rollback balance changes = %d, want 1", len(log.rollbackBalances))
	}
	changes := log.rollbackBalances[0].Changes
	if len(changes) != 2 || changes[0] != spotDelta("USD", 1, 0) || changes[1] != borrowedDelta("USD", 1, 0) {
		t.Fatalf("rollback changes = %+v, want spot and debt reversal", changes)
	}
}
