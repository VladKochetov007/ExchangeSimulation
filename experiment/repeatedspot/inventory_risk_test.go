package repeatedspot

import (
	"bytes"
	"math"
	"strings"
	"testing"
	"time"
)

func TestInventoryRiskExactWindowAndLimitOccupation(t *testing.T) {
	series := newInventoryRiskSeries(0, MeasurementWindow{StartAt: 5, EndAt: 15}, 2)
	for _, fill := range []struct{ at, delta int64 }{{2, 2}, {8, -1}, {12, -1}} {
		if err := series.add(fill.at, fill.delta); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := series.finish(20)
	if err != nil {
		t.Fatal(err)
	}
	if summary.WindowNanos != 10 || summary.SignedBaseUnitNanos != "10" ||
		summary.AbsoluteBaseUnitNanos != "10" || summary.SquaredBaseUnitNanos != "16" ||
		summary.MaxAbsoluteBaseUnits != "2" || summary.TimeAtFilledLimitNanos != 3 ||
		summary.TerminalNetFillBaseUnits != 0 {
		t.Fatalf("incorrect clipped inventory integrals: %+v", summary)
	}
	if err := series.add(19, 1); err == nil {
		t.Fatal("time regression after completion was accepted")
	}
}

func TestInventoryRiskOverflowFailsClosed(t *testing.T) {
	series := newInventoryRiskSeries(0, MeasurementWindow{StartAt: 0, EndAt: 10}, 0)
	if err := series.add(1, math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if err := series.add(2, 1); err == nil || !strings.Contains(err.Error(), "overflows") {
		t.Fatalf("overflow was accepted: %v", err)
	}
}

func TestReplayInventoryRiskSeparatesExchangeFromDelayedMaker(t *testing.T) {
	contract, events, directory := capturedMakerFill(t)
	streamReplay := func(window MeasurementWindow) *EconomicReplay {
		t.Helper()
		var raw bytes.Buffer
		recorder := NewRecorder(&raw)
		for _, event := range events {
			recorder.Record(event.Timestamp, event.ClientID, event.Source, event.Name, event.Route, event.Payload)
		}
		identity, err := recorder.Finish()
		if err != nil {
			t.Fatal(err)
		}
		replay, err := ReplayWindow(contract, bytes.NewReader(raw.Bytes()), identity, directory, window)
		if err != nil {
			t.Fatal(err)
		}
		return replay
	}
	window := MeasurementWindow{StartAt: 5 * int64(time.Second), EndAt: 9 * int64(time.Second)}
	replay := streamReplay(window)
	maker := replay.Accounts[1]
	if replay.MeasurementWindow != window || maker.InventoryRisk.Local == nil ||
		maker.InventoryRisk.Exchange.TerminalNetFillBaseUnits != -1 ||
		maker.InventoryRisk.Local.TerminalNetFillBaseUnits != -1 ||
		maker.InventoryRisk.Exchange.AbsoluteBaseUnitNanos != "3000000000" ||
		maker.InventoryRisk.Local.AbsoluteBaseUnitNanos != "2000000000" {
		t.Fatalf("venue exposure and delayed local belief were conflated: %+v", maker.InventoryRisk)
	}
}

func TestReplayRejectsMeasurementWindowOutsideWorld(t *testing.T) {
	contract, events, directory := capturedMakerFill(t)
	var raw bytes.Buffer
	recorder := NewRecorder(&raw)
	for _, event := range events {
		recorder.Record(event.Timestamp, event.ClientID, event.Source, event.Name, event.Route, event.Payload)
	}
	identity, err := recorder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range []MeasurementWindow{
		{StartAt: -1, EndAt: int64(time.Second)},
		{StartAt: 0, EndAt: 13 * int64(time.Second)},
		{StartAt: 6 * int64(time.Second), EndAt: 6 * int64(time.Second)},
	} {
		if _, err := ReplayWindow(contract, bytes.NewReader(raw.Bytes()), identity, directory, window); err == nil {
			t.Fatalf("invalid window %+v was accepted", window)
		}
	}
}
