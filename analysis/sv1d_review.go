package analysis

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	SV1DReviewAttestationContract = "v2-r2-sv1d-independent-review-v1"
	sv1dReviewSignatureDomain     = "v2-r2-sv1d-independent-review-signature-v1\x00"
)

// SV1DReviewAttestation is an externally produced acceptance record. Its
// signature is verified against a public key supplied by launch policy rather
// than a key carried by the record itself.
type SV1DReviewAttestation struct {
	SchemaVersion            int    `json:"schema_version"`
	Contract                 string `json:"contract"`
	Verdict                  string `json:"verdict"`
	ReviewedSourceRevision   string `json:"reviewed_source_revision"`
	ReviewedTreeRevision     string `json:"reviewed_tree_revision"`
	ReviewedProbeID          string `json:"reviewed_probe_id"`
	PlanSHA256               string `json:"plan_sha256"`
	ReviewerID               string `json:"reviewer_id"`
	ReportSHA256             string `json:"report_sha256"`
	ParentRegistrationSHA256 string `json:"parent_registration_sha256"`
	AmendmentSHA256          string `json:"amendment_sha256"`
	TreatmentConfigSHA256    string `json:"treatment_config_sha256"`
	ModeOffConfigSHA256      string `json:"mode_off_config_sha256"`
	NoRosterConfigSHA256     string `json:"no_roster_config_sha256"`
	BinarySHA256             string `json:"binary_sha256"`
	AnalyzerSHA256           string `json:"analyzer_sha256"`
	RendererSHA256           string `json:"renderer_sha256"`
	SignatureAlgorithm       string `json:"signature_algorithm"`
	SignatureBase64          string `json:"signature_base64"`
}

// SV1DReviewExpectation contains identities resolved independently by the
// launcher. Empty values are not treated as wildcards: every field is required
// for a strict review gate.
type SV1DReviewExpectation struct {
	SourceRevision           string
	TreeRevision             string
	ProbeID                  string
	PlanSHA256               string
	ParentRegistrationSHA256 string
	AmendmentSHA256          string
	TreatmentConfigSHA256    string
	ModeOffConfigSHA256      string
	NoRosterConfigSHA256     string
	BinarySHA256             string
	AnalyzerSHA256           string
	RendererSHA256           string
	TrustedPublicKey         ed25519.PublicKey
}

type sv1dReviewSignedPayload struct {
	SchemaVersion            int    `json:"schema_version"`
	Contract                 string `json:"contract"`
	Verdict                  string `json:"verdict"`
	ReviewedSourceRevision   string `json:"reviewed_source_revision"`
	ReviewedTreeRevision     string `json:"reviewed_tree_revision"`
	ReviewedProbeID          string `json:"reviewed_probe_id"`
	PlanSHA256               string `json:"plan_sha256"`
	ReviewerID               string `json:"reviewer_id"`
	ReportSHA256             string `json:"report_sha256"`
	ParentRegistrationSHA256 string `json:"parent_registration_sha256"`
	AmendmentSHA256          string `json:"amendment_sha256"`
	TreatmentConfigSHA256    string `json:"treatment_config_sha256"`
	ModeOffConfigSHA256      string `json:"mode_off_config_sha256"`
	NoRosterConfigSHA256     string `json:"no_roster_config_sha256"`
	BinarySHA256             string `json:"binary_sha256"`
	AnalyzerSHA256           string `json:"analyzer_sha256"`
	RendererSHA256           string `json:"renderer_sha256"`
	SignatureAlgorithm       string `json:"signature_algorithm"`
}

// VerifySV1DReviewAttestation checks the immutable review/report pair and
// returns the decoded record for retention in run metadata. It does not infer
// reviewer independence from the reviewer name; the detached signature and
// externally supplied trusted key are the authentication boundary.
func VerifySV1DReviewAttestation(attestationPath, reportPath string, expected SV1DReviewExpectation) (SV1DReviewAttestation, error) {
	var attestation SV1DReviewAttestation
	attestationRaw, err := readSV1DRegularFile(attestationPath)
	if err != nil {
		return attestation, fmt.Errorf("read SV1D review attestation: %w", err)
	}
	if err := rejectSV1DDuplicateJSONKeys(attestationRaw); err != nil {
		return attestation, fmt.Errorf("SV1D review attestation has malformed JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(attestationRaw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&attestation); err != nil {
		return attestation, fmt.Errorf("decode SV1D review attestation: %w", err)
	}
	if _, err := decoder.Token(); !isSV1DEOF(err) {
		if err == nil {
			return attestation, fmt.Errorf("SV1D review attestation has multiple top-level JSON values")
		}
		return attestation, fmt.Errorf("SV1D review attestation has trailing JSON: %w", err)
	}
	if err := validateSV1DReviewIdentity(attestation, expected); err != nil {
		return attestation, err
	}
	reportRaw, err := readSV1DRegularFile(reportPath)
	if err != nil {
		return attestation, fmt.Errorf("read SV1D review report: %w", err)
	}
	reportDigest := sha256.Sum256(reportRaw)
	if attestation.ReportSHA256 != hex.EncodeToString(reportDigest[:]) {
		return attestation, fmt.Errorf("SV1D review report digest does not match its attestation")
	}
	if len(expected.TrustedPublicKey) != ed25519.PublicKeySize {
		return attestation, fmt.Errorf("SV1D review gate requires a %d-byte trusted Ed25519 public key", ed25519.PublicKeySize)
	}
	signature, err := base64.StdEncoding.DecodeString(attestation.SignatureBase64)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return attestation, fmt.Errorf("SV1D review attestation has an invalid Ed25519 signature")
	}
	payload := sv1dReviewSignedPayloadFrom(attestation)
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return attestation, fmt.Errorf("marshal SV1D review signed payload: %w", err)
	}
	signedMessage := append([]byte(sv1dReviewSignatureDomain), payloadRaw...)
	if !ed25519.Verify(expected.TrustedPublicKey, signedMessage, signature) {
		return attestation, fmt.Errorf("SV1D review attestation signature is not valid under the trusted public key")
	}
	return attestation, nil
}

func validateSV1DReviewIdentity(attestation SV1DReviewAttestation, expected SV1DReviewExpectation) error {
	if attestation.SchemaVersion != 1 || attestation.Contract != SV1DReviewAttestationContract || attestation.Verdict != "ACCEPT" || attestation.SignatureAlgorithm != "Ed25519" {
		return fmt.Errorf("SV1D review attestation has an invalid contract, version, verdict, or signature algorithm")
	}
	if strings.TrimSpace(attestation.ReviewerID) == "" || attestation.ReviewedProbeID != expected.ProbeID || attestation.ReviewedSourceRevision != expected.SourceRevision || attestation.ReviewedTreeRevision != expected.TreeRevision || attestation.PlanSHA256 != expected.PlanSHA256 {
		return fmt.Errorf("SV1D review attestation does not match the reviewed source, tree, probe, or plan")
	}
	if attestation.ParentRegistrationSHA256 != expected.ParentRegistrationSHA256 || attestation.AmendmentSHA256 != expected.AmendmentSHA256 {
		return fmt.Errorf("SV1D review attestation does not match the registered scientific documents")
	}
	if attestation.TreatmentConfigSHA256 != expected.TreatmentConfigSHA256 || attestation.ModeOffConfigSHA256 != expected.ModeOffConfigSHA256 || attestation.NoRosterConfigSHA256 != expected.NoRosterConfigSHA256 {
		return fmt.Errorf("SV1D review attestation does not match the registered config triad")
	}
	if attestation.BinarySHA256 != expected.BinarySHA256 || attestation.AnalyzerSHA256 != expected.AnalyzerSHA256 || attestation.RendererSHA256 != expected.RendererSHA256 {
		return fmt.Errorf("SV1D review attestation does not match the reviewed tool identities")
	}
	for name, value := range map[string]string{
		"source revision":            attestation.ReviewedSourceRevision,
		"tree revision":              attestation.ReviewedTreeRevision,
		"plan digest":                attestation.PlanSHA256,
		"parent registration digest": attestation.ParentRegistrationSHA256,
		"amendment digest":           attestation.AmendmentSHA256,
		"treatment config digest":    attestation.TreatmentConfigSHA256,
		"mode-off config digest":     attestation.ModeOffConfigSHA256,
		"no-roster config digest":    attestation.NoRosterConfigSHA256,
		"simulator digest":           attestation.BinarySHA256,
		"analyzer digest":            attestation.AnalyzerSHA256,
		"renderer digest":            attestation.RendererSHA256,
		"report digest":              attestation.ReportSHA256,
	} {
		if name == "source revision" || name == "tree revision" {
			if !isCDFHex(value, 20) {
				return fmt.Errorf("SV1D review attestation has an invalid %s", name)
			}
			continue
		}
		if !isSV1DHexDigest(value) {
			return fmt.Errorf("SV1D review attestation has an invalid %s", name)
		}
	}
	return nil
}

func sv1dReviewSignedPayloadFrom(attestation SV1DReviewAttestation) sv1dReviewSignedPayload {
	return sv1dReviewSignedPayload{
		SchemaVersion: attestation.SchemaVersion, Contract: attestation.Contract, Verdict: attestation.Verdict,
		ReviewedSourceRevision: attestation.ReviewedSourceRevision, ReviewedTreeRevision: attestation.ReviewedTreeRevision,
		ReviewedProbeID: attestation.ReviewedProbeID, PlanSHA256: attestation.PlanSHA256, ReviewerID: attestation.ReviewerID,
		ReportSHA256: attestation.ReportSHA256, ParentRegistrationSHA256: attestation.ParentRegistrationSHA256,
		AmendmentSHA256: attestation.AmendmentSHA256, TreatmentConfigSHA256: attestation.TreatmentConfigSHA256,
		ModeOffConfigSHA256: attestation.ModeOffConfigSHA256, NoRosterConfigSHA256: attestation.NoRosterConfigSHA256,
		BinarySHA256: attestation.BinarySHA256, AnalyzerSHA256: attestation.AnalyzerSHA256,
		RendererSHA256: attestation.RendererSHA256, SignatureAlgorithm: attestation.SignatureAlgorithm,
	}
}

func readSV1DRegularFile(path string) ([]byte, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	current := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(absolute, current), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("path contains a symlink: %s", path)
		}
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path is not a regular file: %s", path)
	}
	return os.ReadFile(absolute)
}

func isSV1DEOF(err error) bool { return err == io.EOF }
