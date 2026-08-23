package exchange

import (
	"testing"
	"time"

	etypes "exchange_sim/types"
)

const (
	valuationBasePrecision  = int64(100_000_000)
	valuationQuotePrecision = int64(100_000)
)

func usdValuationSpec(abcPrice int64) etypes.AccountValuationSpec {
	return etypes.AccountValuationSpec{
		ReportAsset:     "USD",
		ReportPrecision: valuationQuotePrecision,
		AssetMarks: map[string]etypes.AssetValuationMark{
			"USD": {Price: valuationQuotePrecision, Precision: valuationQuotePrecision},
			"ABC": {Price: abcPrice, Precision: valuationBasePrecision},
		},
	}
}

func TestMarkedAccountIncludesLockedCashAndDebtOnce(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.ConnectNewClient(1, nil, &FixedFee{})
	client := ex.Clients[1]
	client.Balances["ABC"] = 2 * valuationBasePrecision
	client.Reserved["ABC"] = valuationBasePrecision
	client.Borrowed["ABC"] = valuationBasePrecision
	client.BorrowedSpot["ABC"] = valuationBasePrecision
	client.PerpBalances["USD"] = 10 * valuationQuotePrecision
	client.PerpReserved["USD"] = 7 * valuationQuotePrecision
	client.Borrowed["USD"] = 2 * valuationQuotePrecision

	report, err := ex.MarkedAccount(1, usdValuationSpec(50*valuationQuotePrecision))
	if err != nil {
		t.Fatalf("MarkedAccount: %v", err)
	}
	if report.SpotEquity != 50*valuationQuotePrecision {
		t.Fatalf("spot equity = %d, want %d", report.SpotEquity, 50*valuationQuotePrecision)
	}
	if report.PerpCashEquity != 8*valuationQuotePrecision {
		t.Fatalf("perp cash equity = %d, want %d", report.PerpCashEquity, 8*valuationQuotePrecision)
	}
	if report.DerivativeUnrealized != 0 || report.Equity != 58*valuationQuotePrecision {
		t.Fatalf("marked report = %#v", report)
	}
}

func TestMarkedAccountUsesOptionRiskMarkInsteadOfBookPrice(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	option := NewEuropeanOption(
		"ABC-TEST-C", "ABC", "USD", "ABC/USD", valuationBasePrecision, valuationQuotePrecision,
		valuationQuotePrecision, valuationBasePrecision/100, 100*valuationQuotePrecision,
		time.Now().Add(time.Hour).UnixNano(), true,
	)
	option.SetMarks(100*valuationQuotePrecision, 12*valuationQuotePrecision)
	ex.AddInstrument(option)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 100*valuationQuotePrecision)
	ex.Positions.UpdatePosition(1, option.Symbol(), valuationBasePrecision, 10*valuationQuotePrecision, Buy, PositionBoth)

	report, err := ex.MarkedAccount(1, usdValuationSpec(100*valuationQuotePrecision))
	if err != nil {
		t.Fatalf("MarkedAccount: %v", err)
	}
	if len(report.Positions) != 1 {
		t.Fatalf("positions = %#v", report.Positions)
	}
	position := report.Positions[0]
	if position.MarkPrice == nil || *position.MarkPrice != 12*valuationQuotePrecision || position.UnrealizedPnL != 2*valuationQuotePrecision {
		t.Fatalf("option position did not use PositionMark: %#v", position)
	}
	if report.DerivativeUnrealized != 0 || report.OptionMarketValue != 12*valuationQuotePrecision || report.Equity != 112*valuationQuotePrecision {
		t.Fatalf("marked report = %#v", report)
	}
}

func TestMarkedAccountAcceptsInitializedOutOfMoneyOptionAtZeroPremium(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	option := NewEuropeanOption(
		"ABC-TEST-P", "ABC", "USD", "ABC/USD", valuationBasePrecision, valuationQuotePrecision,
		valuationQuotePrecision, valuationBasePrecision/100, 100*valuationQuotePrecision,
		time.Now().Add(time.Hour).UnixNano(), false,
	)
	// The nonzero underlying proves this is an initialized mark pair. A zero
	// put premium is economically valid while the option is out of the money.
	option.SetMarks(120*valuationQuotePrecision, 0)
	ex.AddInstrument(option)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 100*valuationQuotePrecision)
	ex.Positions.UpdatePosition(1, option.Symbol(), valuationBasePrecision, 10*valuationQuotePrecision, Buy, PositionBoth)

	report, err := ex.MarkedAccount(1, usdValuationSpec(120*valuationQuotePrecision))
	if err != nil {
		t.Fatalf("MarkedAccount: %v", err)
	}
	if len(report.Positions) != 1 || report.Positions[0].MarkPrice == nil || *report.Positions[0].MarkPrice != 0 {
		t.Fatalf("zero-premium position mark = %#v", report.Positions)
	}
	if report.OptionMarketValue != 0 || report.Equity != 100*valuationQuotePrecision {
		t.Fatalf("out-of-money option report = %#v", report)
	}
}

func TestMarkedAccountOptionPremiumCashAndMarketValueConserveEquity(t *testing.T) {
	ex := NewExchange(3, &RealClock{})
	option := NewEuropeanOption(
		"ABC-TEST-C", "ABC", "USD", "ABC/USD", valuationBasePrecision, valuationQuotePrecision,
		valuationQuotePrecision, valuationBasePrecision/100, 100*valuationQuotePrecision,
		time.Now().Add(time.Hour).UnixNano(), true,
	)
	option.SetMarks(100*valuationQuotePrecision, 12*valuationQuotePrecision)
	ex.AddInstrument(option)
	for clientID := uint64(1); clientID <= 2; clientID++ {
		ex.ConnectNewClient(clientID, nil, &FixedFee{})
		ex.AddPerpBalance(clientID, "USD", 100*valuationQuotePrecision)
	}

	sell := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: option.Symbol(), Side: Sell, Type: LimitOrder,
		Price: 10 * valuationQuotePrecision, Qty: valuationBasePrecision,
		TimeInForce: GTC, Visibility: Normal,
	})
	if !sell.Success {
		t.Fatalf("resting option sell rejected: %s", sell.Error)
	}
	buy := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 2, Symbol: option.Symbol(), Side: Buy, Type: LimitOrder,
		Price: 10 * valuationQuotePrecision, Qty: valuationBasePrecision,
		TimeInForce: GTC, Visibility: Normal,
	})
	if !buy.Success {
		t.Fatalf("crossing option buy rejected: %s", buy.Error)
	}

	buyer, err := ex.MarkedAccount(1, usdValuationSpec(100*valuationQuotePrecision))
	if err != nil {
		t.Fatalf("buyer MarkedAccount: %v", err)
	}
	seller, err := ex.MarkedAccount(2, usdValuationSpec(100*valuationQuotePrecision))
	if err != nil {
		t.Fatalf("seller MarkedAccount: %v", err)
	}
	if buyer.PerpCashEquity != 90*valuationQuotePrecision || buyer.OptionMarketValue != 12*valuationQuotePrecision || buyer.Equity != 102*valuationQuotePrecision {
		t.Fatalf("buyer marked wealth = %#v", buyer)
	}
	if seller.PerpCashEquity != 110*valuationQuotePrecision || seller.OptionMarketValue != -12*valuationQuotePrecision || seller.Equity != 98*valuationQuotePrecision {
		t.Fatalf("seller marked wealth = %#v", seller)
	}
	if buyer.Equity+seller.Equity != 200*valuationQuotePrecision {
		t.Fatalf("marked option wealth not conserved: buyer=%d seller=%d", buyer.Equity, seller.Equity)
	}
}

func TestMarkedAccountIncludesIsolatedCollateral(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.ConnectNewClient(1, nil, &FixedFee{})
	client := ex.Clients[1]
	client.PerpBalances["USD"] = 2 * valuationQuotePrecision
	client.IsolatedPositions["ABC-PERP"] = &IsolatedPosition{
		Symbol: "ABC-PERP", Collateral: map[string]int64{"USD": 3 * valuationQuotePrecision},
		Borrowed: map[string]int64{},
	}

	report, err := ex.MarkedAccount(1, usdValuationSpec(100*valuationQuotePrecision))
	if err != nil {
		t.Fatalf("MarkedAccount: %v", err)
	}
	if report.PerpCashEquity != 2*valuationQuotePrecision || report.IsolatedEquity != 3*valuationQuotePrecision || report.Equity != 5*valuationQuotePrecision {
		t.Fatalf("isolated collateral omitted from marked wealth: %#v", report)
	}
}

func TestMarkedAccountRefusesMissingNonZeroAssetConversion(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.ConnectNewClient(1, map[string]int64{"ABC": valuationBasePrecision}, &FixedFee{})
	_, err := ex.MarkedAccount(1, etypes.AccountValuationSpec{
		ReportAsset: "USD", ReportPrecision: valuationQuotePrecision,
		AssetMarks: map[string]etypes.AssetValuationMark{
			"USD": {Price: valuationQuotePrecision, Precision: valuationQuotePrecision},
		},
	})
	if err == nil {
		t.Fatal("MarkedAccount accepted an unpriced non-zero ABC balance")
	}
}

func TestBalanceSnapshotRetainsDebtOnlyWalletAssets(t *testing.T) {
	client := NewClient(1, &FixedFee{})
	client.Borrowed["ABC"] = valuationBasePrecision
	client.BorrowedSpot["ABC"] = valuationBasePrecision
	client.Borrowed["USD"] = valuationQuotePrecision

	snapshot := client.GetBalanceSnapshot(1)
	if len(snapshot.SpotBalances) != 1 || snapshot.SpotBalances[0].Asset != "ABC" || snapshot.SpotBalances[0].NetAsset != -valuationBasePrecision {
		t.Fatalf("spot debt-only balance omitted: %#v", snapshot.SpotBalances)
	}
	if len(snapshot.PerpBalances) != 1 || snapshot.PerpBalances[0].Asset != "USD" || snapshot.PerpBalances[0].NetAsset != -valuationQuotePrecision {
		t.Fatalf("perp debt-only balance omitted: %#v", snapshot.PerpBalances)
	}
}

func TestZeroFeeSpotSettlementDoesNotCreateEmptyAssetLedger(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	spot := NewSpotInstrument(
		"ABC/USD", "ABC", "USD", valuationBasePrecision, valuationQuotePrecision,
		valuationQuotePrecision, valuationBasePrecision/100,
	)
	ex.AddInstrument(spot)
	ex.ConnectNewClient(1, map[string]int64{"USD": 100 * valuationQuotePrecision}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{"ABC": valuationBasePrecision}, &FixedFee{})

	if response := ex.PlaceOrder(2, &OrderRequest{
		RequestID: 1, Symbol: spot.Symbol(), Side: Sell, Type: LimitOrder,
		Price: 100 * valuationQuotePrecision, Qty: valuationBasePrecision,
		TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("resting sell rejected: %s", response.Error)
	}
	if response := ex.PlaceOrder(1, &OrderRequest{
		RequestID: 2, Symbol: spot.Symbol(), Side: Buy, Type: LimitOrder,
		Price: 100 * valuationQuotePrecision, Qty: valuationBasePrecision,
		TimeInForce: GTC, Visibility: Normal,
	}); !response.Success {
		t.Fatalf("crossing buy rejected: %s", response.Error)
	}
	for clientID, client := range ex.Clients {
		if _, exists := client.Balances[""]; exists {
			t.Fatalf("client %d has empty spot balance asset: %#v", clientID, client.Balances)
		}
		if _, exists := client.Reserved[""]; exists {
			t.Fatalf("client %d has empty spot reservation asset: %#v", clientID, client.Reserved)
		}
	}
	if _, exists := ex.ExchangeBalance.FeeRevenue[""]; exists {
		t.Fatalf("exchange has empty fee-revenue asset: %#v", ex.ExchangeBalance.FeeRevenue)
	}
}
