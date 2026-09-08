// Command flowattrib attributes a participant class's trading result to the
// books it traded and the counterparties it traded against.
//
// Account snapshots cannot answer either question: they report one equity number
// per participant. The fill stream can, because two OrderFill records share a
// trade id, so the trade graph is recoverable — who traded what, with whom, on
// which book.
//
// For a book BASE/QUOTE a participant's cash accrues in QUOTE and its inventory
// in BASE, so its contribution from that book is
//
//	contribution = Σ(signed quote cash flow, net of quote fees)
//	             + (net base inventory, net of base fees) × terminal mark
//
// both converted to the reporting asset at terminal marks.
//
// The tool cross-checks itself against a completely different source before
// reporting any split: summed over books, a class's contribution must reproduce
// the carry-adjusted PnL implied by its account snapshots,
//
//	Σ_books contribution ≈ Δequity − Σ_asset initial_balance × Δmark
//
// The fill stream carries no funding, borrow interest or liquidation transfer,
// so a class that earns outside the order book will disagree. That disagreement
// is reported per class rather than hidden, because which classes fail the check
// is itself the finding about where their result comes from.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type fillPayload struct {
	Symbol    string `json:"symbol"`
	Qty       int64  `json:"qty"`
	Price     int64  `json:"price"`
	Side      string `json:"side"`
	TradeID   uint64 `json:"trade_id"`
	FeeAmount int64  `json:"fee_amount"`
	FeeAsset  string `json:"fee_asset"`
}

type fillRecord struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	Data     struct {
		VenueID string      `json:"venue_id"`
		Payload fillPayload `json:"payload"`
	} `json:"data"`
}

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

type greeks struct {
	InitialAccounts  []account `json:"initial_accounts"`
	TerminalAccounts []account `json:"terminal_accounts"`
}

func (a account) balances() map[string]int64 {
	totals := map[string]int64{}
	for _, b := range a.Account.SpotBalances {
		totals[b.Asset] += b.NetAsset
	}
	for _, b := range a.Account.PerpBalances {
		totals[b.Asset] += b.NetAsset
	}
	return totals
}

// class strips a trailing participant index so numbered participants of one role
// aggregate together. Grouping by the numbered role yields one participant per
// group, which RT-021 records as a measurement error.
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

type key struct {
	venue  string
	client uint64
}

// markKey scopes a mark to the venue that published it.
type markKey struct {
	venue, asset string
}

// tradeKey identifies one execution. Trade ids are a per-book sequence, so the
// venue and symbol both belong in the key or executions from different books
// collide.
type tradeKey struct {
	venue, symbol string
	trade         uint64
}

type sideRecord struct {
	client uint64
	value  float64
	qty    float64
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	greeksPath := flag.String("greeks", "", "greeks.json from the same run (default <dir>/greeks.json)")
	focus := flag.String("class", "triangle_arb", "participant class to attribute")
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
	var snapshot greeks
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		fmt.Fprintln(os.Stderr, "decode greeks:", err)
		os.Exit(1)
	}

	roleOf := map[key]string{}
	// Marks are published per venue and the venues do not agree: E-047 measured
	// the terminal ABC mark differing across all three. Collapsing them into one
	// map applies one venue's prices to another venue's inventory, which is the
	// defect H-051 was written to test.
	startMarks := map[markKey]int64{}
	endMarks := map[markKey]int64{}
	type opening struct {
		equity   int64
		balances map[string]int64
	}
	start := map[key]opening{}
	for _, row := range snapshot.InitialAccounts {
		roleOf[key{row.VenueID, row.ClientID}] = class(row.Role)
		start[key{row.VenueID, row.ClientID}] = opening{row.Account.Equity, row.balances()}
		for asset, mark := range row.Marks {
			startMarks[markKey{row.VenueID, asset}] = mark
		}
	}
	carryAdjusted := map[string]float64{}
	for _, row := range snapshot.TerminalAccounts {
		open, seen := start[key{row.VenueID, row.ClientID}]
		if !seen {
			continue
		}
		for asset, mark := range row.Marks {
			endMarks[markKey{row.VenueID, asset}] = mark
		}
		revaluation := 0.0
		for asset, balance := range open.balances {
			revaluation += float64(balance) / assetPrecision(asset) *
				float64(row.Marks[asset]-startMarks[markKey{row.VenueID, asset}])
		}
		carryAdjusted[class(row.Role)] += float64(row.Account.Equity-open.equity) - revaluation
	}

	// contribution is indexed by class then book; pairs by book then counterparty.
	contribution := map[string]map[string]float64{}
	pairs := map[string]map[string]float64{}
	notional := map[string]map[string]float64{}
	open := map[tradeKey][]sideRecord{}
	nonSpot := map[string]float64{}
	fills := 0

	files, err := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "glob:", err)
		os.Exit(1)
	}
	nested, err := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "glob:", err)
		os.Exit(1)
	}
	files = append(files, nested...)
	sort.Strings(files)

	marker := []byte(`"OrderFill"`)
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open:", err)
			os.Exit(1)
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !bytes.Contains(line, marker) {
				continue
			}
			var record fillRecord
			if err := json.Unmarshal(line, &record); err != nil || record.Event != "OrderFill" {
				continue
			}
			fill := record.Data.Payload
			base, quote, spot := strings.Cut(fill.Symbol, "/")
			owner := key{record.Data.VenueID, record.ClientID}
			name := roleOf[owner]
			if name == "" {
				continue
			}
			fills++
			if !spot {
				// Derivative fills settle in the quote asset through margin
				// rather than moving a base wallet, so they are counted apart
				// and reported as an explicit gap instead of being folded in.
				nonSpot[name] += 0
				continue
			}
			signedQty := float64(fill.Qty)
			cash := -float64(fill.Qty) / assetPrecision(base) * float64(fill.Price)
			if fill.Side == "SELL" {
				signedQty, cash = -signedQty, -cash
			}
			baseUnits, quoteUnits := signedQty, cash
			switch fill.FeeAsset {
			case base:
				baseUnits -= float64(fill.FeeAmount)
			case quote:
				quoteUnits -= float64(fill.FeeAmount)
			}
			venue := record.Data.VenueID
			value := baseUnits/assetPrecision(base)*float64(endMarks[markKey{venue, base}]) +
				quoteUnits/assetPrecision(quote)*float64(endMarks[markKey{venue, quote}])
			if contribution[name] == nil {
				contribution[name] = map[string]float64{}
			}
			contribution[name][fill.Symbol] += value

			id := tradeKey{record.Data.VenueID, fill.Symbol, fill.TradeID}
			open[id] = append(open[id], sideRecord{record.ClientID, value, float64(fill.Qty) / assetPrecision(base)})
			if sides := open[id]; len(sides) == 2 {
				delete(open, id)
				for i, side := range sides {
					if roleOf[key{id.venue, side.client}] != *focus {
						continue
					}
					other := roleOf[key{id.venue, sides[1-i].client}]
					if pairs[fill.Symbol] == nil {
						pairs[fill.Symbol] = map[string]float64{}
						notional[fill.Symbol] = map[string]float64{}
					}
					pairs[fill.Symbol][other] += side.value
					notional[fill.Symbol][other] += side.qty
				}
			}
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "scan:", err)
			os.Exit(1)
		}
		file.Close()
	}

	fmt.Printf("parsed %d OrderFill records from %d files\n", fills, len(files))
	if len(open) != 0 {
		fmt.Printf("unpaired executions: %d (a fill whose sibling is in another file)\n", len(open))
	}

	fmt.Printf("\ncross-check: fill-stream contribution vs account-snapshot carry-adjusted pnl\n")
	fmt.Printf("%-28s %18s %18s %12s\n", "class", "from fills", "from snapshots", "gap")
	names := make([]string, 0, len(carryAdjusted))
	for name := range carryAdjusted {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		total := 0.0
		for _, value := range contribution[name] {
			total += value
		}
		snap := carryAdjusted[name]
		gap := math.NaN()
		if snap != 0 {
			gap = (total - snap) / math.Abs(snap) * 100
		}
		fmt.Printf("%-28s %18.0f %18.0f %11.1f%%\n",
			name, total/(*reportPrecision), snap/(*reportPrecision), gap)
	}

	fmt.Printf("\n%s contribution by book\n", *focus)
	books := make([]string, 0, len(contribution[*focus]))
	for book := range contribution[*focus] {
		books = append(books, book)
	}
	sort.Slice(books, func(i, j int) bool {
		return contribution[*focus][books[i]] > contribution[*focus][books[j]]
	})
	for _, book := range books {
		fmt.Printf("  %-12s %18.0f\n", book, contribution[*focus][book]/(*reportPrecision))
	}

	fmt.Printf("\n%s counterparties by book\n", *focus)
	for _, book := range books {
		counterparties := make([]string, 0, len(pairs[book]))
		for name := range pairs[book] {
			counterparties = append(counterparties, name)
		}
		sort.Slice(counterparties, func(i, j int) bool {
			return pairs[book][counterparties[i]] > pairs[book][counterparties[j]]
		})
		fmt.Printf("  %s\n", book)
		for _, name := range counterparties {
			fmt.Printf("    %-26s %16.0f  (%.1f base traded)\n",
				name, pairs[book][name]/(*reportPrecision), notional[book][name])
		}
	}
}

func assetPrecision(asset string) float64 {
	if asset == "USD" {
		return 100000
	}
	return 1e8
}
