package analysis

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// CDFBookAvailabilityAudit is the control-side companion to
// CDFActivationAudit. It measures the public CDF/USD book without assuming
// that a control contains the successor supplier roster. Evidence identity,
// lifecycle completeness, and terminal account endpoints remain mandatory.
type CDFBookAvailabilityAudit struct {
	Provenance             CDFActivationProvenance      `json:"provenance"`
	Venues                 []CDFVenueConcentrationAudit `json:"venues"`
	Checks                 []CDFActivationCheck         `json:"checks,omitempty"`
	EvidenceValid          bool                         `json:"evidence_valid"`
	StrictMechanicsValid   bool                         `json:"strict_mechanics_valid"`
	TerminalValuationValid bool                         `json:"terminal_valuation_valid"`
}

// AuditCDFBookAvailability validates a complete control run and measures only
// the public CDF/USD book. Supplier activation is deliberately outside this
// path: a no-roster or mode-off arm must remain a valid comparison arm when
// its own evidence is complete.
func (r *Run) AuditCDFBookAvailability(options CDFActivationOptions) (*CDFBookAvailabilityAudit, error) {
	if r == nil {
		return nil, fmt.Errorf("cdf availability: nil run")
	}
	if err := options.Contract.validate(); err != nil {
		return nil, fmt.Errorf("cdf availability contract: %w", err)
	}
	evidenceDir := options.EvidenceDir
	if evidenceDir == "" {
		evidenceDir = r.Dir
	}
	if !options.AllowLegacyJSON && !sameCDFPath(r.Dir, evidenceDir) {
		return nil, fmt.Errorf("cdf availability: report and evidence directories must be identical in strict mode")
	}
	scanRun, err := cdfActivationScanRun(r, options.RenderedEvidenceDir)
	if err != nil {
		return nil, err
	}
	config, metadata, err := loadCDFActivationIdentity(evidenceDir)
	if err != nil {
		return nil, err
	}
	if config.ExperimentID != options.Contract.ExperimentID || config.HypothesisID != options.Contract.HypothesisID ||
		config.Seed != options.Contract.Seed || metadata.SimulatedHorizon != options.Contract.Horizon ||
		metadata.SimulationStartNano != options.Contract.SimulationStartNano ||
		metadata.SimulationEndNano != options.Contract.SimulationEndNano ||
		!sameCDFStrings(config.VenueIDs, options.Contract.VenueIDs) {
		return nil, fmt.Errorf("cdf availability: run identity does not match the registered control contract")
	}
	strictMechanics := config.EvidenceFormat == "evstream_v3" && config.EvidenceContractVersion >= 2 && !options.AllowLegacyJSON
	if strictMechanics {
		if err := options.ExpectedProvenance.validate(); err != nil {
			return nil, fmt.Errorf("cdf availability expected provenance: %w", err)
		}
		if err := validateCDFExpectedProvenance(metadata, options.ExpectedProvenance); err != nil {
			return nil, err
		}
	}
	if config.EvidenceFormat == "evstream_v3" && config.EvidenceContractVersion >= 2 {
		eventsPath := filepath.Join(evidenceDir, "events.evs")
		if _, statErr := os.Stat(eventsPath); statErr != nil {
			if !options.AllowLegacyJSON {
				return nil, fmt.Errorf("cdf availability: v2 audit requires the canonical binary stream")
			}
		} else {
			if options.RenderedEvidenceDir == "" {
				return nil, fmt.Errorf("cdf availability: v2 audit requires independently rendered binary evidence")
			}
			if err := validateCDFCompletionArtifacts(evidenceDir, metadata, options.Contract.BinarySchemaEpoch); err != nil {
				return nil, err
			}
			if err := validateCDFRenderedGlobalSequence(scanRun, evidenceDir, options.RenderedEvidenceDir, options.Contract.BinarySchemaEpoch); err != nil {
				return nil, err
			}
		}
	}
	result := &CDFBookAvailabilityAudit{
		Provenance: CDFActivationProvenance{
			ConfigSHA256: metadata.ConfigSHA256, SourceRevision: metadata.GitRevision,
			SourceModified: metadata.SourceModified, BinarySHA256: metadata.BinarySHA256, Seed: metadata.Seed,
			Horizon: metadata.SimulatedHorizon, SimulationStartNano: metadata.SimulationStartNano,
			SimulationEndNano: metadata.SimulationEndNano, VenueIDs: append([]string(nil), config.VenueIDs...),
			ExperimentID: config.ExperimentID, HypothesisID: config.HypothesisID,
			EvidenceFormat: config.EvidenceFormat, LogMode: config.LogMode,
		},
		StrictMechanicsValid: strictMechanics,
	}
	if err := validateCDFTerminalValuation(r, metadata, options.Contract); err != nil {
		result.Checks = append(result.Checks, CDFActivationCheck{Failure: err.Error()})
		result.TerminalValuationValid = false
	} else {
		result.TerminalValuationValid = true
	}
	receiptAudit, receiptErr := AuditMarketDataReceipts(evidenceDir)
	if receiptErr != nil {
		return nil, fmt.Errorf("cdf availability receipts: %w", receiptErr)
	}
	if !receiptAudit.Valid {
		result.Checks = append(result.Checks, CDFActivationCheck{Failure: "market-data receipt contract is invalid"})
	}
	if strictMechanics {
		events, err := collectCDFOrderedEvents(scanRun)
		if err != nil {
			return nil, err
		}
		observations, checks := collectCDFPublicDepthObservations(events, options.Contract.VenueIDs, metadata.SimulationStartNano, metadata.SimulationEndNano)
		result.Checks = append(result.Checks, checks...)
		for _, venueID := range options.Contract.VenueIDs {
			result.Venues = append(result.Venues, measureCDFVenueConcentration(venueID, observations[venueID], metadata.SimulationEndNano, options.Contract))
		}
	} else {
		observations, checks, err := collectCDFLegacyPublicDepthObservations(scanRun, options.Contract.VenueIDs, metadata.SimulationStartNano, metadata.SimulationEndNano)
		if err != nil {
			return nil, err
		}
		result.Checks = append(result.Checks, checks...)
		for _, venueID := range options.Contract.VenueIDs {
			result.Venues = append(result.Venues, measureCDFVenueConcentration(venueID, observations[venueID], metadata.SimulationEndNano, options.Contract))
		}
	}
	for _, venue := range result.Venues {
		if venue.SnapshotCount == 0 {
			result.Checks = append(result.Checks, CDFActivationCheck{VenueID: venue.VenueID, Failure: "no public CDF/USD book observations"})
		}
	}
	result.EvidenceValid = len(result.Checks) == 0
	result.StrictMechanicsValid = result.StrictMechanicsValid && result.EvidenceValid
	return result, nil
}

// collectCDFPublicDepthObservations replays only public book transitions. It
// does not infer supplier ownership from order events, which is the key
// distinction between this control audit and the treatment audit.
func collectCDFPublicDepthObservations(events []Event, venueIDs []string, startAt, terminalAt int64) (map[string][]cdfDepthObservation, []CDFActivationCheck) {
	observations := make(map[string][]cdfDepthObservation, len(venueIDs))
	allowedVenues := make(map[string]struct{}, len(venueIDs))
	for _, venueID := range venueIDs {
		allowedVenues[venueID] = struct{}{}
	}
	publicDepth := make(map[string]*cdfPublicDepthState, len(venueIDs))
	var checks []CDFActivationCheck
	for _, event := range events {
		if _, allowed := allowedVenues[event.VenueID]; !allowed {
			checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF event belongs to an unregistered venue"})
			continue
		}
		if event.SimTS < startAt || event.SimTS > terminalAt {
			checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF event lies outside the registered simulation interval"})
			continue
		}
		switch event.Name {
		case "BookSnapshot":
			var snapshot cdfPublicSnapshotEvidence
			if err := decodeRequiredJSON(event.Raw(), &snapshot, "bids", "asks", "source_sequence", "public_bids", "public_asks"); err != nil {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed public CDF snapshot: " + err.Error()})
				continue
			}
			if snapshot.SourceSequence == 0 || snapshot.Bids == nil || snapshot.Asks == nil || snapshot.PublicBids == nil || snapshot.PublicAsks == nil || !validCDFSnapshotProjection(snapshot) {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF snapshot lacks a verified public projection"})
				continue
			}
			state, ok := newCDFPublicDepthState(snapshot.PublicBids, snapshot.PublicAsks)
			if !ok {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF depth is negative or overflows"})
				continue
			}
			publicDepth[event.VenueID] = state
			observation, ok := cdfPublicDepthObservation(event, state)
			if !ok {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF snapshot depth overflows"})
				continue
			}
			observations[event.VenueID] = append(observations[event.VenueID], observation)
		case "BookDelta":
			state := publicDepth[event.VenueID]
			if state == nil || !state.initialized {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "CDF BookDelta precedes a complete public snapshot"})
				continue
			}
			var delta cdfBookDeltaEvidence
			if err := decodeRequiredJSON(event.Raw(), &delta, "side", "price", "visible_qty", "hidden_qty"); err != nil {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed public CDF delta: " + err.Error()})
				continue
			}
			if delta.Price <= 0 || delta.VisibleQty < 0 || delta.HiddenQty < 0 || (delta.Side != "BUY" && delta.Side != "SELL") {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF delta has invalid side, price, or quantity"})
				continue
			}
			levels := state.bids
			if delta.Side == "SELL" {
				levels = state.asks
			}
			if delta.VisibleQty == 0 {
				delete(levels, delta.Price)
			} else {
				levels[delta.Price] = delta.VisibleQty
			}
			observation, ok := cdfPublicDepthObservation(event, state)
			if !ok {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF delta depth overflows"})
				continue
			}
			observations[event.VenueID] = append(observations[event.VenueID], observation)
		}
	}
	return observations, checks
}

func cdfPublicDepthObservation(event Event, state *cdfPublicDepthState) (cdfDepthObservation, bool) {
	if state == nil || !state.initialized {
		return cdfDepthObservation{}, false
	}
	bidDepth, bidOK := totalCDFDepthMap(state.bids)
	askDepth, askOK := totalCDFDepthMap(state.asks)
	if !bidOK || !askOK {
		return cdfDepthObservation{}, false
	}
	return cdfDepthObservation{
		at: event.SimTS, globalSequence: event.GlobalSequence,
		bidDepth: bidDepth, askDepth: askDepth,
		supplierDepthByKey: make(map[cdfParticipantKey]cdfSupplierDepth),
	}, true
}

func collectCDFLegacyPublicDepthObservations(run *Run, venueIDs []string, startAt, terminalAt int64) (map[string][]cdfDepthObservation, []CDFActivationCheck, error) {
	if run == nil {
		return nil, nil, fmt.Errorf("cdf availability: nil legacy evidence run")
	}
	observations := make(map[string][]cdfDepthObservation, len(venueIDs))
	checks := []CDFActivationCheck{}
	allowedVenues := make(map[string]struct{}, len(venueIDs))
	for _, venueID := range venueIDs {
		allowedVenues[venueID] = struct{}{}
	}
	paths := run.Files()
	sort.Strings(paths)
	for _, path := range paths {
		if symbolFromPath(path) != cdfActivationLogName {
			continue
		}
		var callbackErr error
		state := (*cdfPublicDepthState)(nil)
		if err := run.Scan(ScanOptions{Events: []string{"BookSnapshot", "BookDelta"}, Files: []string{path}, FilesSelected: true, Workers: 1}, func(event Event) {
			if callbackErr != nil {
				return
			}
			if _, allowed := allowedVenues[event.VenueID]; !allowed {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF event belongs to an unregistered venue"})
				return
			}
			if event.SimTS < startAt || event.SimTS > terminalAt {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF event lies outside the registered simulation interval"})
				return
			}
			if event.Name == "BookSnapshot" {
				var snapshot cdfPublicSnapshotEvidence
				if err := decodeRequiredJSON(event.Raw(), &snapshot, "bids", "asks", "source_sequence", "public_bids", "public_asks"); err != nil || snapshot.SourceSequence == 0 || snapshot.Bids == nil || snapshot.Asks == nil || snapshot.PublicBids == nil || snapshot.PublicAsks == nil || !validCDFSnapshotProjection(snapshot) {
					checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed or unverifiable public CDF snapshot"})
					return
				}
				var callbackOK bool
				state, callbackOK = newCDFPublicDepthState(snapshot.PublicBids, snapshot.PublicAsks)
				if !callbackOK {
					checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF depth is negative or overflows"})
					return
				}
			} else {
				if state == nil || !state.initialized {
					checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "CDF BookDelta precedes a complete public snapshot"})
					return
				}
				var delta cdfBookDeltaEvidence
				if err := decodeRequiredJSON(event.Raw(), &delta, "side", "price", "visible_qty", "hidden_qty"); err != nil || delta.Price <= 0 || delta.VisibleQty < 0 || delta.HiddenQty < 0 || (delta.Side != "BUY" && delta.Side != "SELL") {
					checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "malformed public CDF delta"})
					return
				}
				levels := state.bids
				if delta.Side == "SELL" {
					levels = state.asks
				}
				if delta.VisibleQty == 0 {
					delete(levels, delta.Price)
				} else {
					levels[delta.Price] = delta.VisibleQty
				}
			}
			observation, ok := cdfPublicDepthObservation(event, state)
			if !ok {
				checks = append(checks, CDFActivationCheck{VenueID: event.VenueID, Ordinal: event.Ordinal, Failure: "public CDF depth overflows"})
				return
			}
			observations[event.VenueID] = append(observations[event.VenueID], observation)
		}); err != nil {
			return nil, nil, fmt.Errorf("cdf availability: scan legacy CDF book in %s: %w", path, err)
		}
		if callbackErr != nil {
			return nil, nil, callbackErr
		}
	}
	return observations, checks, nil
}

func validateCDFTerminalValuation(run *Run, metadata cdfActivationMetadata, contract CDFActivationContract) error {
	if run == nil || len(run.Report.InitialAccounts) == 0 || len(run.Report.TerminalAccounts) == 0 {
		return fmt.Errorf("terminal valuation evidence has no initial or terminal accounts")
	}
	initialVenues := make(map[string]struct{})
	for _, row := range run.Report.InitialAccounts {
		if row.Account.Timestamp != metadata.SimulationStartNano {
			return fmt.Errorf("initial account timestamp does not match the registered simulation start")
		}
		initialVenues[row.VenueID] = struct{}{}
	}
	terminalVenues := make(map[string]struct{})
	for _, row := range run.Report.TerminalAccounts {
		if row.Account.Timestamp != metadata.SimulationEndNano {
			return fmt.Errorf("terminal account timestamp does not match the registered simulation end")
		}
		terminalVenues[row.VenueID] = struct{}{}
	}
	for _, venueID := range contract.VenueIDs {
		if _, ok := initialVenues[venueID]; !ok {
			return fmt.Errorf("initial valuation omits venue %q", venueID)
		}
		if _, ok := terminalVenues[venueID]; !ok {
			return fmt.Errorf("terminal valuation omits venue %q", venueID)
		}
	}
	return nil
}
