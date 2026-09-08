// Command arbresponse asks what an arbitrageur is actually doing when the
// dislocation it exists to close is at its widest.
//
// A position cap and a signal threshold make opposite predictions there. If the
// cap binds, the position sits at the limit precisely when the basis is extreme.
// If the strategy declines to act, the position is small exactly when the
// opportunity is largest. Terminal positions cannot distinguish the two, because
// they say nothing about when the position was held.
//
// The basis is the perpetual's midpoint against the median ABC/USD midpoint
// across venues, matching the venue's own consensus rule; positions are rebuilt
// from the exchange's post-fill position per contract.
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

type inner struct {
	Symbol  string `json:"symbol"`
	NewSize int64  `json:"new_size"`
}

type record struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
	Data     struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Symbol  string  `json:"symbol"`
			Bids    []level `json:"bids"`
			Asks    []level `json:"asks"`
			Payload struct {
				snapshot
				inner
			} `json:"payload"`
		} `json:"payload"`
	} `json:"data"`
}

type account struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
}

type greeks struct {
	InitialAccounts []account `json:"initial_accounts"`
}

type key struct {
	venue  string
	client uint64
}

func symbolFromPath(path string) string {
	base := filepath.Base(path)
	name := base[:len(base)-len(filepath.Ext(base))]
	return strings.ReplaceAll(name, "-", "/")
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	greeksPath := flag.String("greeks", "", "greeks.json (default <dir>/greeks.json)")
	perp := flag.String("perp", "ABC-PERP", "instrument whose basis is measured")
	spot := flag.String("spot", "ABC/USD", "underlying book for the consensus index")
	rolePrefix := flag.String("class", "carry_arb", "arbitrageur class to track")
	capContracts := flag.Float64("class-cap", 3000, "class-wide position cap in contracts")
	basePrecision := flag.Float64("base-precision", 1e8, "base units per whole contract")
	flag.Parse()
	if *logDir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}
	if *greeksPath == "" {
		*greeksPath = filepath.Join(*logDir, "greeks.json")
	}
	raw, err := os.ReadFile(*greeksPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read greeks:", err)
		os.Exit(1)
	}
	var snap greeks
	if err := json.Unmarshal(raw, &snap); err != nil {
		fmt.Fprintln(os.Stderr, "decode greeks:", err)
		os.Exit(1)
	}
	tracked := map[key]bool{}
	for _, row := range snap.InitialAccounts {
		if strings.HasPrefix(row.Role, *rolePrefix) {
			tracked[key{row.VenueID, row.ClientID}] = true
		}
	}

	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	nested, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	files = append(files, nested...)
	sort.Strings(files)

	// Pass 1: consensus spot and perp mid per timestamp; per-participant position
	// timeline from post-fill positions.
	spotMids := map[int64]map[string]int64{}
	perpMids := map[int64][]int64{}
	type posEvent struct {
		ts  int64
		k   key
		pos int64
	}
	var posEvents []posEvent

	snapMarker, fillMarker := []byte(`"BookSnapshot"`), []byte(`"OrderFill"`)
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
			isSnap, isFill := bytes.Contains(line, snapMarker), bytes.Contains(line, fillMarker)
			if !isSnap && !isFill {
				continue
			}
			var rec record
			if err := json.Unmarshal(line, &rec); err != nil {
				continue
			}
			switch rec.Event {
			case "BookSnapshot":
				symbol, shot := rec.Data.Payload.Symbol, rec.Data.Payload.Payload.snapshot
				if symbol == "" {
					symbol = pathSymbol
					shot = snapshot{Bids: rec.Data.Payload.Bids, Asks: rec.Data.Payload.Asks}
				}
				mid, ok := shot.mid()
				if !ok {
					continue
				}
				if symbol == *spot {
					if spotMids[rec.SimTS] == nil {
						spotMids[rec.SimTS] = map[string]int64{}
					}
					spotMids[rec.SimTS][rec.Data.VenueID] = mid
				} else if symbol == *perp {
					perpMids[rec.SimTS] = append(perpMids[rec.SimTS], mid)
				}
			case "OrderFill":
				if rec.Data.Payload.Payload.inner.Symbol != *perp {
					continue
				}
				k := key{rec.Data.VenueID, rec.ClientID}
				if !tracked[k] {
					continue
				}
				posEvents = append(posEvents, posEvent{rec.SimTS, k, rec.Data.Payload.Payload.inner.NewSize})
			}
		}
		handle.Close()
	}
	sort.Slice(posEvents, func(i, j int) bool { return posEvents[i].ts < posEvents[j].ts })

	index := map[int64]int64{}
	for ts, byVenue := range spotMids {
		v := make([]int64, 0, len(byVenue))
		for _, m := range byVenue {
			v = append(v, m)
		}
		sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
		n := len(v)
		med := v[n/2]
		if n%2 == 0 {
			med = (v[n/2-1] + v[n/2]) / 2
		}
		index[ts] = med
	}

	stamps := make([]int64, 0, len(perpMids))
	for ts := range perpMids {
		if _, ok := index[ts]; ok {
			stamps = append(stamps, ts)
		}
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i] < stamps[j] })
	if len(stamps) == 0 {
		fmt.Fprintln(os.Stderr, "no timestamp has both books two-sided")
		os.Exit(1)
	}

	// Walk timestamps forward, advancing the position timeline alongside.
	edges := []float64{0, 5, 10, 20, 30, 1e9}
	labels := []string{"|basis| 0-5%", "5-10%", "10-20%", "20-30%", ">=30%"}
	type stat struct {
		n        int
		sumPos   float64
		maxPos   float64
		sumBasis float64
	}
	stats := make([]stat, len(labels))
	position := map[key]int64{}
	next := 0
	for _, ts := range stamps {
		for next < len(posEvents) && posEvents[next].ts <= ts {
			position[posEvents[next].k] = posEvents[next].pos
			next++
		}
		var aggregate int64
		for _, p := range position {
			aggregate += p
		}
		mids := perpMids[ts]
		sort.Slice(mids, func(i, j int) bool { return mids[i] < mids[j] })
		perpMid := mids[len(mids)/2]
		spotMid := index[ts]
		if spotMid <= 0 {
			continue
		}
		basis := math.Abs(float64(perpMid-spotMid) / float64(spotMid) * 100)
		pos := math.Abs(float64(aggregate) / (*basePrecision))
		for i := 0; i < len(edges)-1; i++ {
			if basis >= edges[i] && basis < edges[i+1] {
				stats[i].n++
				stats[i].sumPos += pos
				stats[i].sumBasis += basis
				if pos > stats[i].maxPos {
					stats[i].maxPos = pos
				}
				break
			}
		}
	}

	fmt.Printf("%s aggregate position against %s |basis|, class cap %.0f contracts\n",
		*rolePrefix, *perp, *capContracts)
	fmt.Printf("%-16s %10s %12s %14s %14s %12s\n",
		"basis band", "samples", "mean basis", "mean position", "max position", "% of cap")
	for i, label := range labels {
		s := stats[i]
		if s.n == 0 {
			continue
		}
		mean := s.sumPos / float64(s.n)
		fmt.Printf("%-16s %10d %11.2f%% %14.1f %14.1f %11.1f%%\n",
			label, s.n, s.sumBasis/float64(s.n), mean, s.maxPos, mean / *capContracts * 100)
	}
}
