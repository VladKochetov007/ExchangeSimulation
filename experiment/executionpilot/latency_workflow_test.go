package executionpilot

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestLatencyActorReportWireIsDecodableAndCrossChecksReplay(t *testing.T) {
	raw, identity, world, actorReport := latencyFixture(t, time.Millisecond, 0)
	outcome, err := ReconstructLatency(bytes.NewReader(raw), identity, world, 500_000_000)
	if err != nil {
		t.Fatal(err)
	}
	actorRaw, err := json.MarshalIndent(actorReport, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLatencyActorReport(actorRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := compareActorReport(outcome, decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Policy != "immediate" || decoded.Side != "BUY" || decoded.UnfilledQty != outcome.UnfilledQty ||
		decoded.SubmittedChildren != 1 || len(decoded.Children) != decoded.SubmittedChildren {
		t.Fatalf("actor report wire did not preserve lifecycle: %+v", decoded)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(actorRaw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "Side")
	missing, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeLatencyActorReport(missing); err == nil {
		t.Fatal("missing actor report field accepted")
	}
	fields["Side"] = json.RawMessage(`"BUY"`)
	fields["Invented"] = json.RawMessage(`1`)
	extra, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeLatencyActorReport(extra); err == nil {
		t.Fatal("unknown actor report field accepted")
	}
}
