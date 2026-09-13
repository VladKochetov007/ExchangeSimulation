# V2 successor binary-evidence review closure — 2026-09-12

This note is append-only scientific process history. It does not rewrite the
predecessor R2 result or promote the SV1D successor.

## Reviewed candidate

- Scientific branch: `autoresearch/ffa-ecology-gen0`
- Exact reviewed tree: `81a66653108afc5458a9a0a3a8b61bce807dc898`
- Independent reviewer: Sol-xhigh, `gpt-5.6-sol`, review agent
  `01a095e9-46e1-7c42-b364-a46f0d7908ab`
- Verdict: `REJECT`
- Performance feed: `origin/autoresearch/v2-performance-research`, reviewed
  through `b1847ac`; a subsequent fetch found no newer commit.

The review found two promotion blockers:

1. The private capacity `--internal-arm` accepted a forged environment marker
   plus a caller-opened descriptor for the expected lock path. The direct
   reproduction reached `run_capacity_arm` and created its arm metadata before
   failing at a later sentinel (`exit 95`). The existing descriptor and flock
   checks did not authenticate the handoff from the resource wrapper.
2. CDF decision schema v4 accepted contradictory numeric presence: an absent
   optional value could carry nonzero wire bytes, and a present optional value
   could carry zero. Both forms were silently normalized by decoding.

## Closure

- `0d9c5e1 fix: reject noncanonical CDF numeric presence` validates every v4
  optional numeric field before normalization and adds both contradiction
  regressions.
- `11da855 fix: require trusted SV1D arm handoff` adds an opt-in
  resource-wrapper child handoff on a pipe-backed FD3, binds the token to the
  actual `sv1dresource` parent PID, rejects direct forged lock-marker entry,
  and updates the capacity runner to request the handoff. The outer namespace
  flock remains held by the waiting capacity runner.

The end-to-end adapter probe passed through the handoff and reached only the
controlled missing-sentinel failure with harmless fixture binaries. The
control-plane regression rejects direct forged entry before creating the arm
directory.

## Exact-tree verification after closure

At `11da855`, all of the following passed with `GOMAXPROCS=2
GOMEMLIMIT=4GiB`: focused `evstream`, `types`, `exchange`, `analysis`,
`cmd/sv1dresource`, and `simulations/multivenue` suites; clean `make test`
(including integrated-long-run, R2, archive, and parity contracts);
`go vet ./...`; the prescribed targeted race matrix; changed evidence-package
race coverage; fresh-process baseline/binary/log-mode/perp-exposure
determinism checks; shell syntax; the R2 control contract; and `git diff
--check`.

No economic code, R2 calendar, SV1D roster/configuration, historical artifact,
capacity attestation, campaign binary, development cell, freeze, or holdout
changed. Holdouts `619/631/641` remain untouched and unauthorized.

## Next boundary

The next action is one fresh independent Sol-xhigh review of exact tree
`11da855`, covering R2 calendar/lifecycle semantics, current correctness
hardening, binary evidence and rendering, strict scoring/provenance, resource
manifests, and both lock entrypoints. Until that review accepts, do not run
binary capacity, build campaign binaries, run the seed-659 activation probe,
or launch `dev-607`.

## Append-only follow-up review rejection and remediation — 2026-09-12

The fresh Sol-xhigh review of exact tree
`0c19cdf1882ee6558283bb91883cb82645d8776a` was independently rejected.
Reviewer agent `01a09611-4490-7c00-85a2-8422040b19e5` confirmed that the
previous pipe-token change still allowed the public `sv1dresource` adapter to
invoke the private arm: a caller could supply the environment marker and a
caller-opened lock descriptor, while the internal entrypoint trusted only the
marker, descriptor path, and wrapper executable name. The reviewer reproduced
entry into arm setup with harmless binaries.

This is a reachable authorization/instrumentation defect in the capacity
preflight, but it has no historical scientific impact: no capacity arm,
activation probe, development cell, freeze, or holdout has run under the
binary successor contract. The reviewer found the CDF v4 optional numeric
fields semantically correct at this tree; the earlier focused regression was
strengthened to cover all eleven fields in both contradiction directions. The
review also identified a latent sequence-continuity issue in the unused
indexed selective reader; it is deferred and blocks promotion of that reader,
not the current scientific evidence path.

The correction is committed and pushed as `49dbcd4`, with the public-wrapper
attack regression completed in `b33f089` and `86854e3`. The private arm now:

- inherits the already-held namespace lock as FD3 through the resource adapter
  and rechecks its exact path and nonblocking flock;
- requires the current clean source/tree identities and exact arm paths;
- rechecks the signed ACCEPT review with the pinned `sv1dprobe` binary and
  hashes the review inputs, target config, simulator, analyzer, renderer,
  runner, and actual parent resource binary;
- compares the arm config against the corresponding immutable target config
  before creating the arm directory.

Verification after remediation: the bounded focused suites for `evstream`,
`types`, `exchange`, `analysis`, `cmd/sv1dresource`, and
`simulations/multivenue` passed; shell syntax and `git diff --check` passed;
the clean integrated-long-run contract passed; and the actual public-wrapper
regression reached the missing signed-review identity check (status 9) without
creating an arm directory. The latest performance-feed fetch from
`origin/autoresearch/v2-performance-research` found no commit after reviewed
`b1847ac`; no performance implementation was imported.

The promotion gate remains closed. One fresh exact-tree Sol-xhigh review of
the current clean tree (including code checkpoint `86854e3`) is required before binary capacity, pinned Go 1.27 builds, the
seed-659 activation probe, `dev-607`, freeze, or holdout `619/631/641`.

## Append-only follow-up review rejection: sampler cadence — 2026-09-12

The next fresh independent Sol-xhigh review was run against the exact clean
tree `f258af91338b0df41ce7017ab5ba1a68ae7ba045` (tree
`14670c42a06ff4fc88d62d11b85979bb2fdc59ef`) and returned `REJECT`. Reviewer
`McClintock` (`01a0964e-ef84-7821-b711-7a4f3c7feb89`, `gpt-5.6-sol`) accepted
the R2 calendar, correctness hardening, SV1D economics, binary evidence,
scoring, provenance, and authorization design, but found reachable blocker
`SV1D-RSRC-001`: post-collection wall timestamps could exceed the strict
250-ms sample-gap contract. The independent reproduction observed
`maximum_sample_gap_nano=250894382` with 15 samples during a three-second
`/bin/sleep` only; no simulator or scientific cell was run.

This is an operational measurement defect classified as reachable but not
historically activated. The minimal correction is `5cd7b4d`: actual timestamps
remain post-collection, while fixed half-interval deadlines leave collection
headroom and rebase after an overrun. The registered maximum and fail-closed
validator are unchanged. A real multi-tick strict-validation regression passed
five repetitions; the repaired CLI reproduction reported 26 samples and a
`125773682` ns maximum gap. Clean `make test`, vet, targeted race, fresh-process
determinism/evidence-neutrality, focused binary-evidence tests, and diff checks
all pass at the corrected tree.

The performance feed was refetched through reviewed `b1847ac` with no newer
commit and no imported performance code. No capacity, activation, development
cell, freeze, or holdout `619/631/641` was consumed. The promotion gate remains
closed pending one fresh exact-tree Sol-xhigh review of the complete corrected
candidate. Full remediation details are recorded in
`research/v2-r5-sv1d-sampler-cadence-fix-2026-09-12.md`.

## Append-only review-availability stop — 2026-09-12

Two fresh Sol-xhigh launch attempts for exact HEAD
`82f452028740d535253ce2d822be750250466ff2` failed before inference with
backend HTTP 403. The independent reviewer returned no scientific verdict,
correctly declining to fabricate ACCEPT or REJECT. The candidate remains at
the independent-review boundary; no capacity, activation, development cell,
freeze, or holdout was run. Retry the complete exact-tree review when the
Sol-xhigh service/quota is available.

## Append-only repeated review-availability stop — 2026-09-12

Two further fresh Sol-xhigh launch attempts against exact HEAD
`b1d66fec96281e82bafcc3c7b052cd9c68c3281a` failed before inference with
backend HTTP 403 over WebSocket and HTTPS fallback. No scientific verdict was
produced. The promotion gate remains closed and no capacity, activation,
development cell, freeze, or holdout was run.

## Append-only blocked-gate record — 2026-09-12

The same backend failure recurred on the next continuation turn: two fresh
Sol-xhigh launches targeting exact HEAD
`1db902a3c2327da204f2e1b80ee271700ceffdf3` failed before inference with HTTP
403 over WebSocket and HTTPS fallback. No ACCEPT or REJECT verdict was
produced. The successor remains unpromoted and no scientific execution was
authorized.

## Append-only launch-bundle rejection and correction — 2026-09-12

The fresh review of exact `023dfc3` returned `ACCEPT` for the narrow next
promotion step. During independent production of the required signed bundle,
reviewer `Zeno` (`01a097fc-42fc-7590-87cd-ee3542207418`) found that the SV1D
capacity and activation runners, plus the adjacent audit adapter, parsed Go
binary metadata using an obsolete `awk '$1 == "go"'` row. Go 1.27 emits
`binary: go1.27.0` on the first line, so valid pinned binaries failed the
strict toolchain check. The provisional bundle was deleted and no signed
ACCEPT artifact was retained.

This is a reachable launch/provenance defect with no historical activation;
no capacity, activation, development, freeze, or holdout ran. Commit
`527d55a` changes all three paths to parse the first metadata line and adds a
contract regression. Clean `make test`, vet, targeted race, fresh-process
determinism/evidence-neutrality, shell syntax, and diff checks pass. The
corrected candidate now requires one fresh exact-tree Sol-xhigh review before
rebuilding tools and running capacity. Full details are in
`research/v2-r5-go-1.27-launch-parser-fix-2026-09-12.md`.

## Append-only exact-tree review rejection — 2026-09-13

Fresh reviewer `Gauss` (`01a09810-a506-7bf3-bed8-7d8ca07ef6e4`) reviewed exact
HEAD `054c60a` and returned **REJECT** for the narrow next promotion step. The
new launch-parser note used a system-temporary binary path as an illustrative tracked path, and
the repository path-contract test rejects system-temporary paths. The defect
was documentation-only and had no economic, simulator, evidence, or
historical activation impact. No capacity, activation, development, freeze,
or holdout ran.

The placeholder is corrected to `example-binary`; an uncached full `make test`
and a fresh exact-tree review are required before rebuilding or capacity.

## Append-only uncached verification — 2026-09-13

The placeholder correction was pushed as `5ce2c7b`. A fresh uncached full
`make test` passed all package, fresh-process multivenue, integrated-long-run,
R2, and archive contracts. The expected negative resource diagnostic was
observed without changing the zero exit status, and no OOM event occurred.
The next required step is a fresh exact-tree Sol-xhigh review of `5ce2c7b`.
