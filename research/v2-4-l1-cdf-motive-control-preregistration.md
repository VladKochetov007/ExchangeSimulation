# V2-4 L1 — matched CDF/USD motive-control screen

Status: **preregistered before L1 implementation, configuration rendering, or
simulation.** This screen follows the completed L0 activation result and the
CDF roster audit at `602b523`. It is a local motive and non-collapse screen,
not a price-stability, empirical-demand, legacy-roster-replacement, or
long-horizon ecology claim.

## Question

L0 established that an explicit delivery obligation can direct a locally
informed CDF/USD hedge. That result did not distinguish the obligation motive
from the actor's access, clock, finite capital, local feed, IOC form, fee, and
request cap. It also did not establish that the actor can substitute for the
legacy multi-symbol `noise_flow` family.

L1 asks the narrower, falsifiable question:

> Holding a CDF/USD-only slot's access, deposits, local feed, clock, request
> sizing, IOC execution, fee, and liability-state path fixed, does choosing
> side from its signed delivery-liability gap reduce that slot's exposure more
> reliably than an independent random-side activity policy, without an
> immediate CDF/USD activity collapse?

The random-side control is intentionally an activity generator. It is the
counterfactual against which the economic motive is identified. It is not
called realistic demand.

## Parent and scope

Both L1 arms begin from the final V2-4 L0-B configuration and retain all
P0-C, P1-B, P2-B, maker, supplier, legacy `noise_flow`, option/future flow,
router, fee, price, latency, and existing timer settings. Each arm retains all
six legacy multi-symbol noise participants per venue. In particular,
`noise_trader_count` remains six.

The only L1 economic delta is the explicit `policy_mode` of the existing named
`liability_hedger_1` CDF/USD-only slot on each venue:

| arm | policy mode | side at an eligible decision |
| --- | --- | --- |
| L1-A | `random_side_control` | an independent fair random bit; it ignores gap sign |
| L1-B | `delivery_liability` | BUY iff signed `obligation - filled_position` is positive; SELL iff negative |

Both modes are enabled. This is deliberately unlike L0's disabled/enabled
comparison: it holds order opportunity and ordinary execution participation
present in both arms.

## Fixed slot contract

Each venue-local slot has exactly the L0 conditions:

```text
symbol                  CDF/USD
decision interval       2 s
obligation interval     10 s
obligation increment    +/- 200,000,000 raw CDF, reflected at the bound
obligation bound        +/- 2,000,000,000 raw CDF
request cap             100,000,000 raw CDF
order form              IOC limit at latest local executable touch
fee                     ordinary configured 5-bps quote taker fee
request/response link   constant 20 ms
market-data link        constant 40 ms
deposits                100 raw CDF and 100,000,000 raw USD per venue
```

The same per-venue obligation path uses the existing deterministic stream
`flowSeed(master_seed, venue_index, 0, 14)`. The new random-side-control bit
uses a separate deterministic stream
`flowSeed(master_seed, venue_index, 0, 15)`. It is consumed exactly once only
after subscription, a nonzero representable hedge gap, no pending request,
and the declared terminal-round-trip guard, but before selection of the
locally required executable side. The policy is therefore replayable without
using in-memory actor state.

Both modes use the same checked request magnitude:

```text
min(abs(obligation - filled_position), 100,000,000)
```

They use the same terminal censor policy, account, venue-local public
snapshot, required side, IOC condition, fill handling, fee schedule, and
actor-local position accounting. A missing required bid/ask is an explicit
`LOCAL_EXECUTABLE_PRICE_UNAVAILABLE` defer. Numeric zero is never used as an
availability sentinel. The treatment does not receive an index, midpoint,
global book, hidden counterparty, reserve exemption, price concession, or
forced fill.

`random_side_control` is allowed to increase its absolute signed gap; that is
the declared counterfactual, not an analyzer error. The auditor must identify
it as a random-control action rather than applying the liability mode's
reduction predicate to it.

## Evidence and independent replay

Both modes retain L0's full decision, fill, V2-0 receipt, decision-frontier,
and persisted-evidence artifact contracts. Every decision must additionally
persist a required named `policy_mode`; every side-selected decision persists a
named `BUY` or `SELL` field. The control's deterministic draw must be
independently replayed from its declared seed and declared consumption point.

The offline auditor must use only the immutable config, raw evidence, V2
receipt sidecars, generic gateway decisions, and exchange outcomes. It must
recompute:

1. the bounded liability path and actor-local filled position;
2. the slot's eligibility, terminal censor, gap magnitude, cap, required
   local executable touch, request fields, and exact prior delivered receipt;
3. the liability side or the random control bit according to `policy_mode`;
4. accepted/rejected/IOC-terminal outcomes, different-client counterparty,
   positive ordinary fee, and the actor-local fill transition; and
5. `delivered_at <= decision_time` for every submitted request.

It must report (not coerce) whether a fill reduced, preserved, or increased
the slot's absolute signed gap. It must reject a missing/duplicate decision,
missing mode, impossible obligation transition, mode/side mismatch,
future/missing/ambiguous receipt, wrong touch/cap/IOC field, unmatched
outcome, missing terminal IOC cancellation, self fill, fee-free/wrong-fee
fill, or actor/exchange fill mismatch.

Recorder on/off remains required to preserve the ordered execution hash across
fresh processes and GOMAXPROCS 1 and 4. Any telemetry-only scheduler event,
RNG draw, actor-visible-state mutation, or request-order change invalidates
the screen.

## Required implementation and adversarial tests

Before rendering cells, focused tests must prove:

- the legacy empty/missing `policy_mode` preserves the existing L0
  `delivery_liability` behavior; L1 rendered configs always state it
  explicitly;
- equal positive/negative gaps lead to the declared liability side;
- random control consumes exactly one independently seeded bit only at the
  declared eligible point and selects the persisted named side;
- both policies use precisely the same cap, local one-sided touch, IOC,
  deposits, fee, fill bookkeeping, and terminal censor rule;
- a missing required side defers observably rather than becoming price zero;
- the independent replay catches an altered random bit/seed, a treatment side
  reversal, an omitted policy mode, a hidden side fallback, a future receipt,
  a cap change, a dropped IOC cancellation, a free/self/fake fill, and a
  false claim that a random-control fill reduced the gap; and
- positive-domain L0 behavior is unchanged when `policy_mode` is absent.

These are evidence/motive tests only. They must not change matching,
scheduler, RNG, latency, fee, or market semantics outside the new L1 policy
selection.

## Immutable cells and completion

After tests pass, render full-evidence 30-minute cells for paired seeds 101
and 103:

```text
L1-A random_side_control  x {101, 103}
L1-B delivery_liability   x {101, 103}
```

The sole economic A/B config difference is
`cdf_liability_hedger.policy_mode`; provenance fields may differ by arm. The
renderer must mechanically reject every other difference. Completion is
established only by final `greeks.json` and `latency.json` sidecars. Before
scoring, extract V2 receipt/frontier audits, evidence artifact hashes, the L1
independent policy replay, CDF/USD activity corridor, role flow, fee/fill
accounting, and immutable analysis metadata. Raw evidence is retained; neither
sidecar completion nor a passing generic metric authorizes pruning.

## Activation and non-collapse gates

All four cells must have valid receipt/frontier and evidence-artifact audits.
Each of the three slots must show at least 120 nonzero liability updates (the
30-minute deterministic expectation is 180; 120 leaves only declared
terminal/warmup margin). Each arm/seed must have at least one accepted request
from every venue-local slot. Otherwise the affected policy is **NOT
EXERCISED**, not a motive result.

The following is a minimum CDF/USD non-collapse floor, deliberately weaker
than an ecology-viability claim. Across each full 30-minute cell and each
venue, CDF/USD must have:

```text
at least 150 taker trades
at least 2 distinct taker roles
at least 1 maker role
at least 95% two-sided published snapshots after the first 10 simulated seconds
```

The P2/L0 parent already has high trade counts and generally two-sided later
snapshots, but it fails stronger concentration rules. Thus passing this floor
does **not** show a viable, unconcentrated market. If an arm fails the floor,
its state-alignment evidence can still be mechanically scored, but that arm is
not an eligible activity-replacement candidate.

## Primary scoring and interpretation

Score in order:

1. evidence and policy replay integrity;
2. activation/exercise and minimum non-collapse floor;
3. per-fill gap transition; and
4. paired terminal and time-average absolute gap.

The narrow motive prediction is supported at screening level only when, in
both seeds, all exercised L1-B fills reduce its own absolute signed gap, L1-A
contains at least one exercised fill that does not reduce its gap, and L1-B's
time-average absolute gap is lower than L1-A's. Raw paired values remain
reported; two seeds are never called robust.

An L1-B submission/fill whose direction does not reduce its gap is **INVALID**.
A control fill that reduces its gap is expected by chance and is reported, not
treated as a failure. A missing control counterexample because its action path
never executes is **NOT EXERCISED**. A non-collapse-floor failure is a
**FALSIFIED replacement-candidate claim**, even if the internal motive policy
is mechanically valid.

No outcome may claim CDF/USD price stability, empirical price elasticity,
realistic demand, legacy `noise_flow` replacement, wealth/ecological success,
or phase robustness. A later L1-P offset experiment is required before any
cadence-insensitive conclusion; a later L2 deployment first has to isolate a
CDF allocation from the current multi-symbol noise roster.
