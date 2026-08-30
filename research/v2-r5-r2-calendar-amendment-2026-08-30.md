# V2 r5 R2 calendar amendment

Status: design and implementation gate; successor candidate to the rolling-
tenor r5 population.  This document does not amend or rescore historical
artifacts.

Date: 2026-08-30

Scientific parent: `e29f26b` (`autoresearch/ffa-ecology-gen0`)

Performance feed last reviewed: `9dc6b08`

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
| A | 2h / 6h | 4h / 12h | 8h / 16h | 12 + 6 + 3 = 21 | 10 | enough density, natural overlaps, bounded board |
| B | 1h / 4h | 3h / 8h | 6h / 16h | 24 + 8 + 4 = 36 | 21 | too many books and option chains for a first 24h gate |
| C | 3h / 8h | 6h / 16h | 12h / 24h | 8 + 4 + 2 = 14 | 8 | sparse and phase-zero families do not exercise the desired overlap pattern |
| D | 2h / 6h | 6h / 12h | 12h / 24h | 12 + 4 + 2 = 18 | 10 | viable, but fewer medium and long replacement observations |

Candidate A is registered.  Its distinct expiry sequence through 24h is:

```
6, 8, 10, 12, 14, 16, 18, 20, 22, 24 hours
```

The short family requests expiries `6,8,10,...,28`; medium requests
`12,16,20,24,28,32`; long requests `16,24,32`.  Thus short/medium collide at
12h, 16h, 20h, and 24h, and all three collide at 16h and 24h.  At the start
the board has three maturities (6h, 12h, 16h); after the first short expiry,
new listings replace expiring contracts while several later maturities remain
simultaneously open.  The calendar produces these overlaps arithmetically; it
does not inspect prices or encode a convergence target.

The schedule is deliberately compressed rather than a literal month model.
The 16h long lead is longer than the 12h medium lead while still allowing two
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
4. Deduplicate requests by expiry before creating instruments.  The futures
   symbol remains derived from underlying/base and expiry, so a collision is
   visible as one economic book.
5. For options, deduplicate by expiry before building the call/put chain.  A
   configured strike cap remains a safety bound, not a second listing event.
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
next review starts at `9dc6b08`.

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
