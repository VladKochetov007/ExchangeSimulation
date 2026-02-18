package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	assetPrecision = 100_000_000 // 1e8
	usdPrecision   = 100_000     // 1e5
	simEpochNs     = int64(1704099600_000_000_000)
	bootstrapEndNs = simEpochNs + int64(120_000_000_000)
)

type event struct {
	Event     string  `json:"event"`
	SimTime   int64   `json:"sim_time"`
	ClientID  int     `json:"client_id"`
	Type      string  `json:"type"` // LIMIT | MARKET
	Price     float64 `json:"price"`
	Qty       float64 `json:"qty"`
	FilledQty float64 `json:"filled_qty"`
	TotalQty  float64 `json:"total_qty"`
}

type symbolReport struct {
	Symbol       string  `json:"symbol"`
	Type         string  `json:"type"` // spot_usd | spot_abc | perp
	Trades       int     `json:"trades"`
	VolBase      float64 `json:"vol_base"`
	VolQuote     float64 `json:"vol_quote"`
	Limits       int     `json:"limits"`
	Markets      int     `json:"markets"`
	Cancels      int     `json:"cancels"`
	Rejects      int     `json:"rejects"`
	Fills        int     `json:"fills"`
	FirstLPExitNs int64  `json:"first_lp_exit_ns"` // 0 if not observed
	FirstLPExitSec float64 `json:"first_lp_exit_sim_sec"`
}

type clientReport struct {
	ClientID int     `json:"client_id"`
	Limits   int     `json:"limits"`
	Markets  int     `json:"markets"`
	Cancels  int     `json:"cancels"`
	Rejects  int     `json:"rejects"`
	Fills    int     `json:"fills"`
	VolBase  float64 `json:"vol_base"`
}

type report struct {
	LogDir         string                   `json:"log_dir"`
	SimEpochNs     int64                    `json:"sim_epoch_ns"`
	BootstrapEndNs int64                    `json:"bootstrap_end_ns"`
	Symbols        map[string]*symbolReport `json:"symbols"`
	Clients        map[int]*clientReport    `json:"clients"`
}

func symbolType(sym string) string {
	if strings.HasSuffix(sym, "-PERP") {
		return "perp"
	}
	if strings.HasSuffix(sym, "/USD") {
		return "spot_usd"
	}
	return "spot_abc"
}

func quotePrecision(symType string) float64 {
	if symType == "spot_abc" {
		return assetPrecision
	}
	return usdPrecision
}

func filenameToSymbol(dir, base string) string {
	name := strings.TrimSuffix(base, ".log")
	if filepath.Base(dir) == "perp" {
		return name // e.g. ABC-PERP
	}
	// spot: replace last '-' with '/'
	idx := strings.LastIndex(name, "-")
	if idx < 0 {
		return name
	}
	return name[:idx] + "/" + name[idx+1:]
}

func processFile(path string, rep *report) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	sym := filenameToSymbol(dir, base)

	symRep, ok := rep.Symbols[sym]
	if !ok {
		symRep = &symbolReport{Symbol: sym, Type: symbolType(sym)}
		rep.Symbols[sym] = symRep
	}
	qprec := quotePrecision(symRep.Type)

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open %s: %v\n", path, err)
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}

		mainPhase := ev.SimTime >= bootstrapEndNs

		switch ev.Event {
		case "Trade":
			symRep.Trades++
			base := ev.Qty / assetPrecision
			quote := (ev.Qty * ev.Price) / (assetPrecision * qprec)
			symRep.VolBase += base
			symRep.VolQuote += quote

		case "OrderAccepted":
			symRep.Limits += boolInt(ev.Type == "LIMIT")
			symRep.Markets += boolInt(ev.Type == "MARKET")
			if mainPhase {
				cr := ensureClient(rep, ev.ClientID)
				cr.Limits += boolInt(ev.Type == "LIMIT")
				cr.Markets += boolInt(ev.Type == "MARKET")
			}

		case "OrderFill":
			symRep.Fills++
			if mainPhase {
				cr := ensureClient(rep, ev.ClientID)
				cr.Fills++
				cr.VolBase += ev.FilledQty / assetPrecision
			}

		case "OrderCancelled":
			symRep.Cancels++
			if mainPhase {
				ensureClient(rep, ev.ClientID).Cancels++
			}

		case "OrderRejected":
			symRep.Rejects++
			if mainPhase {
				ensureClient(rep, ev.ClientID).Rejects++
			}

		case "BookDelta":
			if mainPhase && ev.TotalQty == 0 && symRep.FirstLPExitNs == 0 {
				symRep.FirstLPExitNs = ev.SimTime
				symRep.FirstLPExitSec = float64(ev.SimTime-simEpochNs) / 1e9
			}
		}
	}
}

func ensureClient(rep *report, id int) *clientReport {
	cr, ok := rep.Clients[id]
	if !ok {
		cr = &clientReport{ClientID: id}
		rep.Clients[id] = cr
	}
	return cr
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func main() {
	logDir := flag.String("dir", "logs/microstructure_v1", "log directory")
	outFile := flag.String("out", "", "JSON output file (default: <dir>/liquidity_report.json)")
	flag.Parse()

	if *outFile == "" {
		*outFile = filepath.Join(*logDir, "liquidity_report.json")
	}

	rep := &report{
		LogDir:         *logDir,
		SimEpochNs:     simEpochNs,
		BootstrapEndNs: bootstrapEndNs,
		Symbols:        make(map[string]*symbolReport),
		Clients:        make(map[int]*clientReport),
	}

	for _, sub := range []string{"spot", "perp"} {
		pattern := filepath.Join(*logDir, sub, "*.log")
		files, _ := filepath.Glob(pattern)
		for _, f := range files {
			processFile(f, rep)
		}
	}

	printReport(rep)

	data, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(*outFile, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *outFile, err)
		os.Exit(1)
	}
	fmt.Printf("\nJSON written to %s\n", *outFile)
}

func printReport(rep *report) {
	syms := make([]*symbolReport, 0, len(rep.Symbols))
	for _, s := range rep.Symbols {
		syms = append(syms, s)
	}
	sort.Slice(syms, func(i, j int) bool { return syms[i].Symbol < syms[j].Symbol })

	fmt.Println("=== Liquidity Report ===")
	fmt.Printf("Bootstrap end: sim+%.0fs\n\n", float64(bootstrapEndNs-simEpochNs)/1e9)

	fmt.Printf("%-14s %-8s %6s %12s %12s %7s %7s %7s %7s %7s %12s\n",
		"Symbol", "Type", "Trades", "VolBase", "VolQuote", "Limits", "Mkts", "Cancels", "Rejects", "Fills", "FirstLPExit(s)")
	fmt.Println(strings.Repeat("-", 110))

	noTrades := []string{}
	for _, s := range syms {
		exit := "—"
		if s.FirstLPExitNs > 0 {
			exit = fmt.Sprintf("%.1f", s.FirstLPExitSec)
		}
		if s.Trades == 0 {
			noTrades = append(noTrades, s.Symbol)
		}
		fmt.Printf("%-14s %-8s %6d %12.2f %12.2f %7d %7d %7d %7d %7d %12s\n",
			s.Symbol, s.Type, s.Trades,
			s.VolBase, s.VolQuote,
			s.Limits, s.Markets, s.Cancels, s.Rejects, s.Fills,
			exit)
	}

	if len(noTrades) > 0 {
		fmt.Printf("\n[WARN] Symbols with 0 trades: %s\n", strings.Join(noTrades, ", "))
	} else {
		fmt.Println("\n[OK] All symbols traded")
	}

	clients := make([]*clientReport, 0, len(rep.Clients))
	for _, c := range rep.Clients {
		clients = append(clients, c)
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].ClientID < clients[j].ClientID })

	fmt.Println("\n=== Per-Client Stats (main phase) ===")
	fmt.Printf("%-8s %7s %7s %7s %7s %7s %12s\n",
		"ClientID", "Limits", "Mkts", "Cancels", "Rejects", "Fills", "VolBase")
	fmt.Println(strings.Repeat("-", 65))

	noFills := []int{}
	for _, c := range clients {
		if c.Fills == 0 && c.Limits+c.Markets > 0 {
			noFills = append(noFills, c.ClientID)
		}
		fmt.Printf("%-8d %7d %7d %7d %7d %7d %12.2f\n",
			c.ClientID, c.Limits, c.Markets, c.Cancels, c.Rejects, c.Fills, c.VolBase)
	}

	if len(noFills) > 0 {
		fmt.Printf("\n[WARN] Clients with orders but 0 fills: %v\n", noFills)
	}

	var totTrades, totLimits, totMkts, totCancels, totRejects, totFills int
	var totVolBase, totVolQuote float64
	for _, s := range syms {
		totTrades += s.Trades
		totLimits += s.Limits
		totMkts += s.Markets
		totCancels += s.Cancels
		totRejects += s.Rejects
		totFills += s.Fills
		totVolBase += s.VolBase
		totVolQuote += s.VolQuote
	}
	fmt.Printf("\n=== Totals ===\n")
	fmt.Printf("Trades: %d  Limits: %d  Markets: %d  Cancels: %d  Rejects: %d  Fills: %d\n",
		totTrades, totLimits, totMkts, totCancels, totRejects, totFills)
	fmt.Printf("VolBase: %.2f  VolQuote: %.2f\n", totVolBase, totVolQuote)

	fmt.Println("\n=== LP First Exit After Bootstrap ===")
	exited := 0
	for _, s := range syms {
		if s.FirstLPExitNs > 0 {
			exited++
			fmt.Printf("  %-14s sim+%.1fs\n", s.Symbol, s.FirstLPExitSec)
		}
	}
	if exited == 0 {
		fmt.Println("  No LP exits observed (book never hit 0 after bootstrap)")
	}
}
