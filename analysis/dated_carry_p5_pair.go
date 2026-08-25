package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"sync"

	etypes "exchange_sim/types"
)

const (
	p5BasisMinuteNanos = int64(60 * 1_000_000_000)
	p5BasisMaxAgeNanos = int64(2 * 1_000_000_000)
	p5BasisTailNanos   = int64(5 * 60 * 1_000_000_000)
)

// DatedCarryP5Pair is one preregistered same-seed shadow/active causal result.
// It never assigns the cross-seed verdict; both development pairs must exist
// before that classification is licensed.
type DatedCarryP5Pair struct {
	StructuralConfigValid bool                   `json:"structural_config_valid"`
	SameSourceRevision    bool                   `json:"same_source_revision"`
	ControlAudit          *DatedCarryP5Audit     `json:"control_audit"`
	TreatmentAudit        *DatedCarryP5Audit     `json:"treatment_audit"`
	ControlMandate        *DatedMandateP5Audit   `json:"control_mandate_audit"`
	TreatmentMandate      *DatedMandateP5Audit   `json:"treatment_mandate_audit"`
	Terms                 []DatedCarryP5TermPair `json:"terms"`
	QualifiedTerms        int                    `json:"qualified_terms"`
	QualifiedByVenue      map[string]int         `json:"qualified_by_venue"`
	ActivationValid       bool                   `json:"activation_valid"`
	ExecutionValid        bool                   `json:"execution_valid"`
	LifecycleValid        bool                   `json:"lifecycle_valid"`
	BasisMeasurable       bool                   `json:"basis_measurable"`
	SeedStatistic         *float64               `json:"seed_statistic,omitempty"`
	SeedStatisticExact    string                 `json:"seed_statistic_exact,omitempty"`
	SeedSign              int                    `json:"seed_statistic_sign"`
	Checks                []string               `json:"checks,omitempty"`
	Valid                 bool                   `json:"valid"`
}

type DatedCarryP5TermPair struct {
	VenueID                  string                  `json:"venue_id"`
	FutureSymbol             string                  `json:"future_symbol"`
	ListedNano               int64                   `json:"listed_nano"`
	ExpiryNano               int64                   `json:"expiry_nano"`
	TreatmentCandidateAt     int64                   `json:"treatment_candidate_at"`
	Direction                int64                   `json:"direction"`
	ControlEligibleSameClock bool                    `json:"control_eligible_same_clock"`
	ControlDirection         int64                   `json:"control_direction"`
	CostPolicyComparable     bool                    `json:"cost_policy_comparable"`
	TargetChanged            bool                    `json:"target_changed"`
	ExecutionQualified       bool                    `json:"execution_qualified"`
	LifecycleQualified       bool                    `json:"lifecycle_qualified"`
	ControlBasis             DatedCarryP5BasisWindow `json:"control_basis"`
	TreatmentBasis           DatedCarryP5BasisWindow `json:"treatment_basis"`
	ContractCompression      *float64                `json:"contract_compression_bps,omitempty"`
	ContractCompressionExact string                  `json:"contract_compression_bps_exact,omitempty"`
	ContractCompressionSign  int                     `json:"contract_compression_sign"`
	Qualified                bool                    `json:"qualified"`
	Failure                  string                  `json:"failure,omitempty"`
}

type DatedCarryP5BasisWindow struct {
	ExpectedSamples int      `json:"expected_samples"`
	ObservedSamples int      `json:"observed_samples"`
	Coverage        float64  `json:"coverage"`
	MeanBps         *float64 `json:"mean_oriented_basis_bps,omitempty"`
	MeanExact       string   `json:"mean_oriented_basis_bps_exact,omitempty"`
	Measurable      bool     `json:"measurable"`
	UndefinedSpot   int      `json:"undefined_spot_denominator"`
	StaleOrMissing  int      `json:"stale_or_missing"`
}

type p5BasisKey struct{ venue, symbol string }

func p5ComparableConfigs(controlDir, treatmentDir string) (bool, bool, error) {
	type manifestRecord struct {
		Build struct {
			Revision string `json:"revision"`
		} `json:"build"`
		Config map[string]any `json:"config"`
	}
	read := func(dir string) (manifestRecord, error) {
		raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			return manifestRecord{}, err
		}
		var manifest manifestRecord
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return manifestRecord{}, err
		}
		return manifest, nil
	}
	manifestA, err := read(controlDir)
	if err != nil {
		return false, false, err
	}
	manifestB, err := read(treatmentDir)
	if err != nil {
		return false, false, err
	}
	sameRevision := manifestA.Build.Revision != "" && manifestA.Build.Revision == manifestB.Build.Revision
	a, b := manifestA.Config, manifestB.Config
	seedA, seedB := a["seed"], b["seed"]
	if seedA != seedB {
		return false, sameRevision, nil
	}
	carryA, okA := a["dated_term_carry_allocator"].(map[string]any)
	carryB, okB := b["dated_term_carry_allocator"].(map[string]any)
	if !okA || !okB || carryA["trade_enabled"] != false || carryB["trade_enabled"] != true {
		return false, sameRevision, nil
	}
	delete(carryA, "trade_enabled")
	delete(carryB, "trade_enabled")
	for _, config := range []map[string]any{a, b} {
		delete(config, "experiment_id")
		delete(config, "description")
	}
	rawA, err := json.Marshal(a)
	if err != nil {
		return false, sameRevision, err
	}
	rawB, err := json.Marshal(b)
	if err != nil {
		return false, sameRevision, err
	}
	return bytes.Equal(rawA, rawB), sameRevision, nil
}

func p5ControlCandidateAt(run *Run, policy datedCarryP5Config, treatment DatedCarryP5Term) (datedCarryP5Decision, p5Financials, bool, error) {
	var matches []datedCarryP5Decision
	if err := run.Scan(ScanOptions{Events: []string{"dated_term_carry_decision"}, Workers: 1}, func(event Event) {
		if event.VenueID != treatment.VenueID {
			return
		}
		var decision datedCarryP5Decision
		if event.Decode(&decision) != nil || decision.FutureSymbol != treatment.FutureSymbol || decision.ListedNano != treatment.ListedNano || decision.ExpiryNano != treatment.ExpiryNano || decision.DecisionTime != treatment.TargetChangedAt {
			return
		}
		matches = append(matches, decision)
	}); err != nil {
		return datedCarryP5Decision{}, p5Financials{}, false, err
	}
	if len(matches) != 1 || matches[0].Action != "SHADOW_ELIGIBLE" {
		return datedCarryP5Decision{}, p5Financials{}, false, nil
	}
	financials, err := recomputeP5Financials(policy, matches[0])
	if err != nil || !financials.eligible {
		return matches[0], financials, false, nil
	}
	return matches[0], financials, true, nil
}

func p5LoadBasisSeries(run *Run, symbols map[p5BasisKey]bool) (map[p5BasisKey][]p4Quote, error) {
	series := make(map[p5BasisKey][]p4Quote, len(symbols))
	var mu sync.Mutex
	if err := run.Scan(ScanOptions{Events: []string{"BookSnapshot"}}, func(event Event) {
		if !isPeriodicSnapshot(event) {
			return
		}
		var snapshot bookSnapshotEnvelope
		if event.Decode(&snapshot) != nil {
			return
		}
		symbol := event.Symbol
		if symbol == "" {
			symbol = snapshot.Symbol
		}
		if symbol == "" {
			symbol = symbolFromSpotFile(event.File)
		}
		key := p5BasisKey{event.VenueID, symbol}
		if !symbols[key] {
			return
		}
		bids, asks := snapshot.levels()
		bid, _, bidOK := bestWithDepth(bids, true)
		ask, _, askOK := bestWithDepth(asks, false)
		if !bidOK || !askOK || bid > ask {
			return
		}
		quote := p4Quote{at: event.SimTS, ordinal: event.Ordinal, mid: etypes.Midpoint(bid, ask)}
		mu.Lock()
		series[key] = append(series[key], quote)
		mu.Unlock()
	}); err != nil {
		return nil, err
	}
	for key := range series {
		sort.Slice(series[key], func(i, j int) bool {
			if series[key][i].at != series[key][j].at {
				return series[key][i].at < series[key][j].at
			}
			return series[key][i].ordinal < series[key][j].ordinal
		})
	}
	return series, nil
}

func p5CeilMinute(at int64) int64 {
	q, r := at/p5BasisMinuteNanos, at%p5BasisMinuteNanos
	if r > 0 {
		q++
	}
	return q * p5BasisMinuteNanos
}

func measureP5BasisWindow(series map[p5BasisKey][]p4Quote, venue, future string, t0, expiry, direction int64) DatedCarryP5BasisWindow {
	result := DatedCarryP5BasisWindow{}
	start, end := p5CeilMinute(t0), expiry-p5BasisTailNanos
	if direction == 0 || start >= end {
		return result
	}
	spotQuotes, futureQuotes := series[p5BasisKey{venue, "ABC/USD"}], series[p5BasisKey{venue, future}]
	spotIndex, futureIndex := 0, 0
	var spot, dated p4Quote
	spotFound, futureFound := false, false
	values := make([]*big.Rat, 0, int((end-start)/p5BasisMinuteNanos)+1)
	for at := start; at < end; at += p5BasisMinuteNanos {
		result.ExpectedSamples++
		spotIndex, spot, spotFound = advanceP4Quote(spotQuotes, spotIndex, at, spot, spotFound)
		futureIndex, dated, futureFound = advanceP4Quote(futureQuotes, futureIndex, at, dated, futureFound)
		if !spotFound || !futureFound || at-spot.at > p5BasisMaxAgeNanos || at-dated.at > p5BasisMaxAgeNanos {
			result.StaleOrMissing++
			continue
		}
		if spot.mid <= 0 {
			result.UndefinedSpot++
			continue
		}
		numerator := new(big.Int).Sub(big.NewInt(dated.mid), big.NewInt(spot.mid))
		numerator.Mul(numerator, big.NewInt(10_000*direction))
		values = append(values, new(big.Rat).SetFrac(numerator, big.NewInt(spot.mid)))
	}
	result.ObservedSamples = len(values)
	if result.ExpectedSamples > 0 {
		result.Coverage = float64(result.ObservedSamples) / float64(result.ExpectedSamples)
	}
	if result.ExpectedSamples == 0 || result.ObservedSamples*10 < result.ExpectedSamples*9 {
		return result
	}
	mean := meanRat(values)
	value, _ := mean.Float64()
	result.MeanBps, result.MeanExact, result.Measurable = &value, mean.RatString(), true
	return result
}

func MeasureDatedCarryP5Pair(control, treatment *Run) (*DatedCarryP5Pair, error) {
	result := &DatedCarryP5Pair{QualifiedByVenue: make(map[string]int)}
	comparable, sameRevision, err := p5ComparableConfigs(control.Dir, treatment.Dir)
	if err != nil {
		return nil, err
	}
	result.StructuralConfigValid = comparable
	result.SameSourceRevision = sameRevision
	if !comparable {
		result.Checks = append(result.Checks, "config_delta_not_trade_permission_only")
	}
	if !sameRevision {
		result.Checks = append(result.Checks, "source_revision_missing_or_different")
	}
	result.ControlAudit, err = control.MeasureDatedCarryP5()
	if err != nil {
		return nil, err
	}
	result.TreatmentAudit, err = treatment.MeasureDatedCarryP5()
	if err != nil {
		return nil, err
	}
	result.ControlMandate, err = control.MeasureDatedMandateP5()
	if err != nil {
		return nil, err
	}
	result.TreatmentMandate, err = treatment.MeasureDatedMandateP5()
	if err != nil {
		return nil, err
	}
	controlManifest, err := loadDatedCarryP5Manifest(control.Dir)
	if err != nil {
		return nil, err
	}
	if result.ControlAudit.TradeEnabled || !result.TreatmentAudit.TradeEnabled {
		result.Checks = append(result.Checks, "arm_authority_reversed")
	}

	symbols := make(map[p5BasisKey]bool)
	for _, term := range result.TreatmentAudit.Terms {
		symbols[p5BasisKey{term.VenueID, "ABC/USD"}] = true
		symbols[p5BasisKey{term.VenueID, term.FutureSymbol}] = true
	}
	controlSeries, err := p5LoadBasisSeries(control, symbols)
	if err != nil {
		return nil, fmt.Errorf("P5 control basis: %w", err)
	}
	treatmentSeries, err := p5LoadBasisSeries(treatment, symbols)
	if err != nil {
		return nil, fmt.Errorf("P5 treatment basis: %w", err)
	}
	allActivation, allExecution, allLifecycle, allBasis := true, true, true, true
	compressions := make([]*big.Rat, 0, len(result.TreatmentAudit.Terms))
	for _, term := range result.TreatmentAudit.Terms {
		pair := DatedCarryP5TermPair{VenueID: term.VenueID, FutureSymbol: term.FutureSymbol, ListedNano: term.ListedNano, ExpiryNano: term.ExpiryNano, TreatmentCandidateAt: term.TargetChangedAt, Direction: term.Direction}
		controlDecision, controlFinancials, controlOK, err := p5ControlCandidateAt(control, *controlManifest.Config.DatedTermCarry, term)
		if err != nil {
			return nil, err
		}
		pair.ControlEligibleSameClock = controlOK
		if controlOK {
			pair.ControlDirection = 1
			if controlFinancials.direction == "CHEAP_FUTURE" {
				pair.ControlDirection = -1
			}
		}
		pair.CostPolicyComparable = result.StructuralConfigValid
		pair.TargetChanged = term.TargetChangedAt > 0 && term.TargetSpot == term.Direction*controlManifest.Config.DatedTermCarry.MaxPosition && term.TargetFuture == -term.TargetSpot && controlDecision.TargetSpot == 0 && controlDecision.TargetFuture == 0
		pair.ExecutionQualified, pair.LifecycleQualified = term.ExecutionQualified, term.LifecycleQualified
		if !controlOK || pair.ControlDirection != pair.Direction {
			pair.Failure = "control_not_eligible_same_contract_clock_direction"
			allActivation = false
		}
		if !pair.TargetChanged {
			if pair.Failure == "" {
				pair.Failure = "target_not_changed"
			}
			allActivation = false
		}
		if !pair.ExecutionQualified {
			if pair.Failure == "" {
				pair.Failure = "matched_execution_missing"
			}
			allExecution = false
		}
		if !pair.LifecycleQualified {
			if pair.Failure == "" {
				pair.Failure = "settlement_or_real_close_missing"
			}
			allLifecycle = false
		}
		if pair.ControlEligibleSameClock && pair.ControlDirection == pair.Direction && pair.TargetChanged && pair.ExecutionQualified && pair.LifecycleQualified {
			pair.ControlBasis = measureP5BasisWindow(controlSeries, term.VenueID, term.FutureSymbol, term.TargetChangedAt, term.ExpiryNano, term.Direction)
			pair.TreatmentBasis = measureP5BasisWindow(treatmentSeries, term.VenueID, term.FutureSymbol, term.TargetChangedAt, term.ExpiryNano, term.Direction)
			if pair.ControlBasis.Measurable && pair.TreatmentBasis.Measurable {
				left, leftOK := new(big.Rat).SetString(pair.ControlBasis.MeanExact)
				right, rightOK := new(big.Rat).SetString(pair.TreatmentBasis.MeanExact)
				if leftOK && rightOK {
					compression := new(big.Rat).Sub(left, right)
					value, _ := compression.Float64()
					pair.ContractCompression, pair.ContractCompressionExact, pair.ContractCompressionSign = &value, compression.RatString(), compression.Sign()
					pair.Qualified = true
					compressions = append(compressions, compression)
					result.QualifiedTerms++
					result.QualifiedByVenue[term.VenueID]++
				}
			}
			if !pair.Qualified {
				pair.Failure = "basis_coverage_below_registered_90_percent"
				allBasis = false
			}
		} else {
			allBasis = false
		}
		result.Terms = append(result.Terms, pair)
	}
	result.ActivationValid = allActivation && len(result.Terms) > 0
	result.ExecutionValid = result.ActivationValid && allExecution
	result.LifecycleValid = result.ExecutionValid && allLifecycle
	venueGate := len(result.QualifiedByVenue) == 3
	for _, count := range result.QualifiedByVenue {
		if count < 2 {
			venueGate = false
		}
	}
	result.BasisMeasurable = result.LifecycleValid && allBasis && result.QualifiedTerms >= 6 && venueGate && result.QualifiedTerms == len(result.Terms)
	if result.BasisMeasurable {
		mean := meanRat(compressions)
		value, _ := mean.Float64()
		result.SeedStatistic, result.SeedStatisticExact, result.SeedSign = &value, mean.RatString(), mean.Sign()
	}
	if len(result.TreatmentAudit.Terms) == 0 {
		result.Checks = append(result.Checks, "no_treatment_eligible_terms")
	}
	if !venueGate {
		result.Checks = append(result.Checks, "fewer_than_two_qualified_terms_per_venue")
	}
	result.Valid = result.StructuralConfigValid && result.SameSourceRevision && result.ControlAudit.Valid && result.TreatmentAudit.Valid && result.ControlMandate.Valid && result.TreatmentMandate.Valid && result.BasisMeasurable
	return result, nil
}
