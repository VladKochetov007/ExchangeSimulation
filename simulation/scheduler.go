package simulation

import (
	"container/heap"
	"sync"
)

// ScheduledEvent represents a single event scheduled to occur at a specific simulation time
type ScheduledEvent struct {
	Time      int64
	Callback  func()
	Repeating bool
	Interval  int64
	id        uint64
}

// EventScheduler manages scheduled events and fires them when simulation time advances
type EventScheduler struct {
	clock  *SimulatedClock
	events eventHeap
	nextID uint64
	// cancelled holds IDs whose Cancel raced with the event being popped for
	// firing: the event is not in the heap at that moment, so the flag is what
	// stops a repeating event from being re-pushed as an uncancellable zombie.
	cancelled map[uint64]struct{}
	mu        sync.Mutex
}

// NewEventScheduler creates a new event scheduler
func NewEventScheduler(clock *SimulatedClock) *EventScheduler {
	es := &EventScheduler{
		clock:     clock,
		events:    make(eventHeap, 0),
		cancelled: make(map[uint64]struct{}),
	}
	heap.Init(&es.events)
	return es
}

// Schedule schedules a one-time event at the specified simulation time
func (es *EventScheduler) Schedule(atTime int64, callback func()) uint64 {
	es.mu.Lock()
	defer es.mu.Unlock()

	es.nextID++
	event := &ScheduledEvent{
		Time:     atTime,
		Callback: callback,
		id:       es.nextID,
	}
	heap.Push(&es.events, event)
	return es.nextID
}

// ScheduleRepeating schedules a callback to be called every interval nanoseconds
func (es *EventScheduler) ScheduleRepeating(interval int64, callback func()) uint64 {
	// A non-positive interval never advances the event's due time, turning
	// ProcessUntil into an infinite loop that hangs the whole simulation.
	if interval < 1 {
		interval = 1
	}
	es.mu.Lock()
	defer es.mu.Unlock()

	es.nextID++
	event := &ScheduledEvent{
		Time:      es.clock.NowUnixNano() + interval,
		Callback:  callback,
		Repeating: true,
		Interval:  interval,
		id:        es.nextID,
	}
	heap.Push(&es.events, event)
	return es.nextID
}

// ScheduleRepeatingWithOffset schedules the first callback after interval plus
// offset, then repeats at interval. It leaves ScheduleRepeating unchanged so
// legacy zero-phase tickers retain their established scheduling path.
func (es *EventScheduler) ScheduleRepeatingWithOffset(interval, offset int64, callback func()) uint64 {
	if interval < 1 {
		interval = 1
	}
	if offset < 0 || offset >= interval {
		panic("simulation: repeating ticker offset must be in [0, interval)")
	}
	es.mu.Lock()
	defer es.mu.Unlock()

	es.nextID++
	event := &ScheduledEvent{
		Time:      es.clock.NowUnixNano() + interval + offset,
		Callback:  callback,
		Repeating: true,
		Interval:  interval,
		id:        es.nextID,
	}
	heap.Push(&es.events, event)
	return es.nextID
}

// Cancel removes a scheduled event by ID
func (es *EventScheduler) Cancel(id uint64) {
	es.mu.Lock()
	defer es.mu.Unlock()

	for i, event := range es.events {
		if event.id == id {
			heap.Remove(&es.events, i)
			return
		}
	}
	// Not in the heap: the event is mid-fire in ProcessUntil (or already gone).
	// Flag it so a repeating event is dropped instead of re-pushed.
	es.cancelled[id] = struct{}{}
}

// ProcessUntil fires all events up to and including the given time
// Called by SimulatedClock.Advance()
func (es *EventScheduler) ProcessUntil(untilTime int64) {
	for {
		es.mu.Lock()
		if len(es.events) == 0 || es.events[0].Time > untilTime {
			es.mu.Unlock()
			return
		}

		event := heap.Pop(&es.events).(*ScheduledEvent)
		es.mu.Unlock()

		// Advance the simulation clock to this event's scheduled time before
		// firing, so the callback sees its own instant rather than the end of
		// the enclosing Advance() jump. Events pop in non-decreasing time order,
		// so the guard only ever moves the clock forward; a past-due event fires
		// at the current time instead of rewinding it.
		if es.clock != nil && event.Time > es.clock.NowUnixNano() {
			es.clock.SetTime(event.Time)
		}

		// Fire callback (unlocked to prevent deadlock if callback schedules events)
		event.Callback()

		// Reschedule if repeating — unless a Cancel landed while the event was
		// mid-fire (it was not in the heap, so Cancel could only flag it).
		es.mu.Lock()
		if _, wasCancelled := es.cancelled[event.id]; wasCancelled {
			delete(es.cancelled, event.id)
		} else if event.Repeating {
			event.Time += event.Interval
			heap.Push(&es.events, event)
		}
		es.mu.Unlock()
	}
}

// eventHeap implements heap.Interface for priority queue of events
type eventHeap []*ScheduledEvent

func (h eventHeap) Len() int { return len(h) }

// Less orders by time, then by schedule sequence so equal-timestamp events
// fire in FIFO order — heap sift order is not deterministic across runs.
func (h eventHeap) Less(i, j int) bool {
	if h[i].Time != h[j].Time {
		return h[i].Time < h[j].Time
	}
	return h[i].id < h[j].id
}
func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *eventHeap) Push(x any) {
	*h = append(*h, x.(*ScheduledEvent))
}

func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
