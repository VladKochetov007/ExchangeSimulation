package analysis

import "fmt"

type CrossVenueFirstAttemptSpec struct {
	EvidenceDir             string
	Venues                  [2]string
	Clients                 map[string]uint64
	RouterID                uint64
	Symbol                  string
	BaseAsset               string
	QuoteAsset              string
	HorizonNano             int64
	LotQty                  int64
	BasePrecision           int64
	TakerFeeBps             int64
	MaxAttempts             int
	MaxBookEvidenceAgeNanos int64
}

type CrossVenueFirstAttemptEvidence struct {
	Counters      CrossVenueRouterEvidenceCounters
	Timeline      *CrossVenueOpportunityTimeline
	Groups        []CrossVenueSubmissionGroup
	Placements    []CrossVenuePlacementResult
	Fills         []CrossVenueExchangeFill
	Terminal      *CrossVenueFirstAttemptTerminal
	Accounts      map[string]CrossVenueAccountDelta
	Movements     map[string]CrossVenueRouterMovementAccount
	Funding       *CrossVenueQuoteFunding
	TerminalValue CrossVenueTerminalValue
	PublicReplay  *CrossVenuePublicReplay
}

// ReconstructCrossVenueFirstAttempt joins the registered router's entire
// opportunity-to-terminal path. The caller must open a verified rendered
// binary run and separately bind its report and raw sidecars to one manifest.
func (r *Run) ReconstructCrossVenueFirstAttempt(spec CrossVenueFirstAttemptSpec) (*CrossVenueFirstAttemptEvidence, error) {
	if r == nil || spec.EvidenceDir == "" || spec.RouterID == 0 || spec.Symbol == "" ||
		spec.BaseAsset == "" || spec.QuoteAsset == "" || spec.BaseAsset == spec.QuoteAsset ||
		spec.Venues[0] == "" || spec.Venues[1] == "" || spec.Venues[0] == spec.Venues[1] ||
		len(spec.Clients) != 2 || spec.Clients[spec.Venues[0]] == 0 || spec.Clients[spec.Venues[1]] == 0 ||
		spec.HorizonNano <= 0 || spec.LotQty <= 0 || spec.BasePrecision <= 0 || spec.TakerFeeBps < 0 ||
		spec.MaxAttempts != 1 || spec.MaxBookEvidenceAgeNanos <= 0 {
		return nil, fmt.Errorf("cross-venue first-attempt world: invalid registered contract")
	}
	counters, err := r.CrossVenueRouterCounters(spec.RouterID)
	if err != nil {
		return nil, err
	}
	evaluations, err := r.CollectCrossVenueEvaluations(spec.Venues, spec.RouterID, counters.QuoteEvaluations)
	if err != nil {
		return nil, err
	}
	receipts, err := r.CollectCrossVenueResponseReceipts(spec.Venues, spec.RouterID, counters.ResponseReceipts)
	if err != nil {
		return nil, err
	}
	placements, err := r.CollectCrossVenuePlacements(spec.Venues, spec.Clients, spec.Symbol, spec.LotQty)
	if err != nil {
		return nil, err
	}
	fills, err := r.CollectCrossVenueExchangeFills(spec.Venues, spec.Clients, spec.Symbol, receipts)
	if err != nil {
		return nil, err
	}
	publicEvents, err := r.CollectCrossVenuePublicEvents(spec.Venues, spec.Symbol)
	if err != nil {
		return nil, err
	}
	timeline, err := ReconstructCrossVenueOpportunityTimeline(evaluations, publicEvents, spec.EvidenceDir, spec.Venues, spec.Symbol, spec.HorizonNano, spec.LotQty, spec.BasePrecision, spec.TakerFeeBps)
	if err != nil {
		return nil, err
	}
	decisions, err := selectCrossVenueFirstAttemptDecisions(spec, evaluations)
	if err != nil {
		return nil, err
	}
	groups, err := BindCrossVenueSubmissionOutcomes(evaluations, decisions, placements, spec.Symbol, spec.LotQty, spec.MaxAttempts)
	if err != nil {
		return nil, err
	}
	if err := VerifyCrossVenueResponseActors(evaluations, groups, receipts, spec.Clients); err != nil {
		return nil, err
	}
	placementResults, err := ReconcileCrossVenuePlacementReceipts(placements, receipts, fills, spec.HorizonNano, spec.LotQty)
	if err != nil {
		return nil, err
	}
	terminal, err := ReconcileCrossVenueFirstAttemptTerminal(groups, placementResults, fills, counters, spec.LotQty)
	if err != nil {
		return nil, err
	}
	accountConvention := CrossVenueAccountConvention{
		Symbol: spec.Symbol, BaseAsset: spec.BaseAsset, QuoteAsset: spec.QuoteAsset,
		BasePrecision: spec.BasePrecision, TakerFeeBps: spec.TakerFeeBps,
	}
	accounts, err := ReconcileCrossVenueRouterAccounts(r.Report, spec.Venues, spec.Clients, fills, accountConvention)
	if err != nil {
		return nil, err
	}
	movements, err := r.AuditCrossVenueRouterMovements(spec.Venues, spec.Clients, fills, accountConvention)
	if err != nil {
		return nil, err
	}
	funding, err := AssessCrossVenueFirstAttemptFunding(evaluations, timeline, movements, spec.Venues, spec.LotQty, spec.BasePrecision, spec.TakerFeeBps)
	if err != nil {
		return nil, err
	}
	replay, err := ReplayCrossVenuePublicEvents(publicEvents, CrossVenuePublicReplayOptions{
		Venues: spec.Venues, Symbol: spec.Symbol, InitiallyEmpty: true,
	})
	if err != nil {
		return nil, err
	}
	terminalValue, err := ValueCrossVenueTerminalState(r.Report, replay, accounts, CrossVenueTerminalConvention{
		Venues: spec.Venues, Clients: spec.Clients, HorizonNano: spec.HorizonNano,
		MaxBookEvidenceAgeNanos: spec.MaxBookEvidenceAgeNanos,
		BasePrecision:           spec.BasePrecision, TakerFeeBps: spec.TakerFeeBps,
	})
	if err != nil {
		return nil, err
	}
	return &CrossVenueFirstAttemptEvidence{
		Counters: counters, Timeline: timeline, Groups: groups, Placements: placementResults,
		Fills: fills, Terminal: terminal, Accounts: accounts, Movements: movements,
		Funding: funding, TerminalValue: terminalValue, PublicReplay: replay,
	}, nil
}

func selectCrossVenueFirstAttemptDecisions(spec CrossVenueFirstAttemptSpec, evaluations []CrossVenueEvaluationRecord) ([]SelectedDecisionFrontierVector, error) {
	links := make(map[uint32]uint64, 2)
	for _, evaluation := range evaluations {
		for _, feed := range evaluation.Payload.Feeds {
			if spec.Clients[feed.VenueID] != feed.ClientID {
				return nil, fmt.Errorf("cross-venue first-attempt world: feed client disagrees with registered venue account")
			}
			if feed.Frontier.LinkID == 0 {
				continue
			}
			if prior := links[feed.Frontier.LinkID]; prior != 0 && prior != feed.ClientID {
				return nil, fmt.Errorf("cross-venue first-attempt world: link changes client owner")
			}
			links[feed.Frontier.LinkID] = feed.ClientID
		}
	}
	if len(links) == 0 {
		audit, err := AuditDecisionFrontierVectors(spec.EvidenceDir)
		if err != nil || audit == nil || !audit.Valid {
			return nil, fmt.Errorf("cross-venue first-attempt world: vector audit unavailable or invalid: %v", err)
		}
		return nil, nil
	}
	return SelectVerifiedDecisionFrontierVectors(spec.EvidenceDir, DecisionFrontierVectorSelection{
		Symbol: spec.Symbol, ClientByLink: links,
	})
}
