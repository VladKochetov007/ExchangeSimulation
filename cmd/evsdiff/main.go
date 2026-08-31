// Command evsdiff finds the first frame at which two evidence streams differ.
//
// Two runs that should be identical and are not is a question about the first
// divergence, not the last: everything after it is consequence. The streams are
// a total order with a sequence on every frame, so the first differing frame is
// exactly locatable rather than inferred from aggregate hashes.
//
//	evsdiff -a <dir>/events.evs -b <dir>/events.evs
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"exchange_sim/evstream"
)

type frameRecord struct {
	seq      uint64
	simTS    int64
	clientID uint64
	schema   uint16
	venue    string
	payload  []byte
}

func main() {
	pathA := flag.String("a", "", "first stream")
	pathB := flag.String("b", "", "second stream")
	flag.Parse()

	framesA := load(*pathA)
	framesB := load(*pathB)
	fmt.Printf("a %s  %d frames\nb %s  %d frames\n\n", *pathA, len(framesA), *pathB, len(framesB))

	limit := min(len(framesA), len(framesB))
	for i := 0; i < limit; i++ {
		a, b := framesA[i], framesB[i]
		if a.seq == b.seq && a.simTS == b.simTS && a.clientID == b.clientID &&
			a.schema == b.schema && a.venue == b.venue && bytes.Equal(a.payload, b.payload) {
			continue
		}
		fmt.Printf("first divergence at index %d\n", i)
		classify(framesA, framesB, limit)
		describe("a", a)
		describe("b", b)
		fmt.Printf("\npreceding frames, for context:\n")
		for j := max(0, i-4); j < i; j++ {
			describe(" ", framesA[j])
		}
		return
	}
	if len(framesA) != len(framesB) {
		fmt.Printf("streams agree on the first %d frames but differ in length\n", limit)
		return
	}
	fmt.Println("streams are identical")
}

// classify answers the question a first divergence raises but does not settle:
// whether the difference stays in one field or reaches the economics. Payloads
// that are opaque JSON are compared key by key, so the report names which
// fields ever differ rather than only that the streams do.
func classify(a, b []frameRecord, limit int) {
	differing := 0
	fields := map[string]int{}
	for i := 0; i < limit; i++ {
		if bytes.Equal(a[i].payload, b[i].payload) {
			continue
		}
		differing++
		left, okA := decodeOpaque(a[i].payload)
		right, okB := decodeOpaque(b[i].payload)
		if !okA || !okB {
			fields["<not opaque JSON>"]++
			continue
		}
		for key, valueA := range left {
			if valueB, ok := right[key]; !ok || fmt.Sprint(valueA) != fmt.Sprint(valueB) {
				fields[key]++
			}
		}
		for key := range right {
			if _, ok := left[key]; !ok {
				fields[key]++
			}
		}
	}
	fmt.Printf("\n%d of %d frames differ\n", differing, limit)
	fmt.Println("fields that ever differ:")
	for key, count := range fields {
		fmt.Printf("  %-20s %d\n", key, count)
	}
	fmt.Println()
}

// decodeOpaque unwraps the sink envelope (an interned event-name ref) and the
// opaque JSON length prefix.
func decodeOpaque(payload []byte) (map[string]any, bool) {
	if len(payload) < 8 {
		return nil, false
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload[8:], &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}

func describe(tag string, f frameRecord) {
	fmt.Printf("%s seq=%d simTS=%d client=%d schema=%d venue=%q payload=%x\n",
		tag, f.seq, f.simTS, f.clientID, f.schema, f.venue, f.payload)
}

func load(path string) []frameRecord {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("evsdiff: open %s: %v", path, err)
	}
	defer file.Close()
	reader, err := evstream.NewReader(file, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		log.Fatalf("evsdiff: reader %s: %v", path, err)
	}
	var records []frameRecord
	if err := reader.Range(func(frame evstream.Frame) error {
		venue, _ := reader.Lookup(frame.Header.VenueRef)
		records = append(records, frameRecord{
			seq: frame.Header.Seq, simTS: frame.Header.SimTS,
			clientID: frame.Header.ClientID, schema: frame.Header.SchemaID,
			venue: venue, payload: append([]byte(nil), frame.Payload...),
		})
		return nil
	}); err != nil {
		log.Fatalf("evsdiff: read %s: %v", path, err)
	}
	return records
}
