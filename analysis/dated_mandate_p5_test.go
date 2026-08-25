package analysis

import (
	"testing"

	"exchange_sim/exchange"
)

func p5TestMandatePolicy() datedMandateP5Config {
	return datedMandateP5Config{
		Enabled: true, Underlying: "ABC/USD", TargetTenor: 28_800_000_000_000, Side: exchange.Buy.String(),
		ParentQty: 200_000_000, ChildQty: 10_000_000, StartDelay: 600_000_000_000, ExecutionDuration: 7_200_000_000_000,
		DecisionPeriod: 300_000_000_000, MaxMarketAge: 10_000_000_000, SlippageBps: 15, TickSize: 1_000_000,
	}
}

func p5TestMandateSubmission(policy datedMandateP5Config) datedMandateP5Decision {
	listed := int64(1_000_000_000)
	start := listed + policy.StartDelay
	return datedMandateP5Decision{
		VenueID: "north", Desk: "dated_execution_mandate_1", ClientID: 8,
		PolicyVersion: p5DatedMandatePolicyVersion, DecisionTime: start + 1_000_000_000,
		Action: "SUBMIT_CHILD_IOC", Enabled: true, Subscribed: true,
		Symbol: "ABC-FUT", Underlying: policy.Underlying, Side: policy.Side,
		ListedNano: listed, ExpiryNano: listed + policy.TargetTenor, OriginalTenorNanos: policy.TargetTenor,
		ParentQty: policy.ParentQty, RemainingQty: policy.ParentQty, StartAt: start, EndAt: start + policy.ExecutionDuration,
		HasBook: true, BookPublishedAt: start, BookSequence: 4, HasBid: true, Bid: 9_990_000_000, BidQty: 20_000_000,
		HasAsk: true, Ask: 10_000_000_000, AskQty: 20_000_000, MarketAgeNanos: 1_000_000_000,
		LimitPrice: 10_015_000_000, RequestedQty: policy.ChildQty, RequestID: 42,
	}
}

func TestP5MandateOutwardLimitIsIndependentlyRounded(t *testing.T) {
	tests := []struct {
		name       string
		touch, bps int64
		tick       int64
		side       exchange.Side
		want       int64
	}{
		{"buy exact", 10_000_000_000, 15, 1_000_000, exchange.Buy, 10_015_000_000},
		{"buy outward", 10_000_000_001, 15, 1_000_000, exchange.Buy, 10_016_000_000},
		{"sell outward", 10_000_000_001, 15, 1_000_000, exchange.Sell, 9_985_000_000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := p5MandateOutwardLimit(tc.touch, tc.bps, tc.tick, tc.side)
			if !ok || got != tc.want {
				t.Fatalf("outward limit = %d,%t want %d", got, ok, tc.want)
			}
		})
	}
}

func TestP5MandateDecisionMutationsFailClosed(t *testing.T) {
	policy := p5TestMandatePolicy()
	decision := p5TestMandateSubmission(policy)
	if err := validateP5MandateDecision(policy, decision); err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*datedMandateP5Decision)
	}{
		{"wrong tenor", func(d *datedMandateP5Decision) { d.OriginalTenorNanos++ }},
		{"future book", func(d *datedMandateP5Decision) { d.BookPublishedAt = d.DecisionTime + 1 }},
		{"forged parent progress", func(d *datedMandateP5Decision) { d.FilledQty++ }},
		{"wrong limit", func(d *datedMandateP5Decision) { d.LimitPrice++ }},
		{"wrong child", func(d *datedMandateP5Decision) { d.RequestedQty++ }},
		{"outside horizon", func(d *datedMandateP5Decision) { d.DecisionTime = d.EndAt }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			mutant := decision
			tc.mutate(&mutant)
			if err := validateP5MandateDecision(policy, mutant); err == nil {
				t.Fatal("mandate mutation survived")
			}
		})
	}
}

func TestP5MandateGatewayCannotForgeAtomicOrHiddenExecution(t *testing.T) {
	policy := p5TestMandatePolicy()
	decision := p5TestMandateSubmission(policy)
	gateway := fundingCarryGatewayDecision{
		clientID: decision.ClientID, linkID: 1, requestID: decision.RequestID, symbol: decision.Symbol,
		decisionAt: decision.DecisionTime, price: decision.LimitPrice, qty: decision.RequestedQty,
		side: uint8(exchange.Buy), orderType: uint8(exchange.LimitOrder), tif: uint8(exchange.IOC),
	}
	if !p5MandateGatewayMatches(decision, gateway) {
		t.Fatal("exact mandate gateway rejected")
	}
	gateway.qty = policy.ParentQty
	if p5MandateGatewayMatches(decision, gateway) {
		t.Fatal("parent-sized atomic substitution survived")
	}
}
