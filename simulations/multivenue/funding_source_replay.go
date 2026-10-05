package multivenue

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"exchange_sim/evstream"
	"exchange_sim/exchange"
	"exchange_sim/types"
)

const fundingSourceRoute = "funding_source.jsonl"

// ReplayFundingSourceObservations verifies a complete expected source grid
// against the canonical ordered stream. lastBoundaryNano comes from the
// pinned run contract, never from the largest observed event timestamp.
// Stream/trailer integrity is checked here; external artifact binding and
// live-book fidelity remain separate obligations.
func ReplayFundingSourceObservations(stream io.Reader, config exchange.FundingWindowSourceConfig, lastBoundaryNano int64) ([]exchange.FundingSourceRecord, error) {
	if config.VenueID == "" || config.SpotSymbol == "" || config.PerpSymbol == "" ||
		config.SpotSymbol == config.PerpSymbol || config.FirstBoundaryNano < 0 ||
		config.SampleSpacingNano <= 0 || config.SampleCount <= 0 ||
		lastBoundaryNano < config.FirstBoundaryNano ||
		(lastBoundaryNano-config.FirstBoundaryNano)%config.SampleSpacingNano != 0 {
		return nil, fmt.Errorf("funding source replay: invalid expected source grid")
	}
	reader, err := evstream.NewReader(stream, evstream.ReaderOptions{VerifyHash: true})
	if err != nil {
		return nil, fmt.Errorf("funding source replay: open canonical stream: %w", err)
	}
	if reader.SchemaEpoch() < binaryEvidenceSchemaEpoch {
		return nil, fmt.Errorf("funding source replay: stream has no successor source/evidence contract")
	}
	expectedAt := config.FirstBoundaryNano
	seenLast := false
	var lastVenueSequence uint64
	records := make([]exchange.FundingSourceRecord, 0)
	err = reader.Range(func(frame evstream.Frame) error {
		if len(frame.Payload) < 8 {
			return fmt.Errorf("frame %d lacks event envelope", frame.Header.Seq)
		}
		eventRef := binary.LittleEndian.Uint32(frame.Payload[4:8])
		eventName, ok := reader.Lookup(eventRef)
		if !ok {
			return fmt.Errorf("frame %d has unknown event name", frame.Header.Seq)
		}
		if eventName != "funding_book_observation" {
			return nil
		}
		route, rendered, err := renderBinaryFrameVersioned(reader, frame, true)
		if err != nil {
			return err
		}
		if route.route != fundingSourceRoute {
			return fmt.Errorf("frame %d funding source has wrong route %q", frame.Header.Seq, route.route)
		}
		var persisted renderPersistedEvent
		if err := json.Unmarshal(rendered.raw, &persisted); err != nil {
			return fmt.Errorf("frame %d funding source envelope: %w", frame.Header.Seq, err)
		}
		var observation exchange.FundingBookObservation
		if err := json.Unmarshal(persisted.Data.Payload, &observation); err != nil {
			return fmt.Errorf("frame %d funding source payload: %w", frame.Header.Seq, err)
		}
		canonicalPayload, err := json.Marshal(observation)
		if err != nil || !bytes.Equal(canonicalPayload, persisted.Data.Payload) {
			return fmt.Errorf("frame %d funding source payload is not the canonical observation schema", frame.Header.Seq)
		}
		if frame.Header.ClientID != 0 || observation.VenueID != frame.Venue ||
			observation.TimestampNano != frame.Header.SimTS ||
			persisted.Data.GlobalSequence != frame.Header.Seq {
			return fmt.Errorf("frame %d funding source header/payload identity mismatch", frame.Header.Seq)
		}
		if frame.Venue != config.VenueID {
			return nil
		}
		if seenLast || persisted.Data.Sequence <= lastVenueSequence || observation.TimestampNano != expectedAt ||
			observation.SpotSymbol != config.SpotSymbol || observation.PerpSymbol != config.PerpSymbol ||
			!replayFundingObservationValid(observation) {
			return fmt.Errorf("frame %d funding source is missing, duplicate, out of order or contradictory at %d; expected %d", frame.Header.Seq, observation.TimestampNano, expectedAt)
		}
		records = append(records, exchange.FundingSourceRecord{Observation: observation, EventSeq: frame.Header.Seq})
		lastVenueSequence = persisted.Data.Sequence
		if expectedAt == lastBoundaryNano {
			seenLast = true
		} else {
			var fits bool
			expectedAt, fits = types.TryAdd(expectedAt, config.SampleSpacingNano)
			if !fits || expectedAt > lastBoundaryNano {
				return fmt.Errorf("funding source replay: expected grid overflows before last boundary")
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("funding source replay: %w", err)
	}
	if !reader.Terminated() || !seenLast {
		return nil, fmt.Errorf("funding source replay: incomplete stream or missing expected observation at %d", expectedAt)
	}
	return records, nil
}

func replayFundingObservationValid(observation exchange.FundingBookObservation) bool {
	if observation.Version != exchange.FundingBookObservationVersion {
		return false
	}
	if !observation.Available {
		return observation.Pair == nil && exchange.FundingBookUnavailableReason(observation.Reason).Valid()
	}
	if observation.Reason != "" || observation.Pair == nil {
		return false
	}
	pair := observation.Pair
	if pair.VenueID != observation.VenueID || pair.TimestampNano != observation.TimestampNano ||
		pair.Spot.Symbol != observation.SpotSymbol || pair.Perp.Symbol != observation.PerpSymbol ||
		pair.BaseAsset == "" || pair.QuoteAsset == "" || pair.BasePrecision <= 0 || pair.QuotePrecision <= 0 {
		return false
	}
	return replayFundingTopValid(pair.Spot, observation.TimestampNano) &&
		replayFundingTopValid(pair.Perp, observation.TimestampNano)
}

func replayFundingTopValid(top exchange.FundingBookTop, atNano int64) bool {
	if top.Bid.Price <= 0 || top.Ask.Price < top.Bid.Price ||
		top.MidPrice != types.Midpoint(top.Bid.Price, top.Ask.Price) {
		return false
	}
	for _, quote := range [...]exchange.FundingVisibleQuote{top.Bid, top.Ask} {
		if quote.VisibleQty <= 0 || quote.OldestVisibleOrderAcceptedAtNano < 0 ||
			quote.OldestVisibleOrderAcceptedAtNano >= atNano ||
			quote.OldestVisibleOrderAgeNano != atNano-quote.OldestVisibleOrderAcceptedAtNano {
			return false
		}
	}
	return true
}
