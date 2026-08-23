package simulation

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"exchange_sim/actor"
	"exchange_sim/exchange"
)

// boundaryGateway is deliberately only a message source. The test exercises
// the courier boundary itself, where an accidental negative latency would make
// a market observation available to an actor before its publication instant.
type boundaryGateway struct {
	id   uint64
	resp chan exchange.Response
	md   chan *exchange.MarketDataMsg
}

// TestMarketDataReceiptAttestsInboxArrival verifies schedule, receipt, and
// decision sidecars at the actor-facing boundary without calling recorder
// internals to decode their bytes.
func TestMarketDataReceiptAttestsInboxArrival(t *testing.T) {
	const delay = 10 * time.Millisecond
	dir := t.TempDir()
	recorder, err := NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := NewSimulatedClock(int64(time.Second))
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	inner := &boundaryGateway{
		id: 9, resp: make(chan exchange.Response, 1), md: make(chan *exchange.MarketDataMsg, 2),
	}
	delayed := NewDelayedGateway(inner, nil, nil, NewConstantLatency(delay))
	delayed.UseScheduler(scheduler, clock)
	delayed.SetMarketDataReceiptRecorder(recorder, "north", "north/spot_maker", "spot_maker")
	if err := delayed.EnableDeterministicPhases(); err != nil {
		t.Fatal(err)
	}
	delayed.Start()
	defer delayed.Stop()

	publishedAt := clock.NowUnixNano()
	inner.md <- &exchange.MarketDataMsg{Type: exchange.MDSnapshot, Symbol: "ABC/USD", SeqNum: 17, Timestamp: publishedAt}
	inner.md <- &exchange.MarketDataMsg{Type: exchange.MDTrade, Symbol: "ABC/USD", SeqNum: 18, Timestamp: publishedAt}
	if !delayed.PumpDeterministicPhase() {
		t.Fatal("market-data receipts were not scheduled")
	}
	clock.Advance(delay)
	if !delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("market-data receipts never entered actor inbox")
	}
	delayed.Send(exchange.Request{Type: exchange.ReqPlaceOrder, OrderReq: &exchange.OrderRequest{
		RequestID: 41, Symbol: "ABC/USD", Side: exchange.Buy, Type: exchange.LimitOrder, Price: 50_000, Qty: 1,
	}})
	for range 2 {
		select {
		case <-delayed.MarketDataCh():
		default:
			t.Fatal("receipt test actor inbox is short")
		}
	}
	if err := recorder.Finalize(clock.NowUnixNano()); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "market-data-receipts-v2.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 2*88 {
		t.Fatalf("receipt byte length = %d, want 176", len(raw))
	}
	for i := 0; i < 2; i++ {
		record := raw[i*88 : (i+1)*88]
		if clientID := binary.BigEndian.Uint64(record[0:8]); clientID != 9 {
			t.Fatalf("receipt %d client = %d, want 9", i, clientID)
		}
		if linkID := binary.BigEndian.Uint32(record[8:12]); linkID != 1 {
			t.Fatalf("receipt %d link ID = %d, want 1", i, linkID)
		}
		if symbolID := binary.BigEndian.Uint32(record[12:16]); symbolID != 1 {
			t.Fatalf("receipt %d symbol ID = %d, want 1", i, symbolID)
		}
		if got := binary.BigEndian.Uint64(record[20:28]); got != uint64(17+i) {
			t.Fatalf("receipt %d sequence = %d, want %d", i, got, 17+i)
		}
		if got := int64(binary.BigEndian.Uint64(record[44:52])); got != publishedAt {
			t.Fatalf("receipt %d publication = %d, want %d", i, got, publishedAt)
		}
		if got := int64(binary.BigEndian.Uint64(record[52:60])); got != publishedAt+delay.Nanoseconds() {
			t.Fatalf("receipt %d schedule = %d", i, got)
		}
		if got := int64(binary.BigEndian.Uint64(record[60:68])); got != publishedAt+delay.Nanoseconds() {
			t.Fatalf("receipt %d delivery = %d", i, got)
		}
		if ordinal := binary.BigEndian.Uint64(record[68:76]); ordinal != uint64(i+1) {
			t.Fatalf("receipt %d ordinal = %d, want %d", i, ordinal, i+1)
		}
	}

	decisions, err := os.ReadFile(filepath.Join(dir, "market-data-decisions-v2.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 96 || binary.BigEndian.Uint64(decisions[24:32]) != 41 || binary.BigEndian.Uint64(decisions[40:48]) != 2 {
		t.Fatalf("decision does not cite received local frontier: %v", decisions)
	}

	manifestRaw, err := os.ReadFile(filepath.Join(dir, "market-data-evidence-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Domain    string `json:"domain"`
		Ordering  string `json:"ordering"`
		Schedules struct {
			Records int64 `json:"records"`
		} `json:"schedules"`
		Receipts struct {
			Records int64 `json:"records"`
		} `json:"receipts"`
		Decisions struct {
			Records int64 `json:"records"`
		} `json:"decisions"`
		Links []struct {
			ID          uint32 `json:"id"`
			SourceVenue string `json:"source_venue"`
			Link        string `json:"link"`
		} `json:"links"`
		Symbols []struct {
			ID     uint32 `json:"id"`
			Symbol string `json:"symbol"`
		} `json:"symbols"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Domain != "participant_information_boundary_v2" || manifest.Ordering != "per_link_fifo_schedule_receipt_decision" ||
		manifest.Schedules.Records != 2 || manifest.Receipts.Records != 2 || manifest.Decisions.Records != 1 {
		t.Fatalf("bad receipt manifest: %+v", manifest)
	}
	if len(manifest.Links) != 1 || manifest.Links[0].SourceVenue != "north" || manifest.Links[0].Link != "north/spot_maker" ||
		len(manifest.Symbols) != 1 || manifest.Symbols[0].Symbol != "ABC/USD" {
		t.Fatalf("receipt catalog is incomplete: %+v", manifest)
	}
}

var _ actor.Gateway = (*boundaryGateway)(nil)

func (g *boundaryGateway) ID() uint64                                   { return g.id }
func (g *boundaryGateway) Send(exchange.Request)                        {}
func (g *boundaryGateway) Responses() <-chan exchange.Response          { return g.resp }
func (g *boundaryGateway) MarketDataCh() <-chan *exchange.MarketDataMsg { return g.md }
func (g *boundaryGateway) IsRunning() bool                              { return true }

// TestMarketDataCannotArriveBeforePublicationPlusLatency is a direct
// information-boundary invariant. It does not trust a later actor decision:
// the observed message must be absent before its publication time plus the
// modeled courier latency, then appear at that exact delivery boundary.
func TestMarketDataCannotArriveBeforePublicationPlusLatency(t *testing.T) {
	const delay = 10 * time.Millisecond
	clock := NewSimulatedClock(int64(time.Second))
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	stats := NewLatencyStats()
	inner := &boundaryGateway{
		id: 7, resp: make(chan exchange.Response, 1), md: make(chan *exchange.MarketDataMsg, 1),
	}
	delayed := NewDelayedGateway(inner, nil, nil, NewConstantLatency(delay))
	delayed.UseScheduler(scheduler, clock)
	delayed.SetLatencyTelemetry(stats, "boundary-test")
	if err := delayed.EnableDeterministicPhases(); err != nil {
		t.Fatal(err)
	}
	delayed.Start()
	defer delayed.Stop()

	publishedAt := clock.NowUnixNano()
	inner.md <- &exchange.MarketDataMsg{Symbol: "ABC-USD", Timestamp: publishedAt}
	if !delayed.PumpDeterministicPhase() {
		t.Fatal("market-data publication was not scheduled")
	}
	if delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("market data reached the actor at its publication instant")
	}
	select {
	case got := <-delayed.MarketDataCh():
		t.Fatalf("future information delivered at %d: %+v", clock.NowUnixNano(), got)
	default:
	}

	clock.Advance(delay - time.Nanosecond)
	if delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("market data reached the actor before publication plus latency")
	}
	clock.Advance(time.Nanosecond)
	if !delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("market data was not delivered at publication plus latency")
	}
	select {
	case got := <-delayed.MarketDataCh():
		if got == nil || clock.NowUnixNano() < publishedAt+delay.Nanoseconds() {
			t.Fatalf("invalid delayed delivery at %d: %+v", clock.NowUnixNano(), got)
		}
	default:
		t.Fatal("actor inbox missing due market data")
	}

	rows := stats.Summary().Rows
	if len(rows) != 1 || rows[0].Scheduled != 1 || rows[0].Delivered != 1 || rows[0].MeanDeliveryNanoseconds < float64(delay) {
		t.Fatalf("latency evidence does not support the boundary: %+v", rows)
	}
}

// Zero delay is still an explicit modeled courier path when a link is
// configured with a latency provider. The telemetry must retain that observed
// zero rather than silently dropping the link and making an accidental direct
// connection indistinguishable from missing evidence.
func TestScheduledZeroLatencyProducesZeroTelemetry(t *testing.T) {
	clock := NewSimulatedClock(int64(time.Second))
	scheduler := NewEventScheduler(clock)
	clock.SetScheduler(scheduler)
	stats := NewLatencyStats()
	inner := &boundaryGateway{
		id: 8, resp: make(chan exchange.Response, 1), md: make(chan *exchange.MarketDataMsg, 1),
	}
	delayed := NewDelayedGateway(inner, nil, nil, NewConstantLatency(0))
	delayed.UseScheduler(scheduler, clock)
	delayed.SetLatencyTelemetry(stats, "zero-boundary-test")
	if err := delayed.EnableDeterministicPhases(); err != nil {
		t.Fatal(err)
	}
	delayed.Start()
	defer delayed.Stop()

	inner.md <- &exchange.MarketDataMsg{Symbol: "ABC-USD", Timestamp: clock.NowUnixNano()}
	if !delayed.PumpDeterministicPhase() {
		t.Fatal("zero-delay market data was not scheduled")
	}
	if !delayed.DrainDeterministicPhaseEgress() {
		t.Fatal("zero-delay market data was not delivered at publication")
	}
	select {
	case <-delayed.MarketDataCh():
	default:
		t.Fatal("actor inbox missing zero-delay market data")
	}

	rows := stats.Summary().Rows
	if len(rows) != 1 || rows[0].Scheduled != 1 || rows[0].Delivered != 1 ||
		rows[0].MeanDrawnNanoseconds != 0 || rows[0].MeanDeliveryNanoseconds != 0 {
		t.Fatalf("zero-delay telemetry is incomplete or nonzero: %+v", rows)
	}
}
