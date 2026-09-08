// Command elasticpeg tests whether a book's terminal price is the price-elastic
// participants' configuration read back out rather than a market outcome.
//
// An ElasticSupplier with a fixed reference targets
//
//	position = −(percent above reference) × ElasticityPerPercent
//
// so at rest the price satisfies
//
//	percent above reference = −aggregate position / aggregate elasticity
//
// The tool reports, per venue, the aggregate terminal position those
// participants actually hold, the position their venue's terminal price
// predicts, and the ratio. A ratio near one means the level is pinned by the
// participants' configured reference. A ratio below one is consistent with
// rate-limited convergence, because the gap is closed at RebalanceLot per tick;
// only an overshoot or a sign disagreement contradicts the peg reading.
//
// It reports the position from wallet balances rather than the actor's own
// counter, so the number does not depend on the actor being correct.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
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
	} `json:"account"`
}

type run struct {
	InitialAccounts  []account `json:"initial_accounts"`
	TerminalAccounts []account `json:"terminal_accounts"`
}

func (a account) net(asset string) int64 {
	for _, balance := range a.Account.SpotBalances {
		if balance.Asset == asset {
			return balance.NetAsset
		}
	}
	return 0
}

// key identifies one participant. Client ids are allocated per venue, so the
// venue has to be part of the key or the three venues' rows collide.
type key struct {
	venue  string
	client uint64
}

func main() {
	path := flag.String("file", "", "greeks.json from a run")
	rolePrefix := flag.String("role-prefix", "elastic_supplier", "participant class to aggregate")
	asset := flag.String("asset", "ABC", "base asset the participants supply")
	basePrecision := flag.Float64("base-precision", 1e8, "base-asset units per whole unit")
	reference := flag.Float64("reference", 5e9, "the participants' configured reference price")
	elasticity := flag.Float64("elasticity-per-percent", 15e9, "one participant's base units per percent")
	maxPosition := flag.Float64("max-position", 1e12, "one participant's position cap in base units")
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

	start := map[key]int64{}
	for _, row := range data.InitialAccounts {
		if !strings.HasPrefix(row.Role, *rolePrefix) {
			continue
		}
		start[key{row.VenueID, row.ClientID}] = row.net(*asset)
	}

	type venueTotals struct {
		position int64
		heads    int
		capped   int
		mark     int64
	}
	byVenue := map[string]*venueTotals{}
	for _, row := range data.TerminalAccounts {
		if !strings.HasPrefix(row.Role, *rolePrefix) {
			continue
		}
		initial, seen := start[key{row.VenueID, row.ClientID}]
		if !seen {
			continue
		}
		if byVenue[row.VenueID] == nil {
			byVenue[row.VenueID] = &venueTotals{}
		}
		totals := byVenue[row.VenueID]
		position := row.net(*asset) - initial
		totals.position += position
		totals.heads++
		if float64(position) >= *maxPosition || float64(position) <= -*maxPosition {
			totals.capped++
		}
		if mark, ok := row.Marks[*asset]; ok {
			totals.mark = mark
		}
	}

	venues := make([]string, 0, len(byVenue))
	for venue := range byVenue {
		venues = append(venues, venue)
	}
	sort.Strings(venues)

	fmt.Printf("class %q, asset %s, reference %.0f\n", *rolePrefix, *asset, *reference)
	for _, venue := range venues {
		totals := byVenue[venue]
		percent := (float64(totals.mark)/(*reference) - 1) * 100
		predicted := -percent * *elasticity * float64(totals.heads)
		fmt.Printf("\n%s: %d participants, terminal mark %d (%+.4f%% vs reference)\n",
			venue, totals.heads, totals.mark, percent)
		fmt.Printf("  aggregate position  %+.2f %s\n", float64(totals.position)/(*basePrecision), *asset)
		fmt.Printf("  predicted by price  %+.2f %s\n", predicted/(*basePrecision), *asset)
		if predicted != 0 {
			fmt.Printf("  ratio actual/predicted %.4f\n", float64(totals.position)/predicted)
		}
		fmt.Printf("  at position cap: %d of %d\n", totals.capped, totals.heads)
	}
}
