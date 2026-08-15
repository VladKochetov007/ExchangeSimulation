# Research Status, 2026-08-15

## Accepted Foundations

- Fixed-seed simulations are only admissible after byte/state repeatability.
  Direct phase simulations and scheduled-latency fee simulations agree across
  `GOMAXPROCS=1` and `14`.
- The engine remains Go-first. Profiling showed scheduler barriers, channel
  orchestration, logging, and repeated map scans dominate, not matching. A
  C++/Rust rewrite would preserve the causal bugs while making them harder to
  inspect. Native kernels remain a future option only after a profiled,
  deterministic contract exists.
- The earlier random-walk graphic defect was real source data in some runs:
  actor fill-before-accept handling and side-blind quote tracking caused stale
  or one-sided books. Terminal-event replay and side-aware quote tracking were
  fixed; plots now preserve missing states instead of forward-filling them.

## Rejected Or Qualified Assumptions

- Coarsening the runner step to make an animation faster changes market
  behavior. Simulation step stays at the smallest model cadence; rendering
  alone controls timelapse compression.
- Equal random seeds did not previously imply equal economies because ticker
  registration, actor select order, ingress, and delayed delivery depended on
  host scheduling. Deterministic phases plus the ordered latency courier are
  now required for research runs.
- Midpoint-only cash-and-carry/parity signals are not executable arbitrage.
  A spread, fees, fill status, expiry TWAP basis risk, and leg residuals must
  be accounted for before any convergence or PnL interpretation.
- Unbounded option relisting is not a harmless realism detail: it changes both
  economics and complexity with path-dependent contract count. The multivenue
  scenario now caps active strikes per expiry.

## Current Evidence Boundary

- Controlled TWAP: neutral at low displayed-depth participation and lower
  completion-adjusted target shortfall at tested 2 and 5 ABC sizes. The 1 Hz
  and 5 Hz schedules tie in this fast-replenishing ecology. See
  `executionlab-2026-08-15.md`.
- Low latency: a deterministic scarce-conversion lab now proves an earlier
  signal observer wins when latency labels are swapped and client/actor order
  are controlled. It is not yet an ecology-level profitability result. See
  `latencylab-2026-08-15.md`.
- Hedged options: 20 seed-level three-venue pairs show lower mean and peak
  absolute delta with hedging. Gamma differs endogenously and static-IV vega is
  a local sensitivity, not realised PnL. See
  `options-hedge-replicates-2026-08-15.md`.
- The linear market makers are Avellaneda-Stoikov-inspired controls, not a
  calibrated optimality claim. See `stoikov-control-audit-2026-08-15.md`.

## Non-Negotiable Experiment Gates

1. Fixed seed produces identical canonical output across at least two GOMAX
   settings.
2. Every order-based metric reports requested, filled, cancelled, rejected,
   fees by asset, and residual inventory.
3. All PnL/equity claims use an explicit numeraire and mark hierarchy.
4. Static-IV reports label vega as sensitivity, never realized vega PnL.
5. A model result that fails completion, determinism, or accounting is a bug
   report, not market behavior.
