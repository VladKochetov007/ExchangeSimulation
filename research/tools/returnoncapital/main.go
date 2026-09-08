// Command returnoncapital ranks the actor classes by rate of return as well as
// by absolute result, and reports whether the two orderings agree.
//
// The classes are endowed at different scales by design, so an absolute
// class-level PnL is a comparison of differently-sized books. The population
// artifact already carries each participant's initial marked equity in USD, so
// the normalisation needs no new instrumentation — only the division nobody
// currently performs.
//
// If the two rankings agree, absolute PnL is a fair summary. If they disagree,
// it is ranking capital allocation as much as skill, and every comparison drawn
// from it inherits that.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

type account struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
	Account  struct {
		Equity int64 `json:"equity"`
	} `json:"account"`
}

type runFile struct {
	InitialAccounts  []account `json:"initial_accounts"`
	TerminalAccounts []account `json:"terminal_accounts"`
}

func roleClass(role string) string {
	cut := strings.LastIndex(role, "_")
	if cut < 0 {
		return role
	}
	if _, err := strconv.Atoi(role[cut+1:]); err != nil {
		return role
	}
	return role[:cut]
}

type stats struct {
	capital, change float64
	heads           int
}

func main() {
	path := flag.String("file", "", "greeks.json from a run")
	precision := flag.Float64("precision", 100000, "report-asset units per whole unit")
	flag.Parse()
	if *path == "" {
		fmt.Fprintln(os.Stderr, "-file is required")
		os.Exit(2)
	}
	raw, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	var data runFile
	if err := json.Unmarshal(raw, &data); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}

	// Keyed by venue and client: ids are allocated per venue.
	start := map[string]int64{}
	for _, row := range data.InitialAccounts {
		start[row.VenueID+"/"+fmt.Sprint(row.ClientID)] = row.Account.Equity
	}
	byClass := map[string]*stats{}
	for _, row := range data.TerminalAccounts {
		key := row.VenueID + "/" + fmt.Sprint(row.ClientID)
		initial, seen := start[key]
		if !seen {
			continue
		}
		name := roleClass(row.Role)
		if byClass[name] == nil {
			byClass[name] = &stats{}
		}
		byClass[name].capital += float64(initial) / *precision
		byClass[name].change += float64(row.Account.Equity-initial) / *precision
		byClass[name].heads++
	}

	names := make([]string, 0, len(byClass))
	for name := range byClass {
		names = append(names, name)
	}

	byAbs := append([]string(nil), names...)
	sort.Slice(byAbs, func(i, j int) bool { return byClass[byAbs[i]].change > byClass[byAbs[j]].change })
	rate := func(name string) float64 {
		s := byClass[name]
		if s.capital == 0 {
			return math.NaN()
		}
		return 100 * s.change / math.Abs(s.capital)
	}
	byRate := append([]string(nil), names...)
	sort.Slice(byRate, func(i, j int) bool { return rate(byRate[i]) > rate(byRate[j]) })

	absRank := map[string]int{}
	for i, name := range byAbs {
		absRank[name] = i + 1
	}

	fmt.Printf("%-26s %6s %16s %16s %10s %8s %8s\n",
		"class (by return)", "heads", "capital", "change", "return", "rank R", "rank abs")
	for i, name := range byRate {
		s := byClass[name]
		fmt.Printf("%-26s %6d %16.0f %16.0f %9.3f%% %8d %8d\n",
			name, s.heads, s.capital, s.change, rate(name), i+1, absRank[name])
	}

	// How far does the ordering move? Sum of absolute rank displacement.
	moved, maxMove := 0, 0
	for i, name := range byRate {
		d := absRank[name] - (i + 1)
		if d < 0 {
			d = -d
		}
		moved += d
		if d > maxMove {
			maxMove = d
		}
	}
	fmt.Printf("\ntotal rank displacement: %d over %d classes (max single move %d)\n",
		moved, len(names), maxMove)
	fmt.Println("top three by absolute change:", strings.Join(byAbs[:min(3, len(byAbs))], ", "))
	fmt.Println("top three by return:        ", strings.Join(byRate[:min(3, len(byRate))], ", "))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
