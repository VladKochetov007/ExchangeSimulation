# V2-4 L1-P3 — untouched-seed relative-phase replication

Status: **preregistered before rendering, running, or inspecting any V2-4
result for seeds 107, 109, or 113.**

## Question

L1-P2 found the preregistered positive relative-phase interaction between the
CDF/USD delivery-liability hedger and broad `noise_flow_*` population in paired
seeds 101 and 103. Those are still only two screening seeds and were used in
the preceding L0/L1/L1-P work.

L1-P3 asks whether the same, already implemented and already measured 2×2
effect appears in untouched seed worlds without any change to policy,
population, price, spread, fees, latency, frequency, or phase values.

## Fixed design

Use new full-evidence 30-minute worlds for seeds **107, 109, and 113**. A
repository search before this preregistration found no previous V2-line use of
those seeds. The arms are exactly the L1-P2 factorial:

| arm | liability phase | broad `noise_flow_*` phase | relation |
| --- | ---: | ---: | --- |
| A | 0 s | 0 s | aligned |
| B | 1 s | 0 s | de-aligned |
| C | 0 s | 1 s | de-aligned |
| D | 1 s | 1 s | aligned |

Every configuration is derived from the immutable L1-P2 A/101 parent while
changing only run provenance, seed, and the two declared phase fields. It
retains full logging, V2 receipts, liability decision evidence, and noise
phase evidence. No simulator or analyzer source change is permitted in this
replication.

## Required evidence

Each cell is complete only after final `greeks.json` and `latency.json`, and
must retain a valid receipt audit, persisted-evidence artifact hash, liability
policy/phase replay, noise-phase replay, full/post-warmup viability, the L1
local activity gate, and immutable analysis metadata. Raw evidence is retained
and may not be pruned. The P2 launch-shell attempt is historical non-evidence
and is irrelevant to P3.

## Registered endpoint and score

For each seed, preserve exact `M_arm = absolute_gap_sum/gap_samples`. Define:

```
M_aligned   = (M_A + M_D) / 2
M_dealigned = (M_B + M_C) / 2
interaction = (M_D - M_C) - (M_B - M_A)
```

The prediction is `M_aligned > M_dealigned` and a positive interaction in all
three untouched seeds.

Verdict:

- **SUPPORTED (screening replication):** all 12 cells have valid evidence and
  activation, all exercised liability fills are gap-reducing, every floor
  passes, and every one of seeds 107/109/113 has the predicted direction and
  positive interaction.
- **FALSIFIED:** all three valid holdout seeds have the opposite aligned versus
  de-aligned direction.
- **MIXED:** any other valid outcome.
- **NOT IDENTIFIED:** a phase intervention or required evidence contract fails.

This remains a replication of one local endpoint, not a price, demand,
ecology, or unique-LCM claim. It does not license L2 population changes. If it
replicates, later clock work must still vary other counterpart clocks and
nearby phases before a general timing claim.
