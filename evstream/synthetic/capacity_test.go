package synthetic

import (
	"bytes"
	"testing"
)

func TestSyntheticCapacityStreamIsDeterministicAndGloballyOrdered(t *testing.T) {
	profile := NewProfile(137)
	var first bytes.Buffer
	firstReport, err := Write(&first, profile)
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	var second bytes.Buffer
	secondReport, err := Write(&second, profile)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("synthetic stream changed across identical runs")
	}
	if firstReport != secondReport {
		t.Fatalf("synthetic report changed across identical runs:\nfirst=%+v\nsecond=%+v", firstReport, secondReport)
	}
	if err := Verify(bytes.NewReader(first.Bytes()), firstReport); err != nil {
		t.Fatalf("readback verification: %v", err)
	}
	if firstReport.FamilyCounts != [3]uint64{111, 13, 13} {
		t.Fatalf("family counts = %v, want [111 13 13]", firstReport.FamilyCounts)
	}
}

func TestSyntheticCapacityRejectsReservedActivationSeed(t *testing.T) {
	profile := NewProfile(10)
	profile.WorkloadSeed = 659
	if err := profile.Validate(); err == nil {
		t.Fatal("reserved activation seed was accepted by synthetic profile")
	}
}

func TestSyntheticCapacityReadbackRejectsCorruption(t *testing.T) {
	profile := NewProfile(31)
	var stream bytes.Buffer
	report, err := Write(&stream, profile)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	corrupt := append([]byte(nil), stream.Bytes()...)
	corrupt[len(corrupt)-1] ^= 0x80
	if err := Verify(bytes.NewReader(corrupt), report); err == nil {
		t.Fatal("corrupted synthetic stream was accepted")
	}
}
