# SV1D activation bootstrap-frame contract repair — 2026-09-19

## Classification

This is a fail-closed evidence-contract correction, not an economic amendment.
The retained seed-659 activation from the predecessor candidate remains
`INVALID_EVIDENCE` and is not rescored. No R2 calendar, roster, actor,
matching, risk, seed, horizon, or holdout rule changed.

## Exact predecessor and retained failure

The failed activation was run from source commit
`66555a50348d9cda8af94966bb99c373f702e581`, tree
`0fced7671b458fa11f991287f7eee9760fb7bedf`, with the fresh Go 1.27 supplied
binaries and the reviewed SV1D bundle. The retained namespace is:

`/home/vlad/external-scratch/sv1d-activation-66555a5-20260919-cgroup8g`

It completed all three five-minute simulations and renderings, but the strict
score was `INVALID_EVIDENCE`; the launcher correctly exited nonzero and did
not certify an executable tri-arm result. The capacity preflight was separate,
complete, and verified; it remains outcome-ineligible.

The exact score failure was:

* treatment: strict evidence invalid, with the first failures including three
  `public CDF snapshot lacks explicit sequence or side presence` findings and
  three `public CDF snapshot lacks a verified public projection` findings;
* mode-off: three `public CDF snapshot lacks a verified public projection`
  findings;
* no-roster: three `public CDF snapshot lacks a verified public projection`
  findings.

Independent rendering of the retained treatment `events.evs` showed one
opening CDF/USD `BookSnapshot` per venue with explicit empty `bids`, `asks`,
`public_bids`, and `public_asks`, and `source_sequence=0`. The first
publishable snapshots at the same opening boundary carried positive source
sequences and valid complete-to-public projections. The three control failures
therefore matched the three venues and were not a supplier market outcome.

## Intended invariant

The simulator emits an explicit empty sequence-zero frame as its book
bootstrap. It is not an actor-observable market-data publication and must not
be used as a receipt frontier or an observed public-book sample. The registered
cadence contract already reconstructs the initial empty state and expects the
first publishable snapshot one observation interval after simulation start.

The strict parser now applies this exact rule:

1. a sequence-zero frame is silently admitted only when all four required side
   arrays are present and empty;
2. that bootstrap frame is skipped from snapshot identity/indexing and public
   observations;
3. a positive-sequence frame still requires non-nil sides and an exact,
   finite complete-to-public projection;
4. a sequence-zero frame with any depth, an omitted/null side, or malformed
   projection still fails closed.

This preserves the information boundary: no sequence-zero frame can become an
actor observation, while a malformed or nonempty zero-sequence frame cannot
hide from the audit.

## Correction and tests

Commit `3d1135f` (`fix: accept explicit empty SV1D bootstrap snapshots`) adds
the shared bootstrap predicate and applies it consistently to treatment,
mode-off, no-roster, strict availability, and legacy availability extraction.
Regression tests cover:

* acceptance of an explicit all-empty bootstrap;
* rejection of nonempty and omitted-side sequence-zero frames; and
* exclusion of the bootstrap from public observation extraction.

The focused suites passed on the modified tree:

`go test ./analysis ./cmd/sv1dprobe ./cmd/sv1dresource ./cmd/mvanalyze ./cmd/prunegate ./tests -count=1`

`git diff --check` also passed. A new independent review, clean full tests,
fresh pinned binaries, capacity preflight, and a fresh seed-659 probe are still
required. The old bundle and activation are not reused.

## Scientific impact and next boundary

The failed activation cannot support a scientific activation claim and remains
preserved as an invalid-evidence control. This parser defect did not alter any
historical scientific trajectory because no successor SV1D activation result
was accepted, and no 24-hour development cell or holdout was run under this
candidate. No historical result is rewritten.

The next gate is one fresh independent Luna review of the exact final tree
after the append-only state record, followed by a new Go 1.27 bundle, verified
finite-cgroup seed-977 capacity preflight, and only then another registered
development-only seed-659 activation. Development cells and holdouts remain
closed.
