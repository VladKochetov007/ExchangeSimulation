package exchange_test

// Randomized operation-sequence invariant fuzzer. Drives a real exchange
// single-threaded (direct PlaceOrder/CancelOrder calls, no gateways) with a
// seeded op stream and asserts global invariants after every step:
//
//	I1 per-asset conservation: Σ spot + Σ perp + fee revenue + insurance = initial
//	I2 Σ position sizes = 0 per symbol
//	I3 no negative reserved; no negative spot balances
//	I4 book integrity: sorted levels, Best == ActiveHead, TotalQty == Σ remainders,
//	   Orders index ↔ level list consistent
//	I5 margin ledger: Margin ≥ 0; flat position ⇒ Margin == 0
//
// Deterministic per seed: failures reproduce with the printed seed and step.
// FUZZ_STEPS overrides the per-run step count for deep background runs.

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"

	ebook "exchange_sim/book"
	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
)

const (
	fuzzSpotSym = "ABC/USD"
	fuzzPerpSym = "ABC-PERP"
)

type fuzzConfig struct {
	name         string
	seeds        []int64
	steps        int
	feePlan      func() FeeModel
	useIceberg   bool
	useTIF       bool // mix in IOC/FOK
	useHedge     bool
	boundaryQty  bool // qty=1 / min-order edges
	proRata      bool
	perpEnabled  bool
	liquidations bool
	thinMargin   bool // underfunded perp wallets → liquidation cascades
}

type fuzzHarness struct {
	t                *testing.T
	ex               *Exchange
	rng              *rand.Rand
	cfg              fuzzConfig
	clients          []uint64
	perp             *einstrument.PerpFutures
	initial          map[string]int64 // per-asset system totals
	reqSeq           uint64
	fills            int64 // perp entry-averaging events, drives dust tolerance
	dustTolerance    int64
	stats            map[string]int64
	initialInsurance int64
}

// fuzzLiqCounter records liquidation machinery activity for the depth check.
type fuzzLiqCounter struct{ h *fuzzHarness }

func (c *fuzzLiqCounter) OnMarginCall(*MarginCallEvent)       { c.h.stats["margin_calls"]++ }
func (c *fuzzLiqCounter) OnLiquidation(*LiquidationEvent)     { c.h.stats["liquidations"]++ }
func (c *fuzzLiqCounter) OnInsuranceFund(*InsuranceFundEvent) { c.h.stats["insurance_events"]++ }

func fuzzSteps(def int) int {
	if v := os.Getenv("FUZZ_STEPS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func newFuzzHarness(t *testing.T, cfg fuzzConfig, seed int64) *fuzzHarness {
	return newFuzzHarnessWithClock(t, cfg, seed, &RealClock{})
}

func newFuzzHarnessWithClock(t *testing.T, cfg fuzzConfig, seed int64, clock Clock) *fuzzHarness {
	ex := NewExchangeWithConfig(ExchangeConfig{EstimatedClients: 8, Clock: clock})
	if cfg.proRata {
		ex.Matcher = NewProRataMatcher()
	}

	spot := NewSpotInstrument(fuzzSpotSym, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(spot)

	h := &fuzzHarness{
		t: t, ex: ex, rng: rand.New(rand.NewSource(seed)), cfg: cfg,
		initial: make(map[string]int64),
		stats:   make(map[string]int64),
	}

	if cfg.perpEnabled {
		h.perp = NewPerpFutures(fuzzPerpSym, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
		ex.AddInstrument(h.perp)
	}
	ex.LiquidationHandler = &fuzzLiqCounter{h: h}

	perpFunds := USDAmount(500_000)
	if cfg.thinMargin {
		// Just enough for a handful of levered positions: forces the
		// liquidation/insurance machinery to actually run.
		perpFunds = USDAmount(40)
	}
	for id := uint64(1); id <= 4; id++ {
		client := NewClient(id, cfg.feePlan())
		client.Balances["ABC"] = 1_000 * BTC_PRECISION
		client.Balances["USD"] = USDAmount(1_000_000)
		client.PerpBalances["USD"] = perpFunds
		ex.Clients[id] = client
		h.clients = append(h.clients, id)
	}
	if cfg.thinMargin {
		ex.ExchangeBalance.InsuranceFund["USD"] = USDAmount(100_000)
		h.initialInsurance = ex.ExchangeBalance.InsuranceFund["USD"]
	}

	h.initial["ABC"] = h.systemTotal("ABC")
	h.initial["USD"] = h.systemTotal("USD")
	return h
}

func (h *fuzzHarness) systemTotal(asset string) int64 {
	var total int64
	for _, c := range h.ex.Clients {
		total += c.Balances[asset] + c.PerpBalances[asset]
	}
	total += h.ex.ExchangeBalance.FeeRevenue[asset]
	total += h.ex.ExchangeBalance.InsuranceFund[asset]
	return total
}

// openPositionCostBasis returns Σ size×entry/precision over open perp-core
// positions (perps and dated futures) — the quote-asset value still "inside"
// positions. Option positions are excluded: their premium moved as zero-sum
// cash at trade time, so they carry no unrealized mint.
func (h *fuzzHarness) openPositionCostBasis() int64 {
	pm := h.ex.Positions.(*PositionManager)
	var basis int64
	for _, id := range h.clients {
		for _, pos := range pm.GetPositions(id) {
			if inst := h.ex.Instruments[pos.Symbol]; inst != nil && inst.InstrumentType() == "OPTION" {
				continue
			}
			basis += MulDiv(pos.Size, pos.EntryPrice, BTC_PRECISION)
		}
	}
	return basis
}

func (h *fuzzHarness) nextReqID() uint64 {
	h.reqSeq++
	return h.reqSeq
}

// randPrice returns a tick-aligned price around $100 (±30 ticks).
func (h *fuzzHarness) randPrice() int64 {
	base := int64(100)
	offset := h.rng.Int63n(61) - 30
	price := (base + offset) * DOLLAR_TICK
	if price < DOLLAR_TICK {
		price = DOLLAR_TICK
	}
	return price
}

func (h *fuzzHarness) randQty() int64 {
	if h.cfg.boundaryQty && h.rng.Intn(3) == 0 {
		// Boundary quantities: single unit, min order, off-by-one around a lot.
		switch h.rng.Intn(3) {
		case 0:
			return 1
		case 1:
			return BTC_PRECISION - 1
		default:
			return BTC_PRECISION + 1
		}
	}
	return (1 + h.rng.Int63n(3)) * BTC_PRECISION / 2
}

func (h *fuzzHarness) randSymbol() string {
	if h.cfg.perpEnabled && h.rng.Intn(2) == 0 {
		return fuzzPerpSym
	}
	return fuzzSpotSym
}

func (h *fuzzHarness) placeOrder() {
	clientID := h.clients[h.rng.Intn(len(h.clients))]
	sym := h.randSymbol()
	side := Buy
	if h.rng.Intn(2) == 0 {
		side = Sell
	}
	req := &OrderRequest{
		RequestID:   h.nextReqID(),
		Side:        side,
		Type:        LimitOrder,
		Price:       h.randPrice(),
		Qty:         h.randQty(),
		Symbol:      sym,
		TimeInForce: GTC,
	}
	if h.rng.Intn(5) == 0 {
		req.Type = Market
		req.Price = 0
	}
	if h.cfg.useTIF && req.Type == LimitOrder {
		switch h.rng.Intn(4) {
		case 0:
			req.TimeInForce = IOC
		case 1:
			req.TimeInForce = FOK
		}
	}
	if h.cfg.useIceberg && req.Type == LimitOrder && h.rng.Intn(4) == 0 {
		if h.rng.Intn(2) == 0 {
			req.Visibility = Iceberg
			req.IcebergQty = req.Qty / 4
			if req.IcebergQty == 0 {
				req.IcebergQty = 1
			}
		} else {
			req.Visibility = Hidden
		}
	}
	if h.cfg.useHedge && sym == fuzzPerpSym && h.rng.Intn(3) == 0 {
		if h.rng.Intn(2) == 0 {
			req.PositionSide = PositionLong
		} else {
			req.PositionSide = PositionShort
		}
	}
	resp := h.ex.PlaceOrder(clientID, req)
	if resp.Success {
		h.stats["accepted"]++
	} else {
		h.stats["reject:"+string(resp.Error)]++
	}
}

func (h *fuzzHarness) cancelRandom() {
	clientID := h.clients[h.rng.Intn(len(h.clients))]
	client := h.ex.Clients[clientID]
	if len(client.OrderIDs) == 0 {
		return
	}
	orderID := client.OrderIDs[h.rng.Intn(len(client.OrderIDs))]
	h.ex.CancelOrder(clientID, &CancelRequest{RequestID: h.nextReqID(), OrderID: orderID})
}

func (h *fuzzHarness) transferRandom() {
	clientID := h.clients[h.rng.Intn(len(h.clients))]
	amount := USDAmount(float64(1 + h.rng.Intn(1000)))
	if h.rng.Intn(2) == 0 {
		h.ex.Transfer(clientID, "spot", "perp", "USD", amount)
	} else {
		h.ex.Transfer(clientID, "perp", "spot", "USD", amount)
	}
}

func (h *fuzzHarness) settleFunding() {
	if h.perp == nil {
		return
	}
	mark := h.randPrice()
	h.perp.UpdateFundingRate(mark, mark+h.rng.Int63n(3)*DOLLAR_TICK-DOLLAR_TICK)
	h.ex.SettleFunding(h.perp)
}

func (h *fuzzHarness) sweepLiquidations() {
	if h.perp == nil || !h.cfg.liquidations {
		return
	}
	h.ex.CheckLiquidations(fuzzPerpSym, h.perp, h.randPrice())
}

func (h *fuzzHarness) step(n int) {
	// Entry averaging truncates < 1 price unit per event; scaled by max qty
	// (~2.5 base units) that is ≤ 3 quote units of dust per step.
	h.dustTolerance += 3
	switch r := h.rng.Intn(100); {
	case r < 55:
		h.placeOrder()
	case r < 75:
		h.cancelRandom()
	case r < 85:
		h.transferRandom()
	case r < 90:
		h.settleFunding()
	default:
		h.sweepLiquidations()
	}
	h.checkInvariants(n)
}

// --- invariant checks ---

func (h *fuzzHarness) fail(step int, format string, args ...any) {
	h.t.Helper()
	h.t.Fatalf("[step %d] %s", step, fmt.Sprintf(format, args...))
}

func (h *fuzzHarness) checkInvariants(step int) {
	h.t.Helper()

	// I1 conservation per asset. Perp realized PnL legitimately mints/destroys
	// cash mid-stream against the cost basis carried in open positions: total
	// wealth at any mark M is Σcash + Σ size×(M−entry)/prec, and the mark term
	// vanishes because positions are zero-sum — so the conserved quantity for
	// the quote asset is Σcash − Σ size×entry/precision. Entry-price averaging
	// truncates through float64, so a small dust tolerance accrues per fill.
	for _, asset := range []string{"ABC", "USD"} {
		got := h.systemTotal(asset)
		if asset == "USD" && h.cfg.perpEnabled {
			got -= h.openPositionCostBasis()
		}
		delta := got - h.initial[asset]
		if delta < 0 {
			delta = -delta
		}
		if delta > h.dustTolerance {
			h.fail(step, "I1 conservation violated for %s: delta=%d (tolerance %d)", asset, got-h.initial[asset], h.dustTolerance)
		}
	}

	// I2 positions zero-sum + I5 margin ledger.
	if h.cfg.perpEnabled {
		pm := h.ex.Positions.(*PositionManager)
		var sum int64
		for _, id := range h.clients {
			for _, pos := range pm.GetPositions(id) {
				sum += pos.Size
				if pos.Margin < 0 {
					h.fail(step, "I5 negative margin: client %d %s margin=%d", id, pos.Symbol, pos.Margin)
				}
				if pos.Size == 0 && pos.Margin != 0 {
					h.fail(step, "I5 flat position holds margin: client %d %s margin=%d", id, pos.Symbol, pos.Margin)
				}
			}
		}
		if sum != 0 {
			h.fail(step, "I2 position sum nonzero: %d", sum)
		}
	}

	// I3 reserved and balance floors.
	for _, id := range h.clients {
		c := h.ex.Clients[id]
		for asset, v := range c.Reserved {
			if v < 0 {
				h.fail(step, "I3 negative spot reserved: client %d %s %d", id, asset, v)
			}
		}
		for asset, v := range c.PerpReserved {
			if v < 0 {
				h.fail(step, "I3 negative perp reserved: client %d %s %d", id, asset, v)
			}
		}
		for asset, v := range c.Balances {
			if v < 0 {
				h.fail(step, "I3 negative spot balance: client %d %s %d", id, asset, v)
			}
		}
	}

	// I4 book integrity.
	for sym, book := range h.ex.Books {
		h.checkBookSide(step, sym, book.Bids, true)
		h.checkBookSide(step, sym, book.Asks, false)
	}
}

func (h *fuzzHarness) checkBookSide(step int, sym string, side *ebook.Book, isBid bool) {
	h.t.Helper()
	if side.Best != side.ActiveHead {
		h.fail(step, "I4 %s: Best != ActiveHead", sym)
	}
	linked := make(map[uint64]bool)
	var prevPrice int64
	first := true
	for l := side.ActiveHead; l != nil; l = l.Next {
		if !first {
			if isBid && l.Price >= prevPrice {
				h.fail(step, "I4 %s bids unsorted: %d then %d", sym, prevPrice, l.Price)
			}
			if !isBid && l.Price <= prevPrice {
				h.fail(step, "I4 %s asks unsorted: %d then %d", sym, prevPrice, l.Price)
			}
		}
		prevPrice = l.Price
		first = false

		var qty int64
		cnt := 0
		for o := l.Head; o != nil; o = o.Next {
			qty += o.Qty - o.FilledQty
			cnt++
			linked[o.ID] = true
			if side.Orders[o.ID] == nil {
				h.fail(step, "I4 %s: linked order %d missing from index", sym, o.ID)
			}
		}
		if qty != l.TotalQty {
			h.fail(step, "I4 %s level %d: TotalQty=%d but Σremainders=%d", sym, l.Price, l.TotalQty, qty)
		}
		if int32(cnt) != l.OrderCnt {
			h.fail(step, "I4 %s level %d: OrderCnt=%d but counted=%d", sym, l.Price, l.OrderCnt, cnt)
		}
	}
	for id, o := range side.Orders {
		if !linked[id] {
			h.fail(step, "I4 %s: indexed order %d (status %v, parent %v) not linked to any level",
				sym, id, o.Status, o.Parent != nil)
		}
	}
}

func (h *fuzzHarness) run() {
	steps := fuzzSteps(h.cfg.steps)
	for n := 0; n < steps; n++ {
		h.step(n)
	}
	// Depth check: a fuzzer that mostly rejects exercises nothing.
	for sym, book := range h.ex.Books {
		h.stats["trades:"+sym] = int64(book.SeqNum)
	}
	if testing.Verbose() {
		h.t.Logf("stats: %v", h.stats)
	}
	if h.stats["accepted"] == 0 {
		h.t.Fatal("fuzzer depth failure: zero accepted orders")
	}
}

// --- experiment configs (gen-0 matrix from PLAN.md) ---

func percentageQuoteFees() FeeModel { return &PercentageFee{MakerBps: 2, TakerBps: 8, InQuote: true} }
func percentageBaseFees() FeeModel  { return &PercentageFee{MakerBps: 2, TakerBps: 8, InQuote: false} }
func rebateFees() FeeModel          { return &PercentageFee{MakerBps: -3, TakerBps: 8, InQuote: true} }

func runFuzz(t *testing.T, cfg fuzzConfig) {
	for _, seed := range cfg.seeds {
		seed := seed
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			h := newFuzzHarness(t, cfg, seed)
			h.run()
		})
	}
}

func TestFuzzSpotPerpCore(t *testing.T) {
	runFuzz(t, fuzzConfig{
		name: "core", seeds: []int64{1, 2, 3}, steps: 20_000,
		feePlan: percentageQuoteFees, perpEnabled: true, liquidations: true,
	})
}

func TestFuzzLifecycleMixes(t *testing.T) {
	runFuzz(t, fuzzConfig{
		name: "lifecycle", seeds: []int64{11, 12, 13}, steps: 20_000,
		feePlan: percentageQuoteFees, useIceberg: true, useTIF: true,
		perpEnabled: true, liquidations: true,
	})
}

func TestFuzzBoundaryValues(t *testing.T) {
	runFuzz(t, fuzzConfig{
		name: "boundary", seeds: []int64{21, 22, 23}, steps: 20_000,
		feePlan: percentageBaseFees, boundaryQty: true, useTIF: true,
		perpEnabled: true, liquidations: true,
	})
}

func TestFuzzRebateEconomics(t *testing.T) {
	runFuzz(t, fuzzConfig{
		name: "rebates", seeds: []int64{31, 32}, steps: 20_000,
		feePlan: rebateFees, useIceberg: true, perpEnabled: true, liquidations: true,
	})
}

func TestFuzzProRataDifferential(t *testing.T) {
	runFuzz(t, fuzzConfig{
		name: "prorata", seeds: []int64{41, 42, 43}, steps: 20_000,
		feePlan: percentageQuoteFees, proRata: true, useIceberg: true, useTIF: true,
		perpEnabled: true, liquidations: true,
	})
}

func TestFuzzHedgeMode(t *testing.T) {
	runFuzz(t, fuzzConfig{
		name: "hedge", seeds: []int64{51, 52, 53}, steps: 20_000,
		feePlan: percentageQuoteFees, useHedge: true, perpEnabled: true, liquidations: true,
	})
}

func TestFuzzLiquidationCascade(t *testing.T) {
	runFuzz(t, fuzzConfig{
		name: "cascade", seeds: []int64{61, 62, 63}, steps: 20_000,
		feePlan: percentageQuoteFees, thinMargin: true, useTIF: true,
		perpEnabled: true, liquidations: true,
	})
}
