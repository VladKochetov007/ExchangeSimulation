// Command closureregression identifies the population's accounting gap by
// regression instead of by valuation surgery.
//
// RT-024 could bound the gap only by magnitude, because the residual mixes it
// with revaluation. E-036 tried to remove revaluation by fixing the valuation
// spec and failed: AccountValuationSpec reaches wallet balances only, while
// derivative exposure is valued from the instruments' own marks.
//
// This separates the two by their different behaviour across seeds:
//
//	residual(seed) = inventory × drift(seed) + gap
//
// The slope is the population's net long inventory; the INTERCEPT is the
// accounting gap. It needs no new API — only runs at several seeds, which
// produce different drifts.
//
// The experiment answers nothing if the drifts cluster: with no spread in the
// regressor the intercept is not identified. The tool reports the drift spread
// so that outcome is visible rather than hidden behind a fitted number.
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

type runFile struct {
	InitialAccounts  []account `json:"initial_accounts"`
	TerminalAccounts []account `json:"terminal_accounts"`
	VenueLedgers     []ledger  `json:"venue_ledgers"`
}

type point struct {
	label    string
	drift    float64
	residual float64
}

func main() {
	asset := flag.String("asset", "ABC", "base asset whose drift drives revaluation")
	quote := flag.String("quote", "USD", "report asset")
	precision := flag.Float64("precision", 100000, "report-asset units per whole unit")
	flag.Parse()
	files := flag.Args()
	if len(files) < 3 {
		fmt.Fprintln(os.Stderr, "give at least three greeks.json paths; the fit needs spread in the drift")
		os.Exit(2)
	}

	var points []point
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read", path, err)
			os.Exit(1)
		}
		var data runFile
		if err := json.Unmarshal(raw, &data); err != nil {
			fmt.Fprintln(os.Stderr, "decode", path, err)
			os.Exit(1)
		}
		// Client IDs are allocated per venue, so a bare client key collides
		// across venues. Measured to be benign — every shared id carries the
		// same role and initial equity — but keyed properly here anyway.
		start := map[string]int64{}
		var from, to int64
		for _, row := range data.InitialAccounts {
			start[row.VenueID+"/"+fmt.Sprint(row.ClientID)] = row.Account.Equity
			if mark, ok := row.Marks[*asset]; ok {
				from = mark
			}
		}
		var residual int64
		for _, row := range data.TerminalAccounts {
			initial, seen := start[row.VenueID+"/"+fmt.Sprint(row.ClientID)]
			if !seen {
				continue
			}
			residual += row.Account.Equity - initial
			if mark, ok := row.Marks[*asset]; ok {
				to = mark
			}
		}
		for _, l := range data.VenueLedgers {
			residual += l.FeeRevenue[*quote] + l.InsuranceFund[*quote]
		}
		if from == 0 {
			fmt.Fprintln(os.Stderr, "no initial mark for", *asset, "in", path)
			os.Exit(1)
		}
		points = append(points, point{
			label:    path,
			drift:    float64(to-from) / float64(from),
			residual: float64(residual) / *precision,
		})
	}

	sort.Slice(points, func(i, j int) bool { return points[i].drift < points[j].drift })
	fmt.Printf("%-44s %12s %18s\n", "run", "drift", "residual (quote)")
	for _, p := range points {
		fmt.Printf("%-44s %11.4f%% %18.0f\n", p.label, 100*p.drift, p.residual)
	}

	n := float64(len(points))
	var sx, sy, sxx, sxy float64
	for _, p := range points {
		sx += p.drift
		sy += p.residual
		sxx += p.drift * p.drift
		sxy += p.drift * p.residual
	}
	denom := n*sxx - sx*sx
	if denom == 0 {
		fmt.Println("\nthe drifts have no spread: the intercept is not identified and this run answers nothing")
		return
	}
	slope := (n*sxy - sx*sy) / denom
	intercept := (sy - slope*sx) / n

	var ssRes, ssTot, meanAbs float64
	meanY := sy / n
	for _, p := range points {
		fit := slope*p.drift + intercept
		ssRes += (p.residual - fit) * (p.residual - fit)
		ssTot += (p.residual - meanY) * (p.residual - meanY)
		meanAbs += math.Abs(p.residual)
	}
	meanAbs /= n
	r2 := 1.0
	if ssTot > 0 {
		r2 = 1 - ssRes/ssTot
	}
	stdErr := math.Sqrt(ssRes/math.Max(n-2, 1)) * math.Sqrt(1/n+(sx/n)*(sx/n)/(sxx-sx*sx/n))

	fmt.Printf("\ndrift spread: %.4f%% (min %.4f%%, max %.4f%%)\n",
		100*(points[len(points)-1].drift-points[0].drift),
		100*points[0].drift, 100*points[len(points)-1].drift)
	fmt.Printf("slope    (implied inventory, quote units per unit drift): %18.0f\n", slope)
	fmt.Printf("intercept (ACCOUNTING GAP):                               %18.0f  +/- %.0f\n", intercept, stdErr)
	fmt.Printf("R^2: %.6f     mean |residual|: %.0f\n", r2, meanAbs)
	fmt.Printf("intercept as %% of mean |residual|: %.3f%%\n", 100*intercept/meanAbs)
}
