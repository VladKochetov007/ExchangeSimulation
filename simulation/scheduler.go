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
	mu     sync.Mutex
}

// NewEventScheduler creates a new event scheduler
func NewEventScheduler(clock *SimulatedClock) *EventScheduler {
	es := &EventScheduler{
		clock:  clock,
		events: make(eventHeap, 0),
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

		// Fire callback (unlocked to prevent deadlock if callback schedules events)
		event.Callback()

		// Reschedule if repeating
		if event.Repeating {
			es.mu.Lock()
			event.Time += event.Interval
			heap.Push(&es.events, event)
			es.mu.Unlock()
		}
	}
}

// eventHeap implements heap.Interface for priority queue of events
type eventHeap []*ScheduledEvent

func (h eventHeap) Len() int           { return len(h) }
func (h eventHeap) Less(i, j int) bool { return h[i].Time < h[j].Time }
func (h eventHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

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
