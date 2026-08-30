package analysis

import "bytes"

// needleSet answers "does this line contain any registered event name" in one
// pass instead of one standard-library substring search per needle.
//
// The prefilter runs on every physical line of every scan, including the large
// majority that are discarded, so it is the one piece of work proportional to
// raw evidence size rather than to selected records. On a 10.89GB cell it was
// 15.7% of the dominant metric's CPU, and the fused scan searches the union of
// every metric's event names, where the per-needle cost multiplies.
//
// The automaton is Aho-Corasick: it reports a match exactly when some needle
// occurs as a substring, which is the same predicate bytes.Contains computes,
// so the set of admitted lines is unchanged. That matters beyond dispatch —
// which records a metric sees is decided later by its event filter — because
// the prefilter also decides which malformed records a metric attempts to
// decode and therefore fails on.
// needleSetThreshold is the measured crossover between the two strategies. The
// standard library's per-needle search is assembly and beats a table walk while
// the needle count is small; the automaton's cost is flat in the needle count
// and wins once there are enough. Measured on a 101MiB retained evidence corpus
// at GOMAXPROCS=1, median of three, in MB/s:
//
//	needles      4      6      7     10     12
//	per-needle 663.8  551.4  472.5  389.9  357.3
//	automaton  530.8  549.3  555.0  569.3  567.3
//
// A per-metric scan usually registers a handful of event names and takes the
// per-needle path; the fused scan searches the union of every metric's names and
// takes the automaton.
const needleSetThreshold = 7

type needleSet struct {
	// next[state][b] is the state after consuming b. Dense transitions cost
	// 1KiB per state and a handful of states, which buys a branch-free step.
	next [][256]int32
	// terminal[state] reports whether some needle ends at this state.
	terminal []bool
	// single is used when there is exactly one needle: the standard library's
	// assembly search beats a table walk, and one needle is the common case for
	// a per-metric scan.
	single []byte
	// perNeedle is used below the threshold, where the standard library's
	// assembly search is faster than walking the table.
	perNeedle [][]byte
	empty     bool
}

// newNeedleSet builds the automaton. An empty set matches nothing, which
// callers treat as "no prefilter configured".
func newNeedleSet(needles [][]byte) *needleSet {
	set := &needleSet{}
	if len(needles) == 0 {
		set.empty = true
		return set
	}
	if len(needles) == 1 {
		set.single = needles[0]
		return set
	}
	if len(needles) < needleSetThreshold {
		set.perNeedle = needles
		return set
	}

	// Trie. State 0 is the root; goto-misses at the root stay at the root.
	set.next = make([][256]int32, 1, 64)
	set.terminal = make([]bool, 1, 64)
	// child records real trie edges before failure links are folded in, so
	// building the failure function can distinguish a genuine edge from one
	// inherited during construction.
	child := []map[byte]int32{{}}

	for _, needle := range needles {
		state := int32(0)
		for _, b := range needle {
			nextState, ok := child[state][b]
			if !ok {
				set.next = append(set.next, [256]int32{})
				set.terminal = append(set.terminal, false)
				child = append(child, map[byte]int32{})
				nextState = int32(len(set.next) - 1)
				child[state][b] = nextState
			}
			state = nextState
		}
		set.terminal[state] = true
	}

	// Breadth-first failure links, folded directly into the dense table so the
	// scan never follows a failure pointer at match time.
	fail := make([]int32, len(set.next))
	queue := make([]int32, 0, len(set.next))
	for b := 0; b < 256; b++ {
		if target, ok := child[0][byte(b)]; ok {
			fail[target] = 0
			set.next[0][b] = target
			queue = append(queue, target)
		} else {
			set.next[0][b] = 0
		}
	}
	for head := 0; head < len(queue); head++ {
		state := queue[head]
		// A needle ending at the failure state also ends here.
		if set.terminal[fail[state]] {
			set.terminal[state] = true
		}
		for b := 0; b < 256; b++ {
			if target, ok := child[state][byte(b)]; ok {
				fail[target] = set.next[fail[state]][b]
				set.next[state][b] = target
				queue = append(queue, target)
			} else {
				set.next[state][b] = set.next[fail[state]][b]
			}
		}
	}
	return set
}

// matches reports whether any needle occurs in line.
func (s *needleSet) matches(line []byte) bool {
	if s.empty {
		return false
	}
	if s.single != nil {
		return bytes.Contains(line, s.single)
	}
	if s.perNeedle != nil {
		for _, needle := range s.perNeedle {
			if bytes.Contains(line, needle) {
				return true
			}
		}
		return false
	}
	state := int32(0)
	for _, b := range line {
		state = s.next[state][b]
		if s.terminal[state] {
			return true
		}
	}
	return false
}
