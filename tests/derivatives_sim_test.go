package exchange_test

import (
	"context"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

// TestDerivativesEndToEndSim runs the full automation stack under simulated
// time: chains and futures list themselves on schedule, quoted and taken by
// injected flow, expire mid-run, settle, and relist — while USD is conserved
// across every client wallet plus exchange fee revenue.
func TestDerivativesEndToEndSim(t *testing.T) {
	simClock := simulation.NewSimulatedClock(derivStart)
	scheduler := simulation.NewEventScheduler(simClock)
	simClock.SetScheduler(scheduler)
	timerFact := simulation.NewSimTimerFactory(scheduler)

	ex := NewExchangeWithConfig(ExchangeConfig{
		EstimatedClients: 8,
		Clock:            simClock,
		TickerFactory:    timerFact,
	})
	defer ex.Shutdown()

	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(spot)

	tenor := int64(2 * time.Minute)
	spec := einstrument.ContractSpec{
		Base: "ABC", Quote: "USD",
		BasePrecision: BTC_PRECISION, QuotePrecision: USD_PRECISION,
		TickSize: DOLLAR_TICK, MinOrderSize: 1,
	}
	optSpec := spec
	optSpec.TickSize = USD_PRECISION // $1 premium tick
	ex.ConfigureAutomation(AutomationConfig{
		IndexProvider:       NewMidPriceOracle(ex),
		PriceUpdateInterval: 5 * time.Second,
		ListingPolicies: []ListingPolicy{
			&einstrument.DatedFuturesLister{
				Underlying: "ABC/USD", Spec: spec,
				TenorsNano: []int64{tenor}, DeliveryFeeBps: 5,
			},
			&einstrument.OptionChainLister{
				Underlying: "ABC/USD", Spec: optSpec,
				TenorsNano: []int64{tenor}, StrikeStep: PriceUSD(1000, DOLLAR_TICK),
				StrikesPerSide: 1, IV: 0.8, DeliveryFeeBps: 10,
			},
		},
	})

	// Clients: 1 = derivatives MM, 2 = taker, 3 = spot MM.
	spotBalances := map[string]int64{"USD": USDAmount(10_000_000), "ABC": 1000 * BTC_PRECISION}
	initialPerpUSD := USDAmount(5_000_000)
	gws := make(map[uint64]*ClientGateway)
	for id := uint64(1); id <= 3; id++ {
		ex.ConnectNewClient(id, spotBalances, &FixedFee{})
		ex.AddPerpBalance(id, "USD", initialPerpUSD)
		gws[id] = ex.Gateways[id]
	}

	// Drain all responses; count settles seen on the reference-data feed.
	var settledFutures, settledOptions atomic.Int64
	drainCtx, drainCancel := context.WithCancel(context.Background())
	defer drainCancel()
	for _, gw := range gws {
		go func(gw *ClientGateway) {
			for {
				select {
				case <-drainCtx.Done():
					return
				case <-gw.ResponseCh:
				}
			}
		}(gw)
	}
	go func() {
		gw := gws[1]
		for {
			select {
			case <-drainCtx.Done():
				return
			case md := <-gw.MarketDataCh():
				if md.Type == MDInstrument {
					if ann, ok := md.Data.(*InstrumentAnnouncement); ok && ann.Action == "settled" {
						switch ann.InstrumentType {
						case "FUTURE":
							settledFutures.Add(1)
						case "OPTION":
							settledOptions.Add(1)
						}
					}
				}
			}
		}
	}()
	gws[1].RequestCh <- Request{Type: ReqSubscribe, QueryReq: &QueryRequest{
		RequestID: 1, Symbol: InstrumentFeedSymbol, Types: []MDType{MDInstrument},
	}}

	totalUSD := func() int64 {
		ex.RLock()
		defer ex.RUnlock()
		sum := ex.ExchangeBalance.FeeRevenue["USD"]
		for _, c := range ex.Clients {
			sum += c.PerpBalances["USD"] + c.Balances["USD"]
		}
		return sum
	}
	initialTotal := totalUSD()

	ctx, cancel := context.WithCancel(context.Background())
	ex.StartAutomation(ctx)
	defer func() {
		cancel()
		ex.StopAutomation()
	}()

	rng := rand.New(rand.NewSource(7))
	spotMid := PriceUSD(50_000, DOLLAR_TICK)

	quote := func(gwID uint64, reqID *uint64, symbol string, mid, tick, qty int64) {
		if mid <= 2*tick {
			return
		}
		for _, o := range []struct {
			side  Side
			price int64
		}{{Buy, ((mid - tick) / tick) * tick}, {Sell, ((mid + tick) / tick) * tick}} {
			*reqID++
			gws[gwID].RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
				RequestID: *reqID, Symbol: symbol, Side: o.side, Type: LimitOrder,
				Price: o.price, Qty: qty, TimeInForce: GTC, Visibility: Normal,
			}}
		}
	}

	var reqID uint64 = 100
	simSeconds := 6 * 60 // three full tenor cycles
	for sec := 0; sec < simSeconds; sec++ {
		// Spot random walk quoted by client 3.
		spotMid += (rng.Int63n(3) - 1) * DOLLAR_TICK * 10
		ex.CancelAllClientOrders(3)
		quote(3, &reqID, "ABC/USD", spotMid, DOLLAR_TICK, BTC_PRECISION)

		// Client 1 quotes every listed derivative at its theoretical value.
		if sec%5 == 0 {
			ex.CancelAllClientOrders(1)
			now := simClock.NowUnixNano()
			for _, inst := range ex.ListInstruments("", "") {
				switch d := inst.(type) {
				case *ExpiringFutures:
					quote(1, &reqID, d.Symbol(), spotMid, DOLLAR_TICK, BTC_PRECISION)
				case *EuropeanOption:
					yearsLeft := float64(d.ExpiryNano()-now) / float64(365*24*time.Hour)
					mark := Black76Premium(spotMid, d.Strike, d.IV, yearsLeft, d.IsCall)
					quote(1, &reqID, d.Symbol(), (mark/USD_PRECISION)*USD_PRECISION, USD_PRECISION, BTC_PRECISION)
				}
			}
		}

		// Client 2 lifts a random derivative quote.
		if sec%3 == 1 {
			derivs := make([]string, 0, 8)
			for _, inst := range ex.ListInstruments("", "") {
				if inst.InstrumentType() != "SPOT" {
					derivs = append(derivs, inst.Symbol())
				}
			}
			if len(derivs) > 0 {
				side := Buy
				if rng.Intn(2) == 1 {
					side = Sell
				}
				reqID++
				gws[2].RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
					RequestID: reqID, Symbol: derivs[rng.Intn(len(derivs))], Side: side,
					Type: Market, Qty: BTC_PRECISION / 10,
				}}
			}
		}

		simClock.Advance(time.Second)
		time.Sleep(200 * time.Microsecond) // let goroutines drain
	}

	// Let every remaining contract expire with flow stopped.
	simClock.Advance(time.Duration(2 * tenor))
	time.Sleep(50 * time.Millisecond)

	if got := settledFutures.Load(); got < 2 {
		t.Fatalf("expected >=2 futures settlements over 3 tenor cycles, got %d", got)
	}
	if got := settledOptions.Load(); got < 4 {
		t.Fatalf("expected >=4 option settlements, got %d", got)
	}
	for _, inst := range ex.ListInstruments("", "") {
		if _, ok := inst.(etypes.Expirable); ok {
			if simClock.NowUnixNano() >= inst.(etypes.Expirable).ExpiryNano() {
				t.Fatalf("expired instrument still listed: %s", inst.Symbol())
			}
		}
	}

	if finalTotal := totalUSD(); finalTotal != initialTotal {
		t.Fatalf("USD not conserved: initial %d final %d (leak %d)",
			initialTotal, finalTotal, finalTotal-initialTotal)
	}
}
