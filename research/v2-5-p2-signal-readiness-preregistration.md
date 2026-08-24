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

## Result — retained-evidence extraction

Status: **public-signal ready in both B seeds.** The independently written
analyzer at `8c29024` (SHA-256
`6120c632b2e33120bbe5fc9fe57c80c4c62b0f1b046a91cbf0f6e87bbed1cac4`) was
run over all four full-evidence P2a cells. Its machine-readable verdict is
[`p2-signal-readiness-verdict.json`](artifacts/v2-5-p2/p2-signal-readiness-verdict.json);
per-cell artifacts are retained as `perpsignals.json` beside the raw evidence.

| cell | all required venues observed | mark updates per venue | funding updates per venue | pooled distinct mark/index pairs | pooled distinct rates | settlements | ready |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| A-101 | yes | 298 | 298 | 197 | 3 | 0 | yes |
| A-103 | yes | 298 | 298 | 195 | 4 | 0 | yes |
| B-101 | yes | 298 | 298 | 196 | 3 | 0 | yes |
| B-103 | yes | 298 | 298 | 202 | 4 | 0 | yes |

There are zero invalid mark/funding records in every cell. In particular, the
last published rate is a present numeric zero for north and south in seed 103;
it is retained as such, not treated as unavailable. Zero funding settlements
are expected because the horizon is five minutes while the configured funding
interval is eight hours.

This passes only the declared feed-readiness condition. A is also ready in
both seeds, so the retained evidence does **not** show that P2 hedges caused
the variation. Nor does it make P1a active: the fee-aware four-leg policy has
no submitted inventory-changing order. P2b therefore remains prohibited until
a separately designed viable carry participant passes its own local-economic
activation contract.
