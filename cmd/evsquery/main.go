// Command evsquery measures selective queries against a stream the simulator
// actually produced, using its persisted block index.
//
// Every earlier query figure came either from a synthetic corpus or from a
// stream converted out of JSONL for the purpose. This reads `events.evs` and
// `events.evx` exactly as written by a real run, so the block boundaries, the
// family interleaving, the dictionary and the time distribution are the ones
// the simulator produces rather than ones chosen to suit the format.
//
//	evsquery -dir <logdir>
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"exchange_sim/evstream"
)

func main() {
	dir := flag.String("dir", "", "run directory holding events.evs and events.evx")
	flag.Parse()
	if *dir == "" {
		log.Fatal("evsquery: -dir is required")
	}

	streamPath := filepath.Join(*dir, "events.evs")
	stream, err := os.Open(streamPath)
	if err != nil {
		log.Fatalf("evsquery: open stream: %v", err)
	}
	defer stream.Close()
	info, err := stream.Stat()
	if err != nil {
		log.Fatalf("evsquery: stat stream: %v", err)
	}

	indexFile, err := os.Open(filepath.Join(*dir, "events.evx"))
	if err != nil {
		log.Fatalf("evsquery: open index: %v", err)
	}
	index, err := evstream.ReadIndex(indexFile)
	indexFile.Close()
	if err != nil {
		log.Fatalf("evsquery: read index: %v", err)
	}

	// A full sequential pass first: it establishes the frame count, verifies
	// the digest and the gap-free sequence, and gives the baseline every
	// selective query is measured against.
	dictionary, frames, minTS, maxTS, fullElapsed := fullScan(streamPath)

	fmt.Printf("stream          %s\n", streamPath)
	fmt.Printf("bytes           %d\n", info.Size())
	fmt.Printf("frames          %d\n", frames)
	fmt.Printf("blocks          %d\n", len(index.Blocks))
	fmt.Printf("full scan       %v  (verified digest and gap-free sequence)\n\n",
		fullElapsed.Round(time.Millisecond))

	span := maxTS - minTS
	windows := []struct {
		name     string
		fraction int64
	}{
		{"1% of the run", 100},
		{"5% of the run", 20},
		{"25% of the run", 4},
	}

	fmt.Printf("%-18s %12s %12s %10s %s\n", "window", "elapsed", "matched", "speedup", "blocks read")
	for _, window := range windows {
		from := minTS + span/2
		to := from + span/window.fraction
		query := evstream.Query{FromTS: from, ToTS: to}
		selected, skipped := index.Select(query)

		matched, elapsed := selectiveScan(streamPath, dictionary, selected, query)
		fmt.Printf("%-18s %12v %12d %9.1fx %d of %d\n", window.name,
			elapsed.Round(time.Microsecond), matched,
			float64(fullElapsed)/float64(elapsed),
			len(selected), len(selected)+skipped)
	}
}

// fullScan reads every frame, which is both the causal-replay path and the way
// the dictionary is learned. A selective reader needs the dictionary, and ids
// are learned by reading in order, so one sequential pass precedes any number
// of selective ones.
func fullScan(path string) (*evstream.Dictionary, uint64, int64, int64, time.Duration) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("evsquery: open: %v", err)
	}
	defer file.Close()

	started := time.Now()
	reader, err := evstream.NewReader(file, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		log.Fatalf("evsquery: reader: %v", err)
	}
	dictionary := evstream.NewDictionary()
	minTS, maxTS := int64(1)<<62, -(int64(1) << 62)
	frames := uint64(0)
	if err := reader.Range(func(frame evstream.Frame) error {
		frames++
		if frame.Header.SimTS < minTS {
			minTS = frame.Header.SimTS
		}
		if frame.Header.SimTS > maxTS {
			maxTS = frame.Header.SimTS
		}
		return nil
	}); err != nil {
		log.Fatalf("evsquery: full scan: %v", err)
	}
	elapsed := time.Since(started)

	// Rebuild the dictionary from the reader so selective scans can resolve
	// references in blocks they skip past.
	for id := uint32(1); ; id++ {
		value, ok := reader.Lookup(id)
		if !ok {
			break
		}
		if err := dictionary.Define(id, value); err != nil {
			break
		}
	}
	return dictionary, frames, minTS, maxTS, elapsed
}

func selectiveScan(path string, dictionary *evstream.Dictionary,
	blocks []evstream.BlockDescriptor, query evstream.Query) (int, time.Duration) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("evsquery: reopen: %v", err)
	}
	defer file.Close()

	started := time.Now()
	reader := evstream.NewIndexedReader(file, evstream.CodecNone, dictionary, nil)
	matched := 0
	if err := reader.RangeSelected(blocks, query, func(evstream.Frame) error {
		matched++
		return nil
	}); err != nil {
		log.Fatalf("evsquery: selective scan: %v", err)
	}
	return matched, time.Since(started)
}
