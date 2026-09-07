// Command booksidedness measures how often each venue's book for a symbol is
// one-sided, which is the condition under which that venue stops contributing
// to the index consensus while its previous observation keeps voting (RT-021).
//
// It reads the periodic BookSnapshot events the venues already emit, so it
// measures a SAMPLED silence rate: silence that begins and ends between two
// snapshots is invisible to it.
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

type snapshot struct {
	Event string `json:"event"`
	SimTS int64  `json:"sim_ts"`
	Data  struct {
		VenueID string `json:"venue_id"`
		Payload struct {
			Bids []struct{} `json:"bids"`
			Asks []struct{} `json:"asks"`
		} `json:"payload"`
	} `json:"data"`
}

type counter struct{ total, oneSided int64 }

func main() {
	dir := flag.String("dir", "", "run log directory")
	symbol := flag.String("symbol", "ABC-USD", "spot log basename, e.g. ABC-USD")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	perVenue := map[string]*counter{}
	// silentAt records, per timestamp, how many venues were one-sided, so the
	// simultaneous case — the one where a live market is outvoted — can be
	// counted rather than inferred from the per-venue rates.
	silentAt := map[int64]int{}
	venuesAt := map[int64]int{}

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
			if !strings.Contains(string(line), "\"BookSnapshot\"") {
				continue
			}
			var ev snapshot
			if json.Unmarshal(line, &ev) != nil || ev.Event != "BookSnapshot" {
				continue
			}
			venue := ev.Data.VenueID
			if perVenue[venue] == nil {
				perVenue[venue] = &counter{}
			}
			perVenue[venue].total++
			venuesAt[ev.SimTS]++
			if len(ev.Data.Payload.Bids) == 0 || len(ev.Data.Payload.Asks) == 0 {
				perVenue[venue].oneSided++
				silentAt[ev.SimTS]++
			}
		}
		return scanner.Err()
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}

	venues := make([]string, 0, len(perVenue))
	for venue := range perVenue {
		venues = append(venues, venue)
	}
	sort.Strings(venues)
	fmt.Printf("%s book snapshots\n", *symbol)
	for _, venue := range venues {
		c := perVenue[venue]
		fmt.Printf("  %-8s %8d snapshots, %8d one-sided (%.2f%%)\n",
			venue, c.total, c.oneSided, 100*float64(c.oneSided)/float64(c.total))
	}

	// How often were enough venues silent at once for stale votes to decide the
	// median? With three venues that needs two silent.
	var instants, twoPlus, allSilent int64
	for ts, n := range venuesAt {
		if n == 0 {
			continue
		}
		instants++
		switch s := silentAt[ts]; {
		case s >= n:
			allSilent++
			twoPlus++
		case s >= 2:
			twoPlus++
		}
	}
	if instants == 0 {
		fmt.Println("no snapshot instants found")
		return
	}
	fmt.Printf("instants with any snapshot: %d\n", instants)
	fmt.Printf("  two or more venues one-sided at once: %d (%.2f%%)\n",
		twoPlus, 100*float64(twoPlus)/float64(instants))
	fmt.Printf("  every reporting venue one-sided at once: %d (%.2f%%)\n",
		allSilent, 100*float64(allSilent)/float64(instants))
}
