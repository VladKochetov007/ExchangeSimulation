# Long FROZEN-2 candidate gate: independent review

Date: 2026-08-27

## Scope

This review was performed independently after the candidate runs were
complete.  It examined the candidate configuration, run metadata, checkpoint
streams, runtime/offline evidence digests, sidecars, the `548f92c` source
correction, and the focused/full test results.  No simulation or evidence file
was modified by the reviewer.

## Verdict

**ACCEPT WITH NARROWER CLAIM.**

The package establishes exact fresh-process, parallelism, and logging
neutrality for the declared ordered `execution_observations` domain of the
historical FROZEN-2 control configuration.  It also establishes exact
reconstruction of the persisted JSON-record multiset for the full-log cases.
It is not an immutable integrated-V2 freeze package.

## Evidence checked

Four independent 24-hour seed-101 cases (full logging and no logging at
GOMAXPROCS 10, 1, and 4) each contain 1,441 checkpoints.  Every checkpoint
object is byte-identical across the four cases, ending at 105,712,997
observations with execution digest
`d7c0952906af41316391d9ce436f88aa86213e2c5f105e857b5f7321cb893c30`.
`greeks.json` and `latency.json` are byte-identical as well.  Full-log seeds
101, 102, and 103 have successful zero-exit runs, final sidecars, and exact
runtime/offline persisted-record digest equality:

| seed | observations | execution digest | evidence digest |
|---:|---:|---|---|
| 101 | 105,712,997 | `d7c09529…` | `f10918bf…` |
| 102 | 111,419,965 | `4085ad89…` | `547d3431…` |
| 103 | 105,533,283 | `e4b2f3b6…` | `d622d65a…` |

The complete values and per-case config/sidecar hashes are in
`research/artifacts/v2-freeze-candidate/long-candidate-determinism-attestation.json`.

The full repository test suite and vet pass at the current source state.  The
changed-boundary race suite (`exchange`, `matching`, `price`, `logger`,
`analysis`, and `simulation`) passes.  The broad race suite remains limited by
the pre-existing long-running replenishment test timeout; no race report was
observed.

## Important claim boundaries

1. The candidate uses the historical shared-consensus FROZEN-2 control.  Its
   local-feed, router, post-only, liability/carry, and receipt/frontier
   mechanisms are not enabled.  The result therefore does not attest an
   integrated V2 ecology or its information-boundary claims.
2. Checkpoints hash ordered execution observations; evidence hashes identify
   an unordered multiset of persisted JSON records.  Equality is not claimed
   for unlogged scheduler ticks, no-op decisions, or evidence-only sidecars.
3. `548f92c` is a simulator-observation semantic correction: canonical
   cross-margin book traversal changes which unavailable-mark diagnostic is
   selected.  The affected caller discards the whole profile on any such
   error, and the focused test plus byte-identical sidecars support no changed
   successful-path economic transition.  It must not be labelled a purely
   instrumentation-only change.
4. GOMAXPROCS repeat configs differ in identity/description metadata but have
   byte-identical normalized behavioral fields; this equivalence is recorded
   rather than hidden.
5. The evidence artifact hash is a persisted-record multiset digest.  The
   package does not claim crash recovery or directory-level durability beyond
   the validated zero-exit, final-sidecar, fresh-directory contract.

## Promotion decision

Do not declare an immutable V2 freeze from this package alone.  Before freeze,
select and preregister the integrated V2 reference configuration, bind its
mechanical/accounting/lifecycle/risk/information/mutation gates to one clean
source revision, and then generate final calibration and holdout evidence from
that declared revision.  No prior evidence is invalidated by this review.
