package multivenue

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	etypes "exchange_sim/types"
)

func TestTerminalOutcomeClassifiesUnavailablePriceAndSealing(t *testing.T) {
	sim := &Sim{
		Config:       Config{EvidenceFormat: "evstream_v3", StrictPopulationAccounting: true},
		startNano:    10,
		terminalNano: 20,
	}
	runErr := errors.Join(errors.New("terminal valuation failed"), etypes.ErrNoPrice)
	outcome := sim.TerminalOutcomeFor(runErr, nil)
	if outcome.SchemaVersion != 1 || outcome.Status != "terminal_failure" || outcome.Code != "PRICE_UNAVAILABLE" {
		t.Fatalf("unavailable-price outcome = %+v", outcome)
	}
	if outcome.SimulationStartNano != 10 || outcome.SimulationEndNano != 20 || !outcome.EvidenceSealed {
		t.Fatalf("unavailable-price endpoint metadata = %+v", outcome)
	}
	if outcome.TerminalPopulationCaptured || outcome.TerminalRiskCaptured {
		t.Fatalf("incomplete endpoint claimed terminal captures: %+v", outcome)
	}
}

func TestWriteTerminalOutcomePublishesTypedJSON(t *testing.T) {
	logDir := t.TempDir()
	sim := &Sim{Config: Config{EvidenceFormat: "evstream_v3"}, startNano: 10, terminalNano: 20}
	want := sim.TerminalOutcomeFor(nil, nil)
	if err := WriteTerminalOutcome(logDir, want); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(logDir, "terminal-outcome.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got TerminalOutcome
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("published terminal outcome = %+v, want %+v", got, want)
	}
}
