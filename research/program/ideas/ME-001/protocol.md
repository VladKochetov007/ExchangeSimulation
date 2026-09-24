# ME-001 — development protocol lock, revision 1

Status: **PREREGISTERED DEVELOPMENT ONLY; NO WORLD RUN AT LOCK**.
This lock incorporates the [merged design draft](../../../market-ecology-capacity-pilot-protocol-DRAFT.md)
as the economic protocol. This file has precedence only where it states an
explicit prospective clarification or operational amendment. No outcome was
inspected to select these terms. The owner's 2026-09-23 overnight instruction
conditionally authorizes this finite development batch after ME-000 readiness
and a committed protocol; it does not authorize holdouts or ME-002+.

## Question, prediction and causal scope

How do target buy-execution size and fixed-resource maker/random-taker
replacement change executability and all-in target implementation shortfall
for the existing immediate policy on one ABC/USD venue? The strongest
possible conclusion is a **three-seed simulation-internal causal screen of
the bundled population-composition intervention**, not strategy profit,
capital capacity, equilibrium or empirical realism.

The registered prediction, opportunity-separation rule and six directional
shortfall contrasts are exactly draft §§1 and 9. For C+ versus C0, the
median paired delivered five-level ask-depth difference must be positive;
for C− versus C0, negative. If a channel separates opportunity, all three
size-specific target-shortfall contrasts are defined, and none has the
predicted sign, that directional channel is unsupported. Mixed signs are a
response map. Absent depth separation is an identification limitation, not
a rescued or falsified mechanism. Maker count, taker count, client IDs,
timing and fee roles change together; the study cannot isolate one factor.

## Exact cells and candidate

Use the accepted ME-000 executable implementation at
`b25223a4593a9e5129bab27cdbd5f2f8be146bcf` (code tree
`2dd21b21b4b78669ebff6b3ca289a8b59b427704`), plus documentation-only
commits containing this lock. The immutable execution commit is the first
clean commit containing this file; each locked plan records that actual full
commit/tree, Go 1.27.0 toolchain, simulator/analyzer SHA-256 values, effective
world and `execution-pilot-opaque-v3` schema. No source/config change after
plan generation may reuse those plans. A protocol review must inspect the
exact committed lock before execution. Review reports written afterward do
not retroactively alter the candidate.

| Arm | Makers | Delayed random takers | Target quantities (ABC) |
|---|---:|---:|---|
| C0 | 4 | 8 | 0.5, 2, 5 |
| C+ | 6 | 6 | 0.5, 2, 5 |
| C− | 2 | 10 | 0.5, 2, 5 |

Each of the nine arm/size cells uses development seeds `1009`, `1013`, and
`1019`: **27 economic worlds**. These are the draft's proposed candidates,
now selected before any ME-001 outcome. The deterministic selection rule was
to take those three draft candidates in ascending order if the registry and
tracked metadata showed neither an exact ME-001 run nor a reserved holdout.
A 2026-09-23 provenance-only registry/path check found no such conflict; it
did not inspect external/raw outcomes or assert that these integers have
never appeared in other research. Any newly discovered conflict stops this
revision and requires a prospective amended lock before seed replacement.
Historical reserved holdouts, including 619/631/641, are not used.

The effective world is whatever `executionpilot.Lock` reconstructs from
`executionlab.NewSim`; its entire typed contract is authoritative over prose.
It has one parent (client 13), twelve background accounts, equal initial
endowments of 100,000 ABC and USD 100,000,000 per account, maker fee 0,
focal/random-taker fee 5 bp in USD, deterministic price-time matching,
one ABC/USD spot book, no borrowing/derivatives, and fixed 4-second logical
horizon (1-second warm-up). The parent polls every 1 ms, decides no earlier
than 1 second, and has fixed 1 ms inbound market-data, outbound request and
inbound response delay. Background takers have 2 ms delay and the draft's
25 ms decision interval. Maker parameters and realized indexed cadences are
fixed by draft §2 and the effective plan. The policy, economics, clocks and
latencies are not retuned between cells. Increasing target changes the actual
market request quantity, not merely an idle capital denominator.

## Endpoint, eligibility and missingness

Primary endpoint: independently reconstructed all-in target implementation
shortfall in bp, exactly draft §§7–8. For buy fills, the analyzer uses checked
fixed-point `Σ(q×p/B)`, quote-asset fees, decision midpoint and a separately
identified terminal two-sided midpoint for any unfilled obligation. The
terminal mark does not imply execution. Completion must be 100% and target
shortfall at most 10 bp for a cell to pass the declared admissibility mandate.
All 27 assignments remain in the table, including zero-fill, rejection,
partial fill, unpriceable terminal, invalid evidence and process failure.
No-trade is not silently dropped or called profitable.

For the fixed Immediate policy, a complete 4,000-tick clock with no eligible
delivered two-sided state is `NO_OPPORTUNITY`. A valid eligible tick without
the required immediate send is an **implementation/evidence defect**, not
`VALID_INACTIVE`: this prospectively resolves the draft's generic class
against the actual policy contract. `REJECTED`, `ACCEPTED_UNFILLED`,
`PARTIALLY_FILLED`, and `FULLY_FILLED` are distinct reconstructed outcomes.
An active world with valid evidence but unavailable terminal mark is retained
as `VALID_ACTIVE` with `shortfall_unavailable` and admissibility
`NOT_ASSESSABLE`; it is not assigned zero cost or a mandate failure. A
validly priced active world exceeding 10 bp or missing completion is an
`ECONOMIC_FAILURE`. `INVALID_EVIDENCE` and `FAILED_PROCESS` remain separate.
The exact raw outcome status and each orthogonal validity/valuation/mandate
field must accompany any top-level summary label.

Paired shortfall is defined only if all three matched seed pairs have valid,
defined target-shortfall outcomes in both arms. Otherwise report
`NOT_DEFINED` for that scale/contrast, with the all-assigned availability
and admissibility map instead; never compute a survivor-only estimate.
For defined contrasts publish each paired difference, median and range. No
event-row independence, asymptotic p-value, equivalence claim, or tail-risk
claim is licensed by three seeds. The exact signed-price domain is the
positive ABC/USD spot book; undefined prices remain unavailable.

Supporting endpoints are draft §7's completion/quantity, filled-only and
target shortfall, fees, realized fill timing, residual, delivered touch and
five-level depth, decision spread, mechanical sweep, and cash/ABC ledger.
Background fill share is optional only if independently reconstructible and
may not enter the primary verdict. Markout is not causal impact; ledger
payments are not automatically losses or strategy profitability.

## Controls, resources and stop rules

Before the 27-cell matrix, run **two fresh-process technical duplicates**
of C0/S2/1009, one with `GOMAXPROCS=1` and one with `GOMAXPROCS=7`. This
prospectively replaces draft `14` with `7` because the host has eight CPUs
and the owner's headroom constraint applies. Both use identical binaries,
plan bytes, seed and economic config; only the declared process parallelism
differs. Require byte-identical canonical evidence execution/file hashes and
identical reconstructed outcomes. Controls are not extra independent seeds.
Evidence-on/off fixture neutrality and corruption tests were completed in
ME-000; they do not substitute for these fresh-process controls.

After controls pass, execute S1 for all arms/seeds, then S2, then S3, with
stable arm order C0/C+/C− and ascending seed. If all nine S1 cells fail
mechanically, stop under draft §13. Do not stop because an economic result is
unfavorable. If one cell's infrastructure/evidence fails, stop its matched
block and diagnose before interpretation; a source correction requires a
new exact candidate and rerun of every affected comparable cell, preserving
old attempts. No selective rerun of valid negative seeds.

Run at most one world process at a time with `GOMAXPROCS≤7`. Hard ceilings:
29 executions including controls, four simulated seconds per world, 30 s
per-world timeout, 15 min total batch wall time, 4 GiB peak process RSS and
1 GiB retained evidence/reports. Use the first control's actual resource
measurement to extrapolate the full batch; stop before the second control
or economic matrix if any ceiling cannot be met. Do not lower evidence or
delete protected history to pass. Output paths are fresh, exclusive and
external to the clean source checkout; incomplete attempts are preserved
and never interpreted as completed worlds. Real process exit codes and
evidence manifests determine completion. No new world starts after
2026-09-24T06:15Z; all work stops by 07:00Z.

## Provenance, review and later stages

The pinned `mepilot plan/run` and `mepilotanalyze` binaries and each raw/typed
plan, run manifest, binary evidence hash and independently reconstructed
result are required for every cell. Keep the source checkout clean while
planning/running/analyzing. Any later Go response-surface code must implement
the formulas above against the retained per-world analyzer results and cite
its own revision; it cannot change the registered endpoint or retrospectively
alter a simulator trajectory. Independent evidence and causal/statistical
claim reviews are required before a final ME-001 report is treated as
accepted. Provider failure is not approval.

This is DEVELOPMENT screening only. No unseen confirmation partition,
empirical corridor, capital adaptation, restoration arm, latency/geography
intervention, multi-venue/derivative study, or R2/SV1D revival is part of
these 29 executions. The corresponding modules are N/A because they do not
identify the one-venue immediate-execution composition/quantity question.
Any later confirmation or ME-002+ study needs a separate prospective owner
decision. Valid nulls and losses are retained without parameter rescue.
