# ME-005 implementation checkpoint — development readiness, not a result

Source baseline: merged `main` `eb921aa29b26b43cd39e1613dabc44ec60dbf5c9`.
Latest clean code gate: `1463586` on `research/market-ecology-me005-20260924`.
The authoritative [readiness assessment](readiness-20260924.md) and
[prospective preparation contract](prepare-contract-20260924.md) still define
four acceptance fixes. This note records implementation progress without
registering a protocol or scoring a market world. ME-005 economic worlds run:
**zero**. Holdouts consumed: **zero**.

| Fix | Implemented and mechanically exercised | Still required before ME-005 RUN |
|---|---|---|
| 1. Two-venue wiring | Exactly two distinct venues and finite, separate router accounts; historical three-venue default retained. Two-venue router configs now require explicit `auto_borrow_spot=false`. | Pin the effective two-venue config, initial balances, lot, attempt cap and deployment in a preregistration; independently review this successor population choice. |
| 2. Opportunity/execution reconstruction | Optional canonical callback evidence including no-action and consumed source identity; public-book replay in global frame order; receipt-prefix and actor-local book checks; actor-inbox response receipts; terminal count binding; first trade ID zero, Trade/OrderFill/response joins. Exchange-side market-FOK placement outcomes now join to settled full-lot fills and delayed actor acknowledgements, preserving missing inbox delivery. | Complete submitted request/decision-vector to placement and cancellation joins; prove the public→delivered→evaluated→feasible→attempted opportunity denominator and horizon censoring. Reconcile complete evidence under the registered config and verified run manifest. |
| 3. Costed closeout | Independent venue-local bid/ask depth walk with quote taker fees and insufficient-depth failure; initial/terminal venue-account deltas independently checked against settled fills, fee rounding and zero debt/locked inventory. The terminal helper requires both account timestamps at the declared horizon and bounds the age of each replayed book. A static fixture gives +48 matched cashflow but −13 local terminal liquidation value. | Pin the book-evidence recency bound prospectively; prove replay and accounts derive from one verified completed run; join pending-request/FOK terminal status. Verify complete movement/conservation evidence and absence of financing/transfer flows over the interval. |
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

## Admission-refusal identity correction

While completing fix 2, a focused regression found that a configured request
policy could refuse a placement without returning its nonzero `RequestID`.
The actor then emitted an unidentifiable rejection, and the router could leave
the affected two-leg group pending. Commit `0620b86` copies the original
request identity at the common admission gate, covering both direct and
gateway entry. Direct, gateway-inbox, request-kind, focused race and vet checks
passed. A clean full `GOMAXPROCS=7 make test` passed at that commit, including
the R2 archive fixture. A preceding dirty-tree invocation passed its Go and
contract tests but correctly rejected the clean-worktree parity fixture; it
is not counted as a clean full gate.

Fresh read-only Sol-6 medium reviewer
`01a0d344-6455-76f1-b0b8-1d67b674893c` independently confirmed the defect
on pre-fix `c4bcbd3`. This is **not** a standalone ME-005 execution gate when
the candidate config has no rate-limit tiers; it is still a real conditional
simulator correction. Historical tiered configs exist, so no global
non-activation claim is made. One retained G4 smoke summary reports zero
rate-limit and overload counts, but that observation does not establish the
activation status of every historical run. No historical trajectory has been
rewritten or rerun on account of this fix. The final ME-005 reviews remain
unperformed.

## Terminal-state valuation boundary

Commit `ce96909` adds a separate terminal-state valuation check with mutation
tests for early account capture, future/missing book evidence, mismatched
client identity, aged book evidence and insufficient visible depth. Focused
analysis race/vet and a clean full `GOMAXPROCS=7 make test` passed. The
`MaxBookEvidenceAgeNanos` parameter is an evidence-recency cutoff, **not** an
estimate of an individual order's quote lifetime. A quiet but live book can
have an old last transition; the protocol must choose this cutoff before
outcomes, and an over-age state yields an unavailable value rather than an
invented midpoint. The helper still requires an upstream verified binary
stream, fill/account reconciliation and proof that terminal FOK orders are
settled. It does not by itself complete fix 3 or license an economic run.

## Exchange-side placement outcomes

Commit `f11f9f2` adds a fail-closed collector for dedicated router accounts'
book-log `OrderAccepted` and `OrderRejected` events. It enforces the registered
market-FOK lot shape, unique request/order identity, and global frame ordering;
synthetic malformed, duplicate, wrong-venue and missing-field fixtures pass.
Focused analysis race/vet and clean full `GOMAXPROCS=7 make test` passed. This
collector does **not** infer a submitted request from a missing book event or
cover a rate-policy refusal outside the book log. By itself, it cannot prove
accepted FOK terminal fill or actor receipt; the next join addresses those
specific links but does not complete fix 2.

Commit `0dfadc6` adds the next join: accepted FOK placements require settled
fills summing exactly to the lot; rejected placements cannot acquire a fill;
actor acknowledgements must match request, order, reason and causal ordering.
An acknowledgement absent from the terminal inbox is represented as absent,
not silently converted into an exchange rejection. Split fills and mutated
identity, reason, quantity, sequence and horizon cases are covered. Focused
analysis race/vet and clean full `GOMAXPROCS=7 make test` passed. This pure
join assumes its inputs came from the independently validated collectors;
it does not yet prove that every submitted decision vector reached an
exchange outcome, or that terminal cancellation and pending requests are
completely classified. Those are still readiness blockers.

The existing ten-second two-venue synthetic production fixture was extended
at `b03fc70` to pass rendered binary logs through the placement/fill/inbox
join and to bind replayed terminal books to both account timestamps. Focused
race/vet and a clean full `GOMAXPROCS=7 make test` passed. It produced **zero
submitted router groups, zero placements and zero fills**; terminal local
valuation was available at zero inventory change. This is a useful
zero-attempt completeness control, not a positive end-to-end execution test.
A distinct manufactured positive-edge fixture through the production exchange
path was added at `1463586` but still does not estimate endogenous opportunity
frequency.

That fixture posts finite, deliberately crossed venue-local books and sends
the existing router's two FOK orders through the actual deterministic
exchange/actor path. Both legs fill in the completed case: matched-leg
cashflow is +50 quote units, while restoring inventory *hypothetically at the
remaining local displayed books* has value −10. A second case withdraws the
observed sell-side bid before venue arrival: the north buy fills, the south
sell FOK rejects, global base residual is +5, and local hypothetical exit
value is −5. Repeated focused runs, targeted race/vet and clean full
`GOMAXPROCS=7 make test` passed. These values are integer-unit fixture
identities, not empirical or development-world arbitrage results. The fixture
does not yet produce a complete canonical two-venue evidence bundle, and the
decision-vector-to-outcome audit remains required.

No ME-005 final preregistration, two final independent reviews, new development
seed, OFF/ON comparison, result, freeze or holdout claim exists. ME-001/002/003
and historical three-venue results remain attached to their original source
and experiment contracts.
