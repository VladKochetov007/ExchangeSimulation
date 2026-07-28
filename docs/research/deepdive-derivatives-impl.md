# Deep dive: derivatives implementation details

Second research pass, July 2026. Companion to
[`deepdive-derivatives-mechanics.md`](deepdive-derivatives-mechanics.md), which
covered mark price construction, the funding formula at spec level, the
liquidation waterfall's four stages, Deribit/OKX portfolio margin, and dated
futures basis. This document deliberately does **not** repeat those. It covers
the angles that pass left thin, at the level of detail needed to actually write
the code:

1. Inverse (coin-margined) contracts — the whole contract class is absent from pass one.
2. IV surface construction — pass one said "marks come from an IV surface" and stopped.
3. Funding: sampling frequency, the depth-walk algorithm, exact clamp, venue variants.
4. Liquidation: exact sequencing, bankruptcy price algebra, insurance fund flows.
5. Settlement/expiry: window mechanics and manipulation defences.

Repo claims are cited to `file:line` in this tree and were verified by reading
the source, not by grep alone.

---

## 1. Inverse (coin-margined) contracts

### The payoff

- **Linear (USDT-margined)**: `PnL = contracts × multiplier × (Exit − Entry)`,
  margin and PnL in the quote asset.
- **Inverse (coin-margined)**: `PnL = contracts × multiplier × (1/Entry − 1/Exit)`,
  margin and PnL in the **base** asset. Contract size is a fixed *quote* amount
  (BitMEX XBTUSD is $1/contract; Binance COIN-M BTC contracts are $100, most
  altcoin contracts $10).
  Sources: [BitMEX inverse perpetuals guide](https://www.bitmex.com/app/inversePerpetualsGuide),
  [linear/inverse/quanto walkthrough with worked numbers](https://romanornr.medium.com/bitmex-quanto-linear-inverse-futures-contracts-0c553280324c),
  [Binance COIN-M contract specifications](https://www.binance.com/en/support/faq/binance-coin-margined-futures-contract-specifications-a4470430e3164c13932be8967961aede).
- Worked example from the BitMEX guide: 100 contracts, 0.01 multiplier, entry
  15.789, exit 24.292 → `100 × 0.01 × (1/15.789 − 1/24.292) = 0.0222 XBT`.

**Note the sign convention.** The formula is `1/Entry − 1/Exit`, not
`1/Exit − 1/Entry`. A long profits when price rises because `1/Exit` shrinks.
Getting this backwards is the classic first bug in an inverse implementation.

### Why it is non-linear in quote terms

- PnL is linear in `1/P`, therefore **convex in `P`**. For a *short* inverse
  position measured in the base asset, the loss as price rises is bounded
  (`1/Entry − 1/∞ → 1/Entry`), while for a *long*, the loss as price falls is
  unbounded in base terms because `1/Exit → ∞`. The Medium walkthrough puts it
  plainly: "as you divide by a smaller and smaller number (the price of Bitcoin,
  which is getting lower), the results get larger and larger."
- The practical consequence: **an inverse long is short convexity in its own
  collateral**. The collateral is BTC, and BTC is falling exactly when the long
  is losing, so margin value and margin requirement move against the trader
  simultaneously. This double exposure has no analogue in a linear contract and
  is the single most important dynamic an inverse implementation must reproduce.
- A short inverse position (short BTCUSD, margined in BTC) is the natural
  "hodler hedge": it is roughly delta-flat in USD terms and has positive
  convexity in base terms.
- **Quanto** is the third case and behaves differently again: underlying priced
  in one currency, settled in another at a fixed rate (BitMEX ETHUSD used a
  0.00001 XBT multiplier), producing accelerating PnL in the settlement asset.
  Out of scope for us but worth naming so it is not conflated with inverse.

### Margin and liquidation math

- **Position value is in the base asset**: Bybit states position value =
  `position size / mark price` for inverse contracts, with margin "calculated
  and settled in the base coin / underlying cryptocurrency (for example BTC in
  BTCUSD contracts)".
  Sources: [Bybit initial margin, inverse](https://www.bybit.com/en/help-center/article/Initial-Margin-Inverse-Contract),
  [Bybit maintenance margin, inverse](https://www.bybit.com/en/help-center/article/Maintenance-Margin-Inverse-Contract).
- Binance's COIN-M margin formula:
  `Margin = Opening Quantity × Futures Size / (Leverage × Average Entry Price)`
  — note the division by price, which is the inverse signature.
- **Tiers still exist, and they are steeper.** Binance's COIN-M example: a
  BTCUSD position of 300 BTC carries `MMR = 12.5%` with a maintenance amount of
  `11.605 BTC`.
  Source: [How to calculate liquidation price of COIN-M futures](https://www.binance.com/en/support/faq/ceccfcfb4e3a45e3b48b0b1bb1a8ae46).
  The variable set is the same shape as the linear one (`WB` wallet balance,
  `MMR`, `cum` maintenance amount, `CM` contract size, `TMM1`/`UPNL1` for other
  contracts) — Binance publishes the formula as an image, so it is not
  transcribable verbatim from the page, but the variable definitions are text.
- **Why the liquidation math genuinely differs, not just cosmetically**:
  - Bybit is explicit that because position value is `size / mark price` and the
    mark moves, "the position value will also change accordingly. As a result,
    your risk limit tier adjusts in real time, which in turn affects the
    required maintenance margin rate (MMR)." In a linear contract the notional
    is `size × mark` and the tier also moves, but in the *same* direction as
    the PnL. In an inverse contract a **falling** price **raises** the base-denominated
    position value, so a losing long is pushed into a **higher** MMR tier at the
    same moment its collateral is depreciating. Three effects compound instead of two.
  - Equity, requirement, and PnL are all in the base asset, so the account-level
    solvency test is `baseEquity ≥ Σ maintenance(base)` — a different currency
    from the one the book quotes in.
- **Deribit's inverse options changed materially on 1 August 2026**: inverse
  options now "physically settle into a futures contract, which itself then
  settles into cash upon expiry", with the future entered at the option's strike
  and `position_size_future = position_size_option × strike`. The old direct-cash
  payoffs were `size × (index − strike)/index` for calls and
  `size × (strike − index)/index` for puts; the new path routes through
  `size × (1/entry − 1/exit)`.
  Source: [Change to inverse option delivery process](https://insights.deribit.com/exchange-updates/change-to-inverse-option-delivery-process/).
  The `/index` divisor in the old formulas is the inverse-payoff fingerprint:
  the payoff is quoted in base units, so the quote-denominated intrinsic is
  divided by the settlement price.

### Would our engine misprice these

**Yes, and not by a tunable amount — structurally.** Two hardcoded places:

- `exchange/funding.go:339` — `realizedPerpPnL` returns
  `sign × MulDiv(closedQty, tradePrice − oldEntryPrice, basePrecision)`.
  This is `Δprice × qty`, the linear payoff, and the comment above it states the
  result is in quote precision. An inverse contract needs
  `multiplier × contracts × (1/entry − 1/exit)` credited in **base** units.
- `exchange/funding.go:283-291` — `settleFunding` computes
  `positionValue = MulDiv(abs(pos.Size), price, precision)` and debits/credits
  `client.PerpBalances[quote]`. Inverse funding accrues on a base-denominated
  notional and settles in base.

Everything downstream inherits the assumption: `buildAccountMarginProfile`
(`exchange/exchange.go:1016`) aggregates equity and requirement per **quote
asset**, and `PerpFutures.MarginRequired` (`instrument/perp.go:48`) is
`qty × price × rate / 10000`, again quote-denominated.

**What interfaces inverse contracts would need.** The good news is that the
existing seams are close. The gap is that no interface currently lets an
instrument declare *which asset* its margin and PnL are denominated in — the
exchange assumes `QuoteAsset()` throughout. Minimum additions:

- A `SettlementAsset() string` method (or a `Settled` interface) so the exchange
  stops assuming quote. This is the load-bearing change; without it nothing else
  helps.
- A `PnLCalculator`-shaped interface owning the entry/exit → cash-delta map, so
  `realizedPerpPnL` becomes a default implementation rather than a package
  function. Signature must return the asset alongside the amount.
- `Margined` (`types/interfaces.go:139`) already has the right *shape*
  (`MarginRequired(qty, price, precision)`) — an inverse instrument can
  implement it as `qty × multiplier / price × rate`, returning base units,
  provided the caller knows which asset to debit.
- Per-asset margin profiles in `buildAccountMarginProfile` — the aggregation key
  becomes the settlement asset rather than the quote asset. Cross-margin between
  a BTC-settled inverse and a USDT-settled linear then correctly does *not* net,
  matching Deribit PME's rule that BTC and ETH margin never offset.

---

## 2. IV surface construction for crypto options

### What Deribit actually publishes, and what it does not

- Deribit's API exposes `mark_iv`, `bid_iv`, `ask_iv` per option instrument, and
  the UI displays IV next to each side of the book (a worked screenshot in
  Deribit's own primer shows bid 0.0365 BTC ≈ 48% IV, ask 0.037 BTC ≈ 48.5% IV).
  Source: [Deribit: introduction to options pricing and IV](https://cryptarbitrage.medium.com/introduction-to-options-pricing-and-implied-volatility-iv-a232d70d8fd2).
- **Honest caveat: Deribit does not publish the exact mark-IV algorithm.** The
  [Mark Prices support article](https://support.deribit.com/hc/en-us/articles/25944746962973-Mark-Prices)
  is the canonical page but blocks automated fetching, and neither the primer nor
  the [PME model deck](https://statics.deribit.com/files/DeribitPortfolioMarginModel.pdf)
  spells out the bid-IV/ask-IV weighting or the outlier rule. Any claim of the
  form "Deribit weights mark IV as X% bid + Y% ask" that you find on the internet
  is reverse-engineered, not documented. **Do not encode a specific weighting as
  if it were the venue spec.** What *is* documented and safe to rely on:
  - The underlying for option IV is the **forward for that expiry**, not the
    index (PME deck, and pass one's §1 already cites this).
  - Marks derive from a **bounded mid**, not from last trade: for futures
    Deribit computes VWAP-based bid/ask from the book and "locally bounds" the
    mid. The same bounding philosophy applies to options.
  - Deribit builds **synthetic forwards from put-call parity** at the strike with
    the tightest implied bid-ask spread, and injects those as virtual liquidity
    into the futures book. This matters for us: the forward is inferred from the
    *options* market, not only from the futures book.
  - IV is solved numerically (Newton-Raphson) — there is no closed form.

### SVI: the parameterization to use

The reference is Gatheral & Jacquier,
[*Arbitrage-free SVI volatility surfaces*](https://arxiv.org/pdf/1204.0646)
(arXiv:1204.0646, Quantitative Finance 2014). Everything below is from that paper.

**Raw SVI**, in total implied variance `w = σ²t` against log-forward moneyness `k`:

```
w(k) = a + b·{ ρ(k − m) + √((k − m)² + σ²) }
```

- Parameters `{a, b, ρ, m, σ}` with `b ≥ 0`, `|ρ| < 1`, `σ > 0`,
  and `a + bσ√(1 − ρ²) ≥ 0`.
- Interpretation, verbatim: "`a` increases the general level of variance; `b`
  tightens the smile; `ρ` rotates the smile; `m` translates it; `σ` reduces ATM
  curvature."
- **`ρ` is the one that matters for crypto**: it controls skew direction, and
  BTC's skew flips sign across regimes (forward skew in rallies, put skew in
  crashes) in a way equity indices never do.

**SVI-JW** (jump-wings) reparameterizes to quantities a trader can set directly —
`v_t` ATM variance, `ψ_t` ATM skew, `p_t` put-wing slope, `c_t` call-wing slope,
`ṽ_t` minimum variance:

```
v_t = [a + b(−ρm + √(m² + σ²))] / t
ψ_t = (1/√w_t)·(b/2)·(−m/√(m² + σ²) + ρ)
p_t = (1/√w_t)·b(1 − ρ)
c_t = (1/√w_t)·b(1 + ρ)
ṽ_t = [a + bσ√(1 − ρ²)] / t
```

For a **simulation** this is the right entry point: you want to drive the
surface by `(ATM vol, skew, wing slopes)` because those are the knobs an
experiment varies, and then map to raw SVI for evaluation.

**SSVI** (Surface SVI) — one global surface, `θ_t` = ATM total variance:

```
w(k, θ_t) = (θ_t/2)·{ 1 + ρφ(θ_t)k + √((φ(θ_t)k + ρ)² + (1 − ρ²)) }
```

with a power-law `φ(θ) = η·θ^(−γ)`, `η > 0`, `0 < γ < 1` (their Example 4.2).
**This is the minimal viable surface**: three parameters (`ρ, η, γ`) plus an ATM
term structure `θ_t`, and it is arbitrage-free by construction under conditions
below.

**No-arbitrage conditions** (these are the reason to use SVI rather than
interpolating a grid):

- *Calendar spread*, Lemma 2.1: `∂_t w(k,t) ≥ 0` for all `k`, `t > 0` — total
  variance must be non-decreasing in maturity. For SSVI (Theorem 4.1) this
  becomes `∂_t θ_t ≥ 0` and
  `0 ≤ ∂_θ(θφ(θ)) ≤ [(1 + √(1 − ρ²))/ρ²]·φ(θ)`.
- *Butterfly*, Lemma 2.2 (the Durrleman condition): define
  ```
  g(k) = [1 − k·w'(k)/(2w(k))]² − (w'(k)²/4)·(1/w(k) + 1/4) + w''(k)/2
  ```
  A slice is butterfly-arbitrage-free iff `g(k) ≥ 0` for all `k` and
  `lim_{k→+∞} d₊(k) = −∞`. For SSVI (Theorem 4.2) two sufficient conditions
  suffice: `θφ(θ)(1 + |ρ|) < 4` and `θφ(θ)²(1 + |ρ|) ≤ 4`.

Butterfly arbitrage is not an academic nicety here: a surface that violates it
implies negative risk-neutral density, which means a short-options book can be
constructed with a guaranteed profit against our own mark prices. In a sim with
optimizing agents, that is an exploit, and it will be found.

**SVI vs SABR for crypto**: SVI interpolates *total variance*, SABR interpolates
*implied volatility* directly. Practitioner consensus is SVI for equity/crypto,
SABR for rates; SVI's parametric form is simpler and no-arbitrage conditions are
characterized in closed form, which SABR lacks. A comparative calibration study
is [Multi-day IV surface calibration: SVI vs SABR](https://repositori.upf.edu/items/eceeb187-f169-483e-bf67-416fd9e00d70?locale=en);
a crypto-specific real-time calibration treatment is Magadini, Sinclair & Chepal,
[*Techniques for creating consistent, stable and robust real time implied
volatility calibrations in the nascent cryptocurrency markets*](https://papers.ssrn.com/sol3/papers.cfm?abstract_id=4899357)
(SSRN 4899357; abstract page is fetch-blocked, cited by title/authors).

### Sticky strike vs sticky delta

- **Sticky strike**: IV at a fixed strike is unchanged when spot moves; the
  surface is pinned to absolute strikes. Characteristic of equity index markets.
- **Sticky delta** (a.k.a. sticky moneyness): the whole smile translates with
  spot, so IV at a fixed *moneyness* is unchanged. Characteristic of FX and
  rates.
- The distinction is not cosmetic — it changes the **delta**. Under sticky
  delta, the smile moving with spot adds a `∂σ/∂S` term to the hedge ratio, so
  the correct delta differs from the Black-Scholes delta by `vega × ∂σ/∂S`.
- Derman's three regimes (sticky strike ≈ trending markets, sticky moneyness ≈
  range-bound) are the standard framing.
  Overview: [Hull, Daglish & Suo, *Volatility surfaces: theory, rules of thumb,
  and empirical evidence*](https://www-2.rotman.utoronto.ca/~hull/downloadablepublications/DaglishHullSuoRevised.pdf).
- **Crypto-specific evidence**: Alexander & Imeraj,
  [*Delta hedging bitcoin options with a smile*](https://www.tandfonline.com/doi/full/10.1080/14697688.2023.2181205)
  (Quantitative Finance 23(5), 2023; [SSRN 4097909](https://papers.ssrn.com/sol3/papers.cfm?abstract_id=4097909))
  tests smile-implied deltas on hourly Deribit data and finds "bitcoin implied
  volatility curves behave very differently from those of equity index options."
  Efficiency gains from smile-adjusted deltas exceed **30% for OTM puts** when
  hedging with the perpetual swap, averaging **15% for short-term OTM calls**
  during upward-sloping-IV periods. Underlying is the Deribit BTC index, an
  equally-weighted average across 11 exchanges updated every second.
- **Implication for us**: BTC's smile is regime-dependent and often *forward*-skewed
  (the [Financial Innovation study of Deribit BTC options](https://link.springer.com/article/10.1186/s40854-021-00280-y)
  finds BTC "belongs to the commodity class of assets based on the presence of a
  volatility forward skew"). A hardcoded equity-style put skew would be wrong for
  BTC. Make the skew sign a parameter.

### Term structure

- Interpolate in **total variance** `w = σ²t` linearly in `t`, never in `σ`
  directly — linear-in-σ interpolation violates the calendar-spread condition
  `∂_t w ≥ 0` and creates arbitrage between adjacent expiries.
- Empirically the smile is deepest at short tenors and flattens monotonically
  with maturity; the SSVI power law `φ(θ) = ηθ^(−γ)` reproduces exactly this
  flattening with one exponent.
- Deribit's own tenor scaling in the PME margin model uses
  `(30 / days_to_expiry)^VegaPower` with `VegaPower = 0.3` under 30 days and
  `0.13` beyond — an independent confirmation that short tenors need materially
  more vol-shock headroom (already cited in pass one §4).

### Minimal viable surface for our simulation

Ranked by work, replacing the flat `IV = 0.8` at `instrument/option.go:32-33,62`:

1. **SSVI with power-law φ** — parameters `(ρ, η, γ)` plus an ATM term structure
   `θ_t`. Arbitrage-free by construction if the two Theorem 4.2 inequalities are
   checked at construction time. Three numbers give skew, smile curvature, and
   term-structure flattening. This is the recommendation.
2. **Dynamics**: drive `θ_t` (ATM level) by a mean-reverting process with a
   **negative correlation to spot returns** — that single coupling produces the
   leverage effect, gives short-vol books real vega risk on a crash, and is what
   makes Deribit's "vol up 45%" stress scenario bite in the sim.
3. **Sticky rule as a switch**: sticky-strike vs sticky-delta as a configuration
   choice, because it changes hedge ratios and therefore the behaviour of any
   delta-hedging actor.
4. Below the line: full per-slice raw-SVI calibration to traded prices. Only
   worth it if you want the sim's *own* option flow to feed back into the
   surface, which is a genuinely different (and interesting) experiment.

---

## 3. Funding: implementation-grade detail

Pass one gave the Binance formula. Here are the numbers you need to write the code.

### Sampling frequency — the number pass one omitted

- **Binance samples the premium index every 5 seconds**, i.e. 12 samples/minute.
  For an 8-hour interval, `n = (60/5) × 60 × 8 = 5,760` data points.
  Source: [Introduction to Binance Futures funding rates](https://www.binance.com/en/support/faq/introduction-to-binance-futures-funding-rates-360033525031).
- **OKX samples every minute**, so `n = 480` for an 8-hour interval — the funding
  rate at 07:59 uses the premium index for every minute from 00:00 to 07:59.
  Source: [OKX perpetual funding fee mechanism](https://www.okx.com/en-us/help/perps-funding-fee-mechanism).
- A 12× difference in sample count between the two largest venues. For a
  simulation this is the knob that controls how manipulable the rate is, so it
  must be configurable, not baked in.

### Premium index and the impact depth-walk

Both venues use the identical premium formula:

```
P = [ max(0, ImpactBid − Index) − max(0, Index − ImpactAsk) ] / Index
```

- `P > 0` only when the impact **bid** is above the index (perp trading rich);
  `P < 0` only when the impact **ask** is below it. Inside the band the premium
  is exactly zero — a deliberate dead zone.
- **Impact bid/ask are average fill prices for a fixed notional**, obtained by
  walking the book:
  - Binance: `IMN = 200 USDT / (initial margin rate at max leverage)`. For a
    20× symbol that is 4,000 USDT. COIN-M uses "200 USD worth of margin" instead.
  - OKX: `Impact value = 200 × max leverage allowed for this perpetual` —
    algebraically the same quantity, since max leverage is `1/IMR`.
- **The walk algorithm**, explicitly:
  ```
  walkImpactPrice(levels, targetNotional):
      remaining = targetNotional
      cost = 0; filled = 0
      for level in levels (best → worst):
          levelNotional = level.price × level.qty
          take = min(remaining, levelNotional)
          cost += take
          filled += take / level.price
          remaining -= take
          if remaining == 0: break
      if remaining > 0: return no impact price (insufficient depth)
      return cost / filled          // notional-weighted average fill price
  ```
  Note it is the **notional-weighted** average (cost divided by base quantity
  filled), not a quantity-weighted average of level prices. Insufficient depth
  must be an explicit outcome — silently returning the worst level would let a
  thin book fake an extreme premium, which is the exact failure the mechanism
  exists to prevent.

### Time-weighted average with linear ramp

```
P̄ = (1·P₁ + 2·P₂ + … + n·Pₙ) / (1 + 2 + … + n)
```

Identical on Binance and OKX. Weights increase linearly with recency, so the
denominator is `n(n+1)/2`. Recent samples dominate, but no single sample can
dominate: the last sample's weight is `n / (n(n+1)/2) = 2/(n+1)`, which for
Binance's `n = 5760` is **0.035%**. That bound is the anti-manipulation property
— buy the book at the settlement instant and you move the rate by three
hundredths of a percent of the move.

### The clamp, exactly

Binance:
```
F = P̄ + clamp(InterestRate − P̄, −0.05%, +0.05%)
```
then scaled by interval: divide by `(8/N)` for an `N`-hour interval.

OKX states it with an outer cap as well:
```
F = clamp( [ P̄ + clamp(InterestRate − P̄, −0.05%, +0.05%) ] / (8/N),
           FundingRateCap, FundingRateFloor )
```
with `InterestRate = 0.01%` fixed across settlement intervals (equivalently
`0.03%/(24/N)`).
Source: [OKX revision of the funding rate formula](https://www.okx.com/en-us/help/important-update-revision-of-the-funding-rate-formula-for-okx-perpetual).

**Read the clamp carefully — it is not a cap on the rate.** It is a cap on the
*deviation of the interest term*. Unpacking:

- If `|InterestRate − P̄| ≤ 0.05%`, the inner clamp is transparent and
  `F = P̄ + (I − P̄) = I` **exactly the interest rate**. Binance's own wording:
  "if the interest rate minus premium index falls between ±0.05%, the funding
  rate equals the interest rate."
- Only when the premium escapes that band does `F` track the premium, offset by
  ±0.05%.
- With `I = 0.01%` per 8h, the dead zone is `P̄ ∈ [−0.04%, +0.06%]`. Inside it,
  funding is pinned at 1 bp regardless of basis.

Binance's default interest rate is **0.03% daily = 0.01% per 8-hour interval**,
and under extreme volatility intervals may shorten to hourly when rates hit the
cap/floor.

### Consequences for our `SimpleFundingCalc`

`instrument/funding.go:14-27` differs on every axis: instantaneous sample
instead of a 5,760-point ramp average; mid-vs-index instead of impact prices;
`BaseRate: 10` bps (`instrument/perp.go:37`) against Binance's 1 bp; and a
symmetric cap on the total rather than a clamp on the interest deviation. The
10 bps base is the one that most distorts dynamics — it is 10× the real
interest anchor, so it dominates the premium signal at typical basis levels.

---

## 4. Liquidation engine implementation patterns

### The sequence, in order

BitMEX is the most explicit venue about ordering, and the ordering is what a
simulator must reproduce:

1. **Cancel open orders in the same contract first.** BitMEX's Phase 2 cancels
   all of the user's open orders in that contract to release their reserved
   margin. **If the released margin restores the position above the liquidation
   threshold, liquidation stops and the position reverts to user control.**
   Source: [BitMEX: why did I get margin call/liquidated](https://support.bitmex.com/hc/en-gb/articles/15863777636637-Why-Did-I-Get-Margin-Call-Liquidated).
   This is a free realism win: it is pure bookkeeping, requires no market
   impact, and it materially reduces the number of liquidations that reach the
   book at all.
2. **Reduce the position** — Binance sends one large IOC and stops once the
   remaining assets cover the requirement (pass one §3 covers this).
3. **Insurance fund** absorbs the bankrupt remainder.
4. **ADL** when the fund cannot.

### Bankruptcy price vs liquidation price

- **Bankruptcy price**: "the price at which a position has zero equity left
  (**all** posted margin has been depleted)."
- **Liquidation price**: the price at which "the difference between the
  unrealised loss and the margin posted is equal to the Maintenance Margin" —
  strictly between entry and bankruptcy.
- BitMEX's worked example: long XBTUSD at 6,000 at 100× (1% IM) → bankruptcy at
  **5,940** (1% below entry); with 0.50% maintenance margin the liquidation
  price is **5,970**.
  Source: [BitMEX insurance fund: your questions answered](https://www.bitmex.com/blog/bitmex-insurance-fund-your-questions-answered).
- Binance's example uses the same structure: long at $8,000, liquidation
  $7,700, bankruptcy $7,600 — "a $100 buffer representing potential slippage."
  Source: [Binance: liquidation & insurance funds, part 2](https://www.binance.com/en/blog/futures/liquidation--insurance-funds-how-they-work-and-why-they-are-important-to-cryptoderivatives-part-2-421499824684900373).
- BitMEX notes maintenance margin is not a bare 0.50%: it is
  **0.50% + exit taker fee + funding rate** — the buffer must pay for its own
  exit. Ours has no such term.

**For linear contracts**, from `equity(P_bk) = 0`:

```
Long:   P_bk = Entry − (Margin / Size)
Short:  P_bk = Entry + (Margin / Size)
```

and with tiered maintenance margin, the liquidation price solves
`Equity(P) = Notional(P)×MMR − cum`:

```
Long:   P_liq = (Entry·Size − WB − cum) / (Size·(1 − MMR))
Short:  P_liq = (Entry·Size + WB + cum) / (Size·(1 + MMR))
```

For **inverse**, the same identities in `1/P` — the `1/(1 ∓ MMR)` factor
attaches to the reciprocal, which is why the inverse liquidation price is not a
symmetric distance from entry.

### Insurance fund flows — both directions

The critical structural point, and the one our engine gets wrong:

- **The fund is credited on good liquidations.** BitMEX: when the engine closes
  liquidated positions "at prices better than the average entry price, that
  profit is **added** to the Insurance Fund, providing funds for future
  liquidations to draw against." Binance: the engine "places an immediate order
  to sell above [the bankruptcy price]"; filling at 7,650 against a 7,600
  bankruptcy price sends the surplus to the fund.
- **Plus an explicit fee.** Binance charges a **liquidation clearance fee**:
  ```
  Insurance Clearance Fee = Position Nominal Value × Liquidation Fee Rate
  ```
  with a **0.3%** rate cited in their own example; the fee is transferred to the
  insurance fund.
  Source: [Binance Futures on the clearance fee](https://x.com/BinanceFutures/status/1867762247378088224),
  corroborated by the part-2 blog above.
- **The fund is debited only when the fill is worse than bankruptcy** — that is
  the deficit case.
- BitMEX prices the liquidation order *using* the fund: "the trading engine uses
  the available insurance funds to submit an order with a more aggressive limit
  price." The fund is an active budget for aggression, not just a passive
  backstop.
- **ADL triggers** when the engine "reaches its own Bankruptcy Price and can no
  longer trade" (BitMEX) or when the fund is depleted (Binance).
- **ADL ranking**: `Score = PnL% × EffectiveLeverage`, where `PnL% =
  unrealizedPnL / notional` and `EffectiveLeverage = notional / marginBalance`.
  BitMEX's own blog describes the ranking as "by profit and leverage in that
  contract" without publishing the closed form; the formula above is the
  widely-reproduced version and matches pass one's citation of the BitMEX
  effective-leverage definition. Treat the exact form as reconstructed, and note
  pass one already recommends the theoretically-optimal alternative (Campbell et
  al. 2026 minimax-leverage water-filling).

Our fund at `exchange/exchange.go:1235` has exactly one write and it is a
decrement. Adding the two credit paths (clearance fee, surplus-over-bankruptcy)
turns a monotonically-negative counter into a real balance sheet that can be
sized, stressed, and exhausted.

---

## 5. Settlement and expiry

### Deribit's window, with the numbers

- Expiry is 08:00 UTC; the delivery price is a **30-minute TWAP of the index**
  over 07:30–08:00.
- The **index is computed every 4 seconds**, so the TWAP averages
  **450 index observations** (`30 × 60 / 4 = 450`).
  Source: [Deribit settlement](https://support.deribit.com/hc/en-us/articles/29734325712413-Settlement).
- The index itself is a multi-exchange basket. For BTC it is an equally-weighted
  average across **11 exchanges, updated every second** (per the Alexander &
  Imeraj description of the Deribit BTC index).

### Manipulation defences, layered

1. **Length**: a 30-minute window means a manipulator must hold the price away
   from fair value for the full window, not spike it at an instant. Cost scales
   with duration.
2. **Breadth**: the index takes the **median** across constituents and "major
   indices always have at least three sources", so moving it requires
   simultaneous pressure on at least two major venues.
3. **Circuit breakers**: if the index moves **more than 10% between index ticks
   (1 second)**, the related derivatives are **halted**.
4. **Delta decay**: over the final 30 minutes, the delta of expiring options and
   futures **decays linearly to zero**, and the PME risk-matrix underlying
   converges to the Estimated Delivery Price. So as the settlement window opens,
   the position stops responding to spot for margin purposes — which removes the
   incentive to manipulate spot in order to trigger margin effects on others.
   Sources: settlement article above and [Deribit index prices](https://support.deribit.com/hc/en-us/articles/25944739377309-Index-Prices).
5. **Delivery fee cap**: 0.015% of the underlying capped at 12.5% of option
   value, so worthless options settle free (pass one §5; we already implement
   this at `instrument/option.go:146`).

The four defences are independent and multiplicative. A simulation that
implements only the TWAP gets defence 1 and none of the rest.

**Where we stand**: `instrument/settlement_obs.go` implements a mean over a
**60-second** window (default set at `instrument/option.go:65`), frozen on first
read. Correct shape, 1/30th the length, no median-of-sources, no circuit
breaker, no delta decay. The window length is a one-line config change; the
other three are real work.

---

## Implementation checklist for this repo

Every interface named below was verified present by reading the source.

| Interface | Location | Signature (verbatim) |
|---|---|---|
| `FundingCalculator` | `instrument/funding.go:4` | `Calculate(indexPrice, markPrice int64) int64` |
| `OrderMarginer` | `types/interfaces.go:149` | `MarginForOrder(side Side, qty, price, precision int64) int64`; `MarginForMarketOrder(side Side, qty, refPrice, precision int64) int64` |
| `PositionMarginer` | `types/interfaces.go:159` | `PositionMark() int64`; `MaintenanceForPosition(size, precision int64) int64` |
| `LiquidationHandler` | `exchange/exchange.go:21` | `OnMarginCall(*MarginCallEvent)`; `OnLiquidation(*LiquidationEvent)`; `OnInsuranceFund(*InsuranceFundEvent)` |
| `Margined` | `types/interfaces.go:139` | `MarginRequired(qty, price, precision int64) int64`; `MarginForMarket(...)`; `MarginOnCancel(...)` |
| `Expirable` | `types/interfaces.go:171` | `ExpiryNano() int64`; `ObserveSettlement(price, tsNano int64)`; `SettlementPrice() int64`; `ExpiryCashFlow(size, entryPrice, settlementPrice, basePrecision int64) int64`; `DeliveryFee(size, settlementPrice, basePrecision int64) int64` |
| `MarkPriceCalculator` | `price/price.go:5` | `Calculate(book *ebook.OrderBook) int64` |
| `PriceSource` | `types/interfaces.go:21` | `Price(symbol string) int64` |
| `Instrument` | `types/interfaces.go:81` | `Symbol/BaseAsset/QuoteAsset/BasePrecision/QuotePrecision/TickSize/MinOrderSize/ValidatePrice/ValidateQty/IsPerp/InstrumentType` |

### A. Funding — `BinanceFundingCalc` (highest ratio of realism to work)

- **Blocker**: `FundingCalculator.Calculate(indexPrice, markPrice int64) int64`
  (`instrument/funding.go:4`) takes two scalars. It **cannot** express a
  5,760-sample ramp average of impact prices. The interface must gain a
  sampling-aware variant — e.g. a separate `PremiumIndexSampler` that the price
  loop drives, whose accumulated state the calculator reads. Do not widen
  `Calculate`; add a second interface and let `SimpleFundingCalc` keep
  implementing the existing one.
- **Depth walk is feasible today**: `book.OrderBook` exposes `Bids`/`Asks` as
  `*book.Book` (`book/orderbook.go:6-13`), and `Book.GetSnapshot() []PriceLevel`
  (`book/book.go:263`) returns the levels. The `walkImpactPrice` pseudocode in §3
  drops straight onto that. Note `OrderBook` itself exposes only
  `GetBestBid`/`GetBestAsk`/`GetMidPrice` (`book/orderbook.go:22`, `:29`, `:38`)
  — the depth walk must go through the `Bids`/`Asks` fields.
- Config surface, all defaulted, none hardcoded: sample interval (Binance 5s /
  OKX 60s), impact margin notional, interest rate (1 bp per 8h), inner clamp
  (±5 bps), outer cap/floor, interval hours.
- Drop `SimpleFundingCalc`'s `BaseRate` default from 10 bps to 1 bp
  (`instrument/perp.go:37`) regardless of whether the new calculator ships.

### B. Liquidation — sequencing and fund flows

- **Cheapest realism win in this entire document**: cancel-orders-first with
  recheck, before any market impact. Today `liquidate`
  (`exchange/exchange.go:1178`) goes straight to `forceClose`
  (`exchange/liquidation.go:14`). Insert the cancel + margin-release + re-evaluate
  step; if the account now covers maintenance, abort. Pure bookkeeping, no new
  interface needed.
- **Bankruptcy price** already almost exists: `EstimateLiquidationPrice`
  (`exchange/exchange.go:1162`) computes essentially this quantity. Split it into
  `bankruptcyPrice` (equity = 0) and `liquidationPrice` (equity = maintenance),
  and add the BitMEX exit-fee term to the latter.
- **Insurance fund credits**: add the clearance fee
  (`notional × feeRate`, default 30 bps) and the surplus
  `(fillPrice − bankruptcyPrice) × size` alongside the existing debit at
  `exchange/exchange.go:1235` — currently the only write that changes the
  balance (`exchange/exchange.go:1256` merely reads it into an event).
- `LiquidationHandler` (`exchange/exchange.go:21`) needs no change — it already
  has `OnInsuranceFund(*InsuranceFundEvent)`, which becomes meaningful once the
  fund has two-directional flow. Check `InsuranceFundEvent` carries a signed
  delta and a reason, and extend that struct rather than the interface.

### C. IV surface — replacing flat `IV = 0.8`

- Current state: `EuropeanOption.IV float64` (`instrument/option.go:32-33`),
  defaulted to `0.8` at `instrument/option.go:62`, never updated.
- Introduce a `VolatilitySurface` interface — `Vol(strike, expiryNano int64) float64`
  or, better, keyed on log-moneyness and year fraction so SVI maps directly.
  Ship an `SSVISurface` default with `(ρ, η, γ, θ_t)` and assert the two
  Theorem 4.2 inequalities at construction.
- `PositionMarginer` (`types/interfaces.go:159`) is the seam that makes the
  surface *matter*, and it is **already implemented**:
  `EuropeanOption.MaintenanceForPosition` (`instrument/option.go:102-109`)
  charges shorts `MMBps × underlyingMark + markPremium`, the Deribit standard
  shape. It can return a vol-shocked requirement instead of a static one. Note
  the signature takes no vol argument, so the option instrument must hold a
  reference to the surface — which is the right ownership anyway.
- Mark against the **forward** for the expiry, not the spot mid (currently
  `exchange/expiry.go:121` passes the underlying mid to `Black76Premium`).
- Pass one's recommendation 5 (put options into the risk engine) was listed as a
  **prerequisite** for the surface to matter. See the correction below — step
  one of it is already done, so the surface work is unblocked.

#### Correction to pass one: options are no longer invisible to the risk engine

`deepdive-derivatives-mechanics.md` makes three related claims that the current
tree contradicts. Verified by reading the source, not grep:

- Pass one §4: "`OptionMarginParams.MMBps` exists at 750 bps but is **unused**."
  **Stale** — it is consumed at `instrument/option.go:107`. Only the code comment
  at `instrument/option.go:15` ("reserved for MTM sweeps") still says otherwise.
- Pass one §5: "Options are invisible to the risk engine… `marginCore`
  (`exchange/exchange.go:915-924`) returns non-nil only for `*PerpFutures`."
  **Stale** — `marginCore` no longer exists. `buildAccountMarginProfile` now
  type-asserts `PositionMarginer` at `exchange/exchange.go:1024-1025` and folds
  option exposure in via `addPositionMarginerExposure`
  (`exchange/exchange.go:1063`), which adds maintenance at
  `exchange/exchange.go:1077`.
- Pass one "elephant #5": "a trader can sell unlimited premium and only ever be
  checked against the initial reservation." **No longer true** — short option
  maintenance now enters the cross-margin profile.

**Why this matters for planning**: pass one's recommendation 5 step one is
already implemented, so the "cheap and urgent correctness hole" it describes is
closed. What remains from that recommendation is step two (portfolio margin /
risk matrix) and, more importantly for this document, the vega risk that only a
real surface provides. Re-verify before scheduling work against pass one's
options items.

**Bonus finding for inverse contracts (§1)**: the filter at
`exchange/exchange.go:1024` is `book.Instrument.QuoteAsset() == quote`. This is
the quote-asset assumption, in the risk engine, in one line — direct confirmation
that per-settlement-asset keying is the blocker for inverse support.

### D. Inverse contracts — the honest assessment

- **Not a drop-in.** `Instrument` (`types/interfaces.go:81`) has no
  settlement-asset concept, and the exchange assumes `QuoteAsset()` for every
  margin, PnL, and balance operation.
- Two hardcoded linear assumptions must become injectable first:
  `realizedPerpPnL` (`exchange/funding.go:316-339`, computes
  `Δprice × qty` in quote) and `settleFunding` (`exchange/funding.go:262-311`,
  accrues `qty × mark` and settles in `PerpBalances[quote]`).
- Then: `SettlementAsset() string` on the instrument, per-asset keying in
  `buildAccountMarginProfile` (`exchange/exchange.go:1016`), and an inverse
  implementation of `Margined.MarginRequired` returning base units.
- `OrderMarginer` (`types/interfaces.go:149`) is **not** sufficient on its own —
  its doc comment states "Reservations are taken from the perp wallet in the
  quote asset", which is precisely the assumption inverse contracts break.
- **Recommendation: defer.** Inverse contracts are a larger change than
  everything else in this document combined, and the payoff for our current
  experiments is the smallest. The reason to do it eventually is that
  inverse-long collateral convexity is a genuinely distinct cascade mechanism
  (collateral value and margin requirement move against the trader at once) that
  cannot be approximated by tuning a linear contract. Sequence it after A, B, C.

### E. Settlement window

- One-line: raise the settlement observation window from 60s
  (`instrument/option.go:65`, `instrument/settlement_obs.go`) to Deribit's 1800s.
- `Expirable.ObserveSettlement(price, tsNano int64)` (`types/interfaces.go:171`)
  already has the right signature for a longer window and for a median-of-sources
  variant — a user-side implementation can median across sources before calling in.
- Circuit breaker (halt on >10% index move between ticks) and linear delta decay
  over the final window are separate features; the delta decay only matters once
  options are in the risk engine (item C).

---

## What this document does not claim

- **Deribit's exact mark-IV weighting is not public.** The support article that
  would settle it blocks automated access. Anything specific you read about
  bid-IV/ask-IV weights or outlier thresholds is reverse-engineered.
- **BitMEX's ADL ranking closed form is not published** in the sources reachable
  here; `PnL% × EffectiveLeverage` is the widely-reproduced reconstruction and is
  consistent with BitMEX's stated effective-leverage definition.
- **Binance's COIN-M liquidation price formula is published as an image**, so it
  is transcribed here from the algebra plus their documented variable
  definitions, not quoted verbatim.
