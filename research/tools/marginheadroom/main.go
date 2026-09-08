// Command marginheadroom reports how close a population came to liquidation.
//
// A simulation whose participants never approach insolvency is not exercising its
// margin engine, its liquidation ordering, its insurance fund or its bankruptcy
// accounting. Counting liquidations answers only whether the path was taken;
// headroom answers whether it was ever near.
//
// Headroom is equity over maintenance margin, so liquidation is due at 1.0. A
// participant holding no derivative position has zero maintenance and carries no
// liquidation risk at all: those are counted separately rather than folded in as
// infinitely safe, which would flatter the population.
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

type account struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
	Account  struct {
		Equity      int64 `json:"equity"`
		Maintenance int64 `json:"maintenance"`
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
	paths := flag.String("files", "", "comma-separated greeks.json paths, one per seed")
	precision := flag.Float64("precision", 100000, "report-asset units per whole unit")
	flag.Parse()
	if *paths == "" {
		fmt.Fprintln(os.Stderr, "-files is required")
		os.Exit(2)
	}

	fmt.Printf("%-28s %8s %10s %12s %14s %16s\n",
		"file", "accounts", "no margin", "min headroom", "median", "closest account")
	for _, path := range strings.Split(*paths, ",") {
		path = strings.TrimSpace(path)
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "read:", err)
			os.Exit(1)
		}
		var snap greeks
		if err := json.Unmarshal(raw, &snap); err != nil {
			fmt.Fprintln(os.Stderr, "decode:", err)
			os.Exit(1)
		}
		ratios := make([]float64, 0, len(snap.TerminalAccounts))
		zeroMargin := 0
		minRatio, minRole := math.Inf(1), ""
		var minEquity, minMaint int64
		for _, row := range snap.TerminalAccounts {
			if row.Account.Maintenance <= 0 {
				zeroMargin++
				continue
			}
			ratio := float64(row.Account.Equity) / float64(row.Account.Maintenance)
			ratios = append(ratios, ratio)
			if ratio < minRatio {
				minRatio, minRole = ratio, row.Role
				minEquity, minMaint = row.Account.Equity, row.Account.Maintenance
			}
		}
		if len(ratios) == 0 {
			fmt.Printf("%-28s %8d %10d %12s %14s %16s\n",
				shorten(path), len(snap.TerminalAccounts), zeroMargin, "-", "-", "none at risk")
			continue
		}
		sort.Float64s(ratios)
		fmt.Printf("%-28s %8d %10d %12.1fx %13.1fx %16s\n",
			shorten(path), len(snap.TerminalAccounts), zeroMargin,
			minRatio, ratios[len(ratios)/2], minRole)
		fmt.Printf("%-28s   closest: equity %.0f vs maintenance %.0f\n",
			"", float64(minEquity)/(*precision), float64(minMaint)/(*precision))
		_ = class
	}
	fmt.Printf("\nliquidation is due at 1.0x; accounts with zero maintenance hold no position the risk engine can act on\n")
}

func shorten(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return path
	}
	return strings.Join(parts[len(parts)-2:], "/")
}
