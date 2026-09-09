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
	// Settlement consumes a delivered declared-reference observation, not the
	// cached mark pair alone.
	option.ObserveSettlement(100*valuationQuotePrecision, clock.NowUnixNano())
	ex.AddInstrument(option)
	for _, clientID := range []uint64{1, 2} {
		ex.ConnectNewClient(clientID, nil, &FixedFee{})
		ex.AddPerpBalance(clientID, "USD", 100*valuationQuotePrecision)
	}
	ex.Positions.UpdatePosition(1, option.Symbol(), valuationBasePrecision, 10*valuationQuotePrecision, Buy, PositionBoth)
	// The closed simulation settles one net option book. Keep the hook's
	// single-account observation while supplying the matching short required
	// for a terminal zero-net settlement.
	ex.Positions.UpdatePosition(2, option.Symbol(), valuationBasePrecision, 10*valuationQuotePrecision, Sell, PositionBoth)

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
		if len(report.Positions) != 1 || report.Positions[0].MarkPrice == nil || *report.Positions[0].MarkPrice != 10*valuationQuotePrecision {
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

func TestPostDerivativeMarkHookCanReadCurrentMarks(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	defer ex.Shutdown()

	hookResult := make(chan error, 1)
	ex.ConfigureAutomation(AutomationConfig{PostDerivativeMarkHook: func() {
		hookResult <- ex.ValidateMaintenanceAtCurrentMarks()
	}})

	updateDone := make(chan struct{})
	go func() {
		ex.UpdateDerivativeMarks()
		close(updateDone)
	}()

	select {
	case err := <-hookResult:
		if err != nil {
			t.Fatalf("post-derivative hook risk inspection: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("post-derivative mark hook could not re-enter public risk inspection")
	}

	select {
	case <-updateDone:
	case <-time.After(2 * time.Second):
		t.Fatal("derivative mark update did not complete after post-mark hook")
	}
}
