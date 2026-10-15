package me014composition_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
	"exchange_sim/types"
)

type recordedEvent struct {
	name string
	data any
}

type eventRecorder struct {
	events []recordedEvent
}

func (r *eventRecorder) LogEvent(_ int64, _ uint64, name string, data any) {
	r.events = append(r.events, recordedEvent{name: name, data: data})
}

type perpFixture struct {
	instrument       *exchange.PerpFutures
	underlyingSymbol string
}

type exposureFixture struct {
	venueID string
	market  *exchange.DefaultExchange
	perps   []perpFixture
	clients []uint64
	log     *eventRecorder
}

type positionFixture struct {
	clientID uint64
	symbol   string
	quantity int64
	entry    int64
	side     exchange.Side
}

type preparedExposure struct {
	fixture   *exposureFixture
	timestamp int64
	epoch     uint64
	marks     map[string]int64
}

func makeExposureFixture(t *testing.T, venueID string, spots []spotFixture, perps []perpFixtureSpec, positions []positionFixture) *exposureFixture {
	t.Helper()
	clock := simulation.NewSimulatedClock(int64(time.Hour))
	market := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID:                                   venueID,
		Clock:                                clock,
		DeterministicIngress:                 true,
		DeterministicPhases:                  true,
		RequireExactLinearPositionAccounting: true,
	})

	for index, spot := range spots {
		market.AddInstrument(exchange.NewSpotInstrument(spot.symbol, spot.base, "USD", 1, 1, 1, 1))
		bidClientID := uint64(10_000 + index*2)
		askClientID := bidClientID + 1
		market.ConnectNewClient(bidClientID, map[string]int64{"USD": 10_000}, &exchange.FixedFee{})
		bid := market.PlaceOrder(bidClientID, &exchange.OrderRequest{
			RequestID: 1, Symbol: spot.symbol, Side: exchange.Buy, Type: exchange.LimitOrder,
			Price: spot.bid, Qty: 1, TimeInForce: exchange.GTC, Visibility: exchange.Normal,
		})
		if !bid.Success {
			t.Fatalf("seed %s bid on %s: %s", venueID, spot.symbol, bid.Error)
		}
		if spot.hasAsk {
			market.ConnectNewClient(askClientID, map[string]int64{spot.base: 10_000}, &exchange.FixedFee{})
			ask := market.PlaceOrder(askClientID, &exchange.OrderRequest{
				RequestID: 1, Symbol: spot.symbol, Side: exchange.Sell, Type: exchange.LimitOrder,
				Price: spot.ask, Qty: 1, TimeInForce: exchange.GTC, Visibility: exchange.Normal,
			})
			if !ask.Success {
				t.Fatalf("seed %s ask on %s: %s", venueID, spot.symbol, ask.Error)
			}
		}
	}

	fixture := &exposureFixture{venueID: venueID, market: market, log: &eventRecorder{}}
	market.SetLogger("_global", fixture.log)
	for _, perpSpec := range perps {
		perp := exchange.NewPerpFutures(perpSpec.symbol, perpSpec.base, "USD", 1, 1, 1, 1)
		perp.MaintenanceMarginRate = 5_000
		market.AddInstrument(perp)
		fixture.perps = append(fixture.perps, perpFixture{
			instrument:       perp,
			underlyingSymbol: perpSpec.underlyingSymbol,
		})
	}

	for _, position := range positions {
		if _, connected := market.Clients[position.clientID]; !connected {
			market.ConnectNewClient(position.clientID, nil, &exchange.FixedFee{})
			fixture.clients = append(fixture.clients, position.clientID)
		}
		market.Positions.UpdatePosition(position.clientID, position.symbol, position.quantity, position.entry, position.side, exchange.PositionBoth)
	}
	slices.Sort(fixture.clients)
	return fixture
}

type spotFixture struct {
	symbol string
	base   string
	bid    int64
	ask    int64
	hasAsk bool
}

type perpFixtureSpec struct {
	symbol           string
	base             string
	underlyingSymbol string
}

func preflightLocalMidMarks(venues []*exposureFixture) ([]preparedExposure, error) {
	orderedVenues := slices.Clone(venues)
	slices.SortFunc(orderedVenues, func(left, right *exposureFixture) int {
		return compareString(left.venueID, right.venueID)
	})
	if len(orderedVenues) == 0 {
		return nil, errors.New("risk preflight requires at least one venue")
	}

	worldTimestamp := orderedVenues[0].market.Clock.NowUnixNano()
	prepared := make([]preparedExposure, len(orderedVenues))
	for venueIndex, fixture := range orderedVenues {
		if fixture.market.Clock.NowUnixNano() != worldTimestamp {
			return nil, fmt.Errorf("venue %s clock is not at the common risk timestamp", fixture.venueID)
		}
		marks := make(map[string]int64, len(fixture.perps))
		orderedPerps := slices.Clone(fixture.perps)
		slices.SortFunc(orderedPerps, func(left, right perpFixture) int {
			return compareString(left.instrument.Symbol(), right.instrument.Symbol())
		})
		for _, perp := range orderedPerps {
			mark, available := fixture.market.TwoSidedMidPrice(perp.underlyingSymbol)
			if !available || mark <= 0 {
				return nil, fmt.Errorf("venue %s has no admissible public midpoint for %s", fixture.venueID, perp.underlyingSymbol)
			}
			marks[perp.instrument.Symbol()] = mark
		}
		prepared[venueIndex] = preparedExposure{
			fixture: fixture, timestamp: worldTimestamp, marks: marks,
		}
	}

	for _, candidate := range prepared {
		for _, perp := range candidate.fixture.perps {
			mark := candidate.marks[perp.instrument.Symbol()]
			perp.instrument.UpdateMarkReferences(mark, mark)
		}
	}

	for index := range prepared {
		fixture := prepared[index].fixture
		symbols := make([]string, 0, len(fixture.perps))
		for _, perp := range fixture.perps {
			symbols = append(symbols, perp.instrument.Symbol())
		}
		slices.Sort(symbols)
		epoch, err := fixture.market.CommitMarkEpoch(symbols)
		if err != nil {
			return nil, fmt.Errorf("venue %s commit coherent mark epoch: %w", fixture.venueID, err)
		}
		prepared[index].epoch = epoch
	}

	for _, candidate := range prepared {
		for _, clientID := range candidate.fixture.clients {
			if len(candidate.fixture.market.Positions.GetAllPositions(clientID)) == 0 {
				continue
			}
			report, err := candidate.fixture.market.MarkedAccount(clientID, types.AccountValuationSpec{
				ReportAsset: "USD", ReportPrecision: 1,
				AssetMarks: map[string]types.AssetValuationMark{
					"USD": {Price: 1, Precision: 1},
				},
			})
			if err != nil {
				return nil, fmt.Errorf("venue %s account %d risk preflight: %w", candidate.fixture.venueID, clientID, err)
			}
			for _, position := range report.Positions {
				expected, registered := candidate.marks[position.Symbol]
				if !registered || position.MarkPrice == nil || *position.MarkPrice != expected {
					return nil, fmt.Errorf("venue %s account %d mark for %s does not match the preflight epoch", candidate.fixture.venueID, clientID, position.Symbol)
				}
			}
		}
	}

	for _, candidate := range prepared {
		if candidate.fixture.market.Clock.NowUnixNano() != candidate.timestamp {
			return nil, fmt.Errorf("venue %s clock advanced during risk preflight", candidate.fixture.venueID)
		}
	}
	return prepared, nil
}

func sweepPreflighted(prepared []preparedExposure) {
	for _, candidate := range prepared {
		orderedPerps := slices.Clone(candidate.fixture.perps)
		slices.SortFunc(orderedPerps, func(left, right perpFixture) int {
			return compareString(left.instrument.Symbol(), right.instrument.Symbol())
		})
		for _, perp := range orderedPerps {
			candidate.fixture.market.CheckLiquidations(
				perp.instrument.Symbol(), perp.instrument,
				candidate.marks[perp.instrument.Symbol()],
			)
		}
	}
}

func runLocalMidRiskBoundary(venues ...*exposureFixture) error {
	prepared, err := preflightLocalMidMarks(venues)
	if err != nil {
		return err
	}
	sweepPreflighted(prepared)
	return nil
}

func TestConsumerCanComposeLocalMidPreflightBeforeCrossMarginSweep(t *testing.T) {
	fixture := makeExposureFixture(t, "N", []spotFixture{
		{symbol: "ABC/USD", base: "ABC", bid: 99, ask: 101, hasAsk: true},
		{symbol: "CDF/USD", base: "CDF", bid: 199, ask: 201, hasAsk: true},
	}, []perpFixtureSpec{
		{symbol: "ABC-PERP", base: "ABC", underlyingSymbol: "ABC/USD"},
		{symbol: "CDF-PERP", base: "CDF", underlyingSymbol: "CDF/USD"},
	}, []positionFixture{
		{clientID: 1, symbol: "ABC-PERP", quantity: 1, entry: 100, side: exchange.Buy},
		{clientID: 1, symbol: "CDF-PERP", quantity: 1, entry: 100, side: exchange.Sell},
	})

	prepared, err := preflightLocalMidMarks([]*exposureFixture{fixture})
	if err != nil {
		t.Fatalf("preflightLocalMidMarks: %v", err)
	}
	if len(prepared) != 1 || prepared[0].epoch == 0 {
		t.Fatalf("committed epoch = %#v", prepared)
	}
	if prepared[0].marks["ABC-PERP"] != 100 || prepared[0].marks["CDF-PERP"] != 200 {
		t.Fatalf("preflight marks = %#v", prepared[0].marks)
	}

	fixture.market.CheckLiquidations("ABC-PERP", fixture.perps[0].instrument, 101)
	if countEvents(fixture.log.events, "liquidation_check") != 0 {
		t.Fatal("liquidation ran with a trigger mark that differs from the committed epoch")
	}

	fixture.market.CheckLiquidations("ABC-PERP", fixture.perps[0].instrument, 100)
	checks := eventsNamed(fixture.log.events, "liquidation_check")
	if len(checks) != 1 {
		t.Fatalf("liquidation checks = %d, want one", len(checks))
	}
	fields, ok := checks[0].data.(map[string]any)
	if !ok {
		t.Fatalf("liquidation check payload = %T", checks[0].data)
	}
	if fields["mark_price"] != int64(100) || fields["derivative_equity_contribution"] != int64(-100) || fields["maintenance_margin"] != int64(150) {
		t.Fatalf("risk sweep did not use the preflighted cross-margin marks: %#v", fields)
	}
}

func TestUnavailableVenuePreflightPreventsEveryVenueSweep(t *testing.T) {
	priceable := makeLiquidatableVenue(t, "N", true)
	unpriceable := makeLiquidatableVenue(t, "S", false)

	err := runLocalMidRiskBoundary(priceable, unpriceable)
	if err == nil {
		t.Fatal("risk boundary accepted a venue without a two-sided local mark")
	}
	if got := countEvents(priceable.log.events, "liquidation_check"); got != 0 {
		t.Fatalf("priceable venue had %d liquidation checks before the complete roster preflight passed", got)
	}
	position := priceable.market.Positions.GetPosition(1, "ABC-PERP")
	if position == nil || position.Size != 1 {
		t.Fatalf("priceable venue position changed despite failed all-venue preflight: %#v", position)
	}
}

func TestRunnerStopsBeforeVenuePhaseWhenLocalMidPreflightFails(t *testing.T) {
	venue := makeLiquidatableVenue(t, "N", false)
	counted := &countingVenue{DefaultExchange: venue.market}
	runner := simulation.NewRunner(venue.market.Clock, simulation.RunnerConfig{
		Iterations: 1, Step: time.Second, DeterministicPhases: true,
		BeforeTimestampPhase: func(int64) error {
			return runLocalMidRiskBoundary(venue)
		},
	})
	runner.AddMount(&simulation.Mount{Market: counted})
	var finalizedAt int64
	runner.SetShutdownHook(func() { finalizedAt = venue.market.Clock.NowUnixNano() })

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("runner accepted a risk preflight without a two-sided local mark")
	}
	if counted.phasePumps != 0 {
		t.Fatalf("same-time venue phases pumped %d times after preflight failure", counted.phasePumps)
	}
	if got := countEvents(venue.log.events, "liquidation_check"); got != 0 {
		t.Fatalf("preflight failure produced %d liquidation checks", got)
	}
	if finalizedAt != venue.market.Clock.NowUnixNano() || finalizedAt != int64(time.Hour) {
		t.Fatalf("shutdown finalization time=%d clock=%d, want actual stop time %d", finalizedAt, venue.market.Clock.NowUnixNano(), int64(time.Hour))
	}
}

func TestRunnerStopFinalizesAtActualClockBeforeMountPhase(t *testing.T) {
	clock := simulation.NewSimulatedClock(77)
	market := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{
		ID: "N", Clock: clock, DeterministicIngress: true, DeterministicPhases: true,
	})
	counted := &countingVenue{DefaultExchange: market}
	runner := simulation.NewRunner(clock, simulation.RunnerConfig{
		Iterations: 1, Step: time.Second, DeterministicPhases: true,
		BeforeTimestampPhase: func(int64) error { return errRegisteredEconomicStop },
	})
	runner.AddMount(&simulation.Mount{Market: counted})
	var finalizedAt int64
	runner.SetShutdownHook(func() { finalizedAt = clock.NowUnixNano() })

	err := runner.Run(context.Background())
	if !errors.Is(err, errRegisteredEconomicStop) {
		t.Fatalf("runner error = %v, want registered stop", err)
	}
	if counted.phasePumps != 0 {
		t.Fatalf("same-time venue phases pumped %d times after the stop", counted.phasePumps)
	}
	if finalizedAt != 77 || clock.NowUnixNano() != 77 {
		t.Fatalf("shutdown finalization time=%d clock=%d, want actual stop time 77", finalizedAt, clock.NowUnixNano())
	}
}

var errRegisteredEconomicStop = errors.New("registered economic stop")

type countingVenue struct {
	*exchange.DefaultExchange
	phasePumps int
}

func (v *countingVenue) PumpDeterministicPhase() bool {
	v.phasePumps++
	return v.DefaultExchange.PumpDeterministicPhase()
}

func makeLiquidatableVenue(t *testing.T, venueID string, twoSided bool) *exposureFixture {
	spots := []spotFixture{{symbol: "ABC/USD", base: "ABC", bid: 99, ask: 101, hasAsk: twoSided}}
	return makeExposureFixture(t, venueID, spots, []perpFixtureSpec{
		{symbol: "ABC-PERP", base: "ABC", underlyingSymbol: "ABC/USD"},
	}, []positionFixture{
		{clientID: 1, symbol: "ABC-PERP", quantity: 1, entry: 100, side: exchange.Buy},
	})
}

func countEvents(events []recordedEvent, name string) int {
	return len(eventsNamed(events, name))
}

func eventsNamed(events []recordedEvent, name string) []recordedEvent {
	var matching []recordedEvent
	for _, event := range events {
		if event.name == name {
			matching = append(matching, event)
		}
	}
	return matching
}

func compareString(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
