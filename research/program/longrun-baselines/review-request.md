# External read-only design review request — E0/E2 planning package

The fresh Sol-6 medium reviewer request returned `agent thread limit reached`; **execution UNAVAILABLE, verdict NOT_ISSUED**. This file is a bounded handoff, not an approval or an instruction to run anything. Review the final published planning-branch commit (record its full SHA before starting) against `main` source baseline `ef61b4cb6e0124aa797f281e379ab143e847646e`; do not infer review of later executable/config changes.

Read only [plan.md](plan.md), [policy-contracts.md](policy-contracts.md), [mathematical-notes.md](mathematical-notes.md), [E0 draft](config-drafts/e0-repeated-spot.md), [E2 draft](config-drafts/e2-two-venue-funding.md), [readiness.json](readiness.json), [ME-013](../ideas/ME-013/idea.md), [ME-014](../ideas/ME-014/idea.md), and the linked exact source/test files needed to adjudicate them. Existing ME-001/002/003/002-B/005 reports are historical context; do not inspect untouched holdouts or run economic worlds. Inspect `git diff ef61b4c..HEAD -- research/program research/README.md` in a clean checkout of the reviewed branch.

Bounded questions:

1. Does E0 actually isolate pure versus AS-style quote control once endowments, worst-case cap, quote lifecycle, delivered observations and initial seed are matched? Identify any hidden nonmatching mechanic in source that the readiness list missed.
2. Are E0 account wealth and passive-inventory benchmark dimensionally and economically correct, including fee asset and unavailable terminal mark? Is 10+45 minutes meaningful for repeated behavior without claiming stationarity?
3. Does the E2 proposed compressed rate contract unambiguously specify sample cutoff, publication, same-time ordering, phase, settlement eligibility, missing-window rule, precision and remainder? Identify unsupported present-source claims and any unavoidable economic ambiguity.
4. Can the proposed venue-local term desk encounter several actual payments and exit without a shared wallet, global oracle or guaranteed opportunity? Are OFF/ON accounts and RNG coupling treated honestly?
5. Do the reduced equations and game-theory section label closure assumptions, non-stationarity and untested claims? Do the registry and navigation preserve historical verdicts and authorization boundaries?

Return concrete file/line findings with severity and a scoped conclusion: **PLAN_USABLE**, **PLAN_USABLE_AFTER_SPECIFIC_REVISIONS**, or **PLAN_NOT_USABLE**. A source-only design finding is not an executable candidate review. No reviewer may edit this branch, run worlds, or convert a planning draft into a locked protocol. A later implementation needs its own prospective source/config/tests/evidence review.
