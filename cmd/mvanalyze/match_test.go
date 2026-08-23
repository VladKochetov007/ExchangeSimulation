package main

import (
	"os"
	"path/filepath"
	"testing"

	"exchange_sim/exchange"
	"exchange_sim/simulation"
)

// A symbol is not a path. filepath.Match refuses to let "*" cross a separator,
// so the catch-all pattern silently failed to match every spot book and those
// books were judged against whichever thresholds the flags happened to carry.
func TestMatchSymbolCrossesTheSlashInABookName(t *testing.T) {
	cases := []struct {
		pattern, symbol string
		want            bool
	}{
		{"*", "ABC/USD", true},
		{"*", "ABC-1735696803-48000-C", true},
		{"*-C", "ABC-1735696803-48000-C", true},
		{"*-C", "ABC-1735696803-48000-P", false},
		{"ABC-FUT-*", "ABC-FUT-1735696801", true},
		{"ABC-FUT-*", "ABC-PERP", false},
		{"ABC-PERP", "ABC-PERP", true},
		{"ABC-PERP", "ABC/USD", false},
		{"ABC/*", "ABC/USD", true},
		{"*/USD", "CDF/USD", true},
		{"*/USD", "ABC/CDF", false},
		{"A*C*D", "ABCD", true},
		{"A*C*D", "ABDC", false},
	}
	for _, c := range cases {
		if got := matchSymbol(c.pattern, c.symbol); got != c.want {
			t.Errorf("matchSymbol(%q, %q) = %v, want %v", c.pattern, c.symbol, got, c.want)
		}
	}
}

// Compact V2 evidence must remain analyzable when a short diagnostic retains
// no raw JSON event log or unrelated Greek report. Requiring analysis.Open
// here would silently turn an evidence-complete run into an analyzer failure.
func TestStandaloneObservationReceiptsNeedNoRawRunArtifacts(t *testing.T) {
	dir := t.TempDir()
	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	if linkID := recorder.RegisterLink("south", "south/v2_remote_feed/client/7", "v2_remote_feed"); linkID == 0 {
		t.Fatal("did not register compact evidence link")
	}
	schedule := simulation.MarketDataSchedule{
		ClientID: 7, SourceVenue: "south", Link: "south/v2_remote_feed/client/7", Symbol: "ABC/USD",
		Type: exchange.MDSnapshot, Sequence: 1, Fingerprint: [16]byte{1}, PublishedAt: 100, ScheduledAt: 110, LinkOrdinal: 1,
	}
	if linkID := recorder.RecordSchedule(schedule); linkID == 0 {
		t.Fatal("did not record compact evidence schedule")
	}
	frontier := recorder.RecordReceipt(simulation.MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: 110})
	if frontier.LinkID == 0 {
		t.Fatal("did not record compact evidence receipt")
	}
	if err := recorder.Finalize(110); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "greeks.json")); !os.IsNotExist(err) {
		t.Fatalf("fixture unexpectedly has a raw-run report: %v", err)
	}
	handled, err := emitStandaloneEvidenceMetric("observationreceipts", dir, true)
	if err != nil || !handled {
		t.Fatalf("compact evidence metric rejected sidecar-only run: handled=%t err=%v", handled, err)
	}
	if handled, err := emitStandaloneEvidenceMetric("roles", dir, true); err != nil || handled {
		t.Fatalf("raw-run metric misclassified as standalone: handled=%t err=%v", handled, err)
	}
}
