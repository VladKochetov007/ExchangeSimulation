package types

import (
	"encoding/json"
	"testing"
)

// Persisted order and fill evidence must retain the zero-valued enum members:
// BUY, MARKET, GTC, NORMAL, OPEN, and BOTH are all semantic values, never an
// absent field. This decodes the JSON into an independent generic envelope so
// the test checks the on-disk wire contract rather than Go struct defaults.
func TestPersistentEvidenceRetainsZeroValuedEnums(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  map[string]string
	}{
		{
			name:  "accepted order",
			value: Order{Side: Buy, PositionSide: PositionBoth, Type: Market, TimeInForce: GTC, Visibility: Normal, Status: Open},
			want: map[string]string{
				"side":          `"BUY"`,
				"position_side": `0`,
				"type":          `"MARKET"`,
				"time_in_force": `"GTC"`,
				"visibility":    `"NORMAL"`,
				"status":        `0`,
			},
		},
		{
			name:  "fill notification",
			value: FillNotification{Side: Buy, PositionSide: PositionBoth},
			want: map[string]string{
				"side":          `"BUY"`,
				"position_side": `0`,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			for field, want := range tc.want {
				got, ok := fields[field]
				if !ok {
					t.Fatalf("zero-valued semantic field %q omitted from %s evidence: %s", field, tc.name, raw)
				}
				if string(got) != want {
					t.Errorf("%s %q = %s, want %s", tc.name, field, got, want)
				}
			}
		})
	}
}
