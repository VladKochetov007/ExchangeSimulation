# ME-005 — two-venue executable arbitrage readiness

Status: **READY AFTER FOUR SPECIFIC FIXES**. This is a source-grounded design
gate, not a protocol lock, economic run, result, or claim that arbitrage works.
Inspected development HEAD: `04b6ff42342aba1dfdac81268f330dc623cb94d1`
on `research/market-ecology-me002-20260923`. The owner's bounded overnight
continuation authorized ME-005 readiness after the completed ME-002/ME-003
screens. No ME-005 world, seed, lot, horizon, or economic source amendment was
registered or executed by this assessment.

## Question and existing path

The proposed claim is conditional: in one fixed two-venue ABC/USD ecology,
does a finite prefunded router observing a positive *delivered and feasible*
edge complete two non-atomic legs, obtain an economically costed result, and
change the later executable dislocation relative to a router-off world?
Competing explanations include background quote changes, the common price
anchor, stale feeds, FOK venue-arrival failure, local inventory depletion and
the sampling clock. A manufactured positive-edge fixture would test mechanics,
not natural opportunity incidence or profit.

The existing [`CrossVenueArb`](../../../../simulations/multivenue/router.go)
is useful but not a two-venue pilot as configured. It keeps independently
funded venue accounts, receives each venue's delayed local book, tests
top-level quantity and positive spot bid/ask edge after integer quote fees,
then sends separate market FOK buy/sell requests. It records group legs,
rejections, fees and global base residual. Decision-frontier vectors bind
*submitted orders* to complete delivered feed prefixes when instrumentation
is enabled. Existing router tests cover signed-quote exclusion, fee/depth
gates, one-leg failure and a complete three-feed frontier.

The actual production [`Config.normalize`](../../../../simulations/multivenue/sim.go)
requires exactly three venue IDs; [`NewCrossVenueArb`](../../../../simulations/multivenue/router.go)
also requires exactly three endpoints. `addCrossVenueRouters` supplies all
configured venues and hard-codes 1,000 ABC plus 100,000,000 USD **per venue**
for each tier. These are finite but not a prospectively chosen, tunable
two-venue actor budget. The existing three-venue history is not a two-venue
replication.

`ExecutableSignals` counts a positive *locally quoted* price/depth/fee
candidate that passed the router's policy gate. It does not prove either
venue would admit and fully execute both FOK legs when requests arrive, nor
that capital and future exit costs permit the trade. The report's
signal count also excludes otherwise relevant quote updates while a group
is in flight or the attempt cap is exhausted. It is not the denominator
for all public, delivered, or missed opportunities. The older omniscient
arbitrage scan is not a drop-in oracle for this integer lot/fee rule.
The report's
`CompletedCashflow` is actual sell notional minus buy notional and quote fees
for complete groups. It is **not** net economic profit: even equal buy/sell
quantities leave ABC in the buy-venue account and consume ABC in the
sell-venue account. A zero *global* base residual does not restore each
local inventory. Failed one-leg groups retain an explicit global residual.

The current [`MeasureCrossVenueDispersion`](../../../../analysis/crossvenue.go)
is an omniscient, staleness-bounded **periodic midpoint** observer (default
minimum three venues), not an executable bid/ask, actor-delivered, or
event-time post-trade edge estimator. Existing market-data receipts carry
link/timing/fingerprint identities rather than quote payloads; the vector
ledger is emitted for orders, not every no-action evaluation. An independent
opportunity-to-action denominator and route-attributed convergence estimator
have not been demonstrated for this new two-venue claim.

## Required acceptance work before a prospective protocol

1. **Explicit two-venue wiring.** Permit exactly two distinct configured
   venues for this new candidate while retaining the current three-venue
   default and historical configs. Verify two independent accounts, two
   complete delayed feed prefixes, deterministic tie/order behavior and
   no single/duplicate venue. Keep router capital/lot/attempt limits
   explicit and finite rather than relying on the current very large
   fixed endowment. Test both two-venue and unchanged three-venue paths.
   This is a new instrument/population configuration, not a retroactive
   mechanical correction to historical worlds.
2. **Opportunity and execution reconstruction.** Specify exact integer
   units and independently recover public positive bid/ask/depth/fee edge,
   delivered local edge, actor feasible/no-action/attempt state, gateway
   admission, two exchange-time FOK outcomes and actor receipt times.
   Define the sampling clock and opportunity lifetime before outcomes.
   Distinguish a quote-policy signal from balance-feasible and
   venue-arrival-executable quantity; classify missing/stale, zero, negative,
   out-of-domain and censored episodes explicitly. Add small positive,
   zero, negative, rounding, stale, no-action, one-leg rejection and
   dropped/mismatched-evidence fixtures. A valid zero-trade world may be a
   rational result, not an invalid run.
3. **Prospective closeout economics.** Choose and review *one* explicit
   location-specific exit convention before a market world: an executable
   transfer/rebalance or venue-local unwind with its delay, fees, financing,
   price/depth, limits, failures and terminal inventory treatment. Then
   reconstruct by venue and asset from raw events:

   `net_result = sell_proceeds - buy_cost - charged_fees - financing_cost - explicit_closeout_cost`.

   Unclosed residual remains valued/unavailable under a stated terminal
   rule, never silently counted as realized arbitrage profit. Reconcile
   account cash, ABC, borrowing and failed-leg paths independently. A
   no-closeout screen could report completed cashflow only, but it would
   answer a narrower question than the owner's requested net result.
4. **Causal dislocation measurement.** Build an independent two-venue
   event-time executable-edge estimator and predeclare how route attempts
   are linked to subsequent quote/trade changes. Keep midpoint dispersion
   descriptive and separate. Use paired router-off/on worlds, retain
   inactive/failed/unpriceable cells, and test same-timestamp order, venue
   swap, receipt lag, no-op routing and sampled-snapshot aliasing. A router
   can complete trades with no measured convergence; a common anchor or
   background actors can close an edge without router causation.

All four are bounded acceptance targets, not an authorization to implement
or run them now. In particular, the closeout convention is an economic design
choice, and the current three-venue harness cannot be silently renamed as a
two-venue experiment. No new economic world should launch until source,
effective configuration, evidence reconstruction, static fixtures, finite
resources and a prospective protocol pass independent review.

## Possible later screen, not locked

After these gates, the smallest owner-proposed screen is router OFF/ON at
one fixed lot, fee, latency and background ecology, paired over no more than
three development seeds. The older idea card's four-world/two-seed example
is a budget suggestion, not a registered matrix. Choose seeds, horizon and
resource limits prospectively from observed clocks and opportunity lifetimes;
do not tune the ecology after observing whether edges appear. Estimate a
route funnel and event-time edge-duration contrast with seed/world as the
uncertainty unit. A valid no-opportunity or loss result is reportable.

An independent [Sol-6 medium source-readiness review](../../reviews/me005-readiness-20260924.md)
agreed that the existing router could support a narrower two-venue execution/
cashflow screen after wiring and evidence fixtures, but that realizable net
profit needs a prospectively chosen transfer/unwind/financing convention.

Historical [`V2-2b`](../../../v2-2b-price-discovery-smoke-results.md)
found completed router groups but no change in its registered sampled edge
endpoint, while informed maker quotes reduced sampled dispersion. The
[`three-venue latency race`](../../../multivenue-crossvenue-latency-race-2026-08-15.md)
reported completed quote cashflow and non-atomic failures without transfer
cost. Both remain attached to their original source, population and
measurement contracts; neither supplies the missing ME-005 two-venue net
result. ME-006 and ME-007 remain planning ideas until this path is credible.
