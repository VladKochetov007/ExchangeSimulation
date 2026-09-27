# ME-015 — Finite actor-local reference for repeated spot makers

Stage: **PLAN**. This is a new E0 development successor, not a repair, rescore or confirmation of [ME-013 r1](../ME-013/report.md). No ME-015 economic world has run.

Question: when a maker's latest delayed ABC/USD snapshot becomes one-sided or empty, can it use a recently **delivered** two-sided snapshot for a short, finite period to make an ordinary risk-bounded quote decision? Does that change observed two-sided market time without imposing a price path or a standing liquidity obligation?

The [draft protocol](protocol.md) specifies a concurrent reference-off/on contrast. Its strongest prospective claim is a **simulation-internal causal development contrast** for this exact roster. The registered primary outcome is two-sided public-book time, not maker profit. A later P-versus-A wealth comparison is conditional on the original strict terminal-mark rule and is not rescued by cash-only or stale-price scoring.

The local reference is an actor belief formed from delayed public data. It expires; no qualifying prior snapshot means no fallback. Ordinary inventory limits, balances, cancel/replace waiting, post-only admission and policy abstention still apply. No simulator state, shared fair value, automatic capitalization or forced two-sided quote is added. A zero or negative treatment effect is a valid result. Self-referential price persistence and price drift remain explicit failure modes, not evidence of realism.

Dependencies: accepted [ME-013 r1 boundary](../ME-013/report.md), the existing recurring maker in `simulations/repeatedspot/recurring_maker.go`, strict delayed-observation replay in `experiment/repeatedspot/replay.go`, and the existing immutable-plan/evidence workflow. Neither historical R2/SV1D nor ME-001/002/003/005 is reopened.

Next action: review this pre-outcome design; implement only the source/evidence extension and adversarial fixtures needed to distinguish an actual cached-reference decision from a future, fabricated or expired reference. Pin a clean candidate and review it before deciding whether the fixed development matrix may launch.
