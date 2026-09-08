// Command classpnl ranks participant classes by trading result with the shared
// revaluation tide removed.
//
// Terminal-minus-initial equity cannot rank participants here. Every one of them
// is net long the base asset, so one mark move lifts or drops all of them at
// once, and a class endowed with more inventory shows a larger "result" without
// having traded. RT-024 records that mistake. This tool removes the term
// exactly:
//
//	carry_adjusted_pnl = Δequity − Σ_asset initial_balance × Δmark
//
// Every participant starts flat in derivatives, so initial wallet balances carry
// the whole revaluation term.
//
// The decomposition is only worth reading if it closes, so the tool runs that
// check on itself before printing any ranking:
//
//	Σ carry_adjusted_pnl + exchange_take ≈ 0
//
// A closure error above the configured tolerance makes the ranking meaningless
// rather than merely noisy, and the tool says so and exits non-zero instead of
// printing numbers a reader would be tempted to quote.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type assetBalance struct {
	Asset    string `json:"asset"`
	NetAsset int64  `json:"net_asset"`
}

type account struct {
	VenueID  string           `json:"venue_id"`
	ClientID uint64           `json:"client_id"`
	Role     string           `json:"role"`
	Marks    map[string]int64 `json:"marks"`
	Account  struct {
		SpotBalances []assetBalance `json:"spot_balances"`
		PerpBalances []assetBalance `json:"perp_balances"`
		Equity       int64          `json:"equity"`
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

// balances totals every wallet an account holds per asset. Perp and spot wallets
// are both revalued by the same marks, so both belong in the revaluation term.
func (a account) balances() map[string]int64 {
	totals := map[string]int64{}
	for _, balance := range a.Account.SpotBalances {
		totals[balance.Asset] += balance.NetAsset
	}
	for _, balance := range a.Account.PerpBalances {
		totals[balance.Asset] += balance.NetAsset
	}
	return totals
}

// class strips a participant's trailing index so numbered participants of one
// role aggregate together. Grouping by the numbered role instead produces one
// participant per group, which RT-021 records as a measurement error.
func class(role string) string {
	cut := strings.LastIndex(role, "_")
	if cut <= 0 {
		return role
	}
	for _, r := range role[cut+1:] {
		if r < '0' || r > '9' {
			return role
		}
	}
	if cut+1 == len(role) {
		return role
	}
	return role[:cut]
}

// key identifies one participant. Client ids are allocated per venue, so the
// venue has to be part of the key or the three venues' rows collide.
type key struct {
	venue  string
	client uint64
}

type classTotals struct {
	carry, raw, gross int64
	heads             int
}

func main() {
	path := flag.String("file", "", "greeks.json from a run")
	reportAsset := flag.String("asset", "USD", "reporting asset of the equity figures")
	precision := flag.Float64("precision", 100000, "report-asset units per whole unit")
	tolerance := flag.Float64("closure-tolerance", 0.01, "maximum |closure| as a fraction of gross flow before the ranking is refused")
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

	type opening struct {
		equity   int64
		balances map[string]int64
	}
	start := map[key]opening{}
	startMarks := map[string]int64{}
	for _, row := range data.InitialAccounts {
		start[key{row.VenueID, row.ClientID}] = opening{row.Account.Equity, row.balances()}
		for asset, mark := range row.Marks {
			startMarks[asset] = mark
		}
	}

	byClass := map[string]*classTotals{}
	endMarks := map[string]int64{}
	var totalCarry, totalGross int64
	unmatched := 0
	for _, row := range data.TerminalAccounts {
		open, seen := start[key{row.VenueID, row.ClientID}]
		if !seen {
			unmatched++
			continue
		}
		for asset, mark := range row.Marks {
			endMarks[asset] = mark
		}
		revaluation := int64(0)
		for asset, balance := range open.balances {
			delta := row.Marks[asset] - startMarks[asset]
			if delta == 0 {
				continue
			}
			// Balances are in the asset's own precision and marks are report
			// units per whole asset, so the product needs the asset precision
			// out. The mark carries it: USD is quoted against itself.
			revaluation += int64(float64(balance) / assetPrecision(asset) * float64(delta))
		}
		delta := row.Account.Equity - open.equity
		name := class(row.Role)
		if byClass[name] == nil {
			byClass[name] = &classTotals{}
		}
		totals := byClass[name]
		totals.raw += delta
		totals.carry += delta - revaluation
		totals.gross += abs(delta - revaluation)
		totals.heads++
		totalCarry += delta - revaluation
		totalGross += abs(delta - revaluation)
	}

	take := int64(0)
	for _, l := range data.VenueLedgers {
		take += l.FeeRevenue[*reportAsset] + l.InsuranceFund[*reportAsset]
	}

	closure := totalCarry + take
	ratio := math.NaN()
	if totalGross != 0 {
		ratio = math.Abs(float64(closure)) / float64(totalGross)
	}
	fmt.Printf("closure self-test\n")
	fmt.Printf("  Σ carry-adjusted pnl %+.0f %s\n", float64(totalCarry)/(*precision), *reportAsset)
	fmt.Printf("  exchange take        %+.0f %s\n", float64(take)/(*precision), *reportAsset)
	fmt.Printf("  residual             %+.0f %s (%.4f%% of gross)\n",
		float64(closure)/(*precision), *reportAsset, ratio*100)
	if unmatched != 0 {
		fmt.Printf("  unmatched terminal rows: %d\n", unmatched)
	}
	if !(ratio <= *tolerance) {
		fmt.Fprintf(os.Stderr,
			"\nREFUSED: closure residual is %.4f%% of gross flow, above the %.4f%% tolerance.\n"+
				"The decomposition does not account for the run, so the class ranking it would\n"+
				"produce is not evidence. No ranking printed.\n", ratio*100, *tolerance*100)
		os.Exit(3)
	}

	names := make([]string, 0, len(byClass))
	for name := range byClass {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return byClass[names[i]].carry < byClass[names[j]].carry
	})
	fmt.Printf("\nclass ranking by carry-adjusted pnl, donors first\n")
	fmt.Printf("%-28s %5s %18s %18s %16s\n", "class", "n", "carry-adjusted", "raw Δequity", "per head")
	for _, name := range names {
		totals := byClass[name]
		fmt.Printf("%-28s %5d %18.0f %18.0f %16.0f\n", name, totals.heads,
			float64(totals.carry)/(*precision), float64(totals.raw)/(*precision),
			float64(totals.carry)/(*precision)/float64(totals.heads))
	}
}

// assetPrecision is the fixed-point scale of one whole unit of an asset. USD is
// the reporting asset and is quoted against itself.
func assetPrecision(asset string) float64 {
	switch asset {
	case "USD":
		return 100000
	default:
		return 1e8
	}
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
