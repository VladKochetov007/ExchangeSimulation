// Command bookspread reports the quoted half-spread of each book, so that a
// taker's realised cost can be compared against what the book was actually
// charging to cross it.
//
// A taker's loss decomposes into the spread it paid and the revaluation of the
// inventory it accumulated. Those are different defects with different fixes, and
// the quoted spread is the only one of the two that the book itself controls.
//
// The evidence has two snapshot shapes and this tool handles both: a per-book
// file writes the levels at the top of the payload and names the book by its file
// path, while a shared file nests them under a symbol. Three earlier tools in
// this audit each returned a confident empty or zero result by assuming one shape.
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

type record struct {
	Event string `json:"event"`
	SimTS int64  `json:"sim_ts"`
	Data  struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Symbol  string   `json:"symbol"`
			Asks    []level  `json:"asks"`
			Bids    []level  `json:"bids"`
			Payload snapshot `json:"payload"`
		} `json:"payload"`
	} `json:"data"`
}

func symbolFromPath(path string) string {
	base := filepath.Base(path)
	name := base[:len(base)-len(filepath.Ext(base))]
	return strings.ReplaceAll(name, "-", "/")
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	flag.Parse()
	if *logDir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	// halfSpreads is keyed by book and holds relative half-spreads in bps.
	halfSpreads := map[string][]float64{}
	oneSided := map[string]int{}

	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	nested, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	files = append(files, nested...)
	sort.Strings(files)

	marker := []byte(`"BookSnapshot"`)
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
			if !bytes.Contains(line, marker) {
				continue
			}
			var rec record
			if err := json.Unmarshal(line, &rec); err != nil || rec.Event != "BookSnapshot" {
				continue
			}
			symbol, shot := rec.Data.Payload.Symbol, rec.Data.Payload.Payload
			if symbol == "" {
				symbol = pathSymbol
				shot = snapshot{Bids: rec.Data.Payload.Bids, Asks: rec.Data.Payload.Asks}
			}
			if len(shot.Bids) == 0 || len(shot.Asks) == 0 {
				oneSided[symbol]++
				continue
			}
			bid, ask := shot.Bids[0].Price, shot.Asks[0].Price
			if bid <= 0 || ask <= 0 || ask < bid {
				oneSided[symbol]++
				continue
			}
			mid := float64(bid+ask) / 2
			halfSpreads[symbol] = append(halfSpreads[symbol], float64(ask-bid)/2/mid*10000)
		}
		handle.Close()
	}

	books := make([]string, 0, len(halfSpreads))
	for book := range halfSpreads {
		books = append(books, book)
	}
	sort.Strings(books)

	fmt.Printf("%-26s %10s %10s %10s %10s %10s %12s\n",
		"book", "samples", "median", "mean", "p90", "max", "one-sided")
	for _, book := range books {
		values := halfSpreads[book]
		sort.Float64s(values)
		n := len(values)
		mean := 0.0
		for _, v := range values {
			mean += v
		}
		mean /= float64(n)
		fmt.Printf("%-26s %10d %8.2fbp %8.2fbp %8.2fbp %8.2fbp %12d\n",
			book, n, values[n/2], mean, values[n*9/10], values[n-1], oneSided[book])
	}
	fmt.Printf("\nhalf-spreads are relative to the midpoint, in basis points\n")
}
