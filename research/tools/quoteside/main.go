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

type record struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
	Data     struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Symbol  string `json:"symbol"`
			Side    string `json:"side"`
			Payload struct {
				placement
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

// moneyness reports whether an option symbol is in the money at the given spot,
// and whether the symbol is an option at all.
func moneyness(symbol string, spot float64) (inTheMoney, isOption bool) {
	m := optionPattern.FindStringSubmatch(symbol)
	if m == nil {
		return false, false
	}
	strike, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return false, false
	}
	if m[2] == "C" {
		return strike < spot, true
	}
	return strike > spot, true
}

type tally struct {
	bid, ask [4]int
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	greeksPath := flag.String("greeks", "", "greeks.json (default <dir>/greeks.json)")
	spot := flag.Float64("spot", 49295.05, "terminal spot used to classify moneyness")
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

	byGroup := map[string]map[string]*tally{"in the money": {}, "out of the money": {}}
	scan(func(rec record) {
		symbol := rec.Data.Payload.Symbol
		inMoney, isOption := moneyness(symbol, *spot)
		if !isOption {
			return
		}
		side := rec.Data.Payload.Side
		if side == "" {
			side = rec.Data.Payload.Payload.Side
		}
		name := roleOf[key{rec.Data.VenueID, rec.ClientID}]
		if name == "" {
			return
		}
		group := "out of the money"
		if inMoney {
			group = "in the money"
		}
		if byGroup[group][name] == nil {
			byGroup[group][name] = &tally{}
		}
		quarter := int((rec.SimTS - first) * 4 / span)
		if quarter > 3 {
			quarter = 3
		}
		if side == "BUY" {
			byGroup[group][name].bid[quarter]++
		} else {
			byGroup[group][name].ask[quarter]++
		}
	})

	for _, group := range []string{"in the money", "out of the money"} {
		fmt.Printf("\n%s option books — placements by class\n", group)
		fmt.Printf("%-24s %10s %10s %8s   %s\n", "class", "bids", "asks", "bid/ask", "bid/ask by quarter")
		names := make([]string, 0, len(byGroup[group]))
		for name := range byGroup[group] {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool {
			a, b := byGroup[group][names[i]], byGroup[group][names[j]]
			return sum(a.ask)+sum(a.bid) > sum(b.ask)+sum(b.bid)
		})
		for _, name := range names {
			t := byGroup[group][name]
			bids, asks := sum(t.bid), sum(t.ask)
			ratio := "-"
			if asks > 0 {
				ratio = fmt.Sprintf("%.1f%%", float64(bids)/float64(asks)*100)
			}
			quarters := make([]string, 4)
			for q := 0; q < 4; q++ {
				// Print the counts alongside the ratio: a ratio computed on a
				// handful of placements reads the same as one computed on
				// hundreds of thousands, and only the counts show the difference.
				if t.ask[q] > 0 {
					quarters[q] = fmt.Sprintf("%.0f%%(%d/%d)", float64(t.bid[q])/float64(t.ask[q])*100, t.bid[q], t.ask[q])
				} else if t.bid[q] > 0 {
					quarters[q] = fmt.Sprintf("inf(%d/0)", t.bid[q])
				} else {
					quarters[q] = "-(0/0)"
				}
			}
			fmt.Printf("%-24s %10d %10d %8s   %s\n",
				name, bids, asks, ratio, strings.Join(quarters, " "))
		}
	}
}

func sum(v [4]int) int {
	total := 0
	for _, x := range v {
		total += x
	}
	return total
}
