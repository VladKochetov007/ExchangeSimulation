package exchange

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"testing"

	ematching "exchange_sim/matching"
	etypes "exchange_sim/types"
)

// This file audits one claim the admission path depends on: the MatchResult
// previewMatchExcluding produces on a detached copy of the book is the same
// MatchResult the configured matcher produces when it is actually run against
// the live book (with the excluded makers removed, which is what the caller
// does before committing). The comparison is differential and randomised.

type previewFuzzClock struct{ now int64 }

func (c *previewFuzzClock) NowUnixNano() int64 { return c.now }
func (c *previewFuzzClock) NowUnix() int64     { return c.now / 1e9 }

// execFacts is the observable content of an execution. Timestamp is included:
// both paths run under the same fixed clock, so it must agree too.
type execFacts struct {
	TakerOrderID   uint64
	MakerOrderID   uint64
	TakerClientID  uint64
	MakerClientID  uint64
	Price          int64
	Qty            int64
	Timestamp      int64
	TakerFilledQty int64
	MakerFilledQty int64
	MakerTotalQty  int64
	MakerSide      etypes.Side
	MakerPosSide   etypes.PositionSide
}

func factsOf(executions []*etypes.Execution) []execFacts {
	out := make([]execFacts, 0, len(executions))
	for _, e := range executions {
		if e == nil {
			out = append(out, execFacts{})
			continue
		}
		out = append(out, execFacts{
			TakerOrderID: e.TakerOrderID, MakerOrderID: e.MakerOrderID,
			TakerClientID: e.TakerClientID, MakerClientID: e.MakerClientID,
			Price: e.Price, Qty: e.Qty, Timestamp: e.Timestamp,
			TakerFilledQty: e.TakerFilledQty, MakerFilledQty: e.MakerFilledQty,
			MakerTotalQty: e.MakerTotalQty, MakerSide: e.MakerSide, MakerPosSide: e.MakerPosSide,
		})
	}
	return out
}

// bookFacts is a full structural fingerprint of one side: level order, level
// aggregates, and the exact queue at each level with per-order live state.
func bookFacts(b *Book) string {
	s := fmt.Sprintf("side=%d best=", b.Side)
	if b.Best != nil {
		s += fmt.Sprintf("%d", b.Best.Price)
	} else {
		s += "nil"
	}
	s += " levels["
	for l := b.ActiveHead; l != nil; l = l.Next {
		s += fmt.Sprintf("{p=%d tq=%d n=%d q=", l.Price, l.TotalQty, l.OrderCnt)
		for o := l.Head; o != nil; o = o.Next {
			s += fmt.Sprintf("(%d c%d qty=%d f=%d vis=%d ice=%d disp=%d st=%d)",
				o.ID, o.ClientID, o.Qty, o.FilledQty, o.Visibility, o.IcebergQty, o.DisplayRemaining, o.Status)
		}
		s += "}"
	}
	s += "] index["
	// Deterministic: walk IDs in ascending numeric order.
	for id := uint64(0); id < 4096; id++ {
		if o, ok := b.Orders[id]; ok {
			s += fmt.Sprintf("(%d f=%d st=%d disp=%d)", id, o.FilledQty, o.Status, o.DisplayRemaining)
		}
	}
	return s + "]"
}

func orderBookFacts(ob *OrderBook) string {
	return bookFacts(ob.Bids) + "||" + bookFacts(ob.Asks)
}

// coverage records which of the deliberately targeted conditions the random
// corpus actually reached, so a passing run is a claim about tested states
// rather than about the generator's intentions.
type coverage map[string]int

func (c coverage) hit(feature string) { c[feature]++ }

func (c coverage) report(t *testing.T, name string) {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("  %-34s %7d", k, c[k])
	}
}

func opposite(ob *OrderBook, side etypes.Side) *Book {
	if side == etypes.Buy {
		return ob.Asks
	}
	return ob.Bids
}

func crosses(incoming *etypes.Order, price int64) bool {
	if incoming.Type == etypes.Market {
		return true
	}
	if incoming.Side == etypes.Buy {
		return incoming.Price >= price
	}
	return incoming.Price <= price
}

// tallyScenario records the structural facts of one scenario before matching.
func tallyScenario(c coverage, ob *OrderBook, incoming *etypes.Order, excluded map[uint64]struct{}) {
	opp := opposite(ob, incoming.Side)
	if opp.ActiveHead == nil {
		c.hit("book/opposite-side-empty")
	}
	if ob.Bids.ActiveHead == nil && ob.Asks.ActiveHead == nil {
		c.hit("book/both-sides-empty")
	}
	levels, reachable, unreachable := 0, 0, 0
	for l := opp.ActiveHead; l != nil; l = l.Next {
		levels++
		if crosses(incoming, l.Price) {
			reachable++
		} else {
			unreachable++
		}
		if l.OrderCnt == 1 {
			c.hit("level/single-order")
		}
		if l.OrderCnt > 1 {
			c.hit("level/queue-depth>1")
		}
		if l.OrderCnt > 3 {
			c.hit("level/queue-depth>3")
		}
		included, ownedByIncoming := 0, 0
		for o := l.Head; o != nil; o = o.Next {
			if _, skip := excluded[o.ID]; !skip {
				included++
			}
			if o.ClientID == incoming.ClientID {
				ownedByIncoming++
			}
			switch o.Visibility {
			case etypes.Iceberg:
				c.hit("resting/iceberg")
				if o.FilledQty > 0 {
					c.hit("resting/iceberg-already-refreshed")
				}
			case etypes.Hidden:
				c.hit("resting/hidden")
			}
			if o.FilledQty > 0 {
				c.hit("resting/partially-filled")
			}
		}
		if included == 0 && l.OrderCnt > 0 {
			c.hit("excl/level-fully-excluded")
		}
		if int32(ownedByIncoming) == l.OrderCnt && l.OrderCnt > 0 && crosses(incoming, l.Price) {
			c.hit("selftrade/level-is-all-own-orders")
		}
		if ownedByIncoming > 0 && crosses(incoming, l.Price) {
			c.hit("selftrade/own-order-at-crossable-level")
		}
	}
	if levels > 1 {
		c.hit("book/multiple-levels")
	}
	if reachable > 1 {
		c.hit("book/multiple-reachable-levels")
	}
	if unreachable > 0 && reachable > 0 {
		c.hit("book/has-unreachable-level")
	}
	if len(excluded) == 0 {
		c.hit("excl/empty")
	} else {
		c.hit("excl/non-empty")
		if opp.ActiveHead != nil {
			if _, skip := excluded[opp.ActiveHead.Head.ID]; skip {
				c.hit("excl/touch-order-excluded")
			}
		}
	}
	switch incoming.Type {
	case etypes.Market:
		c.hit("incoming/market")
	default:
		c.hit("incoming/limit")
	}
	switch incoming.TimeInForce {
	case etypes.IOC:
		c.hit("incoming/tif-IOC")
	case etypes.FOK:
		c.hit("incoming/tif-FOK")
	default:
		c.hit("incoming/tif-GTC")
	}
	if incoming.PostOnly {
		c.hit("incoming/post-only")
	}
	switch incoming.Visibility {
	case etypes.Iceberg:
		c.hit("incoming/iceberg")
	case etypes.Hidden:
		c.hit("incoming/hidden")
	}
}

func tallyOutcome(c coverage, facts []execFacts, fullyFilled bool, qty int64) {
	switch {
	case len(facts) == 0:
		c.hit("outcome/no-fill")
	case fullyFilled:
		c.hit("outcome/full-fill")
	default:
		c.hit("outcome/partial-fill")
	}
	prices := map[int64]bool{}
	makers := map[uint64]int{}
	for _, f := range facts {
		prices[f.Price] = true
		makers[f.MakerOrderID]++
	}
	if len(prices) > 1 {
		c.hit("outcome/executions-across-levels")
	}
	if len(facts) > 1 {
		c.hit("outcome/multiple-executions")
	}
	for _, n := range makers {
		if n > 1 {
			c.hit("outcome/same-maker-filled-twice")
			break
		}
	}
}

const (
	fuzzMinPrice = 95
	fuzzMaxPrice = 105
	fuzzClients  = 4
)

func newFuzzOrderBook() *OrderBook {
	return &OrderBook{
		Symbol:     "ABC/USD",
		Instrument: NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1),
		Bids:       newBook(Buy),
		Asks:       newBook(Sell),
	}
}

type fuzzGen struct {
	rng    *rand.Rand
	nextID uint64
	// priceSpan controls how many distinct prices the book uses. A narrow span
	// forces deep queues at one level (queue order, pro-rata denominators); a
	// wide span forces multi-level traversal and unreachable levels.
	priceSpan int
}

func (g *fuzzGen) price() int64 {
	return int64(fuzzMinPrice + g.rng.Intn(g.priceSpan))
}

func (g *fuzzGen) id() uint64 { g.nextID++; return g.nextID }

// randomResting builds a resting order with a reachable visibility state.
func (g *fuzzGen) randomResting(side etypes.Side) *etypes.Order {
	qty := int64(1 + g.rng.Intn(20))
	o := &etypes.Order{
		ID:          g.id(),
		ClientID:    uint64(1 + g.rng.Intn(fuzzClients)),
		Side:        side,
		Type:        etypes.LimitOrder,
		TimeInForce: etypes.GTC,
		Price:       g.price(),
		Qty:         qty,
		Status:      etypes.Open,
	}
	switch g.rng.Intn(6) {
	case 0, 1:
		o.Visibility = etypes.Hidden
	case 2, 3:
		o.Visibility = etypes.Iceberg
		o.IcebergQty = int64(1 + g.rng.Intn(int(qty)))
	default:
		o.Visibility = etypes.Normal
	}
	return o
}

// populate fills both sides, then warms the book up with real matcher passes so
// that partial fills, exhausted-and-refreshed iceberg tranches, and re-queued
// time priority are reachable states rather than hand-forged ones.
func (g *fuzzGen) populate(ob *OrderBook, matcher ematching.MatchingEngine) {
	for i, n := 0, g.rng.Intn(14); i < n; i++ {
		o := g.randomResting(etypes.Buy)
		ob.Bids.AddOrder(o)
	}
	for i, n := 0, g.rng.Intn(14); i < n; i++ {
		o := g.randomResting(etypes.Sell)
		ob.Asks.AddOrder(o)
	}
	for i, n := 0, g.rng.Intn(5); i < n; i++ {
		side := etypes.Buy
		if g.rng.Intn(2) == 0 {
			side = etypes.Sell
		}
		taker := &etypes.Order{
			ID:          g.id(),
			ClientID:    uint64(1 + g.rng.Intn(fuzzClients)),
			Side:        side,
			Type:        etypes.LimitOrder,
			TimeInForce: etypes.IOC,
			Price:       g.price(),
			Qty:         int64(1 + g.rng.Intn(25)),
			Status:      etypes.Open,
		}
		result := matcher.Match(ob.Bids, ob.Asks, taker)
		// Emulate the exchange's post-settlement cleanup: filled makers leave
		// the ID index once settled.
		for _, exec := range result.Executions {
			if maker := ob.FindOrder(exec.MakerOrderID); maker != nil && maker.FilledQty >= maker.Qty {
				ob.Bids.RemoveFilledOrder(exec.MakerOrderID)
				ob.Asks.RemoveFilledOrder(exec.MakerOrderID)
			}
			ematching.PutExecution(exec)
		}
	}
}

func (g *fuzzGen) randomIncoming() *etypes.Order {
	side := etypes.Buy
	if g.rng.Intn(2) == 0 {
		side = etypes.Sell
	}
	o := &etypes.Order{
		ID:       g.id(),
		ClientID: uint64(1 + g.rng.Intn(fuzzClients)),
		Side:     side,
		Qty:      int64(1 + g.rng.Intn(30)),
		Status:   etypes.Open,
	}
	if g.rng.Intn(5) == 0 {
		o.Type = etypes.Market
	} else {
		o.Type = etypes.LimitOrder
		o.Price = int64(fuzzMinPrice-2) + int64(g.rng.Intn(g.priceSpan+4))
	}
	switch g.rng.Intn(4) {
	case 0:
		o.TimeInForce = etypes.IOC
	case 1:
		o.TimeInForce = etypes.FOK
	default:
		o.TimeInForce = etypes.GTC
	}
	o.PostOnly = g.rng.Intn(8) == 0
	switch g.rng.Intn(8) {
	case 0:
		o.Visibility = etypes.Hidden
	case 1:
		o.Visibility = etypes.Iceberg
		o.IcebergQty = int64(1 + g.rng.Intn(int(o.Qty)))
	}
	return o
}

func (g *fuzzGen) randomExclusion(ob *OrderBook) map[uint64]struct{} {
	if g.rng.Intn(3) == 0 {
		return nil
	}
	excluded := make(map[uint64]struct{})
	for _, side := range []*Book{ob.Bids, ob.Asks} {
		for id, o := range side.Orders {
			if o.Parent == nil {
				continue
			}
			if g.rng.Intn(4) == 0 {
				excluded[id] = struct{}{}
			}
		}
	}
	if len(excluded) == 0 {
		return nil
	}
	return excluded
}

type matcherCase struct {
	name string
	make func(etypes.Clock) ematching.MatchingEngine
}

var fuzzMatchers = []matcherCase{
	{"pricetime", func(c etypes.Clock) ematching.MatchingEngine { return ematching.NewPriceTimeMatcher(c) }},
	{"prorata", func(c etypes.Clock) ematching.MatchingEngine { return ematching.NewProRataMatcher(c) }},
}

// TestPreviewMatchesCommittedMatching is the core differential: for each random
// scenario the preview result must equal what running the configured matcher
// against the live book actually produces, and the preview must leave the live
// book byte-identical.
func TestPreviewMatchesCommittedMatching(t *testing.T) {
	const iterations = 40000
	for _, mc := range fuzzMatchers {
		t.Run(mc.name, func(t *testing.T) {
			cov := coverage{}
			var crossing, previewRefused int
			for seed := 0; seed < iterations; seed++ {
				clock := &previewFuzzClock{now: 1_700_000_000_000_000_000}
				matcher := mc.make(clock)
				g := &fuzzGen{rng: rand.New(rand.NewSource(int64(seed)))}
				g.priceSpan = []int{1, 2, 3, fuzzMaxPrice - fuzzMinPrice + 1}[seed%4]
				ob := newFuzzOrderBook()
				g.populate(ob, matcher)
				incoming := g.randomIncoming()
				excluded := g.randomExclusion(ob)

				tallyScenario(cov, ob, incoming, excluded)
				// previewCannotCross reads the touch from ActiveHead while the
				// matcher starts from Best. The short-circuit is only sound if
				// those never disagree.
				for _, side := range []*Book{ob.Bids, ob.Asks} {
					if side.Best != side.ActiveHead {
						t.Fatalf("seed %d: book invariant broken, Best != ActiveHead", seed)
					}
					for l := side.ActiveHead; l != nil; l = l.Next {
						if l.OrderCnt == 0 {
							t.Fatalf("seed %d: empty level %d left linked in the book", seed, l.Price)
						}
					}
				}
				before := orderBookFacts(ob)
				ex := &DefaultExchange{Matcher: matcher}
				previewCopy := *incoming
				result, ok := ex.previewMatchExcluding(ob, &previewCopy, excluded)
				after := orderBookFacts(ob)
				if before != after {
					t.Fatalf("seed %d: preview MUTATED the live book\nbefore %s\nafter  %s", seed, before, after)
				}
				if previewCopy.FilledQty != incoming.FilledQty || previewCopy.Status != incoming.Status ||
					previewCopy.DisplayRemaining != incoming.DisplayRemaining || previewCopy.Parent != nil {
					t.Fatalf("seed %d: preview mutated the incoming order: %+v vs %+v", seed, previewCopy, *incoming)
				}

				var previewFacts []execFacts
				var previewFull bool
				if ok {
					previewFacts = factsOf(result.Executions)
					previewFull = result.FullyFilled
					releasePreviewExecutions(result.Executions)
				} else {
					previewRefused++
				}

				// Committed matching: the caller removes excluded makers before
				// committing, so that is the state the real match sees.
				for id := range excluded {
					ob.Bids.CancelOrder(id)
					ob.Asks.CancelOrder(id)
				}
				committedOrder := *incoming
				committed := matcher.Match(ob.Bids, ob.Asks, &committedOrder)
				committedFacts := factsOf(committed.Executions)
				committedFull := committed.FullyFilled
				for _, e := range committed.Executions {
					ematching.PutExecution(e)
				}
				if len(committedFacts) > 0 {
					crossing++
				}
				tallyOutcome(cov, committedFacts, committedFull, incoming.Qty)

				if !ok {
					if len(committedFacts) > 0 {
						t.Fatalf("seed %d: preview REFUSED but committed matching produced %d executions %+v",
							seed, len(committedFacts), committedFacts)
					}
					continue
				}
				if previewFull != committedFull {
					t.Fatalf("seed %d: FullyFilled differs preview=%v committed=%v (preview %+v, committed %+v)",
						seed, previewFull, committedFull, previewFacts, committedFacts)
				}
				if len(previewFacts) != len(committedFacts) {
					t.Fatalf("seed %d: execution count differs preview=%d committed=%d\npreview  %+v\ncommitted %+v\nbook %s\nincoming %+v excluded %v",
						seed, len(previewFacts), len(committedFacts), previewFacts, committedFacts, before, *incoming, excluded)
				}
				for i := range previewFacts {
					if previewFacts[i] != committedFacts[i] {
						t.Fatalf("seed %d: execution %d differs\npreview   %+v\ncommitted %+v\nbook %s\nincoming %+v excluded %v",
							seed, i, previewFacts[i], committedFacts[i], before, *incoming, excluded)
					}
				}
			}
			t.Logf("%s: %d iterations, %d produced executions, %d previews refused",
				mc.name, iterations, crossing, previewRefused)
			cov.report(t, mc.name)
		})
	}
}

// ---------------------------------------------------------------------------
// Boundary and refusal cases the random corpus cannot reach on its own.
// ---------------------------------------------------------------------------

func addOrFatal(t *testing.T, side *Book, o *etypes.Order) {
	t.Helper()
	if !side.AddOrder(o) {
		t.Fatalf("AddOrder(%d) refused", o.ID)
	}
}

// runDifferential is the same comparison the fuzz makes, for a hand-built case.
func runDifferential(t *testing.T, matcher ematching.MatchingEngine, ob *OrderBook, incoming *etypes.Order, excluded map[uint64]struct{}) (previewOK bool, previewFacts, committedFacts []execFacts) {
	t.Helper()
	before := orderBookFacts(ob)
	ex := &DefaultExchange{Matcher: matcher}
	previewCopy := *incoming
	result, ok := ex.previewMatchExcluding(ob, &previewCopy, excluded)
	if after := orderBookFacts(ob); after != before {
		t.Fatalf("preview mutated the live book\nbefore %s\nafter  %s", before, after)
	}
	var previewFull bool
	if ok {
		previewFacts = factsOf(result.Executions)
		previewFull = result.FullyFilled
		releasePreviewExecutions(result.Executions)
	}
	for id := range excluded {
		ob.Bids.CancelOrder(id)
		ob.Asks.CancelOrder(id)
	}
	committedOrder := *incoming
	committed := matcher.Match(ob.Bids, ob.Asks, &committedOrder)
	committedFacts = factsOf(committed.Executions)
	committedFull := committed.FullyFilled
	for _, e := range committed.Executions {
		ematching.PutExecution(e)
	}
	if ok {
		if previewFull != committedFull {
			t.Fatalf("FullyFilled differs: preview=%v committed=%v", previewFull, committedFull)
		}
		if len(previewFacts) != len(committedFacts) {
			t.Fatalf("execution count differs: preview=%d committed=%d\npreview %+v\ncommitted %+v",
				len(previewFacts), len(committedFacts), previewFacts, committedFacts)
		}
		for i := range previewFacts {
			if previewFacts[i] != committedFacts[i] {
				t.Fatalf("execution %d differs\npreview   %+v\ncommitted %+v", i, previewFacts[i], committedFacts[i])
			}
		}
	}
	return ok, previewFacts, committedFacts
}

// TestPreviewBoundaryQuantities covers quantities the random corpus never draws:
// zero, one, and the representable maximum.
func TestPreviewBoundaryQuantities(t *testing.T) {
	cases := []struct {
		name       string
		restingQty int64
		incomeQty  int64
		incomeType etypes.OrderType
	}{
		{"zero-qty incoming limit", 10, 0, etypes.LimitOrder},
		{"zero-qty incoming market", 10, 0, etypes.Market},
		{"unit qty", 1, 1, etypes.LimitOrder},
		{"maxint resting, unit taker", math.MaxInt64, 1, etypes.LimitOrder},
		{"maxint taker, unit resting", 1, math.MaxInt64, etypes.LimitOrder},
		{"maxint both", math.MaxInt64, math.MaxInt64, etypes.Market},
	}
	for _, mc := range fuzzMatchers {
		for _, tc := range cases {
			t.Run(mc.name+"/"+tc.name, func(t *testing.T) {
				matcher := mc.make(&previewFuzzClock{now: 1})
				ob := newFuzzOrderBook()
				addOrFatal(t, ob.Asks, &etypes.Order{ID: 1, ClientID: 2, Side: Sell,
					Type: etypes.LimitOrder, TimeInForce: etypes.GTC, Price: 100, Qty: tc.restingQty})
				incoming := &etypes.Order{ID: 99, ClientID: 1, Side: Buy, Type: tc.incomeType,
					TimeInForce: etypes.GTC, Price: 100, Qty: tc.incomeQty}
				ok, _, _ := runDifferential(t, matcher, ob, incoming, nil)
				if !ok {
					t.Logf("preview refused (safe direction: caller rejects the order)")
				}
			})
		}
	}
}

// TestPreviewRefusesOnlyOnACorruptBook pins the one reachable refusal path and
// its direction. marketDepthSaneExcluding is a preview-only gate with no
// counterpart in committed matching, so if it ever fired on a well-formed book
// the preview would reject an order the matcher would have filled. It cannot:
// LinkOrder already enforces a representable level aggregate on insertion, and
// the gate sums a subset of it.
func TestPreviewRefusesOnlyOnACorruptBook(t *testing.T) {
	matcher := ematching.NewPriceTimeMatcher(&previewFuzzClock{now: 1})
	ob := newFuzzOrderBook()
	resting := &etypes.Order{ID: 1, ClientID: 2, Side: Sell,
		Type: etypes.LimitOrder, TimeInForce: etypes.GTC, Price: 100, Qty: 10}
	addOrFatal(t, ob.Asks, resting)
	// Corrupt the book the way only a matcher bug could: overfilled resting order.
	resting.FilledQty = 11

	incoming := &etypes.Order{ID: 99, ClientID: 1, Side: Buy, Type: etypes.LimitOrder,
		TimeInForce: etypes.GTC, Price: 100, Qty: 5}
	ex := &DefaultExchange{Matcher: matcher}
	if _, ok := ex.previewMatchExcluding(ob, incoming, nil); ok {
		t.Fatal("preview accepted a corrupt book; the depth-sanity gate is not doing its job")
	}
}

// TestPreviewRequiresAnUnfilledIncomingOrder pins an undocumented precondition
// found by this audit. previewMatchExcluding verifies its own result with
//
//	if result.FullyFilled != (filled == order.Qty)
//
// where `filled` is the sum of THIS preview's execution quantities. That
// identity only holds when the incoming order arrives unfilled. Hand a preview
// an order with FilledQty > 0 and the matcher correctly fills the residual and
// reports FullyFilled, but the self-check compares the residual against the
// order's total quantity, disagrees with itself, and refuses.
//
// Every production caller builds a fresh order, so this is latent, and the
// direction is safe (a refusal becomes an order rejection, never a phantom
// fill). It is pinned here so a future caller that previews a partially filled
// order — an amend, a re-preview after a partial IOC — fails loudly.
func TestPreviewRequiresAnUnfilledIncomingOrder(t *testing.T) {
	for _, mc := range fuzzMatchers {
		t.Run(mc.name, func(t *testing.T) {
			matcher := mc.make(&previewFuzzClock{now: 1})
			ob := newFuzzOrderBook()
			addOrFatal(t, ob.Asks, &etypes.Order{ID: 1, ClientID: 2, Side: Sell,
				Type: etypes.LimitOrder, TimeInForce: etypes.GTC, Price: 100, Qty: 10})
			incoming := &etypes.Order{ID: 99, ClientID: 1, Side: Buy, Type: etypes.LimitOrder,
				TimeInForce: etypes.GTC, Price: 100, Qty: 10, FilledQty: 7}

			ex := &DefaultExchange{Matcher: matcher}
			previewCopy := *incoming
			if _, ok := ex.previewMatchExcluding(ob, &previewCopy, nil); ok {
				t.Fatal("preview now accepts a partially filled incoming order; " +
					"if that is intentional the self-check must compare against the residual, " +
					"and this test should assert the executions instead")
			}

			// Committed matching has no such restriction: it fills the residual.
			committed := *incoming
			result := matcher.Match(ob.Bids, ob.Asks, &committed)
			if len(result.Executions) != 1 || result.Executions[0].Qty != 3 || !result.FullyFilled {
				t.Fatalf("committed matching changed: %d executions, fullyFilled=%v",
					len(result.Executions), result.FullyFilled)
			}
			for _, e := range result.Executions {
				ematching.PutExecution(e)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// End-to-end: the preview's decision versus what placing the order really does.
// ---------------------------------------------------------------------------

// TestFOKAdmissionMatchesRealExecution drives the whole PlaceOrder path rather
// than the matcher boundary. FOK is the preview's load-bearing consumer: the
// preview decides, before any book mutation, whether the order can fill
// completely. If the preview and committed matching ever disagreed, an accepted
// FOK would leave a residual resting on the book (a partially filled
// fill-or-kill), or a fillable FOK would be rejected while the book was left
// untouched. Both are checked directly.
func TestFOKAdmissionMatchesRealExecution(t *testing.T) {
	const iterations = 3000
	for _, mc := range fuzzMatchers {
		t.Run(mc.name, func(t *testing.T) {
			var accepted, killed int
			reasons := map[RejectReason]int{}
			for seed := 0; seed < iterations; seed++ {
				rng := rand.New(rand.NewSource(int64(seed)))
				clock := &previewFuzzClock{now: 1_700_000_000_000_000_000}
				ex := NewExchange(4, clock)
				ex.Matcher = mc.make(clock)
				ex.AddInstrument(NewSpotInstrument("X/USD", "X", "USD", 1, 1, 1, 1))
				for c := uint64(1); c <= 4; c++ {
					ex.ConnectNewClient(c, map[string]int64{"USD": 1 << 40, "X": 1 << 40}, &FixedFee{})
				}

				// Seed a book of resting quotes from clients 2..4 only, so the
				// taker below never meets its own liquidity unless we want it to.
				for i, n := 0, 1+rng.Intn(10); i < n; i++ {
					side := Buy
					if rng.Intn(2) == 0 {
						side = Sell
					}
					req := &OrderRequest{
						RequestID: uint64(1000 + i), Symbol: "X/USD", Side: side, Type: LimitOrder,
						Price: int64(95 + rng.Intn(3)), Qty: int64(1 + rng.Intn(15)),
						TimeInForce: GTC, Visibility: Normal,
					}
					if rng.Intn(4) == 0 {
						req.Visibility = Iceberg
						req.IcebergQty = int64(1 + rng.Intn(int(req.Qty)))
					} else if rng.Intn(4) == 0 {
						req.Visibility = Hidden
					}
					ex.PlaceOrder(uint64(2+rng.Intn(3)), req)
				}

				book := ex.Books["X/USD"]
				takerSide := Buy
				if rng.Intn(2) == 0 {
					takerSide = Sell
				}
				fokQty := int64(1 + rng.Intn(25))
				fok := &OrderRequest{
					RequestID: 9999, Symbol: "X/USD", Side: takerSide, Type: LimitOrder,
					Price: int64(94 + rng.Intn(5)), Qty: fokQty, TimeInForce: FOK, Visibility: Normal,
				}
				takerClient := uint64(1)
				if rng.Intn(3) == 0 {
					// Sometimes let the taker own resting liquidity, exercising
					// self-trade prevention inside the FOK decision.
					takerClient = uint64(2 + rng.Intn(3))
				}

				before := orderBookFacts(book)
				resp := ex.PlaceOrder(takerClient, fok)
				after := orderBookFacts(book)

				if resp.Success {
					accepted++
					orderID, ok := resp.Data.(uint64)
					if !ok {
						t.Fatalf("seed %d: accepted FOK returned %T, want the order ID", seed, resp.Data)
					}
					if resting := book.FindOrder(orderID); resting != nil && resting.Parent != nil {
						t.Fatalf("seed %d: accepted FOK left %d of %d resting on the book — "+
							"the preview said it would fill completely and it did not",
							seed, resting.Qty-resting.FilledQty, resting.Qty)
					}
				} else {
					killed++
					reasons[resp.Error]++
					if resp.Error != RejectFOKNotFilled {
						// Other rejections are out of scope here; only require
						// that they too left the book alone.
						if before != after {
							t.Fatalf("seed %d: rejected FOK (%s) mutated the book", seed, resp.Error)
						}
						ex.Shutdown()
						continue
					}
					if before != after {
						t.Fatalf("seed %d: killed FOK mutated the book\nbefore %s\nafter  %s", seed, before, after)
					}
				}
				ex.Shutdown()
			}
			t.Logf("%s: %d FOK orders, %d filled completely, %d killed %v", mc.name, iterations, accepted, killed, reasons)
		})
	}
}

// TestPreviewCloneNormalisesAnExhaustedIcebergTranche records that the detached
// copy is not a faithful copy of one field. cloneBookForPreviewExcluding routes
// every order through Book.AddOrder, and LinkOrder grants a fresh display
// tranche to any iceberg it is handed with DisplayRemaining == 0. A live
// resting iceberg therefore cannot be copied in an exhausted state: the clone
// silently repairs it.
//
// On a well-formed book this is invisible, because DisplayRemaining > 0 is an
// invariant of every resting iceberg (LinkOrder establishes it on insertion and
// refreshIcebergTranche restores it the moment a tranche is consumed). It is
// pinned because the asymmetry is real and points the wrong way: on a book that
// HAS broken that invariant, the preview reports a clean fill while committed
// matching panics on the same state. The clone hides a corruption signal rather
// than propagating it.
func TestPreviewCloneNormalisesAnExhaustedIcebergTranche(t *testing.T) {
	matcher := ematching.NewPriceTimeMatcher(&previewFuzzClock{now: 1})
	ob := newFuzzOrderBook()
	ice := &etypes.Order{ID: 1, ClientID: 2, Side: Sell, Type: etypes.LimitOrder,
		TimeInForce: etypes.GTC, Price: 100, Qty: 10, Visibility: etypes.Iceberg, IcebergQty: 4}
	addOrFatal(t, ob.Asks, ice)
	if ice.DisplayRemaining != 4 {
		t.Fatalf("LinkOrder set DisplayRemaining=%d, want 4", ice.DisplayRemaining)
	}
	// Forge the state the invariant forbids.
	ice.DisplayRemaining = 0

	incoming := &etypes.Order{ID: 99, ClientID: 1, Side: Buy, Type: etypes.LimitOrder,
		TimeInForce: etypes.GTC, Price: 100, Qty: 5}
	ex := &DefaultExchange{Matcher: matcher}
	previewCopy := *incoming
	result, ok := ex.previewMatchExcluding(ob, &previewCopy, nil)
	if !ok {
		t.Fatal("preview refused; if the clone now rejects an exhausted tranche instead of " +
			"repairing it, that is an improvement and this test should assert the refusal")
	}
	if !result.FullyFilled {
		t.Fatalf("preview did not report a full fill: %+v", result)
	}
	releasePreviewExecutions(result.Executions)

	panicked := func() (p bool) {
		defer func() { p = recover() != nil }()
		committed := *incoming
		matcher.Match(ob.Bids, ob.Asks, &committed)
		return false
	}()
	if !panicked {
		t.Fatal("committed matching no longer rejects an exhausted tranche; the two paths " +
			"now agree and this test should be rewritten as a positive assertion")
	}
}
