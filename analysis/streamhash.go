package analysis

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"sync"
)

// Evidence multiset hash.
//
// Two runs of the same commit, config and seed must produce the same events.
// Comparing log files directly cannot show that, because the files are written
// by several goroutines and their interleaving is not part of the model. What
// is part of the model is the multiset of events: which events happened, at
// which simulated instants, carrying which payloads.
//
// This is deliberately not an execution-stream hash: the persisted evidence
// is partitioned across concurrently written files and has no global causal
// order. The digest is order-independent by construction. Each event line is
// hashed, and the digests are summed as 256-bit integers with wraparound. Two
// runs agree exactly when they emitted the same events the same number of
// times, whatever order the writers happened to run in. A difference in any
// price, quantity, timestamp or count moves the sum.
//
// Summation rather than XOR because XOR cannot see a duplicated pair: emitting
// one event twice and never emitting it are the same under XOR and different
// under addition.

// StreamHashOptions selects inputs.
type StreamHashOptions struct {
	Files         []string
	FilesSelected bool
	// PerEvent reports a separate digest for each event name, which localises
	// a divergence to the mechanism that produced it.
	PerEvent bool
}

// EventDigest is one event name's contribution.
type EventDigest struct {
	Event  string `json:"event"`
	Count  int64  `json:"count"`
	Digest string `json:"digest"`
}

// StreamHash is the whole digest.
type StreamHash struct {
	// Domain and Ordering make the evidence/execution distinction explicit in
	// machine-readable artifacts.
	Domain string `json:"domain"`
	Ordering string `json:"ordering"`
	Events int64 `json:"events"`
	// Digest identifies the persisted evidence multiset.
	Digest string `json:"digest"`
	// ByEvent is present when PerEvent was requested, sorted by event name.
	ByEvent []EventDigest `json:"by_event,omitempty"`
	// ByVenue localises a divergence to one venue.
	ByVenue []EventDigest `json:"by_venue,omitempty"`
}

// sum256 is a 256-bit accumulator over event digests.
type sum256 struct {
	limbs [4]uint64
	count int64
}

func (s *sum256) add(digest [32]byte) {
	var carry uint64
	for i := 3; i >= 0; i-- {
		limb := binary.BigEndian.Uint64(digest[i*8 : i*8+8])
		sum := s.limbs[i] + limb
		newCarry := uint64(0)
		if sum < s.limbs[i] {
			newCarry = 1
		}
		sum += carry
		if sum < carry {
			newCarry = 1
		}
		s.limbs[i] = sum
		carry = newCarry
	}
	s.count++
}

func (s *sum256) hex() string {
	var out [32]byte
	for i := 0; i < 4; i++ {
		binary.BigEndian.PutUint64(out[i*8:i*8+8], s.limbs[i])
	}
	return hex.EncodeToString(out[:])
}

// ForEachRaw visits every event with the fields the canonical digest is built
// from. It exists so a divergence between two runs can be localised to an
// instant and an event without re-deriving the canonical form elsewhere.
// Called concurrently, like Scan.
func (r *Run) ForEachRaw(visit func(ts int64, name, venue, symbol string, clientID uint64, payload []byte)) error {
	return r.Scan(ScanOptions{}, func(event Event) {
		visit(event.SimTS, event.Name, event.VenueID, event.Symbol, event.ClientID, event.payload)
	})
}

// MeasureStreamHash computes the canonical digest of a run's event stream.
func (r *Run) MeasureStreamHash(opts StreamHashOptions) (*StreamHash, error) {
	var mu sync.Mutex
	total := &sum256{}
	byEvent := map[string]*sum256{}
	byVenue := map[string]*sum256{}

	scan := ScanOptions{
		Files:         opts.Files,
		FilesSelected: opts.FilesSelected,
	}
	if err := r.Scan(scan, func(event Event) {
		digest := canonicalDigest(&event)
		mu.Lock()
		total.add(digest)
		if opts.PerEvent {
			acc := byEvent[event.Name]
			if acc == nil {
				acc = &sum256{}
				byEvent[event.Name] = acc
			}
			acc.add(digest)
			venue := byVenue[event.VenueID]
			if venue == nil {
				venue = &sum256{}
				byVenue[event.VenueID] = venue
			}
			venue.add(digest)
		}
		mu.Unlock()
	}); err != nil {
		return nil, err
	}

	result := &StreamHash{Domain: "persisted_evidence", Ordering: "unordered_multiset", Events: total.count, Digest: total.hex()}
	if opts.PerEvent {
		result.ByEvent = digestRows(byEvent)
		result.ByVenue = digestRows(byVenue)
	}
	return result, nil
}

// canonicalDigest hashes the parts of an event that belong to the model: when
// it happened, to whom, on which venue and book, and what it carried. The
// file it was written to and the order it was written in are properties of the
// logger rather than of the simulation and are deliberately excluded.
func canonicalDigest(event *Event) [32]byte {
	hasher := sha256.New()
	var scratch [8]byte
	binary.BigEndian.PutUint64(scratch[:], uint64(event.SimTS))
	hasher.Write(scratch[:])
	binary.BigEndian.PutUint64(scratch[:], event.ClientID)
	hasher.Write(scratch[:])
	hasher.Write([]byte(event.Name))
	hasher.Write([]byte{0})
	hasher.Write([]byte(event.VenueID))
	hasher.Write([]byte{0})
	hasher.Write([]byte(event.Symbol))
	hasher.Write([]byte{0})
	hasher.Write(event.payload)
	var out [32]byte
	copy(out[:], hasher.Sum(nil))
	return out
}

func digestRows(table map[string]*sum256) []EventDigest {
	rows := make([]EventDigest, 0, len(table))
	for name, acc := range table {
		rows = append(rows, EventDigest{Event: name, Count: acc.count, Digest: acc.hex()})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Event < rows[j].Event })
	return rows
}
