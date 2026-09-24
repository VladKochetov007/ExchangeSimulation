# ME-003 — limit IOC versus limit FOK development protocol

Status: **PROSPECTIVE DESIGN LOCKED / EXECUTION NOT YET RELEASED / NO ME-003
DEVELOPMENT WORLD RUN**. This contract is locked before the first ME-003
development outcome under the owner's bounded 2026-09-23 continuation. The
source implementation through `3012682` requires fresh prospective review of
this exact protocol tree, clean tests and fresh-process controls before any
economic cell. A lock is not a positive reviewer verdict or run release. It
keeps the same immediate finite buy mandate as
ME-001/002 and changes only the child order's time-in-force. Passive GTC,
post-only, market, and adaptive deadline variants are outside this comparison.

## Question, mechanism and allowed claim

At one ABC/USD venue with the same C0=4-maker/8-random-taker background,
focal actor, initial resources, latency, fixed price cap and one-shot
decision, how does a limit IOC child versus an otherwise identical limit FOK
child change execution completion and non-fill risk? The strongest possible
claim is a three-seed **simulation-internal causal development response map**
for this exact instruction contrast. No alpha, trading-PnL, adaptive-policy,
capital-capacity, equilibrium or empirical-exchange claim follows.

IOC may fill a subset of the available legal ask quantity at/below the cap,
then cancel the remainder. FOK preflights the full order against its venue
state and rejects without a fill if it cannot complete. Under complete
reachable depth the two may have the same fill; under partial reachable depth
FOK may avoid inventory but leave the whole mandate unmet. That is a
different objective trade-off, not proof that one instruction is universally
better. An FOK rejection cannot be called low-cost success merely because
the unfilled target receives a terminal midpoint mark.

## Fixed population, cap, clock and matrix

Use `executionlab`'s existing immediate BUY policy and C0 background,
single ABC/USD price-time venue, focal ID 13, 5-bp quote-asset taker fee,
zero maker fee, 1-ms symmetric focal feed/request/response delay, 1-ms poll,
earliest decision at 1 second and 4-second terminal horizon. All actor
endowments and background indexed clocks remain those of the ME-001 C0
world. Focal actor sends at most one child and never reissues or modifies it.
The actor has finite initial balances and no borrow/liability objective in
this spot-only pilot. The endpoint is mandate execution, not account PnL.

The buy limit cap is **50,100 USD/ABC**, encoded as `5,010,000,000` quote
precision units. This is the structural bootstrap price 50,000 USD plus ten
10-USD ticks, chosen from the venue's declared tick and five-level maker
ladder before any ME-003 outcome. It is an absolute cap, not a mutable
reference to the treatment book or a price-convergence target. It may be
unmarketable in a particular world; that is an assigned outcome, not a
reason to move the cap post hoc. IOC and FOK use the same cap and target.

Prospective development matrix: `IOC/FOK × target {0.5, 5} ABC × seeds
{14001, 14011, 14017}` = **12 economic worlds**, plus two fresh-process
IOC/0.5/14001 technical controls (`GOMAXPROCS=1` and `7`) = 14 maximum
executions. These seed labels were absent from the tracked program/protocol
metadata search before outcomes; that search cannot establish what an
unseen private machine contains. None is a registered historical holdout.
Run target 0.5 then 5; within each target IOC then FOK; seeds ascending.
Do not add extra worlds to manufacture a partial-depth episode.

## Registered estimand, status and opportunity denominator

Primary all-assigned outcome is `Y = venue-filled ABC / assigned target ABC`
for each evidence-valid world. A valid no-send, admission rejection or
accepted-unfilled order scores zero. Partial IOC fills remain fractional;
invalid/incomplete evidence is `UNASSESSABLE`, never zero or silently
excluded. The paired contrast is `Y_IOC - Y_FOK` by seed and target. Report
each assigned cell's process/evidence status and each pair. Define the
three-seed median and observed range only if **all three pairs at that
target** have valid evidence; otherwise leave that aggregate `UNDEFINED`,
show every available individual outcome, and identify the missing pair.
Do not use a convenient complete-case median. No p-value or equivalence
claim is made. Full-completion indicator is secondary and shown for every
assigned valid world. Three seed pairs are the uncertainty units, not orders
or book events.

Supporting outcomes: request sent, admission/rejection reason, matched fills,
first/last venue fill time, cancellation reason and residual, quote fees,
terminal two-sided mark availability, and the ME-001 all-in *target* shortfall
only where a positive decision midpoint and terminal two-sided mark are
both defined. A valid no-send has no decision midpoint, so neither shortfall
is defined. Filled shortfall requires a decision midpoint; a full fill can
have filled shortfall without a residual terminal mark, but target shortfall
under this analyzer contract remains undefined if that mark is unavailable.
Report filled shortfall separately. For a rejected FOK with valid marks,
target shortfall marks the entire residual at terminal midpoint; it is not
purchased inventory or a profit. A missing mark never makes the primary
filled-fraction result invalid by itself. Report `UNAVAILABLE` with reason
for any undefined secondary statistic; do not impute it.

Opportunity funnel: venue public snapshot publication -> focal delivery ->
selected positive two-sided local quote -> displayed ask quantity at/below
the fixed cap -> order send -> venue arrival -> IOC admission/partial/full
or FOK rejection/full -> response receipt -> terminal state. The selected
snapshot is the last *processed* positive two-sided focal quote at decision;
the actor retains it if a later one-sided message arrives. Sum visible ask
quantity over **all ask levels emitted in that selected public snapshot**
whose prices are ≤50,100 USD, rather than assuming five levels are always
present. Record the latest delivered message and whether retention followed
a one-sided message. This cap-bounded quantity is an **actor-observed sampled
proxy**, not continuous or guaranteed executable depth at later arrival.
FOK's explicit `FOK_NOT_FILLED` reason can arise from matcher preview or
fee/spot-plan pruning, so the reason alone does **not** identify depth
failure. Classify all other admission rejections separately; do not infer
exact at-arrival opportunity size or an instruction-caused price impact
without separately validated venue-state/preflight reconstruction. IOC's
actual fills/cancel and FOK's rejection are distinct outcomes, not
interchangeable estimates of unlogged pre-arrival depth. If no comparable
partial-depth opportunity is evidenced, classify mechanism activation as
limited rather than tuning cap, seed or horizon.

Same numeric seed is a pairing label, not automatic proof of aligned
endogenous shocks. The order payload differs before arrival and may change
later book/actor responses; those are possible treatment effects. Compare
pre-arrival clock, background config, decision snapshot and selected quote
identities where supported, and report any divergence rather than forcing
paired trajectories to be identical.

## Readiness, independent reconstruction and falsifiers

Before a world, extend the externally configurable parent order instruction
in `executionlab` without changing default market/GTC economics or the
historical ME-001/002 evidence schemas. Bind the effective order type, TIF
and fixed price cap in a new pinned typed plan and a new versioned evidence
schema. The analyzer must independently join locked intent to send,
admission, FOK rejection or IOC cancellation, exchange-time fills, delayed
receipts, actual quote-asset fees, ABC/USD ledger deltas and terminal mark.
Require admitted quantity = filled + cancelled residual for IOC; require no
execution, reservation leak or cancelled live order for rejected FOK. Verify
the actor report against reconstructed evidence, never use it as the oracle.
FOK/IOC mechanics must not be inferred from a market/GTC-only replay.

Focused fixtures before promotion: full reachable depth; partial reachable
depth with IOC fill/cancel and FOK rejection; no reachable depth; cap exactly
at best ask; cap one tick below; price movement between local snapshot and
venue arrival; FOK rejection after cost/fee preflight; fill/cancel response
reordering; terminal mark missing; balance/reservation release. Mutation
tests must fail closed on wrong TIF, cap, request ID, order ID, fill timestamp,
fee, cancel reason/quantity, duplicate/missing response, ledger delta,
terminal book and raw ordering, including rehashed adversarial evidence.
Preserve the earlier legitimate exchange-fill/later-receipt distinction.

Readiness fixture map at lock: `instruction_evidence_test.go` covers full,
partial, no-reachable-ask, exact-selected-ask and one-tick-below cap,
cancelled/rejected, unavailable terminal mark, queued fill receipt and
rehashed corruption. `tests/me003_instruction_fixture_test.go` separately
forces a local/arrival ask reprice. `exchange/order_admission_regression_test.go`
`TestSpotPlanRejectsFOKWithoutCancellingUnfundedMaker` establishes that
fee/spot-plan pruning can cause a FOK non-fill rejection even when top-of-book
depth exists; this is a venue mechanic fixture, not a claim that such a case
occurred in a development world. The canonical frame timestamp, not an
`OrderFill` JSON payload field, anchors venue fill time; the direct replay
invariant fixture shifts that time. This map is subject to independent review.

Falsifier for the narrow mechanism is a verified partial-fillable venue
condition where the reconstructed IOC/FOK lifecycle contradicts the venue
contract, which is a **mechanical defect**, not an economic result. If no
partial-depth cases arise naturally, the economic contrast may be zero or
uninformative; retain it as a development result. No threshold for a
practically beneficial instruction has been registered, so do not convert a
small median difference into a positive mandate claim.

## Review, resources, ordering and stop

Use clean Go 1.27.0 binaries from one exact reviewed source/protocol commit.
Obtain one fresh prospective Sol-6 medium review of design, effective config,
new evidence contract, fixtures and corruption tests before the first world.
Fresh controls must have byte-identical canonical evidence and independently
reconstructed results. Their measured wall/RSS/disk costs must support all
12 cells before launch. Run one world at a time, `GOMAXPROCS≤7`, ≤30 s per
world, ≤15 min whole batch, ≤4 GiB process RSS and ≤1 GiB retained evidence.
No requirement is lowered to pass. Preserve all incomplete attempts; a
failed valid economic outcome is not a trigger for retuning. After complete
all-assigned replay, obtain separate bounded mechanics/evidence and
causal/statistical Sol-6 medium result reviews.

No ME-003 economic world starts after 2026-09-24 06:15 UTC; the entire
bounded continuation stops at 07:00 UTC. No confirmation, historical
holdout, ME-005, or new instruction variant follows automatically. If the
independent opportunity or lifecycle join is not ready in time, stop at
**READY AFTER FIXES**, preserve this draft, and launch no economic cell.
