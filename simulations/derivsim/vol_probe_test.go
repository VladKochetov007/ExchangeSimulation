package derivsim

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	eprice "exchange_sim/price"
	etypes "exchange_sim/types"
)

func TestProbeDealerEstimatorUpdates(t *testing.T) {
	gw := newStubGateway()
	est := eprice.NewRealizedVolatility(0.8, 300, 1.0, 0.2, 3.0)
	mm := NewOptionMarketMaker(1, gw, OptionMMConfig{
		Underlying: "ABC/USD", IV: 0.8, VolModel: est, QuoteInterval: time.Second, BasePrecision: 100_000_000,
	})
	price := int64(4990305000)
	for i := 1; i <= 300; i++ {
		if i%2 == 0 {
			price += 200000
		} else {
			price -= 150000
		}
		mm.HandleEvent(context.Background(), &actor.Event{
			Type: actor.EventBookSnapshot,
			Data: actor.BookSnapshotEvent{Symbol: "ABC/USD", Timestamp: int64(i) * int64(time.Second),
				Snapshot: &etypes.BookSnapshot{
					Bids: []etypes.PriceLevel{{Price: price - 1000, VisibleQty: 1}},
					Asks: []etypes.PriceLevel{{Price: price + 1000, VisibleQty: 1}},
				}},
		})
	}
	t.Logf("samples=%d dealerVol=%v", est.Samples(), mm.volatility(0, 0, true))
}
