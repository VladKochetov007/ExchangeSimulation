package repeatedspot

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	worldspot "exchange_sim/simulations/repeatedspot"
)

func TestBufferedFillReceiptMayPrecedeProcessedAcceptance(t *testing.T) {
	account := &accountState{actorID: 7, clientID: 2, maker: &makerParameters{}, workingLimit: 5,
		acceptedLocally: make(map[uint64]bool), localRisk: newInventoryRiskSeries(0, MeasurementWindow{StartAt: 0, EndAt: 10}, 5)}
	accepted := worldspot.MakerProcessedResponse{ActorID: 7, Kind: "accepted", RequestID: 8, OrderID: 10}
	fill := worldspot.MakerProcessedResponse{ActorID: 7, Kind: "fill", OrderID: 10, TradeID: 0,
		Symbol: "ABC/USD", Qty: 1, Price: 101, Side: "BUY", ExchangeAt: 1}
	state := &replayState{accounts: map[uint64]*accountState{2: account}, makerReceipts: map[makerReceiptKey]*makerReceipt{},
		contract: replayContract{Instrument: worldspot.InstrumentConfig{Symbol: "ABC/USD"}}}
	state.makerReceipts[makerKey(2, fill)] = &makerReceipt{response: fill, deliveredAt: 2, sequence: 1}
	state.makerReceipts[makerKey(2, accepted)] = &makerReceipt{response: accepted, deliveredAt: 3, sequence: 2}
	process := func(response worldspot.MakerProcessedResponse, sequence uint64) error {
		response.ProcessedAt = 3
		if response.Kind == "fill" {
			response.FilledInventory = 1
		}
		payload, err := json.Marshal(response)
		if err != nil {
			return err
		}
		return state.makerProcessedResponse(Event{Sequence: sequence, Timestamp: 3, ClientID: 2,
			Source: "actor", Name: "maker_processed_response", Route: "ABC/USD", Payload: payload})
	}
	if err := process(accepted, 3); err != nil {
		t.Fatal(err)
	}
	if err := process(fill, 4); err != nil {
		t.Fatal(err)
	}
	if account.localResponses.NetProcessedFillBase != 1 || !state.makerReceipts[makerKey(2, fill)].consumed {
		t.Fatal("queued first fill was not attributed after acceptance")
	}
}

func capturedMakerFill(t *testing.T) ([]byte, []Event, string) {
	t.Helper()
	world := fixtureWorldWithAggressorSchedule(t, true, false, 1, 2*time.Second, true, 2, 5*time.Second)
	return capturedFixtureWorld(t, world)
}

func makerFillReceiptIndex(t *testing.T, events []Event) int {
	t.Helper()
	for index, event := range events {
		if event.Source != "gateway" || event.Name != "maker_response_receipt" || event.ClientID != 2 {
			continue
		}
		var wire makerWireResponse
		if err := json.Unmarshal(event.Payload, &wire); err != nil {
			t.Fatal(err)
		}
		var data map[string]json.RawMessage
		if err := json.Unmarshal(wire.Data, &data); err == nil && data["trade_id"] != nil {
			return index
		}
	}
	t.Fatal("fixture lacks maker fill receipt")
	return -1
}

func makerProcessedFillIndex(t *testing.T, events []Event) int {
	t.Helper()
	for index, event := range events {
		if event.Source != "actor" || event.Name != "maker_processed_response" || event.ClientID != 2 {
			continue
		}
		var processed struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(event.Payload, &processed); err != nil {
			t.Fatal(err)
		}
		if processed.Kind == "fill" {
			return index
		}
	}
	t.Fatal("fixture lacks maker processed fill")
	return -1
}

func TestMakerFillResponseTimeline(t *testing.T) {
	contract, events, directory := capturedMakerFill(t)
	replay, err := replayMutated(t, contract, events, directory)
	if err != nil {
		t.Fatal(err)
	}
	maker := replay.Accounts[1]
	if maker.ClientID != 2 || maker.LocalResponses.FillReceived != 1 ||
		maker.LocalResponses.FillProcessed != 1 || maker.LocalResponses.NetProcessedFillBase != -1 {
		t.Fatalf("maker fill did not settle on its processed local response: %+v", maker)
	}
	receiptAt := makerFillReceiptIndex(t, events)
	processedAt := makerProcessedFillIndex(t, events)
	if receiptAt >= processedAt {
		t.Fatal("maker processed fill before gateway delivery")
	}
	var receipt makerWireResponse
	if err := json.Unmarshal(events[receiptAt].Payload, &receipt); err != nil {
		t.Fatal(err)
	}
	var fill makerWireFill
	if err := json.Unmarshal(receipt.Data, &fill); err != nil {
		t.Fatal(err)
	}
	if fill.TradeID != 1 || fill.ClientID != 2 || fill.Side != "SELL" ||
		events[receiptAt].Timestamp < fill.Timestamp || events[processedAt].Timestamp < events[receiptAt].Timestamp {
		t.Fatalf("wrong maker-fill identity or causal times: %+v", fill)
	}
}

func TestMakerResponseTimelineRejectsRehashedCorruption(t *testing.T) {
	contract, original, directory := capturedMakerFill(t)
	mutations := map[string]func(*testing.T, []Event) []Event{
		"missing acceptance receipt and processing": func(t *testing.T, events []Event) []Event {
			var requestID, orderID uint64
			for index, event := range events {
				if event.ClientID != 2 || event.Name != "maker_processed_response" {
					continue
				}
				var response worldspot.MakerProcessedResponse
				if err := json.Unmarshal(event.Payload, &response); err != nil {
					t.Fatal(err)
				}
				if response.Kind == "accepted" {
					requestID, orderID = response.RequestID, response.OrderID
					events = append(events[:index], events[index+1:]...)
					break
				}
			}
			if requestID == 0 || orderID == 0 {
				t.Fatal("fixture lacks maker acceptance")
			}
			for index, event := range events {
				if event.ClientID != 2 || event.Name != "maker_response_receipt" {
					continue
				}
				var wire makerWireResponse
				if err := json.Unmarshal(event.Payload, &wire); err != nil {
					t.Fatal(err)
				}
				if wire.RequestID == requestID {
					return append(events[:index], events[index+1:]...)
				}
			}
			t.Fatal("fixture lacks corresponding maker acceptance receipt")
			return nil
		},
		"missing fill receipt": func(t *testing.T, events []Event) []Event {
			index := makerFillReceiptIndex(t, events)
			return append(events[:index], events[index+1:]...)
		},
		"missing receipt and processing": func(t *testing.T, events []Event) []Event {
			processed := makerProcessedFillIndex(t, events)
			events = append(events[:processed], events[processed+1:]...)
			receipt := makerFillReceiptIndex(t, events)
			return append(events[:receipt], events[receipt+1:]...)
		},
		"duplicate receipt": func(t *testing.T, events []Event) []Event {
			index := makerFillReceiptIndex(t, events)
			return append(events[:index+1], append([]Event{events[index]}, events[index+1:]...)...)
		},
		"forged delivered price": func(t *testing.T, events []Event) []Event {
			index := makerFillReceiptIndex(t, events)
			var response map[string]json.RawMessage
			if err := json.Unmarshal(events[index].Payload, &response); err != nil {
				t.Fatal(err)
			}
			var data map[string]json.RawMessage
			if err := json.Unmarshal(response["data"], &data); err != nil {
				t.Fatal(err)
			}
			data["price"] = json.RawMessage("999")
			response["data"], _ = json.Marshal(data)
			events[index].Payload, _ = json.Marshal(response)
			return events
		},
		"forged processed inventory": func(t *testing.T, events []Event) []Event {
			index := makerProcessedFillIndex(t, events)
			replacePayloadField(t, &events[index], "filled_inventory", 0)
			return events
		},
		"processing before receipt": func(t *testing.T, events []Event) []Event {
			receipt := makerFillReceiptIndex(t, events)
			processed := makerProcessedFillIndex(t, events)
			if events[receipt].Timestamp != events[processed].Timestamp {
				t.Fatal("fixture no longer has same-time gateway and actor phases")
			}
			events[receipt], events[processed] = events[processed], events[receipt]
			return events
		},
		"maker decision invents inventory": func(t *testing.T, events []Event) []Event {
			for index := range events {
				if events[index].Name != "maker_decision" || events[index].ClientID != 2 {
					continue
				}
				var decision worldspot.MakerDecision
				if err := json.Unmarshal(events[index].Payload, &decision); err != nil {
					t.Fatal(err)
				}
				if decision.FilledInventory != 0 {
					replacePayloadField(t, &events[index], "filled_inventory", 0)
					return events
				}
			}
			t.Fatal("fixture lacks post-fill maker decision")
			return nil
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			events := append([]Event(nil), original...)
			for index := range events {
				events[index].Payload = bytes.Clone(events[index].Payload)
			}
			events = mutate(t, events)
			if _, err := replayMutated(t, contract, events, directory); err == nil {
				t.Fatal("rehashed maker-response corruption was accepted")
			}
		})
	}
}
