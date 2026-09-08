package multivenue

import (
	"strings"
	"testing"
)

const (
	checkpointValidationStartNano = int64(1735689600000000000)
	checkpointValidationEndNano   = int64(1735689900000000000)
)

func TestValidateBinaryCheckpointStreamPreservesProductionEpochIntegers(t *testing.T) {
	stream := validBinaryCheckpointStream(2)
	attestation := &BinaryCheckpointAttestation{
		EventFrames:         2,
		ExecutionStreamHash: strings.Repeat("a", 64),
	}
	if err := ValidateBinaryCheckpointStream(strings.NewReader(stream), checkpointValidationStartNano, checkpointValidationEndNano, attestation); err != nil {
		t.Fatalf("valid production-epoch stream rejected: %v", err)
	}
}

func TestValidateBinaryCheckpointStreamRejectsProductionEpochFraction(t *testing.T) {
	stream := strings.Replace(
		validBinaryCheckpointStream(2),
		`"sim_time":1735689600000000000`,
		`"sim_time":1735689600000000000.5`,
		1,
	)
	if err := ValidateBinaryCheckpointStream(strings.NewReader(stream), checkpointValidationStartNano, checkpointValidationEndNano, nil); err == nil {
		t.Fatal("fractional production-epoch simulation time was accepted")
	}
}

func TestValidateBinaryCheckpointStreamRejectsFractionalEventCount(t *testing.T) {
	stream := strings.Replace(
		validBinaryCheckpointStream(2),
		`"event_count":2`,
		`"event_count":2.5`,
		1,
	)
	if err := ValidateBinaryCheckpointStream(strings.NewReader(stream), checkpointValidationStartNano, checkpointValidationEndNano, nil); err == nil {
		t.Fatal("fractional event count was accepted")
	}
}

func TestValidateBinaryCheckpointStreamRejectsDuplicateKeys(t *testing.T) {
	stream := strings.Replace(
		validBinaryCheckpointStream(2),
		`"event_count":0`,
		`"event_count":0,"event_count":0`,
		1,
	)
	if err := ValidateBinaryCheckpointStream(strings.NewReader(stream), checkpointValidationStartNano, checkpointValidationEndNano, nil); err == nil {
		t.Fatal("duplicate checkpoint key was accepted")
	}
}

func TestValidateBinaryCheckpointStreamRejectsAttestationMismatch(t *testing.T) {
	attestation := &BinaryCheckpointAttestation{
		EventFrames:         3,
		ExecutionStreamHash: strings.Repeat("a", 64),
	}
	if err := ValidateBinaryCheckpointStream(strings.NewReader(validBinaryCheckpointStream(2)), checkpointValidationStartNano, checkpointValidationEndNano, attestation); err == nil {
		t.Fatal("mismatched event-frame attestation was accepted")
	}
	attestation.EventFrames = 2
	attestation.ExecutionStreamHash = strings.Repeat("b", 64)
	if err := ValidateBinaryCheckpointStream(strings.NewReader(validBinaryCheckpointStream(2)), checkpointValidationStartNano, checkpointValidationEndNano, attestation); err == nil {
		t.Fatal("mismatched stream-hash attestation was accepted")
	}
}

func TestReadBinaryCheckpointAttestationRejectsFractionalEventFrames(t *testing.T) {
	_, err := ReadBinaryCheckpointAttestation(strings.NewReader(`{"event_frames":2.5,"execution_stream_hash":"` + strings.Repeat("a", 64) + `"}`))
	if err == nil {
		t.Fatal("fractional event_frames was accepted")
	}
}

func validBinaryCheckpointStream(eventCount int) string {
	hash := strings.Repeat("a", 64)
	return strings.Join([]string{
		`{"domain":"execution_observations","ordering":"ordered_stream","sim_time":1735689600000000000,"event_count":0,"execution_stream_hash":"` + strings.Repeat("0", 64) + `","rolling_hash":"` + strings.Repeat("0", 64) + `","representation":"evstream_v3","unencodable_payloads":0}`,
		`{"domain":"execution_observations","ordering":"ordered_stream","sim_time":1735689900000000000,"event_count":` + string(rune('0'+eventCount)) + `,"execution_stream_hash":"` + hash + `","rolling_hash":"` + hash + `","representation":"evstream_v3","unencodable_payloads":0}`,
		`{"domain":"execution_observations","ordering":"ordered_stream","sim_time":1735689900000000000,"event_count":` + string(rune('0'+eventCount)) + `,"execution_stream_hash":"` + hash + `","rolling_hash":"` + hash + `","representation":"evstream_v3","unencodable_payloads":0,"final":true}`,
	}, "\n") + "\n"
}
