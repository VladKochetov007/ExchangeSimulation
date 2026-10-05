package exchange

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"exchange_sim/instrument"
)

type fundingPairSourceFunc func(FundingBookPairRequest) (FundingBookPair, error)

func (source fundingPairSourceFunc) CaptureFundingBookPair(request FundingBookPairRequest) (FundingBookPair, error) {
	return source(request)
}

type fundingObservationAppendFunc func(FundingBookObservation) (uint64, error)

func (appendObservation fundingObservationAppendFunc) AppendFundingObservation(observation FundingBookObservation) (uint64, error) {
	return appendObservation(observation)
}

func fundingWindowTestPair(request FundingBookPairRequest) FundingBookPair {
	quote := func(price int64) FundingVisibleQuote {
		return FundingVisibleQuote{Price: price, VisibleQty: 2,
			OldestVisibleOrderAcceptedAtNano: request.TimestampNano - 1,
			OldestVisibleOrderAgeNano:        1}
	}
	return FundingBookPair{VenueID: request.VenueID, TimestampNano: request.TimestampNano,
		BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, QuotePrecision: 1,
		Spot: FundingBookTop{Symbol: request.SpotSymbol, Bid: quote(99), Ask: quote(101), MidPrice: 100},
		Perp: FundingBookTop{Symbol: request.PerpSymbol, Bid: quote(103), Ask: quote(105), MidPrice: 104}}
}

func fundingWindowTestConfig() FundingWindowSourceConfig {
	return FundingWindowSourceConfig{VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP",
		FirstBoundaryNano: 20, SampleSpacingNano: 1, SampleCount: 2}
}

func TestFundingWindowRecorderUsesOnlyPriorCanonicalObservations(t *testing.T) {
	ex, clock := fundingBookPairFixture(t, true)
	seq := uint64(7)
	recorder, err := NewFundingWindowRecorder(fundingWindowTestConfig(), ex,
		fundingObservationAppendFunc(func(observation FundingBookObservation) (uint64, error) {
			if !observation.Available || observation.Pair == nil || observation.TimestampNano != clock.now {
				t.Fatalf("wrong live observation: %+v", observation)
			}
			seq += 2 // Dictionary or unrelated frames may occupy the intervening slots.
			return seq, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []int64{20, 21, 22} {
		clock.now = at
		if _, err := recorder.ObserveAt(at); err != nil {
			t.Fatal(err)
		}
	}
	window, err := recorder.WindowBefore(22)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(window.Samples, []instrument.FundingWindowSample{
		{TimestampNano: 20, IndexPrice: 100, MarkPrice: 104},
		{TimestampNano: 21, IndexPrice: 100, MarkPrice: 104},
	}) || len(window.Unavailable) != 0 || len(window.Sources) != 2 ||
		window.Sources[0].EventSeq != 9 || window.Sources[1].EventSeq != 11 {
		t.Fatalf("current t or noncanonical sequence entered the prior window: %+v", window)
	}
	terms, err := (instrument.WindowedFundingRateContract{SampleCount: 2, SampleSpacingNano: 1,
		PremiumWeightNumerator: 1, PremiumWeightDenominator: 1, MaxAbsRateBps: 100,
		NormalizationSeconds: 1, RateUnitsPerBp: 1_000_000}).SettlementTermsAt(22, 1, window.Samples)
	if err != nil || terms.NotionalMarkPrice != 104 {
		t.Fatalf("live public source did not produce valid prior settlement terms: %+v, %v", terms, err)
	}
	window.Sources[0].Observation.Pair.Spot.Bid.Price = 1
	secondRead, err := recorder.WindowBefore(22)
	if err != nil || secondRead.Sources[0].Observation.Pair.Spot.Bid.Price != 99 {
		t.Fatalf("caller mutated retained source evidence: %+v, %v", secondRead, err)
	}
}

func TestFundingWindowRecorderDistinguishesUnavailableFromOmitted(t *testing.T) {
	for _, unavailableAt := range []int64{21, 22} {
		t.Run(map[int64]string{21: "prior", 22: "current"}[unavailableAt], func(t *testing.T) {
			seq := uint64(0)
			recorder, err := NewFundingWindowRecorder(fundingWindowTestConfig(),
				fundingPairSourceFunc(func(request FundingBookPairRequest) (FundingBookPair, error) {
					if request.TimestampNano == unavailableAt {
						return FundingBookPair{}, FundingBookUnavailableError{Reason: FundingBookNoDisplayedSide}
					}
					return fundingWindowTestPair(request), nil
				}), fundingObservationAppendFunc(func(observation FundingBookObservation) (uint64, error) {
					if observation.TimestampNano == unavailableAt && (observation.Available || observation.Pair != nil ||
						observation.Reason != string(FundingBookNoDisplayedSide)) {
						t.Fatalf("unavailable live book was not explicit: %+v", observation)
					}
					seq++
					return seq, nil
				}))
			if err != nil {
				t.Fatal(err)
			}
			for _, at := range []int64{20, 21, 22} {
				if _, err := recorder.ObserveAt(at); err != nil {
					t.Fatal(err)
				}
			}
			window, err := recorder.WindowBefore(22)
			if err != nil {
				t.Fatal(err)
			}
			if unavailableAt == 21 && (len(window.Unavailable) != 1 || window.Samples != nil) {
				t.Fatalf("prior unavailable pair became a numeric window: %+v", window)
			}
			if unavailableAt == 22 && (len(window.Unavailable) != 0 || len(window.Samples) != 2) {
				t.Fatalf("current one-sided book cancelled complete prior window: %+v", window)
			}
		})
	}
	recorder, err := NewFundingWindowRecorder(fundingWindowTestConfig(),
		fundingPairSourceFunc(func(request FundingBookPairRequest) (FundingBookPair, error) {
			return fundingWindowTestPair(request), nil
		}), fundingObservationAppendFunc(func(FundingBookObservation) (uint64, error) { return 1, nil }))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.ObserveAt(20); err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.ObserveAt(22); err == nil {
		t.Fatal("omitted 21 observation was treated as book unavailability")
	}
	if _, err := recorder.WindowBefore(22); err == nil {
		t.Fatal("missing canonical observation produced a rate window")
	}
}

func TestFundingWindowRecorderWritesVersionedUnavailableReasonCode(t *testing.T) {
	var recorded FundingBookObservation
	recorder, err := NewFundingWindowRecorder(fundingWindowTestConfig(),
		fundingPairSourceFunc(func(FundingBookPairRequest) (FundingBookPair, error) {
			return FundingBookPair{}, FundingBookUnavailableError{Reason: FundingBookCrossedOrNonpositivePair}
		}), fundingObservationAppendFunc(func(observation FundingBookObservation) (uint64, error) {
			recorded = observation
			return 1, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.ObserveAt(20); err != nil {
		t.Fatal(err)
	}
	if recorded.Version != FundingBookObservationVersion || recorded.Available || recorded.Pair != nil ||
		recorded.Reason != string(FundingBookCrossedOrNonpositivePair) {
		t.Fatalf("unavailable source reason was not emitted in its canonical schema: %+v", recorded)
	}
}

func TestFundingWindowRecorderFailsClosedOnSourceAndEvidenceContradictions(t *testing.T) {
	for _, test := range []struct {
		name string
		pair func(FundingBookPairRequest) (FundingBookPair, error)
	}{
		{"wrong-venue", func(request FundingBookPairRequest) (FundingBookPair, error) {
			pair := fundingWindowTestPair(request)
			pair.VenueID = "S"
			return pair, nil
		}},
		{"fabricated-mid", func(request FundingBookPairRequest) (FundingBookPair, error) {
			pair := fundingWindowTestPair(request)
			pair.Perp.MidPrice = 1
			return pair, nil
		}},
		{"future-quote", func(request FundingBookPairRequest) (FundingBookPair, error) {
			pair := fundingWindowTestPair(request)
			pair.Spot.Bid.OldestVisibleOrderAcceptedAtNano = request.TimestampNano
			return pair, nil
		}},
		{"stale-pair-on-error", func(request FundingBookPairRequest) (FundingBookPair, error) {
			return fundingWindowTestPair(request), FundingBookUnavailableError{Reason: FundingBookNoDisplayedSide}
		}},
		{"structural-error", func(FundingBookPairRequest) (FundingBookPair, error) {
			return FundingBookPair{}, errors.New("wrong instrument binding")
		}},
		{"wrapped-unavailable-structural-error", func(FundingBookPairRequest) (FundingBookPair, error) {
			return FundingBookPair{}, fmt.Errorf("tick violation: %w", FundingBookUnavailableError{Reason: FundingBookNoDisplayedSide})
		}},
		{"joined-unavailable-structural-error", func(FundingBookPairRequest) (FundingBookPair, error) {
			return FundingBookPair{}, errors.Join(ErrFundingBookUnavailable, errors.New("tick violation"))
		}},
		{"unknown-unavailable-reason", func(FundingBookPairRequest) (FundingBookPair, error) {
			return FundingBookPair{}, FundingBookUnavailableError{Reason: "TICK_VIOLATION"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			appended := 0
			captureCalls := 0
			recorder, err := NewFundingWindowRecorder(fundingWindowTestConfig(), fundingPairSourceFunc(func(request FundingBookPairRequest) (FundingBookPair, error) {
				captureCalls++
				return test.pair(request)
			}),
				fundingObservationAppendFunc(func(FundingBookObservation) (uint64, error) { appended++; return 1, nil }))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := recorder.ObserveAt(20); err == nil || appended != 0 {
				t.Fatalf("contradictory source was recorded: appended=%d err=%v", appended, err)
			}
			if _, err := recorder.ObserveAt(20); err == nil || captureCalls != 1 || appended != 0 {
				t.Fatalf("failed source boundary was retried: captures=%d appended=%d err=%v", captureCalls, appended, err)
			}
		})
	}
	appendErr := errors.New("evidence writer failed")
	appended := 0
	recorder, err := NewFundingWindowRecorder(fundingWindowTestConfig(), fundingPairSourceFunc(func(request FundingBookPairRequest) (FundingBookPair, error) {
		return fundingWindowTestPair(request), nil
	}), fundingObservationAppendFunc(func(FundingBookObservation) (uint64, error) {
		appended++
		if appended == 1 {
			return 0, appendErr
		}
		return 1, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.ObserveAt(20); !errors.Is(err, appendErr) {
		t.Fatalf("writer failure lost: %v", err)
	}
	if _, err := recorder.ObserveAt(20); !errors.Is(err, appendErr) || appended != 1 {
		t.Fatalf("possibly committed failed event was retried: calls=%d err=%v", appended, err)
	}
	sequences := []uint64{1, 1, 2}
	sequenceCalls := 0
	recorder, err = NewFundingWindowRecorder(fundingWindowTestConfig(),
		fundingPairSourceFunc(func(request FundingBookPairRequest) (FundingBookPair, error) {
			return fundingWindowTestPair(request), nil
		}), fundingObservationAppendFunc(func(FundingBookObservation) (uint64, error) {
			sequence := sequences[sequenceCalls]
			sequenceCalls++
			return sequence, nil
		}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.ObserveAt(20); err != nil {
		t.Fatal(err)
	}
	if _, err := recorder.ObserveAt(21); err == nil {
		t.Fatal("duplicate committed event sequence was accepted")
	}
	if _, err := recorder.ObserveAt(21); err == nil || sequenceCalls != 2 {
		t.Fatalf("duplicate committed event was retried: calls=%d err=%v", sequenceCalls, err)
	}
	if _, err := recorder.WindowBefore(22); err == nil {
		t.Fatal("failed source recorder returned a complete numeric window")
	}
}
