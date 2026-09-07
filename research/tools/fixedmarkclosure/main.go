// Command fixedmarkclosure checks the population ledger exactly rather than by
// estimate.
//
// RT-024's screen could only separate revaluation from an accounting gap by
// magnitude, so a gap below roughly two percent of the endowment was invisible
// to it. This removes the estimate: it runs the simulation, then values the
// TERMINAL population a second time at the INITIAL marks. Revaluation is then
// identically zero by construction and whatever is left is the accounting.
//
//	residual = Σ (terminal equity at initial marks − initial equity)
//	           + fee revenue + insurance fund
//
// A residual of a few quote units per venue is expected: fees, funding and
// interest all truncate per item (RT-008, RT-013, RT-014). Anything larger is
// unaccounted value.
//
// It reads only the public surface the campaign itself uses — NewSim, Run,
// MarkedAccount, Venues, Participants — so it is a reader of the system, not a
// fork of it.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"exchange_sim/simulations/multivenue"
	etypes "exchange_sim/types"
)

func main() {
	configPath := flag.String("config", "", "multivenue config JSON")
	seed := flag.Int64("seed", 0, "seed override")
	duration := flag.Duration("duration", 5*time.Hour, "simulated duration")
	logDir := flag.String("logdir", "", "log directory")
	flag.Parse()
	if *configPath == "" || *logDir == "" {
		fmt.Fprintln(os.Stderr, "-config and -logdir are required")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read config:", err)
		os.Exit(1)
	}
	var cfg multivenue.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "decode config:", err)
		os.Exit(1)
	}
	cfg.LogDir = *logDir
	cfg.LogMode = "none"
	if *seed != 0 {
		cfg.Seed = *seed
	}

	sim, err := multivenue.NewSim(*duration, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "new sim:", err)
		os.Exit(1)
	}
	defer sim.Close()
	if err := sim.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}

	// The marks each venue's initial capture used. Valuing the terminal
	// population at these is what removes revaluation from the residual.
	initialSpec := map[string]etypes.AccountValuationSpec{}
	initialEquity := map[uint64]int64{}
	for _, row := range sim.InitialAccounts {
		initialEquity[row.ClientID] = row.Account.Equity
		if _, seen := initialSpec[row.VenueID]; seen {
			continue
		}
		marks := make(map[string]etypes.AssetValuationMark, len(row.Marks))
		for asset, price := range row.Marks {
			precision := int64(100_000)
			if asset != "USD" {
				precision = 100_000_000
			}
			marks[asset] = etypes.AssetValuationMark{Price: price, Precision: precision}
		}
		initialSpec[row.VenueID] = etypes.AccountValuationSpec{
			ReportAsset: "USD", ReportPrecision: 100_000, AssetMarks: marks,
		}
	}

	ledgers := map[string]int64{}
	for _, l := range sim.CaptureVenueLedgers() {
		ledgers[l.VenueID] = l.FeeRevenue["USD"] + l.InsuranceFund["USD"]
	}

	type row struct {
		venue                      string
		participants               int
		changeAtInitialMarks, take int64
	}
	var rows []row
	for _, venue := range sim.Venues {
		spec, ok := initialSpec[venue.ID]
		if !ok {
			continue
		}
		var change int64
		counted := 0
		for _, participant := range venue.Participants {
			initial, seen := initialEquity[participant.ClientID]
			if !seen {
				continue
			}
			account, err := venue.Exchange.MarkedAccount(participant.ClientID, spec)
			if err != nil {
				fmt.Fprintf(os.Stderr, "marked account %s/%d: %v\n", venue.ID, participant.ClientID, err)
				os.Exit(1)
			}
			change += account.Equity - initial
			counted++
		}
		rows = append(rows, row{venue.ID, counted, change, ledgers[venue.ID]})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].venue < rows[j].venue })

	fmt.Printf("%-10s %8s %22s %14s %16s\n",
		"venue", "heads", "change at initial marks", "venue take", "residual")
	total := int64(0)
	for _, r := range rows {
		residual := r.changeAtInitialMarks + r.take
		total += residual
		fmt.Printf("%-10s %8d %22d %14d %16d\n",
			r.venue, r.participants, r.changeAtInitialMarks, r.take, residual)
	}
	fmt.Printf("%-10s %8s %22s %14s %16d  (quote units)\n", "TOTAL", "", "", "", total)
}
