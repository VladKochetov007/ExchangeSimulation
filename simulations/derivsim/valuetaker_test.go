package derivsim

import (
	"context"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	eprice "exchange_sim/price"
	etypes "exchange_sim/types"
)

const (
	valueTakerBasePrecision = int64(100_000_000)
	valueTakerSpot          = 50_000 * valueTakerBasePrecision
	valueTakerStrike        = 50_000 * valueTakerBasePrecision
)

func snapshotEvent(symbol string, bid, ask int64) *actor.Event {
	snapshot := &etypes.BookSnapshot{}
	if bid > 0 {
		snapshot.Bids = []etypes.PriceLevel{{Price: bid, VisibleQty: valueTakerBasePrecision}}
	}
	if ask > 0 {
		snapshot.Asks = []etypes.PriceLevel{{Price: ask, VisibleQty: valueTakerBasePrecision}}
	}
	return &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: symbol, Snapshot: snapshot, Timestamp: int64(time.Second)},
	}
}

func newValueTaker(t *testing.T, vol float64, edgeBps int64) (*OptionValueTaker, *stubGateway) {
	t.Helper()
	gw := newStubGateway()
	taker := NewOptionValueTaker(2, gw, OptionValueTakerConfig{
		Underlying:    "ABC/USD",
		VolModel:      eprice.FlatVolatility(vol),
		EdgeBps:       edgeBps,
		LotQty:        valueTakerBasePrecision,
		MaxPosition:   2 * valueTakerBasePrecision,
		Interval:      time.Second,
		BasePrecision: valueTakerBasePrecision,
	})
	ctx := context.Background()
	taker.onTick(time.Unix(0, int64(time.Second)))
	listOption(taker, "ABC-OPT", valueTakerStrike, int64(30*24*time.Hour))
	taker.HandleEvent(ctx, snapshotEvent("ABC/USD", valueTakerSpot-1, valueTakerSpot+1))
	return taker, gw
}

// fairPremium is the taker's own valuation of the listed contract.
func fairPremium(vol float64) int64 {
	years := float64(30*24*time.Hour) / float64(365*24*time.Hour)
	return eprice.Black76Premium(valueTakerSpot, valueTakerStrike, vol, years, true)
}

// The point of the participant: a quote that agrees with its valuation is left
// alone, however often it looks at it.
func TestValueTakerHoldsWhenTheQuoteAgrees(t *testing.T) {
	taker, gw := newValueTaker(t, 0.8, 10)
	fair := fairPremium(0.8)
	taker.HandleEvent(context.Background(), snapshotEvent("ABC-OPT", fair-10, fair+10))
	for i := 2; i < 6; i++ {
		taker.onTick(time.Unix(0, int64(i)*int64(time.Second)))
	}
	if orders := gw.placedOrders(); len(orders) != 0 {
		t.Errorf("traded against a fair quote: %d orders", len(orders))
	}
}

// An offer well below its valuation is bought, and a bid well above it is sold.
func TestValueTakerLiftsCheapAndHitsRich(t *testing.T) {
	fair := fairPremium(0.8)
	cheap := fair / 2
	taker, gw := newValueTaker(t, 0.8, 10)
	taker.HandleEvent(context.Background(), snapshotEvent("ABC-OPT", cheap-10, cheap))
	taker.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gw.placedOrders()
	if len(orders) != 1 {
		t.Fatalf("orders = %d, want 1: %+v", len(orders), orders)
	}
	if orders[0].Side != exchange.Buy || orders[0].Symbol != "ABC-OPT" {
		t.Errorf("order = %v %s, want a buy of ABC-OPT", orders[0].Side, orders[0].Symbol)
	}

	rich := fair * 2
	sellSide, sellGw := newValueTaker(t, 0.8, 10)
	sellSide.HandleEvent(context.Background(), snapshotEvent("ABC-OPT", rich, rich+10))
	sellSide.onTick(time.Unix(0, int64(2*time.Second)))
	sold := sellGw.placedOrders()
	if len(sold) != 1 || sold[0].Side != exchange.Sell {
		t.Fatalf("orders = %+v, want a single sell", sold)
	}
}

// Two takers differing only in their volatility view must take opposite sides
// of the same quote. Disagreement is what the population trades on.
func TestValueTakersWithDifferentViewsTakeOppositeSides(t *testing.T) {
	fair := fairPremium(0.8)
	bull, bullGw := newValueTaker(t, 1.2, 10)
	bear, bearGw := newValueTaker(t, 0.4, 10)
	quote := snapshotEvent("ABC-OPT", fair-10, fair+10)
	bull.HandleEvent(context.Background(), quote)
	bear.HandleEvent(context.Background(), quote)
	bull.onTick(time.Unix(0, int64(2*time.Second)))
	bear.onTick(time.Unix(0, int64(2*time.Second)))
	bullOrders, bearOrders := bullGw.placedOrders(), bearGw.placedOrders()
	if len(bullOrders) != 1 || bullOrders[0].Side != exchange.Buy {
		t.Fatalf("the high-volatility taker did not buy: %+v", bullOrders)
	}
	if len(bearOrders) != 1 || bearOrders[0].Side != exchange.Sell {
		t.Fatalf("the low-volatility taker did not sell: %+v", bearOrders)
	}
}

// A view the market never comes round to must not become an unbounded
// position: the cap is what stops one opinion from consuming a whole balance.
func TestValueTakerStopsAtItsPositionCap(t *testing.T) {
	fair := fairPremium(0.8)
	taker, gw := newValueTaker(t, 0.8, 10)
	ctx := context.Background()
	taker.HandleEvent(ctx, snapshotEvent("ABC-OPT", fair/2-10, fair/2))
	filled := 0
	for i := 2; i < 8; i++ {
		taker.onTick(time.Unix(0, int64(i)*int64(time.Second)))
		// Each order that went out is filled straight back, since the cap is
		// checked against realised inventory rather than orders sent.
		for ; filled < len(gw.placedOrders()); filled++ {
			taker.onFill("ABC-OPT", actor.OrderFillEvent{
				Symbol: "ABC-OPT", Side: exchange.Buy, Qty: valueTakerBasePrecision, IsFull: true,
			})
		}
	}
	if filled == 0 {
		t.Fatal("the taker never traded a quote at half its valuation")
	}
	if position := taker.Position("ABC-OPT"); position > 2*valueTakerBasePrecision {
		t.Errorf("position = %d, want no more than the cap %d", position, 2*valueTakerBasePrecision)
	}
}
