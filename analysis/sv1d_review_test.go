package analysis

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifySV1DReviewAttestationBindsReportIdentitiesAndSignature(t *testing.T) {
	dir := t.TempDir()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(dir, "review.md")
	report := []byte("independent review report\nverdict: ACCEPT\n")
	if err := os.WriteFile(reportPath, report, 0o644); err != nil {
		t.Fatal(err)
	}
	expected := testSV1DReviewExpectation(report)
	attestation := testSV1DReviewAttestation(privateKey, report)
	attestationPath := filepath.Join(dir, "review-attestation.json")
	writeSV1DReviewAttestation(t, attestationPath, attestation)
	if _, err := VerifySV1DReviewAttestation(attestationPath, reportPath, withTrustedKey(expected, publicKey)); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(reportPath, append(report, 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySV1DReviewAttestation(attestationPath, reportPath, withTrustedKey(expected, publicKey)); err == nil {
		t.Fatal("mutated review report remained accepted")
	}
	if err := os.WriteFile(reportPath, report, 0o644); err != nil {
		t.Fatal(err)
	}
	wrongSource := expected
	wrongSource.SourceRevision = strings.Repeat("f", 40)
	if _, err := VerifySV1DReviewAttestation(attestationPath, reportPath, withTrustedKey(wrongSource, publicKey)); err == nil {
		t.Fatal("wrong source revision remained accepted")
	}
	otherPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySV1DReviewAttestation(attestationPath, reportPath, withTrustedKey(expected, otherPublicKey)); err == nil {
		t.Fatal("review signed by another key remained accepted")
	}
}

func TestVerifySV1DReviewAttestationRejectsUnknownFieldsAndSymlinkedReport(t *testing.T) {
	dir := t.TempDir()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	report := []byte("review\n")
	reportPath := filepath.Join(dir, "review.md")
	if err := os.WriteFile(reportPath, report, 0o644); err != nil {
		t.Fatal(err)
	}
	expected := testSV1DReviewExpectation(report)
	attestation := testSV1DReviewAttestation(privateKey, report)
	attestationRaw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	unknownRaw := bytes.Replace(attestationRaw, []byte("}"), []byte(`,"unexpected":true}`), 1)
	unknownPath := filepath.Join(dir, "unknown.json")
	if err := os.WriteFile(unknownPath, unknownRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySV1DReviewAttestation(unknownPath, reportPath, withTrustedKey(expected, publicKey)); err == nil {
		t.Fatal("unknown review-attestation field was accepted")
	}
	symlinkPath := filepath.Join(dir, "review-link.md")
	if err := os.Symlink(reportPath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	attestationPath := filepath.Join(dir, "review-attestation.json")
	writeSV1DReviewAttestation(t, attestationPath, attestation)
	if _, err := VerifySV1DReviewAttestation(attestationPath, symlinkPath, withTrustedKey(expected, publicKey)); err == nil {
		t.Fatal("symlinked review report was accepted")
	}
}

func testSV1DReviewExpectation(report []byte) SV1DReviewExpectation {
	return SV1DReviewExpectation{
		SourceRevision:           strings.Repeat("a", 40),
		TreeRevision:             strings.Repeat("b", 40),
		ProbeID:                  "v2-r2-sv1d-activation-659",
		PlanSHA256:               strings.Repeat("c", 64),
		ParentRegistrationSHA256: strings.Repeat("d", 64),
		AmendmentSHA256:          strings.Repeat("e", 64),
		TreatmentConfigSHA256:    strings.Repeat("1", 64),
		ModeOffConfigSHA256:      strings.Repeat("2", 64),
		NoRosterConfigSHA256:     strings.Repeat("3", 64),
		BinarySHA256:             strings.Repeat("4", 64),
		AnalyzerSHA256:           strings.Repeat("5", 64),
		RendererSHA256:           strings.Repeat("6", 64),
		TrustedPublicKey:         nil,
	}
}

func testSV1DReviewAttestation(privateKey ed25519.PrivateKey, report []byte) SV1DReviewAttestation {
	reportDigest := sha256BytesHex(report)
	attestation := SV1DReviewAttestation{
		SchemaVersion: 1, Contract: SV1DReviewAttestationContract, Verdict: "ACCEPT",
		ReviewedSourceRevision: strings.Repeat("a", 40), ReviewedTreeRevision: strings.Repeat("b", 40),
		ReviewedProbeID: "v2-r2-sv1d-activation-659", PlanSHA256: strings.Repeat("c", 64), ReviewerID: "sol-xhigh-test",
		ReportSHA256: reportDigest, ParentRegistrationSHA256: strings.Repeat("d", 64), AmendmentSHA256: strings.Repeat("e", 64),
		TreatmentConfigSHA256: strings.Repeat("1", 64), ModeOffConfigSHA256: strings.Repeat("2", 64), NoRosterConfigSHA256: strings.Repeat("3", 64),
		BinarySHA256: strings.Repeat("4", 64), AnalyzerSHA256: strings.Repeat("5", 64), RendererSHA256: strings.Repeat("6", 64),
		SignatureAlgorithm: "Ed25519",
	}
	payloadRaw, err := json.Marshal(sv1dReviewSignedPayloadFrom(attestation))
	if err != nil {
		panic(err)
	}
	signature := ed25519.Sign(privateKey, append([]byte(sv1dReviewSignatureDomain), payloadRaw...))
	attestation.SignatureBase64 = base64.StdEncoding.EncodeToString(signature)
	return attestation
}

func withTrustedKey(expectation SV1DReviewExpectation, publicKey ed25519.PublicKey) SV1DReviewExpectation {
	expectation.TrustedPublicKey = publicKey
	return expectation
}

func writeSV1DReviewAttestation(t *testing.T, path string, attestation SV1DReviewAttestation) {
	t.Helper()
	raw, err := json.Marshal(attestation)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sha256BytesHex(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
