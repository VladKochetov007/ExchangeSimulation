package analysis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSV1DConfigTriadFilesAcceptsRegisteredConfigs(t *testing.T) {
	root := filepath.Join("..", "research", "configs", "v2-r2-sv1d-activation")
	triad, err := ValidateSV1DConfigTriadFiles(SV1DConfigPaths{
		Treatment: filepath.Join(root, "activation-659-treatment.json"),
		ModeOff:   filepath.Join(root, "activation-659-mode-off.json"),
		NoRoster:  filepath.Join(root, "activation-659-no-roster.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if triad.Treatment.ExperimentID != "v2-r2-sv1d-activation-659-treatment" || triad.ModeOff.ExperimentID != "v2-r2-sv1d-activation-659-mode-off" || triad.NoRoster.ExperimentID != "v2-r2-sv1d-activation-659-no-roster" {
		t.Fatalf("triad identities = %+v", triad)
	}
	raw, err := os.ReadFile(filepath.Join(root, "activation-659-treatment.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if triad.Treatment.ConfigSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("treatment config digest = %s; want %s", triad.Treatment.ConfigSHA256, hex.EncodeToString(digest[:]))
	}
}

func TestValidateSV1DConfigTriadRejectsEconomicOrArmDrift(t *testing.T) {
	root := filepath.Join("..", "research", "configs", "v2-r2-sv1d-activation")
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	treatment := read("activation-659-treatment.json")
	modeOff := read("activation-659-mode-off.json")
	noRoster := read("activation-659-no-roster.json")
	tests := []struct {
		name      string
		treatment []byte
		modeOff   []byte
		noRoster  []byte
	}{
		{
			name:      "treatment supplier economics",
			treatment: bytes.Replace(treatment, []byte(`"elasticity_per_percent": 12000000000`), []byte(`"elasticity_per_percent": 12000000001`), 1),
			modeOff:   modeOff,
			noRoster:  noRoster,
		},
		{
			name:      "common matching policy",
			treatment: treatment,
			modeOff:   bytes.Replace(modeOff, []byte(`"matching_rule": "price_time"`), []byte(`"matching_rule": "pro_rata"`), 1),
			noRoster:  noRoster,
		},
		{
			name:      "arm identity",
			treatment: bytes.Replace(treatment, []byte(`"experiment_id": "v2-r2-sv1d-activation-659-treatment"`), []byte(`"experiment_id": "different-arm"`), 1),
			modeOff:   modeOff,
			noRoster:  noRoster,
		},
		{
			name:      "duplicate JSON key",
			treatment: bytes.Replace(treatment, []byte(`"seed": 659`), []byte(`"seed": 659, "seed": 659`), 1),
			modeOff:   modeOff,
			noRoster:  noRoster,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateSV1DConfigTriad(test.treatment, test.modeOff, test.noRoster); err == nil {
				t.Fatal("config drift was accepted")
			}
		})
	}
}

func TestScoreSV1DProbeRejectsDurationOverflow(t *testing.T) {
	plan := testSV1DProbePlan()
	validArms := func() []SV1DProbeArmResult {
		return []SV1DProbeArmResult{
			testSV1DProbeArm(plan.Treatment, true, true, true, true, true, []CDFVenueConcentrationAudit{
				testSV1DVenue("north", 10, 0, 0, 10, 10, "two_sided"), testSV1DVenue("central", 10, 0, 0, 10, 10, "two_sided"), testSV1DVenue("south", 10, 0, 0, 10, 10, "two_sided"),
			}),
			testSV1DProbeArm(plan.ModeOff, true, true, true, true, false, []CDFVenueConcentrationAudit{
				testSV1DVenue("north", 20, 0, 0, 20, 20, "two_sided"), testSV1DVenue("central", 20, 0, 0, 20, 20, "two_sided"), testSV1DVenue("south", 20, 0, 0, 20, 20, "two_sided"),
			}),
			testSV1DProbeArm(plan.NoRoster, true, true, true, true, false, []CDFVenueConcentrationAudit{
				testSV1DVenue("north", 30, 0, 0, 30, 30, "two_sided"), testSV1DVenue("central", 30, 0, 0, 30, 30, "two_sided"), testSV1DVenue("south", 30, 0, 0, 30, 30, "two_sided"),
			}),
		}
	}
	arms := validArms()
	arms[0].Venues[0].BidOnlyDurationNano = math.MaxInt64
	arms[0].Venues[0].AskOnlyDurationNano = 1
	if status := ScoreSV1DProbe(plan, arms).Status; status != SV1DProbeStatusInvalidEvidence {
		t.Fatalf("overflowed metric status = %q; want %q", status, SV1DProbeStatusInvalidEvidence)
	}
	plan.ProbeDurationNano = math.MaxInt64
	if status := ScoreSV1DProbe(plan, validArms()).Status; status != SV1DProbeStatusInvalidEvidence {
		t.Fatalf("overflowed denominator status = %q; want %q", status, SV1DProbeStatusInvalidEvidence)
	}
}

func TestScoreSV1DProbePassesOnlyStrictlyLowerTreatmentDuration(t *testing.T) {
	plan := testSV1DProbePlan()
	arms := []SV1DProbeArmResult{
		testSV1DProbeArm(plan.Treatment, true, true, true, true, true, []CDFVenueConcentrationAudit{
			testSV1DVenue("north", 5, 5, 0, 10, 10, "two_sided"),
			testSV1DVenue("central", 10, 0, 10, 10, 20, "two_sided"),
			testSV1DVenue("south", 0, 0, 0, 0, 0, "two_sided"),
		}),
		testSV1DProbeArm(plan.ModeOff, true, true, true, true, true, []CDFVenueConcentrationAudit{
			testSV1DVenue("north", 100, 0, 0, 100, 100, "two_sided"),
			testSV1DVenue("central", 100, 0, 0, 100, 100, "two_sided"),
			testSV1DVenue("south", 100, 0, 0, 100, 100, "two_sided"),
		}),
		testSV1DProbeArm(plan.NoRoster, true, true, true, true, true, []CDFVenueConcentrationAudit{
			testSV1DVenue("north", 200, 0, 0, 200, 200, "two_sided"),
			testSV1DVenue("central", 200, 0, 0, 200, 200, "two_sided"),
			testSV1DVenue("south", 200, 0, 0, 200, 200, "two_sided"),
		}),
	}
	score := ScoreSV1DProbe(plan, arms)
	if score.Status != SV1DProbeStatusPass {
		t.Fatalf("probe status = %q, failures = %v", score.Status, score.FailedPredicates)
	}
	if score.TreatmentNonTwoSidedDurationNano != 30 || score.ModeOffNonTwoSidedDurationNano != 300 || score.NoRosterNonTwoSidedDurationNano != 600 {
		t.Fatalf("probe durations = treatment %d mode-off %d no-roster %d", score.TreatmentNonTwoSidedDurationNano, score.ModeOffNonTwoSidedDurationNano, score.NoRosterNonTwoSidedDurationNano)
	}
	reordered := []SV1DProbeArmResult{arms[2], arms[0], arms[1]}
	reorderedScore := ScoreSV1DProbe(plan, reordered)
	firstJSON, err := json.Marshal(score)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(reorderedScore)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("permuting arm input order changed the canonical probe score")
	}
}

func TestScoreSV1DProbeRejectsInvalidIncompleteAndNonDirectionalInputs(t *testing.T) {
	plan := testSV1DProbePlan()
	validArm := func(spec SV1DProbeArmSpec, treatment bool) SV1DProbeArmResult {
		return testSV1DProbeArm(spec, true, true, true, true, treatment, []CDFVenueConcentrationAudit{
			testSV1DVenue("north", 10, 0, 0, 10, 10, "two_sided"),
			testSV1DVenue("central", 10, 0, 0, 10, 10, "two_sided"),
			testSV1DVenue("south", 10, 0, 0, 10, 10, "two_sided"),
		})
	}
	base := []SV1DProbeArmResult{validArm(plan.Treatment, true), validArm(plan.ModeOff, false), validArm(plan.NoRoster, false)}
	tests := []struct {
		name   string
		mutate func([]SV1DProbeArmResult)
		want   string
	}{
		{
			name: "equal duration",
			mutate: func(arms []SV1DProbeArmResult) {
				for index := range arms[1].Venues {
					arms[1].Venues[index] = arms[0].Venues[index]
				}
				for index := range arms[2].Venues {
					arms[2].Venues[index] = arms[0].Venues[index]
				}
			},
			want: SV1DProbeStatusNoDirectionalEffect,
		},
		{
			name:   "incomplete control",
			mutate: func(arms []SV1DProbeArmResult) { arms[1].Complete = false },
			want:   SV1DProbeStatusIncompleteArm,
		},
		{
			name:   "treatment activation missing",
			mutate: func(arms []SV1DProbeArmResult) { arms[0].ActivationSatisfied = false },
			want:   SV1DProbeStatusTreatmentNotActivated,
		},
		{
			name:   "treatment anti-cheating failure",
			mutate: func(arms []SV1DProbeArmResult) { arms[0].AntiCheatingSatisfied = false },
			want:   SV1DProbeStatusAntiCheatingRejected,
		},
		{
			name:   "identity mismatch",
			mutate: func(arms []SV1DProbeArmResult) { arms[0].ConfigSHA256 = strings.Repeat("f", 64) },
			want:   SV1DProbeStatusInvalidEvidence,
		},
		{
			name:   "metric mismatch",
			mutate: func(arms []SV1DProbeArmResult) { arms[0].Venues[0].NonTwoSidedDurationNano++ },
			want:   SV1DProbeStatusInvalidEvidence,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arms := cloneSV1DProbeArms(base)
			test.mutate(arms)
			if got := ScoreSV1DProbe(plan, arms).Status; got != test.want {
				t.Fatalf("probe status = %q; want %q", got, test.want)
			}
		})
	}
}

func testSV1DProbePlan() SV1DProbePlan {
	return SV1DProbePlan{
		ExperimentID: "probe", HypothesisID: "hypothesis", Seed: 659, Horizon: "5m", ProbeDurationNano: 300,
		VenueIDs: []string{"north", "central", "south"}, MaxUninterruptedNonTwoSidedDurationNano: 30,
		Treatment: SV1DProbeArmSpec{Name: "treatment", ExperimentID: "treatment", HypothesisID: "treatment-hypothesis", ConfigSHA256: strings.Repeat("a", 64), SourceRevision: "revision", BinarySHA256: strings.Repeat("b", 64)},
		ModeOff:   SV1DProbeArmSpec{Name: "mode-off", ExperimentID: "mode-off", HypothesisID: "mode-off-hypothesis", ConfigSHA256: strings.Repeat("c", 64), SourceRevision: "revision", BinarySHA256: strings.Repeat("d", 64)},
		NoRoster:  SV1DProbeArmSpec{Name: "no-roster", ExperimentID: "no-roster", HypothesisID: "no-roster-hypothesis", ConfigSHA256: strings.Repeat("e", 64), SourceRevision: "revision", BinarySHA256: strings.Repeat("f", 64)},
	}
}

func testSV1DProbeArm(spec SV1DProbeArmSpec, complete, evidenceValid, strictMechanics, terminalValuation, activation bool, venues []CDFVenueConcentrationAudit) SV1DProbeArmResult {
	return SV1DProbeArmResult{
		ArmName: spec.Name, ExperimentID: spec.ExperimentID, HypothesisID: spec.HypothesisID,
		ConfigSHA256: spec.ConfigSHA256, SourceRevision: spec.SourceRevision, BinarySHA256: spec.BinarySHA256,
		Complete: complete, EvidenceValid: evidenceValid, StrictMechanicsValid: strictMechanics,
		TerminalValuationValid: terminalValuation, ActivationSatisfied: activation, AntiCheatingSatisfied: activation,
		Venues: venues,
	}
}

func testSV1DVenue(venueID string, bidOnly, askOnly, empty, oneSided, nonTwoSided int64, terminalMode string) CDFVenueConcentrationAudit {
	return CDFVenueConcentrationAudit{
		VenueID: venueID, BidOnlyDurationNano: bidOnly, AskOnlyDurationNano: askOnly, EmptyBookDurationNano: empty,
		OneSidedDurationNano: oneSided, NonTwoSidedDurationNano: nonTwoSided,
		MaxUninterruptedNonTwoSidedDurationNano: nonTwoSided, TerminalBookMode: terminalMode,
	}
}

func cloneSV1DProbeArms(arms []SV1DProbeArmResult) []SV1DProbeArmResult {
	clone := make([]SV1DProbeArmResult, len(arms))
	copy(clone, arms)
	for index := range clone {
		clone[index].Venues = append([]CDFVenueConcentrationAudit(nil), arms[index].Venues...)
	}
	return clone
}
