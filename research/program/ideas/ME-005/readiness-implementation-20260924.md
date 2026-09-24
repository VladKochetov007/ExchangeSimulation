# ME-005 implementation checkpoint — development readiness, not a result

Current prospective source successor: `0fd30445bd15ae0aa45407a59b39ee494ef5f6ae`.
The [r1a protocol](protocol.md) and four analysis contracts now pin this
source, leaving the original r1 configs/hashes unchanged. An initial bounded
Reviewer A pass on the predecessor protocol returned required changes, not
acceptance: nonzero response actor IDs needed to bind to the submitted leg,
actor-terminal reconstruction needed to account for inbox response ordering,
and the protocol had incorrectly described prefunded balance checking as a
router-side policy gate. The source successor adds actor-ID joins and mutation
tests. Its ordering regression follows `BaseActor`'s actual early-fill buffer:
a pre-acceptance fill is replayed after acceptance, while a missing acceptance
leaves the actor outcome unobserved. The protocol now classifies funding as an
independent quote-time analysis following submission, not router abstention.
No final Reviewer A acceptance or Reviewer B verdict has been issued for r1a;
no ME-005 economic world has run. The candidate's full clean test gate must
also be recorded after this documentation amendment.

Source baseline: merged `main` `eb921aa29b26b43cd39e1613dabc44ec60dbf5c9`.
The table below records the earlier clean code gate `006afb1` on
`research/market-ecology-me005-20260924`; its pending-work column is
historical, not the current run instruction.
The authoritative [readiness assessment](readiness-20260924.md) and
[prospective preparation contract](prepare-contract-20260924.md) still define
four acceptance fixes. This note records implementation progress without
registering a protocol or scoring a market world. ME-005 economic worlds run:
**zero**. Holdouts consumed: **zero**.

## Current prospective candidate after that checkpoint

Source/config candidate `b9ed591e255ff951b53dc2c001c6ac25a90b7b0e`
adds the completed first-attempt world analyzer, strict source/config/seed
and raw/rendered binary binding, run-wide conservation failure gate, an
effective-config-derived OFF/ON background identity, and four prospectively
fixed two-venue configs. Synthetic ten-second OFF and ON binary fixtures,
static matched/unmatched local-closeout examples, corruption tests, focused
race/vet and clean full `make test` pass at that candidate. Those are
**mechanical fixtures, not ME-005 economic worlds**. The effective hashes
are asserted by a normalization-only test; no development outcome selected
the seeds, lot, horizon or closeout bound. See the later
[conditional r1 protocol](protocol.md) for the current cell list and stop
rules. The four scoped readiness implementations are prepared for final
review, **not independently accepted yet**. Two fresh reviews and resource
preflight remain mandatory before the first economic world.

| Fix | Implemented and mechanically exercised | Still required before ME-005 RUN |
|---|---|---|
| 1. Two-venue wiring | Exactly two distinct venues and finite, separate router accounts; historical three-venue default retained. Two-venue router configs now require explicit `auto_borrow_spot=false`. | Pin the effective two-venue config, initial balances, lot, attempt cap and deployment in a preregistration; independently review this successor population choice. |
| 2. Opportunity/execution reconstruction | Optional canonical callback evidence including no-action and consumed source identity; public-book replay in global frame order; receipt-prefix and actor-local book checks; actor-inbox response receipts; terminal count binding; first trade ID zero, Trade/OrderFill/response joins. Submitted FOK requests join audited decision vectors to exchange placements, fills and acknowledgements. A public-episode/actor-evaluation timeline distinguishes public-active, locally aligned, locally inactive and post-public local-positive states, with half-open frame ordering and censoring. The first submitted quote can now be checked against audited initial venue balances. Dedicated market-FOK router cancellations fail closed; a first-attempt terminal join separates settled exchange outcome from actor-observed status and requires accepted-order acknowledgements before fill receipts can establish actor knowledge. A narrow binder checks manifest/config/source, raw/rendered report and stream-attestation identities after a successful binary render. | Classify disappearance reasons only where the event chain actually identifies them; unclassified causes must remain unknown. Quote-time funding is not arrival-time admission. Integrate the binder into the registered-world driver with a prospectively pinned effective config. |
| 3. Costed closeout | Independent venue-local bid/ask depth walk with quote taker fees and insufficient-depth failure; initial/terminal venue-account deltas checked against settled fills, fee rounding and zero debt/locked inventory. The terminal helper requires both account timestamps at the declared horizon and bounds replayed-book evidence age when base inventory changed; a zero-base-change account needs no price update. A static fixture gives +48 matched cashflow but −13 local terminal value. A manufactured production path binds binary evidence, venue accounts, local value and run-wide conservation. A per-router movement audit rejects undeclared balance changes and pairs each settlement with one canonical fill. | Pin the book-evidence recency bound prospectively; invoke the renderer, binder and entire reconstruction on each registered completed world. The final independent review remains. |
| 4. Dislocation measurement | Two-venue, event-ordered, one-lot executable edge and half-open episode functions; a pure OFF/ON seed-pair estimator reports episode counts and positive-edge duration, retains same-time episodes and rejects missing/mismatched cells. A ten-second no-op production fixture leaves final displayed background ABC/USD books unchanged when the router submits nothing. A static test proves an episode between periodic samples is visible to the event-time estimator but absent at both sample endpoints. | Bind actual OFF/ON world evidence and background config/clock identities, retain receipt-lag classification, and preregister missing-run and uncertainty rules. Trade-attributed convergence remains `NOT_IDENTIFIED` absent stronger attribution evidence. |

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
identities, not empirical or development-world arbitrage results. The original
actor-level fixture does not itself produce a canonical evidence bundle.

Commit `50dcbcf` closes that narrower measurement gap. An audited selector
reads binary decision vectors only after the existing complete vector/scalar
audit and rejects off-symbol decisions on selected router links. A fail-closed
join binds each `SUBMIT` evaluation to buy-then-sell gateway decisions,
complete consumed-feed components and unique exchange outcomes. Mutation
tests reject missing vectors/outcomes, wrong request/side/frontier, future
information and an unpaired no-action row. The existing zero-attempt
production fixture checks the empty path; a separate ten-second *injected
crossed-book* production fixture obtains one two-leg FOK attempt and
independently reconstructs its placements, settled fills, inbox receipts and
venue-local account deltas from rendered binary evidence. It uses registered
maker accounts for finite fixture quotes, without an extra unmounted gateway
or a runner change. This is a mechanical positive path, **not** a development
world, natural opportunity rate, net-profit estimate or convergence result.
Focused tests, vet, targeted race checks, the skill validator and a clean
`GOMAXPROCS=7 make test` passed at `50dcbcf`. A dirty-tree invocation passed
all Go packages but failed the clean-worktree archive parity fixture as
designed; it is not counted as a clean gate.

Commit `ea92e2e` adds a pure event-time OFF/ON world-pair summary. It retains
zero-duration positive episodes, censored episodes and valid no-opportunity
worlds; pairing rejects duplicate, missing, or mismatched seed/arm, horizon,
lot, fee and background-identity cells. Venue-list permutation is harmless.
Its `BackgroundIdentity` is supplied by the caller and **does not prove**
common-random-stream alignment or causal attribution. The output explicitly
labels trade attribution `NOT_IDENTIFIED`. Focused race/vet and clean full
tests passed. No OFF/ON economic worlds were run.

Commit `5b6b6fb` tightens the positive synthetic binary fixture. Independently
replayed books and terminal account deltas produce a *hypothetical* local
liquidation value of −44,000,000 quote atoms for its one completed route;
the all-participant movement stream has zero malformed/chain/fee mismatches
and zero asset and venue-asset residuals. This is a constructed arithmetic
and evidence check, not a profitability result. Focused race/vet and a clean
full test gate passed. The registered-world recency limit and complete
run-manifest/account/evidence binding remain prospective.

An advisory read-only Sol-6 medium review at `44dc477`, agent
`01a0d38b-3809-7310-b843-1422c7325f35`, challenged the opportunity
denominator, not the completed candidate. It recommended one row per public
positive episode, joined by canonical half-open frame order to zero or more
verified delayed evaluations; a missing callback is not an ignored
opportunity. It emphasized that local positive state can persist after the
public edge ends and that balance feasibility cannot be inferred from quote
or receipt evidence. It also identified a concrete source ambiguity: a
`BookDelta` log does not carry the publisher sequence, so two identical
same-time deltas can otherwise both match one claimed actor trigger.

Commit `7dad7b8` makes that exact duplicate-publication case fail closed and
adds a mutation fixture. This can make an ambiguous retained trace
unattributable; no historical result was rewritten, and no blanket claim is
made that it never occurs elsewhere. Commit `cdc1c23` adds a no-op OFF/ON
simulator fixture: with no router attempts, the displayed terminal ABC/USD
books match across arms. It does not prove every background RNG stream or
total-capital identity matches, so the future paired design still needs a
registered background check.

Commit `b8ca004` implements the evidence-gated public/local timeline. It
requires the compact receipt audit, consumed-public-source match, local-book
reconstruction and canonical public replay before assigning evaluations to
half-open positive episodes. It retains zero-evaluation episodes, same-time
ordering and the latest matched publication frame for each venue. In the
manufactured production fixture, one public episode is horizon-censored,
22 delayed evaluations are reconstructed, 20 have a locally aligned positive
route, and one submits. These are **synthetic fixture counts**, not an ME-005
world or natural opportunity denominator. Focused race/vet and a clean full
test gate passed at `b8ca004`. The advisory review is not Reviewer A or B's
final acceptance of all four readiness fixes.

Commit `9bc1c8b` adds a dedicated audit of the two router accounts across
*all* rendered venue event files. Each account must begin with exactly one
deposit matching its initial snapshot; every later balance change must be a
spot `trade_settlement` whose base/quote delta and timestamp match one
canonical router fill; terminal balances must close the movement chain.
Transfers, interest, borrowing, undeclared wallets, missing/duplicate fills
and outer/payload client-ID mismatch fail closed. A first-attempt-only check
then compares the submitted buy's quote cost including charged fee and the
sell's base lot against these audited prefunded balances. It makes **no**
claim that the same depth or balances will be present when delayed FOK legs
reach their venues. The positive synthetic production fixture and adversarial
mutations, focused race/vet and clean full tests passed. This is not a
development-world funding result.

No ME-005 final preregistration, two final independent reviews, new development
seed, OFF/ON comparison, result, freeze or holdout claim exists. ME-001/002/003
and historical three-venue results remain attached to their original source
and experiment contracts.

Commit `7d3ef13` adds the dedicated market-FOK cancellation rejection, the
exchange-versus-actor first-attempt terminal join, and the periodic-snapshot
aliasing fixture. The actor terminal join requires an accepted-order inbox
acknowledgement as well as every fill receipt: the current router cannot attach
a fill by order ID before learning that ID. Missing delivery yields
`ACTOR_TERMINAL_UNOBSERVED`, not an assumed horizon censor or an exchange
failure. Focused tests, affected-package vet and race checks passed. A dirty
`make test` passed Go packages and contract fixtures but, as designed, failed
the archive parity check requiring a clean worktree; the clean committed
`GOMAXPROCS=7 make test` passed in full. This is a mechanical readiness
checkpoint, not an accepted final four-fix review or a development result.

Commit `6a54a79` makes hypothetical terminal valuation independent of a fresh
price when the audited account has no change in base inventory. A no-trade
world can therefore retain its valid zero-value endpoint even if its last
book update is old; any nonzero base change still needs fresh executable
local depth. This analyzer-only correction has focused race/vet coverage and
a clean full test gate. It does not make an unavailable nonzero position
priceable.

Commit `0a1c0d0` composes the already-tested actor callback, receipt, public
book, decision vector, exchange placement, fill, account movement and
terminal-value joins into one reusable first-attempt analysis entry point.
Both zero-attempt and manufactured matched-FOK binary fixtures pass through
it. The entry point explicitly assumes the caller has already verified that
the rendered binary tree, terminal report and raw sidecars belong to the same
completed manifest/config/source identity; that binding is **not yet
implemented** by this function. Focused race/vet and clean full tests passed.
The wrapper is not a ME-005 development result or final four-fix acceptance.

Commit `006afb1` adds a narrow run-binding check for the exact effective
manifest-config digest, clean 40-hex source revision, two-venue finite
one-attempt evidence settings, byte-identical raw/rendered terminal report,
and matching terminated-binary/rendered-attestation stream identities. It
rejects changed source/config/report/stream and a borrowing-enabled config
even when its new digest is supplied. The positive production-path fixture
uses a **synthetic** 40-hex revision because `go test` stamps that fixture's
binary `unknown`; this substitution is confined to the test. Actual economic
runs must use a clean production build at a pinned committed revision.
The binding consumes the execution hash returned by the existing completed
binary renderer, which must run first; it is not a second byte-for-byte
decode of `events.evs`. Focused race/vet and clean full tests passed.

Still open before preregistration/review: a single finite registered-world
driver that invokes the renderer, copies and binds the report without
overwriting evidence, extracts both arms and emits the complete funnel and
per-attempt economics; an exact effective config and horizon/recency choice;
paired background/clock identity and missing-cell rules; and the two final
independent scoped reviews. No ME-005 economic cell has run.
