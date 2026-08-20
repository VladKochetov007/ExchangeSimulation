package derivsim

import (
	"context"
	"math"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	eprice "exchange_sim/price"
	etypes "exchange_sim/types"
)

const (
	vvBasePrecision = int64(100_000_000)
	vvSpot          = 50_000 * vvBasePrecision
	vvExpiry        = int64(30 * 24 * time.Hour)
)

func newVannaVolgaHedger(t *testing.T, exposure func() []ContractExposure) (*VannaVolgaHedger, *stubGateway) {
	t.Helper()
	gw := newStubGateway()
	hedger := NewVannaVolgaHedger(3, gw, VannaVolgaHedgerConfig{
		Underlying:     "ABC/USD",
		VolModel:       eprice.FlatVolatility(0.8),
		VegaTolerance:  1e6,
		VannaTolerance: 0.05,
		VolgaTolerance: 1e6,
		LotQty:         vvBasePrecision,
		MaxContracts:   5 * vvBasePrecision,
		Interval:       time.Second,
		BasePrecision:  vvBasePrecision,
		Exposure:       exposure,
	})
	hedger.onTick(time.Unix(0, int64(time.Second)))
	listOption(hedger, "ABC-CALL-45", 45_000*vvBasePrecision, vvExpiry)
	listOption(hedger, "ABC-CALL-50", 50_000*vvBasePrecision, vvExpiry)
	listOption(hedger, "ABC-CALL-60", 60_000*vvBasePrecision, vvExpiry)
	hedger.HandleEvent(context.Background(), &actor.Event{
		Type: actor.EventBookSnapshot,
		Data: actor.BookSnapshotEvent{Symbol: "ABC/USD", Timestamp: int64(time.Second), Snapshot: bookAt(vvSpot)},
	})
	return hedger, gw
}

// bookAt is a two-sided underlying book centred on mid.
func bookAt(mid int64) *etypes.BookSnapshot {
	return &etypes.BookSnapshot{
		Bids: []etypes.PriceLevel{{Price: mid - 1, VisibleQty: vvBasePrecision}},
		Asks: []etypes.PriceLevel{{Price: mid + 1, VisibleQty: vvBasePrecision}},
	}
}

// A book inside every tolerance is left alone, however long the desk runs.
func TestVannaVolgaHedgerHoldsWhenTheBookIsFlat(t *testing.T) {
	hedger, gw := newVannaVolgaHedger(t, func() []ContractExposure { return nil })
	for i := 2; i < 6; i++ {
		hedger.onTick(time.Unix(0, int64(i)*int64(time.Second)))
	}
	if orders := gw.placedOrders(); len(orders) != 0 {
		t.Errorf("a flat book was hedged: %d orders", len(orders))
	}
}

// A short wing position carries vega the underlying cannot hedge at any size,
// so the desk has to buy options and the trade has to reduce the exposure.
func TestVannaVolgaHedgerBuysBackAShortWing(t *testing.T) {
	short := []ContractExposure{{
		Symbol: "ABC-CALL-60", Strike: 60_000 * vvBasePrecision, IsCall: true,
		ExpiryNano: vvExpiry, Position: -5 * vvBasePrecision,
	}}
	hedger, gw := newVannaVolgaHedger(t, func() []ContractExposure { return short })
	before := hedger.MeasureRisk(int64(2 * time.Second))
	if math.Abs(before.Vega) <= hedger.cfg.VegaTolerance && math.Abs(before.Vanna) <= hedger.cfg.VannaTolerance {
		t.Fatalf("the test position is inside tolerance: %+v", before)
	}
	hedger.onTick(time.Unix(0, int64(2*time.Second)))
	orders := gw.placedOrders()
	if len(orders) != 1 {
		t.Fatalf("orders = %d, want 1: %+v", len(orders), orders)
	}
	if orders[0].Side != exchange.Buy {
		t.Errorf("side = %v, want a buy to cover a short book", orders[0].Side)
	}
	hedger.onFill(orders[0].Symbol, actor.OrderFillEvent{
		Symbol: orders[0].Symbol, Side: exchange.Buy, Qty: vvBasePrecision, IsFull: true,
	})
	after := hedger.MeasureRisk(int64(2 * time.Second))
	if hedger.severity(after) >= hedger.severity(before) {
		t.Errorf("the hedge did not reduce the book's risk: %+v then %+v", before, after)
	}
}

// The desk must not build an unbounded position in one contract while hedging.
func TestVannaVolgaHedgerStopsAtItsContractCap(t *testing.T) {
	short := []ContractExposure{{
		Symbol: "ABC-CALL-60", Strike: 60_000 * vvBasePrecision, IsCall: true,
		ExpiryNano: vvExpiry, Position: -500 * vvBasePrecision,
	}}
	hedger, gw := newVannaVolgaHedger(t, func() []ContractExposure { return short })
	filled := 0
	for i := 2; i < 40; i++ {
		hedger.onTick(time.Unix(0, int64(i)*int64(time.Second)))
		for ; filled < len(gw.placedOrders()); filled++ {
			order := gw.placedOrders()[filled]
			hedger.onFill(order.Symbol, actor.OrderFillEvent{
				Symbol: order.Symbol, Side: order.Side, Qty: vvBasePrecision, IsFull: true,
			})
		}
	}
	for _, symbol := range []string{"ABC-CALL-45", "ABC-CALL-50", "ABC-CALL-60"} {
		if position := hedger.Position(symbol); position > hedger.cfg.MaxContracts || position < -hedger.cfg.MaxContracts {
			t.Errorf("%s position = %d, outside the cap %d", symbol, position, hedger.cfg.MaxContracts)
		}
	}
}

// Severity has to be measured in units of each tolerance, or the desk chases
// whichever exposure happens to be numerically largest in its own units.
func TestSeverityScoresEachExposureAgainstItsOwnTolerance(t *testing.T) {
	hedger, _ := newVannaVolgaHedger(t, func() []ContractExposure { return nil })
	vegaOnly := hedger.severity(BookRisk{Vega: hedger.cfg.VegaTolerance})
	vannaOnly := hedger.severity(BookRisk{Vanna: hedger.cfg.VannaTolerance})
	if math.Abs(vegaOnly-vannaOnly) > 1e-9 {
		t.Errorf("one tolerance of vega scored %v and one of vanna %v", vegaOnly, vannaOnly)
	}
}
