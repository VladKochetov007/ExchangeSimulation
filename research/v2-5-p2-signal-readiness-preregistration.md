# V2-5 P2 signal-readiness audit preregistration

Status: **preregistered before aggregate signal extraction.** This is a
descriptive evidence-readiness audit over the retained P2a raw logs, not a
test that physical hedgers caused funding, a funding/carry treatment, or a
basis result.

## Question

P2a has already established the actor-local order path. Before proposing a new
carry-participant representation, determine whether that exact five-minute
evidence contains a public `ABC-PERP` mark and funding signal that a later
participant could legally receive. P1a remains `NOT EXERCISED`; no result here
may be used to reinterpret its fee-aware hurdle.

## Inputs and independent observable

Read only persisted `mark_price_update` and `funding_rate_update` events for
`ABC-PERP` from all four retained P2a cells, in their physical venue streams.
For every venue, retain event counts, distinct `(mark_price,index_price)`
pairs, distinct funding-rate values, and explicit first/last values with an
availability bit. An event containing price/rate zero is present evidence, not
an absent value. The audit must not import a P2 actor, a funding policy, or
simulator state.

The companion analyzer must also count `funding_settlement` balance changes.
The registered five-minute horizon is shorter than the eight-hour funding
interval, so zero settlements are expected and are neither a missing signal
nor a semantic failure.

## Readiness rule declared before extraction

The enabled B arm is **public-signal ready** only if, in each seed:

1. every one of central, north, and south has at least one `ABC-PERP` mark
   update and at least one funding-rate update;
2. pooled across the three venues, it has at least two distinct
   `(mark_price,index_price)` pairs; and
3. pooled across the three venues, it has at least two distinct funding-rate
   values.

A or B parsing failure, a missing required venue, or a failed independent
metric invalidates the audit. A failed readiness rule means only that a later
funding-response market screen is `NOT IDENTIFIED` on this retained signal; it
does not authorize a fee, demand, clock, spread, funding-rate, or population
change. A passing rule establishes public-input variation only—never causal
funding economics—and does not authorize P2b while no viable carry actor has
submitted ordinary inventory-changing orders.

## Required outputs and mutations

Write `perpsignals.json` for all four cells before any new run. The analyzer
must reject malformed mark/funding payloads and distinguish absent data from a
present zero. Fixtures must catch a dropped mark update, a dropped funding
update, a zero rate incorrectly classified unavailable, and a falsely counted
funding settlement. Retained P2a evidence remains non-prunable.
