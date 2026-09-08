// Command futbasis measures whether a dated future converges to its underlying
// as expiry approaches.
//
// A perpetual's tether is a soft one — funding, a mark clamp — and it has no
// terminal date to force the issue. A dated contract does: at expiry it settles
// against the underlying, so its price must converge or the settlement transfers
// value the book never priced. That makes convergence arithmetic rather than a
// modelling preference, and it is the one property these books must have.
//
// Spot is rebuilt as the median ABC/USD midpoint across venues, matching the
// venue's own consensus rule, and evaluated at or before each futures snapshot.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type level struct {
	Price int64 `json:"price"`
}

type snapshot struct {
	Bids []level `json:"bids"`
	Asks []level `json:"asks"`
}

func (s snapshot) mid() (int64, bool) {
	if len(s.Bids) == 0 || len(s.Asks) == 0 {
		return 0, false
	}
	bid, ask := s.Bids[0].Price, s.Asks[0].Price
	if bid <= 0 || ask <= 0 || ask < bid {
		return 0, false
	}
	return (bid + ask) / 2, true
}

type record struct {
	Event string `json:"event"`
	SimTS int64  `json:"sim_ts"`
	Data  struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Symbol  string   `json:"symbol"`
			Bids    []level  `json:"bids"`
			Asks    []level  `json:"asks"`
			Payload snapshot `json:"payload"`
		} `json:"payload"`
	} `json:"data"`
}

var futPattern = regexp.MustCompile(`^ABC-FUT-(\d+)$`)

func symbolFromPath(path string) string {
	base := filepath.Base(path)
	name := base[:len(base)-len(filepath.Ext(base))]
	return strings.ReplaceAll(name, "-", "/")
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	spotSymbol := flag.String("spot", "ABC/USD", "underlying book the consensus is built from")
	flag.Parse()
	if *logDir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	nested, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	files = append(files, nested...)
	sort.Strings(files)

	spotMids := map[int64]map[string]int64{}
	type futSample struct {
		ts     int64
		expiry int64
		mid    int64
	}
	var futures []futSample

	marker := []byte(`"BookSnapshot"`)
	for _, path := range files {
		handle, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open:", err)
			os.Exit(1)
		}
		pathSymbol := symbolFromPath(path)
		scanner := bufio.NewScanner(handle)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.Contains(line, marker) {
				continue
			}
			var rec record
			if err := json.Unmarshal(line, &rec); err != nil || rec.Event != "BookSnapshot" {
				continue
			}
			symbol, shot := rec.Data.Payload.Symbol, rec.Data.Payload.Payload
			if symbol == "" {
				symbol = pathSymbol
				shot = snapshot{Bids: rec.Data.Payload.Bids, Asks: rec.Data.Payload.Asks}
			}
			mid, ok := shot.mid()
			if !ok {
				continue
			}
			if symbol == *spotSymbol {
				if spotMids[rec.SimTS] == nil {
					spotMids[rec.SimTS] = map[string]int64{}
				}
				spotMids[rec.SimTS][rec.Data.VenueID] = mid
				continue
			}
			if m := futPattern.FindStringSubmatch(symbol); m != nil {
				expiry, err := strconv.ParseInt(m[1], 10, 64)
				if err != nil {
					continue
				}
				futures = append(futures, futSample{rec.SimTS, expiry, mid})
			}
		}
		handle.Close()
	}

	consensus := map[int64]int64{}
	stamps := make([]int64, 0, len(spotMids))
	for ts, byVenue := range spotMids {
		values := make([]int64, 0, len(byVenue))
		for _, mid := range byVenue {
			values = append(values, mid)
		}
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		n := len(values)
		med := values[n/2]
		if n%2 == 0 {
			med = (values[n/2-1] + values[n/2]) / 2
		}
		consensus[ts] = med
		stamps = append(stamps, ts)
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i] < stamps[j] })
	if len(stamps) == 0 {
		fmt.Fprintln(os.Stderr, "no two-sided spot snapshot")
		os.Exit(1)
	}
	spotAt := func(ts int64) (int64, bool) {
		i := sort.Search(len(stamps), func(i int) bool { return stamps[i] > ts })
		if i == 0 {
			return 0, false
		}
		return consensus[stamps[i-1]], true
	}

	edges := []float64{0, 0.5, 1, 2, 4, 1e9}
	labels := []string{"<0.5h to expiry", "0.5-1h", "1-2h", "2-4h", ">=4h"}
	byBand := make([][]float64, len(labels))
	skipped := 0
	for _, f := range futures {
		spot, ok := spotAt(f.ts)
		if !ok || spot <= 0 {
			skipped++
			continue
		}
		hours := float64(f.expiry*1_000_000_000-f.ts) / 3.6e12
		if hours < 0 {
			continue
		}
		basis := float64(f.mid-spot) / float64(spot) * 100
		for i := 0; i < len(edges)-1; i++ {
			if hours >= edges[i] && hours < edges[i+1] {
				byBand[i] = append(byBand[i], basis)
				break
			}
		}
	}

	fmt.Printf("dated futures basis against the median-of-venues %s, by time to expiry\n", *spotSymbol)
	if skipped > 0 {
		fmt.Printf("  samples with no prior spot: %d\n", skipped)
	}
	fmt.Printf("%-18s %10s %12s %12s %12s %12s\n",
		"band", "samples", "median", "mean |basis|", "p90 |basis|", "max |basis|")
	for i, label := range labels {
		v := byBand[i]
		if len(v) == 0 {
			continue
		}
		abs := make([]float64, len(v))
		sum := 0.0
		for j, x := range v {
			abs[j] = math.Abs(x)
			sum += math.Abs(x)
		}
		sort.Float64s(abs)
		fmt.Printf("%-18s %10d %11.3f%% %11.3f%% %11.3f%% %11.3f%%\n",
			label, len(v), median(v), sum/float64(len(v)),
			abs[len(abs)*9/10], abs[len(abs)-1])
	}
	fmt.Printf("\nmedian is signed; the rest are absolute. A dated contract must converge to 0 at expiry.\n")
}
