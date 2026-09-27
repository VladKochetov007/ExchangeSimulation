package exchange

import (
	"fmt"
	"math/big"
	"slices"

	"exchange_sim/instrument"
)

// FundingBatchAccount is one registered venue-local perp cash account. The
// caller must include flat accounts that retain a fractional funding claim.
type FundingBatchAccount struct {
	ClientID    uint64
	NetPosition int64
	PerpCash    int64
	Accrual     ScaledFundingAccrualSnapshot
}

// FundingBatchInput expects a source-attested snapshot taken before any same-time
// order or liquidation. This pure preview cannot attest the book, position
// store, account registry or ownership of a persisted accrual snapshot.
type FundingBatchInput struct {
	VenueID                string
	Symbol                 string
	QuoteAsset             string
	BasePrecision          int64
	Terms                  instrument.FundingSettlementTerms
	RegisteredClientIDs    []uint64
	InitialRoundingReserve int64
	CurrentRoundingReserve int64
	Accounts               []FundingBatchAccount
}

type FundingAccountCashPreview struct {
	ClientID      uint64
	NetPosition   int64
	CashBefore    int64
	CashDelta     int64
	CashAfter     int64
	AccrualBefore ScaledFundingAccrualSnapshot
	AccrualAfter  ScaledFundingAccrualSnapshot
}

type FundingBatchPreview struct {
	VenueID               string
	Symbol                string
	QuoteAsset            string
	AccountCash           []FundingAccountCashPreview
	RoundingReserveBefore int64
	RoundingReserveDelta  int64
	RoundingReserveAfter  int64
}

// FundingPayerCashShortfall is an economic inability to pay from the venue-
// local perp wallet, not a malformed evidence or arithmetic failure. The
// caller must stop the E2 world without committing this venue's preview.
type FundingPayerCashShortfall struct {
	VenueID   string
	Symbol    string
	ClientIDs []uint64
}

func (failure *FundingPayerCashShortfall) Error() string {
	return fmt.Sprintf("funding payer cash shortfall at %s/%s for clients %v", failure.VenueID, failure.Symbol, failure.ClientIDs)
}

// PreviewFundingBatch preflights a complete, matched-position venue batch.
// It never mutates client cash, venue money or the supplied accrual snapshots.
// The later exchange adapter must compare the whole snapshot at commit and
// record both account and finite-reserve movements atomically.
func PreviewFundingBatch(input FundingBatchInput) (FundingBatchPreview, error) {
	if input.VenueID == "" || input.Symbol == "" || input.QuoteAsset == "" || input.BasePrecision <= 0 ||
		input.Terms.NotionalMarkPrice <= 0 ||
		len(input.Accounts) != len(input.RegisteredClientIDs) ||
		input.InitialRoundingReserve < int64(len(input.RegisteredClientIDs)) ||
		input.CurrentRoundingReserve < 0 {
		return FundingBatchPreview{}, fmt.Errorf("funding batch: invalid identity, price, registry or finite reserve")
	}
	expectedAccrual, err := NewScaledFundingAccrual(input.BasePrecision, input.Terms.Rate.UnitsPerBp())
	if err != nil {
		return FundingBatchPreview{}, fmt.Errorf("funding batch: %w", err)
	}
	accounts := slices.Clone(input.Accounts)
	registeredClientIDs := slices.Clone(input.RegisteredClientIDs)
	slices.Sort(registeredClientIDs)
	slices.SortFunc(accounts, func(left, right FundingBatchAccount) int {
		if left.ClientID < right.ClientID {
			return -1
		}
		if left.ClientID > right.ClientID {
			return 1
		}
		return 0
	})

	states := make([]ScaledFundingAccrual, len(accounts))
	netPosition := new(big.Int)
	priorRemainders := new(big.Int)
	for index, account := range accounts {
		if index > 0 && registeredClientIDs[index] == registeredClientIDs[index-1] {
			return FundingBatchPreview{}, fmt.Errorf("funding batch: duplicate registered client %d", registeredClientIDs[index])
		}
		if account.ClientID != registeredClientIDs[index] {
			return FundingBatchPreview{}, fmt.Errorf("funding batch: account client %d is not the registered client %d", account.ClientID, registeredClientIDs[index])
		}
		if account.PerpCash < 0 {
			return FundingBatchPreview{}, fmt.Errorf("funding batch: client %d begins with negative perp cash", account.ClientID)
		}
		state, restoreErr := RestoreScaledFundingAccrual(input.BasePrecision, input.Terms.Rate.UnitsPerBp(), account.Accrual)
		if restoreErr != nil {
			return FundingBatchPreview{}, fmt.Errorf("funding batch: client %d: %w", account.ClientID, restoreErr)
		}
		states[index] = state
		netPosition.Add(netPosition, big.NewInt(account.NetPosition))
		priorRemainders.Add(priorRemainders, &state.remainder)
	}
	if netPosition.Sign() != 0 {
		return FundingBatchPreview{}, fmt.Errorf("funding batch: unmatched signed perp positions %s", netPosition)
	}
	priorReserveChange := new(big.Int).Sub(big.NewInt(input.CurrentRoundingReserve), big.NewInt(input.InitialRoundingReserve))
	priorReserveChange.Mul(priorReserveChange, new(big.Int).Set(&expectedAccrual.denominator))
	if priorReserveChange.Cmp(priorRemainders) != 0 {
		return FundingBatchPreview{}, fmt.Errorf("funding batch: reserve and prior fractional claims disagree")
	}

	result := FundingBatchPreview{
		VenueID: input.VenueID, Symbol: input.Symbol, QuoteAsset: input.QuoteAsset,
		AccountCash:           make([]FundingAccountCashPreview, 0, len(accounts)),
		RoundingReserveBefore: input.CurrentRoundingReserve,
	}
	cashTotal := new(big.Int)
	nextRemainders := new(big.Int)
	shortfalls := make([]uint64, 0)
	for index, account := range accounts {
		cashDelta, next, previewErr := states[index].Preview(account.NetPosition, input.Terms.NotionalMarkPrice, input.Terms.Rate)
		if previewErr != nil {
			return FundingBatchPreview{}, fmt.Errorf("funding batch: client %d: %w", account.ClientID, previewErr)
		}
		cashAfter := new(big.Int).Add(big.NewInt(account.PerpCash), big.NewInt(cashDelta))
		if !cashAfter.IsInt64() {
			return FundingBatchPreview{}, fmt.Errorf("funding batch: client %d cash overflows int64", account.ClientID)
		}
		if cashAfter.Sign() < 0 {
			shortfalls = append(shortfalls, account.ClientID)
		}
		cashTotal.Add(cashTotal, big.NewInt(cashDelta))
		nextRemainders.Add(nextRemainders, &next.remainder)
		result.AccountCash = append(result.AccountCash, FundingAccountCashPreview{
			ClientID: account.ClientID, NetPosition: account.NetPosition,
			CashBefore: account.PerpCash, CashDelta: cashDelta, CashAfter: cashAfter.Int64(),
			AccrualBefore: account.Accrual, AccrualAfter: next.Snapshot(),
		})
	}
	reserveDelta := new(big.Int).Neg(cashTotal)
	reserveAfter := new(big.Int).Add(big.NewInt(input.CurrentRoundingReserve), reserveDelta)
	if !reserveDelta.IsInt64() || !reserveAfter.IsInt64() || reserveAfter.Sign() < 0 {
		return FundingBatchPreview{}, fmt.Errorf("funding batch: finite reserve cannot post residual")
	}
	nextReserveChange := new(big.Int).Sub(reserveAfter, big.NewInt(input.InitialRoundingReserve))
	nextReserveChange.Mul(nextReserveChange, new(big.Int).Set(&expectedAccrual.denominator))
	if nextReserveChange.Cmp(nextRemainders) != 0 {
		return FundingBatchPreview{}, fmt.Errorf("funding batch: next reserve and fractional claims disagree")
	}
	if len(shortfalls) > 0 {
		return FundingBatchPreview{}, &FundingPayerCashShortfall{VenueID: input.VenueID, Symbol: input.Symbol, ClientIDs: shortfalls}
	}
	result.RoundingReserveDelta = reserveDelta.Int64()
	result.RoundingReserveAfter = reserveAfter.Int64()
	return result, nil
}
