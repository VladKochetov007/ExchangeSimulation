// Command pricerange reports how far a symbol's traded price wanders from a
// reference, which is what decides whether a static collateral oracle is a
// modelling choice or a live mispricing.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type event struct {
	Event string `json:"event"`
	Data  struct {
		Payload struct {
			Price int64 `json:"price"`
		} `json:"payload"`
	} `json:"data"`
}

func main() {
	dir := flag.String("dir", "", "run log directory")
	// The Trade payload carries no symbol: the book it belongs to is the file
	// it is written in. Selecting by path is therefore the only correct filter.
	symbol := flag.String("symbol", "ABC-USD", "spot log basename, e.g. ABC-USD")
	reference := flag.Int64("reference", 50_000*100_000, "the collateral oracle's fixed price")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	var low, high, last int64
	count := 0
	err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, *symbol+".jsonl") {
			return nil
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1<<20), 1<<24)
		for scanner.Scan() {
			line := scanner.Bytes()
			// The event name is "Trade", capitalised. Searching for the
			// lowercase form finds nothing and reports a confident empty result.
			if !strings.Contains(string(line), "\"Trade\"") {
				continue
			}
			var ev event
			if json.Unmarshal(line, &ev) != nil || ev.Event != "Trade" {
				continue
			}
			if ev.Data.Payload.Price <= 0 {
				continue
			}
			price := ev.Data.Payload.Price
			if count == 0 || price < low {
				low = price
			}
			if count == 0 || price > high {
				high = price
			}
			last = price
			count++
		}
		return scanner.Err()
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}
	if count == 0 {
		fmt.Printf("no trades found for %s\n", *symbol)
		return
	}
	pct := func(p int64) float64 { return 100 * float64(p-*reference) / float64(*reference) }
	fmt.Printf("%s trades: %d\n", *symbol, count)
	fmt.Printf("  oracle reference: %d\n", *reference)
	fmt.Printf("  low  %d (%+.2f%% vs oracle)\n", low, pct(low))
	fmt.Printf("  high %d (%+.2f%% vs oracle)\n", high, pct(high))
	fmt.Printf("  last %d (%+.2f%% vs oracle)\n", last, pct(last))
	fmt.Printf("  widest excursion: %.2f%%\n", maxAbs(pct(low), pct(high)))
}

func maxAbs(a, b float64) float64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a > b {
		return a
	}
	return b
}
