package analysis

import (
	"encoding/json"
	"math"
	"testing"
)

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

func TestReactionKeepsSpotBooksApartWhenRecordsCarryNoSymbol(t *testing.T) {
	const horizonNano = int64(1e9)
	own := []string{
		symbolless("OrderFill", 0, 7, map[string]any{
			"price": 5000000000, "qty": 10, "side": "BUY", "role": "maker",
		}),
		symbolless("Trade", horizonNano, 0, map[string]any{
			"price": 5000500000, "qty": 1, "side": "BUY",
		}),
	}
	foreign := []string{
		symbolless("Trade", horizonNano, 0, map[string]any{
			"price": 300000000, "qty": 1, "side": "BUY",
		}),
	}

	run, err := Open(writeRun(t, Report{}, map[string][]string{
		"north/spot/ZZZ-USD.jsonl": own,
		"north/spot/AAA-USD.jsonl": foreign,
	}))
	if err != nil {
		t.Fatalf("open run: %v", err)
	}
	reaction, err := run.MeasureReaction(ReactionOptions{HorizonSeconds: 1, MaxReactionSeconds: 30})
	if err != nil {
		t.Fatalf("measure reaction: %v", err)
	}
	if len(reaction.Adverse) != 1 {
		t.Fatalf("expected one maker row, got %d: %+v", len(reaction.Adverse), reaction.Adverse)
	}
	const wantMarkoutBps = -1.0
	if math.Abs(reaction.Adverse[0].MeanMarkoutBps-wantMarkoutBps) > 0.001 {
		t.Fatalf("markout %.3f bps, want %.3f: the fill was marked against another book", reaction.Adverse[0].MeanMarkoutBps, wantMarkoutBps)
	}
}

func symbolless(event string, simTimestamp int64, clientID uint64, payload map[string]any) string {
	raw, err := json.Marshal(map[string]any{
		"client_id": clientID,
		"event":     event,
		"sim_ts":    simTimestamp,
		"data":      map[string]any{"venue_id": "north", "payload": payload},
	})
	if err != nil {
		panic(err)
	}
	return string(raw)
}
