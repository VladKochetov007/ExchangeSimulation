# ME-005 implementation checkpoint — development readiness, not a result

Source baseline: merged `main` `eb921aa29b26b43cd39e1613dabc44ec60dbf5c9`.
Current checkpoint: `6d0ab92` on `research/market-ecology-me005-20260924`.
The authoritative [readiness assessment](readiness-20260924.md) and
[prospective preparation contract](prepare-contract-20260924.md) still define
four acceptance fixes. This note records implementation progress without
registering a protocol or scoring a market world. ME-005 economic worlds run:
**zero**. Holdouts consumed: **zero**.

| Fix | Implemented and mechanically exercised | Still required before ME-005 RUN |
|---|---|---|
| 1. Two-venue wiring | Exactly two distinct venues and finite, separate router accounts; historical three-venue default retained. Two-venue router configs now require explicit `auto_borrow_spot=false`. | Pin the effective two-venue config, initial balances, lot, attempt cap and deployment in a preregistration; independently review this successor population choice. |
| 2. Opportunity/execution reconstruction | Optional canonical callback evidence including no-action and consumed source identity; public-book replay in global frame order; receipt-prefix and actor-local book checks; actor-inbox response receipts; terminal count binding; first trade ID zero, Trade/OrderFill/response joins; targeted corruption fixtures. | Complete request/decision-vector to admission/rejection/cancellation and FOK terminal joins; prove the public→delivered→evaluated→feasible→attempted opportunity denominator and horizon censoring. Reconcile complete evidence under the registered config and verified run manifest. |
| 3. Costed closeout | Independent venue-local bid/ask depth walk with quote taker fees and insufficient-depth failure; initial/terminal venue-account deltas independently checked against settled fills, fee rounding and zero debt/locked inventory. A static fixture gives +48 matched cashflow but −13 local terminal liquidation value. | Bind both displayed books and both account snapshots to the same registered terminal state; define and test quote freshness, pending-request status and unavailable-closeout classification. Verify complete movement/conservation evidence and absence of financing/transfer flows over the interval. |
| 4. Dislocation measurement | Two-venue, event-ordered, one-lot executable edge and half-open episode functions with same-time tests. | Build and test the registered paired OFF/ON world-level estimator, timing/attribution rules, missing-run handling and no-op controls. Keep trade-attributed convergence `NOT_IDENTIFIED` if the retained evidence cannot distinguish it from background movement. |

The positive matched cashflow in the static fixture is **not** profit after
restoring prefunded venue-local inventory. For an unchanged uncrossed book,
a buy at venue A's ask followed by a local sale at A's bid is nonpositive;
a sell at venue B's bid followed by a local repurchase at B's ask is also
nonpositive, before nonnegative fees. The cross-venue quote gap cancels from
the two local round trips. A later positive terminal value may reflect
favorable local book movement; it is not realized transferable arbitrage
profit from the original two legs. No transfer policy is present in this
candidate, and this limitation must survive into the preregistration and
report regardless of observed outcomes.

## Independent design challenge, bounded scope

Fresh read-only Sol-6 medium reviewer `01a0d33b-357d-72c3-86fb-e28989e6f204`
examined exact committed source `2b2b84b`, the readiness/preparation contracts,
router, fee, closeout and account/fill paths. Review execution: `COMPLETED`.
Verdict scope: **DESIGN COHERENT AS HYPOTHETICAL LOCAL TERMINAL VALUATION;
NOT REALIZED TRANSFERABLE ARBITRAGE PROFIT.** The reviewer independently
derived the stationary-book nonpositivity above and flagged terminal book/
account synchronization, full order-outcome and financing/transfer checks as
remaining prerequisites. Uncommitted account changes visible during that
review were explicitly excluded. This was not Reviewer A's final review of
four completed fixes, not Reviewer B's causal/statistical review, and not an
authorization to preregister or run.

## Verification and provenance limits

At `6d0ab92`, a clean `GOMAXPROCS=7 make test` passed, including the integrated
long-run contract and archive fixtures. Focused race checks, `go vet` for
affected packages and `git diff --check` also passed at their respective
checkpoints. The synthetic two-venue fixture exercises binary rendering,
receipt/callback/source joins and strict initial/terminal account capture;
its ten-second test horizon is not an economic development world. The static
positive-edge fixtures prove arithmetic and evidence handling only.

No ME-005 final preregistration, two final independent reviews, new development
seed, OFF/ON comparison, result, freeze or holdout claim exists. ME-001/002/003
and historical three-venue results remain attached to their original source
and experiment contracts.
