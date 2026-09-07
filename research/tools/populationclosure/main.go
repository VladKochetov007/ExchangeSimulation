// Command populationclosure screens whether the participants' gains and losses
// close against the venue's take.
//
//	residual = Σ (terminal equity − initial equity) + fee revenue + insurance fund
//
// The only legitimate source of a non-zero residual is revaluation: participants
// are net long the base asset, so a mark change lifts or drops everyone at once
// without anyone trading. The tool therefore prints the residual beside the gross
// participant flow and the mark drift, so the two can be compared.
//
// The residual is reported as the base-asset inventory it IMPLIES — residual
// divided by the mark drift — because that number can be checked against the
// configured endowment. Reporting it as a percentage of gross participant flow
// is worse than useless: gross flow is itself dominated by the same revaluation,
// so the ratio makes every run look catastrophic. That mistake is recorded in
// RT-024.
//
// It is a screen, not a proof: it does not compute revaluation exactly, and a
// residual consistent with the mark drift is not evidence that nothing is wrong.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
)

type account struct {
	VenueID  string           `json:"venue_id"`
	ClientID uint64           `json:"client_id"`
	Role     string           `json:"role"`
	Marks    map[string]int64 `json:"marks"`
	Account  struct {
		Equity int64 `json:"equity"`
	} `json:"account"`
}

type ledger struct {
	VenueID       string           `json:"venue_id"`
	FeeRevenue    map[string]int64 `json:"fee_revenue"`
	InsuranceFund map[string]int64 `json:"insurance_fund"`
}

type run struct {
	InitialAccounts  []account `json:"initial_accounts"`
	TerminalAccounts []account `json:"terminal_accounts"`
	VenueLedgers     []ledger  `json:"venue_ledgers"`
}

func main() {
	path := flag.String("file", "", "greeks.json from a run")
	asset := flag.String("asset", "USD", "report asset for the venue ledger")
	precision := flag.Float64("precision", 100000, "report-asset units per whole unit")
	base := flag.String("base", "ABC", "base asset whose mark drift can explain a residual")
	basePrecision := flag.Float64("base-precision", 1e8, "base-asset units per whole unit")
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
	var data run
	if err := json.Unmarshal(raw, &data); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}

	start := map[uint64]int64{}
	startMarks := map[string]int64{}
	for _, row := range data.InitialAccounts {
		start[row.ClientID] = row.Account.Equity
		for asset, mark := range row.Marks {
			startMarks[asset] = mark
		}
	}
	endMarks := map[string]int64{}
	type venueTotals struct {
		change, gross int64
		heads         int
	}
	byVenue := map[string]*venueTotals{}
	for _, row := range data.TerminalAccounts {
		initial, seen := start[row.ClientID]
		if !seen {
			continue
		}
		for asset, mark := range row.Marks {
			endMarks[asset] = mark
		}
		if byVenue[row.VenueID] == nil {
			byVenue[row.VenueID] = &venueTotals{}
		}
		delta := row.Account.Equity - initial
		byVenue[row.VenueID].change += delta
		byVenue[row.VenueID].gross += abs(delta)
		byVenue[row.VenueID].heads++
	}

	take := map[string]int64{}
	for _, l := range data.VenueLedgers {
		take[l.VenueID] = l.FeeRevenue[*asset] + l.InsuranceFund[*asset]
	}

	venues := make([]string, 0, len(byVenue))
	for venue := range byVenue {
		venues = append(venues, venue)
	}
	sort.Strings(venues)

	drift := 0.0
	if from, ok := startMarks[*base]; ok && from != 0 {
		drift = float64(endMarks[*base]-from) / float64(from)
	}
	fmt.Printf("%-10s %16s %14s %16s %14s %12s\n",
		"venue", "participant net", "venue take", "residual", "implied "+*base, "per head")
	var totalResidual, totalGross int64
	for _, venue := range venues {
		t := byVenue[venue]
		residual := t.change + take[venue]
		totalResidual += residual
		totalGross += t.gross
		impliedValue, perHead := math.NaN(), math.NaN()
		if drift != 0 {
			impliedValue = float64(residual) / drift / (*precision)
			if marks, ok := startMarks[*base]; ok && marks != 0 {
				units := impliedValue * (*precision) / float64(marks) * (*basePrecision)
				impliedValue = units / (*basePrecision)
				perHead = impliedValue / float64(byVenue[venue].heads)
			}
		}
		fmt.Printf("%-10s %16.0f %14.0f %16.0f %14.0f %12.0f\n",
			venue, float64(t.change)/(*precision), float64(take[venue])/(*precision),
			float64(residual)/(*precision), impliedValue, perHead)
	}
	fmt.Printf("%-10s %16s %14s %16.0f\n", "TOTAL", "", "", float64(totalResidual)/(*precision))
	_ = totalGross

	fmt.Println("\nmark drift (initial -> terminal), the only legitimate source of a residual:")
	assets := make([]string, 0, len(startMarks))
	for a := range startMarks {
		assets = append(assets, a)
	}
	sort.Strings(assets)
	for _, a := range assets {
		from, to := startMarks[a], endMarks[a]
		if from == 0 {
			continue
		}
		fmt.Printf("  %-6s %14d -> %14d  (%+.3f%%)\n", a, from, to,
			100*float64(to-from)/float64(from))
	}
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
