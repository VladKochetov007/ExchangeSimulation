package multivenue

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sync"
	"testing"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
)

type fundingTestPairSource func(exchange.FundingBookPairRequest) (exchange.FundingBookPair, error)

func (source fundingTestPairSource) CaptureFundingBookPair(request exchange.FundingBookPairRequest) (exchange.FundingBookPair, error) {
	return source(request)
}

func fundingTestPublicPair(request exchange.FundingBookPairRequest) exchange.FundingBookPair {
	quote := func(price int64) exchange.FundingVisibleQuote {
		return exchange.FundingVisibleQuote{Price: price, VisibleQty: 1,
			OldestVisibleOrderAcceptedAtNano: request.TimestampNano - 1,
			OldestVisibleOrderAgeNano:        1}
	}
	return exchange.FundingBookPair{VenueID: request.VenueID, TimestampNano: request.TimestampNano,
		BaseAsset: "ABC", QuoteAsset: "USD", BasePrecision: 1, QuotePrecision: 1,
		Spot: exchange.FundingBookTop{Symbol: request.SpotSymbol, Bid: quote(99), Ask: quote(101), MidPrice: 100},
		Perp: exchange.FundingBookTop{Symbol: request.PerpSymbol, Bid: quote(103), Ask: quote(105), MidPrice: 104}}
}

func TestFundingSourceObservationUsesActualCanonicalFrameSequence(t *testing.T) {
	var output bytes.Buffer
	sink := &checkpointSink{binary: newBinaryEvidence(&output), includeEvidenceOnly: true,
		replaceRaw: true, firstEvent: true}
	venueSequence := uint64(0)
	logger := venueLogger{venueID: "N", route: "funding_source.jsonl", sink: sink,
		sequence: &venueSequence, sequenceMu: &sync.Mutex{}}
	recorder, err := exchange.NewFundingWindowRecorder(exchange.FundingWindowSourceConfig{
		VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP",
		FirstBoundaryNano: 20, SampleSpacingNano: 1, SampleCount: 2,
	}, fundingTestPairSource(func(request exchange.FundingBookPairRequest) (exchange.FundingBookPair, error) {
		if request.TimestampNano == 21 {
			return exchange.FundingBookPair{}, exchange.FundingBookUnavailableError{Reason: exchange.FundingBookNoDisplayedSide}
		}
		return fundingTestPublicPair(request), nil
	}), logger)
	if err != nil {
		t.Fatal(err)
	}
	var retained []exchange.FundingSourceRecord
	for _, at := range []int64{20, 21, 22} {
		record, err := recorder.ObserveAt(at)
		if err != nil {
			t.Fatal(err)
		}
		retained = append(retained, record)
	}
	window, err := recorder.WindowBefore(22)
	if err != nil || len(window.Unavailable) != 1 || window.Samples != nil {
		t.Fatalf("explicit prior one-sided book became a price: %+v, %v", window, err)
	}
	if err := sink.close(); err != nil {
		t.Fatal(err)
	}
	reader, err := evstream.NewReader(bytes.NewReader(output.Bytes()), evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		t.Fatal(err)
	}
	var decoded []exchange.FundingBookObservation
	var globalSeq []uint64
	err = reader.Range(func(frame evstream.Frame) error {
		_, rendered, err := renderBinaryFrameVersioned(reader, frame, true)
		if err != nil {
			return err
		}
		var persisted renderPersistedEvent
		if err := json.Unmarshal(rendered.raw, &persisted); err != nil {
			return err
		}
		if persisted.Event != "funding_book_observation" || persisted.Data.GlobalSequence != frame.Header.Seq ||
			persisted.Data.Sequence != uint64(len(decoded)+1) || frame.Header.SimTS != int64(20+len(decoded)) {
			t.Fatalf("wrong evidence envelope: %+v, frame %+v", persisted, frame.Header)
		}
		var observation exchange.FundingBookObservation
		if err := json.Unmarshal(persisted.Data.Payload, &observation); err != nil {
			return err
		}
		decoded = append(decoded, observation)
		globalSeq = append(globalSeq, frame.Header.Seq)
		return nil
	})
	if err != nil || !reader.Terminated() {
		t.Fatalf("canonical observation stream incomplete: %v", err)
	}
	if len(globalSeq) != 3 || globalSeq[0] <= 1 ||
		!reflect.DeepEqual(globalSeq, []uint64{retained[0].EventSeq, retained[1].EventSeq, retained[2].EventSeq}) ||
		decoded[1].Available || decoded[1].Reason != string(exchange.FundingBookNoDisplayedSide) || decoded[1].Pair != nil {
		t.Fatalf("global sequence or explicit unavailable observation lost: seq=%v records=%+v", globalSeq, decoded)
	}
	replayed, err := ReplayFundingSourceObservations(bytes.NewReader(output.Bytes()), exchange.FundingWindowSourceConfig{
		VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP",
		FirstBoundaryNano: 20, SampleSpacingNano: 1, SampleCount: 2,
	}, 22)
	if err != nil || !reflect.DeepEqual(replayed, retained) {
		t.Fatalf("canonical replay differs from producer: got %+v, want %+v, err=%v", replayed, retained, err)
	}
	truncated := output.Bytes()[:output.Len()-5]
	if _, err := ReplayFundingSourceObservations(bytes.NewReader(truncated), exchange.FundingWindowSourceConfig{
		VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP",
		FirstBoundaryNano: 20, SampleSpacingNano: 1, SampleCount: 2,
	}, 22); err == nil {
		t.Fatal("truncated canonical source stream passed replay")
	}
}

func TestFundingSourceReplayRejectsMissingReorderedAndContradictoryEvents(t *testing.T) {
	config := exchange.FundingWindowSourceConfig{VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP",
		FirstBoundaryNano: 20, SampleSpacingNano: 1, SampleCount: 2}
	for _, test := range []struct {
		name     string
		times    []int64
		mutate   func(*exchange.FundingBookObservation)
		route    string
		sequence func(int) uint64
		wantErr  bool
	}{
		{name: "complete", times: []int64{20, 21, 22}},
		{name: "missing-middle", times: []int64{20, 22}, wantErr: true},
		{name: "missing-last", times: []int64{20, 21}, wantErr: true},
		{name: "duplicate", times: []int64{20, 21, 21, 22}, wantErr: true},
		{name: "duplicate-venue-sequence", times: []int64{20, 21, 22}, sequence: func(index int) uint64 {
			if index == 1 {
				return 1
			}
			return uint64(index + 1)
		}, wantErr: true},
		{name: "reordered", times: []int64{20, 22, 21}, wantErr: true},
		{name: "foreign-venue", times: []int64{20, 21, 22}, mutate: func(observation *exchange.FundingBookObservation) {
			if observation.TimestampNano == 21 {
				observation.VenueID = "S"
			}
		}, wantErr: true},
		{name: "fabricated-mid", times: []int64{20, 21, 22}, mutate: func(observation *exchange.FundingBookObservation) {
			observation.Pair.Perp.MidPrice = 1
		}, wantErr: true},
		{name: "unknown-version", times: []int64{20, 21, 22}, mutate: func(observation *exchange.FundingBookObservation) {
			observation.Version++
		}, wantErr: true},
		{name: "valid-unavailable-reason", times: []int64{20, 21, 22}, mutate: func(observation *exchange.FundingBookObservation) {
			if observation.TimestampNano == 21 {
				observation.Available = false
				observation.Pair = nil
				observation.Reason = string(exchange.FundingBookCrossedOrNonpositivePair)
			}
		}},
		{name: "unknown-unavailable-reason", times: []int64{20, 21, 22}, mutate: func(observation *exchange.FundingBookObservation) {
			if observation.TimestampNano == 21 {
				observation.Available = false
				observation.Pair = nil
				observation.Reason = "TICK_VIOLATION"
			}
		}, wantErr: true},
		{name: "wrong-route", times: []int64{20, 21, 22}, route: "general.jsonl", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			binarySink := newBinaryEvidence(&output)
			route := fundingSourceRoute
			if test.route != "" {
				route = test.route
			}
			for index, at := range test.times {
				request := exchange.FundingBookPairRequest{VenueID: "N", SpotSymbol: "ABC-USD", PerpSymbol: "ABC-PERP", TimestampNano: at}
				observation := exchange.FundingBookObservation{Version: exchange.FundingBookObservationVersion,
					VenueID: "N", SpotSymbol: request.SpotSymbol,
					PerpSymbol: request.PerpSymbol, TimestampNano: at, Available: true}
				pair := fundingTestPublicPair(request)
				observation.Pair = &pair
				if test.mutate != nil {
					test.mutate(&observation)
				}
				venueSequence := uint64(index + 1)
				if test.sequence != nil {
					venueSequence = test.sequence(index)
				}
				if _, err := binarySink.recordRequired(at, 0, "funding_book_observation", "N",
					observation, route, venueSequence); err != nil {
					t.Fatal(err)
				}
			}
			if err := binarySink.finish(); err != nil {
				t.Fatal(err)
			}
			replayed, err := ReplayFundingSourceObservations(bytes.NewReader(output.Bytes()), config, 22)
			if (err != nil) != test.wantErr {
				t.Fatalf("replay = %+v, err=%v, wantErr=%t", replayed, err, test.wantErr)
			}
		})
	}
}

func TestFundingSourceRequiredEvidenceFailsClosedOnUnencodablePayload(t *testing.T) {
	var output bytes.Buffer
	binary := newBinaryEvidence(&output)
	if _, err := binary.recordRequired(20, 0, "funding_book_observation", "N",
		binaryUnencodablePayload{}, "funding_source.jsonl", 1); err == nil {
		t.Fatal("required source evidence was silently substituted")
	}
	if _, err := binary.recordRequired(21, 0, "funding_book_observation", "N",
		map[string]int{"price": 100}, "funding_source.jsonl", 2); err == nil {
		t.Fatal("failed canonical source sink accepted a later event")
	}
}

func TestFundingSourceAdapterRejectsMissingCanonicalSink(t *testing.T) {
	logger := venueLogger{venueID: "N", route: "funding_source.jsonl"}
	if _, err := logger.AppendFundingObservation(exchange.FundingBookObservation{VenueID: "N", TimestampNano: 20}); err == nil {
		t.Fatal("source adapter accepted a missing canonical sink")
	}
	var output bytes.Buffer
	sink := &checkpointSink{binary: newBinaryEvidence(&output), firstEvent: true}
	venueSequence := uint64(0)
	logger = venueLogger{venueID: "N", route: "funding_source.jsonl", sink: sink,
		sequence: &venueSequence, sequenceMu: &sync.Mutex{}}
	if _, err := logger.AppendFundingObservation(exchange.FundingBookObservation{VenueID: "N", TimestampNano: 20}); err == nil {
		t.Fatal("source adapter accepted a binary sink without evidence-only contract")
	}
	if venueSequence != 0 {
		t.Fatalf("unexpected local sequence after rejected source event: %d", venueSequence)
	}
	if err := sink.close(); err != nil {
		t.Fatal(err)
	}
	discardSink := &checkpointSink{binary: newBinaryEvidence(&bytes.Buffer{}),
		includeEvidenceOnly: true, discardBinary: true, firstEvent: true}
	logger.sink = discardSink
	if _, err := logger.AppendFundingObservation(exchange.FundingBookObservation{VenueID: "N", TimestampNano: 20}); err == nil {
		t.Fatal("source adapter accepted a discarded canonical stream")
	}
}
