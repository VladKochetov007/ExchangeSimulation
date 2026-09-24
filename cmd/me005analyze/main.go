package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/crossvenue"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("me005analyze", flag.ContinueOnError)
	rawDir := flags.String("raw", "", "completed immutable binary run")
	renderedDir := flags.String("rendered", "", "new empty rendered-evidence directory")
	contractPath := flags.String("contract", "", "pinned one-world ME-005 contract JSON")
	outputPath := flags.String("out", "", "new machine-readable result file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *rawDir == "" || *renderedDir == "" || *contractPath == "" || *outputPath == "" || flags.NArg() != 0 {
		return fmt.Errorf("me005analyze: -raw, -rendered, -contract and -out are required")
	}
	if err := crossvenue.ValidateResultPath(*rawDir, *renderedDir, *outputPath); err != nil {
		return err
	}
	contractRaw, err := os.ReadFile(*contractPath)
	if err != nil {
		return err
	}
	contract, err := crossvenue.DecodeContract(contractRaw)
	if err != nil {
		return err
	}
	result, err := crossvenue.AnalyzeCompletedWorld(*rawDir, *renderedDir, contract)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(*outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("me005analyze: new result: %w", err)
	}
	encoded = append(encoded, '\n')
	written, err := file.Write(encoded)
	if err != nil || written != len(encoded) {
		_ = file.Close()
		return fmt.Errorf("me005analyze: incomplete result write: %d/%d bytes: %v", written, len(encoded), err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
