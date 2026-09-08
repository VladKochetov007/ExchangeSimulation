// Command quoteside attributes quote provision to participant classes, per book
// and per side, from order placements rather than from fills.
//
// Attributing quote provision from fills is circular: a book with no bid has no
// bid-side fills by construction, so the absence would be its own evidence.
// OrderAccepted records what was placed, whether or not anything traded.
//
// Placements are split into run quarters, because the two candidate explanations
// for a missing side make opposite time predictions. A quoting policy that never
// bids for a contract shows a low ratio from the first minutes and flat
// thereafter; a dealer withdrawing under an inventory or margin limit starts
// healthy and declines.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type placement struct {
	Side  string `json:"side"`
	Price int64  `json:"price"`
}

type level struct {
	Price int64 `json:"price"`
}

// snapshot carries only what a midpoint needs. Moneyness has to be evaluated at
// the instant a quote is placed: options relist through the run, so a contract
// classified by terminal spot may have been on the other side of the strike when
// it was actually being quoted. RT-052 recorded that confound.
type snapshot struct {
	Bids []level `json:"bids"`
	Asks []level `json:"asks"`
}

func (sn snapshot) mid() (int64, bool) {
	if len(sn.Bids) == 0 || len(sn.Asks) == 0 {
		return 0, false
	}
	bid, ask := sn.Bids[0].Price, sn.Asks[0].Price
	if bid <= 0 || ask <= 0 || ask < bid {
		return 0, false
	}
	return (bid + ask) / 2, true
}

type record struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
	Data     struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Symbol  string  `json:"symbol"`
			Side    string  `json:"side"`
			Bids    []level `json:"bids"`
			Asks    []level `json:"asks"`
			Payload struct {
				placement
				snapshot
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

func class(role string) string {
	cut := strings.LastIndex(role, "_")
	if cut <= 0 || cut+1 == len(role) {
		return role
	}
	for _, r := range role[cut+1:] {
		if r < '0' || r > '9' {
			return role
		}
	}
	return role[:cut]
}

var optionPattern = regexp.MustCompile(`^ABC-\d+-(\d+)-([CP])$`)

// contract returns an option's strike and whether it is a call.
func contract(symbol string) (strike float64, isCall, isOption bool) {
	m := optionPattern.FindStringSubmatch(symbol)
	if m == nil {
		return 0, false, false
	}
	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false, false
	}
	return value, m[2] == "C", true
}

// signedMoneyness is positive when the contract is in the money, for both calls
// and puts, so one scale orders the whole surface.
func signedMoneyness(spot, strike float64, isCall bool) float64 {
	if spot <= 0 {
		return 0
	}
	if isCall {
		return (spot - strike) / spot
	}
	return (strike - spot) / spot
}

// bucketOf names a moneyness band. The bands are wider away from the money
// because that is where contracts are sparse.
func bucketOf(m float64) (int, string) {
	switch {
	case m > 0.05:
		return 0, "deep ITM  (>+5%)"
	case m > 0.01:
		return 1, "ITM  (+1..+5%)"
	case m >= -0.01:
		return 2, "at the money (±1%)"
	case m >= -0.05:
		return 3, "OTM  (-1..-5%)"
	default:
		return 4, "deep OTM  (<-5%)"
	}
}

type tally struct {
	bid, ask [4]int
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	greeksPath := flag.String("greeks", "", "greeks.json (default <dir>/greeks.json)")
	focus := flag.String("class", "option_dealer", "participant class to attribute")
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
	roleOf := map[key]string{}
	for _, row := range snap.InitialAccounts {
		roleOf[key{row.VenueID, row.ClientID}] = class(row.Role)
	}

	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	nested, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	files = append(files, nested...)
	sort.Strings(files)

	// Two passes: the first establishes the time span so quarters are defined
	// against the actual run rather than against an assumed horizon.
	var first, last int64
	marker := []byte(`"OrderAccepted"`)
	scan := func(fn func(rec record)) {
		for _, path := range files {
			handle, err := os.Open(path)
			if err != nil {
				fmt.Fprintln(os.Stderr, "open:", err)
				os.Exit(1)
			}
			scanner := bufio.NewScanner(handle)
			scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
			for scanner.Scan() {
				line := scanner.Bytes()
				if !bytes.Contains(line, marker) {
					continue
				}
				var rec record
				if err := json.Unmarshal(line, &rec); err != nil || rec.Event != "OrderAccepted" {
					continue
				}
				fn(rec)
			}
			handle.Close()
		}
	}
	scan(func(rec record) {
		if first == 0 || rec.SimTS < first {
			first = rec.SimTS
		}
		if rec.SimTS > last {
			last = rec.SimTS
		}
	})
	span := last - first
	if span <= 0 {
		fmt.Fprintln(os.Stderr, "no OrderAccepted events found")
		os.Exit(1)
	}

	// Pass 2: contemporaneous ABC/USD consensus mid per timestamp, built with the
	// venue's own median-across-venues rule.
	spotByTS := map[int64]map[string]int64{}
	snapMarker := []byte(`"BookSnapshot"`)
	for _, path := range files {
		if !strings.Contains(path, "ABC-USD") {
			continue
		}
		handle, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open:", err)
			os.Exit(1)
		}
		scanner := bufio.NewScanner(handle)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.Contains(line, snapMarker) {
				continue
			}
			var rec record
			if err := json.Unmarshal(line, &rec); err != nil || rec.Event != "BookSnapshot" {
				continue
			}
			shot := snapshot{Bids: rec.Data.Payload.Bids, Asks: rec.Data.Payload.Asks}
			mid, ok := shot.mid()
			if !ok {
				continue
			}
			if spotByTS[rec.SimTS] == nil {
				spotByTS[rec.SimTS] = map[string]int64{}
			}
			spotByTS[rec.SimTS][rec.Data.VenueID] = mid
		}
		handle.Close()
	}
	consensus := make(map[int64]float64, len(spotByTS))
	stamps := make([]int64, 0, len(spotByTS))
	for ts, byVenue := range spotByTS {
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
		consensus[ts] = float64(med) / 100000
		stamps = append(stamps, ts)
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i] < stamps[j] })
	if len(stamps) == 0 {
		fmt.Fprintln(os.Stderr, "no ABC/USD snapshot is two-sided")
		os.Exit(1)
	}
	spotAt := func(ts int64) (float64, bool) {
		i := sort.Search(len(stamps), func(i int) bool { return stamps[i] > ts })
		if i == 0 {
			return 0, false
		}
		return consensus[stamps[i-1]], true
	}

	// Pass 3: bucket dealer placements by quote-time moneyness.
	buckets := make([]tally, 5)
	names := make([]string, 5)
	unpriced := 0
	scan(func(rec record) {
		symbol := rec.Data.Payload.Symbol
		strike, isCall, isOption := contract(symbol)
		if !isOption {
			return
		}
		if roleOf[key{rec.Data.VenueID, rec.ClientID}] != *focus {
			return
		}
		spot, ok := spotAt(rec.SimTS)
		if !ok {
			unpriced++
			return
		}
		idx, label := bucketOf(signedMoneyness(spot, strike, isCall))
		names[idx] = label
		side := rec.Data.Payload.Side
		if side == "" {
			side = rec.Data.Payload.Payload.Side
		}
		quarter := int((rec.SimTS - first) * 4 / span)
		if quarter > 3 {
			quarter = 3
		}
		if side == "BUY" {
			buckets[idx].bid[quarter]++
		} else {
			buckets[idx].ask[quarter]++
		}
	})

	fmt.Printf("%s placements by moneyness AT QUOTE TIME (spot = median ABC/USD mid)\n", *focus)
	if unpriced > 0 {
		fmt.Printf("  placements with no prior spot: %d\n", unpriced)
	}
	fmt.Printf("%-22s %10s %10s %9s   %s\n", "bucket", "bids", "asks", "bid/ask", "bid/ask by quarter")
	for i := 0; i < 5; i++ {
		bids, asks := sum(buckets[i].bid), sum(buckets[i].ask)
		if bids+asks == 0 {
			continue
		}
		ratio := "-"
		if asks > 0 {
			ratio = fmt.Sprintf("%.1f%%", float64(bids)/float64(asks)*100)
		}
		quarters := make([]string, 4)
		for q := 0; q < 4; q++ {
			if buckets[i].ask[q] > 0 {
				quarters[q] = fmt.Sprintf("%.0f%%", float64(buckets[i].bid[q])/float64(buckets[i].ask[q])*100)
			} else if buckets[i].bid[q] > 0 {
				quarters[q] = "inf"
			} else {
				quarters[q] = "-"
			}
		}
		fmt.Printf("%-22s %10d %10d %9s   %s\n",
			names[i], bids, asks, ratio, strings.Join(quarters, " "))
	}
}

func sum(v [4]int) int {
	total := 0
	for _, x := range v {
		total += x
	}
	return total
}
