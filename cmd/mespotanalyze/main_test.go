package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestME016AnalyzerCommandPropagatesMissingEvidenceError(t *testing.T) {
	if err := run([]string{"-study", "ME-016", "-repo", t.TempDir(), "-simulator", "missing",
		"-plan", "missing", "-run", "missing", "-out", t.TempDir() + "/result.json"}); err == nil {
		t.Fatal("missing ME-016 plan/evidence was treated as completed analysis")
	}
}

func TestME016AnalyzerProcessExitsNonzeroOnIncompleteOutput(t *testing.T) {
	command := exec.Command("go", "run", ".", "-study", "ME-016", "-repo", t.TempDir(),
		"-simulator", "missing", "-plan", "missing-plan.json", "-run", "missing-run",
		"-out", filepath.Join(t.TempDir(), "must-not-exist.json"))
	outputBytes, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("incomplete ME-016 analysis did not propagate process failure: %v, %s", err, outputBytes)
	}
}
