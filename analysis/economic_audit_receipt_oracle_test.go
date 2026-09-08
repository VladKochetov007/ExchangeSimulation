package analysis

import "testing"

func TestAuditMarketDataEvidenceOracleRejectsStoredOutOfOrderRecords(t *testing.T) {
	dir := writeEvidenceFixture(t, func(schedules, _, _ []byte) {
		first := append([]byte(nil), schedules[:marketDataScheduleRecordBytes]...)
		copy(schedules[:marketDataScheduleRecordBytes], schedules[marketDataScheduleRecordBytes:])
		copy(schedules[marketDataScheduleRecordBytes:], first)
	})

	streaming, err := AuditMarketDataReceipts(dir)
	if err != nil {
		t.Fatalf("streaming audit failed: %v", err)
	}
	buffered, err := auditMarketDataReceiptsBuffered(dir)
	if err != nil {
		t.Fatalf("buffered audit failed: %v", err)
	}
	if streaming.Valid {
		t.Fatalf("streaming audit accepted records stored out of event order: %+v", streaming)
	}
	if buffered.Valid {
		t.Fatalf("buffered oracle repaired records stored out of event order: %+v", buffered)
	}
}
