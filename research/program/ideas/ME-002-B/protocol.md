# ME-002-B — first-action cadence × network development protocol r1

Status: **PROSPECTIVE DRAFT; NO ECONOMIC WORLD AUTHORIZED BY THIS FILE ALONE**.
The owner authorized this bounded study on 2026-09-26, conditional on readiness,
independent review, committed preregistration and resource checks. This is a
new development study, not a rescore or additional ME-002 cell. The reviewed
[idea](idea.md) fixes processing at zero. Its Sol-6 medium plan acceptance
covered the idea only; it did not approve this executable candidate or outcomes.

## Question, estimand and competing explanations

For the same finite one-child immediate ABC/USD BUY policy and C0 counterparties,
does changing the policy's first-action poll interval from 1 to 80 ms change
the realized first order time and assigned 5-ABC filled fraction, and does it
modify the response to a fixed 1 versus 90 ms directed network assignment?
The strongest possible conclusion is a **simulation-internal causal
development response map for this fixed gate and phase**. It cannot establish
general high-frequency strategy behavior, pure network-speed returns,
profitability, a capital-capacity frontier or empirical latency effects.

The mechanism is publication → delayed feed receipt → cached two-sided quote
→ first ticker at/after the one-second threshold → one request → delayed
venue arrival → exchange execution → delayed response. A coarser tick might
move arrival across a relevant book state, but the effect need not be
monotone or favorable. Competing explanations include changed selected
information, 100-ms periodic snapshots, one-sided cache retention, gate/poll
phase, endogenous book movement after the request and a filled-fraction
ceiling. [The pre-outcome source trace](timing-readiness-20260926.md) defines
which timestamps actually exist. Network and processing are synthetic logical
time; no wall-clock sleep or city geography is modeled.

The primary endpoint for **every assigned valid world** is
`Y = exchange-filled ABC base atoms / 500000000 assigned base atoms`.
Valid no-send, rejected or accepted-unfilled worlds have `Y=0`; partial fills
retain their fraction. Invalid/incomplete evidence is `UNASSESSABLE`, not zero.
No full-completion or profitability requirement is imposed. For each seed,
write the four raw Y values and:

```text
poll effect at network n = Y(80ms,n) - Y(1ms,n)
network effect at poll p = Y(p,90ms) - Y(p,1ms)
interaction = [Y(80ms,90ms)-Y(1ms,90ms)]
            - [Y(80ms,1ms)-Y(1ms,1ms)]
```

Report each seed, the three-seed median and observed range. Three development
seeds support a screen only; no p-value, equivalence margin, population
threshold or tail estimate is registered. Equal seed labels provide matched
initial conditions but do not by themselves prove coupled realized background
random streams after the intervention.

## Exact fixed world and matrix

The source candidate C is `404091350440daf8ee0d1299730a932b1bc105af`
(tree `0e947767e723f48c872b1945136fa9ccad57f61e`). The research-skill
tree at C is `60b91e73f1000a285e003bed122648636a737b2d`.
Go 1.27.0 binary digests and the governing protocol P will be recorded in
a separate lock **before** any economic outcome. Plans bind source, both executable digests, toolchain,
effective world, typed-plan digest and raw-plan digest. Builds and worlds use
a clean C checkout; later report/diagnostic commits do not relabel C.

One ABC/USD price-time venue contains four finite makers, eight finite random
takers and focal client 13. The focal policy is `Immediate`, one market-GTC
BUY child, 5 ABC target, `DecisionAfter=1 s`, fixed initial ABC/USD balances,
5-bp quote-denominated taker fee and zero maker fee. Background accounts,
capital, clocks, matching, fee schedule, RNG initialization and venue
publication policy remain those of `executionlab.DefaultSimConfig` at C.
Only the focal `PollInterval` and its symmetric directed feed/request/response
network assignment vary. Processing delay is **zero in every cell**; the old
ME-002 0/120-ms processing result is historical motivation, not another
factor. The 1-ms simulation runner step and four-simulated-second horizon
are fixed; terminal responses and balances must drain. No other target size,
instruction, actor roster, latency layer or recurring policy is included.

| Arm | Poll interval | Feed/request/response | First gate-eligible tick if quote usable |
|---|---:|---:|---:|
| P1-N1 | 1 ms | 1/1/1 ms | 1.000 s |
| P80-N1 | 80 ms | 1/1/1 ms | 1.040 s |
| P1-N90 | 1 ms | 90/90/90 ms | 1.000 s |
| P80-N90 | 80 ms | 90/90/90 ms | 1.040 s |

For each arm, run development seeds `13001`, `13011`, `13017` in ascending
order: **12 economic worlds**. These labels were prospectively chosen after a
bounded search of the current registry/config metadata found no use or
reserved-holdout collision; they are not untouched confirmation conditions.
The two technical controls independently repeat P1-N1/13001 at
`GOMAXPROCS=1` and `7` before the economic matrix. Controls are not extra
economic observations. Within the matrix run arm order as in the table and
seeds ascending. Do not fill the owner's larger 24-world ceiling with new
arms after viewing results.

## Secondary timing and opportunity diagnostics

From independently reconstructed evidence report publication/receipt,
first usable cache time, selected snapshot sequence and age, scheduled
first gate-eligible tick, actual decision/send/venue arrival, admission,
exchange fills, and response receipt. `gate_wait = max(0, gate-first_usable)`;
`gate_adjusted_poll_wait = decision_at-max(gate,first_usable)`. No-action
decision-dependent intervals are null/censored, not zero. Compare selected
information, action/arrival time, observed public quote state and execution
as separate questions. The primary Y does not require a continuous
opportunity lifetime. A sampled public two-sided ask-depth condition is only
a snapshot proxy; if complete lifetime or exact pre-match book cannot be
reconstructed, report `NOT_RECONSTRUCTIBLE`, not a synthetic ratio.

All cash/ABC deltas, filled quantity, charged quote fees and terminal balances
must reconcile. Target shortfall is secondary and defined only when both
decision and terminal marks exist; a cheap unfilled obligation is not
execution. The spot price domain here is positive, but unavailable marks
remain explicit. Fills use exchange timestamps; actor receipts may arrive
later without becoming new executions. A later first action encounters a
later endogenous state by design, so the total intervention cannot be
described as a pure transport or computation cost.

## Readiness, review, resource and stop rules

Before lock, focused fixtures must prove 1.000/1.040-s first action and
neighboring gate phase; the unchanged 1-ms replay and new replay must agree
on the same fixture. A versioned cadence plan must reject any cell outside
this matrix. The evidence walker must fail closed on missing/duplicate/
misphased ticks, processing-before-tick errors, future-state reads, changed
source or request/response identity, incomplete output, wrong fees/ledger
and unavailable required terminal evidence. Run clean full `make test`,
`go vet ./...`, targeted race, evidence-neutrality/fresh-process controls,
skill validation and whitespace checks on the exact source candidate.

Two fresh independent Sol-6 medium reviews must assess (A) timing/evidence/
execution correctness and (B) the prospective causal/statistical scope.
Both must accept the exact C/P package before any economic cell. A service
error is `UNAVAILABLE / NOT_ISSUED`; a substantive rejection requires a
prospective fix, not a different reviewer. Post-result checks revisit the
same bounded scopes without converting development into confirmation.

Run one process at a time with at most seven Go workers, finite 4-GiB memory
and zero-swap cgroup and CPU quota at most 700%. Verify child placement,
limit enforcement, exit code and stream completion. Owner's later operational
allowance permits up to **20 GiB** of fresh logs/staging and **60 minutes**
of batch wall time, with a 2-minute per-world timeout; these are ceilings,
not targets. Before each cell require at least 8 GiB available host RAM and
25 GiB free disk; stop earlier if projected retained/staging volume violates
the floor, a cgroup cap, wall cap, 20-GiB cap or evidence contract. Never
delete protected evidence to continue. Use fresh exclusive external paths.

Retain valid loss, null, no-send, rejection and partial outcomes. Do not
selectively rerun an inconvenient cell. An analyzer-only defect may rescore
immutable evidence under a versioned correction for **all affected cells**;
a simulator semantic change invalidates comparable trajectories and needs
a prospective amendment within the owner's unchanged total run ceiling.
No confirmation seeds, historical reserved holdouts, ME-002/003/005 reruns,
ME-006/007 or economic retuning are licensed. The separately authorized
ME-005 offline funnel is exploratory and cannot change this protocol.
