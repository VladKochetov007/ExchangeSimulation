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
