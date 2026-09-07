package exchange_test

import (
	"testing"

	. "exchange_sim/exchange"
)

// H-028. Funding is charged on position value at the mark, and liquidation
// triggers on equity measured at the mark. If the mark followed the perp's own
// book, one actor's order would change what every other actor pays and when
// they are closed out.
//
// It does not follow the book naively. With no mark calculator supplied — which
// is the campaign's configuration — every margined book is given a
// ClampedEMAMarkPrice anchored to the index, and that calculator
//
//   - returns the index alone when the book has no mid,
//   - returns an ERROR when the index itself is unavailable, rather than falling
//     back to the manipulable mid, and
//   - returns index + clamp(EMA(mid - index), ±band/2) otherwise.
//
// So the question is not whether a defence exists but how much room it leaves.
// This test measures that room instead of asserting it away.
type staticIndex struct{ price int64 }

func (s staticIndex) Price(string) (int64, error) { return s.price, nil }

func TestAuditHowFarOneActorCanWalkTheMark(t *testing.T) {
	const symbol = "ABC-PERP"
	const index = 50_000 * USD_PRECISION
	const bandBps = 600 // the default: mark stays within ±3% of index

	ex := NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients: 4,
		Clock:            &RealClock{},
	})
	perp := NewPerpFutures(symbol, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(perp)
	ex.ConfigureAutomation(AutomationConfig{IndexProvider: staticIndex{index}})

	for _, id := range []uint64{1, 2} {
		ex.ConnectNewClient(id, map[string]int64{}, &FixedFee{})
		ex.AddPerpBalance(id, "USD", USDAmount(100_000_000))
	}

	// An honest two-sided market straddling the index, wide enough that a
	// manipulating quote can improve the bid without crossing the ask. A quote
	// posted through the ask is not a resting manipulation, it is a marketable
	// order — the first version of this test made that mistake and measured a
	// crossed book instead of a moved mid.
	if _, reject := InjectLimitOrder(ex, 2, symbol, Buy, index-USDAmount(1_000), BTCAmount(1)); reject != "" {
		t.Fatalf("honest bid rejected: %s", reject)
	}
	if _, reject := InjectLimitOrder(ex, 2, symbol, Sell, index+USDAmount(1_000), BTCAmount(1)); reject != "" {
		t.Fatalf("honest ask rejected: %s", reject)
	}
	ex.UpdatePerpPrices()
	seeded := perp.GetFundingRate().MarkPrice
	t.Logf("index %d, honest mark %d (%.4f%% from index)",
		index, seeded, 100*float64(seeded-index)/float64(index))

	// One participant now posts a single minimum-size bid just inside the ask
	// and leaves it there. It never trades, it costs one unit of size, and it
	// drags the mid from the index up to nearly the ask.
	if _, reject := InjectLimitOrder(ex, 1, symbol, Buy, index+USDAmount(900), 1); reject != "" {
		t.Fatalf("manipulating bid rejected: %s", reject)
	}
	if mid, err := ex.GetBook(symbol).GetMidPrice(); err != nil {
		t.Fatalf("book has no mid after the quote: %v", err)
	} else {
		t.Logf("book mid moved to %d (%+.4f%% from index) on one unit of size",
			mid, 100*float64(mid-index)/float64(index))
	}

	var mark int64
	passes := 0
	for ; passes < 200; passes++ {
		ex.UpdatePerpPrices()
		mark = perp.GetFundingRate().MarkPrice
	}
	drift := float64(mark-index) / float64(index)
	t.Logf("after %d mark passes with one minimum-size order resting: mark %d (%+.4f%% from index)",
		passes, mark, 100*drift)

	halfBand := float64(bandBps) / 2 / 10000
	if drift > halfBand+1e-9 {
		t.Errorf("the mark walked %.4f%% past the ±%.2f%% clamp: the band is not holding",
			100*drift, 100*halfBand)
	}
	if drift <= 0 {
		t.Errorf("the resting order moved the mark by %+.4f%%: this fixture no longer demonstrates the mechanism", 100*drift)
	}
	t.Logf("one resting minimum-size order moves the mark to the clamp: %+.2f%% of index, "+
		"against a %d bps maintenance margin", 100*drift, perp.MaintenanceMarginRate)
}
