# V2-2b — informed makers × executable routers smoke factorial

## Question and scope

Can the already-audited V2-1 informed-maker path and V2-2 router path both
activate in a world where temporary venue-local dislocations arise from actual
execution objectives rather than a seeded crossed book or a direct price
repair rule?

This is a five-minute feasibility/measurement factorial, not a calibration
run, a realism result, or a robust causal price-discovery claim. It is Tier A
for simulator determinism and evidence checks, but only Tier C for any
market-level interpretation.

## Fixed population cell

Starting configuration: `research/configs/frozen-baseline-2026-08-22.json`,
with these resolved deltas:

```text
maker_anchor                 = own_mid
metaorder_trader_count       = 1 per venue
metaorder min/max parent qty = 100 ABC
metaorder child qty          = 5 ABC
metaorder participation rate = 0
metaorder max slippage       = 50 bps
metaorder rest interval      = 5 minutes
cross-venue router lot       = 0.01 ABC
cross-venue router latency   = 1 second on every router link
cross-venue max attempts     = 100
duration                     = 5 minutes
seeds                        = 101, 103
```

The metaorder signs are independently seeded and are not functions of price.
Each parent has a finite execution quantity, latency, slippage cap, funding,
and a recorded completion/abandon outcome. It supplies a local execution
objective, not a terminal price, cross-venue order, common fair value, or
router-specific quote.

## Factorial arms

| arm | local cache | remote maker feeds | router | required evidence roles |
|---|---|---|---|---|
| I0R0 | yes | no | no | `spot_maker` |
| I1R0 | yes | V2-1d three-policy roster | no | `spot_maker`, `v2_remote_feed` |
| I0R1 | yes | no | one V2-2 tier | `spot_maker`, `cross_venue_router_tier` |
| I1R1 | yes | V2-1d three-policy roster | one V2-2 tier | all three |

Every evidence-bearing arm enables both scalar V2-0 receipts and V3 decision
vectors. Router cells use the V2-2 three-venue frontier contract. All worlds
keep raw logs through the smoke measurement contract; no evidence is pruned.

## Preregistered activation checks

1. Every venue has one parent execution record with nonzero planned quantity;
   each completed/abandoned status and VWAP availability is explicit.
2. I1 cells have exactly the declared feed-only remote sessions, activated
   remote caches, and valid V2-0/V3 evidence.
3. R1 cells have valid V2-0/V3 router evidence and zero route before any
   three-venue delivered frontier.
4. At least one R1 seed has a submitted route. If neither does, router
   activity is **NOT IDENTIFIED** and the factorial stops before an outcome
   interpretation.
5. Every router report reconciles its per-leg quantities/notionals/fees to its
   aggregate report. A residual must remain visible if a leg fails; a zero
   residual in a complete group is not evidence of atomic execution.

## Measurements

Primary feasibility outputs:

- an independently reconstructed, same-time/staleness-bounded cross-venue
  midpoint-dispersion distribution;
- an independent after-fee executable-edge distribution and lifetime;
- router submitted/completed/failed/residual groups from the direct report;
- parent execution state, positive VWAP availability, and signed impact.

The cross-venue dispersion and executable-edge analyzer must be tested against
synthetic stale, missing-side, and known-dispersion fixtures before it is used.
An omniscient offline edge scan is an upper-bound diagnostic; it is never
labelled as a router observation or router PnL.

## Predictions and decision rules

| claim | prediction | falsifier / interpretation |
|---|---|---|
| local execution produces dislocation | all arms show nonzero fresh cross-venue dispersion during at least one parent | source is inactive or the metric is weak; do not score channels |
| router activation | R1 reduces residual executable-edge time relative to corresponding R0 in both seeds | equal/higher residual may be latency/leg-risk feedback; report, do not tune |
| informed-maker activation | I1 has the exact receipt/vector contract and route-independent quote decisions | missing evidence = NOT IDENTIFIED |
| channel separation | router activity is attributed only to its accepted/fill-qualified legs; quote activity only to receipt-backed maker decisions | unlinked events cannot be counted for either channel |

No direction for total dispersion is promoted from this smoke: delayed informed
quotes and non-atomic router legs can each widen or narrow a short-horizon
dislocation. A two-seed effect is screening evidence only and must not be
called robust.

## Mutations and stop rules

The existing V2-0/V3 future/drop/reorder mutations and V2-2 missing-router
frontier mutations are release gates. The new market metric must catch an
injected stale quote, a one-sided quote, and a known cross-venue midpoint gap.
If the metric cannot distinguish these cases, do not run the eight-cell smoke.
If a raw-log or sidecar defect appears, preserve the run as diagnostic history,
fix and mutate the analyzer, then regenerate only affected cells.

## Pre-campaign measurement gate

Passed before any factorial world: `analysis.MeasureCrossVenueDispersion` has
independent synthetic fixtures for a known three-venue midpoint gap, stale
quotes, a one-sided quote, and two same-time publications whose persisted file
ordinal selects the final quote. The metric deliberately rejects unavailable
quotes; it has no zero-price representation. Verification command:

```text
go test -race ./analysis ./cmd/mvanalyze -run 'Test(CrossVenue|Arbitrage)' -count=1
```

This is only an analyzer gate. It does not establish that a population-wide
dislocation will activate in any factorial cell.

## Immutable cell inputs

The eight exact inputs are
`research/configs/v2-2b/{I0R0,I1R0,I0R1,I1R1}-{101,103}.json`, rendered by
`scripts/render-v2-2b-configs.sh` from the retained frozen baseline plus only
the deltas declared above. `scripts/run-v2-2b-cell.sh` refuses an existing
cell directory, saves `run-config.json` and a pre-run metadata record, and
runs the five-minute horizon. `greeks.json` plus `latency.json` are the only
completion sentinels; a process name or a partial directory is never treated
as a completed world.
