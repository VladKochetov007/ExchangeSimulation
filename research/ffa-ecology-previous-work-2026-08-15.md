# FFA Ecology Previous Work

## Current baseline

- The accepted multivenue baseline is three independently funded deterministic
  venues on one phase-ordered clock. Each venue lists only `ABC/USD`,
  `ABC-PERP`, rolling dated `ABC` futures, and rolling European `ABC` options.
  It has local margin borrowing, one A-S-inspired spot maker, one perpetual
  maker, a futures maker, an option dealer, seeded random spot takers, and
  seeded option takers. See [sim.go](../simulations/multivenue/sim.go).
- The default world has one noise and one option-flow participant per venue;
  roster four is a denser control, not a broad participant ecology.
- FFA-01 now selects existing price-time or exact pro-rata matching per venue
  through `VenueRules`, while preserving a price-time default. Allocation
  minimums, top allocation, split/leveling, auction rules, and hybrids remain
  unimplemented and must not be implied by the `pro_rata` label.
- `CrossVenueArb` is a non-atomic, pre-funded FOK router for one cross-listed
  symbol, `ABC/USD`. It accounts for per-venue legs and residuals, but it is
  not an FX graph, a clearing layer, or a multi-asset arbitrage population.

## What was tried and kept

- The deterministic phase runtime is a hard research gate: direct and
  scheduler-backed accepted configurations reproduce across `GOMAXPROCS=1`
  and `14`. E-021, E-027, E-029, E-031, E-034, and E-037 in
  [experiments.jsonl](experiments.jsonl) retain commands and artifacts.
- Exact-tenor rolling options/futures, derivative logging, option premium
  valuation, strict USD marked-account reporting, and positive-horizon Greek
  telemetry were retained before the 3-venue Greek and hedge controls.
- Execution experiments show a controlled TWAP advantage only in the tested
  replenishing order-book ecology; a one-venue executable latency-arbitrage
  signal was falsified after fees. These are mechanisms, not a general
  ecology result.
- The linear maker has an A-S-inspired reservation-price/spread control. It
  has not yet been compared with the previous linear skew under common random
  numbers. See [stoikov-control-audit-2026-08-15.md](stoikov-control-audit-2026-08-15.md).

## What is not yet present

- A minimal `ABC/CDF`, `ABC/USD`, and `CDF/USD` graph is implemented, but its
  first strict control is superseded pending revalidation after an `ABC/CDF`
  quote-precision/variance-unit correction. Dozens of assets and cross-listed
  derivatives remain absent.
- Per-venue matching-rule heterogeneity, venue-specific tick/minimum/fee
  regimes, auction mechanisms, self-match policy, and a clearing/transfer
  model are absent.
- There is no capital-weighted survival, bankruptcy, mutation, reproduction,
  strategy selection, or population-mixture tournament.
- Agents are hand-coded policy actors. They do not estimate an opponent model,
  learn, or evolve. A public schedule and contract metadata are currently
  available through the ordinary instrument feed; an explicit information-set
  audit has not yet been added.
- Existing option models use static IV and a spot-forward proxy. They are
  unsuitable for realised-volatility, smile, or vega-PnL claims.
- A complete multi-currency marked-account/FX graph has not been implemented.
  Current accepted valuation is USD-only and fails closed when a necessary
  two-sided conversion mark is missing.

## Known fragile areas

- Before FFA-00, `OptionBuyProbability=0` was normalized to `0.65` in the
  multivenue configuration (and `OptionPBuy=0` to `0.5` in derivsim). Any
  all-sell option-flow result was therefore invalid. Nullable configuration
  plus regression coverage is a mandatory prerequisite for directional-flow
  arms.
- Derivative per-execution fee fragmentation remains outside the exact spot
  fee-plan admission path. Do not use arbitrary fixed derivative fees in
  economic experiments.
- The current A-S control uses an uncalibrated fill-decay parameter and a
  rolling horizon. It is not a proof of optimal market making.
- A cross-venue FOK router has non-atomic leg risk by design. It must retain
  incomplete groups and terminal exposure rather than discard failures.
- Existing `feesim` and `randomwalk` triangle actors are not suitable for FFA
  evidence: their cross-leg sizing, partial-fill handling, fee accounting, and
  residual lifecycle are incomplete. Preserve them as historical prototypes;
  do not extend or delete them while the replacement is specified and tested.
- Lossless feed/backpressure and market-data recovery are still explicit
  simulator gaps at high event rates.
