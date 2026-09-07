// Command venueeffect asks whether an actor's venue placement, rather than its
// strategy, explains part of its result.
//
// The campaign places the same participant counts on all three venues, so every
// role class exists three times over. That gives a clean within-class design:
// hold the strategy fixed and vary only the environment. For each role class it
// reports the mean marked-equity change per venue, the spread between venues,
// and the spread between participants of the same class on the same venue.
//
// The second number is the yardstick. A between-venue spread that sits inside
// the within-venue spread is not evidence of a venue effect.
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

// roleClass strips the participant index from a role, so elastic_supplier_3
// becomes elastic_supplier. Without this every group holds one participant per
// venue, the within-venue spread is zero by construction, and the comparison
// has no yardstick — which is exactly what the first run of this tool produced.
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

type snapshot struct {
	VenueID string `json:"venue_id"`
	Role    string `json:"role"`
	Account struct {
		Equity int64 `json:"equity"`
	} `json:"account"`
	ClientID uint64 `json:"client_id"`
}

type outcome struct {
	InitialAccounts  []snapshot `json:"initial_accounts"`
	TerminalAccounts []snapshot `json:"terminal_accounts"`
}

func main() {
	path := flag.String("file", "", "greeks.json from a run")
	precision := flag.Float64("precision", 100000, "report-asset units per whole unit")
	flag.Bool("detail", false, "print each class's per-venue means")
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
	var run outcome
	if err := json.Unmarshal(raw, &run); err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}
	if len(run.TerminalAccounts) == 0 {
		fmt.Println("no terminal accounts in this artifact")
		return
	}

	start := map[uint64]int64{}
	for _, row := range run.InitialAccounts {
		start[row.ClientID] = row.Account.Equity
	}

	// role -> venue -> per-participant equity change
	byRole := map[string]map[string][]float64{}
	for _, row := range run.TerminalAccounts {
		initial, seen := start[row.ClientID]
		if !seen {
			continue
		}
		change := float64(row.Account.Equity-initial) / *precision
		class := roleClass(row.Role)
		if byRole[class] == nil {
			byRole[class] = map[string][]float64{}
		}
		byRole[class][row.VenueID] = append(byRole[class][row.VenueID], change)
	}

	roles := make([]string, 0, len(byRole))
	for role := range byRole {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	detail := flag.Lookup("detail").Value.String() == "true"
	// The campaign configures north as price_time and central/south as pro_rata,
	// with funding intervals of 8h, 1h and 2h. That yields a decomposition the
	// raw spread hides: central vs south isolates the funding interval, and
	// north vs the pro-rata mean isolates the matching rule.
	fmt.Printf("%-24s %8s %12s %12s %12s %10s\n",
		"role", "n/venue", "rule axis", "funding axis", "mean", "rule %")
	fmt.Println("  between = max-min of the per-venue means; within = mean per-venue max-min")
	type reported struct {
		role                   string
		between, within, ratio float64
		venues, perVenue       int
	}
	var rows []reported
	for _, role := range roles {
		venues := byRole[role]
		if len(venues) < 2 {
			continue
		}
		means := make([]float64, 0, len(venues))
		withinSum, withinCount, perVenue := 0.0, 0, 0
		for _, values := range venues {
			if len(values) == 0 {
				continue
			}
			perVenue = len(values)
			sum, low, high := 0.0, values[0], values[0]
			for _, v := range values {
				sum += v
				low = math.Min(low, v)
				high = math.Max(high, v)
			}
			means = append(means, sum/float64(len(values)))
			if len(values) > 1 {
				withinSum += high - low
				withinCount++
			}
		}
		if len(means) < 2 {
			continue
		}
		low, high := means[0], means[0]
		for _, m := range means {
			low = math.Min(low, m)
			high = math.Max(high, m)
		}
		between := high - low
		within := 0.0
		if withinCount > 0 {
			within = withinSum / float64(withinCount)
		}
		ratio := math.Inf(1)
		if within > 0 {
			ratio = between / within
		}
		// A class with one participant per venue has no within-venue spread and
		// therefore no yardstick; reporting a ratio against zero would be a
		// division artefact, not a measurement.
		if withinCount == 0 {
			fmt.Printf("%-28s %10d %10d %12.2f %12s %7s  (no yardstick)\n",
				role, len(venues), perVenue, between, "n/a", "n/a")
			continue
		}
		rows = append(rows, reported{role, between, within, ratio, len(venues), perVenue})
		if detail {
			names := make([]string, 0, len(venues))
			for venue := range venues {
				names = append(names, venue)
			}
			sort.Strings(names)
			for _, venue := range names {
				values := venues[venue]
				sum := 0.0
				for _, v := range values {
					sum += v
				}
				fmt.Printf("      %-24s %s mean %.2f over %d\n", role, venue, sum/float64(len(values)), len(values))
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ratio > rows[j].ratio })
	for _, r := range rows {
		venues := byRole[r.role]
		mean := func(id string) (float64, bool) {
			values := venues[id]
			if len(values) == 0 {
				return 0, false
			}
			sum := 0.0
			for _, v := range values {
				sum += v
			}
			return sum / float64(len(values)), true
		}
		north, okN := mean("north")
		central, okC := mean("central")
		south, okS := mean("south")
		if !okN || !okC || !okS {
			continue
		}
		proRata := (central + south) / 2
		ruleAxis := north - proRata
		fundingAxis := central - south
		overall := (north + central + south) / 3
		rulePct := math.Inf(1)
		if overall != 0 {
			rulePct = 100 * ruleAxis / math.Abs(overall)
		}
		fmt.Printf("%-24s %8d %12.0f %12.0f %12.0f %9.2f%%\n",
			r.role, r.perVenue, ruleAxis, fundingAxis, overall, rulePct)
	}
}
