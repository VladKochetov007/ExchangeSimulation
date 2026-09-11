# V2-R2-SV1D activation-contract amendment: tri-arm endpoint

Date: 2026-09-11  
Parent registration: `research/v2-r2-sv1d-one-sided-elastic-successor-preregistration-2026-09-10.md`  
Candidate: `V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY`  
Scientific tree at amendment: `7d4466025b3d47f13191cc86926140d8883cdfe0`

Status: preregistration amendment only. This document does not authorize a
capacity run, simulation, development cell, freeze, or holdout. It does not
alter the R2 predecessor, the registered supplier economics, or any historical
result.

## Scientific boundary and verification tier

R2 remains **NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE**. SV1D is a separately
named development successor and must be judged against the three registered
arms, not against a desired realism score.

The market simulator is a deterministic digital model with a material gap to
real markets. Mechanical evidence and reconstruction are Tier A/B checks;
economic interpretation of a simulated liquidity effect is Tier C. A passing
probe therefore supports only that the registered mechanism changed the
registered simulated endpoint under this contract. It is not evidence of
real-world market-making profitability or universal market survival.

## Registered arms and allowed differences

The probe uses exactly seed `659`, the registered five simulated minutes from
`2025-01-01T00:00:00Z`, one-second simulation/snapshot/automation cadence, and
the three checked-in configurations under:

    research/configs/v2-r2-sv1d-activation/

The arms are:

| arm | roster | one-sided mode | decision evidence |
| --- | --- | --- | --- |
| `treatment` | four finite CDF/USD suppliers per venue | enabled | recorded |
| `mode-off` | the identical four-supplier roster | disabled | recorded |
| `no-roster` | empty successor roster | absent | absent |

The semantic config checker is authoritative for the triad. Treatment and
mode-off may differ only in the registered one-sided option and identity
metadata. Mode-off and no-roster may differ only in identity metadata, the
successor roster, the supplier-decision recorder, and the corresponding
`cdf_elastic_supplier` receipt role. Seed, horizon, venue rules, matching,
calendar, risk, funding, evidence version, historical suppliers, and every
supplier economic field must remain identical where the arm contract requires
identity. Reordering, duplication, unknown roles, omitted required fields, or
hash/arm swaps invalidate the probe.

## Primary endpoint: non-two-sided duration

The primary endpoint is the amount of event-time during which a venue's CDF/USD
public book is not simultaneously usable on both sides:

    non_two_sided_duration
      = bid_only_duration + ask_only_duration + empty_book_duration

For every reconstructed CDF/USD public observation, let `bid_depth` and
`ask_depth` be the checked integer sums of positive displayed quantities on the
respective sides. The interval starts at the observation's simulated time and
ends at the next observation's simulated time, or at the registered five-minute
endpoint. Missing, unordered, or unresolvable observations invalidate the arm;
they are never treated as a zero-depth interval.

The state classification is:

| condition | state | contribution |
| --- | --- | --- |
| `bid_depth > 0` and `ask_depth <= 0` | `bid_only` | bid-only duration |
| `bid_depth <= 0` and `ask_depth > 0` | `ask_only` | ask-only duration |
| `bid_depth <= 0` and `ask_depth <= 0` | `empty` | empty-book duration |
| `bid_depth > 0` and `ask_depth > 0` | `two_sided` | zero |

Durations are computed separately for each of `north`, `central`, and `south`
and then summed across venue-time for the arm total. The normalized diagnostic
is:

    non_two_sided_fraction =
        non_two_sided_duration / (3 * 300 seconds)

The denominator is fixed by the registered probe, not by observed activity.
Per-venue values are retained so a healthy aggregate cannot hide a dead venue.
The existing `one_sided_duration_nano` remains a compatibility diagnostic;
the amended primary endpoint additionally separates empty, bid-only, and
ask-only time.

## Precommitted persistence and terminal predicates

For each venue, retain the maximum uninterrupted interval in any non-two-sided
state. A sequence is uninterrupted only when adjacent valid observations cover
the interval without a missing evidence gap. The fixed persistence threshold is

    max_uninterrupted_non_two_sided_duration = 30 seconds per venue.

Thirty seconds is 10% of the fixed five-minute probe and is selected before any
arm outcome. If treatment exceeds this threshold in any venue, or ends with a
one-sided or empty book in any venue, the probe cannot pass. This is a kill
criterion, not a target used by the supplier.

For a valid directional comparison:

1. all three arms must reach the registered endpoint with complete,
   reconstructible, strict evidence;
2. all three arm identities, config hashes, source/binary identities, event
   ordering, and terminal checkpoints must match their external attestation;
3. treatment must have strict terminal valuation and a two-sided terminal book
   on every venue;
4. treatment's total non-two-sided duration must be strictly lower than both
   mode-off and no-roster; and
5. treatment must satisfy the 30-second per-venue persistence bound.

The strict inequality is evaluated on integer nanoseconds before any display
rounding. Equal values are `NO_DIRECTIONAL_EFFECT`, not a pass. A control that
fails to reach the endpoint is a real recorded non-survival observation but
does not produce a valid three-arm directional score; the probe status is then
`INCOMPLETE_ARM`, not a favorable treatment result.

## Activation and anti-cheating gates

The existing supplier-level activation contract remains binding. Every one of
the twelve supplier/venue instances must have an eligible delayed observation,
an accepted passive order, a fill that changes inventory, an exchange-reconciled
balance/PnL/risk transition, and a later inventory-responsive decision. At
least one decision must use one-sided mode, and at least one order must cancel,
reprice, or withdraw for a registered local/inventory/risk reason.

The treatment is `TREATMENT_NOT_ACTIVATED` if those requirements are not met.
It is `ANTI_CHEATING_REJECTED` if any of the following is observed:

- guaranteed two-sided quoting or forced replenishment;
- capital, inventory, position, or loss absorption beyond the finite checked-in
  limits;
- a hidden instantaneous/global/terminal price anchor or simulator-state
  access;
- a quote that is not bounded by delayed local data, integer ticks, balances,
  inventory, position, observation age, and loss limits;
- no actual fill/PnL exposure or no ability to cancel, reprice, or withdraw;
- supplier concentration above the parent registration's 75% volume/depth
  limits or more than 50% of active-interval depth dominance;
- evidence that is self-attested, reordered, incomplete, or not independently
  renderer/reconstruction verified.

Supplier diagnostics must retain, by venue and role, CDF volume and bid/ask
displayed-depth shares, time-weighted and qualifying-quantity-weighted shares,
inventory, cash, position, PnL, quote lifetime, submit/accept/fill/cancel/
reprice/withdraw counts, observation age/sequence, private-reference path, and
risk-limit state. These are anti-cheating and mechanism-activation diagnostics,
not tunable success targets.

## Fixed result vocabulary

The scorer emits exactly one of these strings, with no implicit conversion to a
boolean:

- `INVALID_EVIDENCE`: hashes, schemas, ordering, reconstruction, provenance,
  or fail-closed checks do not hold;
- `INCOMPLETE_ARM`: an arm does not reach the fixed endpoint or lacks a
  required complete result;
- `TREATMENT_NOT_ACTIVATED`: the treatment lacks the registered mechanism
  activation evidence;
- `ANTI_CHEATING_REJECTED`: a finite-participant or evidence-integrity
  anti-cheating predicate fails;
- `NO_DIRECTIONAL_EFFECT`: evidence is valid but the precommitted strict
  treatment improvement and terminal/persistence predicates do not all hold;
- `DEVELOPMENT_PROBE_PASSES`: all validity, activation, anti-cheating,
  endpoint, terminal, persistence, and strict directional predicates hold.

The scorer must also emit the component durations, per-venue durations and
maximum intervals, terminal book modes, arm completion states, and all failed
predicates. It must be deterministic: identical verified inputs produce
byte-identical canonical output and hash.

## Reproduction, interpretation, and stop rule

The scorer is run only after the raw binary evidence has passed the immutable
manifest/status/renderer/extractor contract. A fresh process must reproduce the
same arm score from the retained canonical evidence. No score may be repaired
by changing a historical trajectory or by silently dropping a failed arm.

`DEVELOPMENT_PROBE_PASSES` permits an independent activation review and,
separately, consideration of the registered 24-hour development cells. It does
not authorize freeze or holdout execution. Holdouts `619`, `631`, and `641`
remain inaccessible until a later explicit freeze authorization.

If the probe does not pass, preserve its complete evidence and classify the
mechanism as inactive, falsified, unsupported, or invalid according to the
failed status. Do not retune the supplier or rescue the result by changing this
amendment after observing the outcome; a material change requires a new named
successor and preregistration.
