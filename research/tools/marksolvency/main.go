// Command marksolvency asks whether the risk engine's mark hides insolvency that
// the order book would reveal.
//
// A margin instrument is marked at index + clamp(EMA(mid - index), +/- band/2),
// so once the clamp binds the mark stops following the book. Margin and
// liquidation consume that mark. This tool revalues every terminal perp position
// at the book midpoint instead:
//
//	equity_at_book = equity - unrealized_at_mark + (book_mid - entry) * size / precision
//
// and reports accounts that are non-negative at the mark and negative at the
// book. Those are positions the risk engine believes are solvent and that the
// book says cannot be closed out at that value.
//
// It reports the population's perp exposure first, because a sign-flip count of
// zero means something quite different when nobody holds a position than when
// everybody does.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type positionSnapshot struct {
	Symbol        string `json:"symbol"`
	Size          int64  `json:"size"`
	EntryPrice    int64  `json:"entry_price"`
	UnrealizedPnL int64  `json:"unrealized_pnl"`
	MarkPrice     *int64 `json:"mark_price,omitempty"`
}

type account struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
	Account  struct {
		Equity    int64              `json:"equity"`
		Positions []positionSnapshot `json:"positions"`
	} `json:"account"`
}

type greeks struct {
	TerminalAccounts []account `json:"terminal_accounts"`
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

func main() {
	path := flag.String("file", "", "greeks.json from a run")
	symbol := flag.String("symbol", "ABC-PERP", "margin instrument to revalue")
	mids := flag.String("book-mids", "", "terminal book midpoints as venue=price,venue=price")
	basePrecision := flag.Float64("base-precision", 1e8, "base-asset units per whole unit")
	reportPrecision := flag.Float64("precision", 100000, "report-asset units per whole unit")
	flag.Parse()
	if *path == "" || *mids == "" {
		fmt.Fprintln(os.Stderr, "-file and -book-mids are required")
		os.Exit(2)
	}

	bookMid := map[string]int64{}
	for _, pair := range strings.Split(*mids, ",") {
		venue, value, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if !ok {
			fmt.Fprintln(os.Stderr, "bad -book-mids entry:", pair)
			os.Exit(2)
		}
		var price int64
		if _, err := fmt.Sscan(value, &price); err != nil || price <= 0 {
			fmt.Fprintln(os.Stderr, "bad midpoint:", pair)
			os.Exit(2)
		}
		bookMid[venue] = price
	}

	raw, err := os.ReadFile(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	var data greeks
	if err := json.Unmarshal(raw, &data); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}

	type flip struct {
		venue, role            string
		size                   int64
		equityMark, equityBook int64
	}
	var flips []flip
	holders, longs, shorts := 0, int64(0), int64(0)
	var grossExposure, worstShortfall, totalShortfall int64
	byClass := map[string]int64{}
	revaluation := map[string]int64{}

	for _, row := range data.TerminalAccounts {
		mid, known := bookMid[row.VenueID]
		if !known {
			continue
		}
		delta := int64(0)
		var size int64
		for _, pos := range row.Account.Positions {
			if pos.Symbol != *symbol || pos.Size == 0 {
				continue
			}
			size += pos.Size
			atBook := int64(float64(mid-pos.EntryPrice) * float64(pos.Size) / *basePrecision)
			delta += atBook - pos.UnrealizedPnL
		}
		if size == 0 {
			continue
		}
		holders++
		if size > 0 {
			longs += size
		} else {
			shorts += size
		}
		if size > 0 {
			grossExposure += size
		} else {
			grossExposure -= size
		}
		byClass[class(row.Role)] += size
		// delta is book minus mark, so its negation is how much the reported
		// equity sits above what the book would pay.
		revaluation[class(row.Role)] += -delta
		equityBook := row.Account.Equity + delta
		if row.Account.Equity >= 0 && equityBook < 0 {
			flips = append(flips, flip{row.VenueID, row.Role, size, row.Account.Equity, equityBook})
			totalShortfall += -equityBook
			if -equityBook > worstShortfall {
				worstShortfall = -equityBook
			}
		}
	}

	fmt.Printf("%s exposure at terminal\n", *symbol)
	fmt.Printf("  accounts holding a position: %d\n", holders)
	fmt.Printf("  long %.2f, short %.2f, gross %.2f contracts\n",
		float64(longs)/(*basePrecision), float64(shorts)/(*basePrecision),
		float64(grossExposure)/(*basePrecision))
	names := make([]string, 0, len(byClass))
	for name := range byClass {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return byClass[names[i]] > byClass[names[j]] })
	fmt.Printf("  net position by class:\n")
	for _, name := range names {
		fmt.Printf("    %-26s %+.2f\n", name, float64(byClass[name])/(*basePrecision))
	}

	// Even when no account changes sign, the clamp still moves reported value
	// between classes: a class long the perp is credited above what the book
	// would pay it, and a class short it is credited below. That is a scope
	// limit on any ranking computed at marks, so it is reported whether or not
	// solvency is threatened.
	fmt.Printf("\nmark-vs-book revaluation by class (positive = reported above book)\n")
	revalNames := make([]string, 0, len(revaluation))
	for name := range revaluation {
		revalNames = append(revalNames, name)
	}
	sort.Slice(revalNames, func(i, j int) bool { return revaluation[revalNames[i]] > revaluation[revalNames[j]] })
	for _, name := range revalNames {
		fmt.Printf("  %-26s %+16.0f\n", name, float64(revaluation[name])/(*reportPrecision))
	}

	fmt.Printf("\nsolvent at mark, insolvent at book: %d accounts\n", len(flips))
	if len(flips) == 0 {
		fmt.Printf("  no account changes sign. The mark gap is real but does not\n")
		fmt.Printf("  threaten solvency in this run at these position sizes.\n")
		return
	}
	sort.Slice(flips, func(i, j int) bool { return flips[i].equityBook < flips[j].equityBook })
	fmt.Printf("%-9s %-24s %14s %16s %16s\n", "venue", "role", "size", "equity@mark", "equity@book")
	for _, f := range flips {
		fmt.Printf("%-9s %-24s %14.2f %16.0f %16.0f\n", f.venue, f.role,
			float64(f.size)/(*basePrecision),
			float64(f.equityMark)/(*reportPrecision),
			float64(f.equityBook)/(*reportPrecision))
	}
	fmt.Printf("\naggregate hidden shortfall %.0f, worst single account %.0f\n",
		float64(totalShortfall)/(*reportPrecision), float64(worstShortfall)/(*reportPrecision))
}
