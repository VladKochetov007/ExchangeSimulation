package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"exchange_sim/analysis"
)

type capacityVerificationInputs struct {
	AttestationPath string

	SourceRevision string
	TreeRevision   string
	PlanSHA256     string

	ReviewAttestationSHA256 string
	ReviewReportSHA256      string
	TrustedReviewKeySHA256  string

	TreatmentConfigPath string
	ModeOffConfigPath   string
	NoRosterConfigPath  string

	CapacityTreatmentConfigPath string
	CapacityModeOffConfigPath   string
	CapacityNoRosterConfigPath  string
	CapacityConfigDeltaSHA256   string

	BinarySHA256         string
	AnalyzerSHA256       string
	RendererSHA256       string
	RunnerSHA256         string
	MeasurerSHA256       string
	ResourcePolicySHA256 string

	OutputParent             string
	MeasurementRoot          string
	MeasurementRecordsRoot   string
	MeasurementRecordsSHA256 string
	FilesystemDevice         string
	FilesystemID             string
	FilesystemType           string
	FilesystemMountID        string
	FilesystemUUID           string
}

func verifyCapacity(inputs capacityVerificationInputs) error {
	if inputs.AttestationPath == "" || inputs.SourceRevision == "" || inputs.TreeRevision == "" || inputs.PlanSHA256 == "" ||
		inputs.ReviewAttestationSHA256 == "" || inputs.ReviewReportSHA256 == "" || inputs.TreatmentConfigPath == "" ||
		inputs.ModeOffConfigPath == "" || inputs.NoRosterConfigPath == "" || inputs.CapacityTreatmentConfigPath == "" ||
		inputs.CapacityModeOffConfigPath == "" || inputs.CapacityNoRosterConfigPath == "" || inputs.CapacityConfigDeltaSHA256 == "" ||
		inputs.BinarySHA256 == "" || inputs.AnalyzerSHA256 == "" || inputs.RendererSHA256 == "" || inputs.RunnerSHA256 == "" ||
		inputs.MeasurerSHA256 == "" || inputs.ResourcePolicySHA256 == "" || inputs.OutputParent == "" || inputs.MeasurementRoot == "" || inputs.MeasurementRecordsRoot == "" || inputs.MeasurementRecordsSHA256 == "" ||
		inputs.FilesystemDevice == "" || inputs.FilesystemID == "" || inputs.FilesystemType == "" || inputs.FilesystemMountID == "" || inputs.FilesystemUUID == "" {
		return fmt.Errorf("verify-capacity mode requires the attestation, exact config/tool/review identities, paths, and filesystem identity")
	}
	configSHA256 := func(path string) (string, error) {
		raw, err := readRegularNoSymlink(path)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(raw)
		return hex.EncodeToString(digest[:]), nil
	}
	targetTreatment, err := configSHA256(inputs.TreatmentConfigPath)
	if err != nil {
		return fmt.Errorf("hash target treatment config: %w", err)
	}
	targetModeOff, err := configSHA256(inputs.ModeOffConfigPath)
	if err != nil {
		return fmt.Errorf("hash target mode-off config: %w", err)
	}
	targetNoRoster, err := configSHA256(inputs.NoRosterConfigPath)
	if err != nil {
		return fmt.Errorf("hash target no-roster config: %w", err)
	}
	capacityTreatment, err := configSHA256(inputs.CapacityTreatmentConfigPath)
	if err != nil {
		return fmt.Errorf("hash capacity treatment config: %w", err)
	}
	capacityModeOff, err := configSHA256(inputs.CapacityModeOffConfigPath)
	if err != nil {
		return fmt.Errorf("hash capacity mode-off config: %w", err)
	}
	capacityNoRoster, err := configSHA256(inputs.CapacityNoRosterConfigPath)
	if err != nil {
		return fmt.Errorf("hash capacity no-roster config: %w", err)
	}
	_, err = analysis.VerifySV1DCapacityAttestation(inputs.AttestationPath, analysis.SV1DCapacityExpectation{
		SourceRevision: inputs.SourceRevision, TreeRevision: inputs.TreeRevision, ProbeID: probeID, PlanSHA256: inputs.PlanSHA256,
		ReviewAttestationSHA256: inputs.ReviewAttestationSHA256, ReviewReportSHA256: inputs.ReviewReportSHA256,
		TrustedReviewKeySHA256:      inputs.TrustedReviewKeySHA256,
		TargetTreatmentConfigSHA256: targetTreatment, TargetModeOffConfigSHA256: targetModeOff, TargetNoRosterConfigSHA256: targetNoRoster,
		CapacityTreatmentConfigSHA256: capacityTreatment, CapacityModeOffConfigSHA256: capacityModeOff, CapacityNoRosterConfigSHA256: capacityNoRoster,
		CapacityConfigDeltaSHA256: inputs.CapacityConfigDeltaSHA256,
		BinarySHA256:              inputs.BinarySHA256, AnalyzerSHA256: inputs.AnalyzerSHA256, RendererSHA256: inputs.RendererSHA256,
		RunnerSHA256: inputs.RunnerSHA256, MeasurerSHA256: inputs.MeasurerSHA256, ResourcePolicySHA256: inputs.ResourcePolicySHA256,
		OutputParent: inputs.OutputParent, MeasurementRoot: inputs.MeasurementRoot, MeasurementRecordsRoot: inputs.MeasurementRecordsRoot, MeasurementRecordsSHA256: inputs.MeasurementRecordsSHA256,
		FilesystemDevice: inputs.FilesystemDevice, FilesystemID: inputs.FilesystemID, FilesystemType: inputs.FilesystemType,
		FilesystemMountID: inputs.FilesystemMountID, FilesystemUUID: inputs.FilesystemUUID,
	})
	return err
}
