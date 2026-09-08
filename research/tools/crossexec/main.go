// Command crossexec measures what a participant class actually paid, relative to
// fair value, on a cross book at the moment each trade happened.
//
// A taker's result decomposes into spread, inventory revaluation, and execution
// away from fair value. When the first two are excluded, the third is what is
// left, and it can only be measured per fill: comparing against an end-of-run
// rate would confound the dislocation with the drift.
//
// Fair value is rebuilt the way the venue's own index does it — the median across
// venues of the ABC/USD midpoint divided by the median of the CDF/USD midpoint —
// so the benchmark is the consensus the simulation itself publishes, not an
// outside opinion.
//
// The implied loss it reports is computed from fill records and snapshot mids.
// The measured loss it is compared against comes from a separate tool's
// contribution calculation. The two share no arithmetic beyond reading the same
// evidence, so agreement between them is a cross-check rather than a restatement.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type level struct {
	Price int64 `json:"price"`
}

type snapshot struct {
	Bids []level `json:"bids"`
	Asks []level `json:"asks"`
}

func (s snapshot) mid() (int64, bool) {
	if len(s.Bids) == 0 || len(s.Asks) == 0 {
		return 0, false
	}
	bid, ask := s.Bids[0].Price, s.Asks[0].Price
	if bid <= 0 || ask <= 0 || ask < bid {
		return 0, false
	}
	return (bid + ask) / 2, true
}

type fillPayload struct {
	Symbol string `json:"symbol"`
	Qty    int64  `json:"qty"`
	Price  int64  `json:"price"`
	Side   string `json:"side"`
}

type record struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
	Data     struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Symbol  string  `json:"symbol"`
			Asks    []level `json:"asks"`
			Bids    []level `json:"bids"`
			Qty     int64   `json:"qty"`
			Price   int64   `json:"price"`
			Side    string  `json:"side"`
			Payload struct {
				snapshot
				fillPayload
			} `json:"payload"`
		} `json:"payload"`
	} `json:"data"`
}

type account struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
}

type greeks struct {
	InitialAccounts  []account `json:"initial_accounts"`
	TerminalAccounts []account `json:"terminal_accounts"`
}

type key struct {
	venue  string
	client uint64
}

func symbolFromPath(path string) string {
	base := filepath.Base(path)
	name := base[:len(base)-len(filepath.Ext(base))]
	return strings.ReplaceAll(name, "-", "/")
}

func median(values []int64) int64 {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	n := len(values)
	if n%2 == 1 {
		return values[n/2]
	}
	return (values[n/2-1] + values[n/2]) / 2
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	greeksPath := flag.String("greeks", "", "greeks.json (default <dir>/greeks.json)")
	crossSymbol := flag.String("cross", "ABC/CDF", "cross book to measure")
	baseSpot := flag.String("base-spot", "ABC/USD", "base asset's own quote book")
	quoteSpot := flag.String("quote-spot", "CDF/USD", "quote asset's own quote book")
	rolePrefix := flag.String("class", "noise_flow", "participant class to measure")
	basePrecision := flag.Float64("base-precision", 1e8, "base units per whole unit")
	quotePrecision := flag.Float64("quote-precision", 1e8, "quote-asset units per whole unit")
	reportPrecision := flag.Float64("precision", 100000, "report-asset units per whole unit")
	flag.Parse()
	if *logDir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}
	if *greeksPath == "" {
		*greeksPath = filepath.Join(*logDir, "greeks.json")
	}

	raw, err := os.ReadFile(*greeksPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read greeks:", err)
		os.Exit(1)
	}
	var snap greeks
	if err := json.Unmarshal(raw, &snap); err != nil {
		fmt.Fprintln(os.Stderr, "decode greeks:", err)
		os.Exit(1)
	}
	inClass := map[key]bool{}
	for _, row := range snap.InitialAccounts {
		if strings.HasPrefix(row.Role, *rolePrefix) {
			inClass[key{row.VenueID, row.ClientID}] = true
		}
	}

	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	nested, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	files = append(files, nested...)
	sort.Strings(files)

	// Pass 1: consensus mid per timestamp for the two single-asset books.
	baseMids := map[int64]map[string]int64{}
	quoteMids := map[int64]map[string]int64{}
	snapMarker := []byte(`"BookSnapshot"`)
	for _, path := range files {
		handle, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open:", err)
			os.Exit(1)
		}
		pathSymbol := symbolFromPath(path)
		scanner := bufio.NewScanner(handle)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.Contains(line, snapMarker) {
				continue
			}
			var rec record
			if err := json.Unmarshal(line, &rec); err != nil || rec.Event != "BookSnapshot" {
				continue
			}
			symbol := rec.Data.Payload.Symbol
			shot := rec.Data.Payload.Payload.snapshot
			if symbol == "" {
				symbol = pathSymbol
				shot = snapshot{Bids: rec.Data.Payload.Bids, Asks: rec.Data.Payload.Asks}
			}
			if symbol != *baseSpot && symbol != *quoteSpot {
				continue
			}
			mid, ok := shot.mid()
			if !ok {
				continue
			}
			target := baseMids
			if symbol == *quoteSpot {
				target = quoteMids
			}
			if target[rec.SimTS] == nil {
				target[rec.SimTS] = map[string]int64{}
			}
			target[rec.SimTS][rec.Data.VenueID] = mid
		}
		handle.Close()
	}

	consensus := func(byTS map[int64]map[string]int64) map[int64]int64 {
		out := make(map[int64]int64, len(byTS))
		for ts, byVenue := range byTS {
			values := make([]int64, 0, len(byVenue))
			for _, mid := range byVenue {
				values = append(values, mid)
			}
			out[ts] = median(values)
		}
		return out
	}
	baseIndex, quoteIndex := consensus(baseMids), consensus(quoteMids)
	stamps := make([]int64, 0, len(baseIndex))
	for ts := range baseIndex {
		if _, ok := quoteIndex[ts]; ok {
			stamps = append(stamps, ts)
		}
	}
	sort.Slice(stamps, func(i, j int) bool { return stamps[i] < stamps[j] })
	if len(stamps) == 0 {
		fmt.Fprintln(os.Stderr, "no timestamp has both single-asset books two-sided")
		os.Exit(1)
	}

	// fairAt returns the consensus cross rate at or before ts, in quote units per
	// whole base unit, matching the cross book's own price convention.
	fairAt := func(ts int64) (float64, bool) {
		i := sort.Search(len(stamps), func(i int) bool { return stamps[i] > ts })
		if i == 0 {
			return 0, false
		}
		at := stamps[i-1]
		base, quote := baseIndex[at], quoteIndex[at]
		if base <= 0 || quote <= 0 {
			return 0, false
		}
		// base is USD per whole ABC in USD precision; quote is USD per whole CDF.
		// Their ratio is CDF per whole ABC, scaled into the quote asset's
		// precision so it is comparable with the cross book's price.
		return float64(base) / float64(quote) * (*quotePrecision), true
	}

	// Pass 2: walk cross-book fills for the class.
	var impliedLoss, buyQty, sellQty, buyNotional, sellNotional, fairBuy, fairSell float64
	fills, skipped := 0, 0
	fillMarker := []byte(`"OrderFill"`)
	for _, path := range files {
		handle, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open:", err)
			os.Exit(1)
		}
		pathSymbol := symbolFromPath(path)
		scanner := bufio.NewScanner(handle)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.Contains(line, fillMarker) {
				continue
			}
			var rec record
			if err := json.Unmarshal(line, &rec); err != nil || rec.Event != "OrderFill" {
				continue
			}
			symbol := rec.Data.Payload.Symbol
			qty, price, side := rec.Data.Payload.Qty, rec.Data.Payload.Price, rec.Data.Payload.Side
			if qty == 0 {
				inner := rec.Data.Payload.Payload.fillPayload
				qty, price, side = inner.Qty, inner.Price, inner.Side
				if inner.Symbol != "" {
					symbol = inner.Symbol
				}
			}
			if symbol == "" {
				symbol = pathSymbol
			}
			if symbol != *crossSymbol || qty == 0 {
				continue
			}
			if !inClass[key{rec.Data.VenueID, rec.ClientID}] {
				continue
			}
			fair, ok := fairAt(rec.SimTS)
			if !ok {
				skipped++
				continue
			}
			fills++
			whole := float64(qty) / (*basePrecision)
			quoteUnits := float64(quoteIndex[stamps[sort.Search(len(stamps), func(i int) bool { return stamps[i] > rec.SimTS })-1]])
			usdPerQuote := quoteUnits / (*reportPrecision)
			// A buy above fair overpays; a sell below fair under-receives. Both
			// are losses, expressed in the quote asset then valued in USD.
			diff := (float64(price) - fair) / (*quotePrecision) * whole
			if side == "SELL" {
				diff = -diff
				sellQty += whole
				sellNotional += float64(price) / (*quotePrecision) * whole
				fairSell += fair / (*quotePrecision) * whole
			} else {
				buyQty += whole
				buyNotional += float64(price) / (*quotePrecision) * whole
				fairBuy += fair / (*quotePrecision) * whole
			}
			impliedLoss += diff * usdPerQuote
		}
		handle.Close()
	}

	fmt.Printf("%s on %s, fair rate = median(%s) / median(%s)\n",
		*rolePrefix, *crossSymbol, *baseSpot, *quoteSpot)
	fmt.Printf("  fills matched %d, skipped for missing fair rate %d\n", fills, skipped)
	if buyQty > 0 {
		fmt.Printf("  bought %.2f base at VWAP %.4f vs fair %.4f (%+.2f%%)\n",
			buyQty, buyNotional/buyQty, fairBuy/buyQty,
			(buyNotional/buyQty)/(fairBuy/buyQty)*100-100)
	}
	if sellQty > 0 {
		fmt.Printf("  sold   %.2f base at VWAP %.4f vs fair %.4f (%+.2f%%)\n",
			sellQty, sellNotional/sellQty, fairSell/sellQty,
			(sellNotional/sellQty)/(fairSell/sellQty)*100-100)
	}
	fmt.Printf("  net base flow %.2f\n", buyQty-sellQty)
	fmt.Printf("\nimplied loss from execution away from fair: %+.0f USD\n", -impliedLoss)
}
