# V2-5 P3e P0 — attempt 0 interruption record

Status: **NON-EVIDENCE — user-interrupted before completion.** This record
preserves provenance for an incomplete P0 launch and prevents it from being
mistaken for the registered 98-hour cell.

## Launch and interruption

| item | value |
| --- | --- |
| source revision at launch | `02191c79505dc28424d845fb2606a6af61f0a16c` |
| config / SHA-256 | `configs/v2-5-p3e/p0-B-107.json` / `206fc24ee0bc7f16aacabcbebc794b1fedd611922ef58e4578d5df142b323835` |
| requested setting | seed 107 / 98 simulated hours / full evidence / `GOMAXPROCS=4` |
| simulator / analyzer / prune-gate SHA-256 | `3e98b2142d374c137d2d8211540b6f294c7a796b28224d01b8fd612386c8826f` / `d388d16a4ca18e1412a29926972e06637827e7ad5f31d8239fd4ebefa9acd0a5` / `211439b1f386cdd97e09041b3867389067628112a26b8d5af47e6ea1e9b28816` |
| interruption | 2026-08-24T20:29:58+03:00, explicit user stop request |

The process was interrupted with `SIGINT`. It did not produce either required
final-only completion sentinel: `greeks.json` and `latency.json` are absent.
The retained partial directory contains only a manifest, checkpoints, and
incomplete compact receipt/schedule sidecars; its decision sidecar is zero
bytes. It has been moved without deletion to
`artifacts/historical/v2-5-p3e-p0-attempt0-interrupted/`.

## Prohibited use

This attempt must never be extracted, scored, compared, cited, or pruned.
It cannot establish a passive-exit action, failure, latency, determinism,
accounting, or market outcome. It remains solely a restart/provenance record.

## Restart gate

The independent five-minute preflight remains valid at
[`v2-5-p3e-passive-exit-p0-preflight.md`](v2-5-p3e-passive-exit-p0-preflight.md).
After reboot, verify the worktree and disk, rebuild all three executables from
the current committed source, byte-check the immutable config, and launch a
fresh 98-hour world into the now-empty registered path
`artifacts/v2-5-p3e/p0-B-107/`. Completion is defined only by nonempty final
`greeks.json` **and** `latency.json`; then run
`scripts/extract-v2-5-p3e-metrics.sh` before any interpretation.
