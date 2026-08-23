package analysis

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type fixtureEvidenceManifest struct {
	SchemaVersion int              `json:"schema_version"`
	Domain        string           `json:"domain"`
	Ordering      string           `json:"ordering"`
	TerminalAt    int64            `json:"terminal_at"`
	Schedules     fixtureFile      `json:"schedules"`
	Receipts      fixtureFile      `json:"receipts"`
	Decisions     fixtureFile      `json:"decisions"`
	Links         []map[string]any `json:"links"`
	Symbols       []map[string]any `json:"symbols"`
}

type fixtureFile struct {
	File    string `json:"file"`
	Records int64  `json:"records"`
	Digest  string `json:"digest"`
}

func TestAuditMarketDataEvidenceAcceptsIndependentValidFixture(t *testing.T) {
	audit, err := AuditMarketDataReceipts(writeEvidenceFixture(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !audit.Valid || audit.Schedules != 2 || audit.Receipts != 2 || audit.Decisions != 2 {
		t.Fatalf("valid V2-0 evidence rejected: %+v", audit)
	}
	if len(audit.LinkActivity) != 2 || audit.LinkActivity[0] != (MarketDataLinkActivity{
		LinkID: 1, SourceVenue: "north", Link: "north/spot_maker/client/1", Role: "spot_maker",
		Schedules: 2, Receipts: 2, Decisions: 2,
	}) || audit.LinkActivity[1] != (MarketDataLinkActivity{
		LinkID: 2, SourceVenue: "south", Link: "south/v2_remote_feed/north/maker_1/client/2", Role: "v2_remote_feed",
	}) {
		t.Fatalf("link activity = %+v, want active and visibly inactive links", audit.LinkActivity)
	}
}

// Each mutation rewrites every file digest. The auditor must detect broken
// semantics, not merely a checksum mismatch.
func TestAuditMarketDataEvidenceCatchesAdversarialMutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(schedules, receipts, decisions []byte)
		caught func(*MarketDataReceiptAudit) bool
	}{
		{
			name: "future injected observation",
			mutate: func(s, r, _ []byte) {
				binary.BigEndian.PutUint64(s[44:52], 100)
				binary.BigEndian.PutUint64(s[52:60], 99)
				binary.BigEndian.PutUint64(r[44:52], 100)
				binary.BigEndian.PutUint64(r[52:60], 99)
				binary.BigEndian.PutUint64(r[60:68], 99)
			},
			caught: func(a *MarketDataReceiptAudit) bool { return a.ScheduledBeforePub > 0 },
		},
		{
			name: "dropped due observation",
			mutate: func(_, r, _ []byte) {
				for i := range r[88:] {
					r[88+i] = 0
				}
			},
			caught: func(a *MarketDataReceiptAudit) bool { return a.MissingDueReceipt > 0 || a.BadReceiptOrdinal > 0 },
		},
		{
			name: "delayed observation used early",
			mutate: func(_, r, d []byte) {
				binary.BigEndian.PutUint64(r[60:68], 130)
				binary.BigEndian.PutUint64(d[32:40], 120)
			},
			caught: func(a *MarketDataReceiptAudit) bool { return a.FutureDecisionUse > 0 || a.BadDecisionFrontier > 0 },
		},
		{
			name: "duplicate source identity",
			mutate: func(s, r, _ []byte) {
				copy(s[88+20:88+28], s[20:28])
				copy(s[88+28:88+44], s[28:44])
				copy(r[88+20:88+28], r[20:28])
				copy(r[88+28:88+44], r[28:44])
			},
			caught: func(a *MarketDataReceiptAudit) bool { return a.DuplicateSource > 0 },
		},
		{
			name: "reordered schedule ordinal",
			mutate: func(s, _, _ []byte) {
				binary.BigEndian.PutUint64(s[68:76], 2)
				binary.BigEndian.PutUint64(s[88+68:88+76], 1)
			},
			caught: func(a *MarketDataReceiptAudit) bool { return a.BadScheduleOrdinal > 0 || a.ScheduleMismatch > 0 },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := writeEvidenceFixture(t, test.mutate)
			audit, err := AuditMarketDataReceipts(dir)
			if err != nil {
				t.Fatal(err)
			}
			if audit.Valid || !audit.ScheduleDigestMatches || !audit.ReceiptDigestMatches || !audit.DecisionDigestMatches || !test.caught(audit) {
				t.Fatalf("mutation survived: %+v", audit)
			}
		})
	}
}

func writeEvidenceFixture(t *testing.T, mutate func(schedules, receipts, decisions []byte)) string {
	t.Helper()
	dir := t.TempDir()
	schedules := make([]byte, 2*marketDataScheduleRecordBytes)
	receipts := make([]byte, 2*marketDataReceiptRecordBytes)
	decisions := make([]byte, 2*marketDataDecisionRecordBytes)
	writeObservation(schedules[:88], 1, 1, 1, 11, 100, 110, 0, 1, 1)
	writeObservation(receipts[:88], 1, 1, 1, 11, 100, 110, 110, 1, 2)
	writeObservation(schedules[88:], 1, 1, 1, 12, 120, 130, 0, 2, 4)
	writeObservation(receipts[88:], 1, 1, 1, 12, 120, 130, 130, 2, 5)
	writeDecision(decisions[:96], receipts[:88], 1, 1, 1, 1001, 115, 1, 110, 3)
	writeDecision(decisions[96:], receipts, 1, 1, 1, 1002, 140, 2, 130, 6)
	if mutate != nil {
		mutate(schedules, receipts, decisions)
	}
	writeFixtureFile(t, dir, "market-data-schedules-v2.bin", schedules)
	writeFixtureFile(t, dir, "market-data-receipts-v2.bin", receipts)
	writeFixtureFile(t, dir, "market-data-decisions-v2.bin", decisions)
	manifest := fixtureEvidenceManifest{
		SchemaVersion: 2, Domain: "participant_information_boundary_v2", Ordering: "per_link_fifo_schedule_receipt_decision", TerminalAt: 200,
		Schedules: fixtureArtifact("market-data-schedules-v2.bin", schedules, 88),
		Receipts:  fixtureArtifact("market-data-receipts-v2.bin", receipts, 88),
		Decisions: fixtureArtifact("market-data-decisions-v2.bin", decisions, 96),
		Links: []map[string]any{
			{"id": 1, "source_venue": "north", "link": "north/spot_maker/client/1", "role": "spot_maker"},
			{"id": 2, "source_venue": "south", "link": "south/v2_remote_feed/north/maker_1/client/2", "role": "v2_remote_feed"},
		},
		Symbols: []map[string]any{{"id": 1, "symbol": "ABC/USD"}},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "market-data-evidence-v2.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeObservation(raw []byte, client uint64, link, symbol uint32, sequence uint64, published, scheduled, delivered int64, ordinal, event uint64) {
	binary.BigEndian.PutUint64(raw[0:8], client)
	binary.BigEndian.PutUint32(raw[8:12], link)
	binary.BigEndian.PutUint32(raw[12:16], symbol)
	raw[16] = 0
	binary.BigEndian.PutUint64(raw[20:28], sequence)
	raw[28] = byte(sequence)
	binary.BigEndian.PutUint64(raw[44:52], uint64(published))
	binary.BigEndian.PutUint64(raw[52:60], uint64(scheduled))
	binary.BigEndian.PutUint64(raw[60:68], uint64(delivered))
	binary.BigEndian.PutUint64(raw[68:76], ordinal)
	binary.BigEndian.PutUint64(raw[76:84], event)
}

func writeDecision(raw, receipts []byte, client uint64, link, symbol uint32, request uint64, decisionAt, frontierOrdinal, deliveredAt, event uint64) {
	binary.BigEndian.PutUint64(raw[0:8], client)
	binary.BigEndian.PutUint32(raw[8:12], link)
	binary.BigEndian.PutUint32(raw[12:16], symbol)
	binary.BigEndian.PutUint64(raw[24:32], request)
	binary.BigEndian.PutUint64(raw[32:40], decisionAt)
	binary.BigEndian.PutUint64(raw[40:48], frontierOrdinal)
	binary.BigEndian.PutUint64(raw[48:56], deliveredAt)
	var digest [16]byte
	for offset := 0; offset < int(frontierOrdinal)*marketDataReceiptRecordBytes; offset += marketDataReceiptRecordBytes {
		chain := sha256.New()
		_, _ = chain.Write(digest[:])
		_, _ = chain.Write(receipts[offset : offset+marketDataReceiptRecordBytes])
		copy(digest[:], chain.Sum(nil))
	}
	copy(raw[56:72], digest[:])
	binary.BigEndian.PutUint64(raw[72:80], 50_000)
	binary.BigEndian.PutUint64(raw[80:88], 1)
	binary.BigEndian.PutUint64(raw[88:96], event)
}

func fixtureArtifact(name string, raw []byte, recordBytes int) fixtureFile {
	digest := sha256.Sum256(raw)
	return fixtureFile{File: name, Records: int64(len(raw) / recordBytes), Digest: hex.EncodeToString(digest[:])}
}

func writeFixtureFile(t *testing.T, dir, name string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0644); err != nil {
		t.Fatal(err)
	}
}
