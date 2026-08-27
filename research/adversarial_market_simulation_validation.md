# Adversarial Validation Plan for the Multi-Venue Multi-Asset Market Simulator

## Purpose

You have now built enough of the market simulator. **Stop adding new market features for now.**

The next phase is not feature development. It is an adversarial scientific audit whose purpose is to determine whether the simulator is:

1. mechanically correct;
2. economically coherent;
3. free of accidental arbitrage/accounting bugs;
4. genuinely endogenous rather than held alive by hand-designed flow;
5. causally interpretable;
6. capable of producing realistic market-level behavior that was not explicitly hard-coded.

Treat the current simulator as a hostile object whose claims must be falsified.

Your job is **not to defend the current implementation**.

Your job is:

> Try as hard as possible to prove that this simulated market is fake, circular, mechanically inconsistent, over-tuned, or generating its apparent realism for the wrong reasons.

Only claims that survive this process should remain.

Use:

`/home/vlad/development/exchange_simulation/emergent_multi_market_simulation_research.md`

as the methodological guideline.

Also use the existing research ledger and previous FFA/lifecycle experiments, but do not trust conclusions simply because they were previously recorded.

---

# 0. Freeze the model before looking at realism

This is critical.

Create a named frozen baseline configuration from the best current population, probably the latest v7/v8-derived population once any currently running jobs have completed.

From that point onward:

**DO NOT tune the frozen baseline in response to stylized-fact results.**

Do not do things like:

- volatility clustering is too weak → increase volatility-sensitive flow;
- tails are too thin → add larger random takers;
- impact is too weak → change maker depth;
- basis is too wide → increase basis arbitrage;
- IV skew is wrong → alter SABR parameters;
- volume is too low → add more forced flow.

That would turn validation into calibration.

Instead divide future work explicitly into:

### FROZEN VALIDATION BASELINE
Never changed while evaluating holdout facts.

### EXPERIMENTAL ARMS
Changed only to test a pre-registered causal hypothesis.

### FUTURE CALIBRATION VERSION
May eventually be tuned, but only after the current frozen model has been honestly measured.

Record the exact commit, config, seeds, simulator version, and analysis version for the frozen baseline.

---

# 1. First audit the audit tools

Before trusting any market result, attack the analysis layer itself.

This simulator has already produced several false conclusions because:

- an apparent empirical closure was actually an arithmetic identity;
- censoring deleted the largest-impact observations;
- a claimed sign-consistent result included a ratio below one;
- a statistic was mislabeled;
- a supposed validation metric was circular because it read essentially the same quantity as the estimator.

Assume similar bugs may exist in:

- viability;
- lifecycle;
- hedging;
- role attribution;
- arbitrage metrics;
- PnL;
- spread;
- basis;
- impact;
- volatility;
- option-surface metrics.

For every major analysis metric:

1. state the exact mathematical definition;
2. state what raw events it uses;
3. state what observations it excludes;
4. state what missing/censored data means;
5. prove simple identities analytically where possible;
6. build tiny synthetic fixtures where the expected answer is known exactly;
7. deliberately mutate the data and verify the metric changes in the predicted direction;
8. check that the metric is not using the outcome to validate itself;
9. check conditioning/sample-selection effects;
10. check weighting and denominator consistency;
11. check per-event vs per-order vs per-time-window aggregation;
12. check that zero masses do not dominate misleading regression/R² statistics;
13. check whether the same event appears in multiple log streams and could be double-counted.

Use an independent critique subagent specifically against the analysis package.

Do not accept “tests pass” as sufficient evidence. Ask whether the tests are capable of catching the wrong implementation.

---

# 2. Build a completely independent accounting reconstruction

This is one of the highest-priority tasks.

Do not rely only on internal account snapshots.

Replay the full event log independently and reconstruct:

- cash balances;
- spot inventories;
- perpetual positions;
- dated futures positions;
- option positions;
- realized P&L;
- unrealized P&L;
- fees;
- rebates;
- funding;
- settlements;
- exercise/assignment;
- liquidation transfers;
- bankruptcy losses;
- exchange/treasury balances;
- external capital/endowment flows.

For every asset and instrument, derive conservation identities.

For conserved spot asset \(a\):

\[
\sum_i Q_{i,a}+Q_{exchange,a}=Q_a^{initial}+ExternalInflow_a-ExternalOutflow_a
\]

For zero-net-supply derivative contracts:

\[
\sum_i Position_{i,c}=0
\]

For funding:

\[
\sum_i FundingPayment_{i,t}=0
\]

For futures settlement:

\[
\sum_i SettlementPnL_i=0
\]

before fees/external transfers.

For options, verify long/short assignment symmetry and final payoff identities.

Run this audit across an entire >10-cycle simulation, not only unit fixtures.

Report residuals at:

- every funding event;
- every option expiry;
- every dated-future expiry;
- end of every lifecycle window;
- end of the run.

Residuals should ideally be exactly zero under integer/fixed-point accounting.

If rounding is unavoidable, derive the exact theoretical bound and verify the residual never exceeds it.

If any unexplained wealth or inventory appears or disappears, stop and investigate before doing realism research.

---

# 3. Audit lifecycle semantics independently

The fact that listings and settlements occur is not enough.

Verify semantics around boundaries.

For each of at least several sampled contracts:

## Dated future
Check listing timestamp, initial tradability, last trading instant, orders around expiry, settlement reference, settlement price, all long/short payouts, cancellation of resting orders, position closure, delisting, no trading after expiry, and replacement/front-contract lifecycle.

## Option
Check listing, strike/type/maturity metadata, quoting, exercise state, ITM/OTM determination, exact payoff, assignment symmetry, cancellation at expiry, no post-expiry orders, and delisting.

## Perpetual
Check funding observation interval, premium/index/mark calculation, settlement timestamp, payer/receiver sign, overlapping funding times across venues, funding while positions change exactly at the boundary, and event ordering when a fill and funding event share a timestamp.

Explicitly test timestamp collisions:

```text
trade
funding
expiry
liquidation
maker requote
reference-data update
cross-venue hedge
```

at exactly the same simulated timestamp.

Document the deterministic ordering rule and determine whether changing it materially changes economic outcomes.

If it does, that ordering is a real model parameter and must be treated as such.

---

# 4. Search systematically for free-money loops

Construct an explicit graph of tradable transformations between assets/contracts.

Search for executable cycles with positive guaranteed return after fees, spread, tick/lot rounding, funding, borrow/financing, latency, settlement, and margin requirements.

Actually execute candidate cycles through the simulated matching engines.

Build an adversarial **omniscient arbitrage auditor** that is NOT part of normal ecology and exists solely to detect structural inconsistencies.

Search at least:

- spot triangular loops;
- cross-venue same-asset loops;
- put-call parity;
- synthetic forward vs dated future;
- perp vs spot/future carry;
- calendar spreads;
- settlement-boundary loops;
- rounding/tick loops.

Distinguish:

1. transient economic arbitrage that disappears through trading;
2. persistent arbitrage caused by limited arb capital;
3. permanent guaranteed arbitrage caused by simulator inconsistency.

Category 3 is a bug.

Measure opportunity duration and profitability.

---

# 5. Prove that every major agent class actually does what it is supposed to do

Configuration is not evidence.

Generalize the hedging instrumentation to all major classes.

For each role, measure:

- observations received;
- decisions made;
- orders submitted;
- orders accepted/rejected;
- cancels;
- fills;
- passive/aggressive share;
- gross traded volume;
- inventory;
- capital usage;
- PnL decomposition;
- fees;
- funding;
- hedge cost;
- latency cost;
- realized edge;
- survival over time.

## Market makers
Measure spread capture, adverse selection, inventory distribution, quote lifetime, fill rate, quote distance, inventory-to-skew response, hedge activity, and PnL by source. Ensure they are not guaranteed profit.

## Cross-venue arbitrageurs
Measure opportunities seen, attempts, first-leg fills, second-leg fills, orphan legs, completion time, gross edge, net edge, and latency losses.

## Triangular arbitrageurs
Reconstruct actual three-leg cycles.

## Basis/funding arbitrageurs
Measure basis signal, expected funding, capital used, hedge legs, missed fills, funding earned/paid, and realized PnL.

## Dated-future carry traders
Measure convergence-related trades and financing.

## Put-call-parity traders
Measure parity residual before and after action.

## Option value takers
Measure own fair value, market price, disagreement threshold, trade direction, and later outcome.

## Vanna–Volga desks
Do not merely count option fills. Measure vega, vanna, and volga before and after hedge cycles and show whether exposures are actually reduced.

---

# 6. Audit information boundaries and look-ahead

For every actor class, explicitly document its information set \(I_i(t)\).

Search for direct access to:

- current global simulator state;
- future event schedule;
- post-fill book before message delivery;
- true hidden fundamental;
- other agents' positions;
- undisclosed orders;
- market state with zero latency;
- settlement values before they become public.

Instrument:

```text
decision timestamp
information timestamp
message timestamp
exchange timestamp
```

and verify:

\[
information\_timestamp \le decision\_timestamp
\]

for every observation used.

Mutation test: intentionally inject future information into one actor and verify the information audit detects it.

Pay particular attention to cross-venue arbitrageurs, option dealers, basis traders, liquidation logic, and reference/index prices.

---

# 7. Determine whether liveness is endogenous or hand-fed

Audit every “dedicated flow” class.

For each one ask:

> Why does this participant trade?

Classify the motive as:

### Economically legitimate
Hedging an external exposure, portfolio rebalancing, cash-and-carry, inventory liquidation, volatility demand, execution of a metaorder, liability hedging, producer/miner/treasury inventory conversion, or relative-value speculation.

### Pure simulation support
Randomly trading an otherwise dead contract, choosing undertraded contracts specifically to satisfy viability, or otherwise producing flow solely because the benchmark requires activity.

Any class in the second category is suspect.

For every liquidity-demand actor, define its objective/utility, for example:

\[
U=E[W_T]-\lambda Var(W_T)
\]

or an execution/liability objective.

Remove support flows one at a time and measure whether the ecosystem collapses.

If a market only survives because a trader is hard-coded to keep it alive, state that explicitly and propose a more economically grounded source of demand before implementing anything.

---

# 8. Audit population survival and market ecology

Track wealth and activity by strategy class over long runs.

For class \(k\):

\[
w_k(t)=rac{W_k(t)}{\sum_j W_j(t)}
\]

Measure wealth share, PnL, drawdowns, turnover, capital usage, inventory, participation share, bankruptcies, and entries/exits.

Ask:

- Does one class capture almost all wealth?
- Do takers slowly bleed to zero?
- Are makers subsidized by injected capital?
- Does an arbitrage strategy obtain effectively infinite scalable edge?
- Does the system approach a trivial monopoly?
- Does activity depend on resetting or replenishing agents?
- Which classes survive 10, 50, 100 cycles?

Run substantially longer low-log/aggregate-log simulations if feasible.

The goal is to understand the long-run attractor, not merely survive ten cycles.

---

# 9. Freeze a blinded stylized-fact scoreboard

Before measuring the following statistics, write down the list and do not modify the frozen population based on the results.

## Returns
Measure return distribution, tail behavior, skewness, kurtosis, QQ behavior, raw-return ACF, absolute-return ACF, squared-return ACF, volatility clustering, and aggregation across horizons.

## Order flow
Measure buy/sell sign autocorrelation, long memory, order-size distribution, interarrival times, cancellation rates, trade clustering, and volume clustering.

## Limit order book
Measure spread distribution, spread vs volatility, depth distribution, depth profile by distance from mid, imbalance, queue depletion, fill probability, fill time, resilience after aggressive trades, and touch replenishment.

## Market impact
Measure \(E[\Delta p\mid Q]\) across size buckets and horizons. Test concavity, temporary vs persistent impact, reversion, zero-impact mass, conditional impact, and liquidity-regime dependence. Be extremely careful about censoring and mechanical/revision definitions.

## Cross-venue behavior
Measure price differences, correlation, cointegration where applicable, lead-lag, arbitrage opportunity duration, price discovery share, and shock transmission.

## Perpetuals
Measure perp premium, basis distribution, basis autocorrelation, basis half-life, funding vs premium, funding-event response, open interest, liquidations, and spot/perp price discovery.

## Dated futures
Measure basis vs time to expiry, convergence, roll behavior, volume/open-interest migration, calendar structure, and expiry dislocations.

There must be **no explicit price-convergence force**. If convergence exists, identify the trading mechanism causing it.

## Options
Compute market-implied volatility independently from actual quoted/traded market prices, not from agent internal models.

Evaluate IV vs strike, skew/smile, term structure, put-call parity residuals, liquidity vs moneyness/maturity, expiry effects, underlying hedge-flow response, and relationship between dealer Greek exposure and underlying volatility.

---

# 10. Separate endogenous option structure from SABR/Vanna–Volga priors

An observed smile/skew is NOT automatically emergent because the current simulation contains SABR and Vanna–Volga beliefs.

Pre-register:

### O0 — flat/simple beliefs
Remove SABR-shaped strike dependence and VV surface beliefs.

### O1 — heterogeneous flat vols
Agents disagree about volatility level, not smile shape.

### O2 — option demand + dealer inventory + delta hedge
Still no SABR smile target.

### O3 — minority SABR takers
Introduce SABR views to a minority.

### O4 — Vanna–Volga actors
Add VV relative-value behavior.

Compare resulting market-implied surfaces.

Ask:

- Does skew exist under O0/O1/O2?
- Does inventory/hedging alone create strike dependence?
- How much of the final surface is inherited from SABR priors?
- Does VV reduce or amplify deviations?
- What happens if SABR parameters are intentionally wrong?

A strong emergent result would be a nontrivial surface arising even when no participant is directly programmed with that final surface.

---

# 11. Causal ablation suite

Pre-register predictions before running each arm.

## Cross-venue arbitrage OFF
Expected: greater price dispersion, longer arb lifetime, weaker synchronization.

## Basis arbitrage OFF
Expected: wider spot/perp and spot/future basis, slower mean reversion, weaker maturity convergence.

## Funding OFF
Expected: altered perp anchoring and basis-trader behavior.

## Option delta hedging OFF
Expected: weaker option-to-underlying flow transmission and reduced gamma-related feedback.

## Vanna–Volga OFF
Expected: altered cross-strike relative-value dynamics.

## Put-call parity arbitrage OFF
Expected: larger and longer parity violations.

## Cross-venue latency ×10
Expected: longer dispersion, more leg risk, potentially larger gross arb opportunities.

## Maker inventory skew OFF
Expected: inventory accumulation and different quote asymmetry.

## Liquidation OFF
Expected: disappearance of liquidation cascades and different leveraged-tail behavior.

For every intervention report predicted effect, observed effect, effect size, seed robustness, and possible confounds.

Withdraw causal claims that do not survive ablation.

---

# 12. Search for shared upstream causes

Whenever two metrics co-vary, do not automatically infer causality or separate mechanisms.

Search for:

```text
C → A
C → B
```

Examples:

- quote spread and impact both driven by inventory skew;
- option volume and underlying volatility both driven by one taker clock;
- basis and cross-venue dispersion both driven by one shared reference feed;
- liquidity and price discovery both driven by one dominant actor.

Manipulate an independent second lever whenever possible.

---

# 13. Clock-artifact audit

List every important simulator timescale:

- maker requote;
- maker hedge;
- noise-trader tick;
- metaorder interval;
- arbitrage scan;
- latency;
- funding;
- mark/index update;
- liquidation evaluation;
- option repricing;
- option hedge;
- future roll;
- listing;
- expiry.

Sweep response horizons relative to these clocks.

Look for step functions at 1×, 2×, integer multiples, and least common multiples.

If an empirical effect changes discontinuously at an internal timer, report it as a simulator clock effect unless an analogous real-market mechanism exists.

---

# 14. Stress testing

Create pre-registered stress scenarios without changing agent logic:

- large informed metaorder;
- sudden latent-value jump;
- temporary maker withdrawal;
- latency spike on one venue;
- extreme funding;
- collateral shock;
- option expiry with large dealer gamma;
- simultaneous future expiry and funding;
- cross-venue outage;
- major arb desk capital constraint;
- liquidation cascade.

Measure spread, depth, volume, impact, recovery time, basis, liquidation count, bankruptcies, and cross-market propagation.

The objective is coherent response, not dramatic charts.

---

# 15. Mutation testing of the simulator

Create intentionally broken variants:

- reverse funding sign;
- double funding;
- duplicate/delete fill;
- violate price-time priority;
- allow future information;
- make arbitrage legs atomic;
- omit one settlement;
- settle option with wrong strike;
- use stale collateral for liquidation;
- remove one book delta;
- double-count fee;
- drop cancellation;
- give one venue accidental zero latency;
- execute post-expiry order;
- use wrong Black-76 delta sign;
- ignore option multiplier;
- swap call/put payoff;
- fail to cancel expired resting orders.

For each mutation state which invariant or causal/realism test should fail.

If the mutation passes, strengthen the audit suite.

---

# 16. Independent critics

Use multiple critique agents with separate responsibilities:

### Critic A — accounting and conservation
Try to find wealth creation/destruction.

### Critic B — market microstructure
Attack LOB, impact, spread, queue, latency, and order-flow methodology.

### Critic C — derivatives
Attack futures funding, settlement, options, Greeks, parity, and lifecycle.

### Critic D — causal inference/statistics
Search for tautologies, conditioning errors, censoring, shared denominators, circular metrics, multiple testing, and overinterpretation.

### Critic E — economic ecology
Ask whether agents have legitimate motives or merely supply required activity.

Do not tell critics what conclusion you want.

Where critics disagree, reproduce the issue quantitatively.

---

# 17. Statistical discipline

Do not claim significance from three seeds.

Use paired seeds wherever possible.

Before expensive experiments state:

- primary metric;
- expected direction;
- kill criterion;
- sample size or stopping rule.

Do not change the rule after seeing results.

Report individual seeds, mean/median, dispersion, confidence intervals where meaningful, and effect sizes.

Avoid p-value theater on tiny samples.

---

# 18. Robustness outside the tuned configuration

After the frozen baseline is measured, perturb primitive parameters moderately without deliberately improving realism:

- ±20% activity;
- ±20% maker capital;
- latency scale;
- fee scale;
- initial wealth;
- volatility-belief dispersion;
- inventory limits;
- funding intervals;
- tick size.

Ask which emergent facts survive.

Map regimes rather than reporting one knife-edge good configuration.

---

# 19. Compare against real market data only after frozen measurements exist

Choose a concrete empirical reference market if possible, ideally a liquid crypto ecosystem where spot, perp, futures, and options coexist.

Compare normalized/dimensionless quantities where appropriate.

For every empirical fact mark it permanently as:

- CALIBRATION;
- VALIDATION;
- DISCOVERY.

Do not retroactively move facts between categories without recording the change.

---

# 20. Deliverables

Produce:

## A. `research/validation-audit.md`
Frozen baseline definition, commit/config, methodology, discovered bugs, withdrawn claims, surviving claims, unresolved issues.

## B. `research/accounting-audit.md`
Conservation equations and long-run residuals.

## C. `research/economic-ecology-audit.md`
For every actor: motive, utility, activity, capital, PnL, survival, and whether it only exists to keep the market alive.

## D. `research/no-arbitrage-audit.md`
All searched arbitrage cycles and results.

## E. `research/stylized-facts-baseline.md`
Frozen untuned baseline measurements.

## F. `research/causal-ablations.md`
Predictions, experiments, and results.

## G. Machine-readable JSON/CSV summaries
Enough to independently reproduce the tables.

---

# 21. Success criteria

Do NOT call the simulator “realistic” merely because all books remain alive.

The validation phase succeeds only if we can defend statements like:

### Mechanical
> Accounting, matching, funding, margin, settlement, and lifecycle invariants survive long-run replay and adversarial mutation tests.

### Economic
> Major actor classes trade for explicit economic motives and exhibit finite capital, risk, execution cost, and non-guaranteed profitability.

### Cross-market
> Spot/perp/future/options relationships arise through actual trading mechanisms and materially weaken under the corresponding causal ablations.

### Emergent
> Several market-level stylized facts appear in the frozen model despite not being direct calibration targets.

### Causal
> At least some emergent effects disappear under the predicted ablation and return when the mechanism is restored.

### Robust
> Important results survive multiple seeds and moderate primitive-parameter perturbations.

Until those statements are supported, describe the system as:

> a mechanically rich, persistent multi-market artificial ecology under validation

rather than:

> a realistic market simulator.

---

# 22. Working style

Be skeptical.

When a result looks surprisingly good, attack it first.

Whenever you write “this proves X”, ask a critic to construct at least three alternative explanations.

Whenever an experiment confirms the hypothesis, try:

- a null control;
- an independent lever;
- a denominator check;
- a conditioning check;
- a timestamp/clock check;
- a censoring check.

Prefer withdrawing a result over defending a weak one.

Do not optimize for positive findings.

Negative results are valuable.

If the simulator fails realism badly, report exactly how and why rather than tuning it immediately.

The purpose of this phase is to discover what this market **actually is**.

Only after this audit should we decide what economic mechanisms need to be added or redesigned.

---

# Recommended New `/goal`

```text
/goal Freeze the current multi-venue multi-asset market ecology and adversarially validate it as a scientific model. Do not add features or tune the frozen baseline to improve market statistics. Try to falsify the simulator: independently audit accounting and conservation across full lifecycle runs; verify funding, margin, liquidation, option exercise and futures settlement semantics; search for permanent free-arbitrage loops; audit information leakage and latency; prove that every agent class actually performs its intended economic role and has a legitimate objective rather than merely keeping books alive; measure strategy PnL, capital use, survival and ecological concentration; independently validate all analysis metrics against tautologies, censoring, circularity and denominator bugs; measure a blinded holdout suite of microstructure, return, order-flow, cross-venue, perp/futures and option stylized facts without tuning; separate genuinely emergent option structure from SABR/Vanna-Volga priors; run pre-registered causal ablations for arbitrage, funding, hedging, maker inventory, liquidation and latency mechanisms; perform clock-artifact sweeps, stress tests, mutation tests and multi-seed robustness checks. The goal is not to make the simulator look realistic. The goal is to determine rigorously which behaviors are mechanically correct, economically coherent, genuinely endogenous, causally identified and robust, and to explicitly withdraw anything that is not. Follow /home/vlad/development/exchange_simulation/emergent_multi_market_simulation_research.md and this adversarial validation plan.
```

## Recommended order

\[
oxed{
	ext{freeze}
ightarrow
	ext{measure}
ightarrow
	ext{falsify}
ightarrow
	ext{ablate}
ightarrow
	ext{compare to reality}
}
\]

Only after that:

\[
	ext{redesign mechanisms}
ightarrow
	ext{new frozen version}
\]
