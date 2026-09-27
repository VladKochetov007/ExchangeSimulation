# ME-013 — Repeated spot-maker competition (E0)

Stage: **REPORT** for r1. The [source-pinned development report](report.md),
[standard claim/evidence result](result.json), [24-cell machine surface](surface.json)
and [independent review record](../../reviews/me013-r1-result-20260927.md)
close r1 at a valid **primary `NOT_IDENTIFIED`** boundary: all 24 terminal
midpoints are unavailable after early book extinction. This card remains a
pointer, not a new run protocol or confirmation authorization.

Question: how do symmetric versus inventory-aware quote policies change maker inventory risk, fee-net benchmark-relative wealth and liquidity when they compete repeatedly in the same ABC/USD book? Competing explanations include unequal cap/lifecycle behavior, seed depth, RNG coupling, price drift and missing terminal marks.

The [long-run plan](../../longrun-baselines/plan.md), [policy contracts](../../longrun-baselines/policy-contracts.md), [mathematical notes](../../longrun-baselines/mathematical-notes.md), [historical E0 draft](../../longrun-baselines/config-drafts/e0-repeated-spot.md), and prospective [E0 r1 development protocol](protocol.md) identify the current design and its amendment from the older 15-cell draft. The C2 [launch record](../../longrun-baselines/e0-r1-launch-gate-20260927.md) is historical: its first process attempt failed before `World.Run` and any market event. The [C3 technical amendment](../../longrun-baselines/e0-r1-plan-roundtrip-amendment-20260927.md) pins the reviewed replacement plan/binaries and retains that invalid attempt. No `report.md`, `result.json` or economic world existed at this amendment. The r1 seed IDs are assigned for **development only**; no confirmation seed is selected. The [readiness record](../../longrun-baselines/readiness.json) retains the implementation history.

Historical context: [ME-001](../ME-001/report.md) was immediate execution, [ME-002-B](../ME-002-B/report.md) was a one-child first-action gate screen, and [ME-005](../ME-005/report.md) had no qualifying executable arbitrage route. None measures repeated-maker PnL or inventory competition. [R2/SV1D](../../../v2-r2-sv1d-iteration-closeout.md) stays closed; its supplier result is not this policy comparison.

The former first-cell action is complete: C3=`17b9e8a` passed its counted
capacity/evidence preflight, and all 24 registered cells plus two controls
completed. The [maker-local response](../../longrun-baselines/e0-local-response-checkpoint-20260927.md),
[inventory/resting-depth](../../longrun-baselines/e0-risk-series-checkpoint-20260927.md),
and [pending-inclusive envelope](../../longrun-baselines/e0-envelope-checkpoint-20260927.md)
checkpoints remain mechanical history; the last measures working-limit cap
clipping and placement denial, not every reason to withhold liquidity. Earlier
individual increments remain historically **UNREVIEWED DEVELOPMENT**; C3 has
bounded pre-outcome mechanics/design review and r1 has bounded post-result
review, not confirmation or broad market validation. The next economic design,
if pursued under the [owner authorization](../../longrun-baselines/authorization-20260927.md),
must be a separately preregistered local-reference recovery successor; do not
rerun or relabel r1.
