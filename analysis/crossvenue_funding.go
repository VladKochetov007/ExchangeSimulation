package analysis

import (
	"fmt"

	etypes "exchange_sim/types"
)

type CrossVenueQuoteFunding struct {
	Generation           uint64
	BuyVenue             string
	SellVenue            string
	BuyQuoteRequired     int64
	BuyQuoteAvailable    int64
	SellBaseRequired     int64
	SellBaseAvailable    int64
	SufficientAtDecision bool
}

// AssessCrossVenueFirstAttemptFunding tests only the first router decision's
// displayed-touch cash requirement against audited initial venue accounts.
// It does not predict book depth, fee or balance at later venue arrival.
func AssessCrossVenueFirstAttemptFunding(evaluations []CrossVenueEvaluationRecord, timeline *CrossVenueOpportunityTimeline, accounts map[string]CrossVenueRouterMovementAccount, venues [2]string, lotQty, basePrecision, feeBps int64) (*CrossVenueQuoteFunding, error) {
	if timeline == nil || len(timeline.Evaluations) != len(evaluations) || len(accounts) != 2 ||
		venues[0] == "" || venues[1] == "" || venues[0] == venues[1] || lotQty <= 0 || basePrecision <= 0 || feeBps < 0 {
		return nil, fmt.Errorf("cross-venue first-attempt funding: incomplete evidence or convention")
	}
	for _, venue := range venues {
		account := accounts[venue]
		if account.VenueID != venue || account.ClientID == 0 || account.DepositFrame == 0 ||
			account.InitialBase < 0 || account.InitialQuote < 0 {
			return nil, fmt.Errorf("cross-venue first-attempt funding: unanchored initial venue account")
		}
	}
	var result *CrossVenueQuoteFunding
	for index, evaluation := range evaluations {
		row := evaluation.Payload
		observed := timeline.Evaluations[index]
		if observed.Generation != row.Generation || observed.EvaluationFrame != evaluation.Event.GlobalSequence || observed.Reason != row.Reason {
			return nil, fmt.Errorf("cross-venue first-attempt funding: evaluation/timeline mismatch")
		}
		if row.Reason != "SUBMIT" {
			continue
		}
		if result != nil || row.AttemptsUsed != 0 || row.InFlightGroupID != 0 ||
			observed.LocalStatus != "POSITIVE_EDGE" || observed.LocalBuyVenue != row.SelectedBuy ||
			observed.LocalSellVenue != row.SelectedSell || row.SelectedBuy == row.SelectedSell ||
			!containsCrossVenue(venues, row.SelectedBuy) || !containsCrossVenue(venues, row.SelectedSell) {
			return nil, fmt.Errorf("cross-venue first-attempt funding: first submitted quote is not independently anchored")
		}
		buyAccount, sellAccount := accounts[row.SelectedBuy], accounts[row.SelectedSell]
		for _, account := range []CrossVenueRouterMovementAccount{buyAccount, sellAccount} {
			if account.DepositFrame >= evaluation.Event.GlobalSequence ||
				account.FirstSettlementFrame != 0 && account.FirstSettlementFrame <= evaluation.Event.GlobalSequence {
				return nil, fmt.Errorf("cross-venue first-attempt funding: router account changed before decision")
			}
		}
		books, err := crossVenueEvaluationTouches(row.Books, venues)
		if err != nil {
			return nil, err
		}
		var ask int64
		for venueIndex, venue := range venues {
			if venue == row.SelectedBuy {
				ask = books[venueIndex].Ask
			}
		}
		if ask <= 0 {
			return nil, fmt.Errorf("cross-venue first-attempt funding: missing positive buy ask")
		}
		buyNotional, ok := etypes.TryMulDiv(lotQty, ask, basePrecision)
		if !ok {
			return nil, fmt.Errorf("cross-venue first-attempt funding: buy notional overflows")
		}
		fee, ok := etypes.TryMulBps(buyNotional, feeBps)
		if !ok {
			return nil, fmt.Errorf("cross-venue first-attempt funding: quote fee overflows")
		}
		buyRequired, ok := etypes.TryAdd(buyNotional, fee)
		if !ok {
			return nil, fmt.Errorf("cross-venue first-attempt funding: buy requirement overflows")
		}
		result = &CrossVenueQuoteFunding{
			Generation: row.Generation, BuyVenue: row.SelectedBuy, SellVenue: row.SelectedSell,
			BuyQuoteRequired: buyRequired, BuyQuoteAvailable: buyAccount.InitialQuote,
			SellBaseRequired: lotQty, SellBaseAvailable: sellAccount.InitialBase,
			SufficientAtDecision: buyAccount.InitialQuote >= buyRequired && sellAccount.InitialBase >= lotQty,
		}
	}
	return result, nil
}
