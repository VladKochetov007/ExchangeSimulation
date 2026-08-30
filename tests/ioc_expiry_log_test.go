package exchange_test

import (
	"encoding/json"
	"sync"
	"testing"

	. "exchange_sim/exchange"
)

// expiryLogger records the terminal events an order can produce.
//
// Cancellations are captured as their persisted JSON rather than as a Go type.
// The earlier version type-asserted map[string]any and appended only on
// success, so when the payload became a struct the test reported zero
// cancellations instead of a type error — a silent skip that looked like a
// behavioural regression. What this test is about is the evidence a desk can
// read back, so the evidence is what it asserts.
type expiryLogger struct {
	mu        sync.Mutex
	cancelled []map[string]any
	fills     int
	decodeErr error
}

func (l *expiryLogger) LogEvent(_ int64, _ uint64, eventName string, event any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch eventName {
	case "OrderCancelled":
		encoded, err := json.Marshal(event)
		if err != nil {
			l.decodeErr = err
			return
		}
		var record map[string]any
		if err := json.Unmarshal(encoded, &record); err != nil {
			l.decodeErr = err
			return
		}
		l.cancelled = append(l.cancelled, record)
	case "OrderFill":
		l.fills++
	}
}

// An IOC that does not fill leaves the book without any logged termination, so
// its outcome can only be inferred from the absence of a fill. A reference run
// logged 42915 IOC acceptances and not one cancellation, which is why a desk
// missing the touch 899 times in a row looked like a desk sitting idle.
func TestUnfilledIOCLogsItsExpiry(t *testing.T) {
	for _, tc := range []struct {
		name         string
		restingQty   int64
		takerQty     int64
		wantRemains  int64
		wantAnyFills bool
	}{
		{"no liquidity at all", 0, BTCAmount(1), BTCAmount(1), false},
		{"partial liquidity leaves a remainder", BTCAmount(0.25), BTCAmount(1), BTCAmount(0.75), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ex := NewExchange(10, &RealClock{})
			ex.AddInstrument(NewSpotInstrument("BTC/USD", "BTC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, USD_PRECISION/1000))
			logger := &expiryLogger{}
			ex.SetLogger("BTC/USD", logger)

			balances := map[string]int64{"BTC": BTCAmount(10), "USD": USDAmount(1_000_000)}
			ex.ConnectNewClient(1, balances, &FixedFee{})
			ex.ConnectNewClient(2, balances, &FixedFee{})

			restingPrice := PriceUSD(50000, DOLLAR_TICK)
			if tc.restingQty > 0 {
				maker := &OrderRequest{
					RequestID: 1, Symbol: "BTC/USD", Side: Sell, Type: LimitOrder,
					Price: restingPrice, Qty: tc.restingQty, TimeInForce: GTC,
				}
				if resp := ex.PlaceOrder(1, maker); !resp.Success {
					t.Fatalf("resting sell rejected: %v", resp.Error)
				}
			}

			taker := &OrderRequest{
				RequestID: 2, Symbol: "BTC/USD", Side: Buy, Type: LimitOrder,
				Price: restingPrice, Qty: tc.takerQty, TimeInForce: IOC,
			}
			if resp := ex.PlaceOrder(2, taker); !resp.Success {
				t.Fatalf("IOC buy rejected: %v", resp.Error)
			}

			if (logger.fills > 0) != tc.wantAnyFills {
				t.Fatalf("fills = %d, wantAny = %v", logger.fills, tc.wantAnyFills)
			}
			if len(logger.cancelled) != 1 {
				t.Fatalf("expected one logged expiry for the unfilled remainder, got %d", len(logger.cancelled))
			}
			if logger.decodeErr != nil {
				t.Fatalf("cancellation evidence did not encode: %v", logger.decodeErr)
			}
			// Through JSON a number arrives as float64, so compare numerically
			// rather than by Go type.
			got, ok := logger.cancelled[0]["remaining_qty"].(float64)
			if !ok || int64(got) != tc.wantRemains {
				t.Fatalf("logged remaining_qty = %v, want %d",
					logger.cancelled[0]["remaining_qty"], tc.wantRemains)
			}
			if got := logger.cancelled[0]["reason"]; got != "IOC_EXPIRED" {
				t.Fatalf("logged reason = %v, want IOC_EXPIRED", got)
			}
		})
	}
}
