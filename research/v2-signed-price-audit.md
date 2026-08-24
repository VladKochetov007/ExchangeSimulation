# V2 signed-price audit

Status: **historical signed-price migration merged at `320262e`; post-merge
hardening gate closed at `5afdd45` and is an ancestor of the active V2 research
head.**

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

## Completed migration

The branch implements the staged contract above in small commits, starting
with `54a4d3e`. The most important completed boundaries are:

| Boundary | Completed contract |
| --- | --- |
| `PriceDomain` | Explicit `Positive`, `NonNegative`, and `Signed` domains with a tick. Storage remains signed `int64`; availability is never inferred from its numeric value. |
| book prices | Best bid, ask, last trade, strict midpoint, and named one-sided reference return present signed numeric values; unavailable book state is an error. |
| midpoint | `types.Midpoint` is correct over ordered `int64` endpoints and truncates the exact mean toward zero without forming an overflowing signed distance. |
| orders and matching | A signed-domain dated future admits negative and zero limits. Matching, post-only admission, detached IOC/FOK preview, market sweeps, price-time order, and self-trade prevention use signed ordering. |
| sources and marks | `PriceSource` succeeds for any numeric `int64`; `ErrNoPrice` alone signals absence. Positive index/borrow/fee/funding consumers use a named positive-domain boundary and get `ErrPriceDomain` for a present unsuitable value. |
| expiry, risk, analysis | Signed dated settlement and zero option premiums survive independently from availability. Analyses retain signed/zero values and annotate log/ratio/BPS/IV observations that are mathematically undefined. |

The call graph is deliberately not universally permissive. Current crypto
spot, current crypto perps, their funding premium calculation, positive
index-mark models, and Black-76/SABR remain positive-domain consumers. The
existing multivenue actors declare that positive-domain policy through named
helpers. A future commodity-future actor must select a signed-future policy
explicitly; it cannot obtain one by treating zero as cache absence.

### Arithmetic proof and fixtures

`types.Midpoint(a,b)` uses a bit decomposition of the exact two's-complement
sum, then corrects a negative odd floor mean to Go's truncation-toward-zero
result. Tests compare it with `math/big` over exhaustive small signed values
and `MinInt64/MinInt64`, `MaxInt64/MaxInt64`, `MinInt64/MaxInt64`, negative /
negative, negative / zero, negative / positive, zero / positive, equal
endpoints, ordinary positive endpoints, and odd signed distances.

`TryPriceChangeMulDiv` computes future PnL without forming
`settlement-entry` as an `int64`; it is tested against exact arithmetic across
the signed endpoint range. Risk magnitude uses `TryAbsMulDiv`, not an implicit
`abs(price)` substitution for cash-flow logic. Signed fixtures cover
`+20 -> -20`, `-20 -> +20`, `-20 -> -40`, and `-40 -> -20` for long and short
positions, negative-price queue priority, zero-crossing matching, negative
post-only, IOC/FOK preview, market execution, and self-trade prevention.

The independent settlement/exercise replayers had a separate defect: an
unrepresentable fixed-point `a*b/c` became numeric zero. They now report
explicit `unrepresentable` / arithmetic-failure fields, so a validator cannot
falsely accept a zero payout produced by its own overflow. The positive-price
ABC-perp margin replay also records and excludes a present out-of-domain mark
rather than treating it as missing.

### Explicit semantic changes

1. A dated future opting into `SignedPriceDomain` can trade and settle at
   negative or zero prices. Its cash PnL is signed while margin and delivery
   fee use their declared non-negative risk quantity.
2. A zero-premium option short now reserves its defined short margin; it is no
   longer incorrectly treated as a no-price/no-risk state.
3. An expired Black-76 option with only a zero/negative forward remains halted
   and visibly `SETTLEMENT_PENDING` until a valid declared reference arrives;
   there is no automatic last-trade or zero-price fallback.
4. A zero-priced signed-future liquidation fill is a real fill, not an empty
   book. Clearance fees use their explicit non-negative exposure base.

No P0 passive-refresh setting, spread, clock, inventory skew, population,
latency, scheduler ordering, or RNG draw changed on this branch.

### Legitimate numeric zero fields

- A `Market` order request price is a protocol placeholder identified by type.
- A signed dated-future order/trade/book/mark/settlement price may be zero.
- An option premium may be zero under its non-negative contract.
- Margin/risk/notional results, fees, PnL, funding rates, position/quantity,
  balance deltas, and timestamps may be zero when their own type defines it.
- In-process cache/frontier booleans are explicit availability state; no cache
  infers availability from numeric price.

## Positive-world equivalence and determinism gate

The comparison parent is P0 commit `198a3b2`. Both revisions ran the retained
representative V2 world:

```text
config: research/configs/frozen-baseline-2026-08-22.json
config SHA-256: ca933bf2244eec8e104d4313456bed386809703bb7a1179125b8d9255f1b1036
seed: 101
simulated horizon: 30 minutes
receipt evidence: spot_maker
```

With full raw logs at `GOMAXPROCS=1`, every persisted JSON record file and
receipt sidecar was byte-identical (679,873,179 raw venue bytes per run).
Both persisted-evidence artifacts state:

```text
domain: persisted_json_records / unordered_multiset
events: 2,126,782
digest: c530af24a5c75950e2090b95f858c271d70ab47e55b1e44d66ec531885f7bb75
```

With raw logging disabled and 60-second ordered checkpoints, all 31 checkpoint
records were byte-identical (file SHA-256
`abdc784b75645120ec0b3157f0b88af9b7ae5004888bce096d16f302dfcb7b03`). The
terminal execution attestation was identical:

```text
domain: execution_observations / ordered_stream
events: 2,126,782
execution_stream_hash: 1eb482c7d5a21a08092c751252ca31dc6e4a0b8decf50fedefa08b2904afb2c7
```

The candidate repeated the no-log run at `GOMAXPROCS=4`; its complete
checkpoint file, count, and hash were again exact. The signed representation
therefore does not change a positive-domain trajectory or make host
parallelism a model input.

Raw run/profile provenance is retained under:

```text
scratch/signed-price-parent.sHRI9N
scratch/signed-price-current.LRrZoX
scratch/signed-exec-parent.rqDzH4
scratch/signed-exec-current.h6hjky
scratch/signed-exec-gomax4.rwXPUH
scratch/signed-prof-parent.g2jUOy
scratch/signed-prof-current.11qyEy
```

## Performance gate

No performance optimization was introduced. The controlled no-log profile
comparison (`GOMAXPROCS=1`, receipt evidence and checkpoints enabled) is:

| Measure | `198a3b2` | signed branch | Difference |
| --- | ---: | ---: | ---: |
| wall time | 23.77 s | 24.21 s | +1.9% |
| simulated seconds / wall second | 75.72 | 74.35 | -1.8% |
| sampled allocation | 9.390 GB | 9.396 GB | +0.1% |
| peak RSS | 811,512 KiB | 827,796 KiB | +2.0% |
| execution hash | `1eb482…afb2c7` | `1eb482…afb2c7` | exact |

The full-log confirmation was 25.27 s versus 26.40 s (+4.5%), within observed
wall variation; its raw files and evidence digest remained identical. CPU
profiles remain dominated by order admission/matching, checkpoint JSON
canonicalization, and detached preview allocation. The branch introduced no
new material hotspot: `PlaceOrder` cumulative CPU was 32.9% versus 33.5%,
checkpoint observation 17.2% versus 17.0%, and sampled allocation was nearly
unchanged. Block profiles contain only pprof shutdown delay; mutex profiles
show no application contention.

This passes the branch criterion of no material measured regression. It does
not authorize a JSON-library replacement: `encoding/json` remains the evidence
reference; `goccy/go-json` remains rejected by overflow compatibility; jsoniter
and Sonic require broader differential screening before offline adoption.

## Tests and operational checks

- `gofmt`, `go vet ./...`, and `go test ./...` passed, including signed
  matcher, lifecycle, accounting, funding, margin/liquidation, analyzer,
  receipt/frontier, and multivenue fixtures.
- Race coverage passed for `types`, `book`, `matching`, `instrument`, `price`,
  `exchange`, `simulation`, `analysis`, `simulations/multivenue`, and `tests`.
- `multivenue`, `mvanalyze`, and `prunegate` were rebuilt from this branch.
- The hardened prune gate ran without `-prune`: frozen baselines 101/102/103
  are `SAFE_TO_PRUNE`; incomplete treatment rows remain blocked. No raw
  evidence was deleted.
- `golangci-lint` is not installed on this host. This is not represented as a
  lint pass; `go vet` is the completed static check.

## Remaining intentional limitations

1. A signed commodity future is mechanically represented and tested, but the
   current multivenue ecology does not instantiate an economically motivated
   commodity population. That is a later V2 mechanism, not a reason to loosen
   current crypto domains.
2. Percentage/log returns, basis bps, relative spread, funding premium, and
   Black-76/SABR/IV quantities remain undefined at or across zero. Analysis
   must report undefined coverage and never normalize with `abs`.
3. Options on a negative-capable future require an explicit normal (Bachelier)
   or shifted model. This branch intentionally does not make Black-76 accept a
   negative forward.
4. Cached in-process actor observations may use explicit availability booleans
   on hot paths. Public/client/action boundaries use errors; no cache derives
   availability from a numeric zero.

## Post-merge hardening, 2026-08-24

The original migration was already merged at `320262e`.  Later V2 work added
new evidence, routing, mark, lifecycle, and analysis boundaries, so a narrow
hardening branch (`autoresearch/v2-signed-price-hardening`, parent `99ce69c`)
was used to re-audit the claim rather than reopening signed-price economics.
It contains no P0/P2/L1 population, clock, latency, scheduler, RNG, or market
rule change.

### Findings and corrections

- `InstrumentAnnouncement.SettlementPrice` had a new wire ambiguity: an
  `omitempty` scalar could not distinguish absent settlement from a present
  zero settlement.  The final form is `*int64`: `nil` for nonterminal or
  unavailable, and a nonnil pointer for negative, zero, or positive terminal
  settlement.  Wire fixtures cover all four states.  The explicit
  `settlement_price_available` field was deliberately removed because writing
  it on every ordinary listing announcement changed otherwise unchanged
  evidence.
- Multivenue and fee-sim executable-touch selection now tracks side presence,
  so negative books and books spanning zero cannot be reinterpreted as empty.
  Current router/fee economics remain an explicit positive-spot policy and
  record a *domain deferral* separately from an unavailable quote.
- Risk bootstrap/mark capture, consensus-index input, settlement audit,
  liquidation fills, impact, arbitrage, triangular ratios, BPS basis, IV, and
  log returns now preserve signed raw values.  Where the statistic is not
  mathematically defined at/crossing zero, the result reports an explicit
  unavailable-domain count instead of filtering the observation as absent.

The commits are `1d731a7`, `82147bf`, `8bb7a72`, `fd6dda8`, `098e9af`,
`e4eecdf`, `4c71bfa`, `3859fbf`, `b3faf44`, `a982dd1`, `2fca45e`,
`665381e`, and `0f6a3c6`.  The first full equivalence run on the branch is
retained as an invalid diagnostic: it diverged at the first ordinary
`instrument_listed` announcement because it wrote settlement fields which the
parent did not write.  It is not evidence for or against economics.  Commit
`0f6a3c6` fixed the representation, after which the final gate below was run
from scratch.

### Corrected positive-world equivalence

The parent (`99ce69c`) and corrected candidate (`0f6a3c6`) ran the same
retained V2 world: `frozen-baseline-2026-08-22.json`
(`ca933bf…f1b1036`), seed 101, 30 simulated minutes, 60-second checkpoints,
and `spot_maker` receipt evidence.

| Contract | Result |
| --- | --- |
| ordered execution | 2,126,782 observations and `5db76448ebb8c5ca60d04366a5fe89540e745564c7fb86cc328be7515989e5f6` on parent and candidate |
| persisted evidence | 2,126,782 JSON records and unordered-multiset digest `f2544069b4d332985c25b4f9b3382cf61271a57f4feb8bd0cf6bd4b1bf20dd3d` on both |
| raw evidence | every persisted venue JSON file byte-identical |
| information evidence | receipt, schedule, decision, and frontier-summary sidecars byte-identical |
| logging/parallelism | candidate full and none logging at `GOMAXPROCS=1`, plus none at `GOMAXPROCS=4`, all produced the same complete checkpoint file and execution hash |

This is a behavior-preservation result for the existing positive-domain world,
not evidence that ratio-based funding or Black-76 support signed underlyings.
Machine-readable provenance, hashes, profile paths, and sampled timings are in
[`v2-signed-price-hardening-gate.json`](artifacts/v2-signed-price-hardening-gate.json).

### Performance comparison

The alternating no-log A/B/A/B runs were 29.39/27.70 seconds for the parent
and 26.48/25.52 seconds for the candidate.  The profiled single pair varied in
the opposite direction (29.82 vs 33.45 seconds), so wall time is treated as
noisy rather than as an improvement claim.  The more stable profiles show only
+1.44% sampled allocation and +0.55% peak RSS.  CPU remained dominated by
order admission/matching (about 41% cumulative), logging/`encoding/json`
(about 25–31%), and execution processing (about 21%); no signed-price-specific
hotspot appeared.  This passes the no-material-regression gate.

`encoding/json` remains the evidence reference.  No JSON library was adopted:
the previous `goccy/go-json` overflow incompatibility remains disqualifying,
and Sonic/jsoniter require a fresh analyzer-only differential gate after this
branch is merged.

### Remaining explicit domains

The generic infrastructure and persisted evidence now preserve all signed
values.  Existing Stoikov, fixed-distance, metaorder, and Black-76 actor paths
remain deliberately positive-domain crypto/model policies; their internal
reference helpers must explicitly defer/reject an out-of-domain *present*
price and cannot use it as a generic unavailable sentinel.  A signed commodity
future population, signed funding ratios, and options on negative forwards are
new economic mechanisms, not cleanup tasks.  They require their own declared
model and causal validation before V2-5/V2-6 claims can extend to them.
