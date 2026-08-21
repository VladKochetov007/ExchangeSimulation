# Adversarial validation of the frozen market ecology

## The freeze

| | |
|---|---|
| Frozen commit | `01c9ceb11aa1` (re-frozen; see V-003) |
| Frozen config | `research/configs/frozen-baseline-2026-08-21.json` |
| Derived from | L-010 (`tencycle_v8_24h.json`), the arm with demand spread across classes |
| Run length | 24 simulated hours, eleven completed expiry rounds per venue |
| Seeds | 101, 102, 103 (`logs/frozen_10{1,2,3}`) |
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
| 3. Lifecycle semantics | dated settlement, funding and option exercise audited and passing; timestamp-collision ordering not yet tested |
| 4. Free-money loops | triangular and cross-venue searched; V-001 raised |
| 5. Agent-role proof | tool built and first results taken; to be redone on the frozen runs |
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

## V-003 — the first freeze did not describe the runs it froze

The freeze recorded commit `6bab42af38a7`. The runs it pointed at carry a build
stamp of `cf213e68`, thirteen commits earlier, with 111 lines of difference in
the simulation packages between the two — including the change that lets maker
classes quote the perpetual, which the frozen config uses. The build stamp also
reports a modified tree, though that flag is raised by untracked files as well
as by edited ones, so it does not by itself establish what was edited.

The consequence is that no measurement taken before this point can be
attributed to a named version of the simulator. That includes every number in
the liveness campaign and the first pass of this audit.

The freeze is therefore re-declared at `01c9ceb11aa1`, and the baseline is
being re-run from a binary built at that commit. Findings V-001 and V-002 are
carried over as provisional and marked for reproduction under the new freeze:
they are gross effects, a forty-fold price move and an unbounded arbitrage, and
are unlikely to be artefacts of a thirteen-commit difference, but "unlikely" is
not the standard this audit is held to.

What caught it was the run manifest recording its own build revision. That is
the provenance mechanism working; it went unread until now, which is the part
that failed.

## Audit-tool defects found by auditing the audit tools

The plan's first instruction is to attack the analysis layer before trusting
any market result. Four defects were found in tools written during this audit,
three of them by the tools' own tests and one by comparison against hand
arithmetic on a single contract.

| defect | consequence had it stood |
| Option positions read from position updates, which are never published for options | the exercise audit found no holders and passed all 150 expiries without testing anything |
| Exercise checked on the summed payout only | a holder overpaid against another underpaid nets to zero and would have passed |
| Funding and exercise residuals compared against zero rather than against the per-account truncation bound | 13 of 17 funding instants and 61 of 150 expiries reported as broken when off by one or two units |
|---|---|
| Settlement payout computed in int64: `(price − entry) × size` overflows at 1e19 against a 9.2e18 ceiling | reported three of fifteen settlements as wrong by 3.7e11 units; the engine is exact |
| Settlement audit kept each holder's latest position update rather than its latest before expiry | dropped every holder whose close-out was logged after expiry, which is all of them |
| Arbitrage cycles evaluated inside a concurrent scan | priced instants against quotes from later in the run; reported 99.4% profitable and +705 bps where a time-ordered pass reports 86–89% and +284 bps |
| Conservation audit required settlement instants to net to zero | they are not required to: a settlement pays each holder against its own entry price |

Every number in this document was produced after those fixes.

## Mechanical results that survive

| statement | evidence |
|---|---|
| Spot settlement conserves exactly | base assets net to 0 and quote debits equal logged fee revenue to the unit, per book |
| The venues' fee ledger agrees with the fee-revenue event stream | exactly, per asset, on the 24-hour run |
| Contracts are zero net supply | every contract's reconstructed net size is 0, from position updates alone |
| Independent position reconstruction agrees with the report | unrealised value gap of exactly 0 |
| Dated-future settlement is exact | 15 of 15 settlements: payout residual 0, every holder paid, no fill after expiry |
| Perpetual funding is a transfer, not a payment by the venue | 17 of 17 instants net to zero within the truncation bound of one unit per account; every instant has payers and receivers when the rate is non-zero and neither when it is zero |
| Option exercise pays intrinsic value | 150 expiries, 13 to 16 holders each, 75 in the money: no contract off by more than its rounding bound, no worthless option paid, and no holder mispaid — checked per holder, since a summed check cannot see one holder overpaid against another underpaid |
| The closed-system identity holds for ABC and CDF | residual exactly 0 |
| The USD residual is rounding, not leakage | pre-registered test passed: −1.39 units per derivative record at 6h, −1.12 at 12h, against a kill criterion of 10 and a prediction of no growth with run length |

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

## Pre-registered ablation: what makes CDF/USD run away (V-002)

**Question.** Is the runaway caused by the absence of a participant who cares
about the book's level, or by the maker's own inventory-skew rule?

**Arms**, all at 24 simulated hours on seeds 101, 102 and 103, against the
frozen baseline as control:

| arm | change | config |
|---|---|---|
| control | none | `frozen-baseline-2026-08-21.json` |
| A | price-elastic participants split between ABC/USD and CDF/USD | `v002-arm-elastic-cdf.json` |
| B | maker inventory skew switched off for every Stoikov quoter | `v002-arm-no-skew.json` |

**Primary metric.** CDF/USD terminal trade price divided by its opening price,
per seed.

**Prediction.** If the missing level-caring participant is the cause, arm A
stays inside 2× on every seed and arm B still runs away on the seeds where the
control does. If the quoting rule is the cause, the reverse.

**Kill criterion.** If both arms run away, or neither does, the stated mechanism
is wrong and the finding reverts to "unexplained instability".

**Note on arm B.** Switching the skew off changes every book, not only CDF/USD,
so it is not a clean intervention on the hypothesis; it is the closest lever
that exists without adding one. Its result is read as suggestive, not decisive.

Stated before the arms were run.
