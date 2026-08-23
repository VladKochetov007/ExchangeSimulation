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

    setsid nohup bash scratch/reap.sh > scratch/reap.out 2>&1 < /dev/null &

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

    setsid nohup bash scratch/parallel_runs.sh scratch/jobs_f2_w1.txt 9 80 > scratch/f2_w1.out 2>&1 < /dev/null &
    # after wave 1 is reaped and pruned:
    setsid nohup bash scratch/parallel_runs.sh scratch/jobs_f2_w2.txt 9 80 > scratch/f2_w2.out 2>&1 < /dev/null &

Keep the reaper running throughout. A run takes ~30 minutes wall for 24
simulated hours when nine share the machine.

**Prune only through the gate**, never by hand:

    ./bin/prunegate --prune

It refuses any run that is not SAFE_TO_PRUNE, which requires every measurement
that arm's own preregistration depends on to exist, parse and be non-empty.

### 4. Phase 4 — stress, on the new freeze

    setsid nohup bash scratch/parallel_runs.sh scratch/jobs_f2_stress.txt 2 80 > scratch/f2_stress.out 2>&1 < /dev/null &

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
- V-017 makes the independently reconstructed exact JSON-record digest a
  mandatory prune-gate artifact for every run.
- V-018 replaced the reaper's sandbox-incompatible process-name test with
  final-only `greeks.json` + `latency.json` completion sentinels. Wave 1's
  temporary partial derived artifacts were discarded. The interrupted raw
  treatment logs were then discarded too because no run reached final state;
  restart every Wave 1 arm from scratch.

## 2026-08-23 clean stop — restart here

The ae13f9a baseline control evidence remains intact. The last completed
checks were the V-015 strict full-evidence scans:

| seed | normalized persisted evidence events | digest |
|---:|---:|---|
| 101 | 105,650,553 | `91de88d54532c004572670511cfe3b7da76c0cc597c55adccf67bf47fb2f7c69` |
| 102 | 111,048,322 | `cfaf7c52df4ea95ae795ae06c6cd85e8645afd136d05f5be6f4047757963b95a` |
| 103 | 105,192,595 | `42d14cd85a3cefb4c0832a5c891fd5643c25ef8b36394b540c159502f06d5fb3` |

All three compact latency controls are attached to their baseline scoreboard
directories. Seed 102's final compact control was:

```text
event_count=111,048,322
checkpoint=9a0265579fff9e8a1477aaf2e606782889f7842bc512fccd9096ac7a17a6fb05
scheduled=193,376,039 delivered=193,374,477 undelivered=1,562
mean sampled=11.92ms; mean actual delivery=41.20ms
```

Three instrumentation/analyzer-only commits were made after `472004a`:

- `beaacf3 fix(audit): separate execution and evidence provenance`
- `c609b1b fix(audit): bind prune verdicts to simulator freeze`
- `5d3c49a fix(audit): gate exact persisted evidence digest`

The prerequisite regression suite passed, including the fresh-process
`MULTIVENUE_DETERMINISM=1` test across full/no-log and GOMAXPROCS variants.

**Wave 1 was deliberately stopped, not completed.** Nine treatment processes
were interrupted at about 9.5 simulated hours; none emitted final
`greeks.json`, and all nine partial raw directories and their accidentally
created partial scoreboard directories were deleted. They are not evidence.
No treatment or stress result exists at ae13f9a.

Before restarting Wave 1:

1. Rebuild `bin/multivenue`, `bin/mvanalyze`, and `bin/prunegate`.
2. Finish V-017 for all three baselines: calculate and retain
   `evidenceartifacthash.json` from the preserved raw logs. Historic baselines
   predate runtime artifact sidecars, so record their independently scanned
   exact-record digest without pretending it can be compared to a missing
   runtime sidecar.
3. Confirm `./bin/prunegate -json` leaves controls measurement-incomplete only
   for that new required artifact; do not prune the baseline logs.
4. Start `research/artifacts/jobs_f2_w1.txt` again from scratch.
5. Start `scratch/reap.sh` only after the jobs have begun. Its repaired
   completion rule requires both final-only `greeks.json` and `latency.json`,
   and its metric list now includes `evidenceartifacthash`. Never use host
   process-name detection again.

The disk is clean at roughly 395 GB free. The frozen baseline raw logs remain
on disk; no evidence authorized for retention was removed.

### Post-restart update

V-017 is now complete for all three baselines. Exact persisted-record digests
are in each scoreboard directory as `evidenceartifacthash.json`; see V-017 in
`validation-audit.md` for values. Baseline gates pass but their raw logs remain
retained. Wave 1 was restarted from scratch with the completion-sentinel
reaper; no interim treatment metric is evidence until its paired full runs
finish and their contracts pass.
- Baseline 101/102/103 extraction and their compact latency sidecars are
  complete. The stricter V-015 full-evidence revalidation scan is the final
  gate before Phase 2; raw logs remain retained even though prunegate now
  reports the control contract safe.

### 2026-08-23 continuing audit update

- V-005 preregistered stress is complete. It exercised 7,054 forced closes in
  seed 101 and 7,177 in seed 103. Every observed close has an independently
  reconstructed pre-to-post position reduction and zero per-close contract
  residual. No bankruptcy, deficit, or insurance-fund absorption occurred, so
  those paths remain NOT EXERCISED. See `v005-stress-ae13f9a.json`.
- V-022 caught a validator blind spot: a scratch `funding_twice` mutant passed
  the former net/sign audit. The new symbol-scoped, per-account detector caught
  85 duplicate payments over five exercised instants. All 23 retained current
  baseline/treatment/stress derivative artifacts were replayed and are clean;
  pre-fix artifacts are preserved under `historical/v022-pre-duplicate-funding-derivatives/`.
- The ae13f9a baseline scoreboard is appended to
  `research/stylized-facts-baseline.md`. Next: clock-artifact experiments and
  remaining high-value mutations, then frozen-autopsy synthesis. Do not enter
  v2 design yet.
