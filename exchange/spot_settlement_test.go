package exchange

import (
	"testing"
)

// settlementLogger keeps the balance_change events a settlement emitted, which
// is how the ledger reports itself to anything downstream.
type settlementLogger struct{ balanceChanges []BalanceChangeEvent }

func (l *settlementLogger) LogEvent(_ int64, _ uint64, eventName string, event any) {
	if eventName != "balance_change" {
		return
	}
	if change, ok := event.(BalanceChangeEvent); ok {
		l.balanceChanges = append(l.balanceChanges, change)
	}
}

func (l *settlementLogger) changesFor(clientID uint64, reason string) []BalanceDelta {
	for _, change := range l.balanceChanges {
		if change.ClientID == clientID && change.Reason == reason {
			return change.Changes
		}
	}
	return nil
}

const (
	spotTestPrice = 50_000
	spotTestQty   = BTC_PRECISION
)

// spotTradeResult is what one settled spot trade did to both parties.
type spotTradeResult struct {
	buyerBase, buyerQuote   int64
	sellerBase, sellerQuote int64
	buyerFeeAsset           int64
	sellerFeeAsset          int64
	buyerReserved           int64
	sellerReserved          int64
	log                     *settlementLogger
}

// runSpotTrade crosses one BTC at 50k and returns each party's balance moves.
// takerSide picks which party is the aggressor, which is the only thing that
// distinguishes the two settlement directions from each other.
func runSpotTrade(t *testing.T, takerSide Side, buyerPlan, sellerPlan FeeModel, feeAsset string) spotTradeResult {
	t.Helper()
	const buyerID, sellerID = uint64(1), uint64(2)

	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	log := &settlementLogger{}
	ex.SetLogger("BTC/USD", log)
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))

	// Both sides are funded in every asset so a fee in base, quote or a third
	// asset is payable and the settlement path is what is under test.
	buyerFunds := map[string]int64{"USD": USDAmount(1_000_000), "BTC": BTCAmount(10)}
	sellerFunds := map[string]int64{"USD": USDAmount(1_000_000), "BTC": BTCAmount(10)}
	if feeAsset != "" && feeAsset != "USD" && feeAsset != "BTC" {
		buyerFunds[feeAsset] = 1_000_000
		sellerFunds[feeAsset] = 1_000_000
	}
	ex.ConnectNewClient(buyerID, buyerFunds, buyerPlan)
	ex.ConnectNewClient(sellerID, sellerFunds, sellerPlan)

	before := func(clientID uint64, asset string) int64 { return ex.Clients[clientID].Balances[asset] }
	buyerBase0, buyerQuote0 := before(buyerID, "BTC"), before(buyerID, "USD")
	sellerBase0, sellerQuote0 := before(sellerID, "BTC"), before(sellerID, "USD")
	buyerFee0, sellerFee0 := int64(0), int64(0)
	if feeAsset != "" {
		buyerFee0, sellerFee0 = before(buyerID, feeAsset), before(sellerID, feeAsset)
	}

	price := PriceUSD(spotTestPrice, DOLLAR_TICK)
	restingClient, restingSide := sellerID, Sell
	takingClient := buyerID
	if takerSide == Sell {
		restingClient, restingSide = buyerID, Buy
		takingClient = sellerID
	}
	if response := ex.PlaceOrder(restingClient, &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: restingSide, Type: LimitOrder,
		Price: price, Qty: spotTestQty, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("resting order rejected: %s", response.Error)
	}
	if response := ex.PlaceOrder(takingClient, &OrderRequest{
		RequestID: 2, Symbol: "BTC/USD", Side: takerSide, Type: LimitOrder,
		Price: price, Qty: spotTestQty, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("taking order rejected: %s", response.Error)
	}

	result := spotTradeResult{
		buyerBase:   before(buyerID, "BTC") - buyerBase0,
		buyerQuote:  before(buyerID, "USD") - buyerQuote0,
		sellerBase:  before(sellerID, "BTC") - sellerBase0,
		sellerQuote: before(sellerID, "USD") - sellerQuote0,
		log:         log,
	}
	if feeAsset != "" {
		result.buyerFeeAsset = before(buyerID, feeAsset) - buyerFee0
		result.sellerFeeAsset = before(sellerID, feeAsset) - sellerFee0
	}
	for asset, amount := range ex.Clients[buyerID].Reserved {
		_ = asset
		result.buyerReserved += amount
	}
	for asset, amount := range ex.Clients[sellerID].Reserved {
		_ = asset
		result.sellerReserved += amount
	}
	return result
}

func zeroFees() FeeModel { return &FixedFee{} }

// A buy and a sell are the same settlement seen from opposite ends: whichever
// side aggresses, the buyer must gain exactly the base the seller loses and
// lose exactly the quote the seller gains. This is the invariant the merged
// buyer/seller path has to preserve, and the one a direction-specific bug
// would break on exactly one of the two runs.
func TestSpotSettlementIsSymmetricAcrossAggressor(t *testing.T) {
	notional := MulDiv(spotTestQty, PriceUSD(spotTestPrice, DOLLAR_TICK), BTC_PRECISION)

	for _, takerSide := range []Side{Buy, Sell} {
		result := runSpotTrade(t, takerSide, zeroFees(), zeroFees(), "")

		if result.buyerBase != spotTestQty {
			t.Errorf("taker=%s: buyer base delta = %d, want %d", takerSide, result.buyerBase, spotTestQty)
		}
		if result.sellerBase != -spotTestQty {
			t.Errorf("taker=%s: seller base delta = %d, want %d", takerSide, result.sellerBase, -spotTestQty)
		}
		if result.buyerQuote != -notional {
			t.Errorf("taker=%s: buyer quote delta = %d, want %d", takerSide, result.buyerQuote, -notional)
		}
		if result.sellerQuote != notional {
			t.Errorf("taker=%s: seller quote delta = %d, want %d", takerSide, result.sellerQuote, notional)
		}
		if result.buyerBase+result.sellerBase != 0 || result.buyerQuote+result.sellerQuote != 0 {
			t.Errorf("taker=%s: settlement not conservative: %+v", takerSide, result)
		}
		if result.buyerReserved != 0 || result.sellerReserved != 0 {
			t.Errorf("taker=%s: fully filled trade left reservations: buyer=%d seller=%d",
				takerSide, result.buyerReserved, result.sellerReserved)
		}
	}
}

// The same trade must settle identically whichever side happened to aggress,
// once the fee schedule makes maker and taker indistinguishable.
func TestSpotSettlementIndependentOfAggressorWithSymmetricFees(t *testing.T) {
	symmetric := func() FeeModel { return &PercentageFee{MakerBps: 10, TakerBps: 10, InQuote: true} }

	buyerTakes := runSpotTrade(t, Buy, symmetric(), symmetric(), "USD")
	sellerTakes := runSpotTrade(t, Sell, symmetric(), symmetric(), "USD")

	if buyerTakes.buyerBase != sellerTakes.buyerBase || buyerTakes.buyerQuote != sellerTakes.buyerQuote {
		t.Errorf("buyer settled differently by aggressor: %+v vs %+v", buyerTakes, sellerTakes)
	}
	if buyerTakes.sellerBase != sellerTakes.sellerBase || buyerTakes.sellerQuote != sellerTakes.sellerQuote {
		t.Errorf("seller settled differently by aggressor: %+v vs %+v", buyerTakes, sellerTakes)
	}
	if buyerTakes.buyerQuote >= -MulDiv(spotTestQty, PriceUSD(spotTestPrice, DOLLAR_TICK), BTC_PRECISION) {
		t.Errorf("buyer quote delta %d does not include the quote fee", buyerTakes.buyerQuote)
	}
}

// A fee charged in an asset that is neither base nor quote is its own debit and
// its own ledger line; base and quote must move by the trade alone.
func TestSpotSettlementFeeInThirdAsset(t *testing.T) {
	const feeAmount = int64(7)
	plan := func() FeeModel {
		fee := Fee{Asset: "BNB", Amount: feeAmount}
		return &FixedFee{MakerFee: fee, TakerFee: fee}
	}
	notional := MulDiv(spotTestQty, PriceUSD(spotTestPrice, DOLLAR_TICK), BTC_PRECISION)

	result := runSpotTrade(t, Buy, plan(), plan(), "BNB")

	if result.buyerFeeAsset != -feeAmount || result.sellerFeeAsset != -feeAmount {
		t.Errorf("third-asset fee not debited: buyer=%d seller=%d", result.buyerFeeAsset, result.sellerFeeAsset)
	}
	if result.buyerQuote != -notional || result.sellerQuote != notional {
		t.Errorf("third-asset fee leaked into quote: %+v", result)
	}
	if result.buyerBase != spotTestQty || result.sellerBase != -spotTestQty {
		t.Errorf("third-asset fee leaked into base: %+v", result)
	}
	// Both directions must report the fee asset as a third ledger line rather
	// than folding it silently into the base/quote pair.
	for _, clientID := range []uint64{1, 2} {
		deltas := result.log.changesFor(clientID, "trade_settlement")
		if len(deltas) != 3 {
			t.Fatalf("client %d logged %d deltas, want base, quote and fee asset: %+v", clientID, len(deltas), deltas)
		}
		if deltas[0].Asset != "BTC" || deltas[1].Asset != "USD" || deltas[2].Asset != "BNB" {
			t.Errorf("client %d delta assets = %s/%s/%s, want BTC/USD/BNB",
				clientID, deltas[0].Asset, deltas[1].Asset, deltas[2].Asset)
		}
	}
}

// A fee charged in base is already inside the base delta, so it must not be
// logged twice — and it must reduce what the buyer receives and increase what
// the seller gives up, by the same amount on both sides.
func TestSpotSettlementFeeInBaseAsset(t *testing.T) {
	const feeAmount = int64(1_000)
	plan := func() FeeModel {
		fee := Fee{Asset: "BTC", Amount: feeAmount}
		return &FixedFee{MakerFee: fee, TakerFee: fee}
	}

	result := runSpotTrade(t, Buy, plan(), plan(), "BTC")

	if result.buyerBase != spotTestQty-feeAmount {
		t.Errorf("buyer base delta = %d, want %d", result.buyerBase, spotTestQty-feeAmount)
	}
	if result.sellerBase != -spotTestQty-feeAmount {
		t.Errorf("seller base delta = %d, want %d", result.sellerBase, -spotTestQty-feeAmount)
	}
	for _, clientID := range []uint64{1, 2} {
		deltas := result.log.changesFor(clientID, "trade_settlement")
		if len(deltas) != 2 {
			t.Errorf("client %d logged %d deltas, want base and quote only: %+v", clientID, len(deltas), deltas)
		}
	}
}

// Both reports of a fill are built from the same fillSide, so they cannot
// disagree about the role, quantities or fee of either party.
func TestSettlementOutcomeDescribesBothParties(t *testing.T) {
	outcome := settlementOutcome{
		taker: fillSide{clientID: 1, orderID: 10, side: Buy, role: "taker", filledQty: 5, totalQty: 10},
		maker: fillSide{clientID: 2, orderID: 20, side: Sell, role: "maker", filledQty: 7, totalQty: 7},
	}
	if outcome.taker.isFull() {
		t.Error("partially filled taker reported as full")
	}
	if !outcome.maker.isFull() {
		t.Error("fully filled maker reported as partial")
	}
	if outcome.taker.side == outcome.maker.side {
		t.Error("both sides of a match reported the same side")
	}
}

func TestOppositeSideIsAnInvolution(t *testing.T) {
	for _, side := range []Side{Buy, Sell} {
		if got := oppositeSide(oppositeSide(side)); got != side {
			t.Errorf("oppositeSide(oppositeSide(%s)) = %s", side, got)
		}
		if oppositeSide(side) == side {
			t.Errorf("oppositeSide(%s) returned the same side", side)
		}
	}
}

// A disconnected client keeps its resting orders, and they keep filling. The
// fill must settle normally with nobody to notify rather than panicking on the
// dropped gateway.
func TestFillSettlesAfterMakerDisconnects(t *testing.T) {
	const makerID, takerID = uint64(1), uint64(2)

	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(makerID, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})
	ex.ConnectNewClient(takerID, map[string]int64{"USD": USDAmount(1_000_000)}, &FixedFee{})

	price := PriceUSD(spotTestPrice, DOLLAR_TICK)
	if response := ex.PlaceOrder(makerID, &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder,
		Price: price, Qty: spotTestQty, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("maker order rejected: %s", response.Error)
	}

	ex.DisconnectClient(makerID)
	if ex.Gateways[makerID] != nil {
		t.Fatal("disconnect left a gateway behind; test no longer covers the nil-gateway path")
	}

	if response := ex.PlaceOrder(takerID, &OrderRequest{
		RequestID: 2, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
		Price: price, Qty: spotTestQty, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("taker order rejected: %s", response.Error)
	}

	notional := MulDiv(spotTestQty, price, BTC_PRECISION)
	if got := ex.Clients[makerID].Balances["USD"]; got != notional {
		t.Errorf("disconnected maker USD = %d, want %d", got, notional)
	}
	if got := ex.Clients[takerID].Balances["BTC"]; got != spotTestQty {
		t.Errorf("taker BTC = %d, want %d", got, spotTestQty)
	}
}

// Settling against a party that is not a registered client would mint or
// destroy assets on one side of the trade. The spot planner catches this first,
// before the live book is touched; requireParties is the backstop for the paths
// that have no plan (margined instruments, custom matchers).
func TestSettlementRejectsUnregisteredParty(t *testing.T) {
	const makerID, takerID = uint64(1), uint64(2)

	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	ex.ConnectNewClient(makerID, map[string]int64{"BTC": BTCAmount(10)}, &FixedFee{})
	ex.ConnectNewClient(takerID, map[string]int64{"USD": USDAmount(1_000_000)}, &FixedFee{})

	price := PriceUSD(spotTestPrice, DOLLAR_TICK)
	if response := ex.PlaceOrder(makerID, &OrderRequest{
		RequestID: 1, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder,
		Price: price, Qty: spotTestQty, TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("maker order rejected: %s", response.Error)
	}

	takerUSD := ex.Clients[takerID].Balances["USD"]
	// Corrupt the registry the way only a bug could: the book still holds the
	// maker's order but the account behind it is gone.
	delete(ex.Clients, makerID)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("settling against an unregistered maker did not fail")
			}
		}()
		ex.PlaceOrder(takerID, &OrderRequest{
			RequestID: 2, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
			Price: price, Qty: spotTestQty, TimeInForce: GTC, Visibility: Normal,
		})
	}()

	if got := ex.Clients[takerID].Balances["USD"]; got != takerUSD {
		t.Errorf("taker USD moved against a missing counterparty: %d, want %d", got, takerUSD)
	}
	if got := ex.Clients[takerID].Balances["BTC"]; got != 0 {
		t.Errorf("taker received %d BTC from a missing counterparty", got)
	}
}

// requireParties names which side was missing, since a settlement that dies
// without saying so leaves nothing to debug.
func TestRequirePartiesNamesTheMissingSide(t *testing.T) {
	book := &OrderBook{Symbol: "BTC/USD"}
	exec := &Execution{TakerClientID: 1, TakerOrderID: 10, MakerClientID: 2, MakerOrderID: 20}
	present := &Client{}

	for _, testCase := range []struct {
		name         string
		taker, maker *Client
		want         string
	}{
		{"missing taker", nil, present, "taker 1"},
		{"missing maker", present, nil, "maker 2"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			defer func() {
				recovered := recover()
				message, ok := recovered.(string)
				if !ok {
					t.Fatalf("requireParties did not fail: %v", recovered)
				}
				if !contains(message, testCase.want) {
					t.Errorf("panic %q does not name %q", message, testCase.want)
				}
			}()
			executionContext{book: book, exec: exec, taker: testCase.taker, maker: testCase.maker}.requireParties()
		})
	}

	// Both present is the normal case and must not fail.
	executionContext{book: book, exec: exec, taker: present, maker: present}.requireParties()
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
