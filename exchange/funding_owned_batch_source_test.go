package exchange

import (
	"reflect"
	"slices"
	"testing"

	"exchange_sim/instrument"
)

func fundingOwnedBatchTerms(t *testing.T) instrument.FundingSettlementTerms {
	t.Helper()
	rate, err := instrument.NewQuantizedFundingRate(1, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	return instrument.FundingSettlementTerms{Rate: rate, NotionalMarkPrice: 100}
}

func endowedFundingBatchFixture(t *testing.T) (*DefaultExchange, *fundingSourceClock) {
	t.Helper()
	ex, clock := fundingBookPairFixture(t, true)
	journal := fundingEndowmentAppendFunc(func(FundingReserveEndowment, VenueBalanceEvent) (FundingEndowmentReceipt, error) {
		return FundingEndowmentReceipt{EndowmentEventSeq: 17, MovementEventSeq: 18}, nil
	})
	if err := ex.EndowFundingRoundingReserve(fundingReserveTestEndowment(), journal); err != nil {
		t.Fatal(err)
	}
	return ex, clock
}

func TestCaptureOwnedFundingBatchInputJoinsLiveCashPositionsFractionsAndFiniteReserve(t *testing.T) {
	ex, clock := endowedFundingBatchFixture(t)
	clock.now = 15
	fill := ex.PlaceOrder(1, &OrderRequest{Symbol: "ABC-PERP", Side: Buy,
		Type: LimitOrder, Price: 105, Qty: 1, TimeInForce: GTC})
	if !fill.Success {
		t.Fatalf("perp fixture trade rejected: %v", fill.Error)
	}
	clock.now = 20
	request := fundingAccountSourceRequest()
	request.ExpectedClientIDs = []uint64{4, 2, 1, 3}
	input, err := ex.CaptureOwnedFundingBatchInput(request, fundingOwnedBatchTerms(t))
	if err != nil {
		t.Fatal(err)
	}
	if input.VenueID != "N" || input.Symbol != "ABC-PERP" || input.QuoteAsset != "USD" ||
		input.BasePrecision != 1 || input.InitialRoundingReserve != 4 || input.CurrentRoundingReserve != 4 ||
		!slices.Equal(input.RegisteredClientIDs, []uint64{1, 2, 3, 4}) || len(input.Accounts) != 4 {
		t.Fatalf("owned source omitted identity, frozen roster or finite money: %+v", input)
	}
	for index, account := range input.Accounts {
		if account.ClientID != uint64(index+1) || account.PerpCash != 100_000 ||
			!reflect.DeepEqual(account.Accrual, ex.fundingStates["ABC-PERP"].remainders[account.ClientID]) {
			t.Fatalf("cash or owned fraction disagrees with venue source: %+v", account)
		}
	}
	if input.Accounts[0].NetPosition != 1 || input.Accounts[3].NetPosition != -1 ||
		input.Accounts[1].NetPosition != 0 || input.Accounts[2].NetPosition != 0 {
		t.Fatalf("the signed trade or flat accounts disappeared: %+v", input.Accounts)
	}
	if _, err := PreviewFundingBatch(input); err != nil {
		t.Fatalf("source-attested account state did not pass pure preview: %v", err)
	}
	want := input
	want.RegisteredClientIDs = slices.Clone(input.RegisteredClientIDs)
	want.Accounts = slices.Clone(input.Accounts)
	input.RegisteredClientIDs[0] = 99
	input.Accounts[0].PerpCash = 0
	input.Accounts[0].Accrual.RemainderNumerator = "5"
	again, err := ex.CaptureOwnedFundingBatchInput(request, fundingOwnedBatchTerms(t))
	if err != nil || !reflect.DeepEqual(again, want) || !slices.Equal(request.ExpectedClientIDs, []uint64{4, 2, 1, 3}) {
		t.Fatalf("caller mutated the owned source or request: %+v, %v", again, err)
	}
}

func TestCaptureOwnedFundingBatchInputFailsClosedOnMissingOrMismatchedOwnership(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*DefaultExchange, *fundingSourceClock, *FundingAccountSnapshotRequest, *instrument.FundingSettlementTerms)
	}{
		{"wrong-clock", func(_ *DefaultExchange, _ *fundingSourceClock, request *FundingAccountSnapshotRequest, _ *instrument.FundingSettlementTerms) {
			request.TimestampNano--
		}},
		{"wrong-roster", func(_ *DefaultExchange, _ *fundingSourceClock, request *FundingAccountSnapshotRequest, _ *instrument.FundingSettlementTerms) {
			request.ExpectedClientIDs = []uint64{1, 2, 3, 5}
		}},
		{"missing-owned-fraction", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest, _ *instrument.FundingSettlementTerms) {
			delete(ex.fundingStates["ABC-PERP"].remainders, 4)
		}},
		{"changed-rate-scale", func(_ *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest, terms *instrument.FundingSettlementTerms) {
			terms.Rate, _ = instrument.NewQuantizedFundingRate(1, 100)
		}},
		{"wrong-reserve-asset", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest, _ *instrument.FundingSettlementTerms) {
			reserve := ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"]
			reserve.Asset = "ABC"
			ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"] = reserve
		}},
		{"changed-initial-endowment", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest, _ *instrument.FundingSettlementTerms) {
			reserve := ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"]
			reserve.Initial = 5
			ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"] = reserve
		}},
		{"orphan-closed-position", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest, _ *instrument.FundingSettlementTerms) {
			ex.Positions.UpdatePosition(99, "ABC-PERP", 1, 100, Buy, PositionBoth)
			ex.Positions.UpdatePosition(99, "ABC-PERP", 1, 100, Sell, PositionBoth)
		}},
		{"negative-wallet", func(ex *DefaultExchange, _ *fundingSourceClock, _ *FundingAccountSnapshotRequest, _ *instrument.FundingSettlementTerms) {
			ex.Clients[1].PerpBalances["USD"] = -1
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ex, clock := endowedFundingBatchFixture(t)
			clock.now = 20
			request := fundingAccountSourceRequest()
			terms := fundingOwnedBatchTerms(t)
			test.mutate(ex, clock, &request, &terms)
			input, err := ex.CaptureOwnedFundingBatchInput(request, terms)
			if err == nil || !reflect.DeepEqual(input, FundingBatchInput{}) {
				t.Fatalf("invalid owned source yielded preview input: %+v, %v", input, err)
			}
		})
	}
}

func TestCaptureOwnedFundingBatchInputRequiresSuccessfulEndowment(t *testing.T) {
	ex, clock := fundingBookPairFixture(t, true)
	clock.now = 20
	if input, err := ex.CaptureOwnedFundingBatchInput(fundingAccountSourceRequest(), fundingOwnedBatchTerms(t)); err == nil || !reflect.DeepEqual(input, FundingBatchInput{}) {
		t.Fatalf("unendowed venue yielded preview input: %+v, %v", input, err)
	}
}
