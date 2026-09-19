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
