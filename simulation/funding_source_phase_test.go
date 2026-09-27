package simulation

import (
	"context"
	"testing"
	"time"

	"exchange_sim/exchange"
)

type fundingPhaseAppendFunc func(exchange.FundingBookObservation) (uint64, error)

func (appendObservation fundingPhaseAppendFunc) AppendFundingObservation(observation exchange.FundingBookObservation) (uint64, error) {
	return appendObservation(observation)
}

func TestFundingSourcePreInstantIncludesOffGridButExcludesSameTimeIngress(t *testing.T) {
	for _, test := range []struct {
		name                string
		arrival             time.Duration
		availableAtBoundary bool
	}{
		{name: "earlier-off-grid-trade", arrival: 1500 * time.Microsecond},
		{name: "same-time-trade", arrival: 2 * time.Millisecond, availableAtBoundary: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := NewSimulatedClock(0)
			scheduler := NewEventScheduler(clock)
			clock.SetScheduler(scheduler)
			timers := NewSimTimerFactory(scheduler)
			venue := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
				ID: "N", Clock: clock, TickerFactory: timers, DeterministicPhases: true,
			})
			t.Cleanup(venue.Shutdown)
			venue.AddInstrument(exchange.NewSpotInstrument("ABC-USD", "ABC", "USD",
				exchange.BTC_PRECISION, exchange.USD_PRECISION,
				exchange.DOLLAR_TICK, exchange.BTC_PRECISION/10))
			perp := exchange.NewPerpFutures("ABC-PERP", "ABC", "USD",
				exchange.BTC_PRECISION, exchange.USD_PRECISION,
				exchange.DOLLAR_TICK, exchange.BTC_PRECISION/10)
			perp.GetFundingRate().MarkPrice = 100 * exchange.USD_PRECISION
			perp.GetFundingRate().MarkAvailable = true
			venue.AddInstrument(perp)
			mount := NewMount(venue, LatencyConfig{})
			quantity := int64(exchange.BTC_PRECISION / 10)
			seedOrders := []struct {
				clientID uint64
				request  exchange.OrderRequest
			}{
				{1, exchange.OrderRequest{Symbol: "ABC-USD", Side: exchange.Buy, Type: exchange.LimitOrder,
					Price: 99 * exchange.USD_PRECISION, Qty: quantity, TimeInForce: exchange.GTC}},
				{2, exchange.OrderRequest{Symbol: "ABC-USD", Side: exchange.Sell, Type: exchange.LimitOrder,
					Price: 101 * exchange.USD_PRECISION, Qty: quantity, TimeInForce: exchange.GTC}},
				{3, exchange.OrderRequest{Symbol: "ABC-PERP", Side: exchange.Buy, Type: exchange.LimitOrder,
					Price: 103 * exchange.USD_PRECISION, Qty: quantity, TimeInForce: exchange.GTC}},
				{4, exchange.OrderRequest{Symbol: "ABC-PERP", Side: exchange.Sell, Type: exchange.LimitOrder,
					Price: 105 * exchange.USD_PRECISION, Qty: quantity, TimeInForce: exchange.GTC}},
			}
			seedActors := make([]*phaseOrderActor, 0, len(seedOrders))
			for _, order := range seedOrders {
				gateway := mount.ConnectNewClient(order.clientID, map[string]int64{
					"ABC": 10 * exchange.BTC_PRECISION,
					"USD": 100_000 * exchange.USD_PRECISION,
				}, &exchange.PercentageFee{})
				venue.AddPerpBalance(order.clientID, "USD", 100_000*exchange.USD_PRECISION)
				request := order.request
				var maker *phaseOrderActor
				maker = newPhaseOrderActor(order.clientID, gateway, func() {
					maker.SubmitOrder(request.Symbol, request.Side, request.Type, request.Price, request.Qty)
				})
				seedActors = append(seedActors, maker)
			}
			takerGateway := mount.ConnectNewClient(5, map[string]int64{
				"USD": 100_000 * exchange.USD_PRECISION,
			}, &exchange.PercentageFee{})
			taker := newPhaseOrderActor(5, takerGateway, func() {})
			scheduler.Schedule(int64(test.arrival), func() {
				taker.SubmitOrder("ABC-USD", exchange.Buy, exchange.Market, 0, quantity)
			})
			sequence := uint64(0)
			recorder, err := exchange.NewFundingWindowRecorder(exchange.FundingWindowSourceConfig{
				VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP",
				FirstBoundaryNano: int64(time.Millisecond), SampleSpacingNano: int64(time.Millisecond), SampleCount: 1,
			}, venue, fundingPhaseAppendFunc(func(exchange.FundingBookObservation) (uint64, error) {
				sequence++
				return sequence, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			runner := NewRunner(clock, RunnerConfig{Iterations: 2, Step: time.Millisecond,
				DeterministicPhases: true, DrainIntermediateEvents: true,
				BeforeTimestampPhase: func(atNano int64) error {
					if atNano < int64(time.Millisecond) {
						return nil
					}
					_, err := recorder.ObserveAt(atNano)
					return err
				},
			})
			runner.AddMount(mount)
			runner.AddIdler(timers)
			for _, maker := range seedActors {
				runner.AddActor(maker)
			}
			runner.AddActor(taker)
			if err := runner.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			window, err := recorder.WindowBefore(int64(2 * time.Millisecond))
			if err != nil || len(window.Samples) != 1 || window.Samples[0].TimestampNano != int64(time.Millisecond) {
				t.Fatalf("earlier complete source window changed: %+v, %v", window, err)
			}
			if _, err := venue.CaptureFundingBookPair(exchange.FundingBookPairRequest{
				VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP", TimestampNano: int64(2 * time.Millisecond),
			}); err == nil {
				t.Fatal("same/off-grid taker did not remove the spot ask by run end")
			}
			if latest, err := recorder.WindowBefore(int64(3 * time.Millisecond)); err != nil ||
				(len(latest.Unavailable) == 0) != test.availableAtBoundary {
				t.Fatalf("wrong source frontier at 2ms: %+v, %v", latest, err)
			}
		})
	}
}
