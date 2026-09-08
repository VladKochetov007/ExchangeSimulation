package exchange

import "testing"

func TestCollateralInterestCarriesFractionalRemainder(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	client := ex.Clients[1]
	client.Borrowed["USD"] = collateralInterestDenominator / 2
	client.PerpBalances["USD"] = 100
	ex.CollateralRate = 1
	log := &recordingLogger{}
	ex.SetLogger("_global", log)

	ex.ChargeCollateralInterest()
	if got := client.PerpBalances["USD"]; got != 100 {
		t.Fatalf("first fractional accrual changed balance: got %d want 100", got)
	}
	if got := ex.ExchangeBalance.FeeRevenue["USD"]; got != 0 {
		t.Fatalf("first fractional accrual posted revenue: got %d want 0", got)
	}
	if got := ex.collateralInterestRemainders[1]["USD"]; got != collateralInterestDenominator/2 {
		t.Fatalf("first remainder = %d want %d", got, collateralInterestDenominator/2)
	}

	clock.now = 2_000
	ex.ChargeCollateralInterest()
	if got := client.PerpBalances["USD"]; got != 99 {
		t.Fatalf("second accrual balance = %d want 99", got)
	}
	if got := ex.ExchangeBalance.FeeRevenue["USD"]; got != 1 {
		t.Fatalf("second accrual revenue = %d want 1", got)
	}
	if got := ex.collateralInterestRemainders[1]["USD"]; got != 0 {
		t.Fatalf("second remainder = %d want 0", got)
	}

	var accruals []MarginInterestAccrualEvent
	for _, record := range log.records {
		if record.event != "margin_interest_accrual" {
			continue
		}
		accrual, ok := record.data.(MarginInterestAccrualEvent)
		if !ok {
			t.Fatalf("accrual event type = %T", record.data)
		}
		accruals = append(accruals, accrual)
	}
	if len(accruals) != 2 {
		t.Fatalf("accrual event count = %d want 2: %#v", len(accruals), log.records)
	}
	if accruals[0].RemainderBefore != 0 || accruals[0].RemainderAfter != collateralInterestDenominator/2 ||
		accruals[0].Interest != 0 || accruals[0].SpotInterest != 0 || accruals[0].PerpInterest != 0 {
		t.Fatalf("first accrual event = %+v", accruals[0])
	}
	if accruals[1].RemainderBefore != collateralInterestDenominator/2 || accruals[1].RemainderAfter != 0 ||
		accruals[1].Interest != 1 || accruals[1].PerpInterest != 1 {
		t.Fatalf("second accrual event = %+v", accruals[1])
	}
	for _, accrual := range accruals {
		if accrual.IntervalSeconds != collateralInterestIntervalSeconds || accrual.Denominator != collateralInterestDenominator || accrual.RateBps != 1 {
			t.Fatalf("accrual contract fields = %+v", accrual)
		}
	}
}

func TestCollateralInterestRejectsDuplicateTimestampAtomically(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	client := ex.Clients[1]
	client.Borrowed["USD"] = collateralInterestDenominator
	client.PerpBalances["USD"] = 10
	ex.CollateralRate = 1
	log := &recordingLogger{}
	ex.SetLogger("_global", log)

	ex.ChargeCollateralInterest()
	beforeBalance := client.PerpBalances["USD"]
	beforeRevenue := ex.ExchangeBalance.FeeRevenue["USD"]
	beforeRemainder := ex.collateralInterestRemainders[1]["USD"]

	ex.ChargeCollateralInterest()
	if client.PerpBalances["USD"] != beforeBalance || ex.ExchangeBalance.FeeRevenue["USD"] != beforeRevenue || ex.collateralInterestRemainders[1]["USD"] != beforeRemainder {
		t.Fatalf("duplicate timestamp mutated state: balance=%d revenue=%d remainder=%d", client.PerpBalances["USD"], ex.ExchangeBalance.FeeRevenue["USD"], ex.collateralInterestRemainders[1]["USD"])
	}
	failureObserved := false
	for _, record := range log.records {
		if record.event == "margin_interest_failed" {
			failureObserved = true
		}
	}
	if !failureObserved {
		t.Fatalf("duplicate timestamp did not emit failure: %#v", log.records)
	}
}

func TestCollateralInterestRemainderCloseIsExplicit(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:     true,
		BorrowRates: map[string]int64{"USD": 1},
		PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}),
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}
	client := ex.Clients[1]
	principal := collateralInterestDenominator / 2
	client.Borrowed["USD"] = principal
	client.PerpBalances["USD"] = principal
	ex.CollateralRate = 1
	log := &recordingLogger{}
	ex.SetLogger("_global", log)
	ex.ChargeCollateralInterest()

	if err := ex.RepayMargin(1, "USD", principal); err != nil {
		t.Fatalf("repay full debt: %v", err)
	}
	if _, present := ex.collateralInterestRemainders[1]; present {
		t.Fatalf("closed debt retained remainder map: %#v", ex.collateralInterestRemainders[1])
	}
	closed := false
	for _, record := range log.records {
		if record.event != "margin_interest_remainder_closed" {
			continue
		}
		event, ok := record.data.(MarginInterestRemainderClosedEvent)
		if !ok {
			t.Fatalf("close event type = %T", record.data)
		}
		if event.RemainderBefore != principal || event.RemainderAfter != 0 || event.Reason != "debt_repaid" {
			t.Fatalf("close event = %+v", event)
		}
		closed = true
	}
	if !closed {
		t.Fatalf("fractional remainder close was not recorded: %#v", log.records)
	}
}

func TestCollateralInterestUsesBorrowingRateAuthority(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:     true,
		BorrowRates: map[string]int64{"USD": 1},
		PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}),
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}
	client := ex.Clients[1]
	client.Borrowed["USD"] = collateralInterestDenominator
	client.PerpBalances["USD"] = 2
	// A post-enable mutation of the legacy field must not create a different
	// rate from the one recorded by the borrowing configuration.
	ex.CollateralRate = 500
	log := &recordingLogger{}
	ex.SetLogger("_global", log)

	ex.ChargeCollateralInterest()
	if got := client.PerpBalances["USD"]; got != 1 {
		t.Fatalf("accrual used automation rate: balance=%d want 1", got)
	}
	for _, record := range log.records {
		if record.event != "margin_interest_accrual" {
			continue
		}
		accrual, ok := record.data.(MarginInterestAccrualEvent)
		if !ok {
			t.Fatalf("accrual event type = %T", record.data)
		}
		if accrual.RateBps != 1 || accrual.Interest != 1 {
			t.Fatalf("accrual rate authority = %+v", accrual)
		}
		return
	}
	t.Fatalf("no accrual event: %#v", log.records)
}

func TestCollateralInterestClosedRemainderDoesNotTransferToNewDebt(t *testing.T) {
	clock := &expiryManualClock{now: 1_000}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	if err := ex.EnableBorrowing(BorrowingConfig{
		Enabled:     true,
		BorrowRates: map[string]int64{"USD": 1},
		PriceSource: NewStaticPriceOracle(map[string]int64{"USD": 1}),
	}); err != nil {
		t.Fatalf("enable borrowing: %v", err)
	}
	client := ex.Clients[1]
	principal := collateralInterestDenominator / 2
	client.Borrowed["USD"] = principal
	client.PerpBalances["USD"] = principal
	log := &recordingLogger{}
	ex.SetLogger("_global", log)

	ex.ChargeCollateralInterest()
	if err := ex.RepayMargin(1, "USD", principal); err != nil {
		t.Fatalf("repay first debt: %v", err)
	}
	if _, present := ex.collateralInterestRemainders[1]; present {
		t.Fatalf("closed first debt retained remainder: %#v", ex.collateralInterestRemainders[1])
	}

	// Simulate a later loan after the first debt's explicit terminal write-off.
	clock.now = 2_000
	client.Borrowed["USD"] = principal
	client.PerpBalances["USD"] = principal
	ex.ChargeCollateralInterest()

	var accruals []MarginInterestAccrualEvent
	for _, record := range log.records {
		if record.event != "margin_interest_accrual" {
			continue
		}
		accrual, ok := record.data.(MarginInterestAccrualEvent)
		if !ok {
			t.Fatalf("accrual event type = %T", record.data)
		}
		accruals = append(accruals, accrual)
	}
	if len(accruals) != 2 || accruals[1].RemainderBefore != 0 || accruals[1].RemainderAfter != principal {
		t.Fatalf("new debt inherited closed remainder: %#v", accruals)
	}
}

func TestCollateralInterestArithmeticHelperRejectsUnrepresentableQuotient(t *testing.T) {
	if _, _, ok := collateralInterestAccrual(^int64(0)>>1, ^int64(0)>>1, 0); ok {
		t.Fatal("unrepresentable interest quotient was accepted")
	}
	if _, _, ok := collateralInterestAccrual(1, -1, 0); ok {
		t.Fatal("negative interest rate was accepted")
	}
	if _, _, ok := collateralInterestAccrual(1, 1, collateralInterestDenominator); ok {
		t.Fatal("out-of-range previous remainder was accepted")
	}
}
