# Deep dive: how derivatives venues actually price, margin, and liquidate

Research pass, July 2026. Every venue claim below is cited to a primary source
(exchange docs or the vendor's own model paper). Every repo claim is cited to
`file:line` in this tree. The purpose is to decide which mechanisms our
simulation should adopt and which simplifications are safe.

Scope: perpetuals, dated futures, European options, cross/portfolio margin,
liquidation and its failure modes.

---

## 1. Mark price construction

### What venues do

- **Binance USDⓈ-M perpetual**: `Mark = median(Price1, Price2, Contract Price)`
  where `Price1 = Index × (1 + LastFundingRate × TimeUntilFunding/FundingPeriod)`
  and `Price2 = Index + MA(30s basis)`, the basis MA being
  `Σ[(Bid1ᵢ + Ask1ᵢ)/2 − PIᵢ] / 30` over 30 one-second samples.
  Source: [Mark Price in USDⓈ-M Futures](https://www.binance.com/en/support/faq/mark-price-in-usd%E2%93%A2-margined-futures-360033525071).
- The **median of three** is the manipulation defence: moving the mark requires
  moving two of {index, funding-adjusted index, contract price} at once.
- **Index price is a basket**, not one venue. Binance names ~15 constituents
  (Binance, KuCoin, OKX, Coinbase, Kraken, Bybit, Bitfinex, plus DEX sources)
  weighted by relative volume, with two protective rules: a single source
  deviating >3% from the median is capped at 1.03×/0.97× median (1% for some
  symbols), and a source unavailable or >5 minutes stale has its weight zeroed.
  A "Last Price Protected" mode activates when no stable index exists. Same source.
- **Deribit** takes a different route to the same place: futures mark is the
  index plus a 30-second EMA of (last trade, or best bid/ask when last is
  outside the spread) minus index. Option marks come from an IV surface, and the
  IV calculation uses **the forward for that expiry, not the index**, as the
  underlying. Sources:
  [Deribit mark prices (via search)](https://support.deribit.com/hc/en-us/articles/25944746962973-Mark-Prices),
  [Deribit PME model PDF](https://statics.deribit.com/files/DeribitPortfolioMarginModel.pdf).

### Why mark ≠ last trade for liquidation

The liquidation engine trades *into* the same book that would set the mark. If
mark is derived from that book, each forced sale lowers the mark, which triggers
the next liquidation, which lowers the mark again. Index anchoring is what breaks
the loop. Empirically the loop is not hypothetical: in the Oct 10–11 2025
cascade, top-of-book BTC depth fell >90% on major venues while liquidation
throughput went from ~$0.71B/hr baseline to ~$10.39B/hr.
Sources: [Amberdata deleveraging analysis](https://blog.amberdata.io/leverage-liquidations-the-31b-deleveraging),
[SSRN event study of the Oct 2025 cascade](https://papers.ssrn.com/sol3/Delivery.cfm/5611392.pdf?abstractid=5611392&mirid=1).

### What we have

- Five calculators in `price/calculators.go`: `LastPriceCalculator`,
  `MidPriceCalculator`, `WeightedMidPriceCalculator`, `MedianMarkPrice`
  (median of bid/ask/index, `price/calculators.go:74`), `EMAMarkPrice`,
  `ClampedEMAMarkPrice` (index + clamped EMA basis, `price/calculators.go:159`),
  and `TWAPMarkPrice`.
- **The default is `MidPriceCalculator`** — the perp's own book mid
  (`exchange/exchange.go:710-711`). Index-anchored variants are opt-in.
- Index resolution (`exchange/exchange.go:849-857`) tries the underlying spot
  book mid, then `indexProvider`, then **falls back to the mark itself**, at
  which point the basis is identically zero.
- `price/oracle.go` offers `MidPriceOracle` (one mapped book) and
  `StaticPriceOracle` (constant). No basket, no deviation cap, no staleness rule.

---

## 2. Funding

### What venues do

Binance's formula, verbatim from
[Introduction to Binance Futures Funding Rates](https://www.binance.com/en/support/faq/introduction-to-binance-futures-funding-rates-360033525031):

- `F = [Average Premium Index P + clamp(interest rate − P, −0.05%, +0.05%)] / (8/N)`
- `P = [max(0, ImpactBid − Index) − max(0, Index − ImpactAsk)] / Index`
- Impact bid/ask are **average fill prices for the Impact Margin Notional**,
  `IMN = 200 USDT / (initial margin rate at max leverage)` — e.g. 4,000 USDT for
  a 20× symbol. Using depth-consumed prices instead of top-of-book is what makes
  the premium resistant to a one-lot quote.
- Interest rate is fixed at 0.03%/day (0.01% per 8h interval) by default.
- The average premium is **time-weighted with linearly increasing weights**
  across the interval: `(1·P₁ + 2·P₂ + … + n·Pₙ) / (1+2+…+n)`.
- `Funding Payment = Position Notional × F`, notional at mark. Intervals 00:00,
  08:00, 16:00 UTC.

Two structural properties worth naming: the **clamp is around the interest
rate**, so a premium inside roughly [−0.04%, +0.06%] yields exactly the interest
rate; and funding is an *interval average*, not a settlement-instant snapshot,
which removes the incentive to push the book at the funding timestamp.

### What we have

- `instrument/funding.go:14-27` — `SimpleFundingCalc.Calculate`:
  `premium_bps = (mark − index)·10000/index`, then
  `rate = BaseRate + premium·Damping/100`, clamped to `±MaxRate`.
- Defaults (`instrument/perp.go:36-40`): `BaseRate: 10` bps, `Damping: 100`,
  `MaxRate: 75` bps.
- Settlement (`exchange/funding.go:262-311`) marks position value at mark price
  and routes the long/short imbalance to fee revenue; the comment at
  `exchange/funding.go:305` already notes real venues send it to the insurance fund.
- `FundingCalculator` is a proper injectable interface
  (`instrument/funding.go:3-6`), so a replacement is a user-side change.

**Gaps against the spec**: the rate is a single instantaneous sample at the
settlement instant rather than a weighted interval average; premium comes from
mid − index rather than impact prices; the base rate is 10 bps per interval
(10× Binance's 1 bp interest term); and there is no interest-rate-centred clamp,
only a symmetric cap on the total.

---

## 3. Liquidation waterfall

### What venues do

The real sequence has four stages, and we implement roughly one of them.

1. **Tiered maintenance margin.** `MM = Notional × MMR(tier) − MaintenanceAmount`,
   with brackets by notional. Binance's API returns them per symbol —
   an ETHUSDT bracket 1 is `notionalFloor 0, notionalCap 10000,
   initialLeverage 75, maintMarginRatio 0.0065, cum 0`. Larger notional →
   higher MMR and lower max leverage.
   Sources: [Leverage and Margin of USDⓈ-M Futures](https://www.binance.com/en/support/faq/leverage-and-margin-of-usd%E2%93%A2-m-futures-360033162192),
   [Notional and Leverage Brackets API](https://developers.binance.com/docs/derivatives/usds-margined-futures/account/rest-api/Notional-and-Leverage-Brackets).
2. **Partial liquidation.** Cancel open orders, then send *one large IOC* to
   reduce the position; if remaining assets then cover the requirement (net of
   realized loss and the clearance fee), liquidation **stops** — the trader keeps
   a reduced position. A **Liquidation Clearance Fee = Notional × Fee Rate** is
   charged unless the position went bankrupt.
   Source: [Binance Futures Liquidation Protocols](https://www.binance.com/en/support/faq/binance-futures-liquidation-protocols-360033525271).
3. **Insurance fund.** Unfilled remainder becomes a *Bankrupt Position* taken
   over by the fund. The fund is **funded**, not just drained: by clearance fees
   and by profit when the engine exits better than the bankruptcy price; Binance
   tops it up to a minimum sized on a 99.9% confidence interval over historical
   stressed scenarios.
   Sources: [Binance Futures Insurance Funds](https://www.binance.com/en/support/faq/introduction-to-futures-insurance-funds-360033525371),
   [BitMEX Insurance Fund Q&A](https://www.bitmex.com/blog/bitmex-insurance-fund-your-questions-answered).
4. **ADL.** When the bankrupt position exceeds the fund's takeover capacity (or
   the fund allocation hits zero, in BitMEX's phrasing), the engine force-closes
   **profitable opposing traders at the bankruptcy price**. BitMEX ranks by
   `PNL% × EffectiveLeverage` when PnL% > 0, else `PNL% / EffectiveLeverage`,
   with `EffectiveLeverage = |MarkValue| / (MarkValue − BankruptValue)`.
   Bybit ranks by leveraged return and matches at the liquidated order's
   bankruptcy price. Sources: BitMEX link above,
   [Bybit ADL mechanism](https://www.bybit.com/en/help-center/article/Auto-Deleveraging-ADL).

There is now a proper theory for stage 4: **Campbell, Hey, Moallemi & Nutz,
*Risk-Based Auto-Deleveraging*, Columbia, March 2026**
([PDF](https://www.math.columbia.edu/~mnutz/docs/Risk_Based_ADL.pdf)). Framing ADL
as minimizing the exchange's expected equity shortfall, the unique optimal policy
under a risk-neutral objective **minimizes the maximum leverage among
participants** — a water-filling ("leverage-draining") rule that is
distribution-free, wash-trade resistant, Sybil resistant, and path-independent.
Under CVaR the minimax policy remains optimal but non-unique. In the cross-margin
case, naive gross leverage misranks hedged accounts; the optimal rule uses a
factor-adjusted leverage so hedged portfolios are deleveraged less.

### What we have

- `exchange/exchange.go:1026-1092` `CheckLiquidations`: cross-margin equity
  `= PerpBalance(quote) − Borrowed[quote] + Σ uPnL` across every margined book
  in that quote asset, against `Σ notional × MaintenanceMarginRate/10000`
  aggregated the same way (`buildAccountMarginProfile`,
  `exchange/exchange.go:985-1017`). The cross-book aggregation is right and was
  a deliberate fix.
- Maintenance rate is **flat**: `MaintenanceMarginRate: 500` bps for every size
  (`instrument/perp.go:42`). Grepping the tree finds no tier/bracket concept anywhere.
- On breach, `liquidate` (`exchange/exchange.go:1112`) closes the **entire**
  position with a market order via `forceClose` (`exchange/liquidation.go:14`).
  No partial reduction, no IOC, no stop-when-covered.
- **No clearance fee. No bankruptcy price. The liquidated trader keeps whatever
  margin survives the close.**
- **The insurance fund is decrement-only**: the sole write is
  `e.ExchangeBalance.InsuranceFund[quote] -= debt` at `exchange/exchange.go:1169`.
  It starts at zero and goes unboundedly negative; `tests/automation_test.go:239`
  asserts exactly that behaviour.
- **No ADL, no socialized loss** — grep finds no such path.
- Nice detail we do get right: an empty book aborts the liquidation and retries
  on the next mark update (`exchange/exchange.go:1123-1126`) rather than
  fabricating a fill.

---

## 4. Margin systems

### What venues do

- **Isolated vs cross** is the easy axis. The interesting one is **portfolio
  margin**, which prices the whole book under scenarios rather than
  summing per-position charges.
- **Deribit PME** (parameters straight from the
  [model deck](https://statics.deribit.com/files/DeribitPortfolioMarginModel.pdf)):
  - Price Range **±15%** for BTC/ETH, evaluated on an 11-column grid
    (−15, −12, −9, −6, −3, 0, +3, +6, +9, +12, +15%).
  - **Volatility Range Up 45%, Down 30%**, applied *relatively* (a 60 vol with
    45% up becomes 87), and scaled by tenor:
    `VolRange% × (30 / days_to_expiry)^VegaPower` with
    **Short Term Vega Power 0.3** (< 30 days) and **Long Term Vega Power 0.13**.
    Each price column carries three vol scenarios (down, same, up).
  - **Maintenance margin = most negative cell of the risk matrix + contingency.**
    **Initial margin = 120% of portfolio maintenance margin.**
  - Contingencies: **0.6% of cumulative absolute futures position**;
    **0.01 BTC per net short option per strike**; ATM Range 10% with
    `AdjustedStrikePos = StrikePos × |S − K| / (S × ATMRange)` inside the band,
    and net short strikes rolled against further-OTM strikes both up and down.
  - Options interest rate 0%. Underlying in the matrix is the **forward for that
    expiry**, not the index. PM is per (sub)account and per base currency — BTC
    and ETH margin never offset.
- **Deribit standard (non-PM) options margin**: short IM
  `= max(0.15 − OTM/S, 0.10)·S + option mark`, maintenance
  `= 0.075·S + option mark`.
  Source: [Deribit standard margin](https://support.deribit.com/hc/en-us/articles/25944811528477-Standard-Margin).
- **OKX unified account PM** groups instruments into **risk units by underlying**
  and stress-tests each with MR1 spot shock, MR4 basis risk, MR6 extreme move,
  MR7 adjusted minimum charge (explicitly sized to cover liquidation fee,
  transaction cost and slippage), MR9 stablecoin depeg; spot and derivatives
  inside a risk unit offset when hedged.
  Source: [OKX portfolio margin mode](https://www.okx.com/help/v-portfolio-margin-mode-cross-margin-trading).
  The MR7 minimum charge is the piece most simulations forget: without it,
  portfolio margin lets a perfectly hedged book run at zero margin and ignore the
  cost of unwinding it.

### What we have

- Cross/isolated mode switch and isolated collateral allocation
  (`exchange/margin.go`), but the liquidation engine is cross-only —
  `buildAccountMarginProfile` aggregates by quote asset across all books and
  never consults `IsolatedPositions`.
- Flat IM 1000 bps / MM 500 bps / warning 750 bps (`instrument/perp.go:41-43`).
- Short-option IM in `instrument/option.go:151-171` implements the Deribit
  standard formula correctly: `max(IMBaseBps − OTM_bps, IMFloorBps)·S + mark`,
  defaults 1500/1000 bps. It over-margins (2× premium) before the first mark
  arrives, which is the safe direction.
- `OptionMarginParams.MMBps` exists at 750 bps but is **unused** — the comment
  at `instrument/option.go:15` says "reserved for MTM sweeps".
- No portfolio margin, no risk matrix, no cross-instrument offsets, no minimum
  charge.

---

## 5. Options

### What venues do

- European, cash-settled, auto-exercised if ITM.
- **Settlement price is a TWAP of the index**: for 08:00 UTC expiries Deribit
  uses the **30-minute** TWAP from 07:30 to 08:00, index computed every 4 seconds.
  Source: [Deribit settlement](https://support.deribit.com/hc/en-us/articles/29734325712413-Settlement).
- **Marks come from an IV surface**, not a single flat vol, and IV is quoted
  against the forward.
- **Delivery fee**: 0.015% of the underlying, capped at 12.5% of the option
  value, so worthless options settle free.
- During the settlement window, Deribit **decays option delta linearly to zero**
  and converges the risk-matrix underlying to the Estimated Delivery Price, so an
  ITM call reports delta ≈ 0 near 08:00 UTC. This is a real margin effect, not a
  display quirk: margin drifts to the projected post-settlement requirement.

### What we have

- `price/black76.go` — Black-76 premium and delta, zero rate, degenerate inputs
  collapsing to intrinsic. Correct and adequate; zero rate matches Deribit's
  0% options interest rate.
- Marks refreshed in `exchange/expiry.go:119-123` as
  `Black76Premium(underlyingMid, K, opt.IV, yearsLeft, isCall)`.
- **`IV` is a single flat float per instrument**, defaulted to 0.8
  (`instrument/option.go:62`), never updated from traded prices. No skew, no term
  structure, no vol-of-vol.
- Underlying is the **spot book mid**, not a forward.
- Settlement is the mean of samples inside a **60-second** window
  (`instrument/settlement_obs.go`, default at `instrument/option.go:65`),
  frozen on first read — the right shape, one-thirtieth the length.
- Delivery fee with cap is implemented and matches the venue pattern
  (`instrument/option.go:126-142`, default cap 1250 bps).
- **Options are invisible to the risk engine.** `marginCore`
  (`exchange/exchange.go:915-924`) returns non-nil only for `*PerpFutures` and
  things exposing `Perp()`; `*EuropeanOption` is neither. So option positions
  contribute **no maintenance requirement and no unrealized PnL** to
  `buildAccountMarginProfile`. Their initial margin is locked as `PerpReserved`,
  but a short option can never trigger or fail a liquidation check.

---

## 6. Dated futures

### What venues do

- Basis converges to zero at expiry by arbitrage. Normal BTC contango on CME and
  Deribit runs **5–15% APR**, spiking to **25–40% APR** in euphoric phases;
  backwardation is rare outside severe stress (March 2020, 3AC, FTX).
  Sources: [CFB on the Bitcoin basis](https://www.cfbenchmarks.com/blog/revisiting-the-bitcoin-basis-how-momentum-sentiment-impact-the-structural-drivers-of-basis-activity),
  [CEPR on crypto carry](https://cepr.org/voxeu/columns/crypto-carry-market-segmentation-and-price-distortions-digital-asset-markets).
- The important mechanism for a simulator is that **convergence is not free**:
  the short futures leg of a cash-and-carry trade marks to market against you
  while the basis widens, and at only 10× leverage that leg would have been
  liquidated in over half the months of a historical sample. Convergence trades
  die of margin, not of being wrong. Same CEPR source.
- Settlement is a TWAP/EDP over a defined window, and traditional futures
  **settle variation margin daily** rather than carrying entry price to expiry.

### What we have

- `instrument/dated.go` — `ExpiringFutures` embeds `PerpFutures` (so it inherits
  the margin, mark and liquidation machinery via `Perp()`), reports
  `IsPerp() == false` to skip the funding sweep, and cash-settles at a 60s TWAP
  of the underlying. `ExpiryCashFlow = size × (settlement − entry)`.
- `exchange/expiry.go:160+` `settleExpiredInstrument` halts the book, cancels
  resting orders with exact ledger releases and forced-cancel notifications,
  settles every position, releases margin, delists. Cascading expiry is handled
  by `instrument/listing.go`.
- **No daily settlement / variation margin.** PnL accrues inside the position
  until close or expiry. Given that, `ExpiryCashFlow` from entry price is
  internally consistent — but the liquidation *path* differs from a real venue,
  where daily cash settlement realizes losses and can bankrupt an account before
  expiry.
- No structural basis: the dated book's price comes from whatever actors quote.
  Nothing enforces or rewards convergence, and there is no carry/interest term.

---

## Assumptions we currently make

Stated plainly, so they can be accepted or rejected on purpose rather than by default.

1. **Mark price is the perp's own mid** unless the user injects an index-anchored
   calculator (`exchange/exchange.go:710`).
2. **Index is a single spot book's mid**, with no basket, deviation cap or
   staleness handling; if absent, mark is used as index and basis is zero.
3. **Funding is a settlement-instant snapshot** of `(mark − index)/index`, scaled
   and capped, with a 10 bps base rate — no interval TWAP, no impact prices, no
   interest-rate clamp.
4. **Maintenance margin is a flat rate of notional** at every size.
5. **Liquidation closes 100% of the position at market**, in one shot.
6. **The liquidated trader keeps residual margin**; there is no bankruptcy price
   and no clearance fee.
7. **The insurance fund is a loss counter** that only decrements and may go
   arbitrarily negative; there is no ADL and no socialized loss.
8. **Options carry initial margin but no maintenance risk**, and their MTM does
   not enter cross-margin equity.
9. **Implied volatility is one flat number per option**, constant for the
   instrument's life.
10. **Dated futures do not settle daily**; entry price is carried to expiry.
11. **Cross-margin nets across every book in a quote asset**, with no correlation
    modelling, no offsets, and no minimum charge.
12. **Interest on borrowing is a flat annual rate** charged per simulated minute
    (`exchange/exchange.go:928-960`), with no utilisation curve.

Safe simplifications, in my judgement: zero interest rate in Black-76 (matches
Deribit), cash settlement everywhere, per-minute interest accrual, flat borrow
rate, no cross-currency margin. These change levels, not dynamics.

---

## Elephants in the room

Ranked by how badly each distorts emergent dynamics, not by how wrong it is in
isolation.

1. **The mark is derived from the book the liquidation trades into.** With the
   default `MidPriceCalculator`, a forced sale lowers the mark, which triggers the
   next liquidation. Cascade experiments run today measure a feedback loop the
   real venue design specifically removes; conversely, mark manipulation is
   trivially profitable in a way no live venue permits. This one contaminates
   every downstream conclusion about leverage and stability.

2. **Solvency is fiction.** The insurance fund only loses money
   (`exchange/exchange.go:1169`) and there is no ADL, so bad debt is absorbed by
   an infinitely deep imaginary balance sheet. Two opposite errors follow: the
   winners never pay (no ADL unwinds, so profitable traders' PnL is
   overstated in stress), and the exchange never runs out (so no experiment can
   ever produce the terminal state real venues are engineered around).

3. **All-or-nothing liquidation at market.** Real venues reduce with a single
   large IOC and stop once covered. Ours dumps the full position, which
   simultaneously overstates the price impact per liquidation event and
   understates the number of events. The shape of the cascade — many small
   partial reductions vs few total dumps — is qualitatively different.

4. **No bankruptcy price, no clearance fee, so leverage is under-punished.** A
   liquidated trader on Binance loses the margin backing the position and pays a
   fee; ours hands back the residual. Any agent-based experiment on optimal
   leverage will find leverage too attractive.

5. **Options are risk-free to the engine.** Short option positions can never be
   liquidated and their MTM never affects perp equity
   (`marginCore` returns nil for `*EuropeanOption`). Any cross-margin experiment
   mixing options and perps is unsound as it stands — a trader can sell unlimited
   premium and only ever be checked against the initial reservation.

6. **Flat maintenance rate.** Without notional tiers there is no size-dependent
   risk pricing, so nothing caps concentration and whale positions are as cheap
   per unit notional as retail ones. Position-size distributions in the sim will
   be too fat-tailed relative to reality.

7. **Funding is a manipulable point sample with a 10× base rate.** Because the
   rate is read at the settlement instant from mid − index, a single actor moving
   the book at that instant sets the payment for everyone — the exact attack the
   time-weighted impact-price premium exists to prevent. And a 10 bps base drowns
   the interest-rate anchor.

8. **Flat IV means shorts have no vega risk.** Option marks are a deterministic
   function of the underlying, so the dominant real risk of a short options book
   (vol expansion on a price shock, which is precisely what Deribit's 45% vol-up
   scenario prices) cannot occur in the sim.

---

## Concrete recommendations

Ranked by realism gain per unit of work. All are shaped as injectable
interfaces per the library-first rule: none require a user to edit library files
or add enum values.

### 1. Make the mark index-anchored by default, and add the venue formula

**Mechanism.** Change the fallback at `exchange/exchange.go:710-711` from
`MidPriceCalculator` to `ClampedEMAMarkPrice` whenever an index source is
configured (keep the mid as the documented no-index degenerate case). Add a
`BinanceMedianMarkPrice` calculator implementing
`median(Index·(1 + F·t/T), Index + MA(basis), contract price)` — the existing
`MarkPriceCalculator` interface already supports it, and `MedianMarkPrice` plus
`TWAPMarkPrice` are 80% of the parts. Remove the
`indexPrice = markPrice` fallback at `exchange/exchange.go:857` in favour of an
explicit "no index configured" state so a silent zero basis is impossible.

**Expected gain.** Highest of anything on this list. It breaks the
liquidation→mark→liquidation loop, so cascade magnitude becomes a property of
book depth and leverage rather than an artifact of the mark definition. It also
makes basis, funding and carry strategies meaningful, since the basis stops
being definitionally zero in single-venue configurations.

### 2. Real liquidation waterfall: bankruptcy price → funded insurance fund → ADL

**Mechanism.** Introduce a `LiquidationPolicy` interface the exchange calls
instead of the inline `liquidate`, with a default implementation that:
- computes the **bankruptcy price** (mark at which position equity hits zero —
  `EstimateLiquidationPrice` at `exchange/exchange.go:1096` already computes
  essentially this);
- takes the position over at the bankruptcy price, so the trader's margin for
  that position is forfeit;
- charges a configurable **clearance fee** on notional;
- credits `InsuranceFund += (fill − bankruptcy) × size` when the engine exits
  better than bankruptcy, and debits it otherwise — turning
  `exchange/exchange.go:1169` from the only write into one of three;
- when the fund cannot cover, invokes an injectable `ADLPolicy` that force-closes
  ranked opposing positions **at the bankruptcy price**. Ship two: BitMEX-style
  `PNL% × EffectiveLeverage`, and the water-filling minimax-leverage rule from
  Campbell et al. (2026), which is the theoretically optimal one and is
  wash-trade and Sybil resistant.

**Expected gain.** Very high, and it buys a *testable invariant*: with ADL, total
system PnL conserves exactly (every bankrupt dollar is paid by an identified
counterparty), which the existing conservation-check machinery can assert. It
also makes the winner side of a cascade behave correctly — today profitable
traders are never touched.

### 3. Tiered maintenance margin + partial liquidation

**Mechanism.** Replace the scalar `MaintenanceMarginRate` with an injectable
`MaintenanceMarginModel` interface (`Requirement(notional int64) int64`), whose
default is the current flat rate and whose venue implementation is a bracket
table evaluating `notional × MMR(tier) − maintAmount`. Then change `liquidate` to
reduce toward a target margin ratio with an IOC-style partial close, stopping
once equity covers the requirement net of realized loss and clearance fee,
instead of always passing `abs(pos.Size)` to `forceClose`.

**Expected gain.** High. Tiers give the system a natural concentration limit, and
partial liquidation changes the cascade from a few large dumps into many small
reductions — a different impact profile and, in practice, the difference between
a cascade that self-arrests and one that does not.

### 4. Funding: interval-averaged premium from impact prices, with the interest clamp

**Mechanism.** Add a `PremiumIndexSampler` that the price loop calls on each tick
to accumulate `P = [max(0, ImpactBid − Index) − max(0, Index − ImpactAsk)]/Index`,
where impact prices come from walking the book for a configurable Impact Margin
Notional (the book already exposes levels; `WeightedMidPriceCalculator` shows the
traversal pattern). Then ship a `BinanceFundingCalc` implementing
`F = timeWeightedAvg(P) + clamp(interestRate − P, ±5bps)`, scaled by interval
length, and drop the default `BaseRate` from 10 bps to 1 bp.
`FundingCalculator` is already injectable, so only the sampler is new plumbing.

**Expected gain.** High for anything touching basis, carry, or funding-arb
actors. It removes the settlement-instant manipulation channel and makes the
premium depth-weighted, so a thin book can no longer set the funding rate for the
whole population.

### 5. Put options into the risk engine

**Mechanism.** Two steps, in order.
- *Cheap and urgent*: make `buildAccountMarginProfile` see options. Give
  `EuropeanOption` a maintenance contribution using the already-present
  `MMBps` (Deribit standard: `0.075·S + mark` per unit short) and include option
  MTM (`mark − entry premium`) in cross equity. This closes the "sell unlimited
  premium" hole at `exchange/exchange.go:988`.
- *Then*: add a `PortfolioMarginer` interface returning an account-level
  requirement, with a Deribit-PME-shaped default — 11-column ±15% price grid ×
  {vol down 30%, flat, vol up 45%} scaled by `(30/days)^vegaPower`
  (0.3 short-dated, 0.13 long-dated), worst cell plus contingencies
  (0.6% of gross futures notional, 0.01 per net short option per strike),
  `IM = 1.2 × MM`. Include an OKX-style **minimum charge** so a hedged book
  still pays for its unwind cost.

**Expected gain.** Step one is small work and removes a correctness hole rather
than adding realism — do it regardless. Step two unlocks a class of experiments
(vol sellers blowing up on a vol-expansion shock) that is currently impossible
because our option marks have no vega.

### Below the line, in rough order

6. **Basket index** with per-source deviation caps (3% → clamp at 1.03×/0.97×
   median) and staleness eviction (>5 min → weight 0), behind the existing
   `PriceSource` interface. Needed only when studying oracle failure or
   cross-venue manipulation.
7. **IV surface** as a pluggable interface (skew + term structure) replacing the
   flat `opt.IV`, plus marking against the forward rather than spot mid.
   Prerequisite for step 5's second half to be interesting.
8. **Daily settlement for dated futures** — realize variation margin on a
   configurable schedule instead of carrying entry to expiry. Changes when a
   losing carry trade dies, which is the whole point of the basis-convergence
   literature.
9. **Settlement window length** — the 60s default in
   `instrument/settlement_obs.go` versus Deribit's 30 minutes. One-line config
   change; matters only for expiry-manipulation experiments, where 60s is
   cheap to push and 30 min is not.
10. **Delta decay during the settlement window** for options, per Deribit. Low
    priority; affects margin only in the final minutes of an expiry.
