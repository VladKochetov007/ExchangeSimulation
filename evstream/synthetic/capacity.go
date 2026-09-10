// Package synthetic contains deterministic, outcome-neutral workloads used to
// measure the storage and resource cost of the promoted binary evidence
// contract. It has no simulator, exchange, actor, or market-state dependency.
package synthetic

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"strconv"

	"exchange_sim/evstream"
	"exchange_sim/evstream/exsim"
)

const (
	// Contract identifies the fixed synthetic capacity workload contract.
	Contract = "v2-r2-sv1d-synthetic-capacity-workload-v1"
	// EvidenceContract identifies the persisted binary evidence contract being
	// measured. It must remain identical to the production global sink.
	EvidenceContract = "route_and_global_sequence_neutral_v2"
	// DefaultProfileName is the only profile accepted by the registered
	// capacity runner.
	DefaultProfileName = "sv1d-production-mix-v1"
	// DefaultWorkloadSeed is deliberately distinct from every activation seed.
	DefaultWorkloadSeed uint64 = 2026091001
	// DefaultStartNano and DefaultEndNano are the registered 24-hour research
	// time interval. They are timestamps for synthetic frames, not a simulator
	// world and not evidence of a terminal state.
	DefaultStartNano int64 = 1735689600000000000
	DefaultEndNano   int64 = 1735776000000000000
	// DefaultBookDeltaPerHour is the retained high-frequency planning rate used
	// to precommit the event volume.
	DefaultBookDeltaPerHour uint64 = 870000
	DefaultHours            uint64 = 24
)

// Profile is the complete deterministic input to a synthetic workload.
// Counts are explicit so a report can be independently reconstructed without
// replaying the generator.
type Profile struct {
	Name                   string `json:"name"`
	WorkloadSeed           uint64 `json:"workload_seed"`
	StartNano              int64  `json:"start_nano"`
	EndNano                int64  `json:"end_nano"`
	EventCount             uint64 `json:"event_count"`
	BookDeltaEvents        uint64 `json:"book_delta_events"`
	BalanceChangeEvents    uint64 `json:"balance_change_events"`
	OpaqueScientificEvents uint64 `json:"opaque_scientific_events"`
}

// NewProfile creates a profile with the registered 80/10/10 event-family
// pattern. It is also used by small deterministic unit tests; the production
// runner uses ProductionProfile instead.
func NewProfile(eventCount uint64) Profile {
	fullBuckets := eventCount / 10
	remainder := eventCount % 10
	book := fullBuckets * 8
	balance := fullBuckets
	opaque := fullBuckets
	if remainder <= 8 {
		book += remainder
	} else if remainder == 9 {
		book += 8
		balance++
	}
	return Profile{
		Name:                   DefaultProfileName,
		WorkloadSeed:           DefaultWorkloadSeed,
		StartNano:              DefaultStartNano,
		EndNano:                DefaultEndNano,
		EventCount:             eventCount,
		BookDeltaEvents:        book,
		BalanceChangeEvents:    balance,
		OpaqueScientificEvents: opaque,
	}
}

// ProductionProfile is the preregistered 24-hour workload. It represents
// 20,880,000 high-frequency BookDelta frames plus equal-sized accounting and
// lower-frequency event buckets.
func ProductionProfile() Profile {
	return NewProfile(DefaultBookDeltaPerHour * DefaultHours * 10 / 8)
}

// Validate checks the profile independently of the writer. A capacity profile
// must be finite, use the fixed event mix, and never use activation seed 659.
func (p Profile) Validate() error {
	if p.Name != DefaultProfileName {
		return fmt.Errorf("synthetic: unsupported profile %q", p.Name)
	}
	if p.WorkloadSeed == 0 || p.WorkloadSeed == 659 {
		return errors.New("synthetic: workload seed is missing or reserved for activation")
	}
	if p.StartNano >= p.EndNano || p.EventCount == 0 {
		return errors.New("synthetic: invalid time interval or empty workload")
	}
	if p.BookDeltaEvents+p.BalanceChangeEvents+p.OpaqueScientificEvents != p.EventCount {
		return errors.New("synthetic: family counts do not sum to event count")
	}
	expected := NewProfile(p.EventCount)
	if p.BookDeltaEvents != expected.BookDeltaEvents ||
		p.BalanceChangeEvents != expected.BalanceChangeEvents ||
		p.OpaqueScientificEvents != expected.OpaqueScientificEvents {
		return errors.New("synthetic: family counts do not match the registered 80/10/10 pattern")
	}
	return nil
}

// Report is deterministic except for no runtime/resource fields. The shell
// capacity runner records those separately, so this object can be reproduced
// byte-for-byte on a fresh process.
type Report struct {
	SchemaVersion       int       `json:"schema_version"`
	Contract            string    `json:"contract"`
	Profile             Profile   `json:"profile"`
	EvidenceFormat      string    `json:"evidence_format"`
	Hashing             string    `json:"hashing"`
	Ordering            string    `json:"ordering"`
	EventFrames         uint64    `json:"event_frames"`
	StreamFrames        uint64    `json:"stream_frames"`
	StreamBytes         uint64    `json:"stream_bytes"`
	ExecutionStreamHash string    `json:"execution_stream_hash"`
	CanonicalStreamHash string    `json:"canonical_stream_hash"`
	FamilyCounts        [3]uint64 `json:"family_counts"`
	UnencodablePayloads uint64    `json:"unencodable_payloads"`
	ReadbackVerified    bool      `json:"readback_verified"`
}

// HashGlobalFrame is the execution-hash projection used by the production
// global binary sink. Route-local and global ordinals are persistence metadata;
// all other canonical bytes remain part of the identity.
func HashGlobalFrame(digest hash.Hash, frame []byte) {
	header, err := evstream.ParseFrameHeader(frame)
	if err != nil || header.SchemaID == evstream.SchemaDictionary || len(frame) < evstream.FrameHeaderSize+24 {
		_, _ = digest.Write(frame)
		return
	}
	sequenceStart := evstream.FrameHeaderSize + 8
	globalSequenceEnd := sequenceStart + 16
	_, _ = digest.Write(frame[:sequenceStart])
	var zeroSequences [16]byte
	_, _ = digest.Write(zeroSequences[:])
	_, _ = digest.Write(frame[globalSequenceEnd:])
}

// Write produces one complete, canonical synthetic stream and returns its
// deterministic report. The caller owns and closes out.
func Write(out io.Writer, profile Profile) (Report, error) {
	if out == nil {
		return Report{}, errors.New("synthetic: nil output")
	}
	if err := profile.Validate(); err != nil {
		return Report{}, err
	}
	counted := &countingWriter{out: out}
	writer := evstream.NewWriter(counted, evstream.WriterOptions{HashFrame: HashGlobalFrame})

	venueRefs, routeRefs, eventRefs, err := internNames(writer)
	if err != nil {
		return Report{}, err
	}
	state := profile.WorkloadSeed
	var book exsim.EncodedBookDelta
	var balance exsim.EncodedBalanceChange
	balanceChanges := [2]exsim.BalanceDelta{}
	routeSequences := [3]uint64{}
	familyCounts := [3]uint64{}

	for eventIndex := uint64(0); eventIndex < profile.EventCount; eventIndex++ {
		timestamp := profile.StartNano + int64((uint64(profile.EndNano-profile.StartNano)*eventIndex)/profile.EventCount)
		slot := eventIndex % 10
		venueIndex := int(eventIndex % uint64(len(venueRefs)))
		var payload evstream.PayloadAppender
		var eventName string
		var routeIndex int
		var clientID uint64
		switch {
		case slot < 8:
			value := makeBookDelta(timestamp, nextValue(&state), eventIndex)
			if err := exsim.InternBookDelta(writer, value, &book); err != nil {
				return Report{}, err
			}
			payload = &book
			eventName = "book_delta"
			routeIndex = 0
			clientID = 0
			familyCounts[0]++
		case slot == 8:
			value := makeBalanceChange(timestamp, nextValue(&state), eventIndex, balanceChanges[:])
			if err := exsim.InternBalanceChange(writer, value, &balance); err != nil {
				return Report{}, err
			}
			payload = &balance
			eventName = "balance_change"
			routeIndex = 1
			clientID = 1 + eventIndex%128
			familyCounts[1]++
		default:
			opacity := makeOpaquePayload(timestamp, nextValue(&state), eventIndex)
			payload = opacity
			eventName = opacity.eventName()
			routeIndex = 2
			clientID = 1 + eventIndex%128
			familyCounts[2]++
		}

		routeSequences[routeIndex]++
		envelope := globalEnvelope{
			routeRef:       routeRefs[routeIndex],
			eventRef:       eventRefs[eventName],
			sequence:       routeSequences[routeIndex],
			globalSequence: eventIndex + 1,
			inner:          payload,
		}
		if err := writer.AppendInterning(timestamp, clientID, venueRefs[venueIndex], envelope); err != nil {
			return Report{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return Report{}, err
	}

	executionHash := writer.ExecutionHash()
	canonicalHash := writer.RawExecutionHash()
	return Report{
		SchemaVersion:       1,
		Contract:            Contract,
		Profile:             profile,
		EvidenceFormat:      "evstream_v3",
		Hashing:             EvidenceContract,
		Ordering:            "ordered_stream",
		EventFrames:         profile.EventCount,
		StreamFrames:        writer.Count(),
		StreamBytes:         uint64(counted.bytes),
		ExecutionStreamHash: hexDigest(executionHash),
		CanonicalStreamHash: hexDigest(canonicalHash),
		FamilyCounts:        familyCounts,
		ReadbackVerified:    false,
	}, nil
}

// Verify replays only the binary stream structure. It does not execute a
// market world; it proves termination, frame sequencing, envelope ordinals,
// and both canonical hashes against the deterministic report.
func Verify(in io.Reader, expected Report) error {
	if in == nil {
		return errors.New("synthetic: nil input")
	}
	reader, err := evstream.NewReader(in, evstream.ReaderOptions{VerifyHash: true, HashFrame: HashGlobalFrame})
	if err != nil {
		return err
	}
	var eventFrames uint64
	var expectedGlobal uint64
	if err := reader.Range(func(frame evstream.Frame) error {
		if frame.Header.SchemaID == evstream.SchemaDictionary {
			return nil
		}
		if len(frame.Payload) < 24 {
			return fmt.Errorf("synthetic: event frame %d has a short envelope", frame.Header.Seq)
		}
		routeRef := binary.LittleEndian.Uint32(frame.Payload[0:4])
		eventRef := binary.LittleEndian.Uint32(frame.Payload[4:8])
		routeSequence := binary.LittleEndian.Uint64(frame.Payload[8:16])
		globalSequence := binary.LittleEndian.Uint64(frame.Payload[16:24])
		if routeRef == 0 || eventRef == 0 || routeSequence == 0 {
			return fmt.Errorf("synthetic: incomplete envelope at frame %d", frame.Header.Seq)
		}
		expectedGlobal++
		if globalSequence != expectedGlobal {
			return fmt.Errorf("synthetic: global sequence %d, want %d", globalSequence, expectedGlobal)
		}
		eventFrames++
		return nil
	}); err != nil {
		return err
	}
	if !reader.Terminated() {
		return errors.New("synthetic: stream is not terminated")
	}
	if eventFrames != expected.EventFrames || reader.Count() != expected.StreamFrames {
		return fmt.Errorf("synthetic: frame count mismatch events=%d/%d stream=%d/%d", eventFrames, expected.EventFrames, reader.Count(), expected.StreamFrames)
	}
	if hexDigest(reader.ExecutionHash()) != expected.ExecutionStreamHash ||
		hexDigest(reader.RawExecutionHash()) != expected.CanonicalStreamHash {
		return errors.New("synthetic: readback hash mismatch")
	}
	return nil
}

type countingWriter struct {
	out   io.Writer
	bytes int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	n, err := w.out.Write(data)
	w.bytes += int64(n)
	return n, err
}

type globalEnvelope struct {
	routeRef       uint32
	eventRef       uint32
	sequence       uint64
	globalSequence uint64
	inner          evstream.PayloadAppender
}

func (e globalEnvelope) SchemaID() uint16      { return e.inner.SchemaID() }
func (e globalEnvelope) SchemaVersion() uint16 { return e.inner.SchemaVersion() }

func (e globalEnvelope) AppendPayloadInterning(dst []byte, in evstream.Interner) ([]byte, error) {
	dst = evstream.AppendUint32(dst, e.routeRef)
	dst = evstream.AppendUint32(dst, e.eventRef)
	dst = evstream.AppendUint64(dst, e.sequence)
	dst = evstream.AppendUint64(dst, e.globalSequence)
	_ = in
	return e.inner.AppendPayload(dst), nil
}

func internNames(writer *evstream.Writer) ([]uint32, map[int]uint32, map[string]uint32, error) {
	venues := []string{"north", "central", "south"}
	routes := []string{"market-data.jsonl", "accounting.jsonl", "derivatives.jsonl"}
	events := []string{"book_delta", "balance_change", "trade", "mark_price", "funding_settlement"}
	venueRefs := make([]uint32, len(venues))
	for i, name := range venues {
		ref, err := writer.Intern(name)
		if err != nil {
			return nil, nil, nil, err
		}
		venueRefs[i] = ref
	}
	routeRefs := make(map[int]uint32, len(routes))
	for i, name := range routes {
		ref, err := writer.Intern(name)
		if err != nil {
			return nil, nil, nil, err
		}
		routeRefs[i] = ref
	}
	eventRefs := make(map[string]uint32, len(events))
	for _, name := range events {
		ref, err := writer.Intern(name)
		if err != nil {
			return nil, nil, nil, err
		}
		eventRefs[name] = ref
	}
	return venueRefs, routeRefs, eventRefs, nil
}

func makeBookDelta(timestamp int64, value uint64, index uint64) exsim.BookDelta {
	symbols := [...]string{"ABC/USD", "CDF/USD", "ABC/CDF", "ABC-PERP", "ABC-FUT-1735696801"}
	return exsim.BookDelta{
		Timestamp:  timestamp,
		Symbol:     symbols[index%uint64(len(symbols))],
		Side:       uint8(index % 2),
		Price:      int64(100000000 + value%900000000),
		VisibleQty: int64(100000 + value%900000),
		HiddenQty:  int64(value % 10000),
		TotalQty:   int64(100000 + value%1000000),
	}
}

func makeBalanceChange(timestamp int64, value uint64, index uint64, changes []exsim.BalanceDelta) exsim.BalanceChange {
	changeCount := int(index%2) + 1
	for i := 0; i < changeCount; i++ {
		oldBalance := int64(100000000 + (value+uint64(i)*7919)%100000000)
		delta := int64((value+uint64(i)*104729)%1000000) - 500000
		changes[i] = exsim.BalanceDelta{
			Asset:      []string{"USD", "CDF"}[(index+uint64(i))%2],
			Wallet:     []string{"available", "margin"}[(index+uint64(i))%2],
			OldBalance: oldBalance,
			NewBalance: oldBalance + delta,
			Delta:      delta,
		}
	}
	return exsim.BalanceChange{
		Timestamp:    timestamp,
		ClientID:     1 + index%128,
		Symbol:       []string{"CDF/USD", "ABC-PERP"}[index%2],
		PositionSide: []string{"BOTH", "LONG"}[index%2],
		HasSide:      index%3 != 0,
		Reason:       []string{"trade", "funding", "settlement"}[index%3],
		Changes:      changes[:changeCount],
	}
}

func makeOpaquePayload(timestamp int64, value, index uint64) opaquePayload {
	return opaquePayload{timestamp: timestamp, value: value, index: index}
}

type opaquePayload struct {
	timestamp int64
	value     uint64
	index     uint64
}

func (p opaquePayload) eventName() string {
	switch p.index % 4 {
	case 0:
		return "trade"
	case 1:
		return "mark_price"
	case 2:
		return "funding_settlement"
	default:
		return "trade"
	}
}

func (p opaquePayload) SchemaID() uint16      { return evstream.SchemaOpaqueJSON }
func (p opaquePayload) SchemaVersion() uint16 { return 1 }

func (p opaquePayload) AppendPayload(dst []byte) []byte {
	encoded, _ := p.AppendPayloadInterning(dst, nil)
	return encoded
}

func (p opaquePayload) AppendPayloadInterning(dst []byte, _ evstream.Interner) ([]byte, error) {
	lengthOffset := len(dst)
	dst = append(dst, 0, 0, 0, 0)
	jsonStart := len(dst)
	dst = append(dst, '{')
	dst = appendJSONKeyInt(dst, "timestamp", p.timestamp, true)
	dst = appendJSONKeyString(dst, "symbol", "CDF/USD", false)
	dst = appendJSONKeyUint(dst, "id", p.index+1, false)
	dst = appendJSONKeyInt(dst, "price", int64(100000000+p.value%900000000), false)
	dst = appendJSONKeyInt(dst, "qty", int64(100000+p.value%1000000), false)
	dst = appendJSONKeyString(dst, "state", p.eventName(), false)
	dst = append(dst, '}')
	binary.LittleEndian.PutUint32(dst[lengthOffset:lengthOffset+4], uint32(len(dst)-jsonStart))
	return dst, nil
}

func appendJSONKeyInt(dst []byte, key string, value int64, first bool) []byte {
	dst = appendJSONKeyPrefix(dst, key, first)
	return strconv.AppendInt(dst, value, 10)
}

func appendJSONKeyUint(dst []byte, key string, value uint64, first bool) []byte {
	dst = appendJSONKeyPrefix(dst, key, first)
	return strconv.AppendUint(dst, value, 10)
}

func appendJSONKeyString(dst []byte, key, value string, first bool) []byte {
	dst = appendJSONKeyPrefix(dst, key, first)
	dst = append(dst, '"')
	dst = append(dst, value...)
	return append(dst, '"')
}

func appendJSONKeyPrefix(dst []byte, key string, first bool) []byte {
	if !first {
		dst = append(dst, ',')
	}
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"')
	return append(dst, ':')
}

func nextValue(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func hexDigest(digest [sha256.Size]byte) string {
	const hex = "0123456789abcdef"
	encoded := make([]byte, len(digest)*2)
	for i, value := range digest {
		encoded[i*2] = hex[value>>4]
		encoded[i*2+1] = hex[value&0x0f]
	}
	return string(encoded)
}
