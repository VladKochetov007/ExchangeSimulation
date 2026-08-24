package fee

import (
	"errors"
	"testing"

	etypes "exchange_sim/types"
)

type optionFeeSource struct {
	price int64
	err   error
}

func (s optionFeeSource) Price(string) (int64, error) { return s.price, s.err }

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

	takerFee, err := fee.CalculateFee(etypes.FillContext{Exec: exec, IsMaker: false, BaseAsset: "BTC", QuoteAsset: "USD", Precision: btcPrecision})
	if err != nil {
		t.Fatalf("taker fee: %v", err)
	}
	if takerFee.Asset != "USD" {
		t.Errorf("taker fee asset: want USD, got %s", takerFee.Asset)
	}
	tradeValue := (exec.Price * exec.Qty) / btcPrecision
	if want := (tradeValue * 10) / BPS; takerFee.Amount != want {
		t.Errorf("taker fee: want %d, got %d", want, takerFee.Amount)
	}

	makerFee, err := fee.CalculateFee(etypes.FillContext{Exec: exec, IsMaker: true, BaseAsset: "BTC", QuoteAsset: "USD", Precision: btcPrecision})
	if err != nil {
		t.Fatalf("maker fee: %v", err)
	}
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

	takerFee, err := fee.CalculateFee(etypes.FillContext{Exec: exec, IsMaker: false, BaseAsset: "BTC", QuoteAsset: "USD", Precision: usdPrecision})
	if err != nil {
		t.Fatalf("taker fee: %v", err)
	}
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

	gotFee, err := fee.CalculateFee(etypes.FillContext{Exec: exec, IsMaker: false, BaseAsset: "BTC", QuoteAsset: "USD", Precision: usdPrecision})
	if err != nil {
		t.Fatalf("taker fee: %v", err)
	}
	if got := gotFee.Amount; got != 200 {
		t.Errorf("taker fee: want 200, got %d", got)
	}
	gotFee, err = fee.CalculateFee(etypes.FillContext{Exec: exec, IsMaker: true, BaseAsset: "BTC", QuoteAsset: "USD", Precision: usdPrecision})
	if err != nil {
		t.Fatalf("maker fee: %v", err)
	}
	if got := gotFee.Amount; got != 100 {
		t.Errorf("maker fee: want 100, got %d", got)
	}
}

func TestOptionFeeUsesOnlyItsDeclaredPriceSchedule(t *testing.T) {
	ctx := etypes.FillContext{
		Exec:       &etypes.Execution{Price: 1_000_000, Qty: 1},
		BaseAsset:  "ABC-C",
		QuoteAsset: "USD",
		Precision:  1,
	}
	withUnderlying := &OptionFee{
		TakerUnderlyingBps: 1,
		Source:             optionFeeSource{price: 1},
		SymbolMap:          func(_, _ string) string { return "ABC/USD" },
	}
	fee, err := withUnderlying.CalculateFee(ctx)
	if err != nil {
		t.Fatalf("underlying schedule: %v", err)
	}
	if fee.Amount != 0 {
		t.Fatalf("rounded underlying fee = %d, want 0 rather than premium fallback", fee.Amount)
	}

	withoutUnderlying := &OptionFee{TakerUnderlyingBps: 1}
	fee, err = withoutUnderlying.CalculateFee(ctx)
	if err != nil {
		t.Fatalf("premium schedule: %v", err)
	}
	if fee.Amount != 100 {
		t.Fatalf("premium fee = %d, want 100", fee.Amount)
	}

	withUnderlying.Source = optionFeeSource{err: errors.New("index unavailable")}
	if _, err := withUnderlying.CalculateFee(ctx); err == nil {
		t.Fatal("configured unavailable underlying silently selected premium schedule")
	}
}

func TestOptionFeeRejectsPresentNonPositiveUnderlyingAsDomainError(t *testing.T) {
	ctx := etypes.FillContext{
		Exec:       &etypes.Execution{Price: 1_000_000, Qty: 1},
		BaseAsset:  "ABC-C",
		QuoteAsset: "USD",
		Precision:  1,
	}
	for _, value := range []int64{-1, 0} {
		fee := &OptionFee{
			TakerUnderlyingBps: 1,
			Source:             optionFeeSource{price: value},
			SymbolMap:          func(_, _ string) string { return "ABC/USD" },
		}
		if _, err := fee.CalculateFee(ctx); !errors.Is(err, etypes.ErrPriceDomain) || errors.Is(err, etypes.ErrNoPrice) {
			t.Fatalf("underlying %d error = %v, want domain error only", value, err)
		}
	}
}
