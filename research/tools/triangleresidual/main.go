// Command triangleresidual measures the triangular pricing inconsistency the
// arbitrageur trades on, and asks whether it is a market or an artifact.
//
// The residual is implied minus actual, where implied = ABC/USD divided by
// CDF/USD, expressed in the cross book's own units. A residual that oscillates
// around zero is a contested market being disciplined; one that keeps its sign
// for the whole run is a modelling choice being harvested.
//
// Sign persistence is the test, not magnitude: a large residual that changes
// sign is a volatile market, and a small one that never does is still an
// artifact.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type snapshot struct {
	Event string `json:"event"`
	SimTS int64  `json:"sim_ts"`
	Data  struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Bids []struct {
				Price int64 `json:"price"`
			} `json:"bids"`
			Asks []struct {
				Price int64 `json:"price"`
			} `json:"asks"`
		} `json:"payload"`
	} `json:"data"`
}

func mids(dir, venue, symbol string) (map[int64]int64, error) {
	path := filepath.Join(dir, "venues", venue, "spot", symbol+".jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	out := map[int64]int64{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		line := scanner.Bytes()
		if !strings.Contains(string(line), "\"BookSnapshot\"") {
			continue
		}
		var ev snapshot
		if json.Unmarshal(line, &ev) != nil || ev.Event != "BookSnapshot" {
			continue
		}
		b, a := ev.Data.Payload.Bids, ev.Data.Payload.Asks
		if len(b) == 0 || len(a) == 0 {
			continue
		}
		out[ev.SimTS] = (b[0].Price + a[0].Price) / 2
	}
	return out, scanner.Err()
}

func main() {
	dir := flag.String("dir", "", "run log directory")
	venue := flag.String("venue", "north", "venue to measure")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	abcusd, err := mids(*dir, *venue, "ABC-USD")
	if err != nil {
		fmt.Fprintln(os.Stderr, "ABC-USD:", err)
		os.Exit(1)
	}
	cdfusd, err := mids(*dir, *venue, "CDF-USD")
	if err != nil {
		fmt.Fprintln(os.Stderr, "CDF-USD:", err)
		os.Exit(1)
	}
	abccdf, err := mids(*dir, *venue, "ABC-CDF")
	if err != nil {
		fmt.Fprintln(os.Stderr, "ABC-CDF:", err)
		os.Exit(1)
	}

	stamps := make([]int64, 0, len(abccdf))
	for ts := range abccdf {
		if _, ok := abcusd[ts]; !ok {
			continue
		}
		if _, ok := cdfusd[ts]; !ok {
			continue
		}
		stamps = append(stamps, ts)
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i] < stamps[j] })
	if len(stamps) == 0 {
		fmt.Println("no instants where all three books have a two-sided mid")
		return
	}

	var positive, negative, zero, flips int
	var sumAbs, maxAbs float64
	last := 0
	for _, ts := range stamps {
		// ABC/CDF is quoted with the base asset's precision as its quote
		// precision, so the implied rate is (ABC/USD) / (CDF/USD) scaled by the
		// cross book's quote precision, which equals mvBasePrecision = 1e8.
		implied := float64(abcusd[ts]) / float64(cdfusd[ts]) * 1e8
		residual := (implied - float64(abccdf[ts])) / implied
		sign := 0
		switch {
		case residual > 0:
			sign = 1
			positive++
		case residual < 0:
			sign = -1
			negative++
		default:
			zero++
		}
		if sign != 0 && last != 0 && sign != last {
			flips++
		}
		if sign != 0 {
			last = sign
		}
		sumAbs += math.Abs(residual)
		maxAbs = math.Max(maxAbs, math.Abs(residual))
	}
	n := float64(len(stamps))
	fmt.Printf("venue %s, instants with all three books two-sided: %d\n", *venue, len(stamps))
	fmt.Printf("  residual positive: %d (%.1f%%)\n", positive, 100*float64(positive)/n)
	fmt.Printf("  residual negative: %d (%.1f%%)\n", negative, 100*float64(negative)/n)
	fmt.Printf("  sign flips: %d\n", flips)
	fmt.Printf("  mean |residual|: %.4f%%   max |residual|: %.4f%%\n",
		100*sumAbs/n, 100*maxAbs)
	dominant := float64(positive)
	if negative > positive {
		dominant = float64(negative)
	}
	fmt.Printf("  dominant sign holds %.1f%% of instants\n", 100*dominant/n)
}
