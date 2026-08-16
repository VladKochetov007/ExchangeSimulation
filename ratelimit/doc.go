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
// Nothing here assumes a particular exchange. Binance's published spot scheme
// is one composition of these parts; a venue that charges a flat cost per
// request and never overloads is another.
package ratelimit
