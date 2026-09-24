package analysis

import (
	"testing"

	etypes "exchange_sim/types"
)

func crossVenueAccountFixture() (Report, []CrossVenueExchangeFill) {
	row := func(venue, phase string, client uint64, timestamp, abc, usd int64) AccountRow {
		return AccountRow{VenueID: venue, ClientID: client, Role: "cross_venue_router_tier_1", Phase: phase,
			Account: Account{Timestamp: timestamp, SpotBalances: []Balance{
				{Asset: "ABC", Free: abc, NetAsset: abc},
				{Asset: "USD", Free: usd, NetAsset: usd},
			}},
		}
	}
	report := Report{
		InitialAccounts:  []AccountRow{row("north", "initial", 7, 0, 100, 1000), row("south", "initial", 8, 0, 100, 1000)},
		TerminalAccounts: []AccountRow{row("north", "terminal_post_mark", 7, 20, 105, 499), row("south", "terminal_post_mark", 8, 20, 95, 1549)},
	}
	fills := []CrossVenueExchangeFill{
		{Event: Event{VenueID: "north", ClientID: 7}, OrderID: 3, TradeID: 0, Symbol: "ABC/USD", Side: "BUY", Qty: 5, Price: 100, FeeAmount: 1, FeeAsset: "USD", Role: "taker"},
		{Event: Event{VenueID: "south", ClientID: 8}, OrderID: 4, TradeID: 0, Symbol: "ABC/USD", Side: "SELL", Qty: 5, Price: 110, FeeAmount: 1, FeeAsset: "USD", Role: "taker"},
	}
	return report, fills
}

func TestCrossVenueRouterAccountsReconcileSegregatedCashAndInventory(t *testing.T) {
	report, fills := crossVenueAccountFixture()
	clients := map[string]uint64{"north": 7, "south": 8}
	got, err := ReconcileCrossVenueRouterAccounts(report, [2]string{"north", "south"}, clients, fills, CrossVenueAccountConvention{
		Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, TakerFeeBps: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["north"].BaseDelta != 5 || got["north"].QuoteDelta != -501 || got["south"].BaseDelta != -5 || got["south"].QuoteDelta != 549 || got["north"].QuoteFees != 1 || got["south"].QuoteFees != 1 {
		t.Fatalf("venue-local account deltas = %#v", got)
	}
}

func TestCrossVenueRouterAccountConventionIsNotHardwiredToABCUSD(t *testing.T) {
	report, fills := crossVenueAccountFixture()
	for _, rows := range [][]AccountRow{report.InitialAccounts, report.TerminalAccounts} {
		for index := range rows {
			rows[index].Account.SpotBalances[0].Asset = "XYZ"
			rows[index].Account.SpotBalances[1].Asset = "EUR"
		}
	}
	for index := range fills {
		fills[index].Symbol = "XYZ/EUR"
		fills[index].FeeAsset = "EUR"
	}
	clients := map[string]uint64{"north": 7, "south": 8}
	if _, err := ReconcileCrossVenueRouterAccounts(report, [2]string{"north", "south"}, clients, fills, CrossVenueAccountConvention{
		Symbol: "XYZ/EUR", BaseAsset: "XYZ", QuoteAsset: "EUR", BasePrecision: 1, TakerFeeBps: 20,
	}); err != nil {
		t.Fatalf("generic base/quote convention rejected: %v", err)
	}
}

func TestCrossVenueLocalCloseoutCanErasePositiveMatchedCashflow(t *testing.T) {
	report, fills := crossVenueAccountFixture()
	clients := map[string]uint64{"north": 7, "south": 8}
	deltas, err := ReconcileCrossVenueRouterAccounts(report, [2]string{"north", "south"}, clients, fills, CrossVenueAccountConvention{
		Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, TakerFeeBps: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	matchedCashflow := deltas["north"].QuoteDelta + deltas["south"].QuoteDelta
	if matchedCashflow != 48 {
		t.Fatalf("matched quote cashflow = %d, want 48", matchedCashflow)
	}
	north := ValueVenueLocalInventory(VenueLocalCloseoutInput{
		BaseDelta: deltas["north"].BaseDelta, QuoteDelta: deltas["north"].QuoteDelta,
		BasePrecision: 1, TakerFeeBps: 20, Bids: []etypes.PriceLevel{{Price: 99, VisibleQty: 5}},
	})
	south := ValueVenueLocalInventory(VenueLocalCloseoutInput{
		BaseDelta: deltas["south"].BaseDelta, QuoteDelta: deltas["south"].QuoteDelta,
		BasePrecision: 1, TakerFeeBps: 20, Asks: []etypes.PriceLevel{{Price: 111, VisibleQty: 5}},
	})
	final, err := SumVenueLocalCloseouts(map[string]VenueLocalCloseout{"north": north, "south": south})
	if err != nil || final != -13 {
		t.Fatalf("local liquidation value = %d, %v; want -13 despite positive matched cashflow", final, err)
	}
}

func TestCrossVenueRouterAccountsRejectFinancingAndUnexplainedChanges(t *testing.T) {
	clients := map[string]uint64{"north": 7, "south": 8}
	convention := CrossVenueAccountConvention{Symbol: "ABC/USD", BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, TakerFeeBps: 20}
	for _, test := range []struct {
		name   string
		mutate func(*Report, []CrossVenueExchangeFill)
	}{
		{"fee", func(_ *Report, fills []CrossVenueExchangeFill) { fills[0].FeeAmount++ }},
		{"terminal-cash", func(report *Report, _ []CrossVenueExchangeFill) {
			report.TerminalAccounts[0].Account.SpotBalances[1].Free++
			report.TerminalAccounts[0].Account.SpotBalances[1].NetAsset++
		}},
		{"borrow", func(report *Report, _ []CrossVenueExchangeFill) {
			report.TerminalAccounts[0].Account.Borrowed = map[string]int64{"USD": 1}
		}},
		{"locked-inconsistent", func(report *Report, _ []CrossVenueExchangeFill) {
			report.TerminalAccounts[0].Account.SpotBalances[0].Locked = 1
		}},
		{"locked-consistent", func(report *Report, _ []CrossVenueExchangeFill) {
			balance := &report.TerminalAccounts[0].Account.SpotBalances[0]
			balance.Free--
			balance.Locked++
		}},
		{"undeclared-asset", func(report *Report, _ []CrossVenueExchangeFill) {
			report.TerminalAccounts[0].Account.SpotBalances = append(report.TerminalAccounts[0].Account.SpotBalances, Balance{Asset: "CDF", Free: 1, NetAsset: 1})
		}},
		{"missing-account", func(report *Report, _ []CrossVenueExchangeFill) { report.InitialAccounts = report.InitialAccounts[:1] }},
		{"unsettled-fill", func(_ *Report, fills []CrossVenueExchangeFill) { fills[1].Qty-- }},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, fills := crossVenueAccountFixture()
			test.mutate(&report, fills)
			if _, err := ReconcileCrossVenueRouterAccounts(report, [2]string{"north", "south"}, clients, fills, convention); err == nil {
				t.Fatal("unexplained account change accepted")
			}
		})
	}
}
