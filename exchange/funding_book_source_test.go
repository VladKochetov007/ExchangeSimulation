package exchange

import (
	"errors"
	"testing"
)

type fundingSourceClock struct{ now int64 }

func (clock *fundingSourceClock) NowUnixNano() int64 { return clock.now }
func (clock *fundingSourceClock) NowUnix() int64     { return clock.now / 1_000_000_000 }

func TestCaptureFundingBookPairUsesOneVenueLockedVisibleSource(t *testing.T) {
	ex, clock := fundingBookPairFixture(t, true)
	clock.now = 20
	pair, err := ex.CaptureFundingBookPair(FundingBookPairRequest{
		VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP", TimestampNano: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pair.VenueID != "N" || pair.BaseAsset != "ABC" || pair.QuoteAsset != "USD" ||
		pair.BasePrecision != 1 || pair.QuotePrecision != 1 ||
		pair.Spot.MidPrice != 100 || pair.Perp.MidPrice != 104 {
		t.Fatalf("wrong venue, instrument or positive-domain midpoints: %+v", pair)
	}
	if pair.Spot.Bid.VisibleQty != 1 || pair.Spot.Bid.OldestVisibleOrderAcceptedAtNano != 10 ||
		pair.Spot.Bid.OldestVisibleOrderAgeNano != 10 ||
		pair.Spot.Ask.VisibleQty != 1 || pair.Perp.Bid.VisibleQty != 1 {
		t.Fatalf("hidden depth or quote age was misrepresented: %+v", pair)
	}
	sample := pair.RateWindowSample()
	if sample.TimestampNano != 20 || sample.IndexPrice != 100 || sample.MarkPrice != 104 {
		t.Fatalf("wrong numeric funding sample: %+v", sample)
	}
}

func TestCaptureFundingBookPairRejectsWrongSourceAndIncompleteBooks(t *testing.T) {
	for _, test := range []struct {
		name          string
		spotAsk       bool
		request       FundingBookPairRequest
		mutate        func(*DefaultExchange, *fundingSourceClock)
		isUnavailable bool
	}{
		{"wrong-venue", true, FundingBookPairRequest{"S", "ABC-USD", "ABC-PERP", 20}, nil, false},
		{"wrong-time", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 19}, nil, false},
		{"wrong-type", true, FundingBookPairRequest{"N", "ABC-PERP", "ABC-USD", 20}, nil, false},
		{"wrong-underlying", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				ex.Instruments["ABC-PERP"] = NewPerpFutures("ABC-PERP", "CDF", "USD", 1, 1, 1, 1)
				ex.mu.Unlock()
			}, false},
		{"misbound-map-instrument", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				ex.Instruments["ABC-USD"] = NewSpotInstrument("ABC-USD", "ABC", "USD", 1, 1, 2, 1)
				ex.mu.Unlock()
			}, false},
		{"live-quote-invalid-tick", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				stricterTick := NewSpotInstrument("ABC-USD", "ABC", "USD", 1, 1, 2, 1)
				ex.Instruments["ABC-USD"] = stricterTick
				ex.Books["ABC-USD"].Instrument = stricterTick
				ex.mu.Unlock()
			}, false},
		{"misbound-book-symbol", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				ex.Books["ABC-PERP"].Symbol = "OTHER-PERP"
				ex.mu.Unlock()
			}, false},
		{"misbound-book-instrument", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				ex.Books["ABC-PERP"].Instrument = NewPerpFutures("ABC-PERP", "CDF", "USD", 1, 1, 1, 1)
				ex.mu.Unlock()
			}, false},
		{"distinct-book-instrument-with-matching-fields", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				ex.Books["ABC-PERP"].Instrument = NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
				ex.mu.Unlock()
			}, false},
		{"one-sided-spot", false, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20}, nil, true},
		{"same-time-quote", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 10},
			func(_ *DefaultExchange, clock *fundingSourceClock) { clock.now = 10 }, false},
		{"future-quote", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				ex.Books["ABC-USD"].Bids.Best.Head.Timestamp = 30
				ex.mu.Unlock()
			}, false},
		{"crossed-spot", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				ex.Books["ABC-USD"].Bids.Best.Price = 102
				ex.mu.Unlock()
			}, true},
		{"zero-visible-best", true, FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20},
			func(ex *DefaultExchange, _ *fundingSourceClock) {
				ex.mu.Lock()
				ex.Books["ABC-USD"].Bids.Best.Head.DisplayRemaining = 0
				ex.mu.Unlock()
			}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ex, clock := fundingBookPairFixture(t, test.spotAsk)
			clock.now = 20
			if test.mutate != nil {
				test.mutate(ex, clock)
			}
			pair, err := ex.CaptureFundingBookPair(test.request)
			if err == nil || pair != (FundingBookPair{}) {
				t.Fatalf("invalid source returned a pair: %+v, %v", pair, err)
			}
			if errors.Is(err, ErrFundingBookUnavailable) != test.isUnavailable {
				t.Fatalf("wrong structural/unavailable classification: %v", err)
			}
		})
	}
}

func TestCaptureFundingBookPairUsesBestDisplayedLevel(t *testing.T) {
	ex, clock := fundingBookPairFixture(t, true)
	clock.now = 12
	response := ex.PlaceOrder(2, &OrderRequest{Symbol: "ABC-USD", Side: Buy, Type: LimitOrder,
		Price: 100, Qty: 4, Visibility: Hidden, TimeInForce: GTC})
	if !response.Success {
		t.Fatalf("hidden best fixture rejected: %v", response.Error)
	}
	if got := ex.Books["ABC-USD"].Bids.Best.Price; got != 100 {
		t.Fatalf("fixture's executable best bid = %d, want 100", got)
	}
	public := ex.Books["ABC-USD"].Bids.GetPublicSnapshot()
	if len(public) != 1 || public[0].Price != 99 || public[0].VisibleQty != 1 {
		t.Fatalf("fixture public bid differs from expected displayed level: %+v", public)
	}
	clock.now = 20
	pair, err := ex.CaptureFundingBookPair(FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20})
	if err != nil {
		t.Fatal(err)
	}
	if pair.Spot.Bid.Price != 99 || pair.Spot.Bid.VisibleQty != 1 || pair.Spot.MidPrice != 100 {
		t.Fatalf("hidden executable best displaced public funding source: %+v", pair.Spot)
	}
}

func TestCaptureFundingBookPairMixedVisibilityAndIcebergRefreshAge(t *testing.T) {
	ex, clock := fundingBookPairFixture(t, true)
	clock.now = 11
	hidden := ex.PlaceOrder(3, &OrderRequest{Symbol: "ABC-USD", Side: Buy, Type: LimitOrder,
		Price: 99, Qty: 4, Visibility: Hidden, TimeInForce: GTC})
	if !hidden.Success {
		t.Fatalf("hidden same-level fixture rejected: %v", hidden.Error)
	}
	clock.now = 12
	normal := ex.PlaceOrder(4, &OrderRequest{Symbol: "ABC-USD", Side: Buy, Type: LimitOrder,
		Price: 99, Qty: 2, TimeInForce: GTC})
	if !normal.Success {
		t.Fatalf("normal same-level fixture rejected: %v", normal.Error)
	}
	clock.now = 20
	pair, err := ex.CaptureFundingBookPair(FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20})
	if err != nil {
		t.Fatal(err)
	}
	if pair.Spot.Bid.VisibleQty != 3 || pair.Spot.Bid.OldestVisibleOrderAcceptedAtNano != 10 {
		t.Fatalf("mixed level included hidden reserve or lost oldest visible age: %+v", pair.Spot.Bid)
	}

	refreshedExchange, refreshedClock := fundingBookPairFixture(t, true)
	refreshedClock.now = 15
	fill := refreshedExchange.PlaceOrder(2, &OrderRequest{Symbol: "ABC-USD", Side: Sell, Type: LimitOrder,
		Price: 99, Qty: 1, TimeInForce: IOC})
	if !fill.Success {
		t.Fatalf("iceberg-refresh fixture rejected: %v", fill.Error)
	}
	refreshedClock.now = 20
	refreshedPair, err := refreshedExchange.CaptureFundingBookPair(FundingBookPairRequest{"N", "ABC-USD", "ABC-PERP", 20})
	if err != nil {
		t.Fatal(err)
	}
	if refreshedPair.Spot.Bid.VisibleQty != 1 ||
		refreshedPair.Spot.Bid.OldestVisibleOrderAcceptedAtNano != 10 ||
		refreshedPair.Spot.Bid.OldestVisibleOrderAgeNano != 10 {
		t.Fatalf("iceberg refresh must keep original admission age: %+v", refreshedPair.Spot.Bid)
	}
}

func fundingBookPairFixture(t *testing.T, includeSpotAsk bool) (*DefaultExchange, *fundingSourceClock) {
	t.Helper()
	clock := &fundingSourceClock{now: 10}
	ex := NewExchangeWithConfig(ExchangeConfig{ID: "N", Clock: clock})
	t.Cleanup(ex.Shutdown)
	ex.AddInstrument(NewSpotInstrument("ABC-USD", "ABC", "USD", 1, 1, 1, 1))
	perp := NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1)
	perp.GetFundingRate().MarkPrice = 100
	perp.GetFundingRate().MarkAvailable = true
	ex.AddInstrument(perp)
	for clientID := uint64(1); clientID <= 4; clientID++ {
		ex.ConnectNewClient(clientID, map[string]int64{"ABC": 100, "USD": 100_000}, &FixedFee{})
		ex.AddPerpBalance(clientID, "USD", 100_000)
	}
	orders := []struct {
		clientID uint64
		request  OrderRequest
	}{
		{1, OrderRequest{Symbol: "ABC-USD", Side: Buy, Type: LimitOrder, Price: 99, Qty: 10,
			Visibility: Iceberg, IcebergQty: 1, TimeInForce: GTC}},
		{3, OrderRequest{Symbol: "ABC-PERP", Side: Buy, Type: LimitOrder, Price: 103, Qty: 1,
			TimeInForce: GTC}},
		{4, OrderRequest{Symbol: "ABC-PERP", Side: Sell, Type: LimitOrder, Price: 105, Qty: 1,
			TimeInForce: GTC}},
	}
	if includeSpotAsk {
		orders = append(orders, struct {
			clientID uint64
			request  OrderRequest
		}{2, OrderRequest{Symbol: "ABC-USD", Side: Sell, Type: LimitOrder, Price: 101, Qty: 1,
			TimeInForce: GTC}})
	}
	for _, order := range orders {
		response := ex.PlaceOrder(order.clientID, &order.request)
		if !response.Success {
			t.Fatalf("fixture order %s %v rejected: %v", order.request.Symbol, order.request.Side, response.Error)
		}
	}
	return ex, clock
}
