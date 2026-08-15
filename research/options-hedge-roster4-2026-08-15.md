# Dense Three-Venue Option-Hedge Study, 2026-08-15

## Hypothesis

Filled spot hedges should reduce a short-option dealer's linear ABC exposure
in a denser, independently funded three-venue ecology. They should not be
assumed to neutralize gamma or vega, because they alter future option fills
and inventory paths.

## Design

- Three direct deterministic venues: each has two Stoikov-inspired spot
  makers, one perpetual maker, one dated-future maker, one option dealer,
  four independently seeded spot/perpetual noise traders, and four
  independently seeded option/futures flow traders. This is 13 actors per
  venue and 39 actors per world.
- Each venue lists spot, perpetuals, dated futures, and five-strike European
  option chains. The short option tenor is five hours and settles within the
  six-hour simulation; the 24-hour tenor remains live. Local spot
  auto-borrow is enabled for actual hedge fills.
- Ten complete on/off seed pairs are summarized from the predeclared
  `42..52` range. One additional hedge-on world is retained as an explicit
  terminal-liquidity failure rather than being imputed or replaced.
- Risk snapshots are exchange-owned. Terminal equity uses wallet debt once,
  futures-style entry-to-mark PnL, and signed option market value. Delta,
  gamma, and vega use the exchange-paired option underlying mark at strictly
  positive time to expiry. IV is fixed at 80%, so vega is a local model
  sensitivity, not realised vol PnL.

Configuration: [multivenue-hedge-expiry-6h-roster4-2026-08-15.json](multivenue-hedge-expiry-6h-roster4-2026-08-15.json).
Complete-world summaries: [JSONL](artifacts/multivenue-hedge-roster4-10seeds-2026-08-15.jsonl) and [paired statistics](artifacts/multivenue-hedge-roster4-10seeds-summary-2026-08-15.json).
Failure record: [JSON](artifacts/multivenue-hedge-roster4-failures-2026-08-15.json).

## Complete-World Results

All values below are per-world averages across the three venues; the seed is
the replication unit.

| Metric | Hedge on | Hedge off | On - off | 95% paired bootstrap interval | Direction |
| --- | ---: | ---: | ---: | ---: | --- |
| Mean absolute net delta, ABC | 0.6441 | 0.8350 | -0.1910 | [-0.2703, -0.1121] | on lower 9/10; sign p=0.0215 |
| Mean peak absolute net delta, ABC | 5.5147 | 5.6454 | -0.1306 | [-0.2112, -0.0498] | on lower 7/10; sign p=0.3438 |
| Mean absolute gamma | 1.0505e-7 | 1.0262e-7 | +2.4286e-9 | [+2.53e-10, +5.35e-9] | on higher 9/10 |
| Mean absolute vega | 2.5203e9 | 2.5192e9 | +1.0829e6 | [+4.61e5, +1.74e6] | on higher 9/10 |
| Mean marked-equity change, USD | +235,250.10 | +235,586.67 | -336.57 | [-454.63, -222.06] | on lower 10/10 |
| Mean terminal maintenance, USD | 344,199.90 | 344,110.09 | +89.81 | [+47.18, +135.22] | on higher 8/10 |

Every complete world has three terminal venue snapshots and at least 4,142
positive-horizon option-position rows. Seed 42 hedge-on produces identical
`greeks.json` at `GOMAXPROCS=1` and `14`
(`sha256=ba5c7a4a73920fbce2d1cea76d755ae52f28b628a61dcf7cc970fee12757232a`).

## Failure And Interpretation

Seed 49 hedge-on fails strict terminal valuation because south has a nonzero
ABC wallet but no two-sided `ABC/USD` conversion mark. Hedge-off completes.
This is finite liquidity/inventory behavior under the dense flow roster. The
world is not assigned zero PnL, not included in the ten complete pairs, and
not replaced by a later seed. Thus the table is a conditional complete-world
result, not an unconditional stability claim.

Within that explicit boundary, spot hedging lowers average linear delta. The
effect on peak delta is directionally favorable but less robust. Gamma and
vega do not become neutral: their changes are endogenous to altered option
inventory, and the flat-IV vega quantity has no realised-volatility
interpretation. The lower marked-equity change is compatible with hedge cost
and path-dependent fills, not a risk-adjusted profitability ranking.

The next required model decision is an explicit finite-inventory policy:
preserve liquidity withdrawal as a scenario, or add an inventory hedge whose
own latency, cost, partial fills, and residual exposure are measured.
