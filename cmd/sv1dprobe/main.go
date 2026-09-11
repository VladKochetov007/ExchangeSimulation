// Command sv1dprobe is the CLI adapter for the development-only SV1D tri-arm
// contract. It creates a pre-run plan, audits one retained arm, or scores the
// three already audited arm results. Economic logic remains in analysis.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"exchange_sim/analysis"
)

const (
	probePlanContract = "v2-r2-sv1d-probe-plan-v1"
	armResultContract = "v2-r2-sv1d-arm-result-v1"
	scoreContract     = "v2-r2-sv1d-score-v1"
	probeID           = "v2-r2-sv1d-activation-659"
)

type planDocument struct {
	SchemaVersion int                    `json:"schema_version"`
	Contract      string                 `json:"contract"`
	ProbeID       string                 `json:"probe_id"`
	Plan          analysis.SV1DProbePlan `json:"plan"`
}

type armResultDocument struct {
	SchemaVersion int                         `json:"schema_version"`
	Contract      string                      `json:"contract"`
	ProbeID       string                      `json:"probe_id"`
	Arm           analysis.SV1DProbeArmResult `json:"arm"`
}

type scoreDocument struct {
	SchemaVersion int                           `json:"schema_version"`
	Contract      string                        `json:"contract"`
	ProbeID       string                        `json:"probe_id"`
	Score         analysis.SV1DProbeScore       `json:"score"`
	Arms          []analysis.SV1DProbeArmResult `json:"arms"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sv1dprobe:", err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "", "plan, audit, or score")
	out := flag.String("out", "", "new JSON output path")
	planPath := flag.String("plan", "", "pre-run SV1D plan JSON")
	armName := flag.String("arm", "", "treatment, mode-off, or no-roster")
	runDir := flag.String("run-dir", "", "retained arm run directory")
	renderedDir := flag.String("rendered-dir", "", "independently rendered evidence directory")
	treatmentConfig := flag.String("treatment-config", "", "checked-in treatment config")
	modeOffConfig := flag.String("mode-off-config", "", "checked-in mode-off config")
	noRosterConfig := flag.String("no-roster-config", "", "checked-in no-roster config")
	sourceRevision := flag.String("source-revision", "", "externally resolved source revision")
	binarySHA256 := flag.String("binary-sha256", "", "externally resolved simulator binary SHA-256")
	analyzerSHA256 := flag.String("analyzer-sha256", "", "externally resolved sv1dprobe analyzer SHA-256")
	rendererSHA256 := flag.String("renderer-sha256", "", "externally resolved evsrender SHA-256")
	modeOffResult := flag.String("mode-off-result", "", "audited mode-off arm result")
	noRosterResult := flag.String("no-roster-result", "", "audited no-roster arm result")
	treatmentResult := flag.String("treatment-result", "", "audited treatment arm result")
	flag.Parse()

	switch *mode {
	case "plan":
		return createPlan(*out, *treatmentConfig, *modeOffConfig, *noRosterConfig, *sourceRevision, *binarySHA256, *analyzerSHA256, *rendererSHA256)
	case "audit":
		return auditArm(*out, *planPath, *armName, *runDir, *renderedDir)
	case "score":
		return scoreArms(*out, *planPath, *treatmentResult, *modeOffResult, *noRosterResult)
	default:
		return fmt.Errorf("-mode must be plan, audit, or score")
	}
}

func createPlan(out, treatmentPath, modeOffPath, noRosterPath, sourceRevision, binarySHA256, analyzerSHA256, rendererSHA256 string) error {
	if out == "" || treatmentPath == "" || modeOffPath == "" || noRosterPath == "" || sourceRevision == "" || binarySHA256 == "" || analyzerSHA256 == "" || rendererSHA256 == "" {
		return fmt.Errorf("plan mode requires -out, all three config paths, -source-revision, -binary-sha256, -analyzer-sha256, and -renderer-sha256")
	}
	if err := verifyCurrentAnalyzer(analyzerSHA256); err != nil {
		return err
	}
	triad, err := analysis.ValidateSV1DConfigTriadFiles(analysis.SV1DConfigPaths{
		Treatment: treatmentPath, ModeOff: modeOffPath, NoRoster: noRosterPath,
	})
	if err != nil {
		return err
	}
	plan, err := analysis.BuildRegisteredSV1DProbePlan(triad, sourceRevision, binarySHA256, analyzerSHA256, rendererSHA256)
	if err != nil {
		return err
	}
	return publishJSON(out, planDocument{SchemaVersion: 1, Contract: probePlanContract, ProbeID: probeID, Plan: plan})
}

func auditArm(out, planPath, armName, runDir, renderedDir string) error {
	if out == "" || planPath == "" || armName == "" || runDir == "" || renderedDir == "" {
		return fmt.Errorf("audit mode requires -out, -plan, -arm, -run-dir, and -rendered-dir")
	}
	document, err := readPlan(planPath)
	if err != nil {
		return err
	}
	spec, contract, treatment, err := planArm(document.Plan, armName)
	if err != nil {
		return err
	}
	if err := verifyCurrentAnalyzer(document.Plan.AnalyzerSHA256); err != nil {
		return err
	}
	run, err := analysis.Open(runDir)
	if err != nil {
		return err
	}
	result, err := run.AuditSV1DProbeArm(analysis.SV1DProbeArmAuditOptions{
		Spec: spec, Contract: contract, Treatment: treatment,
		Activation: analysis.CDFActivationOptions{
			Contract: contract, EvidenceDir: runDir, RenderedEvidenceDir: renderedDir,
			ExpectedProvenance: analysis.CDFExpectedProvenance{
				ConfigSHA256: spec.ConfigSHA256, SourceRevision: spec.SourceRevision,
				BinarySHA256: spec.BinarySHA256, BinaryGOOS: "linux", BinaryGOARCH: "amd64", BinaryGOAMD64: "v1",
				RendererSHA256: spec.RendererSHA256, RendererSourceRevision: spec.SourceRevision,
				RendererGOOS: "linux", RendererGOARCH: "amd64", RendererGOAMD64: "v1",
				RendererTrimpath: true, RendererCGOEnabled: "0",
			},
		},
	})
	if err != nil {
		return err
	}
	return publishJSON(out, armResultDocument{SchemaVersion: 1, Contract: armResultContract, ProbeID: probeID, Arm: result})
}

func scoreArms(out, planPath, treatmentPath, modeOffPath, noRosterPath string) error {
	if out == "" || planPath == "" || treatmentPath == "" || modeOffPath == "" || noRosterPath == "" {
		return fmt.Errorf("score mode requires -out, -plan, and all three arm result paths")
	}
	document, err := readPlan(planPath)
	if err != nil {
		return err
	}
	if err := verifyCurrentAnalyzer(document.Plan.AnalyzerSHA256); err != nil {
		return err
	}
	paths := []string{treatmentPath, modeOffPath, noRosterPath}
	arms := make([]analysis.SV1DProbeArmResult, 0, len(paths))
	for _, path := range paths {
		result, err := readArmResult(path)
		if err != nil {
			return err
		}
		if result.SchemaVersion != 1 || result.Contract != armResultContract || result.ProbeID != probeID {
			return fmt.Errorf("arm result %s has an invalid contract identity", path)
		}
		arms = append(arms, result.Arm)
	}
	score := analysis.ScoreSV1DProbe(document.Plan, arms)
	return publishJSON(out, scoreDocument{SchemaVersion: 1, Contract: scoreContract, ProbeID: probeID, Score: score, Arms: arms})
}

func verifyCurrentAnalyzer(expectedSHA256 string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve sv1dprobe executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("resolve sv1dprobe executable symlinks: %w", err)
	}
	file, err := os.Open(resolved)
	if err != nil {
		return fmt.Errorf("open sv1dprobe executable: %w", err)
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("hash sv1dprobe executable: %w", err)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != expectedSHA256 {
		return fmt.Errorf("sv1dprobe executable hash %s does not match plan %s", actual, expectedSHA256)
	}
	return nil
}

func readPlan(path string) (planDocument, error) {
	var document planDocument
	if err := readStrictJSON(path, &document); err != nil {
		return planDocument{}, fmt.Errorf("read plan: %w", err)
	}
	if document.SchemaVersion != 1 || document.Contract != probePlanContract || document.ProbeID != probeID {
		return planDocument{}, fmt.Errorf("plan has an invalid contract identity")
	}
	if err := analysis.ValidateRegisteredSV1DProbePlan(document.Plan); err != nil {
		return planDocument{}, err
	}
	return document, nil
}

func readArmResult(path string) (armResultDocument, error) {
	var document armResultDocument
	if err := readStrictJSON(path, &document); err != nil {
		return armResultDocument{}, fmt.Errorf("read arm result %s: %w", path, err)
	}
	return document, nil
}

func planArm(plan analysis.SV1DProbePlan, name string) (analysis.SV1DProbeArmSpec, analysis.CDFActivationContract, bool, error) {
	contract := analysis.RegisteredSV1DActivationContract()
	switch name {
	case "treatment":
		contract.ExperimentID = plan.Treatment.ExperimentID
		contract.HypothesisID = plan.Treatment.HypothesisID
		return plan.Treatment, contract, true, nil
	case "mode-off":
		contract.ExperimentID = plan.ModeOff.ExperimentID
		contract.HypothesisID = plan.ModeOff.HypothesisID
		return plan.ModeOff, contract, false, nil
	case "no-roster":
		contract.ExperimentID = plan.NoRoster.ExperimentID
		contract.HypothesisID = plan.NoRoster.HypothesisID
		return plan.NoRoster, contract, false, nil
	default:
		return analysis.SV1DProbeArmSpec{}, analysis.CDFActivationContract{}, false, fmt.Errorf("unknown arm %q", name)
	}
}

func publishJSON(path string, value any) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	directory := filepath.Dir(absolute)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".sv1dprobe-output-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	raw, err := json.Marshal(value)
	if err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(raw, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryName, absolute); err != nil {
		return fmt.Errorf("publish without overwrite: %w", err)
	}
	removeTemporary = true
	_ = os.Remove(temporaryName)
	return nil
}

func readStrictJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := walkJSONTokens(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple top-level JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return json.Unmarshal(raw, target)
}

func walkJSONTokens(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key: %s", key)
			}
			seen[key] = struct{}{}
			if err := walkJSONTokens(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkJSONTokens(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}
