package fee

import (
	"testing"

	etypes "exchange_sim/exchange/types"
)

const (
	btcPrecision = 100_000_000
	usdPrecision = 100_000
	dollarTick   = usdPrecision
)

func priceUSD(price float64) int64 {
	raw := int64(price * float64(usdPrecision))
	return (raw / dollarTick) * dollarTick
}

func TestPercentageFeeInQuote(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: true}
	exec := &etypes.Execution{
		Price: priceUSD(50000),
		Qty:   btcPrecision,
	}

	takerFee := fee.CalculateFee(exec, etypes.Buy, false, "BTC", "USD", btcPrecision)
	if takerFee.Asset != "USD" {
		t.Errorf("taker fee asset: want USD, got %s", takerFee.Asset)
	}
	tradeValue := (exec.Price * exec.Qty) / btcPrecision
	if want := (tradeValue * 10) / BPS; takerFee.Amount != want {
		t.Errorf("taker fee: want %d, got %d", want, takerFee.Amount)
	}

	makerFee := fee.CalculateFee(exec, etypes.Sell, true, "BTC", "USD", btcPrecision)
	if makerFee.Asset != "USD" {
		t.Errorf("maker fee asset: want USD, got %s", makerFee.Asset)
	}
	if want := (tradeValue * 5) / BPS; makerFee.Amount != want {
		t.Errorf("maker fee: want %d, got %d", want, makerFee.Amount)
	}
}

func TestPercentageFeeInBase(t *testing.T) {
	fee := &PercentageFee{MakerBps: 5, TakerBps: 10, InQuote: false}
	exec := &etypes.Execution{
		Price: priceUSD(50000),
		Qty:   btcPrecision,
	}

	takerFee := fee.CalculateFee(exec, etypes.Buy, false, "BTC", "USD", usdPrecision)
	if takerFee.Asset != "BTC" {
		t.Errorf("taker fee asset: want BTC, got %s", takerFee.Asset)
	}
	if want := int64((btcPrecision * 10) / BPS); takerFee.Amount != want {
		t.Errorf("taker fee: want %d, got %d", want, takerFee.Amount)
	}
}

func TestFixedFee(t *testing.T) {
	fee := &FixedFee{
		MakerFee: etypes.Fee{Asset: "USD", Amount: 100},
		TakerFee: etypes.Fee{Asset: "USD", Amount: 200},
	}
	exec := &etypes.Execution{Price: priceUSD(50000), Qty: btcPrecision}

	if got := fee.CalculateFee(exec, etypes.Buy, false, "BTC", "USD", usdPrecision).Amount; got != 200 {
		t.Errorf("taker fee: want 200, got %d", got)
	}
	if got := fee.CalculateFee(exec, etypes.Sell, true, "BTC", "USD", usdPrecision).Amount; got != 100 {
		t.Errorf("maker fee: want 100, got %d", got)
	}
}
