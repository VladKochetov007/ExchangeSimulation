// Command actorliveness asks whether every configured actor actually acts.
//
// A fair battle of actors requires that each configured actor fights. An actor
// whose quoting loop stalls still appears in the population, still holds its
// endowment, and still contributes a row to every class-level average — reading
// as "this strategy performs near zero" when the truth is "this strategy stopped
// playing".
//
// It counts OrderAccepted per client from the venue logs, splits the run at its
// midpoint, and maps client ids to role classes using the population artifact.
// A class with orders in the first half and none in the second has stalled; a
// class with few orders throughout is quiet by design and is reported as such
// rather than flagged.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type event struct {
	ClientID uint64 `json:"client_id"`
	Event    string `json:"event"`
	SimTS    int64  `json:"sim_ts"`
	Data     struct {
		VenueID string `json:"venue_id"`
	} `json:"data"`
}

type population struct {
	InitialAccounts []struct {
		VenueID  string `json:"venue_id"`
		ClientID uint64 `json:"client_id"`
		Role     string `json:"role"`
	} `json:"initial_accounts"`
}

func roleClass(role string) string {
	cut := strings.LastIndex(role, "_")
	if cut < 0 {
		return role
	}
	if _, err := strconv.Atoi(role[cut+1:]); err != nil {
		return role
	}
	return role[:cut]
}

type counts struct{ first, second int }

func main() {
	dir := flag.String("dir", "", "run log directory")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "-dir is required")
		os.Exit(2)
	}

	raw, err := os.ReadFile(filepath.Join(*dir, "greeks.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read greeks.json:", err)
		os.Exit(1)
	}
	var pop population
	if err := json.Unmarshal(raw, &pop); err != nil {
		fmt.Fprintln(os.Stderr, "decode greeks.json:", err)
		os.Exit(1)
	}
	// Client ids are allocated per venue, so the key must carry the venue.
	class := map[string]string{}
	for _, row := range pop.InitialAccounts {
		class[row.VenueID+"/"+fmt.Sprint(row.ClientID)] = roleClass(row.Role)
	}

	var low, high int64
	type stamped struct {
		key string
		ts  int64
	}
	var events []stamped
	err = filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
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
			if !strings.Contains(string(line), "\"OrderAccepted\"") {
				continue
			}
			var ev event
			if json.Unmarshal(line, &ev) != nil || ev.Event != "OrderAccepted" {
				continue
			}
			key := ev.Data.VenueID + "/" + fmt.Sprint(ev.ClientID)
			if _, known := class[key]; !known {
				continue
			}
			if low == 0 || ev.SimTS < low {
				low = ev.SimTS
			}
			if ev.SimTS > high {
				high = ev.SimTS
			}
			events = append(events, stamped{key, ev.SimTS})
		}
		return scanner.Err()
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "walk:", err)
		os.Exit(1)
	}
	if len(events) == 0 {
		fmt.Println("no OrderAccepted events found")
		return
	}

	mid := low + (high-low)/2
	byClass := map[string]*counts{}
	for _, e := range events {
		name := class[e.key]
		if byClass[name] == nil {
			byClass[name] = &counts{}
		}
		if e.ts <= mid {
			byClass[name].first++
		} else {
			byClass[name].second++
		}
	}
	// A configured class with no events at all never reaches the loop above.
	for _, name := range class {
		if byClass[name] == nil {
			byClass[name] = &counts{}
		}
	}

	names := make([]string, 0, len(byClass))
	for name := range byClass {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return byClass[names[i]].first+byClass[names[i]].second <
			byClass[names[j]].first+byClass[names[j]].second
	})
	// A zero-test is not a liveness test: a class that falls by 99.7% is not
	// "active" in any useful sense, and the first version of this tool called
	// it that. Report the ratio and let the reader see the decline.
	fmt.Printf("%-28s %12s %12s %9s  %s\n", "class", "first half", "second half", "ratio", "verdict")
	for _, name := range names {
		c := byClass[name]
		verdict := "active"
		ratio := "n/a"
		switch {
		case c.first == 0 && c.second == 0:
			verdict = "INERT - never placed an order"
		case c.first == 0:
			verdict = "late start"
		default:
			r := float64(c.second) / float64(c.first)
			ratio = fmt.Sprintf("%.3f", r)
			switch {
			case c.second == 0:
				verdict = "STALLED - stopped at the midpoint"
			case r < 0.1:
				verdict = "COLLAPSED - second half under a tenth of the first"
			case r < 0.5:
				verdict = "declining"
			}
		}
		fmt.Printf("%-28s %12d %12d %9s  %s\n", name, c.first, c.second, ratio, verdict)
	}
}
