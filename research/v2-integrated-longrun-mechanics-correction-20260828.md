# Integrated V2 exact linear accounting correction

Date: 2026-08-28  
Parent failure: [`v2-integrated-longrun-v3-failure-20260828.md`](v2-integrated-longrun-v3-failure-20260828.md)

## Scientific decision

The v3 residual was a repeated loss of weighted cost-basis precision, not an
event omission. The correction preserves the public average-cost `EntryPrice`
display while making a signed aggregate price-times-quantity numerator the
accounting authority for linear margined instruments.

The policy is deterministic integer-lattice accounting: partial reductions
allocate the remaining basis proportionally with toward-zero division; realized
cash is the toward-zero cumulative lifecycle total; sub-cash-unit lifecycle
remainders carry across flat, flip, and reopen transitions. Exact marked PnL,
expiry cash flow, and liquidation thresholds use the same state. Liquidation
returns the integer price boundary whose next adverse tick crosses the
toward-zero equity condition.

## Fail-closed mechanics

The optional exact store interface is all-or-nothing and embeds the unchanged
`PositionStore`; `PositionDelta` was not extended, preserving existing unkeyed
literals. Strictness travels in `SettlementContext`, so a custom exact store
cannot silently fall back to legacy accounting. Exact transitions are dry-run
checked for quantity direction, `MinInt64`, size overflow, and representable
entry price before public position and accounting state commit.

Only `Margined` linear instruments register precision. Options retain their
premium/intrinsic accounting and legacy valuation. Successful delisting clears
the symbol precision registration. Exact expiry performs a non-mutating full
terminalization preview, checks clients and client/venue arithmetic, then
compare-and-clears the predicted carry before applying rounding ledger events.

## Evidence contract change

`position_rounding` events now carry timestamp and quote asset. The Go
conservation analyzer checks bounded remainders (`abs(remainder) < precision`),
one terminal event per venue/client/symbol/timestamp/asset, `perp` balance
links, `fee_revenue` venue links, missing/wrong links, relisting timestamps,
and checked aggregate arithmetic. The registered extractor requires the new
fail-closed predicate.

## Verification

The independent Sol-xhigh reviewer ACCEPTed the complete current diff after
the mechanics and audit corrections. Focused exchange, instrument, types, and
analysis tests pass. The complete multivenue suite passes in about 192 seconds;
the accepted-tree repository `make test` gate passes with `GOMAXPROCS=4`,
`CGO_ENABLED=0`, and no swap activity. No holdout seed was read.

The source correction is not yet a V2 freeze result. A fresh clean,
provenance-pinned development run must pass the amended conservation bound
before cells 613/617, parity, freeze, or holdout authorization.
