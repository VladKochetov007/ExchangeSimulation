// sv1dresource measures one capacity-preflight command. It is an adapter only;
// resource aggregation and filesystem/process inspection live in analysis.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"exchange_sim/analysis"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sv1dresource:", err)
		os.Exit(1)
	}
}

func run() error {
	inspectFilesystem := flag.String("inspect-filesystem", "", "print the filesystem identity for PATH and exit")
	out := flag.String("out", "", "new resource-measurement JSON path")
	outputParent := flag.String("output-parent", "", "filesystem path whose free space is measured")
	measurementRoot := flag.String("measurement-root", "", "fresh output tree measured for footprint")
	interval := flag.Duration("sample-interval", 250*time.Millisecond, "resource sampling interval")
	requireFiniteCgroup := flag.Bool("require-finite-cgroup", true, "reject an unbounded cgroup memory limit")
	requireChildHandoff := flag.Bool("require-child-handoff", false, "pass a one-shot parent handoff on child file descriptor 3")
	flag.Parse()
	if *inspectFilesystem != "" {
		identity, err := analysis.InspectSV1DFilesystem(*inspectFilesystem)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(identity)
	}
	command := flag.Args()
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	if *out == "" || *outputParent == "" || *measurementRoot == "" || len(command) == 0 {
		return errors.New("usage: sv1dresource -out PATH -output-parent PATH -measurement-root PATH [options] -- command [args...]")
	}
	measurement, measurementErr := analysis.MeasureSV1DCommand(context.Background(), analysis.SV1DResourceOptions{
		Command:                  command,
		OutputParent:             *outputParent,
		MeasurementRoot:          *measurementRoot,
		SampleInterval:           *interval,
		RequireFiniteCgroupLimit: *requireFiniteCgroup,
		RequireChildHandoff:      *requireChildHandoff,
	})
	if err := publishMeasurement(*out, measurement); err != nil {
		if measurementErr != nil {
			return errors.Join(measurementErr, fmt.Errorf("publish resource measurement: %w", err))
		}
		return fmt.Errorf("publish resource measurement: %w", err)
	}
	if measurementErr != nil {
		return measurementErr
	}
	if *requireFiniteCgroup {
		if measurementErr := analysis.ValidateSV1DResourceMeasurement(measurement, true); measurementErr != nil {
			return measurementErr
		}
	}
	if !measurement.Complete {
		return fmt.Errorf("measured command was incomplete: exit status %d", measurement.ExitStatus)
	}
	return nil
}

func publishMeasurement(path string, measurement analysis.SV1DResourceMeasurement) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if filepath.Clean(absolute) != absolute || absolute == string(filepath.Separator) || strings.ContainsRune(absolute, '\x00') {
		return fmt.Errorf("measurement output path is not clean and absolute: %s", path)
	}
	if err := requireNoSymlinkComponents(absolute); err != nil {
		return err
	}
	directory := filepath.Dir(absolute)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	if err := requireNoSymlinkComponents(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".sv1dresource-output-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(measurement); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryName, absolute); err != nil {
		return fmt.Errorf("publish without overwrite: %w", err)
	}
	return nil
}

func requireNoSymlinkComponents(path string) error {
	current := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(path, current), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains a symlink: %s", path)
		}
	}
	return nil
}
