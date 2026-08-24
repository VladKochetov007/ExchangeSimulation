# V2 signed-price audit

Status: **pre-implementation audit complete; migration not yet started.**

This branch starts at `198a3b2`, after the P0 passive-refresh result. It does
not modify P0 economics. The earlier
[`v2-price-api-audit.md`](v2-price-api-audit.md) closed the different,
positive-domain V2 task: eliminate numeric *absence* sentinels. Its claim that
a successful `PriceSource` is positive is intentionally superseded here.

## Representation decision

The engine's economically meaningful price storage is already `int64`, so it
is already a signed machine representation. This migration will retain that
canonical representation rather than introduce a nominal `type Price int64`
that would require pervasive casts without preventing a single availability
mistake. Auditability comes from two explicit contracts instead:

```text
price numeric value: int64, may be negative / zero / positive
availability:       error (or a documented bool only for in-process cache state)
admissibility:      instrument price-domain policy, not sign
```

An unavailable price is **only** a non-nil error at API boundaries. A returned
numeric zero with a non-nil error is not a price. Market-order
`OrderRequest.Price == 0` remains a protocol field distinguished by
`OrderRequest.Type == Market`, not by availability.

`int64` numeric extrema are part of the representation. A financial operation
whose notional/risk magnitude cannot be represented must return an explicit
overflow/error result; it must not reinterpret `MinInt64`, zero, or a wrapped
value as a price.

## Required domains

| Contract | Current state | Signed-price target |
| --- | --- | --- |
| spot cash pair (BTC/USD, ABC/USD, CDF/USD) | `price > 0` | remains strictly positive by its instrument policy |
| current crypto perpetual | inherits strictly positive spot validation | remains strictly positive by its instrument policy |
| dated future | inherits positive perp validation | explicit contract policy; crypto dated futures remain positive, commodity dated futures can opt into signed-or-zero tick prices |
| option premium | `price > 0` | non-negative tick price: zero premium is valid, negative premium is not |
| option strike | positive implicit arithmetic assumption | explicitly strictly positive; it is not an option premium |
| book/order/execution/last-trade/reference/mark/index/settlement storage | signed `int64` but success is often guarded with `> 0` | signed numeric storage, with absence only via error/availability; consumer validates its own mathematical/economic domain |
| Black-76/SABR/log-return/funding-premium ratio | positive guard | remains explicitly positive-domain; zero/cross-zero result is undefined/error/omitted metric, never `abs`-patched |

The first signed fixture will be a dated future whose declared price domain
allows negative and zero. It does not make spot, crypto perp, option premium,
or Black-76 accept economically invalid values.

## Complete public price API call graph

| API / field family | Current consumers | Current no-price / sign behavior | Required migration |
| --- | --- | --- | --- |
| `OrderBook.GetBestBid`, `GetBestAsk`, `GetLastPrice` | true mid, executable preview, last-price calculator, marks | error for `<= 0` | error only for absent side/trade; return any valid stored signed price |
| `OrderBook.GetMidPrice` | `DefaultExchange.MidPrice`, `MidPriceOracle`, mark calculators, book reference | needs positive uncrossed sides; unsafe across full signed span | needs only both sides and `bid <= ask`; use proved full-int64 midpoint primitive |
| `DefaultExchange.bookMidPrice[Locked]` | listing, index/mark path, settlement observer | converts non-positive into `ErrNoBookPrice` | preserve strict two-side rule; do not sign-test availability |
| `bookReferencePrice[Locked]`, `liveBookReferencePrice` | derivative/index source, valuation/cross margin | two-side mid then one positive side | named one-sided policy remains; side availability is pointer/error, not sign |
| `PriceSource.Price`, `ListingPriceSource.Price` | static/mid/basket/degraded index, borrowing, fee source, listing, mark calculators | docs and several implementations treat `<= 0` as unavailable | successful source value may be signed; each consumer applies its declared domain or returns error |
| `marketRefPrice` historical name / current `liveBookReferencePrice` | valuation and initial market risk | prior hidden zero/last fallbacks removed | keep explicit source naming; no generic fallback |
| `OrderRequest.Price`, `Order.Price`, `Limit.Price`, `Execution.Price`, `Trade.Price`, `PriceLevel.Price` | matcher, settlement, logs, actor caches, analyzer replay | signed storage; order validation and preview reject/skip `<= 0` | retain signed storage; validate limit order through instrument policy; market request zero remains protocol-only |
| `FundingRate.MarkPrice/IndexPrice` plus availability booleans | funding, margin, MD, analyzers | numeric zero cleared with unavailable flag | preserve explicit booleans; signed successful values allowed only where a funding model defines them |
| `SettlementPrice`, observer state, lifecycle announcement | expiry, option exercise, dated delivery | observation ignores `<=0`; settlement rejects mean `<=0` | error only for absent observation; signed dated settlement allowed by contract; option underlying retains its declared positive model domain |
| `PositionMark`, `MarkPrice`, `LiquidationPrice` | PnL, liquidation, account display | pointers already distinguish display absence, but risk paths sign-test | retain pointer/error availability; split signed PnL cash flow from non-negative risk magnitude |
| actor local mid/touch caches | makers, arbs, supplier, metaorder, bootstrap, router | `0`/sign used as unavailable and many manual `(bid+ask)/2` operations | cache explicit side/frontier availability; shared safe midpoint; each actor declares positive-only or signed-future policy |
| analyzer-derived prices | impact, viability, basis, surface, stylized, replay | many `<=0` filters are implicit availability | price-independent book/trade replay becomes signed; ratio/log/IV metrics return undefined outside their mathematical domain |

## Repository-wide sign-test inventory and classification

The following is the audit inventory from `rg` over production Go files. Test
fixtures are included only when they encode a domain assumption that must be
revised.

| Area / locations | Existing predicate role | Classification / signed migration decision |
| --- | --- | --- |
| `book/orderbook.go`, `exchange/expiry.go`, `price/oracle.go`, `price/price.go`, `price/basket.go`, `exchange/borrowing.go` | `price <= 0` means unavailable | **availability test — remove.** Source absence must be an error; downstream collateral/risk domain remains explicit. |
| `instrument/spot.go`, `instrument/perp.go` inherited validation, `instrument/option.go`, `instrument/listing.go` strike test | validate an admitted order/strike | **instrument domain — retain explicitly.** Spot/perp/strike strictly positive; option premium non-negative. |
| `instrument/dated.go`, `instrument/settlement_obs.go`, `instrument/perp.go` margin/funding | settlement, margin, fee, funding treats sign as invalid | **arithmetic/economic policy — redesign.** Dated future may be signed; variation PnL signed; margin and delivery fee use a defined non-negative exposure magnitude with overflow handling. Funding remains positive-domain until a signed-price funding model is separately defined. |
| `exchange/order_handling.go`, `exchange/spot_execution_plan.go`, `exchange/settlement.go`, `fee/*.go` | preflight/matching/fee/notional rejects `price <= 0` | **mixed.** Matcher/execution must admit a signed-domain instrument; positive-domain fee schedules must not issue a negative fee. Cash settlement is signed only for instruments whose policy permits it. |
| `matching/*.go`, `book/book.go` | numerical order comparisons, map keys, pool resets | **ordering / legitimate reset.** Price ordering already uses signed comparisons and works for negative levels; pooled reset to zero is not availability. Add signed matcher fixtures. |
| `price/black76.go`, `price/sabr.go`, `price/volatility.go`, `instrument/funding.go`, `analysis/stylized.go`, `analysis/surface.go`, `analysis/basis.go`, `analysis/triangular.go` | positive forward/strike/log/ratio denominator | **mathematical domain — retain explicitly.** Report undefined/error/omitted observation at or across zero; do not use absolute values. |
| `price/calculators.go`, `price/venue_marks.go` | mark/index clamp, EMA/TWAP | **model-domain dependent.** Current models are positive-index models; return explicit unavailable/undefined if their configured reference domain is not positive. |
| `simulations/multivenue/{stoikov,naive,latent,bootstrap,carry,router,supplier,metaorder,anchor,local_book_cache}.go`, `simulations/{feesim,randomwalk,derivsim}/*` | local cache/touch/ref validity, tick rounding, quote policy | **actor policy + arithmetic.** Replace sentinel side checks with explicit cache availability. Positive-only actors defer on a signed reference outside their declared domain; a future signed-future maker gets its own policy. All manual midpoint/ceil-floor arithmetic needs a signed-safe primitive. |
| `analysis/{replay,viability,spacing,crossvenue,reaction,resting,sweep,arbitrage}.go` | snapshot replay / best price / analysis filtering | **mixed.** Replay and executable-book metrics must support signed levels without sentinel zero; percentage/log/IV metrics remain undefined where denominators/models require positivity. |
| `types/market_data.go`, `types/events.go`, `actor/events.go`, account reports | serialized numeric field plus availability boolean/pointer in some cases | **representation.** Keep signed `int64`; add/retain explicit availability only where no-price can occur. Do not encode an absent signed price as zero. |

Non-price constraints such as `qty <= 0`, tick/precision validity, rate bounds,
or map/pool zero initialization are not price-domain changes. They are left
alone unless a line combines them with a price availability assumption.

## Arithmetic and lifecycle hazards found before edits

1. `bid + (ask-bid)/2` is safe only when `ask-bid` cannot overflow. It fails
   for `MinInt64, MaxInt64`. The signed midpoint primitive must use an exact
   decomposition or wider arithmetic and specify truncation toward zero for
   an odd signed half-distance.
2. Dated `DeliveryFee` currently multiplies by signed settlement directly;
   a negative settlement would create a negative fee. The migration must name
   its non-negative settlement exposure magnitude and reject an unrepresentable
   magnitude rather than use accidental two's-complement `abs`.
3. Perp/dated margin currently assumes non-negative price. A signed dated
   future needs signed variation cash flow but non-negative initial/
   maintenance exposure. `MinInt64` needs an overflow result, not `-MinInt64`.
4. Spot settlement currently treats price as a quote transfer. Negative cash
   settlement is not enabled for spot by this branch; positive spot domain
   prevents a silent reversal of buyer/seller cash legs.
5. Option seller margin, delivery fee, Black-76, SABR, and implied-volatility
   inversion require explicitly positive underlying/strike/model inputs. Zero
   option *premium* is valid, but it is not evidence that the underlying is
   available or Black-76-defined.
6. Percentage returns, log returns, funding premium, basis bps, relative
   spread, and several actor percentage controls divide by price. At/crossing
   zero they must expose an undefined result or defer, never silently use
   `abs(price)`.
7. Existing fee and risk code uses `MulDiv`/`TryMulDiv` in many paths. Each
   signed-domain operation will be tested for signed cash flow versus
   non-negative requirement before broad conversion; no raw `qty*price`
   product is accepted as proof of safe arithmetic.

## Migration order and gates

1. **S1 — price primitive and book/matcher:** signed-safe midpoint, explicit
   price-domain policy, signed best/mid/reference APIs, negative book matcher
   fixtures, positive-world equivalence.
2. **S2 — dated future accounting:** signed settlement observation, variation
   PnL, explicit magnitude for margin/delivery fee, overflow tests, exact
   delayed-settlement lifecycle regression.
3. **S3 — external sources/marks/actors:** remove sign-as-absence from
   `PriceSource`, index/reference cache and client preflight; retain explicit
   model-domain deferrals and observable reasons.
4. **S4 — analytics:** signed replay/book metrics; explicit undefined domains
   for log/ratio/Black-76/IV statistics.
5. **S5 — integration:** all required unit/race/mechanical/lifecycle/risk/
   V2-receipt tests, fresh-process determinism, positive-world execution and
   evidence equivalence, and before/after profile.

Every S-stage is a separate commit. No P0 setting, scheduler order, RNG use,
or actor timing changes belong to this branch.
