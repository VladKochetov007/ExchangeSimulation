# V2 successor Go 1.27 launch-parser correction — 2026-09-12

This is append-only scientific process history. It records a promotion-gate
defect found before any capacity or scientific execution. It does not rewrite
the R2 predecessor, historical evidence, or earlier review verdicts.

## Exact candidate and independent finding

- Scientific branch: `autoresearch/ffa-ecology-gen0`.
- Reviewed candidate before correction: `023dfc3313b581f4fa0764f850fd142eafb01984`.
- Git tree: `92d488645b93342a94f38b3181aa87601c4ae244`.
- The fresh exact-tree Sol-xhigh review accepted the scientific candidate,
  but its separate signed-bundle production/revalidation was rejected by
  reviewer `Zeno`, agent
  `01a097fc-42fc-7590-87cd-ee3542207418`.
- Performance feed was fetched through reviewed `b1847ac`; no newer commit or
  performance implementation was imported.

The reviewer reproduced a reachable operational defect in all three SV1D
launch/audit adapters. `go version -m` under Go 1.27 begins with a line such as:

    example-binary: go1.27.0

The capacity and activation runners, and the adjacent SV1D audit adapter,
looked for a nonexistent `go` metadata row using `awk '$1 == "go" {print $2; exit}'`.
The result was empty, so every otherwise valid Go 1.27 pinned binary failed
the strict version predicate. The signed review bundle was invalidated and
deleted; no review ACCEPT artifact or private signing key was retained.

## Intended invariant and classification

Every strict SV1D binary check must extract the toolchain version emitted by the
registered Go tool itself, require a nonempty `go1.27*` value, and bind that
identity into the launch metadata. A valid pinned binary must not be rejected
because the parser assumes an obsolete `go version -m` layout.

Classification: **BUG REACHABLE BUT NO HISTORICAL ACTIVATION**. The defect
could block the registered binary capacity/activation path, but no capacity arm,
seed-659 activation probe, development cell, freeze, or holdout ran under this
successor contract. Historical R2 JSON/binary results and all previous verdicts
are unaffected.

## Minimal correction

Commit `527d55a` (`fix: parse Go 1.27 binary metadata`) changes the three
adapters to parse the first `go version -m` line with:

    sed -n '1s/.*: //p'

The R2 contract test now checks the Go 1.27 first-line form and rejects the
obsolete row parser in all three paths. No economic, calendar, roster,
evidence, scoring, configuration, or historical artifact changed.

## Verification after correction

At clean committed `527d55a`, all of the following passed under
`GOMAXPROCS=2 GOMEMLIMIT=4GiB` where applicable:

- the focused R2 contract and shell syntax/diff checks;
- clean full `make test`, including package, integrated-long-run, R2,
  archive, and parity contracts;
- `go vet ./...`;
- targeted race tests for analysis, SV1D tools, analyzers, prunegate, and
  tests;
- fresh-process event-stream determinism;
- fresh-process binary-evidence and perp-exposure evidence neutrality.

The seven pre-correction pinned tools were valid Go 1.27 builds, but their
embedded source revision is `023dfc3`; they are not launch artifacts for the
corrected `527d55a` tree. They must be rebuilt cleanly after the next accepted
exact-tree review.

## Remaining gate

The promotion gate remains closed pending one fresh exact-tree independent
Sol-xhigh review of `527d55a`, followed by clean pinned rebuilds, a newly
authenticated review bundle bound to that corrected tree and tool set, and the
outcome-ineligible binary capacity preflight. No activation, development cell,
freeze, or holdout `619/631/641` is authorized by this correction.

## Append-only exact-tree review rejection — 2026-09-13

Fresh reviewer `Gauss` (`01a09810-a506-7bf3-bed8-7d8ca07ef6e4`) independently
reviewed exact HEAD `054c60a` and returned **REJECT** for the narrow next gate.
The reviewer found that this note’s illustrative metadata line used the
system-temporary binary path, while
`tests/repository_paths_test.go:TestTrackedFilesAvoidSystemTempPaths` rejects
system-temporary paths in tracked files. This was a documentation-contract
defect, not an economic, simulator, evidence, or historical-result defect.

The illustrative path is corrected above to `example-binary`, with no change to
the Go 1.27 finding or its classification. The exact `054c60a` tree must still
demonstrate an uncached full `make test` and receive a fresh exact-tree review.
No capacity, activation, development, freeze, or holdout action was authorized.

## Append-only correction verification — 2026-09-13

At exact HEAD `5ce2c7b`, after the placeholder correction, a fresh uncached
`make test` was run after clearing the Go build and test caches. It passed the
complete package suite, the fresh-process multivenue evidence matrix, the
integrated-long-run contracts, the R2 contracts, and both archive contracts.
The expected incomplete-resource diagnostic was confined to the negative
contract case; the run exited zero. Memory and disk remained within the
registered envelope and the cgroup OOM counters remained zero.

The exact tree is now ready for one new independent Sol-xhigh review. No
capacity, activation, development, freeze, or holdout action has occurred.

## Append-only capacity-runner executable-bit finding and correction — 2026-09-13

The first authorized capacity invocation against `5bcc897` reached the
runner’s lock handoff but exited with status 126 before creating any output.
The tracked capacity runner had mode `0644`, while its trusted lock re-entry
uses `env ... "$0"` and therefore requires the runner itself to be executable.
This was a reachable launch-contract bug, not a simulator or capacity result;
no arm, evidence stream, attestation, or scientific outcome was produced.

Commit `7d35958` changes only the runner mode to `0755` and adds a focused R2
contract assertion that the capacity runner is executable. The focused R2
contract passed. A full suite attempted before committing the mode fix stopped
only at the intentional clean-worktree parity prerequisite; after the commit,
the clean full `make test` passed package, evidence, integrated-long-run, R2,
and archive/parity contracts. No OOM event or retained-evidence deletion
occurred.

The prior signed bundle is invalidated by the source-tree change. A fresh
exact-tree review, pinned rebuild, independently authenticated bundle, and
capacity preflight are required again. No capacity arm completed, and no
activation, development, freeze, or holdout action was authorized.
