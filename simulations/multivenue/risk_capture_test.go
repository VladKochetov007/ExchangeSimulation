package multivenue

import (
	"io"
	"testing"
	"time"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
	"exchange_sim/simulations/derivsim"
)

func newUnpricedOptionRiskVenue(t *testing.T) (*Venue, *simulation.SimulatedClock, *exchange.EuropeanOption) {
	t.Helper()
	start := int64(time.Second)
	clock := simulation.NewSimulatedClock(start)
	exchangeState := exchange.NewExchange(4, clock)
	spot := exchange.NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1)
	option := exchange.NewEuropeanOption("ABC-C-100", "ABC", "USD", spot.Symbol(), 1, 1, 1, 1, 100, start+int64(time.Hour), true)
	exchangeState.AddInstrument(spot)
	exchangeState.AddInstrument(option)
	exchangeState.ConnectNewClient(1, map[string]int64{"USD": 1_000_000}, &exchange.FixedFee{})
	positions := exchangeState.Positions.(*exchange.PositionManager)
	positions.Lock()
	positions.InjectPosition(1, option.Symbol(), &exchange.Position{
		ClientID: 1, Symbol: option.Symbol(), PositionSide: exchange.PositionBoth,
		Size: 1, EntryPrice: 10,
	})
	positions.Unlock()
	venue := &Venue{
		ID: "north", Exchange: exchangeState,
		OptionDealer: &derivsim.OptionMarketMaker{}, OptionDealerClientID: 1,
	}
	t.Cleanup(exchangeState.Shutdown)
	return venue, clock, option
}

func TestScheduledRiskDefersUnavailableMarkAndAuditsRecovery(t *testing.T) {
	venue, clock, option := newUnpricedOptionRiskVenue(t)
	sink := &checkpointSink{binary: newBinaryEvidence(io.Discard), includeEvidenceOnly: true}
	var sequence uint64
	venue.makerStateLog = venueLogger{
		venueID: "north", route: "general.jsonl", sink: sink,
		sequence: &sequence,
	}
	interval := time.Minute
	captureScheduledVenueRisk(venue, interval, time.Second)
	if venue.riskErr != nil {
		t.Fatalf("transient missing option mark became fatal: %v", venue.riskErr)
	}
	if len(venue.RiskTimeline) != 0 || len(venue.RiskCaptureDiagnostics) != 1 {
		t.Fatalf("after unavailable mark timeline=%d diagnostics=%d, want 0/1", len(venue.RiskTimeline), len(venue.RiskCaptureDiagnostics))
	}
	first := venue.RiskCaptureDiagnostics[0]
	if first.AttemptKind != "initial" || first.Outcome != "deferred" || first.NextAttemptNano != clock.NowUnixNano()+interval.Nanoseconds() {
		t.Fatalf("initial risk diagnostic = %+v", first)
	}

	clock.SetTime(clock.NowUnixNano() + 30*time.Second.Nanoseconds())
	captureScheduledVenueRisk(venue, interval, time.Second)
	if len(venue.RiskCaptureDiagnostics) != 1 {
		t.Fatalf("risk retried before its scheduled boundary: %+v", venue.RiskCaptureDiagnostics)
	}

	option.SetMarks(100, 10)
	clock.SetTime(first.NextAttemptNano)
	captureScheduledVenueRisk(venue, interval, time.Second)
	if venue.riskErr != nil || len(venue.RiskTimeline) != 1 {
		t.Fatalf("recovered risk capture error=%v timeline=%d", venue.riskErr, len(venue.RiskTimeline))
	}
	if len(venue.RiskCaptureDiagnostics) != 2 {
		t.Fatalf("diagnostics after recovery = %+v", venue.RiskCaptureDiagnostics)
	}
	second := venue.RiskCaptureDiagnostics[1]
	if second.AttemptKind != "retry" || second.Outcome != "captured" || second.Error != "" {
		t.Fatalf("recovery diagnostic = %+v", second)
	}
	if sink.binary.count() != 2 || sink.binary.unencodableCount() != 0 {
		t.Fatalf("risk diagnostics evidence frames=%d unencodable=%d, want 2/0", sink.binary.count(), sink.binary.unencodableCount())
	}
	if err := sink.close(); err != nil {
		t.Fatalf("close diagnostic evidence: %v", err)
	}
}

func TestScheduledRiskStillFailsClosedOnOutOfDomainMark(t *testing.T) {
	venue, _, option := newUnpricedOptionRiskVenue(t)
	option.SetMarks(0, 10)
	captureScheduledVenueRisk(venue, time.Minute, time.Second)
	if venue.riskErr == nil {
		t.Fatal("out-of-domain option mark was deferred instead of failing closed")
	}
	if len(venue.RiskTimeline) != 0 || len(venue.RiskCaptureDiagnostics) != 1 {
		t.Fatalf("fatal risk capture timeline=%d diagnostics=%d, want 0/1", len(venue.RiskTimeline), len(venue.RiskCaptureDiagnostics))
	}
	diagnostic := venue.RiskCaptureDiagnostics[0]
	if diagnostic.AttemptKind != "initial" || diagnostic.Outcome != "failed" || diagnostic.Error == "" {
		t.Fatalf("fatal risk diagnostic = %+v", diagnostic)
	}
}
