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
  filled cost at the tested mid size. See `executionlab-2026-08-15.md`.
- Low latency: transport is now deterministic, but no valid tier-profit result
  exists yet. Build a two-leg execution ledger and run label-permutation and
  completion-adjusted trials.
- Hedged options: the switch reduces delta in several pilot cells but not all;
  gamma/vega stay as expected residual risks. See
  `options-hedge-pilot-2026-08-15.md`.

## Non-Negotiable Experiment Gates

1. Fixed seed produces identical canonical output across at least two GOMAX
   settings.
2. Every order-based metric reports requested, filled, cancelled, rejected,
   fees by asset, and residual inventory.
3. All PnL/equity claims use an explicit numeraire and mark hierarchy.
4. Static-IV reports label vega as sensitivity, never realized vega PnL.
5. A model result that fails completion, determinism, or accounting is a bug
   report, not market behavior.
