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
	plan, err := BuildRegisteredSV1DProbePlan(triad, strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSV1DProbePlan(plan); err != nil {
		t.Fatal(err)
	}
	if plan.ProbeDurationNano != 300_000_000_000 || plan.MaxUninterruptedNonTwoSidedDurationNano != 30_000_000_000 {
		t.Fatalf("registered plan boundary = %+v", plan)
	}
}

func TestAuditSV1DProbeArmKeepsExternalIdentitySeparateFromLegacyEvidence(t *testing.T) {
	run := writeRegisteredCDFActivationFixture(t, cdfActivationFixtureOptions{})
	configRaw, err := os.ReadFile(filepath.Join(run.Dir, "run-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	configDigest := sha256.Sum256(configRaw)
	configSHA256 := hex.EncodeToString(configDigest[:])
	sourceRevision := strings.Repeat("a", 40)
	binarySHA256 := strings.Repeat("b", 64)
	spec := SV1DProbeArmSpec{
		Name: "treatment", ExperimentID: "v2-r2-sv1d-activation-659-treatment",
		HypothesisID: "V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY", ConfigSHA256: configSHA256,
		SourceRevision: sourceRevision, BinarySHA256: binarySHA256,
	}
	contract := RegisteredSV1DActivationContract()
	result, err := run.AuditSV1DProbeArm(SV1DProbeArmAuditOptions{
		Spec: spec, Contract: contract, Treatment: true,
		Activation: CDFActivationOptions{
			Contract: contract, EvidenceDir: run.Dir, AllowLegacyJSON: true,
			ExpectedProvenance: CDFExpectedProvenance{
				ConfigSHA256: configSHA256, SourceRevision: sourceRevision, BinarySHA256: binarySHA256,
				BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || !result.EvidenceValid || !result.ActivationSatisfied || !result.AntiCheatingSatisfied {
		t.Fatalf("legacy treatment arm result = %+v", result)
	}
	if result.StrictMechanicsValid || result.TerminalValuationValid {
		t.Fatalf("legacy evidence was promoted to strict/terminal validity: %+v", result)
	}
	mutated := spec
	mutated.BinarySHA256 = strings.Repeat("c", 64)
	_, err = run.AuditSV1DProbeArm(SV1DProbeArmAuditOptions{
		Spec: mutated, Contract: contract, Treatment: true,
		Activation: CDFActivationOptions{
			Contract: contract, EvidenceDir: run.Dir, AllowLegacyJSON: true,
			ExpectedProvenance: CDFExpectedProvenance{
				ConfigSHA256: configSHA256, SourceRevision: sourceRevision, BinarySHA256: binarySHA256,
				BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
			},
		},
	})
	if err == nil {
		t.Fatal("arm identity mismatch was accepted")
	}
}

func TestValidateSV1DRendererAttestationBindsRenderedAttestation(t *testing.T) {
	dir := t.TempDir()
	mainAttestation := []byte(`{"domain":"rendered_binary_evidence","source_execution_stream_hash":"` + strings.Repeat("a", 64) + `"}`)
	if err := os.WriteFile(filepath.Join(dir, "rendered-binary-evidence-attestation.json"), mainAttestation, 0o644); err != nil {
		t.Fatal(err)
	}
	mainDigest := sha256.Sum256(mainAttestation)
	mainDigestHex := hex.EncodeToString(mainDigest[:])
	rendererDigest := strings.Repeat("b", 64)
	raw, err := json.Marshal(sv1dRendererAttestation{
		SchemaVersion: 1, Contract: "v2-r2-sv1d-renderer-attestation-v1",
		RendererSHA256: rendererDigest, RendererSourceRevision: strings.Repeat("c", 40),
		RendererGOOS: "linux", RendererGOARCH: "amd64", RendererGOAMD64: "v1",
		RendererGoVersion: "go1.27.0", RendererTrimpath: true, RendererCGOEnabled: "0",
		RenderedAttestationSHA256: mainDigestHex,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "renderer-attestation.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	expected := CDFExpectedProvenance{
		RendererSHA256: rendererDigest, RendererSourceRevision: strings.Repeat("c", 40),
		RendererGOOS: "linux", RendererGOARCH: "amd64", RendererGOAMD64: "v1",
		RendererTrimpath: true, RendererCGOEnabled: "0",
	}
	if err := validateSV1DRendererAttestation(dir, expected); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rendered-binary-evidence-attestation.json"), append(mainAttestation, 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateSV1DRendererAttestation(dir, expected); err == nil {
		t.Fatal("renderer attestation remained valid after rendered evidence mutation")
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

func TestValidateRegisteredSV1DProbePlanRejectsArmSlotDrift(t *testing.T) {
	root := filepath.Join("..", "research", "configs", "v2-r2-sv1d-activation")
	triad, err := ValidateSV1DConfigTriadFiles(SV1DConfigPaths{
		Treatment: filepath.Join(root, "activation-659-treatment.json"),
		ModeOff:   filepath.Join(root, "activation-659-mode-off.json"),
		NoRoster:  filepath.Join(root, "activation-659-no-roster.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	registered, err := BuildRegisteredSV1DProbePlan(triad, strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*SV1DProbePlan)
	}{
		{name: "renamed arm", mutate: func(plan *SV1DProbePlan) { plan.Treatment.Name = "mode-off" }},
		{name: "swapped experiment", mutate: func(plan *SV1DProbePlan) { plan.ModeOff.ExperimentID = plan.NoRoster.ExperimentID }},
		{name: "swapped hypothesis", mutate: func(plan *SV1DProbePlan) { plan.NoRoster.HypothesisID = plan.ModeOff.HypothesisID }},
		{name: "binary divergence", mutate: func(plan *SV1DProbePlan) { plan.NoRoster.BinarySHA256 = strings.Repeat("e", 64) }},
		{name: "source divergence", mutate: func(plan *SV1DProbePlan) { plan.ModeOff.SourceRevision = strings.Repeat("f", 40) }},
		{name: "duplicate config identity", mutate: func(plan *SV1DProbePlan) { plan.NoRoster.ConfigSHA256 = plan.Treatment.ConfigSHA256 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := registered
			test.mutate(&candidate)
			if err := ValidateRegisteredSV1DProbePlan(candidate); err == nil {
				t.Fatal("registered arm drift was accepted")
			}
		})
	}
}

func TestSV1DProbePlanSHA256IsDomainSeparatedAndMutationSensitive(t *testing.T) {
	plan := testSV1DProbePlan()
	first, err := SV1DProbePlanSHA256(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !isSV1DHexDigest(first) {
		t.Fatalf("plan digest = %q", first)
	}
	mutated := plan
	mutated.Seed++
	second, err := SV1DProbePlanSHA256(mutated)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("semantic plan mutation did not change plan digest")
	}
	legacyDigest := sha256.Sum256(mustJSON(t, plan))
	if first == hex.EncodeToString(legacyDigest[:]) {
		t.Fatal("plan digest lacks the registered domain separator")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
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
			name: "incomplete control",
			mutate: func(arms []SV1DProbeArmResult) {
				arms[1].Complete = false
				arms[1].Venues = nil
			},
			want: SV1DProbeStatusIncompleteArm,
		},
		{
			name: "complete arm with invalid evidence",
			mutate: func(arms []SV1DProbeArmResult) {
				arms[1].EvidenceValid = false
			},
			want: SV1DProbeStatusInvalidEvidence,
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
	analyzerSHA256 := strings.Repeat("c", 64)
	rendererSHA256 := strings.Repeat("d", 64)
	return SV1DProbePlan{
		ExperimentID: "probe", HypothesisID: "hypothesis", Seed: 659, Horizon: "5m", ProbeDurationNano: 300,
		VenueIDs: []string{"north", "central", "south"}, MaxUninterruptedNonTwoSidedDurationNano: 30,
		AnalyzerSHA256: analyzerSHA256, RendererSHA256: rendererSHA256,
		Treatment: SV1DProbeArmSpec{Name: "treatment", ExperimentID: "treatment", HypothesisID: "treatment-hypothesis", ConfigSHA256: strings.Repeat("a", 64), SourceRevision: strings.Repeat("e", 40), BinarySHA256: strings.Repeat("b", 64), AnalyzerSHA256: analyzerSHA256, RendererSHA256: rendererSHA256},
		ModeOff:   SV1DProbeArmSpec{Name: "mode-off", ExperimentID: "mode-off", HypothesisID: "mode-off-hypothesis", ConfigSHA256: strings.Repeat("f", 64), SourceRevision: strings.Repeat("e", 40), BinarySHA256: strings.Repeat("a", 64), AnalyzerSHA256: analyzerSHA256, RendererSHA256: rendererSHA256},
		NoRoster:  SV1DProbeArmSpec{Name: "no-roster", ExperimentID: "no-roster", HypothesisID: "no-roster-hypothesis", ConfigSHA256: strings.Repeat("1", 64), SourceRevision: strings.Repeat("e", 40), BinarySHA256: strings.Repeat("2", 64), AnalyzerSHA256: analyzerSHA256, RendererSHA256: rendererSHA256},
	}
}

func testSV1DProbeArm(spec SV1DProbeArmSpec, complete, evidenceValid, strictMechanics, terminalValuation, activation bool, venues []CDFVenueConcentrationAudit) SV1DProbeArmResult {
	planSHA256, err := SV1DProbePlanSHA256(testSV1DProbePlan())
	if err != nil {
		panic(err)
	}
	return SV1DProbeArmResult{
		ArmName: spec.Name, ExperimentID: spec.ExperimentID, HypothesisID: spec.HypothesisID,
		ConfigSHA256: spec.ConfigSHA256, SourceRevision: spec.SourceRevision, BinarySHA256: spec.BinarySHA256,
		AnalyzerSHA256: spec.AnalyzerSHA256, RendererSHA256: spec.RendererSHA256,
		PlanSHA256: planSHA256,
		Complete:   complete, EvidenceValid: evidenceValid, StrictMechanicsValid: strictMechanics,
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
