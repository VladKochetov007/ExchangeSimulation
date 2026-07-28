# Deep dive: mark price and index construction, at implementation grade

Research pass, July 2026. This document exists to make a venue-grade mark price
implementable in Go without further reading. It assumes
`deepdive-derivatives-mechanics.md` §1 and `deepdive-derivatives-impl.md` have
been read; those cover *why* mark ≠ last trade and the funding formula. This one
covers *exactly* what each venue computes, with the constants, the sampling
cadence, the degenerate-input fallbacks, and the index basket rules that the
summary docs skip.

Two things changed at the major venues in late 2025 that invalidate older
write-ups, including parts of our own mechanics doc:

- **Binance moved Price 2 from a 1-minute basis MA to a 30-second basis MA**,
  effective 2025-09-18 08:01 UTC, and simultaneously rescaled the funding rate
  formula by the settlement frequency.
- **Bybit replaced the median-of-three mark with a shrunk-basis blend** for
  newly listed and selected symbols, rolled out 2025-11-14 to 2025-11-16.

A note on sourcing: `support.deribit.com` returns HTTP 403 to automated fetches.
The Deribit formulas below were recovered from search-engine snippets that quote
those articles verbatim, and cross-checked against two independent secondary
sources. They are marked accordingly. Everything else was fetched directly.

---

## 1. Binance USDⓈ-M and COIN-M perpetuals

### 1.1 The mark price

$$
\text{Mark} = \operatorname{median}\bigl(P_1,\; P_2,\; P_{\text{contract}}\bigr)
$$

$$
P_1 = I \cdot \Bigl(1 + r_{\text{last}} \cdot \frac{T_{\text{until funding}}}{T_{\text{funding period}}}\Bigr)
$$

$$
P_2 = I + \operatorname{MA}_{30}(\text{basis})
\qquad
\operatorname{MA}_{30}(\text{basis}) = \frac{1}{30}\sum_{i=1}^{30}\left(\frac{B_i + A_i}{2} - I_i\right)
$$

where $I$ is the price index, $r_{\text{last}}$ the last settled funding rate,
$B_i, A_i$ the best bid and best ask of the *contract* book at sample $i$, and
$I_i$ the index at that same sample.

The COIN-M article writes the ramp denominator as a literal 8, i.e.
$P_1 = I \cdot [1 + r_{\text{last}} \cdot (T_{\text{until funding}}/8)]$ with
$T$ in hours. That is the same expression as the USDⓈ-M form for an 8-hour
contract; for 4-hour contracts the denominator is the contract's own funding
period, not 8. Treat the funding period as a per-instrument constant.

The basis MA is sampled **once per second over a 30-second window**, giving
exactly 30 data points. Before 2025-09-18 this was a 1-minute window; the
announcement states the new form recalculates "every 30 seconds using 30 data
points instead of minute-based calculations."

Binance also documents the median tie-break behaviour explicitly, which is worth
quoting because it confirms the median is over the *values*, not a priority
ordering: "If Price 1 < Price 2 < Contract Price, then Price 2 will be used as
the Mark Price."

### 1.2 What "Contract Price" means

**Binance never defines it in either the USDⓈ-M or COIN-M article.** This is a
real documentation gap, not an oversight in the reading. The defensible
interpretation is *the perpetual's own last traded price on Binance*, on three
grounds:

1. Binance's own API surfaces `markPrice`, `indexPrice`, and a separate last
   price; nothing else in the response set is a candidate.
2. Bybit's pre-November formula is structurally identical and writes the third
   term explicitly as **"Last Traded Price"** — see §3.1. Two venues converging
   on the same three-term median with one term spelled out is strong evidence.
3. If it were the mid, $P_2$ would be a smoothed version of the same quantity
   and the median would degenerate: two of three inputs would be near-duplicates
   of the book mid, defeating the "must move two of three" property.

Implement it as last trade. Note the consequence: with an empty or never-traded
book the third term is zero or stale, and the median silently collapses onto
$\min(P_1, P_2)$ or $\max(P_1, P_2)$. That degenerate case is the one to guard.

### 1.3 Fallback when a component is missing

The COIN-M article gives the only documented rule: "during extreme market
conditions or deviations in price sources, which may lead to the Mark Price
deviating from the Spot price, Binance will take additional protective measures
(e.g., setting Mark Price to Price 2)."

So the documented fallback is **collapse to $P_2$** — index plus smoothed basis
— which is the most index-anchored of the three terms that still tracks the
contract. Below that sits **Last Price Protected**, which activates "when
Binance cannot obtain a stable reference data for the Price Index and the Mark
Price," and uses "the latest transaction price of the contract within a certain
limit as a reference." The bound on that limit is not published.

The implementable fallback ladder, in order:

1. All three terms valid → median.
2. Index valid, contract last trade missing → $P_2$.
3. Index invalid → last trade, clamped to a band (Last Price Protected).

### 1.4 The index price basket

Constituents, as currently listed: Binance, KuCoin, OKX, HitBTC, Gate.io, MEXC,
Coinbase, Kraken, Bitget, Bitfinex, Bybit, PancakeSwap (BNB Chain), Uniswap
(Ethereum), Raydium (Solana), and Aster. Fifteen sources, including three DEXs —
the DEX inclusion is recent and worth noting as a design signal.

Weighting is by **relative volume**:

$$
w_i = \frac{v_i}{\sum_j v_j}, \qquad I = \sum_i w_i \cdot \tilde{p}_i
$$

Two protective rules apply before the weighted sum:

**Deviation clamp.** "If the latest price of a specific exchange deviates by more
than 3% from the median price of all sources, the value will be immediately
capped at either 1.03 times or 0.97 times the median price." Note this is a
**clamp, not an exclusion** — the source keeps its weight and contributes the
bounded value:

$$
\tilde{p}_i = \operatorname{clamp}\bigl(p_i,\; 0.97 \cdot m,\; 1.03 \cdot m\bigr),
\qquad m = \operatorname{median}_j(p_j)
$$

**Staleness.** "If an exchange has not updated its trading data within the last
five minutes, the weight of that exchange will be set to zero." Weight zeroing,
with the remaining weights implicitly renormalised by the $\sum_j v_j$
denominator.

Binance does not publish a minimum source count. The Last Price Protected mode
is the documented behaviour when the basket cannot produce a stable value, which
functions as the minimum-count fallback without naming a number.

The COIN-M index is a different, smaller basket: Bitstamp, Coinbase Pro, Kraken,
Binance, Huobi, KuCoin, OKX — seven sources, "weighted by relative volume to
reduce the risk of price manipulation."

---

## 2. OKX

OKX publishes the shape but withholds the constants, and says so: the index
article states OKX "may ... refrain from disclosing certain constituent pricing
sources, weighting information."

**Mark price.** "Mark price = Index price + Basis of moving average", where the
basis is the "moving average of [(Best ask of contract + Best bid of contract) /
2 − Index price]". This is Binance's $P_2$ used *alone* — no median, no funding
ramp term, no published deviation cap against the index. The window length and
sampling interval are not documented in the help centre. Community and
comparison sources put it in the 30-second to 60-second range; treat the window
as unknown and configurable rather than pinning a number you cannot cite.

**Index price.** OKX selects "the last traded prices from three or more other
exchange venues with adequate market liquidity as the weighted index
constituents." The published rules differ by source count, which is unusually
explicit and worth copying:

- **3 or more valid sources**: equally weighted average, with the deviation rule
  applied — "if the price of any exchange deviates more than 3% from the median
  price of all exchanges, the exchange's price will be bounded within median
  price × 0.97 and median price × 1.03." Same clamp-not-exclude semantics as
  Binance, same 3% threshold. The newer article describes the weighting as
  "weighted by pre-set values" for 3+ sources, so the equal-weight statement may
  be the older regime; implement weights as configurable with equal as default.
- **2 valid sources**: equal weighting.
- **1 valid source**: that single price is used.

**Staleness.** "Price sources that underwent system maintenance or didn't update
their latest price during a specified time period won't be taken into
calculation." The threshold varies by currency and is not published — the older
rules article phrases it as "no update for a certain time (vary from currency)."

**Cross-currency conversion.** OKX documents the chaining rule, which our
`PriceSource` design will eventually need: "If the exchange trading pair in the
index sample is inconsistent with that of the index, it shall be converted into
the corresponding price of the same currency based on BTC/USDT, BTC/USD,
USDC/USD, USDT/USD, etc. For example, if a component of the ETH/USD index is
ETH/USDT, multiply the component by the price of the USDT/USD index."

---

## 3. Bybit

Bybit is the most interesting of the four because it now runs **two different
mark price formulas simultaneously**, split by symbol.

### 3.1 Legacy formula — most perpetuals

$$
\text{Mark} = \operatorname{median}\bigl(P_1,\; P_2,\; P_{\text{last traded}}\bigr)
$$

$$
P_1 = I \cdot \Bigl[1 + r_{\text{last}} \cdot \frac{T_{\text{until funding}}}{8}\Bigr],
\qquad
P_2 = I + \operatorname{MA}_{150}\Bigl(\frac{B_1 + A_1}{2} - I\Bigr)
$$

The MA window is **2.5 minutes sampled every second** — 150 data points, five
times Binance's window. The third median term is documented explicitly as "Last
Traded Price," which is the corroboration for §1.2.

### 3.2 New formula — symbols listed after 2025-11-14, plus a named set

Applies to RIFUSDT, QUICKUSDT, OXTUSDT, PYRUSDT, 10000ELONUSDT, FIOUSDT,
MILKUSDT, DODOUSDT, NTRNUSDT, BOBAUSDT, and every perpetual listed after
2025-11-14 10:00 UTC.

$$
\text{Mark} = P_3 \cdot C + I \cdot (1 - C)
$$

$$
P_3 = I + \operatorname{MA}(\Delta P), \qquad
\Delta P = \frac{B_1 + A_1}{2} - I \;\;\text{(sampled every second)}
$$

$$
C = \operatorname{clamp}\!\left(\frac{\Delta P}{\Delta P_{\max}},\; 0.3,\; 0.7\right),
\qquad
\Delta P_{\max} = \text{$R$-minute maximum basis, sampled every second,}
$$

excluding the most recent data point from the maximum.

**The algebraic reduction is the part that matters for implementation.**
Substituting $P_3$:

$$
\text{Mark} = \bigl(I + \operatorname{MA}(\Delta P)\bigr) C + I(1 - C)
            = I + C \cdot \operatorname{MA}(\Delta P)
$$

So Bybit's new mark is **index plus a shrunk basis**, where the shrinkage factor
$C \in [0.3, 0.7]$ is driven by how large the instantaneous basis is relative to
its recent maximum. At minimum shrinkage the mark carries 30% of the smoothed
basis; at maximum, 70%. It never carries the full basis, and it never drops the
basis entirely.

This is a *soft* alternative to the hard band in our `ClampedEMAMarkPrice`: a
hard clamp is discontinuous at the band edge and pins there, whereas the Bybit
form degrades continuously and always retains index dominance of at least 30%.
Both bound $|\text{Mark} - I|$, but the shrunk-basis form does it without a
kink. The $\Delta P_{\max}$ normalisation is self-calibrating — an instrument
that normally runs a wide basis gets a large denominator and therefore a small
$C$, so the rule adapts per symbol without per-symbol configuration. That is a
genuinely good design and it is cheap to implement.

Note the two documented clamp bounds diverge across Bybit's own surfaces: the
help-centre article says $(0.3, 0.7)$ and at least one syndication of the
announcement says $(0.1, 0.9)$. The help centre is the authoritative surface and
was fetched directly, so $(0.3, 0.7)$ is the value to use; make it configurable.
$R$ is not published.

### 3.3 TradFi perpetuals

$$
\text{Mark}_{\text{TradFi}} = \operatorname{clamp}\bigl(\text{Mark}_{\text{perp}},\; I \cdot 0.97,\; I \cdot 1.03\bigr)
$$

An explicit ±3% hard band on top of the normal mark. This is the only *published*
numeric deviation cap on a mark price across the four venues, and it is a useful
anchor for calibrating our `bandBps`.

### 3.4 Index price

Bybit's index differs from Binance/OKX in that deviation triggers **exclusion
with smoothed weight decay**, not a clamp:

> "If the Spot price of any component trading platform diverges by more than 5%
> from the median of all Spot price sources, the system will temporarily exclude
> the respective component from index price calculation. During the exclusion
> period, the original component's weight is gradually reduced using a smoothing
> algorithm and redistributed among the remaining non-excluded components until
> its price lies within 5% of all Spot prices' median."

Staleness: "If no Spot trading pair has been traded on the exchange for more
than 15 minutes, the trading pair will be excluded." Three times Binance's
5-minute threshold.

The smoothed weight redistribution is the detail most implementations miss. A
hard exclusion produces a step discontinuity in the index at the moment a source
crosses the 5% line, and a matching step back when it recovers — which is
exactly the kind of artificial jump that can trigger liquidations. Ramping the
weight to zero over some horizon removes the step.

---

## 4. Deribit futures and perpetuals

*Sourced from search snippets quoting the Deribit support articles;
`support.deribit.com` blocks automated fetch. Cross-checked against Deribit
Insights and an independent venue-comparison site, which agree on the numbers.*

**Mark price**, futures and perpetuals alike:

$$
\text{Mark} = I_{\text{Deribit}} + \operatorname{EMA}_{30s}\bigl(P_{\text{fair}} - I_{\text{Deribit}}\bigr)
$$

An **EMA, not an SMA** — the only one of the four venues to use exponential
weighting, and the direct analogue of our `EMAMarkPrice`. The window is 30
seconds, matching Binance's post-2025 window length despite the different
weighting scheme.

$P_{\text{fair}}$ is not the top-of-book mid. It is the **depth-weighted mid**:
"the 1 BTC deep average of bid and ask of the order book," bounded around best
bid and best ask. Deribit's own phrasing for futures is "the bounded (around best
bid and best ask) mid price." This is a meaningful difference from Binance and
Bybit, both of which use $(B_1 + A_1)/2$: a one-lot quote at an absurd price
moves the Binance basis sample and does not move Deribit's, because 1 BTC of
depth has to be crossed to shift the average.

**Deviation limits.** Deribit does not cap the mark against the index directly.
It caps *trading*, with a two-tier band:

$$
\text{tradeable range} = \Bigl[I + \operatorname{EMA}_{60s}(P_{\text{bounded mid}} - I)\Bigr] \pm 1.5\%
$$

intersected with a fixed band of $\pm 7.5\%$ around the index for linear
perpetuals, $\pm 5\%$ for inverse perpetuals. Deribit states these "can be
adjusted at the sole discretion of Deribit."

Two windows coexist: 30 s EMA for the mark, 60 s EMA for the trading-range
centre. The slower window on the range makes the band harder to walk.

**Index price.** The BTC/ETH index currently has **three constituents** —
Coinbase Pro, Kraken, LMAX Digital — down from eight historically. Deribit
retrieves best bid and best ask from each and computes the mid, then:

> "The sample exchanges are then benchmarked against the median price of the
> included exchanges. The values that fall outside the +/-0.5% range of the
> median price are adjusted to the closest bandwidth price limit. Consequently,
> the index is calculated as the equally-weighted average of these values."

**±0.5% is six times tighter than Binance's and OKX's 3% and ten times tighter
than Bybit's 5%.** That is the design trade-off in one number: three
high-quality institutional venues clamped hard, versus fifteen mixed-quality
venues clamped loosely. Deribit computes the index every 4 seconds and measures
it continuously against an external benchmark. Excluded sources are those with
"invalid data or delayed order book data."

Historically Deribit used a trimmed mean instead — "the highest and lowest price
are taken out, and the remaining exchanges are each for 25% accountable," capped
at four constituents. The move from trimming to capped-median is the same
direction Binance and OKX went: clamping preserves the information in an
outlier's direction while bounding its magnitude, where trimming discards it.

---

## 5. Cross-venue constants

### Mark price

| | Binance USDⓈ-M / COIN-M | OKX | Bybit (legacy) | Bybit (new, post-2025-11) | Deribit |
|---|---|---|---|---|---|
| Structure | median of 3 | index + MA(basis) | median of 3 | index + shrunk basis | index + EMA(basis) |
| Basis smoothing | SMA | MA | SMA | MA | **EMA** |
| Window | **30 s** (was 60 s pre-2025-09-18) | undisclosed | **2.5 min** | undisclosed | **30 s** |
| Sample interval | 1 s | undisclosed | 1 s | 1 s | continuous |
| Basis input | $(B_1+A_1)/2$ | $(B_1+A_1)/2$ | $(B_1+A_1)/2$ | $(B_1+A_1)/2$ | **1 BTC-deep bounded mid** |
| Contract term | last trade (undocumented) | none | last trade | none | none |
| Funding ramp term | yes, $P_1$ | no | yes, $P_1$ | no | no |
| Deviation cap vs index | none (median is the defence) | none published | none | soft: $C\in[0.3,0.7]$ | none on mark; trade range ±1.5% / ±7.5% |
| Documented fallback | collapse to $P_2$, then Last Price Protected | none published | none published | none published | none published |

### Index price

| | Binance USDⓈ-M | OKX | Bybit | Deribit |
|---|---|---|---|---|
| Constituents | 15 (incl. 3 DEX) | 3+ | "top global spot exchanges" | **3** (Coinbase Pro, Kraken, LMAX) |
| Weighting | relative volume | preset weights (3+), equal (2) | real-time weight | **equal** |
| Per-source input | last price | last price | spot price | **best bid/ask mid** |
| Deviation threshold | **3%** | **3%** | **5%** | **0.5%** |
| Deviation action | **clamp** to $m \cdot [0.97, 1.03]$ | **clamp** to $m \cdot [0.97, 1.03]$ | **exclude**, weight decays smoothly | **clamp** to $m \cdot [0.995, 1.005]$ |
| Staleness threshold | **5 min** | varies by currency | **15 min** | delayed data → excluded |
| Staleness action | weight → 0 | exclude | exclude | exclude |
| Min sources | not published | 1 (degrades gracefully) | not published | not published |
| Recompute cadence | continuous | continuous | continuous | **4 s** |

The convergent design across all four, and the thing to copy: **clamp toward the
median rather than exclude**, because exclusion is a step function and clamping
is continuous; **zero the weight on staleness** rather than carrying a stale
print; and **never let a single source set the index**, which is precisely the
failure in §6.3.

---

## 6. Why any of this exists: three documented failures

### 6.1 BitMEX / Bitstamp, 2019-05-17 — concentrated index weight

At roughly 02:30 UTC a sell order variously reported at 4,300–5,000 BTC hit
Bitstamp and drove BTC from about \$7,944 to \$6,177. Bitstamp at that time
constituted **half the BitMEX index**. The index followed the single venue down,
XBTUSD marked down with it, and **more than \$230 million of positions were
liquidated on BitMEX**, with open interest falling from \$630 M to \$400 M. The
canonical framing: roughly \$30 M of spot selling on one venue triggered \$230 M
of liquidations on another — a 7-to-1 amplification.

The comparative evidence is the useful part. BitMEX's XBT perpetual reached a
price \$251 (3.78%) below the average low of four other sample derivatives
venues and \$267 (4.01%) below three institutional BTC indices. Venues that
either excluded Bitstamp or applied outlier adjustment — Deribit and OKEx among
them — saw materially fewer liquidations. Deribit's index at the time
"continuously retriev[ed] mid-price from its six index constituents, and
exclude[d] the highest and the lowest values," which is exactly the mechanism
that absorbed the shock.

**Design lesson:** the attack cost scales with how many constituents you must
move simultaneously. One source at 50% weight makes the index as cheap to move
as the cheapest venue in the basket.

### 6.2 Mango Markets, 2022-10-11 — thin spot, honest oracle

Avraham Eisenberg opened offsetting positions across two accounts he controlled,
shorting 488 million MNGO on one and buying it on the other, then spent about
\$4 million buying MNGO across **three separate spot exchanges**. The
oracle-reported price rose roughly 13-fold in 30 minutes (reported elsewhere as
2,300%), marking his long to about \$400 million in paper gains, against which
he borrowed roughly \$110–117 million and drained the protocol.

The oracle was not broken. It faithfully reported a real, thin market. The
failure was that MNGO's total spot liquidity was small enough that \$4 million
moved every constituent at once, so multi-source aggregation bought nothing.
Eisenberg was charged by the CFTC and SEC and convicted at trial in Manhattan on
commodities fraud, commodities manipulation, and wire fraud.

**Design lesson:** basket diversity is necessary but not sufficient. For thin
assets the binding constraint is total spot depth versus the notional the
derivative permits, which is a *position limit and margin tier* problem, not an
index problem. An index cannot save you from a market that costs \$4 M to move.

### 6.3 Binance USDe / wBETH / BNSOL, 2025-10-10 — venue-internal oracle

During the largest liquidation event on record — over \$19 billion, about 1.6
million accounts — collateral posted in USDe, wBETH, and BNSOL "had liquidation
prices based on Binance's own volatile spot market, not reliable external data."
USDe printed **\$0.65 on Binance, a 35% depeg, while trading at roughly \$1.00
everywhere else**. Liquidated collateral was sold into the same thin internal
book that set its price, which lowered the price, which triggered the next
liquidation. Prices bottomed 21:20–21:21 UTC, with the severe depeg after 21:36
UTC. Binance paid **\$283 million** in compensation.

The remediation Binance announced is the most direct endorsement of index
construction available: incorporate **the assets' redemption prices into the
index weights**, and install a **soft price floor in the reference index** for
USDe.

**Design lesson:** this is the self-referential mark loop from
`deepdive-derivatives-mechanics.md` §1, realised at scale on a top-tier venue
just nine months ago, and it happened on the *collateral valuation* path rather
than the perp mark path. Anchoring perp marks to an index while still valuing
collateral at the venue's own book leaves the loop fully intact. Worth checking
against our own `PositionManager` collateral valuation, which is out of scope
here but squarely in scope for the accounting invariants work.

---

## 7. Go implementation sketch for this repo

### 7.1 What already exists

`price/price.go:5` defines the whole contract:

```go
type MarkPriceCalculator interface {
	Calculate(book *ebook.OrderBook) int64
}
```

`price/calculators.go` provides seven implementations. Three map onto venue
formulas already:

- `MedianMarkPrice` (`price/calculators.go:65`) — median of index, best bid,
  best ask. Structurally a median-of-three, but **not** Binance's: Binance's
  three terms are $P_1$, $P_2$, and last trade, whereas ours are index, bid, ask.
  Ours takes the median of two highly correlated book quantities and one index,
  so two of three inputs move together and the "must move two of three" property
  is weaker than it looks.
- `EMAMarkPrice` (`price/calculators.go:98`) — $I + \operatorname{EMA}(\text{mid} - I)$.
  This is **Deribit's formula exactly**, modulo the fair-price input (we use
  top-of-book mid, Deribit uses 1 BTC-deep bounded mid).
- `ClampedEMAMarkPrice` (`price/calculators.go:154`) — the same with a hard band.
  No venue publishes this exact form; it is a defensible synthesis, and Bybit's
  new $C$-shrinkage is the smooth alternative to its hard clamp.
- `TWAPMarkPrice` (`price/calculators.go:207`) — rolling SMA of basis with a
  band. **This is Binance's $P_2$ and OKX's entire mark price**, already written.

The engine already anchors by default: `ConfigureAutomation`
(`exchange/exchange.go:709`) sets `autoAnchorMarks` when no calculator is
injected, and `ensureAnchoredMarkCalcs` (`exchange/exchange.go:873`) installs a
per-symbol `ClampedEMAMarkPrice` for every margined book with a resolvable
index. `indexSourceLocked` (`exchange/exchange.go:851`) resolves the index from
the underlying spot book mid, then the `IndexProvider`, and — correctly —
returns 0 rather than falling back to the book's own price.

### 7.2 Three problems to fix before adding anything

**(a) The EMA window is calibrated 60× too slow.** `Calculate` is invoked once
per `updateAllPerpPrices` pass (`exchange/exchange.go:934`), which runs on the
`PriceUpdateInterval` ticker, defaulting to 3 seconds. The default
`MarkPriceEMAWindow` is 600 *samples*. Effective time constant:

$$
600 \text{ samples} \times 3 \text{ s/sample} = 1800 \text{ s} = 30 \text{ minutes}
$$

Every venue in §5 uses 30 s to 2.5 min. A 30-minute basis EMA means the mark
essentially ignores the contract and tracks the index, which suppresses the
liquidation cascade but also makes funding and unrealised PnL wrong in any
trending regime. To match Binance at a 3-second tick, `MarkPriceEMAWindow`
should be **10**; to match Bybit's 2.5 minutes, **50**. Recommend 10 as the
default and documenting the sample-count-not-seconds semantics on the field.

**(b) The band is calibrated too tight.** `halfBand = index * bandBps / 2 / 10000`
(`price/calculators.go:195`), so the default `MarkPriceBandBps = 100` yields a
**±0.5%** cap on the basis. The only published venue cap on a mark is Bybit
TradFi's ±3%. A perp in strong contango routinely runs a basis above 0.5%, so
the current default clips genuine signal and pins the mark at the band edge.
Recommend `bandBps = 600` (±3%) as the default, with the tight value reserved
for deliberately illiquid sim books.

**(c) Stateful calculators mutate under a read lock — a latent data race.**
`EMAMarkPrice.Calculate` writes `c.emaBasis` and `c.seeded`
(`price/calculators.go:142-147`); `TWAPMarkPrice.Calculate` writes `c.window`,
`c.pos`, `c.size`. All are invoked at `exchange/exchange.go:934` inside
`e.mu.RLock()` (`exchange/exchange.go:920`), and a read lock permits concurrent
holders. `UpdatePerpPrices()` is exported specifically so simulations and tests
can drive the pass by hand (`exchange/exchange.go:900`).

Nothing races today: `tests/markprice_anchor_regression_test.go` calls
`ConfigureAutomation` but never `StartAutomation`, so the manual pass is the
only writer. The hazard is that the two entry points are documented as
interchangeable — `UpdatePerpPrices` is described as "the same pass the
automation price loop runs on its ticker" — so a simulation that starts
automation and also steps prices manually gets two goroutines writing
`emaBasis` with no synchronisation, and `go test -race` will only catch it once
someone writes that combination. Fix by giving stateful calculators their own
`sync.Mutex`; the critical section is a handful of arithmetic ops, so contention
is irrelevant.

### 7.3 What to add

Everything below is new files in `price/`, constructor-injected, requiring no
edits to existing library code and no changes to the `MarkPriceCalculator`
interface. That last constraint matters: `Calculate(book)` gives no timestamp
and no funding rate, and the fix is **composition, not interface widening** —
pass a `Clock` and a funding-rate accessor into the constructor. Widening the
interface would break every existing calculator and every user implementation,
which the library-first rule forbids.

**`BasketIndex`, implementing `types.PriceSource`.** The single highest-value
addition, because §6 says every real failure was an index failure. Structure it
so the two policy decisions are injected rather than hardcoded, since venues
genuinely disagree on both (§5):

```go
// Deviation policy: clamp-to-median (Binance, OKX, Deribit) vs
// exclude-with-decay (Bybit). Users pick; neither is hardcoded.
type DeviationPolicy interface {
	Adjust(price, median int64, weight int64) (adjPrice, adjWeight int64)
}
```

with `ClampToMedian(thresholdBps)` and `ExcludeWithDecay(thresholdBps, decay)`
as provided implementations. Per-source state: last price, last update
timestamp, base weight. The pass is: drop sources staler than
`stalenessNanos` → compute median of survivors → apply the deviation policy →
weighted average → if survivor count is below `minSources`, return 0 so the
caller's existing zero-check triggers its own fallback. Returning 0 rather than
a bad price is the right contract here, because every calculator in
`price/calculators.go` already treats `indexPrice == 0` as "fall back to book
mid" and that path is well-tested.

**`BinanceMedianMarkPrice`.** Needs `Clock` for the funding ramp and a funding
rate source, both constructor-injected. $P_2$ is `TWAPMarkPrice` with an
effectively infinite band — reuse rather than reimplement. $P_3$ is
`book.GetLastPrice()`. Then `median3` from `price/price.go:14`, which is correct
(verified against the permutations, including the two that exercise the third
comparison). Implement the §1.3 fallback ladder explicitly: last-trade of 0 must
route to $P_2$, not into `median3`, or the mark silently becomes
$\min(P_1, P_2)$.

**`ShrunkBasisMarkPrice`** — Bybit's post-November form, and the one I would
recommend as the eventual default over `ClampedEMAMarkPrice`. Implemented on
the reduced identity from §3.2, which avoids computing $P_3$ at all:

$$
\text{Mark} = I + C \cdot \operatorname{MA}(\Delta P),
\qquad C = \operatorname{clamp}\!\left(\frac{|\Delta P|}{\Delta P_{\max}}, c_{\min}, c_{\max}\right)
$$

State: the existing ring buffer from `TWAPMarkPrice` for
$\operatorname{MA}(\Delta P)$, plus a second ring over the $R$-minute window for
the running max of $|\Delta P|$ excluding the newest sample. It self-calibrates
per symbol — a habitually wide-basis instrument gets a large $\Delta P_{\max}$
and therefore small $C$ — so it needs no per-symbol tuning, which is exactly
what a simulation with heterogeneous synthetic instruments wants. Guard
$\Delta P_{\max} = 0$ (flat book, or fewer samples than the window) by falling
back to $c_{\min}$.

**`DepthWeightedMid`, a `BookProvider`-level helper.** Deribit's 1 BTC-deep
bounded mid, parameterised by depth rather than hardcoded to 1 BTC. This is the
cheapest single robustness win available: every basis sample in every calculator
currently reads `book.GetMidPrice()`, so one thin quote at a silly price
contaminates the sample. Walking a configurable depth into each side kills that
whole class of manipulation, and it composes with all the calculators above
because it changes the input, not the formula.

### 7.4 Suggested ordering

The band and window recalibration in §7.2(a) and §7.2(b) are one-line config
changes with immediate effect on every existing simulation, and should land
first. The race in §7.2(c) is latent but will surface under `-race` as soon as a
test drives `UpdatePerpPrices()` alongside automation, so it should land with
them. `BasketIndex` is next on the evidence of §6 — our `MidPriceOracle`
(`price/oracle.go:4`) is a single-source oracle, which is the §6.3 failure mode
verbatim. `ShrunkBasisMarkPrice` and `DepthWeightedMid` follow.
`BinanceMedianMarkPrice` is last: it is the most-cited formula but the least
useful to us, because its distinguishing term is a funding ramp that only
matters in the minutes before settlement, and its median defence is weaker than
the shrunk-basis form for a simulated book that may have no trades at all.

---

## Sources

Binance:
[Mark price and price index, USDⓈ-M futures](https://www.binance.com/en/support/faq/what-are-mark-price-and-price-index-in-usd%E2%93%A2-margined-futures-360033525071),
[Mark price and price index, COIN-M contracts](https://www.binance.com/en/support/faq/mark-price-and-price-index-of-coin-margined-contracts-fff6ec1b3f5845d19f1fb924644a0877),
[Funding rate formula and mark price update, 2025-09-16](https://www.binance.com/en/square/post/09-16-2025-binance-to-update-funding-rate-formula-and-mark-price-calculations-29758484976186),
[Index price and mark price API reference](https://developers.binance.com/docs/derivatives/coin-margined-futures/market-data/rest-api/Index-Price-and-Mark-Price).

OKX:
[Mark price and last price](https://www.okx.com/en-us/help/ii-mark-price-and-last-price),
[Spot index prices](https://www.okx.com/en-us/help/i-spot-index-prices),
[Indices computation rules](https://www.okex.com/support/hc/en-us/articles/115001762951),
[Perpetual futures guide](https://www.okx.com/en-us/help/i-perpetual-swaps).

Bybit:
[Mark price, perpetual and expiry contracts](https://www.bybitglobal.com/en/help-center/article/Mark-Price-Calculation-Perpetual-Contracts/),
[Mark price, perpetual and expiry contracts (mirror)](https://www.bybit.com/en/help-center/article/Mark-Price-Calculation-Perpetual-Expiry-Contracts),
[Adjustments to mark price calculation for perpetual contracts](https://announcements.bybit.com/en/article/adjustments-to-mark-price-calculation-for-perpetual-contracts-blt272cffde1403608d/),
[Index price calculation](https://www.bybit.com/en/help-center/article/Index-Price-Calculation).

Deribit:
[Mark prices](https://support.deribit.com/hc/en-us/articles/25944746962973-Mark-Prices),
[Index prices](https://support.deribit.com/hc/en-us/articles/25944739377309-Index-Prices),
[Linear perpetual](https://support.deribit.com/hc/en-us/articles/31424969384605-Linear-Perpetual),
[Inverse perpetual](https://support.deribit.com/hc/en-us/articles/31424954847133-Inverse-Perpetual),
[Crypto derivatives exchanges: liquidation pioneers](https://insights.deribit.com/market-research/crypto-derivatives-exchanges-liquidation-pioneers/).

Incidents:
[The Bitstamp flash crash: why robust indices matter](https://bravenewcoin.com/insights/the-bitstamp-flash-crash-why-robust-indices-matter),
[Flash crash causes \$200m BitMEX liquidation](https://www.financemagnates.com/cryptocurrency/news/bitcoin-flash-crash-causes-200m-liquidation-on-bitmex/),
[CFTC charges Avraham Eisenberg over Mango Markets](https://www.cftc.gov/PressRoom/PressReleases/8647-23),
[How low liquidity led to Mango Markets losing over \$116 million](https://cointelegraph.com/news/how-low-liquidity-led-to-mango-markets-losing-over-116-million),
[Oracle manipulation attacks rising](https://www.chainalysis.com/blog/oracle-manipulation-attacks-rising/),
[Binance pays \$283 million in compensation following Friday's depegs](https://www.theblock.co/post/374295/binance-pays-283-million-in-compensation-following-fridays-depegs-covering-user-losses),
[What is October 10th? Crypto's 10/10 mass market liquidation event](https://www.coingecko.com/learn/october-10-crypto-crash-explained).

Comparative:
[Comparing anti-manipulation approaches in Bitcoin futures products](https://bitcoinfuturesinfo.com/articles/comparing-anti-manipulation-approaches-in-bitcoin-futures-products).
