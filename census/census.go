// Package census counts how often a high-frequency operation does useful work
// and how often it does nothing.
//
// The structural optimizations that paid off in this campaign were all found
// the same way: an operation that ran millions of times and, in most of those
// runs, produced no effect. Preview matching built a detached book for 62 % of
// orders that could not cross anything; the risk sweep probed 94.9 % of
// (account, symbol) pairs that held no position. Both were invisible in a CPU
// profile, which shows where time goes but not whether the work was needed.
//
// A census closes that gap: for each site it records calls and useless calls,
// so waste can be ranked by calls x useless-fraction x cost rather than by
// self-time alone. It deliberately does not measure time — the profiler already
// does that, and adding timing to a hot path perturbs what is being measured.
//
// Counting is off unless EXSIM_CENSUS is set, and the disabled path is a single
// predictable branch on an immutable global. Sites register themselves at init
// so adding one costs nothing at a call site and requires no central table to
// edit.
package census

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"sync"
	"sync/atomic"
)

// Enabled reports whether counting is on. It is read on every instrumented
// call, so it is set once from the environment at init and never written again.
var Enabled = os.Getenv("EXSIM_CENSUS") != ""

// Site is one counted operation. Obtain one with Register at package init and
// keep it in a package-level variable; the call site then costs one branch when
// counting is off.
type Site struct {
	name     string
	note     string
	calls    atomic.Uint64
	useless  atomic.Uint64
	quantity atomic.Uint64
}

var (
	mu    sync.Mutex
	sites []*Site
)

// Register names a counted operation. note states what "useless" means for this
// site in economic terms, because a high useless fraction is only actionable
// once someone can say whether doing nothing was the expected outcome.
func Register(name, note string) *Site {
	s := &Site{name: name, note: note}
	mu.Lock()
	sites = append(sites, s)
	mu.Unlock()
	return s
}

// Call records one invocation. useless is true when the call produced no
// effect the simulation would have missed.
func (s *Site) Call(useless bool) {
	if !Enabled {
		return
	}
	s.calls.Add(1)
	if useless {
		s.useless.Add(1)
	}
}

// Quantity records an amount alongside a call — items scanned, orders copied,
// bytes hashed. It is what separates "this ran often" from "this ran often and
// touched a lot each time".
func (s *Site) Quantity(n int) {
	if !Enabled || n <= 0 {
		return
	}
	s.quantity.Add(uint64(n))
}

// Snapshot is one site's counts at the end of a run.
type Snapshot struct {
	Name           string
	Note           string
	Calls          uint64
	Useless        uint64
	Quantity       uint64
	UselessPercent float64
}

// Report returns every registered site, ordered by useless calls descending so
// the ranking question — which operation wastes the most invocations — is
// answered by reading down the page.
func Report() []Snapshot {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Snapshot, 0, len(sites))
	for _, s := range sites {
		calls, useless := s.calls.Load(), s.useless.Load()
		percent := 0.0
		if calls > 0 {
			percent = float64(useless) / float64(calls) * 100
		}
		out = append(out, Snapshot{
			Name: s.name, Note: s.note, Calls: calls, Useless: useless,
			Quantity: s.quantity.Load(), UselessPercent: percent,
		})
	}
	slices.SortFunc(out, func(a, b Snapshot) int {
		if c := cmp.Compare(b.Useless, a.Useless); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return out
}

// Write prints the census. Sites that were never reached are printed too: a
// zero row means the run did not exercise that path, which is a different and
// equally useful statement from "this path ran and did nothing".
func Write(w io.Writer) {
	report := Report()
	if len(report) == 0 {
		return
	}
	fmt.Fprintf(w, "%-44s %14s %14s %8s %16s  %s\n",
		"site", "calls", "useless", "useless%", "quantity", "what useless means")
	for _, row := range report {
		fmt.Fprintf(w, "%-44s %14d %14d %7.1f%% %16d  %s\n",
			row.Name, row.Calls, row.Useless, row.UselessPercent, row.Quantity, row.Note)
	}
}

// repeats remembers the last fingerprint seen for a key, so a site can ask
// whether it just recomputed something it already had. It is census-only state
// and is never consulted when counting is off.
var repeats sync.Map // string -> uint64

// Repeated reports whether fingerprint matches the previous one recorded under
// key, and stores it either way. A true result means the work produced a value
// identical to the one already in hand — the cheapest kind of waste to find and
// usually the hardest to see in a profile, because the work looks productive.
func Repeated(key string, fingerprint uint64) bool {
	if !Enabled {
		return false
	}
	previous, seen := repeats.Swap(key, fingerprint)
	return seen && previous.(uint64) == fingerprint
}

// FNV1a hashes a sequence of integers into a fingerprint. It exists so census
// sites can cheaply describe "the value I just produced" without allocating.
type FNV1a uint64

// NewFNV1a returns a seeded hash.
func NewFNV1a() FNV1a { return FNV1a(1469598103934665603) }

// Add folds one integer into the hash.
func (h FNV1a) Add(v int64) FNV1a {
	for shift := 0; shift < 64; shift += 8 {
		h ^= FNV1a(byte(v >> shift))
		h *= 1099511628211
	}
	return h
}

// byName lazily registers sites discovered at runtime, for censuses keyed by a
// value that is not known when the package is compiled — an event name, a
// symbol, a payload type.
var byName sync.Map // string -> *Site

// SiteFor returns the site for name, registering it on first use. Safe to call
// on a hot path: it is a map lookup once counting is on and returns nil when
// counting is off, so callers must nil-check or use CountFor.
func SiteFor(name, note string) *Site {
	if !Enabled {
		return nil
	}
	if existing, ok := byName.Load(name); ok {
		return existing.(*Site)
	}
	// Build first, publish only if this call won the race. Registering before
	// LoadOrStore and unwinding afterwards would pop whichever site happened to
	// be last, which is not necessarily this one.
	site := &Site{name: name, note: note}
	actual, loaded := byName.LoadOrStore(name, site)
	if !loaded {
		mu.Lock()
		sites = append(sites, site)
		mu.Unlock()
	}
	return actual.(*Site)
}

// CountFor records one call and an associated quantity against a lazily
// registered site, doing nothing when counting is off.
func CountFor(name, note string, useless bool, quantity int) {
	if !Enabled {
		return
	}
	site := SiteFor(name, note)
	site.Call(useless)
	site.Quantity(quantity)
}
