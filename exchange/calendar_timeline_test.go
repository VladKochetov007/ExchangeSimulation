package exchange

import (
	"context"
	"testing"
	"time"

	instrumentpkg "exchange_sim/instrument"
)

// calendarTimelineTickerFactory keeps only the exchange's expiry automation
// ticker live. The test still advances through every one-second automation poll,
// but avoids unrelated price, funding, and collateral work so lifecycle times
// remain the only assertion target.
type calendarTimelineTickerFactory struct {
	clock   *calendarTimelineClock
	tickers []*calendarTimelineTicker
	calls   int
}

type calendarTimelineTicker struct {
	ch      chan time.Time
	nextAt  int64
	stopped bool
}

func (t *calendarTimelineTicker) C() <-chan time.Time { return t.ch }

func (t *calendarTimelineTicker) Stop() {
	t.stopped = true
}

type calendarTimelineClock struct {
	now     int64
	factory *calendarTimelineTickerFactory
}

func (c *calendarTimelineClock) NowUnixNano() int64 { return c.now }
func (c *calendarTimelineClock) NowUnix() int64     { return c.now / int64(time.Second) }

func (c *calendarTimelineClock) Advance(interval time.Duration) {
	c.now += int64(interval)
	if c.factory != nil {
		c.factory.deliverDueTicks()
	}
}

func (f *calendarTimelineTickerFactory) NewTicker(interval time.Duration) Ticker {
	f.calls++
	ticker := &calendarTimelineTicker{ch: make(chan time.Time, 1)}
	if f.calls == 4 {
		ticker.nextAt = f.clock.now + interval.Nanoseconds()
	}
	f.tickers = append(f.tickers, ticker)
	return ticker
}

func (f *calendarTimelineTickerFactory) deliverDueTicks() {
	for _, ticker := range f.tickers {
		if ticker.stopped || ticker.nextAt == 0 || f.clock.now < ticker.nextAt {
			continue
		}
		ticker.ch <- time.Unix(0, f.clock.now)
		ticker.nextAt += int64(time.Second)
	}
}

func TestCalendarLifecycleUsesEveryAutomationPollThroughTwentyFourHours(t *testing.T) {
	const epochNano = int64(1735689600000000000)
	clock := &calendarTimelineClock{now: epochNano}
	factory := &calendarTimelineTickerFactory{clock: clock}
	clock.factory = factory
	ex := NewExchangeWithConfig(ExchangeConfig{
		Clock:               clock,
		TickerFactory:       factory,
		DeterministicPhases: true,
	})
	defer ex.Shutdown()

	ex.AddInstrument(NewSpotInstrument("ABC/USD", "ABC", "USD", 1, 1, 1, 1))
	addBookPriceQuote(t, ex, Buy, 100)
	addBookPriceQuote(t, ex, Sell, 102)
	calendar := instrumentpkg.ExpiryCalendar{Schedules: []instrumentpkg.ExpirySchedule{
		{Name: "short", ListingIntervalNano: int64(time.Hour), TimeToExpiryNano: int64(2 * time.Hour)},
		{Name: "medium", ListingIntervalNano: int64(3 * time.Hour), TimeToExpiryNano: int64(6 * time.Hour)},
		{Name: "long", ListingIntervalNano: int64(6 * time.Hour), TimeToExpiryNano: int64(12 * time.Hour)},
	}}
	spec := instrumentpkg.ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 1, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1}
	globalLog := &recordingLogger{}
	ex.SetLogger("_global", globalLog)
	ex.ConfigureAutomation(AutomationConfig{
		ListingPolicies: []ListingPolicy{
			&instrumentpkg.DatedFuturesLister{Underlying: "ABC/USD", Spec: spec, Calendar: &calendar, CalendarEpochNano: epochNano},
			&instrumentpkg.OptionChainLister{Underlying: "ABC/USD", Spec: spec, Calendar: &calendar, CalendarEpochNano: epochNano, StrikeStep: 10, StrikesPerSide: 2},
		},
	})
	ex.StartAutomation(context.Background())
	if factory.calls != 4 {
		t.Fatalf("automation ticker count = %d, want four registered tickers", factory.calls)
	}

	for second := int64(0); second < int64(24*time.Hour/time.Second); second++ {
		clock.Advance(time.Second)
		if !ex.PumpDeterministicPhase() {
			t.Fatalf("expiry automation did not run at second %d", second+1)
		}
	}
	ex.StopAutomation()

	type lifecycleKey struct {
		kind   string
		expiry int64
	}
	type lifecycleObservation struct {
		firstAt int64
		count   int
	}
	observations := make(map[lifecycleKey]lifecycleObservation)
	for _, record := range globalLog.records {
		if record.event != "instrument_listed" {
			continue
		}
		announcement, ok := record.data.(*InstrumentAnnouncement)
		if !ok || (announcement.InstrumentType != "FUTURE" && announcement.InstrumentType != "OPTION") {
			continue
		}
		key := lifecycleKey{kind: announcement.InstrumentType, expiry: announcement.ExpiryNano}
		observation := observations[key]
		if observation.count == 0 {
			observation.firstAt = announcement.Timestamp
		}
		observation.count++
		observations[key] = observation
	}

	futureExpiries := make(map[int64]struct{})
	optionExpiries := make(map[int64]struct{})
	for key, observation := range observations {
		switch key.kind {
		case "FUTURE":
			futureExpiries[key.expiry] = struct{}{}
			if observation.count != 1 {
				t.Fatalf("future expiry %d listing count = %d, want one", key.expiry, observation.count)
			}
		case "OPTION":
			optionExpiries[key.expiry] = struct{}{}
			if observation.count != 10 {
				t.Fatalf("option expiry %d listing count = %d, want ten-strike chain", key.expiry, observation.count)
			}
		}
	}
	if len(futureExpiries) != 28 || len(optionExpiries) != 28 {
		t.Fatalf("realized expiry counts = futures %d, options %d; want 28 each", len(futureExpiries), len(optionExpiries))
	}

	expected := []struct {
		expiryHours int64
		firstAfter  time.Duration
	}{
		{expiryHours: 2, firstAfter: time.Second},
		{expiryHours: 3, firstAfter: time.Hour},
		{expiryHours: 5, firstAfter: 3 * time.Hour},
		{expiryHours: 18, firstAfter: 6 * time.Hour},
		{expiryHours: 26, firstAfter: 24 * time.Hour},
		{expiryHours: 27, firstAfter: 21 * time.Hour},
		{expiryHours: 30, firstAfter: 18 * time.Hour},
		{expiryHours: 36, firstAfter: 24 * time.Hour},
	}
	for _, expectedEntry := range expected {
		expiryNano := epochNano + expectedEntry.expiryHours*int64(time.Hour)
		wantFirstAt := epochNano + int64(expectedEntry.firstAfter)
		for _, kind := range []string{"FUTURE", "OPTION"} {
			observation, ok := observations[lifecycleKey{kind: kind, expiry: expiryNano}]
			if !ok {
				t.Fatalf("missing %s expiry %dh lifecycle listing", kind, expectedEntry.expiryHours)
			}
			if observation.firstAt != wantFirstAt {
				t.Fatalf("%s expiry %dh first listing = %d, want %d", kind, expectedEntry.expiryHours, observation.firstAt, wantFirstAt)
			}
		}
	}
}
