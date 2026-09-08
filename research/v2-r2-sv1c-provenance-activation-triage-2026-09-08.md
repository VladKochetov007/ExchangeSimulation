# SV1C provenance and activation triage

Date: 2026-09-08  
Candidate: `V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK`  
Scientific source checkpoint: `7d91d141ad5a9f33155afd7a0508286487efdc07`  
Branch: `feature/r2-cdf-survival-successor`  
Stage: development-only; no activation, capacity, development, freeze, or holdout world has run

## Purpose and boundary

This note records the provenance/activation triage performed after two
independent read-only Sol-xhigh-style critics rejected the earlier candidate
`8f03546`. It is a successor-gate record, not a rewrite of the earlier review
or of any historical experiment. The critics were asked to inspect the exact
source and contract boundary, not to approve the expected result. Their
rejections are retained as useful negative evidence; they are not acceptance
of the current tree.

The current candidate preserves the accepted R2 calendar/lifecycle semantics,
the eight historical ABC/USD suppliers, the finite delayed-local CDF supplier
roster, the strict-risk amendment, and the existing evidence representation.
The R2 predecessor remains closed as `NON-VIABLE AT THE 24H MARKET-SURVIVAL
GATE`. No historical trajectory has been repaired, rescored, or overwritten.

## Independent review findings against `8f03546`

The first critic identified:

* the normalizer source closure bound `*.go`, `go.mod`, and `go.sum` but did not
  conservatively cover workspaces, assembly, native sources/headers, vendored
  inputs, or other relevant build inputs;
* the renderer could enroll the supplied normalizer's own digest and revision,
  rather than comparing against an independently committed expected value;
* normalizer pathname validation allowed a check/use replacement window; and
* manifest normalizer `.go_version` and `.package` fields were not validated
  against an independent registration.

The second critic independently identified:

* the inherited `scripts/v2-r2-sv1-terminal-outcome.jq` dependency was used by
  the integrated contract without being declared in the SV1C dependency set;
* the normalizer path/flags and replacement behavior were not sufficiently
  constrained to prevent self-blessing or alternate invocation semantics;
* manifest normalizer metadata was not fully checked;
* activation-status hash command substitutions could mask a failed
  `sha256sum`; and
* the repository-wide source check was not conservative enough about current
  build inputs (a conservative rejection policy is preferable to silently
  accepting unbound assets).

These findings were classified as **provenance-contract defects**. They did
not activate any simulator semantic path because the candidate had produced no
world, and they did not alter retained historical evidence.

## Invariants and repairs

The intended invariant is:

> A registered effective configuration can be regenerated only by the exact
> committed generator/contract dependency set, using the exact independently
> registered normalizer binary and its unchanged source/build-input closure;
> every generated artifact and activation status must be hashed by a checked
> operation before it is published.

The current implementation enforces this through the following checkpoints:

| checkpoint | repair |
| --- | --- |
| `f239b8e` | precommitted a clean Go 1.27 normalizer registration, including binary digest, source revision, package, platform, build flags, and reproducibility statement |
| `3b2f4e0` | bound normalizer registration and terminal-outcome dependency into the SV1C manifest; closed the build-input/source-revision checks; validated normalizer metadata; checked activation hashes and publication failures |
| `7d91d14` | removed a repository-path-sensitive `mktemp` invocation from the renderer so the repository safety contract passes while retaining snapshot-and-rehash protection |

The registration currently names `bin/multivenue`, source revision
`c2a752f3bb5d6b38c8ad8ee066ad91bbf355aae0`, Go `go1.27.0`, and package
`exchange_sim/cmd/multivenue`. Two clean builds from that revision reproduced
SHA-256
`dde13e4eda920b874eadba81a360ece3ff0951d2ef907acd1ddff24b800f5c87`.
The active renderer snapshots the registered binary to an anonymous temporary
file, preserves executable mode, rehashes it, revalidates the pinned build,
and uses only that snapshot for normalization. The manifest binds the
registration file and the three declared contract dependencies, including the
terminal-outcome JQ program.

The source-revision check is intentionally fail-closed for unrecognized
`go:embed` inputs and conservatively checks Go, module/workspace, native,
assembly, and vendor paths plus working/staged/untracked/ignored changes. This
may reject a future source tree that needs an explicit registration update;
that is a planned audit boundary, not permission to bypass the check.

## Evidence and activation assessment

Evidence searched:

* the current worktree status, exact commit history, and `git diff --check`;
* current SV1C config/provenance and normalizer-registration JSON;
* the full `make test` log at `7d91d14`;
* the `go vet ./...` and targeted race logs;
* the exact focused package log at
  `/tmp/sv1c-focused-7d91d14.log`; and
* retained external-scratch artifacts, which contain predecessor SV1B
  activation/review material but no accepted SV1C exact-tree attestation.

Observed status:

* clean `make test`, `go vet ./...`, targeted race, focused packages, binary
  evidence, renderer, calendar, and fresh-process determinism checks pass;
* the focused packages `evstream`, `types`, `exchange`, and
  `simulations/multivenue` pass at the exact current source;
* no SV1C normalizer-generated world exists;
* no activation, capacity, development, freeze, or holdout evidence exists;
* no evidence deletion or historical artifact rewrite occurred; and
* holdouts `619`, `631`, and `641` were not read or run.

Therefore the activation condition is **not yet tested**, not “passed.” The
mechanical candidate is ready for a fresh exact-tree independent review, but
the review is a required promotion gate. Only after an accepted attestation
may the clean pinned binaries be built for the registered seed-643 activation
probe. The probe itself must then be independently extracted and reviewed
before the 24-hour development cells are considered.

## Remote-feed state

The asynchronous refs were fetched read-only without switching the scientific
worktree. No commit was newer than:

* `origin/perf/r2-cdf-survival-port` `39768df`;
* `origin/autoresearch/v2-performance-research` `b1847ac`; or
* `origin/redteam/economic-audit` `e85e16c`.

No performance branch implementation or red-team implementation was merged.
The binary-evidence prototype remains a separate future promotion candidate;
SV1C's current scientific evidence contract is unchanged.

## Decision

Classification: **MECHANICAL SUCCESSOR READY FOR FRESH REVIEW; NOT PROMOTED**.

No historical rerun or rescore is justified by these provenance findings: the
defects were detected before a successor trajectory existed, and retained
historical JSON evidence has a separate immutable identity. The next action is
one fresh independent Sol-xhigh review of the exact complete tree. A reviewer
rejection requires a focused repair and a new mechanical gate; a reviewer
acceptance is the only authorization to proceed to pinned binaries and the
development-only activation probe. Holdouts remain forbidden before explicit
freeze authorization.
