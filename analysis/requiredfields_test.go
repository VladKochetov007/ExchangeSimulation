package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
)

// referenceRequiredFields is the pre-optimization presence check, kept verbatim
// so the fast scanner can be held to it.
func referenceRequiredFields(raw json.RawMessage, required []string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("payload is not a JSON object")
	}
	for _, name := range required {
		value, present := fields[name]
		if !present || string(bytes.TrimSpace(value)) == "null" {
			return fmt.Errorf("missing required payload field %q", name)
		}
	}
	return nil
}

// checkAgreement asserts that the scanner either declines or reaches exactly
// the reference verdict. Inputs must already be valid JSON, which is what
// decodeRequiredJSON guarantees before it consults the scanner.
func checkAgreement(t *testing.T, raw string, required []string) {
	t.Helper()
	if !json.Valid([]byte(raw)) {
		return
	}
	missing, decided := scanRequiredFields(json.RawMessage(raw), required)
	if !decided {
		return
	}
	want := referenceRequiredFields(json.RawMessage(raw), required)
	if missing == "" {
		if want != nil {
			t.Fatalf("scanner accepted %s for %v but reference rejected: %v", raw, required, want)
		}
		return
	}
	wantMessage := fmt.Sprintf("missing required payload field %q", missing)
	if want == nil || want.Error() != wantMessage {
		t.Fatalf("scanner reported %q for %s / %v; reference said %v", wantMessage, raw, required, want)
	}
}

func TestScanRequiredFieldsMatchesReference(t *testing.T) {
	required := []string{"order_id", "qty", "type"}
	cases := []string{
		`{"order_id":1,"qty":2,"type":"limit"}`,
		`{"order_id":1,"qty":2}`,
		`{}`,
		`{"order_id":null,"qty":2,"type":"x"}`,
		`{"order_id":1,"qty":null,"type":"x"}`,
		`{"order_id": null ,"qty":2,"type":"x"}`,
		`{"order_id":1,"order_id":null,"qty":2,"type":"x"}`,
		`{"order_id":null,"order_id":7,"qty":2,"type":"x"}`,
		`{"qty":2,"type":"x","order_id":9223372036854775807}`,
		`{"order_id":-9223372036854775808,"qty":1,"type":"x"}`,
		`{"order_id":1e400,"qty":1,"type":"x"}`,
		`{"order_id":{"a":[1,2,{"qty":null}]},"qty":1,"type":"x"}`,
		`{"order_id":[1,2,3],"qty":1,"type":"x"}`,
		`{"order_id":"a\"b}","qty":1,"type":"x"}`,
		`{"order_id":"a\\","qty":1,"type":"x"}`,
		`{"order_id":true,"qty":false,"type":"x"}`,
		`{ "order_id" : 1 , "qty" : 2 , "type" : "x" }`,
		"{\n\t\"order_id\":1,\n\t\"qty\":2,\n\t\"type\":\"x\"\n}",
		`{"nested":{"order_id":1},"qty":2,"type":"x"}`,
		`{"order_idx":1,"qty":2,"type":"x"}`,
		`{"order_id":1,"qty":2,"type":"x"}`,
		`null`,
		`[1,2,3]`,
		`"scalar"`,
		`12`,
		`{"order_id":1,"qty":2,"type":"x","extra":{"deep":[{"deeper":"}"}]}}`,
	}
	for _, raw := range cases {
		checkAgreement(t, raw, required)
		checkAgreement(t, raw, nil)
		checkAgreement(t, raw, []string{"qty"})
	}
}

// TestScanRequiredFieldsRandomDifferential builds random objects whose keys,
// value shapes and duplication patterns exercise the paths that decide a
// verdict, and requires agreement with the reference on every one.
func TestScanRequiredFieldsRandomDifferential(t *testing.T) {
	keys := []string{"order_id", "qty", "type", "remaining_qty", "is_full", "other", "ordering"}
	values := []string{
		`1`, `0`, `-1`, `null`, `"x"`, `""`, `true`, `false`, `1.5`, `1e10`,
		`9223372036854775807`, `-9223372036854775808`, `18446744073709551615`,
		`{}`, `[]`, `{"payload":{"a":1}}`, `[null,{"qty":null}]`, `"}"`, `"\"q\""`,
		`" null "`, `"qty"`, `"qty"`,
	}
	spacing := []string{"", " ", "\n", "\t", "  \n\t "}
	random := rand.New(rand.NewSource(20260829))
	requiredSets := [][]string{
		{"order_id"},
		{"order_id", "qty"},
		{"order_id", "qty", "type", "remaining_qty", "is_full"},
		{"is_full", "order_id"},
		nil,
	}
	for iteration := 0; iteration < 200000; iteration++ {
		count := random.Intn(6)
		var builder bytes.Buffer
		builder.WriteString("{")
		for i := 0; i < count; i++ {
			if i > 0 {
				builder.WriteString(",")
			}
			builder.WriteString(spacing[random.Intn(len(spacing))])
			fmt.Fprintf(&builder, "%q", keys[random.Intn(len(keys))])
			builder.WriteString(spacing[random.Intn(len(spacing))])
			builder.WriteString(":")
			builder.WriteString(spacing[random.Intn(len(spacing))])
			builder.WriteString(values[random.Intn(len(values))])
			builder.WriteString(spacing[random.Intn(len(spacing))])
		}
		builder.WriteString("}")
		raw := builder.String()
		checkAgreement(t, raw, requiredSets[random.Intn(len(requiredSets))])
	}
}

// TestMayNestPayloadIsConservative requires the nesting prefilter to admit
// every payload the reference unwrap would have acted on.
func TestMayNestPayloadIsConservative(t *testing.T) {
	cases := []string{
		`{"payload":{"a":1},"symbol":"ABC-PERP"}`,
		`{"symbol":"x","payload":{}}`,
		`{"payload":{"a":1}}`,
		`{"payload":{"a":1}}`,
		`{"a":1}`,
		`{}`,
		`{"note":"payload"}`,
		`[]`,
		`null`,
		`"payload"`,
		``,
	}
	for _, raw := range cases {
		var inner dataLayer
		unwraps := len(raw) > 0 && json.Unmarshal([]byte(raw), &inner) == nil && len(inner.Payload) > 0
		if unwraps && !mayNestPayload([]byte(raw)) {
			t.Fatalf("prefilter rejected %s but the reference unwrap accepts it", raw)
		}
	}
}

func TestMayNestPayloadRandomDifferential(t *testing.T) {
	fragments := []string{
		`"payload"`, `"pay"`, `"load"`, `"payload"`, `"payload"`, `"payloadx"`,
		`"symbol"`, `"a"`,
	}
	valueFragments := []string{`{"a":1}`, `{}`, `[]`, `1`, `null`, `"payload"`, `"p"`}
	random := rand.New(rand.NewSource(7))
	for iteration := 0; iteration < 100000; iteration++ {
		count := random.Intn(4)
		var builder bytes.Buffer
		builder.WriteString("{")
		for i := 0; i < count; i++ {
			if i > 0 {
				builder.WriteString(",")
			}
			builder.WriteString(fragments[random.Intn(len(fragments))])
			builder.WriteString(":")
			builder.WriteString(valueFragments[random.Intn(len(valueFragments))])
		}
		builder.WriteString("}")
		raw := builder.Bytes()
		if !json.Valid(raw) {
			continue
		}
		var inner dataLayer
		unwraps := json.Unmarshal(raw, &inner) == nil && len(inner.Payload) > 0
		if unwraps && !mayNestPayload(raw) {
			t.Fatalf("prefilter rejected %s but the reference unwrap accepts it", raw)
		}
	}
}
