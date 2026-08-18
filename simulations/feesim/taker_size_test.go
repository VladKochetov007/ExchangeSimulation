package feesim

import (
	"context"
	"math"
	"math/rand"
	"sort"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// An order worth a fraction of the quoted depth is absorbed whole and cannot
// move a price, so a market whose flow is uniformly small has no impact and no
// return tails. The size draw must be able to produce the occasional order that
// walks the book.
func TestDrawSizeParetoProducesATail(t *testing.T) {
	const target = int64(1_000_000)
	uniform := &RandomTaker{cfg: TakerConfig{}, rng: rand.New(rand.NewSource(1))}
	tailed := &RandomTaker{cfg: TakerConfig{SizeParetoAlpha: 1.5}, rng: rand.New(rand.NewSource(1))}

	draw := func(taker *RandomTaker) []float64 {
		out := make([]float64, 20000)
		for i := range out {
			out[i] = float64(taker.drawSize(target))
		}
		sort.Float64s(out)
		return out
	}
	uniformDraws, tailedDraws := draw(uniform), draw(tailed)

	uniformMax := uniformDraws[len(uniformDraws)-1]
	if uniformMax > 1.6*float64(target) {
		t.Fatalf("uniform draw exceeded its band: max %.0f against target %d", uniformMax, target)
	}
	tailedMax := tailedDraws[len(tailedDraws)-1]
	if tailedMax < 5*float64(target) {
		t.Errorf("Pareto draw produced no tail: max %.0f, want at least five times the target", tailedMax)
	}
	// The median should stay near the target, so a tail does not simply inflate
	// every order into a sweep.
	median := tailedDraws[len(tailedDraws)/2]
	if median > 3*float64(target) {
		t.Errorf("Pareto median %.0f is far above the target %d", median, target)
	}
}

func TestDrawSizeCapBoundsATailDraw(t *testing.T) {
	const target = int64(1_000)
	taker := &RandomTaker{cfg: TakerConfig{SizeParetoAlpha: 0.5, SizeCapMultiple: 10}, rng: rand.New(rand.NewSource(7))}
	for i := 0; i < 50000; i++ {
		if size := taker.drawSize(target); size > 10*target {
			t.Fatalf("draw %d exceeded the configured cap of %d", size, 10*target)
		}
	}
	// A very heavy tail with no configured cap still has to stay finite.
	uncapped := &RandomTaker{cfg: TakerConfig{SizeParetoAlpha: 0.3}, rng: rand.New(rand.NewSource(9))}
	for i := 0; i < 50000; i++ {
		size := uncapped.drawSize(target)
		if size <= 0 || float64(size) > 50*float64(target) || math.IsInf(float64(size), 0) {
			t.Fatalf("uncapped draw produced %d", size)
		}
	}
}

func TestDrawSizeRejectsNonPositiveTarget(t *testing.T) {
	taker := &RandomTaker{cfg: TakerConfig{SizeParetoAlpha: 1.5}, rng: rand.New(rand.NewSource(1))}
	if got := taker.drawSize(0); got != 0 {
		t.Fatalf("drawSize(0) = %d, want 0", got)
	}
}

// An order larger than the book does not walk it: the engine fills what is
// there and discards the rest with no liquidity. Because the side is drawn
// before the book is consulted, that truncation lands on whichever side is
// thinner, so executed flow becomes conditioned on book state even though the
// drawn flow is not. Measured with a heavy size tail this produced 1557
// evaporated orders against 11 with uniform sizes, and a 3.8% net selling
// imbalance from a requested imbalance of 1.3%.
func TestTakerBoundsOrdersByTheDepthFacingThem(t *testing.T) {
	gw := newStubGateway()
	taker := NewRandomTaker(1, gw, TakerConfig{
		Symbols: []string{"ABC/USD"}, TargetQtys: map[string]int64{"ABC/USD": 1_000_000},
		TakeInterval: time.Second, Seed: 3, SizeParetoAlpha: 1.2,
	})
	now := time.Unix(0, 0)
	taker.onTick(now)
	if len(gw.placed()) != 0 {
		t.Fatal("the subscribing tick must not trade: no maker has quoted yet")
	}
	taker.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{
			Symbol: "ABC/USD",
			Snapshot: &exchange.BookSnapshot{
				Bids: []exchange.PriceLevel{{Price: 99, VisibleQty: 400_000}},
				Asks: []exchange.PriceLevel{{Price: 101, VisibleQty: 250_000}},
			},
		},
	})
	for i := 0; i < 400; i++ {
		now = now.Add(time.Second)
		taker.onTick(now)
	}
	var buys, sells int
	for _, order := range gw.placed() {
		limit := int64(250_000)
		if order.Side == exchange.Sell {
			limit = 400_000
			sells++
		} else {
			buys++
		}
		if order.Qty > limit {
			t.Fatalf("%v order of %d exceeded the %d facing it", order.Side, order.Qty, limit)
		}
	}
	if buys == 0 || sells == 0 {
		t.Fatalf("expected both sides to trade, got %d buys and %d sells", buys, sells)
	}
}
