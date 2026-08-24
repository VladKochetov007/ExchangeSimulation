package exchange_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	. "exchange_sim/exchange"
	einstrument "exchange_sim/instrument"
	etypes "exchange_sim/types"
)

// derivClock is a manually advanced clock for expiry tests.
type derivClock struct{ now int64 }

func (c *derivClock) NowUnixNano() int64 { return c.now }
func (c *derivClock) NowUnix() int64     { return c.now / 1e9 }

type constPrice int64

func (p constPrice) Price(symbol string) (int64, error) {
	if p <= 0 {
		return 0, fmt.Errorf("const price for %s unavailable", symbol)
	}
	return int64(p), nil
}

type listingPrice int64

func (p listingPrice) Price(symbol string) (int64, error) {
	if p <= 0 {
		return 0, fmt.Errorf("listing price for %s unavailable", symbol)
	}
	return int64(p), nil
}

func pendingListings(t *testing.T, policy etypes.ListingPolicy, now int64, value int64) []etypes.Instrument {
	t.Helper()
	listed, err := policy.PendingListings(now, listingPrice(value))
	if err != nil {
		t.Fatal(err)
	}
	return listed
}

const derivStart = int64(1_700_000_000_000_000_000)

// newDerivExchange builds an exchange with a spot ABC/USD book quoted around
// $50,000 (client 3 rests both sides) so derivatives have an underlying mid.
func newDerivExchange(t *testing.T, clock *derivClock) *Exchange {
	t.Helper()
	ex := NewExchange(10, clock)
	spot := NewSpotInstrument("ABC/USD", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1)
	ex.AddInstrument(spot)

	ex.ConnectNewClient(3, map[string]int64{
		"USD": USDAmount(10_000_000),
		"ABC": 1000 * BTC_PRECISION,
	}, &FixedFee{})
	gw := ex.Gateways[3]
	for i, ord := range []OrderRequest{
		{Side: Buy, Price: PriceUSD(49_900, DOLLAR_TICK), Qty: BTC_PRECISION},
		{Side: Sell, Price: PriceUSD(50_100, DOLLAR_TICK), Qty: BTC_PRECISION},
	} {
		ord.RequestID = uint64(900 + i)
		ord.Symbol = "ABC/USD"
		ord.Type = LimitOrder
		ord.TimeInForce = GTC
		ord.Visibility = Normal
		req := ord
		gw.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &req}
		resp := <-gw.ResponseCh
		if !resp.Success {
			t.Fatalf("spot quote rejected: %v", resp.Error)
		}
	}
	return ex
}

func connectDerivTrader(ex *Exchange, id uint64, usd float64) *ClientGateway {
	ex.ConnectNewClient(id, nil, &FixedFee{})
	ex.AddPerpBalance(id, "USD", USDAmount(usd))
	return ex.Gateways[id]
}

func placeLimit(t *testing.T, gw *ClientGateway, reqID uint64, symbol string, side Side, price, qty int64) {
	t.Helper()
	gw.RequestCh <- Request{Type: ReqPlaceOrder, OrderReq: &OrderRequest{
		RequestID: reqID, Symbol: symbol, Side: side, Type: LimitOrder,
		Price: price, Qty: qty, TimeInForce: GTC, Visibility: Normal,
	}}
	resp := <-gw.ResponseCh
	if !resp.Success {
		t.Fatalf("order %d rejected: %v", reqID, resp.Error)
	}
}

func TestDatedFuturesLifecycle(t *testing.T) {
	clock := &derivClock{now: derivStart}
	ex := newDerivExchange(t, clock)
	defer ex.Shutdown()

	expiry := derivStart + int64(60*time.Second)
	fut := NewExpiringFutures("ABC-FUT-1", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, expiry)
	fut.Underlying = "ABC/USD"
	ex.AddInstrument(fut)

	gw1 := connectDerivTrader(ex, 1, 100_000)
	gw2 := connectDerivTrader(ex, 2, 100_000)

	entry := PriceUSD(50_500, DOLLAR_TICK)
	placeLimit(t, gw2, 1, "ABC-FUT-1", Sell, entry, BTC_PRECISION)
	placeLimit(t, gw1, 2, "ABC-FUT-1", Buy, entry, BTC_PRECISION)
	<-gw1.ResponseCh // fill
	<-gw2.ResponseCh // fill

	wantMargin, err := fut.MarginRequired(BTC_PRECISION, entry, BTC_PRECISION)
	if err != nil {
		t.Fatalf("expected future margin: %v", err)
	}
	for _, id := range []uint64{1, 2} {
		if got := ex.Clients[id].PerpReserved["USD"]; got != wantMargin {
			t.Fatalf("client %d position margin = %d, want %d", id, got, wantMargin)
		}
	}

	// A far-from-market resting order must be cancelled and released at expiry.
	placeLimit(t, gw1, 3, "ABC-FUT-1", Buy, PriceUSD(40_000, DOLLAR_TICK), BTC_PRECISION)

	// Observe the underlying mid a few times, then expire.
	for i := 0; i < 5; i++ {
		clock.now += int64(10 * time.Second)
		ex.UpdateDerivativeMarks()
	}
	clock.now = expiry + 1
	ex.CheckExpiries()

	if ex.GetBook("ABC-FUT-1") != nil {
		t.Fatal("expired book still listed")
	}
	settle := PriceUSD(50_000, DOLLAR_TICK) // spot mid
	wantLong := USDAmount(100_000) + MulDiv(BTC_PRECISION, settle-entry, BTC_PRECISION)
	wantShort := USDAmount(100_000) + MulDiv(BTC_PRECISION, entry-settle, BTC_PRECISION)
	if got := ex.Clients[1].PerpBalances["USD"]; got != wantLong {
		t.Fatalf("long settled balance = %d, want %d", got, wantLong)
	}
	if got := ex.Clients[2].PerpBalances["USD"]; got != wantShort {
		t.Fatalf("short settled balance = %d, want %d", got, wantShort)
	}
	for _, id := range []uint64{1, 2} {
		if got := ex.Clients[id].PerpReserved["USD"]; got != 0 {
			t.Fatalf("client %d reserved after expiry = %d, want 0", id, got)
		}
		if pos := ex.Positions.GetPosition(id, "ABC-FUT-1"); pos != nil && pos.Size != 0 {
			t.Fatalf("client %d position not closed: %+v", id, pos)
		}
	}
	// Forced cancel notification for the resting order. Delivery is
	// asynchronous (gateway outbox), so wait rather than poll.
	select {
	case resp := <-gw1.ResponseCh:
		if _, ok := resp.Data.(*ForcedCancelNotification); !ok {
			t.Fatalf("expected forced cancel, got %#v", resp.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no forced-cancel notification for resting order at expiry")
	}
}

func TestEuropeanOptionITMLifecycle(t *testing.T) {
	clock := &derivClock{now: derivStart}
	ex := newDerivExchange(t, clock)
	defer ex.Shutdown()

	expiry := derivStart + int64(60*time.Second)
	strike := PriceUSD(48_000, DOLLAR_TICK)
	opt := NewEuropeanOption("ABC-1-48000-C", "ABC", "USD", "ABC/USD",
		BTC_PRECISION, USD_PRECISION, USD_PRECISION, 1, strike, expiry, true)
	ex.AddInstrument(opt)
	ex.UpdateDerivativeMarks() // cache underlying mark for the margin formula

	gw1 := connectDerivTrader(ex, 1, 100_000) // buyer
	gw2 := connectDerivTrader(ex, 2, 100_000) // seller

	premium := int64(3000) * USD_PRECISION
	placeLimit(t, gw2, 1, opt.Symbol(), Sell, premium, BTC_PRECISION)
	placeLimit(t, gw1, 2, opt.Symbol(), Buy, premium, BTC_PRECISION)
	<-gw1.ResponseCh
	<-gw2.ResponseCh

	// Premium moved buyer -> seller in the perp wallet.
	if got := ex.Clients[1].PerpBalances["USD"]; got != USDAmount(100_000)-USDAmount(3000) {
		t.Fatalf("buyer balance after premium = %d", got)
	}
	if got := ex.Clients[2].PerpBalances["USD"]; got != USDAmount(100_000)+USDAmount(3000) {
		t.Fatalf("seller balance after premium = %d", got)
	}
	// Buyer holds no margin; seller holds IM at the exec premium.
	if got := ex.Clients[1].PerpReserved["USD"]; got != 0 {
		t.Fatalf("buyer reserved = %d, want 0", got)
	}
	wantIM := opt.MarginForOrder(Sell, BTC_PRECISION, premium, BTC_PRECISION)
	if got := ex.Clients[2].PerpReserved["USD"]; got != wantIM {
		t.Fatalf("seller reserved = %d, want %d", got, wantIM)
	}

	clock.now += int64(30 * time.Second)
	ex.UpdateDerivativeMarks()
	clock.now = expiry + 1
	ex.CheckExpiries()

	// Settlement 50k vs strike 48k: intrinsic $2000/BTC.
	payoff := USDAmount(2000)
	if got := ex.Clients[1].PerpBalances["USD"]; got != USDAmount(100_000)-USDAmount(3000)+payoff {
		t.Fatalf("buyer settled balance = %d", got)
	}
	if got := ex.Clients[2].PerpBalances["USD"]; got != USDAmount(100_000)+USDAmount(3000)-payoff {
		t.Fatalf("seller settled balance = %d", got)
	}
	for _, id := range []uint64{1, 2} {
		if got := ex.Clients[id].PerpReserved["USD"]; got != 0 {
			t.Fatalf("client %d reserved after expiry = %d", id, got)
		}
	}
}

func TestEuropeanOptionOTMExpiresWorthless(t *testing.T) {
	clock := &derivClock{now: derivStart}
	ex := newDerivExchange(t, clock)
	defer ex.Shutdown()

	expiry := derivStart + int64(60*time.Second)
	strike := PriceUSD(55_000, DOLLAR_TICK)
	opt := NewEuropeanOption("ABC-1-55000-C", "ABC", "USD", "ABC/USD",
		BTC_PRECISION, USD_PRECISION, USD_PRECISION, 1, strike, expiry, true)
	ex.AddInstrument(opt)
	ex.UpdateDerivativeMarks()

	gw1 := connectDerivTrader(ex, 1, 100_000)
	gw2 := connectDerivTrader(ex, 2, 100_000)

	premium := int64(500) * USD_PRECISION
	placeLimit(t, gw2, 1, opt.Symbol(), Sell, premium, BTC_PRECISION)
	placeLimit(t, gw1, 2, opt.Symbol(), Buy, premium, BTC_PRECISION)
	<-gw1.ResponseCh
	<-gw2.ResponseCh

	clock.now = expiry + 1
	ex.CheckExpiries()

	if got := ex.Clients[1].PerpBalances["USD"]; got != USDAmount(99_500) {
		t.Fatalf("buyer lost more than premium: %d", got)
	}
	if got := ex.Clients[2].PerpBalances["USD"]; got != USDAmount(100_500) {
		t.Fatalf("seller keeps premium: %d", got)
	}
	if got := ex.Clients[2].PerpReserved["USD"]; got != 0 {
		t.Fatalf("seller margin not released: %d", got)
	}
}

func TestEuropeanOptionCloseBeforeExpiry(t *testing.T) {
	clock := &derivClock{now: derivStart}
	ex := newDerivExchange(t, clock)
	defer ex.Shutdown()

	expiry := derivStart + int64(600*time.Second)
	strike := PriceUSD(48_000, DOLLAR_TICK)
	opt := NewEuropeanOption("ABC-1-48000-C", "ABC", "USD", "ABC/USD",
		BTC_PRECISION, USD_PRECISION, USD_PRECISION, 1, strike, expiry, true)
	ex.AddInstrument(opt)
	ex.UpdateDerivativeMarks()

	gw1 := connectDerivTrader(ex, 1, 100_000)
	gw2 := connectDerivTrader(ex, 2, 100_000)

	// Open at 3000, unwind at 3500: same counterparties, reversed sides.
	open := int64(3000) * USD_PRECISION
	placeLimit(t, gw2, 1, opt.Symbol(), Sell, open, BTC_PRECISION)
	placeLimit(t, gw1, 2, opt.Symbol(), Buy, open, BTC_PRECISION)
	<-gw1.ResponseCh
	<-gw2.ResponseCh

	unwind := int64(3500) * USD_PRECISION
	placeLimit(t, gw1, 3, opt.Symbol(), Sell, unwind, BTC_PRECISION)
	placeLimit(t, gw2, 4, opt.Symbol(), Buy, unwind, BTC_PRECISION)
	<-gw1.ResponseCh
	<-gw2.ResponseCh

	if got := ex.Clients[1].PerpBalances["USD"]; got != USDAmount(100_500) {
		t.Fatalf("buyer round-trip pnl wrong: %d", got)
	}
	if got := ex.Clients[2].PerpBalances["USD"]; got != USDAmount(99_500) {
		t.Fatalf("seller round-trip pnl wrong: %d", got)
	}
	for _, id := range []uint64{1, 2} {
		if got := ex.Clients[id].PerpReserved["USD"]; got != 0 {
			t.Fatalf("client %d margin dust after flat: %d", id, got)
		}
		if pos := ex.Positions.GetPosition(id, opt.Symbol()); pos != nil && pos.Size != 0 {
			t.Fatalf("client %d not flat: %+v", id, pos)
		}
	}
}

func TestOptionChainListerAndDedup(t *testing.T) {
	spec := einstrument.ContractSpec{
		Base: "ABC", Quote: "USD",
		BasePrecision: BTC_PRECISION, QuotePrecision: USD_PRECISION,
		TickSize: USD_PRECISION, MinOrderSize: 1,
	}
	lister := &einstrument.OptionChainLister{
		Underlying: "ABC/USD", Spec: spec,
		TenorsNano:     []int64{int64(10 * time.Minute)},
		StrikeStep:     PriceUSD(1000, DOLLAR_TICK),
		StrikesPerSide: 2,
		IV:             0.9,
	}
	listed := pendingListings(t, lister, derivStart, PriceUSD(50_000, DOLLAR_TICK))
	if len(listed) != 10 { // 5 strikes x C/P
		t.Fatalf("chain size = %d, want 10", len(listed))
	}
	calls := 0
	for _, inst := range listed {
		o := inst.(*einstrument.EuropeanOption)
		if o.IsCall {
			calls++
		}
		if o.IV != 0.9 {
			t.Fatalf("IV not propagated: %v", o.IV)
		}
		if !o.ValidatePrice(USD_PRECISION) {
			t.Fatal("single-tick premium must validate")
		}
	}
	if calls != 5 {
		t.Fatalf("calls = %d, want 5", calls)
	}
	if again := pendingListings(t, lister, derivStart, PriceUSD(50_000, DOLLAR_TICK)); len(again) != 0 {
		t.Fatalf("dedup failed: %d relisted", len(again))
	}
}

func TestDatedFuturesListerSchedule(t *testing.T) {
	spec := einstrument.ContractSpec{
		Base: "ABC", Quote: "USD",
		BasePrecision: BTC_PRECISION, QuotePrecision: USD_PRECISION,
		TickSize: DOLLAR_TICK, MinOrderSize: 1,
	}
	tenor := int64(10 * time.Minute)
	lister := &einstrument.DatedFuturesLister{
		Underlying: "ABC/USD", Spec: spec, TenorsNano: []int64{tenor},
	}
	first := pendingListings(t, lister, derivStart, 0)
	if len(first) != 1 {
		t.Fatalf("first listing count = %d, want 1", len(first))
	}
	fut := first[0].(*einstrument.ExpiringFutures)
	if got, want := fut.ExpiryNano(), derivStart+tenor; got != want {
		t.Fatalf("expiry = %d, want exact tenor expiry %d", got, want)
	}
	if again := pendingListings(t, lister, derivStart, 0); len(again) != 0 {
		t.Fatalf("relisted same expiry: %d", len(again))
	}
	// Once the contract expires a new exact-tenor generation appears.
	if next := pendingListings(t, lister, fut.ExpiryNano(), 0); len(next) != 1 {
		t.Fatalf("next tenor not listed: %d", len(next))
	} else if got, want := next[0].(*einstrument.ExpiringFutures).ExpiryNano(), fut.ExpiryNano()+tenor; got != want {
		t.Fatalf("successor expiry = %d, want %d", got, want)
	}
}

func TestListingTenorIsNeverShortenedByCalendarAlignment(t *testing.T) {
	spec := einstrument.ContractSpec{
		Base: "ABC", Quote: "USD",
		BasePrecision: BTC_PRECISION, QuotePrecision: USD_PRECISION,
		TickSize: DOLLAR_TICK, MinOrderSize: 1,
	}
	tenor := int64(10 * time.Minute)
	now := derivStart + int64(123*time.Second)
	deadline := now + tenor

	futures := pendingListings(t, &einstrument.DatedFuturesLister{Underlying: "ABC/USD", Spec: spec, TenorsNano: []int64{tenor}}, now, 0)
	if len(futures) != 1 {
		t.Fatalf("future listings = %d, want 1", len(futures))
	}
	if expiry := futures[0].(*einstrument.ExpiringFutures).ExpiryNano(); expiry != deadline {
		t.Fatalf("future expiry = %d, want exact tenor expiry %d", expiry, deadline)
	}

	options := pendingListings(t, &einstrument.OptionChainLister{
		Underlying: "ABC/USD", Spec: spec, TenorsNano: []int64{tenor}, StrikeStep: PriceUSD(1000, DOLLAR_TICK),
	}, now, PriceUSD(50_000, DOLLAR_TICK))
	if len(options) == 0 {
		t.Fatal("option chain was not listed")
	}
	if expiry := options[0].(*einstrument.EuropeanOption).ExpiryNano(); expiry != deadline {
		t.Fatalf("option expiry = %d, want exact tenor expiry %d", expiry, deadline)
	}

	if got := pendingListings(t, &einstrument.DatedFuturesLister{Underlying: "ABC/USD", Spec: spec, TenorsNano: []int64{tenor}}, math.MaxInt64-1, 0); len(got) != 0 {
		t.Fatalf("overflowing future listing = %d, want 0", len(got))
	}
}

func TestInstrumentAnnouncementsAndQuery(t *testing.T) {
	clock := &derivClock{now: derivStart}
	ex := newDerivExchange(t, clock)
	defer ex.Shutdown()

	spec := einstrument.ContractSpec{
		Base: "ABC", Quote: "USD",
		BasePrecision: BTC_PRECISION, QuotePrecision: USD_PRECISION,
		TickSize: DOLLAR_TICK, MinOrderSize: 1,
	}
	ex.ConfigureAutomation(AutomationConfig{
		IndexProvider: constPrice(0),
		ListingPolicies: []ListingPolicy{&einstrument.DatedFuturesLister{
			Underlying: "ABC/USD", Spec: spec, TenorsNano: []int64{int64(10 * time.Minute)},
		}},
	})

	gw := connectDerivTrader(ex, 1, 1000)
	gw.RequestCh <- Request{Type: ReqSubscribe, QueryReq: &QueryRequest{
		RequestID: 1, Symbol: InstrumentFeedSymbol, Types: []MDType{MDInstrument},
	}}
	if resp := <-gw.ResponseCh; !resp.Success {
		t.Fatalf("subscribe to %s failed: %v", InstrumentFeedSymbol, resp.Error)
	}

	ex.CheckListings()

	select {
	case md := <-gw.MarketDataCh():
		if md.Type != MDInstrument {
			t.Fatalf("md type = %v, want MDInstrument", md.Type)
		}
		ann := md.Data.(*InstrumentAnnouncement)
		if ann.Action != "listed" || ann.InstrumentType != "FUTURE" || ann.Underlying != "ABC/USD" {
			t.Fatalf("bad announcement: %+v", ann)
		}
	case <-time.After(time.Second):
		t.Fatal("no listing announcement received")
	}

	gw.RequestCh <- Request{Type: ReqQueryInstruments, QueryReq: &QueryRequest{RequestID: 2}}
	resp := <-gw.ResponseCh
	if !resp.Success {
		t.Fatal("QueryInstruments failed")
	}
	instruments := resp.Data.([]*etypes.InstrumentAnnouncement)
	var haveSpot, haveFut bool
	for _, ann := range instruments {
		switch ann.InstrumentType {
		case "SPOT":
			haveSpot = true
		case "FUTURE":
			haveFut = true
		}
	}
	if !haveSpot || !haveFut {
		t.Fatalf("query missing instruments: %+v", instruments)
	}
}

func TestSettlementObserverTWAP(t *testing.T) {
	expiry := derivStart + int64(60*time.Second)
	fut := NewExpiringFutures("F", "ABC", "USD", BTC_PRECISION, USD_PRECISION, DOLLAR_TICK, 1, expiry)
	fut.SetObservationWindow(int64(30 * time.Second))

	// Early sample falls outside the window and must be dropped.
	fut.ObserveSettlement(PriceUSD(10_000, DOLLAR_TICK), derivStart)
	fut.ObserveSettlement(PriceUSD(50_000, DOLLAR_TICK), expiry-int64(20*time.Second))
	fut.ObserveSettlement(PriceUSD(51_000, DOLLAR_TICK), expiry-int64(10*time.Second))
	fut.ObserveSettlement(PriceUSD(52_000, DOLLAR_TICK), expiry)

	want := PriceUSD(51_000, DOLLAR_TICK)
	if got, err := fut.SettlementPrice(); err != nil || got != want {
		t.Fatalf("settlement TWAP = (%d, %v), want (%d, nil)", got, err, want)
	}
	// Frozen after first read.
	fut.ObserveSettlement(PriceUSD(90_000, DOLLAR_TICK), expiry)
	if got, err := fut.SettlementPrice(); err != nil || got != want {
		t.Fatalf("settlement price not frozen: (%d, %v)", got, err)
	}
}
