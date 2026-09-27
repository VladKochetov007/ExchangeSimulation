package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestME016CommandRejectsUnregisteredAndAmbiguousPlansBeforeStartup(t *testing.T) {
	for _, arguments := range [][]string{
		{"cells", "-study", "ME-unknown"},
		{"plan", "-study", "ME-016", "-repo", t.TempDir(), "-analyzer", "missing",
			"-out", filepath.Join(t.TempDir(), "plan.json"), "-composition", "M1", "-seed", "18101"},
		{"plan", "-study", "ME-016", "-repo", t.TempDir(), "-analyzer", "missing",
			"-out", filepath.Join(t.TempDir(), "plan.json"), "-composition", "M1", "-seed", "18101",
			"-signal-gain-bps", "2", "-quote-qty", "1"},
		{"run", "-study", "ME-unknown", "-repo", t.TempDir(), "-analyzer", "missing",
			"-plan", "missing", "-out", filepath.Join(t.TempDir(), "run"), "-timeout", "1s"},
	} {
		if err := run(arguments); err == nil {
			t.Fatalf("invalid ME-016 command unexpectedly succeeded: %v", arguments)
		}
	}
}

func TestME016RunnerProcessExitsNonzeroBeforeStartupOnMissingLockedPlan(t *testing.T) {
	output := filepath.Join(t.TempDir(), "must-not-exist")
	command := exec.Command("go", "run", ".", "run", "-study", "ME-016", "-repo", t.TempDir(),
		"-analyzer", "missing", "-plan", "missing-plan.json", "-out", output, "-timeout", "1s")
	outputBytes, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("missing locked plan did not propagate process failure: %v, %s", err, outputBytes)
	}
}
