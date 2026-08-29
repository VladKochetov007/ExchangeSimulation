package exchange_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	. "exchange_sim/exchange"
)

// pnlLogger captures realised PnL as the exchange reports it.
//
// It reads the field out of the payload's persisted JSON rather than by
// asserting the payload's Go type. The evidence contract is the field name and
// its serialized value; whether the engine builds that payload from a map or a
// struct is an implementation detail, and a test that asserts the Go type fails
// on a change that leaves the persisted bytes identical. A malformed or missing
// field is reported rather than skipped, so a silent zero cannot pass for a
// measured zero.
type pnlLogger struct {
	mu     sync.Mutex
	total  int64
	fills  int
	failed error
}

func (l *pnlLogger) LogEvent(_ int64, _ uint64, eventName string, event any) {
	if eventName != "OrderFill" {
		return
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		l.record(fmt.Errorf("marshal OrderFill evidence: %w", err))
		return
	}
	var payload struct {
		RealizedPnL *int64 `json:"realized_pnl"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		l.record(fmt.Errorf("decode OrderFill evidence %s: %w", encoded, err))
		return
	}
	if payload.RealizedPnL == nil {
		l.record(fmt.Errorf("OrderFill evidence has no realized_pnl field: %s", encoded))
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total += *payload.RealizedPnL
	l.fills++
}

func (l *pnlLogger) record(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failed == nil {
		l.failed = err
	}
}

// A maker hedging on the perpetual pays 8.8 to 43 basis points per unit hedged
// in reference runs, which execution slippage (0.41 bps), the price move over
// the hedge delay (0.82 bps) and its net directional carry (+200 USD) together
// fail to explain by a factor of twenty. This pins realised PnL against known
// open and close prices, including the partial-close path where a wrong entry
// price would compound across many small closes.
func TestPerpRealizedPnLMatchesEntryAndExit(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		openSide              Side
		openPrice, closePrice float64
		openQty, closeQty     float64
		wantPnLUSD            float64
	}{
		{"long closed higher", Buy, 50_000, 50_100, 1, 1, 100},
		{"long closed lower", Buy, 50_000, 49_900, 1, 1, -100},
		{"short closed lower", Sell, 50_000, 49_900, 1, 1, 100},
		{"short closed higher", Sell, 50_000, 50_100, 1, 1, -100},
		{"long half closed higher", Buy, 50_000, 50_100, 2, 1, 100},
		{"short half closed lower", Sell, 50_000, 49_900, 2, 1, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := NewExchange(10, &RealClock{})
			ex.AddInstrument(NewPerpFutures("BTC-PERP", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
			logger := &pnlLogger{}
			ex.SetLogger("BTC-PERP", logger)

			for _, id := range []uint64{1, 2, 3} {
				ex.ConnectNewClient(id, map[string]int64{}, &FixedFee{})
				ex.AddPerpBalance(id, "USD", USDAmount(10_000_000))
			}

			counter := Buy
			if tc.openSide == Buy {
				counter = Sell
			}
			// Counterparty rests at the open price; subject crosses into it.
			rest := &OrderRequest{
				RequestID: 1, Symbol: "BTC-PERP", Side: counter, Type: LimitOrder,
				Price: PriceUSD(tc.openPrice, DOLLAR_TICK), Qty: BTCAmount(tc.openQty), TimeInForce: GTC,
			}
			if resp := ex.PlaceOrder(2, rest); !resp.Success {
				t.Fatalf("resting open rejected: %v", resp.Error)
			}
			open := &OrderRequest{
				RequestID: 2, Symbol: "BTC-PERP", Side: tc.openSide, Type: LimitOrder,
				Price: PriceUSD(tc.openPrice, DOLLAR_TICK), Qty: BTCAmount(tc.openQty), TimeInForce: IOC,
			}
			if resp := ex.PlaceOrder(1, open); !resp.Success {
				t.Fatalf("open rejected: %v", resp.Error)
			}

			logger.mu.Lock()
			logger.total, logger.fills = 0, 0
			logger.mu.Unlock()

			closeSide := Sell
			if tc.openSide == Sell {
				closeSide = Buy
			}
			counterClose := Buy
			if closeSide == Buy {
				counterClose = Sell
			}
			restClose := &OrderRequest{
				RequestID: 3, Symbol: "BTC-PERP", Side: counterClose, Type: LimitOrder,
				Price: PriceUSD(tc.closePrice, DOLLAR_TICK), Qty: BTCAmount(tc.closeQty), TimeInForce: GTC,
			}
			if resp := ex.PlaceOrder(3, restClose); !resp.Success {
				t.Fatalf("resting close rejected: %v", resp.Error)
			}
			closeOrder := &OrderRequest{
				RequestID: 4, Symbol: "BTC-PERP", Side: closeSide, Type: LimitOrder,
				Price: PriceUSD(tc.closePrice, DOLLAR_TICK), Qty: BTCAmount(tc.closeQty), TimeInForce: IOC,
			}
			if resp := ex.PlaceOrder(1, closeOrder); !resp.Success {
				t.Fatalf("close rejected: %v", resp.Error)
			}

			want := USDAmount(tc.wantPnLUSD)
			logger.mu.Lock()
			got := logger.total
			failure := logger.failed
			logger.mu.Unlock()
			if failure != nil {
				t.Fatalf("OrderFill evidence unusable: %v", failure)
			}
			if got != want {
				t.Fatalf("realised PnL = %d (%.2f USD), want %d (%.0f USD)",
					got, float64(got)/float64(USD_PRECISION), want, tc.wantPnLUSD)
			}
		})
	}
}
