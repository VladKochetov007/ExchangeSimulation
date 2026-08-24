package analysis

import (
	"testing"
)

func termCarryAuditPolicy() termCarryPolicyConfig {
	return termCarryPolicyConfig{
		SpotSymbol: "ABC/USD", PerpSymbol: "ABC-PERP", DecisionPeriod: 1_000_000_000, CommitmentIntervals: 12, MaxFundingAge: 60 * 1_000_000_000,
		TakerFeeBps: 5, LongSpotFundingBps: 500, ShortSpotBorrowBps: 700,
		BalanceSheetBps: 1, MarginRiskBps: 1, LegRiskBps: 1, MinNetCarryBps: 1,
		MaxPosition: 50, LotQty: 50, MinOrderSize: 1, SpotTick: 1, PerpTick: 1,
	}
}

func validTermCarryEntry(t *testing.T, policy termCarryPolicyConfig, rate int64) termCarryDecision {
	t.Helper()
	decision := termCarryDecision{
		VenueID: "north", Desk: "term_carry_allocator_1", ClientID: 9, PolicyVersion: "v2_5_p3_term_carry_v1",
		DecisionTime: 1_000, Enabled: true, Subscribed: true, State: "ENTRY_SPOT", Action: "SUBMIT_ENTRY_SPOT_IOC",
		SpotSymbol: policy.SpotSymbol, PerpSymbol: policy.PerpSymbol, CommitmentIntervals: policy.CommitmentIntervals,
		HasSpotBook: true, SpotPublishedAt: 900, SpotSequence: 11, HasSpotBid: true, SpotBid: 100, SpotBidQty: 50, HasSpotAsk: true, SpotAsk: 101, SpotAskQty: 50,
		HasPerpBook: true, PerpPublishedAt: 900, PerpSequence: 12, HasPerpBid: true, PerpBid: 102, PerpBidQty: 50, HasPerpAsk: true, PerpAsk: 103, PerpAskQty: 50,
		HasFunding: true, FundingRateBps: rate, FundingPublishedAt: 900, FundingSequence: 13, FundingNextAt: 1_000 + 8*60*60*1_000_000_000, FundingIntervalSeconds: 8 * 60 * 60, FundingAgeNanos: 100,
		TargetSpot: policy.MaxPosition, TargetPerp: -policy.MaxPosition, Leg: "ENTRY_SPOT_IOC", Side: "BUY", LimitPrice: 101, RequestedQty: 50, RequestID: 99,
	}
	financials, ok := termCarryAuditFinancials(policy, decision, 1)
	if !ok {
		t.Fatal("build financials")
	}
	decision.TermEnd = financials.termEnd
	decision.ExpectedFundingBps = financials.funding.String()
	decision.ExecutionFeeBps = financials.fees.String()
	decision.FinancingBpsNumerator = financials.financing.String()
	decision.NetCarryBpsNumerator = financials.net.String()
	decision.RationalDenominator = financials.denominator.String()
	decision.FinancingDirection = financials.direction
	return decision
}

func TestTermCarryEntryEconomicsRejectsForgedFundingAndFinancing(t *testing.T) {
	policy := termCarryAuditPolicy()
	decision := validTermCarryEntry(t, policy, 3)
	if err := validateTermCarryEntryEconomics(policy, decision); err != nil {
		t.Fatalf("valid decision rejected: %v", err)
	}
	decision.ExpectedFundingBps = "-36"
	if err := validateTermCarryEntryEconomics(policy, decision); err == nil {
		t.Fatal("reversed funding-sign mutation survived")
	}
	decision = validTermCarryEntry(t, policy, 3)
	decision.FinancingBpsNumerator = "0"
	if err := validateTermCarryEntryEconomics(policy, decision); err == nil {
		t.Fatal("dropped nonzero financing mutation survived")
	}
}

func TestTermCarryEntryEconomicsTreatsPresentZeroFundingAsEconomicInput(t *testing.T) {
	policy := termCarryAuditPolicy()
	decision := validTermCarryEntry(t, policy, 0)
	decision.Action, decision.RequestID, decision.Leg, decision.Side, decision.RequestedQty = "NET_CARRY_BELOW_MINIMUM", 0, "", "", 0
	if err := validateTermCarryEntryEconomics(policy, decision); err != nil {
		t.Fatalf("present zero funding became invalid/unavailable: %v", err)
	}
	if decision.HasFunding != true || decision.FundingRateBps != 0 {
		t.Fatalf("zero funding presence changed: %+v", decision)
	}
}

func TestTermCarrySubmissionRejectsForgedTouch(t *testing.T) {
	policy := termCarryAuditPolicy()
	decision := validTermCarryEntry(t, policy, 3)
	if err := validateTermCarrySubmission(policy, decision); err != nil {
		t.Fatalf("valid touch rejected: %v", err)
	}
	decision.LimitPrice = 100
	if err := validateTermCarrySubmission(policy, decision); err == nil {
		t.Fatal("off-touch entry mutation survived")
	}
}
