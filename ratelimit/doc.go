// Package ratelimit models the request budgets exchanges publish, and the
// admission decisions a venue makes when its execution queue saturates.
//
// The pieces are deliberately separate so a venue's scheme is a composition
// rather than a special case:
//
//   - a CostModel prices each request kind, which is how weight-based schemes
//     charge more for a deep order book than for a ping;
//   - a Limiter decides whether a cost fits the budget now, with fixed-window
//     and token-bucket implementations covering the documented schemes;
//   - a Scope decides whose budget is charged, since published limits are
//     variously per connection, per IP and per account;
//   - an AdmissionQueue decides what happens once the engine itself is the
//     bottleneck, keeping risk-reducing requests in a lane that saturates last.
//
// # Concurrency
//
// Nothing here is safe for concurrent use. The limiters hold per-scope maps and
// the queue holds plain counters, so two goroutines admitting at once would
// either panic on a concurrent map write or silently lose a counter update. The
// second failure is the dangerous one for a simulator: depths drift, overload
// decisions differ between runs, and byte-reproducibility is lost without
// anything crashing.
//
// This is deliberate. The simulator drives the venue from one goroutine, and a
// mutex in the admission path would cost every request to protect against a
// caller that does not exist. A caller that does need it should wrap a limiter
// rather than pay for locking here.
//
// Nothing here assumes a particular exchange. Binance's published spot scheme
// is one composition of these parts; a venue that charges a flat cost per
// request and never overloads is another.
package ratelimit
