# V2-8 Evidence-Durability Review

Date: 2026-08-27

## Scope

This review covers the instrumentation-only evidence-transport correction at
commits `6f988a3`, `eee70d4`, and `ba96597`. It does not alter simulator
economics, scheduler ordering, RNG use, actor state, matching, or lifecycle
semantics. The changes make raw logger, checkpoint, receipt, and frontier
transport failures fail closed before a run can publish a successful evidence
artifact.

The primary checks passed for the affected `feesim`, `derivsim`,
`multivenue`, and command packages, including targeted race tests and
`go vet`. A full `go test ./...` passed. A broad full-suite race invocation
was not promoted: it exceeded the ten-minute budget in the pre-existing
`TestV23P3PerpReplenishmentEvidenceIsFreshProcessDeterministicAndNeutral`
test without reporting a race; changed-boundary race tests passed.

## Independent review verdict

**ACCEPT WITH NARROWER CLAIM.** The available Sol-family review and its
adversarial pass found no successful-path trajectory, RNG, scheduler,
event-byte, or ordering regression. The review does not authorize a claim of
reusable-directory safety, transactional publication, or crash durability.

Confirmed by tests and source inspection:

- raw logger write, short-write, newline, flush, close, and multi-logger
  failures are latched and returned;
- checkpoint and trace write/close failures are detected and returned;
- receipt/frontier finalization failures remain observable on repeated
  finalization;
- a failed close does not emit the raw evidence artifact hash;
- the raw evidence digest remains the unordered multiset of persisted JSON
  records, while checkpoint hashing remains the distinct ordered execution
  stream.

## Required narrower contract

Campaign launchers must use a fresh output directory for every run and must
require a zero simulator exit plus freshly validated `greeks.json`,
`latency.json`, and runtime/offline evidence-digest equality. Existing
completion sidecars must never be treated as evidence for a failed or reused
run. The current code does not yet make reused directories impossible at the
generic `NewSim`/CLI boundary.

The evidence-artifact hash is currently written before `latency.json` is
attempted. Therefore it attests only the persisted JSONL multiset; it is not a
sealed statement that every compact sidecar has been durably published.
Hash/latency publication uses direct `os.WriteFile`, so atomic rename,
`fsync`, and crash-recovery durability are not claimed. A later close can also
lose a late publication error at the `Sim` level even though the underlying
logger/checkpoint errors are sticky.

These are correctness follow-ups for the eventual freeze candidate, not
performance optimizations. They do not invalidate completed scientific runs
that used fresh directories and passed the existing completion contract.

## Freeze implication

The evidence transport is materially stronger and safe for fresh-directory,
zero-exit, sidecar-validated runs. Before an immutable V2 freeze, either the
runner contract must be enforced mechanically (reject reused/non-empty output
directories and publish completion only after all required sidecars succeed),
or the freeze documentation must retain this narrower limitation and show
that every freeze run obeys the fresh-directory contract. No scientific
outcome may be scored from a directory that was reused after a failed run.

