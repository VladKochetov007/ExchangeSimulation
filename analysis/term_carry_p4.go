package analysis

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"

	etypes "exchange_sim/types"
)

const (
	p4BasisPreNanos       = int64(30_000_000_000)
	p4BasisPostNanos      = int64(300_000_000_000)
	p4BasisMaxAgeNanos    = int64(2_000_000_000)
	p4BasisMinPreSamples  = 24
	p4BasisMinPostSamples = 240
	p4AnalysisCutoff      = int64(1736038805000000000)
)

// TermCarryP4Chain is the independently reconstructed P4 causal chain for one
// evidence artifact. Actor-emitted financial fields are retained only after
// MeasureTermCarry has recomputed and validated them from delivered receipts.
type TermCarryP4Chain struct {
	FundingCapBps       int64                  `json:"funding_cap_bps"`
	BaseAudit           *TermCarryAudit        `json:"base_audit"`
	DecisionsEvaluated  int64                  `json:"exact_cost_decisions_evaluated"`
	BelowMinimum        int64                  `json:"below_minimum_decisions"`
	SubmittedCandidates []TermCarryP4Candidate `json:"submitted_candidates"`
	Checks              []string               `json:"checks,omitempty"`
	Valid               bool                   `json:"valid"`
}

// TermCarryP4Candidate is one exact-cost entry decision and its independently
// linked ordinary execution. A submitted target is not an executed position.
type TermCarryP4Candidate struct {
	VenueID             string `json:"venue_id"`
	ClientID            uint64 `json:"client_id"`
	DecisionTime        int64  `json:"decision_time"`
	Action              string `json:"action"`
	FundingRateBps      int64  `json:"funding_rate_bps"`
	FundingPublishedAt  int64  `json:"funding_published_at"`
	FundingSequence     uint64 `json:"funding_sequence"`
	SpotBid             int64  `json:"spot_bid"`
	SpotAsk             int64  `json:"spot_ask"`
	PerpBid             int64  `json:"perp_bid"`
	PerpAsk             int64  `json:"perp_ask"`
	Direction           int64  `json:"direction"`
	ExpectedFundingBps  string `json:"expected_funding_bps"`
	ExecutionFeeBps     string `json:"execution_fee_bps"`
	FinancingNumerator  string `json:"financing_bps_numerator"`
	FixedCostBps        int64  `json:"fixed_cost_bps"`
	NetCarryNumerator   string `json:"net_carry_bps_numerator"`
	RationalDenominator string `json:"rational_denominator"`
	MinimumNumerator    string `json:"minimum_net_return_numerator"`
	Eligible            bool   `json:"eligible"`
	TargetSpot          int64  `json:"target_spot"`
	TargetPerp          int64  `json:"target_perp"`
	SpotRequestID       uint64 `json:"spot_request_id"`
	SpotAccepted        bool   `json:"spot_accepted"`
	SpotFilledQty       int64  `json:"spot_filled_qty"`
	PerpRequestID       uint64 `json:"perp_request_id"`
	PerpAccepted        bool   `json:"perp_accepted"`
	PerpFilledQty       int64  `json:"perp_filled_qty"`
	MatchedExposureQty  int64  `json:"matched_exposure_qty"`
	ExecutionQualified  bool   `json:"execution_qualified"`
}

// TermCarryP4Pair is the registered same-seed A/B identification result. It
// does not assign a cross-seed verdict; the campaign scorer does that only
// after both development pairs exist.
type TermCarryP4Pair struct {
	ControlCapBps   int64                  `json:"control_cap_bps"`
	TreatmentCapBps int64                  `json:"treatment_cap_bps"`
	ControlValid    bool                   `json:"control_valid"`
	TreatmentValid  bool                   `json:"treatment_valid"`
	Venues          []TermCarryP4VenuePair `json:"venues"`
	Checks          []string               `json:"checks,omitempty"`
	ActivationValid bool                   `json:"activation_valid"`
	ExecutionValid  bool                   `json:"execution_valid"`
	BasisMeasurable bool                   `json:"basis_measurable"`
	SeedStatistic   *float64               `json:"seed_statistic,omitempty"`
	Valid           bool                   `json:"valid"`
}

// TermCarryP4VenuePair preserves every registered link and basis component.
type TermCarryP4VenuePair struct {
	VenueID                   string                 `json:"venue_id"`
	DecisionTime              int64                  `json:"decision_time"`
	Control                   *TermCarryP4Candidate  `json:"control,omitempty"`
	Treatment                 *TermCarryP4Candidate  `json:"treatment,omitempty"`
	LocalInputsComparable     bool                   `json:"local_inputs_comparable"`
	FundingChangedAsPredicted bool                   `json:"funding_changed_as_predicted"`
	ExactCarryCrossed         bool                   `json:"exact_carry_crossed"`
	TargetChangedAsPredicted  bool                   `json:"target_changed_as_predicted"`
	ExecutionQualified        bool                   `json:"execution_qualified"`
	ControlBasis              TermCarryP4BasisWindow `json:"control_basis"`
	TreatmentBasis            TermCarryP4BasisWindow `json:"treatment_basis"`
	PairedConvergence         *float64               `json:"paired_convergence,omitempty"`
}

// TermCarryP4BasisWindow is the sole preregistered 30-second/300-second event
// study. Missing samples remain missing; they are never numeric zero.
type TermCarryP4BasisWindow struct {
	PreSamples       int      `json:"pre_samples"`
	PostSamples      int      `json:"post_samples"`
	PreMeanBps       *float64 `json:"pre_mean_oriented_premium_bps,omitempty"`
	PostMeanBps      *float64 `json:"post_mean_oriented_premium_bps,omitempty"`
	DeltaBps         *float64 `json:"delta_oriented_premium_bps,omitempty"`
	Measurable       bool     `json:"measurable"`
	CensoredByCutoff bool     `json:"censored_by_cutoff"`
}

type p4Manifest struct {
	Config struct {
		FundingMaxRateBps int64 `json:"funding_max_rate_bps"`
	} `json:"config"`
}

func decodeManifest(dir string, target any) error {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("read P4 manifest: %w", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode P4 manifest: %w", err)
	}
	return nil
}

// MeasureTermCarryP4Chain exposes P4 links 1--5 only after the existing
// independent evidence replay passes in full.
func (r *Run) MeasureTermCarryP4Chain() (*TermCarryP4Chain, error) {
	base, err := r.MeasureTermCarry()
	if err != nil {
		return nil, err
	}
	policy, err := loadTermCarryPolicy(r.Dir)
	if err != nil {
		return nil, err
	}
	var manifest p4Manifest
	if err := decodeManifest(r.Dir, &manifest); err != nil {
		return nil, err
	}
	result := &TermCarryP4Chain{FundingCapBps: manifest.Config.FundingMaxRateBps, BaseAudit: base}
	if !base.Valid {
		result.Checks = append(result.Checks, "base_term_carry_audit_failed")
		return result, nil
	}

	var decisions []termCarryDecision
	var outcomes []termCarryOutcome
	if err := r.Scan(ScanOptions{Events: []string{"term_carry_decision", "term_carry_leg_outcome"}, Workers: 1}, func(event Event) {
		switch event.Name {
		case "term_carry_decision":
			var decision termCarryDecision
			if event.Decode(&decision) == nil {
				decisions = append(decisions, decision)
			}
		case "term_carry_leg_outcome":
			var outcome termCarryOutcome
			if event.Decode(&outcome) == nil {
				outcomes = append(outcomes, outcome)
			}
		}
	}); err != nil {
		return nil, fmt.Errorf("P4 chain scan: %w", err)
	}
	sort.Slice(decisions, func(i, j int) bool {
		if decisions[i].DecisionTime != decisions[j].DecisionTime {
			return decisions[i].DecisionTime < decisions[j].DecisionTime
		}
		if decisions[i].VenueID != decisions[j].VenueID {
			return decisions[i].VenueID < decisions[j].VenueID
		}
		return decisions[i].ClientID < decisions[j].ClientID
	})

	for _, decision := range decisions {
		if decision.Action != "NET_CARRY_BELOW_MINIMUM" && decision.Action != "SUBMIT_ENTRY_SPOT_IOC" {
			continue
		}
		financials, ok := p4CandidateFinancials(policy, decision)
		if !ok {
			result.Checks = append(result.Checks, fmt.Sprintf("%s/%d/%d: independent_financial_recompute_failed", decision.VenueID, decision.ClientID, decision.DecisionTime))
			continue
		}
		result.DecisionsEvaluated++
		candidate := TermCarryP4Candidate{
			VenueID: decision.VenueID, ClientID: decision.ClientID, DecisionTime: decision.DecisionTime,
			Action: decision.Action, FundingRateBps: decision.FundingRateBps,
			FundingPublishedAt: decision.FundingPublishedAt, FundingSequence: decision.FundingSequence,
			SpotBid: decision.SpotBid, SpotAsk: decision.SpotAsk, PerpBid: decision.PerpBid, PerpAsk: decision.PerpAsk,
			Direction: financials.direction, ExpectedFundingBps: financials.funding.String(),
			ExecutionFeeBps: financials.fees.String(), FinancingNumerator: financials.financing.String(),
			FixedCostBps:      policy.BalanceSheetBps + policy.MarginRiskBps + policy.LegRiskBps,
			NetCarryNumerator: financials.net.String(), RationalDenominator: financials.denominator.String(),
			MinimumNumerator: financials.minimum.String(), Eligible: financials.eligible,
			TargetSpot: decision.TargetSpot, TargetPerp: decision.TargetPerp,
			SpotRequestID: decision.RequestID,
		}
		if decision.Action == "SUBMIT_ENTRY_SPOT_IOC" {
			populateP4Execution(&candidate, decision, decisions, outcomes, policy.LotQty)
			result.SubmittedCandidates = append(result.SubmittedCandidates, candidate)
		} else {
			result.BelowMinimum++
		}
	}
	if result.DecisionsEvaluated == 0 {
		result.Checks = append(result.Checks, "no_exact_cost_candidates")
	}
	result.Valid = len(result.Checks) == 0
	return result, nil
}

type p4Financials struct {
	funding, fees, financing, net, denominator, minimum *big.Int
	direction                                           int64
	eligible                                            bool
}

func p4CandidateFinancials(policy termCarryPolicyConfig, decision termCarryDecision) (p4Financials, bool) {
	if decision.SpotBid <= 0 || decision.SpotAsk <= 0 || decision.PerpBid <= 0 || decision.PerpAsk <= 0 || decision.SpotBid > decision.SpotAsk || decision.PerpBid > decision.PerpAsk {
		return p4Financials{}, false
	}
	spot := etypes.Midpoint(decision.SpotBid, decision.SpotAsk)
	perp := etypes.Midpoint(decision.PerpBid, decision.PerpAsk)
	if spot == perp {
		return p4Financials{}, false
	}
	direction := int64(1)
	if perp < spot {
		direction = -1
	}
	financials, ok := termCarryAuditFinancials(policy, decision, direction)
	if !ok {
		return p4Financials{}, false
	}
	minimum := new(big.Int).Mul(big.NewInt(policy.MinNetCarryBps), financials.denominator)
	return p4Financials{
		funding: new(big.Int).Set(financials.funding), fees: new(big.Int).Set(financials.fees),
		financing: new(big.Int).Set(financials.financing), net: new(big.Int).Set(financials.net),
		denominator: new(big.Int).Set(financials.denominator), minimum: minimum,
		direction: direction, eligible: financials.net.Cmp(minimum) >= 0,
	}, true
}

func populateP4Execution(candidate *TermCarryP4Candidate, entry termCarryDecision, decisions []termCarryDecision, outcomes []termCarryOutcome, lot int64) {
	for _, outcome := range outcomes {
		if outcome.VenueID != entry.VenueID || outcome.ClientID != entry.ClientID || outcome.RequestID != entry.RequestID {
			continue
		}
		if outcome.Event == "ORDER_ACCEPTED" {
			candidate.SpotAccepted = true
		}
		if outcome.Event == "ORDER_FILL" && outcome.Leg == "ENTRY_SPOT_IOC" {
			candidate.SpotFilledQty += outcome.Qty
		}
	}
	for _, decision := range decisions {
		if decision.VenueID != entry.VenueID || decision.ClientID != entry.ClientID || decision.Action != "SUBMIT_ENTRY_PERP_IOC" || decision.PlanCreatedAt != entry.PlanCreatedAt || decision.DecisionTime < entry.DecisionTime {
			continue
		}
		candidate.PerpRequestID = decision.RequestID
		for _, outcome := range outcomes {
			if outcome.VenueID != decision.VenueID || outcome.ClientID != decision.ClientID || outcome.RequestID != decision.RequestID {
				continue
			}
			if outcome.Event == "ORDER_ACCEPTED" {
				candidate.PerpAccepted = true
			}
			if outcome.Event == "ORDER_FILL" && outcome.Leg == "ENTRY_PERP_IOC" {
				candidate.PerpFilledQty += outcome.Qty
			}
		}
		break
	}
	candidate.MatchedExposureQty = min64(candidate.SpotFilledQty, candidate.PerpFilledQty)
	candidate.ExecutionQualified = candidate.SpotAccepted && candidate.PerpAccepted && candidate.MatchedExposureQty >= lot
}

// MeasureTermCarryP4Pair applies the treatment clock to both arms and computes
// the sole frozen basis statistic. control and treatment must be the same seed.
func MeasureTermCarryP4Pair(control, treatment *Run) (*TermCarryP4Pair, error) {
	a, err := control.MeasureTermCarryP4Chain()
	if err != nil {
		return nil, fmt.Errorf("control P4 chain: %w", err)
	}
	b, err := treatment.MeasureTermCarryP4Chain()
	if err != nil {
		return nil, fmt.Errorf("treatment P4 chain: %w", err)
	}
	result := &TermCarryP4Pair{ControlCapBps: a.FundingCapBps, TreatmentCapBps: b.FundingCapBps, ControlValid: a.Valid, TreatmentValid: b.Valid}
	if !a.Valid || !b.Valid {
		result.Checks = append(result.Checks, "invalid_arm_evidence_chain")
		return result, nil
	}
	if a.FundingCapBps != 1 || b.FundingCapBps != 75 {
		result.Checks = append(result.Checks, "unregistered_funding_caps")
		return result, nil
	}

	keys := make(map[p4CandidateKey]struct{}, len(b.SubmittedCandidates))
	for i := range b.SubmittedCandidates {
		candidate := &b.SubmittedCandidates[i]
		keys[p4CandidateKey{venue: candidate.VenueID, at: candidate.DecisionTime}] = struct{}{}
	}
	controlAt, err := loadP4CandidatesAt(control, keys)
	if err != nil {
		return nil, err
	}
	for i := range b.SubmittedCandidates {
		bc := &b.SubmittedCandidates[i]
		if bc.Action != "SUBMIT_ENTRY_SPOT_IOC" {
			continue
		}
		ac := controlAt[p4CandidateKey{venue: bc.VenueID, at: bc.DecisionTime}]
		venue := TermCarryP4VenuePair{VenueID: bc.VenueID, DecisionTime: bc.DecisionTime, Control: ac, Treatment: bc}
		if ac != nil {
			venue.LocalInputsComparable, venue.FundingChangedAsPredicted, venue.ExactCarryCrossed, venue.TargetChangedAsPredicted = evaluateP4Links(*ac, *bc)
		}
		venue.ExecutionQualified = bc.ExecutionQualified
		if venue.LocalInputsComparable && venue.FundingChangedAsPredicted && venue.ExactCarryCrossed && venue.TargetChangedAsPredicted && venue.ExecutionQualified {
			venue.ControlBasis, err = measureP4BasisWindow(control, bc.VenueID, bc.DecisionTime, bc.Direction)
			if err != nil {
				return nil, err
			}
			venue.TreatmentBasis, err = measureP4BasisWindow(treatment, bc.VenueID, bc.DecisionTime, bc.Direction)
			if err != nil {
				return nil, err
			}
			if venue.ControlBasis.Measurable && venue.TreatmentBasis.Measurable {
				value := *venue.ControlBasis.DeltaBps - *venue.TreatmentBasis.DeltaBps
				venue.PairedConvergence = &value
			}
		}
		result.Venues = append(result.Venues, venue)
	}
	if len(result.Venues) == 0 {
		result.Checks = append(result.Checks, "no_treatment_target_crossing")
	}
	result.ActivationValid = len(result.Venues) > 0
	result.ExecutionValid = len(result.Venues) > 0
	var values []float64
	for _, venue := range result.Venues {
		result.ActivationValid = result.ActivationValid && venue.LocalInputsComparable && venue.FundingChangedAsPredicted && venue.ExactCarryCrossed && venue.TargetChangedAsPredicted
		result.ExecutionValid = result.ExecutionValid && venue.ExecutionQualified
		if venue.PairedConvergence != nil {
			values = append(values, *venue.PairedConvergence)
		}
	}
	result.BasisMeasurable = result.ExecutionValid && len(values) == len(result.Venues)
	if result.BasisMeasurable {
		mean := meanFloat64(values)
		result.SeedStatistic = &mean
	}
	result.Valid = len(result.Checks) == 0 && result.ActivationValid && result.ExecutionValid && result.BasisMeasurable
	return result, nil
}

type p4CandidateKey struct {
	venue string
	at    int64
}

func loadP4CandidatesAt(run *Run, keys map[p4CandidateKey]struct{}) (map[p4CandidateKey]*TermCarryP4Candidate, error) {
	policy, err := loadTermCarryPolicy(run.Dir)
	if err != nil {
		return nil, err
	}
	result := make(map[p4CandidateKey]*TermCarryP4Candidate, len(keys))
	if err := run.Scan(ScanOptions{Events: []string{"term_carry_decision"}, Workers: 1}, func(event Event) {
		key := p4CandidateKey{venue: event.VenueID, at: event.SimTS}
		if _, wanted := keys[key]; !wanted {
			return
		}
		var decision termCarryDecision
		if event.Decode(&decision) != nil || (decision.Action != "NET_CARRY_BELOW_MINIMUM" && decision.Action != "SUBMIT_ENTRY_SPOT_IOC") {
			return
		}
		financials, ok := p4CandidateFinancials(policy, decision)
		if !ok {
			return
		}
		candidate := &TermCarryP4Candidate{
			VenueID: decision.VenueID, ClientID: decision.ClientID, DecisionTime: decision.DecisionTime,
			Action: decision.Action, FundingRateBps: decision.FundingRateBps,
			FundingPublishedAt: decision.FundingPublishedAt, FundingSequence: decision.FundingSequence,
			SpotBid: decision.SpotBid, SpotAsk: decision.SpotAsk, PerpBid: decision.PerpBid, PerpAsk: decision.PerpAsk,
			Direction: financials.direction, ExpectedFundingBps: financials.funding.String(),
			ExecutionFeeBps: financials.fees.String(), FinancingNumerator: financials.financing.String(),
			FixedCostBps:      policy.BalanceSheetBps + policy.MarginRiskBps + policy.LegRiskBps,
			NetCarryNumerator: financials.net.String(), RationalDenominator: financials.denominator.String(),
			MinimumNumerator: financials.minimum.String(), Eligible: financials.eligible,
			TargetSpot: decision.TargetSpot, TargetPerp: decision.TargetPerp,
		}
		if _, duplicate := result[key]; duplicate {
			result[key] = nil
			return
		}
		result[key] = candidate
	}); err != nil {
		return nil, fmt.Errorf("control P4 decision scan: %w", err)
	}
	return result, nil
}

func p4ComparableInputs(a, b TermCarryP4Candidate) bool {
	return a.VenueID == b.VenueID && a.DecisionTime == b.DecisionTime && a.Direction == b.Direction &&
		a.FundingPublishedAt == b.FundingPublishedAt && a.FundingSequence == b.FundingSequence &&
		a.SpotBid == b.SpotBid && a.SpotAsk == b.SpotAsk && a.PerpBid == b.PerpBid && a.PerpAsk == b.PerpAsk
}

func evaluateP4Links(a, b TermCarryP4Candidate) (inputs, funding, carry, target bool) {
	inputs = p4ComparableInputs(a, b)
	funding = b.Direction == a.Direction && compareDecimal(b.ExpectedFundingBps, a.ExpectedFundingBps) > 0
	carry = !a.Eligible && b.Eligible
	target = a.TargetSpot == 0 && a.TargetPerp == 0 && b.TargetSpot == b.Direction*100_000_000 && b.TargetPerp == -b.TargetSpot
	return
}

func compareDecimal(a, b string) int {
	x, xOK := new(big.Int).SetString(a, 10)
	y, yOK := new(big.Int).SetString(b, 10)
	if !xOK || !yOK {
		return 0
	}
	return x.Cmp(y)
}

type p4Quote struct {
	at      int64
	ordinal int64
	mid     int64
}

func measureP4BasisWindow(run *Run, venue string, t0, direction int64) (TermCarryP4BasisWindow, error) {
	result := TermCarryP4BasisWindow{CensoredByCutoff: t0+p4BasisPostNanos > p4AnalysisCutoff}
	if result.CensoredByCutoff || direction == 0 {
		return result, nil
	}
	series := map[string][]p4Quote{"ABC/USD": {}, "ABC-PERP": {}}
	var mu sync.Mutex
	if err := run.Scan(ScanOptions{Events: []string{"BookSnapshot"}}, func(event Event) {
		if event.VenueID != venue || !isPeriodicSnapshot(event) || event.SimTS < t0-p4BasisPreNanos-p4BasisMaxAgeNanos || event.SimTS >= t0+p4BasisPostNanos {
			return
		}
		var envelope bookSnapshotEnvelope
		if event.Decode(&envelope) != nil {
			return
		}
		symbol := event.Symbol
		if symbol == "" {
			symbol = envelope.Symbol
		}
		if symbol == "" {
			symbol = symbolFromPath(event.File)
		}
		if symbol != "ABC/USD" && symbol != "ABC-PERP" {
			return
		}
		bids, asks := envelope.levels()
		bid, _, bidOK := bestWithDepth(bids, true)
		ask, _, askOK := bestWithDepth(asks, false)
		if !bidOK || !askOK || bid > ask {
			return
		}
		quote := p4Quote{at: event.SimTS, ordinal: event.Ordinal, mid: etypes.Midpoint(bid, ask)}
		mu.Lock()
		series[symbol] = append(series[symbol], quote)
		mu.Unlock()
	}); err != nil {
		return result, fmt.Errorf("P4 basis scan: %w", err)
	}
	for symbol := range series {
		sort.Slice(series[symbol], func(i, j int) bool {
			if series[symbol][i].at != series[symbol][j].at {
				return series[symbol][i].at < series[symbol][j].at
			}
			return series[symbol][i].ordinal < series[symbol][j].ordinal
		})
	}
	pre := sampleP4Premium(series, t0-p4BasisPreNanos, t0, direction)
	post := sampleP4Premium(series, t0, t0+p4BasisPostNanos, direction)
	result.PreSamples, result.PostSamples = len(pre), len(post)
	if len(pre) < p4BasisMinPreSamples || len(post) < p4BasisMinPostSamples {
		return result, nil
	}
	preMean, postMean := meanFloat64(pre), meanFloat64(post)
	delta := postMean - preMean
	result.PreMeanBps, result.PostMeanBps, result.DeltaBps = &preMean, &postMean, &delta
	result.Measurable = true
	return result, nil
}

func sampleP4Premium(series map[string][]p4Quote, start, end, direction int64) []float64 {
	first := ceilSecond(start)
	spotIndex, perpIndex := 0, 0
	var spot, perp p4Quote
	spotFound, perpFound := false, false
	values := make([]float64, 0, int((end-start)/1_000_000_000)+1)
	for at := first; at < end; at += 1_000_000_000 {
		spotIndex, spot, spotFound = advanceP4Quote(series["ABC/USD"], spotIndex, at, spot, spotFound)
		perpIndex, perp, perpFound = advanceP4Quote(series["ABC-PERP"], perpIndex, at, perp, perpFound)
		if !spotFound || !perpFound || at-spot.at > p4BasisMaxAgeNanos || at-perp.at > p4BasisMaxAgeNanos || spot.mid <= 0 {
			continue
		}
		premium := 10_000 * (float64(perp.mid) - float64(spot.mid)) / float64(spot.mid)
		if math.IsNaN(premium) || math.IsInf(premium, 0) {
			continue
		}
		values = append(values, float64(direction)*premium)
	}
	return values
}

func advanceP4Quote(quotes []p4Quote, index int, at int64, current p4Quote, found bool) (int, p4Quote, bool) {
	for index < len(quotes) && quotes[index].at <= at {
		current, found = quotes[index], true
		index++
	}
	return index, current, found
}

func ceilSecond(value int64) int64 {
	const second = int64(1_000_000_000)
	quotient, remainder := value/second, value%second
	if remainder > 0 {
		quotient++
	}
	return quotient * second
}

func meanFloat64(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
