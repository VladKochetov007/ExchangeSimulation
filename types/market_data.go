package types

import (
	"crypto/sha256"
	"encoding/json"
	"sync"
)

// fingerprintPool reuses the render buffer. The fingerprint is computed once
// per delivery per participant, so the buffer would otherwise be one of the
// most frequently allocated objects in the simulator.
var fingerprintPool = sync.Pool{New: func() any { return make([]byte, 0, 512) }}

func fingerprintScratch() []byte { return fingerprintPool.Get().([]byte)[:0] }

func releaseFingerprintScratch(buf []byte) { fingerprintPool.Put(buf) }

type MarketDataMsg struct {
	Type      MDType
	Symbol    string
	SeqNum    uint64
	Timestamp int64
	Data      any
}

// MarketDataFingerprint is the canonical identity of one public message as a
// participant receives it. It intentionally covers every actor-visible field
// and is shared by delivery evidence and compact decision attestations.
func MarketDataFingerprint(msg *MarketDataMsg) ([16]byte, error) {
	if msg == nil {
		return [16]byte{}, nil
	}
	// The fast path renders the identical bytes without reflection, and
	// declines whenever anything would make that risky. Profiling put this
	// function at 3.16 % of simulator CPU, reached entirely from market-data
	// receipt scheduling, which runs once per delivery per participant.
	if scratch, ok := appendFingerprintJSON(fingerprintScratch(), msg); ok {
		digest := sha256.Sum256(scratch)
		var fingerprint [16]byte
		copy(fingerprint[:], digest[:])
		releaseFingerprintScratch(scratch)
		return fingerprint, nil
	}

	raw, err := json.Marshal(struct {
		Type      MDType `json:"type"`
		Symbol    string `json:"symbol"`
		Sequence  uint64 `json:"sequence"`
		Timestamp int64  `json:"timestamp"`
		Data      any    `json:"data"`
	}{msg.Type, msg.Symbol, msg.SeqNum, msg.Timestamp, msg.Data})
	if err != nil {
		return [16]byte{}, err
	}
	digest := sha256.Sum256(raw)
	var fingerprint [16]byte
	copy(fingerprint[:], digest[:])
	return fingerprint, nil
}

type BookSnapshot struct {
	Bids []PriceLevel `json:"bids"`
	Asks []PriceLevel `json:"asks"`
}

type BookDelta struct {
	Side       Side  `json:"side"`
	Price      int64 `json:"price"`
	VisibleQty int64 `json:"visible_qty"`
	HiddenQty  int64 `json:"hidden_qty"`
}

type Trade struct {
	TradeID      uint64 `json:"trade_id"`
	Price        int64  `json:"price"`
	Qty          int64  `json:"qty"`
	Side         Side   `json:"side"`
	TakerOrderID uint64 `json:"taker_order_id"`
	MakerOrderID uint64 `json:"maker_order_id"`
}

type PriceLevel struct {
	Price      int64 `json:"price"`
	VisibleQty int64 `json:"visible_qty"`
	HiddenQty  int64 `json:"hidden_qty"`
}

// InstrumentFeedSymbol is the reserved market-data channel carrying
// MDInstrument lifecycle announcements. Subscribe once to track listings.
const InstrumentFeedSymbol = "_instruments"

// InstrumentAnnouncement describes an instrument lifecycle event and doubles
// as the instrument descriptor returned by QueryInstruments.
type InstrumentAnnouncement struct {
	Action         string `json:"action"` // "listed" | "settled"
	Symbol         string `json:"symbol"`
	InstrumentType string `json:"instrument_type"`
	QuoteAsset     string `json:"quote_asset,omitempty"`
	BasePrecision  int64  `json:"base_precision,omitempty"`
	Underlying     string `json:"underlying,omitempty"` // spot symbol the contract references
	Strike         int64  `json:"strike,omitempty"`     // quote precision units (options)
	IsCall         bool   `json:"is_call,omitempty"`
	ExpiryNano     int64  `json:"expiry_nano,omitempty"`
	// ListedNano is the exchange publication time of the original listing.
	// Replays retain it even though the replay message itself has a later
	// Timestamp, so a participant can distinguish original tenor from current
	// time to expiry without parsing a symbol or reading hidden venue state.
	ListedNano *int64 `json:"listed_nano,omitempty"`
	// SettlementPrice is nil when this announcement has no terminal settlement
	// value (including a normal listing), and otherwise may be negative, zero,
	// or positive. Pointer presence is the availability contract: numeric zero
	// is never an absence sentinel. A nullable field avoids adding fake
	// unavailable-settlement fields to unrelated listing evidence.
	SettlementPrice *int64 `json:"settlement_price,omitempty"`
	TickSize        int64  `json:"tick_size,omitempty"`
	MinOrderSize    int64  `json:"min_order_size,omitempty"`
	Timestamp       int64  `json:"timestamp"`
}

type Subscription struct {
	ClientID uint64   `json:"client_id"`
	Symbol   string   `json:"symbol"`
	Types    []MDType `json:"types"`
}

// IndexPrice is a venue's published reference price for a symbol.
type IndexPrice struct {
	Symbol    string `json:"symbol"`
	Price     int64  `json:"price"`
	Timestamp int64  `json:"timestamp"`
}

type FundingRate struct {
	Symbol         string `json:"symbol"`
	Rate           int64  `json:"rate"`
	NextFunding    int64  `json:"next_funding"`
	Interval       int64  `json:"interval"`
	MarkPrice      int64  `json:"mark_price"`
	MarkAvailable  bool   `json:"mark_available"`
	IndexPrice     int64  `json:"index_price"`
	IndexAvailable bool   `json:"index_available"`
}

type OpenInterest struct {
	Symbol         string `json:"symbol"`
	TotalContracts int64  `json:"total_contracts"`
	Timestamp      int64  `json:"timestamp"`
}
