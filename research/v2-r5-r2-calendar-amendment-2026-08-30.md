# V2 r5 R2 calendar amendment

Status: design and implementation gate; successor candidate to the rolling-
tenor r5 population.  This document does not amend or rescore historical
artifacts.

Date: 2026-08-30

Scientific parent: `e29f26b` (`autoresearch/ffa-ecology-gen0`)

Performance feed last reviewed: `52a3eca`

## Decision

R2 adopts an explicit expiry calendar and a separate listing policy.

The economic identity of a dated contract is:

```
underlying + contract type + expiry timestamp
```

The schedule family is not part of that identity.  Schedule families generate
desired listings.  If two families request the same expiry, the lister emits
one instrument and advances both family cursors.  A collision is therefore a
deduplication event, never a parked cursor or a failed listing.

The same calendar is supplied to the futures lister and option-chain lister.
For an option expiry, one fixed strike chain (calls and puts) is constructed
on the first successful listing request.  Later requests for that expiry reuse
the chain and cannot append a new spot-centered chain.  This makes an option
expiry a calendar object rather than a repeatedly re-centered ladder.

The calendar epoch is the deterministic simulation start (`2025-01-01
00:00:00 UTC` in `multivenue`).  A schedule with interval `I`, lead `L`, phase
`P`, and non-negative index `k` requests:

```
listing_time = epoch + P + k*I
expiry       = listing_time + L
```

Listings are published on the first automation poll at or after their
scheduled listing time.  An overdue request whose expiry has already passed is
discarded rather than creating an already-expired instrument.  If an option
price is unavailable, due requests are retained for retry; the cursor advances
only after the batch is successfully listed.  This is a source-availability
failure, not collision semantics.

The calendar mode is opt-in in `multivenue`.  Configurations without the new
calendar field continue to use the historical rolling `TenorsNano` path, so
old evidence remains exactly attributable to its original population.

## Compressed schedule comparison

All rows use phase zero and count listing requests in the half-open 24-hour
window `[0h,24h)`.  “Completed” counts distinct expiries at or before 24h;
shared expiries are counted once.

| candidate | short interval/lead | medium interval/lead | long interval/lead | family requests | completed distinct expiries | assessment |
|---|---:|---:|---:|---:|---:|---|
| A | 1h / 2h | 3h / 6h | 6h / 12h | 24 + 8 + 4 = 36 | 23 | dense short replacement, repeated natural overlaps, bounded active board |
| B | 1h / 3h | 4h / 9h | 8h / 18h | 24 + 6 + 3 = 33 | 22 | useful staggered terms, but fewer multi-family collisions |
| C | 2h / 6h | 4h / 12h | 8h / 16h | 12 + 6 + 3 = 21 | 10 | sparse; the old rolling-style density leaves fewer complete cycles |
| D | 2h / 4h | 6h / 10h | 12h / 20h | 12 + 4 + 2 = 18 | 11 | viable, but medium/long overlaps are too infrequent |

Candidate A is the selected successor schedule.  Its distinct short-family expiry sequence through
the 24h listing horizon is:

```
2, 3, 4, ..., 24, 25 hours
```

The short family requests expiries `2,3,4,...,25`; medium requests
`6,9,12,15,18,21,24,27`; long requests `12,18,24,30`.  Thus short/medium
collide at `6,9,12,15,18,21,24`, and all three collide at `12,18,24`.  At the
start the board has three maturities (2h, 6h, 12h); after the first short
expiry, hourly listings replace expiring contracts while medium and long
maturities remain simultaneously open.  Twenty-three distinct expiries
complete by 24h.  The deterministic runner performs its final automation poll
at exactly 24h, so the realized listing census also includes the endpoint
requests: short expiry 26h, medium expiry 30h, and long expiry 36h (with the
already-listed 30h collision deduplicated).  The resulting realized set has
28 expiries, while the 25h, 26h, 27h, 30h, and 36h contracts provide forward
term structure at the terminal boundary.  The calendar produces these
overlaps arithmetically; it does not inspect prices or encode a convergence
target.

The successor registry is `research/configs/v2-integrated-longrun-r2/`.  The
development runner is `scripts/run-v2-integrated-longrun-r2-cell.sh`; its
development set is `dev-607`, `dev-613`, and `dev-617`, with `dev-607-none`
and `dev-607-g8` reserved as parity controls.  The three holdout configuration
files are copied into the namespace only as reserved declarations; the R2
development runner rejects them and the development scorer asserts that their
outputs do not exist.
Completed full cells are sealed with
`scripts/archive-v2-integrated-longrun-r2-cell.sh`; its optional prune action
is authorized only after the archive, descriptor, external attestation, and
manifest verify.

The schedule is deliberately compressed rather than a literal month model.
The 12h long lead is longer than the 6h medium lead while still allowing three
long-family expiry observations in a 24h run.  A later real-time calibration
may scale these intervals without changing the identity or deduplication
contract.

## Lifecycle contract

1. At construction, validate that names are unique and non-empty, intervals
   and leads are positive, and phase is in `[0, interval)`.
2. On every poll, generate all family requests whose scheduled listing time is
   due, in deterministic `(listing time, expiry, family name, index)` order.
3. Advance every due family cursor, including requests whose expiry is already
   past the poll time.  Do not advance a batch that cannot be priced or
   constructed.
4. Deduplicate requests by expiry before creating instruments.  Calendar
   futures and options use an injective canonical symbol component for the
   underlying plus expiry (and raw strike units for options), so a collision
   is visible as one economic book without cross-underlying symbol aliasing.
5. For options, deduplicate by expiry before building the call/put chain.  A
   configured strike cap remains a safety bound, not a second listing event.

Calendar symbols use a parser-compatible canonical grammar.  Futures place the
expiry token immediately after `-FUT-` and append the hex-encoded underlying;
options place the hex-encoded underlying before the expiry and use a `K<raw>`
strike token.  The analyzers decode these tokens using the typed quote
precision, while legacy rolling symbols retain their historical spelling.
This preserves exact fixed-point identity without silently dropping dated
basis or option-surface observations.

6. Expiry, pending settlement, mark invalidation, and settlement continue to
   use the existing instrument lifecycle.  Calendar listing does not settle a
   contract early and does not alter price discovery.

## Actor compatibility

The calendar does not expose a family as a tradable identity.  Existing actors
that select all dated futures already observe the complete term structure.
Legacy P5 actors retain their `TargetTenor` selector for historical configs;
the R2 successor must use an expiry-aware selector (nearest eligible expiry or
an explicit time-to-expiry range), not infer a schedule family from a symbol.
That actor change is a separate semantic review item if a P5 successor cell
enables it.

## Historical impact and rerun policy

No historical run was generated with the new calendar field.  Retained
manifests and source configuration show that prior derivative, carry, option,
and lifecycle runs used the rolling lister or a fixed single-tenor board.  They
remain valid only for those declared populations; this amendment does not
retroactively relabel them.

The affected claim boundary is therefore:

| artifact class | R2 impact | action |
|---|---|---|
| old futures/options lifecycle and P3/P3e evidence | population and replacement process differ | retain; do not rescore as R2 |
| old P5 configurations and dated-carry records | selector and available maturity set differ | retain as rolling-ladder evidence; successor P5 requires new dev cells |
| old funding, margin, distress, and non-derivative worlds | calendar condition absent | no rerun for this amendment |
| untouched holdouts `619/631/641` | no calendar run and no authorization | remain untouched |
| new R2 successor dev cells | calendar is part of the registered world | rerun only the authorized development cells after clean build/review |

The old results are not “repaired” offline.  If a successor’s actor or
lifecycle decision changes, its trajectory must be generated by the successor
binary.  The existing integrated-long-run raw archives remain preserved.

## Independent-feed checkpoint

The performance branch `origin/autoresearch/v2-performance-research` advanced
from `2989934` to `9dc6b08`.  The new commits measure exact allocations,
conditional book-delta construction, high-frequency no-ops, and hash payload
encoding.  The agent retracted its sampled allocation and appender-coverage
claims; the remaining changes are performance-only and were not merged.  The
then advanced from `9dc6b08` through `feeb6f9` and `e3558df`, a prototype
canonical binary evidence stream followed by additional typed payload work.
Its encode/decode measurements are promising, but it is not an end-to-end,
all-event, differential-tested replacement for the current JSON evidence
contract, so it remains unmerged.  The branch then added `7fdf55f`, an indexed
query prototype with differential query-class tests.  It remains performance
only and unmerged; the next review starts at `7fdf55f`.  The next two commits,
`4b370fa` and `52a3eca`, measured an Amdahl ceiling and decomposed it with a
typed fixed-width hash probe.  Removing serialization and hashing entirely
reduced the measured `dev-607-none` wall time by 17.29%; hashing an 88-byte
typed payload instead of one byte was statistically indistinguishable from
that ceiling.  The evidence therefore supports reflection/allocation removal
as the simulator-CPU mechanism, while compactness remains a storage and
analytics benefit.  These are measurement builds and do not establish a
production replacement for the JSON evidence contract.  No code was merged;
the next feed comparison starts at `52a3eca`.

## Independent semantic review and corrective work

The exact implementation commit `80b5095` was reviewed by an independent
Sol-xhigh agent.  The review was **REJECTED** before any successor cell ran:

1. the successor configs were not yet present in that committed snapshot;
2. calendar symbols did not encode the underlying, so two quote markets could
   collide in the exchange symbol map;
3. arbitrary fixed-point option strikes could collide after integer division;
4. a non-positive rounded strike grid could mark an empty option expiry as
   listed.

The first item is addressed in the successor registry and protocol scripts.
The remaining three are addressed in the successor implementation: calendar
futures/options use an injective hex-encoded underlying component; calendar
options use raw-unit strike labels; and a calendar cursor is not committed
when a due expiry cannot produce a valid call/put chain.  Regression and
`NewSim` activation tests cover these cases.  A fresh independent review of
the resulting commit is required before any development cell.  The rejected
verdict remains historical and is not silently rewritten.

The exact successor implementation `aed83a6` was then reviewed by Hooke
(Sol-xhigh) and **REJECTED**.  The review found three additional gate defects:

1. the new future and option symbol grammars were not consumable by the
   existing dated-basis and option-surface parsers, so R2 derivative evidence
   could be silently discarded;
2. a collision-only option request still performed an unnecessary spot lookup,
   allowing an unavailable price to stall a schedule that required no new
   chain; and
3. the protocol scorer did not attest the defining 24-hour calendar behavior.

The corrective successor moves expiry into the parser-visible symbol token,
decodes raw option strikes with quote precision, filters collision-only
requests before any price lookup, and adds the Go-native `calendar` metric.
The extractor now precommits and fail-closes on the selected 28-expiry set,
23 completed expiry cycles, equal futures/options expiry sets, duplicate-free
identities, and at least three simultaneous futures and option maturities at
each venue.  This review remains a rejected historical gate until the exact
corrective commit receives a fresh independent review.

The fresh independent review of exact commit
`1829bd2c76b3274d27def0e49ccf387623289b91` was also **REJECTED** before any R2
cell ran.  It found that the calendar census sorted all same-timestamp
listings before settlements, allowing a synthetic coexistence peak and masking
a settlement-before-listing sequence.  It also found that the raw archiver
checked only stored booleans and expiry-array lengths before pruning; fabricated
derived artifacts could therefore authorize deletion of raw JSONL without a
fresh recomputation.  The corrective change removes the listing-priority sort,
carries file/ordinal lifecycle positions into the identity pass, adds
same-timestamp regression tests, and invokes the existing clean raw-evidence
verifier immediately before prune.  No raw evidence was deleted and no
development or holdout cell ran from the rejected commit.

The follow-up review of exact commit
`b1e41e7e1048a17af65854db1645b320855cb90d` was **REJECTED** before any R2
cell ran.  It correctly noted that lexically ordering equal-timestamp records
from different files still invented causality, despite the same-file fix, and
that the supported `dev-607-g8` archive path still skipped fresh recomputation.
The next correction rejects any same-venue, same-timestamp lifecycle group
spanning multiple files, making the calendar metric fail closed; it adds a
cross-file regression.  G8 pruning now invokes a verify-existing parity
recomputation that compares a newly generated attestation without overwriting
the sealed one.  No raw evidence was deleted and no development or holdout
cell ran from `b1e41e7`.

## Gate status

This is a semantic amendment, not a no-op repair.  Before any R2 development
cell, the following remain required:

- implementation and edge-case regression tests;
- deterministic listing and option-chain parity tests;
- full test, race, and clean-build gates;
- fresh independent Sol-xhigh review of the exact successor commit;
- updated provenance and a new successor candidate identifier.

No holdout is authorized by this document.  The old r5 hardening commit is
`e29f26b`; its fresh independent approval is also still outstanding because
the first post-fix reviewer slot was unavailable.

## Current corrective gate — `2c79654` (2026-08-30)

The `2c79654` correction rejects any same-venue, same-timestamp lifecycle
group spanning multiple persisted files, because the evidence contract has no
global cross-file ordinal.  It adds a cross-file regression and preserves
same-file physical order.  G8 pruning now runs the parity checker in
verify-existing mode; that checker stages archived/raw evidence, recomputes
raw digests and control identities, and compares a temporary fresh attestation
to the sealed one without overwriting it.  Focused tests, full `make test`,
`go vet`, shell syntax, and diff checks pass.  Fresh independent Sol-xhigh
acceptance of the exact pushed commit remains mandatory before any R2 cell.

## Lock/provenance successor — `211865e` (2026-08-30)

The namespace-lock correction is committed and pushed at exact HEAD
`211865e8f9cf2b93a0960267796f3cbedc30636b`. Nested R2 commands now inherit a
validated descriptor for the exact non-symlink lock path; a forged Boolean
environment marker cannot bypass contention. The contract test proves this
case, and the archive test exercises real G8 parity verification and pruning.
Focused R2 contract/archive tests, ordinary `make test`, `go vet ./...`, and
the affected-package race suite pass. The repository-wide race run timed out
in the unrelated existing V23 P3 replenishment test after ten minutes, with no
race report or OOM; it remains a documented limitation.

The asynchronous performance branch was fetched at the same checkpoint through
`1514c9c`. Its new commits are performance-only binary-schema, benchmark,
measurement, and report work. The prototype is not integrated into this
scientific candidate because complete event-family coverage, differential
analyzer/global-order proof, end-to-end A/B, and independent promotion are
still absent.

The requested fresh Sol-xhigh review could not complete because the reviewer
service reached its usage limit. The candidate is therefore not accepted and
the R2 sequence remains stopped before clean binary rebuild and development
execution. No development, parity, or holdout cell has run; holdouts
`619/631/641` remain untouched. Synthetic gate fixtures are quarantined
outside the canonical namespace and are not scientific evidence.

## Timeline and malformed-input correction — `ff5e714` (2026-08-30)

The independent review of exact `257833f` rejected the successor before any R2
cell ran. Its three findings were valid: the extractor failed to bind the
expected timeline into two jq programs; its expected first-listing timestamps
were at the epoch although the exchange's first one-second automation poll is
epoch+1s; and the calendar analyzer silently ignored a lifecycle payload that
could not decode.

`ff5e714` binds the value into both predicates, attests the actual zero-phase
poll schedule (`epoch+1s` only for epoch-origin requests; later whole-hour
requests at their matching poll), and fails closed on malformed selected
lifecycle payloads. The calendar expiry remains the requested calendar date,
not the poll timestamp. A full-log `NewSim` regression proves the first future
listing is observed at epoch+1s and expires at epoch+2h. Focused tests and the
clean committed full `make test` pass, including R2 contract/archive checks;
the earlier dirty-tree run is retained as a gate diagnostic, not a scientific
result. `go vet ./...` and fresh exact-tree independent review remain pending.

The performance feed was refreshed through `bcb9e91` from last-seen `1514c9c`.
Its binary evidence work is performance/VNext-only, includes useful attestation
and losslessness corrections, and is deferred from this calendar candidate
pending complete schema, ordering, analyzer differential, and promotion review.
No performance code was merged.

## Typed lifecycle correction — `b906705` (2026-08-30)

The fresh review of `666548e` rejected that candidate before any R2 cell ran:
decodable lifecycle records with missing, null, empty, or unknown
`instrument_type` values were silently omitted from the calendar audit.

`b906705` now distinguishes missing/null from a present type, explicitly
recognizes `SPOT` and `PERP` as valid non-derivative lifecycle records to
exclude, and fails closed on every other absent, empty, or unknown type. Tests
cover missing/null/unknown records alongside a valid calendar and verify that
known SPOT/PERP records remain excluded. Clean full `make test`, `go vet`, the
targeted race suite, and R2 contract checks pass. Fresh exact-tree independent
review remains pending; no R2 or holdout evidence has been run.

## Timeline gate review — `f444adb` and correction `1903679` (2026-08-31)

Lagrange (Sol-xhigh) rejected exact `f444adb` before any R2 cell ran. The
review identified three remaining evidence-contract weaknesses: empty-string
`instrument_type` lacked a dedicated adversarial case; the end-to-end test
covered only two seconds instead of the later listing polls and 24-hour
endpoint; and the shell expected timeline was generated by the same helper
that defined the schedule. The review did not establish a semantic failure of
the chosen 1h/2h, 3h/6h, and 6h/12h compressed calendar.

`1903679` adds the missing empty-type case, a real exchange lifecycle test that
advances a manual clock through all 86,400 one-second polls, and exact checks
for later whole-hour requests, endpoint listings, and the deliberate `+30h`
schedule-family overlap. The shell contract now uses an explicit literal
28-expiry fixture. The fixture independently agrees with the maintained
calendar helper, but the contract no longer obtains its expected value from
the implementation under test. Full tests, vet, targeted race checks, focused
calendar tests, and the R2 contract pass.

The amendment remains unapproved pending fresh independent Sol-xhigh review
of exact `1903679`. No clean scientific binaries or development cells have
been run from this candidate, and holdouts `619/631/641` remain untouched.

The performance red-team checkpoint also advanced from `bcb9e91` through
`6b51e973d4ad32f483ce2b3bf0b05514367efac4`. Its new binary evidence/analyzer
work remains separate and does not alter the R2 calendar or JSONL contract;
the branch itself records that file-layout routing and promotion gates remain
open. No active-HEAD semantic finding was reported or reproduced. The next
incremental performance review begins at `6b51e97`.

## Promotion review — `83dc7b1` rejected by Banach (2026-08-31)

Banach (Sol-xhigh) rejected exact `83dc7b1` before any development cell. The
calendar analyzer did not bind lifecycle evidence to the exact registered
`central,north,south` venue set, and accepted empty/renamed venue identity.
The shell literal timeline was also not connected to the default production
expectation path: the test passed the same literal as its expected argument,
while extraction used the schedule helper. The review additionally showed
that the old 5 GiB disk floor was unsafe: retained full evidence measured
30,073,924,660 raw JSONL bytes and 35,341,880,370 bytes including the complete
tree, while the host had only about 28 GiB free.

`494d696` adds reusable expected-venue validation, exact R2 venue predicates,
missing/empty/renamed venue regressions, an independent literal/helper
comparison followed by default-path timeline checks, and a measured-capacity
launch floor of approximately 51 GiB free. Focused tests and the R2 contract
pass. The dirty full suite is not counted as a pass because its clean-worktree
parity/archive guard correctly rejected the uncommitted candidate; a clean
full gate, vet, targeted race check, and fresh exact-tree Sol-xhigh review are
still required. No binary rebuild, dev cell, parity control, or holdout has
run from `494d696`; holdouts `619/631/641` remain untouched.

The corrected clean gate completed on documentation commit `558fe21`: full
`GOMAXPROCS=4 make test`, `go vet ./...`, and targeted race tests pass on the
exact tree. This does not constitute promotion. Fresh independent Sol-xhigh
review remains required, and the approximately 51 GiB capacity floor is above
the host’s approximately 28 GiB free space. No R2 development, parity, or
holdout evidence has run.
