# V2 R5 market-logic triage — 2026-08-30

Status: semantic triage recorded; scientific candidate is not yet authorized
for a new development cell. The current worktree contains the minimal fixes
and regression tests described below, pending a fresh independent Sol-xhigh
review, full test gate, provenance-pinned rebuild, and a new registered dev-607
run.

## Scope and identities

The scientific branch was not switched during this audit. The exact scientific
revision whose retained dev-607 trajectory was inspected is:

```
branch: autoresearch/ffa-ecology-gen0
HEAD:   09d9f18bd48f69696d97cf70d98748c98d2df7ff
```

The independent branch was fetched as `origin/autoresearch/v2-performance-research`
because no `github` remote is configured. The inspected history includes:

| revision | subject |
| --- | --- |
| `57077e9` | `perf: sweep only the accounts that hold a position in the symbol` |
| `af8535d` | `audit: record market-logic findings exposed by structural optimization` |
| `2989934` | `perf: bound the preview clone to the levels the matcher can reach` |

The following independent documents were read completely from that remote
branch:

- `research/performance/v2-risk-semantics-audit.md`
- `research/performance/v2-preview-semantics-audit.md`
- `research/performance/v2-simulator-performance.md`

The actual reproduction tests were also inspected, including
`tests/risk_semantics_audit_test.go`, `tests/risk_ordering_metamorphic_test.go`,
and the complete `exchange/preview_differential_test.go`. No performance
commit was merged or cherry-picked.

The retained current dev-607 raw archive is
`/home/vlad/v2-r5-preserved-dev607-09d9f18-full.tar.zst`, SHA-256
`f1fc266d24494b0ad2eb27ae28d93eb91a5e821a8978c0b8fe83145525336`,
2,440,301,188 bytes. Its analyzer directory is
`/home/vlad/v2-r5-analysis-dev607-09d9f18`. The archive is historical evidence;
it was not rewritten or repaired.

## Evidence extraction

The retained analyzer reports show:

| report | retained result relevant to this triage |
| --- | --- |
| `liquidations.json` | `liquidation_checks=0`, `liquidations=0` |
| `marginchecks.json` | 1,555,062 active mark checks; zero expected/observed breach checks and zero mismatches |
| `activation.json` | 19,097 collateral-borrow events; zero price-unavailable order rejections |
| `lifecycle.json` | 33 future and 330 option settlement records; no pending-settlement record |

As a second check, a Go-only line reader replayed the six retained derivative
and general venue streams. It counted:

- 3,295 `price_unavailable` records, including 24 `perp_index` failures and
  3,244 `option_liquidation` failures; there were zero `liquidation`
  operation failures.
- 1,171,190 `position_update` records. There were 656,038 observations at
  which a client held nonzero positions in more than one symbol, with a
  maximum of 12 symbols; all three venues had 11 such clients.
- zero `liquidation_check`, `liquidation`, `margin_call`,
  `funding_settlement_failed`, `margin_interest_failed`, or
  `expiry_settlement_pending` records.
- 19,863 `borrow` records and no `repay` records. Borrow principal ranged from
  19 to 299,823,750,000 raw units; the aggregate borrowed principal was
  59,885,470,429,174 raw units.
- 49,262 positive `margin_interest` records totaling 6,763,504,467 raw units.
  Replaying the borrow state at each posting gave principal range
  11,063,321–1,999,999,995,074 raw units. Every posted amount exactly matched
  `floor(principal * 500 / 5,256,000,000)`; there were zero floor mismatches
  and zero zero-valued postings (zero postings are not emitted).
- The fractional numerator discarded by those 49,262 per-minute floors was
  122,572,649,651,000, equal to 23,320 whole raw units plus a remainder of
  2,729,651,000 at denominator 5,256,000,000. This is 0.23320 quote units at
  the USD precision used by this world.

These stream counts are activation proxies, not replacements for the
scientific analyzers. In particular, an absent event cannot prove an absent
internal, non-action risk verdict.

## Findings and adjudication

### F1 — an irrelevant unmarked book can abort a margin profile

**Reproduction.** The independent test constructs an account with exposure in
one perpetual and an unrelated perpetual whose mark is unavailable. Before
the fix, `buildAccountMarginProfile` resolved every relevant perpetual mark
before checking whether the account had a position, so the unrelated book
returned `ErrNoBookPrice`. The same test then verifies that the exposed book
still fails closed when its own mark is unavailable.

**Invariant.** An account profile is a portfolio valuation. A book with no
nonzero account position contributes neither equity nor maintenance and must
not be allowed to suppress a decision for the account's actual portfolio.
An exposed book must have a valid risk mark; entry-price, zero, or another
book's mark is not a fallback.

**Classification.** `BUG REACHABLE`. The current candidate now collects the
nonzero positions for a book before resolving its mark. The retained dev-607
archive has 24 `perp_index` unavailability records and 3,244
`option_liquidation` `price_unavailable` records. Those records prove that
unpriceable risk-path work occurred, but the retained payload does not carry
the affected client ID or the complete profile membership needed to prove
whether F1 suppressed a particular decision. Therefore the historical
classification is `CANNOT DETERMINE FROM RETAINED EVIDENCE`, not zero
activation.

**Historical decision.** No offline repair and no reopening of V-005, P3e, or
any frozen line. A fresh dev-607 is required because this is a simulator
semantic correction, even though the retained current trajectory contains no
observed activation.

### F2 — one unpriceable account can terminate a liquidation scan

**Reproduction.** The minimal reproduction places an unpriceable account before
a clearly liquidatable account in client-ID order. Before the fix, a profile
error executed `return`, suppressing every later account. The sibling option
scan already continued on a profile error.

**Invariant.** Account liquidation checks are independent. A missing mark for
one account is a fail-closed result for that account, not permission to skip
other accounts. The scan must continue in deterministic client-ID order.

**Classification.** `BUG REACHABLE`; the candidate changes the profile-error
path to `continue` and records the price-unavailable reason. The retained
archive has zero `liquidation` operation failures and zero liquidation checks,
so this activation condition is not observed in dev-607.

**Historical decision.** `CONDITION POSSIBLE BUT ZERO ACTIVATION IN RETAINED
EVIDENCE`. No historical trajectory is repaired. The corrected simulator must
be used for the new registered dev-607.

### F3 — cross-margin liquidation can use stale sibling marks

**Reproduction.** The independent metamorphic test uses the same account and
portfolio twice, changing only which symbol's mark update is processed first.
Before the fix, the triggering symbol used its current mark while sibling
symbols used their stored marks. Renaming/reordering symbols could therefore
change a borderline cross-margin risk verdict. The current regression installs
all successful linear marks and the option mark refresh before the liquidation
pass, so a portfolio is valued against one mark set for the tick. Both option
and dated-future marks are checked in the local batch regression.

**Invariant.** For independent mark updates belonging to one simulation tick,
cross-margin valuation must not depend on symbol spelling or map/iteration
order. The risk pass must observe all successfully committed marks for that
tick. If a mark is unavailable, risk must fail closed for the affected path;
it must not silently substitute an entry or zero mark.

**Classification.** `BUG REACHABLE`. The retained stream proves the exposure
condition was present: 656,038 multi-symbol position observations across all
three venues. It does not show a liquidation action or warning callback: there
are zero `liquidation_check`, `liquidation`, and `margin_call` records. Thus
the retained historical outcome is `ACTIVATED BUT OUTCOME UNAFFECTED` for
observable risk actions, with the narrower caveat that internal solvent
verdicts are not separately logged. This is sufficient to block promotion of
the old trajectory as corrected evidence, but not sufficient to invalidate
every old scientific claim globally. The corrected contract now also covers
option and dated-future sibling marks, whose liveness is checked at commit.

**Historical decision.** The new dev-607 is a required rerun because this bug
can alter risk decisions when a cross-margin portfolio is borderline. V-005
remains frozen at its explicitly registered old revision `ae13f9a`; its old
margin-profile semantics and retained liquidation results are not silently
reinterpreted. P7a/P7b/P7c and P7d retained summaries do not contain direct
multi-instrument participant-risk activation. Their generic or directional
results remain historical claims, with the limitation recorded here; no old
holdout is reopened before its authorization boundary.

### F6 — settlement-pending positions disappear from risk

**Specification check.** The current price-lookup/lifecycle contract explicitly
defines `SETTLEMENT_PENDING`: positions and collateral remain held, the
contract is halted, and funding, mark updates, liquidation, and post-expiry
fills do not continue while settlement is pending. The expiry implementation
retains positions and retries the declared settlement source. The dedicated
regression `TestExpirySettlementPendingRetriesThenSettlesExactlyOnce` proves
that the state is reachable and can persist across a failed settlement
attempt.

**Invariant.** Pending settlement must not be silently valued as a live
contract or settled at a zero/fallback price. The retained position is not an
economic zero. Because no valid mark exists, the corrected r5 candidate now
fails the entire account profile closed when it contains nonzero pending
exposure; it does not omit that position while evaluating an active sibling.
The same account-wide boundary rejects new orders on active siblings and manual
borrowing until settlement succeeds, so an account cannot increase new order or
manual-borrow obligations while its portfolio is not priceable. Existing debt,
interest, and other scheduled balance changes remain governed by their own
contracts.

**Classification.** `AMBIGUOUS SPECIFICATION` in the original audit, resolved
for the corrected r5 candidate by the explicit pending-exposure risk boundary
added to `research/v2-price-lookup-contract.md`. This is risk isolation, not a
zero valuation. The dedicated regression now verifies both that the lifecycle
clears mark availability and that a cross-margin profile fails closed rather
than ignoring pending exposure. The retained dev-607 archive has 363
`instrument_settled` and 3,569 `expiry_settlement` records but no pending
record, so no historical activation is shown. The addendum and implementation
still require fresh independent review before promotion; changing this policy
would be a new lifecycle preregistration.

### F8 — borrow interest truncation

**Units and arithmetic.** USD uses 100,000 raw units per quote unit. The
configured 500 bps annual rate and one-minute charge interval use the exact
integer denominator:

```
365 * 24 * 3600 * 10000 / 60 = 5,256,000,000
interest_raw = floor(principal_raw * rate_bps / 5,256,000,000)
```

At 500 bps, the first one-raw-unit charge requires 10,512,000 raw units,
which is $105.12. A constant debt below that threshold produces no posting on
each minute and no remainder is carried. The per-minute truncation error is
less than one raw unit per charge: over 24 hours the bound is 1,440 raw units
($0.01440), and over 98 hours it is 5,880 raw units ($0.05880) per continuously
charged debt. For a debt below the threshold, the mathematical charge can be
entirely suppressed over those horizons.

**Observed evidence.** The retained dev-607 stream has 19,863 borrow records,
including a minimum principal of 19 raw units, but only positive interest
records are emitted. The 49,262 observed positive records exactly agree with
the current floor arithmetic. Consequently, the archive cannot establish how
many below-threshold debts survived a charge tick: the zero-valued attempts
are not observable. The current P1a result separately and explicitly records
that its declared annual borrow term rounds to zero at the policy's whole-bps
resolution; that registered result is not silently rewritten.

**Classification.** `AMBIGUOUS SPECIFICATION` / economic-model issue, not a
proven arithmetic defect. Per-minute floor semantics are internally exact and
fail closed; whether financing should accumulate a fractional remainder is a
model choice that changes balances, actor decisions, and potentially distress
outcomes. The observed postings demonstrate truncation but do not prove a
historical zero-cost interval for a specific participant. The corrected r5
scorer explicitly excludes financing-cost magnitude and remainder-carry claims:
it audits only posted-interest arithmetic, wallet attribution, and conservation
for this line. A future financing-mechanics claim requires durable zero-attempt
evidence (or a preregistered remainder accumulator) and a separate review.

**Historical decision.** No offline rescore and no unregistered remainder-carry
change. A remainder-carry policy must be separately specified, tested against
wallet attribution and conservation, and preregistered before any successor
experiment. Existing funding/carry, P7, and integrated results retain their
declared semantics and limitations. The fresh corrected dev-607 does not
silently change this economic policy.

## Adjacent edge cases covered

The new tests cover zero-exposure unmarked books, the first/last-client
liquidation-scan boundary, a clearly underwater account, coherent cross-margin
mark installation across perp, option, and dated-future books, pending-exposure
fail-closed risk and the order/borrow boundary, hedge-mode aggregate funding,
funding and collateral interest overflow atomicity, wallet-specific interest
attribution, strict venue sequence/order checks, and net-zero hedge settlement.
Existing lifecycle tests cover expiry-1/expiry/pending
retry/settle-once behavior. The retained contract continues to reject missing
marks rather than substituting zero or entry values.

The independent preview audit's actual differential harness compared complete
execution records and structural book fingerprints across approximately 80,000
random FIFO/ProRata scenarios, plus 3,000 end-to-end FOK scenarios per mode,
with zero preview-vs-committed divergences. Its two latent cases (partially
filled incoming preview state and malformed exhausted iceberg state) are
currently unreachable or safely refused. This is additional validation only;
the preview performance implementation remains deferred.

## Asynchronous performance-branch checkpoint

On 2026-08-30 the remote was fetched as
`origin/autoresearch/v2-performance-research` and compared after the last
reviewed revision `2989934`. The newly inspected range was
`897f0f6..9dc6b08` (including `897f0f6`, `e067679`, `0fa7e02`, `b960aec`, and
`9dc6b08`). These commits are performance census, conditional book-delta
construction, no-op measurement, hash-payload experiments, and exact
allocation measurement. The agent explicitly retracted its earlier sampled
allocation and appender-coverage claims: the appender covers about 17% of
hashed bytes and gives no measured wall-time win. The code changes preserve
sequence consumption and market behavior in their differential tests; no
plausible semantic blocker was found. No performance patch was merged or
cherry-picked. The next checkpoint must compare only commits newer than
`9dc6b08` and read only newly relevant reports or diffs.

## Asynchronous performance-branch checkpoint — 2026-08-30 (new range)

The remote advanced from the last reviewed commit `9dc6b08` through `feeb6f9`
to `e3558df`.  I inspected only `e3558df` and `feeb6f9` plus their new report
sections.
They prototype a canonical binary evidence stream and report approximately
2.4x encoding and 15.9x decoding on a 20,000-event benchmark, with lower
allocation and compressed size.  The prototype does not yet cover every event
family, prove JSON/typed/binary equivalence, preserve the full global order,
or differentially reproduce all analyzer outputs.  It is therefore a future
analytics acceleration candidate, not a production evidence-contract change;
no code was merged or cherry-picked and no scientific result is affected.
The branch then added `7fdf55f`, a block-index/query prototype.  Its four
query classes agree across JSON, binary full scan, and indexed scan; selective
queries skip blocks while broad interleaved-family queries skip none.  This is
useful future analytics evidence but remains performance-only, with no
scientific-path code merged.  The next feed comparison starts at `7fdf55f`.

The feed then advanced to `4b370fa` and `52a3eca`.  I inspected only their
new report sections.  They are performance-only measurement commits: a
serialization-removal probe measured a 17.29% wall-time ceiling, and a
fixed-width 88-byte hash probe was indistinguishable from that ceiling.  The
new evidence changes the performance explanation (reflection/allocation is the
simulator-CPU opportunity; compactness is primarily storage/analytics), but
does not justify changing the scientific evidence path.  No code was merged or
cherry-picked.  The next feed comparison starts at `52a3eca`.

## R2 successor review checkpoint — 2026-08-30

Hooke (Sol-xhigh) reviewed the exact pushed R2 implementation `aed83a6` and
**REJECTED** it.  The independent review found that calendar future and option
symbols were silently dropped by the existing basis and option-surface
parsers, that collision-only option requests still depended on a spot-price
lookup, and that the R2 scorer had no precommitted predicate for realized
calendar behavior.  No R2 development or holdout cell was run from that
revision.

The current corrective work independently adds parser regressions for legacy,
canonical, and sub-second symbols; moves collision filtering ahead of price
lookup; and introduces a Go-native `calendar` analyzer artifact.  The
extractor/scorer will require the registered 28-expiry set, 23 completed
cycles, equal futures/options expiry sets, zero duplicate or malformed
identities, and at least three simultaneous maturities per venue.  The Hooke
verdict remains historical until a fresh reviewer accepts the exact corrective
commit.

## Independent review checkpoint — 2026-08-30

Reviewer Volta (Sol xhigh) independently inspected the candidate after the
full `make test` and focused race gate. The verdict was **REJECT** because a
failed option-underlying refresh left the previous cached premium available to
risk; the successful-refresh regression did not cover this failure transition.
The reviewer also identified a documentation overclaim: pending exposure
blocks new order and manual-borrow obligations, but existing debt, interest,
and other scheduled balance changes may still evolve under their own contracts.

The corrective successor clears an option's atomic mark pair whenever its
declared underlying is unavailable, with book/instrument identity revalidation
under the exchange lock. The expiry mark path and settlement-pending boundary
use the same invalidation rule. A regression now proves that a previously
valid option mark becomes `ErrNoPrice` and cannot be used by `riskMark`; the
contract wording is narrowed to new obligations. This successor remains
unapproved until a fresh independent review and complete post-fix gates pass.

## Corrective changes awaiting gate

The worktree candidate contains minimal semantic/hardening changes:

1. build profiles only resolve marks for nonzero exposure;
2. liquidation scans continue after an unpriceable account;
3. all successful marks are installed before the cross-margin risk pass;
4. expiry-pending marks are cleared at the lifecycle boundary, and a pending
   exposure fails the whole account profile closed;
5. funding client and venue arithmetic is preflighted atomically, with hedge
   legs aggregated per client, an in-lock zero-cash settlement marker containing
   mark/precision terms, and an independent expected-payment replay;
6. collateral-interest debits and venue revenue are preflighted atomically and
   logged by wallet;
7. venue ledger records carry strict contiguous sequence and trade identity, and
   terminal reports persist the final sequence;
8. derivative/conservation scorers fail closed on malformed wallet, sequence,
   side, or settlement/funding-failure evidence, and use physical same-file
   order when reconstructing funding exposure.

A previous independent Sol review rejected an earlier hardening candidate for
funding atomicity, wallet-blind interest evidence, venue-order validation, and
partial interest mutation. A subsequent review found and led to two more
defects: funding client-delta overflow could be masked by venue-flow arithmetic,
and the derivative scorer mishandled net-zero hedge legs. Those findings are
fixed and regression-tested. A fresh independent Sol-xhigh review of this
combined candidate is still required; its verdict will be appended here before
the semantic commit. The final full `make test` gate passed on 2026-08-30,
including the integrated long-run contract and archive tests; `go vet ./...`
and `git diff --check` also passed.

## Gate decision

The performance branch is an independent audit source, not an implementation
oracle. The confirmed F1/F2/F3 defects are corrected in the scientific
worktree, and their regressions pass in focused tests. Because F3 was reachable
in the retained portfolio topology and can change risk decisions, the old
dev-607 trajectory is preserved but cannot be promoted as the corrected
candidate. No dev-613, dev-617, parity control, or holdout cell is authorized
until the fresh review, full tests, commit, and clean provenance-pinned rebuild
complete. Holdouts `619/631/641` remain untouched.

## Fresh independent review — `1829bd2` (2026-08-30)

Schrodinger (Sol-xhigh) independently reviewed the exact pushed archive
candidate `1829bd2c76b3274d27def0e49ccf387623289b91` and **REJECTED** it before
any R2 cell ran.  The calendar census gave listings priority over settlements
at equal timestamps, so its replay could report a coexistence peak that the
physical record order did not contain and could hide a settlement-before-listing
violation.  The archive adapter also did not run fresh derived-artifact
recomputation before allowing raw JSONL pruning; stored passing sidecars alone
were sufficient.

The current corrective successor preserves same-file physical order and
classifies a reversed same-timestamp lifecycle as a failed measurement.  Its
archive path calls the clean `verify-v2-integrated-longrun-r2-cell.sh` raw
recomputation immediately before any prune.  Regression tests pass for both
cases.  The rejected verdict remains historical; the corrective successor
requires a new independent review and full gate before development cells.

## Follow-up independent review — `b1e41e7` (2026-08-30)

Maxwell (Sol-xhigh) independently reviewed exact pushed commit
`b1e41e7e1048a17af65854db1645b320855cb90d` and **REJECTED** it before any R2
cell ran.  Equal-timestamp lifecycle records from different files were still
ordered lexically by filename, contrary to the evidence-order contract that
marks such records ambiguous.  The G8 archive helper also intentionally did
nothing, so G8 raw pruning was protected only by stored parity sidecars.

The current successor rejects cross-file same-timestamp lifecycle groups and
adds a regression; G8 pruning calls a verify-existing parity recomputation and
requires its fresh attestation to match the sealed one without overwriting it.
The rejected verdict remains historical and no raw, development, or holdout
evidence was deleted or run.

## Current corrective gate — `2c79654` (2026-08-30)

The current successor fails closed on same-venue equal-timestamp lifecycle
records split across files and adds a regression for that ambiguity.  Its G8
archive path performs fresh parity/raw recomputation in a temporary attestation
comparison mode before any prune.  The exact pushed commit has passed focused
tests, full `make test`, `go vet`, shell syntax, and diff checks; fresh
independent Sol-xhigh acceptance is still required before any R2 cell.

## Lock/provenance successor — `211865e` (2026-08-30)

The namespace-lock correction is committed and pushed at exact HEAD
`211865e8f9cf2b93a0960267796f3cbedc30636b`. Nested R2 commands now inherit a
validated open descriptor whose `/proc` target is the exact non-symlink lock
path; a Boolean environment marker is no longer sufficient to bypass locking.
The contract test includes a live contention case with a forged marker and the
archive test exercises the real G8 parity/prune path. Both focused R2 tests
pass, as does ordinary `make test` and `go vet ./...`. The affected packages
also pass `GOMAXPROCS=4 go test -race ./analysis ./cmd/mvanalyze ./cmd/prunegate
./tests`.

The repository-wide race run remains a limitation rather than a pass: its
unrelated existing V23 P3 replenishment test timed out after ten minutes with
no race report or OOM. This is retained as a gate limitation and has not been
silently relabeled green.

At the natural checkpoint, the independent performance branch advanced from
`8f0186f` to `1514c9c`. Commits `e71e941`, `42ebac1`, `65e8f24`, `c72700d`,
`5d41bd9`, and `1514c9c` are performance/report/schema work only. The binary
evidence prototype remains deferred: it covers only a subset of event families
and has no complete simulator/analyzer differential, global-completeness,
end-to-end, or independent-promotion gate. No performance code was merged.

The fresh Sol-xhigh review requested for `211865e` could not complete because
the reviewer service hit its usage limit. This is an unavailable review, not
an acceptance. Consequently the candidate remains unapproved: no clean
scientific binary rebuild, R2 development cell, parity control, or holdout has
been launched from this successor. Holdouts `619/631/641` remain untouched;
all synthetic lock/archive fixtures were quarantined outside the canonical
 evidence namespace and are not scientific results.

## Calendar extraction correction — `ff5e714` (2026-08-30)

The fresh review of `257833f` rejected it before any R2 cell ran. It found
three gate defects: both extractor jq programs referenced the expected listing
timeline without binding it; the registered timeline expected epoch-origin
contracts at the calendar epoch even though the first one-second automation
poll occurs at epoch+1s; and `MeasureCalendar` silently discarded a lifecycle
payload that could not decode into the typed payload.

`ff5e714` fixes all three. The expected timeline is now passed explicitly to
both jq programs. For the zero-phase R2 calendar, an expiry requested at the
epoch is attested at epoch+1s, while later whole-hour requests are attested at
their coincident one-second poll; the expiry timestamp itself is unchanged.
Malformed selected lifecycle payloads now make the calendar analyzer return an
error, while structurally decodable records with missing/invalid identity still
increment the existing fail-closed malformed counter. A simulator-level
full-log regression verifies the first future listing at epoch+1s with the
calendar expiry at epoch+2h.

Focused calendar, multivenue, and R2 contract tests pass. The first full
`make test` started before the timing correction was complete and failed only
because its clean-worktree parity guard saw the in-progress edit; the clean
committed rerun passed all packages and R2 contract/archive checks. Fresh
`go vet ./...` and a new independent Sol-xhigh review of exact `ff5e714` remain
required before rebuild or dev-607. No R2 scientific cell has run and
holdouts `619/631/641` remain untouched.

At this semantic-commit checkpoint, the performance branch was fetched from
`1514c9c` through `bcb9e91`. The new range contains binary evidence fixes for
false execution attestations and unencodable payload truncation, typed
order-lifecycle schemas, binary rendering/index work, and benchmark reports.
These are relevant to future evidence infrastructure but remain a separate
performance/VNext prototype: no active-branch semantic market change was
identified, and it still lacks the complete promotion contract for the current
scientific campaign. Nothing from that branch was merged.

## Typed lifecycle correction — `b906705` (2026-08-30)

The fresh review of exact `666548e` rejected it before any R2 cell ran. It
identified a remaining fail-closed hole: a decodable lifecycle payload whose
`instrument_type` was missing, null, empty, or unknown was silently ignored.
That could let an otherwise complete calendar pass after losing a lifecycle
record.

`b906705` makes the lifecycle type pointer-valued so missing and null are
distinguishable, explicitly accepts only the known non-derivative types `SPOT`
and `PERP` for calendar exclusion, and rejects empty or unknown types. FUTURE
and OPTION continue through the derivative calendar path and retain its
existing malformed identity counters. Regression cases place missing, null,
and unknown types beside a valid lifecycle record, and separately prove that
SPOT/PERP records do not contaminate the derivative census.

The clean committed full suite, `go vet ./...`, targeted race suite, R2
contract test, and diff checks pass on `b906705`. A fresh independent Sol-xhigh
review of this exact candidate is now required. No clean scientific rebuild,
development cell, parity control, or holdout has run; holdouts
`619/631/641` remain untouched.

## Timeline gate review — `f444adb` (2026-08-31)

Lagrange (Sol-xhigh) independently reviewed exact commit
`f444adb9c5313b5950e350b017dc20958d018cb4` and **REJECTED** it before any R2
cell ran. The review found that the empty-string instrument type did not have
its own adversarial regression, the end-to-end timeline test advanced only two
simulated seconds and therefore did not prove later hourly/three-hour/
six-hour polls or the 24-hour endpoint, and the shell timeline fixture was
derived from the same production helper as the expected value. These findings
were valid gate weaknesses, not evidence that the calendar implementation had
the wrong economic schedule.

Commit `1903679` corrects the gate without changing the registered calendar:
it adds the empty-type regression, adds a package-local exchange test with an
independent manual clock/ticker that advances every one-second automation poll
through 24 hours, and asserts exact first-listing times including the
intentional `+30h` overlap (listed first at `+18h`, then requested again at
`+24h` and deduplicated). The shell contract now contains a literal 28-row
timeline fixture rather than generating the expected answer from production
code. A separate consistency check confirms that this fixture matches the
maintained schedule helper, while the adversarial contract mutation still
rejects an all-at-zero timeline.

Focused calendar tests and the R2 contract pass. The clean full `make test`,
`go vet ./...`, targeted `go test -race
./analysis ./cmd/mvanalyze ./cmd/prunegate ./tests`, and diff checks pass on
`1903679`. This is a gate-hardening correction only; the exact candidate still
requires fresh independent Sol-xhigh acceptance before a clean binary rebuild
or development cell. No R2 development, parity, or holdout evidence has run;
holdouts `619/631/641` remain untouched.

## Performance red-team checkpoint — `6b51e97` (2026-08-31)

At the natural pre-promotion checkpoint, `origin/autoresearch/v2-performance-research`
was fetched and inspected from last-seen `bcb9e91` through exact head
`6b51e973d4ad32f483ce2b3bf0b05514367efac4`. The new work is performance/VNext
evidence infrastructure, not a scientific-branch market-logic change. The
relevant safety correction `052c663` makes analyzers and `prunegate` refuse a
declared evidence format they cannot read; the binary corpus/rendering work
also reports unresolved file-layout routing and promotion blockers. The
fingerprint fast path and allocation/GC measurements are performance-only.

No new plausible risk, matching, lifecycle, or historical-impact finding was
identified on the active scientific HEAD, so no reproduction or semantic fix
is warranted. Nothing was merged, the current JSONL evidence contract remains
in force, and the next performance comparison starts at `6b51e97`.

## Promotion review — `83dc7b1` rejected by Banach (2026-08-31)

Banach (Sol-xhigh) independently reviewed exact clean HEAD
`83dc7b18ea4381c99a366adc115ef7f09366382a` and **REJECTED** promotion to
dev-607. The review found that `MeasureCalendar` accepted an empty lifecycle
`venue_id`, and the extractor required only three venue rows rather than the
registered exact set `central,north,south`. A renamed or missing venue could
therefore masquerade as a complete calendar. It also found that the shell
literal timeline was passed as both fixture and expected argument, while real
extraction still obtained its expectation from the production helper; the
claimed independence was not wired into the gate. Finally, only 28 GiB was
free while a retained full run measured 30,073,924,660 raw JSONL bytes and a
35,341,880,370-byte complete tree, making the old 5 GiB launch floor unsafe.

Commit `494d696` is the minimal correction. The analyzer now rejects missing
or empty event venue identity and supports a validated caller-supplied expected
venue set. The R2 extractor and both activation/integrity predicates require
exactly the registered three venues; contract tests mutate a venue to renamed
and empty values. The shell test directly compares its literal normalized
timeline with the maintained helper, then exercises the timeline checker with
its default expected value, preserving the all-at-zero mutation. The runner
now requires capacity derived from the retained full-tree measurement (1.5x
plus 2 GiB), approximately 51 GiB free, before launch.

Focused calendar/exchange tests and the R2 contract pass. The dirty-tree full
suite reached all Go/R2 tests but correctly stopped at parity/archive checks;
those checks require a clean worktree. A clean rerun, vet, targeted race suite,
and fresh independent Sol-xhigh review of exact `494d696` are required before
binary rebuild or dev-607. The current 28 GiB free space intentionally fails
the new launch preflight; no evidence was written and holdouts
`619/631/641` remain untouched.

## Corrected clean mechanical gate — `558fe21` (2026-08-31)

After Banach's rejection of `83dc7b1`, the `494d696` correction was committed
and the state record pushed as `558fe21`. The clean exact-tree full `make test`
passed, including all package tests and R2 contract/archive tests. `go vet
./...` and targeted race coverage for `./analysis ./cmd/mvanalyze
./cmd/prunegate ./tests` also passed. No experiment or evidence cell ran.

Promotion remains blocked pending fresh independent Sol-xhigh acceptance of
the exact current HEAD. Independently measured disk capacity is also below the
runner’s approximately 51 GiB fail-closed floor (about 28 GiB currently
free), so dev-607 cannot safely launch even after review until capacity is
resolved. Holdouts `619/631/641` remain untouched.
