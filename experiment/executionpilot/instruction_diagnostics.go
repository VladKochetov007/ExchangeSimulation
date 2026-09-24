package executionpilot

import (
	"errors"
	"fmt"
	"reflect"
)

type InstructionPairDiagnostic struct {
	TargetQty                           int64         `json:"target_qty"`
	Seed                                int64         `json:"seed"`
	PreArrivalAligned                   bool          `json:"pre_arrival_aligned"`
	IOCStatus                           OutcomeStatus `json:"ioc_status"`
	FOKStatus                           OutcomeStatus `json:"fok_status"`
	IOCSelectedCapAskQty                *int64        `json:"ioc_selected_cap_ask_qty,omitempty"`
	FOKSelectedCapAskQty                *int64        `json:"fok_selected_cap_ask_qty,omitempty"`
	IOCFilledQty                        int64         `json:"ioc_filled_qty"`
	FOKFilledQty                        int64         `json:"fok_filled_qty"`
	IOCRejectReason                     string        `json:"ioc_reject_reason,omitempty"`
	IOCCancelReason                     string        `json:"ioc_cancel_reason,omitempty"`
	FOKRejectReason                     string        `json:"fok_reject_reason,omitempty"`
	FOKCancelReason                     string        `json:"fok_cancel_reason,omitempty"`
	SelectedProxyBelowTargetBothFilled  bool          `json:"selected_proxy_below_target_both_filled"`
	SelectedProxyAtLeastTargetFOKReject bool          `json:"selected_proxy_at_least_target_fok_reject"`
}

type InstructionDiagnostics struct {
	SchemaVersion                            int                         `json:"schema_version"`
	SurfaceFileSHA256                        string                      `json:"surface_file_sha256"`
	ExecutionSource                          string                      `json:"execution_source"`
	AggregationSource                        string                      `json:"aggregation_source"`
	ValidWorlds                              int                         `json:"valid_worlds"`
	AllPreArrivalAligned                     bool                        `json:"all_pre_arrival_aligned"`
	TerminalMarkAvailableWorlds              int                         `json:"terminal_mark_available_worlds"`
	TargetShortfallDefinedWorlds             int                         `json:"target_shortfall_defined_worlds"`
	PartialIOCRejectedFOKPairs               int                         `json:"partial_ioc_rejected_fok_pairs"`
	SelectedProxyBelowTargetBothFillPairs    int                         `json:"selected_proxy_below_target_both_fill_pairs"`
	SelectedProxyAtLeastTargetFOKRejectPairs int                         `json:"selected_proxy_at_least_target_fok_reject_pairs"`
	Pairs                                    []InstructionPairDiagnostic `json:"pairs"`
}

type instructionPreArrival struct {
	DecisionAt            int64
	DecisionMid           int64
	DeliveredSnapshotAt   int64
	PublishedSnapshotAt   int64
	DeliveredSnapshotSeq  uint64
	LatestMessageAt       int64
	LatestMessageSeq      uint64
	LatestMessageTwoSided bool
	RetainedAfterOneSided bool
	DeliveredTouchAskQty  int64
	DeliveredFiveAskQty   int64
	CapBoundedAskQty      *int64
	DeliveredSpread       int64
	MechanicalSweepQty    int64
	MechanicalSweepCost   int64
	OrderSentAt           int64
	VenueArrivalAt        int64
	InitialABC            int64
	InitialUSD            int64
}

func preArrivalInstruction(outcome ReconstructedOutcome) instructionPreArrival {
	return instructionPreArrival{DecisionAt: outcome.DecisionAt, DecisionMid: outcome.DecisionMid,
		DeliveredSnapshotAt: outcome.DeliveredSnapshotAt, PublishedSnapshotAt: outcome.PublishedSnapshotAt,
		DeliveredSnapshotSeq: outcome.DeliveredSnapshotSeq, LatestMessageAt: outcome.LatestMessageAt,
		LatestMessageSeq: outcome.LatestMessageSeq, LatestMessageTwoSided: outcome.LatestMessageTwoSided,
		RetainedAfterOneSided: outcome.RetainedAfterOneSided,
		DeliveredTouchAskQty:  outcome.DeliveredTouchAskQty, DeliveredFiveAskQty: outcome.DeliveredFiveAskQty,
		CapBoundedAskQty: outcome.CapBoundedAskQty, DeliveredSpread: outcome.DeliveredSpread,
		MechanicalSweepQty: outcome.MechanicalSweepQty, MechanicalSweepCost: outcome.MechanicalSweepCost,
		OrderSentAt: outcome.OrderSentAt, VenueArrivalAt: outcome.VenueArrivalAt,
		InitialABC: outcome.InitialABC, InitialUSD: outcome.InitialUSD}
}

func DiagnoseInstructionSurface(path string) (InstructionDiagnostics, error) {
	surface, err := decodeStrictFile[InstructionSurface](path)
	if err != nil {
		return InstructionDiagnostics{}, err
	}
	if surface.SchemaVersion != 1 || surface.AssignedWorlds != 12 || surface.ValidWorlds != 12 ||
		len(surface.Cells) != 12 || len(surface.PairedContrasts) != 6 || len(surface.ContrastRanges) != 2 {
		return InstructionDiagnostics{}, errors.New("instruction pilot: incomplete surface for diagnostics")
	}
	digest, err := fileSHA256(path)
	if err != nil {
		return InstructionDiagnostics{}, err
	}
	diagnostic := InstructionDiagnostics{SchemaVersion: 1, SurfaceFileSHA256: digest,
		ExecutionSource: surface.ExecutionIdentity.SourceCommit, AggregationSource: surface.AggregationCommit,
		ValidWorlds: 12, AllPreArrivalAligned: true}
	byID := map[string]InstructionCellResult{}
	for _, cell := range surface.Cells {
		id, err := instructionCellID(cell.Cell)
		if err != nil || id != cell.ID || cell.Outcome.FilledQty < 0 ||
			cell.Outcome.FilledQty > cell.Cell.TargetQty ||
			cell.FilledFraction != float64(cell.Outcome.FilledQty)/float64(cell.Cell.TargetQty) {
			return InstructionDiagnostics{}, fmt.Errorf("instruction pilot: inconsistent diagnostic cell %s: %w", cell.ID, err)
		}
		if _, exists := byID[id]; exists {
			return InstructionDiagnostics{}, errors.New("instruction pilot: duplicate diagnostic cell")
		}
		byID[id] = cell
		if cell.Outcome.TerminalMarkAvailable {
			diagnostic.TerminalMarkAvailableWorlds++
		}
		if cell.Outcome.TargetShortfallDefined {
			diagnostic.TargetShortfallDefinedWorlds++
		}
	}
	for _, target := range []int64{50_000_000, 500_000_000} {
		for _, seed := range []int64{14001, 14011, 14017} {
			iocID, _ := instructionCellID(InstructionCell{"IOC", target, seed})
			fokID, _ := instructionCellID(InstructionCell{"FOK", target, seed})
			ioc, iocExists := byID[iocID]
			fok, fokExists := byID[fokID]
			if !iocExists || !fokExists {
				return InstructionDiagnostics{}, errors.New("instruction pilot: missing diagnostic pair")
			}
			registeredPair := surface.PairedContrasts[len(diagnostic.Pairs)]
			if registeredPair.TargetQty != target || registeredPair.Seed != seed ||
				registeredPair.IOC != ioc.FilledFraction || registeredPair.FOK != fok.FilledFraction ||
				registeredPair.Difference != ioc.FilledFraction-fok.FilledFraction {
				return InstructionDiagnostics{}, errors.New("instruction pilot: paired contrast differs from cells")
			}
			aligned := reflect.DeepEqual(preArrivalInstruction(ioc.Outcome), preArrivalInstruction(fok.Outcome))
			diagnostic.AllPreArrivalAligned = diagnostic.AllPreArrivalAligned && aligned
			pair := InstructionPairDiagnostic{TargetQty: target, Seed: seed, PreArrivalAligned: aligned,
				IOCStatus: ioc.Outcome.Status, FOKStatus: fok.Outcome.Status,
				IOCSelectedCapAskQty: ioc.Outcome.CapBoundedAskQty,
				FOKSelectedCapAskQty: fok.Outcome.CapBoundedAskQty,
				IOCFilledQty:         ioc.Outcome.FilledQty, FOKFilledQty: fok.Outcome.FilledQty,
				IOCRejectReason: ioc.Outcome.RejectReason, IOCCancelReason: ioc.Outcome.CancelReason,
				FOKRejectReason: fok.Outcome.RejectReason, FOKCancelReason: fok.Outcome.CancelReason}
			if ioc.Outcome.CapBoundedAskQty != nil && fok.Outcome.CapBoundedAskQty != nil {
				pair.SelectedProxyBelowTargetBothFilled = *ioc.Outcome.CapBoundedAskQty < target &&
					*ioc.Outcome.CapBoundedAskQty == *fok.Outcome.CapBoundedAskQty &&
					ioc.Outcome.FilledQty == target && fok.Outcome.FilledQty == target
				pair.SelectedProxyAtLeastTargetFOKReject = *ioc.Outcome.CapBoundedAskQty >= target &&
					*ioc.Outcome.CapBoundedAskQty == *fok.Outcome.CapBoundedAskQty &&
					fok.Outcome.Status == OutcomeRejected && fok.Outcome.RejectReason == "FOK_NOT_FILLED"
			}
			if ioc.Outcome.Status == OutcomePartiallyFilled && fok.Outcome.Status == OutcomeRejected {
				diagnostic.PartialIOCRejectedFOKPairs++
			}
			if pair.SelectedProxyBelowTargetBothFilled {
				diagnostic.SelectedProxyBelowTargetBothFillPairs++
			}
			if pair.SelectedProxyAtLeastTargetFOKReject {
				diagnostic.SelectedProxyAtLeastTargetFOKRejectPairs++
			}
			diagnostic.Pairs = append(diagnostic.Pairs, pair)
		}
	}
	return diagnostic, nil
}

func WriteInstructionDiagnostics(path string, diagnostic InstructionDiagnostics) error {
	if diagnostic.SchemaVersion != 1 || diagnostic.ValidWorlds != 12 || len(diagnostic.Pairs) != 6 {
		return errors.New("instruction pilot: incomplete diagnostics")
	}
	return writeExclusiveJSON(path, diagnostic)
}
