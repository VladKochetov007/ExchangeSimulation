// Command bookshare reports which participant classes actually move each book.
//
// A convergence mechanism that is a rounding error in the order flow cannot
// converge anything, however good its signal. When a design names arbitrage as
// the force that should close a basis, the first question is not whether the
// arbitrageur is clever but whether it is large enough to matter.
//
// Volume is counted once per fill record, so each execution contributes twice
// across the two sides; shares are therefore of total participant-side volume,
// which is the right denominator for "how much of this book is this class".
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

type fillPayload struct {
	Symbol string `json:"symbol"`
	Qty    int64  `json:"qty"`
}

type record struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	Data     struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			fillPayload
			Payload *fillPayload `json:"payload"`
		} `json:"payload"`
	} `json:"data"`
}

// fill returns the fill fields from whichever level carries them: a per-book file
// writes them flat, the shared derivatives file nests them under a symbol.
func (r record) fill() fillPayload {
	if r.Data.Payload.Payload != nil {
		inner := *r.Data.Payload.Payload
		if inner.Symbol == "" {
			inner.Symbol = r.Data.Payload.Symbol
		}
		return inner
	}
	return r.Data.Payload.fillPayload
}

type account struct {
	VenueID  string `json:"venue_id"`
	ClientID uint64 `json:"client_id"`
	Role     string `json:"role"`
}

type greeks struct {
	InitialAccounts []account `json:"initial_accounts"`
}

type key struct {
	venue  string
	client uint64
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

func symbolFromPath(path string) string {
	base := filepath.Base(path)
	name := base[:len(base)-len(filepath.Ext(base))]
	return strings.ReplaceAll(name, "-", "/")
}

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	greeksPath := flag.String("greeks", "", "greeks.json (default <dir>/greeks.json)")
	basePrecision := flag.Float64("base-precision", 1e8, "base units per whole unit")
	top := flag.Int("top", 6, "classes to list per book")
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
	roleOf := map[key]string{}
	for _, row := range snap.InitialAccounts {
		roleOf[key{row.VenueID, row.ClientID}] = class(row.Role)
	}

	volume := map[string]map[string]float64{}
	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*.jsonl"))
	nested, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "*", "*.jsonl"))
	files = append(files, nested...)
	sort.Strings(files)

	marker := []byte(`"OrderFill"`)
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
			if err := json.Unmarshal(line, &rec); err != nil || rec.Event != "OrderFill" {
				continue
			}
			f := rec.fill()
			symbol := f.Symbol
			if symbol == "" {
				symbol = pathSymbol
			}
			name := roleOf[key{rec.Data.VenueID, rec.ClientID}]
			if name == "" || f.Qty == 0 {
				continue
			}
			if volume[symbol] == nil {
				volume[symbol] = map[string]float64{}
			}
			volume[symbol][name] += float64(f.Qty) / (*basePrecision)
		}
		handle.Close()
	}

	books := make([]string, 0, len(volume))
	for book := range volume {
		books = append(books, book)
	}
	sort.Slice(books, func(i, j int) bool {
		return total(volume[books[i]]) > total(volume[books[j]])
	})

	for _, book := range books {
		sum := total(volume[book])
		if sum == 0 {
			continue
		}
		names := make([]string, 0, len(volume[book]))
		for name := range volume[book] {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool { return volume[book][names[i]] > volume[book][names[j]] })
		fmt.Printf("\n%s — total participant-side volume %.1f\n", book, sum)
		for i, name := range names {
			if i >= *top {
				fmt.Printf("  %-26s %12s\n", "(others)", fmt.Sprintf("%.1f%%", remainder(volume[book], names[*top:])/sum*100))
				break
			}
			fmt.Printf("  %-26s %12.1f %7.1f%%\n", name, volume[book][name], volume[book][name]/sum*100)
		}
	}
}

func total(m map[string]float64) float64 {
	sum := 0.0
	for _, v := range m {
		sum += v
	}
	return sum
}

func remainder(m map[string]float64, names []string) float64 {
	sum := 0.0
	for _, n := range names {
		sum += m[n]
	}
	return sum
}
