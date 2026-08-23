# Future calibration and redesign

Nothing in this file is implemented. The frozen economic model is unchanged and
must stay unchanged until the audit is finished. This is the record of what the
audit implies for the *next* model, written down now so the reasoning is not
lost and so it cannot quietly leak into the frozen baseline.

## Autopsy handoff

The frozen ae13f9a audit is closed as
[`frozen-autopsy-ae13f9a.md`](frozen-autopsy-ae13f9a.md), with machine-readable
provenance in
[`artifacts/frozen-autopsy-ae13f9a.json`](artifacts/frozen-autopsy-ae13f9a.json).
It supports deterministic execution and a complete persisted-evidence contract,
but it does **not** establish a realistic or economically complete ecology.

The implementable successor is deliberately staged in
[`v2-design.md`](v2-design.md). That design begins with missing evidence
contracts, then local-information price discovery, before attempting
stabilization, funding, options, or liquidation redesign. Nothing in this
handoff alters ae13f9a or reclassifies its findings.

## Cross-venue informed makers are legitimate

The own-mid-anchor arm is the strongest causal result in the campaign:
cross-venue dispersion widens by 59× and 337× when makers stop anchoring to the
shared index. That is **not** a finding that cross-venue-informed market making
is illegitimate. Real makers on fragmented markets do watch other venues and
build reference prices from them; a model without that is the unrealistic one.

The narrow defect is the **shared, zero-latency aggregate available upstream of
every agent**. One provider computes a consensus midpoint inside the same
automation tick and every Stoikov maker reads the same number, so the venues are
synchronised by a variable no participant could actually observe.

The replacement keeps the behaviour and removes the privileged variable, by
making the information path explicit:

    venue public data
      -> participant-specific market-data latency and staleness
      -> agent-local composite calculation
      -> quote decision
      -> request latency
      -> venue

Makers should differ in which feeds they take, how they weight them, what
horizon they smooth over, and what model they use. There must be no global
instantaneous fair-price variable anywhere in the system.

Read the arm accordingly: **cross-venue information carried by market makers is
the dominant synchronisation channel in this ecology.** It is not evidence that
the channel should be removed.

This probably also explains why explicit arbitrage looks weak here. An informed
maker transmits information by revising quotes, which can close a dislocation
before any two-leg trade becomes executable. Quote-mediated convergence and
trade-mediated convergence are different mechanisms and the campaign has not
separated them.

**Factorial experiment to run later**, measuring quote-mediated against
trade-mediated convergence:

| arm | informed makers | explicit cross-venue arbitrage |
|---|---|---|
| 1 | on | on |
| 2 | off | on |
| 3 | on | off |
| 4 | off | off |

## Funding is not shown to be unimportant

The funding-off arm falsified one specific claim: that funding is the *dominant*
anchoring mechanism **in this configuration**. Removing funding entirely — zero
settlement instants against thirty-six — moved the perpetual premium by 2–3%,
inside seed noise.

It does not follow that funding is economically unimportant. The most likely
reading is that the shared maker reference already pins the perpetual to spot,
leaving funding nothing to do. In a model where price discovery is not
mechanically synchronised, funding should act on expected carry returns and so
on basis-trader positioning and order flow.

**Retest funding after the price-discovery channels are decoupled.** Until then
the honest statement is "not dominant here", not "unimportant".

## CDF/USD runaway: what not to do, and what to do

The V-002 runaway is caused by the maker's inventory skew acting as an amplifier
with no damper present. The obvious fix — add a momentum taker — is wrong:
momentum is a legitimate population but it is another positive-feedback
mechanism and may amplify the runaway rather than damp it.

The preferred design, in order:

1. **Passive quotes are post-only by default.** A requote must never
   accidentally cross and become an aggressive inventory trade. The runaway
   works precisely because the maker's own requote prints a new midpoint.
2. **Aggressive rebalancing is a separate, explicit action** — a rebalance or
   hedge order with taker fees, slippage, participation limits and risk limits,
   not a side effect of quoting.
3. **Inventory affects sizes as well as prices.** Long inventory means a
   smaller or less aggressive bid and a larger or more aggressive ask; short
   inventory the reverse. Skewing price alone is what makes the loop tight.
4. **Flow and adverse-selection sensitivity.** Recent aggressive signed flow
   should be able to widen the spread, thin the toxic side, or move the
   reservation price.
5. **Keep heterogeneous price-elastic and value participants** with real
   economic motives, so that extreme levels attract opposing flow for a reason
   rather than by construction.
6. **Keep momentum agents**, as one destabilising population among others,
   never as the stabiliser.

Longer term: do not let every maker be a variant of one Stoikov policy. Several
genuinely distinct information and risk models are what make the ecology
informative rather than a single rule observed at different parameter values.

## What the determinism work implies for latency

Fixing V-008 turned up a fact worth carrying forward: the frozen configuration's
latency is **non-binding**. At a one-second automation and quote interval, delays
of 0.5–50ms almost always land inside the same step, and the event digest of a
constant-latency run is identical to a run with the full random latency profile
set. The latency-x10 arm's null result on cross-venue dispersion should be read
in that light: the regime, not the implementation, is why latency does not bite
there.

A model that intends latency to matter needs either a much finer step or
latencies comparable to the decision interval. That is a calibration decision,
not a bug fix, and it belongs here rather than in the frozen baseline.
