# V2-5 P2a — execution and extraction plan

Status: **preregistered before execution.** The companion scientific contract
is [`v2-5-p2-perp-exposure-preregistration.md`](v2-5-p2-perp-exposure-preregistration.md).

## Immutable cells

| arm | configs | seeds | horizon | output root |
| --- | --- | --- | --- | --- |
| A, submission disabled | `configs/v2-5-p2/A-{101,103}.json` | 101, 103 | 5 min | `artifacts/v2-5-p2/A/` |
| B, submission enabled | `configs/v2-5-p2/B-{101,103}.json` | 101, 103 | 5 min | `artifacts/v2-5-p2/B/` |

Build `multivenue`, `mvanalyze`, and `prunegate` from the committed source
head. Record the source commit, binary SHA-256, config SHA-256, GOMAXPROCS,
wall time, terminal execution hash, and persisted-evidence artifact hash in
each final cell artifact. Each output directory must be absent before launch.

Run up to four independent worlds concurrently only after checking disk
headroom. Completion requires final non-empty `greeks.json` **and**
`latency.json`; process names, partial sidecars, terminal text, or checkpoints
are not sentinels.

## Required extraction before any score or prune decision

For every completed cell, retain raw evidence and write:

```text
mvanalyze -metric perpexposurehedger -json
mvanalyze -metric observationreceipts -json
mvanalyze -metric streamhash -json
mvanalyze -metric evidenceartifacthash -json
mvanalyze -metric conservation -json
mvanalyze -metric positions -json
```

`latency.json` is already the simulator's compact persisted courier-delivery
artifact, not an `mvanalyze` metric. Validate its non-empty rows directly:
each P2 link must report 40-ms delivered market data and 20-ms delivered
requests. Do not create a replacement analyzer artifact merely to duplicate
that evidence.

Then run the hardened prune gate as a read-only check. P2a is not prunable:
the dedicated P2 measurement contract and paired verdict must be recorded
first, and raw logs remain the sole independently inspectable evidence for
this new actor.

## Stop rules

- Any failed evidence, conservation, position, or latency contract makes that
  cell invalid and halts P2a interpretation.
- A disabled-arm submission is a policy/evidence failure, not a control.
- No enabled accepted hedge in a paired seed is `NOT EXERCISED`, not evidence
  for or against funding/carry economics.
- Do not inspect or score basis/funding outcomes until all four activation
  audits are present and valid.
