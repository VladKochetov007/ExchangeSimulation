package multivenue

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"exchange_sim/analysis"
	"exchange_sim/evstream"
	"exchange_sim/exchange"
)

type fundingEndowmentClock struct{}

func (fundingEndowmentClock) NowUnixNano() int64 { return 0 }
func (fundingEndowmentClock) NowUnix() int64     { return 0 }

func fundingReserveJournalFixture() (exchange.FundingReserveEndowment, exchange.VenueBalanceEvent) {
	endowment := exchange.FundingReserveEndowment{
		VenueID: "N", PerpSymbol: "ABC-PERP", QuoteAsset: "USD", TimestampNano: 0,
		AccountCap: 2, RegisteredClientIDs: []uint64{1, 2}, InitialQuoteAtoms: 2,
		RateUnitsPerBp: 1_000_000, SourceID: exchange.FundingReserveEndowmentSource,
	}
	movement := exchange.VenueBalanceEvent{
		Timestamp: 0, Sequence: 1, Bucket: exchange.VenueFundingRoundingReserve,
		Asset: "USD", Symbol: "ABC-PERP", Reason: "external_endowment",
		OldBalance: 0, NewBalance: 2, Delta: 2,
	}
	return endowment, movement
}

func TestFundingReserveJournalWritesTwoRequiredCanonicalFrames(t *testing.T) {
	var output bytes.Buffer
	sink := &checkpointSink{binary: newBinaryEvidence(&output), includeEvidenceOnly: true,
		replaceRaw: true, firstEvent: true}
	localSequence := uint64(0)
	logger := venueLogger{venueID: "N", route: fundingReserveRoute, sink: sink,
		sequence: &localSequence, sequenceMu: &sync.Mutex{}}
	endowment, movement := fundingReserveJournalFixture()
	receipt, err := logger.AppendFundingEndowment(endowment, movement)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.EndowmentEventSeq == 0 || receipt.MovementEventSeq <= receipt.EndowmentEventSeq || localSequence != 2 {
		t.Fatalf("required event order or local route sequence was lost: %+v local=%d", receipt, localSequence)
	}
	if err := sink.close(); err != nil {
		t.Fatal(err)
	}
	reader, err := evstream.NewReader(bytes.NewReader(output.Bytes()), evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	var frames []uint64
	err = reader.Range(func(frame evstream.Frame) error {
		route, rendered, renderErr := renderBinaryFrameVersioned(reader, frame, true)
		if renderErr != nil {
			return renderErr
		}
		var persisted renderPersistedEvent
		if decodeErr := json.Unmarshal(rendered.raw, &persisted); decodeErr != nil {
			return decodeErr
		}
		if route.route != fundingReserveRoute || frame.Venue != "N" || frame.Header.ClientID != 0 ||
			frame.Header.SimTS != 0 || persisted.Data.GlobalSequence != frame.Header.Seq ||
			persisted.Data.Sequence != uint64(len(names)+1) {
			t.Fatalf("canonical reserve record lost route/header identity: %+v %+v", route, persisted)
		}
		switch persisted.Event {
		case "funding_reserve_endowment":
			var actual exchange.FundingReserveEndowment
			if decodeErr := json.Unmarshal(persisted.Data.Payload, &actual); decodeErr != nil || !reflect.DeepEqual(actual, endowment) {
				t.Fatalf("canonical external source changed: %+v %v", actual, decodeErr)
			}
		case "venue_balance_change":
			var actual exchange.VenueBalanceEvent
			if decodeErr := json.Unmarshal(persisted.Data.Payload, &actual); decodeErr != nil || !reflect.DeepEqual(actual, movement) {
				t.Fatalf("canonical venue movement changed: %+v %v", actual, decodeErr)
			}
		default:
			t.Fatalf("unexpected canonical record %s", persisted.Event)
		}
		names = append(names, persisted.Event)
		frames = append(frames, frame.Header.Seq)
		return nil
	})
	if err != nil || !reader.Terminated() || len(names) != 2 ||
		names[0] != "funding_reserve_endowment" || names[1] != "venue_balance_change" ||
		frames[0] != receipt.EndowmentEventSeq || frames[1] != receipt.MovementEventSeq {
		t.Fatalf("required canonical pair failed reconstruction: names=%v frames=%v receipt=%+v err=%v", names, frames, receipt, err)
	}
}

func TestFundingReserveExchangeUsesRequiredBinaryJournal(t *testing.T) {
	var output bytes.Buffer
	sink := &checkpointSink{binary: newBinaryEvidence(&output), includeEvidenceOnly: true,
		replaceRaw: true, firstEvent: true}
	localSequence := uint64(0)
	logger := venueLogger{venueID: "N", route: fundingReserveRoute, sink: sink,
		sequence: &localSequence, sequenceMu: &sync.Mutex{}}
	ex := exchange.NewExchangeWithConfig(exchange.ExchangeConfig{ID: "N", Clock: fundingEndowmentClock{}})
	t.Cleanup(ex.Shutdown)
	ex.AddInstrument(exchange.NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1))
	for clientID := uint64(1); clientID <= 2; clientID++ {
		ex.ConnectNewClient(clientID, nil, &exchange.FixedFee{})
	}
	endowment, _ := fundingReserveJournalFixture()
	if err := ex.EndowFundingRoundingReserve(endowment, logger); err != nil {
		t.Fatal(err)
	}
	if err := sink.close(); err != nil {
		t.Fatal(err)
	}
	reserve := ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"]
	if reserve.Initial != 2 || reserve.Balance != 2 || reserve.EndowmentEventSeq == 0 ||
		ex.VenueBalanceSequenceForReport() != 1 || localSequence != 2 || len(ex.VerifyConservation()) != 0 {
		t.Fatalf("required canonical construction failed to reconcile: reserve=%+v local=%d", reserve, localSequence)
	}
	reader, err := evstream.NewReader(bytes.NewReader(output.Bytes()), evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		t.Fatal(err)
	}
	var sourceFrame uint64
	var movementFrame uint64
	err = reader.Range(func(frame evstream.Frame) error {
		_, rendered, renderErr := renderBinaryFrameVersioned(reader, frame, true)
		if renderErr != nil {
			return renderErr
		}
		var persisted renderPersistedEvent
		if decodeErr := json.Unmarshal(rendered.raw, &persisted); decodeErr != nil {
			return decodeErr
		}
		switch persisted.Event {
		case "funding_reserve_endowment":
			sourceFrame = frame.Header.Seq
		case "venue_balance_change":
			movementFrame = frame.Header.Seq
		default:
			t.Fatalf("unexpected reserve journal event %s", persisted.Event)
		}
		return nil
	})
	if err != nil || !reader.Terminated() || sourceFrame != reserve.EndowmentEventSeq ||
		movementFrame <= sourceFrame {
		t.Fatalf("exchange's terminal reserve was not bound to sealed canonical source/movement frames: source=%d movement=%d reserve=%+v err=%v", sourceFrame, movementFrame, reserve, err)
	}
	inputDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(inputDir, "events.evs"), output.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"schema_version":2,"config":{"log_mode":"none","evidence_format":"evstream_v3","evidence_contract_version":2}}`)
	if err := os.WriteFile(filepath.Join(inputDir, "manifest.json"), manifest, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sink.binary.executionHash()
	attestation, err := json.Marshal(binaryEvidenceArtifactRecord{
		Domain: "canonical_binary_execution_frames", Ordering: "ordered_stream",
		SchemaEpoch: sink.binary.writer.SchemaEpoch(),
		EventFrames: sink.binary.count(), StreamFrames: sink.binary.writer.Count(),
		ExecutionStreamHash: hex.EncodeToString(digest[:]), EvidenceOnlyIncluded: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputDir, "binary-evidence-attestation.json"), attestation, 0600); err != nil {
		t.Fatal(err)
	}
	renderedDir := filepath.Join(t.TempDir(), "rendered")
	if _, err := RenderBinaryEvidence(inputDir, renderedDir); err != nil {
		t.Fatal(err)
	}
	terminalSequence := ex.VenueBalanceSequenceForReport()
	report := analysis.Report{VenueLedgers: []analysis.VenueLedger{{
		VenueID: "N", FinalSequence: &terminalSequence,
		FundingRoundingReserves: map[string]analysis.FundingReserveBalance{
			"ABC-PERP": {Asset: reserve.Asset, SourceID: reserve.SourceID,
				Initial: reserve.Initial, Balance: reserve.Balance,
				EndowmentEventSeq: reserve.EndowmentEventSeq},
		},
	}}}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(renderedDir, "greeks.json"), reportBytes, 0600); err != nil {
		t.Fatal(err)
	}
	run, err := analysis.Open(renderedDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.ValidateGlobalSequence(2, sink.binary.writer.Count()); err != nil {
		t.Fatal(err)
	}
	audit, err := run.MeasureConservation(analysis.ConservationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if audit.Deltas.FundingReserveEndowmentMismatches != 0 || audit.Deltas.VenueBalanceMismatches != 0 ||
		audit.Deltas.VenueChainMismatches != 0 || audit.Deltas.MalformedVenueRecords != 0 ||
		audit.Deltas.MalformedVenueLedgers != 0 {
		t.Fatalf("binary-rendered reserve did not independently reconcile: %+v", audit.Deltas)
	}
}

func TestFundingReserveJournalRejectsMissingOrWrongSink(t *testing.T) {
	endowment, movement := fundingReserveJournalFixture()
	closedSink := &checkpointSink{binary: newBinaryEvidence(&bytes.Buffer{}), includeEvidenceOnly: true}
	if err := closedSink.close(); err != nil {
		t.Fatal(err)
	}
	overflowSequence := uint64(math.MaxUint64 - 1)
	for _, test := range []struct {
		name   string
		logger venueLogger
	}{
		{"missing-sink", venueLogger{venueID: "N", route: fundingReserveRoute, sequence: new(uint64), sequenceMu: &sync.Mutex{}}},
		{"wrong-route", venueLogger{venueID: "N", route: "general.jsonl", sink: &checkpointSink{binary: newBinaryEvidence(&bytes.Buffer{}), includeEvidenceOnly: true}, sequence: new(uint64), sequenceMu: &sync.Mutex{}}},
		{"discard-sink", venueLogger{venueID: "N", route: fundingReserveRoute, sink: &checkpointSink{binary: newBinaryEvidence(&bytes.Buffer{}), includeEvidenceOnly: true, discardBinary: true}, sequence: new(uint64), sequenceMu: &sync.Mutex{}}},
		{"closed-sink", venueLogger{venueID: "N", route: fundingReserveRoute, sink: closedSink, sequence: new(uint64), sequenceMu: &sync.Mutex{}}},
		{"local-sequence-overflow", venueLogger{venueID: "N", route: fundingReserveRoute, sink: &checkpointSink{binary: newBinaryEvidence(&bytes.Buffer{}), includeEvidenceOnly: true}, sequence: &overflowSequence, sequenceMu: &sync.Mutex{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.logger.AppendFundingEndowment(endowment, movement); err == nil {
				t.Fatal("reserve endowment was accepted without a durable canonical pair")
			}
		})
	}
}
