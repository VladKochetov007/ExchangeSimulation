package exchange

import (
	"testing"
)

type recordingLogger struct {
	records []logRecord
}

type logRecord struct {
	event string
	data  any
}

func (l *recordingLogger) LogEvent(_ int64, _ uint64, event string, data any) {
	l.records = append(l.records, logRecord{event: event, data: data})
}

func TestInstrumentLoggerFallbackDoesNotReplaceGlobalLogger(t *testing.T) {
	ex := NewExchangeWithConfig(ExchangeConfig{})
	global := &recordingLogger{}
	fallback := &recordingLogger{}
	ex.SetLogger("_global", global)
	ex.SetInstrumentLoggerFallback(fallback)

	if got := ex.getLogger("_global"); got != global {
		t.Fatal("global logger was replaced by fallback")
	}
	dynamic := ex.getLogger("DYNAMIC-OPTION")
	if dynamic == nil {
		t.Fatal("dynamic instrument did not use fallback logger")
	}
	dynamic.LogEvent(1, 2, "dynamic_event", nil)
	if len(fallback.records) != 1 || fallback.records[0].event != "dynamic_event" {
		t.Fatalf("fallback did not receive tagged event: %+v", fallback.records)
	}
	tagged, ok := fallback.records[0].data.(instrumentLogEvent)
	if !ok || tagged.Symbol != "DYNAMIC-OPTION" {
		t.Fatalf("fallback record has no source symbol: %+v", fallback.records[0])
	}

	specific := &recordingLogger{}
	ex.SetLogger("DYNAMIC-OPTION", specific)
	if got := ex.getLogger("DYNAMIC-OPTION"); got != specific {
		t.Fatal("symbol-specific logger did not take precedence")
	}

}
