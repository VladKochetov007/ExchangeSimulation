# ME-005 preparation contract — prospective, no economic outcome

Source baseline: merged `main` `eb921aa29b26b43cd39e1613dabc44ec60dbf5c9`.
This document resolves the design choices needed to implement the four fixes in
[the readiness assessment](readiness-20260924.md). It is not a development
preregistration, a reviewed source candidate, or permission to run a world.
ME-001/002/003 results and historical three-venue configs remain unchanged.

## Four required fixes

| Fix | Why and current gap | Minimal change | Economic status | Acceptance |
|---|---|---|---|---|
| Two-venue wiring | The config and router require exactly three venues; router accounts receive fixed large endowments. | Permit exactly two or three distinct configured venues, retaining the three-venue default. Bind a new two-venue router to two separate accounts and explicitly finite base/quote endowments, lot and attempt cap. Disable automatic borrowing in the ME-005 candidate. | New two-venue population and funding contract; not a correction to old runs. | Reject one/duplicate venue, preserve three-venue behavior, verify deterministic venue ordering, distinct accounts, full delayed prefixes, finite balance admission and a two-venue production fixture. |
| Opportunity/execution reconstruction | `ExecutableSignals` counts only route-opening policy signals; submitted-order vectors omit no-action evaluations. Inbox receipt is not proof that the actor consumed a message. | Independently replay public ABC/USD snapshot-plus-delta states and delivered feed prefixes; add optional evidence for every router quote evaluation, including no-action reason and consumed frontiers. Join group/leg request, admission, exchange-time fill/cancellation and actor receipt. | Measurement only; the observer must not alter decisions, RNG or event scheduling. | Positive/zero/negative/overflow/depth/stale/no-action/one-leg fixtures; dropped, duplicate, wrong-fee and wrong-frontier mutations fail closed; evidence-on/off execution parity. |
| Costed closeout | Completed quote cashflow omits local ABC shifts even when global ABC residual is zero. | Use the fixed-horizon venue-local executable *liquidation-value* convention below, reconstructing each router account's ABC/USD changes and charged fees from canonical events. | Prospective endpoint convention, not a simulator transfer or executed liquidation. | Independent ledger and fill reconciliation; multi-level bid/ask, fee rounding, one-leg, insufficient-depth, invalid-book and no-debt fixtures. Never report unavailable closeout as zero. |
| Dislocation measurement | The existing cross-venue measure is periodic midpoint dispersion with a three-venue default, not an event-time executable edge. | Add a two-venue, event-ordered, fee/depth-adjusted one-lot edge and episode estimator. Compare registered router OFF/ON worlds; keep route-attributed convergence distinct. | Measurement only. | Same-timestamp ordering, venue swap, receipt lag, no-op router and sampled-snapshot aliasing fixtures. If causal attribution is not supported, mark convergence `NOT_IDENTIFIED`. |

These are the only readiness fixes. Static tests may expose defects within
their implementations; unrelated simulator improvements are out of scope.

## Prospective economic choice: local liquidation-value closeout

ME-005 will not assume a shared wallet, instantaneous ABC transfer, external
fair value or midpoint. The router remains prefunded and its two FOK orders
remain non-atomic. No liquidation orders are sent into the research market:
doing so would introduce a second trading policy and contaminate the initial
router OFF/ON dispersion comparison. Instead, at one preregistered terminal
exchange state, reconstruct each router account's change from its own initial
balances: `base_delta[v]` in ABC atoms and `quote_delta[v]` in USD atoms.

For positive `base_delta[v]`, value a hypothetical sale into **that venue's**
visible bids, best price first; for negative `base_delta[v]`, value the
necessary hypothetical repurchase from that venue's visible asks. Walk
displayed depth only up to the exact required base quantity, applying the
declared taker-fee and integer-rounding convention per consumed price level.
The price-level calculation is a valuation convention, not a claim that an
actual multi-maker sweep would have identical per-fill rounding or admission.
An empty, stale, invalid, out-of-domain or insufficient-depth book makes the
full terminal liquidation value **unavailable**, with its reason retained.
Partial visible coverage is reported separately; no infinite-depth fallback.
Borrowing, transfers and financing are excluded only if the pinned candidate
proves no debt and no such cashflow for the router. Otherwise the applicable
cost must be reconstructed or the result is unavailable.

Let `L_v(base_delta[v])` be the signed USD cashflow of that local hypothetical
liquidation, including its fees. The portfolio result is

`terminal_liquidation_value = sum_v quote_delta[v] + sum_v L_v(base_delta[v]) - financing_cost`.

For each attempt, independently report actual buy/sell quantities, proceeds,
costs and charged fees. A completed matched pair's *gross matched-leg
cashflow* is not its terminal liquidation value. A one-leg failure has its own
cashflow and ABC exposure. The registered first study should use at most one
attempt per world so terminal closeout is attributable to that attempt without
arbitrary allocation of shared depth among multiple attempts. If a later
study allows several attempts, value the net venue portfolio once and do not
pretend per-attempt closeout costs are additive.

This convention tests an untransferred, locally restored route. With unchanged
books, it generally pays the spreads and fees of local round trips; a negative
value under this convention is **not** evidence that an explicit delayed,
costed intervenue-transfer strategy would also lose. Conversely, positive
matched cashflow alone is not realized transferable arbitrage profit.

## Opportunity and convergence scope

Freeze the current router's additional *two-sided-book* eligibility condition
for the first candidate. Independently count broader leg-side quote edges as
a diagnostic, never as a retrospective reason to change the router. At every
relevant public book transition and consumed local-feed evaluation, classify
missing/stale, unsupported positive-spot domain, insufficient one-lot depth,
nonpositive after-fee edge, positive policy quote, balance-feasible action,
in-flight/attempt-cap abstention, request, admission, fill and terminal
inventory. A quote-time edge is not guaranteed to execute after latency.
Measure public opportunity lifetimes as half-open event-time episodes; record
price, depth, timing, other execution and horizon-censor reasons where the
retained event identities actually distinguish them. Unknown cause stays
unknown.

The separate two-venue dislocation series uses the maximum positive
one-lot, depth-and-fee-adjusted edge across both directions. A paired OFF/ON
world comparison may estimate a bounded *world-level router-enable effect*
under a verified matched-background design. Temporal proximity of a router
fill and a quote change does not identify trade-mediated convergence; if
the event identities cannot distinguish router action from common-background
movement, report route-attributed convergence as `NOT_IDENTIFIED`. The old
periodic midpoint statistic remains descriptive and separate.

## Preliminary read-only challenges, not acceptance

Fresh Sol-6 medium reviewers `01a0d1fe-52e9-7753-8d20-b0e6a74c6b83` and
`01a0d201-bde5-7782-94cf-5ae46c843e91` inspected source tip `3545ee1`
without edits or worlds. The economic critique accepted the *coherence* of a
terminal local mark only if labelled hypothetical and flagged its built-in
stationary-book spread cost and non-additive multi-attempt allocation. The
evidence critique required a record of consumed-feed evaluations and warned
that the receipt ledger alone proves inbox delivery, not callback consumption.
Neither reviewed the eventual implementation, preregistration or any result.
Two fresh bounded reviews of the exact completed candidate remain required
before any ME-005 economic world.
