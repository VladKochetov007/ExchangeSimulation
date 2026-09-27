package exchange

import (
	"errors"
	"math"
	"reflect"
	"slices"
	"testing"

	"exchange_sim/instrument"
)

func TestFundingBatchPreviewCarriesFractionalClaimsAndFiniteReserve(t *testing.T) {
	input := fundingBatchTestInput(t)
	before := fundingBatchTestClone(input)
	first, err := PreviewFundingBatch(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("pure preview mutated source accounts or registry")
	}
	if len(first.AccountCash) != 3 || first.AccountCash[0].ClientID != 1 ||
		first.AccountCash[1].ClientID != 2 || first.AccountCash[2].ClientID != 3 {
		t.Fatalf("cash rows are not in deterministic client order: %+v", first.AccountCash)
	}
	if first.VenueID != input.VenueID || first.Symbol != input.Symbol || first.QuoteAsset != input.QuoteAsset {
		t.Fatalf("preview lost the supplied venue, symbol or quote asset: %+v", first)
	}
	if first.AccountCash[0].CashDelta != -1 || first.AccountCash[1].CashDelta != 0 ||
		first.AccountCash[2].CashDelta != 0 || first.RoundingReserveDelta != 1 ||
		first.RoundingReserveAfter != 4 {
		t.Fatalf("first transfer/residual = %+v", first)
	}
	if first.AccountCash[1].AccrualAfter.RemainderNumerator != "5000" ||
		first.AccountCash[2].AccrualAfter.RemainderNumerator != "5000" {
		t.Fatalf("first fractional short claims = %+v", first.AccountCash)
	}

	nextInput := fundingBatchNextInput(input, first)
	second, err := PreviewFundingBatch(nextInput)
	if err != nil {
		t.Fatal(err)
	}
	if second.AccountCash[0].CashDelta != -1 || second.AccountCash[1].CashDelta != 1 ||
		second.AccountCash[2].CashDelta != 1 || second.RoundingReserveDelta != -1 ||
		second.RoundingReserveAfter != 3 {
		t.Fatalf("second carried payment = %+v", second)
	}
	for _, account := range second.AccountCash {
		if account.AccrualAfter.RemainderNumerator != "0" {
			t.Fatalf("second payment retained unexpected remainder for client %d: %s", account.ClientID, account.AccrualAfter.RemainderNumerator)
		}
	}
	if second.AccountCash[0].CashDelta+second.AccountCash[1].CashDelta+
		second.AccountCash[2].CashDelta+second.RoundingReserveDelta != 0 {
		t.Fatal("customer and finite-venue cash do not reconcile")
	}
}

func TestFundingBatchPreviewKeepsFlatRemaindersAndReopenEconomics(t *testing.T) {
	input := fundingBatchTestInput(t)
	first, err := PreviewFundingBatch(input)
	if err != nil {
		t.Fatal(err)
	}
	flatInput := fundingBatchNextInput(input, first)
	for index := range flatInput.Accounts {
		flatInput.Accounts[index].NetPosition = 0
	}
	flat, err := PreviewFundingBatch(flatInput)
	if err != nil || flat.RoundingReserveAfter != 4 {
		t.Fatalf("flat registered accounts = %+v, %v", flat, err)
	}
	for _, account := range flat.AccountCash {
		if account.CashDelta != 0 || account.AccrualAfter != account.AccrualBefore {
			t.Fatalf("flat account lost its existing fractional claim: %+v", account)
		}
	}
	reopened := fundingBatchNextInput(flatInput, flat)
	for index := range reopened.Accounts {
		if reopened.Accounts[index].ClientID == 1 {
			reopened.Accounts[index].NetPosition = 2
		} else {
			reopened.Accounts[index].NetPosition = -1
		}
	}
	second, err := PreviewFundingBatch(reopened)
	if err != nil || second.AccountCash[1].CashDelta != 1 || second.AccountCash[2].CashDelta != 1 ||
		second.RoundingReserveAfter != 3 {
		t.Fatalf("reopening erased prior fractional obligation: %+v, %v", second, err)
	}
}

func TestFundingBatchPreviewClassifiesPayerCashShortfallWithoutTransfer(t *testing.T) {
	input := fundingBatchTestInput(t)
	for index := range input.Accounts {
		if input.Accounts[index].ClientID == 1 {
			input.Accounts[index].PerpCash = 0
		}
	}
	before := fundingBatchTestClone(input)
	preview, err := PreviewFundingBatch(input)
	var shortfall *FundingPayerCashShortfall
	if !errors.As(err, &shortfall) || shortfall.VenueID != "N" || shortfall.Symbol != "ABC-PERP" ||
		!slices.Equal(shortfall.ClientIDs, []uint64{1}) {
		t.Fatalf("cash shortfall classification = %+v, %v", preview, err)
	}
	if !reflect.DeepEqual(preview, FundingBatchPreview{}) || !reflect.DeepEqual(input, before) {
		t.Fatal("cash shortfall returned a committable preview or mutated input")
	}
}

func TestFundingBatchPreviewNegativeRateReportsBothPayers(t *testing.T) {
	input := fundingBatchTestInput(t)
	input.Terms.Rate = scaledFundingTestRate(t, -1, 1)
	input.Terms.NotionalMarkPrice = 10_000
	before := fundingBatchTestClone(input)
	preview, err := PreviewFundingBatch(input)
	var shortfall *FundingPayerCashShortfall
	if !errors.As(err, &shortfall) || !slices.Equal(shortfall.ClientIDs, []uint64{2, 3}) {
		t.Fatalf("negative-rate short payers = %+v, %v", preview, err)
	}
	if !reflect.DeepEqual(preview, FundingBatchPreview{}) || !reflect.DeepEqual(input, before) {
		t.Fatal("negative-rate shortfall mutated or returned a committable batch")
	}
}

func TestFundingBatchPreviewRejectsCashAndReserveOverflow(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*FundingBatchInput)
	}{
		{"cash-after", func(input *FundingBatchInput) {
			input.Terms.Rate = scaledFundingTestRate(t, -2, 1)
			input.Terms.NotionalMarkPrice = 10_000
			input.Accounts[1].PerpCash = math.MaxInt64
		}},
		{"cash-delta", func(input *FundingBatchInput) {
			input.Accounts[0].NetPosition = 0
			input.Accounts[1].NetPosition = math.MaxInt64
			input.Accounts[2].NetPosition = -math.MaxInt64
			input.Terms.NotionalMarkPrice = math.MaxInt64
		}},
		{"reserve-after", func(input *FundingBatchInput) {
			input.InitialRoundingReserve = math.MaxInt64
			input.CurrentRoundingReserve = math.MaxInt64
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := fundingBatchTestInput(t)
			test.mutate(&input)
			preview, err := PreviewFundingBatch(input)
			if err == nil || !reflect.DeepEqual(preview, FundingBatchPreview{}) {
				t.Fatalf("overflow returned a committable batch: %+v, %v", preview, err)
			}
			var shortfall *FundingPayerCashShortfall
			if errors.As(err, &shortfall) {
				t.Fatalf("overflow mislabeled as economic payer shortfall: %v", err)
			}
		})
	}
}

func TestFundingBatchPreviewRejectsUnmatchedOrMisboundState(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*FundingBatchInput)
	}{
		{"missing-account", func(input *FundingBatchInput) { input.Accounts = input.Accounts[:2] }},
		{"wrong-client", func(input *FundingBatchInput) { input.Accounts[0].ClientID = 9 }},
		{"duplicate-registry", func(input *FundingBatchInput) { input.RegisteredClientIDs[0] = 2 }},
		{"unmatched-position", func(input *FundingBatchInput) { input.Accounts[0].NetPosition++ }},
		{"corrupt-remainder", func(input *FundingBatchInput) { input.Accounts[0].Accrual.RemainderNumerator = "10000" }},
		{"different-scale", func(input *FundingBatchInput) { input.Accounts[0].Accrual.RateUnitsPerBp = 2 }},
		{"unbacked-residual", func(input *FundingBatchInput) { input.CurrentRoundingReserve++ }},
		{"underfunded-reserve", func(input *FundingBatchInput) { input.InitialRoundingReserve = 2 }},
		{"negative-start-cash", func(input *FundingBatchInput) { input.Accounts[0].PerpCash = -1 }},
		{"missing-mark", func(input *FundingBatchInput) { input.Terms.NotionalMarkPrice = 0 }},
		{"missing-quote-asset", func(input *FundingBatchInput) { input.QuoteAsset = "" }},
		{"missing-rate-scale", func(input *FundingBatchInput) { input.Terms.Rate = instrument.QuantizedFundingRate{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := fundingBatchTestInput(t)
			test.mutate(&input)
			before := fundingBatchTestClone(input)
			preview, err := PreviewFundingBatch(input)
			if err == nil || !reflect.DeepEqual(preview, FundingBatchPreview{}) || !reflect.DeepEqual(input, before) {
				t.Fatalf("invalid batch returned %+v, %v or mutated input", preview, err)
			}
			var shortfall *FundingPayerCashShortfall
			if errors.As(err, &shortfall) {
				t.Fatalf("structural defect was mislabeled economic cash shortfall: %v", err)
			}
		})
	}
}

func TestFundingBatchPreviewAcceptsEmptyRegisteredVenueWithoutMoney(t *testing.T) {
	input := fundingBatchTestInput(t)
	input.RegisteredClientIDs = nil
	input.Accounts = nil
	input.InitialRoundingReserve = 0
	input.CurrentRoundingReserve = 0
	preview, err := PreviewFundingBatch(input)
	if err != nil || len(preview.AccountCash) != 0 || preview.RoundingReserveDelta != 0 {
		t.Fatalf("empty venue funding = %+v, %v", preview, err)
	}
}

func fundingBatchTestInput(t *testing.T) FundingBatchInput {
	t.Helper()
	state, err := NewScaledFundingAccrual(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	return FundingBatchInput{
		VenueID: "N", Symbol: "ABC-PERP", QuoteAsset: "USD", BasePrecision: 1,
		Terms:                  instrument.FundingSettlementTerms{Rate: scaledFundingTestRate(t, 1, 1), NotionalMarkPrice: 5_000},
		RegisteredClientIDs:    []uint64{3, 1, 2},
		InitialRoundingReserve: 3, CurrentRoundingReserve: 3,
		Accounts: []FundingBatchAccount{
			{ClientID: 3, NetPosition: -1, PerpCash: 0, Accrual: state.Snapshot()},
			{ClientID: 1, NetPosition: 2, PerpCash: 3, Accrual: state.Snapshot()},
			{ClientID: 2, NetPosition: -1, PerpCash: 0, Accrual: state.Snapshot()},
		},
	}
}

func fundingBatchNextInput(input FundingBatchInput, preview FundingBatchPreview) FundingBatchInput {
	next := fundingBatchTestClone(input)
	next.CurrentRoundingReserve = preview.RoundingReserveAfter
	for index := range next.Accounts {
		for _, account := range preview.AccountCash {
			if next.Accounts[index].ClientID == account.ClientID {
				next.Accounts[index].PerpCash = account.CashAfter
				next.Accounts[index].Accrual = account.AccrualAfter
				break
			}
		}
	}
	return next
}

func fundingBatchTestClone(input FundingBatchInput) FundingBatchInput {
	input.RegisteredClientIDs = slices.Clone(input.RegisteredClientIDs)
	input.Accounts = slices.Clone(input.Accounts)
	return input
}
