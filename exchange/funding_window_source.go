package exchange

import (
	"errors"
	"fmt"
	"math/big"

	"exchange_sim/instrument"
	"exchange_sim/types"
)

// FundingBookPairSource reads one live venue-local pair at the requested
// pre-instant frontier. The runner, not this interface, owns that frontier.
type FundingBookPairSource interface {
	CaptureFundingBookPair(FundingBookPairRequest) (FundingBookPair, error)
}

// FundingObservationAppender returns the actual global sequence of the
// successfully appended canonical event. Venue-local counters are not enough.
type FundingObservationAppender interface {
	AppendFundingObservation(FundingBookObservation) (uint64, error)
}

type FundingWindowSourceConfig struct {
	VenueID           string
	SpotSymbol        string
	PerpSymbol        string
	FirstBoundaryNano int64
	SampleSpacingNano int64
	SampleCount       int
}

const FundingBookObservationVersion uint16 = 1

// FundingBookObservation is emitted at every configured boundary, including
// an explicit unavailable observation when a live book has no public pair.
// A missing event is an evidence defect, not an unavailable market price.
type FundingBookObservation struct {
	Version       uint16           `json:"version"`
	VenueID       string           `json:"venue_id"`
	SpotSymbol    string           `json:"spot_symbol"`
	PerpSymbol    string           `json:"perp_symbol"`
	TimestampNano int64            `json:"timestamp_nano"`
	Available     bool             `json:"available"`
	Reason        string           `json:"reason,omitempty"`
	Pair          *FundingBookPair `json:"pair,omitempty"`
}

type FundingSourceRecord struct {
	Observation FundingBookObservation
	EventSeq    uint64
}

// FundingSourceWindow contains only prior boundary observations. Samples is
// populated only when all sources are available; the complete source records
// remain available to attest both positive and unavailable windows.
type FundingSourceWindow struct {
	Sources     []FundingSourceRecord
	Samples     []instrument.FundingWindowSample
	Unavailable []FundingBookObservation
}

// FundingWindowRecorder is opt-in. It keeps enough observations for a window
// even if the current t boundary has already been captured. Its methods must
// be called by one exclusive deterministic runner phase, in grid order.
type FundingWindowRecorder struct {
	config   FundingWindowSourceConfig
	source   FundingBookPairSource
	append   FundingObservationAppender
	history  []FundingSourceRecord
	observed bool
	lastNano int64
	lastSeq  uint64
	failed   error
}

func NewFundingWindowRecorder(config FundingWindowSourceConfig, source FundingBookPairSource, appender FundingObservationAppender) (*FundingWindowRecorder, error) {
	if config.VenueID == "" || config.SpotSymbol == "" || config.PerpSymbol == "" ||
		config.SpotSymbol == config.PerpSymbol || config.FirstBoundaryNano < 0 ||
		config.SampleSpacingNano <= 0 || config.SampleCount <= 0 || source == nil || appender == nil {
		return nil, fmt.Errorf("funding source window: invalid configuration or source/evidence adapter")
	}
	return &FundingWindowRecorder{config: config, source: source, append: appender}, nil
}

// ObserveAt never substitutes a cached pair. An unavailable live book is
// recorded explicitly; structural source or evidence errors stop the producer.
func (recorder *FundingWindowRecorder) ObserveAt(atNano int64) (FundingSourceRecord, error) {
	if recorder == nil {
		return FundingSourceRecord{}, fmt.Errorf("funding source window: nil recorder")
	}
	if recorder.failed != nil {
		return FundingSourceRecord{}, recorder.failed
	}
	expected := recorder.config.FirstBoundaryNano
	if recorder.observed {
		var ok bool
		expected, ok = types.TryAdd(recorder.lastNano, recorder.config.SampleSpacingNano)
		if !ok {
			recorder.failed = fmt.Errorf("funding source window: next boundary overflows")
			return FundingSourceRecord{}, recorder.failed
		}
	}
	if atNano != expected {
		recorder.failed = fmt.Errorf("funding source window: omitted, duplicate or off-grid boundary %d, expected %d", atNano, expected)
		return FundingSourceRecord{}, recorder.failed
	}
	request := FundingBookPairRequest{VenueID: recorder.config.VenueID,
		SpotSymbol: recorder.config.SpotSymbol, PerpSymbol: recorder.config.PerpSymbol, TimestampNano: atNano}
	observation := FundingBookObservation{Version: FundingBookObservationVersion,
		VenueID: request.VenueID, SpotSymbol: request.SpotSymbol,
		PerpSymbol: request.PerpSymbol, TimestampNano: atNano}
	pair, err := recorder.source.CaptureFundingBookPair(request)
	switch {
	case err == nil:
		if err := validateFundingObservedPair(request, pair); err != nil {
			recorder.failed = err
			return FundingSourceRecord{}, recorder.failed
		}
		observation.Available = true
		observation.Pair = &pair
	case errors.Is(err, ErrFundingBookUnavailable):
		if pair != (FundingBookPair{}) {
			recorder.failed = fmt.Errorf("funding source window: unavailable source returned a pair")
			return FundingSourceRecord{}, recorder.failed
		}
		observation.Reason = err.Error()
	default:
		recorder.failed = fmt.Errorf("funding source window: source contradiction at %d: %w", atNano, err)
		return FundingSourceRecord{}, recorder.failed
	}
	seq, err := recorder.append.AppendFundingObservation(observation)
	if err != nil {
		recorder.failed = fmt.Errorf("funding source window: append canonical observation at %d: %w", atNano, err)
		return FundingSourceRecord{}, recorder.failed
	}
	if seq == 0 || seq <= recorder.lastSeq {
		recorder.failed = fmt.Errorf("funding source window: non-increasing canonical event sequence %d", seq)
		return FundingSourceRecord{}, recorder.failed
	}
	record := FundingSourceRecord{Observation: observation, EventSeq: seq}
	recorder.history = append(recorder.history, record)
	if len(recorder.history)-1 > recorder.config.SampleCount {
		recorder.history = recorder.history[1:]
	}
	recorder.observed, recorder.lastNano, recorder.lastSeq = true, atNano, seq
	return cloneFundingSourceRecord(record), nil
}

// WindowBefore fails on missing capture/evidence, but returns an explicitly
// unavailable source window without numeric samples when a book was one-sided.
func (recorder *FundingWindowRecorder) WindowBefore(settlementNano int64) (FundingSourceWindow, error) {
	if recorder == nil || !recorder.observed || settlementNano < recorder.config.FirstBoundaryNano {
		return FundingSourceWindow{}, fmt.Errorf("funding source window: no complete capture frontier")
	}
	if recorder.failed != nil {
		return FundingSourceWindow{}, recorder.failed
	}
	step := big.NewInt(recorder.config.SampleSpacingNano)
	start := new(big.Int).Sub(big.NewInt(settlementNano),
		new(big.Int).Mul(step, big.NewInt(int64(recorder.config.SampleCount))))
	if !start.IsInt64() || start.Sign() < 0 || start.Int64() < recorder.config.FirstBoundaryNano ||
		new(big.Int).Mod(new(big.Int).Sub(big.NewInt(settlementNano), big.NewInt(recorder.config.FirstBoundaryNano)), step).Sign() != 0 {
		return FundingSourceWindow{}, fmt.Errorf("funding source window: settlement lacks a configured prior window")
	}
	result := FundingSourceWindow{Sources: make([]FundingSourceRecord, 0, recorder.config.SampleCount),
		Samples: make([]instrument.FundingWindowSample, 0, recorder.config.SampleCount)}
	for index := 0; index < recorder.config.SampleCount; index++ {
		at := new(big.Int).Add(start, new(big.Int).Mul(step, big.NewInt(int64(index))))
		if !at.IsInt64() {
			return FundingSourceWindow{}, fmt.Errorf("funding source window: sample timestamp overflows")
		}
		matching := false
		for _, record := range recorder.history {
			if record.Observation.TimestampNano != at.Int64() {
				continue
			}
			if matching {
				return FundingSourceWindow{}, fmt.Errorf("funding source window: duplicate capture at %d", at.Int64())
			}
			matching = true
			result.Sources = append(result.Sources, cloneFundingSourceRecord(record))
			if record.Observation.Available {
				if record.Observation.Pair == nil {
					return FundingSourceWindow{}, fmt.Errorf("funding source window: missing available pair at %d", at.Int64())
				}
				result.Samples = append(result.Samples, record.Observation.Pair.RateWindowSample())
			} else {
				result.Unavailable = append(result.Unavailable, record.Observation)
			}
		}
		if !matching {
			return FundingSourceWindow{}, fmt.Errorf("funding source window: missing canonical observation at %d", at.Int64())
		}
	}
	if len(result.Unavailable) != 0 {
		result.Samples = nil
	}
	return result, nil
}

func validateFundingObservedPair(request FundingBookPairRequest, pair FundingBookPair) error {
	if pair.VenueID != request.VenueID || pair.TimestampNano != request.TimestampNano ||
		pair.Spot.Symbol != request.SpotSymbol || pair.Perp.Symbol != request.PerpSymbol ||
		pair.BaseAsset == "" || pair.QuoteAsset == "" || pair.BasePrecision <= 0 || pair.QuotePrecision <= 0 ||
		!validFundingObservedTop(pair.Spot, request.TimestampNano) ||
		!validFundingObservedTop(pair.Perp, request.TimestampNano) {
		return fmt.Errorf("funding source window: captured pair contradicts source identity or price/depth/age")
	}
	return nil
}

func validFundingObservedTop(top FundingBookTop, atNano int64) bool {
	if top.Bid.Price <= 0 || top.Ask.Price < top.Bid.Price || top.MidPrice != types.Midpoint(top.Bid.Price, top.Ask.Price) {
		return false
	}
	for _, quote := range [...]FundingVisibleQuote{top.Bid, top.Ask} {
		if quote.VisibleQty <= 0 || quote.OldestVisibleOrderAcceptedAtNano < 0 ||
			quote.OldestVisibleOrderAcceptedAtNano >= atNano ||
			quote.OldestVisibleOrderAgeNano != atNano-quote.OldestVisibleOrderAcceptedAtNano {
			return false
		}
	}
	return true
}

func cloneFundingSourceRecord(record FundingSourceRecord) FundingSourceRecord {
	if record.Observation.Pair != nil {
		pair := *record.Observation.Pair
		record.Observation.Pair = &pair
	}
	return record
}
