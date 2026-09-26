# ME-013 — Repeated spot-maker competition (E0)

Stage: **PLAN**. Claim sought later: simulation-internal **CAUSAL** development contrast between finite, equal-resource pure and AS-style maker rosters under recurring demand. This card is a pointer, not a run protocol or authorization.

Question: how do symmetric versus inventory-aware quote policies change maker inventory risk, fee-net benchmark-relative wealth and liquidity when they compete repeatedly in the same ABC/USD book? Competing explanations include unequal cap/lifecycle behavior, seed depth, RNG coupling, price drift and missing terminal marks.

The [long-run plan](../../longrun-baselines/plan.md), [policy contracts](../../longrun-baselines/policy-contracts.md), [mathematical notes](../../longrun-baselines/mathematical-notes.md) and [E0 draft](../../longrun-baselines/config-drafts/e0-repeated-spot.md) are the current authoritative design references. No locked `protocol.md`, `report.md`, `result.json`, seed IDs or economic worlds exist for ME-013. The [readiness record](../../longrun-baselines/readiness.json) enumerates bounded missing tasks.

Historical context: [ME-001](../ME-001/report.md) was immediate execution, [ME-002-B](../ME-002-B/report.md) was a one-child first-action gate screen, and [ME-005](../ME-005/report.md) had no qualifying executable arbitrage route. None measures repeated-maker PnL or inventory competition. [R2/SV1D](../../../v2-r2-sv1d-iteration-closeout.md) stays closed; its supplier result is not this policy comparison.

Current next action: owner decision on **PR1 only** (one-book composable roster, finite opening seed and integrated fixtures). Economic source, protocol review and a separate run authorization are needed before any development world.
