package repeatedspot

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"exchange_sim/simulation"
	worldspot "exchange_sim/simulations/repeatedspot"
)

func capturedFixture(t *testing.T) ([]byte, []Event, string) {
	t.Helper()
	return capturedFixtureWorld(t, fixtureWorld(t))
}

func capturedFixtureWorld(t *testing.T, world *worldspot.World) ([]byte, []Event, string) {
	t.Helper()
	directory := t.TempDir()
	receipts, err := simulation.NewMarketDataReceiptRecorder(directory)
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	capture, err := NewCapture(world, &raw, receipts)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := capture.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err := WalkEvidence(bytes.NewReader(raw.Bytes()), identity, func(event Event) error {
		event.Payload = bytes.Clone(event.Payload)
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return world.ContractJSON(), events, directory
}

func replayMutated(t *testing.T, contract []byte, events []Event, directory string) (*EconomicReplay, error) {
	t.Helper()
	var raw bytes.Buffer
	recorder := NewRecorder(&raw)
	for _, event := range events {
		recorder.Record(event.Timestamp, event.ClientID, event.Source, event.Name, event.Route, event.Payload)
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	return Replay(contract, bytes.NewReader(raw.Bytes()), identity, directory)
}

func firstEvent(t *testing.T, events []Event, name string) int {
	t.Helper()
	for index := range events {
		if events[index].Name == name {
			return index
		}
	}
	t.Fatalf("fixture has no %s", name)
	return -1
}

func replacePayloadField(t *testing.T, event *Event, field string, value any) {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	payload[field] = encoded
	event.Payload, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
}

func TestProductionLocalReferenceEvidenceRejectsRehashedForgedSource(t *testing.T) {
	world := fixtureWorldWithReferencePolicy(t, true, false, 1, 2*time.Second,
		true, 2, 5*time.Second, true, 15*time.Second)
	contract, original, directory := capturedFixtureWorld(t, world)
	if _, err := replayMutated(t, contract, original, directory); err != nil {
		t.Fatalf("valid versioned local-reference fixture did not replay: %v", err)
	}
	selected := -1
	for index, event := range original {
		if event.Name != "maker_decision" {
			continue
		}
		var decision worldspot.MakerDecision
		if err := json.Unmarshal(event.Payload, &decision); err != nil {
			t.Fatal(err)
		}
		if decision.ReferenceMode == "cached_two_sided" {
			selected = index
			break
		}
	}
	if selected < 0 {
		t.Fatal("synthetic one-sided production path did not exercise the cached-reference decision")
	}
	for name, mutate := range map[string]func(*Event){
		"price": func(event *Event) {
			var decision worldspot.MakerDecision
			if err := json.Unmarshal(event.Payload, &decision); err != nil {
				t.Fatal(err)
			}
			replacePayloadField(t, event, "reference_mid", decision.ReferenceMid+1)
		},
		"source sequence": func(event *Event) { replacePayloadField(t, event, "reference_sequence", 999999) },
		"missing mode":    func(event *Event) { replacePayloadField(t, event, "reference_mode", "") },
	} {
		t.Run(name, func(t *testing.T) {
			changed := append([]Event(nil), original...)
			changed[selected].Payload = bytes.Clone(changed[selected].Payload)
			mutate(&changed[selected])
			if _, err := replayMutated(t, contract, changed, directory); err == nil {
				t.Fatal("rehashed forged local reference passed independent replay")
			}
		})
	}
}

func TestIndependentReplayRejectsRehashedSemanticCorruption(t *testing.T) {
	contract, original, directory := capturedFixture(t)
	for name, mutate := range map[string]func(*testing.T, []Event) []Event{
		"wrong fill fee": func(t *testing.T, events []Event) []Event {
			index := firstEvent(t, events, "OrderFill")
			replacePayloadField(t, &events[index], "fee_amount", 2)
			return events
		},
		"missing outbound placement": func(t *testing.T, events []Event) []Event {
			index := firstEvent(t, events, "order_send")
			return append(events[:index], events[index+1:]...)
		},
		"missing venue admission outcome": func(t *testing.T, events []Event) []Event {
			index := firstEvent(t, events, "OrderAccepted")
			return append(events[:index], events[index+1:]...)
		},
		"outbound placement price differs from venue admission": func(t *testing.T, events []Event) []Event {
			for index := range events {
				if events[index].Name != "order_send" {
					continue
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(events[index].Payload, &payload); err != nil {
					t.Fatal(err)
				}
				var order map[string]any
				if err := json.Unmarshal(payload["OrderReq"], &order); err != nil {
					t.Fatal(err)
				}
				order["price"] = order["price"].(float64) + 1
				replacePayloadField(t, &events[index], "OrderReq", order)
				return events
			}
			t.Fatal("fixture lacks outbound order")
			return nil
		},
		"forged cumulative maker fill": func(t *testing.T, events []Event) []Event {
			for index := range events {
				if events[index].Name != "OrderFill" {
					continue
				}
				var fill struct {
					Role string `json:"role"`
				}
				if err := json.Unmarshal(events[index].Payload, &fill); err != nil {
					t.Fatal(err)
				}
				if fill.Role == "maker" {
					replacePayloadField(t, &events[index], "filled_qty", 2)
					replacePayloadField(t, &events[index], "remaining_qty", 0)
					return events
				}
			}
			t.Fatal("fixture lacks a maker fill")
			return nil
		},
		"missing settlement": func(t *testing.T, events []Event) []Event {
			index := firstEvent(t, events, "balance_change")
			return append(events[:index], events[index+1:]...)
		},
		"missing final public book delta": func(t *testing.T, events []Event) []Event {
			for index := len(events) - 1; index >= 0; index-- {
				if events[index].Name == "BookDelta" {
					return append(events[:index], events[index+1:]...)
				}
			}
			t.Fatal("fixture lacks a public book delta")
			return nil
		},
		"reordered settlement": func(t *testing.T, events []Event) []Event {
			trade := firstEvent(t, events, "Trade")
			events[trade], events[trade-1] = events[trade-1], events[trade]
			return events
		},
		"duplicate fill": func(t *testing.T, events []Event) []Event {
			first := firstEvent(t, events, "OrderFill")
			second := first + 1
			if events[second].Name != "OrderFill" {
				t.Fatal("fixture lacks adjacent fill pair")
			}
			events[second].Payload = bytes.Clone(events[first].Payload)
			return events
		},
		"wrong terminal account": func(t *testing.T, events []Event) []Event {
			for index := len(events) - 1; index >= 0; index-- {
				if events[index].Name == "balance_snapshot" {
					var payload map[string]json.RawMessage
					if err := json.Unmarshal(events[index].Payload, &payload); err != nil {
						t.Fatal(err)
					}
					var balances []map[string]any
					if err := json.Unmarshal(payload["spot_balances"], &balances); err != nil {
						t.Fatal(err)
					}
					balances[0]["free"] = balances[0]["free"].(float64) + 1
					replacePayloadField(t, &events[index], "spot_balances", balances)
					return events
				}
			}
			t.Fatal("fixture lacks terminal account")
			return nil
		},
		"future maker source": func(t *testing.T, events []Event) []Event {
			index := firstEvent(t, events, "maker_decision")
			replacePayloadField(t, &events[index], "book_seen", true)
			replacePayloadField(t, &events[index], "latest_book_source_at", events[index].Timestamp+1)
			return events
		},
		"forged maker snapshot source": func(t *testing.T, events []Event) []Event {
			for index := range events {
				if events[index].Name == "maker_observation" {
					var observation struct {
						Kind string `json:"kind"`
					}
					if err := json.Unmarshal(events[index].Payload, &observation); err != nil {
						t.Fatal(err)
					}
					if observation.Kind == "snapshot" {
						replacePayloadField(t, &events[index], "source_sequence", 999999)
						return events
					}
				}
			}
			t.Fatal("fixture lacks maker snapshot observation")
			return nil
		},
		"forged maker decision view": func(t *testing.T, events []Event) []Event {
			for index := range events {
				if events[index].Name == "maker_decision" {
					var decision struct {
						BookSeen bool `json:"book_seen"`
					}
					if err := json.Unmarshal(events[index].Payload, &decision); err != nil {
						t.Fatal(err)
					}
					if decision.BookSeen {
						replacePayloadField(t, &events[index], "best_bid", 123456)
						return events
					}
				}
			}
			t.Fatal("fixture lacks maker decision with a book")
			return nil
		},
		"forged delivered variance": func(t *testing.T, events []Event) []Event {
			index := firstEvent(t, events, "maker_decision")
			replacePayloadField(t, &events[index], "log_variance_per_second", 0.5)
			return events
		},
		"omitted scheduled maker decision": func(t *testing.T, events []Event) []Event {
			index := firstEvent(t, events, "maker_decision")
			return append(events[:index], events[index+1:]...)
		},
		"forged target quote": func(t *testing.T, events []Event) []Event {
			for index := range events {
				if events[index].Name != "maker_decision" {
					continue
				}
				var decision struct {
					TargetBid int64 `json:"target_bid"`
				}
				if err := json.Unmarshal(events[index].Payload, &decision); err != nil {
					t.Fatal(err)
				}
				if decision.TargetBid > 0 {
					replacePayloadField(t, &events[index], "target_bid", decision.TargetBid+1)
					return events
				}
			}
			t.Fatal("fixture lacks a computed maker quote")
			return nil
		},
		"forged public depth with same best price": func(t *testing.T, events []Event) []Event {
			var observedSequence uint64
			found := false
			for _, event := range events {
				if event.Name != "maker_observation" {
					continue
				}
				var observation struct {
					Kind           string `json:"kind"`
					SourceSequence uint64 `json:"source_sequence"`
				}
				if err := json.Unmarshal(event.Payload, &observation); err != nil {
					t.Fatal(err)
				}
				if observation.Kind == "snapshot" {
					observedSequence, found = observation.SourceSequence, true
					break
				}
			}
			if !found {
				t.Fatal("fixture lacks maker snapshot observation")
			}
			for index := range events {
				if events[index].Name != "BookSnapshot" {
					continue
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(events[index].Payload, &payload); err != nil {
					t.Fatal(err)
				}
				var sequence uint64
				if err := json.Unmarshal(payload["source_sequence"], &sequence); err != nil {
					t.Fatal(err)
				}
				if sequence != observedSequence {
					continue
				}
				var bids []map[string]any
				if err := json.Unmarshal(payload["public_bids"], &bids); err != nil {
					t.Fatal(err)
				}
				if len(bids) > 0 {
					bids[0]["visible_qty"] = bids[0]["visible_qty"].(float64) + 1
					replacePayloadField(t, &events[index], "public_bids", bids)
					return events
				}
			}
			t.Fatal("fixture lacks public bid projection")
			return nil
		},
		"false completion": func(t *testing.T, events []Event) []Event {
			index := firstEvent(t, events, "world_end")
			events[index].Name = "world_failed"
			return events
		},
	} {
		t.Run(name, func(t *testing.T) {
			events := append([]Event(nil), original...)
			events = mutate(t, events)
			if _, err := replayMutated(t, contract, events, directory); err == nil {
				t.Fatal("rehashed semantic corruption accepted")
			}
		})
	}
}

func TestIndependentReplayReconstructsProductionStoikovQuote(t *testing.T) {
	contract, events, directory := capturedFixtureWorld(t, fixtureWorldWithMaker(t, true, true))
	var computed int
	for _, event := range events {
		if event.Name != "maker_decision" {
			continue
		}
		var decision struct {
			TargetBid int64 `json:"target_bid"`
		}
		if err := json.Unmarshal(event.Payload, &decision); err != nil {
			t.Fatal(err)
		}
		if decision.TargetBid > 0 {
			computed++
		}
	}
	if computed == 0 {
		t.Fatal("Stoikov fixture did not compute a quote")
	}
	if _, err := replayMutated(t, contract, events, directory); err != nil {
		t.Fatal(err)
	}
	for index := range events {
		if events[index].Name != "maker_decision" {
			continue
		}
		var decision struct {
			TargetAsk int64 `json:"target_ask"`
		}
		if err := json.Unmarshal(events[index].Payload, &decision); err != nil {
			t.Fatal(err)
		}
		if decision.TargetAsk > 0 {
			replacePayloadField(t, &events[index], "target_ask", decision.TargetAsk+1)
			if _, err := replayMutated(t, contract, events, directory); err == nil {
				t.Fatal("rehashed Stoikov target mutation was accepted")
			}
			return
		}
	}
	t.Fatal("Stoikov fixture lacks a positive ask target")
}

func TestIndependentReplayKeepsOneSidedTerminalMarkUnavailable(t *testing.T) {
	contract, events, directory := capturedFixtureWorld(t,
		fixtureWorldWithBook(t, true, false, 1, 20*time.Second, false))
	report, err := replayMutated(t, contract, events, directory)
	if err != nil {
		t.Fatal(err)
	}
	if report.TerminalMarkStatus != "UNAVAILABLE_ONE_SIDED_OR_EMPTY" || report.TerminalMidQuote != nil {
		t.Fatalf("one-sided mark treated as available: %+v", report)
	}
	for _, account := range report.Accounts {
		if account.BenchmarkGain != nil {
			t.Fatal("one-sided terminal book yielded a benchmark gain")
		}
	}
}
