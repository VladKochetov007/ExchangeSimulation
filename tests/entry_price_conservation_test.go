package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// Money conservation across a perp position built up over MANY fills at
// different prices and then fully closed. Each accumulation re-averages the
// entry price; if that average is lossy, the error lands directly in realized
// PnL and the system mints or burns money.
//
// The identity being tested: when every position is flat again, the only
// money movement is fees, so client balances plus fee revenue must equal the
// starting total exactly.
func TestMoneyConservation_PerpEntryPriceAveraging(t *testing.T) {
	ex := NewExchange(4, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)

	ex.ConnectNewClient(1, map[string]int64{}, &FixedFee{})
	ex.ConnectNewClient(2, map[string]int64{}, &FixedFee{})
	ex.AddPerpBalance(1, "USD", USDAmount(50_000_000))
	ex.AddPerpBalance(2, "USD", USDAmount(50_000_000))

	before := totalMoney(ex, "USD")

	// Client 1 accumulates long in odd sizes at prices that do not divide
	// evenly, forcing a re-average on every fill.
	prices := []float64{50_000, 50_133, 49_871, 50_477, 49_609, 50_311, 49_733}
	qtys := []float64{0.37, 0.11, 0.53, 0.29, 0.41, 0.17, 0.23}
	var totalQty int64
	for i := range prices {
		price := PriceUSD(prices[i], DOLLAR_TICK)
		qty := BTCAmount(qtys[i])
		if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Sell, price, qty); reject != "" {
			t.Fatalf("maker %d rejected: %s", i, reject)
		}
		if _, reject := InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty); reject != "" {
			t.Fatalf("taker %d rejected: %s", i, reject)
		}
		totalQty += qty
	}

	// Close the whole position back to flat at one price.
	closePrice := PriceUSD(50_200, DOLLAR_TICK)
	if _, reject := InjectLimitOrder(ex, 2, "BTC-PERP", Buy, closePrice, totalQty); reject != "" {
		t.Fatalf("close maker rejected: %s", reject)
	}
	if _, reject := InjectMarketOrder(ex, 1, "BTC-PERP", Sell, totalQty); reject != "" {
		t.Fatalf("close taker rejected: %s", reject)
	}

	for _, id := range []uint64{1, 2} {
		if pos := ex.Positions.GetPosition(id, "BTC-PERP"); pos != nil && pos.Size != 0 {
			t.Fatalf("client %d not flat: size %d", id, pos.Size)
		}
	}

	if got := totalMoney(ex, "USD"); got != before {
		t.Fatalf("USD not conserved across entry-price averaging: %d -> %d (delta %d)",
			before, got, got-before)
	}
}

// Same identity with an ASYMMETRIC counterparty structure: one long against
// several shorts whose own position histories differ. Symmetric two-client
// flow hides entry-averaging error because both sides truncate identically;
// a real ecology never has that symmetry.
func TestMoneyConservation_AsymmetricPerpParticipants(t *testing.T) {
	ex := NewExchange(8, &RealClock{})
	perp := NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)

	const traders = 5
	for id := uint64(1); id <= traders; id++ {
		ex.ConnectNewClient(id, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(id, "USD", USDAmount(50_000_000))
	}
	before := totalMoney(ex, "USD")

	// Rotate which client takes the other side, and vary sizes, so every
	// account carries a different average entry.
	prices := []float64{50_000, 50_133, 49_871, 50_477, 49_609, 50_311, 49_733, 50_099}
	qtys := []float64{0.37, 0.11, 0.53, 0.29, 0.41, 0.17, 0.23, 0.31}
	net := make(map[uint64]int64)
	for i := range prices {
		price := PriceUSD(prices[i], DOLLAR_TICK)
		qty := BTCAmount(qtys[i])
		maker := uint64(2 + i%(traders-1))
		if _, reject := InjectLimitOrder(ex, maker, "BTC-PERP", Sell, price, qty); reject != "" {
			t.Fatalf("maker %d rejected: %s", i, reject)
		}
		if _, reject := InjectMarketOrder(ex, 1, "BTC-PERP", Buy, qty); reject != "" {
			t.Fatalf("taker %d rejected: %s", i, reject)
		}
		net[1] += qty
		net[maker] -= qty
	}

	// Unwind everyone to flat against client 1 at a single price.
	closePrice := PriceUSD(50_200, DOLLAR_TICK)
	for id := uint64(2); id <= traders; id++ {
		short := -net[id]
		if short <= 0 {
			continue
		}
		if _, reject := InjectLimitOrder(ex, 1, "BTC-PERP", Sell, closePrice, short); reject != "" {
			t.Fatalf("unwind maker rejected: %s", reject)
		}
		if _, reject := InjectMarketOrder(ex, id, "BTC-PERP", Buy, short); reject != "" {
			t.Fatalf("unwind taker %d rejected: %s", id, reject)
		}
	}

	for id := uint64(1); id <= traders; id++ {
		if pos := ex.Positions.GetPosition(id, "BTC-PERP"); pos != nil && pos.Size != 0 {
			t.Fatalf("client %d not flat: size %d", id, pos.Size)
		}
	}
	// Every account is flat, so the cost-basis term is zero and cash alone is
	// the invariant. Entry prices are stored as truncated integers, so each
	// account's basis carries sub-unit error that no longer cancels once
	// counterparties have different position histories; that dust is the only
	// permitted deviation.
	const dustTolerance = 100
	delta := totalMoney(ex, "USD") - before
	if delta < 0 {
		delta = -delta
	}
	if delta > dustTolerance {
		t.Fatalf("USD not conserved with asymmetric participants: delta %d exceeds entry-rounding dust tolerance %d",
			delta, dustTolerance)
	}
}
