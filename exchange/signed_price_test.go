package exchange

import (
	"testing"

	etypes "exchange_sim/types"
)

func newSignedFutureExchange(t *testing.T) (*DefaultExchange, *ExpiringFutures) {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	future := NewExpiringFutures("OIL-FUT", "OIL", "USD", 1, 1, 1, 1, 1<<62)
	if err := future.SetPriceDomain(etypes.SignedPriceDomain(1)); err != nil {
		t.Fatalf("set signed dated-future domain: %v", err)
	}
	ex.AddInstrument(future)
	for _, id := range []uint64{1, 2, 3} {
		ex.ConnectNewClient(id, nil, &FixedFee{})
		ex.AddPerpBalance(id, "USD", 1_000)
	}
	return ex, future
}

func placeSignedFutureOrder(t *testing.T, ex *DefaultExchange, clientID, requestID uint64, side Side, price, qty int64, tif TimeInForce, postOnly bool) Response {
	t.Helper()
	return ex.PlaceOrder(clientID, &OrderRequest{
		RequestID: requestID, Symbol: "OIL-FUT", Side: side, Type: LimitOrder,
		Price: price, Qty: qty, TimeInForce: tif, Visibility: Normal, PostOnly: postOnly,
	})
}

func TestSignedDatedFutureAdmissionAndPreviewMatch(t *testing.T) {
	ex, future := newSignedFutureExchange(t)
	defer ex.Shutdown()
	if response := placeSignedFutureOrder(t, ex, 1, 1, Sell, -20, 1, GTC, false); !response.Success {
		t.Fatalf("negative resting ask rejected: %#v", response)
	}
	if response := placeSignedFutureOrder(t, ex, 2, 2, Buy, -20, 1, GTC, false); !response.Success {
		t.Fatalf("negative crossing bid rejected: %#v", response)
	}
	if pos := ex.Positions.GetPosition(2, future.Symbol()); pos == nil || pos.Size != 1 || pos.EntryPrice != -20 {
		t.Fatalf("buyer signed future position = %#v, want +1 at -20", pos)
	}
	wantMargin, err := future.MarginRequired(1, -20, 1)
	if err != nil {
		t.Fatalf("signed future expected margin: %v", err)
	}
	if got := ex.Clients[1].PerpReserved["USD"]; got != wantMargin {
		t.Fatalf("seller signed future margin = %d, want %d", got, wantMargin)
	}
}

func TestSignedPricePostOnlyAndIOCFOkContracts(t *testing.T) {
	t.Run("post-only rejects crossing negative level without mutation", func(t *testing.T) {
		ex, _ := newSignedFutureExchange(t)
		defer ex.Shutdown()
		if response := placeSignedFutureOrder(t, ex, 1, 1, Sell, -20, 1, GTC, false); !response.Success {
			t.Fatalf("seed negative ask: %#v", response)
		}
		beforeOrderID := ex.NextOrderID
		beforeReserved := ex.Clients[2].PerpReserved["USD"]
		response := placeSignedFutureOrder(t, ex, 2, 2, Buy, -20, 1, GTC, true)
		if response.Success || response.Error != RejectPostOnlyWouldTake {
			t.Fatalf("negative crossing post-only = %#v, want post-only reject", response)
		}
		if ex.NextOrderID != beforeOrderID || ex.Clients[2].PerpReserved["USD"] != beforeReserved {
			t.Fatalf("post-only rejection mutated id/reservation: id=%d reserved=%d", ex.NextOrderID, ex.Clients[2].PerpReserved["USD"])
		}
	})

	t.Run("ioc executes negative level and fok uses detached signed preview", func(t *testing.T) {
		ex, _ := newSignedFutureExchange(t)
		defer ex.Shutdown()
		if response := placeSignedFutureOrder(t, ex, 1, 1, Sell, -20, 1, GTC, false); !response.Success {
			t.Fatalf("seed negative ask: %#v", response)
		}
		if response := placeSignedFutureOrder(t, ex, 2, 2, Buy, -20, 1, IOC, false); !response.Success {
			t.Fatalf("negative IOC = %#v", response)
		}
		if response := placeSignedFutureOrder(t, ex, 1, 3, Sell, -10, 1, GTC, false); !response.Success {
			t.Fatalf("seed second negative ask: %#v", response)
		}
		if response := placeSignedFutureOrder(t, ex, 3, 4, Buy, -10, 2, FOK, false); response.Success || response.Error != RejectFOKNotFilled {
			t.Fatalf("negative FOK = %#v, want detached-preflight rejection", response)
		}
		if book := ex.Books["OIL-FUT"]; book.Asks.Best == nil || book.Asks.Best.Price != -10 || book.Asks.Best.TotalQty != 1 {
			t.Fatalf("FOK preview mutated signed book: %#v", book.Asks.Best)
		}
	})
}

func TestSignedDatedFutureRiskTreatsZeroMarkAsPresent(t *testing.T) {
	ex, future := newSignedFutureExchange(t)
	defer ex.Shutdown()
	future.GetFundingRate().MarkAvailable = true
	future.GetFundingRate().MarkPrice = 0

	mark, err := riskMark(future, ex.Books[future.Symbol()])
	if err != nil || mark != 0 {
		t.Fatalf("zero available future mark = (%d, %v), want (0, nil)", mark, err)
	}
	pos := &Position{Size: 1, EntryPrice: -20}
	if pnl, ok := tryPositionUPnL(pos, mark, 1); !ok || pnl != 20 {
		t.Fatalf("zero-mark signed PnL = (%d, %t), want (20, true)", pnl, ok)
	}
	if maintenance, ok := positionMaintenanceAtMark(future, pos.Size, mark); !ok || maintenance != 0 {
		t.Fatalf("zero-mark maintenance = (%d, %t), want (0, true)", maintenance, ok)
	}
}

func TestForceCloseDistinguishesZeroPriceFillFromNoLiquidity(t *testing.T) {
	ex, future := newSignedFutureExchange(t)
	defer ex.Shutdown()
	if response := placeSignedFutureOrder(t, ex, 2, 1, Buy, 0, 1, GTC, false); !response.Success {
		t.Fatalf("zero-price signed bid rejected: %#v", response)
	}
	ex.Positions.UpdatePosition(1, future.Symbol(), 1, 0, Buy, PositionBoth)

	ex.mu.Lock()
	price, quantity, filled := ex.forceClose(1, ex.Clients[1], ex.Books[future.Symbol()], future, Sell, PositionBoth, 1, 1)
	ex.mu.Unlock()
	if !filled || quantity != 1 || price != 0 {
		t.Fatalf("zero-price force close = (price=%d quantity=%d filled=%t), want (0, 1, true)", price, quantity, filled)
	}
}
