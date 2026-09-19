# SV1D C2 producer/verifier schema closure

Candidate `94e63d6abaaac5d790a070f303a08da4445b28af` received a substantive
independent **REJECT** with execution **COMPLETED** from the fresh native
reviewer context recorded in
`/home/vlad/external-scratch/sv1d-capacity-c2-20260913/source-review.md`.
The report SHA-256 is
`c8c081e16ad648ece1b52fbef89f77faba07acb6caf90986dca5e087e82045ed`.
The C2 checkout and report are immutable historical review evidence; no old
bundle or verdict is reused. No simulator, capacity, activation, development,
freeze, or holdout world was run.

The reviewer confirmed the two earlier C67 findings were closed, then found
`SV1D-C2-001`: the normal producer's artifacts could not pass the strict typed
verifier. Specifically:

- `run-metadata.json` emitted `command` and `raw_log_policy`, but the verifier
  did not decode or require them;
- `run-status.json` emitted four market-data SHA-256 fields, but the verifier
  rejected them as unknown and did not bind them to retained bytes;
- `evsrender` emits `multivenue.BinaryRenderReport`, which has event frames,
  dictionary frames and execution hash but no `stream_frames`; the verifier
  incorrectly required that invented field.

The C3 successor patch addresses these findings without changing market
economics, registered configs, resource formulas, evidence format, or scientific
predicates. The strict metadata type now requires the exact registered logical
command and raw-log retention policy. The status type requires all four fields
and verifies each digest against the retained artifact. The renderer verifier
uses the real exported report shape and derives `event_frames + dictionary_frames`
with checked overflow before comparing the registered stream-frame count.

Permanent tests now execute the actual shell `jq` producers for all three arms,
serialize the real `multivenue.BinaryRenderReport`, run the production output
through the strict validators, and reject missing/null/unknown fields, every
retained market-data mutation, metadata mutation, renderer mutation, and frame
overflow. `GOMAXPROCS=2 GOMEMLIMIT=4GiB go test ./analysis -run SV1D -count=1`
passes. A clean detached C3 snapshot also passed `make test`, `go vet ./...`,
the targeted race suites, finite-cgroup validation, and `git diff --check`.
Fresh independent review remains outstanding. Future review must use an
explicitly selected Luna xhigh-or-lower context; Astra is prohibited by owner
amendment.

This is a successor source correction, not a rewrite of C2's rejection and not
an execution authorization. After clean gates and review: build pinned Go 1.27
binaries, create a fresh verified signed bundle, run only the registered seed977
finite-cgroup capacity preflight, and independently verify it before seed659
activation. Holdouts remain forbidden before separate freeze authorization.

## C3 independent review closure — 2026-09-19

The exact C3 candidate `447a492718e78a7cc974c2363deee5ae13c2ae6a`, tree
`35e6060a3488e215143861ac90559abee6305138`, received a completed substantive
REJECT from a fresh Luna xhigh reviewer. The immutable report and separate
execution/verdict record are preserved at
`/home/vlad/external-scratch/sv1d-capacity-review-447a492/`. The finding is
`SV1D-C3-001`: the Go renderer verifier mirrored the producer's `int` route
field but rejected only zero, so a negative retained `routes` value could pass
the final typed verification boundary. The shell producer's positive check did
not close that Go-side fail-open path.

The successor patch changes the predicate to `Routes <= 0` and adds a negative
route-count mutation regression. This is a minimal correctness correction; it
does not change market economics, registered configurations, resource formulas,
evidence format, or scientific predicates. Clean gates and re-review by the
same reviewer are required before any build, capacity, or scientific run.

## C4 accepted review and failed capacity attempt — 2026-09-19

The exact C4 candidate `6cab9343c80045a7c13cc6cfa11fbc029536809a`, tree
`68a2f6ef1df7deebe29d5028f46686528ab2dbd6`, received `COMPLETED / ACCEPT` from
the same fresh Luna xhigh reviewer. The report and separate status record are
preserved at `/home/vlad/external-scratch/sv1d-capacity-review-6cab934/`.
The exact-tree validation snapshot passed `make test`, `go vet ./...`, targeted
race suites, finite-cgroup validation, and `git diff --check`.

A fresh Go 1.27.0 build from two clean C4 checkouts produced seven
byte-identical binaries. The new bundle at
`/home/vlad/external-scratch/sv1d-capacity-pinned-6cab934-20260919/bundle-retry-signature-v1/`
passed canonical-plan replay, source/tree/config/binary identity checks, signed
review verification, all registered tamper rejections, and its final manifest
hash check. The first packaging attempt is retained separately because its
helper used an unregistered signing domain; it was never used for execution.

The authorized outcome-ineligible seed-977, five-minute, three-arm capacity
preflight then ran under `MemoryMax=8G`, `MemorySwapMax=0`, `GOMAXPROCS=2`, and
`GOMEMLIMIT=4GiB` from a clean real Git clone. Treatment, mode-off, and
no-roster simulator children all completed their 5-minute worlds and produced
manifests, greeks, latency, binary evidence, checkpoints, and market-data
sidecars. The resource traces observed zero swap and zero OOM events with
approximately 8 GiB cgroup limits and safe host free-space/RAM margins.

The runner nevertheless returned exit 99 for every arm and correctly issued no
capacity attestation. At the terminal simulated timestamp, the first event of
that timestamp caused the scheduled checkpoint to be written; later events at
the same timestamp extended the binary stream, but `lastCheckpointAt` prevented
the close-time checkpoint from being refreshed. For example, the retained
treatment arm had terminal checkpoint event count `161987` while its binary
attestation had `162663` event frames, with different hashes. This is an
evidence/runner contract defect, not a capacity pass, market result, or
historical scientific result. The complete partial attempt remains immutable at
`/home/vlad/external-scratch/sv1d-capacity-run-6cab934-20260919/`.

No activation, development, freeze, or holdout run used this failed attempt, so
there is no historical outcome to invalidate. The successor correction defers
scheduled terminal-boundary checkpoints until `Close` and adds a regression with
multiple events at the same terminal timestamp. Because simulator evidence
semantics changed, the successor requires full clean gates, a fresh pinned
build/bundle, and fresh independent review before a new capacity namespace.

## C5 accepted review, capacity verification, and seed-659 activation diagnosis — 2026-09-19

The exact C5 candidate is `5015fd00a3720c3bd9312d93ae5635fdbf8647a4`, tree
`9409efe743d02b0dae2d4aa7f65de61c1ad1ca26`. It contains only the terminal
binary-checkpoint correction and its regression on top of the accepted C4
source. The fresh primary Luna-xhigh launch did not return a substantive report
within its bounded observation window; the separate independent Luna review
execution recorded at `/home/vlad/external-scratch/sv1d-capacity-review-5015fd0/`
completed on the exact tree with verdict **ACCEPT**. The report and execution
status are retained outside the candidate tree. No reviewer edited the source
or ran a scientific world.

The C5 validation snapshot passed `make test`, `go vet ./...`, the targeted
race suites, finite-cgroup validation, `git diff --check`, and the binary
evidence contract tests. Clean Go 1.27.0 rebuilds from independent clean
checkouts produced byte-identical registered binaries. The fresh signed bundle
passed canonical-plan replay, exact-tree/review/config/binary identity checks,
registered tamper rejections, and final manifest verification. The bundle is
retained under `/home/vlad/external-scratch/sv1d-capacity-pinned-5015fd0-20260919/`.

The authorized seed-977 five-minute tri-arm capacity preflight then completed
under `MemoryMax=8G`, no swap, `GOMAXPROCS=2`, and `GOMEMLIMIT=4GiB`. It
reported zero cgroup OOM and swap deltas, a peak tree RSS of 212,246,528 bytes,
and a minimum free disk observation of 43,852,029,952 bytes. Its attestation
was outcome-ineligible by contract. The independent verifier passed using the
corrected verification wrapper. Capacity artifacts are retained under
`/home/vlad/external-scratch/sv1d-capacity-run-5015fd0-20260919/`.

The separately authorized seed-659 activation probe was launched only after
those gates. All three arms retained binary evidence but exited with status 1
and were classified `INCOMPLETE_ARM`; the scorer issued no directional result.
The common simulator error was:

    marked position ABC-OPT-U4142432f555344-1735696800-K4900000000-C:
    option risk mark: no usable price

The failure occurred at the same simulated boundary in treatment, mode-off,
and no-roster. The retained binary stream was rendered once for diagnosis; the
derived report shows that the option was listed at
`1735689603000000000`, while at `1735689777000000000` the deterministic
post-derivative-mark phase observed an empty ABC/USD book for the relevant
venue. `derivativeUnderlyingPrice` therefore had no declared usable reference;
`UpdateDerivativeMarks` cleared the option's paired mark and removed its risk
epoch. This behavior is required by the existing stale-option regression and is
not being weakened. The same timestamp later contains actor order work and
public snapshots that restore ABC/USD liquidity, but that later state cannot
retroactively make the earlier risk boundary priceable.

The activation is therefore a non-advancing producer/risk-telemetry gate, not
evidence for or against the CDF supplier hypothesis. It is not a historical
activation: no development cell, freeze, holdout, or prior retained result used
this C5 seed-659 execution. The next decision is an independent review of the
minimal admissible correction. A candidate may defer a transient *scheduled
telemetry* capture only if the strict exchange mark-clearing behavior,
cross-margin fail-closed behavior, pre-expiry requirements, and strict terminal
valuation remain intact and the gap remains auditable. Reusing stale option
marks, changing deterministic phase ordering, altering the registered
configuration/roster/warm-up, or treating an incomplete arm as an economic
result is explicitly rejected pending a new scientific amendment.

## Successor correction — 2026-09-19

The fresh Luna independent adjudication accepted the narrow producer correction
described above. Source commit `88ea00d` now defers only scheduled `ErrNoPrice`
captures, retries at the next normal boundary, fails immediately on other
errors, and records `risk_capture_diagnostic` evidence. Strict option mark
clearing, phase order, pre-expiry capture, and terminal valuation are
unchanged. Focused recovery/renderer regressions and the full
`go test ./simulations/multivenue ./analysis -count=1` gate pass. This is a
successor semantic amendment, not an offline repair of C5; a fresh exact-tree
review, clean pinned build, and new seed-659 activation are still required.
