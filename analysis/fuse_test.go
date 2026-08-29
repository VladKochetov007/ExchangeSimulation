package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// writeFusedFixture builds a minimal run directory whose event logs contain the
// given lines, so fused and unfused scans can be compared on identical input.
func writeFusedFixture(t *testing.T, files map[string][]string) *Run {
	t.Helper()
	dir := t.TempDir()
	report := `{"terminal_accounts":[]}`
	if err := os.WriteFile(filepath.Join(dir, "greeks.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, lines := range files {
		path := filepath.Join(dir, "venues", "north", "spot", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func record(event string, ts int64, payload string) string {
	return fmt.Sprintf(`{"sim_ts":%d,"client_id":1,"event":%q,"data":{"venue_id":"north","symbol":"ABC-USD","payload":%s}}`,
		ts, event, payload)
}

// collector records what a scan delivered, in a form that can be compared
// between a fused and an unfused run.
type collector struct {
	mu     sync.Mutex
	events []string
}

func (c *collector) visit(event Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, fmt.Sprintf("%s|%d|%s|%d|%s",
		event.Name, event.SimTS, event.Symbol, event.Ordinal, string(event.Raw())))
}

func (c *collector) sorted() []string {
	out := append([]string(nil), c.events...)
	sort.Strings(out)
	return out
}

// TestFusedDeliversTheSameEventsAsSeparateScans is the core equivalence claim:
// sharing one physical pass must not change which records a metric observes.
func TestFusedDeliversTheSameEventsAsSeparateScans(t *testing.T) {
	run := writeFusedFixture(t, map[string][]string{
		"ABC-USD.jsonl": {
			record("OrderAccepted", 1, `{"order_id":1}`),
			record("Trade", 2, `{"qty":5}`),
			record("OrderFill", 3, `{"order_id":1,"filled_qty":5}`),
			record("BookDelta", 4, `{"side":0}`),
			record("OrderAccepted", 5, `{"order_id":2,"payload":{"symbol":"ABC-PERP","inner":true}}`),
		},
		"CDF-USD.jsonl": {
			record("Trade", 6, `{"qty":7}`),
			record("OrderCancelled", 7, `{"order_id":2}`),
		},
	})

	specs := []ScanOptions{
		{Events: []string{"OrderAccepted", "OrderFill"}, Workers: 1},
		{Events: []string{"Trade"}},
		{}, // every record
		{Events: []string{"OrderCancelled", "BookDelta"}, Workers: 1},
	}

	separate := make([]*collector, len(specs))
	for i, opts := range specs {
		separate[i] = &collector{}
		if err := run.Scan(opts, separate[i].visit); err != nil {
			t.Fatalf("spec %d: %v", i, err)
		}
	}

	fused := make([]*collector, len(specs))
	tasks := make([]FusedTask, len(specs))
	for i, opts := range specs {
		fused[i] = &collector{}
		index, options := i, opts
		tasks[i] = FusedTask{Name: fmt.Sprint(i), Compute: func(r *Run) error {
			return r.Scan(options, fused[index].visit)
		}}
	}
	for i, err := range run.RunFused(tasks, 4) {
		if err != nil {
			t.Fatalf("fused spec %d: %v", i, err)
		}
	}

	for i := range specs {
		want, got := separate[i].sorted(), fused[i].sorted()
		if len(want) != len(got) {
			t.Fatalf("spec %d: separate delivered %d events, fused delivered %d", i, len(want), len(got))
		}
		for j := range want {
			if want[j] != got[j] {
				t.Fatalf("spec %d event %d:\n separate %s\n fused    %s", i, j, want[j], got[j])
			}
		}
	}
}

// TestFusedIsolatesParseFailures requires a malformed record to fail exactly
// the metrics whose own prefilter admits it, leaving the others untouched.
func TestFusedIsolatesParseFailures(t *testing.T) {
	run := writeFusedFixture(t, map[string][]string{
		"ABC-USD.jsonl": {
			record("Trade", 1, `{"qty":1}`),
			`{"sim_ts":2,"client_id":1,"event":"OrderAccepted",` + `"data":{`, // truncated
			record("Trade", 3, `{"qty":2}`),
		},
	})

	admitted := ScanOptions{Events: []string{"OrderAccepted"}}
	unaffected := ScanOptions{Events: []string{"Trade"}}

	referenceAdmitted := run.Scan(admitted, func(Event) {})
	if referenceAdmitted == nil {
		t.Fatal("reference scan accepted a malformed record")
	}
	referenceOther := &collector{}
	if err := run.Scan(unaffected, referenceOther.visit); err != nil {
		t.Fatalf("reference unaffected scan failed: %v", err)
	}

	fusedOther := &collector{}
	errs := run.RunFused([]FusedTask{
		{Name: "admitted", Compute: func(r *Run) error { return r.Scan(admitted, func(Event) {}) }},
		{Name: "unaffected", Compute: func(r *Run) error { return r.Scan(unaffected, fusedOther.visit) }},
	}, 4)

	if errs[0] == nil {
		t.Fatal("fused scan accepted a malformed record that the reference rejected")
	}
	if errs[0].Error() != referenceAdmitted.Error() {
		t.Fatalf("fused error text differs:\n reference %v\n fused     %v", referenceAdmitted, errs[0])
	}
	if errs[1] != nil {
		t.Fatalf("a metric whose prefilter rejects the malformed record must not fail: %v", errs[1])
	}
	want, got := referenceOther.sorted(), fusedOther.sorted()
	if len(want) != len(got) {
		t.Fatalf("unaffected metric saw %d events fused vs %d separate", len(got), len(want))
	}
}

// TestFusedPreservesSerialOrdering requires a one-worker metric to observe the
// whole run as one ordered sequence, which several reducers depend on.
func TestFusedPreservesSerialOrdering(t *testing.T) {
	run := writeFusedFixture(t, map[string][]string{
		"ABC-USD.jsonl": {record("Trade", 1, `{"qty":1}`), record("Trade", 2, `{"qty":2}`)},
		"CDF-USD.jsonl": {record("Trade", 3, `{"qty":3}`), record("Trade", 4, `{"qty":4}`)},
	})
	opts := ScanOptions{Events: []string{"Trade"}, Workers: 1}

	var reference []int64
	if err := run.Scan(opts, func(event Event) { reference = append(reference, event.SimTS) }); err != nil {
		t.Fatal(err)
	}
	var fused []int64
	// A second, parallel task shares the round so the serial group is really
	// running alongside one rather than alone.
	errs := run.RunFused([]FusedTask{
		{Name: "serial", Compute: func(r *Run) error {
			return r.Scan(opts, func(event Event) { fused = append(fused, event.SimTS) })
		}},
		{Name: "parallel", Compute: func(r *Run) error {
			return r.Scan(ScanOptions{Events: []string{"Trade"}}, func(Event) {})
		}},
	}, 4)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("task %d: %v", i, err)
		}
	}
	if len(fused) != len(reference) {
		t.Fatalf("serial metric saw %d events fused vs %d separate", len(fused), len(reference))
	}
	for i := range reference {
		if fused[i] != reference[i] {
			t.Fatalf("serial order differs at %d: separate %d, fused %d", i, reference[i], fused[i])
		}
	}
}

// TestFusedNestedPayloadUnwrap checks that the shared envelope still performs
// the derivative unwrap, since every consumer depends on it.
func TestFusedNestedPayloadUnwrap(t *testing.T) {
	run := writeFusedFixture(t, map[string][]string{
		"ABC-USD.jsonl": {record("Trade", 1, `{"symbol":"ABC-PERP","payload":{"qty":9}}`)},
	})
	var symbol string
	var raw json.RawMessage
	errs := run.RunFused([]FusedTask{{Name: "t", Compute: func(r *Run) error {
		return r.Scan(ScanOptions{Events: []string{"Trade"}}, func(event Event) {
			symbol, raw = event.Symbol, event.Raw()
		})
	}}}, 2)
	if errs[0] != nil {
		t.Fatal(errs[0])
	}
	if symbol != "ABC-PERP" {
		t.Fatalf("nested symbol not unwrapped: %q", symbol)
	}
	if string(raw) != `{"qty":9}` {
		t.Fatalf("nested payload not unwrapped: %s", raw)
	}
}
