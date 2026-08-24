# V2-5 P2a — physical-exposure hedge result

Status: **SUPPORTED (screening), narrowly.** This result supports an
activation claim only: bounded, named physical ABC exposure can reach a
locally informed, delayed, ordinary perpetual hedge path. It does **not**
support a funding-anchor, carry-profitability, basis, price-stability, or
realism claim.

## Provenance

| item | value |
| --- | --- |
| source commit | `1eed365a832dd920beed16f6ba52f3724a8d7699` |
| simulator SHA-256 | `971a4e159ee31b6c7e920656fa567ed8a3fed95bd22aa42cffc6f7b226a51f1f` |
| analyzer SHA-256 | `9580e3b4c1817f5b9105c8f916d18d316f301ad4085419250b6fac42257447bc` |
| prune-gate SHA-256 | `44676384409b9f0c96c94236d0623585f9a9ecbea5b8b1d539ae20c0aa82657c` |
| horizon / logs | 5 simulated minutes / full retained evidence |
| pairs | A/B × seeds 101, 103 |

Each cell has nonempty final `greeks.json` and `latency.json`, raw venue logs,
manifest, checkpoints, receipt sidecars, and all preregistered analyzer
artifacts. The compact latency sidecars report every P2 link at 40-ms delivered
market-data and 20-ms request latency. `perpexposurehedger` and
`observationreceipts` both pass with zero audit failures; conservation and
positions also pass. The generic prune gate refuses to mark the nested P2
cells safe to prune because it has no dedicated P2 contract. No evidence was
pruned.

## Paired activation result

| seed | arm | decisions | state updates | submitted / accepted / filled | IOC terminal cancellations | gap sum | terminal per-venue gaps (C/N/S) |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 101 | A | 450 | 90 | 0 / 0 / 0 | 0 | 11,770,000,000 | 0 / 40m / 40m |
| 101 | B | 450 | 90 | 119 / 119 / 119 | 29 | 1,060,701,675 | 0 / 0 / 0 |
| 103 | A | 450 | 90 | 0 / 0 / 0 | 0 | 8,470,000,000 | 20m / 60m / 0 |
| 103 | B | 450 | 90 | 124 / 124 / 123 | 34 | 1,073,491,844 | 0 / 0 / 0 |

`B - A` gap sums are -10,709,298,325 (seed 101) and -7,396,508,156 (seed
103). Every enabled venue had at least one accepted and one fill-qualified
hedge in both paired seeds (B101 fills C/N/S: 40/41/38; B103: 40/42/41). All
filled hedges reduced the actor-local target gap. The one B103 unfilled
accepted IOC and all partial IOC terminal cancellations are retained and
audited, rather than inferred as fills.

The receipt joins are exact: A has 447 matched source receipts per seed; B has
119 (101) / 124 (103) scalar decisions joined to valid local frontiers. There
are no future, missing-due, source-fingerprint, gateway, venue-outcome,
actor-fill, exact-fee, or counterparty errors. B USD conservation residuals
are -9 (101) and -6 (103) raw quote units, the documented bounded integer
truncation residual; A is exact. All ABC conservation and position replay
residuals are zero.

## Reproduction and interpretation

A fresh `GOMAXPROCS=1` B101 replay matches the campaign `GOMAXPROCS=4` cell
exactly for its ordered execution hash, persisted-evidence stream digest,
artifact digest, P2 audit counts, and venue fill counts. It also confirms that
the observation/fingerprint instrumentation did not alter the simulated world.

This is two-seed screening evidence, not robustness proof. P2a records no
scored market/funding/basis conclusion. P2b remains prohibited until a
separately preregistered public mark/funding-variation readiness audit is
complete; P1a's fee-aware four-leg carry policy remains `NOT EXERCISED`.

