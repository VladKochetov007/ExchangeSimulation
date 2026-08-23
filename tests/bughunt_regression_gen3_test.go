package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// A negative transfer amount reverses the direction while the availability
// check still guards the declared source wallet — draining reserved margin
// from the undeclared one unchecked.
func TestRegressionTransferRejectsNonPositiveAmount(t *testing.T) {
	ex := NewExchange(1, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(100)}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(100))
	ex.Lock()
	ex.Clients[1].PerpReserved["USD"] = USDAmount(100)
	ex.Unlock()
	client := ex.Clients[1]

	if err := ex.Transfer(1, "spot", "perp", "USD", -USDAmount(50)); err == nil {
		t.Fatal("negative transfer accepted: reserved perp margin siphoned to spot")
	}
	if err := ex.Transfer(1, "spot", "perp", "USD", 0); err == nil {
		t.Fatal("zero transfer accepted")
	}
	if client.Balances["USD"] != USDAmount(100) || client.PerpBalances["USD"] != USDAmount(100) {
		t.Fatalf("rejected transfer moved balances: spot=%d perp=%d",
			client.Balances["USD"], client.PerpBalances["USD"])
	}
}

// After a partial fill the resting remainder must still reserve margin PLUS
// its worst-case fee: the instrument-level release is margin-only, so without
// the exchange-side top-up the first fill frees the entire fee headroom and
// the remainder's future fee settles unbacked.
func TestRegressionPartialFillKeepsFeeHeadroomReserved(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	fees := &PercentageFee{MakerBps: 10, TakerBps: 10, InQuote: true}
	ex.ConnectNewClient(1, map[string]int64{}, fees)
	ex.ConnectNewClient(2, map[string]int64{}, fees)
	ex.AddPerpBalance(1, "USD", USDAmount(1_000_000))
	ex.AddPerpBalance(2, "USD", USDAmount(1_000_000))

	price := PriceUSD(50_000, DOLLAR_TICK)
	makerID, reject := InjectLimitOrder(ex, 1, "BTC-PERP", Buy, price, BTCAmount(10))
	if reject != "" {
		t.Fatalf("maker rejected: %s", reject)
	}
	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Sell, price, BTCAmount(1)); reject != "" {
		t.Fatalf("taker rejected: %s", reject)
	}

	ex.RLock()
	order := ex.Books["BTC-PERP"].FindOrder(makerID)
	ex.RUnlock()
	if order == nil {
		t.Fatal("maker order not resting after partial fill")
	}

	remaining := BTCAmount(9)
	wantMargin := perp.MarginRequired(remaining, price, BTC_PRECISION)
	wantFee := MulDiv(remaining, price, BTC_PRECISION) * 10 / 10000
	if want := wantMargin + wantFee; order.Reserved != want {
		t.Fatalf("remainder Reserved = %d, want margin(9)+fee(9) = %d+%d = %d",
			order.Reserved, wantMargin, wantFee, want)
	}
}

// A spot-credited loan's cash never entered the perp wallet, so it must not
// reduce perp equity: charging it there liquidates a solvent account. Debt
// with no cash behind it (loan spent) must still move the estimate.
func TestRegressionSpotLoanDoesNotSkewPerpLiquidation(t *testing.T) {
	ex, perp := setupPerpExchange(USDAmount(10_000), USDAmount(500_000))

	entryPrice := PriceUSD(50_000, DOLLAR_TICK)
	qty := BTCAmount(1.0)
	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Sell, entryPrice, qty); reject != "" {
		t.Fatalf("maker rejected: %s", reject)
	}
	if _, reject := InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty); reject != "" {
		t.Fatalf("taker rejected: %s", reject)
	}
	pos := ex.Positions.GetPosition(1, "BTC-PERP")
	if pos == nil || pos.Size == 0 {
		t.Fatal("position not opened")
	}
	base, err := ex.EstimateLiquidationPrice(pos, 1, perp, BTC_PRECISION)
	if err != nil {
		t.Fatalf("EstimateLiquidationPrice: %v", err)
	}

	ex.Lock()
	client := ex.Clients[1]
	client.Balances["USD"] += USDAmount(5_000)
	client.Borrowed["USD"] += USDAmount(5_000)
	client.BorrowedSpot["USD"] += USDAmount(5_000)
	ex.Unlock()

	if got, err := ex.EstimateLiquidationPrice(pos, 1, perp, BTC_PRECISION); err != nil || got != base {
		t.Fatalf("spot-credited loan moved perp liquidation price %d -> %d", base, got)
	}

	ex.Lock()
	client.Borrowed["USD"] += USDAmount(2_000)
	ex.Unlock()

	if got, err := ex.EstimateLiquidationPrice(pos, 1, perp, BTC_PRECISION); err != nil || got <= base {
		t.Fatalf("perp-attributed debt without cash did not raise liquidation price: %d -> %d", base, got)
	}
}

// The account-level liability must reduce reported net worth exactly once:
// each wallet row nets only the debt attributed to it.
func TestRegressionSnapshotNetsLiabilityOncePerWallet(t *testing.T) {
	ex := NewExchange(1, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{"USD": USDAmount(100)}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(200))
	ex.Lock()
	client := ex.Clients[1]
	client.Borrowed["USD"] = USDAmount(80)
	client.BorrowedSpot["USD"] = USDAmount(30)
	ex.Unlock()

	snap := client.GetBalanceSnapshot(0)
	for _, row := range snap.SpotBalances {
		if row.Asset != "USD" {
			continue
		}
		if row.Borrowed != USDAmount(30) || row.NetAsset != USDAmount(70) {
			t.Fatalf("spot row borrowed=%d net=%d, want 30/70 in USD units", row.Borrowed, row.NetAsset)
		}
	}
	for _, row := range snap.PerpBalances {
		if row.Asset != "USD" {
			continue
		}
		if row.Borrowed != USDAmount(50) || row.NetAsset != USDAmount(150) {
			t.Fatalf("perp row borrowed=%d net=%d, want 50/150 in USD units", row.Borrowed, row.NetAsset)
		}
	}
}

// Interest on a spot-credited loan must be billed to the spot wallet: charging
// the perp wallet drives a spot-only borrower's empty perp balance negative on
// every sweep.
func TestRegressionInterestChargedToLoanWallet(t *testing.T) {
	ex := NewExchange(1, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	loan := USDAmount(100_000)
	ex.Lock()
	ex.CollateralRate = 500
	client := ex.Clients[1]
	client.Balances["USD"] = loan
	client.Borrowed["USD"] = loan
	client.BorrowedSpot["USD"] = loan
	ex.Unlock()

	ex.ChargeCollateralInterest()

	if client.PerpBalances["USD"] != 0 {
		t.Fatalf("spot-credited loan's interest hit the perp wallet: %d", client.PerpBalances["USD"])
	}
	if client.Balances["USD"] >= loan {
		t.Fatalf("no interest debited from the spot wallet: %d", client.Balances["USD"])
	}
}

// A base-denominated fee on a margined instrument has no base leg to net
// against — it must be pre-checked against the perp wallet like any other
// unbacked fee asset.
func TestRegressionBaseFeeOnPerpRequiresPerpBaseBalance(t *testing.T) {
	newPerpExchange := func(t *testing.T) *Exchange {
		t.Helper()
		ex := NewExchange(10, &RealClock{})
		perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
		ex.AddInstrument(perp)
		baseFee := &PercentageFee{MakerBps: 10, TakerBps: 10, InQuote: false}
		ex.ConnectNewClient(1, map[string]int64{}, baseFee)
		ex.AddPerpBalance(1, "USD", USDAmount(1_000_000))
		return ex
	}
	price := PriceUSD(50_000, DOLLAR_TICK)

	ex := newPerpExchange(t)
	if _, reject := InjectLimitOrder(ex, 1, "BTC-PERP", Buy, price, BTCAmount(1)); reject == "" {
		t.Fatal("order with unbacked base-asset fee accepted: fill would drive perp BTC negative")
	}

	ex = newPerpExchange(t)
	ex.AddPerpBalance(1, "BTC", BTCAmount(1))
	if _, reject := InjectLimitOrder(ex, 1, "BTC-PERP", Buy, price, BTCAmount(1)); reject != "" {
		t.Fatalf("funded base-fee order rejected: %s", reject)
	}
}
