// Command evstream-fixture creates the smallest complete evstream used by the
// shell-level R2 archive contract test. It is test infrastructure, not a
// simulator evidence producer.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"exchange_sim/evstream"
)

type fixturePayload struct{}

func (fixturePayload) SchemaID() uint16      { return evstream.FirstUserSchema }
func (fixturePayload) SchemaVersion() uint16 { return 1 }
func (fixturePayload) AppendPayload(dst []byte) []byte {
	return evstream.AppendString(dst, "archive-fixture")
}

type fixtureEnvelope struct {
	routeRef uint32
	eventRef uint32
	sequence uint64
}

func (fixtureEnvelope) SchemaID() uint16      { return evstream.SchemaOpaqueJSON }
func (fixtureEnvelope) SchemaVersion() uint16 { return 1 }
func (envelope fixtureEnvelope) AppendPayloadInterning(dst []byte, _ evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint32(dst, envelope.routeRef)
	dst = evstream.AppendUint32(dst, envelope.eventRef)
	dst = evstream.AppendUint64(dst, envelope.sequence)
	return evstream.AppendBytes(dst, []byte(`{"fixture":true}`)), nil
}

func main() {
	outputPath := flag.String("out", "", "output evstream path")
	attestationPath := flag.String("attestation", "", "optional binary attestation path")
	sequence := flag.Uint64("sequence", 1, "venue-local sequence in the envelope")
	flag.Parse()
	if *outputPath == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	file, err := os.Create(*outputPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	writer := evstream.NewWriter(file, evstream.WriterOptions{})
	routeRef, err := writer.Intern("general.jsonl")
	if err != nil {
		_ = file.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	eventRef, err := writer.Intern("archive-test")
	if err != nil {
		_ = file.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writer.AppendInterning(1735689600000000000, 607, 0, fixtureEnvelope{routeRef: routeRef, eventRef: eventRef, sequence: *sequence}); err != nil {
		_ = file.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *attestationPath != "" {
		digest := writer.ExecutionHash()
		attestation, err := json.MarshalIndent(struct {
			Domain              string `json:"domain"`
			Ordering            string `json:"ordering"`
			EventFrames         uint64 `json:"event_frames"`
			StreamFrames        uint64 `json:"stream_frames"`
			ExecutionStreamHash string `json:"execution_stream_hash"`
		}{
			Domain: "canonical_binary_execution_frames", Ordering: "ordered_stream",
			EventFrames: 1, StreamFrames: writer.Count(), ExecutionStreamHash: hex.EncodeToString(digest[:]),
		}, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(*attestationPath, append(attestation, '\n'), 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
