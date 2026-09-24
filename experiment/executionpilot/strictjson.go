package executionpilot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := walkJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("execution pilot: trailing JSON content")
	}
	return nil
}

// ValidateStrictJSON rejects duplicate object keys and trailing content before
// another experiment package decodes a pinned plan or manifest.
func ValidateStrictJSON(raw []byte) error { return rejectDuplicateJSONKeys(raw) }

func walkJSONValue(decoder *json.Decoder) error {
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
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return fmt.Errorf("execution pilot: duplicate or invalid JSON field %v", keyToken)
			}
			seen[key] = true
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := walkJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return errors.New("execution pilot: unexpected JSON delimiter")
	}
}
