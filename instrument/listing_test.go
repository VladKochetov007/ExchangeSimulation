package instrument

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	etypes "exchange_sim/types"
)

type listingPriceSource map[string]int64

const calendarUnit = int64(1_000_000_000)

func (p listingPriceSource) Price(symbol string) (int64, error) {
	price, ok := p[symbol]
	if !ok || price <= 0 {
		return 0, fmt.Errorf("listing price for %s unavailable", symbol)
	}
	return price, nil
}

func TestOptionChainListerCapsDistinctStrikesPerExpiry(t *testing.T) {
	lister := &OptionChainLister{
		Underlying:          "ABC/USD",
		Spec:                ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		TenorsNano:          []int64{1_000},
		StrikeStep:          10,
		StrikesPerSide:      1,
		MaxStrikesPerExpiry: 3,
	}
	first, err := lister.PendingListings(0, listingPriceSource{"ABC/USD": 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 6 { // 3 strikes, call and put each
		t.Fatalf("initial chain = %d options, want 6", len(first))
	}
	// A move to a disjoint grid would previously append three new strikes on
	// every price move. The capped venue keeps its bounded live board.
	second, err := lister.PendingListings(1, listingPriceSource{"ABC/USD": 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("capped chain listed %d additional options, want 0", len(second))
	}
}

type unavailableListingPrice struct{ err error }

func (p unavailableListingPrice) Price(string) (int64, error) { return 0, p.err }

func TestOptionChainListerPropagatesPriceAbsence(t *testing.T) {
	sentinel := errors.New("missing reference")
	lister := &OptionChainLister{
		Underlying: "ABC/USD",
		Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		TenorsNano: []int64{1_000}, StrikeStep: 10,
	}
	listed, err := lister.PendingListings(0, unavailableListingPrice{err: sentinel})
	if listed != nil || !errors.Is(err, sentinel) {
		t.Fatalf("option price absence = (%#v, %v), want wrapped sentinel", listed, err)
	}
}

func compressedCalendar() *ExpiryCalendar {
	return &ExpiryCalendar{Schedules: []ExpirySchedule{
		{Name: "short", ListingIntervalNano: calendarUnit, TimeToExpiryNano: 2 * calendarUnit},
		{Name: "medium", ListingIntervalNano: 3 * calendarUnit, TimeToExpiryNano: 6 * calendarUnit},
		{Name: "long", ListingIntervalNano: 6 * calendarUnit, TimeToExpiryNano: 12 * calendarUnit},
	}}
}

func expiringExpiry(t *testing.T, instrument etypes.Instrument) int64 {
	t.Helper()
	expiring, ok := instrument.(interface{ ExpiryNano() int64 })
	if !ok {
		t.Fatalf("instrument %T has no expiry", instrument)
	}
	return expiring.ExpiryNano()
}

func TestExpiryCalendarDeduplicatesOverlappingFamiliesAndAdvancesCursors(t *testing.T) {
	lister := &DatedFuturesLister{
		Underlying: "ABC/USD",
		Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Calendar:   compressedCalendar(),
	}
	var expiries []int64
	for now := int64(0); now < 24*calendarUnit; now += calendarUnit {
		listed, err := lister.PendingListings(now, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, instrument := range listed {
			expiries = append(expiries, expiringExpiry(t, instrument))
		}
	}
	want := []int64{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 27, 30}
	for index := range want {
		want[index] *= calendarUnit
	}
	slices.Sort(expiries)
	if !slices.Equal(expiries, want) {
		t.Fatalf("calendar expiries = %v, want %v", expiries, want)
	}
	if got := lister.calendar.nextIndex["short"]; got != 24 {
		t.Fatalf("short cursor = %d, want 24 after collision requests", got)
	}
	if got := lister.calendar.nextIndex["medium"]; got != 8 {
		t.Fatalf("medium cursor = %d, want 8 after collision requests", got)
	}
	if got := lister.calendar.nextIndex["long"]; got != 4 {
		t.Fatalf("long cursor = %d, want 4", got)
	}
}

func TestExpiryCalendarScheduleOrderDoesNotChangeListingOrder(t *testing.T) {
	leftCalendar := compressedCalendar()
	rightCalendar := &ExpiryCalendar{Schedules: []ExpirySchedule{
		leftCalendar.Schedules[2], leftCalendar.Schedules[0], leftCalendar.Schedules[1],
	}}
	makeLister := func(calendar *ExpiryCalendar) *DatedFuturesLister {
		return &DatedFuturesLister{
			Underlying: "ABC/USD",
			Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
			Calendar:   calendar,
		}
	}
	left, err := makeLister(leftCalendar).PendingListings(8*calendarUnit, nil)
	if err != nil {
		t.Fatal(err)
	}
	right, err := makeLister(rightCalendar).PendingListings(8*calendarUnit, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != len(right) {
		t.Fatalf("permuted calendar lengths = %d and %d", len(left), len(right))
	}
	for index := range left {
		if left[index].Symbol() != right[index].Symbol() {
			t.Fatalf("permuted calendar symbol %d = %s and %s", index, left[index].Symbol(), right[index].Symbol())
		}
	}
}

type countingListingPriceSource struct {
	price int64
	calls int
}

func (p *countingListingPriceSource) Price(string) (int64, error) {
	p.calls++
	if p.price <= 0 {
		return 0, errors.New("price unavailable")
	}
	return p.price, nil
}

func TestOptionCalendarSharesExpiryIdentityAndDoesNotRecenterAListedChain(t *testing.T) {
	lister := &OptionChainLister{
		Underlying:          "ABC/USD",
		Spec:                ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Calendar:            compressedCalendar(),
		StrikeStep:          10,
		StrikesPerSide:      1,
		MaxStrikesPerExpiry: 3,
	}
	first, err := lister.PendingListings(0, listingPriceSource{"ABC/USD": 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 18 {
		t.Fatalf("initial option chains = %d, want 18", len(first))
	}
	second, err := lister.PendingListings(4*calendarUnit, listingPriceSource{"ABC/USD": 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 12 {
		t.Fatalf("overlap option chains = %d, want two new chains without rebuilding shared expiries", len(second))
	}
	for _, instrument := range second {
		if expiry := expiringExpiry(t, instrument); expiry == 6*calendarUnit || expiry == 12*calendarUnit {
			t.Fatalf("overlap rebuilt an already listed expiry %d", expiry/calendarUnit)
		}
	}
	fixedCenter := make(map[int64]bool)
	for _, instrument := range first {
		expiry := expiringExpiry(t, instrument)
		if (expiry == 6*calendarUnit || expiry == 12*calendarUnit) && strings.Contains(instrument.Symbol(), "100") {
			fixedCenter[expiry] = true
		}
	}
	for _, expiry := range []int64{6 * calendarUnit, 12 * calendarUnit} {
		if !fixedCenter[expiry] {
			t.Fatalf("expiry %d chain was not fixed at first listing spot", expiry/calendarUnit)
		}
	}
}

func TestOptionCalendarRetainsDueBatchAfterUnavailablePriceAndSkipsNoOpPriceLookup(t *testing.T) {
	lister := &OptionChainLister{
		Underlying: "ABC/USD",
		Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Calendar:   &ExpiryCalendar{Schedules: []ExpirySchedule{{Name: "short", ListingIntervalNano: 10 * calendarUnit, TimeToExpiryNano: 20 * calendarUnit}}},
		StrikeStep: 10, StrikesPerSide: 0,
	}
	missing := &countingListingPriceSource{}
	if listed, err := lister.PendingListings(0, missing); listed != nil || err == nil || missing.calls != 1 {
		t.Fatalf("unavailable calendar price = (%v, %v), calls=%d", listed, err, missing.calls)
	}
	available := &countingListingPriceSource{price: 100}
	listed, err := lister.PendingListings(0, available)
	if err != nil || len(listed) != 2 {
		t.Fatalf("retry calendar listing = (%d, %v), want two options", len(listed), err)
	}
	noOp := &countingListingPriceSource{price: 100}
	listed, err = lister.PendingListings(calendarUnit, noOp)
	if err != nil || len(listed) != 0 || noOp.calls != 0 {
		t.Fatalf("calendar no-op = (%d, %v), price calls=%d", len(listed), err, noOp.calls)
	}
}

func TestOptionCalendarCollisionOnlyBatchAdvancesWithoutPriceLookup(t *testing.T) {
	lister := &OptionChainLister{
		Underlying: "ABC/USD",
		Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Calendar:   &ExpiryCalendar{Schedules: []ExpirySchedule{{Name: "short", ListingIntervalNano: calendarUnit, TimeToExpiryNano: 2 * calendarUnit}}},
		StrikeStep: 10, StrikesPerSide: 0,
		calendarListed: map[int64]bool{2 * calendarUnit: true},
	}
	missing := &countingListingPriceSource{}
	listed, err := lister.PendingListings(0, missing)
	if err != nil || listed != nil || missing.calls != 0 {
		t.Fatalf("collision-only calendar batch = (%v, %v), price calls=%d; want no-op without price lookup", listed, err, missing.calls)
	}
	if got := lister.calendar.nextIndex["short"]; got != 1 {
		t.Fatalf("collision-only cursor = %d, want 1", got)
	}
}

func TestExpiryCalendarRejectsAmbiguousSchedules(t *testing.T) {
	calendar := ExpiryCalendar{Schedules: []ExpirySchedule{
		{Name: "short", ListingIntervalNano: 2 * calendarUnit, TimeToExpiryNano: 6 * calendarUnit},
		{Name: "short", ListingIntervalNano: 4 * calendarUnit, TimeToExpiryNano: 12 * calendarUnit},
	}}
	if err := calendar.Validate(); err == nil {
		t.Fatal("duplicate schedule name was accepted")
	}
}

func TestCalendarSymbolsPreserveSubsecondExpiryIdentity(t *testing.T) {
	lister := &DatedFuturesLister{
		Underlying: "ABC/USD",
		Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Calendar: &ExpiryCalendar{Schedules: []ExpirySchedule{
			{Name: "nanosecond", ListingIntervalNano: 1, TimeToExpiryNano: 1},
			{Name: "second", ListingIntervalNano: calendarUnit, TimeToExpiryNano: calendarUnit},
		}},
	}
	listed, err := lister.PendingListings(0, nil)
	if err != nil || len(listed) != 2 {
		t.Fatalf("subsecond calendar listing = (%d, %v), want two futures", len(listed), err)
	}
	if listed[0].Symbol() == listed[1].Symbol() {
		t.Fatalf("distinct expiry timestamps collided in symbols: %s", listed[0].Symbol())
	}
}

func TestCalendarInstrumentSymbolsIncludeUnderlyingIdentity(t *testing.T) {
	calendar := &ExpiryCalendar{Schedules: []ExpirySchedule{{Name: "short", ListingIntervalNano: calendarUnit, TimeToExpiryNano: 2 * calendarUnit}}}
	futureSymbols := make(map[string]struct{})
	optionSymbols := make(map[string]struct{})
	for _, underlying := range []string{"ABC/USD", "ABC/USDT"} {
		futureLister := &DatedFuturesLister{
			Underlying: underlying,
			Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
			Calendar:   calendar,
		}
		future, err := futureLister.PendingListings(0, nil)
		if err != nil || len(future) != 1 {
			t.Fatalf("calendar future for %s = (%d, %v), want one", underlying, len(future), err)
		}
		futureSymbols[future[0].Symbol()] = struct{}{}

		optionLister := &OptionChainLister{
			Underlying: underlying,
			Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
			Calendar:   calendar, StrikeStep: 10, StrikesPerSide: 0,
		}
		options, err := optionLister.PendingListings(0, listingPriceSource{underlying: 100})
		if err != nil || len(options) != 2 {
			t.Fatalf("calendar options for %s = (%d, %v), want call and put", underlying, len(options), err)
		}
		for _, option := range options {
			optionSymbols[option.Symbol()] = struct{}{}
		}
	}
	if len(futureSymbols) != 2 || len(optionSymbols) != 4 {
		t.Fatalf("calendar identity collision: futures=%v options=%v", futureSymbols, optionSymbols)
	}
}

func TestCalendarOptionStrikeSymbolsAreInjectiveInRawUnits(t *testing.T) {
	lister := &OptionChainLister{
		Underlying: "ABC/USD",
		Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 2, TickSize: 1, MinOrderSize: 1},
		Calendar:   &ExpiryCalendar{Schedules: []ExpirySchedule{{Name: "short", ListingIntervalNano: calendarUnit, TimeToExpiryNano: 2 * calendarUnit}}},
		StrikeStep: 3, StrikesPerSide: 1,
	}
	options, err := lister.PendingListings(0, listingPriceSource{"ABC/USD": 3})
	if err != nil || len(options) != 4 {
		t.Fatalf("raw-unit strike chain = (%d, %v), want two strikes and two rights", len(options), err)
	}
	symbols := make(map[string]struct{}, len(options))
	strikes := make(map[int64]struct{}, 2)
	for _, option := range options {
		if _, exists := symbols[option.Symbol()]; exists {
			t.Fatalf("duplicate option symbol %q", option.Symbol())
		}
		symbols[option.Symbol()] = struct{}{}
		strikes[option.(*EuropeanOption).Strike] = struct{}{}
	}
	if len(strikes) != 2 {
		t.Fatalf("raw-unit strikes = %v, want 3 and 6", strikes)
	}
}

func TestCalendarOptionRejectsEmptyChainWithoutConsumingExpiry(t *testing.T) {
	lister := &OptionChainLister{
		Underlying: "ABC/USD",
		Spec:       ContractSpec{Base: "ABC", Quote: "USD", BasePrecision: 100, QuotePrecision: 1, TickSize: 1, MinOrderSize: 1},
		Calendar:   &ExpiryCalendar{Schedules: []ExpirySchedule{{Name: "short", ListingIntervalNano: calendarUnit, TimeToExpiryNano: 2 * calendarUnit}}},
		StrikeStep: 10, StrikesPerSide: 0,
	}
	if listed, err := lister.PendingListings(0, listingPriceSource{"ABC/USD": 1}); listed != nil || err == nil {
		t.Fatalf("empty chain result = (%v, %v), want retained configuration error", listed, err)
	}
	listed, err := lister.PendingListings(0, listingPriceSource{"ABC/USD": 100})
	if err != nil || len(listed) != 2 {
		t.Fatalf("retry after empty chain = (%d, %v), want call and put", len(listed), err)
	}
}
