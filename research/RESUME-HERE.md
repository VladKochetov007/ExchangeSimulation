# Resume the frozen audit

Paused mid-campaign for a laptop restart. Nothing here needs re-deriving; it
needs running. Everything below refers to the **2026-08-22 freeze**
(`research/FREEZE.md`, commit `ae13f9a`).

## Where things stand

**Determinism is settled.** Three causes were found and fixed; the acceptance
evidence is in `FREEZE.md`. Independent confirmation arrived afterwards: the
Phase 1 baseline run with **full logging** produced the identical canonical
hash to the logs-off verification run —
`5e9c827e3953faa0bf48973b398d61cc…`, 108,220,950 events. Logging does not
perturb the model.

**Phase 1 is complete as far as the runs go.**

| run | status | events | 24h hash (prefix) |
|---|---|---|---|
| `logs/f2_baseline_101` | complete, raw logs on disk | 108,220,950 | `5e9c827e3953faa0bf48973b398d61cc` |
| `logs/f2_baseline_102` | complete, raw logs on disk | 113,618,719 | `79b7d3f6a00e2f2da606d43123e994a2` |
| `logs/f2_baseline_103` | complete, raw logs on disk | 107,762,992 | `6b8b1571be3c1e648a0fd627fb54b3b6` |

**Metric extraction is partial** and must be finished before anything else:

- `f2_baseline_101`: 14 of 15 metrics extracted (`streamhash` outstanding)
- `f2_baseline_103`: 2 of 15
- `f2_baseline_102`: not started

Interrupted analyzers left empty files; those were deleted, and the stale
`.done` markers were removed, so the reaper resumes cleanly and re-extracts
only what is missing.

**Phase 2 has not produced anything.** Nine arm runs were launched and killed
part-way through their 24 simulated hours; their directories were deleted
because a partial run is not a run. No arm has been re-measured against this
freeze.

## Restart in this order

### 1. Rebuild and confirm the tree

    cd /home/vlad/development/exchange_simulation
    git log --oneline -3
    go build -o bin/multivenue ./cmd/multivenue
    go build -o bin/mvanalyze ./cmd/mvanalyze
    go build -o bin/prunegate ./cmd/prunegate

### 2. Finish extracting the baselines

    nohup bash scratch/reap.sh > scratch/reap.out 2>&1 &

It skips any artifact that is already non-empty, so it picks up exactly where
it stopped, and it now marks a run complete only once every metric has
produced output. If a run was interrupted while extracting, delete its
`.done` marker before starting:

    find research/artifacts/scoreboard/f2_* -name .done -delete

Watch with `tail -f scratch/reap.log`. Expect roughly an hour: the heavy
metrics are `reaction` (it scans every book delta), `optionsurface` and
`streamhash`.

Then confirm nothing is missing:

    ./bin/prunegate | grep -A3 f2_baseline

### 3. Phase 2 — all nine arms, both seeds

Configs are already generated from the new freeze
(`python3 scratch/build_ablation_arms.py` regenerates them if needed). Two
waves, because eighteen 24h runs at ~31GB each will not fit at once:

    nohup bash scratch/parallel_runs.sh scratch/jobs_f2_w1.txt 9 80 > scratch/f2_w1.out 2>&1 &
    # after wave 1 is reaped and pruned:
    nohup bash scratch/parallel_runs.sh scratch/jobs_f2_w2.txt 9 80 > scratch/f2_w2.out 2>&1 &

Keep the reaper running throughout. A run takes ~30 minutes wall for 24
simulated hours when nine share the machine.

**Prune only through the gate**, never by hand:

    ./bin/prunegate --prune

It refuses any run that is not SAFE_TO_PRUNE, which requires every measurement
that arm's own preregistration depends on to exist, parse and be non-empty.

### 4. Phase 4 — stress, on the new freeze

    nohup bash scratch/parallel_runs.sh scratch/jobs_f2_stress.txt 2 80 > scratch/f2_stress.out 2>&1 &

`research/configs/v005-stress-perp-2026-08-22.json` carries exactly the
preregistered delta from the old stress arm — fifty-times uninformed perp flow
and the larger metaorder sizes — and nothing else.

### 5. Phases 3 and 5

Rescore all nine arms from scratch against the new freeze, component by
component, then the provenance table (old diagnostic verdict against new).
Then the frozen stylized-fact scoreboard. Neither needs new runs once Phases 2
and 4 are extracted.

## Rules that still hold

- Never edit either frozen configuration; an arm is a copy with one named
  delta.
- The old freeze's measurements, the eleven earlier reruns and every verdict
  currently in `causal-ablations.md` are **diagnostic artifacts only**.
- Do not revise a preregistered prediction or threshold after seeing a result.
- A result that changes after the determinism fix is itself a finding; do not
  seek reproduction of the old conclusions.
- The economic interpretations in `future-calibration.md` stand and are not to
  be implemented during the audit.

## Note on `scratch/`

`scratch/` is gitignored by project rule, so the run scripts, job lists and the
reaper live on disk rather than in the repository. They survive a restart. The
job lists are also copied into `research/artifacts/` where they are tracked, so
the campaign can be reconstructed even if `scratch/` is lost.

## Useful facts

- 24 simulated hours costs ~31 minutes wall on an idle machine, ~30GB of logs;
  logging is 29% of runtime, the rest is simulation.
- `GOMAXPROCS` no longer affects results, so runs can use whatever the
  scheduler gives them.
- `scratch/dethash_par.sh <label> <duration> <n> [procs]` re-checks determinism
  at any horizon.
- The divergence locator is `checkpoint_interval_seconds` plus
  `trace_from_nano`/`trace_to_nano` in any config; compare `checkpoints.jsonl`
  between runs to bracket a divergence, then trace that window alone.

## 2026-08-22 continuation — provenance and latency measurement

- V-012 was resolved as duplicate checkpoint telemetry only: seed 101's
  2,570,397 excess observations were 2,332,800 `maker_state` and 237,597
  `conservation_violation` observations. Raw evidence is complete.
- Execution checkpoints, normalized evidence multiset, and exact persisted
  JSON-record multiset are now distinct contracts with regression coverage.
- New checkpoints explicitly identify the ordered `execution_observations`
  domain and carry `execution_stream_hash`; legacy `rolling_hash` remains only
  as a compatibility alias for the preserved baseline checkpoint files.
- V-013 made prunegate evaluate its declared manifest predicates rather than
  accept any unrelated nonzero field. No raw logs have been pruned.
- V-014 found that reaction lag is not realized network latency. Compact
  courier `latency.json` instrumentation was added and shown not to perturb a
  5-minute frozen observable stream.
- V-015 made the shared analyzer scanner fail closed on malformed relevant
  evidence. Rescan each baseline's full normalized evidence stream before
  certifying its already-extracted metrics.
- V-016 binds prune-gate verdicts to the exact simulator freeze, so the old
  diagnostic ablation verdicts cannot certify any ae13f9a treatment.
- Baseline 101/102/103 extraction and their compact latency sidecars are
  complete. The stricter V-015 full-evidence revalidation scan is the final
  gate before Phase 2; raw logs remain retained even though prunegate now
  reports the control contract safe.
