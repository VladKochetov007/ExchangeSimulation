# Candidate policy catalogue

Baseline inspected: `32d340f188460712d94b43f76f1de1928b013bf0`.
This is a finite inventory of candidates, not profitability or execution authorization.
IMPLEMENTED means a matching source path exists, not current causal/empirical validation.
PARTIAL means related code omits or has not established part of the requested family.
UNVERIFIED means bounded inspection cannot establish implementation; it is not proof of absence.
Deployment is a separate assignment for every policy. No fast/slow strategy duplicates.
Future acceptance fixtures below are proposals, not newly performed tests.
Historical links retain their source/config/analysis scope; none is a run of merged main.

<a id="s01"></a>
## S01 — Immediate aggressive finite-target execution

- Purpose/counterparties: Complete a finite target at low all-in shortfall; passive quoters are counterparties.
- Source status: **IMPLEMENTED**. [simulations/executionlab/execution.go](../../simulations/executionlab/execution.go)
- Resources/information: Fixed finite balances, TargetQty and delayed public snapshot; one market child.
- Minimal test: Full, partial and rejected child arithmetic.
- Conditional hypothesis: Larger target consumes more available depth.
- Primary outcome: Target shortfall and completion.
- Confound: Terminal residual marking is horizon-dependent.
- Prerequisite: [ME-000](ideas/ME-000/idea.md). HISTORY: [original report](../../research/executionlab-2026-08-15.md).

<a id="s02"></a>
## S02 — TWAP child-order execution

- Purpose/counterparties: Spread a finite target across time; makers and other flow supply fills.
- Source status: **IMPLEMENTED**. [simulations/executionlab/execution.go](../../simulations/executionlab/execution.go)
- Resources/information: Same actor resources as Immediate, configured child count/interval and delivered quotes.
- Minimal test: Remainder division and last-child drain.
- Conditional hypothesis: Slicing can trade timing exposure against immediate depth cost.
- Primary outcome: Target shortfall plus completion.
- Confound: A common seed does not preserve endogenous books after policy changes.
- Prerequisite: [ME-001](ideas/ME-001/idea.md). HISTORY: [original report](../../research/executionlab-2026-08-15.md).

<a id="s03"></a>
## S03 — Participation-of-volume execution

- Purpose/counterparties: Execute a finite mandate at a fraction of observed volume; background flow defines opportunities.
- Source status: **UNVERIFIED**. No exact implementation identified; no source change proposed here.
- Resources/information: Would need finite target, delivered-volume window and cap; no matching implementation located in bounded inspection.
- Minimal test: Volume-window/child cap and no-volume abstention.
- Conditional hypothesis: Participation may reduce footprint when counterpart flow is abundant.
- Primary outcome: Completion and cost versus delivered participation.
- Confound: Own trades in volume denominator can create feedback.
- Prerequisite: [ME-001](ideas/ME-001/idea.md). HISTORY: no matched historical report established.

<a id="s04"></a>
## S04 — Passive deadline execution

- Purpose/counterparties: Rest to satisfy an exit/deadline; aggressive users are counterparties.
- Source status: **PARTIAL**. [simulations/multivenue/term_carry.go](../../simulations/multivenue/term_carry.go)
- Resources/information: Passive term-carry exit exists; generic finite parent/deadline policy is not established.
- Minimal test: Rest/cancel/deadline race and residual valuation.
- Conditional hypothesis: Queue priority can lower paid spread but reduce completion.
- Primary outcome: All-in cost conditional on deadline completion.
- Confound: Term-carry passive exit is not a generic parent executor.
- Prerequisite: [ME-003](ideas/ME-003/idea.md). HISTORY: [original report](../../research/v2-5-p3e-passive-exit-p0-results.md).

<a id="s05"></a>
## S05 — Adaptive passive/aggressive deadline execution

- Purpose/counterparties: Trade execution urgency against fees/adverse selection.
- Source status: **UNVERIFIED**. No exact implementation identified; no source change proposed here.
- Resources/information: Would need finite parent, delayed queue state and explicit urgency rule; not located.
- Minimal test: Deadline switch with partial fills and cancel race.
- Conditional hypothesis: Adaptive urgency may change cost/completion frontier.
- Primary outcome: Target cost and completion.
- Confound: Changing urgency and alpha simultaneously confounds attribution.
- Prerequisite: [ME-003](ideas/ME-003/idea.md). HISTORY: no matched historical report established.

<a id="s06"></a>
## S06 — Symmetric passive market maker

- Purpose/counterparties: Supply spread liquidity to finite takers.
- Source status: **PARTIAL**. [simulations/feesim/mm.go](../../simulations/feesim/mm.go), [simulations/multivenue/naive.go](../../simulations/multivenue/naive.go)
- Resources/information: FixedDistanceMaker has inventory cap; executionlab uses book-responsive levels with zero skew, large finite balances and ordinary limit orders.
- Minimal test: Symmetric quotes, cap suppression and crossing admission.
- Conditional hypothesis: More quoted depth may improve execution capacity.
- Primary outcome: Depth, fills and inventory exposure.
- Confound: Ordinary limits need not remain passive; no dealer optimum is established.
- Prerequisite: [ME-001](ideas/ME-001/idea.md). HISTORY: [original report](../../research/NEXT-STUDY-READINESS-PACKET.md).

<a id="s07"></a>
## S07 — Inventory-sensitive price/size maker

- Purpose/counterparties: Manage inventory risk while quoting to takers.
- Source status: **IMPLEMENTED**. [simulations/multivenue/stoikov.go](../../simulations/multivenue/stoikov.go)
- Resources/information: Local/explicit remote public feeds, quote/inventory inputs; endowment isolation remains a readiness gap.
- Minimal test: Inventory sign, size limits, post-only and cancel-before-replace.
- Conditional hypothesis: Price/size response can redistribute fills and risk.
- Primary outcome: Inventory and quote asymmetry; net objective only after payoff readiness.
- Confound: Programmed skew is not an emergent pricing law.
- Prerequisite: [ME-008](ideas/ME-008/idea.md). HISTORY: [original report](../../research/v2-3-inventory-size-p1-results.md).

<a id="s08"></a>
## S08 — Flow/adverse-selection-aware maker

- Purpose/counterparties: Avoid adverse fills using public imbalance; takers compete for liquidity.
- Source status: **PARTIAL**. [simulations/multivenue/naive.go](../../simulations/multivenue/naive.go)
- Resources/information: ImbalanceMaker lean/suppression built over FixedDistanceMaker; explicit limits and observations.
- Minimal test: Imbalance sign and side suppression at threshold.
- Conditional hypothesis: Public flow information may alter adverse selection.
- Primary outcome: Costed markout and fill share.
- Confound: Imbalance is a signal assumption, not proof of informed trading.
- Prerequisite: [ME-008](ideas/ME-008/idea.md). HISTORY: [original report](../../research/stoikov-control-audit-2026-08-15.md).

<a id="s09"></a>
## S09 — Costly inventory rebalancing

- Purpose/counterparties: Reduce inventory through aggressive costed orders; other makers absorb it.
- Source status: **IMPLEMENTED**. [simulations/multivenue/stoikov.go](../../simulations/multivenue/stoikov.go)
- Resources/information: Separate InventoryRebalanceConfig, local delayed book, interval, limits and fees.
- Minimal test: Disabled decisions, nonreducing action rejection and fee-linked fills.
- Conditional hypothesis: Explicit costly transfer can reduce actor inventory.
- Primary outcome: Inventory reduction and realized execution cost.
- Confound: Actor risk reduction does not prove aggregate stability.
- Prerequisite: [ME-008](ideas/ME-008/idea.md). HISTORY: [original report](../../research/v2-3-inventory-rebalance-p2-results.md).

<a id="s10"></a>
## S10 — Heterogeneous private-value trader

- Purpose/counterparties: Trade relative to an individual believed value; makers and other beliefs are counterparties.
- Source status: **PARTIAL**. [simulations/feesim/value.go](../../simulations/feesim/value.go)
- Resources/information: ValueTrader supports BelievedValue, band, lot and position cap; population heterogeneity requires explicit wiring.
- Minimal test: Band boundaries, inventory cap and fill-derived position.
- Conditional hypothesis: Different beliefs can sustain trade or impose an anchor.
- Primary outcome: Objective surplus and inventory.
- Confound: An imposed private-value prior contributes to observed mean reversion.
- Prerequisite: [ME-009](ideas/ME-009/idea.md). HISTORY: [original report](../../research/no-arbitrage-audit.md).

<a id="s11"></a>
## S11 — Finite-resource momentum/trend follower

- Purpose/counterparties: Take exposure to perceived trend against makers/value traders.
- Source status: **UNVERIFIED**. No exact implementation identified; no source change proposed here.
- Resources/information: No matching dedicated implementation established by bounded source search; would require local signal and finite risk.
- Minimal test: Trend window, sign, cap and closeout.
- Conditional hypothesis: Crowding can amplify or erase trend opportunities.
- Primary outcome: Costed payoff and drawdown.
- Confound: Imposed correlated input can masquerade as endogenous trend.
- Prerequisite: [ME-008](ideas/ME-008/idea.md). HISTORY: no matched historical report established.

<a id="s12"></a>
## S12 — Liability/delivery hedger

- Purpose/counterparties: Reduce an external delivery/exposure gap, potentially paying trading costs.
- Source status: **IMPLEMENTED**. [simulations/multivenue/liability_hedger.go](../../simulations/multivenue/liability_hedger.go), [simulations/multivenue/perp_exposure_hedger.go](../../simulations/multivenue/perp_exposure_hedger.go)
- Resources/information: Finite balances/margin, declared liability process, local books and decision clock.
- Minimal test: Gap-reducing fills, infeasible actions and liability/source accounting.
- Conditional hypothesis: End-user motives can create flow without alpha.
- Primary outcome: Liability-gap/risk reduction and costs.
- Confound: Trading PnL alone omits the liability objective.
- Prerequisite: [ME-009](ideas/ME-009/idea.md). HISTORY: [original report](../../research/v2-4-liability-hedger-l0-results.md).

<a id="s13"></a>
## S13 — Treasury/target-allocation rebalancer

- Purpose/counterparties: Meet portfolio allocation/liquidity mandate.
- Source status: **UNVERIFIED**. No exact implementation identified; no source change proposed here.
- Resources/information: No dedicated portfolio-allocation implementation verified; supplier/hedger roles are not automatically equivalent.
- Minimal test: Allocation target, cash constraints and transfer costs.
- Conditional hypothesis: Rebalancing can supply contrarian flow conditionally.
- Primary outcome: Allocation error and net cost.
- Confound: Target path can encode the claimed mean reversion.
- Prerequisite: [ME-008](ideas/ME-008/idea.md). HISTORY: no matched historical report established.

<a id="s14"></a>
## S14 — Prefunded two-venue executable arbitrage

- Purpose/counterparties: Capture executable venue price differences; venue-local makers are counterparties.
- Source status: **IMPLEMENTED**. [simulations/multivenue/router.go](../../simulations/multivenue/router.go)
- Resources/information: Separate prefunded accounts, delayed feeds, fee/depth checks, non-atomic FOK legs and residual inventory.
- Minimal test: One-leg failure and exact fee/quantity accounting.
- Conditional hypothesis: Routes may reduce persistent tradable dispersion.
- Primary outcome: Completed cashflow and residual exposure.
- Confound: Completed cashflow excludes unrealized local inventory and transfer/closure cost.
- Prerequisite: [ME-005](ideas/ME-005/idea.md). HISTORY: [original report](../../research/v2-2b-price-discovery-smoke-results.md).

<a id="s15"></a>
## S15 — Finite-objective smart order routing

- Purpose/counterparties: Route a mandate for execution quality across venues.
- Source status: **PARTIAL**. [simulations/multivenue/router.go](../../simulations/multivenue/router.go), [simulations/multivenue/metaorder.go](../../simulations/multivenue/metaorder.go)
- Resources/information: CrossVenueArb routes profit-seeking groups; MetaorderTrader executes mandates, but a general cross-venue finite-objective router is not verified.
- Minimal test: Conservation of parent target across concurrent venue children.
- Conditional hypothesis: Routing may improve cost with heterogeneous venue liquidity.
- Primary outcome: Parent completion and all-in shortfall.
- Confound: Arbitrage cashflow is not execution utility.
- Prerequisite: [ME-005](ideas/ME-005/idea.md). HISTORY: [original report](../../research/multivenue-crossvenue-latency-race-2026-08-15.md).

<a id="s16"></a>
## S16 — Cross-venue-informed passive maker

- Purpose/counterparties: Use explicitly delayed remote public information to quote locally.
- Source status: **IMPLEMENTED**. [simulations/multivenue/stoikov.go](../../simulations/multivenue/stoikov.go), [simulation/feed_gateway.go](../../simulation/feed_gateway.go)
- Resources/information: Feed-only session, reference weights/confidence/age and local account; no hidden instantaneous oracle.
- Minimal test: Feed-only emits no orders; decision frontier matches receipts.
- Conditional hypothesis: Remote information can synchronize quotes even without arbitrage fills.
- Primary outcome: Fresh dispersion and maker execution/risk.
- Confound: Information can suppress the router's own opportunities.
- Prerequisite: [ME-006](ideas/ME-006/idea.md). HISTORY: [original report](../../research/v2-2b-price-discovery-smoke-results.md).

<a id="s17"></a>
## S17 — Three-asset triangular conversion

- Purpose/counterparties: Convert through three spot books; finite local counterpart depth.
- Source status: **PARTIAL**. [simulations/feesim/triarb.go](../../simulations/feesim/triarb.go), [simulations/multivenue/naive.go](../../simulations/multivenue/naive.go)
- Resources/information: FeeAwareTriArb sequential state machine and TriangleArbTaker exist; full depth/closing-cost readiness remains unverified.
- Minimal test: Both route directions, charged-asset fees, precision, partial legs and residuals.
- Conditional hypothesis: Executable cycles may shrink when finite conversion trades occur.
- Primary outcome: Net converted amount and residual risk.
- Confound: Midpoint or fee-only signals do not prove realizable profit.
- Prerequisite: [ME-007](ideas/ME-007/idea.md). HISTORY: [original report](../../research/no-arbitrage-audit.md).

<a id="s18"></a>
## S18 — Spot-perpetual carry/reverse carry

- Purpose/counterparties: Earn costed basis/funding subject to exposure and financing.
- Source status: **IMPLEMENTED**. [simulations/multivenue/funding_carry.go](../../simulations/multivenue/funding_carry.go), [simulations/feesim/arb.go](../../simulations/feesim/arb.go)
- Resources/information: Local spot/perp books and funding, declared lot/capital, non-atomic positions.
- Minimal test: Funding sign, borrow/fee cost and unmatched legs.
- Conditional hypothesis: Costed entry can affect basis when matched exposure activates.
- Primary outcome: Net carry and registered basis contrast.
- Confound: Activated desk can still fail the market-level endpoint.
- Prerequisite: [ME-010](ideas/ME-010/idea.md). HISTORY: [original report](../../research/v2-5-p4-funding-carry-results.md).

<a id="s19"></a>
## S19 — Spot-dated-future carry

- Purpose/counterparties: Hold financed spot/future exposure toward expiry.
- Source status: **IMPLEMENTED**. [simulations/multivenue/dated_term_carry.go](../../simulations/multivenue/dated_term_carry.go), [simulations/derivsim/carryarb.go](../../simulations/derivsim/carryarb.go)
- Resources/information: Costed dated allocator, maturity/lifecycle discovery and finite mandate.
- Minimal test: Expiry/settlement, carry costs, ordinary fills and residuals.
- Conditional hypothesis: Eligible costed carry may link dated and spot prices.
- Primary outcome: After-cost carry and term basis.
- Confound: Historical P5 eligibility was absent; code presence is not activation.
- Prerequisite: [ME-010](ideas/ME-010/idea.md). HISTORY: [original report](../../research/v2-5-p5-dated-carry-results.md).

<a id="s20"></a>
## S20 — Calendar-spread relative value

- Purpose/counterparties: Trade near/far relative value; maturity suppliers/hedgers are counterparties.
- Source status: **UNVERIFIED**. [simulations/multivenue/dated_term_carry.go](../../simulations/multivenue/dated_term_carry.go)
- Resources/information: Dated allocation provides related primitives; a two-future spread desk with joint exit is not verified.
- Minimal test: Near/far units, separate maturities, funding and roll residual.
- Conditional hypothesis: Relative-value capital can change term spreads.
- Primary outcome: Costed spread payoff and maturity risk.
- Confound: A calendar spread is not automatically riskless arbitrage.
- Prerequisite: [ME-010](ideas/ME-010/idea.md). HISTORY: [original report](../../research/v2-r5-r2-calendar-amendment-2026-08-30.md).

<a id="s21"></a>
## S21 — Inventory-bearing option dealer with delta hedge

- Purpose/counterparties: Quote options and manage delta against underlying liquidity.
- Source status: **IMPLEMENTED**. [simulations/derivsim/optionmm.go](../../simulations/derivsim/optionmm.go), [simulations/derivsim/hedge.go](../../simulations/derivsim/hedge.go)
- Resources/information: Declared volatility belief, skew, quote inventory, hedge band/policy and strict terminal marks.
- Minimal test: Option fill -> delta -> hedge; expiry and invalid underlying mark.
- Conditional hypothesis: Hedging can transmit option demand into underlying flow.
- Primary outcome: Costed dealer risk and hedge flow.
- Confound: Black-76 spot-mid proxy and programmed hedge are assumptions.
- Prerequisite: [ME-010](ideas/ME-010/idea.md). HISTORY: [original report](../../research/v2-6-p6-options-results.md).

<a id="s22"></a>
## S22 — Costed executable put-call parity

- Purpose/counterparties: Trade synthetic-forward deviations across options and underlying.
- Source status: **PARTIAL**. [simulations/derivsim/parityarb.go](../../simulations/derivsim/parityarb.go)
- Resources/information: Parity actor exists; full financing/fee/depth/leg-close reconstruction requires readiness verification.
- Minimal test: Put/call direction, strike payment, all-leg fees and settlement.
- Conditional hypothesis: Executable parity trading may constrain mispricing when opportunities exist.
- Primary outcome: Completed after-cost parity cashflows plus residuals.
- Confound: Model parity and quoted midpoint relation are not executable profit.
- Prerequisite: [ME-010](ideas/ME-010/idea.md). HISTORY: [original report](../../research/v2-6-p6-options-results.md).

<a id="s23"></a>
## S23 — Heterogeneous option-value takers

- Purpose/counterparties: Trade declared option beliefs against dealer inventory.
- Source status: **IMPLEMENTED**. [simulations/derivsim/valuetaker.go](../../simulations/derivsim/valuetaker.go), [simulations/derivsim/opttaker.go](../../simulations/derivsim/opttaker.go)
- Resources/information: OptionValueTaker volatility model and order limits; flat-vol baseline versus explicitly different smile beliefs.
- Minimal test: Belief-to-order direction and finite budget; no missing-reference action.
- Conditional hypothesis: Belief heterogeneity can change trading and surface outcomes.
- Primary outcome: Costed objective and market-implied surface.
- Confound: A supplied smile prior is not an endogenous smile discovery.
- Prerequisite: [ME-010](ideas/ME-010/idea.md). HISTORY: [original report](../../research/v2-6-p6-options-results.md).

<a id="s24"></a>
## S24 — Gamma/vega and higher-Greek risk transfer

- Purpose/counterparties: Transfer option risk across contracts against option dealers.
- Source status: **PARTIAL**. [simulations/derivsim/vannavolga.go](../../simulations/derivsim/vannavolga.go), [simulations/derivsim/greeks.go](../../simulations/derivsim/greeks.go)
- Resources/information: VannaVolgaHedger has declared book risk and volatility beliefs; general Greek relative-value readiness unverified.
- Minimal test: Greek units, hedge signs, finite limits, missing marks and expiry.
- Conditional hypothesis: Risk-transfer demand can affect surface conditional on exposures.
- Primary outcome: Post-cost risk reduction and residual sensitivities.
- Confound: Vanna–Volga beliefs do not establish endogenous surface structure; historical O4 unexercised.
- Prerequisite: [ME-010](ideas/ME-010/idea.md). HISTORY: [original report](../../research/v2-6-p6-options-results.md).

<a id="s25"></a>
## S25 — Leveraged directional desk

- Purpose/counterparties: Hold finite directional perpetual exposure; ordinary makers absorb orders.
- Source status: **IMPLEMENTED**. [simulations/multivenue/perp_exposure_hedger.go](../../simulations/multivenue/perp_exposure_hedger.go)
- Resources/information: fixed_directional mode, explicit quote/margin and auto-borrow; no physical offset.
- Minimal test: Risk entry, long/short liquidation and no implicit recapitalization.
- Conditional hypothesis: Leverage can create forced flow when margin thresholds activate.
- Primary outcome: Risk decisions and losses/deficits.
- Confound: Physical hedge and speculative position have different economic exposure.
- Prerequisite: [ME-010](ideas/ME-010/idea.md). HISTORY: [original report](../../research/v2-7-p7d-results.md).

<a id="s26"></a>
## S26 — Statistical pairs trader

- Purpose/counterparties: Trade estimated relative misvaluation between assets.
- Source status: **UNVERIFIED**. No exact implementation identified; no source change proposed here.
- Resources/information: No dedicated calibrated pairs policy verified; would need local signals, finite legs and exit.
- Minimal test: Signal-domain, non-atomic legs and residual closeout.
- Conditional hypothesis: Competition may compress a conditional statistical edge.
- Primary outcome: After-cost paired payoff and tail exposure.
- Confound: Estimated correlation does not guarantee stationary spread or arbitrage.
- Prerequisite: [ME-008](ideas/ME-008/idea.md). HISTORY: no matched historical report established.

<a id="s27"></a>
## S27 — Finite distress liquidity absorber

- Purpose/counterparties: Provide bounded inventory-bearing supply/demand to counterpart flows.
- Source status: **PARTIAL**. [simulations/multivenue/supplier.go](../../simulations/multivenue/supplier.go), [simulations/multivenue/cdf_liquidity_supplier.go](../../simulations/multivenue/cdf_liquidity_supplier.go)
- Resources/information: ElasticSupplier variants have finite capital/inventory and local views; distress-specific economics require explicit design.
- Minimal test: Capital/loss exposure, withdrawal, no guaranteed side restoration.
- Conditional hypothesis: Finite willingness can conditionally change available liquidity.
- Primary outcome: Restoration opportunities, concentration and supplier risk.
- Confound: SV1D trading with zero registered restorations is not general supplier failure.
- Prerequisite: [ME-009](ideas/ME-009/idea.md). HISTORY: [original report](../../research/v2-r2-sv1d-iteration-closeout.md).
