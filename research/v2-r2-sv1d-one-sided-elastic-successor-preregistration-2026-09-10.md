# V2-R2-SV1D one-sided elastic-liquidity successor preregistration

Date: 2026-09-10
Candidate: `V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY`
Predecessors: closed R2; rejected `V2-R2-SV1C-STRICT-RISK-CDF-LIQUIDITY`

Status: preregistration only. This document authorizes no simulation, capacity
probe, development cell, freeze, or holdout.

## Scientific boundary

The R2 candidate remains **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**.
SV1C remains archived as a negative activation result. The separate SV1D
attempt on the performance-successor branch is **INVALID ARM EVIDENCE /
NON-ADVANCING GATE**: all arms stopped at an early strict option-risk failure,
none reached its endpoint, and no one-sided supplier decision was observed.
Those results and all historical JSON/binary artifacts remain unchanged.

SV1D is a separately named successor. It preserves the accepted R2 calendar,
matching, strict valuation, cross-margin epoch, actor-information, and
`evstream_v3` contracts. The eight historical ABC/USD suppliers are unchanged.
The new CDF/USD roster is opt-in and is absent from historical R2 configs.

## Mechanism hypothesis

A finite, inventory-sensitive CDF/USD supplier that can quote one selected side
against a delayed local one-sided touch may shorten persistent one-sided book
intervals often enough for the full ecology to remain strictly valuatable. The
effect, if present, must arise from ordinary passive orders, finite balance
sheet, inventory/PnL exposure, and delayed local information—not from a
prescribed price, spread, volume, or survival target.

## Registered participant contract

There are four independently funded CDF/USD suppliers on each of the three
venues. The treatment enables `quote_on_one_sided_local_book`; the same-roster
mode-off control uses the identical roster and disables that option; the
no-roster control has an empty successor roster. All other economic inputs are
identical between paired arms.

The normalized roster is fixed as follows (raw integer units):

| role | base balance | quote balance | half-life | elasticity | max position | max inventory | max quote | loss budget |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `cdf_elastic_supplier_1` | 4,000,000,000 | 18,000,000,000 | 3h | 12,000,000,000 | 4,000,000,000 | 8,000,000,000 | 40,000,000 | 3,000,000,000 |
| `cdf_elastic_supplier_2` | 5,000,000,000 | 21,000,000,000 | 4h | 15,000,000,000 | 5,000,000,000 | 10,000,000,000 | 50,000,000 | 3,600,000,000 |
| `cdf_elastic_supplier_3` | 6,000,000,000 | 24,000,000,000 | 5h | 18,000,000,000 | 6,000,000,000 | 12,000,000,000 | 60,000,000 | 4,200,000,000 |
| `cdf_elastic_supplier_4` | 7,000,000,000 | 27,000,000,000 | 6h | 21,000,000,000 | 7,000,000,000 | 14,000,000,000 | 70,000,000 | 4,800,000,000 |

All four use base precision 100,000,000, quote precision 100,000,
reference price 300,000,000, base holding zero, two-second decisions with
phase offsets 0s/0.5s/1s/1.5s, 60s maximum observation age, 100,000 minimum
executable quantity, 1,000,000 minimum qualifying quantity, the registered
CDF/USD tick of 100,000, and a 5-bps maker fee. Their initial balances,
position limits, gross-inventory limits, quote limits, and loss budgets are
finite and immutable for this candidate.

Each supplier may read only its delayed CDF/USD snapshot and its own private
state. It has no exchange object, global index, remote venue, undelayed book,
terminal-state, or simulator-state access. A private reference moves toward a
delayed local anchor with the registered half-life; it is never reset from a
global or terminal value.

With a valid two-sided local book, the supplier retains the existing midpoint
and inventory-target rule. With exactly one valid side, the supplier may:

1. use that local touch as its anchor;
2. update its private reference toward that anchor;
3. compute the inventory target from the anchor and finite position limits;
4. quote the target-selected present side at the observed touch, or quote the
   missing side at the checked integer-tick midpoint of the private reference
   and present touch, one tick beyond the present touch in the missing-side
   direction; and
5. submit at most one ordinary passive order bounded by cash, inventory,
   position, quantity, observation age, and loss limits.

If both sides are absent, the observation is stale, the book is locked/crossed,
the price arithmetic is invalid, the target gap is zero, or marked equity is
unavailable, the supplier waits, cancels, or withdraws. It does not fabricate a
mark, use its entry price, force a two-sided quote, replenish capital, or
replace a withdrawn order. After a one-sided quote closes, a strictly later
local snapshot is required before another one-sided submission.

The supplier's loss-budget mark is also fail-closed: midpoint for two-sided
data; bid for positive gross inventory on bid-only data; unavailable when
positive gross inventory has only an ask. No unavailable mark becomes zero,
the private reference, or an implicit external price.

## Activation and anti-cheating criteria

All twelve supplier/venue instances must have eligible delayed observations,
an accepted passive order, a fill that changes inventory, an ordinary
exchange-reconciled balance/PnL/risk transition, and a later
inventory-responsive decision. At least one decision must use the one-sided
mode, and at least one order must cancel, reprice, or withdraw because of a
local observation, inventory, risk, or unavailable-side condition. Strict
terminal valuation and complete arm artifacts are mandatory.

The candidate is rejected if survival is primarily produced by guaranteed
liquidity, forced two-sided quoting, forced replenishment, effectively
unlimited capital, a hidden external anchor, direct simulator access, or
unbounded loss absorption. A supplier that never bears fill/PnL risk or cannot
withdraw is not an activation of this hypothesis.

For every venue and supplier, retain volume and displayed-depth share,
inventory, cash, position, PnL, loss budget, quote lifetime, submit/accept/fill/
cancel/reprice/withdraw counts, observation age and sequence, reference path,
and risk-limit state. Report supplier share separately for bid and ask depth,
time-weighted and qualifying-quantity-weighted. A supplier share above 75% of
CDF volume, or above 75% of either side's displayed depth for more than half of
active intervals in any venue, kills the candidate. Persistent one-sided books,
strict valuation failure, missing supplier risk/PnL transitions, or unbounded
replenishment also kill it.

The analyzer must reconstruct public snapshots in global event order and must
join each decision to its delayed observation, not to decision-time state. A
qualifying restoration requires an accepted live supplier order with at least
1,000,000 base units, one to twenty registered ticks from the observed touch,
and a later public snapshot showing the missing side at the same threshold.
Supplier-removal depth is a separate counterfactual diagnostic; incomplete
reconstruction is invalid evidence, not a favorable result.

## Registered probe and promotion sequence

The first development-only probe is seed 659 over exactly five simulated
minutes from `2025-01-01T00:00:00Z`, with one-second simulation/snapshot/
automation cadence, 60-second scheduled risk cadence, deterministic phases,
full `evstream_v3` evidence, and treatment/mode-off/no-roster arms. The seed,
configs, source revision, binary hashes, resource policy, and evidence hashes
must be captured before launch. Holdouts `619`, `631`, and `641` are forbidden.

The prior SV1D invalid gate may be repaired only at the producer/fixture layer
while preserving this seed, configuration, strict-risk semantics, roster,
warm-up, and event ordering. Changing risk ordering, fallback valuation,
roster economics, seed, or warm-up creates a new amendment and stops this
candidate.

Before the probe: implement the opt-in behavior and focused regressions for
two-sided parity, bid-only/ask-only arithmetic, tick boundaries, overflow,
stale observations, withdrawal, finite accounting, delayed provenance, and
evidence neutrality; complete binary evidence contract tests; obtain one fresh
independent Sol-xhigh review of the exact tree; build clean pinned Go 1.27
binaries; and obtain a valid outcome-neutral binary capacity attestation.

After the probe: obtain independent activation review. Only if every
activation and anti-cheating predicate passes may the registered development
cells proceed. Extract and review development evidence before a separate
freeze authorization. No holdout is inspected or consumed before that
authorization.

This preregistration does not rescue R2, SV1C, or the invalid SV1D trajectory,
and it does not authorize offline repair of any historical result.
