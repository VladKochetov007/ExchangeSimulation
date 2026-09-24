package executionpilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// TimingDistribution summarizes complete intervals only. Censored intervals
// remain separate because a sampled quote is not proof of an actual book state
// between publications.
type TimingDistribution struct {
	Count int   `json:"count"`
	MinNS int64 `json:"min_ns"`
	P50NS int64 `json:"p50_ns"`
	P90NS int64 `json:"p90_ns"`
	MaxNS int64 `json:"max_ns"`
}

type DepthEpisodeProfile struct {
	TargetQty             int64              `json:"target_qty"`
	CompleteEpisodes      TimingDistribution `json:"complete_episodes"`
	RightCensoredEpisodes int                `json:"right_censored_episodes"`
	RightCensoredAgeNS    int64              `json:"right_censored_age_ns"`
}

type RetainedTimingProfile struct {
	Symbol               string                `json:"symbol"`
	FocalClientID        uint64                `json:"focal_client_id"`
	CutoffNS             int64                 `json:"cutoff_ns"`
	PublicationCount     int                   `json:"publication_count"`
	ReceiptCount         int                   `json:"receipt_count"`
	DecisionTickCount    int                   `json:"decision_tick_count"`
	PublicationGap       TimingDistribution    `json:"publication_gap"`
	ReceiptGap           TimingDistribution    `json:"receipt_gap"`
	PublicationToReceipt TimingDistribution    `json:"publication_to_receipt"`
	TouchSampledLifetime TimingDistribution    `json:"touch_sampled_lifetime"`
	TouchRightCensored   int                   `json:"touch_right_censored"`
	TouchCensoredAgeNS   int64                 `json:"touch_censored_age_ns"`
	DepthEpisodes        []DepthEpisodeProfile `json:"depth_episodes"`
}

type observedTouch struct {
	BidPrice int64
	BidQty   int64
	AskPrice int64
	AskQty   int64
}

type depthEpisode struct {
	target    int64
	startedAt int64
	active    bool
	durations []int64
	censored  int
}

// ProfileRetainedTiming reads an entire canonical stream and verifies its
// execution hash before returning a pre-cutoff descriptive profile. Depth
// episodes are observed-snapshot proxies, not continuous executable windows.
func ProfileRetainedTiming(input io.Reader, identity EvidenceIdentity, symbol string, focalClientID uint64, cutoffNS int64, targets []int64) (RetainedTimingProfile, error) {
	profile := RetainedTimingProfile{Symbol: symbol, FocalClientID: focalClientID, CutoffNS: cutoffNS}
	if symbol == "" || focalClientID == 0 || cutoffNS <= 0 || len(targets) == 0 {
		return profile, errors.New("execution pilot: invalid timing profile boundary")
	}
	episodes := make([]depthEpisode, len(targets))
	for index, target := range targets {
		if target <= 0 || slices.Contains(targets[:index], target) {
			return profile, errors.New("execution pilot: invalid or repeated timing target")
		}
		episodes[index].target = target
	}
	var publicationGaps, receiptGaps, deliveryLags, touchLifetimes []int64
	var lastPublicationAt, lastReceiptAt, touchStartedAt int64
	var havePublication, haveReceipt, haveTouch bool
	var previousTouch observedTouch
	profile.DepthEpisodes = make([]DepthEpisodeProfile, len(targets))
	err := WalkEvidence(input, identity, func(event RecordedEvent) error {
		if event.Timestamp >= cutoffNS {
			return nil
		}
		if event.Source == "exchange" && event.Name == "BookSnapshot" && event.Route == symbol {
			var publication publicationWire
			if err := json.Unmarshal(event.Payload, &publication); err != nil {
				return fmt.Errorf("decode timing publication: %w", err)
			}
			if publication.SourceSequence == 0 {
				return nil
			}
			if havePublication {
				if event.Timestamp < lastPublicationAt {
					return errors.New("execution pilot: decreasing timing publication")
				}
				publicationGaps = append(publicationGaps, event.Timestamp-lastPublicationAt)
			}
			lastPublicationAt, havePublication = event.Timestamp, true
			profile.PublicationCount++
		}
		if event.Source != "actor" || event.ClientID != focalClientID {
			return nil
		}
		if event.Name == "decision_tick" {
			profile.DecisionTickCount++
			return nil
		}
		if event.Name != "book_snapshot_receipt" {
			return nil
		}
		var snapshot snapshotWire
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil || snapshot.Snapshot == nil {
			return errors.New("execution pilot: malformed timing receipt")
		}
		if snapshot.Symbol != symbol || snapshot.Timestamp > event.Timestamp {
			return errors.New("execution pilot: invalid timing receipt identity")
		}
		if haveReceipt {
			if event.Timestamp < lastReceiptAt {
				return errors.New("execution pilot: decreasing timing receipt")
			}
			receiptGaps = append(receiptGaps, event.Timestamp-lastReceiptAt)
		}
		profile.ReceiptCount++
		deliveryLags = append(deliveryLags, event.Timestamp-snapshot.Timestamp)
		currentTouch := observedTouch{}
		if len(snapshot.Snapshot.Bids) > 0 && len(snapshot.Snapshot.Asks) > 0 {
			bid, ask := snapshot.Snapshot.Bids[0], snapshot.Snapshot.Asks[0]
			currentTouch = observedTouch{BidPrice: bid.Price, BidQty: bid.VisibleQty, AskPrice: ask.Price, AskQty: ask.VisibleQty}
		}
		if haveTouch && currentTouch != previousTouch {
			touchLifetimes = append(touchLifetimes, event.Timestamp-touchStartedAt)
			touchStartedAt = event.Timestamp
		}
		if !haveTouch {
			touchStartedAt, haveTouch = event.Timestamp, true
		}
		previousTouch = currentTouch
		var displayedAskQty int64
		for _, ask := range snapshot.Snapshot.Asks {
			if ask.VisibleQty < 0 || displayedAskQty > int64(^uint64(0)>>1)-ask.VisibleQty {
				return errors.New("execution pilot: invalid timing ask depth")
			}
			displayedAskQty += ask.VisibleQty
		}
		for index := range episodes {
			episode := &episodes[index]
			eligible := currentTouch.BidPrice > 0 && currentTouch.AskPrice > 0 && displayedAskQty >= episode.target
			if eligible && !episode.active {
				episode.active, episode.startedAt = true, event.Timestamp
			} else if !eligible && episode.active {
				episode.durations = append(episode.durations, event.Timestamp-episode.startedAt)
				episode.active = false
			}
		}
		lastReceiptAt, haveReceipt = event.Timestamp, true
		return nil
	})
	if err != nil {
		return profile, err
	}
	if !havePublication || !haveReceipt {
		return profile, errors.New("execution pilot: timing profile lacks publications or focal receipts")
	}
	profile.PublicationGap = summarizeTiming(publicationGaps)
	profile.ReceiptGap = summarizeTiming(receiptGaps)
	profile.PublicationToReceipt = summarizeTiming(deliveryLags)
	profile.TouchSampledLifetime = summarizeTiming(touchLifetimes)
	if haveTouch {
		profile.TouchRightCensored = 1
		profile.TouchCensoredAgeNS = cutoffNS - touchStartedAt
	}
	for index := range episodes {
		if episodes[index].active {
			episodes[index].censored++
		}
		profile.DepthEpisodes[index] = DepthEpisodeProfile{
			TargetQty: episodes[index].target, CompleteEpisodes: summarizeTiming(episodes[index].durations),
			RightCensoredEpisodes: episodes[index].censored,
		}
		if episodes[index].active {
			profile.DepthEpisodes[index].RightCensoredAgeNS = cutoffNS - episodes[index].startedAt
		}
	}
	return profile, nil
}

func summarizeTiming(values []int64) TimingDistribution {
	if len(values) == 0 {
		return TimingDistribution{}
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	return TimingDistribution{
		Count: len(sorted), MinNS: sorted[0], P50NS: sorted[(len(sorted)-1)/2],
		P90NS: sorted[(9*(len(sorted)-1)+9)/10], MaxNS: sorted[len(sorted)-1],
	}
}
