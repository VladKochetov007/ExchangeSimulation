# V2-R2-SV1D one-sided elastic-liquidity successor preregistration

Date: 2026-09-09  
Candidate: `V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY`  
Predecessor: closed R2 and `V2-R2-SV1C-STRICT-RISK-CDF-LIQUIDITY`  
Status: preregistration only; no simulator world, capacity probe, or holdout
has run from this candidate

## Scientific boundary

The R2 candidate remains **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**.
SV1C remains archived as **VALID EVIDENCE / NEGATIVE ACTIVATION** after its
independent review. This document does not rescue either trajectory, select
the seven successful SV1C supplier instances, weaken the SV1C activation
predicate, or alter any historical JSON/binary artifact.

SV1D preserves the accepted R2 calendar/lifecycle, strict-risk, matching,
actor-information, and `evstream_v3` contracts. It preserves the existing
eight ABC/USD `elastic_supplier` participants unchanged. It changes one
configurable behavior of the separately funded CDF/USD supplier class: when a
supplier has a usable delayed local snapshot of a one-sided book, it may
quote its inventory-selected side using that local touch and its own evolving
private reference. The default for the new option is disabled, so historical
R2 and SV1C configurations are not silently changed.

## Mechanism hypothesis

SV1C withdraws when its delayed local CDF/USD book is one-sided because it
requires a two-sided midpoint before choosing both its inventory signal and
quote price. This creates a testable feedback boundary: once one side
disappears, a finite supplier that would economically reduce its inventory
cannot post the missing side, even when it has available balance sheet and a
valid local observation.

SV1D tests the narrower hypothesis that permitting an inventory-sensitive
supplier to quote one selected side against a delayed one-sided local touch
will shorten persistent one-sided intervals often enough for strict terminal
valuation to remain possible. The quote is an ordinary passive order exposed
to fill risk; the mechanism does not prescribe a CDF price, spread, volume,
survival outcome, or two-sided obligation.

## Registered behavior

The new configurable policy is `quote_on_one_sided_local_book`, default
`false`. SV1D treatment sets it to `true` for the already registered CDF
roster; the mode-off paired control uses the same roster and sets it to
`false`. A separate no-roster control remains the predecessor-style ecology
control. No balance, inventory, elasticity, cadence, reference half-life,
latency, or fee value is changed from the SV1C roster.

At each supplier decision, after the normal delayed local snapshot checks:

1. with a positive bid and ask, the supplier uses the existing midpoint and
   existing inventory-target rule;
2. with exactly one positive side, the local anchor is that observed touch;
3. the private reference is updated only toward that delayed local anchor by
   the preregistered half-life; it is never replaced by a global mark, index,
   last trade, remote venue, or simulator field;
4. the inventory target is computed from the local anchor using the existing
   elasticity and finite position limits;
5. if the target gap selects the present side, the quote uses the observed
   touch; if it selects the missing side, the quote price is the fixed-point
   midpoint of the private reference and the observed touch, clamped to at
   least one configured tick beyond the present touch in the missing-side
   direction; and
6. the supplier submits at most one passive quote, bounded by its existing
   cash, inventory, position, quantity, observation-age, and loss limits.

If both sides are absent, the observation is stale, the book is locked/crossed,
the calculated price is invalid, the target gap is zero, or a risk limit is
reached, the supplier withdraws or waits. A cancellation is not automatically
replaced. The supplier has no obligation to quote either side, and no code
path replenishes capital, inventory, or a withdrawn order.

The one-sided missing-side price is deterministic integer arithmetic. Let `T`
be the positive local touch, `R` the current private reference, and `tick` the
configured positive tick. The blended reference is `floor((R+T)/2)` using
overflow-safe fixed-point arithmetic. For a missing ask above a bid, the
candidate is `max(blended_reference, bid+tick)`; for a missing bid below an
ask, it is `min(blended_reference, ask-tick)`. A nonpositive or overflowed
candidate fails closed. The candidate is not a target or convergence rule: it
is a participant's local belief plus its inventory constraint.

## Information and economic constraints

Each supplier may read only the delayed `CDF/USD` snapshot delivered through
its ordinary actor gateway and its own private state. It may not read an
exchange object, global index, other venue, undelayed book, terminal state,
or any hidden simulator oracle. Every decision records observation sequence,
delivery time, age, local touch, private reference, selected side, price,
quantity, inventory, balances, loss/drawdown, and action reason.

Each supplier has finite initial USD/CDF balances, finite gross inventory and
position limits, a finite quote quantity, maker fees, and a positive loss
budget. Fills change its inventory, cash, fees, and PnL through the ordinary
exchange path. It may cancel, reprice, or permanently withdraw when its local
observation, inventory target, or risk budget changes. The existing eight
ABC/USD suppliers remain byte-identical in the treatment and controls.

## Activation criteria

The strict SV1C supplier activation predicate is retained rather than relaxed:
all twelve configured CDF supplier/venue instances must receive eligible
delayed local observations, submit at least one accepted passive order, have a
CDF fill that changes inventory, expose a reconstructible balance/PnL/risk
transition, and produce a post-fill inventory-responsive decision. At least
one supplier decision must use the one-sided local-book policy, and at least
one quote must be cancelled, repriced, or withdrawn for a local observation,
inventory, risk, or unavailable-side reason.

The activation evidence must prove finite-capital accounting, inventory and
position bounds, delayed-information provenance, ordinary fill/PnL
reconciliation, and complete supplier-removal reconstruction. If any
supplier fails the registered predicate or the counterfactual coverage is
incomplete, SV1D is a negative/invalid activation gate and cannot advance to a
24-hour campaign.

## Anti-cheating and kill criteria

Reject SV1D if survival is primarily produced by a guaranteed quote, forced
two-sided quoting, forced replacement, replenishment, effectively unlimited
capital, a hidden external price anchor, simulator-state access, or a private
reference that is reset from market outcomes outside the declared local update
rule. Reject it if any finite balance, inventory, position, quantity, fee, or
loss limit is exceeded or ignored.

Retain the preregistered concentration limits: supplier CDF volume share above
75%, or more than 75% of displayed CDF depth for more than half of measured
active intervals in any venue, is a kill condition. Persistent one-sided CDF
books, strict valuation failure, missing risk/PnL transitions, or unbounded
replenishment also kill the candidate. The candidate fails qualitatively if
the new mode acts as a structural two-sided market-making obligation even if
the numeric limits pass.

## Measurements and falsifiers

For each venue and supplier, retain volume/depth share, inventory and cash
paths, position and loss limits, realized/unrealized PnL, quote lifetime,
submit/accept/fill/cancel/reprice/withdraw counts, local observation age and
sequence, one-sided quote source, private-reference path, and risk-limit state.
For treatment and both controls, retain exact one-sided intervals, empty-side
durations, strict mark availability, terminal valuation status, and the
canonical global evidence hashes.

The mechanism is falsified for SV1D if the activation predicate fails, the
treatment remains strictly unvaluatable, one-sided intervals are not reduced
relative to the matched mode-off control, or any anti-cheating/kill criterion
fires. A negative result is scientifically valid. No post-outcome threshold or
roster selection may rescue it.

## Fixed promotion sequence

1. hash this preregistration and the exact SV1D configs before measurement;
2. implement the behavior as an opt-in configuration with unit tests for
   two-sided parity, one-sided bid/ask pricing, tick boundaries, overflow,
   stale observations, withdrawal, delayed-information provenance, finite
   accounting, and evidence neutrality;
3. obtain one fresh independent Sol-xhigh review of the exact successor tree;
4. build clean provenance-pinned Go 1.27 binaries and measure actual binary
   evidence capacity without deleting retained evidence;
5. run only the new development seed-659 activation probe, then obtain an
   independent activation review;
6. only if that probe satisfies every activation and anti-cheating gate, run
   the separately registered development pairs and controls; and
7. extract/review development evidence before any freeze authorization.

Holdouts `619`, `631`, and `641` remain untouched. No step in this document
authorizes a holdout, a full campaign before activation, economic retuning
after an outcome, or weakening strict terminal accounting.
