package multivenue

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
)

// BinaryCheckpointAttestation is the part of the binary evidence attestation
// that binds the terminal checkpoint to the sealed execution stream.
type BinaryCheckpointAttestation struct {
	EventFrames         uint64
	ExecutionStreamHash string
}

type binaryCheckpointRecord struct {
	fields             map[string]any
	simulationTimeNano int64
	eventCount         int64
	executionHash      string
	final              bool
}

// ValidateBinaryCheckpointStream validates the strict evstream_v3 checkpoint
// contract without converting JSON numbers through float64. The optional
// attestation binds the terminal checkpoint to the sealed binary stream.
func ValidateBinaryCheckpointStream(
	reader io.Reader,
	simulationStartNano int64,
	simulationEndNano int64,
	attestation *BinaryCheckpointAttestation,
) error {
	if simulationStartNano > simulationEndNano {
		return fmt.Errorf("checkpoint bounds are reversed: start=%d end=%d", simulationStartNano, simulationEndNano)
	}
	objects, err := decodeStrictJSONObjects(reader, "checkpoint stream")
	if err != nil {
		return err
	}
	if len(objects) < 2 {
		return fmt.Errorf("checkpoint stream has %d record(s), want at least 2", len(objects))
	}

	records := make([]binaryCheckpointRecord, len(objects))
	finalCount := 0
	for index, fields := range objects {
		record, err := decodeBinaryCheckpointRecord(fields, index)
		if err != nil {
			return err
		}
		if record.simulationTimeNano < simulationStartNano || record.simulationTimeNano > simulationEndNano {
			return fmt.Errorf("checkpoint %d simulation time %d is outside [%d,%d]", index, record.simulationTimeNano, simulationStartNano, simulationEndNano)
		}
		if record.final {
			finalCount++
		}
		records[index] = record
	}
	if finalCount != 1 {
		return fmt.Errorf("checkpoint stream has %d final record(s), want exactly 1", finalCount)
	}
	lastIndex := len(records) - 1
	if !records[lastIndex].final {
		return errors.New("checkpoint stream does not end with a final record")
	}
	for index := 1; index < lastIndex; index++ {
		previous := records[index-1]
		current := records[index]
		if previous.final || current.final {
			return fmt.Errorf("checkpoint %d unexpectedly marks an intermediate record final", index)
		}
		if previous.simulationTimeNano >= current.simulationTimeNano {
			return fmt.Errorf("checkpoint simulation times are not strictly increasing at record %d", index)
		}
		if previous.eventCount >= current.eventCount {
			return fmt.Errorf("checkpoint event counts are not strictly increasing at record %d", index)
		}
	}

	penultimate := records[lastIndex-1]
	final := records[lastIndex]
	if penultimate.final {
		return errors.New("checkpoint penultimate record is final")
	}
	if penultimate.simulationTimeNano != final.simulationTimeNano || penultimate.eventCount != final.eventCount {
		return errors.New("final checkpoint does not repeat the terminal ordinary state")
	}
	if !reflect.DeepEqual(checkpointStateWithoutFinal(penultimate.fields), checkpointStateWithoutFinal(final.fields)) {
		return errors.New("final checkpoint changes state relative to the terminal ordinary record")
	}
	if final.simulationTimeNano != simulationEndNano {
		return fmt.Errorf("final checkpoint simulation time %d does not equal end %d", final.simulationTimeNano, simulationEndNano)
	}
	if attestation != nil {
		if uint64(final.eventCount) != attestation.EventFrames {
			return fmt.Errorf("terminal event count %d does not equal binary attestation event_frames %d", final.eventCount, attestation.EventFrames)
		}
		if final.executionHash != attestation.ExecutionStreamHash {
			return fmt.Errorf("terminal execution hash %q does not equal binary attestation hash %q", final.executionHash, attestation.ExecutionStreamHash)
		}
	}
	return nil
}

// ReadBinaryCheckpointAttestation parses the binary attestation with the same
// duplicate-key and exact-number rules as the checkpoint validator.
func ReadBinaryCheckpointAttestation(reader io.Reader) (BinaryCheckpointAttestation, error) {
	objects, err := decodeStrictJSONObjects(reader, "binary evidence attestation")
	if err != nil {
		return BinaryCheckpointAttestation{}, err
	}
	if len(objects) != 1 {
		return BinaryCheckpointAttestation{}, fmt.Errorf("binary evidence attestation has %d JSON values, want exactly 1", len(objects))
	}
	fields := objects[0]
	eventFrames, err := exactUint64Field(fields, "event_frames")
	if err != nil {
		return BinaryCheckpointAttestation{}, err
	}
	if eventFrames == 0 || eventFrames > math.MaxInt64 {
		return BinaryCheckpointAttestation{}, fmt.Errorf("event_frames %d is outside the checkpoint contract", eventFrames)
	}
	executionHash, err := exactStringField(fields, "execution_stream_hash")
	if err != nil {
		return BinaryCheckpointAttestation{}, err
	}
	if !isLowerSHA256(executionHash) {
		return BinaryCheckpointAttestation{}, fmt.Errorf("execution_stream_hash is not a lowercase SHA-256 digest")
	}
	return BinaryCheckpointAttestation{
		EventFrames:         eventFrames,
		ExecutionStreamHash: executionHash,
	}, nil
}

func decodeBinaryCheckpointRecord(fields map[string]any, index int) (binaryCheckpointRecord, error) {
	domain, err := exactStringField(fields, "domain")
	if err != nil {
		return binaryCheckpointRecord{}, checkpointFieldError(index, err)
	}
	if domain != "execution_observations" {
		return binaryCheckpointRecord{}, fmt.Errorf("checkpoint %d has domain %q", index, domain)
	}
	ordering, err := exactStringField(fields, "ordering")
	if err != nil {
		return binaryCheckpointRecord{}, checkpointFieldError(index, err)
	}
	if ordering != "ordered_stream" {
		return binaryCheckpointRecord{}, fmt.Errorf("checkpoint %d has ordering %q", index, ordering)
	}
	simulationTimeNano, err := exactInt64Field(fields, "sim_time")
	if err != nil {
		return binaryCheckpointRecord{}, checkpointFieldError(index, err)
	}
	eventCount, err := exactInt64Field(fields, "event_count")
	if err != nil {
		return binaryCheckpointRecord{}, checkpointFieldError(index, err)
	}
	if eventCount < 0 {
		return binaryCheckpointRecord{}, fmt.Errorf("checkpoint %d event_count is negative", index)
	}
	executionHash, err := exactStringField(fields, "execution_stream_hash")
	if err != nil {
		return binaryCheckpointRecord{}, checkpointFieldError(index, err)
	}
	if !isLowerSHA256(executionHash) {
		return binaryCheckpointRecord{}, fmt.Errorf("checkpoint %d execution_stream_hash is not a lowercase SHA-256 digest", index)
	}
	representation, err := exactStringField(fields, "representation")
	if err != nil {
		return binaryCheckpointRecord{}, checkpointFieldError(index, err)
	}
	if representation != binaryRepresentation {
		return binaryCheckpointRecord{}, fmt.Errorf("checkpoint %d has representation %q", index, representation)
	}
	rollingHash, err := exactStringField(fields, "rolling_hash")
	if err != nil {
		return binaryCheckpointRecord{}, checkpointFieldError(index, err)
	}
	if !isLowerSHA256(rollingHash) || rollingHash != executionHash {
		return binaryCheckpointRecord{}, fmt.Errorf("checkpoint %d rolling_hash does not equal execution_stream_hash", index)
	}
	if value, present := fields["unencodable_payloads"]; present {
		unencodablePayloads, err := exactNonNegativeInt64(value, "unencodable_payloads")
		if err != nil {
			return binaryCheckpointRecord{}, checkpointFieldError(index, err)
		}
		if unencodablePayloads != 0 {
			return binaryCheckpointRecord{}, fmt.Errorf("checkpoint %d has %d unencodable payloads", index, unencodablePayloads)
		}
	}
	final, err := optionalBoolField(fields, "final")
	if err != nil {
		return binaryCheckpointRecord{}, checkpointFieldError(index, err)
	}
	return binaryCheckpointRecord{
		fields:             fields,
		simulationTimeNano: simulationTimeNano,
		eventCount:         eventCount,
		executionHash:      executionHash,
		final:              final,
	}, nil
}

func checkpointStateWithoutFinal(fields map[string]any) map[string]any {
	state := make(map[string]any, len(fields))
	for key, value := range fields {
		if key != "final" {
			state[key] = value
		}
	}
	return state
}

func decodeStrictJSONObjects(reader io.Reader, description string) ([]map[string]any, error) {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	objects := make([]map[string]any, 0)
	for {
		value, err := decodeStrictJSONValue(decoder)
		if errors.Is(err, io.EOF) {
			return objects, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", description, err)
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("decode %s: top-level value is not an object", description)
		}
		objects = append(objects, object)
	}
}

func decodeStrictJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token := token.(type) {
	case json.Delim:
		switch token {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New("object key is not a string")
				}
				if _, exists := object[key]; exists {
					return nil, fmt.Errorf("duplicate object key %q", key)
				}
				value, err := decodeStrictJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				object[key] = value
			}
			end, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			if end != json.Delim('}') {
				return nil, errors.New("object does not terminate with }")
			}
			return object, nil
		case '[':
			values := make([]any, 0)
			for decoder.More() {
				value, err := decodeStrictJSONValue(decoder)
				if err != nil {
					return nil, err
				}
				values = append(values, value)
			}
			end, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			if end != json.Delim(']') {
				return nil, errors.New("array does not terminate with ]")
			}
			return values, nil
		default:
			return nil, fmt.Errorf("unexpected top-level delimiter %q", token)
		}
	case string, bool, json.Number:
		return token, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported JSON token %T", token)
	}
}

func exactStringField(fields map[string]any, fieldName string) (string, error) {
	value, ok := fields[fieldName]
	if !ok {
		return "", fmt.Errorf("missing %s", fieldName)
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s is not a string", fieldName)
	}
	return stringValue, nil
}

func exactInt64Field(fields map[string]any, fieldName string) (int64, error) {
	value, ok := fields[fieldName]
	if !ok {
		return 0, fmt.Errorf("missing %s", fieldName)
	}
	return exactInt64Value(value, fieldName)
}

func exactNonNegativeInt64(value any, fieldName string) (int64, error) {
	parsed, err := exactInt64Value(value, fieldName)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s is negative", fieldName)
	}
	return parsed, nil
}

func exactInt64Value(value any, fieldName string) (int64, error) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("%s is not an integer JSON number", fieldName)
	}
	token := number.String()
	if !isDecimalIntegerToken(token) {
		return 0, fmt.Errorf("%s is not a decimal integer", fieldName)
	}
	parsed, err := strconv.ParseInt(token, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is outside int64: %w", fieldName, err)
	}
	return parsed, nil
}

func exactUint64Field(fields map[string]any, fieldName string) (uint64, error) {
	value, ok := fields[fieldName]
	if !ok {
		return 0, fmt.Errorf("missing %s", fieldName)
	}
	number, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("%s is not an unsigned integer JSON number", fieldName)
	}
	token := number.String()
	if !isDecimalIntegerToken(token) || (len(token) > 0 && token[0] == '-') {
		return 0, fmt.Errorf("%s is not an unsigned decimal integer", fieldName)
	}
	parsed, err := strconv.ParseUint(token, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is outside uint64: %w", fieldName, err)
	}
	return parsed, nil
}

func optionalBoolField(fields map[string]any, fieldName string) (bool, error) {
	value, ok := fields[fieldName]
	if !ok {
		return false, nil
	}
	boolean, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s is not a boolean", fieldName)
	}
	return boolean, nil
}

func isDecimalIntegerToken(token string) bool {
	if token == "" {
		return false
	}
	digitStart := 0
	if token[0] == '-' {
		digitStart = 1
	}
	if digitStart == len(token) {
		return false
	}
	if token[digitStart] == '0' && len(token)-digitStart > 1 {
		return false
	}
	for _, character := range token[digitStart:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func isLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func checkpointFieldError(index int, err error) error {
	return fmt.Errorf("checkpoint %d: %w", index, err)
}
