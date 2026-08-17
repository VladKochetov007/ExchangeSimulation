# Research contract: FFA ecology campaign

Compiled late, which is itself a finding: several claims were promoted before
this existed.

## Question

In a market where no participant is told a price, which strategies survive
against which, what does the population need to form prices at all, and where
are its failure boundaries?

## Verification tier

**Tier A for mechanics, Tier B for ecology claims.**

The simulator is deterministic and byte-reproducible across `GOMAXPROCS`, so
mechanical claims — an order filled or not, conservation of units, an event
logged — are Tier A and checkable exactly.

Ecology claims are Tier B at best: a payoff comparison is a repeated empirical
measurement whose spread across seeds is unknown until measured. Most claims in
this campaign rest on one seed per arm, which is Tier C in practice. The gap
between the two has been the main source of error.

**Model-to-reality gap.** No latency, registration-order intra-step execution,
one instrument family per asset, flat option IV, no ADL or margin tiers, no
price banding. Documented in `research/exchange-mechanics-reference-2026-08-16.md`.

## Invariants that must never be violated

1. No participant receives information derived from a process that knows a price
   before the market does. Enforced in `NewSim` and guarded by a test.
2. Population sum equals insurance-fund payout minus fees. Checked per run.
3. Asset units conserve against fees collected in each asset.
4. Empty-book step fraction stays low; a high value means a quoting defect, not
   an ecology result.

## Falsifiers and disqualifying evidence

- Any arm whose result exactly equals its control is a tooling failure until
  proven otherwise. This has happened twice, from a stale binary and an ignored
  config field.
- A payoff comparison drawn from arms where fill rates differ is measuring
  execution, not strategy.
- A claim promoted from a single seed is provisional regardless of effect size.

## Distinguishing a result from an accident

A collapsed market and a terminated process both leave a log directory with no
report. They mean opposite things. One arm in this campaign was killed by a tool
timeout reaching its background children and was briefly indistinguishable from
a market failure.

Every arm must be classified with `tools/run_outcome.py`, which separates
COMPLETED, COLLAPSED (the simulator reported no valid two-sided mark), FAILED,
TRUNCATED and INCOMPLETE. Only COLLAPSED counts as evidence about the market.
Long runs are launched with `setsid` so the tool-call window does not bound the
experiment.

## Profile a configuration before comparing on it

Four consecutive experiments were undermined by a property of the base
configuration rather than by their hypothesis: a class that never traded, a
class left unmetered by my own omission, budgets that collapsed the book, and a
requote threshold that erased the arbitrage under study. Every one was found
after the run, one failed arm at a time.

`tools/characterize_run.py` reports requests, refusals and active result per
class, and flags classes that never traded. Run it on a configuration and record
which classes are comparable before designing a comparison on it. Seven of
sixteen classes are inactive in the configuration used for E-170 to E-172, which
the profile states in seconds and nine runs did not.

## Promotion rule

A claim reaches `supported` only after it holds across at least three seeds and
survives one attempt to break its mechanism. Until then it is `provisional`.

## Budgets and stop rules

Runs are 12-48 simulated hours at roughly 6-25 minutes wall each, four
concurrent. Stop a line when three diverse mechanisms fail the same way, or when
the next experiment's expected information gain is below re-running an existing
claim across seeds.
