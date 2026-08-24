# V2 economic-ecology design ledger

## Status and rules

This document begins after the frozen autopsy
[`frozen-autopsy-ae13f9a.md`](frozen-autopsy-ae13f9a.md). It is a design
ledger, not a parameter sheet. No V2 simulation result may be compared with
ae13f9a until the new simulator commit is explicitly frozen and its controls
are regenerated.

The initial V2 target is not “make charts look realistic.” It is a local,
causally identifiable ecology in which price discovery, inventory management,
carry, option supply/demand, and distress occur through participant actions
that can be removed and independently measured.

For every mechanism below, promotion requires: unit/invariant tests;
fresh-process deterministic smoke reproduction; a predeclared targeted control
and treatment; measurement of intervention activation; and a kill criterion.
Any modification to scheduling, RNG use, matching, actor decisions, latency,
or economic state will start a new freeze rather than silently extend ae13f9a.

## Design principles carried forward

- A public venue feed is legitimate information. A shared instantaneous
  simulator object available upstream of all participants is not a participant
  information model.
- A quote, hedge, rebalance, arbitrage leg, and liquidation are distinct
  actions with distinct cost, queue, timing, and fill risk.
- A market-level fact is not endogenous if most participating beliefs directly
  encode it. It may still be useful, but it must be labelled as inherited.
- A survivor count or an active book is not an economic objective. Volume must
  increasingly follow liabilities, valuation, execution, or risk constraints.
- A detector must predict the broken behavior independently. Every new V2
  evidence stream needs at least one adversarial mutation before it supports a
  scientific claim.

## V2-0 — evidence prerequisites

| observed ae13f9a limit | local change | predicted observable | causal/mutation test | kill criterion | risk |
|---|---|---|---|---|---|
| Actor inbox time is absent; message delivery is only aggregate telemetry | Persist compact participant observation receipts: source venue, symbol, publication time, scheduled and delivered time, feed identity, and monotone per-link ordinal | Every decision can be joined to a non-future local observation | Delay/drop/reorder one feed record; independently replay `receipt <= decision` and FIFO | Missing receipt or an observation after decision cannot be detected | High-volume evidence growth; use compact/binary sidecar with a documented schema |
| Option fills have no per-fill position transition | Emit option position transition keyed to participant fill ID | Option fill/position one-to-one audit covers all derivative types | Drop or double one option settlement side effect | Any option path cannot be reconciled from evidence | Logger overhead; make it an append-only compact stream |
| GTC cancel requests and state transitions are unobservable | Persist request receipt, cancel acknowledgement/reject, and terminal book removal reason | A requested cancel can be replayed to its state consequence | Drop one request or acknowledgement | A stale executable GTC order cannot be detected | Request evidence must not feed actor behavior |
| Cross-margin reconstruction excludes heterogeneous portfolios | Persist an ordered compact risk-input snapshot per liquidation evaluation | Independent mark/collateral/borrow replay can include full portfolios | Substitute a stale collateral/option mark | The detector must exclude a row rather than assume ordering | Sidecar must not change risk-evaluation timing |

V2-0 is instrumentation, but it is still a semantic version boundary if events
or scheduling change. It must be shown observational before a V2 freeze.

## V2-1 — participant-local cross-venue information

| field | design |
|---|---|
| Observed failure | The ae13f9a own-mid ablation shows a shared, instantaneous maker index is the dominant convergence channel. IB-2 also makes same-tick composition order an unstated model input. |
| Local mechanism | Each maker subscribes to an explicit, heterogeneous set of public venue feeds. A private deterministic courier applies its feed-specific delay/drop/staleness process. The maker owns a local book cache and computes its own weighted microprice/composite only from delivered observations. Quote submission has an independently configured request delay. |
| Non-negotiables | No participant reads a global composite. No cache may advance from an unpublished or undelivered book event. All source venue weights, feed links, cache age, and local reference terms are logged in V2-0 evidence. |
| Expected observable | Cross-venue prices synchronize through quote revisions with nonzero, heterogeneous delay; the relationship weakens as feeds become stale or sources are removed, not only when all makers are forced to local mid. |
| Causal design | 2×2 factorial: informed makers on/off × explicit executable cross-venue arbitrage on/off. Hold population, fees, and latency distributions fixed. Measure dispersion, lead/lag, quote-revision response, actual arb legs, and price-discovery share. |
| Kill criterion | If an “informed” maker receives an upstream global value, has no receipt evidence, or the on/off intervention does not change its cache/reference path, the experiment is NOT IDENTIFIED. If convergence survives equally with both channels off, find the unmeasured synchronization path before interpretation. |
| Main risk | A uniform common feed merely recreates the index under another name. Heterogeneity in venues, weights, latency, confidence, and horizons is part of the mechanism, not decoration. |

Implementation sequence: first add an isolated read-only cross-venue public
feed subscriber with one deterministic smoke world; then one maker’s local
cache/composite; then a heterogeneous roster; only then delete the old shared
index from the V2 scenario. Preserve an ae13 compatibility mode only for
historical comparison, never as a hidden V2 fallback.

Progress ledger: the local delayed cache prerequisite is
[V2-1a](v2-1-single-feed-cache.md). The required
[frontier-vector artifact](v2-1-frontier-vectors.md) now catches future,
dropped-component, and dropped-decision mutations. Its first live remote,
one-maker proof is [V2-1c](v2-1-remote-feed-smoke.md). It has no economic
claim. The predeclared three-policy roster in
[V2-1d](v2-1d-roster-preregistration.md) now passes those distinct
information-set, activation, and fresh-process evidence tests. It has no
price-discovery claim. The router's corresponding V2-2a evidence/activation
gate is recorded in
[V2-2a](v2-2-router-evidence-preregistration.md): it establishes auditable
three-venue frontiers only, not a price-discovery effect. The next gate is an
economically motivated router activation smoke followed by the factorial, not
a larger maker parameter sweep.

That fixed five-minute V2-2b smoke is now complete in
[its result record](v2-2b-price-discovery-smoke-results.md). Delayed remote
maker feeds have a two-seed screening reduction in fresh midpoint dispersion
and after-fee scanner edges; the small non-atomic router executes only when
those feeds are off and does not reduce the registered snapshot-level edge
metric. Therefore trade-mediated convergence remains unmeasured, not absent:
the next router experiment must preregister meaningful capacity/action-clock
identification rather than tune this smoke result.

## V2-2 — executable cross-venue arbitrage

| field | design |
|---|---|
| Observed failure | ae13f9a has no enabled cross-venue arbitrage population; quote-mediated synchronization masks the distinction. |
| Local mechanism | A capital-constrained router observes delayed local feeds, evaluates executable buy ask/sell bid depth including fees, expected slippage, request latency, leg risk, margin/funding, inventory, and opportunity cost, then sends two independent non-atomic orders from prefunded venue accounts. |
| Expected observable | Dislocations close through actual successful and partial legs; orphan inventory, failed legs, fees, and risk limits affect whether a router trades. |
| Causal design | The V2-1 2×2 factorial. Attribute quote-mediated convergence to revision events and trade-mediated convergence to accepted/fill-qualified router legs, not to an omniscient post-hoc cycle scan. |
| Kill criterion | A router that submits forced equal-size “price repair” orders, ignores depth/fees/latency, or cannot carry partial-leg inventory is not explicit arbitrage. |
| Main risk | A fast router can become a privileged oracle. Its feed cache must be governed by the same receipt contract as every other participant. |

## V2-3 — passive making and inventory control

| field | design |
|---|---|
| Observed failure | The frozen CDF/USD runaway is consistent with inventory-skew feedback and insufficient elastic opposing demand. Maker requotes can become aggressive inventory trades. |
| Local mechanism | Passive refreshes are post-only. Inventory reduction is a separate rebalance policy with an explicit order type, taker fee/slippage, participation cap, cooldown, inventory target, and stop/risk limit. Inventory changes both price and size: a long maker reduces bid size/aggression and increases ask size/aggression; a short maker does the reverse. |
| Expected observable | Passive quotes do not cross. Aggressive inventory turnover is visible, costly, rate-limited, and correlated with inventory excursions rather than with every refresh. Extreme inventory reduces the maker’s marginal appetite before it moves the book. |
| Causal design | Hold price-elastic demand fixed and compare: (a) legacy crossing requotes, (b) post-only with price skew only, (c) post-only with asymmetric size, (d) explicit capped rebalance. Measure self-taking, inventory autocorrelation, signed-flow response, depth, and tail/runaway frequency. |
| Kill criterion | If stability appears only because an external force pins price, or rebalances are free/unbounded/instantaneous, the causal mechanism has not been implemented. |
| Main risk | Overly conservative size rules can kill trade. Require activity, fill, and quote-presence thresholds separately from stability. |

The roster must include distinct maker families rather than Stoikov constants:
cross-venue composite, local microprice/imbalance, queue-sensitive,
flow/toxicity-sensitive, volatility-aware, and inventory-constrained. Each
family needs a stated information set and objective.

Progress ledger: the P0 A/B/C passive-refresh screen is completed in
[`v2-3-passive-making-p0-results.md`](v2-3-passive-making-p0-results.md).
The subsequent P1 size-only screen is completed in
[`v2-3-inventory-size-p1-results.md`](v2-3-inventory-size-p1-results.md):
the declared long/short displayed-size response is active in both paired seeds
without a short-horizon viability collapse. It is not a stabilization claim.
The next mechanism must be a separately preregistered explicit, costly,
rate-limited inventory rebalance; no P1 coefficient or population tuning is
licensed by that result.

The P2 contract is fixed in
[`v2-3-inventory-rebalance-p2-preregistration.md`](v2-3-inventory-rebalance-p2-preregistration.md).
It deliberately scopes the first action to unhedged CDF/USD makers and scores
individual risk transfer, not a market-stability target or aggregate exposure.
The final post-schema-audit A/B × seed-101/103 P2 campaign is now recorded in
[`v2-3-inventory-rebalance-p2-results.md`](v2-3-inventory-rebalance-p2-results.md):
it supports only that mechanism-integrity claim. Its result licenses no P2
coefficient, clock, spread, demand, or population tuning.

## V2-4 — economically motivated demand and ecology

| field | design |
|---|---|
| Observed failure | `noise_flow`, `option_flow`, `future_flow`, and latent liquidity primarily keep books active. The frozen ecology fails wealth/concentration realism. |
| Local mechanism | Replace or demote activity generators with agents that have a liability, target inventory, valuation, execution schedule, production/consumption exposure, treasury/rebalance mandate, or leveraged directional belief. Preserve some momentum/trend capital as a destabilizer rather than a stabilizer. |
| Expected observable | Trade intensity, side choice, and size vary with an agent’s state/cost rather than an unconditional random clock. Capital, drawdown, and extinction emerge from realized objectives. |
| Causal design | Introduce one motive family at a time against an otherwise fixed roster. Remove it after activation and test its predicted flow, inventory, and market contribution. Report wealth shares and turnover by class. |
| Kill criterion | “Agent keeps book alive” or a fixed net directional flow without an economic state is not an objective. A class whose state never changes is inactive, not validated. |
| Main risk | Replacing all generators at once can make every book empty. Stage by asset and retain explicit viability gates. |

The first V2-4 slice is
[`V2-4 L0`](v2-4-liability-hedger-l0-preregistration.md): it first establishes
that a finite-capital, stateful delivery-liability hedger is observable and
locally executable before using it to replace any activity generator. L0's
A/B control retains the same liability state path and changes only the actor's
submit permission; it has no registered price-stability or replacement claim.
The completed A/B × seed-101/103 activation screen is recorded in
[`v2-4-liability-hedger-l0-results.md`](v2-4-liability-hedger-l0-results.md)
and `artifacts/v2-4-l0/l0-summary.json`: it is **SUPPORTED (screening)** for
the narrow local hedge-gap mechanism. It licenses a separately preregistered
L1 replacement screen only; it does not license parameter tuning or a claim
that the actor stabilizes CDF/USD or supplies realistic aggregate demand.

The registered matched L1 side-policy screen is now complete in
[`v2-4-l1-cdf-motive-control-results.md`](v2-4-l1-cdf-motive-control-results.md)
and `artifacts/v2-4-l1/l1-summary.json`. With all six broad legacy
`noise_flow` actors deliberately retained, the delivery-liability policy
reduced its own exact time-averaged gap in both paired seeds relative to an
otherwise matched random-side policy; every exercised delivery fill reduced
the replayed gap, while the control had many nonreducing fills. This is
**SUPPORTED (screening)** for the local motive distinction only. The CDF/USD
non-collapse floor also passed in all cells, but it is explicitly not an
ecology, price-stability, or replacement result. The next V2-4 gate is a
preregistered phase/offset test (L1-P) before any CDF allocation or legacy
roster-demotion claim; it must not tune the L1 policies or parent population.

## V2-5 — identifiable perp funding and dated carry

| field | design |
|---|---|
| Observed failure | Funding-off leaves ae13f9a pooled perp basis bit-identical. Dated futures do not converge into expiry. |
| Local mechanism | Basis traders explicitly estimate expected funding/carry net of borrow, fees, balance-sheet use, latency, margin/liquidation risk, and non-atomic leg risk. Perp participants also receive independent demand, so spot/perp need not be held together solely by makers. Dated-carry traders use time-to-expiry and settlement terms, not a fixed quote-clock artifact. |
| Expected observable | Premium affects expected funding, expected funding changes carry attractiveness and inventory, and inventory produces spot/perp orders. Dated basis changes with time to expiry under active terms. |
| Causal design | After V2-1 has separated price discovery, compare funding on/off at several feasible basis-trader capital levels. Score premium width, half-life, funding-response event study, trader positions, and dated convergence separately. |
| Kill criterion | No observed basis-trader inventory/order response to a changed expected funding rate means funding is not identified; do not infer an anchoring verdict. |
| Main risk | Directly setting price equal to funding-implied fair value would impose the result and is forbidden. |

## V2-6 — staged options ecology

| stage | participants and permitted prior | purpose / causal comparison |
|---|---|---|
| O0 | flat/simple dealer, no smile-belief taker | mechanical chain and flat-vol reference |
| O1 | heterogeneous flat-vol dealers and users | test whether dispersion alone creates non-flat prices |
| O2 | end-user liabilities, inventory-aware dealers, delta hedge; no explicit SABR/VV prior | test hedge feedback and surface structure before smile priors |
| O3 | minority SABR-view users | quantify the incremental inherited component |
| O4 | Vanna-Volga/risk-transfer desks with an explicit delayed inventory disclosure contract | test relative-value risk transfer separately |

For every stage, infer IV only from actual market prices, keep parity and
liquidity diagnostics, and compare ATM IV, slope, curvature, term structure,
cross-strike dispersion, dealer gamma sign, and underlying response. A surface
that exists in O3 but not O2 is inherited rather than emergent. A V2 VV desk
must not call the dealer directly unless it is explicitly a disclosed bilateral
risk-transfer facility; otherwise it receives only its allowed feed.

## V2-7 — liquidation ecology

| field | design |
|---|---|
| Observed failure | The registered stress reaches forced closes but not deficits, insurance, or bankruptcy. Frozen population capital and risk states do not make full distress identifiable. |
| Local mechanism | Add economically motivated leveraged participants with limited collateral, persistent exposure motives, and transparent margin choices. Before changing thresholds, map why the current population avoids deficit: leverage, capital, shocks, mark construction, offsets, or liquidation path. |
| Expected observable | Margin calls and partial/forced liquidation occur for stated economic reasons; deficit/insurance paths may occur only when losses exceed collateral. |
| Causal design | Registered V2 stress ladder varying leverage and shock source, not a one-off stronger shock. Reconstruct every event from V2-0 risk-input evidence. |
| Kill criterion | Lowering a liquidation threshold merely to trigger code, without a viable exposure/collateral mechanism, is not an economic stress result. |
| Main risk | A crisis generator can dominate normal ecology. Distress regimes require separate calibration and holdout seeds. |

## V2-8 — timing and performance contract

The ae13f9a cadence screen means timing is an economic input. V2 needs
explicit initial phases/offsets for periodic agents, separately configurable
from interval frequency, so a pure phase test is possible. All clocks must be
listed in manifests: maker refresh, trader decision, router scan, feed/request
latency, funding, listing, expiry, hedge, and liquidation.

Before large sweeps, profile scheduler, matching, actor, option-Greek,
allocation/GC, and logging costs. Candidate semantics-preserving work includes
stable numeric IDs, preallocated pools, compact telemetry, deterministic slice
iteration, and data-oriented event queues. A C++ rewrite is explicitly out of
scope until the V2 mechanisms and measurements stabilize.

## V2 experimental order and stop rules

1. Build V2-0 evidence prerequisites and mutate them.
2. Build one maker-local feed cache and reproduce it deterministically.
3. Implement V2-1 roster and V2-2 router; run the 2×2 price-discovery
   factorial on smoke horizons before scaling.
4. Implement V2-3 passive/rebalance separation and test runaway mechanisms
   without using momentum as a stabilizer.
5. Replace one activity generator at a time under V2-4.
6. Revisit funding/dated carry, then staged options and liquidation ecology.
7. Freeze V2 only after its mechanics and evidence contracts pass.
8. Partition calibration targets from untouched holdout validation. Promote any
   unexpected effect to discovery only after multi-seed replication, nearby
   parameter robustness, reversal/restore ablation, and phase-artifact tests.

Stop a branch when its activation fails, its kill criterion fires, its evidence
is not independently reconstructible, or its apparent improvement disappears
under the corresponding causal intervention.

## Pre-implementation critique

This design deliberately rejects several tempting shortcuts:

- **Replacing the index with a uniform delayed index is not V2-1.** It changes
  a number while retaining common knowledge. Independent feed membership,
  receipt time, and local cache state are necessary conditions, not optional
  realism parameters.
- **A 2×2 price-discovery table cannot identify a channel by itself.** The
  channel labels require event-level attribution: a qualifying quote revision
  must follow an eligible local receipt, and a qualifying trade contribution
  must be a router's accepted/fill-qualified non-atomic leg. If both channels
  are off and prices still converge, the table is evidence of an omitted path,
  not of a residual force.
- **Post-only can hide an activity collapse.** V2-3 therefore has separate
  book-presence, quote, fill, and economic-survival gates. Lower volatility or
  a quieter price path alone is not stabilization.
- **Price-elastic demand can become a scripted price anchor.** Each value or
  liability agent needs a stateful balance-sheet or payoff motive, finite
  capital, and execution/latency cost. A central demand curve that mechanically
  clears against the last price fails the local-mechanism requirement.
- **Funding is not identified by setting a trader's desired inventory.** The
  required observed chain is premium → expected funding → net carry → position
  change → actual orders. Any missing link makes funding inference
  inconclusive.
- **O2 does not prove an endogenous smile merely because it is non-flat.** The
  result must survive a dealer-belief permutation, liquidity/moneyness controls,
  independent market-price IV inversion, and removal/restore of the purported
  cause. Otherwise it is a parametrization artefact or estimator effect.
- **More distress events do not validate liquidation economics.** V2-7 needs
  a reconstruction from pre-evaluation marks, collateral, positions, and
  transfer postings. A shock tuned only to cross a threshold is a code-path
  exercise, not a market result.
- **New telemetry can change trajectories.** V2-0 must be append-only and
  observational: it may neither consume RNG nor schedule simulation events.
  Logging-on/off fresh-process hashes are a release gate before its data may
  support any claim.

The first executable slice is V2-0a: record and audit a compact delivery
receipt for one existing public feed, without changing consumer behavior. It
must pass a deterministic logging-on/off smoke comparison and an injected
future-receipt mutation before expanding to local maker composites. Failure at
that point blocks V2-1; it does not justify implementing an unobservable cache.

## V2 price-reference boundary

The interim V2 price contract is recorded in
[v2-price-lookup-contract.md](v2-price-lookup-contract.md). It separates a
true midpoint used for automatic option listing from the explicitly named
one-sided derivative/index reference policy, and records the residual legacy
`PriceSource` migration as an unresolved follow-up.

The completed repository-wide ledger is
[v2-price-api-audit.md](v2-price-api-audit.md). It records the finished
migration, explicit lifecycle/fee/funding behavior changes, and the remaining
zero-valued fields that are legitimate quantities rather than sentinels.
