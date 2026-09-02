package multivenue

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	etypes "exchange_sim/types"
)

func TestTerminalOutcomeClassifiesTypedTerminalPriceFailure(t *testing.T) {
	sim := &Sim{
		Config:       Config{EvidenceFormat: "evstream_v3", StrictPopulationAccounting: true},
		startNano:    10,
		terminalNano: 20,
		closed:       true,
	}
	runErr := &riskCaptureFailure{
		Stage:         "terminal_risk_capture",
		FailureAtNano: 20,
		VenueID:       "north",
		Symbol:        "CDF/USD",
		Cause: &valuationMarkFailure{
			Phase:   "terminal_post_mark",
			VenueID: "north",
			Symbol:  "CDF/USD",
			Cause:   etypes.ErrNoPrice,
		},
	}
	outcome := sim.TerminalOutcomeFor(runErr, nil)
	if outcome.SchemaVersion != 2 || outcome.Status != "terminal_failure" || outcome.Code != "PRICE_UNAVAILABLE" {
		t.Fatalf("unavailable-price outcome = %+v", outcome)
	}
	if outcome.SimulationStartNano != 10 || outcome.SimulationEndNano != 20 || !outcome.EvidenceSealed {
		t.Fatalf("unavailable-price endpoint metadata = %+v", outcome)
	}
	if outcome.Stage != "terminal_risk_capture" || outcome.FailureAtNano != 20 ||
		outcome.FailureVenueID != "north" || outcome.FailureSymbol != "CDF/USD" {
		t.Fatalf("unavailable-price failure provenance = %+v", outcome)
	}
	if outcome.TerminalPopulationCaptured || outcome.TerminalRiskCaptured {
		t.Fatalf("incomplete endpoint claimed terminal captures: %+v", outcome)
	}
}

func TestTerminalOutcomeRejectsUntypedOrNonTerminalPriceFailure(t *testing.T) {
	sim := &Sim{Config: Config{EvidenceFormat: "evstream_v3"}, startNano: 10, terminalNano: 20}
	cases := map[string]error{
		"scheduled-stage": &riskCaptureFailure{
			Stage: "scheduled_risk_capture", FailureAtNano: 20, VenueID: "north", Symbol: "CDF/USD",
			Cause: &valuationMarkFailure{Phase: "post_derivative_mark", VenueID: "north", Symbol: "CDF/USD", Cause: etypes.ErrNoPrice},
		},
		"wrong-time": &riskCaptureFailure{
			Stage: "terminal_risk_capture", FailureAtNano: 19, VenueID: "north", Symbol: "CDF/USD",
			Cause: &valuationMarkFailure{Phase: "terminal_post_mark", VenueID: "north", Symbol: "CDF/USD", Cause: etypes.ErrNoPrice},
		},
		"joined": errors.Join(errors.New("unrelated failure"), &riskCaptureFailure{
			Stage: "terminal_risk_capture", FailureAtNano: 20, VenueID: "north", Symbol: "CDF/USD",
			Cause: &valuationMarkFailure{Phase: "terminal_post_mark", VenueID: "north", Symbol: "CDF/USD", Cause: etypes.ErrNoPrice},
		}),
		"untyped": etypes.ErrNoPrice,
		"mismatched-nested-venue": &riskCaptureFailure{
			Stage: "terminal_risk_capture", FailureAtNano: 20, VenueID: "north", Symbol: "CDF/USD",
			Cause: &valuationMarkFailure{Phase: "terminal_post_mark", VenueID: "south", Symbol: "CDF/USD", Cause: etypes.ErrNoPrice},
		},
		"mismatched-nested-symbol": &riskCaptureFailure{
			Stage: "terminal_risk_capture", FailureAtNano: 20, VenueID: "north", Symbol: "CDF/USD",
			Cause: &valuationMarkFailure{Phase: "terminal_post_mark", VenueID: "north", Symbol: "ABC/USD", Cause: etypes.ErrNoPrice},
		},
		"mismatched-nested-phase": &riskCaptureFailure{
			Stage: "terminal_risk_capture", FailureAtNano: 20, VenueID: "north", Symbol: "CDF/USD",
			Cause: &valuationMarkFailure{Phase: "post_derivative_mark", VenueID: "north", Symbol: "CDF/USD", Cause: etypes.ErrNoPrice},
		},
	}
	for name, runErr := range cases {
		t.Run(name, func(t *testing.T) {
			outcome := sim.TerminalOutcomeFor(runErr, nil)
			if outcome.Code != "SIMULATION_FAILURE" || outcome.Stage != "" || outcome.FailureAtNano != 0 ||
				outcome.FailureVenueID != "" || outcome.FailureSymbol != "" {
				t.Fatalf("untrusted terminal error classified as economic endpoint: %+v", outcome)
			}
		})
	}
}

func TestTerminalOutcomeClassifiesTypedPriceDomainFailure(t *testing.T) {
	sim := &Sim{Config: Config{EvidenceFormat: "evstream_v3"}, startNano: 10, terminalNano: 20, closed: true}
	runErr := &riskCaptureFailure{
		Stage: "terminal_population_capture", FailureAtNano: 20, VenueID: "south", Symbol: "ABC/USD",
		Cause: &valuationMarkFailure{Phase: "terminal_post_mark", VenueID: "south", Symbol: "ABC/USD", Cause: etypes.ErrPriceDomain},
	}
	outcome := sim.TerminalOutcomeFor(runErr, nil)
	if outcome.Code != "PRICE_DOMAIN_ERROR" || outcome.Stage != "terminal_population_capture" ||
		outcome.FailureVenueID != "south" || outcome.FailureSymbol != "ABC/USD" {
		t.Fatalf("price-domain endpoint = %+v", outcome)
	}
}

func TestTerminalOutcomeRequiresSuccessfulEvidenceSeal(t *testing.T) {
	sim := &Sim{Config: Config{EvidenceFormat: "evstream_v3"}, startNano: 10, terminalNano: 20}
	outcome := sim.TerminalOutcomeFor(nil, nil)
	if outcome.EvidenceSealed {
		t.Fatalf("unclosed simulation claimed sealed evidence: %+v", outcome)
	}
	sim.closed = true
	sim.closeErr = errors.New("checkpoint close failed")
	outcome = sim.TerminalOutcomeFor(nil, sim.closeErr)
	if outcome.EvidenceSealed || outcome.Code != "EVIDENCE_SEAL_FAILURE" {
		t.Fatalf("failed evidence seal endpoint = %+v", outcome)
	}
}

func TestWriteTerminalOutcomePublishesTypedJSON(t *testing.T) {
	logDir := t.TempDir()
	sim := &Sim{Config: Config{EvidenceFormat: "evstream_v3"}, startNano: 10, terminalNano: 20, closed: true}
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
