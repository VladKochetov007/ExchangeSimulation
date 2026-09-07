package analysis

import (
	"encoding/binary"
	"reflect"
	"testing"
)

// H-012. The participant-information audit has two implementations.
// AuditMarketDataReceipts delegates to auditMarketDataReceiptsStreaming;
// auditMarketDataReceiptsBuffered is retained as a review oracle for the
// transition. They are not a refactor of each other: DuplicateSource is a map
// lookup in one and an external merge sort over spilled runs in the other, and
// MissingDueReceipt is a map iteration in one and a disk-backed scan in the
// other.
//
// The existing tests compare the two on a clean fixture and run the adversarial
// mutations through the streaming path alone. That exercises the oracle exactly
// where the two paths are least likely to differ. Two implementations of the
// same detector diverge under fault, not under health, and Valid gates evidence
// the campaign depends on: if they disagree, whether a fault is caught depends
// on which path ran.
//
// This test therefore drives faults through BOTH and requires byte-identical
// audit results. It deliberately spans the counters whose implementations
// differ most, and asserts on equality rather than on an expected count, so it
// stays correct if the audit's semantics are later extended.
// retentionDivergence pins the one place the two implementations legitimately
// differ: the streaming auditor's schedule spill is bounded, so it retains a
// schedule only while its per-link ordinal is in sequence, while the buffered
// oracle keeps every schedule in a map. A receipt whose schedule was dropped is
// therefore "without schedule" for one and "mismatched" for the other. Both
// still reject the evidence, which is the property that matters.
type retentionDivergence struct {
	streamingWithoutSchedule int64
	streamingMismatch        int64
	bufferedWithoutSchedule  int64
	bufferedMismatch         int64
}

func TestAuditMarketDataEvidenceOracleAgreesUnderFaults(t *testing.T) {
	const (
		scheduleTwo = marketDataScheduleRecordBytes
		receiptTwo  = marketDataReceiptRecordBytes
		decisionTwo = marketDataDecisionRecordBytes
	)
	faults := []struct {
		name   string
		mutate func(schedules, receipts, decisions []byte)
		// retention pins a fault that both implementations reject but classify
		// differently, because the streaming auditor retains a schedule only
		// while it is in sequence and the buffered oracle keeps every one. The
		// divergence is pinned rather than erased.
		retention *retentionDivergence
	}{
		{name: "clean control", mutate: nil},
		{
			// The sharp case for RT-007. Only the storage order of the file
			// changes: the multiset of event ordinals is untouched and every
			// record stays internally consistent. Before the oracle merged
			// instead of sorting, it reported this evidence Valid while the
			// production path rejected it.
			name: "records are stored out of event order",
			retention: &retentionDivergence{
				streamingWithoutSchedule: 2, streamingMismatch: 0,
				bufferedWithoutSchedule: 1, bufferedMismatch: 0,
			},
			mutate: func(s, _, _ []byte) {
				first := make([]byte, scheduleTwo)
				copy(first, s[:scheduleTwo])
				copy(s[:scheduleTwo], s[scheduleTwo:2*scheduleTwo])
				copy(s[scheduleTwo:2*scheduleTwo], first)
			},
		},
		{
			// The frontier a decision cites no longer matches the chain the
			// receipt stream produces.
			name: "decision cites a frontier digest that was never produced",
			mutate: func(_, _, d []byte) {
				for index := 56; index < 72; index++ {
					d[decisionTwo+index] = 0
				}
			},
		},
		{
			name: "decision cites an observation delivered after it acted",
			mutate: func(_, r, d []byte) {
				binary.BigEndian.PutUint64(r[receiptTwo+60:receiptTwo+68], 400)
				binary.BigEndian.PutUint64(d[decisionTwo+48:decisionTwo+56], 400)
				binary.BigEndian.PutUint64(d[decisionTwo+32:decisionTwo+40], 140)
			},
		},
		{
			name: "decision arrives on a link the catalog does not declare",
			mutate: func(_, _, d []byte) {
				binary.BigEndian.PutUint32(d[8:12], 99)
			},
		},
		{
			name: "decision names a symbol the catalog does not declare",
			mutate: func(_, _, d []byte) {
				binary.BigEndian.PutUint32(d[12:16], 7)
			},
		},
		{
			name: "global event order is permuted",
			mutate: func(s, _, _ []byte) {
				binary.BigEndian.PutUint64(s[76:84], 4)
				binary.BigEndian.PutUint64(s[scheduleTwo+76:scheduleTwo+84], 1)
			},
		},
		{
			// Exercises the disk-backed schedule scan against the map.
			name: "receipt claims an ordinal that was never scheduled",
			mutate: func(_, r, _ []byte) {
				binary.BigEndian.PutUint64(r[receiptTwo+68:receiptTwo+76], 3)
			},
		},
		{
			name: "a due observation is never delivered",
			mutate: func(_, r, _ []byte) {
				for index := range r[receiptTwo:] {
					r[receiptTwo+index] = 0
				}
			},
		},
		{
			// Exercises the external merge sort against the map.
			name: "two observations share one source identity",
			mutate: func(s, r, _ []byte) {
				copy(s[scheduleTwo+20:scheduleTwo+28], s[20:28])
				copy(s[scheduleTwo+28:scheduleTwo+44], s[28:44])
				copy(r[receiptTwo+20:receiptTwo+28], r[20:28])
				copy(r[receiptTwo+28:receiptTwo+44], r[28:44])
			},
		},
		{
			name: "source fingerprints are zeroed",
			mutate: func(s, r, _ []byte) {
				for index := 28; index < 44; index++ {
					s[index], r[index] = 0, 0
					s[scheduleTwo+index], r[receiptTwo+index] = 0, 0
				}
			},
		},
		{
			name: "an observation is scheduled before it was published",
			mutate: func(s, r, _ []byte) {
				binary.BigEndian.PutUint64(s[44:52], 100)
				binary.BigEndian.PutUint64(s[52:60], 99)
				binary.BigEndian.PutUint64(r[44:52], 100)
				binary.BigEndian.PutUint64(r[52:60], 99)
				binary.BigEndian.PutUint64(r[60:68], 99)
			},
		},
		{
			name: "an observation is delivered before it was scheduled",
			mutate: func(_, r, _ []byte) {
				binary.BigEndian.PutUint64(r[60:68], 100)
			},
		},
		{
			name: "market-data type is outside the declared domain",
			mutate: func(s, r, _ []byte) {
				s[16], r[16] = 9, 9
			},
		},
		{
			// Both implementations reject this, under different counters. The
			// streaming auditor retains a schedule only while it is in
			// sequence, because its spill is bounded; the buffered oracle keeps
			// every schedule in a map. So the out-of-sequence schedule is
			// missing for one and present-but-wrong for the other. That is a
			// legitimate consequence of bounded memory, not a defect, and it is
			// pinned here so a future change that turns it into a disagreement
			// about validity fails loudly.
			name: "schedule ordinals are reordered",
			retention: &retentionDivergence{
				streamingWithoutSchedule: 2, streamingMismatch: 0,
				bufferedWithoutSchedule: 1, bufferedMismatch: 1,
			},
			mutate: func(s, _, _ []byte) {
				binary.BigEndian.PutUint64(s[68:76], 2)
				binary.BigEndian.PutUint64(s[scheduleTwo+68:scheduleTwo+76], 1)
			},
		},
		{
			name: "reserved decision bytes carry payload",
			mutate: func(_, _, d []byte) {
				d[20] = 1
			},
		},
	}

	for _, fault := range faults {
		t.Run(fault.name, func(t *testing.T) {
			dir := writeEvidenceFixture(t, fault.mutate)
			streaming, err := AuditMarketDataReceipts(dir)
			if err != nil {
				t.Fatalf("streaming audit failed: %v", err)
			}
			buffered, err := auditMarketDataReceiptsBuffered(dir)
			if err != nil {
				t.Fatalf("buffered oracle failed: %v", err)
			}
			// The invariant that matters: an oracle may name a fault
			// differently, but it must never disagree about whether the
			// evidence is admissible.
			if streaming.Valid != buffered.Valid {
				t.Fatalf("the two audit implementations disagree about validity, so whether this fault is caught depends on which path ran:\nstreaming=%+v\nbuffered=%+v",
					streaming, buffered)
			}
			if fault.retention != nil {
				if streaming.ReceiptWithoutSchedule != fault.retention.streamingWithoutSchedule ||
					streaming.ScheduleMismatch != fault.retention.streamingMismatch ||
					buffered.ReceiptWithoutSchedule != fault.retention.bufferedWithoutSchedule ||
					buffered.ScheduleMismatch != fault.retention.bufferedMismatch {
					t.Errorf("the pinned schedule-retention divergence changed shape:\nstreaming=%+v\nbuffered=%+v",
						streaming, buffered)
				}
				// Neutralised so every other counter is still compared exactly.
				streaming.ReceiptWithoutSchedule, streaming.ScheduleMismatch = 0, 0
				buffered.ReceiptWithoutSchedule, buffered.ScheduleMismatch = 0, 0
			}
			if !reflect.DeepEqual(streaming, buffered) {
				t.Fatalf("the two audit implementations classify this fault differently:\nstreaming=%+v\nbuffered=%+v",
					streaming, buffered)
			}
			// A mutation that neither path notices is an audit-coverage gap
			// rather than an agreement result, and must not be reported as one.
			if fault.mutate != nil && streaming.Valid {
				t.Errorf("both implementations agree, but neither detected the fault: %+v", streaming)
			}
			if fault.mutate == nil && !streaming.Valid {
				t.Errorf("control fixture rejected: %+v", streaming)
			}
		})
	}
}
