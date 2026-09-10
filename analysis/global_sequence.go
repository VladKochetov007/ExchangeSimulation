package analysis

import (
	"fmt"
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
	// Do not use the attested count as an allocation hint. The attestation is an
	// input to this validator and may be malformed or adversarial; a corrupt
	// count must not turn a small rendered directory into a huge allocation.
	seen := make(map[uint64]struct{})
	var eventCount uint64
	var maximumSequence uint64
	var validationFailure error
	if err := r.Scan(ScanOptions{Workers: 1}, func(event Event) {
		if validationFailure != nil {
			return
		}
		if eventCount == ^uint64(0) {
			validationFailure = fmt.Errorf("analysis: rendered event count overflows uint64")
			return
		}
		eventCount++
		if event.GlobalSequence == 0 {
			validationFailure = fmt.Errorf("analysis: rendered v2 event has no global frame sequence")
			return
		}
		if _, duplicate := seen[event.GlobalSequence]; duplicate {
			validationFailure = fmt.Errorf("analysis: duplicate rendered global frame sequence %d", event.GlobalSequence)
			return
		}
		seen[event.GlobalSequence] = struct{}{}
		if event.GlobalSequence > maximumSequence {
			maximumSequence = event.GlobalSequence
		}
	}); err != nil {
		return GlobalSequenceAudit{}, err
	}
	if validationFailure != nil {
		return GlobalSequenceAudit{}, validationFailure
	}
	if eventCount != expectedEventFrames {
		return GlobalSequenceAudit{}, fmt.Errorf("analysis: rendered event count %d does not match binary attestation %d", eventCount, expectedEventFrames)
	}
	if expectedStreamFrames > 0 && maximumSequence > expectedStreamFrames {
		return GlobalSequenceAudit{}, fmt.Errorf("analysis: rendered global frame sequence %d exceeds binary stream frame count %d", maximumSequence, expectedStreamFrames)
	}
	return GlobalSequenceAudit{EventCount: eventCount, MaximumSequence: maximumSequence}, nil
}
