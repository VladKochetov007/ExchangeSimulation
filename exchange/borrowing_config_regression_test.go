package exchange

import "testing"

func TestBorrowingRateAuthorityIsCopiedAndCannotChangeWithOutstandingDebt(t *testing.T) {
	rates := map[string]int64{"USD": 1}
	bm := NewBorrowingManager(BorrowingConfig{Enabled: true, BorrowRates: rates})
	rates["USD"] = 999
	if got := bm.getRate("USD"); got != 1 {
		t.Fatalf("borrow manager rate changed with caller map: got %d want 1", got)
	}

	ex := NewExchange(1, &expiryManualClock{now: 1_000})
	defer ex.Shutdown()
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	config := BorrowingConfig{
		Enabled:     true,
		BorrowRates: map[string]int64{"USD": 1},
		PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}),
	}
	if err := ex.EnableBorrowing(config); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}
	ex.Clients[1].Borrowed["USD"] = 1
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:     true,
		BorrowRates: map[string]int64{"USD": 2},
		PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}),
	}); err == nil {
		t.Fatal("replaced borrowing rate while debt was outstanding")
	}
	if got := ex.BorrowingMgr.getRate("USD"); got != 1 {
		t.Fatalf("rejected replacement changed active rate: got %d want 1", got)
	}
}

func TestEnableBorrowingRejectsInvalidFinancingConfiguration(t *testing.T) {
	tests := []BorrowingConfig{
		{
			Enabled: true, BorrowRates: map[string]int64{"USD": -1},
			PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}),
		},
		{
			Enabled: true, CollateralFactors: map[string]float64{"USD": 1.01},
			PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}),
		},
		{
			Enabled: true, AssetPrecisions: map[string]int64{"USD": 0},
			PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}),
		},
	}
	for index, config := range tests {
		ex := NewExchange(1, &RealClock{})
		err := ex.EnableBorrowing(config)
		ex.Shutdown()
		if err == nil {
			t.Fatalf("invalid borrowing config %d was accepted", index)
		}
	}
}
