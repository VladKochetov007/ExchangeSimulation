# SV1 treatment-607 extraction resource incident — 2026-09-02

This is an append-only operational incident record. It is not a scientific
result and does not alter the R2 predecessor or the SV1 economic contract.

## Identity and retained inputs

- Scientific code revision used for the completed trajectory: `3f73f3043c2cb04f01c9acfa86eed132cfadff11`.
- Binary evidence format: `evstream_v3`.
- Cell: `treatment-607` under `/home/vlad/v2-r2-sv1-24h-development-20260901-v1`.
- Raw trajectory completion was verified with exit status zero.
- Current external attestation was rebuilt from the retained raw cell after the
  stale predecessor attestation was preserved as
  `historical-treatment-607-74be64bf510a1b278603bb71e173e26515a9068b.json`.
  Its SHA-256 is `fd23d17feef9066628a2987344fb7d98254e702199193d6a6a392f897011434f`.
- Current treatment attestation SHA-256 is
  `bec7bb2ad9d9685150c9b3ad653fd74fb06f3aa00a0b8e80f1c2a0e3e826fa1e`.
- No derived economic result has been accepted. The raw cell remains retained.

## Failed extraction attempts

The first bounded disk-backed extraction verified the raw contract and then
failed while `evsrender` was reconstructing the route
`central/spot/CDF-USD.jsonl`. Its temporary staging reached approximately
4.19 GB while the filesystem had approximately 356 MB remaining, and the
renderer reported `no space left on device`.

After exact-name quarantine and removal of 67 stale, regenerable test/render/
build directories, 2,723,549,184 bytes were recovered. The preserved
`v2-r2-dev607-pre-reextract-derived.tar.gz`, private objective snapshots,
capacity evidence, treatment raw cell, and incident logs were not removed.

A second disk-backed extraction was stopped before filesystem exhaustion when
the renderer staging reached 6.96 GB and only 319 MB remained. The renderer
RSS was approximately 12 MB. Its cleanup trap removed the transient staging
tree; the raw cell was unchanged.

A third attempt used a RAM-backed temporary filesystem as staging. It demonstrated that the
complete rendered JSONL tree is approximately 16.81 GB. The attempt was
stopped with 16 MB of tmpfs remaining and approximately 11 GiB of available
RAM. No OOM occurred and the cleanup trap removed the tmpfs staging tree.

All three failures are resource-boundary failures, not simulator outcomes.
The extractor exit marker is non-zero and no derived files were produced in
the raw cell.

## Disposition and next safe action

The full JSONL reconstruction is too large for the current retained-evidence
layout to coexist with the active capacity probe on this host. The next
operation is to compact the already-passed current capacity probe through the
existing fail-closed retention protocol, preserving its archive, member list,
checksum, comparison log, and retention receipt before source cleanup. The
capacity source is bound to `3f73f30`; a documentation successor commit will
make that probe explicitly superseded without changing its measurement.

Treatment extraction will then be retried from an exact `3f73f30` clean
worktree with the full renderer staging available on disk. This preserves the
cell's source-revision identity while allowing the append-only incident record
to advance the active branch. No additional development cell, parity control,
freeze action, or holdout access is authorized by this incident.
