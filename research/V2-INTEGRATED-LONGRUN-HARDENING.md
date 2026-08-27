# V2 integrated long-run gate hardening

Date: 2026-08-27
Status: pre-run; no development or holdout cell consumed
Contract: `v2-integrated-longrun-candidate-v2`

## Scope

This note records the control-plane changes made after reading the remote
handoff, current-state audit, and the unpacked private objective from the
parent archive. It does not reopen ae13f9a, P3e, signed-price, P4/P5, P6,
P7d, or the mixed-timing line.

## Precommitted decisions

* Full development cells are exactly 607, 613, and 617. Holdouts 619, 631,
  and 641 remain unconsumed and are not read by the development scorer.
* The runner records numeric `GOMAXPROCS`, immutable run metadata, source and
  binary identity, config/binary hashes, manifest identity, and completion
  sidecar hashes. It refuses dirty source, stale HEAD, reused output, and
  holdouts without explicit freeze authorization.
* The extractor runs registered analyzer metrics sequentially, validates every
  JSON result, records disabled P3/P4/P5 streams as
  `OUT_OF_SCOPE`/`RECORDER_NOT_ENABLED`, and retains raw evidence.
* Candidate activation requires a positive CDF collateral borrow and zero
  `OrderRejected` events with error `PRICE_UNAVAILABLE`.
* Global and per-venue conservation identity residuals use the fixed bound of
  1000 fixed-point report units. Late-path activity requires funding,
  expiry/settlement, and non-empty settlement checks.
* Seed-607 full/no-log/G8 parity is attested by exact checkpoint and sidecar
  comparison; full/G8 persisted evidence hashes must match; no-log venue
  JSONL and runtime evidence hash must be absent.
* The development scorer is write-once and reports a candidate qualification
  only. It cannot produce a market-realism claim or tune thresholds.

## Verification state

Shell syntax, the negative-path contract tests, and the mandatory Go test
suite pass with `GOMAXPROCS=4`. An independent Sol-xhigh review returned
`ACCEPT` at `06bd7cc`; it accessed no simulation, holdout, or private archive
evidence during review. No registered long-run cell has been consumed.
