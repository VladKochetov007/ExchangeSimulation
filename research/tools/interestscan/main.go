// Command interestscan measures how much of the configured collateral interest
// rate a run actually delivers, from the run's own margin_interest evidence.
//
// The charge is interest = floor(borrowed * rate / denominator) per minute, so
// an observed amount A bounds the debt that produced it:
//
//	A * denominator/rate  <=  borrowed  <  (A+1) * denominator/rate
//
// with denominator/rate = 10_512_000 quote units at 500 bps. The delivered rate
// as a fraction of the configured one is therefore bounded by A/(A+1) below and
// 1 above, which is tight enough to decide severity: A = 0 means nothing was
// charged, A = 1 means at most half the intended rate, A >= 500 means within
// 0.2%.
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

type event struct {
	Event    string `json:"event"`
	ClientID uint64 `json:"client_id"`
	Data     struct {
		// The venue logger wraps every event payload, so the fields live one
		// level deeper than the emitting struct suggests. Reading them at the
		// top level yields a silent zero for every record.
		Payload struct {
			Asset  string `json:"asset"`
			Wallet string `json:"wallet"`
			Amount int64  `json:"amount"`
		} `json:"payload"`
	} `json:"data"`
}

func main() {
	dir := flag.String("dir", "", "run log directory")
	divisor := flag.Int64("divisor", 10_512_000, "denominator/rate, the smallest debt that is charged one unit")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	charges := map[key]int64{} // (asset, charged amount) -> occurrences
	perClient := map[uint64]int64{}
	borrowEvents := 0
	total := int64(0)
	files := 0

	err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer file.Close()
		files++
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1<<20), 1<<24)
		for scanner.Scan() {
			line := scanner.Bytes()
			if !strings.Contains(string(line), "margin_interest") && !strings.Contains(string(line), "\"borrow\"") {
				continue
			}
			var ev event
			if json.Unmarshal(line, &ev) != nil {
				continue
			}
			switch ev.Event {
			case "margin_interest":
				charges[key{ev.Data.Payload.Asset, ev.Data.Payload.Amount}]++
				perClient[ev.ClientID] += ev.Data.Payload.Amount
				total += ev.Data.Payload.Amount
			case "borrow":
				borrowEvents++
			}
		}
		return scanner.Err()
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}

	fmt.Printf("log files scanned: %d\n", files)
	fmt.Printf("borrow events: %d\n", borrowEvents)
	fmt.Printf("margin_interest charges: %d, total charged: %d quote units\n", countAll(charges), total)
	if len(charges) == 0 {
		fmt.Println("no interest was charged in this run")
		return
	}
	keys := make([]key, 0, len(charges))
	for k := range charges {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].asset != keys[j].asset {
			return keys[i].asset < keys[j].asset
		}
		return keys[i].amount < keys[j].amount
	})
	fmt.Println("asset, charged amount -> occurrences, implied debt band in raw units, delivered fraction of the configured rate")
	for _, k := range keys {
		low := k.amount * *divisor
		high := (k.amount + 1) * *divisor
		lower := float64(k.amount) / float64(k.amount+1)
		fmt.Printf("  %-4s %6d x%-8d debt in [%d, %d)  delivered >= %.1f%%\n",
			k.asset, k.amount, charges[k], low, high, 100*lower)
	}
	clients := make([]uint64, 0, len(perClient))
	for id := range perClient {
		clients = append(clients, id)
	}
	sort.Slice(clients, func(i, j int) bool { return perClient[clients[i]] > perClient[clients[j]] })
	fmt.Println("top charged clients:")
	for index, id := range clients {
		if index >= 10 {
			break
		}
		fmt.Printf("  client %d: %d quote units\n", id, perClient[id])
	}
}

type key struct {
	asset  string
	amount int64
}

func countAll(m map[key]int64) int64 {
	sum := int64(0)
	for _, count := range m {
		sum += count
	}
	return sum
}
