package feesim

import (
	"math"
	"math/rand"
	"sort"
	"testing"
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
