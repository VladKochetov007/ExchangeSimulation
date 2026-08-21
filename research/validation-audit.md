# Adversarial validation of the frozen market ecology

## The freeze

| | |
|---|---|
| Frozen commit | `6bab42af38a7` |
| Frozen config | `research/configs/frozen-baseline-2026-08-21.json` |
| Derived from | L-010 (`tencycle_v8_24h.json`), the arm with demand spread across classes |
| Run length | 24 simulated hours, eleven completed expiry rounds per venue |
| Seeds | 101, 102, 103 (`logs/cycle11_v8_10{1,2,3}`) |
| Go | 1.26.5 |
| Analysis | the `analysis` package at the frozen commit |

The frozen baseline is not tuned in response to anything measured from here on.
Three tracks are kept apart and labelled in every record:

- **frozen validation baseline** — never changed while holdout facts are measured;
- **experimental arms** — changed only to test a pre-registered causal hypothesis;
- **future calibration version** — may be tuned, but only after the frozen model
  has been honestly measured.

What the frozen baseline is known to do, from the liveness campaign and stated
here only as the starting point rather than as a validation result:

| criterion | frozen baseline, seed 101 |
|---|---|
| goal liveness | 1212 / 1212 windows, 408 / 408 books alive throughout |
| strict corridor | 1112 / 1212 windows, 365 / 408 books alive throughout |
| expiry rounds | 11 option and 11 dated per venue |
| uninformed share of main spot volume | 0.39 |

That the books stay alive is not evidence that the market is realistic, correct
or economically coherent. This document records the attempt to show that it is
none of those things, and what survives that attempt.

## Status

| area | state |
|---|---|
| 0. Freeze | done |
| 1. Audit the audit tools | in progress |
| 2. Independent accounting reconstruction | in progress |
| 3. Lifecycle semantics | not started |
| 4. Free-money loops | not started |
| 5. Agent-role proof | not started |
| 6. Information boundaries | not started |
| 7. Endogenous vs hand-fed liveness | not started |
| 8. Population survival | not started |
| 9. Blinded stylized-fact scoreboard | not started |
| 10. Endogenous option structure | not started |
| 11. Causal ablations | not started |
| 12–15. Shared causes, clock artifacts, stress, mutation | not started |

## Withdrawn claims

Recorded here as they are found, with the measurement that killed them.

## Surviving claims

Recorded here only after an attempt to falsify them has been made and failed.

## Unresolved

Recorded here when an attempt is inconclusive rather than negative.
