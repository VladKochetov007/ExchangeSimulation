package exchange

import (
	"errors"
	"math"
	"testing"
	"time"

	einstrument "exchange_sim/instrument"
	etypes "exchange_sim/types"
)

func newBookPriceExchange(t *testing.T) *DefaultExchange {
	t.Helper()
	ex := NewExchange(4, &RealClock{})
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	return ex
}

func addBookPriceQuote(t *testing.T, ex *DefaultExchange, side Side, price int64) {
	t.Helper()
	book := ex.Books["ABC/USD"]
	orders := book.Bids
	if side == Sell {
		orders = book.Asks
	}
	order := &Order{ID: uint64(len(orders.Orders) + 1), Side: side, Price: price, Qty: 1, Visibility: Normal}
	if !orders.AddOrder(order) {
		t.Fatalf("add %s quote %d", side, price)
	}
}

func TestBookMidPriceLockedArithmeticAndAbsence(t *testing.T) {
	tests := []struct {
		name    string
		bid     int64
		ask     int64
		want    int64
		wantErr bool
	}{
		{name: "missing book", wantErr: true},
		{name: "empty book", wantErr: true},
		{name: "bid only", bid: 100, wantErr: true},
		{name: "ask only", ask: 101, wantErr: true},
		{name: "ordinary positive", bid: 100, ask: 104, want: 102},
		{name: "odd spread floors", bid: 100, ask: 101, want: 100},
		{name: "equal prices", bid: 101, ask: 101, want: 101},
		{name: "large int64 prices", bid: math.MaxInt64 - 2, ask: math.MaxInt64, want: math.MaxInt64 - 1},
		{name: "crossed book is unusable", bid: 101, ask: 100, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ex := NewExchange(4, &RealClock{})
			defer ex.Shutdown()
			if test.name != "missing book" {
				ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
				if test.bid != 0 {
					addBookPriceQuote(t, ex, Buy, test.bid)
				}
				if test.ask != 0 {
					addBookPriceQuote(t, ex, Sell, test.ask)
				}
			}

			got, err := ex.bookMidPrice("ABC/USD")
			if test.wantErr {
				if !errors.Is(err, ErrNoBookPrice) {
					t.Fatalf("bookMidPrice error = %v, want ErrNoBookPrice", err)
				}
				if got != 0 {
					t.Fatalf("bookMidPrice absent value = %d, want 0 only with error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("bookMidPrice = (%d, %v), want (%d, nil)", got, err, test.want)
			}
		})
	}
}

func TestBookMidPriceArithmeticUsesAdmittedPriceDomain(t *testing.T) {
	inst := NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	for _, price := range []int64{1, math.MaxInt64 - 2, math.MaxInt64} {
		if !inst.ValidatePrice(price) {
			t.Fatalf("positive tick-aligned price %d was not admitted", price)
		}
	}
	for _, price := range []int64{0, -1} {
		if inst.ValidatePrice(price) {
			t.Fatalf("non-positive price %d was admitted", price)
		}
	}
}

func TestBookMidPriceLockedSupportsFullSignedArithmetic(t *testing.T) {
	tests := []struct {
		name string
		bid  int64
		ask  int64
		want int64
	}{
		{name: "entirely negative", bid: -20, ask: -10, want: -15},
		{name: "negative to zero odd", bid: -5, ask: 0, want: -2},
		{name: "spans zero odd", bid: -5, ask: 4, want: 0},
		{name: "zero to positive", bid: 0, ask: 5, want: 2},
		{name: "min to max", bid: math.MinInt64, ask: math.MaxInt64, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := newBookPriceExchange(t)
			defer ex.Shutdown()
			addBookPriceQuote(t, ex, Buy, tc.bid)
			addBookPriceQuote(t, ex, Sell, tc.ask)
			got, err := ex.bookMidPrice("ABC/USD")
			if err != nil || got != tc.want {
				t.Fatalf("bookMidPrice = (%d, %v), want (%d, nil)", got, err, tc.want)
			}
		})
	}
}

func TestBookReferencePriceMakesSignedOneSidedPolicyExplicit(t *testing.T) {
	tests := []struct {
		name  string
		side  Side
		price int64
	}{
		{name: "negative bid", side: Buy, price: -20},
		{name: "zero ask", side: Sell, price: 0},
		{name: "positive ask", side: Sell, price: 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ex := newBookPriceExchange(t)
			defer ex.Shutdown()
			addBookPriceQuote(t, ex, tc.side, tc.price)
			got, err := ex.bookReferencePrice("ABC/USD")
			if err != nil || got != tc.price {
				t.Fatalf("bookReferencePrice = (%d, %v), want (%d, nil)", got, err, tc.price)
			}
		})
	}
}

func TestBookReferencePriceMakesOneSidedPolicyExplicit(t *testing.T) {
	tests := []struct {
		name    string
		bid     int64
		ask     int64
		want    int64
		wantErr bool
	}{
		{name: "empty", wantErr: true},
		{name: "bid reference", bid: 100, want: 100},
		{name: "ask reference", ask: 101, want: 101},
		{name: "two-sided midpoint", bid: 100, ask: 101, want: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ex := newBookPriceExchange(t)
			defer ex.Shutdown()
			if test.bid != 0 {
				addBookPriceQuote(t, ex, Buy, test.bid)
			}
			if test.ask != 0 {
				addBookPriceQuote(t, ex, Sell, test.ask)
			}
			got, err := ex.bookReferencePrice("ABC/USD")
			if test.wantErr {
				if !errors.Is(err, ErrNoBookPrice) {
					t.Fatalf("bookReferencePrice error = %v, want ErrNoBookPrice", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("bookReferencePrice = (%d, %v), want (%d, nil)", got, err, test.want)
			}
		})
	}
}

func TestCheckListingsDefersOptionChainWithoutTrueMidpoint(t *testing.T) {
	ex := newBookPriceExchange(t)
	defer ex.Shutdown()
	ex.ConfigureAutomation(AutomationConfig{ListingPolicies: []ListingPolicy{&einstrument.OptionChainLister{
		Underlying: "ABC/USD",
		Spec: einstrument.ContractSpec{
			Base: "ABC", Quote: "USD", BasePrecision: 1, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1,
		},
		TenorsNano: []int64{int64(time.Hour)}, StrikeStep: 10, StrikesPerSide: 0,
	}}})

	addBookPriceQuote(t, ex, Buy, 100)
	ex.CheckListings()
	if got := len(ex.ListInstruments("", "")); got != 1 {
		t.Fatalf("one-sided book listed option chain: instruments=%d", got)
	}

	addBookPriceQuote(t, ex, Sell, 102)
	ex.CheckListings()
	if got := len(ex.ListInstruments("", "")); got != 3 {
		t.Fatalf("two-sided book did not release one-strike option chain: instruments=%d", got)
	}
}

func TestDerivativeMarksDeferEmptyBookAndUseDeclaredReferencePolicy(t *testing.T) {
	ex := newBookPriceExchange(t)
	defer ex.Shutdown()
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, time.Now().Add(time.Hour).UnixNano())
	future.Underlying = "ABC/USD"
	ex.AddInstrument(future)

	ex.UpdateDerivativeMarks()
	if got, err := future.SettlementPrice(); !errors.Is(err, ErrNoBookPrice) || got != 0 {
		t.Fatalf("empty underlying settlement = (%d, %v), want ErrNoBookPrice", got, err)
	}

	addBookPriceQuote(t, ex, Buy, 100)
	ex.UpdateDerivativeMarks()
	if got, err := future.SettlementPrice(); err != nil || got != 100 {
		t.Fatalf("one-sided declared reference settlement = (%d, %v), want (100, nil)", got, err)
	}
}

func TestIndexPriceLockedPropagatesMissingUnderlying(t *testing.T) {
	ex := newBookPriceExchange(t)
	defer ex.Shutdown()
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, time.Now().Add(time.Hour).UnixNano())
	future.Underlying = "ABC/USD"
	ex.AddInstrument(future)

	ex.mu.RLock()
	price, err := ex.indexPriceLocked(future.Symbol())
	ex.mu.RUnlock()
	if price != 0 || !errors.Is(err, ErrNoBookPrice) {
		t.Fatalf("missing underlying index = (%d, %v), want ErrNoBookPrice", price, err)
	}
}

func TestUpdatePerpPricesDefersUnavailableIndexThenUsesDeclaredReference(t *testing.T) {
	ex := newBookPriceExchange(t)
	defer ex.Shutdown()
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, time.Now().Add(time.Hour).UnixNano())
	future.Underlying = "ABC/USD"
	ex.AddInstrument(future)
	ex.ConfigureAutomation(AutomationConfig{})

	ex.UpdatePerpPrices()
	if funding := future.GetFundingRate(); funding.MarkAvailable || funding.IndexAvailable || funding.MarkPrice != 0 || funding.IndexPrice != 0 {
		t.Fatalf("unavailable underlying funding state = %#v, want unavailable", funding)
	}

	addBookPriceQuote(t, ex, Buy, 100)
	ex.UpdatePerpPrices()
	funding := future.GetFundingRate()
	if !funding.IndexAvailable || !funding.MarkAvailable || funding.IndexPrice != 100 || funding.MarkPrice != 100 {
		t.Fatalf("one-sided declared reference update = index %d mark %d, want 100/100", funding.IndexPrice, funding.MarkPrice)
	}
}

type expiryManualClock struct{ now int64 }

func (c *expiryManualClock) NowUnixNano() int64 { return c.now }
func (c *expiryManualClock) NowUnix() int64     { return c.now / int64(time.Second) }

func (c *expiryManualClock) Advance(d time.Duration) { c.now += int64(d) }

func TestExpirySettlementPendingRetriesThenSettlesExactlyOnce(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(4, clock)
	defer ex.Shutdown()
	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, clock.now)
	future.Underlying = "ABC/USD"
	ex.AddInstrument(future)

	log := &recordingLogger{}
	global := &recordingLogger{}
	ex.SetLogger(future.Symbol(), log)
	ex.SetLogger("_global", global)
	for _, id := range []uint64{1, 2} {
		ex.ConnectNewClient(id, nil, &FixedFee{})
		ex.AddPerpBalance(id, "USD", 1_000)
	}
	ex.Positions.UpdatePosition(1, future.Symbol(), 10, 90, Buy, PositionBoth)
	ex.Positions.UpdatePosition(2, future.Symbol(), 10, 90, Sell, PositionBoth)
	openingTotal := ex.Clients[1].PerpBalance("USD") + ex.Clients[2].PerpBalance("USD")
	// Make a visible resting order to prove first expiry halts and cancels the
	// book even though cash settlement has to wait.
	resting := &Order{ID: 99, ClientID: 1, Side: Buy, Type: LimitOrder, Price: 90, Qty: 1, Status: Open, Visibility: Normal}
	if !ex.Books[future.Symbol()].Bids.AddOrder(resting) {
		t.Fatal("could not seed resting pre-expiry order")
	}
	ex.Clients[1].AddOrder(resting.ID)

	ex.CheckExpiries()
	pending, ok := ex.settlementPending[future.Symbol()]
	if !ok || pending.State != expiryStateSettlementPending || pending.Attempts != 1 || pending.Policy != expiryUnavailableRetryForever {
		t.Fatalf("first unavailable expiry state = %#v", pending)
	}
	if future.GetFundingRate().MarkAvailable {
		t.Fatal("pending expiry retained a live mark after the lifecycle boundary")
	}
	if ex.Instruments[future.Symbol()] == nil || ex.Books[future.Symbol()] == nil {
		t.Fatal("unavailable settlement delisted the contract")
	}
	if ex.Books[future.Symbol()].FindOrder(resting.ID) != nil || resting.Status != Cancelled {
		t.Fatalf("expiry-pending contract retained resting order: %#v", resting)
	}
	if response := ex.PlaceOrder(1, &OrderRequest{RequestID: 1, Symbol: future.Symbol(), Side: Buy, Type: LimitOrder, Price: 90, Qty: 1, TimeInForce: GTC, Visibility: Normal}); response.Success || response.Error != RejectInstrumentExpired {
		t.Fatalf("post-expiry order = %#v, want instrument-expired rejection", response)
	}
	for _, id := range []uint64{1, 2} {
		if pos := ex.Positions.GetPosition(id, future.Symbol()); pos == nil || pos.Size == 0 {
			t.Fatalf("client %d position closed while settlement unavailable: %#v", id, pos)
		}
	}

	// Multiple deterministic retry intervals do not release collateral,
	// positions, or the old book; they only make the pending reason/attempt
	// observable. A dated future has no funding, but its mark state must also
	// not be refreshed after expiry merely because it awaits settlement.
	future.GetFundingRate().MarkPrice = 77
	future.GetFundingRate().MarkAvailable = true
	for i := 0; i < 2; i++ {
		clock.Advance(time.Second)
		ex.UpdatePerpPrices()
		ex.CheckExpiries()
	}
	pending = ex.settlementPending[future.Symbol()]
	if pending.Attempts != 3 || future.GetFundingRate().MarkAvailable || future.GetFundingRate().MarkPrice != 77 {
		t.Fatalf("pending retries = %#v mark=%d available=%t, want attempts=3 and unavailable mark", pending, future.GetFundingRate().MarkPrice, future.GetFundingRate().MarkAvailable)
	}
	if total := ex.Clients[1].PerpBalance("USD") + ex.Clients[2].PerpBalance("USD"); total != openingTotal {
		t.Fatalf("pending settlement changed conservation total: got %d want %d", total, openingTotal)
	}

	// The declared one-sided derivative reference is permitted. Recovery is
	// sampled, then the next expiry retry settles exactly once.
	addBookPriceQuote(t, ex, Buy, 120)
	ex.UpdateDerivativeMarks()
	ex.CheckExpiries()
	if ex.Instruments[future.Symbol()] != nil || ex.Books[future.Symbol()] != nil {
		t.Fatal("recovered settlement did not delist contract")
	}
	for _, id := range []uint64{1, 2} {
		if pos := ex.Positions.GetPosition(id, future.Symbol()); pos == nil || pos.Size != 0 {
			t.Fatalf("client %d position after recovery = %#v, want closed", id, pos)
		}
	}
	if total := ex.Clients[1].PerpBalance("USD") + ex.Clients[2].PerpBalance("USD"); total != openingTotal {
		t.Fatalf("settlement broke conservation total: got %d want %d", total, openingTotal)
	}

	// A later expiry check cannot settle or release again: the terminal book is
	// gone and exactly one lifecycle announcement was emitted.
	clock.Advance(time.Second)
	ex.CheckExpiries()
	settled := 0
	for _, record := range global.records {
		if record.event == "instrument_settled" {
			settled++
		}
	}
	if settled != 1 {
		t.Fatalf("instrument_settled count = %d, want exactly one", settled)
	}
	for _, record := range global.records {
		if record.event != "instrument_settled" {
			continue
		}
		announcement, ok := record.data.(*etypes.InstrumentAnnouncement)
		if !ok || announcement.SettlementPrice == nil || *announcement.SettlementPrice != 120 || announcement.ListedNano == nil || *announcement.ListedNano != 100 {
			t.Fatalf("settlement lifecycle evidence = %#v, want present price 120", record.data)
		}
	}
	pendingEvents, unavailableEvents := 0, 0
	for _, record := range log.records {
		switch record.event {
		case "expiry_settlement_pending":
			pendingEvents++
			if event, ok := record.data.(ExpirySettlementPendingEvent); !ok || event.State != string(expiryStateSettlementPending) || event.Policy != expiryUnavailableRetryForever || event.Reason == "" {
				t.Fatalf("pending evidence = %#v", record.data)
			}
		case "price_unavailable":
			unavailableEvents++
		}
	}
	if pendingEvents != 3 || unavailableEvents != 3 {
		t.Fatalf("pending evidence count = pending %d unavailable %d, want 3/3", pendingEvents, unavailableEvents)
	}
}

func TestInstrumentReplayRetainsOriginalListingTime(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(1, clock)
	defer ex.Shutdown()
	future := NewExpiringFutures("ABC-FUT", "ABC", "USD", 1, 1, 1, 1, 8*int64(time.Hour)+clock.now)
	future.Underlying = "ABC/USD"
	ex.AddInstrument(future)

	clock.Advance(5 * time.Minute)
	gateway := ex.ConnectNewClient(1, nil, &FixedFee{}).(*ClientGateway)
	response := ex.Subscribe(1, &QueryRequest{RequestID: 1, Symbol: InstrumentFeedSymbol, Types: []MDType{MDInstrument}}, gateway)
	if !response.Success {
		t.Fatalf("subscribe: %#v", response)
	}
	select {
	case message := <-gateway.MarketDataCh():
		announcement, ok := message.Data.(*etypes.InstrumentAnnouncement)
		if !ok {
			t.Fatalf("replay data = %T", message.Data)
		}
		if message.Timestamp != clock.now || announcement.Timestamp != clock.now {
			t.Fatalf("replay time = message %d announcement %d, want %d", message.Timestamp, announcement.Timestamp, clock.now)
		}
		if announcement.ListedNano == nil || *announcement.ListedNano != 100 {
			t.Fatalf("original listing time = %v, want 100", announcement.ListedNano)
		}
		if announcement.ExpiryNano-*announcement.ListedNano != 8*int64(time.Hour) {
			t.Fatalf("original tenor = %d, want %d", announcement.ExpiryNano-*announcement.ListedNano, 8*int64(time.Hour))
		}
	default:
		t.Fatal("instrument replay was not delivered")
	}
}

func TestOptionExpiryDefersDomainInvalidForwardThenSettlesOnce(t *testing.T) {
	clock := &expiryManualClock{now: 100}
	ex := NewExchange(2, clock)
	defer ex.Shutdown()
	option := NewEuropeanOption("OIL-100-C", "OIL", "USD", "OIL/USD", 1, 1, 1, 1, 100, clock.now, true)
	option.SetObservationWindow(0)
	// This is a delivered, present observation. It is not an unavailable
	// source, but the current Black-76 option contract rejects it explicitly.
	option.ObserveSettlement(0, clock.now)
	ex.AddInstrument(option)
	global := &recordingLogger{}
	ex.SetLogger("_global", global)
	for _, id := range []uint64{1, 2} {
		ex.ConnectNewClient(id, nil, &FixedFee{})
		ex.AddPerpBalance(id, "USD", 1_000)
	}
	ex.Positions.UpdatePosition(1, option.Symbol(), 1, 10, Buy, PositionBoth)
	ex.Positions.UpdatePosition(2, option.Symbol(), 1, 10, Sell, PositionBoth)
	opening := ex.Clients[1].PerpBalance("USD") + ex.Clients[2].PerpBalance("USD")

	ex.CheckExpiries()
	pending, ok := ex.settlementPending[option.Symbol()]
	_, settlementErr := option.SettlementPrice()
	if !ok || pending.Attempts != 1 || !errors.Is(settlementErr, etypes.ErrPriceDomain) {
		// The pending reason is a persisted diagnostic; keep the contract test
		// independent of its wording while still proving this is domain, not
		// availability, deferral.
		t.Fatalf("invalid option forward was not observable as pending domain deferral: %#v", pending)
	}
	if response := ex.PlaceOrder(1, &OrderRequest{RequestID: 1, Symbol: option.Symbol(), Side: Buy, Type: LimitOrder, Price: 1, Qty: 1, TimeInForce: GTC, Visibility: Normal}); response.Success || response.Error != RejectInstrumentExpired {
		t.Fatalf("post-expiry option order = %#v, want instrument-expired rejection", response)
	}
	if total := ex.Clients[1].PerpBalance("USD") + ex.Clients[2].PerpBalance("USD"); total != opening {
		t.Fatalf("domain-pending option settlement changed conservation: got %d want %d", total, opening)
	}

	clock.Advance(time.Second)
	option.ObserveSettlement(120, clock.now)
	ex.CheckExpiries()
	if ex.Instruments[option.Symbol()] != nil || ex.Books[option.Symbol()] != nil {
		t.Fatal("option was not delisted after valid settlement reference arrived")
	}
	for _, id := range []uint64{1, 2} {
		if pos := ex.Positions.GetPosition(id, option.Symbol()); pos == nil || pos.Size != 0 {
			t.Fatalf("client %d option position after delayed settlement = %#v", id, pos)
		}
	}
	if total := ex.Clients[1].PerpBalance("USD") + ex.Clients[2].PerpBalance("USD"); total != opening {
		t.Fatalf("delayed option settlement changed conservation: got %d want %d", total, opening)
	}
	settled := 0
	for _, record := range global.records {
		if record.event == "instrument_settled" {
			settled++
		}
	}
	if settled != 1 {
		t.Fatalf("delayed option instrument_settled count = %d, want 1", settled)
	}
}
