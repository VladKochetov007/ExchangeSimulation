package census

import (
	"bytes"
	"strings"
	"testing"
)

// withCounting turns counting on for one test and restores the process default,
// so a test cannot leave the disabled path silently enabled for the others.
func withCounting(t *testing.T, on bool) {
	t.Helper()
	previous := Enabled
	Enabled = on
	t.Cleanup(func() { Enabled = previous })
}

// TestDisabledSiteCountsNothing pins the property the call sites rely on: with
// counting off, an instrumented hot path must not accumulate state. A site that
// counted anyway would turn every production run into a slow one.
func TestDisabledSiteCountsNothing(t *testing.T) {
	withCounting(t, false)
	site := Register("test.disabled", "never")
	for range 100 {
		site.Call(true)
		site.Quantity(7)
	}
	if got := site.calls.Load(); got != 0 {
		t.Fatalf("calls recorded while disabled = %d, want 0", got)
	}
	if got := site.quantity.Load(); got != 0 {
		t.Fatalf("quantity recorded while disabled = %d, want 0", got)
	}
	if Repeated("test.disabled", 1) {
		t.Fatal("Repeated reported a repeat while disabled")
	}
}

func TestSiteCountsCallsAndUselessSeparately(t *testing.T) {
	withCounting(t, true)
	site := Register("test.counts", "did nothing")
	for i := range 10 {
		site.Call(i%2 == 0)
	}
	site.Quantity(3)
	site.Quantity(4)
	site.Quantity(0) // ignored: a zero quantity is not an observation

	if got := site.calls.Load(); got != 10 {
		t.Fatalf("calls = %d, want 10", got)
	}
	if got := site.useless.Load(); got != 5 {
		t.Fatalf("useless = %d, want 5", got)
	}
	if got := site.quantity.Load(); got != 7 {
		t.Fatalf("quantity = %d, want 7", got)
	}
}

// TestRepeatedNeedsAPriorObservation covers the boundary that decides whether a
// census overstates waste: the first time a key is seen there is nothing to
// repeat, so it must not be reported as one.
func TestRepeatedNeedsAPriorObservation(t *testing.T) {
	withCounting(t, true)
	const key = "test.repeat.first"
	if Repeated(key, 42) {
		t.Fatal("first observation of a key reported as a repeat")
	}
	if !Repeated(key, 42) {
		t.Fatal("identical second observation not reported as a repeat")
	}
	if Repeated(key, 43) {
		t.Fatal("changed value reported as a repeat")
	}
	if !Repeated(key, 43) {
		t.Fatal("the changed value was not remembered for the next comparison")
	}
}

// TestRepeatedKeysAreIndependent matters because the simulator runs several
// venues that share symbol names: keying by symbol alone would compare one
// venue's book against another's and invent repeats that never happened.
func TestRepeatedKeysAreIndependent(t *testing.T) {
	withCounting(t, true)
	Repeated("venueA/ABC", 1)
	if Repeated("venueB/ABC", 1) {
		t.Fatal("a different key saw the first key's value")
	}
}

// TestFNV1aDistinguishesOrderAndValue guards the fingerprint: a hash that
// collided on reordered or shifted values would report changed books as
// unchanged, which is the direction that overstates waste.
func TestFNV1aDistinguishesOrderAndValue(t *testing.T) {
	of := func(values ...int64) uint64 {
		h := NewFNV1a()
		for _, v := range values {
			h = h.Add(v)
		}
		return uint64(h)
	}
	if of(1, 2) == of(2, 1) {
		t.Fatal("fingerprint ignores order")
	}
	if of(1, 2) == of(1, 3) {
		t.Fatal("fingerprint ignores a changed value")
	}
	if of(1, 2) == of(1, 2, 0) {
		t.Fatal("fingerprint ignores a trailing zero, so an added empty level is invisible")
	}
	if of(1, 2) != of(1, 2) {
		t.Fatal("fingerprint is not deterministic")
	}
}

func TestWriteReportsEveryRegisteredSite(t *testing.T) {
	withCounting(t, true)
	busy := Register("test.write.busy", "wasted")
	quiet := Register("test.write.quiet", "never reached")
	for range 4 {
		busy.Call(true)
	}
	busy.Call(false)

	var out bytes.Buffer
	Write(&out)
	text := out.String()

	if !strings.Contains(text, "test.write.busy") {
		t.Fatalf("busy site missing from report:\n%s", text)
	}
	// A site that was never reached is a different statement from a site that
	// ran and did nothing, so it has to appear rather than be filtered out.
	if !strings.Contains(text, "test.write.quiet") {
		t.Fatalf("unreached site missing from report:\n%s", text)
	}
	if !strings.Contains(text, "80.0%") {
		t.Fatalf("useless percentage not reported:\n%s", text)
	}
	_ = quiet
}

// TestSiteForRegistersOnceUnderConcurrency guards the lazy path: a lost race
// must not publish a duplicate, and must not unwind an unrelated site. An
// earlier version popped the last registered site on losing the race, which
// could delete a different site entirely.
func TestSiteForRegistersOnceUnderConcurrency(t *testing.T) {
	withCounting(t, true)
	bystander := Register("test.sitefor.bystander", "must survive")

	const workers = 32
	done := make(chan *Site, workers)
	for range workers {
		go func() { done <- SiteFor("test.sitefor.shared", "shared") }()
	}
	first := <-done
	for range workers - 1 {
		if got := <-done; got != first {
			t.Fatal("SiteFor returned two different sites for one name")
		}
	}

	for range 10 {
		first.Call(true)
	}
	if got := first.calls.Load(); got != 10 {
		t.Fatalf("shared site calls = %d, want 10", got)
	}

	var seenShared, seenBystander int
	for _, row := range Report() {
		switch row.Name {
		case "test.sitefor.shared":
			seenShared++
		case "test.sitefor.bystander":
			seenBystander++
		}
	}
	if seenShared != 1 {
		t.Fatalf("shared site appears %d times in the report, want 1", seenShared)
	}
	if seenBystander != 1 {
		t.Fatalf("bystander site appears %d times, want 1 — the race unwound the wrong entry", seenBystander)
	}
	_ = bystander
}

func TestSiteForIsInertWhenDisabled(t *testing.T) {
	withCounting(t, false)
	if got := SiteFor("test.sitefor.disabled", "never"); got != nil {
		t.Fatalf("SiteFor returned %v while disabled, want nil", got)
	}
	CountFor("test.sitefor.disabled", "never", true, 5) // must not panic
}
