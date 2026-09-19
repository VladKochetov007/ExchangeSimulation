# SV1D activation probe triage — 2026-09-19

## Status

This is a successor-candidate diagnostic, not an activation result. The R2
calendar/lifecycle semantics and the finite CDF/USD supplier hypothesis remain
unchanged. The predecessor R2 candidate remains archived as
`NON-VIABLE AT THE 24H MARKET-SURVIVAL GATE`.

No development cell, freeze, or holdout was consumed by this diagnosis.
Untouched holdouts remain closed.

## Exact candidate and gate context

The candidate under test was scientific commit
`40c627e75ea2f6e685091f9bee954df79922be9`, tree
`772a52eb23fc4bfc36998420faf25b42d254b5bc`. The tree was clean and had already
passed the clean Go test, vet, targeted race, determinism, and evidence-contract
gates. A fresh Luna xhigh review of this exact tree returned
`COMPLETED / ACCEPT WITH CONDITIONS`; the report is retained outside the
candidate checkout at
`/home/vlad/external-scratch/sv1d-review-40c627e-20260919.md`.

The fresh Go 1.27 rebuild was provenance-identical to the supplied bundle. The
independent seed-977 finite-cgroup capacity preflight passed with zero OOM or
swap events and peak cgroup memory of 564,375,552 bytes. Its attestation and
the exact bundle are retained under
`/home/vlad/external-scratch/sv1d-capacity-run-40c627e-20260919-cgroup8g/` and
`/home/vlad/external-scratch/sv1d-capacity-pinned-40c627e-20260919/bundle-final/`.

The authorized seed-659 activation probe used treatment, mode-off, and
no-roster arms. Raw binary evidence, rendered files, arm results, and the
failed score attempt are retained under
`/home/vlad/external-scratch/sv1d-activation-40c627e-20260919-cgroup8g/`.
The scorer returned `INVALID_EVIDENCE` and did not issue an economic verdict.

## Finding A — zero-based trade identity is valid

### Reproduction

The retained north treatment evidence contains the first exchange trade with:

```json
{"trade_id":0,"price":300100000,"qty":10586396,"side":"BUY","taker_order_id":72,"maker_order_id":11}
```

The corresponding `OrderFill` records also carry `trade_id: 0`. The exchange
implementation documents and implements this deliberately: `createTrade`
uses the order book sequence number, and a newly created book starts at zero.
The nonzero order identities and the `(venue, trade_id)` / participant-order
composites provide the identity and duplicate protections.

The strict CDF analyzer nevertheless rejects zero in
`processCDFTrade`, `processCDFOrderFill`, and `processCDFFill`. This caused
three direct trade-identity failures and cascaded into supplier-fill and
exchange-fill attribution failures in the activation attempt.

### Intended invariant and classification

`trade_id == 0` is a valid first trade identity. A trade must have positive
price and quantity, valid side, and (under strict mechanics) distinct nonzero
maker and taker order identities. A supplier/exchange fill must retain its
nonzero order identity and may refer to trade zero. Duplicate prevention uses
the complete scoped composite, not a positive-value sentinel.

Classification: **ANALYZER BUG, REACHABLE IN THE SV1D ACTIVATION PROBE**.
It is not an exchange economic defect and it did not alter any historical R2
trajectory: the CDF successor roster was off in historical R2 configurations,
and no holdout was run under this successor.

The correction is limited to removing the invalid positive-trade-ID sentinel
and adding regressions for strict trade, supplier-fill, and exchange-fill
paths. Malformed identities remain rejected through their other required
fields and composite duplicate checks.

## Finding B — nominal half-phase ticks were labeled before actual execution

### Reproduction

The treatment config registers a one-second deterministic runner step,
10 ms CDF market-data latency, and CDF decision phase offsets of 0, 0.5, 1,
and 1.5 seconds. For north client 24, the retained rendered evidence shows a
submit decision with:

| field | value |
|---|---:|
| nominal/event `sim_ts` | 8.5 s |
| `decision_time` | 8.5 s |
| observation publication | 8.0 s |
| observation delivery | 9.0 s |

The fixed-width actor-gateway decision sidecar records the same request at
9.0 s, with the same price and quantity. This is because the deterministic
runner advances the simulated clock to the next one-second boundary before it
drains the actor phase. `simTimer.fire` correctly preserves the scheduled
timer timestamp (8.5 s), but the callback actually executes at the runner
clock boundary (9.0 s), after the delayed observation entered the actor inbox.

The strict analyzer consequently reports 888 non-causal observation decisions,
264 actor-gateway timestamp/frontier mismatches, and dependent submit/reason
failures. This is not a logging-only discrepancy: the actor’s action timestamp
and time-dependent private reference update are both derived from the nominal
timestamp even though the actor could not observe the message until the later
execution boundary.

### Intended invariant and classification

For an actor-facing delayed feed, the execution decision timestamp must be the
local simulated time at which the actor callback runs. It must satisfy:

```text
publication <= delivery <= decision execution
decision event sim_ts == decision execution
gateway decision time == decision execution
quote submitted time == decision execution
```

The registered phase offset remains the intended schedule phase and remains
evidence; it is not a claim that a callback can execute before the runner’s
fixed-point boundary. The CDF actor’s reference aging, quote submission time,
and evidence decision time must use the same actual local execution timestamp.

Classification: **REACHABLE SUCCESSOR SIMULATOR/EVIDENCE SEMANTIC DEFECT**.
It invalidates the current activation probe but did not alter historical R2
results: this finite CDF successor was not enabled in those worlds, and no
holdout has been consumed.

The minimal correction is dependency-injected. The delayed gateway exposes its
current actor-local simulation clock; the CDF actor receives an optional
`DecisionNow` callback and uses it for one callback consistently. Direct unit
tests without a clock retain the existing nominal-time fallback. This does
not expose venue state or a global market oracle to the actor; it only stamps
the time at which its already-local callback actually executes.

## Failure-cascade and rerun decision

The two findings are independently reproducible. The zero-based identity
correction is analyzer-only. The timestamp correction changes the successor’s
runtime/evidence semantics, so the retained seed-659 trajectory is not repaired
offline and will not be rescored. After focused regressions, full mechanical
gates, fresh independent review, a clean Go 1.27 rebuild, a new capacity
verification, and a new development-only authorization, seed 659 must be rerun
from the corrected binary. No 24-hour development cells or holdouts are
authorized by this note.

## b466 activation adjudication and terminal-contract successor

The exact `b466b57fd2bfcbdcb1319a1942ac8370a86d6ba4` candidate passed its fresh
Go 1.27 rebuild/provenance and finite-cgroup capacity preflight. Its registered
seed-659 development-only activation probe was retained but failed closed with
`INVALID_EVIDENCE`; the old namespace remains immutable at
`/home/vlad/external-scratch/sv1d-activation-b466b57-20260919-cgroup8g`.

A separate fresh Luna xhigh adjudication (`Peirce`) inspected b466 and the
retained raw evidence. It classified the remaining failures as follows:

* The 264 `reprice_for_inventory_or_touch` failures were an analyzer defect.
  The actor compares the desired quote with its live order remainder after a
  partial fill, while the analyzer compared against the prior decision's
  quantity. The strict invariant is comparison with the reconstructed live
  order `(side, price, remaining quantity)`, not with a self-authored prior
  decision.
* Three terminal submissions and three terminal cancellations were
  right-censored transport requests at the exact simulation endpoint. They
  must not be inferred accepted, rejected, or closed; pre-terminal unresolved
  requests remain hard failures.
* Four missing supplier-fill rows and the four corresponding producer-order
  failures were one terminal-tail execution/evidence defect, not eight
  independent failures. The exchange filled live CDF orders at the endpoint,
  but the actor's delayed fill callback could not arrive before shutdown.

These findings are reachable in the successor and invalidate the b466
activation result as a scientific activation claim. They do not affect any
historical R2 result because the finite CDF roster was not enabled there, and
no holdout has been consumed. The retained b466 activation is not rescored or
rewritten.

The successor correction has two independent parts. First, strict analysis
reconstructs live supplier quote identity and remaining quantity from accepted
orders, partial/full fills, and cancellations; the reprice predicate remains
fail-closed when the live order cannot be reconstructed. Second, the CDF actor
receives the registered simulation horizon and uses a two-interval
round-trip-censor window. Within that window it emits the explicit
`simulation_horizon_censored` wait/withdraw lifecycle reason, withdraws any
live quote, and submits no new quote. A strict predicate accepts that reason
only in the registered terminal window and never treats it as an exchange
outcome. This mirrors the existing terminal-tail policy used by other
successor actors and prevents the simulator from creating unobservable
endpoint fills.

Focused actor/analyzer regressions pass, including partial-fill reprice
reconstruction and both live-quote/no-live-quote terminal censor paths. The
candidate still requires the remaining full tests, race/static/evidence gates,
fresh independent review, clean pinned rebuild/capacity verification, and a
new seed-659 probe. No prior namespace is repaired, and no development cells
or holdouts are authorized by this amendment.

The previous invalid activation namespace remains immutable historical evidence.
The performance branch and its binary-evidence prototype remain deferred; no
performance-branch code is imported by this correction.

## Successor implementation checkpoint

The zero-based identity correction is committed as `8103bcd`. The execution
timestamp correction is committed as `4f14dfb`. It adds no serialized
configuration field and no market-state input: `DelayedGateway` exposes only
its current participant-local clock, `Sim.addVenue` injects that clock into the
CDF actor, and the actor uses one sampled execution timestamp for
observation-age checks, private reference aging, decision evidence, and quote
submission. Direct actor tests retain the nominal-ticker fallback.

## Independent review of `4f14dfb`

A fresh read-only Luna xhigh reviewer (`Kierkegaard`) inspected the exact
candidate commit `4f14dfb` and tree
`baa07deb9376e49c57753cbf9310e8505bb86c7c`. Review execution was
`COMPLETED`; the substantive verdict was **ACCEPT WITH CONDITIONS**. The
reviewer found no calendar, exchange, or scheduled-risk regression and agreed
that the zero-based identity correction preserves positive price/quantity,
valid-side, nonzero/distinct maker/taker, scoped uniqueness, and reconciliation
invariants. The reviewer also accepted the execution-clock wiring on the
registered deterministic path: the gateway clock is participant-local, the
same sampled time is used consistently, and the regression covers the nominal
timer-versus-execution boundary.

The reviewer required three bounded follow-ups before promotion:

1. complete a focused multivenue race run;
2. add an end-to-end registered-fixture reconciliation test whose first trade
   identity is zero (the prior tests were direct helper-level regressions);
3. prevent a manually constructed gateway without a scheduler/clock from
   advertising a zero-valued execution clock to a supplier.

The first follow-up's changed-path focused race run passed after review. The
broader multivenue race matrix had previously exceeded its ten-minute test
timeout in the existing fresh-process determinism case without a race report;
it is not treated as a substantive acceptance. The second and third follow-ups
are implemented in the successor checkpoint: the full registered fixture now
accepts a zero-based first trade, and clock injection requires both
`NowUnixNano` and `SimulationClockConfigured` to report a deterministic
scheduler/clock pair. This changes the candidate tree, so a fresh independent
review is required; the `4f14dfb` review does not cover the successor.

The focused simulation, multivenue, and analysis regressions pass at this
checkpoint. The successor still requires the clean full test/vet/race/evidence
gates, a fresh independent review, a fresh pinned Go 1.27 rebuild/capacity
verification, and a new development-only seed-659 probe before it can advance
the scientific program. No 24-hour development cells or holdouts are
authorized by this note.
