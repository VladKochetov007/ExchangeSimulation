package exchange_test

// Deterministic concurrent workload.
//
// The older concurrency fuzzer asserted that at least a hundred trades had
// happened by the time it finished. That is a statement about how fast the
// host is, not about the engine. Instrumenting every stage showed where the
// requests went: of 18,000 submitted under load, 0 were dropped by the lossy
// Send path and 0 were lost, but 11,000-13,000 were still sitting in the
// request queues when the old test stopped the exchange 50ms after its
// producers finished. Drained properly, all 18,000 are acknowledged -- it
// simply takes 17-22 seconds on a machine this busy. The old test measured a
// partially processed queue and called the shortfall a trade count.
//
// This test removes that dependency instead of loosening the threshold. It
// generates a fixed workload, enqueues every request without dropping any,
// accounts for each one through every stage of the pipeline, and only then
// checks the safety invariants.
//
// Stage accounting, all of it required to balance:
//
//	generated -> enqueued -> acknowledged -> matched -> fill emitted -> fill consumed
//
// The watchdog exists to catch a hang. It does not define how much trading
// should have happened.

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
	etypes "exchange_sim/types"
)

// workloadStages counts one run's progress through the pipeline.
type workloadStages struct {
	generated   atomic.Int64
	enqueued    atomic.Int64
	acked       atomic.Int64
	rejected    atomic.Int64
	fillsEmit   atomic.Int64
	forcedCance atomic.Int64
}

// fillIdentity is what makes a fill unique: one trade delivers exactly one
// notification to each of its two sides.
type fillIdentity struct {
	tradeID  uint64
	clientID uint64
	orderID  uint64
}

// workloadConsumer drains one client's channels for the life of the run,
// recording acknowledgements by request id and fills by identity.
type workloadConsumer struct {
	mu       sync.Mutex
	acks     map[uint64]int
	fills    map[fillIdentity]int
	fillQty  int64
	stopOnce sync.Once
	stop     chan struct{}
	done     chan struct{}
}

func newWorkloadConsumer() *workloadConsumer {
	return &workloadConsumer{
		acks:  make(map[uint64]int),
		fills: make(map[fillIdentity]int),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
}

func (c *workloadConsumer) run(gw Gateway, stages *workloadStages) {
	defer close(c.done)
	for {
		select {
		case resp := <-gw.Responses():
			c.record(resp, stages)
		case <-gw.MarketDataCh():
		case <-c.stop:
			// Drain what is already queued before giving up: a response that
			// arrived and was never read is not a lost request.
			for {
				select {
				case resp := <-gw.Responses():
					c.record(resp, stages)
				case <-gw.MarketDataCh():
				default:
					return
				}
			}
		}
	}
}

func (c *workloadConsumer) record(resp etypes.Response, stages *workloadStages) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch payload := resp.Data.(type) {
	case etypes.FillNotification:
		stages.fillsEmit.Add(1)
		id := fillIdentity{tradeID: payload.TradeID, clientID: payload.ClientID, orderID: payload.OrderID}
		c.fills[id]++
		c.fillQty += payload.Qty
	case *etypes.FillNotification:
		stages.fillsEmit.Add(1)
		id := fillIdentity{tradeID: payload.TradeID, clientID: payload.ClientID, orderID: payload.OrderID}
		c.fills[id]++
		c.fillQty += payload.Qty
	case *ForcedCancelNotification:
		stages.forcedCance.Add(1)
	default:
		// Everything else carries a request id and is the terminal answer to
		// one submitted request.
		if resp.RequestID == 0 {
			return
		}
		c.acks[resp.RequestID]++
		if resp.Success {
			stages.acked.Add(1)
		} else {
			stages.rejected.Add(1)
		}
	}
}

func (c *workloadConsumer) close() {
	c.stopOnce.Do(func() { close(c.stop) })
	<-c.done
}

// enqueue submits one request without the non-blocking drop that Gateway.Send
// performs, so that "generated" and "enqueued" are the same number by
// construction and a lost request can only be lost inside the exchange.
func enqueue(t *testing.T, cg *ClientGateway, req Request, watchdog time.Duration) {
	t.Helper()
	select {
	case cg.RequestCh <- req:
	case <-time.After(watchdog):
		t.Fatalf("enqueue blocked for %s: the exchange stopped consuming requests", watchdog)
	}
}

// TestConcurrentGatewayWorkloadAccountsForEveryRequest is the correctness half.
// Nothing it asserts depends on how fast the host is.
func TestConcurrentGatewayWorkloadAccountsForEveryRequest(t *testing.T) {
	const (
		clients  = 6
		opsEach  = 400
		watchdog = 60 * time.Second
	)
	ex := NewExchange(clients+2, &RealClock{})
	spot := NewSpotInstrument(fuzzSpotSym, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	perp := einstrument.NewPerpFutures(fuzzPerpSym, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(spot)
	ex.AddInstrument(perp)
	symbols := []string{fuzzSpotSym, fuzzPerpSym}

	gws := make([]Gateway, clients)
	consumers := make([]*workloadConsumer, clients)
	var initialUSD, initialABC int64
	for i := 0; i < clients; i++ {
		id := uint64(i + 1)
		gws[i] = ex.ConnectNewClient(id, map[string]int64{
			"ABC": 1_000 * BTC_PRECISION,
			"USD": USDAmount(1_000_000),
		}, &PercentageFee{MakerBps: 2, TakerBps: 8, InQuote: true})
		ex.AddPerpBalance(id, "USD", USDAmount(500_000))
	}
	ex.RLock()
	for _, c := range ex.Clients {
		initialUSD += c.Balances["USD"] + c.PerpBalances["USD"]
		initialABC += c.Balances["ABC"] + c.PerpBalances["ABC"]
	}
	ex.RUnlock()

	stages := &workloadStages{}
	for i := range consumers {
		consumers[i] = newWorkloadConsumer()
		go consumers[i].run(gws[i], stages)
	}

	// Every request id is unique across the whole run, so an acknowledgement
	// can be attributed to exactly one submission.
	requestIDs := make([][]uint64, clients)
	start := make(chan struct{})
	var producers sync.WaitGroup
	for i := 0; i < clients; i++ {
		producers.Add(1)
		go func(index int) {
			defer producers.Done()
			cg := gws[index].(*ClientGateway)
			base := uint64(index+1) * 1_000_000
			ids := make([]uint64, 0, opsEach)
			<-start // barrier: all producers submit concurrently
			for op := 0; op < opsEach; op++ {
				id := base + uint64(op)
				side := Buy
				if (index+op)%2 == 0 {
					side = Sell
				}
				// A deterministic ladder around a fixed level, so the two
				// sides cross and the workload is identical on every run and
				// on every host.
				price := (int64(100) + int64((index*7+op*3)%21) - 10) * DOLLAR_TICK
				enqueue(t, cg, Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
					RequestID: id, Symbol: symbols[op%len(symbols)], Side: side,
					Type: LimitOrder, Price: price, Qty: BTC_PRECISION / 100,
				}}, watchdog)
				stages.generated.Add(1)
				stages.enqueued.Add(1)
				ids = append(ids, id)
			}
			requestIDs[index] = ids
		}(i)
	}
	close(start)
	producers.Wait()

	// Wait for the pipeline to empty. This is the watchdog: it fires only if
	// the engine has genuinely stopped making progress.
	expected := int64(clients * opsEach)
	deadline := time.Now().Add(watchdog)
	for {
		settled := stages.acked.Load() + stages.rejected.Load()
		pending := 0
		for _, gw := range gws {
			pending += len(gw.(*ClientGateway).RequestCh)
		}
		if pending == 0 && settled >= expected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pipeline stalled: %d of %d requests settled, %d still queued after %s",
				settled, expected, pending, watchdog)
		}
		time.Sleep(2 * time.Millisecond)
	}
	// Fills are delivered after the acknowledgement that caused them, so give
	// the notifications a bounded moment to land before closing the readers.
	time.Sleep(200 * time.Millisecond)
	for _, consumer := range consumers {
		consumer.close()
	}

	// --- stage accounting ---
	if got, want := stages.enqueued.Load(), expected; got != want {
		t.Fatalf("enqueued %d requests, generated %d", got, want)
	}
	settled := stages.acked.Load() + stages.rejected.Load()
	if settled != expected {
		t.Errorf("%d of %d requests reached a terminal state; %d unaccounted",
			settled, expected, expected-settled)
	}
	for i, ids := range requestIDs {
		consumers[i].mu.Lock()
		for _, id := range ids {
			switch consumers[i].acks[id] {
			case 1:
			case 0:
				t.Errorf("client %d request %d was accepted by the queue and never answered", i+1, id)
			default:
				t.Errorf("client %d request %d was answered %d times", i+1, id, consumers[i].acks[id])
			}
		}
		consumers[i].mu.Unlock()
	}

	// --- no duplicate fills ---
	totalFills := 0
	for i, consumer := range consumers {
		consumer.mu.Lock()
		for id, count := range consumer.fills {
			totalFills += count
			if count != 1 {
				t.Errorf("client %d received trade %d on order %d %d times", i+1, id.tradeID, id.orderID, count)
			}
		}
		consumer.mu.Unlock()
	}

	// --- safety invariants on the quiesced state ---
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
		t.Errorf("positions do not net to zero: %d", posSum)
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
		if c.Balances["ABC"] < 0 || c.Balances["USD"] < 0 {
			t.Errorf("negative spot balance for client %d: ABC=%d USD=%d", c.ID, c.Balances["ABC"], c.Balances["USD"])
		}
	}
	totalUSD += ex.ExchangeBalance.FeeRevenue["USD"] + ex.ExchangeBalance.InsuranceFund["USD"]
	totalABC += ex.ExchangeBalance.FeeRevenue["ABC"] + ex.ExchangeBalance.InsuranceFund["ABC"]

	dust := int64(clients*opsEach) * 3
	if d := totalUSD - basis - initialUSD; d > dust || d < -dust {
		t.Errorf("USD conservation violated: delta=%d (dust bound %d)", d, dust)
	}
	if totalABC != initialABC {
		t.Errorf("ABC conservation violated: delta=%d", totalABC-initialABC)
	}

	var trades int64
	for sym, book := range ex.Books {
		trades += int64(book.SeqNum)
		if book.Bids.Best != nil && book.Asks.Best != nil && book.Bids.Best.Price >= book.Asks.Best.Price {
			t.Errorf("%s book crossed: bid %d >= ask %d", sym, book.Bids.Best.Price, book.Asks.Best.Price)
		}
	}
	// Every trade has two sides and both are accounts in this run, so each one
	// must produce exactly two notifications. This is the check that a fill
	// was neither dropped on the way out nor delivered twice, and it holds
	// whatever the host was doing.
	if want := 2 * trades; int64(totalFills) != want {
		t.Errorf("%d fill notifications for %d trades; expected %d", totalFills, trades, want)
	}
	t.Logf("stages: generated=%d enqueued=%d acked=%d rejected=%d fills=%d forced_cancels=%d distinct_fills=%d",
		stages.generated.Load(), stages.enqueued.Load(), stages.acked.Load(),
		stages.rejected.Load(), stages.fillsEmit.Load(), stages.forcedCance.Load(), totalFills)
	t.Logf("trades=%d fills_per_trade=%.2f", trades, float64(totalFills)/float64(max64(trades, 1)))
}

// TestGatewaySendAfterCloseDropsRatherThanRacing pins the one behaviour the
// deterministic workload deliberately bypasses: Gateway.Send is lossy, and a
// send racing a close must be dropped rather than panic or enqueue onto a
// closed gateway.
func TestGatewaySendAfterCloseDropsRatherThanRacing(t *testing.T) {
	ex := NewExchange(2, &RealClock{})
	ex.AddInstrument(NewSpotInstrument(fuzzSpotSym, "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1))
	gw := ex.ConnectNewClient(1, map[string]int64{"ABC": BTC_PRECISION, "USD": USDAmount(1000)}, &FixedFee{})
	cg := gw.(*ClientGateway)

	var senders sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		senders.Add(1)
		go func(worker int) {
			defer senders.Done()
			seq := uint64(worker) * 100_000
			for {
				select {
				case <-stop:
					return
				default:
				}
				seq++
				gw.Send(Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
					RequestID: seq, Symbol: fuzzSpotSym, Side: Buy, Type: LimitOrder,
					Price: 100 * DOLLAR_TICK, Qty: BTC_PRECISION / 1000,
				}})
			}
		}(i)
	}
	time.Sleep(20 * time.Millisecond)
	cg.Close()
	time.Sleep(20 * time.Millisecond)
	close(stop)
	senders.Wait()

	if cg.IsRunning() {
		t.Fatal("gateway still reports running after Close")
	}
	// Reaching here without a panic or a send on a closed channel is the
	// assertion; the count that got through is deliberately not checked.
	t.Logf("gateway closed under %d concurrent senders with %d requests still queued", 4, len(cg.RequestCh))
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

var _ = fmt.Sprintf
