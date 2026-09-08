// Command perpbasis measures how far a perpetual's book has travelled from its
// index, and whether the mark price's clamp is binding.
//
// The exchange marks a margin instrument at
//
//	mark = index + clamp(EMA(perp_mid - index), +/- index * bandBps/2/10000)
//
// so once the exponentially-weighted basis reaches the half-band the mark stops
// following the book. Margin and liquidation consume that mark, so a binding
// clamp means positions are valued at a price the book has left.
//
// This reads BookSnapshot evidence rather than funding evidence, so it is an
// independent measurement of the same quantity RT-043 inferred from settled
// funding rates. The index is reconstructed the way spotIndexProvider's
// consensus mode builds it: the median of the venues' own ABC/USD midpoints.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type level struct {
	Price int64 `json:"price"`
}

type snapshot struct {
	Bids []level `json:"bids"`
	Asks []level `json:"asks"`
}

// record covers both evidence shapes. A per-book spot file writes the levels
// directly and identifies the book by its file path; the shared derivatives file
// nests the levels under a symbol, because one file carries several books. A
// reader that assumes either shape silently finds nothing.
type record struct {
	Event string `json:"event"`
	SimTS int64  `json:"sim_ts"`
	Data  struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Symbol  string   `json:"symbol"`
			Asks    []level  `json:"asks"`
			Bids    []level  `json:"bids"`
			Payload snapshot `json:"payload"`
		} `json:"payload"`
	} `json:"data"`
}

// symbolFromPath recovers the book a per-book evidence file describes. Spot
// snapshot payloads carry no symbol at all.
func symbolFromPath(path string) string {
	base := filepath.Base(path)
	name := base[:len(base)-len(filepath.Ext(base))]
	return strings.ReplaceAll(name, "-", "/")
}

// mid is the two-sided midpoint. A one-sided book has no midpoint; reporting one
// would invent a price, which RT-018 records as a measurement error.
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

func median(values []int64) int64 {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	n := len(values)
	if n%2 == 1 {
		return values[n/2]
	}
	return (values[n/2-1] + values[n/2]) / 2
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	perpSymbol := flag.String("perp", "ABC-PERP", "perpetual whose basis is measured")
	spotSymbol := flag.String("spot", "ABC/USD", "spot book the index consensus is built from")
	bandBps := flag.Float64("band-bps", 600, "configured mark price band; the clamp is half this")
	flag.Parse()
	if *logDir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	// perpMids[venue][ts] and spotMids[ts][venue]: the index is a cross-venue
	// consensus, so spot has to be grouped by timestamp first.
	perpMids := map[string]map[int64]int64{}
	spotMids := map[int64]map[string]int64{}

	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	nested, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	files = append(files, nested...)
	sort.Strings(files)

	marker := []byte(`"BookSnapshot"`)
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open:", err)
			os.Exit(1)
		}
		pathSymbol := symbolFromPath(path)
		scanner := bufio.NewScanner(file)
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
			symbol := rec.Data.Payload.Symbol
			shot := rec.Data.Payload.Payload
			if symbol == "" {
				// Per-book file: levels are at the top of the payload and the
				// book is named by the path.
				symbol = pathSymbol
				shot = snapshot{Bids: rec.Data.Payload.Bids, Asks: rec.Data.Payload.Asks}
			}
			if symbol != *perpSymbol && symbol != *spotSymbol {
				continue
			}
			mid, ok := shot.mid()
			if !ok {
				continue
			}
			venue := rec.Data.VenueID
			switch symbol {
			case *perpSymbol:
				if perpMids[venue] == nil {
					perpMids[venue] = map[int64]int64{}
				}
				perpMids[venue][rec.SimTS] = mid
			case *spotSymbol:
				if spotMids[rec.SimTS] == nil {
					spotMids[rec.SimTS] = map[string]int64{}
				}
				spotMids[rec.SimTS][venue] = mid
			}
		}
		file.Close()
	}

	// Consensus index per timestamp, median across venues.
	index := map[int64]int64{}
	for ts, byVenue := range spotMids {
		values := make([]int64, 0, len(byVenue))
		for _, mid := range byVenue {
			values = append(values, mid)
		}
		index[ts] = median(values)
	}

	halfBand := *bandBps / 2 / 10000
	venues := make([]string, 0, len(perpMids))
	for venue := range perpMids {
		venues = append(venues, venue)
	}
	sort.Strings(venues)

	fmt.Printf("perp %s against the median-of-venues %s index; clamp is +/-%.2f%%\n",
		*perpSymbol, *spotSymbol, halfBand*100)
	for _, venue := range venues {
		timestamps := make([]int64, 0, len(perpMids[venue]))
		for ts := range perpMids[venue] {
			if _, ok := index[ts]; ok {
				timestamps = append(timestamps, ts)
			}
		}
		if len(timestamps) == 0 {
			fmt.Printf("%-9s no timestamps with both books two-sided\n", venue)
			continue
		}
		sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })
		beyond, worst, sum := 0, 0.0, 0.0
		var lastQuarter []float64
		cut := timestamps[len(timestamps)*3/4]
		for _, ts := range timestamps {
			basis := float64(perpMids[venue][ts]-index[ts]) / float64(index[ts])
			sum += basis
			if basis < worst {
				worst = basis
			}
			if basis > halfBand || basis < -halfBand {
				beyond++
			}
			if ts >= cut {
				lastQuarter = append(lastQuarter, basis)
			}
		}
		tail := 0.0
		for _, b := range lastQuarter {
			tail += b
		}
		fmt.Printf("%-9s samples=%d mean=%+.3f%% worst=%+.3f%% beyond_clamp=%d (%.1f%%) final_quarter_mean=%+.3f%%\n",
			venue, len(timestamps), sum/float64(len(timestamps))*100, worst*100,
			beyond, float64(beyond)/float64(len(timestamps))*100,
			tail/float64(len(lastQuarter))*100)
	}
}
