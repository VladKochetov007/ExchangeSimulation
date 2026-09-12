package analysis

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	SV1DResourceMeasurementContract = "v2-r2-sv1d-resource-measurement-v1"
	SV1DResourceDefaultIntervalNano = uint64(250_000_000)
)

// SV1DFilesystemIdentity is the immutable filesystem identity used to bind a
// capacity measurement to the filesystem that will hold the scientific run.
type SV1DFilesystemIdentity struct {
	Device  string `json:"device"`
	ID      string `json:"id"`
	Type    string `json:"type"`
	MountID string `json:"mount_id"`
	UUID    string `json:"uuid"`
}

// SV1DResourceSample is one wall-clock observation. The sample trace is
// retained so a verifier can recompute every aggregate in the measurement.
type SV1DResourceSample struct {
	ObservedAtUnixNano       int64  `json:"observed_at_unix_nano"`
	AvailableBytes           uint64 `json:"available_bytes"`
	ApparentBytes            uint64 `json:"apparent_bytes"`
	AllocatedBytes           uint64 `json:"allocated_bytes"`
	ProcessTreeRSSBytes      uint64 `json:"process_tree_rss_bytes"`
	CgroupCurrentBytes       uint64 `json:"cgroup_current_bytes"`
	CgroupMemoryLimitBytes   uint64 `json:"cgroup_memory_limit_bytes"`
	HostMemAvailableBytes    uint64 `json:"host_mem_available_bytes"`
	SwapUsedBytes            uint64 `json:"swap_used_bytes"`
	CgroupOOMEvents          uint64 `json:"cgroup_oom_events"`
	CgroupOOMKillEvents      uint64 `json:"cgroup_oom_kill_events"`
	CgroupLocalOOMEvents     uint64 `json:"cgroup_local_oom_events"`
	CgroupLocalOOMKillEvents uint64 `json:"cgroup_local_oom_kill_events"`
}

// SV1DResourceMeasurement is a reproducible observation of one completed or
// failed capacity arm. It is not scientific outcome evidence.
type SV1DResourceMeasurement struct {
	SchemaVersion                  int                    `json:"schema_version"`
	Contract                       string                 `json:"contract"`
	Command                        []string               `json:"command"`
	OutputParent                   string                 `json:"output_parent"`
	MeasurementRoot                string                 `json:"measurement_root"`
	Filesystem                     SV1DFilesystemIdentity `json:"filesystem"`
	SampleIntervalNano             uint64                 `json:"sample_interval_nano"`
	ExitStatus                     int                    `json:"exit_status"`
	Complete                       bool                   `json:"complete"`
	Error                          string                 `json:"error,omitempty"`
	InitialAvailableBytes          uint64                 `json:"initial_available_bytes"`
	MinimumAvailableBytes          uint64                 `json:"minimum_available_bytes"`
	FinalAvailableBytes            uint64                 `json:"final_available_bytes"`
	PeakApparentBytes              uint64                 `json:"peak_apparent_bytes"`
	PeakAllocatedBytes             uint64                 `json:"peak_allocated_bytes"`
	PeakFilesystemConsumptionBytes uint64                 `json:"peak_filesystem_consumption_bytes"`
	PeakProcessTreeRSSBytes        uint64                 `json:"peak_process_tree_rss_bytes"`
	PeakCgroupMemoryBytes          uint64                 `json:"peak_cgroup_memory_bytes"`
	CgroupMemoryLimitBytes         uint64                 `json:"cgroup_memory_limit_bytes"`
	MinimumHostMemAvailableBytes   uint64                 `json:"minimum_host_mem_available_bytes"`
	MaximumSwapUsedBytes           uint64                 `json:"maximum_swap_used_bytes"`
	MaximumSampleGapNano           uint64                 `json:"maximum_sample_gap_nano"`
	SampleCount                    uint64                 `json:"sample_count"`
	SamplesSHA256                  string                 `json:"samples_sha256"`
	CgroupOOMEventsDelta           uint64                 `json:"cgroup_oom_events_delta"`
	CgroupOOMKillEventsDelta       uint64                 `json:"cgroup_oom_kill_events_delta"`
	CgroupLocalOOMDelta            uint64                 `json:"cgroup_local_oom_events_delta"`
	CgroupLocalOOMKillDelta        uint64                 `json:"cgroup_local_oom_kill_events_delta"`
	Samples                        []SV1DResourceSample   `json:"samples"`
}

// SV1DResourceOptions configures one measured child command.
type SV1DResourceOptions struct {
	Command         []string
	OutputParent    string
	MeasurementRoot string
	// SampleInterval is the maximum permitted gap between observed samples.
	// The measurer derives a shorter deadline cadence so filesystem, process,
	// and cgroup collection time cannot consume the entire contract interval.
	SampleInterval           time.Duration
	RequireFiniteCgroupLimit bool
	InheritedFileDescriptors []int
}

// ValidateSV1DResourceMeasurement recomputes the retained resource aggregates
// from the sample trace. A finite cgroup limit is optional for generic local
// diagnostics but mandatory for the capacity-preflight caller.
func ValidateSV1DResourceMeasurement(measurement SV1DResourceMeasurement, requireFiniteCgroup bool) error {
	if measurement.SchemaVersion != 1 || measurement.Contract != SV1DResourceMeasurementContract || len(measurement.Command) == 0 || measurement.SampleIntervalNano == 0 || !absoluteCleanPath(measurement.OutputParent) || !absoluteCleanPath(measurement.MeasurementRoot) || len(measurement.Samples) < 2 || measurement.SampleCount != uint64(len(measurement.Samples)) || !measurement.Complete || measurement.ExitStatus != 0 || measurement.Error != "" {
		return errors.New("resource measurement has an invalid contract, completion state, paths, or sample count")
	}
	if measurement.Filesystem.Device == "" || measurement.Filesystem.ID == "" || measurement.Filesystem.Type == "" || measurement.Filesystem.MountID == "" || measurement.Filesystem.UUID == "" {
		return errors.New("resource measurement has incomplete filesystem identity")
	}
	observedOutputFilesystem, err := InspectSV1DFilesystem(measurement.OutputParent)
	if err != nil {
		return fmt.Errorf("inspect measured output filesystem: %w", err)
	}
	observedMeasurementFilesystem, err := InspectSV1DFilesystem(measurement.MeasurementRoot)
	if err != nil {
		return fmt.Errorf("inspect measured tree filesystem: %w", err)
	}
	if !sameSV1DFilesystem(observedOutputFilesystem, observedMeasurementFilesystem) || !sameSV1DFilesystem(measurement.Filesystem, observedOutputFilesystem) {
		return errors.New("resource measurement filesystem identity changed or differs between paths")
	}
	if measurement.MinimumAvailableBytes > measurement.InitialAvailableBytes || measurement.FinalAvailableBytes == 0 || measurement.MinimumHostMemAvailableBytes == 0 || measurement.MaximumSwapUsedBytes != 0 || measurement.CgroupOOMEventsDelta != 0 || measurement.CgroupOOMKillEventsDelta != 0 || measurement.CgroupLocalOOMDelta != 0 || measurement.CgroupLocalOOMKillDelta != 0 {
		return errors.New("resource measurement has unsafe free-space, memory, swap, or OOM values")
	}
	var minimumAvailable, peakApparent, peakAllocated, peakRSS, peakCgroup, minimumHost uint64
	var maximumSwap, maximumGap uint64
	var cgroupLimit uint64
	for index, sample := range measurement.Samples {
		if sample.ObservedAtUnixNano <= 0 || (index > 0 && sample.ObservedAtUnixNano <= measurement.Samples[index-1].ObservedAtUnixNano) {
			return errors.New("resource measurement samples are not strictly ordered")
		}
		if index == 0 {
			minimumAvailable, peakApparent, peakAllocated = sample.AvailableBytes, sample.ApparentBytes, sample.AllocatedBytes
			peakRSS, peakCgroup, minimumHost, maximumSwap = sample.ProcessTreeRSSBytes, sample.CgroupCurrentBytes, sample.HostMemAvailableBytes, sample.SwapUsedBytes
			cgroupLimit = sample.CgroupMemoryLimitBytes
		} else {
			gap := uint64(sample.ObservedAtUnixNano - measurement.Samples[index-1].ObservedAtUnixNano)
			if gap > maximumGap {
				maximumGap = gap
			}
			if gap > measurement.SampleIntervalNano {
				return errors.New("resource measurement sample gap exceeds the registered interval")
			}
			if sample.AvailableBytes < minimumAvailable {
				minimumAvailable = sample.AvailableBytes
			}
			if sample.ApparentBytes > peakApparent {
				peakApparent = sample.ApparentBytes
			}
			if sample.AllocatedBytes > peakAllocated {
				peakAllocated = sample.AllocatedBytes
			}
			if sample.ProcessTreeRSSBytes > peakRSS {
				peakRSS = sample.ProcessTreeRSSBytes
			}
			if sample.CgroupCurrentBytes > peakCgroup {
				peakCgroup = sample.CgroupCurrentBytes
			}
			if sample.HostMemAvailableBytes < minimumHost {
				minimumHost = sample.HostMemAvailableBytes
			}
			if sample.SwapUsedBytes > maximumSwap {
				maximumSwap = sample.SwapUsedBytes
			}
		}
		if sample.CgroupMemoryLimitBytes != cgroupLimit {
			return errors.New("resource measurement cgroup memory limit changed during sampling")
		}
	}
	if measurement.InitialAvailableBytes != measurement.Samples[0].AvailableBytes || measurement.MinimumAvailableBytes != minimumAvailable || measurement.FinalAvailableBytes != measurement.Samples[len(measurement.Samples)-1].AvailableBytes || measurement.PeakApparentBytes != peakApparent || measurement.PeakAllocatedBytes != peakAllocated || measurement.PeakProcessTreeRSSBytes != peakRSS || measurement.PeakCgroupMemoryBytes != peakCgroup || measurement.CgroupMemoryLimitBytes != cgroupLimit || measurement.MinimumHostMemAvailableBytes != minimumHost || measurement.MaximumSwapUsedBytes != maximumSwap || measurement.MaximumSampleGapNano != maximumGap || measurement.PeakFilesystemConsumptionBytes != measurement.InitialAvailableBytes-minimumAvailable {
		return errors.New("resource measurement aggregate does not match its samples")
	}
	if measurement.PeakApparentBytes == 0 || measurement.PeakAllocatedBytes == 0 || measurement.PeakProcessTreeRSSBytes == 0 || measurement.SamplesSHA256 == "" {
		return errors.New("resource measurement omits a positive footprint or sample digest")
	}
	canonical, err := json.Marshal(measurement.Samples)
	if err != nil {
		return fmt.Errorf("marshal resource sample trace: %w", err)
	}
	digest := sha256.Sum256(canonical)
	if measurement.SamplesSHA256 != hex.EncodeToString(digest[:]) {
		return errors.New("resource measurement sample digest does not match its trace")
	}
	first, last := measurement.Samples[0], measurement.Samples[len(measurement.Samples)-1]
	if measurement.CgroupOOMEventsDelta != resourceCounterDelta(first.CgroupOOMEvents, last.CgroupOOMEvents) || measurement.CgroupOOMKillEventsDelta != resourceCounterDelta(first.CgroupOOMKillEvents, last.CgroupOOMKillEvents) || measurement.CgroupLocalOOMDelta != resourceCounterDelta(first.CgroupLocalOOMEvents, last.CgroupLocalOOMEvents) || measurement.CgroupLocalOOMKillDelta != resourceCounterDelta(first.CgroupLocalOOMKillEvents, last.CgroupLocalOOMKillEvents) {
		return errors.New("resource measurement OOM deltas do not match its trace")
	}
	if requireFiniteCgroup && (measurement.CgroupMemoryLimitBytes == 0 || measurement.PeakCgroupMemoryBytes > measurement.CgroupMemoryLimitBytes) {
		return errors.New("resource measurement lacks a safe finite cgroup memory envelope")
	}
	return nil
}

func sameSV1DFilesystem(left, right SV1DFilesystemIdentity) bool {
	return left.Device == right.Device && left.ID == right.ID && left.Type == right.Type && left.MountID == right.MountID && left.UUID == right.UUID
}

// InspectSV1DFilesystem returns the mount identity for path. It resolves the
// longest matching mountpoint from mountinfo and binds a device UUID when the
// host exposes one.
func InspectSV1DFilesystem(path string) (SV1DFilesystemIdentity, error) {
	var identity SV1DFilesystemIdentity
	absolute, err := absoluteResourcePath(path)
	if err != nil {
		return identity, err
	}
	var stat syscall.Stat_t
	if err := syscall.Stat(absolute, &stat); err != nil {
		return identity, err
	}
	identity.ID = strconv.FormatUint(uint64(stat.Dev), 10)

	mount, err := findResourceMount(absolute)
	if err != nil {
		return identity, err
	}
	identity.Device = mount.source
	identity.Type = mount.filesystemType
	identity.MountID = strconv.FormatUint(mount.mountID, 10)
	identity.UUID = resourceFilesystemUUID(mount.source)
	if identity.Device == "" {
		identity.Device = identity.ID
	}
	if identity.Type == "" || identity.MountID == "" || identity.UUID == "" {
		return identity, fmt.Errorf("filesystem identity is incomplete for %s", absolute)
	}
	return identity, nil
}

// MeasureSV1DCommand runs command and samples its resource envelope until it
// exits. A non-zero child exit is represented in the returned record; only an
// inability to measure the command itself returns a non-nil error.
func MeasureSV1DCommand(ctx context.Context, options SV1DResourceOptions) (SV1DResourceMeasurement, error) {
	measurement := SV1DResourceMeasurement{
		SchemaVersion:      1,
		Contract:           SV1DResourceMeasurementContract,
		Command:            append([]string(nil), options.Command...),
		OutputParent:       options.OutputParent,
		MeasurementRoot:    options.MeasurementRoot,
		SampleIntervalNano: uint64(options.SampleInterval),
	}
	if len(options.Command) == 0 {
		return measurement, errors.New("resource measurement requires a command")
	}
	if options.SampleInterval <= 0 {
		options.SampleInterval = time.Duration(SV1DResourceDefaultIntervalNano)
		measurement.SampleIntervalNano = uint64(options.SampleInterval)
	}
	if _, err := absoluteResourcePath(options.OutputParent); err != nil {
		return measurement, fmt.Errorf("resource measurement output parent: %w", err)
	}
	if _, err := absoluteResourcePath(options.MeasurementRoot); err != nil {
		return measurement, fmt.Errorf("resource measurement root: %w", err)
	}
	filesystem, err := InspectSV1DFilesystem(options.OutputParent)
	if err != nil {
		return measurement, fmt.Errorf("inspect resource measurement filesystem: %w", err)
	}
	measurement.Filesystem = filesystem

	initial, err := captureSV1DResourceSample(0, options.OutputParent, options.MeasurementRoot, "")
	if err != nil {
		return measurement, fmt.Errorf("capture initial resource sample: %w", err)
	}
	measurement.Samples = append(measurement.Samples, initial)

	command := exec.CommandContext(ctx, options.Command[0], options.Command[1:]...)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	inheritedFiles, err := duplicateResourceFileDescriptors(options.InheritedFileDescriptors)
	if err != nil {
		return measurement, err
	}
	command.ExtraFiles = inheritedFiles
	closeInheritedFiles := func() {
		for _, file := range inheritedFiles {
			_ = file.Close()
		}
	}
	if err := command.Start(); err != nil {
		closeInheritedFiles()
		measurement.Error = err.Error()
		finalizeSV1DResourceMeasurement(&measurement)
		return measurement, fmt.Errorf("start measured command: %w", err)
	}
	closeInheritedFiles()
	cgroupPath, err := cgroupPathForPID(command.Process.Pid)
	if err != nil {
		killResourceProcessGroup(command.Process.Pid)
		_ = command.Wait()
		measurement.Error = err.Error()
		finalizeSV1DResourceMeasurement(&measurement)
		return measurement, fmt.Errorf("resolve measured command cgroup: %w", err)
	}

	first, err := captureSV1DResourceSample(command.Process.Pid, options.OutputParent, options.MeasurementRoot, cgroupPath)
	if err != nil {
		killResourceProcessGroup(command.Process.Pid)
		_ = command.Wait()
		measurement.Error = err.Error()
		finalizeSV1DResourceMeasurement(&measurement)
		return measurement, fmt.Errorf("capture first resource sample: %w", err)
	}
	measurement.Samples = append(measurement.Samples, first)

	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()
	samplingCadence := sv1dResourceSamplingCadence(options.SampleInterval)
	nextSampleDeadline := time.Now().Add(samplingCadence)
	for {
		sampleTimer := time.NewTimer(time.Until(nextSampleDeadline))
		select {
		case waitErr := <-waitResult:
			stopSV1DResourceTimer(sampleTimer)
			measurement.ExitStatus = command.ProcessState.ExitCode()
			measurement.Complete = waitErr == nil && measurement.ExitStatus == 0
			if waitErr != nil {
				measurement.Error = waitErr.Error()
			}
			final, finalErr := captureSV1DResourceSample(command.Process.Pid, options.OutputParent, options.MeasurementRoot, cgroupPath)
			if finalErr != nil {
				measurement.Error = finalErr.Error()
				finalErr = fmt.Errorf("capture final resource sample: %w", finalErr)
				finalizeSV1DResourceMeasurement(&measurement)
				return measurement, finalErr
			}
			measurement.Samples = append(measurement.Samples, final)
			finalizeSV1DResourceMeasurement(&measurement)
			if options.RequireFiniteCgroupLimit && measurement.CgroupMemoryLimitBytes == 0 {
				return measurement, errors.New("measured command has no finite cgroup memory limit")
			}
			return measurement, nil
		case <-sampleTimer.C:
			sample, sampleErr := captureSV1DResourceSample(command.Process.Pid, options.OutputParent, options.MeasurementRoot, cgroupPath)
			if sampleErr != nil {
				killResourceProcessGroup(command.Process.Pid)
				<-waitResult
				measurement.Error = sampleErr.Error()
				finalizeSV1DResourceMeasurement(&measurement)
				return measurement, fmt.Errorf("capture resource sample: %w", sampleErr)
			}
			measurement.Samples = append(measurement.Samples, sample)
			nextSampleDeadline = nextSampleDeadline.Add(samplingCadence)
			if !nextSampleDeadline.After(time.Now()) {
				nextSampleDeadline = time.Now().Add(samplingCadence)
			}
		case <-ctx.Done():
			stopSV1DResourceTimer(sampleTimer)
			killResourceProcessGroup(command.Process.Pid)
			<-waitResult
			measurement.ExitStatus = -1
			measurement.Error = ctx.Err().Error()
			finalizeSV1DResourceMeasurement(&measurement)
			return measurement, ctx.Err()
		}
	}
}

func sv1dResourceSamplingCadence(maximumSampleGap time.Duration) time.Duration {
	if maximumSampleGap <= time.Nanosecond {
		return time.Nanosecond
	}
	return maximumSampleGap / 2
}

func stopSV1DResourceTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func duplicateResourceFileDescriptors(descriptors []int) ([]*os.File, error) {
	if len(descriptors) == 0 {
		return nil, nil
	}
	inheritedFiles := make([]*os.File, 0, len(descriptors))
	closeInheritedFiles := func() {
		for _, file := range inheritedFiles {
			_ = file.Close()
		}
	}
	for _, descriptor := range descriptors {
		if descriptor < 0 {
			closeInheritedFiles()
			return nil, fmt.Errorf("inherited file descriptor must be nonnegative: %d", descriptor)
		}
		duplicate, err := syscall.Dup(descriptor)
		if err != nil {
			closeInheritedFiles()
			return nil, fmt.Errorf("duplicate inherited file descriptor %d: %w", descriptor, err)
		}
		file := os.NewFile(uintptr(duplicate), fmt.Sprintf("sv1dresource-inherited-fd-%d", descriptor))
		if file == nil {
			_ = syscall.Close(duplicate)
			closeInheritedFiles()
			return nil, fmt.Errorf("wrap inherited file descriptor %d", descriptor)
		}
		inheritedFiles = append(inheritedFiles, file)
	}
	return inheritedFiles, nil
}

func killResourceProcessGroup(pid int) {
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

func captureSV1DResourceSample(rootPID int, outputParent, measurementRoot, cgroupPath string) (SV1DResourceSample, error) {
	apparent, allocated, err := resourceTreeFootprint(measurementRoot)
	if err != nil {
		return SV1DResourceSample{}, err
	}
	available, err := resourceAvailableBytes(outputParent)
	if err != nil {
		return SV1DResourceSample{}, err
	}
	hostAvailable, swapUsed, err := hostMemorySnapshot()
	if err != nil {
		return SV1DResourceSample{}, err
	}
	if cgroupPath == "" {
		cgroupPath, err = cgroupPathForPID(os.Getpid())
		if err != nil {
			return SV1DResourceSample{}, err
		}
	}
	cgroup, err := readResourceCgroup(cgroupPath)
	if err != nil {
		return SV1DResourceSample{}, err
	}
	return SV1DResourceSample{
		ObservedAtUnixNano:       time.Now().UnixNano(),
		AvailableBytes:           available,
		ApparentBytes:            apparent,
		AllocatedBytes:           allocated,
		ProcessTreeRSSBytes:      resourceProcessTreeRSS(rootPID),
		CgroupCurrentBytes:       cgroup.current,
		CgroupMemoryLimitBytes:   cgroup.limit,
		HostMemAvailableBytes:    hostAvailable,
		SwapUsedBytes:            swapUsed,
		CgroupOOMEvents:          cgroup.events["oom"],
		CgroupOOMKillEvents:      cgroup.events["oom_kill"],
		CgroupLocalOOMEvents:     cgroup.localEvents["oom"],
		CgroupLocalOOMKillEvents: cgroup.localEvents["oom_kill"],
	}, nil
}

func finalizeSV1DResourceMeasurement(measurement *SV1DResourceMeasurement) {
	if len(measurement.Samples) == 0 {
		return
	}
	measurement.InitialAvailableBytes = measurement.Samples[0].AvailableBytes
	measurement.MinimumAvailableBytes = measurement.Samples[0].AvailableBytes
	measurement.FinalAvailableBytes = measurement.Samples[len(measurement.Samples)-1].AvailableBytes
	measurement.PeakApparentBytes = measurement.Samples[0].ApparentBytes
	measurement.PeakAllocatedBytes = measurement.Samples[0].AllocatedBytes
	measurement.PeakFilesystemConsumptionBytes = measurement.InitialAvailableBytes
	measurement.PeakProcessTreeRSSBytes = measurement.Samples[0].ProcessTreeRSSBytes
	measurement.PeakCgroupMemoryBytes = measurement.Samples[0].CgroupCurrentBytes
	measurement.CgroupMemoryLimitBytes = measurement.Samples[0].CgroupMemoryLimitBytes
	measurement.MinimumHostMemAvailableBytes = measurement.Samples[0].HostMemAvailableBytes
	measurement.MaximumSwapUsedBytes = measurement.Samples[0].SwapUsedBytes
	for index, sample := range measurement.Samples {
		if sample.AvailableBytes < measurement.MinimumAvailableBytes {
			measurement.MinimumAvailableBytes = sample.AvailableBytes
		}
		if sample.ApparentBytes > measurement.PeakApparentBytes {
			measurement.PeakApparentBytes = sample.ApparentBytes
		}
		if sample.AllocatedBytes > measurement.PeakAllocatedBytes {
			measurement.PeakAllocatedBytes = sample.AllocatedBytes
		}
		if sample.ProcessTreeRSSBytes > measurement.PeakProcessTreeRSSBytes {
			measurement.PeakProcessTreeRSSBytes = sample.ProcessTreeRSSBytes
		}
		if sample.CgroupCurrentBytes > measurement.PeakCgroupMemoryBytes {
			measurement.PeakCgroupMemoryBytes = sample.CgroupCurrentBytes
		}
		if sample.CgroupMemoryLimitBytes > measurement.CgroupMemoryLimitBytes {
			measurement.CgroupMemoryLimitBytes = sample.CgroupMemoryLimitBytes
		}
		if sample.HostMemAvailableBytes < measurement.MinimumHostMemAvailableBytes {
			measurement.MinimumHostMemAvailableBytes = sample.HostMemAvailableBytes
		}
		if sample.SwapUsedBytes > measurement.MaximumSwapUsedBytes {
			measurement.MaximumSwapUsedBytes = sample.SwapUsedBytes
		}
		if index > 0 {
			gap := uint64(measurement.Samples[index].ObservedAtUnixNano - measurement.Samples[index-1].ObservedAtUnixNano)
			if gap > measurement.MaximumSampleGapNano {
				measurement.MaximumSampleGapNano = gap
			}
		}
	}
	if measurement.MinimumAvailableBytes <= measurement.InitialAvailableBytes {
		measurement.PeakFilesystemConsumptionBytes = measurement.InitialAvailableBytes - measurement.MinimumAvailableBytes
	}
	if len(measurement.Samples) > 0 {
		first := measurement.Samples[0]
		last := measurement.Samples[len(measurement.Samples)-1]
		measurement.CgroupOOMEventsDelta = resourceCounterDelta(first.CgroupOOMEvents, last.CgroupOOMEvents)
		measurement.CgroupOOMKillEventsDelta = resourceCounterDelta(first.CgroupOOMKillEvents, last.CgroupOOMKillEvents)
		measurement.CgroupLocalOOMDelta = resourceCounterDelta(first.CgroupLocalOOMEvents, last.CgroupLocalOOMEvents)
		measurement.CgroupLocalOOMKillDelta = resourceCounterDelta(first.CgroupLocalOOMKillEvents, last.CgroupLocalOOMKillEvents)
	}
	measurement.SampleCount = uint64(len(measurement.Samples))
	canonical, err := json.Marshal(measurement.Samples)
	if err == nil {
		digest := sha256.Sum256(canonical)
		measurement.SamplesSHA256 = hex.EncodeToString(digest[:])
	}
}

func resourceCounterDelta(initial, final uint64) uint64 {
	if final < initial {
		return math.MaxUint64
	}
	return final - initial
}

func absoluteResourcePath(path string) (string, error) {
	if path == "" || strings.ContainsRune(path, '\x00') {
		return "", errors.New("path is empty or contains NUL")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if filepath.Clean(absolute) != absolute || absolute == string(filepath.Separator) {
		return "", fmt.Errorf("path is not a clean non-root absolute path: %s", path)
	}
	return absolute, nil
}

func resourceAvailableBytes(path string) (uint64, error) {
	absolute, err := absoluteResourcePath(path)
	if err != nil {
		return 0, err
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(absolute, &stat); err != nil {
		return 0, err
	}
	if stat.Bavail < 0 || stat.Bsize <= 0 {
		return 0, errors.New("filesystem reports invalid free-space counters")
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}

func resourceTreeFootprint(root string) (uint64, uint64, error) {
	absolute, err := absoluteResourcePath(root)
	if err != nil {
		return 0, 0, err
	}
	var apparent, allocated uint64
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("measurement tree contains symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			if errors.Is(infoErr, os.ErrNotExist) {
				return nil
			}
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > 0 {
			apparent += uint64(info.Size())
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Blocks > 0 {
			allocated += uint64(stat.Blocks) * 512
		} else if info.Size() > 0 {
			allocated += uint64(info.Size())
		}
		return nil
	})
	return apparent, allocated, err
}

func resourceProcessTreeRSS(rootPID int) uint64 {
	if rootPID <= 0 {
		return 0
	}
	queue := []int{rootPID}
	seen := make(map[int]struct{})
	var total uint64
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		total += resourceProcessRSS(pid)
		children, err := resourceProcessChildren(pid)
		if err == nil {
			queue = append(queue, children...)
		}
	}
	return total
}

func resourceProcessRSS(pid int) uint64 {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "VmRSS:" {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err == nil {
				return value * 1024
			}
		}
	}
	return 0
}

func resourceProcessChildren(pid int) ([]int, error) {
	tasks, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	if err != nil {
		return nil, err
	}
	children := make([]int, 0)
	seen := make(map[int]struct{})
	for _, task := range tasks {
		raw, readErr := os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/children", pid, task.Name()))
		if readErr != nil {
			continue
		}
		for _, field := range strings.Fields(string(raw)) {
			child, parseErr := strconv.Atoi(field)
			if parseErr == nil {
				if _, duplicate := seen[child]; !duplicate {
					seen[child] = struct{}{}
					children = append(children, child)
				}
			}
		}
	}
	return children, nil
}

type resourceCgroupSnapshot struct {
	current     uint64
	limit       uint64
	events      map[string]uint64
	localEvents map[string]uint64
}

func cgroupPathForPID(pid int) (string, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && parts[0] == "0" {
			path := parts[2]
			if path == "" {
				return "/sys/fs/cgroup", nil
			}
			if strings.Contains(path, "..") {
				return "", errors.New("cgroup path contains parent traversal")
			}
			return filepath.Join("/sys/fs/cgroup", path), nil
		}
	}
	return "", errors.New("process has no cgroup v2 path")
}

func readResourceCgroup(path string) (resourceCgroupSnapshot, error) {
	snapshot := resourceCgroupSnapshot{events: map[string]uint64{}, localEvents: map[string]uint64{}}
	current, err := readResourceUint(filepath.Join(path, "memory.current"), false)
	if err != nil {
		return snapshot, err
	}
	limit, err := readResourceUint(filepath.Join(path, "memory.max"), true)
	if err != nil {
		return snapshot, err
	}
	events, err := readResourceCounters(filepath.Join(path, "memory.events"))
	if err != nil {
		return snapshot, err
	}
	localEvents, err := readResourceCounters(filepath.Join(path, "memory.events.local"))
	if err != nil {
		return snapshot, err
	}
	snapshot.current, snapshot.limit, snapshot.events, snapshot.localEvents = current, limit, events, localEvents
	return snapshot, nil
}

func readResourceUint(path string, allowMax bool) (uint64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(raw))
	if allowMax && value == "max" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return parsed, nil
}

func readResourceCounters(path string) (map[string]uint64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	counters := make(map[string]uint64)
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed cgroup counter in %s", path)
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse cgroup counter in %s: %w", path, err)
		}
		if _, duplicate := counters[fields[0]]; duplicate {
			return nil, fmt.Errorf("duplicate cgroup counter %q in %s", fields[0], path)
		}
		counters[fields[0]] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for _, required := range []string{"oom", "oom_kill"} {
		if _, present := counters[required]; !present {
			return nil, fmt.Errorf("cgroup counter %q is missing in %s", required, path)
		}
	}
	return counters, nil
}

func hostMemorySnapshot() (uint64, uint64, error) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	return parseHostMemorySnapshot(string(raw))
}

func parseHostMemorySnapshot(raw string) (uint64, uint64, error) {
	var available, swapTotal, swapFree uint64
	var seenAvailable, seenSwapTotal, seenSwapFree bool
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		isRequired := fields[0] == "MemAvailable:" || fields[0] == "SwapTotal:" || fields[0] == "SwapFree:"
		if !isRequired {
			continue
		}
		if len(fields) != 3 || fields[2] != "kB" {
			return 0, 0, fmt.Errorf("malformed host memory counter %q", fields[0])
		}
		value, parseErr := strconv.ParseUint(fields[1], 10, 64)
		if parseErr != nil || value > ^uint64(0)/1024 {
			return 0, 0, fmt.Errorf("malformed host memory counter %q", fields[0])
		}
		bytes := value * 1024
		switch fields[0] {
		case "MemAvailable:":
			if seenAvailable {
				return 0, 0, errors.New("duplicate MemAvailable host memory counter")
			}
			available, seenAvailable = bytes, true
		case "SwapTotal:":
			if seenSwapTotal {
				return 0, 0, errors.New("duplicate SwapTotal host memory counter")
			}
			swapTotal, seenSwapTotal = bytes, true
		case "SwapFree:":
			if seenSwapFree {
				return 0, 0, errors.New("duplicate SwapFree host memory counter")
			}
			swapFree, seenSwapFree = bytes, true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if !seenAvailable || !seenSwapTotal || !seenSwapFree || available == 0 || swapFree > swapTotal {
		return 0, 0, errors.New("host memory counters are incomplete")
	}
	return available, swapTotal - swapFree, nil
}

type resourceMount struct {
	mountID        uint64
	mountpoint     string
	filesystemType string
	source         string
}

func findResourceMount(path string) (resourceMount, error) {
	raw, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return resourceMount{}, err
	}
	var best resourceMount
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		prefix := strings.Fields(parts[0])
		suffix := strings.Fields(parts[1])
		if len(prefix) < 5 || len(suffix) < 2 {
			continue
		}
		mountID, parseErr := strconv.ParseUint(prefix[0], 10, 64)
		if parseErr != nil {
			continue
		}
		mountpoint := decodeMountInfoField(prefix[4])
		if !resourcePathWithinMount(path, mountpoint) || len(mountpoint) < len(best.mountpoint) {
			continue
		}
		best = resourceMount{mountID: mountID, mountpoint: mountpoint, filesystemType: suffix[0], source: decodeMountInfoField(suffix[1])}
	}
	if best.mountpoint == "" {
		return resourceMount{}, fmt.Errorf("no mount found for %s", path)
	}
	return best, nil
}

func resourcePathWithinMount(path, mountpoint string) bool {
	if mountpoint == string(filepath.Separator) {
		return true
	}
	return path == mountpoint || strings.HasPrefix(path, strings.TrimSuffix(mountpoint, string(filepath.Separator))+string(filepath.Separator))
}

func decodeMountInfoField(value string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(value)
}

func resourceFilesystemUUID(source string) string {
	if source == "" || !strings.HasPrefix(source, "/dev/") {
		return ""
	}
	entries, err := os.ReadDir("/dev/disk/by-uuid")
	if err != nil {
		return ""
	}
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		candidate, evalErr := filepath.EvalSymlinks(filepath.Join("/dev/disk/by-uuid", entry.Name()))
		if evalErr == nil && candidate == resolvedSource {
			return entry.Name()
		}
	}
	return ""
}
