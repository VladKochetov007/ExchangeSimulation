# V2-3 P0 run plan — immutable A/B/C screen

Status: **preregistered before the first P0 run.** No result appears here.

## Inputs and horizon

Six five-minute full-evidence worlds use these immutable rendered inputs:

```text
research/configs/v2-3-p0/{A,B,C}-{101,103}.json
```

`scripts/render-v2-3-p0-configs.sh` derives each from
`frozen-baseline-2026-08-22.json`. A source-versus-rendered structural diff
permits only experiment/provenance labels, seed, full logging, the declared
V2 receipt roles, and the two P0 policy fields. The configs retain the frozen
population, consensus reference, latency, clocks, inventory control, spreads,
and demand exactly as inherited.

Each run uses `scripts/run-v2-3-p0-cell.sh`, which refuses an existing output
directory, records binary/config/revision hashes before starting, and accepts
only final `greeks.json` plus `latency.json` as completion sentinels. Raw logs
and receipt sidecars remain retained through scoring.

## Required artifacts per cell

`scripts/extract-v2-3-p0-metrics.sh` must complete successfully before a cell
can be interpreted:

1. `observationreceipts.json` — independently valid V2-0 receipt/decision
   audit for `spot_maker`, `fixed_distance_maker`, and `imbalance_maker`;
2. `evidenceartifacthash.json` — exact persisted-evidence artifact identity;
3. `postonly-cdf.json` plus the three `postonly-cdf-<role>.json` artifacts —
   pooled and per-maker-class accepted/rejected/fill-qualified CDF/USD
   activity. The per-role files make the activation condition testable rather
   than inferring it from a nonzero pooled count;
4. `viability.json` and `cdf-viability.json` — raw 60-second CDF/USD venue
   windows, including snapshots, one-sided count, two-sided share, trades,
   volume, and maker/taker-class counts;
5. `analysis-metadata.json` — analyzer revision and hash plus the completion
   contract.

The final P0 report must state raw values per seed. It must not replace a
failed primary mechanical comparison with a prettier price path.

## Fixed activation and viability gates

For each arm/seed:

- **evidence:** every declared receipt role has a catalog row with nonzero
  schedules and receipts; the audit is valid;
- **maker activity:** each declared maker class has at least one CDF/USD
  accepted order or explicit post-only rejection, so an inactive class cannot
  masquerade as passive compliance;
- **quote presence:** each venue has at least one CDF/USD snapshot;
- **two-sided availability:** pooled CDF/USD two-sided snapshot share is
  reported and must be nonzero to call the book available;
- **trade activity:** pooled CDF/USD fill volume and trades are reported and
  must be nonzero to call the book active.

The last three are viability gates, not stability objectives. A cell that
fails one is reported economically non-viable at this population; no
parameter is tuned and no terminal-price statistic changes that result.

## Causal scores

- **B − A:** exchange-level passive-admission mechanics only. It is supported
  only if selected B quote requests are explicit post-only and a would-take
  rejection has no admission mutation, as independently covered by the
  exchange tests and persisted activity evidence.
- **C − B:** actor submission-order mechanism only. It is supported only if
  the requested cancel/replace ordering is confirmed by the actor tests and
  any observed viability/rejection/inventory difference is reported without
  calling it a post-only effect.

The five-minute, two-seed screen is mechanism identification, not a
price-stability claim or a robustness result.
