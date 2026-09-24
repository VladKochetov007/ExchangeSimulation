package executionpilot

import (
	"bytes"
	"testing"

	"exchange_sim/actor"
	"exchange_sim/exchange"
	"exchange_sim/simulations/executionlab"
)

func TestProfileRetainedTimingSeparatesCompleteAndCensoredIntervals(t *testing.T) {
	var output bytes.Buffer
	recorder := NewRecorder(&output)
	prices := []int64{101, 102, 0}
	quantities := []int64{2, 1, 0}
	for index := range prices {
		publishedAt := int64(index+1) * 100_000_000
		sequence := uint64(index + 1)
		bids := []exchange.PriceLevel{{Price: 99, VisibleQty: 3}}
		asks := []exchange.PriceLevel{}
		if prices[index] != 0 {
			asks = append(asks, exchange.PriceLevel{Price: prices[index], VisibleQty: quantities[index]})
		}
		recorder.Record(executionlab.EvidenceObservation{
			Timestamp: publishedAt, Source: "exchange", Name: "BookSnapshot", Route: "ABC/USD",
			Payload: map[string]any{"source_sequence": sequence, "public_bids": bids, "public_asks": asks},
		})
		recorder.Record(executionlab.EvidenceObservation{
			Timestamp: publishedAt + 1_000_000, ClientID: 13, Source: "actor", Name: "book_snapshot_receipt",
			Payload: actor.BookSnapshotEvent{Symbol: "ABC/USD", Timestamp: publishedAt, SeqNum: sequence,
				Snapshot: &exchange.BookSnapshot{Bids: bids, Asks: asks}},
		})
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	profile, err := ProfileRetainedTiming(bytes.NewReader(output.Bytes()), identity, "ABC/USD", 13, 400_000_000, []int64{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if profile.PublicationCount != 3 || profile.ReceiptCount != 3 ||
		profile.PublicationGap.P50NS != 100_000_000 || profile.ReceiptGap.P50NS != 100_000_000 ||
		profile.PublicationToReceipt.P50NS != 1_000_000 ||
		profile.TouchSampledLifetime.Count != 2 || profile.TouchSampledLifetime.P50NS != 100_000_000 ||
		profile.TouchRightCensored != 1 || profile.TouchCensoredAgeNS != 99_000_000 {
		t.Fatalf("timing profile = %#v", profile)
	}
	if profile.DepthEpisodes[0].CompleteEpisodes.P50NS != 200_000_000 ||
		profile.DepthEpisodes[1].CompleteEpisodes.P50NS != 100_000_000 {
		t.Fatalf("depth episodes = %#v", profile.DepthEpisodes)
	}
	if _, err := ProfileRetainedTiming(bytes.NewReader(output.Bytes()), identity, "ABC/USD", 13, 400_000_000, []int64{1, 1}); err == nil {
		t.Fatal("duplicate target accepted")
	}
	corrupted := append([]byte(nil), output.Bytes()...)
	corrupted[len(corrupted)-1] ^= 1
	if _, err := ProfileRetainedTiming(bytes.NewReader(corrupted), identity, "ABC/USD", 13, 400_000_000, []int64{1}); err == nil {
		t.Fatal("corrupted evidence accepted")
	}
}
