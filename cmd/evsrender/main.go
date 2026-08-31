// Command evsrender reconstructs the venue JSONL records from a binary
// evidence stream.
//
// It exists to answer the question that decides whether the binary format can
// replace the raw log: are the typed frames enough to rebuild the JSON, exactly,
// or only nearly. "Nearly" is a failure — the records are evidence.
//
// Output is one record per line in the venue logger's own format, in global
// event order, which the JSONL files do not have because they are split per
// venue and per symbol.
//
//	evsrender -dir <logdir> > rendered.jsonl
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
)

func main() {
	dir := flag.String("dir", "", "run directory holding events.evs")
	flag.Parse()
	if *dir == "" {
		log.Fatal("evsrender: -dir is required")
	}
	file, err := os.Open(filepath.Join(*dir, "events.evs"))
	if err != nil {
		log.Fatalf("evsrender: open: %v", err)
	}
	defer file.Close()

	reader, err := evstream.NewReader(file, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		log.Fatalf("evsrender: reader: %v", err)
	}
	out := bufio.NewWriterSize(os.Stdout, 1<<20)
	defer out.Flush()

	if err := reader.Range(func(frame evstream.Frame) error {
		return renderFrame(out, reader, frame)
	}); err != nil {
		log.Fatalf("evsrender: render: %v", err)
	}
}

// renderFrame rebuilds one venue log record. The sink envelope prefixes every
// payload with an interned event-name reference, because the frame header
// carries the schema but not the name, and one schema serves nine names.
func renderFrame(out *bufio.Writer, reader *evstream.Reader, frame evstream.Frame) error {
	resolve := reader.Lookup
	if frame.Header.SchemaID == evstream.SchemaDictionary {
		return nil
	}
	if len(frame.Payload) < 4 {
		return fmt.Errorf("frame %d: payload too short for the event name reference", frame.Header.Seq)
	}
	eventRef := uint32(frame.Payload[0]) | uint32(frame.Payload[1])<<8 |
		uint32(frame.Payload[2])<<16 | uint32(frame.Payload[3])<<24
	eventName, ok := resolve(eventRef)
	if !ok {
		return fmt.Errorf("frame %d: event name ref %d never defined", frame.Header.Seq, eventRef)
	}
	venueID := ""
	if frame.Header.VenueRef != 0 {
		if venueID, ok = resolve(frame.Header.VenueRef); !ok {
			return fmt.Errorf("frame %d: venue ref %d never defined", frame.Header.Seq, frame.Header.VenueRef)
		}
	}
	payload, err := exchange.RenderPayloadJSON(frame.Header.SchemaID, frame.Payload[4:], reader)
	if err != nil {
		return fmt.Errorf("frame %d (%s): %w", frame.Header.Seq, eventName, err)
	}

	// The field order is the venue logger's, which is what the JSONL files
	// hold; a differential comparison against them is byte-level, so this is
	// assembled rather than marshalled from a map.
	out.WriteString(`{"client_id":`)
	fmt.Fprintf(out, "%d", frame.Header.ClientID)
	out.WriteString(`,"data":{"venue_id":`)
	encodedVenue, err := json.Marshal(venueID)
	if err != nil {
		return err
	}
	out.Write(encodedVenue)
	out.WriteString(`,"payload":`)
	out.Write(payload)
	out.WriteString(`},"event":`)
	encodedName, err := json.Marshal(eventName)
	if err != nil {
		return err
	}
	out.Write(encodedName)
	out.WriteString(`,"sim_ts":`)
	fmt.Fprintf(out, "%d", frame.Header.SimTS)
	out.WriteString("}\n")
	return nil
}
