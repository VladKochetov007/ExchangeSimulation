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
| 2. Independent accounting reconstruction | done for balances, positions and settlements; residuals below |
| 3. Lifecycle semantics | not started |
| 4. Free-money loops | triangular and cross-venue searched; V-001 raised |
| 5. Agent-role proof | not started |
| 6. Information boundaries | not started |
| 7. Endogenous vs hand-fed liveness | not started |
| 8. Population survival | not started |
| 9. Blinded stylized-fact scoreboard | not started |
| 10. Endogenous option structure | not started |
| 11. Causal ablations | not started |
| 12–15. Shared causes, clock artifacts, stress, mutation | not started |

## Withdrawn claims

| claim | killed by |
|---|---|
| The three spot books form an economically coherent triangle | V-001: the triangular loop is profitable in 99.4% of instants and the dislocation compounds without bound |
| The triangular arbitrageur links the spot books | V-001: it fires the correct direction 2,584 times in thirty minutes at a fixed 0.05 ABC and loses the race by two orders of magnitude |
| Any CDF-denominated price or statistic in the frozen baseline | V-002: CDF/USD rises forty-fold in twenty-four hours, driven by its own market maker |
| That liveness implies a working market | V-002: CDF/USD was two-sided and trading in every window of its life while its price rose forty-fold |

## Findings

- **V-001** (`research/no-arbitrage-audit.md`): persistent, compounding triangular
  arbitrage. Cross-venue same-asset pricing passes.
- **V-002**: the CDF/USD maker becomes the dominant net taker buyer of its own
  book from hour thirteen and takes the price up forty-fold. Stable at six
  hours, 2.27x at twelve, 40.26x at twenty-four. V-001 is downstream of it.

## Surviving claims

Recorded here only after an attempt to falsify them has been made and failed.

## Unresolved

Recorded here when an attempt is inconclusive rather than negative.

## Pre-registered test: is the USD residual truncation or leakage?

**Observation.** The closed-system identity holds exactly for ABC and CDF
(residual 0) and leaves a residual in USD: 4,248 units over 30 simulated
minutes and 7,594,763 units over 24 hours, against an external float of
1.65e16 units (relative 4.6e-10). Spot books conserve exactly — base assets net
to zero and quote debits equal the venues' logged fee revenue to the unit — and
the venues' own fee ledger agrees with the fee-revenue event stream exactly.
The residual is therefore in the derivative cash path.

**Hypothesis.** Every realised profit, funding charge and settlement is an
integer `MulDiv`, which truncates toward zero, so each such operation can lose
up to one unit. If that is the whole story, the residual is bounded by the
number of truncating operations.

**Primary metric.** Residual in USD units divided by the number of derivative
balance-change records in the same run.

**Prediction.** If truncation explains it, that ratio stays below about ten and
does not grow with run length.

**Kill criterion.** If the ratio exceeds ten, or grows with run length, the
residual is not truncation and a mechanism is leaking value.

**Design.** The frozen baseline, seed 101, at 2, 6, 12 and 24 simulated hours.
Stated before the runs were measured.
