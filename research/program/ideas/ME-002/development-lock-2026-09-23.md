# ME-002 — prospective development lock (2026-09-23)

This attests that the *content* of the [ME-002 protocol](protocol.md) at
reviewed candidate `e042e685dd82ee06d8f2d49c52e8c219a7f58091` is locked
for the finite development matrix. The protocol's earlier `DRAFT` status
records its state before review; this later operational lock changes no
scientific arm, endpoint, seed, horizon or threshold. Protocol blob
`eb5222327090bb23339d55467666653a2688acb0`, SHA-256
`4de32291b8f5fd02c55b96fc230a8103ef24a8eef809376ca17a8a8dcfdf9645`.
Candidate tree `6c2d5f6214393d65090d7bfcc4e8663bcf8b28a7` is preserved in a
clean detached source checkout; later status/report commits are not used to
construct worlds.

Independent read-only Sol-6 medium review accepted `7c7b7e6` for the
prospective ME-002 design/evidence scope after an earlier rejection at
`fcb7ee8`. Its technical controls exposed an analyzer-wire defect, so their
analysis was *not* promoted. A fresh bounded Sol-6 medium review then accepted
the exact `e042e68` correction for technical preflight without claiming
market realism. Clean full `make test`, `go vet ./...`, focused regression and
targeted race checks passed at `e042e68`.

Clean Go 1.27.0 binaries:

| Role | SHA-256 |
|---|---|
| simulator | `4cf2ace97d9ad0dc6346d0138695adb082be130cb42100a59f7d92015c5b786c` |
| analyzer | `b6f35a4fbfc08a8d64d904229af122f7fa13b0baf9bcc4d60ac68fd4778a003d` |

Two fresh-process F/F target-0.5 seed-12001 technical controls at
`GOMAXPROCS=1` and `7` exited zero and independently reconstructed
`FULLY_FILLED`. Both evidence files have SHA-256
`6493d699b97de2ef887bbce6372fcca2b570d933d8996b0a7e3b8a1a0be2c5e1`,
canonical execution hash `3361f1267a94aed9aaa166534395ec5319625aabf280d5197327ac88e4ddb40c`,
17,858 frames; both result files have SHA-256
`a27699b86e055ff55e8ba9fc84ac7cd2ca02bb5baa23f73d8044ebb5c16ba88b`.
Measured world wall: 0.40/0.34 s; peak RSS: 39,800/33,024 KiB;
retained run tree: 4.9 MB each. Resource floor for 26 executions is
conservatively 26×5 MB ≈130 MB evidence before extra analysis/metadata,
well below the registered 1 GiB evidence cap and 78 GiB available disk.
No capacity floor was lowered. One world at a time, at most seven Go workers,
30 s/world and 15 min batch ceiling remain binding.

The owner's bounded continuation permits only the already registered **24 ME-002 development cells** in this
order: target 0.5 then 5 ABC; within each target F/F, F/S, S/F, S/S; within
each arm seeds 12001, 12011, 12017 ascending. The two completed technical
controls are additional executions, not substitutes for an assigned cell.
No confirmation, historical holdout, geography, economics retuning, or
ME-002B/ME-003/ME-005 execution is authorized by this lock. The earlier
`7c7b7e6` raw controls and failed analyzer attempt remain untouched at their
original namespace. If any assigned cell is incomplete or fails strict
evidence, preserve it and apply the registered stop rule; do not silently
skip or replace it.
