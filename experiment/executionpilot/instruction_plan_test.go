package executionpilot

import (
	"encoding/json"
	"testing"

	"exchange_sim/exchange"
)

func TestInstructionPlanBindsIOCAndFOKWithoutAdmittingOtherCells(t *testing.T) {
	identity := fixtureIdentity()
	identity.EvidenceSchemaID = InstructionEvidenceSchemaID
	for _, timeInForce := range []string{"IOC", "FOK"} {
		cell := InstructionCell{TimeInForce: timeInForce, TargetQty: 500_000_000, Seed: 14001}
		plan, err := LockInstruction(cell, identity)
		if err != nil {
			t.Fatal(err)
		}
		world, err := VerifyInstruction(plan, identity)
		if err != nil {
			t.Fatal(err)
		}
		contract := world.WorldContract()
		instruction := contract.Parents[0].Config.Instruction
		if instruction == nil || instruction.OrderType != exchange.LimitOrder ||
			instruction.TimeInForce.String() != timeInForce || instruction.LimitPrice != InstructionLimitPrice ||
			contract.Parents[0].ClientID != 13 || contract.Parents[0].Latency != 1_000_000 {
			t.Fatalf("instruction plan did not bind effective world: %+v", contract.Parents[0])
		}
		raw, err := json.Marshal(plan)
		if err != nil {
			t.Fatal(err)
		}
		decoded, _, err := DecodeInstructionPlan(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyInstruction(decoded, identity); err != nil {
			t.Fatal(err)
		}
		decoded.Cell.TargetQty++
		if _, err := VerifyInstruction(decoded, identity); err == nil {
			t.Fatal("mutated target cell passed typed-plan verification")
		}
		var effective map[string]any
		if err := json.Unmarshal(plan.EffectiveWorld, &effective); err != nil {
			t.Fatal(err)
		}
		parents := effective["parents"].([]any)
		config := parents[0].(map[string]any)["config"].(map[string]any)
		config["instruction"].(map[string]any)["limit_price"] = float64(InstructionLimitPrice + 1_000_000)
		plan.EffectiveWorld, err = json.Marshal(effective)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyInstruction(plan, identity); err == nil {
			t.Fatal("mutated cap passed effective-world verification")
		}
	}
	for _, invalid := range []InstructionCell{
		{TimeInForce: "GTC", TargetQty: 50_000_000, Seed: 14001},
		{TimeInForce: "IOC", TargetQty: 200_000_000, Seed: 14001},
		{TimeInForce: "FOK", TargetQty: 50_000_000, Seed: 619},
	} {
		if _, err := LockInstruction(invalid, identity); err == nil {
			t.Fatalf("out-of-matrix instruction cell accepted: %+v", invalid)
		}
	}
	if _, _, err := DecodeInstructionPlan([]byte(`{"schema_version":1,"schema_version":1}`)); err == nil {
		t.Fatal("duplicate raw plan key accepted")
	}
	identity.EvidenceSchemaID = LatencyEvidenceSchemaID
	if _, err := LockInstruction(InstructionCell{TimeInForce: "IOC", TargetQty: 50_000_000, Seed: 14001}, identity); err == nil {
		t.Fatal("ME-002 evidence schema accepted for ME-003")
	}
}
