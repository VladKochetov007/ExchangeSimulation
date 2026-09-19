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
