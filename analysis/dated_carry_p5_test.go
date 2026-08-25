package analysis

import (
	"encoding/hex"
	"testing"

	"exchange_sim/exchange"
)

func p5TestPolicy(trade bool) datedCarryP5Config {
	return datedCarryP5Config{
		Enabled: true, TradeEnabled: trade, SpotSymbol: "ABC/USD", TargetTenor: 8 * 60 * 60 * 1_000_000_000,
		DecisionPeriod: 2_000_000_000, MaxMarketAge: 10_000_000_000, MinTimeToExpiry: 600_000_000_000,
		TakerFeeBps: 5, LongSpotFundingBps: 500, ShortSpotBorrowBps: 500,
		BalanceSheetBps: 1, MarginRiskBps: 1, LegRiskBps: 1, SettlementMismatchBps: 2, PostSettlementExitBps: 2,
		MinNetCarryBps: 1, MaxPosition: 100_000_000, LotQty: 10_000_000, MinOrderSize: 100_000,
		SpotTick: 1_000_000, FutureTick: 1_000_000, PassiveExitSliceQty: 100_000, ExitDeadlineAfterSettlement: 3_600_000_000_000,
	}
}

func p5TestCandidate(policy datedCarryP5Config) datedCarryP5Decision {
	decision := datedCarryP5Decision{
		VenueID: "north", Desk: "dated_term_carry_allocator_1", ClientID: 9,
		PolicyVersion: p5DatedCarryPolicyVersion, DecisionTime: 20_000_000_000,
		Enabled: true, TradeEnabled: policy.TradeEnabled, Subscribed: true, State: "IDLE", SpotSymbol: policy.SpotSymbol,
		FutureSymbol: "ABC-FUT", ListedNano: 1_000_000_000, ExpiryNano: 1_000_000_000 + policy.TargetTenor,
		OriginalTenorNanos: policy.TargetTenor, TimeToExpiryNanos: policy.TargetTenor - 19_000_000_000,
		HasSpotBook: true, SpotPublishedAt: 19_000_000_000, SpotSequence: 2,
		HasSpotBid: true, SpotBid: 9_990_000_000, SpotBidQty: 20_000_000,
		HasSpotAsk: true, SpotAsk: 10_000_000_000, SpotAskQty: 20_000_000,
		HasFutureBook: true, FuturePublishedAt: 19_000_000_000, FutureSequence: 3,
		HasFutureBid: true, FutureBid: 10_100_000_000, FutureBidQty: 20_000_000,
		HasFutureAsk: true, FutureAsk: 10_110_000_000, FutureAskQty: 20_000_000,
		SpotAgeNanos: 1_000_000_000, FutureAgeNanos: 1_000_000_000,
	}
	financials, err := recomputeP5Financials(policy, decision)
	if err != nil {
		panic(err)
	}
	decision.Direction, decision.FinancingDirection = financials.direction, financials.financingDirection
	decision.SpotExecutionReference, decision.FutureExecutionReference = financials.spotReference, financials.futureReference
	decision.GrossLockedSpreadRaw, decision.GrossLockedBpsNumerator = financials.grossSpread.String(), financials.gross.String()
	decision.ExecutionFeeBpsNumerator, decision.FinancingBpsNumerator = financials.fees.String(), financials.financing.String()
	decision.BalanceSheetBpsNumerator, decision.MarginRiskBpsNumerator = financials.balance.String(), financials.margin.String()
	decision.LegRiskBpsNumerator, decision.SettlementMismatchNumerator = financials.leg.String(), financials.settlement.String()
	decision.PostSettlementExitNumerator, decision.NetCarryBpsNumerator = financials.exit.String(), financials.net.String()
	decision.MinimumNetBpsNumerator, decision.RationalDenominator = financials.minimum.String(), financials.denominator.String()
	direction := int64(1)
	if financials.direction == "CHEAP_FUTURE" {
		direction = -1
	}
	decision.ProposedTargetSpot, decision.ProposedTargetFuture = direction*policy.MaxPosition, -direction*policy.MaxPosition
	if policy.TradeEnabled {
		postOnly := false
		decision.Action, decision.State = "SUBMIT_ENTRY_SPOT_IOC", "ENTRY_SPOT"
		decision.TargetSpot, decision.TargetFuture, decision.TargetChangedAt = decision.ProposedTargetSpot, decision.ProposedTargetFuture, decision.DecisionTime
		decision.Leg, decision.Side = "ENTRY_SPOT_IOC", exchange.Buy.String()
		if direction < 0 {
			decision.Side = exchange.Sell.String()
		}
		decision.OrderType, decision.TimeInForce, decision.PostOnly = exchange.LimitOrder.String(), exchange.IOC.String(), &postOnly
		decision.LimitPrice, decision.RequestedQty, decision.RequestID = financials.spotReference, policy.LotQty, 42
	} else {
		decision.Action = "SHADOW_ELIGIBLE"
	}
	return decision
}

func TestP5FinancialReplayCatchesEveryActorAttestationMutation(t *testing.T) {
	policy := p5TestPolicy(false)
	decision := p5TestCandidate(policy)
	financials, err := recomputeP5Financials(policy, decision)
	if err != nil || !financials.eligible {
		t.Fatalf("independent candidate = %+v, %v", financials, err)
	}
	if err := validateP5FinancialAttestation(decision, financials); err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*datedCarryP5Decision)
	}{
		{"direction", func(d *datedCarryP5Decision) { d.Direction = "CHEAP_FUTURE" }},
		{"gross", func(d *datedCarryP5Decision) { d.GrossLockedBpsNumerator = "0" }},
		{"fee", func(d *datedCarryP5Decision) { d.ExecutionFeeBpsNumerator = "0" }},
		{"financing", func(d *datedCarryP5Decision) { d.FinancingBpsNumerator = "0" }},
		{"margin", func(d *datedCarryP5Decision) { d.MarginRiskBpsNumerator = "0" }},
		{"leg risk", func(d *datedCarryP5Decision) { d.LegRiskBpsNumerator = "0" }},
		{"settlement risk", func(d *datedCarryP5Decision) { d.SettlementMismatchNumerator = "0" }},
		{"exit cost", func(d *datedCarryP5Decision) { d.PostSettlementExitNumerator = "0" }},
		{"net", func(d *datedCarryP5Decision) { d.NetCarryBpsNumerator = "999" }},
		{"denominator", func(d *datedCarryP5Decision) { d.RationalDenominator = "1" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			mutant := decision
			tc.mutate(&mutant)
			if err := validateP5FinancialAttestation(mutant, financials); err == nil {
				t.Fatal("forged actor arithmetic survived")
			}
		})
	}
}

func TestP5ShadowAndActivePoliciesHaveOneAuthorityDifference(t *testing.T) {
	shadowPolicy := p5TestPolicy(false)
	shadow := p5TestCandidate(shadowPolicy)
	financials, err := recomputeP5Financials(shadowPolicy, shadow)
	if err != nil || !financials.eligible || validateP5CandidatePolicy(shadowPolicy, shadow, financials) != nil {
		t.Fatalf("shadow candidate invalid: %v", err)
	}
	if shadow.TargetSpot != 0 || shadow.TargetFuture != 0 || shadow.RequestID != 0 {
		t.Fatal("shadow obtained trading authority")
	}

	activePolicy := p5TestPolicy(true)
	active := p5TestCandidate(activePolicy)
	financials, err = recomputeP5Financials(activePolicy, active)
	if err != nil || validateP5CandidatePolicy(activePolicy, active, financials) != nil {
		t.Fatalf("active candidate invalid: %v", err)
	}
	active.RequestID = 0
	if err := validateP5CandidatePolicy(activePolicy, active, financials); err == nil {
		t.Fatal("target without ordinary request survived")
	}
}

func TestP5OrderChainRequiresExactGatewayVenueAndActorEvidence(t *testing.T) {
	policy := p5TestPolicy(true)
	decision := p5TestCandidate(policy)
	gateway := fundingCarryGatewayDecision{
		clientID: decision.ClientID, linkID: 2, requestID: decision.RequestID, symbol: decision.SpotSymbol,
		decisionAt: decision.DecisionTime, price: decision.LimitPrice, qty: decision.RequestedQty,
		side: uint8(exchange.Buy), orderType: uint8(exchange.LimitOrder), tif: uint8(exchange.IOC),
	}
	if !p5GatewayMatches(decision, gateway) {
		t.Fatal("exact gateway decision rejected")
	}
	order := fundingCarryVenueOrder{
		RequestID: decision.RequestID, OrderID: 77, Side: decision.Side, Type: exchange.LimitOrder.String(),
		TimeInForce: exchange.IOC.String(), PostOnly: false, Price: decision.LimitPrice, Qty: decision.RequestedQty,
	}
	if !p5VenueOrderMatches(decision, order) {
		t.Fatal("exact venue admission rejected")
	}
	fill := fundingCarryVenueFill{OrderID: 77, Symbol: decision.SpotSymbol, Side: decision.Side, Qty: decision.RequestedQty, Price: decision.LimitPrice, TradeID: 88, FeeAmount: 5, FeeAsset: "USD"}
	outcomes := []datedCarryP5Outcome{
		{Event: "ORDER_ACCEPTED", OrderID: 77},
		{Event: "ORDER_FILL", OrderID: 77, Symbol: fill.Symbol, Side: fill.Side, Qty: fill.Qty, Price: fill.Price, TradeID: fill.TradeID, FeeAmount: fill.FeeAmount, FeeAsset: fill.FeeAsset},
	}
	if !p5ActorAccepted(outcomes, 77) || !p5ActorFillMatches(outcomes, fill) {
		t.Fatal("exact actor response evidence rejected")
	}
	mutations := []struct {
		name   string
		mutate func(*fundingCarryGatewayDecision)
	}{
		{"wrong symbol", func(v *fundingCarryGatewayDecision) { v.symbol = decision.FutureSymbol }},
		{"reversed side", func(v *fundingCarryGatewayDecision) { v.side = uint8(exchange.Sell) }},
		{"future decision", func(v *fundingCarryGatewayDecision) { v.decisionAt++ }},
		{"atomic quantity substitution", func(v *fundingCarryGatewayDecision) { v.qty++ }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			mutant := gateway
			tc.mutate(&mutant)
			if p5GatewayMatches(decision, mutant) {
				t.Fatal("gateway mutation survived")
			}
		})
	}
	outcomes[1].TradeID++
	if p5ActorFillMatches(outcomes, fill) {
		t.Fatal("forged actor fill survived")
	}
}

func TestP5ReceiptJoinRejectsFutureWrongAndAmbiguousSources(t *testing.T) {
	var digest [16]byte
	digest[0] = 7
	var firstFingerprint, secondFingerprint [16]byte
	firstFingerprint[0], secondFingerprint[0] = 1, 2
	decision := datedCarryP5Decision{
		VenueID: "north", ClientID: 9, DecisionTime: 100,
		DecisionFrontierLinkID: 2, DecisionFrontierOrdinal: 3, DecisionFrontierDeliveredAt: 90,
		DecisionFrontierDigest: hex.EncodeToString(digest[:]),
	}
	key := p5EvidenceKey{9, 2, uint8(exchange.MDInstrument), "_instruments", 0, 50}
	evidence := &p5Evidence{
		sources: map[p5EvidenceKey][]observationRecord{key: {
			{clientID: 9, linkID: 2, ordinal: 2, publishedAt: 50, deliveredAt: 80, fingerprint: firstFingerprint},
			{clientID: 9, linkID: 2, ordinal: 3, publishedAt: 50, deliveredAt: 90, fingerprint: secondFingerprint},
		}},
		frontiers: map[fundingCarryReceiptKey]auditedFrontier{{client: 9, link: 2, ordinal: 3}: {ordinal: 3, deliveredAt: 90, digest: digest}},
		linkRoles: map[uint32]string{2: "dated_term_carry_allocator"}, linkVenues: map[uint32]string{2: "north"},
	}
	if err := p5SourceInFrontier(decision, evidence, exchange.MDInstrument, "_instruments", 0, 50, hex.EncodeToString(firstFingerprint[:])); err != nil {
		t.Fatalf("exact fingerprint did not disambiguate replay: %v", err)
	}
	if err := p5SourceInFrontier(decision, evidence, exchange.MDInstrument, "_instruments", 0, 50, ""); err == nil {
		t.Fatal("ambiguous source survived")
	}
	decision.DecisionTime = 79
	if err := p5SourceInFrontier(decision, evidence, exchange.MDInstrument, "_instruments", 0, 50, hex.EncodeToString(firstFingerprint[:])); err == nil {
		t.Fatal("future delivery survived")
	}
}
