package exchange

import "testing"

func TestPercentageFeeInQuote(t *testing.T) {
	fee := &PercentageFee{
		MakerBps: 5,
		TakerBps: 10,
		InQuote:  true,
	}

	exec := &Execution{
		Price: PriceUSD(50000, DOLLAR_TICK),
		Qty:   BTC_PRECISION,
	}

	takerFee := fee.CalculateFee(exec, Buy, false, "BTC", "USD", BTC_PRECISION)
	if takerFee.Asset != "USD" {
		t.Errorf("Taker fee asset should be USD, got %s", takerFee.Asset)
	}
	tradeValue := (exec.Price * exec.Qty) / BTC_PRECISION
	expectedTakerFee := (tradeValue * 10) / BPS
	if takerFee.Amount != expectedTakerFee {
		t.Errorf("Taker fee should be %d, got %d", expectedTakerFee, takerFee.Amount)
	}

	makerFee := fee.CalculateFee(exec, Sell, true, "BTC", "USD", BTC_PRECISION)
	if makerFee.Asset != "USD" {
		t.Errorf("Maker fee asset should be USD, got %s", makerFee.Asset)
	}
	expectedMakerFee := (tradeValue * 5) / BPS
	if makerFee.Amount != expectedMakerFee {
		t.Errorf("Maker fee should be %d, got %d", expectedMakerFee, makerFee.Amount)
	}
}

func TestPercentageFeeInBase(t *testing.T) {
	fee := &PercentageFee{
		MakerBps: 5,
		TakerBps: 10,
		InQuote:  false,
	}

	exec := &Execution{
		Price: PriceUSD(50000, DOLLAR_TICK),
		Qty:   BTC_PRECISION,
	}

	takerFee := fee.CalculateFee(exec, Buy, false, "BTC", "USD", USD_PRECISION)
	if takerFee.Asset != "BTC" {
		t.Errorf("Taker fee asset should be BTC, got %s", takerFee.Asset)
	}
	expectedTakerFee := int64((BTC_PRECISION * 10) / BPS)
	if takerFee.Amount != expectedTakerFee {
		t.Errorf("Taker fee should be %d, got %d", expectedTakerFee, takerFee.Amount)
	}
}

func TestFixedFee(t *testing.T) {
	fee := &FixedFee{
		MakerFee: Fee{Asset: "USD", Amount: 100},
		TakerFee: Fee{Asset: "USD", Amount: 200},
	}

	exec := &Execution{
		Price: PriceUSD(50000, DOLLAR_TICK),
		Qty:   BTC_PRECISION,
	}

	takerFee := fee.CalculateFee(exec, Buy, false, "BTC", "USD", USD_PRECISION)
	if takerFee.Amount != 200 {
		t.Errorf("Taker fee should be 200, got %d", takerFee.Amount)
	}

	makerFee := fee.CalculateFee(exec, Sell, true, "BTC", "USD", USD_PRECISION)
	if makerFee.Amount != 100 {
		t.Errorf("Maker fee should be 100, got %d", makerFee.Amount)
	}
}
