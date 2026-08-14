package matching

import (
	"testing"

	ebook "exchange_sim/book"
	eclock "exchange_sim/clock"
	etypes "exchange_sim/types"
)

type matcherCase struct {
	name    string
	new     func() MatchingEngine
	proRata bool
}

func matcherCases() []matcherCase {
	return []matcherCase{
		{
			name: "price_time",
			new: func() MatchingEngine {
				return NewPriceTimeMatcher(&eclock.RealClock{})
			},
		},
		{
			name: "pro_rata",
			new: func() MatchingEngine {
				return NewProRataMatcher(&eclock.RealClock{})
			},
			proRata: true,
		},
	}
}

func snapshotLevel(t *testing.T, side *ebook.Book, wantPrice, wantTotal, wantVisible, wantHidden int64) {
	t.Helper()
	if side.Best == nil {
		t.Fatal("expected a resting level")
	}
	if side.Best.Price != wantPrice || side.Best.TotalQty != wantTotal {
		t.Fatalf("best level = price %d total %d, want price %d total %d",
			side.Best.Price, side.Best.TotalQty, wantPrice, wantTotal)
	}
	snapshot := side.GetSnapshot()
	if len(snapshot) != 1 {
		t.Fatalf("snapshot level count = %d, want 1", len(snapshot))
	}
	got := snapshot[0]
	if got.VisibleQty != wantVisible || got.HiddenQty != wantHidden {
		t.Fatalf("snapshot visible/hidden = %d/%d, want %d/%d",
			got.VisibleQty, got.HiddenQty, wantVisible, wantHidden)
	}
}

func TestLevelTotalQtyTracksPartialAndFullFills(t *testing.T) {
	for _, tc := range matcherCases() {
		t.Run(tc.name, func(t *testing.T) {
			bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
			first := mkSell(1, 1, 100, 100)
			second := mkSell(2, 2, 101, 70)
			if !asks.AddOrder(first) || !asks.AddOrder(second) {
				t.Fatal("failed to seed resting book")
			}

			// This fully consumes the best level, then partially consumes the next
			// level. The latter must report only its 30-unit remainder.
			result := tc.new().Match(bids, asks, mkBuy(3, 3, etypes.LimitOrder, 101, 140))
			if !result.FullyFilled || len(result.Executions) != 2 {
				t.Fatalf("first sweep = fullyFilled %v executions %d, want true/2", result.FullyFilled, len(result.Executions))
			}
			if first.FilledQty != 100 || first.Parent != nil {
				t.Fatalf("fully filled maker = filled %d parent %p, want 100/nil", first.FilledQty, first.Parent)
			}
			if second.FilledQty != 40 || second.Parent != asks.Best {
				t.Fatalf("partial maker = filled %d parent %p, want 40/live level", second.FilledQty, second.Parent)
			}
			snapshotLevel(t, asks, 101, 30, 30, 0)

			result = tc.new().Match(bids, asks, mkBuy(4, 4, etypes.LimitOrder, 101, 30))
			if !result.FullyFilled || len(result.Executions) != 1 || asks.Best != nil {
				t.Fatalf("final sweep = fullyFilled %v executions %d best %p, want true/1/nil", result.FullyFilled, len(result.Executions), asks.Best)
			}
		})
	}
}

func TestCancelAfterPartialFillSubtractsOnlyTheRemainder(t *testing.T) {
	for _, tc := range matcherCases() {
		t.Run(tc.name, func(t *testing.T) {
			bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
			first := mkSell(1, 1, 100, 100)
			second := mkSell(2, 2, 100, 50)
			if !asks.AddOrder(first) || !asks.AddOrder(second) {
				t.Fatal("failed to seed resting level")
			}
			result := tc.new().Match(bids, asks, mkBuy(3, 3, etypes.LimitOrder, 100, 40))
			if !result.FullyFilled || first.FilledQty == 0 {
				t.Fatal("expected a partial fill on the first maker")
			}

			if cancelled := asks.CancelOrder(first.ID); cancelled != first {
				t.Fatalf("cancelled order = %p, want first maker %p", cancelled, first)
			}
			remainingSecond := second.Qty - second.FilledQty
			snapshotLevel(t, asks, 100, remainingSecond, remainingSecond, 0)
		})
	}
}

func TestIcebergRefreshPreservesLevelTotalsAndDepth(t *testing.T) {
	for _, tc := range matcherCases() {
		t.Run(tc.name, func(t *testing.T) {
			bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
			iceberg := &etypes.Order{
				ID:               1,
				ClientID:         1,
				Side:             etypes.Sell,
				Type:             etypes.LimitOrder,
				Price:            100,
				Qty:              100,
				Visibility:       etypes.Iceberg,
				IcebergQty:       25,
				DisplayRemaining: 25,
			}
			normal := mkSell(2, 2, 100, 100)
			if !asks.AddOrder(iceberg) || !asks.AddOrder(normal) {
				t.Fatal("failed to seed iceberg level")
			}

			// This first fill exercises the displayed iceberg tranche. FIFO consumes
			// it outright and refreshes; pro-rata shares it with the normal order.
			result := tc.new().Match(bids, asks, mkBuy(3, 3, etypes.LimitOrder, 100, 25))
			if !result.FullyFilled {
				t.Fatal("first fill should consume 25 displayed units")
			}
			if tc.proRata {
				// Pro-rata shares the first 25 between the 25-unit iceberg
				// tranche and the 100-unit normal order. The tranche remains
				// live, so it does not re-queue yet.
				if asks.Best.Head != iceberg || asks.Best.Tail != normal {
					t.Fatalf("pro-rata queue = head %d tail %d, want iceberg then normal", asks.Best.Head.ID, asks.Best.Tail.ID)
				}
				snapshotLevel(t, asks, 100, 175, 100, 75)
			} else {
				if len(result.Executions) != 1 {
					t.Fatalf("FIFO tranche fill emitted %d executions, want 1", len(result.Executions))
				}
				if asks.Best.Head != normal || asks.Best.Tail != iceberg {
					t.Fatalf("refresh queue = head %d tail %d, want normal then iceberg", asks.Best.Head.ID, asks.Best.Tail.ID)
				}
				snapshotLevel(t, asks, 100, 175, 125, 50)
			}

			result = tc.new().Match(bids, asks, mkBuy(4, 4, etypes.LimitOrder, 100, 10))
			if !result.FullyFilled {
				t.Fatal("second fill should consume 10 visible units")
			}
			if tc.proRata {
				snapshotLevel(t, asks, 100, 165, 90, 75)
			} else {
				snapshotLevel(t, asks, 100, 165, 115, 50)
			}

			result = tc.new().Match(bids, asks, mkBuy(5, 5, etypes.LimitOrder, 100, 165))
			if !result.FullyFilled || asks.Best != nil {
				t.Fatalf("final iceberg sweep = fullyFilled %v best %p, want true/nil", result.FullyFilled, asks.Best)
			}
		})
	}
}

func TestIcebergWithoutTransientDisplayDoesNotExposeReserve(t *testing.T) {
	for _, tc := range matcherCases() {
		t.Run(tc.name, func(t *testing.T) {
			bids, asks := ebook.NewBook(etypes.Buy), ebook.NewBook(etypes.Sell)
			// DisplayRemaining is deliberately unset, as it is for a restored or
			// directly injected public Order. Public depth already falls back to
			// IcebergQty; matching must use the same tranche.
			iceberg := &etypes.Order{
				ID:         1,
				ClientID:   1,
				Side:       etypes.Sell,
				Type:       etypes.LimitOrder,
				Price:      100,
				Qty:        100,
				Visibility: etypes.Iceberg,
				IcebergQty: 25,
			}
			if !asks.AddOrder(iceberg) {
				t.Fatal("failed to add iceberg")
			}
			if got := ebook.VisibleQty(asks.Best); got != 25 {
				t.Fatalf("advertised iceberg display = %d, want 25", got)
			}

			result := tc.new().Match(bids, asks, mkBuy(2, 2, etypes.LimitOrder, 100, 100))
			if !result.FullyFilled || len(result.Executions) != 4 {
				t.Fatalf("restored iceberg executions = %d fullyFilled %v, want 4/true", len(result.Executions), result.FullyFilled)
			}
			for i, exec := range result.Executions {
				if exec.Qty != 25 {
					t.Fatalf("execution %d qty = %d, want display tranche 25", i, exec.Qty)
				}
			}
		})
	}
}
