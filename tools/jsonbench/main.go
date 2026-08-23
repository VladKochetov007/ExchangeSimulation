// jsonbench differentially validates and benchmarks JSON decoders on retained
// JSONL evidence. It is deliberately an isolated tool: a candidate decoder is
// not a production dependency until it proves equivalent on this contract.
package main

import (
	"bufio"
	"bytes"
	stdjson "encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"time"

	"github.com/bytedance/sonic"
	gojson "github.com/goccy/go-json"
	jsoniter "github.com/json-iterator/go"
)

type envelope struct {
	SimTS    int64              `json:"sim_ts"`
	ClientID uint64             `json:"client_id"`
	Event    string             `json:"event"`
	Data     stdjson.RawMessage `json:"data"`
}

type decoder struct {
	name      string
	unmarshal func([]byte, any) error
}

type result struct {
	Decoder       string        `json:"decoder"`
	Records       int64         `json:"records"`
	Bytes         int64         `json:"bytes"`
	Elapsed       time.Duration `json:"elapsed"`
	RecordsPerSec float64       `json:"records_per_second"`
	MiBPerSec     float64       `json:"mib_per_second"`
	AllocBytes    uint64        `json:"alloc_bytes"`
	AllocObjects  uint64        `json:"alloc_objects"`
	GCPause       time.Duration `json:"gc_pause"`
	PeakHeap      uint64        `json:"peak_heap_bytes"`
}

func main() {
	root := flag.String("root", "", "run directory containing venues JSONL evidence")
	maxRecords := flag.Int64("max-records", 0, "maximum records per pass; zero means all")
	rounds := flag.Int("rounds", 3, "timed cached passes per decoder")
	flag.Parse()
	if *root == "" || *rounds <= 0 || *maxRecords < 0 {
		fmt.Fprintln(os.Stderr, "usage: jsonbench -root <run-dir> [-max-records N] [-rounds N]")
		os.Exit(2)
	}
	files, err := evidenceFiles(*root)
	if err != nil {
		fatal(err)
	}
	decoders := []decoder{
		{name: "encoding/json", unmarshal: stdjson.Unmarshal},
		{name: "goccy/go-json", unmarshal: gojson.Unmarshal},
		{name: "json-iterator/compatible", unmarshal: jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal},
		{name: "bytedance/sonic-std", unmarshal: sonic.ConfigStd.Unmarshal},
	}
	eligible := []decoder{decoders[0]}
	for _, candidate := range decoders[1:] {
		if err := validate(candidate, files, *maxRecords); err != nil {
			fmt.Printf("REJECT\t%s\t%s\n", candidate.name, err)
			continue
		}
		fmt.Printf("COMPATIBLE\t%s\n", candidate.name)
		eligible = append(eligible, candidate)
	}
	for _, candidate := range eligible {
		var totals result
		totals.Decoder = candidate.name
		for round := 0; round < *rounds; round++ {
			measured, err := measure(candidate, files, *maxRecords)
			if err != nil {
				fatal(err)
			}
			totals.Records += measured.Records
			totals.Bytes += measured.Bytes
			totals.Elapsed += measured.Elapsed
			totals.AllocBytes += measured.AllocBytes
			totals.AllocObjects += measured.AllocObjects
			totals.GCPause += measured.GCPause
			if measured.PeakHeap > totals.PeakHeap {
				totals.PeakHeap = measured.PeakHeap
			}
		}
		totals.RecordsPerSec = float64(totals.Records) / totals.Elapsed.Seconds()
		totals.MiBPerSec = float64(totals.Bytes) / (1024 * 1024) / totals.Elapsed.Seconds()
		fmt.Printf("%s\trecords=%d\tMiB=%.1f\twall=%s\trecords/s=%.0f\tMiB/s=%.1f\talloc=%.1fMiB\tobjects=%d\tgc-pause=%s\tpeak-heap=%.1fMiB\n",
			totals.Decoder, totals.Records, float64(totals.Bytes)/(1024*1024), totals.Elapsed.Round(time.Millisecond),
			totals.RecordsPerSec, totals.MiBPerSec, float64(totals.AllocBytes)/(1024*1024), totals.AllocObjects,
			totals.GCPause.Round(time.Microsecond), float64(totals.PeakHeap)/(1024*1024))
	}
}

func validate(candidate decoder, files []string, maxRecords int64) error {
	for index, line := range contractFixtures() {
		if err := compare(candidate, line, fmt.Sprintf("fixture[%d]", index)); err != nil {
			return err
		}
	}
	return eachRecord(files, maxRecords, func(path string, ordinal int64, line []byte) error {
		if err := compare(candidate, line, fmt.Sprintf("%s:%d", path, ordinal)); err != nil {
			return err
		}
		return nil
	})
}

func contractFixtures() [][]byte {
	return [][]byte{
		[]byte(`{"sim_ts":9223372036854775807,"client_id":18446744073709551615,"event":"boundary","data":{"payload":{"n":9223372036854775807,"raw":[1,2,3]}}}`),
		[]byte(`{"sim_ts":-9223372036854775808,"client_id":0,"event":"large","data":{"payload":"` + string(bytes.Repeat([]byte("x"), 1<<20)) + `"}}`),
		[]byte(`{"sim_ts":9223372036854775808,"client_id":1,"event":"overflow","data":{}}`),
		[]byte(`{"sim_ts":1,"client_id":1,"event":"broken","data":`),
		[]byte(`{"sim_ts":"not-an-int","client_id":1,"event":"wrong-type","data":{}}`),
	}
}

func compare(candidate decoder, line []byte, label string) error {
	var standard, observed envelope
	standardErr := stdjson.Unmarshal(line, &standard)
	candidateErr := candidate.unmarshal(line, &observed)
	if (standardErr == nil) != (candidateErr == nil) {
		return fmt.Errorf("%s: %s accepted=%t, encoding/json accepted=%t", label, candidate.name, candidateErr == nil, standardErr == nil)
	}
	if standardErr != nil {
		return nil
	}
	if !reflect.DeepEqual(standard, observed) {
		return fmt.Errorf("%s: %s decoded value differs from encoding/json", label, candidate.name)
	}
	// Scanner reuses its backing buffer. A RawMessage implementation must retain
	// its own bytes or the analyzer will decode another record's payload.
	before := append([]byte(nil), observed.Data...)
	for index := range line {
		line[index] ^= 0xff
	}
	if !bytes.Equal(before, observed.Data) {
		return fmt.Errorf("%s: %s RawMessage aliases input", label, candidate.name)
	}
	return nil
}

func measure(candidate decoder, files []string, maxRecords int64) (result, error) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	result := result{Decoder: candidate.name}
	err := eachRecord(files, maxRecords, func(_ string, _ int64, line []byte) error {
		var parsed envelope
		if err := candidate.unmarshal(line, &parsed); err != nil {
			return err
		}
		result.Records++
		result.Bytes += int64(len(line)) + 1
		if result.Records&4095 == 0 {
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			if stats.HeapAlloc > result.PeakHeap {
				result.PeakHeap = stats.HeapAlloc
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	runtime.ReadMemStats(&after)
	result.Elapsed = time.Since(started)
	result.AllocBytes = after.TotalAlloc - before.TotalAlloc
	result.AllocObjects = after.Mallocs - before.Mallocs
	result.GCPause = time.Duration(after.PauseTotalNs - before.PauseTotalNs)
	return result, nil
}

func evidenceFiles(root string) ([]string, error) {
	venues := filepath.Join(root, "venues")
	var files []string
	err := filepath.WalkDir(venues, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("no JSONL evidence files")
	}
	sort.Strings(files)
	return files, nil
}

func eachRecord(files []string, maxRecords int64, visit func(path string, ordinal int64, line []byte) error) error {
	var seen int64
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		reader := bufio.NewReaderSize(file, 1<<20)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<26)
		var ordinal int64
		for scanner.Scan() {
			ordinal++
			if maxRecords > 0 && seen >= maxRecords {
				break
			}
			if err := visit(path, ordinal, scanner.Bytes()); err != nil {
				_ = file.Close()
				return err
			}
			seen++
		}
		err = errors.Join(scanner.Err(), file.Close())
		if err != nil {
			return err
		}
		if maxRecords > 0 && seen >= maxRecords {
			break
		}
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
