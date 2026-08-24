# V2 signed-price post-merge hardening ledger

Status: **open — dedicated branch `autoresearch/v2-signed-price-hardening`**

## Scope and provenance

The original signed-price migration was completed on `v2/signed-price`
(`cc91896`) and merged through `320262e`.  It established signed `int64`
storage, explicit `types.ErrNoPrice` availability, `types.PriceDomain`, a
full-range midpoint primitive, signed dated-future settlement, and a
positive-world equivalence gate.  This is not a second representation
migration.

This ledger audits code introduced after that merge, plus wire and actor
boundaries that the first audit claimed but did not mechanically cover.  Its
purpose is to prevent later V2 work from making a present `0` or negative
price look absent.  Parent for equivalence is `99ce69c`; this branch changes
neither P0/P2/L1 economics, event scheduling, RNG, nor timing.

The contract remains:

```text
numeric price (int64)  != availability (error or explicit presence state)
                       != instrument/model admissibility (PriceDomain/policy)
```

## Repository-wide sign-test classification

The inventory was produced with a production-Go `rg` over price-bearing names
and comparisons to zero, then read at every semantic boundary.  Quantity,
timestamps, identifiers, precisions, tick sizes, rates, and protocol-only
market-order `Price == 0` are not price-availability tests.

| Area | Current classification | Required disposition |
| --- | --- | --- |
| `types/price_domain.go`, `instrument/{spot,perp,option,listing}.go` | **B — explicit instrument/model domain.** Spot/perp/strike require positive values; option premium is non-negative. | Retain. These predicates must never be used to infer missing data. |
| `types/{math,interfaces}.go`, `book`, `matching`, `exchange/order_handling.go`, `exchange/spot_execution_plan.go` | **B/C — signed ordering, market-order protocol, or checked arithmetic.** | Keep signed representation and checked arithmetic; regression tests already cover signed matching. |
| `exchange/{borrowing,valuation}.go`, `instrument/funding.go`, `price/{black76,sabr,volatility,calculators,basket}.go` | **B/C — declared positive collateral/index/model/ratio domain.** A source error means unavailable; a present non-positive input is `ErrPriceDomain`. | Retain named boundary. No `abs`, clamp, or zero fallback. |
| `exchange/expiry.go`, `instrument/{dated,option,settlement_obs}.go` | **A — lifecycle evidence wire defect** for `InstrumentAnnouncement.SettlementPrice,omitempty`; numeric settlement itself is signed and present. | Fix with explicit availability on the persisted announcement and fixtures for unavailable/negative/zero/positive values. |
| `simulations/multivenue/sim.go` `twoSidedMark` / `valuationMark` / risk capture | **A — sign currently participates in cache presence.** The caller is a positive USD valuation contract, but cached signed data must remain distinct from absence. | Add explicit cache presence; apply the positive valuation-domain check only at the valuation boundary and report `ErrPriceDomain`, not a missing mark. |
| `simulations/multivenue/router.go` `crossVenueQuoteBook.best` | **A — bid/ask selection initializes from numeric zero.** Entirely negative or zero-spanning books select the wrong touch or report no book. | Track bid/ask presence explicitly. The current router's fee/notional edge is still a **B/C positive-spot policy**; reject a present out-of-domain touch separately from missing book. |
| `simulations/feesim/arb.go` `quoteBook.best` | **A** for touch selection; `executableEdge`, `gainBps`, post-second-leg pricing are **B/C** current positive spot/perp policy. | Same explicit touch presence fix. Preserve strategy domain and make non-positive present values a named domain deferral, not an empty book. |
| `simulations/{multivenue,feesim,randomwalk,derivsim}` maker/taker/arb caches | **B/C** where the actor is deliberately crypto/Black-76/ratio positive-domain; several legacy structs still use raw `0` cache sentinels. | Do not make these actors signed-future strategies on this branch. Replace only caches feeding generic book/reference boundaries; name every remaining positive-domain helper/policy. |
| `analysis/{basis,surface,stylized,reaction,triangular,arbitrage,margin_check,mechanical}` and `cmd/mvanalyze` | **C — statistic/model domain.** Log returns, BPS, IV, positive margin, and triangle ratios are undefined at/crossing zero. | Preserve signed raw observations; expose undefined/skip only at the named mathematical metric boundary. Audit uncovered blanket input filters in a later analyzer-only slice. |
| `analysis/{replay,bookshape,crossvenue,viability,spacing,impact}` | **A/B mixed.** Book presence must come from levels/presence bits, while spread/ratio normalization may have a positive denominator domain. | Verify all raw replay paths retain signed levels; patch any remaining sign-as-book-presence finding. |
| `simulation/{decision_frontier_vectors,gateway}.go`, `analysis/frontier_vectors.go` | **D — protocol state.** `Market` request price is zero because type identifies it; no availability claim. | Retain and test separately. |

## Migration slices

1. **H1 lifecycle wire:** make settlement availability explicit on
   `InstrumentAnnouncement`; preserve numeric signed settlement exactly.
2. **H2 executable-book presence:** repair multivenue router and fee-sim
   basis-book touch selection; distinguish missing vs present/out-of-domain.
3. **H3 valuation/actor cache boundary:** remove sign-as-cache-presence from
   V2 multivenue marks; retain declared positive USD valuation semantics.
4. **H4 regression gate:** focused/race/full tests, signed fixtures,
   fresh-process positive-world execution/evidence equivalence, and
   parent/branch performance comparison.  Append results to
   `v2-signed-price-audit.md`, then merge only if exact positive behavior is
   unchanged.

## Explicit non-goals

- No new signed-price funding, Black-76, SABR, crypto collateral, or
  cross-venue-router economic model is implied by signed storage.
- No re-run of the completed zero-valued-enum audit.
- No P2/L1-P3 population, phase, clock, latency, or economic tuning.
- No compatibility claim for a ratio across zero: the metric must state that
  it is mathematically undefined.
