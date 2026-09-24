package analysis

import (
	"encoding/json"
	"testing"
)

func crossVenueMovementLogRow(t *testing.T, venue string, clientID, frame uint64, at int64, symbol, reason string, baseOld, baseDelta, quoteOld, quoteDelta int64) string {
	t.Helper()
	payload := map[string]any{
		"timestamp": at, "client_id": clientID, "symbol": symbol, "reason": reason,
		"changes": []map[string]any{
			{"asset": "ABC", "wallet": "spot", "old_balance": baseOld, "new_balance": baseOld + baseDelta, "delta": baseDelta},
			{"asset": "USD", "wallet": "spot", "old_balance": quoteOld, "new_balance": quoteOld + quoteDelta, "delta": quoteDelta},
		},
	}
	row := map[string]any{
		"sim_ts": at, "client_id": clientID, "event": "balance_change",
		"data": map[string]any{"venue_id": venue, "global_sequence": frame, "symbol": symbol, "payload": payload},
	}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func crossVenueMovementRun(t *testing.T, mutate func(map[string][]string)) (*Run, []CrossVenueExchangeFill) {
	t.Helper()
	report, fills := crossVenueAccountFixture()
	for index := range report.InitialAccounts {
		report.InitialAccounts[index].Account.Timestamp = 10
		report.TerminalAccounts[index].Account.Timestamp = 30
	}
	for index := range fills {
		fills[index].Event.SimTS = 20
	}
	logs := map[string][]string{
		"north/general.jsonl":      {crossVenueMovementLogRow(t, "north", 7, 1, 10, "", "initial_deposit", 0, 100, 0, 1000)},
		"south/general.jsonl":      {crossVenueMovementLogRow(t, "south", 8, 2, 10, "", "initial_deposit", 0, 100, 0, 1000)},
		"north/spot/ABC-USD.jsonl": {crossVenueMovementLogRow(t, "north", 7, 3, 20, "ABC/USD", "trade_settlement", 100, 5, 1000, -501)},
		"south/spot/ABC-USD.jsonl": {crossVenueMovementLogRow(t, "south", 8, 4, 20, "ABC/USD", "trade_settlement", 100, -5, 1000, 549)},
	}
	if mutate != nil {
		mutate(logs)
	}
	run, err := Open(writeRun(t, report, logs))
	if err != nil {
		t.Fatal(err)
	}
	return run, fills
}

func TestCrossVenueRouterMovementsBindEverySettlementAndRejectOtherFlows(t *testing.T) {
	convention := CrossVenueAccountConvention{Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, TakerFeeBps: 20}
	clients := map[string]uint64{"north": 7, "south": 8}
	run, fills := crossVenueMovementRun(t, nil)
	audit, err := run.AuditCrossVenueRouterMovements([2]string{"north", "south"}, clients, fills, convention)
	if err != nil || audit["north"].Settlements != 1 || audit["south"].Settlements != 1 || audit["north"].InitialBase != 100 || audit["south"].InitialQuote != 1000 {
		t.Fatalf("router movement audit = %#v, %v", audit, err)
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string][]string)
	}{
		{"transfer-instead-of-settlement", func(logs map[string][]string) {
			logs["north/spot/ABC-USD.jsonl"] = []string{crossVenueMovementLogRow(t, "north", 7, 3, 20, "ABC/USD", "transfer", 100, 5, 1000, -501)}
		}},
		{"missing-settlement", func(logs map[string][]string) { logs["north/spot/ABC-USD.jsonl"] = nil }},
		{"duplicate-settlement", func(logs map[string][]string) {
			logs["north/spot/ABC-USD.jsonl"] = append(logs["north/spot/ABC-USD.jsonl"], crossVenueMovementLogRow(t, "north", 7, 5, 20, "ABC/USD", "trade_settlement", 105, 5, 499, -501))
		}},
		{"wrong-wallet", func(logs map[string][]string) {
			var row map[string]any
			if err := json.Unmarshal([]byte(logs["north/spot/ABC-USD.jsonl"][0]), &row); err != nil {
				t.Fatal(err)
			}
			changes := row["data"].(map[string]any)["payload"].(map[string]any)["changes"].([]any)
			changes[0].(map[string]any)["wallet"] = "reserved_spot"
			raw, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			logs["north/spot/ABC-USD.jsonl"][0] = string(raw)
		}},
		{"outer-client-mismatch", func(logs map[string][]string) {
			var row map[string]any
			if err := json.Unmarshal([]byte(logs["north/spot/ABC-USD.jsonl"][0]), &row); err != nil {
				t.Fatal(err)
			}
			row["client_id"] = uint64(99)
			raw, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			logs["north/spot/ABC-USD.jsonl"][0] = string(raw)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			run, fills := crossVenueMovementRun(t, test.mutate)
			if _, err := run.AuditCrossVenueRouterMovements([2]string{"north", "south"}, clients, fills, convention); err == nil {
				t.Fatal("undeclared or missing router movement accepted")
			}
		})
	}
}
