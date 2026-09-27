package repeatedspot

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	worldspot "exchange_sim/simulations/repeatedspot"
)

func TestTradeAuditRejectsCoherentWrongFeeAndMakerPrice(t *testing.T) {
	newCase := func() (*tradeAudit, *tradeRecord, *replayState) {
		instrument := worldspot.InstrumentConfig{Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1}
		audit := newTradeAudit(instrument)
		audit.orders[1] = acceptedOrder{OrderID: 1, ClientID: 1, Side: "BUY", Qty: 1, Type: "MARKET"}
		audit.orders[2] = acceptedOrder{OrderID: 2, ClientID: 2, Side: "SELL", Qty: 1, Price: 101, Type: "LIMIT"}
		record := &tradeRecord{trade: recordedTrade{TradeID: 1, Price: 101, Qty: 1, Side: "BUY", TakerOrderID: 1, MakerOrderID: 2},
			settlements: []settlementRecord{
				{clientID: 1, changes: map[string]int64{"ABC": 1, "USD": -102}},
				{clientID: 2, changes: map[string]int64{"ABC": -1, "USD": 101}},
			}, venueFees: 1, feeEvidence: 1}
		record.fills = append(record.fills,
			struct {
				clientID uint64
				fill     recordedFill
			}{1, recordedFill{OrderID: 1, Qty: 1, Price: 101, Side: "BUY", Role: "taker", FeeAmount: 1, FeeAsset: "USD"}},
			struct {
				clientID uint64
				fill     recordedFill
			}{2, recordedFill{OrderID: 2, Qty: 1, Price: 101, Side: "SELL", Role: "maker", FeeAmount: 0, FeeAsset: "USD"}})
		state := &replayState{accounts: map[uint64]*accountState{
			1: {clientID: 1, fees: replayPercentageFee{takerBps: 100}},
			2: {clientID: 2, fees: replayPercentageFee{takerBps: 100}},
		}}
		return audit, record, state
	}

	t.Run("valid production arithmetic", func(t *testing.T) {
		audit, record, state := newCase()
		if err := audit.checkTrade(record, state); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("coherently wrong fee", func(t *testing.T) {
		audit, record, state := newCase()
		record.fills[0].fill.FeeAmount = 2
		record.settlements[0].changes["USD"] = -103
		record.venueFees = 2
		record.feeEvidence = 2
		if err := audit.checkTrade(record, state); err == nil || !strings.Contains(err.Error(), "fee differs from contracted percentage schedule") {
			t.Fatalf("fee-consistent postings at the wrong contracted rate were not rejected: %v", err)
		}
	})
	t.Run("wrong resting limit price", func(t *testing.T) {
		audit, record, state := newCase()
		order := audit.orders[2]
		order.Price = 100
		audit.orders[2] = order
		if err := audit.checkTrade(record, state); err == nil || !strings.Contains(err.Error(), "trade price violates resting") {
			t.Fatalf("resting maker price mismatch was not independently rejected: %v", err)
		}
	})
	t.Run("incoming limit cannot pay above limit", func(t *testing.T) {
		audit, record, state := newCase()
		order := audit.orders[1]
		order.Type, order.Price = "LIMIT", 100
		audit.orders[1] = order
		if err := audit.checkTrade(record, state); err == nil || !strings.Contains(err.Error(), "trade price violates resting") {
			t.Fatalf("incoming limit-price violation was not rejected: %v", err)
		}
	})
}

func TestReplayFeeScheduleRejectsUnsupportedCurrencyAndModel(t *testing.T) {
	for _, test := range []struct {
		name, model, parameters string
	}{
		{"unsupported model", "custom_fee", `{"maker_bps":0,"taker_bps":1,"in_quote":true}`},
		{"base-denominated fee", "percentage_v1", `{"maker_bps":0,"taker_bps":1,"in_quote":false}`},
		{"negative fee", "percentage_v1", `{"maker_bps":-1,"taker_bps":1,"in_quote":true}`},
		{"unknown parameter", "percentage_v1", `{"maker_bps":0,"taker_bps":1,"in_quote":true,"rebate":1}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			participant := replayParticipant{}
			participant.Fees.Name = test.model
			participant.Fees.Parameters = json.RawMessage(test.parameters)
			if _, err := parseReplayPercentageFee(participant); err == nil {
				t.Fatal("unsupported fee schedule accepted")
			}
		})
	}
}

func TestReplayRejectsContractFeeChangeWithUnchangedEconomicEvidence(t *testing.T) {
	contract, events, directory := capturedFixture(t)
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(contract, &decoded); err != nil {
		t.Fatal(err)
	}
	var participants []map[string]json.RawMessage
	if err := json.Unmarshal(decoded["participants"], &participants); err != nil {
		t.Fatal(err)
	}
	for _, participant := range participants {
		var fees map[string]json.RawMessage
		if err := json.Unmarshal(participant["fees"], &fees); err != nil {
			t.Fatal(err)
		}
		fees["parameters"] = json.RawMessage(`{"maker_bps":0,"taker_bps":200,"in_quote":true}`)
		participant["fees"], _ = json.Marshal(fees)
	}
	decoded["participants"], _ = json.Marshal(participants)
	changed, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(changed)
	for index := range events {
		if events[index].Name == "world_begin" {
			replacePayloadField(t, &events[index], "contract_sha256", fmt.Sprintf("%x", digest))
			break
		}
	}
	if _, err := replayMutated(t, changed, events, directory); err == nil || !strings.Contains(err.Error(), "fee differs from contracted percentage schedule") {
		t.Fatalf("changed contracted fee did not reach the independent fee check: %v", err)
	}
}
