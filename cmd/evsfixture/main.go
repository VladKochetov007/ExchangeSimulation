// Command evsfixture creates a small, valid evstream_v3 fixture for shell
// contract tests. It exercises the same envelope, dictionary, completion, and
// route-neutral hash rules as the production binary sink without running a
// market world.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash"
	"os"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
)

const binaryExecutionHashContract = "route_sequence_neutral_v1"

type fixtureEnvelope struct {
	routeRef uint32
	eventRef uint32
	sequence uint64
	payload  exchange.OpaqueJSON
}

func (e fixtureEnvelope) SchemaID() uint16      { return e.payload.SchemaID() }
func (e fixtureEnvelope) SchemaVersion() uint16 { return e.payload.SchemaVersion() }

func (e fixtureEnvelope) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint32(dst, e.routeRef)
	dst = evstream.AppendUint32(dst, e.eventRef)
	dst = evstream.AppendUint64(dst, e.sequence)
	return e.payload.AppendPayloadInterning(dst, in)
}

func hashBinaryExecutionFrame(digest hash.Hash, frame []byte) {
	header, err := evstream.ParseFrameHeader(frame)
	if err != nil || header.SchemaID == evstream.SchemaDictionary || len(frame) < evstream.FrameHeaderSize+16 {
		_, _ = digest.Write(frame)
		return
	}
	sequenceStart := evstream.FrameHeaderSize + 8
	sequenceEnd := sequenceStart + 8
	_, _ = digest.Write(frame[:sequenceStart])
	var zeroSequence [8]byte
	_, _ = digest.Write(zeroSequence[:])
	_, _ = digest.Write(frame[sequenceEnd:])
}

type fixtureReport struct {
	Domain                       string `json:"domain"`
	Ordering                     string `json:"ordering"`
	Hashing                      string `json:"hashing"`
	EventFrames                  uint64 `json:"event_frames"`
	StreamFrames                 uint64 `json:"stream_frames"`
	ExecutionStreamHash          string `json:"execution_stream_hash"`
	CanonicalExecutionStreamHash string `json:"canonical_execution_stream_hash"`
	UnencodablePayloads          uint64 `json:"unencodable_payloads"`
}

func main() {
	outputPath := flag.String("out", "", "path for the evstream fixture")
	flag.Parse()
	if *outputPath == "" {
		fmt.Fprintln(os.Stderr, "usage: evsfixture -out PATH")
		os.Exit(2)
	}

	output, err := os.Create(*outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create fixture: %v\n", err)
		os.Exit(1)
	}
	writer := evstream.NewWriter(output, evstream.WriterOptions{HashFrame: hashBinaryExecutionFrame})
	eventRef, err := writer.Intern("terminal_fixture")
	if err == nil {
		var venueRef uint32
		venueRef, err = writer.Intern("north")
		if err == nil {
			var routeRef uint32
			routeRef, err = writer.Intern("general.jsonl")
			if err == nil {
				err = writer.AppendInterning(1735689900000000000, 7, venueRef, fixtureEnvelope{
					routeRef: routeRef,
					eventRef: eventRef,
					sequence: 1,
					payload:  exchange.OpaqueJSON{Value: map[string]int{"terminal_fixture": 1}},
				})
			}
		}
	}
	if err == nil {
		err = writer.Close()
	}
	if closeErr := output.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "write fixture: %v\n", err)
		os.Exit(1)
	}

	executionHash := writer.ExecutionHash()
	canonicalHash := writer.RawExecutionHash()
	report := fixtureReport{
		Domain:                       "canonical_binary_execution_frames",
		Ordering:                     "ordered_stream",
		Hashing:                      binaryExecutionHashContract,
		EventFrames:                  1,
		StreamFrames:                 writer.Count(),
		ExecutionStreamHash:          hex.EncodeToString(executionHash[:]),
		CanonicalExecutionStreamHash: hex.EncodeToString(canonicalHash[:]),
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "write fixture report: %v\n", err)
		os.Exit(1)
	}
}
