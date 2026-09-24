package crossvenue

import (
	"encoding/json"
	"testing"

	"exchange_sim/analysis"
)

func TestME005ContractRejectsUnknownOrTrailingFields(t *testing.T) {
	contract := Contract{
		SchemaVersion: 1, Arm: "ON", Seed: 701,
		SourceRevision:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		EffectiveConfigSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Venues:                [2]string{"north", "south"}, Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD",
		HorizonNano: 100, LotQty: 5, BasePrecision: 1, TakerFeeBps: 10,
		MaxBookEvidenceAgeNanos: 10, InitialBasePerVenue: 20, InitialQuotePerVenue: 1000,
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeContract(raw); err != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}
	if _, err := DecodeContract(append(raw, []byte(` {}`)...)); err == nil {
		t.Fatal("trailing contract document accepted")
	}
	if _, err := DecodeContract(append(raw[:len(raw)-1], []byte(`,"unregistered":1}`)...)); err == nil {
		t.Fatal("unknown contract field accepted")
	}
	contract.Arm = "OFF"
	contract.InitialBasePerVenue, contract.InitialQuotePerVenue = 0, 0
	if err := contract.Validate(); err != nil {
		t.Fatalf("off arm without router endowment rejected: %v", err)
	}
}

func TestME005EconomicsSeparatesMatchedCashflowFromLocalRestoration(t *testing.T) {
	venues := [2]string{"north", "south"}
	evidence := &analysis.CrossVenueFirstAttemptEvidence{
		Accounts: map[string]analysis.CrossVenueAccountDelta{
			"north": {VenueID: "north", BaseDelta: 5, QuoteDelta: -501, QuoteFees: 1},
			"south": {VenueID: "south", BaseDelta: -5, QuoteDelta: 549, QuoteFees: 1},
		},
		Terminal:      &analysis.CrossVenueFirstAttemptTerminal{ExchangeOutcome: "MATCHED_FOK"},
		TerminalValue: analysis.CrossVenueTerminalValue{Available: true, Value: -13},
	}
	result, err := SummarizeEconomics(evidence, venues)
	if err != nil || result.ActualQuoteCashflow != 48 || result.ActualLegFees != 2 ||
		result.MatchedEdgeBeforeFees == nil || *result.MatchedEdgeBeforeFees != 50 ||
		result.MatchedEdgeAfterFees == nil || *result.MatchedEdgeAfterFees != 48 || result.GlobalResidualBaseQty != 0 ||
		result.LocalBaseDeltaByVenue["north"] != 5 || result.LocalBaseDeltaByVenue["south"] != -5 ||
		result.HypotheticalLocalRestorationFlow == nil || *result.HypotheticalLocalRestorationFlow != -61 ||
		result.FinalNetValue == nil || *result.FinalNetValue != -13 {
		t.Fatalf("matched local economics = %#v, %v", result, err)
	}
	evidence.Terminal = &analysis.CrossVenueFirstAttemptTerminal{ExchangeOutcome: "UNMATCHED_FOK"}
	evidence.Accounts["south"] = analysis.CrossVenueAccountDelta{VenueID: "south"}
	evidence.TerminalValue.Available = false
	result, err = SummarizeEconomics(evidence, venues)
	if err != nil || result.MatchedEdgeBeforeFees != nil || result.MatchedEdgeAfterFees != nil ||
		result.FinalNetValue != nil || result.GlobalResidualBaseQty != 5 {
		t.Fatalf("one-leg unpriceable economics = %#v, %v", result, err)
	}
}

func TestME005RouterSelectionAndTerminalHorizonFailClosed(t *testing.T) {
	venues := [2]string{"north", "south"}
	report := analysis.Report{
		RouterReports: []analysis.CrossVenueRouterEvidenceCounters{{RouterID: 9}},
		InitialAccounts: []analysis.AccountRow{
			{VenueID: "north", ClientID: 7, Role: "cross_venue_router_tier_1"},
			{VenueID: "south", ClientID: 8, Role: "cross_venue_router_tier_1"},
		},
		TerminalAccounts: []analysis.AccountRow{
			{VenueID: "north", ClientID: 7, Role: "cross_venue_router_tier_1", Phase: "terminal_post_mark", Account: analysis.Account{Timestamp: 100}},
			{VenueID: "south", ClientID: 8, Role: "cross_venue_router_tier_1", Phase: "terminal_post_mark", Account: analysis.Account{Timestamp: 100}},
		},
	}
	routerID, clients, err := selectedRouter(report, venues)
	if err != nil || routerID != 9 || clients["north"] != 7 || clients["south"] != 8 {
		t.Fatalf("router selection = %d %#v %v", routerID, clients, err)
	}
	if err := checkTerminalHorizon(report.TerminalAccounts, 100); err != nil {
		t.Fatal(err)
	}
	report.TerminalAccounts[0].Account.Timestamp = 99
	if err := checkTerminalHorizon(report.TerminalAccounts, 100); err == nil {
		t.Fatal("short terminal horizon accepted")
	}
	report.InitialAccounts = append(report.InitialAccounts, report.InitialAccounts[0])
	if _, _, err := selectedRouter(report, venues); err == nil {
		t.Fatal("duplicate router account accepted")
	}
	report.InitialAccounts = report.InitialAccounts[:2]
	report.TerminalAccounts[1].ClientID = 10
	if _, _, err := selectedRouter(report, venues); err == nil {
		t.Fatal("changed terminal router owner accepted")
	}
}
