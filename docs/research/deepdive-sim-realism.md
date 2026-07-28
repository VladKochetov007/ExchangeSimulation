# Deep Dive: State of the Art in Realistic LOB Simulation

Scope: what makes an agent-based limit-order-book simulation *realistic*, how the
literature validates that claim, and where `exchange_sim` stands. Companion to
`docs/realism-gaps.md` (which covers **venue-mechanics** fidelity — margin,
liquidation, STP, iceberg semantics). This document covers **statistical and
ecological** fidelity: does the emergent tape look like a real tape.

Repo grounding: `simulations/feesim`, `simulations/randomwalk`,
`simulations/derivsim`, `simulations/abcusd`; `simulation/scheduler.go`,
`simulation/latency.go`; metrics in `cmd/loganalyzer/main.go`.

---

## 1. Reference systems

### 1.1 ABIDES (JPMorgan)

- Agent-Based Interactive Discrete Event Simulation.
  Paper: <https://arxiv.org/abs/1904.12066> ·
  Code: <https://github.com/jpmorganchase/abides-jpmc-public> ·
  Gym wrapper: <https://arxiv.org/abs/2110.14771>
- **Kernel**: single priority message queue keyed on delivery timestamp;
  nanosecond resolution; an agent is *only* active when it receives a message.
  Agents self-schedule via `setWakeup` (wakeup messages to self). Same shape as
  our `simulation/scheduler.go` + `AddTicker`.
- **Latency**: three separable components — (a) a **pairwise agent×agent latency
  matrix** (not a per-actor scalar), (b) a **latency noise model** applied per
  message, (c) a **per-agent computation delay** added to that agent's clock
  after every activity, which delays both its sends and its next responsiveness.
- **Protocol**: messages modeled on NASDAQ ITCH (market data) / OUCH (order
  entry). Scales to tens of thousands of agents.
- **Determinism**: one master PRNG seed; **per-agent PRNGs seeded from the master**
  so that changing one agent's behavior does not shift every other agent's random
  stream. This is what makes A/B experiments valid.
- **Shipped ecology** (RMSC04 reference config): 1 exchange, **1000 noise agents,
  102 value agents, 12 momentum agents, 2 market makers**.
  - *Noise*: random market orders, wakeup times drawn from **Beta(1/2, 1/2)** over
    the session → produces the empirical U-shaped intraday volume.
  - *Value*: observe a noisy private signal of an exogenous fundamental
    (Poisson arrivals, idiosyncratic noise σ = ν), quote limit orders around
    their valuation → anchors the price level.
  - *Momentum*: compare short/long moving averages → transient dislocations.
  - *Market maker*: two-sided ladder at constant arrival rate.
  - *Volatility agents*: intensity scaled by distance from midday.

**What ABIDES does that we don't**: pairwise latency matrix + computation delay;
per-agent PRNG isolation; a *population* of hundreds-to-thousands of heterogeneous
background agents rather than a handful of actor objects; an exogenous stochastic
fundamental process that value agents observe through noise.

### 1.2 Queue-reactive and Hawkes order-flow models

- Huang, Lehalle, Rosenbaum, *Simulating and Analyzing Order Book Data: The
  Queue-Reactive Model* (JASA 2015): <https://arxiv.org/pdf/1312.0563>
- Core: within periods where the reference price is constant, the book is a
  **Markov queuing system**; arrival intensities of limit / cancel / market
  orders (λ^L, λ^C, λ^M) at each level are **functions of the current queue
  sizes**. Price moves are handled by a separate reference-price mechanism.
- Extensions worth knowing:
  - Queue-reactive **Hawkes** models — exogenous intensity of a multivariate
    Hawkes process depends on queue state: <https://arxiv.org/pdf/1901.08938>,
    <https://www.worldscientific.com/doi/10.1142/S2382626620500136>
  - Order **sizes** matter and are missing from vanilla QR:
    <https://arxiv.org/pdf/2405.18594>
  - **MDQR** (deep queue-reactive, 2025): relaxes queue independence across
    levels, enriches the state, and models the *distribution* of order sizes;
    validated by reproducing the **square-root law of impact** and conditional
    order-size distributions on Bund futures:
    <https://arxiv.org/abs/2501.08822>
- Relevance: this is the cheapest known way to get a book whose *shape and
  dynamics* are emergent rather than a configured ladder. Our takers already have
  an `ImbalanceCoupling` term — that is a one-dimensional shadow of the QR idea.

### 1.3 Survey and benchmark

- *Limit Order Book Simulations: A Review* (2024):
  <https://arxiv.org/html/2402.17359v1> — taxonomy: point-process /
  zero-intelligence, Hawkes, agent-based (ABIDES, JAX-LOB-style), deep
  generative, SPDE. Stated open problems: no parsimonious model covers most
  stylized facts; Hawkes kernel choice/calibration is hard and O(n²);
  **exchange mechanics (halts, auctions, hidden orders, queue priority) are
  largely unaddressed by academic simulators**.
- **LOB-Bench** (2025), Nagy, Frey, Vyetrenko, Zohren, Foerster et al.:
  <https://arxiv.org/abs/2502.09172> · <https://lobbench.github.io/> ·
  <https://github.com/peernagy/lob_bench> — measures **distributional distance
  between generated and real message streams**, conditional and unconditional,
  plus a trained discriminator score and **market-impact metrics (price response
  functions, cross-correlations)**. This is the modern successor to eyeballing
  stylized facts.

---

## 2. The validation checklist (Cont 2001 + Get Real)

Cont, *Empirical properties of asset returns* (2001):
<https://www.stat.rice.edu/~dobelman/courses/texts/stylized.cont.2001.pdf>

Vyetrenko et al., *Get Real: Realism Metrics for Robust Limit Order Book Market
Simulations* (ICAIF '20): <https://arxiv.org/abs/1912.04941>

Full catalog, with empirical reference values:

**Returns**
- Absence of linear autocorrelation beyond ~20 min.
- Heavy tails + aggregational gaussianity (kurtosis → 3 as horizon grows).
- Volatility clustering: ACF of squared returns positive over days.
- Long-range dependence: ACF of |r| decays as τ^−β, **β ∈ [0.2, 0.4]**.
- Gain/loss asymmetry (negative skew; weaker in FX/rates).
- Volume–volatility positive correlation; return–volatility negative correlation
  (leverage).
- Asymmetric information flow: coarse volatility predicts fine better than
  the reverse.

**Order flow / volumes**
- Book volumes ~ **Gamma**: P(V) ∼ V^(γ−1) e^(−V).
- Order sizes **power law** P(x) ∼ x^−(1+μ): limit orders **1+μ ≈ 2**, market
  orders **1+μ ≈ 2.3–2.7**; strong round-number clustering.
- Orders per fixed window: Gamma or lognormal.
- Inter-arrival times: exponential / lognormal / Weibull (**not** deterministic).
- **New limit-order price offset from best**: power law, **1+μ ≈ 1.6**.
- **Time-to-cancel / time-to-execute**: power law, **1+μ ∈ [1.3, 1.6]**
  (cancelled ≈ 2.1, executed ≈ 1.5 in the econophysics literature:
  <https://arxiv.org/pdf/0909.1974>).
- Cross-agent time correlation of order flow (herding).

**Non-stationarity**
- U-shaped intraday volume; intraday volume–spread negative correlation;
  event/holiday effects.

**Impact**
- Price impact scaling M ∼ α·P^β over participation-rate bins.

**Get Real's own conclusion**: both tested configs (`sparse_zi_100`, `rmsc01`)
**failed** absence-of-autocorrelation, volume–volatility correlation, intraday
volume shape, and inter-arrival times; `rmsc01` was closest on heavy tails.
Realism improved most when **the fundamental value came from historical market
data**, and the authors argue correlated order behavior needs **learning agents**.

---

## 3. Market impact realism

- **Square-root law**: I/σ_D = Y·(Q/V)^(1/2) — impact of a metaorder is concave
  in its size, exponent ≈ 0.5, remarkably universal.
  Bitcoin metaorder evidence: <https://arxiv.org/pdf/1412.4503> ·
  US large-cap confirmation: <https://arxiv.org/pdf/2606.24019> ·
  unifying framework with order imbalance and volatility:
  <https://arxiv.org/abs/2506.07711>
- **What generates it in a simulator** — this is the key operational result.
  *Order Splitting and Liquidity Replenishment Are Jointly Necessary for the
  Square-Root Law: A Counterfactual Dissection*:
  <https://arxiv.org/html/2607.04280>
  - Baseline ABM exponent **δ = 0.549**.
  - Remove **order splitting** (metaorder executed in one shot) → **δ = 0.324**;
    impact then just traces the visible depth profile.
  - Remove **liquidity replenishment** (HFT agents refilling the best quotes
    between child orders) → **δ = 0.386**.
  - Everything else — momentum traders, price limits, splitting-rule details,
    background liquidity level — moves δ by **< 10%**.
  - Conclusion: you cannot get realistic impact without (a) metaorders split into
    child orders and (b) agents that refill depleted levels between children.
- **Long memory of order flow** is the same mechanism seen from the flow side:
  order-sign ACF decays as τ^−α with **α ≈ 0.6, Hurst H ≈ 0.7** (Lillo–Farmer,
  LSE; Bouchaud et al. report H ≈ 0.65–0.9 on Euronext):
  <https://arxiv.org/pdf/1012.0349> · <https://arxiv.org/pdf/1504.04354>.
  Lillo–Mike–Farmer derives it directly from the metaorder size distribution;
  quantitatively confirmed in PRL 2023:
  <https://link.aps.org/doi/10.1103/PhysRevLett.131.197401>
- **Propagator / transient impact**: each trade leaves a decaying price response;
  long-memory flow plus slowly decaying impact is what keeps prices diffusive
  despite predictable flow. Impact after a metaorder decays roughly as t^−1/2.
  Nonparametric self/cross impact estimation: <https://arxiv.org/html/2510.06879>

---

## 4. Order-flow realism details

- **Cancellations dominate**: in real venues the overwhelming majority of limit
  orders are cancelled rather than executed. Lifetime distributions are power
  laws with different exponents for cancelled (≈2.1) vs executed (≈1.5) orders —
  a simulator with deterministic cancel/replace cadence gets neither
  (<https://arxiv.org/pdf/0909.1974>).
- **Placement depth**: arrival rate of new limit orders decays as a power law in
  distance from the best (1+μ ≈ 1.6), while *resting depth* peaks several ticks
  away — most orders arrive near the touch, most volume sits deeper
  (<https://arxiv.org/pdf/cond-mat/0102518>).
- **Crypto-specific deltas** (relevant — our sim is crypto perps):
  - Trading is continuous 24/7; intraday liquidity-cost and trade-frequency
    patterns are **weak**, so the equities U-shape should *not* be forced.
    <https://doi.org/10.3390/jrfm12010025>
  - Trade-size distribution is very broad with a **mode at the minimum size
    increment** (retail dominance).
  - A large fraction of limit-order volume sits **very far from the mid** — a
    distinctive crypto feature.
  - Perp futures (Binance BTC/ETH 2020–2024): evidence favors the Mixture of
    Distributions Hypothesis; seasonal patterns are visible 2022–2024.
    <https://www.sciencedirect.com/science/article/pii/S2214845025001188>

---

## 5. Generative / world-model LOB (know it, don't adopt it yet)

- LOB-Bench (§1.3) is the benchmark that matters.
- Autoregressive message-level models currently beat GANs and parametric models
  on LOB-Bench (<https://arxiv.org/abs/2502.09172>).
- Diffusion: *Painting the Market* renders LOB state as images + inpainting for
  parallel long-sequence generation, SOTA on LOB-Bench
  (<https://arxiv.org/abs/2509.05107>); DiffVolume for volumes
  (<https://arxiv.org/pdf/2508.08698>); DiffLOB for counterfactual generation
  (<https://arxiv.org/html/2602.03776>).
- **Why not adopt**: these are *data-driven replayers*. They need real LOBSTER-
  grade message data we don't have, they are not interactive (an agent trading
  inside them does not change the generated flow, which is the whole point of our
  simulator), and they are black boxes. Their value to us is the **evaluation
  methodology**, not the generators.

---

## 6. Calibration (how you'd ever claim "realistic")

- **Adversarial calibration / MAS-GAN**: train a discriminator to separate real
  from simulated price-volume series, then use the discriminator as an implicit
  objective to tune simulator parameters — specifically **the proportions of each
  agent archetype**. <https://arxiv.org/abs/2108.00664>
- **Bayesian optimization** over output-series moments:
  <https://arxiv.org/pdf/2112.03874>
- **Simulation-based inference** with neural density estimators — recovers the
  full posterior over parameters rather than a point estimate:
  <https://arxiv.org/html/2311.11913>
- Practical takeaway: our agent-mix parameters (`MMCount`, `TakerExciteAlpha`,
  `ImbalanceCoupling`, `SkewTicksPerLot`) are exactly the knobs these methods
  calibrate. A moment-matching objective over `cmd/loganalyzer` output is a
  cheap first version.

---

## 7. What we already reproduce

Verified via `cmd/loganalyzer/main.go` (`SymbolMetrics`) and prior campaigns:

- **Trade-count Fano factor ≈ 1.5** — overdispersed arrivals, i.e. clustering in
  activity. Comes from the Hawkes-lite self-excitation in
  `simulations/feesim/taker.go` (`ExciteAlpha` / `ExciteBetaPerSec`).
- **Excess kurtosis ≈ 8.8** — heavy-tailed returns, in the right ballpark for
  intraday crypto.
- **Volatility clustering** — positive ACF of |r| at lags 1/5/10 (`ACFAbsR1`,
  `ACFAbsR5`, `ACFAbsR10`).
- **~Zero linear return ACF** (`ACFR1`) — no trivially exploitable
  autocorrelation. Note Get Real found *both* of its reference configs failed
  this test, so we are ahead of `rmsc01` here.
- **Spread distribution** (`MeanSpreadBps`, `P95SpreadBps`) and mean top-of-book
  depth are instrumented.
- **State-dependent order flow (weak form)**: `ImbalanceCoupling` tilts taker
  sign toward book imbalance — a scalar queue-reactive coupling.
- **Inventory-skewed MM** (`SkewTicksPerLot`, `SkewCapTicks`) — Avellaneda-
  Stoikov reservation-price logic, with the unit-root failure mode already
  understood and capped.
- **Value anchor** (`simulations/feesim/value.go`) — ABIDES-style value agent
  supplying mean reversion. Known necessary; without it the level random-walks
  into the tick floor.
- **Discrete-event kernel with per-actor latency** — `EventScheduler` +
  `LatencyConfig` (constant / uniform / normal / lognormal providers), FIFO per
  channel, correct under `SimulatedClock`.
- **Venue mechanics well beyond academic simulators**: margin, funding,
  liquidation, icebergs, hidden orders, STP, fees, multi-venue. The 2024 review
  explicitly lists these as *unaddressed* by the literature's simulators — this
  is our genuine edge.

---

## 8. Gaps vs SOTA

Ordered roughly by how much realism each blocks.

1. **No metaorders / no order splitting.** `RandomTaker.fireOrder` submits an
   independent market order sized ±50% around a target
   (`simulations/feesim/taker.go:128`). There is no parent order executed as a
   sequence of children. Per the counterfactual dissection, this alone caps the
   impact exponent near **0.32** instead of **~0.5**, and it means order-sign
   long memory (H ≈ 0.7) cannot exist — our sign process is near-i.i.d. plus a
   weak imbalance tilt.
2. **Order flow is one actor, not a population.** A single `RandomTaker` object
   with a single RNG fires all taker flow for all symbols
   (`simulations/feesim/sim.go:392`). ABIDES runs 1000+ noise agents. One actor
   means no cross-agent heterogeneity, no dispersion of beliefs, and burst
   structure that is a pure function of one excitation scalar.
3. **Takers never post limit orders.** Every taker order is `exchange.Market`.
   Consequences: the sim has **no cancellation process**, so no cancel-to-trade
   ratio, no order-lifetime distribution, no placement-depth distribution — three
   entire Get Real metric families are structurally unreachable.
4. **Book shape is configured, not emergent.** Depth comes from the MM's
   `Levels`/`LevelSpacing`/`LevelSize` ladder. Real depth profiles (hump several
   ticks out, Gamma-distributed level volumes) are an *outcome* in real markets;
   here they are an input. Any depth-profile "match" would be circular.
5. **No order-size distribution.** Sizes are uniform on ±50% of a target.
   Real: power law with 1+μ ≈ 2.3–2.7 for market orders, mode at the minimum
   increment in crypto, heavy round-number clustering.
6. **Deterministic inter-arrival times.** `AddTicker(TakeInterval)` gives a fixed
   grid; excitation only multiplies the *count per tick* (capped at 5). Real
   inter-arrivals are exponential/lognormal/Weibull. Get Real flagged inter-
   arrival times as a failure for both of its configs too — but ours fails in a
   more rigid way (a literal lattice).
7. **No exogenous fundamental process.** `ValueTraderConfig.Fundamental` is a
   **static int64**. ABIDES uses a mean-reverting stochastic fundamental observed
   through per-agent noise; Get Real found realism improved most when the
   fundamental was driven by historical data. Static fundamental ⇒ no news, no
   regime shifts, no genuine information-driven adverse selection.
8. **Latency is per-actor scalar, not pairwise + compute delay.**
   `simulation/latency.go` gives each mount a provider. Missing: agent×agent
   latency matrix, per-message noise, and **per-agent computation delay** (the
   thing that makes a slow strategy actually slow).
9. **PRNG not isolated per agent.** Agents are seeded from config-derived seeds;
   ABIDES seeds each agent from a master PRNG specifically so that changing one
   agent doesn't perturb every other agent's stream. Without this, A/B experiments
   are confounded.
10. **No impact measurement at all.** `cmd/loganalyzer` computes spread, depth,
    kurtosis, ACF, Fano — but **no price-response function, no participation-rate
    impact curve, no propagator decay**. We cannot currently tell whether impact
    is realistic.
11. **No intraday seasonality driver.** Defensible for 24/7 crypto (empirically
    weak), but "weak" is not "absent" and perp futures do show 2022–2024
    seasonal patterns. Currently structurally zero.
12. **No distributional distance metrics.** LOB-Bench-style scoring (conditional
    distribution distances, discriminator score) versus point statistics. We
    report point estimates with no reference intervals — no pass/fail criterion.

---

## 9. Elephants in the room

- **Single MM = the MM *is* the price.** Already known. `MMCount` exists but the
  default is 1 (`simulations/feesim/sim.go:112`). With one MM the mid is that
  MM's reservation price by construction; price "discovery" is the MM's skew
  parameter plus fills. Even at `MMCount > 1` the MMs are clones of one struct
  with the same policy — that is not competition, it is a thicker ladder.
- **No queue-position competition.** Real MMs fight for time priority at the
  touch; that fight determines spread, quote lifetime, and the flicker rate of
  the book. Our MMs refresh on independent timers with no notion of "am I in
  front of the queue". This is what large-tick microstructure is *made of*
  (Dayri–Rosenbaum: for large-tick assets spread ≈ 1 tick always, and the real
  liquidity variable is the *implicit* spread and the continuation/alternation
  ratio η — <https://arxiv.org/abs/1207.6325>).
- **No tick-size regime effects.** `TickSize` is a config number with no
  behavioral consequence. Real markets bifurcate into large-tick (spread pinned
  at one tick, queue priority is everything) and small-tick (spread floats, queue
  irrelevant) regimes with qualitatively different dynamics. Prior campaigns
  already hit a "tick-floor collapse spiral" — that is this regime boundary
  showing up as a bug rather than as modeled behavior.
- **No exogenous news / no information asymmetry.** Nobody in the ecology *knows*
  anything. Value traders act on a constant. Therefore there is no adverse
  selection in the economic sense, so MM spreads are not compensating for
  informed flow, so the spread level is arbitrary. This quietly invalidates any
  fee/rebate or MM-profitability experiment.
- **No agent survivorship / entry-exit.** Real ecologies select: losing MMs widen
  or leave, capital flows to winners. Our agent set is fixed for the run, so a
  systematically unprofitable strategy keeps providing liquidity forever. Any
  long-horizon conclusion about market quality is suspect.
- **Zero-latency MM mount is a modeling decision, not a law.**
  `docs/sim_guide.md` mandates MMs on a zero-latency mount to avoid a
  state-desync artifact. Defensible (co-location), but it means we cannot study
  latency competition among liquidity providers — the mechanism behind quote
  fading and mechanical liquidity erosion.
- **Circularity risk in validation.** Fano 1.5 comes from the excitation
  parameter; kurtosis comes from burst size. Tuning `ExciteAlpha` until Fano hits
  1.5 and then reporting Fano 1.5 as evidence of realism is fitting the metric,
  not the market. Realism claims need *out-of-sample* facts — ones no parameter
  was tuned against (impact exponent, sign ACF decay, lifetime exponents).

---

## 10. Concrete recommendations (ranked)

Each: mechanism → stylized fact unlocked → where it goes.

**R1. Metaorder / order-splitting executor agent.** *(highest value per line of
code)*
- Mechanism: a parent order of size Q drawn from a power law, split into N child
  market orders released on a schedule (POV or TWAP-ish), with N and the child
  interval configurable. Multiple such agents running concurrently.
- Unlocks: **long-memory order-sign ACF (τ^−0.6, H ≈ 0.7)** and, together with
  the existing quoting MM (the "liquidity replenishment" half), the
  **square-root impact law (δ ≈ 0.5)**. The counterfactual dissection shows these
  two mechanisms are jointly necessary and nearly everything else is optional —
  so this is the single highest-leverage change available.
- Where: new actor in `simulations/feesim` (`metaorder.go`), configured by
  archetype proportions. No exchange changes needed.

**R2. Impact + flow measurement in `cmd/loganalyzer`.**
- Mechanism: add (a) order-sign autocorrelation with a power-law fit on the
  exponent, (b) price-response function R(ℓ) = E[(m_{t+ℓ} − m_t)·ε_t], (c) impact
  vs participation rate binned as in Get Real (M ∼ α·P^β, report β), (d) ACF of
  |r| decay exponent β ∈ [0.2, 0.4], (e) trade-size distribution tail exponent.
- Unlocks: the ability to *falsify* R1 and everything after it. Right now we
  cannot measure the facts we most want. Ship this **with** R1, ideally first.
- Where: `cmd/loganalyzer/main.go` alongside `computeStylizedFacts`.

**R3. Population of heterogeneous background agents with randomized arrivals.**
- Mechanism: replace the single `RandomTaker` with N instances (N ∼ 50–500),
  each with its own seed, its own target size, and **exponential/lognormal
  inter-arrival draws** instead of a fixed ticker. Draw sizes from a power law
  with a floor at the minimum increment (crypto shape). Add per-agent PRNG
  isolation seeded from a master, ABIDES-style.
- Unlocks: realistic **inter-arrival distributions** (a documented Get Real
  failure for every published config), **order-size power law (1+μ ≈ 2.3–2.7)**,
  genuine cross-agent flow correlation, and valid A/B experiments.
- Where: `simulations/feesim/sim.go` agent construction + `taker.go` scheduling.
  Needs an actor-level "schedule next wakeup after Δ" primitive if `AddTicker` is
  fixed-period only — check `actor/` before designing.

**R4. Liquidity-taking *and* liquidity-providing background flow (limit orders
with a cancellation process).**
- Mechanism: background agents post limit orders at an offset from best drawn
  from a power law (1+μ ≈ 1.6), each with a lifetime drawn from a power law
  (1+μ ∈ [1.3, 1.6]) after which it cancels. Optionally make intensities
  queue-reactive: λ^L, λ^C, λ^M as functions of queue size (Huang–Lehalle–
  Rosenbaum), generalizing the existing scalar `ImbalanceCoupling`.
- Unlocks: **cancel-to-trade ratio**, **order-lifetime distributions**,
  **placement-depth distribution**, **Gamma-distributed level volumes**, and an
  **emergent book shape** — killing the circularity of item 8.4. Also makes the
  crypto "large volume far from mid" fact reachable.
- Where: new `simulations/feesim/noise.go`; exchange side already supports
  everything needed.

**R5. Stochastic fundamental process + noisy per-agent observation.**
- Mechanism: replace `ValueTraderConfig.Fundamental int64` with a
  `FundamentalProvider` interface (injected, per CLAUDE.md library rules) —
  default an OU / mean-reverting jump process; each value agent observes
  `F_t + N(0, ν²)` with its own ν. Optionally a provider that replays a
  historical series (Get Real found this gave the largest realism gain).
- Unlocks: genuine **information asymmetry and adverse selection**, so MM spread
  becomes economically meaningful; **volume–volatility correlation**;
  news-driven regime shifts; a non-arbitrary answer to fee/rebate experiments.
- Where: `simulations/feesim/value.go` interface + implementations outside the
  library.

**Below the cut** (worth doing, lower leverage):

- **R6. Latency realism**: pairwise latency matrix + per-agent computation delay
  + per-message noise in `simulation/latency.go`. Unlocks latency-competition
  studies and quote fading; also lets MMs come off the zero-latency mount.
- **R7. Heterogeneous MM policies + explicit queue-position awareness.** Two MMs
  with the *same* policy is not competition. Give MMs distinct risk aversion /
  spread / skew and let them observe their own queue rank. Unlocks realistic
  spread dynamics and quote lifetimes; prerequisite for tick-size regime work.
- **R8. Tick-size regime experiments**: sweep tick size, measure Dayri–Rosenbaum
  η (continuation/alternation ratio) and implicit spread. Converts the known
  "tick-floor collapse spiral" from a bug into a modeled regime.
- **R9. Agent entry/exit (survivorship)**: retire agents below a PnL threshold,
  clone successful ones. Unlocks long-horizon market-quality claims.
- **R10. LOB-Bench-style scoring**: replace point statistics with distributional
  distances against a reference (real BTC perp L2 data if obtainable), and record
  reference intervals so metrics have pass/fail semantics rather than vibes.
- **R11. Mild 24/7 seasonality**: a slow intensity multiplier, *not* the equities
  U-shape — crypto intraday patterns are empirically weak but non-zero.

**Explicitly not recommended**: adopting deep generative LOB models (TRADES /
diffusion / LOBGAN). They need message-level real data we lack, they are not
interactive, and the interactive multi-agent property is the entire reason this
simulator exists. Borrow their *evaluation* (LOB-Bench), not their generators.

---

## Sources

- ABIDES: <https://arxiv.org/abs/1904.12066> ·
  <https://github.com/jpmorganchase/abides-jpmc-public> ·
  ABIDES-Gym <https://arxiv.org/abs/2110.14771>
- Get Real (realism metrics): <https://arxiv.org/abs/1912.04941>
- Cont 2001 stylized facts:
  <https://www.stat.rice.edu/~dobelman/courses/texts/stylized.cont.2001.pdf>
- Queue-reactive model: <https://arxiv.org/pdf/1312.0563>
- Queue-reactive Hawkes: <https://arxiv.org/pdf/1901.08938> ·
  <https://www.worldscientific.com/doi/10.1142/S2382626620500136>
- QR order sizes: <https://arxiv.org/pdf/2405.18594>
- Deep queue-reactive (MDQR): <https://arxiv.org/abs/2501.08822>
- LOB simulation review: <https://arxiv.org/html/2402.17359v1>
- LOB-Bench: <https://arxiv.org/abs/2502.09172> · <https://lobbench.github.io/>
- Square-root law counterfactual dissection: <https://arxiv.org/html/2607.04280>
- Square-root law empirics: <https://arxiv.org/pdf/1412.4503> ·
  <https://arxiv.org/pdf/2606.24019> · <https://arxiv.org/abs/2506.07711>
- Long memory of order flow: <https://arxiv.org/pdf/1012.0349> ·
  <https://arxiv.org/pdf/1504.04354> ·
  <https://link.aps.org/doi/10.1103/PhysRevLett.131.197401>
- Self/cross impact estimation: <https://arxiv.org/html/2510.06879>
- Econophysics empirical facts (lifetime/placement exponents):
  <https://arxiv.org/pdf/0909.1974> · <https://arxiv.org/pdf/cond-mat/0102518>
- Tick size / large-tick assets: <https://arxiv.org/abs/1207.6325> ·
  <https://ar5iv.labs.arxiv.org/html/1507.07052>
- Bitcoin LOB stylized facts: <https://doi.org/10.3390/jrfm12010025>
- Bitcoin perp futures microstructure:
  <https://www.sciencedirect.com/science/article/pii/S2214845025001188>
- Adversarial calibration (MAS-GAN): <https://arxiv.org/abs/2108.00664>
- Bayesian-optimization calibration: <https://arxiv.org/pdf/2112.03874>
- Simulation-based inference calibration: <https://arxiv.org/html/2311.11913>
- Generative LOB: <https://arxiv.org/abs/2509.05107> ·
  <https://arxiv.org/pdf/2508.08698> · <https://arxiv.org/html/2602.03776>
