// Command dealerskew tests whether an option dealer's decision to place a bid is
// gated by its inventory in that individual contract.
//
// The dealer prices bid = theo - half - skew and places it only when positive,
// where skew is proportional to its position in *that* contract. A short position
// enters with a minus sign and therefore raises the bid. Measuring the dealer's
// aggregate book cannot test this: the aggregate is short in every quarter while
// the bid appears only sometimes.
//
// The tool streams one venue's derivative evidence in timestamp order, maintains
// a per-contract inventory from the dealer's own fills, and records the side of
// every placement against the inventory held in that contract at that instant.
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

type inner struct {
	Symbol  string `json:"symbol"`
	Side    string `json:"side"`
	Qty     int64  `json:"qty"`
	NewSize int64  `json:"new_size"`
}

type record struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
	Data     struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Symbol  string `json:"symbol"`
			Payload inner  `json:"payload"`
		} `json:"payload"`
	} `json:"data"`
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

// contractKey identifies one dealer's position in one contract. Inventory is per
// dealer and per contract, which is the whole point of the measurement.
type contractKey struct {
	venue    string
	client   uint64
	contract string
}

type bucket struct{ bid, ask int }

func main() {
	logDir := flag.String("dir", "", "log directory of a full-log run")
	greeksPath := flag.String("greeks", "", "greeks.json (default <dir>/greeks.json)")
	rolePrefix := flag.String("class", "option_dealer", "participant class to attribute")
	lotUnits := flag.Float64("lot", 5e6, "base units in one inventory lot (LotQty)")
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

	files, _ := filepath.Glob(filepath.Join(*logDir, "venues", "*", "derivatives.jsonl"))
	sort.Strings(files)
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no derivatives evidence found")
		os.Exit(1)
	}

	// Bucket edges in lots of inventory. Negative is short, which raises the bid.
	edges := []float64{-100, -20, -6, -2, 0, 2, 1e9}
	labels := []string{"<= -20 lots", "-20..-6", "-6..-2", "-2..0", "0..+2", "> +2 lots"}
	buckets := make([]bucket, len(labels))
	q3 := make([]int, len(labels))
	inventory := map[contractKey]int64{}
	var firstTS, lastTS int64
	type placement struct {
		ts    int64
		idx   int
		isBid bool
	}
	var placements []placement

	fillMarker, acceptMarker := []byte(`"OrderFill"`), []byte(`"OrderAccepted"`)
	for _, path := range files {
		handle, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open:", err)
			os.Exit(1)
		}
		scanner := bufio.NewScanner(handle)
		scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
		for scanner.Scan() {
			line := scanner.Bytes()
			isFill, isAccept := bytes.Contains(line, fillMarker), bytes.Contains(line, acceptMarker)
			if !isFill && !isAccept {
				continue
			}
			var rec record
			if err := json.Unmarshal(line, &rec); err != nil {
				continue
			}
			symbol := rec.Data.Payload.Symbol
			if symbol == "" {
				symbol = rec.Data.Payload.Payload.Symbol
			}
			if !strings.Contains(symbol, "-C") && !strings.Contains(symbol, "-P") {
				continue
			}
			owner := key{rec.Data.VenueID, rec.ClientID}
			if !inClass[owner] {
				continue
			}
			if firstTS == 0 || rec.SimTS < firstTS {
				firstTS = rec.SimTS
			}
			if rec.SimTS > lastTS {
				lastTS = rec.SimTS
			}
			ck := contractKey{rec.Data.VenueID, rec.ClientID, symbol}
			switch rec.Event {
			case "OrderFill":
				// new_size is the exchange's post-fill position in this contract.
				inventory[ck] = rec.Data.Payload.Payload.NewSize
			case "OrderAccepted":
				lots := float64(inventory[ck]) / *lotUnits
				idx := len(labels) - 1
				for i := 0; i < len(edges)-1; i++ {
					if lots >= edges[i] && lots < edges[i+1] {
						idx = i
						break
					}
				}
				placements = append(placements, placement{rec.SimTS, idx, rec.Data.Payload.Payload.Side == "BUY"})
			}
		}
		handle.Close()
	}

	span := lastTS - firstTS
	if span <= 0 {
		span = 1
	}
	for _, p := range placements {
		if p.isBid {
			buckets[p.idx].bid++
		} else {
			buckets[p.idx].ask++
		}
		if int((p.ts-firstTS)*4/span) == 2 {
			q3[p.idx]++
		}
	}

	fmt.Printf("%s bid/ask by inventory IN THAT CONTRACT at the moment of the quote\n", *rolePrefix)
	fmt.Printf("(short inventory is negative and raises the bid; one lot = %.0f base units)\n\n", *lotUnits)
	fmt.Printf("%-14s %10s %10s %9s %12s\n", "inventory", "bids", "asks", "bid/ask", "share of Q3")
	totalQ3 := 0
	for _, n := range q3 {
		totalQ3 += n
	}
	for i, label := range labels {
		b := buckets[i]
		if b.bid+b.ask == 0 {
			continue
		}
		ratio := "-"
		if b.ask > 0 {
			ratio = fmt.Sprintf("%.1f%%", float64(b.bid)/float64(b.ask)*100)
		}
		share := "-"
		if totalQ3 > 0 {
			share = fmt.Sprintf("%.1f%%", float64(q3[i])/float64(totalQ3)*100)
		}
		fmt.Printf("%-14s %10d %10d %9s %12s\n", label, b.bid, b.ask, ratio, share)
	}
}
