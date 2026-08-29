package analysis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
)

// referenceEventLayers is the decode the analyzer performed before the
// structural split, kept verbatim so the split can be held to it.
func referenceEventLayers(data json.RawMessage) (venueID, symbol string, payload json.RawMessage, err error) {
	var outer dataLayer
	if err := json.Unmarshal(data, &outer); err != nil {
		return "", "", nil, err
	}
	venueID, symbol, payload = outer.VenueID, outer.Symbol, outer.Payload
	var inner dataLayer
	if len(payload) > 0 && json.Unmarshal(payload, &inner) == nil && len(inner.Payload) > 0 {
		if inner.Symbol != "" {
			symbol = inner.Symbol
		}
		payload = inner.Payload
	}
	return venueID, symbol, payload, nil
}

func requireLayersAgree(t *testing.T, data string) {
	t.Helper()
	if !json.Valid([]byte(data)) {
		// decodeEventLayers is only ever reached with an already validated
		// value, so invalid input is out of contract.
		return
	}
	wantVenue, wantSymbol, wantPayload, wantErr := referenceEventLayers(json.RawMessage(data))
	gotVenue, gotSymbol, gotPayload, gotErr := decodeEventLayers(json.RawMessage(data))

	switch {
	case wantErr == nil && gotErr != nil:
		t.Fatalf("%s: reference accepted, split rejected: %v", data, gotErr)
	case wantErr != nil && gotErr == nil:
		t.Fatalf("%s: reference rejected (%v), split accepted", data, wantErr)
	case wantErr != nil && gotErr != nil:
		if wantErr.Error() != gotErr.Error() {
			t.Fatalf("%s: error text differs:\n reference %v\n split     %v", data, wantErr, gotErr)
		}
		return
	}
	if gotVenue != wantVenue {
		t.Fatalf("%s: venue %q vs reference %q", data, gotVenue, wantVenue)
	}
	if gotSymbol != wantSymbol {
		t.Fatalf("%s: symbol %q vs reference %q", data, gotSymbol, wantSymbol)
	}
	if !bytes.Equal(gotPayload, wantPayload) {
		t.Fatalf("%s: payload %s vs reference %s", data, gotPayload, wantPayload)
	}
}

func TestDecodeEventLayersMatchesReference(t *testing.T) {
	cases := []string{
		`{"venue_id":"north","symbol":"ABC-USD","payload":{"qty":1}}`,
		`{"payload":{"qty":1},"symbol":"ABC-USD","venue_id":"north"}`,
		`{"venue_id":"north","payload":{"symbol":"ABC-PERP","payload":{"qty":2}}}`,
		`{"venue_id":"north","symbol":"outer","payload":{"symbol":"inner","payload":{"a":1}}}`,
		`{"venue_id":"north","symbol":"outer","payload":{"payload":{"a":1}}}`,
		`{"venue_id":"north"}`,
		`{}`,
		`{"venue_id":null,"symbol":null,"payload":null}`,
		`{"venue_id":"a","venue_id":"b","payload":{"x":1}}`,
		`{"symbol":"first","symbol":"second","payload":{"x":1}}`,
		`{"payload":{"a":1},"payload":{"b":2}}`,
		`{"venue_id":"a","payload":{"a":1},"unknown":{"deep":[1,2,{"payload":{"z":1}}]}}`,
		`{"venue_id":"a","payload":[1,2,3]}`,
		`{"venue_id":"a","payload":"text"}`,
		`{"venue_id":"a","payload":12}`,
		`{"venue_id":"a","payload":true}`,
		`{ "venue_id" : "a" , "symbol" : "b" , "payload" : { "q" : 1 } }`,
		"{\n\"venue_id\":\"a\",\n\"payload\":{\"q\":1}\n}",
		`{"venue_id":"escaped","payload":{"a":1}}`,
		`{"venue_id":"back\\slash","payload":{"a":1}}`,
		`{"venue_id":"quote\"inside","payload":{"a":1}}`,
		`{"venue_id":123,"payload":{"a":1}}`,
		`{"venue_id":{"a":1},"payload":{"a":1}}`,
		`{"venue_id":"a","symbol":"}","payload":{"a":"{"}}`,
		`{"payload":{"note":"payload","a":1}}`,
		`{"payload":{"payload":"scalar"}}`,
		`{"payload":{"payload":null}}`,
		`{"payload":{"payload":{}}}`,
		`null`,
		`[1,2]`,
		`"scalar"`,
		`7`,
		`true`,
	}
	for _, data := range cases {
		requireLayersAgree(t, data)
	}
}

// TestDecodeEventLayersRandomDifferential builds random data layers whose key
// sets, duplication, nesting and value shapes exercise every branch, and
// requires exact agreement with the reference decode on all of them.
func TestDecodeEventLayersRandomDifferential(t *testing.T) {
	keys := []string{"venue_id", "symbol", "payload", "other", "venue", "symbols"}
	values := []string{
		`"north"`, `"ABC-USD"`, `null`, `""`, `123`, `true`, `[1,2]`, `{}`,
		`{"qty":1}`, `{"symbol":"inner","payload":{"q":9}}`, `{"payload":{}}`,
		`{"payload":null}`, `{"payload":"s"}`, `"escaped"`, `"back\\slash"`,
		`"brace}"`, `"colon:"`, `{"a":{"payload":{"deep":1}}}`, `"payload"`,
	}
	spacing := []string{"", " ", "\n", "\t", " \n\t "}
	random := rand.New(rand.NewSource(20260830))
	for iteration := 0; iteration < 200000; iteration++ {
		count := random.Intn(5)
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
		requireLayersAgree(t, builder.String())
	}
}

// TestDecodeEventLayersRetainedEvidence holds the split to the reference decode
// over every record of a retained evidence corpus, which is the input shape that
// actually matters.
func TestDecodeEventLayersRetainedEvidence(t *testing.T) {
	lines, ok := retainedCorpusLines(t)
	if !ok {
		return
	}
	decided, declined := 0, 0
	for index, line := range lines {
		var env envelope
		if json.Unmarshal(line, &env) != nil {
			continue
		}
		if _, _, _, ok := splitDataLayer(env.Data); ok {
			decided++
		} else {
			declined++
		}
		func() {
			defer func() {
				if problem := recover(); problem != nil {
					t.Fatalf("record %d: %v", index+1, problem)
				}
			}()
			requireLayersAgree(t, string(env.Data))
		}()
	}
	t.Logf("retained records: %d decided structurally, %d fell back to the reference decode", decided, declined)
	if decided == 0 {
		t.Fatal("no retained record was decided structurally; the fast path is not reached at all")
	}
}
