# Three-Venue Options Hedge Pilot, 2026-08-15

> **Superseded for conclusions.** This early pilot used actor-local Greek
> observations and had no strict terminal marked-equity report. Retained only
> as implementation history. Use
> [options-hedge-replicates-2026-08-15.md](options-hedge-replicates-2026-08-15.md)
> for accepted 20-seed exchange-owned evidence.

## Question

Does a stateful delta hedge reduce option dealer net-delta exposure in a
fragmented three-venue options ecology, while leaving option gamma and vega as
the residual nonlinear risk?

## Correctness Work Before The Pilot

- Added explicit `dealer_hedge_mode: "on"|"off"` configuration. The config
  now has snake-case JSON tags; previously research JSON silently ignored its
  snake-case fields, including the requested hedge-off arm.
- Dealer starts with zero ABC. Spot short hedges can auto-borrow ABC against a
  USD-collateral price oracle instead of being hidden by a large initial long
  inventory.
- Hedge orders now track signed pending quantity through accept, partial fill,
  reject, and forced cancel. The next hedge targets `actual + pending` rather
  than repeatedly sending the same correction.
- Capped each expiry's active strike count in the multivenue scenario. The
  old recentering policy could add another chain whenever the spot crossed a
  strike grid.
- Cached solvent cross-book margin profiles within each option liquidation
  sweep. A seed with a growing chain exposed repeated profile recomputation
  per held contract; the same two-hour run completed after the cache.

## Pilot Design

- Three independent, prefunded venues (`north`, `central`, `south`) on one
  deterministic clock.
- Local spot, perpetual, dated futures, and six-hour/two-day European option
  tenors. There is no cross-venue collateral transfer or atomic arbitrage.
- One-second simulation step, one-minute Greek sampling, fixed IV = 80%,
  short-dated option flow biased 65% to buy, and a Stoikov-style spot/perp
  maker. The option dealer itself remains a fixed-spread inventory-skew
  baseline, not an Avellaneda-Stoikov option model.
- Two four-hour paired seed runs (42 and 43). The hedge treatment changes
  endogenous fills, so venue paths need not match after the first hedge.

## Results

Mean absolute executed net delta, in ABC units:

| Seed | Venue | Hedge on | Hedge off | Reading |
| --- | --- | ---: | ---: | --- |
| 42 | north | 0.110 | 0.187 | hedge lower |
| 42 | central | 0.004 | 0.156 | hedge lower |
| 42 | south | 0.127 | 0.126 | effectively tied/slightly worse |
| 43 | north | 0.004 | 0.833 | hedge lower |
| 43 | central | 0.333 | 0.220 | hedge worse |
| 43 | south | 0.374 | 0.366 | effectively tied/slightly worse |

Gamma and vega remain order-of-magnitude similar across the normal hedge
arms, as expected from a delta-only hedge. A seed-43 central hedge-on run
showed near-zero gamma/vega after its position inventory changed sharply;
without exchange-owned terminal equity and liquidation telemetry, that is a
diagnostic event, not evidence that delta hedging removed curvature risk.

## Conclusion

The pilot validates that the hedge switch is real and can materially reduce
executed net delta. It does **not** establish a universal stability or PnL
claim: two of six venue-seed cells are non-improving, paths are endogenous,
and current Greek profiles are pre-hedge-fill periodic observations. Static
IV also makes vega a local sensitivity, not realized volatility PnL.

## Required Before A Strong Claim

1. Exchange-owned pre-expiry, post-settlement, and terminal-flat snapshots.
2. Marked equity including wallets, borrow debt, option/future marks, hedge
   fees, interest, residual inventory, and liquidation reason.
3. At least 20 independently seeded worlds; three venues are replication
   within a world, not 60 independent observations.
4. Dynamic IV/surface shocks and an explicit forward source before interpreting
   vega or a Black-76 spot proxy economically.
5. A genuine option market maker with reservation price, fill intensity,
   inventory horizon, and gamma/vega penalties before calling it Stoikov.

Configurations are
[hedge on](/home/vlad/development/exchange_simulation/research/multivenue-options-hedge-on-2026-08-15.json)
and [hedge off](/home/vlad/development/exchange_simulation/research/multivenue-options-hedge-off-2026-08-15.json).
