package ratelimit

import "time"

// StreamGuard enforces the limits a venue publishes for its market-data
// connections. These are a different scope from request budgets: they are per
// connection, so one client with two connections has two allowances and one
// connection shared by two clients has one. Keying them by account, as the
// request budgets are, would be the wrong shape.
type StreamGuard struct {
	limits  StreamLimits
	streams map[string]int
	control *TokenBucket
}

func NewStreamGuard(limits StreamLimits) *StreamGuard {
	guard := &StreamGuard{limits: limits, streams: make(map[string]int)}
	if limits.ControlMessagesPerSec > 0 {
		guard.control = NewTokenBucket("control_messages",
			limits.ControlMessagesPerSec, limits.ControlMessagesPerSec, int64(time.Second))
	}
	return guard
}

// Subscribe takes a stream slot on a connection.
func (g *StreamGuard) Subscribe(connection string, now int64) Decision {
	if decision := g.Control(connection, now); !decision.Allowed {
		return decision
	}
	if g.limits.MaxStreamsPerConnection <= 0 {
		g.streams[connection]++
		return Allow()
	}
	if g.streams[connection] >= g.limits.MaxStreamsPerConnection {
		// Not impossible: the client can unsubscribe to make room, which is a
		// different remedy from waiting.
		return Decision{Limit: "streams_per_connection"}
	}
	g.streams[connection]++
	return Allow()
}

// Unsubscribe frees a stream slot. Releasing more than was taken is ignored
// rather than allowed to manufacture capacity.
func (g *StreamGuard) Unsubscribe(connection string) {
	if g.streams[connection] > 0 {
		g.streams[connection]--
	}
}

// Control charges a connection's control-message allowance, which covers
// subscribe, unsubscribe and keepalive traffic. Venues document exceeding it as
// grounds for disconnection rather than a simple refusal, so a caller modelling
// that should drop the connection on a refusal here.
func (g *StreamGuard) Control(connection string, now int64) Decision {
	if g.control == nil {
		return Allow()
	}
	return g.control.Admit(connection, 1, now)
}

// Streams reports how many streams a connection holds, for telemetry.
func (g *StreamGuard) Streams(connection string) int { return g.streams[connection] }
