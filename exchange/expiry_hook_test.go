package exchange

import (
	"testing"
	"time"
)

func TestPreExpiryHookObservesPositionBeforeSettlement(t *testing.T) {
	clock := &RealClock{}
	ex := NewExchange(2, clock)
	option := NewEuropeanOption(
		"ABC-EXP-C", "ABC", "USD", "ABC/USD", valuationBasePrecision, valuationQuotePrecision,
		valuationQuotePrecision, valuationBasePrecision/100, 100*valuationQuotePrecision,
		time.Now().Add(-time.Second).UnixNano(), true,
	)
	option.SetMarks(100*valuationQuotePrecision, 10*valuationQuotePrecision)
	ex.AddInstrument(option)
	ex.ConnectNewClient(1, nil, &FixedFee{})
	ex.AddPerpBalance(1, "USD", 100*valuationQuotePrecision)
	ex.Positions.UpdatePosition(1, option.Symbol(), valuationBasePrecision, 10*valuationQuotePrecision, Buy, PositionBoth)

	called := false
	ex.ConfigureAutomation(AutomationConfig{PreExpiryHook: func() {
		called = true
		if ex.Instruments[option.Symbol()] == nil {
			t.Fatal("pre-expiry hook ran after delisting")
		}
		if got := ex.Positions.GetPosition(1, option.Symbol()); got == nil || got.Size != valuationBasePrecision {
			t.Fatalf("pre-expiry hook lost position: %#v", got)
		}
		report, err := ex.MarkedAccount(1, usdValuationSpec(100*valuationQuotePrecision))
		if err != nil {
			t.Fatalf("MarkedAccount in pre-expiry hook: %v", err)
		}
		if len(report.Positions) != 1 || report.Positions[0].MarkPrice != 10*valuationQuotePrecision {
			t.Fatalf("pre-expiry report = %#v", report)
		}
	}})

	ex.CheckExpiries()
	if !called {
		t.Fatal("pre-expiry hook was not called")
	}
	if ex.Instruments[option.Symbol()] != nil {
		t.Fatalf("expiry settlement did not delist/flatten option")
	}
	if pos := ex.Positions.GetPosition(1, option.Symbol()); pos == nil || pos.Size != 0 {
		t.Fatalf("expiry settlement left position: %#v", pos)
	}
}
