package exchange_test

// Derivatives-lifecycle extension of the invariant fuzzer: dated futures and
// European options trade under a controllable clock and roll through
// listing → trading → mark updates → expiry settlement → relisting, with the
// same global invariants checked every step. Options exchange premium cash
// zero-sum per trade, so their positions carry no unrealized mint — the cost
// basis correction applies to perp-core positions (futures) only.

import (
	"fmt"
	"testing"
	"time"

	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
)

type derivFuzzHarness struct {
	*fuzzHarness
	clock     *testClock
	expirySeq int
	futSyms   []string
	optSyms   []string
	tradeSeqs map[string]int64
}

func newDerivFuzzHarness(t *testing.T, seed int64) *derivFuzzHarness {
	clock := &testClock{now: 1_000_000_000}
	cfg := fuzzConfig{
		name: "deriv", steps: 20_000,
		feePlan: percentageQuoteFees, perpEnabled: true, liquidations: true,
	}
	base := newFuzzHarnessWithClock(t, cfg, seed, clock)
	h := &derivFuzzHarness{fuzzHarness: base, clock: clock, tradeSeqs: make(map[string]int64)}
	h.listContracts()
	return h
}

// listContracts lists one dated future and one call/put pair expiring 60
// simulated seconds out.
func (h *derivFuzzHarness) listContracts() {
	h.expirySeq++
	expiry := h.clock.now + 60*int64(time.Second)

	futSym := fmt.Sprintf("ABC-FUT-%d", h.expirySeq)
	fut := einstrument.NewExpiringFutures(futSym, "ABC", "USD",
		BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, expiry)
	fut.Underlying = fuzzSpotSym
	fut.DeliveryFeeBps = 5
	fut.SetObservationWindow(10 * int64(time.Second))
	h.ex.AddInstrument(fut)
	h.futSyms = append(h.futSyms, futSym)

	strike := int64(100) * DOLLAR_TICK
	for _, isCall := range []bool{true, false} {
		cp := "P"
		if isCall {
			cp = "C"
		}
		sym := fmt.Sprintf("ABC-OPT-%d-%s", h.expirySeq, cp)
		opt := einstrument.NewEuropeanOption(sym, "ABC", "USD", fuzzSpotSym,
			BTC_PRECISION, USD_PRECISION, CENT_TICK, 1, strike, expiry, isCall)
		opt.DeliveryFeeBps = 3
		opt.SetObservationWindow(10 * int64(time.Second))
		h.ex.AddInstrument(opt)
		h.optSyms = append(h.optSyms, sym)
	}
}

// pruneExpired drops symbols the exchange has delisted.
func (h *derivFuzzHarness) pruneExpired() {
	keep := func(syms []string) []string {
		out := syms[:0]
		for _, s := range syms {
			if h.ex.GetBook(s) != nil {
				out = append(out, s)
			}
		}
		return out
	}
	h.futSyms = keep(h.futSyms)
	h.optSyms = keep(h.optSyms)
}

func (h *derivFuzzHarness) placeDerivOrder() {
	pool := append(append([]string{}, h.futSyms...), h.optSyms...)
	if len(pool) == 0 {
		return
	}
	sym := pool[h.rng.Intn(len(pool))]
	clientID := h.clients[h.rng.Intn(len(h.clients))]
	side := Buy
	if h.rng.Intn(2) == 0 {
		side = Sell
	}
	isOption := h.ex.GetBook(sym) != nil && h.ex.GetBook(sym).Instrument.InstrumentType() == "OPTION"
	price := h.randPrice()
	if isOption {
		// Premium-scale prices on the cent grid.
		price = (1 + h.rng.Int63n(500)) * CENT_TICK
	}
	req := &OrderRequest{
		RequestID: h.nextReqID(), Side: side, Type: LimitOrder,
		Price: price, Qty: BTC_PRECISION / 2, Symbol: sym, TimeInForce: GTC,
	}
	if h.rng.Intn(6) == 0 {
		req.Type = Market
		req.Price = 0
	}
	resp := h.ex.PlaceOrder(clientID, req)
	if resp.Success {
		h.stats["deriv_accepted"]++
	} else {
		h.stats["deriv_reject:"+string(resp.Error)]++
	}
}

func (h *derivFuzzHarness) advanceTime() {
	h.clock.now += (1 + h.rng.Int63n(5)) * int64(time.Second)
	h.ex.UpdateDerivativeMarks()
	// Trades on books about to be delisted must be banked before CheckExpiries
	// deletes them, or the depth counter undercounts.
	before := len(h.ex.Books)
	for _, sym := range append(append([]string{}, h.futSyms...), h.optSyms...) {
		if book := h.ex.GetBook(sym); book != nil {
			h.tradeSeqs[sym] = int64(book.SeqNum)
		}
	}
	h.ex.CheckExpiries()
	if len(h.ex.Books) != before {
		h.stats["expiries"]++
		for sym, seq := range h.tradeSeqs {
			if h.ex.GetBook(sym) == nil {
				h.stats["deriv_trades"] += seq
				delete(h.tradeSeqs, sym)
			}
		}
		h.pruneExpired()
		h.listContracts()
	}
}

func (h *derivFuzzHarness) step(n int) {
	h.dustTolerance += 3
	switch r := h.rng.Intn(100); {
	case r < 30:
		h.placeOrder() // spot+perp flow keeps the underlying book alive
	case r < 60:
		h.placeDerivOrder()
	case r < 75:
		h.cancelRandom()
	case r < 85:
		h.advanceTime()
	case r < 92:
		h.transferRandom()
	default:
		h.settleFunding()
	}
	h.checkInvariants(n)
}

func (h *derivFuzzHarness) run() {
	steps := fuzzSteps(h.cfg.steps)
	for n := 0; n < steps; n++ {
		h.step(n)
	}
	if testing.Verbose() {
		h.t.Logf("stats: %v", h.stats)
	}
	if h.stats["expiries"] == 0 {
		h.t.Fatal("fuzzer depth failure: no expiry ever settled")
	}
	if h.stats["deriv_accepted"] == 0 {
		h.t.Fatal("fuzzer depth failure: no derivative orders accepted")
	}
	if h.stats["deriv_trades"] == 0 {
		h.t.Fatal("fuzzer depth failure: no derivative order ever traded")
	}
}

func TestFuzzDerivativesLifecycle(t *testing.T) {
	for _, seed := range []int64{71, 72, 73} {
		seed := seed
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			newDerivFuzzHarness(t, seed).run()
		})
	}
}
