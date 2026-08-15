# Market Objective Audit, 2026-08-15

## Objective

The target is not a backtest of a presumed model. It is a reproducible market
laboratory that can falsify or support conditional effects as complexity grows:
multiple venues, spot margin, dated and perpetual futures, options with short
and long expiries, participant latency, and actual fill accounting.

## Requirement Matrix

| Requirement | Current evidence | Status |
| --- | --- | --- |
| Reproducible simulation state | Direct and scheduler-backed delayed phase runs have fixed ordering. The dense three-venue `greeks.json` is byte-identical at one and fourteen OS threads. | Supported for configured phase-mode paths. |
| Many-participant repeated TWAP | 32-participant one-venue study, 20 staggered 2-ABC parents per world, 20 paired seeds. TWAP target IS 7.000 bp vs immediate 9.356 bp, all complete. | Supported, conditional on fast replenishment and constant latency. |
| Three venues with spot, perp, dated futures, options, and spot margin | Dense six-hour scenario has all listed classes, local auto-borrow, short expiry settlement, long live tenor, 39 actors/world, and exchange-owned risk telemetry. | Implemented and measured. |
| Hedged dealer stability | Ten complete dense pairs: mean absolute delta falls by 0.191 ABC; one retained hedge-on terminal-liquidity failure in predeclared seed range. | Supported only conditionally on a valid terminal two-sided conversion mark. |
| Gamma and vega interpretation | Exchange-owned positive-horizon Black-76 sensitivities are logged. Spot hedging changes subsequent option inventory, and IV is static. | Measurement works; gamma/vega are not hedge-neutrality or realised-PnL claims. |
| Low-latency winner | A deterministic scarce conversion proves the lower-latency actor locks the exact two-leg cashflow; label swaps and actor-order reversal falsify client-ID/order explanations. | Arrival mechanism supported. |
| Low-latency ecology profitability | `FeeAwareBasisArb` still decides from midpoint basis and lacks a complete per-leg cash/fee/terminal-PnL report. | Not supported; do not claim. |
| Calibrated Stoikov optimality | Linear makers use an AVS-inspired inventory controller but no empirical arrival-intensity calibration. | Not supported; control policy only. |
| Cross-venue arbitrage/equilibrium | Venues are deliberately independently funded with no route/transfer/atomic-leg actor. | Not implemented. |

## Why The Gaps Matter

The low-latency mechanism is deliberately simple because it proves causality:
an earlier observer consumes a scarce, fully accounted two-leg conversion. A
general basis arb is not yet eligible for the same claim. It estimates an edge
from midpoints, sends independent legs, and cannot distinguish a favorable
fill from directional residual, financing, or a fee omission. Running it
longer would create a more precise answer to an undefined quantity.

Similarly, the dense option result shows that a hedge can lower linear
exposure, not that it creates a preferred risk-adjusted strategy. The retained
seed-49 failure is important: finite liquidity makes a strict terminal
valuation unavailable. Selecting only worlds that remain liquid would turn a
model limitation into apparent stability.

## Next Admissible Experiments

1. Replace basis-arb midpoint admission with executable depth-walk prices,
   fees, funding/delivery policy, and a fill-driven two-leg `ExecutionGroup`.
   Report every fill, fee asset, residual delta, terminal marked equity, and
   completion state.
2. Give each latency tier an independent deterministic request/response/market
   data stream and run fixed client-ID label swaps in the same many-agent
   ecology. Compare tier-level completed conversions, residuals, and marked
   PnL, not submitted intent count.
3. Decide the finite-inventory rule for cross and option makers. An explicit
   hedge must itself face latency, fills, rejection, and cost; infinite
   replenishment must never be smuggled in as a convenience assumption.
4. Add maturity-matched forwards and a dynamic IV surface before studying
   realised delta/gamma/vega/theta PnL or volatility-trading equilibrium.

Until those gates pass, use the current outputs as controlled mechanisms and
failure cases, not as a statement that the simulator has reached a market
equilibrium or that one trading strategy is universally profitable.
