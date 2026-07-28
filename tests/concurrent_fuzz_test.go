package exchange_test

// Concurrency fuzz: drives the exchange through real client gateways with
// parallel goroutine clients while the automation loops (mark price, funding,
// collateral, expiry) run on fast tickers. Invariants can't be asserted
// mid-flight — instead the run must survive the race detector, and after
// quiescing (all clients stopped, channels drained) the final state must pass
// the same conservation and book-integrity checks as the sequential fuzzer.

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
)

func concurrentClient(wg *sync.WaitGroup, gw Gateway, seed int64, ops int, symbols []string) {
	defer wg.Done()
	rng := rand.New(rand.NewSource(seed))
	var reqSeq uint64
	var myOrders []uint64

	drain := func() {
		for {
			select {
			case resp := <-gw.Responses():
				if resp.Success {
					if id, ok := resp.Data.(uint64); ok {
						myOrders = append(myOrders, id)
					}
				}
			case <-gw.MarketDataCh():
			default:
				return
			}
		}
	}

	for i := 0; i < ops; i++ {
		drain()
		reqSeq++
		sym := symbols[rng.Intn(len(symbols))]
		if len(myOrders) > 0 && rng.Intn(4) == 0 {
			idx := rng.Intn(len(myOrders))
			gw.Send(Request{Type: ReqCancelOrder, CancelReq: &CancelRequest{
				RequestID: reqSeq, OrderID: myOrders[idx],
			}})
			myOrders = append(myOrders[:idx], myOrders[idx+1:]...)
			continue
		}
		side := Buy
		if rng.Intn(2) == 0 {
			side = Sell
		}
		price := (int64(100) + rng.Int63n(61) - 30) * DOLLAR_TICK
		req := &OrderRequest{
			RequestID: reqSeq, Side: side, Type: LimitOrder,
			Price: price, Qty: (1 + rng.Int63n(2)) * BTC_PRECISION / 2,
			Symbol: sym, TimeInForce: GTC,
		}
		if rng.Intn(5) == 0 {
			req.Type = Market
			req.Price = 0
		}
		gw.Send(Request{Type: ReqPlaceOrder, OrderReq: req})
		if rng.Intn(10) == 0 {
			time.Sleep(time.Microsecond)
		}
	}
	drain()
}

func TestFuzzConcurrentGateways(t *testing.T) {
	ex := NewExchange(10, &RealClock{})
	spot := NewSpotInstrument(fuzzSpotSym, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	perp := einstrument.NewPerpFutures(fuzzPerpSym, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(spot)
	ex.AddInstrument(perp)
	symbols := []string{fuzzSpotSym, fuzzPerpSym}

	const clients = 6
	gws := make([]Gateway, clients)
	initialUSD, initialABC := int64(0), int64(0)
	for i := 0; i < clients; i++ {
		id := uint64(i + 1)
		gws[i] = ex.ConnectNewClient(id, map[string]int64{
			"ABC": 1_000 * BTC_PRECISION,
			"USD": USDAmount(1_000_000),
		}, &PercentageFee{MakerBps: 2, TakerBps: 8, InQuote: true})
		ex.AddPerpBalance(id, "USD", USDAmount(500_000))
	}
	for _, asset := range []string{"USD", "ABC"} {
		var total int64
		ex.RLock()
		for _, c := range ex.Clients {
			total += c.Balances[asset] + c.PerpBalances[asset]
		}
		ex.RUnlock()
		if asset == "USD" {
			initialUSD = total
		} else {
			initialABC = total
		}
	}

	ex.ConfigureAutomation(AutomationConfig{PriceUpdateInterval: 5 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	ex.StartAutomation(ctx)

	ops := fuzzSteps(3_000)
	var wg sync.WaitGroup
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go concurrentClient(&wg, gws[i], int64(1000+i), ops, symbols)
	}
	wg.Wait()

	cancel()
	ex.StopAutomation()
	// Drain whatever the exchange goroutines still pushed after clients quit.
	time.Sleep(50 * time.Millisecond)
	for _, gw := range gws {
		for {
			select {
			case <-gw.Responses():
				continue
			case <-gw.MarketDataCh():
				continue
			default:
			}
			break
		}
	}

	// Quiesced: sequential-grade checks on the final state.
	ex.Lock()
	defer ex.Unlock()

	pm := ex.Positions.(*PositionManager)
	var posSum, basis int64
	for id := uint64(1); id <= clients; id++ {
		for _, pos := range pm.GetPositions(id) {
			posSum += pos.Size
			basis += MulDiv(pos.Size, pos.EntryPrice, BTC_PRECISION)
		}
	}
	if posSum != 0 {
		t.Errorf("position sum nonzero after quiesce: %d", posSum)
	}

	var totalUSD, totalABC int64
	for _, c := range ex.Clients {
		totalUSD += c.Balances["USD"] + c.PerpBalances["USD"]
		totalABC += c.Balances["ABC"] + c.PerpBalances["ABC"]
		for asset, v := range c.Reserved {
			if v < 0 {
				t.Errorf("negative reserved %s for client %d: %d", asset, c.ID, v)
			}
		}
		for asset, v := range c.PerpReserved {
			if v < 0 {
				t.Errorf("negative perp reserved %s for client %d: %d", asset, c.ID, v)
			}
		}
	}
	totalUSD += ex.ExchangeBalance.FeeRevenue["USD"] + ex.ExchangeBalance.InsuranceFund["USD"]
	totalABC += ex.ExchangeBalance.FeeRevenue["ABC"] + ex.ExchangeBalance.InsuranceFund["ABC"]

	// Same perp cost-basis correction and averaging-dust bound as the
	// sequential fuzzer (bounded by total op count across clients).
	dust := int64(clients*ops) * 3
	if d := totalUSD - basis - initialUSD; d > dust || d < -dust {
		t.Errorf("USD conservation violated after quiesce: delta=%d (dust bound %d)", d, dust)
	}
	if totalABC != initialABC {
		t.Errorf("ABC conservation violated after quiesce: delta=%d", totalABC-initialABC)
	}

	var trades uint64
	for sym, book := range ex.Books {
		trades += book.SeqNum
		if book.Bids.Best != nil && book.Asks.Best != nil && book.Bids.Best.Price >= book.Asks.Best.Price {
			t.Errorf("%s book crossed after quiesce: bid %d >= ask %d", sym, book.Bids.Best.Price, book.Asks.Best.Price)
		}
	}
	if trades < 100 {
		t.Fatalf("fuzzer depth failure: only %d trades executed", trades)
	}
	t.Logf("concurrent fuzz: %d trades across %d clients × %d ops", trades, clients, ops)
	_ = fmt.Sprintf
}
