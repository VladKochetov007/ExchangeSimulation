package analysis

import (
	"encoding/json"
	"strings"
	"testing"

	etypes "exchange_sim/types"
)

func TestCollectCDFPublicDepthObservationsMeasuresBookModes(t *testing.T) {
	startAt := int64(100)
	terminalAt := int64(110)
	snapshot := func(sequence uint64, at int64, bids, asks []etypes.PriceLevel) Event {
		raw, err := json.Marshal(cdfPublicSnapshotEvidence{
			Bids: bids, Asks: asks, SourceSequence: sequence, PublicBids: bids, PublicAsks: asks,
		})
		if err != nil {
			t.Fatal(err)
		}
		return Event{SimTS: at, Name: "BookSnapshot", VenueID: "north", GlobalSequence: sequence, payload: raw}
	}
	delta := func(sequence uint64, at int64, side string, price, quantity int64) Event {
		raw, err := json.Marshal(cdfBookDeltaEvidence{Side: side, Price: price, VisibleQty: quantity})
		if err != nil {
			t.Fatal(err)
		}
		return Event{SimTS: at, Name: "BookDelta", VenueID: "north", GlobalSequence: sequence, payload: raw}
	}
	events := []Event{
		snapshot(1, 101, []etypes.PriceLevel{{Price: 99, VisibleQty: 10}}, []etypes.PriceLevel{{Price: 101, VisibleQty: 10}}),
		delta(2, 103, "SELL", 101, 0),
		snapshot(3, 105, []etypes.PriceLevel{{Price: 99, VisibleQty: 10}}, []etypes.PriceLevel{{Price: 101, VisibleQty: 10}}),
	}
	observations, checks := collectCDFPublicDepthObservations(events, []string{"north"}, startAt, terminalAt)
	if len(checks) != 0 {
		t.Fatalf("public depth checks = %+v", checks)
	}
	contract := RegisteredSV1DActivationContract()
	metric := measureCDFVenueConcentration("north", observations["north"], terminalAt, contract)
	if metric.BidOnlyDurationNano != 2 || metric.AskOnlyDurationNano != 0 || metric.EmptyBookDurationNano != 0 ||
		metric.NonTwoSidedDurationNano != 2 || metric.MaxUninterruptedNonTwoSidedDurationNano != 2 || metric.TerminalBookMode != "two_sided" {
		t.Fatalf("availability metric = %+v", metric)
	}
}

func TestCollectCDFPublicDepthObservationsFailsClosedOnBoundaryAndProjectionErrors(t *testing.T) {
	makeEvent := func(at int64, name string, payload any) Event {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return Event{SimTS: at, Name: name, VenueID: "north", GlobalSequence: uint64(at), payload: raw}
	}
	validSnapshot := cdfPublicSnapshotEvidence{
		Bids: []etypes.PriceLevel{{Price: 99, VisibleQty: 10}}, Asks: []etypes.PriceLevel{{Price: 101, VisibleQty: 10}},
		SourceSequence: 1, PublicBids: []etypes.PriceLevel{{Price: 99, VisibleQty: 10}}, PublicAsks: []etypes.PriceLevel{{Price: 101, VisibleQty: 10}},
	}
	events := []Event{
		makeEvent(99, "BookDelta", cdfBookDeltaEvidence{Side: "BUY", Price: 99, VisibleQty: 1}),
		makeEvent(100, "BookSnapshot", validSnapshot),
		makeEvent(101, "BookSnapshot", cdfPublicSnapshotEvidence{
			Bids: validSnapshot.Bids, Asks: validSnapshot.Asks, SourceSequence: 2,
			PublicBids: []etypes.PriceLevel{{Price: 99, VisibleQty: 9}}, PublicAsks: validSnapshot.PublicAsks,
		}),
		{SimTS: 102, Name: "BookSnapshot", VenueID: "central", GlobalSequence: 4, payload: func() json.RawMessage {
			raw, err := json.Marshal(validSnapshot)
			if err != nil {
				t.Fatal(err)
			}
			return raw
		}()},
	}
	_, checks := collectCDFPublicDepthObservations(events, []string{"north"}, 100, 110)
	if len(checks) != 3 {
		t.Fatalf("checks = %+v; want three fail-closed findings", checks)
	}
	for _, check := range checks {
		if !strings.Contains(check.Failure, "CDF") && !strings.Contains(check.Failure, "public") {
			t.Fatalf("check is not a public-evidence failure: %+v", check)
		}
	}
}

func TestValidateCDFTerminalValuationRequiresRegisteredEndpoints(t *testing.T) {
	contract := RegisteredSV1DActivationContract()
	run := &Run{Report: Report{
		InitialAccounts: []AccountRow{
			{VenueID: "north", Account: Account{Timestamp: contract.SimulationStartNano}},
			{VenueID: "central", Account: Account{Timestamp: contract.SimulationStartNano}},
			{VenueID: "south", Account: Account{Timestamp: contract.SimulationStartNano}},
		},
		TerminalAccounts: []AccountRow{
			{VenueID: "north", Account: Account{Timestamp: contract.SimulationEndNano}},
			{VenueID: "central", Account: Account{Timestamp: contract.SimulationEndNano}},
			{VenueID: "south", Account: Account{Timestamp: contract.SimulationEndNano}},
		},
	}}
	metadata := cdfActivationMetadata{SimulationStartNano: contract.SimulationStartNano, SimulationEndNano: contract.SimulationEndNano}
	if err := validateCDFTerminalValuation(run, metadata, contract); err != nil {
		t.Fatal(err)
	}
	run.Report.TerminalAccounts[0].Account.Timestamp++
	if err := validateCDFTerminalValuation(run, metadata, contract); err == nil {
		t.Fatal("timestamp-mutated terminal valuation was accepted")
	}
}
