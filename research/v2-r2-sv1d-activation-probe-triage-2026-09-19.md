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

The previous invalid activation namespace remains immutable historical evidence.
The performance branch and its binary-evidence prototype remain deferred; no
performance-branch code is imported by this correction.

## Successor implementation checkpoint

The zero-based identity correction is committed as `8103bcd`. The timestamp
correction is the next uncommitted candidate change in this worktree. It adds
no serialized configuration field and no market-state input: `DelayedGateway`
exposes only its current participant-local clock, `Sim.addVenue` injects that
clock into the CDF actor, and the actor uses one sampled execution timestamp
for observation-age checks, private reference aging, decision evidence, and
quote submission. Direct actor tests retain the nominal-ticker fallback.

The focused simulation and multivenue regressions pass. The change still
requires the clean full test/vet/race/evidence gates, fresh independent review,
fresh pinned rebuild/capacity verification, and a new development-only seed-659
probe before it can advance the scientific program.
