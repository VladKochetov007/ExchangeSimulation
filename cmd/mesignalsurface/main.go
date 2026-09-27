package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"exchange_sim/experiment/repeatedspot"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "mesignalsurface:", err)
		os.Exit(1)
	}
}

func run() error {
	root := flag.String("root", "", "retained ME-016 development root")
	output := flag.String("out", "", "new exclusive machine surface path")
	firstAnalysisResource := flag.String("first-analysis-resource", "", "retained successful resource record after first-cell analysis retry")
	flag.Parse()
	if *root == "" || *output == "" || *firstAnalysisResource == "" || flag.NArg() != 0 {
		return errors.New("required: -root -out -first-analysis-resource")
	}
	surface, err := repeatedspot.BuildME016DevelopmentSurface(*root, map[string]string{
		"ME016-M1-g0-s18101": *firstAnalysisResource,
	})
	if err != nil {
		return err
	}
	return repeatedspot.WriteME016Surface(*output, surface)
}
