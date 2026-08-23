# V2-3 P0 replacement run plan — immutable A/B/C screen

Status: **preregistered after resolving attempt-0 scope defect and before the
first replacement run.** No result appears here.

Attempt 0 is retained and invalidated in
[`v2-3-p0-invalidated-attempt.md`](v2-3-p0-invalidated-attempt.md). This
replacement changes no causal definition, seed, horizon, price objective, or
population parameter. It corrects only the implementation scope of a field
explicitly named `spot_passive_maker_*`: it is now applied only to actual
`SpotInstrument`s, never perp or other derivative makers.

## Inputs and scope

Six five-minute, full-evidence worlds use immutable rendered inputs:

```text
research/configs/v2-3-p0-r1/{A,B,C}-{101,103}.json
```

`scripts/render-v2-3-p0-configs.sh` derives them from
`frozen-baseline-2026-08-22.json` and mechanically rejects any undeclared
structural difference. The allowed differences are provenance labels, seed,
full logging, V2 receipt switches/roles, and the two P0 policy fields. Clocks,
latency, spreads, inventory control, demand, and population remain inherited.

The policy covers the refreshable population on **spot books** (including the
cross-asset spot graph). The primary viability screen is CDF/USD. It does not
alter the perp Stoikov maker or fixed/imbalance makers configured on
derivatives.

`scripts/run-v2-3-p0-cell.sh` writes to
`research/artifacts/v2-3-p0-r1/` and refuses any pre-existing cell. Completion
is defined solely by final `greeks.json` and `latency.json`; raw evidence stays
retained through scoring.

## Arms and interpretation

| Arm | Post-only admission | Actor submission order |
| --- | --- | --- |
| A | no | legacy submit-before-cancel |
| B | yes | legacy submit-before-cancel |
| C | yes | cancel-before-replace |

- **B − A** identifies the exchange-level arrival-time post-only admission
  contract. It does not identify cancellation ordering.
- **C − B** identifies the actor's requested cancellation/replacement order.
  Network/request latency continues to determine actual venue arrival; the
  treatment is not atomic replace.

The five-minute two-seed screen is mechanism identification only—not a price
stability claim or a robustness result. No price-path statistic is a fallback
for an inactive mechanical comparison.

## Required evidence and fixed gates

Extraction must retain, per cell:

1. a valid V2-0 receipt/decision audit for declared `spot_maker`,
   `fixed_distance_maker`, and `imbalance_maker` feed roles;
2. exact persisted-evidence artifact hash;
3. pooled and per-role CDF/USD post-only activity for `cdf_spot_maker`,
   `fixed_distance_maker`, and `imbalance_maker`;
4. a derivative-scope audit: `ABC-PERP` orders from the perp, fixed-distance,
   and imbalance maker roles must have zero accepted post-only orders;
5. raw CDF/USD 60-second venue windows, with snapshots, empty-side count,
   two-sided share, trades, and volume;
6. analyzer revision/hash and completion-sentinel metadata.

For every arm/seed: each CDF maker role must have an accepted order or an
explicit post-only rejection; every CDF venue must have a snapshot; pooled
two-sided share, trades, and volume must be nonzero. Failing a gate is reported
as non-viable or not activated—not tuned away.

The exchange-level invariant is also established by the adversarial unit
mutation: a marketable post-only request rejects without ID allocation,
reserve/borrow, fill, or book mutation, while stripping its bit creates the
ordinary fill. The screen reports any actual would-take rejection, but does
not require one on CDF/USD specifically.
