package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"exchange_sim/simulation"
	etypes "exchange_sim/types"
)

type cdfActivationFixtureOptions struct {
	mutateConfig          func(*cdfActivationConfig)
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
			initialEquity := cdfFixtureInitialEquity(supplier)
			fee := cdfFixtureFee(supplier.ReferencePrice-2*supplier.TickSize, supplier.MinimumQualifyingQty, supplier.BasePrecision, supplier.MakerFeeBps)
			terminalBase := supplier.InitialBaseBalance + supplier.MinimumQualifyingQty
			terminalQuote := supplier.InitialQuoteBalance - cdfFixtureNotional(supplier.ReferencePrice-2*supplier.TickSize, supplier.MinimumQualifyingQty, supplier.BasePrecision) - fee
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
	firstSnapshots := make(map[string]etypes.BookSnapshot, len(contract.VenueIDs))
	secondSnapshots := make(map[string]etypes.BookSnapshot, len(contract.VenueIDs))
	for _, venueID := range contract.VenueIDs {
		firstSnapshots[venueID] = cdfFixtureSnapshot(venueID, 1, options)
		secondSnapshots[venueID] = cdfFixtureSnapshot(venueID, 2, options)
	}
	recordCDFFixtureReceiptRound(t, recorder, participants, firstSnapshots, 1, firstAt, func(participant *cdfActivationFixtureParticipant, frontier simulation.MarketDataFrontier) {
		participant.first = frontier
	})
	for _, participant := range participants {
		decisionAt := contract.SimulationStartNano + 2_000_000_000 + participant.contract.DecisionPhaseOffset
		recorder.RecordDecision(simulation.MarketDataDecision{
			ClientID: participant.clientID, SourceVenue: participant.venueID, Link: participant.link,
			Symbol: cdfActivationSymbol, RequestID: cdfFixtureRequestID(participant.clientID, 1),
			Side: etypes.Buy, OrderType: etypes.LimitOrder, TimeInForce: etypes.GTC,
			Price: participant.contract.ReferencePrice - 2*participant.contract.TickSize,
			Qty:   participant.contract.MinimumQualifyingQty, DecisionAt: decisionAt, Frontier: participant.first,
		})
	}
	recordCDFFixtureReceiptRound(t, recorder, participants, secondSnapshots, 2, secondAt, func(participant *cdfActivationFixtureParticipant, frontier simulation.MarketDataFrontier) {
		participant.second = frontier
	})
	for _, participant := range participants {
		decisionAt := contract.SimulationStartNano + 12_000_000_000 + participant.contract.DecisionPhaseOffset
		recorder.RecordDecision(simulation.MarketDataDecision{
			ClientID: participant.clientID, SourceVenue: participant.venueID, Link: participant.link,
			Symbol: cdfActivationSymbol, RequestID: cdfFixtureRequestID(participant.clientID, 2),
			Side: etypes.Sell, OrderType: etypes.LimitOrder, TimeInForce: etypes.GTC,
			Price: cdfFixturePostPrice(participant), Qty: participant.contract.MinimumQualifyingQty,
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
		if options.dominantVolume {
			independentQty = 1_000_000
		}
		bookEvents[venueID] = append(bookEvents[venueID], cdfFixtureEvent{
			at: contract.SimulationStartNano + 9_000_000_000, event: "Trade", symbol: cdfActivationSymbol,
			payload: cdfTradeEvidence{TradeID: 900_000 + uint64(len(venueID)), Price: 300_000_000, Qty: independentQty, Side: "BUY"},
		})
		thirdAt := contract.SimulationStartNano + 17_000_000_000
		fourthAt := contract.SimulationStartNano + 25_000_000_000
		if options.dominantDepth {
			fourthAt = contract.SimulationEndNano - 1
		}
		bookEvents[venueID] = append(bookEvents[venueID],
			cdfSnapshotFixtureEvent(thirdAt, venueID, 3, cdfFixtureSnapshot(venueID, 3, options)),
			cdfFixtureEvent{
				at: thirdAt + 1_000_000_000, event: "BookDelta", symbol: cdfActivationSymbol,
				payload: cdfBookDeltaEvidence{Side: "BUY", Price: 299_800_000, VisibleQty: 20_000_000},
			},
			cdfSnapshotFixtureEvent(fourthAt, venueID, 4, cdfFixtureSnapshot(venueID, 4, options)),
		)
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
			Fingerprint: fingerprint, PublishedAt: publishedAt, ScheduledAt: publishedAt + 10_000_000,
			LinkOrdinal: sequence,
		}
		if recorder.RecordSchedule(schedule) == 0 {
			t.Fatal("record CDF fixture schedule")
		}
		schedules[participant] = schedule
	}
	for _, participant := range participants {
		frontier := recorder.RecordReceipt(simulation.MarketDataReceipt{
			MarketDataSchedule: schedules[participant], DeliveredAt: publishedAt + 10_000_000,
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
	quantity := supplier.MinimumQualifyingQty
	initialPrice := supplier.ReferencePrice - 2*supplier.TickSize
	fee := cdfFixtureFee(initialPrice, quantity, supplier.BasePrecision, supplier.MakerFeeBps)
	notional := cdfFixtureNotional(initialPrice, quantity, supplier.BasePrecision)
	initialEquity := cdfFixtureInitialEquity(supplier)
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

	firstSnapshot := cdfFixtureSnapshot(participant.venueID, 1, options)
	firstDecision := cdfFixtureDecision(participant, participant.first, firstSnapshot, firstAt, firstDecisionAt, "submit")
	if options.badFingerprint && participantOrdinal == 0 {
		firstDecision.ObservationFingerprint = "00000000000000000000000000000000"
	}
	(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID],
		cdfFixtureEvent{at: start + 500_000_000 + supplier.DecisionPhaseOffset, clientID: participant.clientID, event: "balance_snapshot", payload: cdfFixtureBalanceSnapshot(participant, start+500_000_000+supplier.DecisionPhaseOffset, 0, false)},
		cdfFixtureEvent{at: firstDecisionAt, clientID: participant.clientID, event: "elastic_liquidity_supplier_decision", payload: firstDecision},
	)
	(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID],
		cdfFixtureEvent{at: acceptedOneAt, clientID: participant.clientID, event: "OrderAccepted", symbol: cdfActivationSymbol, payload: cdfAcceptedEvidence{
			OrderID: orderOne, ClientID: participant.clientID, RequestID: requestOne, Side: "BUY",
			Type: "LIMIT", TimeInForce: "GTC", PostOnly: true, Price: initialPrice, Qty: quantity,
		}},
		cdfFixtureEvent{at: fillAt, event: "Trade", symbol: cdfActivationSymbol, payload: cdfTradeEvidence{TradeID: tradeID, Price: initialPrice, Qty: quantity, Side: "SELL"}},
	)
	if !(options.omitSupplierFill && participantOrdinal == 0) {
		(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID], cdfFixtureEvent{
			at: fillAt, clientID: participant.clientID, event: "elastic_liquidity_supplier_fill",
			payload: cdfFillEvidence{
				Role: supplier.Role, ClientID: participant.clientID, Symbol: cdfActivationSymbol,
				OrderID: orderOne, TradeID: tradeID, Timestamp: fillAt, Side: "BUY", Price: initialPrice,
				Qty: quantity, FeeAmount: fee, FeeAsset: "USD", IsFull: true,
				PositionBefore: 0, PositionAfter: quantity,
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
			OrderID: orderOne, TradeID: tradeID, Side: "BUY", Price: initialPrice, Qty: exchangeFillQty,
			FeeAmount: fee, FeeAsset: "USD", FilledQty: exchangeFillQty, RemainingQty: 0, IsFull: true,
		},
	})
	borrowed := options.borrowedSupplier && participantOrdinal == 0
	(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID], cdfFixtureEvent{
		at: postBalanceAt, clientID: participant.clientID, event: "balance_snapshot",
		payload: cdfFixtureBalanceSnapshot(participant, postBalanceAt, quantity, borrowed),
	})
	if options.omitPostDecision && participantOrdinal == 0 {
		return
	}
	secondSnapshot := cdfFixtureSnapshot(participant.venueID, 2, options)
	postDecision := cdfFixtureDecision(participant, participant.second, secondSnapshot, secondAt, postDecisionAt, "submit")
	postDecision.Position = quantity
	postDecision.TargetPosition = 0
	postDecision.GrossInventory = supplier.InitialBaseBalance + quantity
	postDecision.Side = "SELL"
	postDecision.QuotePrice = cdfFixturePostPrice(participant)
	postDecision.QuoteQty = quantity
	postDecision.QuoteRequestID = requestTwo
	postDecision.QuoteSubmittedAt = postDecisionAt
	postDecision.QuoteCashAvailable = supplier.InitialQuoteBalance - notional - fee
	postDecision.QuoteCashRequired = 0
	postDecision.EquityQuote = cdfFixtureTerminalEquity(supplier, supplier.InitialBaseBalance+quantity, supplier.InitialQuoteBalance-notional-fee)
	postDecision.PeakEquityQuote = maxCDFTestInt64(initialEquity, postDecision.EquityQuote)
	postDecision.LossFromInitialQuote = maxCDFTestInt64(0, initialEquity-postDecision.EquityQuote)
	postDecision.DrawdownQuote = postDecision.PeakEquityQuote - postDecision.EquityQuote
	if participant.venueID == "north" {
		postDecision.QuotePriceSource = "one_sided_missing_side_blended"
	}
	(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID], cdfFixtureEvent{
		at: postDecisionAt, clientID: participant.clientID, event: "elastic_liquidity_supplier_decision", payload: postDecision,
	})
	(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID], cdfFixtureEvent{
		at: acceptedTwoAt, clientID: participant.clientID, event: "OrderAccepted", symbol: cdfActivationSymbol,
		payload: cdfAcceptedEvidence{
			OrderID: orderTwo, ClientID: participant.clientID, RequestID: requestTwo, Side: "SELL",
			Type: "LIMIT", TimeInForce: "GTC", PostOnly: true, Price: postDecision.QuotePrice, Qty: quantity,
		},
	})
	cancelDecision := postDecision
	cancelDecision.DecisionTime = cancelDecisionAt
	cancelDecision.ObservationAge = cancelDecisionAt - secondAt
	cancelDecision.Action = "cancel"
	cancelDecision.Reason = "reprice_for_inventory_or_touch"
	cancelDecision.QuoteOrderID = orderTwo
	cancelDecision.CancelRequestID = cancelRequest
	(*generalEvents)[participant.venueID] = append((*generalEvents)[participant.venueID], cdfFixtureEvent{
		at: cancelDecisionAt, clientID: participant.clientID, event: "elastic_liquidity_supplier_decision", payload: cancelDecision,
	})
	if !(options.omitCancellation && participantOrdinal == 0) {
		(*bookEvents)[participant.venueID] = append((*bookEvents)[participant.venueID], cdfFixtureEvent{
			at: cancelledAt, clientID: participant.clientID, event: "OrderCancelled", symbol: cdfActivationSymbol,
			payload: cdfCancelledEvidence{OrderID: orderTwo, RequestID: cancelRequest, RemainingQty: quantity},
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
		MarkPrice: mark, RiskMarkPrice: mark, LocalBookMode: mode,
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

func cdfFixtureSnapshot(venueID string, sequence uint64, options cdfActivationFixtureOptions) etypes.BookSnapshot {
	const independentDepth = int64(20_000_000)
	snapshot := etypes.BookSnapshot{
		Bids: []etypes.PriceLevel{{Price: 299_800_000, VisibleQty: independentDepth}},
		Asks: []etypes.PriceLevel{{Price: 300_200_000, VisibleQty: independentDepth}},
	}
	if sequence == 2 && venueID == "north" {
		snapshot.Asks = []etypes.PriceLevel{}
	}
	if sequence == 3 && venueID == "north" {
		snapshot.Asks[0].Price = 299_900_000
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
			preFillDecision: cdfDecisionEvidence{ObservationSequence: 1, ReferencePrice: 100, MarkPrice: 100, TargetPosition: 2, Position: 0, QuotePrice: 99, QuoteQty: 2},
			preFillKnown:    true,
		}},
	}
	audit := &CDFActivationAudit{strictMechanics: true}
	audit.recordCDFPostFillResponse(Event{SimTS: 20, GlobalSequence: 5}, state, cdfDecisionEvidence{
		ObservationSequence: 2, ReferencePrice: 101, MarkPrice: 101, Position: 5, TargetPosition: 0,
		Action: "submit", Side: "SELL", QuotePrice: 102, QuoteQty: 5,
	})
	if state.audit.PostFillResponsiveCount != 1 || !state.fillResponses[0].responded {
		t.Fatalf("later observation was not accepted as a post-fill response: %+v", state)
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
