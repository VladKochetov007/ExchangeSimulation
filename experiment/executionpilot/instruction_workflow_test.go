package executionpilot

import (
	"bytes"
	"encoding/json"
	"testing"

	"exchange_sim/exchange"
)

func TestInstructionActorReportChecksChildAgainstIndependentReplay(t *testing.T) {
	for _, timeInForce := range []exchange.TimeInForce{exchange.IOC, exchange.FOK} {
		t.Run(timeInForce.String(), func(t *testing.T) {
			raw, identity, world, actorReport := instructionFixture(t, timeInForce, 500_000_000, 5_002_000_000)
			outcome, err := ReconstructInstruction(bytes.NewReader(raw), identity, world, 500_000_000)
			if err != nil {
				t.Fatal(err)
			}
			actorRaw, err := json.Marshal(actorReport)
			if err != nil {
				t.Fatal(err)
			}
			if err := compareInstructionActorReport(actorRaw, outcome); err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(actorRaw, &fields); err != nil {
				t.Fatal(err)
			}
			var children []map[string]any
			if err := json.Unmarshal(fields["Children"], &children); err != nil {
				t.Fatal(err)
			}
			children[0]["QuoteFee"] = float64(999999)
			fields["Children"], err = json.Marshal(children)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if err := compareInstructionActorReport(changed, outcome); err == nil {
				t.Fatal("changed actor child fee was accepted")
			}
			delete(children[0], "RequestID")
			fields["Children"], err = json.Marshal(children)
			if err != nil {
				t.Fatal(err)
			}
			changed, err = json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if err := compareInstructionActorReport(changed, outcome); err == nil {
				t.Fatal("missing actor child identity was accepted")
			}
		})
	}
}
