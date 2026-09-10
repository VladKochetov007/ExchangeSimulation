package analysis

import (
	"fmt"
	"sort"
)

// GlobalSequenceAudit summarizes validation of a rendered v2 evidence set.
// Dictionary frames are not rendered, so gaps between event sequences are
// valid; duplicate or zero identities are not.
type GlobalSequenceAudit struct {
	EventCount      uint64
	MaximumSequence uint64
}

// ValidateGlobalSequence checks the cross-file event identity emitted by the
// versioned binary renderer. expectedEventFrames and expectedStreamFrames are
// copied from the verified binary attestation; the method does not infer them
// from the rendered files.
func (r *Run) ValidateGlobalSequence(expectedEventFrames, expectedStreamFrames uint64) (GlobalSequenceAudit, error) {
	if r == nil {
		return GlobalSequenceAudit{}, fmt.Errorf("analysis: nil run")
	}
	sequences := make([]uint64, 0, expectedEventFrames)
	if err := r.Scan(ScanOptions{Workers: 1}, func(event Event) {
		if event.GlobalSequence == 0 {
			sequences = append(sequences, 0)
			return
		}
		sequences = append(sequences, event.GlobalSequence)
	}); err != nil {
		return GlobalSequenceAudit{}, err
	}
	if uint64(len(sequences)) != expectedEventFrames {
		return GlobalSequenceAudit{}, fmt.Errorf("analysis: rendered event count %d does not match binary attestation %d", len(sequences), expectedEventFrames)
	}
	for _, sequence := range sequences {
		if sequence == 0 {
			return GlobalSequenceAudit{}, fmt.Errorf("analysis: rendered v2 event has no global frame sequence")
		}
	}
	sort.Slice(sequences, func(left, right int) bool { return sequences[left] < sequences[right] })
	var maximumSequence uint64
	for index, sequence := range sequences {
		if index > 0 && sequence == sequences[index-1] {
			return GlobalSequenceAudit{}, fmt.Errorf("analysis: duplicate rendered global frame sequence %d", sequence)
		}
		if sequence > maximumSequence {
			maximumSequence = sequence
		}
	}
	if expectedStreamFrames > 0 && maximumSequence > expectedStreamFrames {
		return GlobalSequenceAudit{}, fmt.Errorf("analysis: rendered global frame sequence %d exceeds binary stream frame count %d", maximumSequence, expectedStreamFrames)
	}
	return GlobalSequenceAudit{EventCount: uint64(len(sequences)), MaximumSequence: maximumSequence}, nil
}
