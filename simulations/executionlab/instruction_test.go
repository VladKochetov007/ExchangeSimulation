package executionlab

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"exchange_sim/exchange"
)

func TestChildInstructionRejectsInvalidSpotOrderContract(t *testing.T) {
	invalid := []ChildInstruction{
		{OrderType: exchange.Market, TimeInForce: exchange.IOC, LimitPrice: 1},
		{OrderType: exchange.LimitOrder, TimeInForce: exchange.IOC},
		{OrderType: exchange.LimitOrder, TimeInForce: exchange.FOK, LimitPrice: -1},
		{OrderType: exchange.OrderType(99), TimeInForce: exchange.IOC, LimitPrice: 1},
		{OrderType: exchange.LimitOrder, TimeInForce: exchange.TimeInForce(99), LimitPrice: 1},
	}
	for _, instruction := range invalid {
		config := DefaultSimConfig(Immediate)
		config.Parent.Instruction = &instruction
		if _, err := NewSim(config); err == nil {
			t.Fatalf("accepted invalid child instruction %+v", instruction)
		}
	}
}

func TestDefaultChildInstructionIsAbsentFromLegacyContract(t *testing.T) {
	world, err := NewSim(DefaultSimConfig(Immediate))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(world.WorldContract())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(`"instruction"`)) {
		t.Fatal("default child instruction changed the legacy effective world")
	}
}

func TestLimitInstructionFollowsConfigAndWorldContractIsIsolated(t *testing.T) {
	for _, timeInForce := range []exchange.TimeInForce{exchange.IOC, exchange.FOK} {
		config := DefaultSimConfig(Immediate)
		config.Parent.Instruction = &ChildInstruction{
			OrderType: exchange.LimitOrder, TimeInForce: timeInForce,
			LimitPrice: bootstrapPrice + 10*priceTick,
		}
		world, err := NewSim(config)
		if err != nil {
			t.Fatal(err)
		}
		config.Parent.Instruction.LimitPrice = bootstrapPrice
		if got := world.WorldContract().Config.Parent.Instruction.LimitPrice; got != bootstrapPrice+10*priceTick {
			t.Fatalf("caller mutation changed locked instruction: %d", got)
		}
		copyOfContract := world.WorldContract()
		copyOfContract.Config.Parent.Instruction.LimitPrice = bootstrapPrice
		copyOfContract.Parents[0].Config.Instruction.LimitPrice = bootstrapPrice
		if got := world.WorldContract().Config.Parent.Instruction.LimitPrice; got != bootstrapPrice+10*priceTick {
			t.Fatalf("contract-copy mutation changed instruction: %d", got)
		}
		var order *exchange.OrderRequest
		world.SetEvidenceObserver(func(event EvidenceObservation) {
			if event.Source == "actor" && event.Name == "order_send" && event.ClientID == 13 {
				request := event.Payload.(exchange.Request)
				copyOfOrder := *request.OrderReq
				order = &copyOfOrder
			}
		})
		if _, err := world.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if order == nil || order.Type != exchange.LimitOrder || order.TimeInForce != timeInForce ||
			order.Price != bootstrapPrice+10*priceTick || order.Qty != config.Parent.TargetQty {
			t.Fatalf("submitted order does not match configured child instruction: %#v", order)
		}
	}
}
