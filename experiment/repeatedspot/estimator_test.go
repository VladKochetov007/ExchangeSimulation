package repeatedspot

import (
	"math"
	"testing"

	worldspot "exchange_sim/simulations/repeatedspot"
)

func TestDeliveredVarianceUsesOnlyAcceptedPastSamples(t *testing.T) {
	parameters := &makerParameters{initialVariance: 1e-6, halfLifeNanos: 4_000_000_000,
		sampleIntervalNanos: 2_000_000_000}
	estimate := deliveredVariance{value: parameters.initialVariance}
	for _, observation := range []worldspot.MakerObservation{
		{Kind: "trade", SourceAt: 1_000_000_000, TradePrice: 100},
		{Kind: "trade", SourceAt: 2_000_000_000, TradePrice: 109},
		{Kind: "trade", SourceAt: 5_000_000_000, TradePrice: 101},
	} {
		if err := estimate.observe(parameters, observation); err != nil {
			t.Fatal(err)
		}
	}
	alpha := 1 - math.Exp(-math.Ln2)
	instant := math.Pow(math.Log(101.0/100.0), 2) / 4
	want := 1e-6 + alpha*(instant-1e-6)
	if estimate.samples != 2 || estimate.lastPrice != 101 || math.Abs(estimate.value-want) > 1e-16 {
		t.Fatalf("sample-window reconstruction = %+v, want variance %.12g", estimate, want)
	}
	if err := estimate.verifyDecision(worldspot.MakerDecision{
		DeliveredTradeSamples: 2, LogVariancePerSecond: want,
	}); err != nil {
		t.Fatal(err)
	}
	if err := estimate.verifyDecision(worldspot.MakerDecision{
		DeliveredTradeSamples: 3, LogVariancePerSecond: want,
	}); err == nil {
		t.Fatal("invented trade sample accepted")
	}
}
