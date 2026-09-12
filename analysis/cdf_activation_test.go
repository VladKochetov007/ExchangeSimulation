package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
	"exchange_sim/simulation"
	"exchange_sim/simulations/multivenue"
	etypes "exchange_sim/types"
)

type strictCDFTestEnvelope struct {
	routeRef      uint32
	eventRef      uint32
	sequence      uint64
	payloadDigest [sha256.Size]byte
	inner         evstream.InterningAppender
}

func (e strictCDFTestEnvelope) SchemaID() uint16      { return e.inner.SchemaID() }
func (e strictCDFTestEnvelope) SchemaVersion() uint16 { return e.inner.SchemaVersion() }

func (e strictCDFTestEnvelope) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint32(dst, e.routeRef)
	dst = evstream.AppendUint32(dst, e.eventRef)
	dst = evstream.AppendUint64(dst, e.sequence)
	dst = append(dst, e.payloadDigest[:]...)
	return e.inner.AppendPayloadInterning(dst, in)
}

type strictCDFInstrumentPayload struct {
	symbol string
	inner  evstream.InterningAppender
}

func (e strictCDFInstrumentPayload) SchemaID() uint16      { return exchange.SchemaInstrumentLog }
func (e strictCDFInstrumentPayload) SchemaVersion() uint16 { return 1 }

func (e strictCDFInstrumentPayload) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	symbolRef, err := in.Intern(e.symbol)
	if err != nil {
		return nil, err
	}
	dst = evstream.AppendUint32(dst, symbolRef)
	dst = evstream.AppendUint16(dst, e.inner.SchemaID())
	dst = evstream.AppendUint16(dst, e.inner.SchemaVersion())
	return e.inner.AppendPayloadInterning(dst, in)
}

type strictCDFRenderedPayload struct {
	Symbol  string          `json:"symbol"`
	Payload json.RawMessage `json:"payload"`
}

type strictCDFFixtureRow struct {
	SimTS    int64  `json:"sim_ts"`
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	Data     struct {
		VenueID  string          `json:"venue_id"`
		Sequence uint64          `json:"sequence"`
		Symbol   string          `json:"symbol"`
		Payload  json.RawMessage `json:"payload"`
	} `json:"data"`
	Route string `json:"-"`
}

func TestCDFStrictRenderedEvidenceRejectsPayloadMutation(t *testing.T) {
	evidenceDir := t.TempDir()
	eventsFile, err := os.Create(filepath.Join(evidenceDir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	writer := evstream.NewWriter(eventsFile, evstream.WriterOptions{SchemaEpoch: 4})
	routeRef, err := writer.Intern("general.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	eventRef, err := writer.Intern("payload_binding_probe")
	if err != nil {
		t.Fatal(err)
	}
	venueRef, err := writer.Intern("north")
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"symbol": "ABC-PERP", "payload": map[string]int{"value": 1}}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.AppendInterning(100, 7, venueRef, strictCDFTestEnvelope{
		routeRef: routeRef, eventRef: eventRef, sequence: 1,
		payloadDigest: sha256.Sum256(payloadRaw), inner: exchange.OpaqueJSON{Value: payload},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := eventsFile.Close(); err != nil {
		t.Fatal(err)
	}
	globalSequence := writer.Count()
	digest := writer.ExecutionHash()
	attestation := cdfBinaryEvidenceAttestation{
		Domain: "canonical_binary_execution_frames", Ordering: "ordered_stream", SchemaEpoch: 4,
		EventFrames: 1, StreamFrames: writer.Count(), ExecutionStreamHash: hex.EncodeToString(digest[:]),
		EvidenceOnlyIncluded: true,
	}
	attestationRaw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "binary-evidence-attestation.json"), append(attestationRaw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}

	renderedDir := filepath.Join(t.TempDir(), "rendered")
	routeDir := filepath.Join(renderedDir, "venues", "north")
	if err := os.MkdirAll(routeDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeRendered := func(value int) {
		t.Helper()
		renderedPayload, marshalErr := json.Marshal(map[string]any{
			"symbol": "ABC-PERP", "payload": map[string]int{"value": value},
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		record := map[string]any{
			"client_id": uint64(7),
			"data": map[string]any{
				"venue_id": "north", "sequence": uint64(1), "global_sequence": globalSequence,
				"payload": json.RawMessage(renderedPayload),
			},
			"event": "payload_binding_probe", "sim_ts": int64(100),
		}
		raw, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := os.WriteFile(filepath.Join(routeDir, "general.jsonl"), append(raw, '\n'), 0644); writeErr != nil {
			t.Fatal(writeErr)
		}
		renderedDigest, digestErr := digestRenderedEvidenceDirectory(renderedDir)
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		renderedAttestation := cdfRenderedEvidenceAttestation{
			Domain: "rendered_binary_evidence", Ordering: "venue_sequence_files_with_global_frame_identity",
			SourceExecutionHash: attestation.ExecutionStreamHash, SourceEventFrames: 1,
			SourceStreamFrames: writer.Count(), RenderedDigest: renderedDigest, GlobalSequenceIncluded: true,
		}
		renderedRaw, marshalErr := json.Marshal(renderedAttestation)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := os.WriteFile(filepath.Join(renderedDir, "rendered-binary-evidence-attestation.json"), append(renderedRaw, '\n'), 0644); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	writeRendered(1)
	run := &Run{Dir: evidenceDir, files: []string{filepath.Join(routeDir, "general.jsonl")}}
	if err := validateCDFRenderedGlobalSequence(run, evidenceDir, renderedDir, 4); err != nil {
		t.Fatalf("valid strict rendered evidence rejected: %v", err)
	}
	writeRendered(2)
	if err := validateCDFRenderedGlobalSequence(run, evidenceDir, renderedDir, 4); err == nil {
		t.Fatal("strict rendered audit accepted a payload mutation after attestation regeneration")
	}
}

type cdfActivationFixtureOptions struct {
	mutateConfig          func(*cdfActivationConfig)
	strictMechanics       bool
	badFingerprint        bool
	omitPostDecision      bool
	omitSupplierFill      bool
	badExchangeFill       bool
	borrowedSupplier      bool
	recapitalizedSupplier bool
	omitCancellation      bool
	dominantVolume        bool
	dominantDepth         bool
}

type cdfActivationFixtureParticipant struct {
	venueID  string
	clientID uint64
	contract CDFSupplierContract
	link     string
	first    simulation.MarketDataFrontier
	second   simulation.MarketDataFrontier
}

type cdfFixtureEvent struct {
	at       int64
	clientID uint64
	event    string
	symbol   string
	payload  any
	ordinal  int
}

func TestAuditCDFLiquidityActivationAcceptsCompleteRegisteredFixture(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{})
	audit, err := run.AuditCDFLiquidityActivation(CDFActivationOptions{Contract: RegisteredSV1DActivationContract(), AllowLegacyJSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if !audit.Valid || !audit.EvidenceValid || !audit.ActivationSatisfied || !audit.AntiCheatingSatisfied {
		t.Fatalf("valid activation audit = %+v", audit)
	}
	if audit.SupplierCount != 12 || audit.FillCount != 12 || audit.WithdrawalCount != 12 || audit.OneSidedRestorationCount != 4 {
		t.Fatalf("activation counts = suppliers %d fills %d withdrawals %d restorations %d", audit.SupplierCount, audit.FillCount, audit.WithdrawalCount, audit.OneSidedRestorationCount)
	}
}

func TestAuditCDFLiquidityActivationFailsClosedOnContractMutations(t *testing.T) {
	tests := []struct {
		name           string
		options        cdfActivationFixtureOptions
		wantCheck      string
		wantEvidence   bool
		wantActivation bool
		wantAnti       bool
	}{
		{
			name:      "strict risk disabled",
			options:   cdfActivationFixtureOptions{mutateConfig: func(config *cdfActivationConfig) { config.StrictRiskContract = false }},
			wantCheck: "successor strict-risk or evidence configuration is incomplete",
		},
		{
			name: "registered roster truncated",
			options: cdfActivationFixtureOptions{mutateConfig: func(config *cdfActivationConfig) {
				config.ElasticLiquiditySuppliers = config.ElasticLiquiditySuppliers[:3]
			}},
			wantCheck: "configured CDF supplier roster has the wrong size",
		},
		{
			name:      "fingerprint detached from receipt",
			options:   cdfActivationFixtureOptions{badFingerprint: true},
			wantCheck: "CDF decision does not join its exact delayed receipt frontier",
		},
		{
			name:      "actor fill omitted",
			options:   cdfActivationFixtureOptions{omitSupplierFill: true},
			wantCheck: "exchange OrderFill has no supplier inventory transition",
		},
		{
			name:      "exchange fill disagrees",
			options:   cdfActivationFixtureOptions{badExchangeFill: true},
			wantCheck: "exchange OrderFill does not match a live accepted CDF order",
		},
		{
			name:      "borrowed capital",
			options:   cdfActivationFixtureOptions{borrowedSupplier: true},
			wantCheck: "supplier spot balance snapshot violates no-debt arithmetic",
		},
		{
			name:      "unexplained recapitalization",
			options:   cdfActivationFixtureOptions{recapitalizedSupplier: true},
			wantCheck: "terminal supplier balances do not reconcile to finite initial capital and exchange-matched fills",
		},
		{
			name:      "cancellation evidence omitted",
			options:   cdfActivationFixtureOptions{omitCancellation: true},
			wantCheck: "CDF cancellation decision has no terminal OrderCancelled outcome",
		},
		{
			name:           "post-fill response omitted",
			options:        cdfActivationFixtureOptions{omitPostDecision: true},
			wantEvidence:   true,
			wantActivation: false,
			wantAnti:       true,
		},
		{
			name:           "supplier volume dominates",
			options:        cdfActivationFixtureOptions{dominantVolume: true},
			wantEvidence:   true,
			wantActivation: true,
			wantAnti:       false,
		},
		{
			name:           "supplier ask depth dominates active time",
			options:        cdfActivationFixtureOptions{dominantDepth: true},
			wantEvidence:   true,
			wantActivation: true,
			wantAnti:       false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := writeRegisteredCDFActivationFixture(t, test.options)
			audit, err := run.AuditCDFLiquidityActivation(CDFActivationOptions{Contract: RegisteredSV1DActivationContract(), AllowLegacyJSON: true})
			if err != nil {
				t.Fatal(err)
			}
			if audit.Valid {
				t.Fatalf("mutated fixture passed: %+v", audit)
			}
			if test.wantCheck != "" {
				if !hasCDFActivationFailure(audit.Checks, test.wantCheck) {
					t.Fatalf("checks = %+v, want %q", audit.Checks, test.wantCheck)
				}
				return
			}
			if audit.EvidenceValid != test.wantEvidence || audit.ActivationSatisfied != test.wantActivation || audit.AntiCheatingSatisfied != test.wantAnti {
				t.Fatalf("gates = evidence %v activation %v anti %v", audit.EvidenceValid, audit.ActivationSatisfied, audit.AntiCheatingSatisfied)
			}
		})
	}
}

func TestAuditCDFLiquidityActivationSupportsSeparateRenderedEvidence(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{})
	renderedDir := t.TempDir()
	if err := copyCDFEventFixture(t, filepath.Join(run.Dir, "venues"), filepath.Join(renderedDir, "venues")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(run.Dir, "venues")); err != nil {
		t.Fatal(err)
	}
	run, err := Open(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := run.AuditCDFLiquidityActivation(CDFActivationOptions{
		Contract: RegisteredSV1DActivationContract(), RenderedEvidenceDir: renderedDir, AllowLegacyJSON: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !audit.Valid {
		t.Fatalf("separate rendered evidence audit = %+v", audit)
	}
}

func TestAuditCDFLiquidityActivationStrictProductionRenderer(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{strictMechanics: true})
	contract := RegisteredSV1DActivationContract()
	rows := readStrictCDFFixtureRows(t, filepath.Join(run.Dir, "venues"))
	writeStrictCDFBinaryEvidence(t, run.Dir, rows, contract.BinarySchemaEpoch)
	rewriteStrictCDFCompletionIdentity(t, run.Dir, contract)
	if err := os.RemoveAll(filepath.Join(run.Dir, "venues")); err != nil {
		t.Fatal(err)
	}

	renderedDir := filepath.Join(t.TempDir(), "rendered")
	if report, err := multivenue.RenderBinaryEvidence(run.Dir, renderedDir); err != nil {
		t.Fatalf("production binary renderer: %v", err)
	} else if report.EventFrames != uint64(len(rows)) || report.ExecutionHash == "" || report.RenderedDigest == "" {
		t.Fatalf("production render report = %+v", report)
	}
	run, err := Open(run.Dir)
	if err != nil {
		t.Fatal(err)
	}
	binaryHash, err := sha256File("/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	configRaw, err := os.ReadFile(filepath.Join(run.Dir, "run-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	configDigest := sha256.Sum256(configRaw)
	audit, err := run.AuditCDFLiquidityActivation(CDFActivationOptions{
		Contract: contract, EvidenceDir: run.Dir, RenderedEvidenceDir: renderedDir,
		ExpectedProvenance: CDFExpectedProvenance{
			ConfigSHA256: hex.EncodeToString(configDigest[:]), SourceRevision: strings.Repeat("a", 40),
			BinarySHA256: binaryHash, BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !audit.Valid || !audit.EvidenceValid || !audit.ActivationSatisfied || !audit.AntiCheatingSatisfied {
		t.Fatalf("strict production-rendered activation audit = %+v", audit)
	}
}

func TestValidateCDFCompletionArtifactsRejectsMarketDataMutation(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{strictMechanics: true})
	contract := RegisteredSV1DActivationContract()
	rows := readStrictCDFFixtureRows(t, filepath.Join(run.Dir, "venues"))
	writeStrictCDFBinaryEvidence(t, run.Dir, rows, contract.BinarySchemaEpoch)
	rewriteStrictCDFCompletionIdentity(t, run.Dir, contract)
	if err := os.RemoveAll(filepath.Join(run.Dir, "venues")); err != nil {
		t.Fatal(err)
	}
	metadataRaw, err := os.ReadFile(filepath.Join(run.Dir, "run-metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata cdfActivationMetadata
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatal(err)
	}
	if err := validateCDFCompletionArtifacts(run.Dir, metadata, contract.BinarySchemaEpoch); err != nil {
		t.Fatalf("complete binary evidence fixture rejected before mutation: %v", err)
	}
	schedulesPath := filepath.Join(run.Dir, "market-data-schedules-v2.bin")
	schedulesRaw, err := os.ReadFile(schedulesPath)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, schedulesPath, append(schedulesRaw, byte('x')))
	if err := validateCDFCompletionArtifacts(run.Dir, metadata, contract.BinarySchemaEpoch); err == nil || !strings.Contains(err.Error(), "market-data schedules hash mismatch") {
		t.Fatalf("mutated market-data evidence was accepted: %v", err)
	}
}

func TestValidateCDFCheckpointPrefixesRejectsMismatchedIntermediateHash(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{strictMechanics: true})
	contract := RegisteredSV1DActivationContract()
	rows := readStrictCDFFixtureRows(t, filepath.Join(run.Dir, "venues"))
	writeStrictCDFBinaryEvidence(t, run.Dir, rows, contract.BinarySchemaEpoch)
	attestationRaw, err := os.ReadFile(filepath.Join(run.Dir, "binary-evidence-attestation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var attestation cdfBinaryEvidenceAttestation
	if err := json.Unmarshal(attestationRaw, &attestation); err != nil {
		t.Fatal(err)
	}
	if attestation.EventFrames < 2 {
		t.Fatalf("strict fixture has too few event frames for an intermediate checkpoint: %d", attestation.EventFrames)
	}
	checkpoints := map[uint64]string{
		1:                       strings.Repeat("0", sha256.Size*2),
		attestation.EventFrames: attestation.ExecutionStreamHash,
	}
	if err := validateCDFCheckpointPrefixes(run.Dir, checkpoints, attestation, contract.BinarySchemaEpoch); err == nil || !strings.Contains(err.Error(), "checkpoint hash does not match binary prefix") {
		t.Fatalf("mismatched intermediate checkpoint was accepted: %v", err)
	}
}

func TestValidateCDFCompletionSidecarsRejectsFixedFileSymlink(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{strictMechanics: true})
	contract := RegisteredSV1DActivationContract()
	rows := readStrictCDFFixtureRows(t, filepath.Join(run.Dir, "venues"))
	writeStrictCDFBinaryEvidence(t, run.Dir, rows, contract.BinarySchemaEpoch)
	rewriteStrictCDFCompletionIdentity(t, run.Dir, contract)
	metadataRaw, err := os.ReadFile(filepath.Join(run.Dir, "run-metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata cdfActivationMetadata
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(run.Dir, "greeks.json")
	target := filepath.Join(run.Dir, "greeks-target.json")
	if err := os.Rename(original, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, original); err != nil {
		t.Fatal(err)
	}
	if err := validateCDFCompletionSidecars(run.Dir, metadata, contract.BinarySchemaEpoch); err == nil || !strings.Contains(err.Error(), "evidence manifest fixed file") {
		t.Fatalf("fixed-file symlink was accepted: %v", err)
	}
}

func TestCDFStrictDepthDeltaDefersSharedPricePartialReduction(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	const venueID = "north"
	const clientID = uint64(7)
	const price = int64(300_300_000)
	states := map[cdfParticipantKey]*cdfSupplierState{
		{venueID: venueID, clientID: clientID}: {contract: contract},
	}
	orders := map[cdfOrderKey]*cdfOrderState{
		{venueID: venueID, clientID: clientID, orderID: 1}: {side: "BUY", price: price, remainingQty: 60, originalQty: 60},
		{venueID: venueID, clientID: clientID, orderID: 2}: {side: "BUY", price: price, remainingQty: 40, originalQty: 40},
	}
	publicDepth := map[string]*cdfPublicDepthState{
		venueID: {initialized: true, bids: map[int64]int64{price: 100}, asks: map[int64]int64{}},
	}
	depth := make(map[string][]cdfDepthObservation)
	pending := make(map[string][]cdfPendingDepthObservation)
	audit := &CDFActivationAudit{strictMechanics: true}
	deltaRaw, err := json.Marshal(cdfBookDeltaEvidence{Side: "BUY", Price: price, VisibleQty: 60})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFDepthDelta(Event{
		SimTS: 2, GlobalSequence: 2, VenueID: venueID, payload: deltaRaw,
	}, states, orders, depth, publicDepth, pending)
	if len(pending[venueID]) != 1 || len(depth[venueID]) != 0 {
		t.Fatalf("partial shared-price reduction was not deferred: pending=%+v depth=%+v checks=%+v", pending, depth, audit.Checks)
	}
	delete(orders, cdfOrderKey{venueID: venueID, clientID: clientID, orderID: 2})
	audit.flushCDFPendingDepth(venueID, states, orders, depth, pending)
	if len(pending[venueID]) != 0 || len(depth[venueID]) != 1 {
		t.Fatalf("causal order closure did not reconcile pending reduction: pending=%+v depth=%+v checks=%+v", pending, depth, audit.Checks)
	}
	observation := depth[venueID][0]
	if observation.bidDepth != 60 || observation.supplierBid != 60 || len(audit.Checks) != 0 {
		t.Fatalf("reconciled shared-price depth = %+v checks=%+v, want public and supplier depth 60", observation, audit.Checks)
	}
}

func TestCDFStrictDepthDeltaRejectsMismatchedCancellation(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	const venueID = "north"
	const clientID = uint64(7)
	const price = int64(300_300_000)
	states := map[cdfParticipantKey]*cdfSupplierState{{venueID: venueID, clientID: clientID}: {contract: contract}}
	orders := map[cdfOrderKey]*cdfOrderState{
		{venueID: venueID, clientID: clientID, orderID: 1}: {side: "BUY", price: price, remainingQty: 60, originalQty: 60},
		{venueID: venueID, clientID: clientID, orderID: 2}: {side: "BUY", price: price, remainingQty: 40, originalQty: 40},
	}
	publicDepth := map[string]*cdfPublicDepthState{
		venueID: {initialized: true, bids: map[int64]int64{price: 100}, asks: map[int64]int64{}},
	}
	depth := make(map[string][]cdfDepthObservation)
	pending := make(map[string][]cdfPendingDepthObservation)
	audit := &CDFActivationAudit{strictMechanics: true}
	deltaRaw, err := json.Marshal(cdfBookDeltaEvidence{Side: "BUY", Price: price, VisibleQty: 60})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFDepthDelta(Event{SimTS: 2, GlobalSequence: 2, VenueID: venueID, Ordinal: 10, payload: deltaRaw}, states, orders, depth, publicDepth, pending)
	cancelRaw, err := json.Marshal(cdfCancelledEvidence{OrderID: 1, RemainingQty: 60, Reason: "EXCHANGE_FORCED_LIFECYCLE"})
	if err != nil {
		t.Fatal(err)
	}
	audit.flushCDFPendingDepthBeforeEvent(Event{
		Name: "OrderCancelled", SimTS: 3, GlobalSequence: 3, VenueID: venueID, ClientID: clientID, payload: cancelRaw,
	}, states, orders, depth, pending)
	if len(pending[venueID]) != 0 || len(depth[venueID]) != 1 ||
		!hasCDFActivationFailure(audit.Checks, "CDF deferred depth reduction lacks the exact next causal cancellation") {
		t.Fatalf("mismatched same-level cancellation was accepted: pending=%+v depth=%+v checks=%+v", pending, depth, audit.Checks)
	}
}

func TestCDFStrictDepthDeltaResolvesThroughExactCancellation(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	const venueID = "north"
	const clientID = uint64(7)
	const price = int64(300_300_000)
	state := &cdfSupplierState{contract: contract, audit: CDFSupplierActivationAudit{VenueID: venueID, Role: contract.Role, ClientID: clientID}}
	states := map[cdfParticipantKey]*cdfSupplierState{{venueID: venueID, clientID: clientID}: state}
	orders := map[cdfOrderKey]*cdfOrderState{
		{venueID: venueID, clientID: clientID, orderID: 1}: {side: "BUY", price: price, remainingQty: 60, originalQty: 60, acceptedAt: 1, acceptedGlobalSeq: 1},
		{venueID: venueID, clientID: clientID, orderID: 2}: {side: "BUY", price: price, remainingQty: 40, originalQty: 40, acceptedAt: 1, acceptedGlobalSeq: 1},
	}
	publicDepth := map[string]*cdfPublicDepthState{
		venueID: {initialized: true, bids: map[int64]int64{price: 100}, asks: map[int64]int64{}},
	}
	depth := make(map[string][]cdfDepthObservation)
	pending := make(map[string][]cdfPendingDepthObservation)
	audit := &CDFActivationAudit{strictMechanics: true}
	deltaRaw, err := json.Marshal(cdfBookDeltaEvidence{Side: "BUY", Price: price, VisibleQty: 60})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFDepthDelta(Event{SimTS: 2, GlobalSequence: 2, VenueID: venueID, payload: deltaRaw}, states, orders, depth, publicDepth, pending)
	cancelEvent := Event{SimTS: 3, GlobalSequence: 3, VenueID: venueID, ClientID: clientID, Name: "OrderCancelled"}
	cancelEvent.payload, err = json.Marshal(cdfCancelledEvidence{OrderID: 2, RemainingQty: 40, Reason: "EXCHANGE_FORCED_LIFECYCLE"})
	if err != nil {
		t.Fatal(err)
	}
	audit.flushCDFPendingDepthBeforeEvent(cancelEvent, states, orders, depth, pending)
	if len(pending[venueID]) != 1 {
		t.Fatalf("exact causal cancellation was not retained for processing: %+v", pending)
	}
	audit.processCDFCancelled(cancelEvent, states, map[cdfRequestKey]*cdfWithdrawal{}, orders, depth, publicDepth)
	audit.flushCDFPendingDepth(venueID, states, orders, depth, pending, true)
	if len(pending[venueID]) != 0 || len(audit.Checks) != 0 || len(orders) != 1 || len(depth[venueID]) != 2 {
		t.Fatalf("exact causal cancellation did not resolve the pending reduction: pending=%+v orders=%+v depth=%+v checks=%+v", pending, orders, depth, audit.Checks)
	}
	observation := depth[venueID][1]
	if observation.bidDepth != 60 || observation.supplierBid != 60 {
		t.Fatalf("resolved depth = %+v, want public and supplier depth 60", observation)
	}
}

func TestValidateCDFCompletionSidecarsRejectsStructurallyIncompleteLatency(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{strictMechanics: true})
	contract := RegisteredSV1DActivationContract()
	rows := readStrictCDFFixtureRows(t, filepath.Join(run.Dir, "venues"))
	writeStrictCDFBinaryEvidence(t, run.Dir, rows, contract.BinarySchemaEpoch)
	rewriteStrictCDFCompletionIdentity(t, run.Dir, contract)
	metadataRaw, err := os.ReadFile(filepath.Join(run.Dir, "run-metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata cdfActivationMetadata
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(run.Dir, "latency.json"), []byte("{\"domain\":\"courier_delivery\",\"rows\":null}\n"))
	if err := validateCDFCompletionSidecars(run.Dir, metadata, contract.BinarySchemaEpoch); err == nil || !strings.Contains(err.Error(), "latency sidecar is structurally incomplete") {
		t.Fatalf("structurally incomplete latency sidecar was accepted: %v", err)
	}
}

func readStrictCDFFixtureRows(t *testing.T, venuesDir string) []strictCDFFixtureRow {
	t.Helper()
	rows := make([]strictCDFFixtureRow, 0)
	err := filepath.WalkDir(venuesDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		relative, err := filepath.Rel(venuesDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 2 || parts[0] == "" {
			return fmt.Errorf("fixture route %q is not venue-qualified", relative)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, line := range bytes.Split(raw, []byte{'\n'}) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var row strictCDFFixtureRow
			if err := json.Unmarshal(line, &row); err != nil {
				return fmt.Errorf("decode fixture row %s: %w", relative, err)
			}
			if row.Data.VenueID != parts[0] || row.Event == "" || row.Data.Sequence == 0 || len(row.Data.Payload) == 0 {
				return fmt.Errorf("incomplete fixture row in %s", relative)
			}
			row.Route = strings.Join(parts[1:], "/")
			rows = append(rows, row)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(rows, func(left, right int) bool {
		if rows[left].SimTS != rows[right].SimTS {
			return rows[left].SimTS < rows[right].SimTS
		}
		if rows[left].Data.VenueID != rows[right].Data.VenueID {
			return rows[left].Data.VenueID < rows[right].Data.VenueID
		}
		leftRank := strictCDFFixtureProducerRank(rows[left], rows)
		rightRank := strictCDFFixtureProducerRank(rows[right], rows)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if rows[left].Route != rows[right].Route {
			return rows[left].Route < rows[right].Route
		}
		if rows[left].Data.Sequence != rows[right].Data.Sequence {
			return rows[left].Data.Sequence < rows[right].Data.Sequence
		}
		return rows[left].Event < rows[right].Event
	})
	return rows
}

func strictCDFFixtureProducerRank(row strictCDFFixtureRow, rows []strictCDFFixtureRow) int {
	const (
		acceptedRank = iota + 10
		tradeRank
		fillRank
		supplierFillRank
		bookDeltaRank
		cancelledRank
	)
	if row.Event == "BookDelta" {
		for _, peer := range rows {
			if peer.SimTS != row.SimTS || peer.Data.VenueID != row.Data.VenueID {
				continue
			}
			switch peer.Event {
			case "OrderCancelled":
				return 5
			case "OrderFill":
				return 35
			case "OrderAccepted":
				return 15
			}
		}
	}
	switch row.Event {
	case "OrderAccepted":
		return acceptedRank
	case "Trade":
		return tradeRank
	case "OrderFill":
		return fillRank
	case "elastic_liquidity_supplier_fill":
		return supplierFillRank
	case "BookDelta":
		return bookDeltaRank
	case "OrderCancelled":
		return cancelledRank
	default:
		return 0
	}
}

func writeStrictCDFBinaryEvidence(t *testing.T, dir string, rows []strictCDFFixtureRow, schemaEpoch uint32) {
	t.Helper()
	file, err := os.Create(filepath.Join(dir, "events.evs"))
	if err != nil {
		t.Fatal(err)
	}
	writer := evstream.NewWriter(file, evstream.WriterOptions{SchemaEpoch: schemaEpoch})
	venueSequences := make(map[string]uint64)
	for _, row := range rows {
		venueRef, err := writer.Intern(row.Data.VenueID)
		if err != nil {
			t.Fatal(err)
		}
		routeRef, err := writer.Intern(row.Route)
		if err != nil {
			t.Fatal(err)
		}
		eventRef, err := writer.Intern(row.Event)
		if err != nil {
			t.Fatal(err)
		}
		canonicalPayload, err := json.Marshal(json.RawMessage(row.Data.Payload))
		if err != nil {
			t.Fatalf("canonical fixture payload: %v", err)
		}
		inner := evstream.InterningAppender(exchange.OpaqueJSON{Value: json.RawMessage(canonicalPayload)})
		renderedPayload := canonicalPayload
		if row.Data.Symbol != "" {
			inner = strictCDFInstrumentPayload{symbol: row.Data.Symbol, inner: inner}
			renderedPayload, err = json.Marshal(strictCDFRenderedPayload{Symbol: row.Data.Symbol, Payload: json.RawMessage(canonicalPayload)})
			if err != nil {
				t.Fatal(err)
			}
		}
		venueSequences[row.Data.VenueID]++
		if err := writer.AppendInterning(row.SimTS, row.ClientID, venueRef, strictCDFTestEnvelope{
			routeRef: routeRef, eventRef: eventRef, sequence: venueSequences[row.Data.VenueID],
			payloadDigest: sha256.Sum256(renderedPayload), inner: inner,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	digest := writer.ExecutionHash()
	attestation := cdfBinaryEvidenceAttestation{
		Domain: "canonical_binary_execution_frames", Ordering: "ordered_stream", SchemaEpoch: schemaEpoch,
		EventFrames: uint64(len(rows)), StreamFrames: writer.Count(), ExecutionStreamHash: hex.EncodeToString(digest[:]),
		EvidenceOnlyIncluded: true,
	}
	raw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "binary-evidence-attestation.json"), append(raw, '\n'))
}

func rewriteStrictCDFCompletionIdentity(t *testing.T, dir string, contract CDFActivationContract) {
	t.Helper()
	configRaw, err := os.ReadFile(filepath.Join(dir, "run-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	binaryHash, err := sha256File("/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"schema_version": 2, "venue_ids": contract.VenueIDs,
		"build":  map[string]any{"revision": strings.Repeat("a", 40), "modified": false, "goos": "linux", "goarch": "amd64", "goamd64": "v1"},
		"config": json.RawMessage(configRaw),
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "manifest.json"), append(manifestRaw, '\n'))
	var metadata cdfActivationMetadata
	metadataRaw, err := os.ReadFile(filepath.Join(dir, "run-metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata.BinaryPath = "/bin/true"
	configDigest := sha256.Sum256(configRaw)
	metadata.ConfigSHA256 = hex.EncodeToString(configDigest[:])
	metadata.BinarySHA256 = binaryHash
	metadata.GitRevision = strings.Repeat("a", 40)
	metadata.BinaryGOOS, metadata.BinaryGOARCH, metadata.BinaryGOAMD64 = "linux", "amd64", "v1"
	metadataRaw, err = json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "run-metadata.json"), append(metadataRaw, '\n'))
	greeksPath := filepath.Join(dir, "greeks.json")
	greeksRaw, err := os.ReadFile(greeksPath)
	if err != nil {
		t.Fatal(err)
	}
	var greeks map[string]any
	if err := json.Unmarshal(greeksRaw, &greeks); err != nil {
		t.Fatal(err)
	}
	greeks["schema_version"] = 6
	greeks["initial_risk"] = map[string]any{}
	greeks["terminal_risk"] = map[string]any{}
	greeks["risk_timeline"] = map[string]any{}
	greeks["microstructure"] = []any{map[string]any{"venue_id": "north", "timestamp": contract.SimulationEndNano}}
	greeksRaw, err = json.Marshal(greeks)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, greeksPath, append(greeksRaw, '\n'))
	writeCDFFixtureFile(t, filepath.Join(dir, "latency.json"), []byte("{\"domain\":\"courier_delivery\",\"rows\":[{\"link\":\"north/cdf_elastic_supplier/client/1\",\"channel\":\"market_data\",\"scheduled\":1,\"delivered\":1,\"undelivered\":0}]}\n"))
	attestationRaw, err := os.ReadFile(filepath.Join(dir, "binary-evidence-attestation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var attestation cdfBinaryEvidenceAttestation
	if err := json.Unmarshal(attestationRaw, &attestation); err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "checkpoints.jsonl"), []byte(fmt.Sprintf("{\"domain\":\"execution_observations\",\"ordering\":\"ordered_stream\",\"sim_time\":%d,\"event_count\":%d,\"execution_stream_hash\":%q,\"representation\":\"evstream_v3\",\"unencodable_payloads\":0}\n", contract.SimulationEndNano, attestation.EventFrames, attestation.ExecutionStreamHash)))
	fixedPaths := []string{"run-config.json", "run-metadata.json", "manifest.json", "greeks.json", "latency.json", "checkpoints.jsonl", "events.evs", "binary-evidence-attestation.json", "market-data-evidence-v2.json", "market-data-schedules-v2.bin", "market-data-receipts-v2.bin", "market-data-decisions-v2.bin"}
	fixedFiles := make([]map[string]any, 0, len(fixedPaths))
	for _, relative := range fixedPaths {
		path := filepath.Join(dir, relative)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		fixedFiles = append(fixedFiles, map[string]any{"path": relative, "bytes": info.Size(), "sha256": mustCDFFileHash(t, path)})
	}
	evidenceManifestRaw, err := json.Marshal(map[string]any{
		"schema_version": 2, "contract": "v2-integrated-longrun-evidence-manifest-v2", "cell": "sv1d-activation-659",
		"log_mode": "full", "evidence_format": "evstream_v3", "source_revision": strings.Repeat("a", 40),
		"fixed_files": fixedFiles, "raw_jsonl_files": 0, "raw_jsonl_bytes": 0, "raw_files": []map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "evidence-manifest.json"), append(evidenceManifestRaw, '\n'))
	status := cdfRunStatus{
		SchemaVersion: 1, ExitStatus: 0, CompletionVerified: true, CompletionSentinels: []string{"greeks.json", "latency.json"}, SimulatedHorizon: contract.Horizon,
		SimulationStartNano: contract.SimulationStartNano, SimulationEndNano: contract.SimulationEndNano,
		RunMetadataSHA256:      mustCDFFileHash(t, filepath.Join(dir, "run-metadata.json")),
		ManifestSHA256:         mustCDFFileHash(t, filepath.Join(dir, "manifest.json")),
		GreeksSHA256:           mustCDFFileHash(t, filepath.Join(dir, "greeks.json")),
		LatencySHA256:          mustCDFFileHash(t, filepath.Join(dir, "latency.json")),
		CheckpointsSHA256:      mustCDFFileHash(t, filepath.Join(dir, "checkpoints.jsonl")),
		EvidenceManifestSHA:    mustCDFFileHash(t, filepath.Join(dir, "evidence-manifest.json")),
		BinaryAttestationSHA:   mustCDFFileHash(t, filepath.Join(dir, "binary-evidence-attestation.json")),
		MarketDataEvidenceSHA:  mustCDFFileHash(t, filepath.Join(dir, "market-data-evidence-v2.json")),
		MarketDataSchedulesSHA: mustCDFFileHash(t, filepath.Join(dir, "market-data-schedules-v2.bin")),
		MarketDataReceiptsSHA:  mustCDFFileHash(t, filepath.Join(dir, "market-data-receipts-v2.bin")),
		MarketDataDecisionsSHA: mustCDFFileHash(t, filepath.Join(dir, "market-data-decisions-v2.bin")),
	}
	statusRaw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "run-status.json"), append(statusRaw, '\n'))
}

func mustCDFFileHash(t *testing.T, path string) string {
	t.Helper()
	digest, err := sha256File(path)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestAuditCDFLiquidityActivationRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		event string
		field string
		want  string
	}{
		{"elastic_liquidity_supplier_decision", "observation_digest", "malformed CDF decision: missing required payload field \"observation_digest\""},
		{"elastic_liquidity_supplier_fill", "is_full", "malformed CDF supplier fill: missing required payload field \"is_full\""},
	}
	for _, test := range tests {
		t.Run(test.event+"/"+test.field, func(t *testing.T) {
			run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{})
			path := filepath.Join(run.Dir, "venues", "north", "general.jsonl")
			deleteCDFFixturePayloadField(t, path, test.event, test.field)
			run, err := Open(run.Dir)
			if err != nil {
				t.Fatal(err)
			}
			audit, err := run.AuditCDFLiquidityActivation(CDFActivationOptions{Contract: RegisteredSV1DActivationContract(), AllowLegacyJSON: true})
			if err != nil {
				t.Fatal(err)
			}
			if audit.Valid || !hasCDFActivationFailure(audit.Checks, test.want) {
				t.Fatalf("checks = %+v, want fail-closed missing-field rejection", audit.Checks)
			}
		})
	}
}

func TestAuditCDFLiquidityActivationRejectsUnboundProvenance(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{})
	path := filepath.Join(run.Dir, "run-config.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run.AuditCDFLiquidityActivation(CDFActivationOptions{Contract: RegisteredSV1DActivationContract(), AllowLegacyJSON: true}); err == nil {
		t.Fatal("byte-mutated run config retained a valid provenance binding")
	}
}

func TestCDFExpectedProvenanceRejectsSelfAuthoredIdentity(t *testing.T) {
	expected := CDFExpectedProvenance{
		ConfigSHA256:   strings.Repeat("a", 64),
		SourceRevision: strings.Repeat("b", 40),
		BinarySHA256:   strings.Repeat("c", 64),
		BinaryGOOS:     "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
	}
	metadata := cdfActivationMetadata{
		ConfigSHA256: expected.ConfigSHA256, GitRevision: expected.SourceRevision,
		BinarySHA256: expected.BinarySHA256, BinaryGOOS: expected.BinaryGOOS,
		BinaryGOARCH: expected.BinaryGOARCH, BinaryGOAMD64: expected.BinaryGOAMD64,
	}
	if err := expected.validate(); err != nil {
		t.Fatal(err)
	}
	if err := validateCDFExpectedProvenance(metadata, expected); err != nil {
		t.Fatal(err)
	}
	metadata.BinarySHA256 = strings.Repeat("d", 64)
	if err := validateCDFExpectedProvenance(metadata, expected); err == nil {
		t.Fatal("metadata identity mismatch was accepted")
	}
	metadata.BinarySHA256 = expected.BinarySHA256
	metadata.SourceModified = true
	if err := validateCDFExpectedProvenance(metadata, expected); err == nil {
		t.Fatal("modified source was accepted")
	}
}

func writeRegisteredCDFActivationFixture(t *testing.T, options cdfActivationFixtureOptions) *Run {
	t.Helper()
	contract := RegisteredSV1DActivationContract()
	falseValue := false
	config := cdfActivationConfig{
		VenueIDs: append([]string(nil), contract.VenueIDs...), Seed: contract.Seed,
		LogMode: "full", EvidenceFormat: "evstream_v3", EvidenceContractVersion: 2, ExperimentID: contract.ExperimentID,
		HypothesisID: contract.HypothesisID, ElasticSupplierCount: contract.HistoricalSupplierCountPerVenue,
		StrictPopulationAccounting: true, StrictRiskContract: true, AutoBorrowSpot: &falseValue,
		CrossAssetSpotGraph: true, CrossAssetCollateralMarks: false,
		RecordElasticLiquiditySupplierDecisions: true, RecordMarketDataReceipts: true,
		MarketDataReceiptRoles:    []string{"cdf_elastic_supplier"},
		ElasticLiquiditySuppliers: append([]CDFSupplierContract(nil), contract.Suppliers...),
	}
	if options.mutateConfig != nil {
		options.mutateConfig(&config)
	}
	dir := t.TempDir()
	configRaw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFActivationIdentity(t, dir, config, configRaw, contract)

	participants := make([]*cdfActivationFixtureParticipant, 0, len(contract.VenueIDs)*len(contract.Suppliers))
	report := Report{}
	for venueOrdinal, venueID := range contract.VenueIDs {
		for historicalOrdinal := 1; historicalOrdinal <= contract.HistoricalSupplierCountPerVenue; historicalOrdinal++ {
			row := AccountRow{VenueID: venueID, ClientID: uint64(10_000 + venueOrdinal*100 + historicalOrdinal), Role: fmt.Sprintf("elastic_supplier_%d", historicalOrdinal), Account: Account{Equity: 1}}
			report.InitialAccounts = append(report.InitialAccounts, row)
			report.TerminalAccounts = append(report.TerminalAccounts, row)
		}
		for supplierOrdinal, supplier := range contract.Suppliers {
			clientID := uint64(1 + venueOrdinal*100 + supplierOrdinal)
			participant := &cdfActivationFixtureParticipant{
				venueID: venueID, clientID: clientID, contract: supplier,
				link: fmt.Sprintf("%s/cdf_elastic_supplier/client/%d", venueID, clientID),
			}
			participants = append(participants, participant)
			quantity := cdfFixtureActivationQuantity(venueID, supplier, options)
			tradeSide, tradePrice := cdfFixtureActivationTrade(venueID, supplier, options)
			positionDelta := quantity
			if tradeSide == "SELL" {
				positionDelta = -quantity
			}
			initialEquity := cdfFixtureInitialEquity(supplier)
			fee := cdfFixtureFee(tradePrice, quantity, supplier.BasePrecision, supplier.MakerFeeBps)
			tradeNotional := cdfFixtureNotional(tradePrice, quantity, supplier.BasePrecision)
			terminalBase := supplier.InitialBaseBalance + positionDelta
			terminalQuote := supplier.InitialQuoteBalance - tradeNotional - fee
			if tradeSide == "SELL" {
				terminalQuote = supplier.InitialQuoteBalance + tradeNotional - fee
			}
			initial := AccountRow{
				VenueID: venueID, ClientID: clientID, Role: supplier.Role,
				Marks: map[string]int64{"CDF": supplier.ReferencePrice, "USD": supplier.QuotePrecision},
				Account: Account{Equity: initialEquity, SpotBalances: []Balance{
					{Asset: "CDF", NetAsset: supplier.InitialBaseBalance},
					{Asset: "USD", NetAsset: supplier.InitialQuoteBalance},
				}},
			}
			terminal := initial
			terminal.Account.Equity = cdfFixtureTerminalEquity(supplier, terminalBase, terminalQuote)
			terminal.Account.SpotBalances = []Balance{{Asset: "CDF", NetAsset: terminalBase}, {Asset: "USD", NetAsset: terminalQuote}}
			if options.borrowedSupplier && venueOrdinal == 0 && supplierOrdinal == 0 {
				terminal.Account.SpotBalances[1].Borrowed = 1
			}
			if options.recapitalizedSupplier && venueOrdinal == 0 && supplierOrdinal == 0 {
				terminal.Account.SpotBalances[1].NetAsset++
			}
			report.InitialAccounts = append(report.InitialAccounts, initial)
			report.TerminalAccounts = append(report.TerminalAccounts, terminal)
		}
	}
	reportRaw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "greeks.json"), reportRaw)

	recorder, err := simulation.NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, participant := range participants {
		if recorder.RegisterLink(participant.venueID, participant.link, "cdf_elastic_supplier") == 0 {
			t.Fatal("register CDF fixture link")
		}
	}
	firstAt := contract.SimulationStartNano + 1_000_000_000
	secondAt := contract.SimulationStartNano + 10_000_000_000
	if options.strictMechanics {
		secondAt = contract.SimulationStartNano + 2_000_000_000
	}
	firstSnapshots := make(map[string]etypes.BookSnapshot, len(contract.VenueIDs))
	secondSnapshots := make(map[string]etypes.BookSnapshot, len(contract.VenueIDs))
	for _, venueID := range contract.VenueIDs {
		firstSnapshots[venueID] = cdfFixtureSnapshot(venueID, 1, options)
		secondSnapshots[venueID] = cdfFixtureSnapshot(venueID, 2, options)
	}
	recordCDFFixtureReceiptRound(t, recorder, participants, firstSnapshots, 1, firstAt, 1, func(participant *cdfActivationFixtureParticipant, frontier simulation.MarketDataFrontier) {
		participant.first = frontier
	})
	for _, participant := range participants {
		decisionAt := contract.SimulationStartNano + 2_000_000_000 + participant.contract.DecisionPhaseOffset
		if options.strictMechanics {
			decisionAt = contract.SimulationStartNano + 2_000_000_000
		}
		decisionSide := etypes.Buy
		decisionPrice := participant.contract.ReferencePrice - 2*participant.contract.TickSize
		decisionQty := participant.contract.MinimumQualifyingQty
		if options.strictMechanics {
			strictDecision := cdfStrictFixtureDecision(participant, participant.first, firstSnapshots[participant.venueID], firstAt, decisionAt, 0, 0, 0, false, "submit")
			strictDecision.QuoteQty = participant.contract.MinimumQualifyingQty
			decisionSide = cdfFixtureSide(strictDecision.Side)
			decisionPrice = strictDecision.QuotePrice
			decisionQty = strictDecision.QuoteQty
		}
		recorder.RecordDecision(simulation.MarketDataDecision{
			ClientID: participant.clientID, SourceVenue: participant.venueID, Link: participant.link,
			Symbol: cdfActivationSymbol, RequestID: cdfFixtureRequestID(participant.clientID, 1),
			Side: decisionSide, OrderType: etypes.LimitOrder, TimeInForce: etypes.GTC,
			Price: decisionPrice, Qty: decisionQty, DecisionAt: decisionAt, Frontier: participant.first,
		})
	}
	secondDeliveryDelay := int64(1)
	if options.strictMechanics {
		secondDeliveryDelay = 5
	}
	recordCDFFixtureReceiptRound(t, recorder, participants, secondSnapshots, 2, secondAt, secondDeliveryDelay, func(participant *cdfActivationFixtureParticipant, frontier simulation.MarketDataFrontier) {
		participant.second = frontier
	})
	for _, participant := range participants {
		decisionAt := contract.SimulationStartNano + 12_000_000_000 + participant.contract.DecisionPhaseOffset
		if options.strictMechanics {
			decisionAt = contract.SimulationStartNano + 2_000_000_005
		}
		decisionSide := etypes.Sell
		decisionPrice := cdfFixturePostPrice(participant)
		decisionQty := participant.contract.MinimumQualifyingQty
		if options.strictMechanics {
			quantity := cdfFixtureActivationQuantity(participant.venueID, participant.contract, options)
			strictDecision := cdfStrictFixtureDecision(participant, participant.second, secondSnapshots[participant.venueID], secondAt, decisionAt, -quantity, participant.contract.ReferencePrice, contract.SimulationStartNano+2_000_000_000, true, "submit")
			decisionSide = cdfFixtureSide(strictDecision.Side)
			decisionPrice = strictDecision.QuotePrice
			decisionQty = strictDecision.QuoteQty
		}
		recorder.RecordDecision(simulation.MarketDataDecision{
			ClientID: participant.clientID, SourceVenue: participant.venueID, Link: participant.link,
			Symbol: cdfActivationSymbol, RequestID: cdfFixtureRequestID(participant.clientID, 2),
			Side: decisionSide, OrderType: etypes.LimitOrder, TimeInForce: etypes.GTC,
			Price: decisionPrice, Qty: decisionQty,
			DecisionAt: decisionAt, Frontier: participant.second,
		})
	}
	if err := recorder.Finalize(contract.SimulationEndNano); err != nil {
		t.Fatal(err)
	}

	generalEvents := make(map[string][]cdfFixtureEvent, len(contract.VenueIDs))
	bookEvents := make(map[string][]cdfFixtureEvent, len(contract.VenueIDs))
	for _, venueID := range contract.VenueIDs {
		bookEvents[venueID] = append(bookEvents[venueID],
			cdfSnapshotFixtureEvent(firstAt, venueID, 1, cdfFixtureSnapshot(venueID, 1, options)),
			cdfSnapshotFixtureEvent(secondAt, venueID, 2, cdfFixtureSnapshot(venueID, 2, options)),
		)
	}
	for participantOrdinal, participant := range participants {
		appendCDFParticipantEvents(t, participant, participantOrdinal, options, &generalEvents, &bookEvents, firstAt, secondAt)
	}
	for _, venueID := range contract.VenueIDs {
		independentQty := int64(12_000_000)
		if options.strictMechanics {
			independentQty = 20_000_000_000
		}
		if options.dominantVolume {
			independentQty = 1_000_000
		}
		bookEvents[venueID] = append(bookEvents[venueID], cdfFixtureEvent{
			at: contract.SimulationStartNano + 9_000_000_000, event: "Trade", symbol: cdfActivationSymbol,
			payload: cdfTradeEvidence{
				TradeID: 900_000 + uint64(len(venueID)), Price: 300_000_000, Qty: independentQty, Side: "BUY",
				MakerOrderID: 7_000_000 + uint64(len(venueID)), TakerOrderID: 7_100_000 + uint64(len(venueID)),
			},
		})
		thirdAt := contract.SimulationStartNano + 17_000_000_000
		fourthAt := contract.SimulationStartNano + 25_000_000_000
		if options.strictMechanics {
			// Let the second one-sided quote remain live when the public book
			// becomes two-sided so the strict fixture exercises restoration.
			thirdAt = contract.SimulationStartNano + 25_000_000_000
			fourthAt = contract.SimulationStartNano + 40_000_000_000
		}
		if options.dominantDepth {
			fourthAt = contract.SimulationEndNano - 1
		}
		venueBookEvents := append([]cdfFixtureEvent(nil), bookEvents[venueID]...)
		venueBookEvents = append(venueBookEvents,
			cdfSnapshotFixtureEvent(thirdAt, venueID, 3, cdfFixtureSnapshot(venueID, 3, options)),
			cdfFixtureEvent{
				at: thirdAt + 1_000_000_000, event: "BookDelta", symbol: cdfActivationSymbol,
				payload: cdfBookDeltaEvidence{Side: "BUY", Price: 299_800_000, VisibleQty: func() int64 {
					if options.strictMechanics {
						return 100_000_000_000
					}
					return 20_000_000
				}()},
			},
			cdfSnapshotFixtureEvent(fourthAt, venueID, 4, cdfFixtureSnapshot(venueID, 4, options)),
		)
		if options.strictMechanics {
			appendCDFStrictSnapshotCadence(&venueBookEvents, venueID, contract, thirdAt, options)
		}
		bookEvents[venueID] = venueBookEvents
		writeCDFFixtureEvents(t, filepath.Join(dir, "venues", venueID, "general.jsonl"), venueID, generalEvents[venueID])
		writeCDFFixtureEvents(t, filepath.Join(dir, "venues", venueID, "spot", "CDF-USD.jsonl"), venueID, bookEvents[venueID])
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func writeCDFActivationIdentity(t *testing.T, dir string, config cdfActivationConfig, configRaw []byte, contract CDFActivationContract) {
	t.Helper()
	manifest := map[string]any{
		"venue_ids": config.VenueIDs,
		"build": map[string]any{
			"revision": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "modified": false,
			"goos": "linux", "goarch": "amd64", "goamd64": "v1",
		},
		"config": json.RawMessage(configRaw),
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "manifest.json"), manifestRaw)
	writeCDFFixtureFile(t, filepath.Join(dir, "run-config.json"), configRaw)
	configDigest := sha256.Sum256(configRaw)
	metadata := cdfActivationMetadata{
		Seed: config.Seed, SimulatedHorizon: contract.Horizon,
		SimulationStartNano: contract.SimulationStartNano, SimulationEndNano: contract.SimulationEndNano,
		ConfigSHA256: hex.EncodeToString(configDigest[:]),
		BinarySHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		BinaryGOOS:   "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
		GitRevision:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ConfigExperimentID: config.ExperimentID, HypothesisID: config.HypothesisID,
		LogMode: config.LogMode, EvidenceFormat: config.EvidenceFormat,
	}
	metadataRaw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, filepath.Join(dir, "run-metadata.json"), metadataRaw)
}

func recordCDFFixtureReceiptRound(
	t *testing.T,
	recorder *simulation.MarketDataReceiptRecorder,
	participants []*cdfActivationFixtureParticipant,
	snapshots map[string]etypes.BookSnapshot,
	sequence uint64,
	publishedAt int64,
	deliveryDelay int64,
	assign func(*cdfActivationFixtureParticipant, simulation.MarketDataFrontier),
) {
	t.Helper()
	schedules := make(map[*cdfActivationFixtureParticipant]simulation.MarketDataSchedule, len(participants))
	for _, participant := range participants {
		snapshot := snapshots[participant.venueID]
		fingerprint, err := etypes.MarketDataFingerprint(&etypes.MarketDataMsg{
			Type: etypes.MDSnapshot, Symbol: cdfActivationSymbol, SeqNum: sequence,
			Timestamp: publishedAt, Data: &snapshot,
		})
		if err != nil {
			t.Fatal(err)
		}
		schedule := simulation.MarketDataSchedule{
			ClientID: participant.clientID, SourceVenue: participant.venueID, Link: participant.link,
			Symbol: cdfActivationSymbol, Type: etypes.MDSnapshot, Sequence: sequence,
			Fingerprint: fingerprint, PublishedAt: publishedAt, ScheduledAt: publishedAt + 1,
			LinkOrdinal: sequence,
		}
		if recorder.RecordSchedule(schedule) == 0 {
			t.Fatal("record CDF fixture schedule")
		}
		schedules[participant] = schedule
	}
	for _, participant := range participants {
		frontier := recorder.RecordReceipt(simulation.MarketDataReceipt{
			MarketDataSchedule: schedules[participant], DeliveredAt: publishedAt + deliveryDelay,
		})
		if frontier.LinkID == 0 {
			t.Fatal("record CDF fixture receipt")
		}
		assign(participant, frontier)
	}
}

func appendCDFParticipantEvents(
	t *testing.T,
	participant *cdfActivationFixtureParticipant,
	participantOrdinal int,
	options cdfActivationFixtureOptions,
	generalEvents *map[string][]cdfFixtureEvent,
	bookEvents *map[string][]cdfFixtureEvent,
	firstAt int64,
	secondAt int64,
) {
	t.Helper()
	start := RegisteredSV1DActivationContract().SimulationStartNano
	supplier := participant.contract
	quantity := cdfFixtureActivationQuantity(participant.venueID, supplier, options)
	initialSide, initialPrice := cdfFixtureActivationTrade(participant.venueID, supplier, options)
	fee := cdfFixtureFee(initialPrice, quantity, supplier.BasePrecision, supplier.MakerFeeBps)
	notional := cdfFixtureNotional(initialPrice, quantity, supplier.BasePrecision)
	initialEquity := cdfFixtureInitialEquity(supplier)
	positionAfter := quantity
	if initialSide == "SELL" {
		positionAfter = -quantity
	}
	requestOne := cdfFixtureRequestID(participant.clientID, 1)
	requestTwo := cdfFixtureRequestID(participant.clientID, 2)
	cancelRequest := cdfFixtureRequestID(participant.clientID, 3)
	orderOne := cdfFixtureOrderID(participant.clientID, 1)
	orderTwo := cdfFixtureOrderID(participant.clientID, 2)
	tradeID := cdfFixtureTradeID(participant.clientID)
	firstDecisionAt := start + 2_000_000_000 + supplier.DecisionPhaseOffset
	acceptedOneAt := start + 4_000_000_000 + supplier.DecisionPhaseOffset
	fillAt := start + 6_000_000_000 + supplier.DecisionPhaseOffset
	postBalanceAt := start + 8_000_000_000 + supplier.DecisionPhaseOffset
	postDecisionAt := start + 12_000_000_000 + supplier.DecisionPhaseOffset
	acceptedTwoAt := start + 14_000_000_000 + supplier.DecisionPhaseOffset
	cancelDecisionAt := start + 20_000_000_000 + supplier.DecisionPhaseOffset
	cancelledAt := start + 22_000_000_000 + supplier.DecisionPhaseOffset
	if options.strictMechanics {
		strictBase := start + 2_000_000_000
		firstDecisionAt = strictBase
		acceptedOneAt = strictBase + 1
		fillAt = strictBase + 2
		postBalanceAt = strictBase + 3
		postDecisionAt = start + 2_000_000_005
		acceptedTwoAt = start + 45_000_000_000 + supplier.DecisionPhaseOffset
		cancelDecisionAt = start + 46_000_000_000 + supplier.DecisionPhaseOffset
		cancelledAt = start + 48_000_000_000 + supplier.DecisionPhaseOffset
	}

	firstSnapshot := cdfFixtureSnapshot(participant.venueID, 1, options)
	firstDecision := cdfFixtureDecision(participant, participant.first, firstSnapshot, firstAt, firstDecisionAt, "submit")
	if options.strictMechanics {
		firstDecision = cdfStrictFixtureDecision(participant, participant.first, firstSnapshot, firstAt, firstDecisionAt, 0, 0, 0, false, "submit")
		firstDecision.QuoteQty = supplier.MinimumQualifyingQty
		quantity = firstDecision.QuoteQty
	}
	if options.badFingerprint && participantOrdinal == 0 {
		firstDecision.ObservationFingerprint = "00000000000000000000000000000000"
	}
	(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID],
		cdfFixtureEvent{at: start + 500_000_000 + supplier.DecisionPhaseOffset, clientID: participant.clientID, event: "balance_snapshot", payload: cdfFixtureBalanceSnapshotForTrade(participant, start+500_000_000+supplier.DecisionPhaseOffset, 0, false, initialSide, initialPrice)},
		cdfFixtureEvent{at: firstDecisionAt, clientID: participant.clientID, event: "elastic_liquidity_supplier_decision", payload: firstDecision},
	)
	(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID],
		cdfFixtureEvent{at: acceptedOneAt, clientID: participant.clientID, event: "OrderAccepted", symbol: cdfActivationSymbol, payload: cdfAcceptedEvidence{
			OrderID: orderOne, ClientID: participant.clientID, RequestID: requestOne, Side: firstDecision.Side,
			Type: "LIMIT", TimeInForce: "GTC", PostOnly: true, Price: firstDecision.QuotePrice, Qty: quantity,
		}},
		cdfFixtureEvent{at: fillAt, event: "Trade", symbol: cdfActivationSymbol, payload: cdfTradeEvidence{
			TradeID: tradeID, Price: initialPrice, Qty: quantity, Side: cdfOppositeFixtureSide(firstDecision.Side), MakerOrderID: orderOne, TakerOrderID: 8_000_000 + participant.clientID,
		}},
	)
	if options.strictMechanics {
		// Posting a missing-side quote creates the displayed level before the
		// matching engine later removes it ahead of OrderFill.
		(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID], cdfFixtureEvent{
			at: acceptedOneAt, event: "BookDelta", symbol: cdfActivationSymbol,
			payload: cdfBookDeltaEvidence{Side: firstDecision.Side, Price: firstDecision.QuotePrice, VisibleQty: 100_000_000_000},
		})
	}
	if !(options.omitSupplierFill && participantOrdinal == 0) {
		(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID], cdfFixtureEvent{
			at: fillAt, clientID: participant.clientID, event: "elastic_liquidity_supplier_fill",
			payload: cdfFillEvidence{
				Role: supplier.Role, ClientID: participant.clientID, Symbol: cdfActivationSymbol,
				OrderID: orderOne, TradeID: tradeID, Timestamp: fillAt, Side: firstDecision.Side, Price: initialPrice,
				Qty: quantity, FeeAmount: fee, FeeAsset: "USD", IsFull: true,
				PositionBefore: 0, PositionAfter: positionAfter,
			},
		})
	}
	exchangeFillQty := quantity
	if options.badExchangeFill && participantOrdinal == 0 {
		exchangeFillQty++
	}
	(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID], cdfFixtureEvent{
		at: fillAt, clientID: participant.clientID, event: "OrderFill", symbol: cdfActivationSymbol,
		payload: cdfOrderFillEvidence{
			OrderID: orderOne, TradeID: tradeID, Side: firstDecision.Side, Price: initialPrice, Qty: exchangeFillQty,
			FeeAmount: fee, FeeAsset: "USD", FilledQty: exchangeFillQty, RemainingQty: 0, IsFull: true,
		},
	})
	if options.strictMechanics && participantOrdinal == len(RegisteredSV1DActivationContract().Suppliers)-1 {
		// The production exchange reports the fill before publishing the
		// resulting aggregate public-level reduction. The strict fixture follows
		// that order so a later replacement cannot rewrite the fill observation.
		(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID], cdfFixtureEvent{
			at: fillAt, event: "BookDelta", symbol: cdfActivationSymbol,
			payload: cdfBookDeltaEvidence{Side: firstDecision.Side, Price: firstDecision.QuotePrice, VisibleQty: 0},
		})
	}
	borrowed := options.borrowedSupplier && participantOrdinal == 0
	(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID], cdfFixtureEvent{
		at: postBalanceAt, clientID: participant.clientID, event: "balance_snapshot",
		payload: cdfFixtureBalanceSnapshotForTrade(participant, postBalanceAt, positionAfter, borrowed, initialSide, initialPrice),
	})
	if options.omitPostDecision && participantOrdinal == 0 {
		return
	}
	secondSnapshotSequence := uint64(2)
	secondSnapshot := cdfFixtureSnapshot(participant.venueID, secondSnapshotSequence, options)
	postDecision := cdfFixtureDecision(participant, participant.second, secondSnapshot, secondAt, postDecisionAt, "submit")
	if options.strictMechanics {
		postDecision = cdfStrictFixtureDecision(participant, participant.second, secondSnapshot, secondAt, postDecisionAt, positionAfter, supplier.ReferencePrice, firstDecisionAt, true, "submit")
		postDecision.PeakEquityQuote = maxCDFTestInt64(firstDecision.PeakEquityQuote, postDecision.EquityQuote)
		postDecision.DrawdownQuote = postDecision.PeakEquityQuote - postDecision.EquityQuote
	}
	if !options.strictMechanics {
		postDecision.Position = quantity
		postDecision.TargetPosition = 0
		postDecision.GrossInventory = supplier.InitialBaseBalance + quantity
		postDecision.Side = "SELL"
		postDecision.QuotePrice = cdfFixturePostPrice(participant)
		postDecision.QuoteQty = quantity
		postDecision.QuoteCashAvailable = supplier.InitialQuoteBalance - notional - fee
		postDecision.QuoteCashRequired = 0
		postDecision.EquityQuote = cdfFixtureEquityAtMark(supplier, postDecision.RiskMarkPrice, supplier.InitialBaseBalance+quantity, supplier.InitialQuoteBalance-notional-fee)
		postDecision.PeakEquityQuote = maxCDFTestInt64(initialEquity, postDecision.EquityQuote)
		postDecision.LossFromInitialQuote = maxCDFTestInt64(0, initialEquity-postDecision.EquityQuote)
		postDecision.DrawdownQuote = postDecision.PeakEquityQuote - postDecision.EquityQuote
		if participant.venueID == "north" {
			postDecision.QuotePriceSource = "one_sided_missing_side_blended"
		}
	}
	postDecision.QuoteRequestID = requestTwo
	postDecision.QuoteSubmittedAt = postDecisionAt
	(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID], cdfFixtureEvent{
		at: postDecisionAt, clientID: participant.clientID, event: "elastic_liquidity_supplier_decision", payload: postDecision,
	})
	(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID], cdfFixtureEvent{
		at: acceptedTwoAt, clientID: participant.clientID, event: "OrderAccepted", symbol: cdfActivationSymbol,
		payload: cdfAcceptedEvidence{
			OrderID: orderTwo, ClientID: participant.clientID, RequestID: requestTwo, Side: postDecision.Side,
			Type: "LIMIT", TimeInForce: "GTC", PostOnly: true, Price: postDecision.QuotePrice, Qty: postDecision.QuoteQty,
		},
	})
	if options.strictMechanics {
		// A missing-side quote becomes public depth when the exchange accepts
		// it, allowing the later snapshot to prove one-sided restoration.
		(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID], cdfFixtureEvent{
			at: acceptedTwoAt, event: "BookDelta", symbol: cdfActivationSymbol,
			payload: cdfBookDeltaEvidence{Side: postDecision.Side, Price: postDecision.QuotePrice, VisibleQty: 100_000_000_000},
		})
	}
	cancelDecision := postDecision
	if options.strictMechanics {
		cancelDecision = cdfStrictFixtureDecision(participant, participant.second, secondSnapshot, secondAt, cancelDecisionAt, positionAfter, postDecision.ReferencePrice, postDecision.DecisionTime, true, "cancel")
	}
	cancelDecision.DecisionTime = cancelDecisionAt
	cancelDecision.ObservationAge = cancelDecisionAt - secondAt
	cancelDecision.Action = "cancel"
	cancelDecision.Reason = "reprice_for_inventory_or_touch"
	cancelDecision.QuoteOrderID = orderTwo
	cancelDecision.QuoteRequestID = requestTwo
	cancelDecision.QuoteQty = postDecision.QuoteQty
	cancelDecision.CancelRequestID = cancelRequest
	cancelDecision.QuotePrice = postDecision.QuotePrice + supplier.TickSize
	(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID], cdfFixtureEvent{
		at: cancelDecisionAt, clientID: participant.clientID, event: "elastic_liquidity_supplier_decision", payload: cancelDecision,
	})
	if !(options.omitCancellation && participantOrdinal == 0) {
		(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID], cdfFixtureEvent{
			at: cancelledAt, clientID: participant.clientID, event: "OrderCancelled", symbol: cdfActivationSymbol,
			payload: cdfCancelledEvidence{OrderID: orderTwo, RequestID: cancelRequest, RemainingQty: postDecision.QuoteQty},
		})
	}
}

func cdfFixtureDecision(
	participant *cdfActivationFixtureParticipant,
	frontier simulation.MarketDataFrontier,
	snapshot etypes.BookSnapshot,
	observationAt int64,
	decisionAt int64,
	action string,
) cdfDecisionEvidence {
	supplier := participant.contract
	bestBid, bestBidQty := cdfBestBid(snapshot.Bids)
	bestAsk, bestAskQty := cdfBestAsk(snapshot.Asks)
	mark := supplier.ReferencePrice
	mode := "two_sided"
	riskSource := "two_sided_midpoint"
	quoteSource := "two_sided_touch"
	if (bestBid > 0) != (bestAsk > 0) {
		mode = "one_sided"
		mark = bestBid
		riskSource = "one_sided_bid"
		quoteSource = "one_sided_missing_side_blended"
	}
	price := bestBid
	return cdfDecisionEvidence{
		Role: supplier.Role, ClientID: participant.clientID, Symbol: cdfActivationSymbol,
		DecisionTime: decisionAt, DecisionPhaseOffset: supplier.DecisionPhaseOffset,
		ObservationTime: observationAt, ObservationAge: decisionAt - observationAt,
		ObservationSequence: frontier.Ordinal, ObservationLinkID: frontier.LinkID,
		ObservationOrdinal: frontier.Ordinal, ObservationDeliveredAt: frontier.DeliveredAt,
		ObservationFingerprint: hex.EncodeToString(frontier.Fingerprint[:]),
		ObservationDigest:      hex.EncodeToString(frontier.Digest[:]),
		BestBid:                bestBid, BestBidQty: bestBidQty, BestAsk: bestAsk, BestAskQty: bestAskQty,
		MarkPrice: mark, RiskMarkPrice: mark, RiskMarkCurrent: true, LocalBookMode: mode,
		QuotePriceSource: quoteSource, RiskMarkSource: riskSource,
		ReferencePrice: supplier.ReferencePrice, Position: 0, TargetPosition: supplier.MinimumQualifyingQty,
		InventoryLimit: supplier.MaxPosition, InitialBaseBalance: supplier.InitialBaseBalance,
		GrossInventory: supplier.InitialBaseBalance, GrossInventoryLimit: supplier.MaxInventory,
		Action: action, Reason: "inventory_target_gap", Side: "BUY", QuotePrice: price,
		QuoteQty: supplier.MinimumQualifyingQty, MinimumQualifyingQty: supplier.MinimumQualifyingQty,
		RegisteredMinimumExecutableQty: supplier.RegisteredMinimumExecutableQty,
		QuoteRequestID:                 cdfFixtureRequestID(participant.clientID, 1), QuoteSubmittedAt: decisionAt,
		QuoteCashAvailable: supplier.InitialQuoteBalance, QuoteCashReserved: 0,
		QuoteCashRequired:  cdfFixtureNotional(price, supplier.MinimumQualifyingQty, supplier.BasePrecision) + cdfFixtureFee(price, supplier.MinimumQualifyingQty, supplier.BasePrecision, supplier.MakerFeeBps),
		InitialEquityQuote: cdfFixtureInitialEquity(supplier), EquityQuote: cdfFixtureInitialEquity(supplier),
		PeakEquityQuote: cdfFixtureInitialEquity(supplier), MaxLossQuote: supplier.MaxLossQuote,
		EquityAvailable: true,
	}
}

func cdfFixtureActivationQuantity(venueID string, supplier CDFSupplierContract, options cdfActivationFixtureOptions) int64 {
	return supplier.MinimumQualifyingQty
}

func cdfFixtureActivationTrade(venueID string, supplier CDFSupplierContract, options cdfActivationFixtureOptions) (string, int64) {
	if !options.strictMechanics {
		return "BUY", supplier.ReferencePrice - 2*supplier.TickSize
	}
	snapshot := cdfFixtureSnapshot(venueID, 1, options)
	bestBid, bestBidQty := cdfBestBid(snapshot.Bids)
	bestAsk, bestAskQty := cdfBestAsk(snapshot.Asks)
	mode := cdfLocalBookMode(bestBid, bestBidQty, bestAsk, bestAskQty, supplier.TickSize)
	anchor, _ := cdfLocalAnchor(cdfDecisionEvidence{
		BestBid: bestBid, BestBidQty: bestBidQty, BestAsk: bestAsk, BestAskQty: bestAskQty, LocalBookMode: mode,
	}, supplier)
	target := cdfTargetPosition(supplier.ReferencePrice, anchor, supplier)
	if target < 0 {
		probe := cdfDecisionEvidence{
			ReferencePrice: supplier.ReferencePrice, BestBid: bestBid, BestBidQty: bestBidQty,
			BestAsk: bestAsk, BestAskQty: bestAskQty, Side: "SELL", QuoteQty: -target,
		}
		price, ok := expectedCDFMissingSideQuote(probe, supplier)
		if ok {
			return "SELL", price
		}
	}
	return "BUY", bestBid
}

func cdfFixtureSide(side string) etypes.Side {
	if side == "SELL" {
		return etypes.Sell
	}
	return etypes.Buy
}

func cdfOppositeFixtureSide(side string) string {
	if side == "BUY" {
		return "SELL"
	}
	return "BUY"
}

func cdfStrictFixtureDecision(
	participant *cdfActivationFixtureParticipant,
	frontier simulation.MarketDataFrontier,
	snapshot etypes.BookSnapshot,
	observationAt int64,
	decisionAt int64,
	position int64,
	previousReference int64,
	previousReferenceAt int64,
	previousReferenceSet bool,
	action string,
) cdfDecisionEvidence {
	supplier := participant.contract
	bestBid, bestBidQty := cdfBestBid(snapshot.Bids)
	bestAsk, bestAskQty := cdfBestAsk(snapshot.Asks)
	mode := cdfLocalBookMode(bestBid, bestBidQty, bestAsk, bestAskQty, supplier.TickSize)
	anchor, hasAnchor := cdfLocalAnchor(cdfDecisionEvidence{
		BestBid: bestBid, BestBidQty: bestBidQty, BestAsk: bestAsk, BestAskQty: bestAskQty,
		LocalBookMode: mode,
	}, supplier)
	reference := supplier.ReferencePrice
	if previousReferenceSet {
		reference, _, _ = advanceCDFReference(previousReference, previousReferenceAt, true, anchor, decisionAt, supplier.ReferenceHalfLife)
	}
	target := int64(0)
	if hasAnchor {
		target = cdfTargetPosition(reference, anchor, supplier)
	}
	gap, _ := checkedCDFSub(target, position)
	side := ""
	if gap > 0 {
		side = "BUY"
	} else if gap < 0 {
		side = "SELL"
	}
	quoteSource := ""
	quotePrice := int64(0)
	if mode == "two_sided" {
		quoteSource = "two_sided_touch"
		if side == "BUY" {
			quotePrice = bestBid
		} else if side == "SELL" {
			quotePrice = bestAsk
		}
	} else if mode == "one_sided" {
		if side == "BUY" && bestBid > 0 {
			quoteSource, quotePrice = "one_sided_present_touch", bestBid
		} else if side == "SELL" && bestAsk > 0 {
			quoteSource, quotePrice = "one_sided_present_touch", bestAsk
		} else if side != "" {
			quoteSource = "one_sided_missing_side_blended"
			probe := cdfDecisionEvidence{
				ReferencePrice: reference, BestBid: bestBid, BestBidQty: bestBidQty,
				BestAsk: bestAsk, BestAskQty: bestAskQty, Side: side, QuoteQty: absCDFTestInt64(gap),
			}
			quotePrice, _ = expectedCDFMissingSideQuote(probe, supplier)
		}
	}
	quoteQty := absCDFTestInt64(gap)
	if quoteQty > supplier.MaxQuoteQty {
		quoteQty = supplier.MaxQuoteQty
	}
	quoteCashAvailable := supplier.InitialQuoteBalance
	if position != 0 {
		initialSide, initialPrice := cdfFixtureActivationTrade(participant.venueID, supplier, cdfActivationFixtureOptions{strictMechanics: true})
		initialQty := absCDFTestInt64(position)
		initialFee := cdfFixtureFee(initialPrice, initialQty, supplier.BasePrecision, supplier.MakerFeeBps)
		initialNotional := cdfFixtureNotional(initialPrice, initialQty, supplier.BasePrecision)
		if initialSide == "BUY" {
			quoteCashAvailable -= initialNotional + initialFee
		} else {
			quoteCashAvailable += initialNotional - initialFee
		}
	}
	quoteCashRequired := int64(0)
	if side == "BUY" {
		quoteCashRequired = cdfFixtureNotional(quotePrice, quoteQty, supplier.BasePrecision) + cdfFixtureFee(quotePrice, quoteQty, supplier.BasePrecision, supplier.MakerFeeBps)
	}
	riskMark := int64(0)
	riskSource := ""
	equityAvailable := false
	if hasAnchor {
		riskMark = anchor
		riskSource = "two_sided_midpoint"
		if mode == "one_sided" {
			if bestBid > 0 {
				riskSource = "one_sided_bid"
			} else if position == 0 {
				riskSource = "one_sided_ask_zero_inventory"
			} else {
				riskMark = 0
				riskSource = "one_sided_ask_unavailable"
			}
		}
		equityAvailable = riskMark > 0
	}
	equity := cdfFixtureInitialEquity(supplier)
	if equityAvailable {
		equity = cdfFixtureEquityAtMark(supplier, riskMark, supplier.InitialBaseBalance+position, quoteCashAvailable)
	}
	peak := maxCDFTestInt64(cdfFixtureInitialEquity(supplier), equity)
	if previousReferenceSet {
		firstSnapshot := cdfFixtureSnapshot(participant.venueID, 1, cdfActivationFixtureOptions{strictMechanics: true})
		firstBid, firstBidQty := cdfBestBid(firstSnapshot.Bids)
		firstAsk, firstAskQty := cdfBestAsk(firstSnapshot.Asks)
		firstMode := cdfLocalBookMode(firstBid, firstBidQty, firstAsk, firstAskQty, supplier.TickSize)
		firstAnchor, firstAnchorOK := cdfLocalAnchor(cdfDecisionEvidence{
			BestBid: firstBid, BestBidQty: firstBidQty, BestAsk: firstAsk, BestAskQty: firstAskQty, LocalBookMode: firstMode,
		}, supplier)
		if firstAnchorOK {
			firstEquity := cdfFixtureEquityAtMark(supplier, firstAnchor, supplier.InitialBaseBalance, supplier.InitialQuoteBalance)
			peak = maxCDFTestInt64(peak, firstEquity)
		}
	}
	loss := maxCDFTestInt64(0, cdfFixtureInitialEquity(supplier)-equity)
	return cdfDecisionEvidence{
		Role: supplier.Role, ClientID: participant.clientID, Symbol: cdfActivationSymbol,
		DecisionTime: decisionAt, DecisionPhaseOffset: supplier.DecisionPhaseOffset,
		ObservationTime: observationAt, ObservationAge: decisionAt - observationAt,
		ObservationSequence: frontier.Ordinal, ObservationLinkID: frontier.LinkID, ObservationOrdinal: frontier.Ordinal,
		ObservationDeliveredAt: frontier.DeliveredAt, ObservationFingerprint: hex.EncodeToString(frontier.Fingerprint[:]),
		ObservationDigest: hex.EncodeToString(frontier.Digest[:]), BestBid: bestBid, BestBidQty: bestBidQty,
		BestAsk: bestAsk, BestAskQty: bestAskQty, MarkPrice: anchor, RiskMarkPrice: riskMark,
		RiskMarkCurrent: equityAvailable, LocalBookMode: mode, QuotePriceSource: quoteSource, RiskMarkSource: riskSource,
		ReferencePrice: reference, Position: position, TargetPosition: target, InventoryLimit: supplier.MaxPosition,
		InitialBaseBalance: supplier.InitialBaseBalance, GrossInventory: supplier.InitialBaseBalance + position,
		GrossInventoryLimit: supplier.MaxInventory, Action: action, Reason: "inventory_target_gap", Side: side,
		QuotePrice: quotePrice, QuoteQty: quoteQty, MinimumQualifyingQty: supplier.MinimumQualifyingQty,
		RegisteredMinimumExecutableQty: supplier.RegisteredMinimumExecutableQty,
		QuoteRequestID:                 cdfFixtureRequestID(participant.clientID, 1), QuoteSubmittedAt: decisionAt,
		QuoteCashAvailable: quoteCashAvailable, QuoteCashRequired: quoteCashRequired,
		InitialEquityQuote: cdfFixtureInitialEquity(supplier), EquityQuote: equity, PeakEquityQuote: peak,
		LossFromInitialQuote: loss, DrawdownQuote: peak - equity, MaxLossQuote: supplier.MaxLossQuote,
		EquityAvailable: equityAvailable,
	}
}

func absCDFTestInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func cdfFixtureBalanceSnapshot(participant *cdfActivationFixtureParticipant, at, position int64, borrowed bool) cdfBalanceSnapshotEvidence {
	supplier := participant.contract
	price := supplier.ReferencePrice - 2*supplier.TickSize
	fee := cdfFixtureFee(price, position, supplier.BasePrecision, supplier.MakerFeeBps)
	quote := supplier.InitialQuoteBalance - cdfFixtureNotional(price, position, supplier.BasePrecision) - fee
	quoteBalance := cdfBalanceEvidence{Asset: "USD", Free: quote, NetAsset: quote}
	borrowedMap := map[string]int64{}
	if borrowed {
		quoteBalance.Borrowed = 1
		borrowedMap["USD"] = 1
	}
	return cdfBalanceSnapshotEvidence{
		Timestamp: at, ClientID: participant.clientID,
		SpotBalances: []cdfBalanceEvidence{
			{Asset: "CDF", Free: supplier.InitialBaseBalance + position, NetAsset: supplier.InitialBaseBalance + position},
			quoteBalance,
		},
		PerpBalances: []cdfBalanceEvidence{}, Borrowed: borrowedMap,
	}
}

func cdfFixtureBalanceSnapshotForTrade(participant *cdfActivationFixtureParticipant, at, position int64, borrowed bool, side string, price int64) cdfBalanceSnapshotEvidence {
	if side == "BUY" && price == participant.contract.ReferencePrice-2*participant.contract.TickSize {
		return cdfFixtureBalanceSnapshot(participant, at, position, borrowed)
	}
	supplier := participant.contract
	quantity := absCDFTestInt64(position)
	notional := cdfFixtureNotional(price, quantity, supplier.BasePrecision)
	fee := cdfFixtureFee(price, quantity, supplier.BasePrecision, supplier.MakerFeeBps)
	quote := supplier.InitialQuoteBalance
	if side == "BUY" {
		quote -= notional + fee
	} else {
		quote += notional - fee
	}
	return cdfBalanceSnapshotEvidence{
		Timestamp: at, ClientID: participant.clientID,
		SpotBalances: []cdfBalanceEvidence{
			{Asset: "CDF", Free: supplier.InitialBaseBalance + position, NetAsset: supplier.InitialBaseBalance + position},
			{Asset: "USD", Free: quote, NetAsset: quote},
		},
		PerpBalances: []cdfBalanceEvidence{}, Borrowed: map[string]int64{},
	}
}

func cdfFixtureSnapshot(venueID string, sequence uint64, options cdfActivationFixtureOptions) etypes.BookSnapshot {
	independentDepth := int64(20_000_000)
	if options.strictMechanics {
		independentDepth = 100_000_000_000
	}
	snapshot := etypes.BookSnapshot{
		Bids: []etypes.PriceLevel{{Price: 299_800_000, VisibleQty: independentDepth}},
		Asks: []etypes.PriceLevel{{Price: 300_200_000, VisibleQty: independentDepth}},
	}
	if options.strictMechanics && sequence <= 2 {
		snapshot.Bids[0].Price = 300_200_000
		snapshot.Asks = []etypes.PriceLevel{}
	}
	if !options.strictMechanics && sequence == 2 && venueID == "north" {
		snapshot.Asks = []etypes.PriceLevel{}
	}
	if !options.strictMechanics && sequence == 3 && venueID == "north" {
		snapshot.Asks[0].Price = 299_900_000
	}
	if options.strictMechanics && sequence >= 3 {
		snapshot.Asks[0].Price = 300_300_000
	}
	if sequence == 3 && options.dominantDepth {
		snapshot.Asks[0].VisibleQty = 4_000_000
	}
	return snapshot
}

func cdfSnapshotFixtureEvent(at int64, venueID string, sequence uint64, snapshot etypes.BookSnapshot) cdfFixtureEvent {
	return cdfFixtureEvent{
		at: at, event: "BookSnapshot", symbol: cdfActivationSymbol,
		payload: map[string]any{
			"bids": snapshot.Bids, "asks": snapshot.Asks, "source_sequence": sequence,
			"public_bids": snapshot.Bids, "public_asks": snapshot.Asks,
		},
	}
}

func appendCDFStrictSnapshotCadence(events *[]cdfFixtureEvent, venueID string, contract CDFActivationContract, twoSidedAt int64, options cdfActivationFixtureOptions) {
	registeredSnapshotTimes := make(map[int64]struct{})
	for _, event := range *events {
		if event.event == "BookSnapshot" {
			registeredSnapshotTimes[event.at] = struct{}{}
		}
	}
	for at := contract.SimulationStartNano + contract.ObservationIntervalNano; at <= contract.SimulationEndNano; at += contract.ObservationIntervalNano {
		if _, exists := registeredSnapshotTimes[at]; exists {
			continue
		}
		sequence := uint64(1_000_000) + uint64((at-contract.SimulationStartNano)/contract.ObservationIntervalNano)
		snapshot := cdfFixtureSnapshot(venueID, sequence, options)
		if at < twoSidedAt && options.strictMechanics {
			snapshot.Bids[0].Price = 300_200_000
			snapshot.Asks = []etypes.PriceLevel{}
		}
		*events = append(*events, cdfSnapshotFixtureEvent(at, venueID, sequence, snapshot))
	}
}

func cdfFixturePostPrice(participant *cdfActivationFixtureParticipant) int64 {
	if participant.venueID == "north" {
		return participant.contract.ReferencePrice - participant.contract.TickSize
	}
	return participant.contract.ReferencePrice + 2*participant.contract.TickSize
}

func cdfFixtureRequestID(clientID uint64, ordinal uint64) uint64 { return clientID*100 + ordinal }
func cdfFixtureOrderID(clientID uint64, ordinal uint64) uint64   { return clientID*1_000 + ordinal }
func cdfFixtureTradeID(clientID uint64) uint64                   { return clientID*10_000 + 1 }

func cdfFixtureInitialEquity(supplier CDFSupplierContract) int64 {
	return cdfFixtureNotional(supplier.ReferencePrice, supplier.InitialBaseBalance, supplier.BasePrecision) + supplier.InitialQuoteBalance
}

func cdfFixtureTerminalEquity(supplier CDFSupplierContract, baseBalance, quoteBalance int64) int64 {
	return cdfFixtureNotional(supplier.ReferencePrice, baseBalance, supplier.BasePrecision) + quoteBalance
}

func cdfFixtureEquityAtMark(supplier CDFSupplierContract, mark, baseBalance, quoteBalance int64) int64 {
	return cdfFixtureNotional(mark, baseBalance, supplier.BasePrecision) + quoteBalance
}

func maxCDFTestInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func cdfFixtureNotional(price, quantity, basePrecision int64) int64 {
	return price * quantity / basePrecision
}

func cdfFixtureFee(price, quantity, basePrecision, basisPoints int64) int64 {
	return cdfFixtureNotional(price, quantity, basePrecision) * basisPoints / 10_000
}

func writeCDFFixtureEvents(t *testing.T, path, venueID string, events []cdfFixtureEvent) {
	t.Helper()
	for index := range events {
		events[index].ordinal = index
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].at != events[j].at {
			return events[i].at < events[j].at
		}
		return events[i].ordinal < events[j].ordinal
	})
	var output bytes.Buffer
	for sequence, event := range events {
		payload, err := json.Marshal(event.payload)
		if err != nil {
			t.Fatal(err)
		}
		row := map[string]any{
			"client_id": event.clientID,
			"data": map[string]any{
				"venue_id": venueID, "sequence": sequence + 1, "symbol": event.symbol,
				"payload": json.RawMessage(payload),
			},
			"event": event.event, "sim_ts": event.at,
		}
		raw, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		output.Write(raw)
		output.WriteByte('\n')
	}
	writeCDFFixtureFile(t, path, output.Bytes())
}

func writeCDFFixtureFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasCDFActivationFailure(checks []CDFActivationCheck, wanted string) bool {
	for _, check := range checks {
		if check.Failure == wanted {
			return true
		}
	}
	return false
}

func copyCDFEventFixture(t *testing.T, source, destination string) error {
	t.Helper()
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o644)
	})
}

func deleteCDFFixturePayloadField(t *testing.T, path, eventName, fieldName string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	deleted := false
	for index, line := range lines {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(line, &envelope); err != nil {
			t.Fatal(err)
		}
		var event string
		if err := json.Unmarshal(envelope["event"], &event); err != nil {
			t.Fatal(err)
		}
		if deleted || event != eventName {
			continue
		}
		var data map[string]json.RawMessage
		if err := json.Unmarshal(envelope["data"], &data); err != nil {
			t.Fatal(err)
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(data["payload"], &payload); err != nil {
			t.Fatal(err)
		}
		if _, exists := payload[fieldName]; !exists {
			t.Fatalf("fixture field %q is absent before mutation", fieldName)
		}
		delete(payload, fieldName)
		data["payload"], err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		envelope["data"], err = json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		lines[index], err = json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		deleted = true
	}
	if !deleted {
		t.Fatalf("event %q was not found", eventName)
	}
	updated := append(bytes.Join(lines, []byte{'\n'}), '\n')
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCDFReferenceAndTargetReconstructionMatchesRegisteredFormulas(t *testing.T) {
	if got, updatedAt, updateSet := advanceCDFReference(100, 0, false, 200, 1_000_000_000, 1_000_000_000); got != 100 || updatedAt != 1_000_000_000 || !updateSet {
		t.Fatalf("first reference update = (%d, %d, %t), want (100, 1000000000, true)", got, updatedAt, updateSet)
	}
	if got, updatedAt, updateSet := advanceCDFReference(100, 1_000_000_000, true, 200, 2_000_000_000, 1_000_000_000); got != 150 || updatedAt != 2_000_000_000 || !updateSet {
		t.Fatalf("half-life reference update = (%d, %d, %t), want (150, 2000000000, true)", got, updatedAt, updateSet)
	}
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	if got := cdfTargetPosition(300_000_000, 299_900_000, contract); got != 399_999_999 {
		t.Fatalf("target position = %d, want 399999999", got)
	}
}

func TestCDFMarkedPositionEquityUsesCheckedInventoryAndCash(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	mark := contract.ReferencePrice - contract.TickSize
	position := contract.MinimumQualifyingQty
	quoteAvailable := contract.InitialQuoteBalance - cdfFixtureNotional(mark, position, contract.BasePrecision)
	quoteReserved := int64(17)
	want := cdfFixtureNotional(mark, contract.InitialBaseBalance+position, contract.BasePrecision) + quoteAvailable + quoteReserved
	if got, ok := cdfMarkedPositionEquity(position, mark, quoteAvailable, quoteReserved, contract); !ok || got != want {
		t.Fatalf("marked position equity = (%d, %t), want (%d, true)", got, ok, want)
	}
	invalid := []struct {
		name           string
		position       int64
		mark           int64
		quoteAvailable int64
		quoteReserved  int64
	}{
		{name: "non-positive mark", position: 0, mark: 0, quoteAvailable: 1, quoteReserved: 0},
		{name: "negative available cash", position: 0, mark: mark, quoteAvailable: -1, quoteReserved: 0},
		{name: "negative reserved cash", position: 0, mark: mark, quoteAvailable: 1, quoteReserved: -1},
		{name: "inventory overflow", position: math.MaxInt64, mark: mark, quoteAvailable: 1, quoteReserved: 0},
		{name: "cash overflow", position: 0, mark: mark, quoteAvailable: math.MaxInt64, quoteReserved: 1},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := cdfMarkedPositionEquity(test.position, test.mark, test.quoteAvailable, test.quoteReserved, contract); ok {
				t.Fatalf("invalid marked equity = (%d, true)", got)
			}
		})
	}
}

func TestCDFStrictDecisionStateDistinguishesCachedAndCurrentMarks(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	initialEquity := cdfFixtureInitialEquity(contract)
	state := &cdfSupplierState{
		contract:               contract,
		initialBaseBalance:     contract.InitialBaseBalance,
		initialQuoteBalance:    contract.InitialQuoteBalance,
		reconstructedReference: contract.ReferencePrice,
		reconstructedRiskMark:  contract.ReferencePrice,
		reconstructedEquity:    initialEquity,
		reconstructedPeak:      initialEquity,
		equityStateSet:         true,
		audit:                  CDFSupplierActivationAudit{Role: contract.Role, InitialEquity: initialEquity},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	uncurrent := cdfDecisionEvidence{
		ReferencePrice: contract.ReferencePrice, RiskMarkPrice: 0, RiskMarkCurrent: false,
		EquityQuote: initialEquity, PeakEquityQuote: initialEquity,
		QuoteCashAvailable: contract.InitialQuoteBalance,
	}
	if !audit.validateCDFDecisionMechanics(Event{VenueID: "north", ClientID: 7}, state, uncurrent) {
		t.Fatalf("cached-equity decision was rejected: %+v", audit.Checks)
	}
	if got := audit.validateCDFDecisionEquity(Event{VenueID: "north", ClientID: 7}, state, uncurrent, true); got != initialEquity || len(audit.Checks) != 0 {
		t.Fatalf("cached-equity decision changed state: peak=%d checks=%+v", got, audit.Checks)
	}

	quantity := contract.MinimumQualifyingQty
	notional := cdfFixtureNotional(contract.ReferencePrice, quantity, contract.BasePrecision)
	fee := cdfFixtureFee(contract.ReferencePrice, quantity, contract.BasePrecision, contract.MakerFeeBps)
	state.fillQuoteDelta = -(notional + fee)
	expectedCash := contract.InitialQuoteBalance + state.fillQuoteDelta
	state.currentPosition = quantity
	current := uncurrent
	current.BestBid, current.BestBidQty = contract.ReferencePrice-contract.TickSize, quantity
	current.BestAsk, current.BestAskQty = contract.ReferencePrice+contract.TickSize, quantity
	current.MarkPrice, current.RiskMarkPrice, current.RiskMarkCurrent = contract.ReferencePrice, contract.ReferencePrice, true
	current.EquityAvailable = true
	current.LocalBookMode, current.RiskMarkSource = "two_sided", "two_sided_midpoint"
	current.ReferencePrice, current.Position, current.TargetPosition = contract.ReferencePrice, quantity, contract.BaseHolding
	current.GrossInventory = contract.InitialBaseBalance + quantity
	current.QuoteCashAvailable, current.QuoteCashReserved = expectedCash, 0
	current.EquityQuote = cdfFixtureEquityAtMark(contract, contract.ReferencePrice, current.GrossInventory, expectedCash)
	current.PeakEquityQuote = maxCDFTestInt64(initialEquity, current.EquityQuote)
	current.LossFromInitialQuote = maxCDFTestInt64(0, initialEquity-current.EquityQuote)
	current.DrawdownQuote = current.PeakEquityQuote - current.EquityQuote
	if !audit.validateCDFDecisionMechanics(Event{VenueID: "north", ClientID: 7}, state, current) {
		t.Fatalf("current-mark decision mechanics were rejected: %+v", audit.Checks)
	}
	if got := audit.validateCDFDecisionEquity(Event{VenueID: "north", ClientID: 7}, state, current, true); got != current.PeakEquityQuote || len(audit.Checks) != 0 {
		t.Fatalf("current-mark decision was rejected: peak=%d checks=%+v", got, audit.Checks)
	}
	forged := current
	forged.QuoteCashAvailable++
	audit.validateCDFDecisionEquity(Event{VenueID: "north", ClientID: 7}, state, forged, true)
	if !hasCDFActivationFailure(audit.Checks, "CDF decision quote cash does not match the actor-visible fill ledger") {
		t.Fatalf("forged actor cash passed: %+v", audit.Checks)
	}
}

func TestCDFMissingSideQuoteRequiresExactRegisteredPrice(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	decision := cdfDecisionEvidence{
		ReferencePrice: 300_000_000, BestBid: 299_800_000, BestBidQty: 2_000_000,
		Side: "SELL", QuoteQty: contract.MinimumQualifyingQty,
	}
	if got, ok := expectedCDFMissingSideQuote(decision, contract); !ok || got != 299_900_000 {
		t.Fatalf("missing-side sell quote = (%d, %t), want (299900000, true)", got, ok)
	}
	decision.QuotePrice = 299_900_000
	if !validCDFMissingSideQuote(decision, contract) {
		t.Fatal("exact missing-side quote was rejected")
	}
	decision.QuotePrice = 300_000_000
	if validCDFMissingSideQuote(decision, contract) {
		t.Fatal("alternative within-touch quote was accepted")
	}
}

func TestCollectCDFOrderedEventsRejectsDuplicateGlobalIdentity(t *testing.T) {
	dir := t.TempDir()
	generalPath := filepath.Join(dir, "venues", "north", "general.jsonl")
	bookPath := filepath.Join(dir, "venues", "north", "spot", "CDF-USD.jsonl")
	writeOrderedCDFTestEvent(t, generalPath, "north", 1, 5, "balance_snapshot", "general", map[string]any{})
	writeOrderedCDFTestEvent(t, bookPath, "north", 1, 5, "BookSnapshot", cdfActivationLogName, map[string]any{})
	run := &Run{files: []string{generalPath, bookPath}}
	if _, err := collectCDFOrderedEvents(run); err == nil {
		t.Fatal("duplicate global frame identity was accepted")
	}
}

func TestCDFBalanceSnapshotCannotForgeIntermediateFillState(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{
		contract: contract, initialBaseBalance: contract.InitialBaseBalance, initialQuoteBalance: contract.InitialQuoteBalance,
		audit: CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7, InitialEquity: 1},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	fill := cdfFillEvidence{
		Role: contract.Role, ClientID: 7, Symbol: cdfActivationSymbol, OrderID: 11, TradeID: 12,
		Timestamp: 10, Side: "BUY", Price: contract.ReferencePrice, Qty: contract.MinimumQualifyingQty,
		FeeAmount: cdfFixtureFee(contract.ReferencePrice, contract.MinimumQualifyingQty, contract.BasePrecision, contract.MakerFeeBps),
		FeeAsset:  contract.QuoteAsset, IsFull: true, PositionBefore: 0, PositionAfter: contract.MinimumQualifyingQty,
	}
	fillRaw, err := json.Marshal(fill)
	if err != nil {
		t.Fatal(err)
	}
	states := map[cdfParticipantKey]*cdfSupplierState{{"north", 7}: state}
	audit.processCDFFill(Event{SimTS: 10, VenueID: "north", ClientID: 7, payload: fillRaw}, states, map[cdfFillKey]cdfFillEvidence{})
	orders := map[cdfOrderKey]*cdfOrderState{{"north", 7, 11}: {
		side: "BUY", price: fill.Price, originalQty: fill.Qty, remainingQty: fill.Qty,
	}}
	exchangeFill := cdfOrderFillEvidence{
		OrderID: 11, TradeID: 12, Side: "BUY", Price: fill.Price, Qty: fill.Qty,
		FeeAmount: fill.FeeAmount, FeeAsset: fill.FeeAsset, FilledQty: fill.Qty,
		RemainingQty: 0, IsFull: true,
	}
	exchangeFillRaw, err := json.Marshal(exchangeFill)
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFOrderFill(Event{SimTS: 10, VenueID: "north", ClientID: 7, payload: exchangeFillRaw}, states, orders, map[cdfFillKey]cdfOrderFillEvidence{})
	balances := cdfBalanceSnapshotEvidence{
		Timestamp: 11, ClientID: 7,
		SpotBalances: []cdfBalanceEvidence{
			{Asset: contract.BaseAsset, Free: contract.InitialBaseBalance, NetAsset: contract.InitialBaseBalance},
			{Asset: contract.QuoteAsset, Free: contract.InitialQuoteBalance, NetAsset: contract.InitialQuoteBalance},
		},
		PerpBalances: []cdfBalanceEvidence{}, Borrowed: map[string]int64{},
	}
	balancesRaw, err := json.Marshal(balances)
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFBalanceSnapshot(Event{SimTS: 11, VenueID: "north", ClientID: 7, payload: balancesRaw}, states)
	if !hasCDFActivationFailure(audit.Checks, "supplier balance snapshot does not reconcile to prior exchange-matched fills") {
		t.Fatalf("forged intermediate balance passed: %+v", audit.Checks)
	}
}

func TestCDFStrictBalanceReconstructionIgnoresActorOnlyFill(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{
		contract: contract, initialBaseBalance: contract.InitialBaseBalance, initialQuoteBalance: contract.InitialQuoteBalance,
		audit: CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7, InitialEquity: 1},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	fill := cdfFillEvidence{
		Role: contract.Role, ClientID: 7, Symbol: cdfActivationSymbol, OrderID: 11, TradeID: 12,
		Timestamp: 10, Side: "BUY", Price: contract.ReferencePrice, Qty: contract.MinimumQualifyingQty,
		FeeAmount: cdfFixtureFee(contract.ReferencePrice, contract.MinimumQualifyingQty, contract.BasePrecision, contract.MakerFeeBps),
		FeeAsset:  contract.QuoteAsset, IsFull: true, PositionBefore: 0, PositionAfter: contract.MinimumQualifyingQty,
	}
	fillRaw, err := json.Marshal(fill)
	if err != nil {
		t.Fatal(err)
	}
	states := map[cdfParticipantKey]*cdfSupplierState{{"north", 7}: state}
	audit.processCDFFill(Event{SimTS: 10, VenueID: "north", ClientID: 7, payload: fillRaw}, states, map[cdfFillKey]cdfFillEvidence{})
	postFillBalances := cdfBalanceSnapshotEvidence{
		Timestamp: 11, ClientID: 7,
		SpotBalances: []cdfBalanceEvidence{
			{Asset: contract.BaseAsset, Free: contract.InitialBaseBalance + fill.Qty, NetAsset: contract.InitialBaseBalance + fill.Qty},
			{Asset: contract.QuoteAsset, Free: contract.InitialQuoteBalance - cdfFixtureNotional(fill.Price, fill.Qty, contract.BasePrecision) - fill.FeeAmount, NetAsset: contract.InitialQuoteBalance - cdfFixtureNotional(fill.Price, fill.Qty, contract.BasePrecision) - fill.FeeAmount},
		},
		PerpBalances: []cdfBalanceEvidence{}, Borrowed: map[string]int64{},
	}
	postFillRaw, err := json.Marshal(postFillBalances)
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFBalanceSnapshot(Event{SimTS: 11, VenueID: "north", ClientID: 7, payload: postFillRaw}, states)
	if !hasCDFActivationFailure(audit.Checks, "supplier balance snapshot does not reconcile to prior exchange-matched fills") {
		t.Fatalf("actor-only fill advanced authoritative balance state: %+v", audit.Checks)
	}
}

func TestCDFStrictReconciliationRetainsTerminalGTCOrders(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{audit: CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7}}
	audit := &CDFActivationAudit{strictMechanics: true}
	orders := map[cdfOrderKey]*cdfOrderState{{"north", 7, 11}: {
		side: "BUY", price: contract.ReferencePrice, originalQty: 5, remainingQty: 3,
	}}
	audit.reconcileCDFFills(
		map[cdfParticipantKey]*cdfSupplierState{{"north", 7}: state},
		map[cdfFillKey]cdfFillEvidence{}, map[cdfFillKey]cdfOrderFillEvidence{}, orders,
	)
	if len(audit.Checks) != 0 || state.audit.OpenOrderCount != 1 || state.audit.OpenOrderQty != 3 {
		t.Fatalf("terminal live GTC was rejected or not recorded: checks=%+v audit=%+v", audit.Checks, state.audit)
	}
}

func TestCDFPostFillResponseAcceptsLaterObservation(t *testing.T) {
	state := &cdfSupplierState{
		fillResponses: []cdfFillResponseWindow{{
			fillAt: 10, fillGlobalSeq: 3, positionAfter: 5,
			preFillDecision: cdfDecisionEvidence{ObservationSequence: 1, ObservationDeliveredAt: 5, ReferencePrice: 100, MarkPrice: 100, TargetPosition: 2, Position: 0, QuotePrice: 99, QuoteQty: 2},
			preFillKnown:    true,
		}},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	audit.recordCDFPostFillResponse(Event{SimTS: 20, GlobalSequence: 5}, state, cdfDecisionEvidence{
		ObservationSequence: 2, ObservationDeliveredAt: 20, ReferencePrice: 100, MarkPrice: 100, Position: 5, TargetPosition: 2,
		Action: "submit", Side: "SELL", QuotePrice: 102, QuoteQty: 5,
	})
	if state.audit.PostFillResponsiveCount != 1 || !state.fillResponses[0].responded {
		t.Fatalf("later observation was not accepted as a post-fill response: %+v", state)
	}
}

func TestCDFPostFillResponseRequiresFreshObservationOnlyForClosedOneSidedQuote(t *testing.T) {
	state := &cdfSupplierState{
		fillResponses: []cdfFillResponseWindow{{
			fillAt: 10, fillGlobalSeq: 3, positionAfter: 5, requiresFreshObservation: true,
			preFillDecision: cdfDecisionEvidence{
				ObservationSequence: 1, ObservationDeliveredAt: 5, BestBid: 99, BestBidQty: 10,
				BestAsk: 0, BestAskQty: 0, MarkPrice: 99, RiskMarkPrice: 99, RiskMarkCurrent: true,
				LocalBookMode: "one_sided", QuotePriceSource: "one_sided_missing_side_blended",
				ReferencePrice: 100, TargetPosition: 2, Position: 0, Side: "BUY", QuotePrice: 98, QuoteQty: 2,
			},
			preFillKnown: true,
		}},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	audit.recordCDFPostFillResponse(Event{SimTS: 20, GlobalSequence: 5}, state, cdfDecisionEvidence{
		ObservationSequence: 1, ObservationDeliveredAt: 5, BestBid: 99, BestBidQty: 10,
		BestAsk: 0, BestAskQty: 0, MarkPrice: 99, RiskMarkPrice: 99, RiskMarkCurrent: true,
		LocalBookMode: "one_sided", QuotePriceSource: "one_sided_missing_side_blended",
		ReferencePrice: 100, TargetPosition: 2, Position: 5, Action: "submit", Side: "SELL", QuotePrice: 101, QuoteQty: 5,
	})
	if state.audit.PostFillResponsiveCount != 0 || state.fillResponses[0].responded {
		t.Fatalf("same pre-fill observation was credited as a post-fill response: %+v", state)
	}

	twoSided := &cdfSupplierState{fillResponses: []cdfFillResponseWindow{{
		fillAt: 10, fillGlobalSeq: 3, positionAfter: 5,
		preFillDecision: cdfDecisionEvidence{
			ObservationSequence: 1, ObservationDeliveredAt: 5, BestBid: 99, BestBidQty: 10,
			BestAsk: 101, BestAskQty: 10, MarkPrice: 100, RiskMarkPrice: 100, RiskMarkCurrent: true,
			LocalBookMode: "two_sided", ReferencePrice: 100, TargetPosition: 2, Position: 0, Side: "BUY", QuotePrice: 99, QuoteQty: 2,
		},
		preFillKnown: true,
	}}}
	audit = &CDFActivationAudit{strictMechanics: true}
	audit.recordCDFPostFillResponse(Event{SimTS: 20, GlobalSequence: 5}, twoSided, cdfDecisionEvidence{
		ObservationSequence: 1, ObservationDeliveredAt: 5, BestBid: 99, BestBidQty: 10,
		BestAsk: 101, BestAskQty: 10, MarkPrice: 100, RiskMarkPrice: 100, RiskMarkCurrent: true,
		LocalBookMode: "two_sided", ReferencePrice: 100, TargetPosition: 2, Position: 5,
		Action: "submit", Side: "SELL", QuotePrice: 102, QuoteQty: 5,
	})
	if twoSided.audit.PostFillResponsiveCount != 1 || !twoSided.fillResponses[0].responded {
		t.Fatalf("two-sided response was incorrectly forced to wait for a new observation: %+v", twoSided)
	}
}

func TestCDFPostFillResponseRejectsMarketOnlyTargetMovement(t *testing.T) {
	state := &cdfSupplierState{
		fillResponses: []cdfFillResponseWindow{{
			fillAt: 10, fillGlobalSeq: 3, positionAfter: 5,
			preFillDecision: cdfDecisionEvidence{
				ReferencePrice: 100, MarkPrice: 100, Position: 0, TargetPosition: 2,
				LocalBookMode: "two_sided", RiskMarkSource: "two_sided_midpoint",
			},
			preFillKnown: true,
		}},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	audit.recordCDFPostFillResponse(Event{SimTS: 20, GlobalSequence: 5}, state, cdfDecisionEvidence{
		ObservationSequence: 2, ObservationDeliveredAt: 20,
		ReferencePrice: 100, MarkPrice: 100, Position: 5, TargetPosition: 5,
		LocalBookMode: "two_sided", RiskMarkSource: "two_sided_midpoint",
		Action: "rest", QuotePrice: 100, QuoteQty: 2,
	})
	if state.audit.PostFillResponsiveCount != 0 || state.fillResponses[0].responded {
		t.Fatalf("market-only target movement was credited as an inventory response: %+v", state)
	}
}

func TestCDFPostFillResponseRejectsTargetOnlyQuoteReplay(t *testing.T) {
	state := &cdfSupplierState{
		fillResponses: []cdfFillResponseWindow{{
			fillAt: 10, fillGlobalSeq: 3, positionAfter: 5,
			preFillDecision: cdfDecisionEvidence{
				BestBid: 99, BestBidQty: 10, BestAsk: 101, BestAskQty: 10,
				MarkPrice: 100, RiskMarkPrice: 100, RiskMarkCurrent: true,
				LocalBookMode: "two_sided", TargetPosition: 2, Position: 0,
				Action: "submit", Side: "BUY", QuotePrice: 99, QuoteQty: 2, QuoteOrderID: 11,
			},
			preFillKnown: true,
		}},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	audit.recordCDFPostFillResponse(Event{SimTS: 20, GlobalSequence: 5}, state, cdfDecisionEvidence{
		ObservationSequence: 2, ObservationDeliveredAt: 20,
		BestBid: 99, BestBidQty: 10, BestAsk: 101, BestAskQty: 10,
		MarkPrice: 100, RiskMarkPrice: 100, RiskMarkCurrent: true,
		LocalBookMode: "two_sided", Position: 5, TargetPosition: 5,
		Action: "submit", Side: "BUY", QuotePrice: 99, QuoteQty: 2, QuoteOrderID: 11, QuoteRequestID: 12,
	})
	if state.audit.PostFillResponsiveCount != 0 || state.fillResponses[0].responded {
		t.Fatalf("target-only quote replay was credited as an inventory response: %+v", state)
	}
}

func TestCDFPostFillResponseRejectsOrderIdentityOnlyChurn(t *testing.T) {
	state := &cdfSupplierState{
		fillResponses: []cdfFillResponseWindow{{
			fillAt: 10, fillGlobalSeq: 3, positionAfter: 5,
			preFillDecision: cdfDecisionEvidence{
				ObservationSequence: 1, ObservationDeliveredAt: 5,
				BestBid: 99, BestBidQty: 10, BestAsk: 101, BestAskQty: 10,
				MarkPrice: 100, RiskMarkPrice: 100, RiskMarkCurrent: true,
				LocalBookMode: "two_sided", ReferencePrice: 100, TargetPosition: 2, Position: 0,
				Action: "submit", Side: "BUY", QuotePrice: 99, QuoteQty: 2, QuoteOrderID: 11,
			},
			preFillKnown: true,
		}},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	audit.recordCDFPostFillResponse(Event{SimTS: 20, GlobalSequence: 5}, state, cdfDecisionEvidence{
		ObservationSequence: 2, ObservationDeliveredAt: 20,
		BestBid: 99, BestBidQty: 10, BestAsk: 101, BestAskQty: 10,
		MarkPrice: 100, RiskMarkPrice: 100, RiskMarkCurrent: true,
		LocalBookMode: "two_sided", ReferencePrice: 100, TargetPosition: 2, Position: 5,
		Action: "submit", Side: "BUY", QuotePrice: 99, QuoteQty: 2, QuoteOrderID: 99, QuoteRequestID: 100,
	})
	if state.audit.PostFillResponsiveCount != 0 || state.fillResponses[0].responded {
		t.Fatalf("order identity churn was credited as an inventory response: %+v", state)
	}
}

func TestCDFPostFillResponseRejectsReferenceDrivenQuoteChange(t *testing.T) {
	state := &cdfSupplierState{
		fillResponses: []cdfFillResponseWindow{{
			fillAt: 10, fillGlobalSeq: 3, positionAfter: 5,
			preFillDecision: cdfDecisionEvidence{
				ObservationSequence: 1, ObservationDeliveredAt: 5,
				BestBid: 99, BestBidQty: 10, BestAsk: 101, BestAskQty: 10,
				MarkPrice: 100, RiskMarkPrice: 100, RiskMarkCurrent: true,
				LocalBookMode: "two_sided", ReferencePrice: 100, TargetPosition: 2, Position: 0,
				Action: "submit", Side: "BUY", QuotePrice: 99, QuoteQty: 2,
			},
			preFillKnown: true,
		}},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	audit.recordCDFPostFillResponse(Event{SimTS: 20, GlobalSequence: 5}, state, cdfDecisionEvidence{
		ObservationSequence: 2, ObservationDeliveredAt: 20,
		BestBid: 99, BestBidQty: 10, BestAsk: 101, BestAskQty: 10,
		MarkPrice: 100, RiskMarkPrice: 100, RiskMarkCurrent: true,
		LocalBookMode: "two_sided", ReferencePrice: 101, TargetPosition: 3, Position: 5,
		Action: "submit", Side: "SELL", QuotePrice: 102, QuoteQty: 5,
	})
	if state.audit.PostFillResponsiveCount != 0 || state.fillResponses[0].responded {
		t.Fatalf("reference-driven quote change was credited as an inventory response: %+v", state)
	}
}

func TestCDFLimitOrTouchUnavailableRequiresObservableFailure(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{contract: contract}
	baseDecision := cdfDecisionEvidence{
		Action: "wait", Reason: "limit_or_touch_unavailable", QuotePrice: 300_000_000, QuoteQty: contract.MinimumQualifyingQty,
		LocalBookMode: "one_sided",
	}
	if !cdfDecisionReasonPredicate(baseDecision, state) {
		t.Fatal("stale positive quote terms on a one-sided local book were not recognized as an observable unavailable-touch failure")
	}
	validTwoSided := baseDecision
	validTwoSided.LocalBookMode = "two_sided"
	validTwoSided.BestBid = contract.ReferencePrice - contract.TickSize
	validTwoSided.BestAsk = contract.ReferencePrice + contract.TickSize
	if cdfDecisionReasonPredicate(validTwoSided, state) {
		t.Fatal("valid two-sided touch was incorrectly accepted as unavailable")
	}
}

func TestCDFTradeIdentityCannotBeCountedTwice(t *testing.T) {
	r := &CDFActivationAudit{}
	payload, err := json.Marshal(cdfTradeEvidence{TradeID: 9, Price: 100, Qty: 3, Side: "SELL"})
	if err != nil {
		t.Fatal(err)
	}
	event := Event{VenueID: "north", payload: payload}
	r.processCDFTrade(event)
	r.processCDFTrade(event)
	if r.TotalVolumeQty != 3 || !hasCDFActivationFailure(r.Checks, "duplicate CDF trade identity") {
		t.Fatalf("duplicate trade was not rejected: total=%d checks=%+v", r.TotalVolumeQty, r.Checks)
	}
}

func TestCDFStrictTradeAttributionUsesMakerOrderIdentity(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	quantity := contract.MinimumQualifyingQty
	price := contract.ReferencePrice - contract.TickSize
	fee, feeOK := cdfActivationFee(mustCDFTestNotional(t, price, quantity, contract.BasePrecision), contract.MakerFeeBps)
	if !feeOK {
		t.Fatal("fee calculation failed")
	}
	key := cdfFillKey{venueID: "north", clientID: 7, orderID: 701, tradeID: 801}
	state := &cdfSupplierState{
		audit:    CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: key.clientID},
		contract: contract,
	}
	fill := cdfOrderFillEvidence{
		OrderID: key.orderID, TradeID: key.tradeID, Side: "BUY", Price: price, Qty: quantity,
		FeeAmount: fee, FeeAsset: contract.QuoteAsset, FilledQty: quantity, IsFull: true,
	}
	observed := cdfFillEvidence{
		Role: contract.Role, ClientID: key.clientID, Symbol: cdfActivationSymbol,
		OrderID: key.orderID, TradeID: key.tradeID, Timestamp: 10, Side: "BUY", Price: price,
		Qty: quantity, FeeAmount: fee, FeeAsset: contract.QuoteAsset, IsFull: true,
	}
	r := &CDFActivationAudit{
		strictMechanics:    true,
		trades:             map[cdfTradeKey]cdfTradeEvidence{{venueID: key.venueID, tradeID: key.tradeID}: {TradeID: key.tradeID, Price: price, Qty: quantity, Side: "SELL", MakerOrderID: key.orderID, TakerOrderID: 9001}},
		totalVolumeByVenue: map[string]int64{key.venueID: quantity * 2},
	}
	r.reconcileCDFFills(
		map[cdfParticipantKey]*cdfSupplierState{{venueID: key.venueID, clientID: key.clientID}: state},
		map[cdfFillKey]cdfFillEvidence{key: observed},
		map[cdfFillKey]cdfOrderFillEvidence{key: fill},
		map[cdfOrderKey]*cdfOrderState{},
	)
	if len(r.Checks) != 0 {
		t.Fatalf("valid maker attribution produced checks: %+v", r.Checks)
	}
	notional := mustCDFTestNotional(t, price, quantity, contract.BasePrecision)
	if state.audit.TradeCount != 1 || state.audit.VolumeQty != quantity || state.audit.VolumeNotionalQuote != notional || state.audit.FeesPaidQuote != fee {
		t.Fatalf("supplier attribution = %+v, want one verified trade", state.audit)
	}
	if r.SupplierVolumeQty != quantity || r.SupplierVolumeNotional != notional || r.SupplierFeesPaid != fee {
		t.Fatalf("aggregate attribution = volume %d notional %d fees %d", r.SupplierVolumeQty, r.SupplierVolumeNotional, r.SupplierFeesPaid)
	}
}

func TestCDFStrictTradeAttributionRejectsWrongMakerOrder(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	quantity := contract.MinimumQualifyingQty
	price := contract.ReferencePrice - contract.TickSize
	fee := cdfFixtureFee(price, quantity, contract.BasePrecision, contract.MakerFeeBps)
	key := cdfFillKey{venueID: "north", clientID: 7, orderID: 701, tradeID: 801}
	state := &cdfSupplierState{
		audit:    CDFSupplierActivationAudit{VenueID: key.venueID, Role: contract.Role, ClientID: key.clientID},
		contract: contract,
	}
	fill := cdfOrderFillEvidence{
		OrderID: key.orderID, TradeID: key.tradeID, Side: "BUY", Price: price, Qty: quantity,
		FeeAmount: fee, FeeAsset: contract.QuoteAsset, FilledQty: quantity, IsFull: true,
	}
	observed := cdfFillEvidence{
		Role: contract.Role, ClientID: key.clientID, Symbol: cdfActivationSymbol,
		OrderID: key.orderID, TradeID: key.tradeID, Timestamp: 10, Side: "BUY", Price: price,
		Qty: quantity, FeeAmount: fee, FeeAsset: contract.QuoteAsset, IsFull: true,
	}
	r := &CDFActivationAudit{
		strictMechanics:    true,
		trades:             map[cdfTradeKey]cdfTradeEvidence{{venueID: key.venueID, tradeID: key.tradeID}: {TradeID: key.tradeID, Price: price, Qty: quantity, Side: "SELL", MakerOrderID: key.orderID + 1, TakerOrderID: 9001}},
		totalVolumeByVenue: map[string]int64{key.venueID: quantity},
	}
	r.reconcileCDFFills(
		map[cdfParticipantKey]*cdfSupplierState{{venueID: key.venueID, clientID: key.clientID}: state},
		map[cdfFillKey]cdfFillEvidence{key: observed},
		map[cdfFillKey]cdfOrderFillEvidence{key: fill},
		map[cdfOrderKey]*cdfOrderState{},
	)
	if !hasCDFActivationFailure(r.Checks, "exchange OrderFill does not match a unique opposite-side CDF trade and maker order") || r.SupplierVolumeQty != 0 || state.audit.TradeCount != 0 {
		t.Fatalf("wrong maker identity was attributed: checks=%+v audit=%+v aggregate=%d", r.Checks, state.audit, r.SupplierVolumeQty)
	}
}

func TestCDFStrictTradeRequiresOrderIdentities(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"trade_id": 9, "price": 100, "qty": 3, "side": "SELL"})
	if err != nil {
		t.Fatal(err)
	}
	r := &CDFActivationAudit{strictMechanics: true}
	r.processCDFTrade(Event{VenueID: "north", payload: payload})
	if !hasCDFActivationFailure(r.Checks, `malformed CDF trade evidence: missing required payload field "maker_order_id"`) {
		t.Fatalf("strict trade without maker/taker identities was accepted: %+v", r.Checks)
	}
}

func TestCDFStrictReconciliationRejectsInvertedProducerFillOrder(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{audit: CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7}}
	key := cdfFillKey{venueID: "north", clientID: 7, orderID: 11, tradeID: 12}
	tradeKey := cdfTradeKey{venueID: "north", tradeID: 12}
	fee := int64(3)
	observed := cdfFillEvidence{
		Role: contract.Role, ClientID: 7, Symbol: cdfActivationSymbol, OrderID: 11, TradeID: 12,
		Timestamp: 20, Side: "BUY", Price: 100, Qty: 5, FeeAmount: fee, FeeAsset: contract.QuoteAsset,
		IsFull: true,
	}
	actual := cdfOrderFillEvidence{
		OrderID: 11, TradeID: 12, Side: "BUY", Price: 100, Qty: 5, FeeAmount: fee, FeeAsset: contract.QuoteAsset,
		FilledQty: 5, RemainingQty: 0, IsFull: true,
	}
	audit := &CDFActivationAudit{
		strictMechanics: true,
		trades: map[cdfTradeKey]cdfTradeEvidence{tradeKey: {
			TradeID: 12, Price: 100, Qty: 5, Side: "SELL", MakerOrderID: 11, TakerOrderID: 99,
		}},
		tradeGlobal:        map[cdfTradeKey]uint64{tradeKey: 5},
		actualFillGlobal:   map[cdfFillKey]uint64{key: 4},
		observedFillGlobal: map[cdfFillKey]uint64{key: 6},
	}
	audit.reconcileCDFFills(
		map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state},
		map[cdfFillKey]cdfFillEvidence{key: observed},
		map[cdfFillKey]cdfOrderFillEvidence{key: actual},
		map[cdfOrderKey]*cdfOrderState{},
	)
	if !hasCDFActivationFailure(audit.Checks, "CDF fill producer ordering is not Trade < exchange OrderFill < supplier fill") || audit.SupplierVolumeQty != 0 {
		t.Fatalf("inverted producer order was attributed: checks=%+v volume=%d", audit.Checks, audit.SupplierVolumeQty)
	}
}

func TestCDFOrderLifecycleRecordsFilledQuoteLifetime(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	quantity := contract.MinimumQualifyingQty
	price := contract.ReferencePrice
	fee := cdfFixtureFee(price, quantity, contract.BasePrecision, contract.MakerFeeBps)
	state := &cdfSupplierState{
		contract: contract,
		audit:    CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7},
	}
	orderKey := cdfOrderKey{venueID: "north", clientID: 7, orderID: 11}
	orders := map[cdfOrderKey]*cdfOrderState{orderKey: {
		side: "BUY", price: price, originalQty: quantity, remainingQty: quantity,
		acceptedAt: 10, acceptedGlobalSeq: 1,
	}}
	fill := cdfOrderFillEvidence{
		OrderID: 11, TradeID: 12, Side: "BUY", Price: price, Qty: quantity,
		FeeAmount: fee, FeeAsset: contract.QuoteAsset, FilledQty: quantity, IsFull: true,
	}
	raw, err := json.Marshal(fill)
	if err != nil {
		t.Fatal(err)
	}
	audit := &CDFActivationAudit{strictMechanics: true, terminalOrders: make(map[cdfOrderKey]*cdfOrderState)}
	audit.processCDFOrderFill(Event{SimTS: 20, GlobalSequence: 2, VenueID: "north", ClientID: 7, payload: raw},
		map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}, orders,
		map[cdfFillKey]cdfOrderFillEvidence{})
	if len(audit.Checks) != 0 || len(orders) != 0 {
		t.Fatalf("filled lifecycle rejected or remained live: checks=%+v orders=%+v", audit.Checks, orders)
	}
	if state.audit.FilledOrderCount != 1 || state.audit.TotalQuoteLifetimeNano != 10 || state.audit.MaxQuoteLifetimeNano != 10 {
		t.Fatalf("filled lifecycle metrics = %+v", state.audit)
	}
	if audit.terminalOrders[orderKey].terminalState != "filled" {
		t.Fatalf("terminal order state = %+v", audit.terminalOrders[orderKey])
	}
}

func TestCDFCancelRejectedFillRaceIsReconciled(t *testing.T) {
	state := &cdfSupplierState{audit: CDFSupplierActivationAudit{VenueID: "north", Role: "cdf_supplier_1", ClientID: 7}}
	orderKey := cdfOrderKey{venueID: "north", clientID: 7, orderID: 11}
	audit := &CDFActivationAudit{
		strictMechanics: true,
		terminalOrders:  map[cdfOrderKey]*cdfOrderState{orderKey: {terminalState: "filled"}},
	}
	withdrawals := map[cdfRequestKey]*cdfWithdrawal{{venueID: "north", clientID: 7, requestID: 13}: {
		event:    Event{SimTS: 10, GlobalSequence: 1},
		decision: cdfDecisionEvidence{QuoteOrderID: 11, Action: "cancel", Reason: "reprice_for_inventory_or_touch"},
	}}
	payload, err := json.Marshal(cdfCancelRejectedEvidence{
		OrderID: 11, RequestID: 13, Success: false, Error: etypes.RejectOrderNotFound,
	})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFCancelRejected(Event{SimTS: 20, GlobalSequence: 2, VenueID: "north", ClientID: 7, payload: payload},
		map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}, withdrawals,
		map[cdfOrderKey]*cdfOrderState{})
	if len(audit.Checks) != 0 || !withdrawals[cdfRequestKey{venueID: "north", clientID: 7, requestID: 13}].cancelRejected {
		t.Fatalf("fill/cancel race was not reconciled: checks=%+v withdrawals=%+v", audit.Checks, withdrawals)
	}
	if state.audit.CancelRejectedCount != 1 || state.audit.WithdrawalCount != 0 {
		t.Fatalf("race counters = %+v", state.audit)
	}
}

func TestCDFCancelRejectedForcedCancelRaceIsReconciled(t *testing.T) {
	state := &cdfSupplierState{audit: CDFSupplierActivationAudit{VenueID: "north", Role: "cdf_supplier_1", ClientID: 7}}
	orderKey := cdfOrderKey{venueID: "north", clientID: 7, orderID: 11}
	orders := map[cdfOrderKey]*cdfOrderState{orderKey: {
		side: "BUY", price: 100, originalQty: 5, remainingQty: 5, acceptedAt: 10, acceptedGlobalSeq: 1,
	}}
	audit := &CDFActivationAudit{
		strictMechanics:     true,
		terminalOrders:      make(map[cdfOrderKey]*cdfOrderState),
		liveOrderBySupplier: map[cdfParticipantKey]cdfOrderKey{{venueID: "north", clientID: 7}: orderKey},
	}
	states := map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}
	cancelledRaw, err := json.Marshal(cdfCancelledEvidence{
		OrderID: 11, RemainingQty: 5, Reason: "EXCHANGE_FORCED_LIFECYCLE",
	})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFCancelled(Event{SimTS: 20, GlobalSequence: 2, VenueID: "north", ClientID: 7, payload: cancelledRaw},
		states, map[cdfRequestKey]*cdfWithdrawal{}, orders, nil, nil)
	if len(audit.Checks) != 0 || len(orders) != 0 || audit.terminalOrders[orderKey].terminalState != "forced_cancelled" {
		t.Fatalf("forced cancellation lifecycle = checks=%+v orders=%+v terminal=%+v", audit.Checks, orders, audit.terminalOrders[orderKey])
	}
	withdrawalKey := cdfRequestKey{venueID: "north", clientID: 7, requestID: 13}
	withdrawals := map[cdfRequestKey]*cdfWithdrawal{withdrawalKey: {
		event:    Event{SimTS: 10, GlobalSequence: 1},
		decision: cdfDecisionEvidence{QuoteOrderID: 11, Action: "withdraw", Reason: "stale_or_missing_observation"},
	}}
	rejectedRaw, err := json.Marshal(cdfCancelRejectedEvidence{
		OrderID: 11, RequestID: 13, Success: false, Error: etypes.RejectOrderNotFound,
	})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFCancelRejected(Event{SimTS: 21, GlobalSequence: 3, VenueID: "north", ClientID: 7, payload: rejectedRaw},
		states, withdrawals, orders)
	if len(audit.Checks) != 0 || !withdrawals[withdrawalKey].cancelRejected || state.audit.CancelRejectedCount != 1 {
		t.Fatalf("forced cancellation/cancel race = checks=%+v withdrawals=%+v state=%+v", audit.Checks, withdrawals, state)
	}
}

func TestCDFOpenOrderIsRightCensoredAtTerminalHorizon(t *testing.T) {
	state := &cdfSupplierState{audit: CDFSupplierActivationAudit{VenueID: "north", Role: "cdf_supplier_1", ClientID: 7}}
	orderKey := cdfOrderKey{venueID: "north", clientID: 7, orderID: 11}
	orders := map[cdfOrderKey]*cdfOrderState{orderKey: {
		originalQty: 5, remainingQty: 3, acceptedAt: 10,
	}}
	audit := &CDFActivationAudit{strictMechanics: true, terminalOrders: make(map[cdfOrderKey]*cdfOrderState)}
	audit.reconcileCDFFills(
		map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state},
		map[cdfFillKey]cdfFillEvidence{}, map[cdfFillKey]cdfOrderFillEvidence{}, orders, 30,
	)
	if len(audit.Checks) != 0 || state.audit.CensoredOrderCount != 1 || state.audit.CensoredQuoteLifetimeNano != 20 || state.audit.OpenOrderCount != 1 {
		t.Fatalf("right-censored lifecycle = checks=%+v audit=%+v", audit.Checks, state.audit)
	}
}

func TestCDFOrderLifecycleRecordsPartialThenFullFill(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{
		contract: contract,
		audit:    CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7},
	}
	orderKey := cdfOrderKey{venueID: "north", clientID: 7, orderID: 11}
	orders := map[cdfOrderKey]*cdfOrderState{orderKey: {
		side: "BUY", price: contract.ReferencePrice, originalQty: 5, remainingQty: 5,
		acceptedAt: 10, acceptedGlobalSeq: 1,
	}}
	audit := &CDFActivationAudit{
		strictMechanics:     true,
		terminalOrders:      make(map[cdfOrderKey]*cdfOrderState),
		liveOrderBySupplier: map[cdfParticipantKey]cdfOrderKey{{venueID: "north", clientID: 7}: orderKey},
	}
	states := map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}
	actual := make(map[cdfFillKey]cdfOrderFillEvidence)
	for _, fill := range []struct {
		at, sequence, tradeID, qty, filledQty, remainingQty int64
		isFull                                              bool
	}{{20, 2, 12, 2, 2, 3, false}, {30, 3, 13, 3, 5, 0, true}} {
		fee := cdfFixtureFee(contract.ReferencePrice, fill.qty, contract.BasePrecision, contract.MakerFeeBps)
		payload, err := json.Marshal(cdfOrderFillEvidence{
			OrderID: 11, TradeID: uint64(fill.tradeID), Side: "BUY", Price: contract.ReferencePrice,
			Qty: fill.qty, FeeAmount: fee, FeeAsset: contract.QuoteAsset,
			FilledQty: fill.filledQty, RemainingQty: fill.remainingQty, IsFull: fill.isFull,
		})
		if err != nil {
			t.Fatal(err)
		}
		audit.processCDFOrderFill(Event{SimTS: fill.at, GlobalSequence: uint64(fill.sequence), VenueID: "north", ClientID: 7, payload: payload}, states, orders, actual)
	}
	if len(audit.Checks) != 0 || len(orders) != 0 || state.audit.FilledOrderCount != 1 || state.audit.TotalQuoteLifetimeNano != 20 {
		t.Fatalf("partial/full lifecycle = checks=%+v orders=%+v state=%+v", audit.Checks, orders, state)
	}
	if audit.terminalOrders[orderKey].fillCount != 2 || audit.terminalOrders[orderKey].filledQty != 5 {
		t.Fatalf("partial/full terminal order = %+v", audit.terminalOrders[orderKey])
	}
}

func TestCDFStrictOrderLifecycleRejectsInvertedSameTimestampSequence(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	quantity := int64(5)
	price := contract.ReferencePrice
	state := &cdfSupplierState{
		contract: contract,
		audit:    CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7},
	}
	orders := map[cdfOrderKey]*cdfOrderState{{venueID: "north", clientID: 7, orderID: 11}: {
		side: "BUY", price: price, originalQty: quantity, remainingQty: quantity,
		acceptedAt: 20, acceptedGlobalSeq: 10,
	}}
	fee := cdfFixtureFee(price, quantity, contract.BasePrecision, contract.MakerFeeBps)
	payload, err := json.Marshal(cdfOrderFillEvidence{
		OrderID: 11, TradeID: 12, Side: "BUY", Price: price, Qty: quantity,
		FeeAmount: fee, FeeAsset: contract.QuoteAsset, FilledQty: quantity, IsFull: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	audit := &CDFActivationAudit{strictMechanics: true, terminalOrders: make(map[cdfOrderKey]*cdfOrderState)}
	audit.processCDFOrderFill(Event{SimTS: 20, GlobalSequence: 9, VenueID: "north", ClientID: 7, payload: payload},
		map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}, orders,
		map[cdfFillKey]cdfOrderFillEvidence{})
	if !hasCDFActivationFailure(audit.Checks, "exchange OrderFill does not match a live accepted CDF order") || len(orders) != 1 {
		t.Fatalf("inverted same-timestamp fill was accepted: checks=%+v orders=%+v", audit.Checks, orders)
	}
}

func TestCDFRepriceLifecycleSeparatesCancelFromReplacement(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{
		contract: contract,
		audit:    CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7},
	}
	orderKey := cdfOrderKey{venueID: "north", clientID: 7, orderID: 11}
	orders := map[cdfOrderKey]*cdfOrderState{orderKey: {
		side: "BUY", price: contract.ReferencePrice, originalQty: 5, remainingQty: 5,
		acceptedAt: 10, acceptedGlobalSeq: 1,
	}}
	withdrawalKey := cdfRequestKey{venueID: "north", clientID: 7, requestID: 13}
	withdrawals := map[cdfRequestKey]*cdfWithdrawal{withdrawalKey: {
		event:    Event{SimTS: 11, GlobalSequence: 2},
		decision: cdfDecisionEvidence{QuoteOrderID: 11, Action: "cancel", Reason: "reprice_for_inventory_or_touch"},
	}}
	audit := &CDFActivationAudit{strictMechanics: true, terminalOrders: make(map[cdfOrderKey]*cdfOrderState)}
	states := map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}
	cancelPayload, err := json.Marshal(cdfCancelledEvidence{OrderID: 11, RequestID: 13, RemainingQty: 5})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFCancelled(Event{SimTS: 20, GlobalSequence: 3, VenueID: "north", ClientID: 7, payload: cancelPayload},
		states, withdrawals, orders, nil, nil)
	if len(audit.Checks) != 0 || state.audit.RepriceCancelCount != 1 || !state.pendingReprice {
		t.Fatalf("reprice cancellation = checks=%+v state=%+v", audit.Checks, state)
	}
	replacementDecision := cdfDecisionEvidence{
		ClientID: 7, Role: contract.Role, Symbol: cdfActivationSymbol, Action: "submit", Reason: "inventory_target_gap",
		Side: "SELL", QuotePrice: contract.ReferencePrice + contract.TickSize, QuoteQty: 5,
		QuoteRequestID: 17, ReplacesOrderID: 11, MinimumQualifyingQty: contract.MinimumQualifyingQty,
	}
	submissionKey := cdfRequestKey{venueID: "north", clientID: 7, requestID: 17}
	submissions := map[cdfRequestKey]*cdfSubmission{submissionKey: {
		event: Event{SimTS: 21, GlobalSequence: 4}, decision: replacementDecision,
	}}
	acceptedPayload, err := json.Marshal(cdfAcceptedEvidence{
		OrderID: 19, ClientID: 7, RequestID: 17, Side: "SELL", Type: "LIMIT", TimeInForce: "GTC",
		PostOnly: true, Price: replacementDecision.QuotePrice, Qty: replacementDecision.QuoteQty,
	})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFAccepted(Event{SimTS: 22, GlobalSequence: 5, VenueID: "north", ClientID: 7, payload: acceptedPayload}, states, submissions, orders)
	if len(audit.Checks) != 0 || state.audit.CompletedRepriceCount != 1 || state.pendingReprice {
		t.Fatalf("replacement lifecycle = checks=%+v state=%+v", audit.Checks, state)
	}
}

func TestCDFStrictRepriceAcceptsSameTermLinkedReplacement(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{
		contract:       contract,
		audit:          CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7},
		pendingReprice: true, pendingRepriceOrderID: 11, pendingRepriceSide: "BUY",
		pendingRepricePrice: contract.ReferencePrice, pendingRepriceQty: 5,
	}
	audit := &CDFActivationAudit{strictMechanics: true, terminalOrders: make(map[cdfOrderKey]*cdfOrderState)}
	decision := cdfDecisionEvidence{
		ClientID: 7, Role: contract.Role, Symbol: cdfActivationSymbol, Action: "submit", Reason: "inventory_target_gap",
		Side: "BUY", QuotePrice: contract.ReferencePrice, QuoteQty: 5,
		QuoteRequestID: 17, ReplacesOrderID: 11, MinimumQualifyingQty: contract.MinimumQualifyingQty,
	}
	submissionKey := cdfRequestKey{venueID: "north", clientID: 7, requestID: 17}
	submissions := map[cdfRequestKey]*cdfSubmission{submissionKey: {
		event: Event{SimTS: 21, GlobalSequence: 4}, decision: decision,
	}}
	payload, err := json.Marshal(cdfAcceptedEvidence{
		OrderID: 19, ClientID: 7, RequestID: 17, Side: "BUY", Type: "LIMIT", TimeInForce: "GTC",
		PostOnly: true, Price: contract.ReferencePrice, Qty: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	orders := make(map[cdfOrderKey]*cdfOrderState)
	audit.processCDFAccepted(Event{SimTS: 22, GlobalSequence: 5, VenueID: "north", ClientID: 7, payload: payload},
		map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}, submissions, orders)
	if len(audit.Checks) != 0 || state.audit.CompletedRepriceCount != 0 || state.pendingReprice || len(orders) != 1 {
		t.Fatalf("same-term linked replacement was mishandled: checks=%+v state=%+v orders=%+v", audit.Checks, state, orders)
	}
}

func TestCDFStrictRepriceRejectsUnlinkedReplacement(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{
		contract:       contract,
		audit:          CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7},
		pendingReprice: true, pendingRepriceOrderID: 11, pendingRepriceSide: "BUY",
		pendingRepricePrice: contract.ReferencePrice, pendingRepriceQty: 5,
	}
	audit := &CDFActivationAudit{
		strictMechanics:     true,
		terminalOrders:      make(map[cdfOrderKey]*cdfOrderState),
		liveOrderBySupplier: make(map[cdfParticipantKey]cdfOrderKey),
	}
	states := map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}
	decision := cdfDecisionEvidence{
		ClientID: 7, Role: contract.Role, Symbol: cdfActivationSymbol, Action: "submit", Reason: "inventory_target_gap",
		Side: "SELL", QuotePrice: contract.ReferencePrice + contract.TickSize, QuoteQty: 5,
		QuoteRequestID: 17, MinimumQualifyingQty: contract.MinimumQualifyingQty,
	}
	submissions := map[cdfRequestKey]*cdfSubmission{{venueID: "north", clientID: 7, requestID: 17}: {
		event: Event{SimTS: 21, GlobalSequence: 4}, decision: decision,
	}}
	payload, err := json.Marshal(cdfAcceptedEvidence{
		OrderID: 19, ClientID: 7, RequestID: 17, Side: "SELL", Type: "LIMIT", TimeInForce: "GTC",
		PostOnly: true, Price: decision.QuotePrice, Qty: decision.QuoteQty,
	})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFAccepted(Event{SimTS: 22, GlobalSequence: 5, VenueID: "north", ClientID: 7, payload: payload}, states, submissions, map[cdfOrderKey]*cdfOrderState{})
	if !hasCDFActivationFailure(audit.Checks, "CDF replacement acceptance lacks the matching replaced-order identity") || state.audit.CompletedRepriceCount != 0 || !state.pendingReprice {
		t.Fatalf("unlinked replacement was accepted as a completed reprice: checks=%+v state=%+v", audit.Checks, state)
	}
}

func TestCDFStrictAuditRejectsMultipleLiveOrders(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{
		contract: contract,
		audit:    CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7},
	}
	states := map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}
	audit := &CDFActivationAudit{
		strictMechanics:     true,
		terminalOrders:      make(map[cdfOrderKey]*cdfOrderState),
		liveOrderBySupplier: make(map[cdfParticipantKey]cdfOrderKey),
	}
	orders := make(map[cdfOrderKey]*cdfOrderState)
	submissions := make(map[cdfRequestKey]*cdfSubmission)
	for _, lifecycle := range []struct{ requestID, orderID uint64 }{{17, 19}, {27, 29}} {
		requestID, orderID := lifecycle.requestID, lifecycle.orderID
		decision := cdfDecisionEvidence{
			ClientID: 7, Role: contract.Role, Symbol: cdfActivationSymbol, Action: "submit", Reason: "inventory_target_gap",
			Side: "BUY", QuotePrice: contract.ReferencePrice, QuoteQty: 5,
			QuoteRequestID: requestID, MinimumQualifyingQty: contract.MinimumQualifyingQty,
		}
		submissions[cdfRequestKey{venueID: "north", clientID: 7, requestID: requestID}] = &cdfSubmission{
			event: Event{SimTS: int64(requestID), GlobalSequence: requestID - 12}, decision: decision,
		}
		payload, err := json.Marshal(cdfAcceptedEvidence{
			OrderID: orderID, ClientID: 7, RequestID: requestID, Side: "BUY", Type: "LIMIT", TimeInForce: "GTC",
			PostOnly: true, Price: contract.ReferencePrice, Qty: 5,
		})
		if err != nil {
			t.Fatal(err)
		}
		audit.processCDFAccepted(Event{SimTS: int64(requestID + 1), GlobalSequence: requestID - 11, VenueID: "north", ClientID: 7, payload: payload}, states, submissions, orders)
	}
	if !hasCDFActivationFailure(audit.Checks, "CDF supplier accepted order 29 while order 19 was still live") || len(orders) != 1 {
		t.Fatalf("multiple live orders were not rejected: checks=%+v orders=%+v", audit.Checks, orders)
	}
}

func TestCDFStrictAuditRejectsTerminalOrderReuse(t *testing.T) {
	contract := RegisteredSV1DActivationContract().Suppliers[0]
	state := &cdfSupplierState{contract: contract, audit: CDFSupplierActivationAudit{VenueID: "north", Role: contract.Role, ClientID: 7}}
	orderKey := cdfOrderKey{venueID: "north", clientID: 7, orderID: 19}
	audit := &CDFActivationAudit{
		strictMechanics: true,
		terminalOrders:  map[cdfOrderKey]*cdfOrderState{orderKey: {terminalState: "filled"}},
	}
	states := map[cdfParticipantKey]*cdfSupplierState{{venueID: "north", clientID: 7}: state}
	decision := cdfDecisionEvidence{
		ClientID: 7, Role: contract.Role, Symbol: cdfActivationSymbol, Action: "submit", Reason: "inventory_target_gap",
		Side: "BUY", QuotePrice: contract.ReferencePrice, QuoteQty: 5, QuoteRequestID: 17,
		MinimumQualifyingQty: contract.MinimumQualifyingQty,
	}
	submissions := map[cdfRequestKey]*cdfSubmission{{venueID: "north", clientID: 7, requestID: 17}: {
		event: Event{SimTS: 21, GlobalSequence: 4}, decision: decision,
	}}
	payload, err := json.Marshal(cdfAcceptedEvidence{
		OrderID: 19, ClientID: 7, RequestID: 17, Side: "BUY", Type: "LIMIT", TimeInForce: "GTC",
		PostOnly: true, Price: contract.ReferencePrice, Qty: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	audit.processCDFAccepted(Event{SimTS: 22, GlobalSequence: 5, VenueID: "north", ClientID: 7, payload: payload}, states, submissions, map[cdfOrderKey]*cdfOrderState{})
	if !hasCDFActivationFailure(audit.Checks, "CDF order identity was reused after a terminal outcome") {
		t.Fatalf("terminal order reuse was accepted: %+v", audit.Checks)
	}
}

func mustCDFTestNotional(t *testing.T, price, quantity, precision int64) int64 {
	t.Helper()
	notional, ok := cdfActivationNotional(price, quantity, precision)
	if !ok {
		t.Fatalf("notional calculation failed for price=%d quantity=%d precision=%d", price, quantity, precision)
	}
	return notional
}

func TestCDFSnapshotProjectionRequiresExactPublicView(t *testing.T) {
	valid := cdfPublicSnapshotEvidence{
		Bids: []etypes.PriceLevel{
			{Price: 101, VisibleQty: 4, HiddenQty: 2},
			{Price: 100, VisibleQty: 3},
		},
		Asks:           []etypes.PriceLevel{{Price: 102, VisibleQty: 5}},
		SourceSequence: 9,
		PublicBids: []etypes.PriceLevel{
			{Price: 101, VisibleQty: 4},
			{Price: 100, VisibleQty: 3},
		},
		PublicAsks: []etypes.PriceLevel{{Price: 102, VisibleQty: 5}},
	}
	if !validCDFSnapshotProjection(valid) {
		t.Fatal("valid complete/public snapshot projection was rejected")
	}

	tests := []struct {
		name   string
		mutate func(*cdfPublicSnapshotEvidence)
	}{
		{
			name: "public hidden quantity",
			mutate: func(snapshot *cdfPublicSnapshotEvidence) {
				snapshot.PublicBids[0].HiddenQty = 1
			},
		},
		{
			name: "public level mismatch",
			mutate: func(snapshot *cdfPublicSnapshotEvidence) {
				snapshot.PublicAsks[0].Price++
			},
		},
		{
			name: "duplicate full level",
			mutate: func(snapshot *cdfPublicSnapshotEvidence) {
				snapshot.Bids = append(snapshot.Bids, snapshot.Bids[0])
			},
		},
		{
			name: "zero quantity full level",
			mutate: func(snapshot *cdfPublicSnapshotEvidence) {
				snapshot.Asks[0].VisibleQty = 0
				snapshot.Asks[0].HiddenQty = 0
			},
		},
		{
			name: "public order mismatch",
			mutate: func(snapshot *cdfPublicSnapshotEvidence) {
				snapshot.PublicBids[0], snapshot.PublicBids[1] = snapshot.PublicBids[1], snapshot.PublicBids[0]
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			candidate.Bids = append([]etypes.PriceLevel(nil), valid.Bids...)
			candidate.Asks = append([]etypes.PriceLevel(nil), valid.Asks...)
			candidate.PublicBids = append([]etypes.PriceLevel(nil), valid.PublicBids...)
			candidate.PublicAsks = append([]etypes.PriceLevel(nil), valid.PublicAsks...)
			test.mutate(&candidate)
			if validCDFSnapshotProjection(candidate) {
				t.Fatal("invalid complete/public snapshot projection was accepted")
			}
		})
	}
}

func TestCDFVenueConcentrationSeparatesBookAvailabilityStates(t *testing.T) {
	contract := RegisteredSV1DActivationContract()
	observations := []cdfDepthObservation{
		{at: 0, globalSequence: 1, bidDepth: 10},
		{at: 10, globalSequence: 2, askDepth: 10},
		{at: 20, globalSequence: 3},
		{at: 30, globalSequence: 4, bidDepth: 10, askDepth: 10},
	}

	result := measureCDFVenueConcentration("north", observations, 40, contract)
	if result.BidOnlyDurationNano != 10 || result.AskOnlyDurationNano != 10 || result.EmptyBookDurationNano != 10 {
		t.Fatalf("availability durations = bid-only %d, ask-only %d, empty %d; want 10, 10, 10", result.BidOnlyDurationNano, result.AskOnlyDurationNano, result.EmptyBookDurationNano)
	}
	if result.OneSidedDurationNano != 20 || result.NonTwoSidedDurationNano != 30 {
		t.Fatalf("aggregate non-two-sided durations = one-sided %d, total %d; want 20, 30", result.OneSidedDurationNano, result.NonTwoSidedDurationNano)
	}
	if result.MaxUninterruptedNonTwoSidedDurationNano != 30 {
		t.Fatalf("maximum uninterrupted non-two-sided duration = %d; want 30", result.MaxUninterruptedNonTwoSidedDurationNano)
	}
	if result.TerminalBookMode != "two_sided" {
		t.Fatalf("terminal book mode = %q; want two_sided", result.TerminalBookMode)
	}
}

func TestCDFSupplierConcentrationEnforcesVenueLocalVolume(t *testing.T) {
	contract := RegisteredSV1DActivationContract()
	base := CDFSupplierActivationAudit{
		VolumeQty:                     3,
		VenueVolumeDenominatorQty:     4,
		DepthObservationCount:         1,
		BidDepthDominanceTimeFraction: 0,
		AskDepthDominanceTimeFraction: 0,
	}
	if !cdfSupplierConcentrationSatisfied(base, contract) {
		t.Fatal("supplier exactly at venue-local volume threshold was rejected")
	}

	justOver := base
	justOver.VolumeQty = 4
	if cdfSupplierConcentrationSatisfied(justOver, contract) {
		t.Fatal("supplier above venue-local volume threshold was accepted")
	}

	localMonopoly := base
	localMonopoly.VolumeQty = 76
	localMonopoly.VenueVolumeDenominatorQty = 100
	if cdfSupplierConcentrationSatisfied(localMonopoly, contract) {
		t.Fatal("local supplier monopoly was hidden by aggregate-share-independent input")
	}
}

func TestCDFVenueConcentrationClampsIntervalsAndPreservesInputOrder(t *testing.T) {
	contract := RegisteredSV1DActivationContract()
	observations := []cdfDepthObservation{
		{at: 20, globalSequence: 3, bidDepth: 20, askDepth: 20},
		{at: 5, globalSequence: 2, bidDepth: 20},
		{at: 0, globalSequence: 1, bidDepth: 20},
	}

	result := measureCDFVenueConcentration("north", observations, 10, contract)
	if result.BidOnlyDurationNano != 10 || result.NonTwoSidedDurationNano != 10 {
		t.Fatalf("clamped availability durations = bid-only %d, total %d; want 10, 10", result.BidOnlyDurationNano, result.NonTwoSidedDurationNano)
	}
	if result.MaxUninterruptedNonTwoSidedDurationNano != 10 {
		t.Fatalf("clamped maximum uninterrupted duration = %d; want 10", result.MaxUninterruptedNonTwoSidedDurationNano)
	}
	if result.TerminalBookMode != "bid_only" {
		t.Fatalf("terminal book mode = %q; want bid_only", result.TerminalBookMode)
	}
	if observations[0].at != 20 || observations[1].at != 5 || observations[2].at != 0 {
		t.Fatal("venue concentration measurement reordered the caller's observations")
	}
}

func writeOrderedCDFTestEvent(t *testing.T, path, venue string, localSequence, globalSequence uint64, eventName, symbol string, payload any) {
	t.Helper()
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{
		"client_id": uint64(7), "event": eventName, "sim_ts": int64(globalSequence),
		"data": map[string]any{"venue_id": venue, "symbol": symbol, "sequence": localSequence, "global_sequence": globalSequence, "payload": json.RawMessage(rawPayload)},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeCDFFixtureFile(t, path, append(raw, '\n'))
}
