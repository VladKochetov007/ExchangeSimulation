# Frozen baseline

**Commit:** `ae13f9aa6e5fd23539637a8c4a3d2d4f4c3ad107`
**Configuration:** `research/configs/frozen-baseline-2026-08-22.json`
**Declared:** 2026-08-22

## Why this is a new freeze

The 2026-08-21 freeze was not reproducible. Runs of the same commit, config and
seed produced different event streams, and pinning to one core made that rarer
rather than absent. Three separate accidental inputs were removed (V-008 in
`validation-audit.md`), all of them changes to code that decides event
ordering or random-draw sequence, so this is a new freeze rather than a
continuation of the old one.

The configuration is parameter-identical to `frozen-baseline-2026-08-21.json`.
Only `experiment_id` and `description` differ. The old file is left untouched.

## Verification

Canonical event-stream identity at the experiment horizon, from fresh
processes:

| check | runs | horizon | result |
|---|---|---|---|
| seed 101, GOMAXPROCS 1 | 3 | 24h | 1441/1441 checkpoints identical, 108,220,950 events, one hash |
| seed 101, GOMAXPROCS 4 | 2 | 24h | identical to the GOMAXPROCS 1 runs |
| seed 103, GOMAXPROCS 1 and 2 | 3 | 3h | identical |

Terminal hash for seed 101 over 24h:

    5e9c827e3953faa0bf48973b398d61cc8dcd22c46062721bea775e847581cf56

Host parallelism is not a model parameter: the same seed produces the same
events whatever GOMAXPROCS is set to.

## Rules

- Never edit either frozen configuration. An arm is a copy with one named
  delta.
- Do not compare post-freeze treatment runs against pre-freeze baseline logs.
  Everything measured before this commit is a diagnostic artifact, including
  the eleven ablation reruns completed on 2026-08-22 and every verdict in
  `causal-ablations.md`.
- Any later change to code that affects event ordering or random-draw
  sequence requires another freeze and another 24h verification.

## What must be re-run against this freeze

1. Paired baseline, seeds 101 and 103.
2. The incomplete targeted ablation arms: basis-off, delta-hedge-off,
   vanna-volga-off, option-value-takers-off, latency-x10.
3. Seed 102, later, to complete the three-seed stylized-fact baseline.

## Reproducing the verification

    ./bin/multivenue -config research/configs/frozen-baseline-2026-08-22.json \
      -seed 101 -duration 24h -logdir <dir>

with `log_mode: none` and `checkpoint_interval_seconds: 60` added to a copy of
the config; then compare `checkpoints.jsonl` between runs. A full-log run's
terminal digest can also be compared with `mvanalyze -metric streamhash`.
