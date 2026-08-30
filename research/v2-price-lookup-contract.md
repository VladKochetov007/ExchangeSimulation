# V2 price-lookup contract

This contract applies only to the in-progress V2 branch. It does not amend
frozen `ae13f9a` results.

## Price categories

| Need | API/policy | Absence |
| --- | --- | --- |
| True midpoint | `OrderBook.GetMidPrice`, `DefaultExchange.MidPrice`, `bookMidPrice`, `MidPriceOracle` | error unless positive, uncrossed bid and ask exist |
| One-sided reference | `bookReferencePrice`, `liveBookReferencePrice` | true midpoint first; otherwise sole ask then sole bid; error if neither |
| Last trade | `OrderBook.GetLastPrice`, `LastPriceCalculator`, named Binance protected-mark policy | error unless a positive prior trade exists |
| Configured external/index | `PriceSource.Price`, `configuredIndexPrice`, basket/index providers | error unless the named source yields a positive price |
| Executable price | best bid/ask or detached-match executions | reject/defer when no displayed executable liquidity exists |
| Settlement source | delivered `ObserveSettlement` observations | lifecycle becomes `SETTLEMENT_PENDING`; no generic mark/last-trade/zero fallback |

No helper called “mid” may hide a one-sided or last-trade source. No caller may
turn an error into numeric zero.

## Arithmetic and domain

True midpoint is intentionally:

```text
bid + (ask - bid) / 2
```

not `bid + (bid - ask)/2`. Exchange-admitted prices are positive `int64`s and
an uncrossed book satisfies `bid <= ask`; therefore `ask-bid` cannot overflow.
Integer division floors an odd one-tick spread. The same form is used in local
participant cache arithmetic.

## Lifecycle rule

Expiry immediately disables new trading. If the declared settlement source is
unavailable, the contract is permanently halted in `SETTLEMENT_PENDING` under
the explicit `RETRY_FOREVER` policy. Retried settlement is idempotent and
settles exactly once when the declared reference becomes available. Funding,
mark updates, liquidation, and post-expiry fills do not continue during this
state. A source that never becomes available leaves the contract pending; any
future terminal fallback requires an explicit new policy.

### Pending-exposure risk boundary

`SETTLEMENT_PENDING` does not mean that a retained position has zero value or
zero liability. It means that the position has no valid current valuation. For
the corrected r5 candidate, any account profile containing nonzero exposure to
that pending contract fails closed for the whole account. The profile may not
silently omit the position while evaluating an active sibling, and no
liquidation, solvency, or margin verdict is admissible for that account until
the declared settlement source succeeds. The position and collateral remain
stored for the retry path; existing debt and other scheduled balance changes
may still evolve under their own contracts. New order and manual-borrow
obligations are blocked by this boundary. This is an explicit risk-isolation
policy, not a fallback valuation, and changing it requires a new lifecycle
preregistration.

## Client and autonomous behavior

- Price-dependent client fee/collateral/borrowing preflight rejects before any
  account, order-ID, reserve, loan, or book mutation.
- Strict account valuation returns a wrapped error. Ordinary account display
  uses nil marks plus an explanatory reason.
- Automatic listing, marks, funding, and actors defer only with a structured
  `price_unavailable` diagnostic; they never silently continue at zero.
- A fallback is named at its policy boundary. Generic midpoints do not select
  last trade, and configured fee sources do not quietly select a different fee
  schedule.

For the completed caller inventory, tests, V2 behavior changes, and remaining
legitimate zero quantities, see
[v2-price-api-audit.md](v2-price-api-audit.md).
