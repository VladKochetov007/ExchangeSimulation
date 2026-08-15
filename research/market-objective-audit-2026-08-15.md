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
| Low-latency ecology profitability | The one-venue fee basis ecology remains inactive under all-in admission. An explicit three-venue FOK router has a fast-tier cashflow advantage in seven jointly completed label-swapped routes, but completion and local residuals do not monotonically improve. | Supported only for the routed one-config mechanism; not a general profitability or equilibrium claim. |
| Calibrated Stoikov optimality | Linear makers use an AVS-inspired inventory controller but no empirical arrival-intensity calibration. | Not supported; control policy only. |
| Cross-venue arbitrage/equilibrium | An opt-in spot router has three independent venue accounts, deterministic per-channel latency, visible-depth all-in admission, FOK legs, and a residual ledger. It has no transfer/rebalance policy. | Execution mechanism implemented; equilibrium not supported. |

## Why The Gaps Matter

The low-latency mechanism is deliberately simple because it proves causality:
an earlier observer consumes a scarce, fully accounted two-leg conversion. The
fee-simulation basis arb uses displayed bid/ask updates, all-in fees, fill
accounting, residuals, and a terminal passive-inventory control, yet its first
label-swapped eight-seed matrix found zero executable signals. The configured
single-venue ecology therefore does not instantiate the mechanism.

The new three-venue router is a distinct model, not a relaxation of that null
result. It uses one prefunded account per venue, deterministic market-data and
request latency, visible full-lot top-of-book admission, and independent FOK
legs. Seven jointly completed seeds show a fast-tier all-in cashflow advantage
under label and thread-count controls, while the retained full ledger contains
non-atomic failures and local residuals. That is conditional execution
evidence, not terminal PnL or an equilibrium.

Similarly, the dense option result shows that a hedge can lower linear
exposure, not that it creates a preferred risk-adjusted strategy. The retained
seed-49 failure is important: finite liquidity makes a strict terminal
valuation unavailable. Selecting only worlds that remain liquid would turn a
model limitation into apparent stability.

## Next Admissible Experiments

1. Add an explicit transfer/rebalance policy to the cross-venue router. It
   must face its own venue withdrawal delay, cost, capacity, rejection, and
   mark-to-market accounting; location-neutral base inventory is not a free
   balance-sheet transfer.
2. Extend the routed study from one full-lot FOK attempt to a predeclared
   multi-attempt design. Compare completion, residual carrying, transfer cost,
   and terminal marked PnL without conditioning away failed routes.
3. Decide the finite-inventory rule for cross and option makers. An explicit
   hedge must itself face latency, fills, rejection, and cost; infinite
   replenishment must never be smuggled in as a convenience assumption.
4. Add maturity-matched forwards and a dynamic IV surface before studying
   realised delta/gamma/vega/theta PnL or volatility-trading equilibrium.

Until those gates pass, use the current outputs as controlled mechanisms and
failure cases, not as a statement that the simulator has reached a market
equilibrium or that one trading strategy is universally profitable.
