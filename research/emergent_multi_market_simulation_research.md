# Designing a Fully Endogenous Multi-Venue, Multi-Asset Market Ecology

## Research findings, design principles, related work, validation protocol, and a roadmap toward genuinely emergent market behavior

### Objective

The target is **not** merely an exchange simulator and not merely an agent-based model that reproduces a few stylized facts.

The target is a unified artificial financial system in which:

- multiple exchanges coexist;
- spot, perpetual futures, dated futures, and options coexist;
- each venue has an explicit limit-order book and matching engine;
- participants have heterogeneous information, objectives, constraints, capital, reaction times, and risk limits;
- market makers, takers, hedgers, basis traders, funding arbitrageurs, cross-venue arbitrageurs, triangular arbitrageurs, option market makers, volatility traders, and liquidation flows interact through actual orders;
- contracts are listed, funded, margined, liquidated, exercised, rolled, and expired;
- prices, spreads, volume, basis, volatility regimes, impact, liquidity crises, and option surfaces are **outputs of the interaction**, not injected target paths;
- the market remains economically alive through repeated funding, listing, expiry, and roll cycles;
- emergent statistics are compared against empirical market effects **after** the mechanisms have been specified.

The scientific goal is therefore:

> Build a market from local rules and heterogeneous economic motives, then ask which real market phenomena arise without explicitly programming those phenomena.

This is materially different from replay backtesting, a stochastic-price simulator, or a single-market toy ABM.

---

# 1. What “fully emergent” should mean

A useful definition is:

> A market property is emergent if it is not directly encoded as a target process or deterministic rule, but appears from the interaction of independently motivated agents under explicit institutional constraints.

For example, these should **not** be hard-coded:

- a GARCH price process to obtain volatility clustering;
- a Student-t return process to obtain fat tails;
- a mean-reverting perp premium to obtain spot/perp convergence;
- a predefined volatility smile;
- a forced correlation between exchanges;
- an impact function that directly moves price after a market order;
- an arbitrary “liquidity regeneration” process that refills the book;
- a rule saying futures converge to spot because maturity is approaching.

Instead, the model should encode mechanisms that could plausibly produce them:

- trend following + inventory-sensitive liquidity provision + heterogeneous reaction times;
- finite balance sheets and endogenous order placement;
- basis arbitrage constrained by capital, execution risk, latency, fees, and funding;
- option demand + market-maker inventory + delta/gamma hedging;
- cross-venue arbitrage;
- contract settlement rules;
- different utility functions and non-speculative trading needs.

## 1.1 Emergence does not mean “nothing may be exogenous”

A completely closed financial system is usually a bad model of a persistent real market.

Some **primitive causes** must be external to the trading game:

- endowments;
- capital inflows/outflows;
- dividends, yields, staking rewards, borrow costs, or financing rates;
- issuance and contract listings;
- external news or private information;
- heterogeneous consumption/hedging needs;
- production or inventory shocks;
- exchange fee schedules and contract rules;
- technological latency;
- risk limits.

The important boundary is:

**External causes may be exogenous; market outcomes should be endogenous.**

A strong version of the simulator can therefore use a latent common-value or information process, but agents should receive heterogeneous noisy observations of it. The market price itself should not be mechanically tied to that process.

---

# 2. Closest existing work

No single public system I found implements the whole target ecology. However, many projects solve important subproblems.

## 2.1 ABIDES

**ABIDES: Agent-Based Interactive Discrete Event Simulation**

- Paper: [Byrd, Hybinette & Balch (2019), *ABIDES: Towards High-Fidelity Market Simulation for AI Research*](https://arxiv.org/abs/1904.12066)
- Current JPMorgan repository: [jpmorganchase/abides-jpmc-public](https://github.com/jpmorganchase/abides-jpmc-public)
- Older public repository: [abides-sim/abides](https://github.com/abides-sim/abides)

ABIDES is probably the most important architectural reference for the event/messaging layer.

Relevant ideas:

- discrete-event simulation;
- tens of thousands of agents;
- explicit pairwise communication latency;
- exchange interaction through messages;
- NASDAQ ITCH/OUCH-inspired protocol structure;
- isolated agent decision logic;
- experimental agents interacting with a simulated background market.

What it does **not** provide out of the box is the desired complete derivatives ecology.

### What to borrow

Borrow the separation:

```text
agent state
    ↓ message
network / latency
    ↓
exchange
    ↓ fill / market data
network / latency
    ↓
agent
```

An agent should never directly mutate the order book or another agent's state.

---

## 2.2 Plham and PAMS

Plham/PAMS is the closest research family to the desired *multi-market artificial ecology*.

- Plham documentation: [plham.github.io](https://plham.github.io/)
- PAMS repository: [masanorihirano/pams](https://github.com/masanorihirano/pams)
- PAMS documentation: [pams.hirano.dev](https://pams.hirano.dev/en/latest/)
- PAMS paper: [Hirano, Takata & Izumi (2023), *PAMS: Platform for Artificial Market Simulations*](https://arxiv.org/abs/2309.10729)

PAMS explicitly supports:

- multiple stocks;
- long simulations;
- index/ETF-like linked instruments;
- options;
- heterogeneous agents;
- high-frequency agents;
- arbitrage agents.

The predecessor Plham has particularly useful options examples.

### Options

See:

- [Plham OptionMain](https://plham.github.io/tutorial/OptionMain)
- [Plham OptionMain strategy examples](https://plham.github.io/tutorial/OptionMain_UseCases02)

The framework includes or demonstrates:

- European option markets;
- expiration and automatic exercise;
- fundamentalist/chartist/noise option agents;
- `DeltaHedgeOptionAgent`;
- `PutCallParityOptionAgent`;
- `SyntheticOptionAgent`;
- `StraddleOptionAgent`;
- `StrangleOptionAgent`;
- leverage/prospect-theory option agents.

This is a major precedent: option prices and volatility surfaces can be studied as outcomes of interacting option and underlying-market participants rather than imposed surfaces.

### Multiple markets and arbitrage

See:

- [Plham MarketShareMain](https://plham.github.io/tutorial/MarketShareMain)
- [Plham ShockTransferMain](https://plham.github.io/tutorial/ShockTransferMain)

The shock-transfer example uses multiple markets, an index market, and arbitrage agents to examine propagation of a shock through linked markets.

### What to borrow

The key conceptual decomposition is extremely good:

```text
Agent
Market
Order
OrderBook
Fundamentals
Events
```

For the present project, extend that idea further by separating **Instrument**, **Venue**, and **Clearing/Risk** rather than allowing “Market” to absorb all three.

---

## 2.3 Spot–futures cross-market ABMs

A particularly relevant paper is:

[Hai-Chuan Xu, Wei Zhang, Xiong Xiong & Wei-Xing Zhou (2014), *An Agent-Based Computational Model for China's Stock Market and Stock Index Futures Market*](https://arxiv.org/abs/1404.1052)

Journal version: [DOI 10.1155/2014/563912](https://doi.org/10.1155/2014/563912)

The model contains:

- several spot stocks;
- an index futures market;
- heterogeneous investors;
- explicit wealth and risk constraints;
- spot–futures arbitrageurs;
- endogenous order submission.

It reproduces, among other things:

- the spot–futures basis distribution;
- bid–ask spread distribution;
- volatility clustering;
- long memory in absolute returns.

This is a strong precedent for using arbitrage agents to **cause** cross-market coupling rather than imposing it statistically.

---

## 2.4 Perpetual futures ABM

[Ramshreyas Rao (2025), *Agent-Based Simulation of a Perpetual Futures Market*](https://arxiv.org/abs/2501.09404)

This work models:

- a perpetual futures market;
- a central limit order book;
- heterogeneous long/short participants;
- positional traders;
- basis traders;
- perp premium dynamics.

The key result is that relatively simple participant behavior can reproduce the basic pegging relationship between a perp and its spot reference.

Its main limitation for our objective is important: the spot process is largely an external driver. For a **fully endogenous multi-market ecology**, spot price discovery itself should come from agents.

### What to borrow

Use the paper as a **subsystem benchmark** for the perp module, not as the architecture of the complete system.

---

## 2.5 Coupled option and underlying markets

Two very relevant artificial-market studies:

- [Kawakubo, Izumi & Yoshimura (2014), *How Does High Frequency Risk Hedge Activity Have an Affect on Underlying Market?*](https://doi.org/10.20965/jaciii.2014.p0558)
- [Kawakubo & Izumi (2016), *Analysis of the Interaction between Option Market and Its Underlying Market by Coupled Artificial Markets*](https://doi.org/10.1527/tjsai.AG-D)

They model:

- an underlying market;
- an option market;
- local agents trading only one market;
- global agents linking both markets;
- delta-hedging activity.

They show that dynamic hedging can increase or decrease underlying volatility depending on conditions.

This is exactly the kind of mechanism the target simulator should be able to rediscover.

There is also strong theoretical/empirical literature on hedging feedback:

- [Platen & Schweizer (1998), *On Feedback Effects from Hedging Derivatives*](https://doi.org/10.1111/1467-9965.00045)
- [Anderegg, Ulmann & Sornette (2022), *The impact of option hedging on the spot market volatility*](https://doi.org/10.1016/j.jimonfin.2022.102627)
- [Aubert, Chevalier & Ly Vath (2025), *Option market making with hedging-induced market impact*](https://arxiv.org/abs/2511.02518)

These works motivate treating option-market making and underlying hedging as a **closed feedback loop**, not two independent simulations.

---

## 2.6 N-ABLE / Sandia

An older but surprisingly broad precedent is:

[Mark A. Ehlen & Andrew J. Scholand (2005), *An Agent Model of Agricultural Commodity Trade: Developing Financial Market Capability within N-ABLE*](https://www.researchgate.net/publication/263504592_An_Agent_Model_of_Agricultural_Commodity_Trade_Developing_Financial_Market_Capability_within_the_NISAC_Agent-Based_Laboratory_for_Economics_N-ABLE)

The work adapted the N-ABLE agent laboratory to commodity markets and explicitly discusses:

- spot trading;
- futures;
- options;
- commodity-market participants.

This demonstrates that multi-derivative agent-based market modeling is not a new idea by itself.

What appears much less common is combining it with modern microstructure-level exchange mechanics, multiple venues, latency, margin, liquidation, crypto-style perpetual funding, and long-lived interacting derivative ecosystems.

---

## 2.7 Market ecology: Evology

Relevant work:

- [Vie et al. (2022), *Towards Evology: a Market Ecology Agent-Based Model of US Equity Mutual Funds*](https://arxiv.org/abs/2210.11344)
- [Vie & Farmer (2023), *Towards Evology II*](https://arxiv.org/abs/2302.01216)
- [Evology workshop paper](https://openreview.net/pdf?id=jycOgqMX0xA)

Evology treats strategies as ecological species competing for capital.

Important lessons:

- strategy profitability depends on population composition;
- returns are density-dependent;
- a strategy can destroy its own edge as its wealth share grows;
- some strategy populations can approach negligible wealth;
- market composition itself is a dynamic state variable.

This is crucial for a simulator expected to stay alive for many lifecycle cycles.

A healthy market should not be defined only by nonzero volume. It should contain a viable ecology of heterogeneous motives.

---

## 2.8 CHAD: phase transitions and validity corridors

A very recent useful reference is:

[CHAD: A Scalable Turn-Based Simulator (2026)](https://openreview.net/pdf?id=JGad8gf6E6)

The paper emphasizes:

- heterogeneous agent ecology;
- empirical BTC/USDT stylized facts;
- a **validity corridor** methodology rather than one magic goodness-of-fit number;
- sensitivity to population composition;
- phase transitions;
- a “fragility pocket” in which agent timescales synchronize with cascade dynamics and generate flash-crash-like behavior.

This matters enormously for design.

A simulator may move from:

```text
stable market
```

to

```text
fragile market
```

because of an apparently innocuous relationship between:

- maker refresh frequency;
- taker intensity;
- liquidation delay;
- arbitrage latency;
- funding timing.

Those are exactly the kinds of effects that a fully emergent simulator should expose rather than hide.

---

## 2.9 Self-organized learned market ecology

A particularly relevant 2026 paper is:

[Hashimoto et al. (2026), *Financial Market as a Self-Organized Ecosystem: Simulation via Learning with Heterogeneous Preferences*](https://arxiv.org/abs/2604.23975)

The authors combine:

- heterogeneous risk aversion;
- heterogeneous discount factors;
- heterogeneous information;
- agents that **learn** rather than receive fixed strategy labels.

They report functional differentiation and role specialization emerging through interaction, along with realistic dynamics including fat tails and volatility clustering.

This suggests a powerful later-stage direction:

> Start with interpretable hand-coded archetypes for debugging and causal attribution, then allow a subset of agents to learn policies under heterogeneous objectives.

Do not start there. Learned agents make causal debugging much harder.

---

## 2.10 QuantReplay

- Repository: [Quod-Financial/quantreplay](https://github.com/Quod-Financial/quantreplay)
- Documentation: [QuantReplay docs](https://quod-financial.github.io/quantreplay/)
- Overview: [QuantReplay](https://quantreplay.com/)

QuantReplay is a strong reference for **exchange infrastructure**:

- multi-asset support;
- order-driven venues;
- configurable matching;
- continuous and auction phases;
- FIX connectivity;
- multiple listings;
- multi-venue deployments;
- equities, FX, derivatives, and digital assets.

It is less interesting as a source of endogenous market ecology.

### What to borrow

Borrow venue realism, configuration boundaries, and market-phase handling.

---

## 2.11 Bourse and independent builders

- Bourse documentation: [zombie-einstein.github.io/bourse](https://zombie-einstein.github.io/bourse/)
- Reddit announcement: [r/quant: Bourse](https://www.reddit.com/r/quant/comments/1bigzmz)

Bourse uses:

- a high-performance Rust limit-order-book core;
- a discrete-event simulator;
- Python bindings for agent/research work.

The architectural lesson is useful: a fast systems-language matching/event core plus a convenient research layer is a practical design.

Another independent exchange simulator discussed on Reddit is qmrExchange:

- [r/quant discussion](https://www.reddit.com/r/quant/comments/xs1y54)
- [earlier thread](https://www.reddit.com/r/quant/comments/xja55l)

These projects confirm that exchange simulation itself is a reasonably common engineering project.

The rare part is the **persistent coupled derivative ecology**.

---

# 3. Novelty assessment

It would be incorrect to claim:

> “This is the first agent-based market with spot, futures, options, and arbitrage.”

There are clear precedents.

A more defensible statement is:

> **To the best of our knowledge, there is no publicly available market simulator that jointly models modern microstructure-level multi-venue spot, perpetual futures, dated futures, and options trading with endogenous liquidity, explicit funding/margin/liquidation/settlement mechanics, heterogeneous cross-market agents, and repeated contract lifecycle events in one persistent market ecology.**

That statement should still be treated as a literature-search claim rather than a mathematical proof of uniqueness.

Proprietary systems may exist at trading firms and exchanges.

---

# 4. The main scientific difficulty

The hardest part is not the matching engine.

It is:

> **causal validity under a complex endogenous ecology.**

A simulator can look realistic for the wrong reason.

Examples of false confidence:

- volatility clustering caused almost entirely by one hard-coded timer;
- book shape caused by one fixed-distance maker;
- futures convergence caused by an explicit mean-reversion term;
- realistic spread caused by a parameter chosen directly from empirical spread;
- an apparent market-impact law caused by measurement definition;
- cross-venue correlation caused by both venues reading the same external reference price.

The core research problem is therefore **identifiability**:

> If the simulation reproduces a real effect, which mechanisms actually caused it?

This is why ablations, interventions, mutation tests, and held-out validation are at least as important as realism metrics.

---

# 5. Architecture for a research-grade simulator

Use strict layers.

## 5.1 Simulation kernel

Responsible only for:

- deterministic logical time;
- event ordering;
- priority queue or equivalent schedule;
- random-number streams;
- network/message delivery;
- reproducibility.

It should know nothing about option pricing or trading strategy.

## 5.2 Venue

Responsible for:

- order acceptance/rejection;
- matching rules;
- queue priority;
- order types;
- tick size;
- lot size;
- fees/rebates;
- market phases;
- halts;
- self-trade prevention;
- market data production.

A venue should not decide the economic value of an asset.

## 5.3 Instrument

Contract semantics:

```text
Spot
Perpetual
DatedFuture
EuropeanOption
```

Instrument state includes only economic terms such as:

- underlying;
- quote asset;
- multiplier;
- strike;
- option type;
- maturity;
- settlement method;
- funding schedule;
- expiry schedule.

## 5.4 Clearing and risk

This must be separate from the matching engine.

Responsible for:

- cash balances;
- positions;
- realized P&L;
- unrealized P&L;
- initial margin;
- maintenance margin;
- cross/isolated margin;
- collateral haircuts;
- liquidation;
- bankruptcy handling;
- funding transfer;
- settlement;
- exercise/assignment.

A fill should generate an accounting event. The trading agent should never “update its own balance”.

## 5.5 Information layer

Every agent should have an explicit information set:

```text
I_i(t)
```

Examples:

- own fills instantly;
- public trades after venue latency;
- L1 after one delay;
- full book after another delay;
- funding estimate;
- index/mark price;
- private signal;
- other-venue book with network latency;
- option surface snapshots;
- liquidation feed;
- open interest.

This prevents accidental omniscience.

## 5.6 Agent policy

The agent receives observations and emits **intent**:

```text
Observation
    ↓
Policy
    ↓
OrderIntent / CancelIntent
```

The agent must not:

- mutate the book;
- mutate its balance;
- know future events;
- see hidden engine state unless the real participant would have it.

## 5.7 Lifecycle engine

Responsible for scheduled institutional events:

- funding;
- listing;
- delisting;
- expiry;
- exercise;
- settlement;
- futures roll;
- option strike listing;
- index updates;
- maintenance windows.

The lifecycle engine changes contract state, not prices directly.

## 5.8 Instrumentation

Every important state transition must be reconstructable.

Record:

- parent decision ID;
- observation timestamp;
- agent ID;
- order ID;
- venue;
- instrument;
- message send/receive timestamps;
- queue position where feasible;
- fills;
- balance changes;
- funding;
- liquidation;
- settlement;
- cancellations;
- market-data events.

A research simulator that cannot explain why two runs diverged is not trustworthy.

---

# 6. Accounting invariants

These should be property tests, not dashboards.

For a conserved token/asset:

\[
\sum_i Q_{i,a} + Q_{\text{venue/treasury},a} = Q^{\text{total}}_a
\]

For cash, account explicitly for every external source and sink:

\[
\Delta \sum_i C_i
=
\text{external inflows}
-
\text{external outflows}
-
\text{fees retained outside agents}
\]

Funding should be zero-sum except for explicitly modeled exchange charges:

\[
\sum_i FundingPayment_i = 0
\]

Derivative settlement should satisfy contract identities.

European call payoff:

\[
C_T = \max(S_T-K,0)
\]

European put payoff:

\[
P_T = \max(K-S_T,0)
\]

A long future and short future must settle symmetrically.

Every accounting invariant should be checked after arbitrary randomized event sequences.

---

# 7. How to create a market that does not die

This is the deepest economic design problem.

A closed population of pure profit maximizers is often unstable as a long-run market ecology.

If trading is approximately zero-sum before fees:

\[
\sum_i PnL_i \approx 0
\]

and negative-sum for traders after fees:

\[
\sum_i PnL_i < 0
\]

then persistently unprofitable liquidity takers eventually lose their capital and disappear.

Real markets continue because participants have **different utility functions** and because capital and risk continually enter the system.

## 7.1 Do not make every agent maximize trading P&L

Add economically motivated agents.

### Hedger

Objective:

\[
\max E[W_T] - \lambda \operatorname{Var}(W_T)
\]

or even primarily:

\[
\min \operatorname{Var}(W_T)
\]

A hedge may have negative expected trading P&L and still be rational.

### Producer / miner / treasury

Receives an external asset endowment and wants to convert some future exposure to cash.

### Consumer / liability hedger

Has a future liability and needs the opposite side of the hedge.

### Option end-user

Buys convexity because its external business exposure has nonlinear risk.

### Index/rebalancing flow

Trades because portfolio weights changed, not because a 10 ms alpha signal appeared.

### Liquidator

Executes because solvency rules require it.

### Funding/basis trader

Earns relative-value return but consumes capital and faces leg risk.

### Market maker

Trades spread capture against inventory, adverse-selection, and hedge costs.

### Speculator

Trades expected return.

These motives create ecological niches.

---

# 8. Open-system capital and endowments

To keep long simulations meaningful, explicitly model capital entry and exit.

Possible mechanisms:

- periodic investor subscriptions/redemptions;
- mining/staking/issuance income;
- producer inventory creation;
- corporate cash flows;
- strategy funds receiving/losing AUM based on performance;
- risk-budget reallocation;
- bankrupt-agent replacement by new entrants.

Do **not** secretly reset losing agents.

If new capital enters, record it as an external flow.

A useful ecology statistic is:

\[
w_k(t)=\frac{\text{wealth of strategy class }k}
{\text{total market wealth}}
\]

Track whether niches survive, dominate, cycle, or go extinct.

Evology is particularly relevant here because it treats strategy returns as density-dependent rather than fixed.

---

# 9. A viability corridor for a “living market”

Do not use only:

```text
volume > 0
```

Define a multidimensional viability corridor.

For example, over most rolling windows:

\[
V_t > V_{\min}
\]

\[
Spread_t < S_{\max}
\]

\[
Depth_t > D_{\min}
\]

\[
N_{\text{active strategy classes}} \ge k
\]

\[
\max_i \frac{W_i}{\sum_j W_j} < c
\]

\[
|Basis_t| < B_{\max}
\]

Also require:

- no permanent crossed markets;
- no persistent free-arbitrage loop;
- bounded leverage distribution;
- finite liquidation rate outside stress regimes;
- no exchange treasury creating money unintentionally;
- no permanent empty option surface;
- new contracts obtain liquidity without scripted fills.

This should be checked across many seeds.

---

# 10. Agent ecology

Start with interpretable archetypes.

## 10.1 Market makers

Use several genuinely different mechanisms.

### Inventory-skew maker

Reservation price:

\[
r_t = m_t - \gamma q_t \sigma_t^2 T
\]

or another inventory-sensitive model.

### Fixed-distance maker

Quotes mechanically at a distance from reference/mid without reservation-price skew.

This is valuable as a causal control because it generates depth through a different mechanism.

### Volatility-adaptive maker

Spread responds to recent realized volatility and/or adverse selection.

### Queue-sensitive maker

Changes quote placement based on fill probability and queue position.

### Cross-venue maker

Quotes on one venue while hedging on another.

Do not let all maker classes share one upstream “fair value” calculation. Otherwise apparently different strategies may be the same mechanism with different parameters.

---

## 10.2 Takers

Avoid one generic random taker population.

Use:

- uninformed liquidity takers;
- informed traders with noisy private signals;
- trend followers;
- mean-reversion traders;
- breakout/momentum traders;
- metaorder executors;
- inventory/liability hedgers;
- forced liquidators;
- option end-users.

A random trader is useful as a control, not as the sole source of order flow.

---

## 10.3 Arbitrageurs

Important classes:

- cross-exchange same-instrument arbitrage;
- triangular spot arbitrage;
- spot–perp basis arbitrage;
- spot–dated-future cash-and-carry;
- calendar spread arbitrage;
- future–perp arbitrage;
- put–call parity arbitrage;
- synthetic-forward arbitrage;
- option box/conversion/reversal where appropriate;
- cross-venue option arbitrage.

### Critical rule

Never implement arbitrage as:

```text
if mispricing > fee:
    instantly force prices together
```

Actual expected profit should include:

\[
\Pi =
\text{gross mispricing}
-
\text{fees}
-
\text{slippage}
-
\text{expected leg risk}
-
\text{inventory cost}
-
\text{funding cost}
-
\text{latency risk}
-
\text{margin cost}
\]

The arbitrageur should submit real orders.

Leg A may fill while leg B does not.

That execution risk is part of the market mechanism.

---

# 11. Spot market without an externally imposed price path

If the goal is maximum emergence, do not use:

```text
spot_price(t) = stochastic_process(t)
```

as the market price.

Instead separate:

```text
latent economic state
        ↓ noisy heterogeneous observations
agents form beliefs
        ↓
orders
        ↓
LOB
        ↓
market price
```

For example:

\[
F_{t+1}=F_t+\epsilon_t
\]

may represent latent common value or aggregate information, while agent \(i\) observes:

\[
s_{i,t}=F_t+\eta_{i,t}
\]

with heterogeneous:

- observation delays;
- noise;
- priors;
- horizons;
- confidence;
- models.

The **trade price** remains entirely endogenous.

A harder version can derive value from dividends/cash flows rather than an abstract latent price.

---

# 12. Perpetual futures

The perp market should contain real coupling mechanisms.

## 12.1 Funding

Funding should be a transfer, not a price adjustment.

Example:

\[
FundingPayment_i
=
Position_i \times MarkPrice \times FundingRate
\]

Funding rate may depend on an index/premium formula determined by the venue.

Different venues should have:

- different intervals;
- different clamps;
- different TWAP windows;
- different mark/index constructions.

This creates natural clock interactions.

## 12.2 Basis traders

Perp–spot basis traders should react to:

- expected funding;
- borrow cost;
- fees;
- available balance sheet;
- execution risk;
- latency;
- liquidation risk.

The perp premium should then be an emergent equilibrium between directional demand, market makers, and basis capital.

## 12.3 What to compare against reality

Examples:

- premium distribution;
- mean reversion of basis;
- funding/premium relationship;
- extreme positive/negative basis regimes;
- price discovery lead–lag;
- liquidation cascades;
- basis widening when arbitrage capital is constrained.

Rao (2025) is a useful first benchmark.

---

# 13. Dated futures

Dated futures are especially valuable because maturity gives a sharp structural test.

Do **not** hard-code convergence.

Instead let:

- settlement rules;
- arbitrage;
- financing;
- inventory;
- roll behavior

produce convergence.

The simulator should be able to test:

\[
F_{t,T} \to S_T
\]

near maturity without a direct convergence force.

Important actor classes:

- cash-and-carry arbitrageurs;
- reverse cash-and-carry arbitrageurs;
- hedgers;
- directional traders;
- calendar spread traders;
- market makers.

Important lifecycle events:

- listing;
- liquidity migration;
- roll;
- last trading period;
- settlement;
- delisting;
- new maturity listing.

An important emergent question is whether liquidity migrates from the expiring contract to the next maturity without a scripted handoff.

---

# 14. Options

Options are likely the hardest subsystem.

Black–Scholes itself is easy.

The hard part is the feedback loop:

```text
option demand
    ↓
dealer inventory / greeks
    ↓
option quotes
    ↓
fills
    ↓
delta/gamma/vega exposure
    ↓
underlying/perp hedging
    ↓
underlying price impact
    ↓
new greeks / implied volatility
    ↓
new option quotes and demand
```

This loop can generate instability, volatility amplification, or damping.

The Kawakubo artificial-market papers demonstrate that dynamic hedging can change underlying volatility.

The empirical/theoretical hedging-feedback literature also supports this mechanism.

## 14.1 Stage options carefully

### Stage A

- European calls/puts;
- Black–Scholes fair-value estimates;
- simple option market maker;
- exogenous but economically motivated option demand;
- delta hedge in underlying.

### Stage B

- multiple strikes;
- multiple maturities;
- inventory-aware quoting;
- implied-volatility inference;
- put–call parity arbitrage.

### Stage C

- dynamic delta hedging;
- gamma-aware hedge thresholds;
- volatility traders;
- straddles/strangles;
- synthetic positions.

### Stage D

- stochastic-volatility beliefs;
- surface market makers;
- vega hedging;
- skew trading;
- Vanna–Volga or similar relative-value actors.

Vanna–Volga should be late-stage. It is not needed to obtain the first useful emergent option feedback effects.

---

# 15. Option surface: do not inject the smile

The simulator should **not** initialize every option permanently from:

```text
IV(K,T) = empirical_smile_function(K,T)
```

if the research question is whether a smile emerges.

A permissible initialization is a temporary seed surface so the market can open, after which quotes must be maintained by agents.

Then measure whether the system develops:

- smile/skew;
- term structure;
- moneyness-dependent liquidity;
- expiration effects;
- put–call parity deviations;
- surface deformation under directional or volatility demand.

Plham is the most directly useful public precedent here.

---

# 16. Information heterogeneity

A free market should not mean all agents possess identical information.

Give each participant:

- an information subscription;
- a latency;
- a model;
- a horizon;
- private signals;
- risk constraints.

For example:

| Agent | L1 | Full book | Other venue | Funding | Options surface | Private signal |
|---|---:|---:|---:|---:|---:|---:|
| retail taker | yes | no | delayed | yes | maybe | weak |
| HFT MM | yes | yes | fast | yes | yes | none |
| basis fund | yes | partial | fast | strong | no | none |
| informed trader | yes | no | normal | no | no | strong |
| option MM | yes | yes | fast | yes | full | vol model |

This creates real informational niches.

---

# 17. Heterogeneous clocks

Avoid one global “agent step”.

Realistic interactions require different clocks:

- exchange matching: event-driven;
- HFT maker: micro/milliseconds;
- inventory maker: tens/hundreds of ms;
- arbitrage scanner: venue-dependent;
- retail/taker: seconds/minutes;
- funding: hours;
- expiry: days/weeks;
- capital allocation: days/months.

This is not an implementation detail.

CHAD's “fragility pocket” result is a warning that relationships between timescales can create phase transitions.

Therefore every actor class should have an explicit reaction-time distribution.

---

# 18. Fairness and market rules

If the goal is a “free, fair market”, fairness should mean:

- identical venue rules for equivalent participants;
- no privileged access except explicitly modeled technology/latency differences;
- no hidden simulator oracle accessible only to favored agents;
- no guaranteed maker profit;
- no guaranteed taker fills;
- no artificial intervention to preserve a preferred price path;
- no post-hoc balance corrections.

It does **not** mean every agent has equal information, capital, or latency. Real market heterogeneity is part of the mechanism.

---

# 19. Validation: the major literature lesson

The most useful reference is:

[Vyetrenko et al. (2019), *Get Real: Realism Metrics for Robust Limit Order Book Market Simulations*](https://arxiv.org/abs/1912.04941)

The paper emphasizes that simulated markets can reproduce some statistics while significantly missing others.

A useful recent overview is:

[Agent-Based Modeling in Financial Markets: Modeling Frameworks, Validation Challenges, and Emerging Applications (2026)](https://doi.org/10.3934/nhm.2026043)

It distinguishes:

- **verification** — is the code implementing the specified model?
- **calibration** — were parameters chosen to match known data?
- **validation** — does the model reproduce facts not used for fitting?

That distinction should be enforced in the project.

---

# 20. Three separate datasets of facts

Maintain three disjoint sets.

## 20.1 Calibration facts

Allowed for parameter search.

Examples:

- average spread;
- average event rate;
- typical order-size distribution;
- rough market-maker participation;
- average funding interval/rule;
- fee schedules.

## 20.2 Validation facts

Never used in fitting.

Examples:

- return-tail exponent;
- volatility autocorrelation;
- impact concavity;
- order-sign memory;
- basis distribution;
- cross-venue lead–lag;
- liquidity recovery after shock;
- IV-skew response to underlying moves.

## 20.3 Discovery facts

Effects not deliberately targeted in either calibration or validation.

These are scientifically the most interesting.

Example:

> An expiry/funding synchronization unexpectedly creates temporary basis instability.

That is the kind of emergent mechanism worth investigating.

---

# 21. Stylized-fact scoreboard

The realism suite should be broad enough that one mechanism cannot fake everything.

## 21.1 Price/return statistics

Test:

- approximately negligible linear autocorrelation of raw returns at ordinary lags;
- heavy tails;
- volatility clustering;
- long memory in absolute/squared returns;
- aggregation toward more Gaussian behavior at longer horizons;
- asymmetric drawdown/recovery behavior where empirically appropriate.

## 21.2 Volume/order-flow statistics

Test:

- volume distribution;
- volume–volatility relationship;
- clustered activity;
- order-size distribution;
- order-sign autocorrelation / long memory;
- cancellation-to-trade behavior;
- event interarrival distribution.

## 21.3 Limit-order-book statistics

Test:

- spread distribution;
- spread vs volatility;
- depth profile;
- imbalance distribution;
- fill-time distribution;
- queue depletion;
- resilience after aggressive flow;
- price impact vs order size;
- impact decay/reversion.

`Get Real` should be used as a starting checklist.

## 21.4 Cross-venue statistics

Test:

- same-asset price dispersion;
- cointegration;
- lead–lag;
- fragmentation;
- arbitrage duration;
- liquidity migration after fee/tick changes.

## 21.5 Futures/perp statistics

Test:

- basis distribution;
- basis convergence near dated-future expiry;
- funding/premium relationship;
- open-interest dynamics;
- funding-event response;
- cross-venue funding arbitrage;
- liquidation clustering.

Xu et al. (2014) is a useful benchmark for spot–futures stylized facts.

## 21.6 Options statistics

Test:

- IV smile/skew;
- IV term structure;
- put–call parity deviations;
- liquidity vs moneyness/maturity;
- expiration effects;
- underlying response to option hedge flows;
- gamma-exposure-dependent volatility effects.

## 21.7 Ecology statistics

Test:

- wealth-share dynamics;
- strategy extinction;
- crowding;
- density-dependent profitability;
- capital migration;
- regime transitions;
- concentration of liquidity provision;
- concentration of arbitrage capital.

Evology and CHAD are relevant references.

---

# 22. Do not optimize one weighted realism score too early

A single objective like

\[
L = \sum_k w_k d_k
\]

is useful for search but dangerous scientifically.

A simulator can compensate for a terrible failure in one dimension by improving another.

Prefer a **validity corridor**:

```text
tail exponent           ∈ acceptable interval
spread distribution     ∈ acceptable interval
impact exponent         ∈ acceptable interval
volatility ACF          ∈ acceptable interval
basis variance          ∈ acceptable interval
fill-time distribution  ∈ acceptable interval
...
```

A run passes only if the important dimensions are individually plausible.

This is conceptually close to the CHAD validation approach.

---

# 23. Causal validation through interventions

This should be the distinguishing feature of the project.

For every proposed mechanism, write a prediction **before** running the experiment.

Examples:

### Remove cross-venue arbitrageurs

Prediction:

- same-instrument venue dispersion increases;
- mispricings persist longer;
- venue price correlation falls.

### Remove basis arbitrageurs

Prediction:

- spot/perp and spot/future basis widen;
- funding becomes less effective at anchoring the perp;
- maturity convergence weakens.

### Increase arbitrage latency

Prediction:

- larger short-lived price dispersion;
- higher arbitrage P&L per successful trade;
- potentially greater shock propagation.

### Remove option delta hedging

Prediction:

- option fills no longer create the same underlying hedge-flow footprint;
- gamma-dependent feedback on underlying volatility should weaken.

### Increase maker inventory aversion

Prediction:

- quote skew increases;
- inventory variance falls;
- spread/depth may change.

### Remove noise/liquidity-demand agents

Prediction:

- volume declines substantially;
- maker revenues decline;
- some ecological niches disappear.

If the predicted structure does not move, the claimed mechanism is not established.

---

# 24. Mechanism closure tests

When observing:

\[
Y = f(X)
\]

do not immediately claim causality.

Decompose the chain.

Example:

```text
maker skew
    ↓
quote placement
    ↓
book geometry
    ↓
taker walk
    ↓
mechanical price move
    ↓
maker inventory
    ↓
requote
    ↓
later price move
```

Measure every arrow separately.

If two purportedly independent channels co-move, search for a common upstream cause.

Then manipulate a **second independent lever**.

This is essential for avoiding false “two-channel” or “one-channel” conclusions.

---

# 25. Mutation testing for scientific instruments

Ordinary unit tests are insufficient.

Deliberately create broken simulators and make sure the research suite catches them.

Mutations:

- reverse funding sign;
- double-count settlement;
- violate price-time priority;
- allow an agent to see future market data;
- remove self-trade prevention;
- delay all maker messages;
- execute both arbitrage legs atomically;
- ignore one option hedge fill;
- shift expiration by one event;
- let liquidation use stale collateral;
- duplicate a trade;
- drop a book delta;
- give one exchange zero latency accidentally.

If the realism/causal suite stays green, it is not sensitive enough.

---

# 26. Determinism and replay

A strong research simulator needs:

\[
SameConfig + SameSeed \Rightarrow SameEventLog
\]

Every experiment should be replayable.

Prefer separate random streams for:

- agent activation;
- private signals;
- order sizing;
- network latency;
- exogenous shocks;
- strategy exploration.

This makes paired experiments much more powerful.

Example:

```text
seed 91 control
seed 91 treatment
```

should differ only because of the manipulated variable.

Independent builders discussing market simulators on Reddit also repeatedly emphasize reproducibility and deterministic replay as prerequisites for debugging meaningful differences.

Relevant discussions:

- [Bourse announcement on r/quant](https://www.reddit.com/r/quant/comments/1bigzmz)
- [Agent based market simulation discussion](https://www.reddit.com/r/algotrading/comments/xx7be3)
- [qmrExchange discussion](https://www.reddit.com/r/quant/comments/xs1y54)

---

# 27. Hybrid residual “world agent” as an optional realism layer

A major difficulty in hand-written ABMs is that we do not know the true decomposition of real order flow into strategy classes.

Relevant work:

- [Coletta et al. (2021), *Towards Realistic Market Simulations: a Generative Adversarial Networks Approach*](https://arxiv.org/abs/2110.13287)
- [Coletta et al. (2022), *Learning to simulate realistic limit order book markets from data as a World Agent*](https://arxiv.org/abs/2210.09897)

The “World Agent” learns aggregate order-flow behavior directly from historical data rather than requiring every market participant to be manually modeled.

A newer approach:

- [Berti, Prenkaj & Velardi (2025), *TRADES: Generating Realistic Market Simulations with Diffusion Models*](https://arxiv.org/abs/2502.07071)
- Code: [LeonardoBerti00/DeepMarket](https://github.com/LeonardoBerti00/DeepMarket)

TRADES generates responsive market order flow conditioned on the simulated state.

## Recommended use

Do **not** replace the whole ecology with a neural world agent if the research objective is causal understanding.

Instead consider:

\[
Flow =
Flow_{\text{explicit interpretable agents}}
+
Flow_{\text{residual world agent}}
\]

Explicit agents:

- market makers;
- arbitrageurs;
- hedgers;
- liquidators;
- option dealers;
- experimental strategies.

Residual learned flow:

- the unmodeled long tail of real participants.

This may improve microstructure realism without sacrificing the interpretability of the mechanisms under study.

Use it only after the hand-coded ecology is well understood.

---

# 28. Danger of closed-loop learned generators

The World Agent literature identifies an important failure mode.

A generative model trained on historical states may encounter states created by its own imperfect earlier actions during closed-loop simulation. It can then drift into unrealistic regions.

The 2022 World Agent work explicitly addresses this with unrolled training.

The general lesson is broader:

> Any learned background agent must be trained and tested under the states generated by interaction with experimental agents, not only under historical teacher-forced states.

This applies to diffusion/transformer order generators too.

---

# 29. Build order

Do not build all instruments simultaneously.

## Phase 0 — kernel correctness

- deterministic event engine;
- message latency;
- replay;
- accounting ledger;
- conservation tests;
- one LOB.

No realism claims.

## Phase 1 — one endogenous spot market

Agents:

- inventory maker;
- fixed-distance maker;
- noise/liquidity taker;
- informed trader;
- trend/mean-reversion traders.

Goal:

- persistent endogenous price formation;
- reasonable spread/depth/volume;
- no exogenous price path.

## Phase 2 — two exchanges, same spot asset

Add:

- distinct fees;
- distinct tick sizes;
- distinct latency;
- cross-venue arbitrage.

Research:

- fragmentation;
- price discovery;
- arbitrage duration;
- shock transfer.

## Phase 3 — perpetual futures

Add:

- perp venue;
- margin;
- mark/index;
- funding;
- basis traders;
- liquidations.

Goal:

- emergent spot–perp anchoring.

## Phase 4 — dated future

Add:

- expiry;
- settlement;
- cash-and-carry;
- calendar spreads;
- roll.

Goal:

- emergent maturity convergence and liquidity migration.

## Phase 5 — basic options

Add:

- European options;
- strikes/maturities;
- BS-based dealer beliefs;
- option demand;
- delta hedge;
- exercise/expiry.

Goal:

- functioning option market whose hedge flows affect underlying markets.

## Phase 6 — option ecology

Add:

- put–call parity agents;
- straddle/strangle traders;
- volatility beliefs;
- gamma-sensitive hedging;
- surface market makers.

Goal:

- nontrivial endogenous surface.

## Phase 7 — advanced vol strategies

Add:

- stochastic-volatility models;
- vega risk;
- Vanna–Volga actors;
- multi-expiry hedging.

## Phase 8 — long lifecycle test

Run through at least:

- multiple funding clocks;
- multiple dated-future listings/expiries;
- multiple option listings/expiries;
- overlapping expirations;
- liquidity shocks;
- volatility shocks;
- agent bankruptcies/entry.

Require the market to remain inside the viability corridor.

---

# 30. “Ten cycles alive” should be a research benchmark

Define one full lifecycle cycle clearly.

For example:

```text
new future listed
new option expiry listed
several funding events
liquidity and positions migrate
option expiry
future expiry
settlement
next contracts become front month
```

Run this repeatedly.

Track:

- volume by instrument;
- spread;
- depth;
- open interest;
- capital by strategy;
- bankruptcies;
- basis;
- funding;
- IV surface;
- arbitrage opportunities;
- liquidation count;
- price-discovery share by venue/instrument.

The simulation fails if it technically keeps executing trades but becomes economically degenerate.

---

# 31. Suggested “emergence tiers”

A useful way to communicate scientific strength:

## Tier 0 — mechanical

Exchange mechanics are correct.

## Tier 1 — statistical

The simulation reproduces selected stylized facts.

## Tier 2 — cross-market

Linked-market relationships emerge from trading rather than imposed price equations.

## Tier 3 — causal

Ablations/interventions reproduce predicted mechanism changes.

## Tier 4 — ecological

Strategy populations survive, crowd, specialize, and change profitability endogenously.

## Tier 5 — discovery

The simulator produces a robust real-market-like phenomenon that was **not** an explicit calibration target, and the causal chain can be identified experimentally.

Tier 5 is the real research prize.

---

# 32. Candidate real-market effects to attempt to rediscover

Do not tune all of these simultaneously.

Choose subsets as held-out discoveries.

### Generic microstructure

- fat-tailed returns;
- volatility clustering;
- near-zero raw return autocorrelation;
- long memory in absolute returns;
- volume–volatility relationship;
- concave market impact;
- liquidity recovery after shocks;
- order-flow persistence.

### Fragmented markets

- transient cross-venue price dispersion;
- lead–lag price discovery;
- arbitrage-mediated shock propagation;
- liquidity migration under fee/tick changes.

### Perpetuals

- premium/funding relationship;
- basis compression by arbitrage;
- basis widening under leverage constraints;
- liquidation cascades;
- derivative-led price discovery in some regimes.

### Dated futures

- maturity convergence;
- calendar structure;
- liquidity/open-interest roll;
- temporary expiry dislocations.

### Options

- volatility smile/skew;
- maturity term structure;
- put–call parity mostly holding but temporarily breaking;
- option-hedging feedback into underlying volatility;
- different behavior under positive vs negative dealer gamma;
- expiry-related underlying flow.

### Ecology

- strategy crowding;
- endogenous extinction;
- regime changes after capital reallocation;
- flash-crash fragility from synchronized reaction times;
- specialization of adaptive agents.

---

# 33. How not to fool ourselves

## Rule 1

Never add a behavior solely because “the chart needs more volatility clustering”.

Add a plausible micro mechanism, then test what it produces.

## Rule 2

Every stylized fact used to choose parameters is **calibration**, not independent validation.

## Rule 3

Keep a frozen holdout list of market effects.

## Rule 4

For every major conclusion, run at least one intervention that should destroy the effect.

## Rule 5

Use multiple independent mechanisms for similar functions.

Example: at least two maker types with structurally different quoting logic.

## Rule 6

Measure full distributions, not only means.

## Rule 7

Use paired seeds for treatments.

## Rule 8

Search for clock artifacts explicitly.

Sweep horizons relative to:

- maker refresh intervals;
- funding intervals;
- liquidation delays;
- network delays.

## Rule 9

Track who owns resting liquidity and who caused each trade.

## Rule 10

A convincing graph is not a mechanism.

---

# 34. Research program

The simulator itself can support a series of papers.

## Paper direction A — unified open derivatives ecology

Contribution:

> A public microstructure-level simulator in which spot, perp, futures, and options coexist endogenously across multiple venues.

## Paper direction B — validation methodology

Contribution:

> Causal validation and mutation testing for agent-based financial market simulators.

## Paper direction C — ecology and survival

Question:

> Which combinations of hedgers, makers, takers, and arbitrage capital sustain a persistent market rather than collapsing to illiquidity or strategy monopoly?

## Paper direction D — derivative feedback

Question:

> Under which inventory/gamma regimes does endogenous option hedging amplify or dampen underlying volatility?

## Paper direction E — synchronized clocks

Question:

> Can funding, expiry, maker refresh, and liquidation timescales create endogenous fragility pockets?

## Paper direction F — market structure

Question:

> How do different fee, tick, latency, and margin regimes determine where price discovery occurs across spot and derivatives?

---

# 35. Recommended immediate design changes

If the existing simulator already has multi-venue matching, makers, takers, arbitrageurs, funding, and detailed replay, the highest-value changes are:

1. **Make information access explicit.**
   No agent should call arbitrary simulator internals.

2. **Separate venue, instrument, and clearing.**
   This becomes essential once one account holds spot, perp, futures, and options simultaneously.

3. **Create a single immutable accounting ledger.**
   Balances derive from ledger events.

4. **Introduce non-PnL utility agents.**
   This is required for long-run ecological viability.

5. **Add explicit external capital/endowment flows.**
   Record them rather than resetting agents.

6. **Create lifecycle events as first-class objects.**
   Funding, listing, expiry, exercise, and settlement should all be observable events.

7. **Make latency and reaction clocks first-class parameters.**

8. **Add ownership/lineage attribution to book liquidity.**
   This enables causal statements about which actor class creates observed depth.

9. **Freeze a validation holdout set before the next calibration campaign.**

10. **Build mutation tests for analysis instruments, not only engine code.**

---

# 36. Final assessment

There is substantial prior work on every major ingredient:

- high-fidelity agent-based LOB simulation — ABIDES;
- multi-market artificial markets — Plham/PAMS;
- options and delta hedging — Plham and Kawakubo et al.;
- spot–futures coupling — Xu et al.;
- perpetual futures ABM — Rao;
- spot/futures/options in older commodity ABMs — N-ABLE;
- market ecology — Evology;
- timescale-driven fragility — CHAD;
- self-organized learned agents — Hashimoto et al.;
- data-driven residual market agents — World Agent and TRADES;
- realistic multi-venue exchange infrastructure — QuantReplay.

What I did **not** find in public/open form is a single system that combines all of the following simultaneously:

```text
multiple real order-driven venues
+
endogenous spot price discovery
+
perpetual futures
+
dated futures
+
options
+
margin / liquidation / funding
+
listing / expiry / settlement lifecycle
+
heterogeneous market makers
+
heterogeneous liquidity takers
+
cross-venue arbitrage
+
cross-instrument arbitrage
+
option hedging
+
persistent evolving market ecology
+
causal and statistical validation
```

That integrated target appears meaningfully novel as an open research platform.

The most important design principle is:

> **Do not optimize the simulator to look like a market. Build economically plausible local mechanisms, constrain them using real institutional and micro-level data, and then test whether market-level regularities appear.**

The strongest result will not be:

> “Our simulated returns have fat tails.”

It will be:

> “We did not encode this market-level effect. It appears robustly across seeds and parameter ranges, disappears under the predicted causal ablation, reappears when the mechanism is restored, and its statistics fall inside an empirical validity corridor.”

That is the standard required for genuinely emergent behavior.

---

# References and useful links

## Core simulation frameworks

1. Byrd, D., Hybinette, M., & Balch, T. (2019). **ABIDES: Towards High-Fidelity Market Simulation for AI Research.**  
   https://arxiv.org/abs/1904.12066

2. JPMorgan. **ABIDES public repository.**  
   https://github.com/jpmorganchase/abides-jpmc-public

3. Hirano, M., Takata, R., & Izumi, K. (2023). **PAMS: Platform for Artificial Market Simulations.**  
   https://arxiv.org/abs/2309.10729

4. PAMS repository.  
   https://github.com/masanorihirano/pams

5. PAMS documentation.  
   https://pams.hirano.dev/en/latest/

6. Plham documentation.  
   https://plham.github.io/

7. Quod Financial. **QuantReplay.**  
   https://github.com/Quod-Financial/quantreplay

8. Bourse documentation.  
   https://zombie-einstein.github.io/bourse/

## Options and cross-market artificial markets

9. Plham. **OptionMain.**  
   https://plham.github.io/tutorial/OptionMain

10. Plham. **Option strategy examples: delta hedge, put-call parity, straddles, etc.**  
    https://plham.github.io/tutorial/OptionMain_UseCases02

11. Plham. **ShockTransferMain.**  
    https://plham.github.io/tutorial/ShockTransferMain

12. Kawakubo, S., Izumi, K., & Yoshimura, S. (2014). **How Does High Frequency Risk Hedge Activity Have an Affect on Underlying Market?**  
    https://doi.org/10.20965/jaciii.2014.p0558

13. Kawakubo, S., & Izumi, K. (2016). **Analysis of the Interaction between Option Market and Its Underlying Market by Coupled Artificial Markets.**  
    https://doi.org/10.1527/tjsai.AG-D

14. Xu, H.-C., Zhang, W., Xiong, X., & Zhou, W.-X. (2014). **An Agent-Based Computational Model for China's Stock Market and Stock Index Futures Market.**  
    https://arxiv.org/abs/1404.1052

15. Rao, R. (2025). **Agent-Based Simulation of a Perpetual Futures Market.**  
    https://arxiv.org/abs/2501.09404

16. Ehlen, M. A., & Scholand, A. J. (2005). **An Agent Model of Agricultural Commodity Trade: Developing Financial Market Capability within N-ABLE.**  
    https://www.researchgate.net/publication/263504592_An_Agent_Model_of_Agricultural_Commodity_Trade_Developing_Financial_Market_Capability_within_the_NISAC_Agent-Based_Laboratory_for_Economics_N-ABLE

## Hedging feedback

17. Platen, E., & Schweizer, M. (1998). **On Feedback Effects from Hedging Derivatives.**  
    https://doi.org/10.1111/1467-9965.00045

18. Anderegg, B., Ulmann, F., & Sornette, D. (2022). **The impact of option hedging on the spot market volatility.**  
    https://doi.org/10.1016/j.jimonfin.2022.102627

19. Aubert, P., Chevalier, E., & Ly Vath, V. (2025). **Option market making with hedging-induced market impact.**  
    https://arxiv.org/abs/2511.02518

## Market ecology and emergence

20. Vie, A., Scholl, M., Kleinnijenhuis, A. M., & Farmer, J. D. (2022). **Towards Evology: a Market Ecology Agent-Based Model of US Equity Mutual Funds.**  
    https://arxiv.org/abs/2210.11344

21. Vie, A., & Farmer, J. D. (2023). **Towards Evology II.**  
    https://arxiv.org/abs/2302.01216

22. **CHAD: A Scalable Turn-Based Simulator** (2026).  
    https://openreview.net/pdf?id=JGad8gf6E6

23. Hashimoto, R., Takata, R., Suzuki, M., Tanaka, Y., & Izumi, K. (2026). **Financial Market as a Self-Organized Ecosystem: Simulation via Learning with Heterogeneous Preferences.**  
    https://arxiv.org/abs/2604.23975

## Realism and validation

24. Vyetrenko, S., Byrd, D., Petosa, N., et al. (2019). **Get Real: Realism Metrics for Robust Limit Order Book Market Simulations.**  
    https://arxiv.org/abs/1912.04941

25. **Agent-Based Modeling in Financial Markets: Modeling Frameworks, Validation Challenges, and Emerging Applications** (2026).  
    https://doi.org/10.3934/nhm.2026043

26. Fagiolo et al./ABM validation literature overview: **Empirical Validation of Agent-Based Models** (2018).  
    https://doi.org/10.1016/bs.hescom.2018.02.003

## Data-driven market background agents

27. Coletta, A., Prata, M., Conti, M., et al. (2021). **Towards Realistic Market Simulations: a Generative Adversarial Networks Approach.**  
    https://arxiv.org/abs/2110.13287

28. Coletta, A., Moulin, A., Vyetrenko, S., & Balch, T. (2022). **Learning to simulate realistic limit order book markets from data as a World Agent.**  
    https://arxiv.org/abs/2210.09897

29. Berti, L., Prenkaj, B., & Velardi, P. (2025). **TRADES: Generating Realistic Market Simulations with Diffusion Models.**  
    https://arxiv.org/abs/2502.07071

30. DeepMarket / TRADES code.  
    https://github.com/LeonardoBerti00/DeepMarket

## Community / independent implementations

31. **Bourse: An Open-Source Python and Rust Simulated Limit Order-Book and Agent Based Simulation Library.**  
    https://www.reddit.com/r/quant/comments/1bigzmz

32. **Agent based market simulation — r/algotrading discussion.**  
    https://www.reddit.com/r/algotrading/comments/xx7be3

33. **qmrExchange / Trading Exchange Engine — r/quant.**  
    https://www.reddit.com/r/quant/comments/xs1y54
