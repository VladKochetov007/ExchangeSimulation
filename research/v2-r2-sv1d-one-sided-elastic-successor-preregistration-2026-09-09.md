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

The exchange admission minimum and the scientific survival threshold are
separate registered quantities. The current CDF/USD instrument admits orders
at `minimum_executable_qty = 100000` base units. SV1D records
`minimum_qualifying_qty = 1000000` base units (ten admission lots); only the
latter threshold can establish that a missing side was materially restored or
that a weak-side interval is cured. Both values are retained in the normalized
manifest, and the supplier's declared admission minimum must equal the actual
registered instrument minimum before a bounded-risk supplier is constructed.

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

After a one-sided quote is fully filled or canceled, the supplier must observe a
strictly later local snapshot before submitting another one-sided quote. This
prevents a close event from becoming a same-observation replenishment path.

The one-sided missing-side quote price is deterministic integer arithmetic. The
`tick` is read from the registered `CDF/USD` instrument and is not an
independent supplier parameter; the configuration and normalized manifest must
carry the same value. The grid origin is zero, and an admissible positive price
is an integer multiple of `tick`. Let `T` be the positive local touch and `R`
the current private reference. First compute `sum = checked_add(R, T)` and
`B = floor(sum / 2)`. For bid-only data, compute
`lower = checked_add(T, tick)`, `candidate = max(B, lower)`, then
`quote = ceil_to_tick(candidate)`. For ask-only data, compute
`upper = checked_sub(T, tick)`, `candidate = min(B, upper)`, then
`quote = floor_to_tick(candidate)`. For positive operands,
`floor_to_tick(x) = (x / tick) * tick`; `ceil_to_tick(x)` adds one to the
quotient exactly when `x % tick != 0`, with checked multiplication. Every
intermediate addition, subtraction, quotient increment, and multiplication is
checked; a nonpositive, overflowed, or exchange-invalid result fails closed.
This fixed-point statement applies only to the one-sided quote-price
construction. The existing exponential private-reference update and
inventory-target calculation retain their registered floating-point form and
are covered by pinned-toolchain determinism tests. The candidate is not a
target or convergence rule: it is a participant's local belief plus its
inventory constraint.

### One-sided supplier risk mark

The supplier's loss-budget mark is a top-of-book policy mark; it does not claim
that the entire gross inventory can be liquidated at the displayed quantity.
With both sides present, marked equity uses the midpoint, as in the
predecessor. With bid-only data, positive gross CDF inventory is valued at the
bid; with ask-only data, because the supplier's positive gross inventory has no
displayed bid at which it could be reduced, marked equity is unavailable and
the supplier withdraws. The supplier's gross CDF inventory is always
nonnegative under the registered inventory limits. A zero-inventory diagnostic
may record the touch but cannot initialize a positive loss budget from it. If a
future roster permits net short inventory, its buy-to-close risk mark must
analogously require an ask and use that ask. Missing required policy marks
therefore fail closed; they never become zero, entry price, private reference,
or a fabricated midpoint.

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

The evidence must also make touch provenance auditable. For every one-sided
decision, the decision record is joined to the public snapshot at the delayed
observation's `source_sequence` and global `event_seq`, not to the public state
at decision time. The analyzer records whether the present touch was public,
whether a registered successor-supplier order occupied that price level, the
successor-supplier displayed depth at the touch, and the public displayed depth
at the touch. It reconstructs independent depth by removing all configured
SV1D orders from that same globally ordered snapshot. An accepted missing-side
decision becomes a restoration candidate only when the accepted passive order
has at least `minimum_qualifying_qty` and lies one to twenty configured ticks
from the observed present-side touch. The independent present-side touch is
used only by the separate supplier-removal self-reference test. Unaccepted or
sub-threshold decisions remain diagnostic and cannot enter `N_restore`. A
candidate is credited only if that same accepted order is reconstructibly live
with at least the qualifying minimum remaining quantity at the first later
public snapshot, in global event order, where the previously missing side has
at least `minimum_qualifying_qty` displayed quantity. A canceled, fully filled,
or otherwise closed candidate is unresolved rather than credited to unrelated
liquidity. The per-candidate audit records the order outcome, restoration
sequence, candidate executable quantity, supplier and independent depth, and
whether a later inventory-responsive decision followed a partial fill. The
restoration self-reference test uses the present-side anchor from the delayed
observation's `source_sequence`, not the side that the supplier is trying to
restore. Remove all SV1D orders from that source snapshot. The restoration is
self-referential if the observed anchor disappears, changes, or has less than
`minimum_qualifying_qty` residual non-SV1D depth after removal. This permits
the legitimate causal case in which independent present-side liquidity
motivates the supplier to add the missing side. Let `N_self` be the number of
qualifying restorations whose observed anchor fails that counterfactual and
`N_restore` the number of all qualifying restorations; the preregistered
self-reference fraction is `N_self / N_restore`, and SV1D is killed when it
exceeds `0.50`; `N_restore = 0` also fails the activation requirement.
Separately, report whether the restored side is supplier-only or
supplier-dominated; those side-specific concentration diagnostics remain
subject to the limits below and do not redefine `N_self`. This objective rule
rejects a roster that manufactures its own observation anchor while allowing
a supplier-only restored side when the other side was independently present.
The same self-reference threshold is applied independently to each venue audit;
an aggregate pass cannot conceal a self-referential venue.

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
reconciliation, and complete supplier-removal reconstruction over all public
venue snapshots. Client-specific market-data snapshots are not public-book
observations and are excluded from that denominator; the analyzer must report
both counts explicitly and fail if any public snapshot is not reconstructible.
The causal chain must include at least one delayed one-sided observation that
produces an inventory-selected missing-side quote, that quote is submitted and
accepted, and the public snapshot afterward shows that side restored. The
evidence must then show whether the quote rested or filled and whether a later
inventory-responsive decision followed. If any
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
active intervals in any venue, is a kill condition. Apply the same threshold
separately to aggregate supplier bid depth and aggregate supplier ask depth,
including time-weighted and executable-quantity-qualified shares. The
registered SV1D roster sets `minimum_executable_qty = 100000` base units and
`minimum_qualifying_qty = 1000000` base units. A one-sided restoration
candidate qualifies only when the supplier's accepted resting quantity is at
least `minimum_qualifying_qty` and its price is between one and twenty
`CDF/USD` ticks from the observed present-side touch; it is credited only
while that same order remains live with at least the qualifying minimum
quantity at the qualifying snapshot. The current registered
instrument has `tick = 100000` quote units; the normalized manifest must bind
both values to the instrument rather than duplicate them as actor economics.
The analyzer reports quote distance, displayed and executable depth, and
whether the previously missing side was supplier-only. A tiny or far-away
quote that merely makes a book technically two-sided cannot satisfy the
survival predicate. Persistent one-sided CDF
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
canonical global evidence hashes. The frozen survival comparator is a
public-only, event-time-weighted weak-side duration. For every client-zero
`CDF/USD` public snapshot, sum displayed quantity across all retained levels on
each side. The snapshot owns the interval from its publication timestamp up to
the next client-zero public snapshot, and the final snapshot owns the interval
through the terminal simulation timestamp. An interval is weak when either
side's total displayed quantity is below the registered
`minimum_qualifying_qty`; client-specific snapshots are excluded. The analyzer
must prove ordered snapshots, a positive observed duration, and complete
terminal coverage. The paired SV1D treatment and same-roster mode-off control
must have equal observed duration, and the treatment passes this effect
predicate only when its weak-side duration is strictly smaller than control's.
This is a measured causal development predicate, not a target encoded in the
actor or calendar.

The mechanism is falsified for SV1D if the activation predicate fails, the
treatment remains strictly unvaluatable, the public weak-side duration is not
strictly reduced relative to the matched mode-off control, or any
anti-cheating/kill criterion fires. A negative result is scientifically valid.
No post-outcome threshold or roster selection may rescue it.

The five-minute seed-659 activation probe has a deliberately asymmetric
diagnostic reach. The registered roster starts each supplier with positive CDF
inventory, so a short probe can exercise a bid-only public book and a
missing-ask restoration, but it cannot be represented as a test of the
ask-only/positive-inventory branch. At the configured `40,000,000,000` to
`70,000,000,000` raw base-unit initial inventories (0.4 to 0.7 CDF at the
registered `100,000,000` base precision) and `40,000,000` to `70,000,000` raw
base-unit maximum quote quantities (0.4 to 0.7 CDF per order), exhausting a
supplier's inventory would require approximately 1,000 full fills, well
beyond the probe's bounded decision budget. Absence of an ask-only event in
this probe is therefore diagnostic, not evidence that the branch is activated
or validated. The full development campaign must report the two directions
separately; any claim requiring ask-only activation needs a separately
registered depletion probe or a later campaign cell, with the same finite-risk
and no-replenishment contract.

The audit must also report capital utilization rather than treating finite
configuration alone as proof that capital binds. For each supplier it records
the maximum observed gross CDF balance divided by the configured gross
inventory limit (`max_inventory_utilization`) alongside the raw maximum gross
base/quote balances, position path, quote quantity, and loss budget. This is a
diagnostic: no minimum utilization is silently promoted into the activation
predicate, and a supplier that never approaches a limit must be reported as
capital-unconstrained over that probe horizon.

## Fixed promotion sequence

1. hash this preregistration and the exact SV1D treatment, same-roster
   mode-off control, and no-roster control configs before measurement. Freeze
   seed `659` for the five-minute interval
   `2025-01-01T00:00:00Z` through `2025-01-01T00:05:00Z` (terminal boundary
   exclusive), with no warm-up exclusion, one-second simulation/snapshot/
   automation steps, deterministic scheduler phases, supplier offsets of
   `0s`, `0.5s`, `1s`, and `1.5s`, and the registered liability-hedger phase.
   The immutable launch manifest must then record the actual normalized config
   hashes, source revision, pinned binary hashes, evstream schema/hash
   contract, and the thresholds above before seed 659 is started;
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

## Append-only capital-unit amendment — 2026-09-09

The paragraph above describing the source roster's `40,000,000,000` to
`70,000,000,000` raw base-unit balances as `0.4` to `0.7` CDF was a unit
conversion error. At the registered `100,000,000` base precision those
retained SV1C source balances are `400` to `700` CDF; the source maximum quote
quantities are `0.4` to `0.7` CDF per order. That source roster remains
unchanged and is historical input only.

The SV1D seed-659 activation package is amended before generation to use a
separate, explicitly registered activation roster. It divides each source
supplier's `initial_base_balance`, `initial_quote_balance`, `max_position`,
`max_inventory`, and `max_loss_quote` by exactly ten, preserving integer raw
units; quote quantity, pricing, observation, cadence, and matching mechanics
are unchanged. The resulting initial CDF inventories are `40` to `70` CDF,
with `40` to `70` CDF position limits and `80` to `140` CDF gross-inventory
limits. This makes finite capital reachable within the 150-decision,
five-minute probe budget without changing the retained SV1C source or any
historical R2 configuration.

The activation contract now requires both this horizon-relative capacity
condition and an observed filled quantity at least equal to the registered
`minimum_qualifying_qty` for every treatment supplier. A positive configured
balance or nonzero utilization alone is not activation evidence. The amendment
does not authorize ask-only depletion claims: the seed-659 probe still has
positive initial inventory and reports that direction separately as untested.
