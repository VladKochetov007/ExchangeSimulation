# SV1 binary renderer resource incident and retained compaction

Date: 2026-09-01  
Candidate: `feature/r2-cdf-survival-successor`  
Scientific status: operational evidence incident; no new economic result

## Scope

The completed 24-hour SV1 treatment-607 run produced a complete raw
`evstream_v3` evidence tree at source revision
`74be64bf510a1b278603bb71e173e26515a9068b`. The first offline extraction was
stopped because the pre-c80 renderer retained reconstructed event bytes for
the whole run. This was a renderer resource failure, not a simulator failure.
No activation, integrity, lifecycle, or economic conclusion was accepted from
that extraction attempt.

The raw treatment tree was retained before cleanup as:

    /home/vlad/v2-r2-sv1-treatment-607-raw-74be64b-render-fix-retained.tar.zst

Its source payload was 32,929,471,192 bytes. The archive is 4,020,717,828
bytes and has SHA-256
`3142ccbe71aed0be0a79f9cb2fa7117b354a17bc4ec9b1dd596deadf9c1ff81f`.
The archive member list, sidecar hash, retention receipt, and empty
`tar --compare` log are retained beside it. The original source was removed
only after those checks passed.

## Correction

`c80a757f99f7d7c29821dc278d326af3f4669aa2` changes only binary evidence
reconstruction and its regression coverage. `RenderBinaryEvidence` now reads
bounded per-route sidecar cursors, merges them by venue and exact
`event_seq`, writes staged route output, validates the attestation, and
atomically installs the rendered result. It rejects duplicate, missing,
out-of-order, and inconsistent route sequences. The focused multivenue suite
and a clean `GOMAXPROCS=4 make test` passed at c80. The post-c80 targeted race
gate and fresh-process promotion checks remain pending.

The old capacity probe for the pre-c80 binary was also compacted after the
treatment source had been removed. It is retained as:

    /home/vlad/v2-r2-sv1-capacity-archive-74be64b-render-fix-retained.tar.zst

Its measured payload was 32,929,468,561 bytes; the archive is 4,020,725,415
bytes with SHA-256
`d061bb2f8d8c05e4cd5934b47c45fcc5affa0f6fff7ae53c04a5c4371099392c`.
The receipt records source revision `74be64b`, binary hash
`9836839b536e51a22f0020b490c851d4287a4c3637c1078360d30a26ac2d5fa4`,
`tar_compare_clean`, and removal of the superseded probe root and attestation.
This archive is historical retention, not a current capacity attestation.

## Scientific boundary and next action

The c80 renderer correction does not alter economic state, actor inputs,
scheduling, RNG use, matching, or event ordering. The aborted extraction is
not promoted and does not invalidate the completed raw run as retained
evidence; it simply has no derived scientific verdict yet.

The next valid sequence is to commit this append-only record, rebuild the
required binaries from the resulting exact tree with pinned Go 1.27, measure
current-revision binary capacity, and rerun/extract treatment-607 only. No
other development cell, freeze authorization, or holdout is authorized by
this incident record.

