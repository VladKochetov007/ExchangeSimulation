# V2 R2 SV1B market-logic triage — 2026-09-08

Status: the asynchronous red-team feed has been inspected against the
scientific successor without switching worktrees or importing its performance
implementation. Four evidence/analyzer defects were independently reproduced
on their pre-fix scientific revisions and repaired in attributable commits.
The current successor is not yet authorized for a new world: a clean
mechanical gate and a fresh exact-tree Sol-xhigh review remain required.

No development, activation, capacity, freeze, or holdout world was run during
this checkpoint. Holdouts `619`, `631`, and `641` remain untouched. No retained
evidence was deleted or rewritten.

## Identities and feed inspection

The exact scientific tree inspected after the repairs is:

```text
branch: feature/r2-cdf-survival-successor
HEAD:   498a476071b505b5f5033ebb36d9b79c513451e8
```

The following remote refs were fetched with `git fetch origin --prune` and
read read-only:

| ref | tip | disposition |
| --- | --- | --- |
| `origin/redteam/economic-audit` | `f346b617386cdb564a31519ce863007569b44678` | economic and analyzer red-team reports; no wholesale merge |
| `origin/perf/r2-cdf-survival-port` | `39768dfed4ba4a5134f0c5ccf53351a79a0b1d64` | promotion package; no code imported |
| `origin/perf/ffa-gen0-port` | `899a6113ce0ee68bc28afe903ab985eba8bb047b` | superseded port branch; deferred |
| `origin/autoresearch/v2-performance-research` | `b1847ac40e8b7483e6e8a3f94b3705b4058884b` | asynchronous marker unchanged; no new commit |
| `origin/autoresearch/ffa-ecology-gen0` | `230e78fd202c78ea34ac3f0857089a2344a7cebd` | older scientific ref; design note only, not merged |

The economic red-team reports explicitly used base
`a666d02faede3d40f046b11e60eb672c59386a94`, which predates the current
scientific hardening. Its seven-hour dev-607 probe is not evidence from the
current SV1B candidate. The reported activation counts and metric values are
therefore treated as hypotheses and historical diagnostics, not as current
candidate outcomes.

The retained invalid seed-643 attempt remains at:

```text
/home/vlad/external-scratch/v2-r2-sv1b-activation-643-8b3d6c7eb76225f6f7895dba71edc4f1c20c5e06
```

It is `INVALID_AUDIT_EVIDENCE`; it is not a negative control, activation, or
capacity result.

## Independent reproduction and fixes

### RT-001 — expiry settlement bypassed the conservation tracker

**Classification:** BUG REACHABLE; evidence/detector defect, not an economic
trajectory defect. **Historical class:** ACTIVATED in old full-log dev-607
evidence, but OUTCOME UNAFFECTED.

The red-team reproduction failed on the pre-fix scientific tree
`e85f632a5cc155f014c22bd81eb68044c7736b22`. `settleExpiredInstrument` changed
perp cash and emitted a hand-built `balance_change` event without calling the
exchange conservation tracker. A discriminating unit fixture failed with a
nonzero gap, while the balanced fixture correctly demonstrated why this could
be hidden by a total-only detector.

The minimal repair is `9299b00ae308e6d3b68cc55e873c06cea40e39af`: record the
exact settlement cash changes before the logger is consulted and reuse those
changes for the existing event payload. The regression
`TestExpirySettlementIsRecordedForConservation` fails before the repair and
passes after it.

The red-team full-run comparison found that the patch removed only spurious
`conservation_violation` records: all non-general venue streams were
byte-identical and the general streams were identical after those false
violations were removed. No actor order, fill, balance, position, or lifecycle
decision changed. Historical raw evidence remains valid for simulator
trajectory claims, but conservation-violation counts from the old analyzer
must be rescored/re-audited under the repaired detector. A historical rerun is
not justified.

### RT-007 — buffered receipt oracle silently repaired stored order

**Classification:** BUG REACHABLE in the review oracle; analyzer/evidence
integrity defect. **Historical class:** no production trajectory impact; any
old buffered-oracle validity result is superseded and must be recomputed.

On the pre-fix scientific tree, the buffered receipt audit sorted events by
ordinal before checking order. The streaming production path rejects the same
out-of-order evidence. An independently constructed fixture therefore gave
`Valid=false` in streaming mode and `Valid=true` in buffered mode.

The repair is `b0f40c87a94041dfda67b4f014322acebd89b74d`: the buffered oracle now
preserves stored order and uses a three-way merge for the information-event
families. `analysis/economic_audit_receipt_oracle_test.go` locks the
discrepancy. This does not change the simulator, execution stream, or actor
state. Immutable raw evidence is sufficient; no simulator rerun is required.

### Reaction markout — symbol-less spot records used a pooled venue tape

**Classification:** ANALYZER BUG. **Historical class:** OUTCOME POTENTIALLY
AFFECTED for reaction/markout metrics only; simulator trajectory UNAFFECTED.

The red-team reproduction used the exact pre-fix analyzer behavior and showed
that ABC/USD and CDF/USD spot records sharing an empty payload symbol were
keyed as one `(venue, "")` book. The reaction markout was then measured against
whichever foreign spot book traded next. The independent branch measured values
from approximately `-37,540` to `+68,801` bps across cells, while the corrected
same-book values were within roughly 7 bps of zero.

The repair is `4d6ecf31225a91e03b31c6075de3ff6ff2eb888a`: use the payload symbol
when present and the canonical file-derived symbol for symbol-less spot
records. The regression in
`analysis/economic_audit_receipt_oracle_test.go` distinguishes the foreign
book from the correct book. Derivative records carrying their own symbol remain
unchanged.

The old raw streams are immutable and sufficient for a corrected analyzer
replay. Any historical claim that used the old reaction values must be marked
for rescore/replay; it must not be repaired by editing a trajectory or by
rerunning unrelated experiments. No new scientific world is needed solely for
this analyzer correction.

### Analyzer nondeterminism — tied records and map iteration

**Classification:** ANALYZER BUG. **Historical class:** OUTCOME POTENTIALLY
AFFECTED for nondeterministic analyzer metrics only; simulator trajectory
UNAFFECTED.

The independent reproduction repeated the same evidence and obtained differing
`reaction` values and differing `resting` role order. Causes included worker
arrival order, trade ordering on timestamp alone, unsorted fills, and map
iteration. This made the metric unsuitable for a frozen claim even when the
raw evidence was identical.

The repair is `498a476071b505b5f5033ebb36d9b79c513451e8`: reaction records carry
their `(File, Ordinal)` origin, ties use that total order, fills and map keys
are sorted, and equal-median resting roles use a deterministic name tie-break.
`analysis/reaction_determinism_test.go` repeats both cases and requires equal
outputs.

This defines a new deterministic analyzer rendering of old raw evidence. It
does not assert that the previous arbitrary answer was correct. Historical
reaction/resting outputs should be rescored from retained raw evidence before
reuse. No simulator rerun is required.

## Remaining economic/model findings

The following observations were inspected from the latest red-team reports and
against the current code/contracts. They are not silently changed in this
checkpoint.

| finding | current adjudication | reachability / historical decision |
| --- | --- | --- |
| RT-002 option-expiry cash leaves a few raw units under unequal position slicing | bounded fixed-point rounding; the existing accounting contract permits a one-unit-per-account style residual and the choice between per-position truncation and book-level carry is economic | no current blocker; retain as a limitation, no rerun or unregistered payout change |
| RT-005 bankrupt spot wallet is not seized | ambiguous wallet-segregation specification; `BorrowedSpot` and perp/spot wallet separation provide a deliberate competing interpretation | no confirmed defect or current activation; requires a named economic amendment before changing liquidation |
| RT-011 margin is cross-book but liquidation action is trigger-symbol scoped | current policy/specification question, not a newly proven implementation regression; coherent mark batching is already hardened | no code change; any “close all portfolio legs” policy would be a new liquidation amendment |
| RT-012 pending sibling suspends account risk/admission | intentional fail-closed boundary in the current price/lifecycle contract; pending exposure is not valued as zero | regression-covered and current policy accepted; no change |
| RT-014 interest is floored per charge interval without fractional remainder | exact fixed-point arithmetic with a materially model-dependent remainder policy; current scorer claims posted arithmetic, not an annual-rate convergence claim | borrowing is enabled, but no current SV1B trajectory exists; no silent financing retune; future remainder policy needs preregistration |
| RT-015 borrow gate values collateral cash/oracle rather than the full risk profile | real valuation asymmetry in the audit fixture, but the permissible leverage rule is an economic design choice and is not part of the CDF supplier amendment | no current activation evidence; do not change the registered market without a separate leverage amendment and review |
| RT-016 static collateral oracle | explicit anti-circularity design with bounded measured consequence; not an unambiguous defect | latent under current anchored development configuration; retain as a limitation |
| RT-018 borrowed spot debt has no position-liquidation entry point | potentially real missing exposure policy, but whether spot debt is liquidatable is a material economic choice | auto-borrow is reachable, negative-equity spot debt was not shown in retained SV1B evidence; no unregistered liquidation mechanism |
| RT-025/026 log-only failure reporting | the old report's `log-mode none` observation was based on an older path; the registered SV1B `evstream_v3` path installs a checkpoint-backed venue logger even when JSON venue logs are disabled, so failure diagnostics remain in the canonical binary stream | no current SV1B blocker; legacy no-sink configurations remain outside the registered contract and must not claim full audit coverage |
| RT-055/056 OTM option bid absence | participant quoting outcome attributed to the dealer's own conditional per-contract inventory gate; not hidden oracle/forced-liquidity code | market-design observation, not a simulator defect; preserve as an explicit limitation/discovery |

RT-014, RT-015, and RT-018 can alter economics if their alternative policies
are adopted, but none has been promoted as a defect requiring an unregistered
change to the CDF survival successor. They must be reconsidered if a future
development run activates the relevant boundary or if the scientific claim is
expanded to leverage-cost or spot-debt enforcement.

## Historical impact and decision matrix

The older red-team dev-607 probe used a different base and is not a substitute
for retained SV1B evidence. The current retained SV1B seed-643 attempt did not
produce valid audit evidence and has no accepted trajectory. No current
development cell has run after the R2/SV1B protocol repairs.

| defect class | raw evidence sufficient? | simulator rerun? | required treatment |
| --- | --- | --- | --- |
| expiry conservation recording | yes, for detector/residual re-audit | no; no trajectory mutation | rescore conservation and preserve old event counts as historical |
| buffered receipt ordering | yes | no | replay the review oracle; do not use old buffered validity |
| reaction book identity | yes | no | rescore reaction/markout metrics from immutable raw streams |
| analyzer tie ordering | yes | no | rescore reaction/resting and pin the deterministic analyzer revision |
| a confirmed actor/risk/matching/lifecycle semantic defect | not applicable to the four repaired findings | would require a rerun | none confirmed by this checkpoint on current SV1B evidence |

The earlier R2 F1/F2/F3/F6/F8 adjudications remain in
`research/v2-r5-market-logic-triage-2026-08-30.md`. Their current fixes and
explicit pending/financing limitations remain in force. This append-only note
does not reopen `ae13f9a`, P3e, signed-price, P4/P5, P6, P7d, or the mixed
timing line.

## Gate consequence

The current scientific tree contains the minimal RT-001, RT-007, reaction, and
determinism repairs with focused regressions. The repairs change detector or
analyzer outputs only; they do not change the R2 calendar, finite CDF supplier
economics, eight historical ABC/USD suppliers, registered configs, or the
binary evidence representation.

Before any new activation or development cell, the remaining required sequence
is:

1. run a clean full `make test`, `go vet ./...`, focused package tests, and the
   bounded targeted race/evidence matrix;
2. obtain one fresh independent Sol-xhigh review of the exact final tree,
   including these repairs and the R2/SV1B protocol;
3. build provenance-pinned Go 1.27 binaries from that accepted clean tree;
4. run only the authorized paired seed-643 activation probe and independently
   validate its evidence;
5. only after a valid activation boundary, recompute capacity and proceed to
   registered development cells; never consume holdouts before explicit freeze
   authorization.

The performance-port fingerprint optimization, binary prototype, and indexed
analytics work remain deferred. The next asynchronous feed comparison starts
from `b1847ac40e8b7483e6e8a3f94b3705b4058884b`.

## Independent review checkpoint: Euler rejection of exact current tree — 2026-09-08

After the mechanical gate completed on the exact scientific revision, fresh
independent Sol-xhigh reviewer `Euler` inspected the predecessor review tree
`498a476071b505b5f5033ebb36d9b79c513451e8` (the source tree was unchanged by
the later documentation-only checkpoint) and rejected full SV1B promotion.
The reviewer response is retained in the orchestration record, but no local
report file or SHA was materialized. It has no acceptance attestation; its
review is a blocking negative gate and cannot be reused as a promotion
artifact.

The review identifies the following concrete pre-activation concerns:

* RT-002: unequal option expiry positions can leave a book-level quote-unit
  residual because payoff is truncated per position; the accounting contract
  must explicitly route or otherwise account for this residual before a
  24-hour candidate can claim exact conservation.
* RT-011: cross-margin risk is aggregated across books while the liquidation
  action is trigger-symbol scoped; the successor must define and test the
  action invariant, or fail closed on a policy it cannot justify.
* RT-014: per-minute collateral interest floors without a carried fractional
  remainder. Euler reports this can activate in the current auto-borrow path,
  so a successor must either carry the remainder with auditable state or
  explicitly narrow the registered financing claim.
* RT-015/016: the borrow gate values wallet/oracle collateral rather than a
  risk-coherent unencumbered portfolio, and CDF collateral uses a static
  price source. The successor must inject an explicit collateral policy,
  exclude dynamic CDF collateral, or provide a reviewed coherent valuation;
  this cannot remain an unstated leverage rule.
* RT-018: borrowed spot debt has no complete account-risk enforcement entry
  point. The successor must guard or reject undercollateralized spot debt, or
  prove the registered population cannot enter that state.

RT-005 (wallet segregation), RT-012 (pending exposure fail-closed), and
RT-055/056 (conditional option-maker behavior) remain specification or
participant-behavior matters, but their policy boundaries must be stated in
the successor contract. No new world ran at the rejected tree, and no
holdout was inspected. The prior invalid seed-643 attempt remains preserved.

The current full test/vet/focused gate is green. It does not promote the
candidate: the review rejection blocks the pinned rebuild and activation until
the concrete economic invariants above are preregistered, regression-tested,
implemented where required, and accepted by a new exact-tree review.

## RT-002 repair checkpoint — option expiry book-level rounding — 2026-09-08

RT-002 is confirmed as a reachable fixed-point accounting defect for a closed
option book with unequal position slicing. The existing per-position payout
rule is retained: each holder receives the integer result of its own contract
cash-flow calculation. The economic book invariant is that the sum of those
integer postings must differ from the integer cash flow of the net position by
rounding only. A nonzero net option position is not rounding and remains
unmatched.

The successor repair computes the aggregate signed position and posted cash
for each expiring European option. It routes only
`posted_cash - cash_flow(net_position)` with the opposite sign through the
existing `VenueFeeRevenue` ledger under reason `option_expiry_rounding`.
That reason is included in the audited venue stream and does not add a new
binary payload schema. The analyzer now reports both participant-only
`option_expiry_instants` and `option_expiry_system_instants`, the latter
including the explicit venue correction. A regression forces +1, +1, -2
positions at an intrinsic value whose per-position payouts leave a one-unit
residual; a companion regression proves an unmatched net position is not
relabelled as rounding.

This repair changes the venue ledger and conservation rendering, not holder
payouts, orders, marks, positions, or lifecycle decisions. Retained historical
raw evidence is sufficient for rescore of option expiry/conservation metrics;
no trajectory rerun is justified solely by this repair. The exact successor
tree still requires the remaining risk/borrow repairs and a new independent
review.

## Independent review record

Sol-xhigh reviewer Zeno independently rejected the exact pre-repair tree
`e85f632a5cc155f014c22bd81eb68044c7736b22` for RT-001. The report is retained
at
`/home/vlad/external-scratch/v2-r2-sv1b-review-e85f632/independent-review-report.md`
with SHA-256
`8018e69528a7b702660c56b49f63ece7446a8da9766bb55cbf29e9bfae9e912e`.
The rejection is historical evidence for the repair, not an acceptance of the
current `498a476` tree. A new exact-tree review is required after the clean
post-repair mechanical gate.
