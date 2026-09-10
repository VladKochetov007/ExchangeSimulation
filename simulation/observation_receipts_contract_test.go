package simulation

import "testing"

func TestReceiptRejectsEmptyMessageFingerprint(t *testing.T) {
	dir := t.TempDir()
	recorder, err := NewMarketDataReceiptRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	link := "north/spot_maker/client/7"
	schedule := MarketDataSchedule{
		ClientID: 7, SourceVenue: "north", Link: link, Symbol: "ABC/USD",
		PublishedAt: 1, ScheduledAt: 2, LinkOrdinal: 1,
	}
	if recorder.RegisterLink("north", link, "spot_maker") == 0 {
		t.Fatal("link registration failed")
	}
	if recorder.RecordReceipt(MarketDataReceipt{MarketDataSchedule: schedule, DeliveredAt: 2}) != (MarketDataFrontier{}) {
		t.Fatal("empty-fingerprint receipt returned a usable frontier")
	}
	if err := recorder.Finalize(2); err == nil {
		t.Fatal("empty-fingerprint receipt finalized successfully")
	}
}
