package exchange

import (
	"errors"
	"reflect"
	"testing"

	"exchange_sim/instrument"
)

type fundingEndowmentAppendFunc func(FundingReserveEndowment, VenueBalanceEvent) (FundingEndowmentReceipt, error)

func (appendEndowment fundingEndowmentAppendFunc) AppendFundingEndowment(event FundingReserveEndowment, movement VenueBalanceEvent) (FundingEndowmentReceipt, error) {
	return appendEndowment(event, movement)
}

func fundingReserveTestEndowment() FundingReserveEndowment {
	return FundingReserveEndowment{VenueID: "N", PerpSymbol: "ABC-PERP", QuoteAsset: "USD",
		SpotSymbol: "ABC-USD", TimestampNano: 10, AccountCap: 4, RegisteredClientIDs: []uint64{4, 2, 1, 3},
		InitialQuoteAtoms: 4, RateUnitsPerBp: 1_000_000,
		SourceID: "E2_ROUNDING_RESERVE_ENDOWMENT",
		Calendar: instrument.FundingCalendar{EpochNano: 10, IntervalSeconds: 300},
		RateContract: instrument.WindowedFundingRateContract{SampleCount: 60,
			SampleSpacingNano: 1_000_000_000, PremiumWeightNumerator: 1,
			PremiumWeightDenominator: 1, MaxAbsRateBps: 75,
			NormalizationSeconds: 28_800, RateUnitsPerBp: 1_000_000}}
}

func TestFundingReserveEndowmentIsFiniteExternalMoneyNotFeeRevenue(t *testing.T) {
	ex, _ := fundingBookPairFixture(t, true)
	endowment := fundingReserveTestEndowment()
	appended := 0
	journal := fundingEndowmentAppendFunc(func(event FundingReserveEndowment, movement VenueBalanceEvent) (FundingEndowmentReceipt, error) {
		appended++
		if !reflect.DeepEqual(event.RegisteredClientIDs, []uint64{1, 2, 3, 4}) || event.InitialQuoteAtoms != 4 {
			t.Fatalf("noncanonical endowment: %+v", event)
		}
		if movement.Bucket != VenueFundingRoundingReserve || movement.Symbol != "ABC-PERP" ||
			movement.Asset != "USD" || movement.Reason != "external_endowment" ||
			movement.Sequence != 1 || movement.OldBalance != 0 || movement.NewBalance != 4 || movement.Delta != 4 {
			t.Fatalf("external endowment has wrong required movement: %+v", movement)
		}
		return FundingEndowmentReceipt{EndowmentEventSeq: 17, MovementEventSeq: 18}, nil
	})
	if err := ex.EndowFundingRoundingReserve(endowment, journal); err != nil {
		t.Fatal(err)
	}
	reserve := ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"]
	if reserve != (FundingReserveBalance{Asset: "USD", SourceID: endowment.SourceID, Initial: 4, Balance: 4,
		EndowmentEventSeq: 17, EndowmentMovementEventSeq: 18}) ||
		ex.ExchangeBalance.FeeRevenue["USD"] != 0 || ex.ExchangeBalance.InsuranceFund["USD"] != 0 ||
		ex.VenueBalanceSequenceForReport() != 1 || appended != 1 ||
		len(ex.fundingStates["ABC-PERP"].remainders) != 4 {
		t.Fatalf("reserve was mixed with fee revenue or roster state: reserve=%+v state=%+v", reserve, ex.fundingStates["ABC-PERP"])
	}
	if violations := ex.VerifyConservation(); len(violations) != 0 {
		t.Fatalf("recorded external endowment did not reconcile: %+v", violations)
	}
	// The endowment has a required journal and needs no optional logger.
	if err := ex.EndowFundingRoundingReserve(endowment, journal); err == nil || appended != 1 ||
		ex.ExchangeBalance.FundingRoundingReserves["ABC-PERP"].Balance != 4 {
		t.Fatalf("duplicate endowment/top-up was accepted: calls=%d err=%v", appended, err)
	}
	endowment.RegisteredClientIDs[0] = 99
	if ex.fundingStates["ABC-PERP"].roster[3] != 4 {
		t.Fatal("caller mutated the frozen roster")
	}
	newSession := ex.ConnectNewClient(5, map[string]int64{"USD": 100}, &FixedFee{})
	if newSession == nil || newSession.IsRunning() || len(ex.Clients) != 4 || ex.Clients[5] != nil {
		t.Fatal("new account entered after the endowment froze four identities")
	}
}

func TestFundingReserveEndowmentAtConstructionTimeZero(t *testing.T) {
	clock := &fundingSourceClock{now: 0}
	ex := NewExchangeWithConfig(ExchangeConfig{ID: "N", Clock: clock})
	t.Cleanup(ex.Shutdown)
	ex.AddInstrument(NewSpotInstrument("ABC-USD", "ABC", "USD", 1, 1, 1, 1))
	ex.AddInstrument(NewPerpFutures("ABC-PERP", "ABC", "USD", 1, 1, 1, 1))
	for clientID := uint64(1); clientID <= 4; clientID++ {
		ex.ConnectNewClient(clientID, nil, &FixedFee{})
	}
	endowment := fundingReserveTestEndowment()
	endowment.TimestampNano = 0
	journal := fundingEndowmentAppendFunc(func(record FundingReserveEndowment, movement VenueBalanceEvent) (FundingEndowmentReceipt, error) {
		if record.TimestampNano != 0 || movement.Timestamp != 0 {
			t.Fatalf("construction-time evidence lost zero timestamp: %+v %+v", record, movement)
		}
		return FundingEndowmentReceipt{EndowmentEventSeq: 1, MovementEventSeq: 2}, nil
	})
	if err := ex.EndowFundingRoundingReserve(endowment, journal); err != nil {
		t.Fatal(err)
	}
}

func TestFundingReserveEndowmentRejectsUnderfundingAndJournalFailure(t *testing.T) {
	wantWriterError := errors.New("required endowment journal failed")
	for _, test := range []struct {
		name      string
		mutate    func(*FundingReserveEndowment)
		writerErr error
		zeroSeq   bool
		reversed  bool
	}{
		{"underfunded-three-for-four", func(event *FundingReserveEndowment) { event.InitialQuoteAtoms = 3 }, nil, false, false},
		{"incomplete-roster", func(event *FundingReserveEndowment) { event.RegisteredClientIDs[0] = 99 }, nil, false, false},
		{"missing-source", func(event *FundingReserveEndowment) { event.SourceID = "" }, nil, false, false},
		{"undeclared-source", func(event *FundingReserveEndowment) { event.SourceID = "OTHER" }, nil, false, false},
		{"rate-precision-not-endowed", func(event *FundingReserveEndowment) { event.RateContract.RateUnitsPerBp = 10 }, nil, false, false},
		{"invalid-calendar", func(event *FundingReserveEndowment) { event.Calendar.PhaseSeconds = event.Calendar.IntervalSeconds }, nil, false, false},
		{"unbound-spot", func(event *FundingReserveEndowment) { event.SpotSymbol = "OTHER-USD" }, nil, false, false},
		{"journal-failure", nil, wantWriterError, false, false},
		{"zero-canonical-sequence", nil, nil, true, false},
		{"reversed-canonical-sequence", nil, nil, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ex, _ := fundingBookPairFixture(t, true)
			endowment := fundingReserveTestEndowment()
			if test.mutate != nil {
				test.mutate(&endowment)
			}
			calls := 0
			journal := fundingEndowmentAppendFunc(func(FundingReserveEndowment, VenueBalanceEvent) (FundingEndowmentReceipt, error) {
				calls++
				if test.zeroSeq {
					return FundingEndowmentReceipt{}, nil
				}
				if test.reversed {
					return FundingEndowmentReceipt{EndowmentEventSeq: 18, MovementEventSeq: 17}, nil
				}
				return FundingEndowmentReceipt{EndowmentEventSeq: 17, MovementEventSeq: 18}, test.writerErr
			})
			err := ex.EndowFundingRoundingReserve(endowment, journal)
			if err == nil || test.writerErr != nil && !errors.Is(err, test.writerErr) ||
				len(ex.ExchangeBalance.FundingRoundingReserves) != 0 || len(ex.fundingStates) != 0 ||
				ex.VenueBalanceSequenceForReport() != 0 || len(ex.VerifyConservation()) != 0 {
				t.Fatalf("failed endowment became usable: calls=%d err=%v", calls, err)
			}
			if test.writerErr == nil && !test.zeroSeq && !test.reversed && calls != 0 || (test.writerErr != nil || test.zeroSeq || test.reversed) && calls != 1 {
				t.Fatalf("required journal called at wrong point: calls=%d", calls)
			}
			if test.writerErr != nil || test.zeroSeq || test.reversed {
				if retryErr := ex.EndowFundingRoundingReserve(endowment, journal); retryErr == nil || calls != 1 {
					t.Fatalf("failed possibly committed endowment was retried: calls=%d err=%v", calls, retryErr)
				}
			}
		})
	}
}
