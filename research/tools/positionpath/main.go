// Command positionpath reconstructs each participant's signed position over time
// from fill evidence, and reports how much of the run it spent against a
// configured position limit.
//
// A terminal snapshot cannot distinguish a strategy that trades actively and
// happens to end at its limit from one that reached the limit early and stopped
// responding. The distinction matters: the second is not a strategy competing,
// it is a constant. The tool therefore reports time-to-limit and the fraction of
// the run spent there, per participant, alongside the terminal position.
//
// Positions are built from the fill stream rather than the account snapshots, so
// this is an independent reconstruction of a quantity the snapshots also carry.
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

type fillPayload struct {
	Symbol  string `json:"symbol"`
	Qty     int64  `json:"qty"`
	Side    string `json:"side"`
	NewSize int64  `json:"new_size"`
}

// fillRecord covers both evidence shapes. A spot book writes the fill fields
// directly under the payload; the shared derivatives file wraps them, keeping
// only the symbol at the outer level. A reader that assumes the flat shape finds
// a symbol, counts the record, and then reads a zero quantity — which looks like
// a flat position rather than a parse failure.
type fillRecord struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
	Data     struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			fillPayload
			Payload *fillPayload `json:"payload"`
		} `json:"payload"`
	} `json:"data"`
}

// fill returns the fill fields from whichever level carries them.
func (r fillRecord) fill() fillPayload {
	if r.Data.Payload.Payload != nil {
		inner := *r.Data.Payload.Payload
		if inner.Symbol == "" {
			inner.Symbol = r.Data.Payload.Symbol
		}
		return inner
	}
	return r.Data.Payload.fillPayload
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

type path struct {
	role     string
	position int64
	lastTS   int64
	atLimit  int64 // nanoseconds spent at or beyond the limit
	firstHit int64
	hasHit   bool
	fills    int
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	greeksPath := flag.String("greeks", "", "greeks.json (default <dir>/greeks.json)")
	symbol := flag.String("symbol", "ABC-PERP", "instrument whose position is tracked")
	rolePrefix := flag.String("role-prefix", "carry_arb", "participant class to report")
	limit := flag.Float64("limit", 500, "position limit in whole contracts")
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
	var snapshot greeks
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "decode greeks:", err)
		os.Exit(1)
	}
	tracked := map[key]*path{}
	for _, row := range snapshot.InitialAccounts {
		if strings.HasPrefix(row.Role, *rolePrefix) {
			tracked[key{row.VenueID, row.ClientID}] = &path{role: row.Role}
		}
	}
	if len(tracked) == 0 {
		fmt.Fprintf(os.Stderr, "no participants with role prefix %q\n", *rolePrefix)
		os.Exit(1)
	}

	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	nested, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	files = append(files, nested...)
	sort.Strings(files)

	limitUnits := int64(*limit * *basePrecision)
	marker := []byte(`"OrderFill"`)
	var firstTS, lastTS int64
	for _, file := range files {
		handle, err := os.Open(file)
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
			var rec fillRecord
			if err := json.Unmarshal(line, &rec); err != nil || rec.Event != "OrderFill" {
				continue
			}
			if firstTS == 0 || rec.SimTS < firstTS {
				firstTS = rec.SimTS
			}
			if rec.SimTS > lastTS {
				lastTS = rec.SimTS
			}
			fill := rec.fill()
			if fill.Symbol != *symbol {
				continue
			}
			p := tracked[key{rec.Data.VenueID, rec.ClientID}]
			if p == nil {
				continue
			}
			// Credit the interval just ended to whatever state the position was
			// already in, then apply the fill.
			if p.lastTS != 0 && abs(p.position) >= limitUnits {
				p.atLimit += rec.SimTS - p.lastTS
			}
			p.lastTS = rec.SimTS
			// new_size is the exchange's own post-fill position, which is
			// authoritative and avoids accumulating sign errors across a long
			// fill stream.
			p.position = fill.NewSize
			p.fills++
			if !p.hasHit && abs(p.position) >= limitUnits {
				p.firstHit, p.hasHit = rec.SimTS, true
			}
		}
		handle.Close()
	}

	// The final interval runs to the end of the run.
	for _, p := range tracked {
		if p.lastTS != 0 && abs(p.position) >= limitUnits {
			p.atLimit += lastTS - p.lastTS
		}
	}

	span := lastTS - firstTS
	names := make([]key, 0, len(tracked))
	for k := range tracked {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if tracked[names[i]].role != tracked[names[j]].role {
			return tracked[names[i]].role < tracked[names[j]].role
		}
		return names[i].venue < names[j].venue
	})

	fmt.Printf("%s position paths, limit %.2f contracts, run span %.2f h\n",
		*symbol, *limit, float64(span)/3.6e12)
	fmt.Printf("%-9s %-16s %8s %14s %14s %12s\n",
		"venue", "role", "fills", "terminal", "first at limit", "time at limit")
	for _, k := range names {
		p := tracked[k]
		hit := "never"
		if p.hasHit {
			hit = fmt.Sprintf("%.2f h", float64(p.firstHit-firstTS)/3.6e12)
		}
		share := 0.0
		if span > 0 {
			share = float64(p.atLimit) / float64(span) * 100
		}
		fmt.Printf("%-9s %-16s %8d %14.2f %14s %11.1f%%\n",
			k.venue, p.role, p.fills, float64(p.position)/(*basePrecision), hit, share)
	}
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
