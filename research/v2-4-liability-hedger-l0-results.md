# V2-4 L0 results — finite-capital delivery-liability hedger

Status: **SUPPORTED (screening), mechanism integrity only.** This is a
five-minute A/B × seed-101/103 activation screen, not robustness evidence and
not a claim about CDF/USD stability, realistic demand elasticity, liquidity,
or a replacement for `noise_flow`.

## Provenance and evidence gate

The four registered cells used immutable configs in
`research/configs/v2-4-l0/`, source revision
`5d5df3175cefd055523b4377d4eee20091d235be`, rebuilt
`multivenue` binary SHA-256
`692261802d7359c4a4cd297ea6ec90b33a6040a472cd46baec494c80de1c07cc`,
and `GOMAXPROCS=4`. Raw full evidence remains retained in
`research/artifacts/v2-4-l0/{A,B}/seed-{101,103}`; the extractor did not
prune anything.

All four cells have final `greeks.json` and `latency.json` sentinels, valid
V2-0 receipt sidecars and digests, a non-empty persisted-evidence artifact
digest, and a valid independent L0 replay with zero checks. The replay uses
only the preserved config, V2 receipt ledger, generic gateway decisions and
exchange outcomes, and raw L0 decision/fill evidence; it does not call the
actor implementation.

| arm / seed | L0 decisions | obligation transitions | submitted / accepted | fills / filled CDF | tail defers | L0 replay |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| A / 101 | 450 | 90 | 0 / 0 | 0 / 0 | 0 | valid |
| A / 103 | 450 | 90 | 0 / 0 | 0 / 0 | 0 | valid |
| B / 101 | 450 | 90 | 310 / 310 | 480 / 15,689,798,578 | 0 | valid |
| B / 103 | 450 | 90 | 282 / 282 | 466 / 15,599,737,574 | 6 | valid |

Each of the three venue-local hedgers recorded 30 nonzero obligation updates in
every cell, exceeding the preregistered minimum of 20. In A, the three initial
`NOT_SUBSCRIBED` rows are followed by 447 `POLICY_DISABLED` rows; no L0
request was emitted. In B, every submitted request had an exact prior local
snapshot receipt, named direction, executable touch, IOC terms, ordinary
positive taker fee, different-client counterparty, and actor-local fill whose
post-fill `abs(obligation - position)` was strictly lower. No assertion was
rescued by an aggregate inventory or price statistic.

Seed 103 had six explicit `SIMULATION_HORIZON_CENSORED` decisions. They are
not missing outcomes: the new actor policy deliberately avoided requests whose
venue execution and delayed actor response could not both be observed before
the fixed horizon. Seed 101 was in-band at those final opportunities.

## Interpretation

The preregistered primary claim is supported at screening level:

> A finite-capital participant with an evolving, independently observable
> delivery obligation can submit locally informed, capped, ordinary-cost
> CDF/USD hedges whose direction reduces its own explicit hedge gap.

This result establishes only that local mechanism. It does **not** establish
that the liability process is empirically realistic, price elastic, stabilizing,
or sufficient to replace an activity generator. L1 must use a separately
preregistered population-replacement control, viability gate, phase sweep, and
longer holdout seeds. The 15-second pre-registration startup smoke that first
revealed tail-response censoring remains in `scratch/v2-4-l0-smoke-gsNxBi` as
diagnostic history and is not a registered cell.
