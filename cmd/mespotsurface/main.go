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
	"exchange_sim/experiment/executionpilot"
	"exchange_sim/experiment/repeatedspot"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mespotsurface:", err)
		os.Exit(1)
	}
}

func run() error {
	root := flag.String("root", "", "retained E0 development root")
	output := flag.String("out", "", "new exclusive machine summary path")
	study := flag.String("study", "ME-013", "registered study: ME-013 or ME-015")
	diagnosticBinary := flag.String("diagnostic-binary", "", "pinned mespotdiagnose binary")
	diagnosticCommit := flag.String("diagnostic-commit", "", "exact diagnostic source commit")
	flag.Parse()
	if *root == "" || *output == "" || *diagnosticBinary == "" || len(*diagnosticCommit) != 40 || flag.NArg() != 0 {
		return errors.New("required: -root -out -diagnostic-binary -diagnostic-commit")
	}
	binaryRaw, err := os.ReadFile(*diagnosticBinary)
	if err != nil {
		return err
	}
	switch *study {
	case "ME-013":
		return runOriginalSurface(*root, *output, *diagnosticCommit, digest(binaryRaw))
	case "ME-015":
		return runReferenceSurface(*root, *output, *diagnosticCommit, digest(binaryRaw))
	default:
		return fmt.Errorf("unregistered surface study %q", *study)
	}
}

func runOriginalSurface(root, output, diagnosticCommit, diagnosticBinaryHash string) error {
	inputs := make([]repeatedspot.E0CellArtifacts, 0, len(repeatedspot.E0DevelopmentCells()))
	for _, cell := range repeatedspot.E0DevelopmentCells() {
		id := cell.ID()
		resultPath := filepath.Join(root, "analysis", id+"-result.json")
		diagnosticPath := filepath.Join(root, "analysis", id+"-liquidity-diagnostic-v3.json")
		var result repeatedspot.E0AnalyzedResult
		resultHash, err := readStrictJSON(resultPath, &result)
		if err != nil {
			return fmt.Errorf("%s result: %w", id, err)
		}
		var diagnostic repeatedspot.E0LiquidityDiagnostic
		diagnosticHash, err := readStrictJSON(diagnosticPath, &diagnostic)
		if err != nil {
			return fmt.Errorf("%s diagnostic: %w", id, err)
		}
		inputs = append(inputs, repeatedspot.E0CellArtifacts{Result: result, Diagnostic: diagnostic,
			ResultSHA256: resultHash, DiagnosticSHA256: diagnosticHash})
	}
	surface, err := repeatedspot.SummarizeE0Development(inputs)
	if err != nil {
		return err
	}
	surface.DiagnosticSourceCommit = diagnosticCommit
	surface.DiagnosticBinarySHA256 = diagnosticBinaryHash
	for index, cell := range repeatedspot.E0DevelopmentCells() {
		resources, err := readCellResources(root, cell.ID())
		if err != nil {
			return err
		}
		surface.Cells[index].RunResourceSHA256 = resources.runHash
		surface.Cells[index].AnalysisResourceSHA256 = resources.analysisHash
		surface.Cells[index].PeakRunAllocatedBytes = resources.peakRunAllocated
		surface.Cells[index].PeakRunCgroupMemoryBytes = resources.peakRunCgroup
		surface.Cells[index].PeakAnalysisCgroupBytes = resources.peakAnalysisCgroup
	}
	controls, err := verifiedTechnicalControls(root, surface.Cells[0].Cell, surface.Cells[0].ResultSHA256,
		surface.Cells[0].ManifestSHA256, surface.ExecutionIdentity)
	if err != nil {
		return err
	}
	surface.TechnicalControls = controls
	return writeSurface(output, surface)
}

func runReferenceSurface(root, output, diagnosticCommit, diagnosticBinaryHash string) error {
	inputs := make([]repeatedspot.E0CellArtifacts, 0, len(repeatedspot.E0LocalReferenceDevelopmentCells()))
	for _, cell := range repeatedspot.E0LocalReferenceDevelopmentCells() {
		id := cell.ID()
		var result repeatedspot.E0AnalyzedResult
		resultHash, err := readStrictJSON(filepath.Join(root, "analysis", id+"-result.json"), &result)
		if err != nil {
			return fmt.Errorf("%s result: %w", id, err)
		}
		var diagnostic repeatedspot.E0LiquidityDiagnostic
		diagnosticHash, err := readStrictJSON(filepath.Join(root, "analysis", id+"-liquidity-diagnostic-v4.json"), &diagnostic)
		if err != nil {
			return fmt.Errorf("%s diagnostic: %w", id, err)
		}
		inputs = append(inputs, repeatedspot.E0CellArtifacts{Result: result, Diagnostic: diagnostic,
			ResultSHA256: resultHash, DiagnosticSHA256: diagnosticHash})
	}
	surface, err := repeatedspot.SummarizeE0LocalReference(inputs)
	if err != nil {
		return err
	}
	surface.DiagnosticSourceCommit = diagnosticCommit
	surface.DiagnosticBinarySHA256 = diagnosticBinaryHash
	for index, cell := range repeatedspot.E0LocalReferenceDevelopmentCells() {
		resources, err := readCellResources(root, cell.ID())
		if err != nil {
			return err
		}
		surface.Cells[index].RunResourceSHA256 = resources.runHash
		surface.Cells[index].AnalysisResourceSHA256 = resources.analysisHash
		surface.Cells[index].PeakRunAllocatedBytes = resources.peakRunAllocated
		surface.Cells[index].PeakRunCgroupMemoryBytes = resources.peakRunCgroup
		surface.Cells[index].PeakAnalysisCgroupBytes = resources.peakAnalysisCgroup
	}
	controls, err := verifiedTechnicalControls(root, surface.Cells[0].Cell, surface.Cells[0].ResultSHA256,
		surface.Cells[0].ManifestSHA256, surface.ExecutionIdentity)
	if err != nil {
		return err
	}
	surface.TechnicalControls = controls
	return writeSurface(output, surface)
}

type cellResources struct {
	runHash, analysisHash           string
	peakRunAllocated, peakRunCgroup uint64
	peakAnalysisCgroup              uint64
}

func readCellResources(root, id string) (cellResources, error) {
	runHash, runResource, err := readResource(filepath.Join(root, "resource-"+id+".json"), filepath.Join(root, id+"-run"))
	if err != nil {
		return cellResources{}, fmt.Errorf("%s run resource: %w", id, err)
	}
	analysisHash, analysisResource, err := readResource(filepath.Join(root, "resource-analyze-"+id+".json"), filepath.Join(root, "analysis"))
	if err != nil {
		return cellResources{}, fmt.Errorf("%s analysis resource: %w", id, err)
	}
	return cellResources{runHash: runHash, analysisHash: analysisHash,
		peakRunAllocated: runResource.PeakAllocatedBytes, peakRunCgroup: runResource.PeakCgroupMemoryBytes,
		peakAnalysisCgroup: analysisResource.PeakCgroupMemoryBytes}, nil
}

func verifiedTechnicalControls(root string, referenceCell repeatedspot.E0Cell, referenceResultHash,
	referenceManifestHash string, identity executionpilot.Identity) ([]repeatedspot.E0TechnicalControlSummary, error) {
	controls := make([]repeatedspot.E0TechnicalControlSummary, 0, 2)
	for _, workers := range []int{1, 4} {
		id := fmt.Sprintf("technical-%s-g%d", referenceCell.ID(), workers)
		var control repeatedspot.E0AnalyzedResult
		resultHash, err := readStrictJSON(filepath.Join(root, "analysis", id+"-result.json"), &control)
		if err != nil {
			return nil, err
		}
		if resultHash != referenceResultHash || control.Cell != referenceCell ||
			control.Identity != identity || control.ManifestFileSHA256 != referenceManifestHash {
			return nil, fmt.Errorf("technical control %s is not bit-identical to the reference", id)
		}
		resources, err := readCellResources(root, id)
		if err != nil {
			return nil, err
		}
		controls = append(controls, repeatedspot.E0TechnicalControlSummary{
			Workers: workers, ResultSHA256: resultHash, ManifestSHA256: control.ManifestFileSHA256,
			RunResourceSHA256: resources.runHash, AnalysisResourceSHA256: resources.analysisHash,
			BitIdenticalToReference: true})
	}
	return controls, nil
}

func writeSurface(output string, surface any) error {
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(surface)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func readResource(path, expectedRoot string) (string, analysis.SV1DResourceMeasurement, error) {
	var measurement analysis.SV1DResourceMeasurement
	hash, err := readStrictJSON(path, &measurement)
	if err != nil {
		return "", measurement, err
	}
	if measurement.MeasurementRoot != expectedRoot ||
		measurement.CgroupMemoryLimitBytes == 0 || measurement.CgroupMemoryLimitBytes > 8<<30 ||
		measurement.MinimumAvailableBytes < 10<<30 || measurement.PeakAllocatedBytes > 3<<29 ||
		measurement.MaximumSwapUsedBytes != 0 ||
		measurement.CgroupOOMEventsDelta != 0 || measurement.CgroupOOMKillEventsDelta != 0 {
		return "", measurement, errors.New("resource measurement exceeds registered envelope")
	}
	if err := analysis.ValidateSV1DResourceMeasurement(measurement, true); err != nil {
		return "", measurement, err
	}
	return hash, measurement, nil
}

func readStrictJSON(path string, destination any) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if err := executionpilot.ValidateStrictJSON(raw); err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return "", err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return "", errors.New("trailing JSON content")
	}
	return digest(raw), nil
}

func digest(raw []byte) string {
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}
