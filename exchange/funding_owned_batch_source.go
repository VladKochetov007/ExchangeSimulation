package exchange

import (
	"fmt"
	"slices"

	"exchange_sim/instrument"
)

// CaptureOwnedFundingBatchInput joins the venue's own frozen roster, live
// accounts, persistent fractions and finite reserve. The returned value is a
// copied preview input, not a committable plan: the caller-supplied terms and
// pre-instant phase are not attested here. A later joint preflight must bind
// those sources and compare still-locked state before either venue can post.
func (e *DefaultExchange) CaptureOwnedFundingBatchInput(request FundingAccountSnapshotRequest, terms instrument.FundingSettlementTerms) (FundingBatchInput, error) {
	if e == nil || e.Clock == nil || request.VenueID == "" || request.PerpSymbol == "" || request.TimestampNano < 0 {
		return FundingBatchInput{}, fmt.Errorf("owned funding batch: invalid venue, symbol or timestamp")
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.captureOwnedFundingBatchInputLocked(request, terms)
}

func (e *DefaultExchange) captureOwnedFundingBatchInputLocked(request FundingAccountSnapshotRequest, terms instrument.FundingSettlementTerms) (FundingBatchInput, error) {
	if terms.Rate.UnitsPerBp() <= 0 || terms.NotionalMarkPrice <= 0 {
		return FundingBatchInput{}, fmt.Errorf("owned funding batch: invalid settlement terms")
	}
	input, err := e.captureOwnedFundingBatchStateLocked(request, terms.Rate.UnitsPerBp())
	if err != nil {
		return FundingBatchInput{}, err
	}
	input.Terms = terms
	return input, nil
}

func (e *DefaultExchange) captureOwnedFundingBatchStateLocked(request FundingAccountSnapshotRequest, rateUnitsPerBp int64) (FundingBatchInput, error) {
	if e.ExchangeBalance == nil || e.Clock == nil || request.VenueID == "" || request.PerpSymbol == "" || request.TimestampNano < 0 {
		return FundingBatchInput{}, fmt.Errorf("owned funding batch: missing venue ledger, clock or request identity")
	}
	owned := e.fundingStates[request.PerpSymbol]
	reserve, hasReserve := e.ExchangeBalance.FundingRoundingReserves[request.PerpSymbol]
	if owned == nil || !hasReserve || e.fundingEndowmentFailures[request.PerpSymbol] != nil || owned.settlementFailure != nil ||
		len(owned.roster) == 0 || len(owned.remainders) != len(owned.roster) ||
		owned.basePrecision <= 0 || owned.rateUnitsPerBp <= 0 || rateUnitsPerBp != owned.rateUnitsPerBp ||
		reserve.SourceID != FundingReserveEndowmentSource || reserve.EndowmentEventSeq == 0 ||
		reserve.EndowmentMovementEventSeq <= reserve.EndowmentEventSeq ||
		reserve.Initial != int64(len(owned.roster)) || reserve.Balance < 0 {
		return FundingBatchInput{}, fmt.Errorf("owned funding batch: missing or inconsistent finite source, rate scale or reserve")
	}
	accounts, err := e.captureFundingAccountSnapshotLocked(request)
	if err != nil {
		return FundingBatchInput{}, err
	}
	if accounts.QuoteAsset != reserve.Asset || accounts.BasePrecision != owned.basePrecision ||
		len(accounts.Accounts) != len(owned.roster) {
		return FundingBatchInput{}, fmt.Errorf("owned funding batch: source account instrument or roster changed")
	}
	registered := slices.Clone(request.ExpectedClientIDs)
	slices.Sort(registered)
	if !slices.Equal(registered, owned.roster) {
		return FundingBatchInput{}, fmt.Errorf("owned funding batch: requested roster is not the endowed roster")
	}
	rate, err := instrument.NewQuantizedFundingRate(0, owned.rateUnitsPerBp)
	if err != nil {
		return FundingBatchInput{}, fmt.Errorf("owned funding batch: invalid rate precision: %w", err)
	}
	result := FundingBatchInput{
		VenueID: request.VenueID, Symbol: request.PerpSymbol,
		QuoteAsset: reserve.Asset, BasePrecision: owned.basePrecision,
		Terms: instrument.FundingSettlementTerms{Rate: rate}, RegisteredClientIDs: slices.Clone(owned.roster),
		InitialRoundingReserve: reserve.Initial, CurrentRoundingReserve: reserve.Balance,
		Accounts: make([]FundingBatchAccount, 0, len(accounts.Accounts)),
	}
	for index, account := range accounts.Accounts {
		if account.ClientID != owned.roster[index] {
			return FundingBatchInput{}, fmt.Errorf("owned funding batch: source account ordering changed")
		}
		if e.Clients[account.ClientID].PerpBalances == nil {
			return FundingBatchInput{}, fmt.Errorf("owned funding batch: client %d has no perpetual cash wallet", account.ClientID)
		}
		fraction, present := owned.remainders[account.ClientID]
		if !present {
			return FundingBatchInput{}, fmt.Errorf("owned funding batch: registered account lost its fractional state")
		}
		result.Accounts = append(result.Accounts, FundingBatchAccount{
			ClientID: account.ClientID, NetPosition: account.NetPosition,
			PerpCash: account.PerpCash, Accrual: fraction,
		})
	}
	if _, _, _, err := validateFundingBatchPrior(result); err != nil {
		return FundingBatchInput{}, fmt.Errorf("owned funding batch: invalid persistent prior state: %w", err)
	}
	return result, nil
}
