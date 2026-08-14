package randomwalk

import (
	"context"
	"testing"
	"time"

	etypes "exchange_sim/types"
)

func TestMarketMakerDepthIsNotionalNormalized(t *testing.T) {
	for _, asset := range assets {
		qty := quantityForUSDNotional(marketMakerLevelNotional, asset.price)
		notional, ok := etypes.TryMulDiv(qty, asset.price, btcPrecision)
		if !ok {
			t.Fatalf("%s: level notional overflow", asset.name)
		}
		tolerance := asset.price / btcPrecision
		if tolerance < 1 {
			tolerance = 1
		}
		if diff := notional - marketMakerLevelNotional; diff < -tolerance || diff > tolerance {
			t.Fatalf("%s: level notional = %d, want approximately %d", asset.name, notional, marketMakerLevelNotional)
		}
	}
}

func TestRandomWalkMaintainsTwoSidedQuotes(t *testing.T) {
	sim, err := NewSimWithConfig(10*time.Second, SimConfig{LogDir: t.TempDir(), SnapshotOnly: true})
	if err != nil {
		t.Fatalf("NewSimWithConfig: %v", err)
	}
	defer sim.Close()

	ctx := context.Background()
	sim.Exchange().StartAutomation(ctx)
	defer sim.Exchange().StopAutomation()
	var emptyAt []time.Duration
	sim.Runner.SetProgressCallback(1_000, func(done, _ int) {
		for _, mm := range sim.MMs {
			for _, symbol := range mm.Symbols() {
				bidQty, askQty := sim.Exchange().GetBestLiquidity(symbol)
				if bidQty == 0 || askQty == 0 {
					emptyAt = append(emptyAt, time.Duration(done)*time.Millisecond)
					return
				}
			}
		}
	})
	if err := sim.Runner.Run(ctx); err != nil {
		t.Fatalf("Runner.Run: %v", err)
	}
	if len(emptyAt) > 0 {
		t.Fatalf("one or more quiescent snapshots had an empty book: %v", emptyAt)
	}

	for _, mm := range sim.MMs {
		for _, symbol := range mm.Symbols() {
			bidQty, askQty := sim.Exchange().GetBestLiquidity(symbol)
			if bidQty == 0 || askQty == 0 {
				t.Errorf("%s has empty final book: bid=%d ask=%d pending=%d requests=%d", symbol, bidQty, askQty, len(mm.pending[symbol]), len(mm.reqToSym))
			}
		}
	}
}
