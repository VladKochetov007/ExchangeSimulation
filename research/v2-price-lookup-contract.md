# V2 price-lookup contract

## Scope

This V2 change removes the `0`-means-no-book convention from the exchange's
private `bookMidPrice` path. It does not modify frozen `ae13f9a`; it changes
the in-progress V2 branch before its next freeze.

## Contract

`DefaultExchange.bookMidPrice{,Locked}` now returns `(int64, error)` and
returns `ErrNoBookPrice` for a missing, empty, one-sided, crossed, or invalid
book. A successful result is the positive two-sided midpoint, computed as:

```
bid + (ask - bid) / 2
```

Integer division deliberately floors an odd one-tick spread. Live admitted
limit prices are positive `int64`s and an uncrossed book has `bid <= ask`, so
the subexpression `ask - bid` is in `[0, MaxInt64-1]`; it cannot overflow.
Boundary tests cover those invariants and a near-`MaxInt64` pair.

## Consumer decisions

| Consumer | Required price | Absence behavior |
| --- | --- | --- |
| Option-chain listing | true midpoint | defer listing; do not center strikes at zero or a one-sided quote |
| Dated/option settlement marks | declared underlying reference | defer sample/mark when no reference exists |
| Perp/future index, funding, margin, liquidation update | declared underlying reference | defer the complete mark/funding/margin update |
| Pre-existing configured index provider | explicit fallback | accept only positive provider values |

The historical derivative/index contract intentionally accepts a sole best bid
or ask. It is now represented by separately named
`bookReferencePrice{,Locked}`: true midpoint if two-sided, otherwise the sole
displayed best quote. An adversarial lifecycle fuzzer showed why this matters:
making settlement strictly two-sided caused sparse sampled futures to retain a
zero settlement price and then fail conservation. The named policy preserves
that existing economic behavior while making it auditable; it is not called a
midpoint.

## Deliberate semantic change

Automatic option listings now wait for both sides, where the old path could
center a strike grid from a one-sided pseudo-midpoint. This is a V2 economic
semantic change and needs activation/causal validation before the V2 freeze.
Missing or empty derivative references now explicitly defer rather than cross
a legacy `PriceSource` boundary as `0`.

## Adjacent legacy APIs, not silently changed

`OrderBook.GetMidPrice`, `DefaultExchange.MidPrice`, and the legacy
`PriceSource` calculators retain their documented mid-or-last-trade/zero
contract. `marketRefPrice` separately and explicitly permits a one-sided
valuation fallback for current order-margin and account-display paths. They
are outside the `bookMidPriceLocked` call graph and require a separate,
repository-wide error-aware price-source migration; this change neither
asserts that their zero sentinels are valid nor conflates them with a true
midpoint.

## Regression coverage

Tests cover midpoint arithmetic, admitted-price bounds, absent/one-sided/
crossed books, explicit one-sided references, option-listing deferral,
derivative-mark deferral, and index/mark propagation. The implementation has
no scheduler, RNG, actor-state, or concurrency changes.
