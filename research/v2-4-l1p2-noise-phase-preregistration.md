# V2-4 L1-P2 — liability/noise phase-decomposition screen

Status: **preregistered before the noise-flow phase implementation,
instrumentation, configuration rendering, smoke execution, or L1-P2
simulation.** It follows the completed L1-P screen, whose same-direction
descriptive gap/fill differences make an all-zero cadence assumption untenable
but do not identify a counterpart.

## Question

The L1 liability hedger decides every two seconds. The broad legacy
`noise_flow_*` `RandomTaker` population also decides every two seconds. In
L1-P, P0 placed both on the original phase and P1 shifted only the liability
hedger by one second. The observed lower P1 mean local gap could therefore be
a relative liability/noise decision-time interaction rather than a generic
property of a later liability decision.

L1-P2 asks:

> Holding L1-B policies, quantities, seeds, frequencies, prices, fees,
> latency, all non-noise clocks, and the entire participant population fixed,
> does the relative phase between the liability hedger and the named broad
> `noise_flow_*` population explain the observed local-gap contrast?

The phase control applies **only** to `feesim.RandomTaker` instances connected
as `noise_flow_*`. It does not change option flow, future flow, latent
liquidity, suppliers, makers, routers, market-data/request latency, funding,
listing, expiry, settlement, or liquidation clocks.

## Factorial arms

Both periodic intervals remain exactly two seconds. Each first callback is
`simulation_start + interval + phase`, then repeats every two seconds.

| arm | liability decision phase | broad noise-flow decision phase | relative phase |
| --- | ---: | ---: | --- |
| A | 0 s | 0 s | aligned |
| B | 1 s | 0 s | de-aligned |
| C | 0 s | 1 s | de-aligned |
| D | 1 s | 1 s | aligned |

All four arms are newly run for paired seeds 101 and 103 with full evidence.
The retained L1-P cells are diagnostic history only and are not pooled into
this factorial: L1-P2 adds a new required noise-decision timing evidence
contract.

## Required implementation and evidence contract

`noise_flow_decision_phase_offset` must have the same declared domain as the
liability phase: `0 <= p < noise_interval`. Zero must route through the legacy
ticker path. A nonzero phase changes only the first periodic noise decision;
it consumes no RNG and creates no deterministic-simulation goroutine.

For every evaluated `noise_flow_*` tick in every L1-P2 cell, persist compact
pre-ingress timing evidence containing at least venue, client, role, decision
time, configured phase, action (`SUBSCRIBE` or `EVALUATE`), and submitted
request count. It must be sufficient for an offline analyzer to prove the
actor tick lies on the declared phase lattice and to distinguish a no-order
evaluation from an omitted row. The recorder is optional, append-only,
excluded from the execution checkpoint domain, and must not affect RNG,
scheduling, actor-visible state, matching, or economics.

Before cells are rendered, require:

1. absent and explicit-zero noise phase have identical fresh-process execution
   hashes at GOMAXPROCS 1 and 4;
2. one-second noise phase moves the first noise tick by exactly one second and
   preserves the two-second interval;
3. invalid negative or interval-or-larger noise phase is rejected before a
   simulation runs;
4. independent noise timing replay catches a missing/mismatched phase field,
   an off-phase time, a duplicate tick, and a dropped decision row;
5. noise timing evidence on/off is execution-hash neutral across fresh
   processes and GOMAXPROCS 1 and 4; and
6. ordinary L1 policy/receipt evidence remains valid under all four arms.

Each cell is complete only after final `greeks.json` and `latency.json`, and
must retain: V2 receipt/frontier audit, persisted-evidence artifact hash,
liability policy/phase replay, noise timing replay, full and post-warmup
viability, L1 local activation gates, and immutable analyzer metadata. Raw
evidence may not be pruned.

## Preregistered scoring

Score evidence integrity, both phase activations, ordinary local-policy
integrity, and CDF/USD non-collapse before interpreting the phase contrast.
The predeclared primary descriptive-to-causal metric is each seed's exact
decision-time mean absolute liability gap, `M` (exact sum/sample retained).

Define aligned and de-aligned arm means per seed as:

```
M_aligned   = (M_A + M_D) / 2
M_dealigned = (M_B + M_C) / 2
```

The predicted direction is `M_aligned > M_dealigned` in both paired seeds.
The interaction check is equivalently:

```
(M_D - M_C) - (M_B - M_A) > 0.
```

The L1-P2 relative-noise-phase attribution is **SUPPORTED (screening)** only
if all eight cells have valid evidence and activation, every exercised
liability fill remains gap-reducing, every CDF/USD floor passes, and both
seeds meet the aligned-vs-dealigned direction and positive interaction. It is
**FALSIFIED** if both valid seeds have the opposite aligned-vs-dealigned
direction. Otherwise it is **MIXED**. If either timing intervention fails to
change its own persisted decision timestamps, it is **NOT IDENTIFIED**.

The threshold deliberately makes no claim about price, spread, volume,
wealth, ecological viability, or demand realism. A null or mixed interaction
means only that broad noise-flow alignment is not the identified explanation;
it does not prove clocks are economically irrelevant.

## Registered result — appended after complete extraction

All eight A/B/C/D × seed-{101,103} full-evidence cells completed with final
`greeks.json` and `latency.json`, then passed receipt, persisted-artifact,
liability-policy/phase, noise-timing, activity, and CDF/USD floor contracts.
The initial shell-terminated attempt has neither completion sidecar and is
retained only as `NON_EVIDENCE` under
`artifacts/historical/v2-4-l1p2-attempt0-shell-terminated`.

The scored attempt is **SUPPORTED (screening)** under the criterion above:
both seeds have `M_aligned > M_dealigned` and a positive exact interaction.
The cell-level integer sums, denominators, artifact digests, and exact
interaction numerators are retained in
`artifacts/v2-4-l1p2/l1p2-summary.json`; interpretation is in
`v2-4-l1p2-noise-phase-results.md`.

This supports a narrow relative-clock attribution in the retained L1
population. It does not establish a unique cadence mechanism, phase robustness
of the wider ecology, demand realism, price stability, or a population-change
decision. The next gate is a fresh holdout-seed 2×2 replication, not tuning.
