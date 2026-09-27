package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"exchange_sim/experiment/repeatedspot"
)

func main() {
	manifestPath := flag.String("manifest", "", "completed E0 run manifest")
	resultPath := flag.String("result", "", "accepted strict replay result")
	evidencePath := flag.String("evidence", "", "retained canonical evidence stream")
	outputPath := flag.String("output", "", "new diagnostic JSON path")
	measurementStart := flag.Int64("measurement-start-ns", -1, "registered measurement start")
	measurementEnd := flag.Int64("measurement-end-ns", -1, "exclusive registered measurement end")
	tickSize := flag.Int64("tick-size", 0, "registered instrument tick size")
	flag.Parse()
	if *manifestPath == "" || *resultPath == "" || *evidencePath == "" || *outputPath == "" ||
		*measurementStart < 0 || *measurementEnd <= *measurementStart || *tickSize <= 0 || flag.NArg() != 0 {
		fail(errors.New("required: -manifest -result -evidence -output -measurement-start-ns -measurement-end-ns -tick-size"))
	}
	manifestRaw, err := os.ReadFile(*manifestPath)
	if err != nil {
		fail(err)
	}
	var manifest repeatedspot.E0RunManifest
	if err := decodeJSON(manifestRaw, &manifest); err != nil {
		fail(err)
	}
	resultRaw, err := os.ReadFile(*resultPath)
	if err != nil {
		fail(err)
	}
	var result repeatedspot.E0AnalyzedResult
	if err := decodeJSON(resultRaw, &result); err != nil {
		fail(err)
	}
	manifestHash := sha256.Sum256(manifestRaw)
	if result.EconomicReconstruction == nil ||
		result.ManifestFileSHA256 != hex.EncodeToString(manifestHash[:]) ||
		result.EvidenceFileSHA256 != manifest.EvidenceFileSHA256 ||
		result.Identity != manifest.Identity || result.Cell != manifest.Cell ||
		result.EconomicReconstruction.Evidence != manifest.Evidence ||
		result.EconomicReconstruction.ContractSHA256 != manifest.ContractSHA256 ||
		result.EconomicReconstruction.MeasurementWindow != (repeatedspot.MeasurementWindow{StartAt: *measurementStart, EndAt: *measurementEnd}) {
		fail(errors.New("strict replay result does not bind the retained manifest and evidence"))
	}
	input, err := os.Open(*evidencePath)
	if err != nil {
		fail(err)
	}
	defer input.Close()
	rawHasher := sha256.New()
	diagnostic, err := repeatedspot.DiagnoseE0LiquidityEvidence(io.TeeReader(input, rawHasher), manifest.Evidence,
		repeatedspot.MeasurementWindow{StartAt: *measurementStart, EndAt: *measurementEnd}, *tickSize)
	if err != nil {
		fail(err)
	}
	if hex.EncodeToString(rawHasher.Sum(nil)) != manifest.EvidenceFileSHA256 {
		fail(errors.New("raw evidence file digest differs from retained manifest"))
	}
	market := result.EconomicReconstruction.Market
	book := diagnostic.BookDurations
	if book.HorizonNanos != market.HorizonNanos || book.TwoSidedNanos != market.TwoSidedNanos ||
		book.BidOnlyNanos != market.BidOnlyNanos || book.AskOnlyNanos != market.AskOnlyNanos ||
		book.EmptyNanos != market.EmptyNanos || diagnostic.TradeCount != int64(result.EconomicReconstruction.TradeCount) {
		fail(errors.New("diagnostic disagrees with accepted strict replay"))
	}
	output, err := os.OpenFile(*outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(diagnostic)
	closeErr := output.Close()
	if writeErr != nil {
		fail(writeErr)
	}
	if closeErr != nil {
		fail(closeErr)
	}
}

func decodeJSON(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON content")
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
