# V2-5 P3d — explicit exit-liquidity policy design

Status: **design only; no P3d configuration, code, preflight, or market world
has run.** This proposal follows the retained P3c negative result. It does not
alter P3a–P3c configs, scores, or evidence.

## Observed failure

P3c formed two legal, matched 10m ABC cash-and-carry terms but did not send a
single close order in its two-hour post-term window. The local perpetual asks
were present, priced, and persistent, yet only 16,286 raw units (central) and
16,348 (south) were displayed. P3's *actor-local* `min_order_size=100,000`
was applied equally to entry and exit. It rejected every touch-limited close
child before exchange admission, emitted 7,200
`EXECUTABLE_SIZE_UNAVAILABLE` decisions, left both pairs open, and allowed one
south funding transfer after the declared term.

The exchange itself admits any positive integer quantity; this 100,000 floor
is not a venue rule. It was an entry-materiality policy inadvertently reused
for risk reduction. P3c therefore does not show that no executable price or
liquidity existed. It shows that the current participant policy cannot use the
available liquidity.

## Local hypothesis

Entry materiality and finite-term exit are different economic actions.

```text
entry:  only establish a new balance-sheet term when the locally executable
        child is economically material

exit:   reduce already-owned exposure with an explicitly bounded IOC child
        at the locally delivered executable touch, even when that child is
        below the entry-materiality threshold
```

The P3c block should disappear if the *exit-only* child minimum is decoupled
from the entry threshold. This is not a claim that a finite term is always
liquid, nor that a small IOC has negligible impact. It isolates one named
participant-policy constraint.

## Minimal proposed P3d contract

Add an optional, persisted `unwind_min_order_size` to the term-carry policy.
It applies only in `UNWIND_PERP` and `UNWIND_SPOT` states:

```text
entry child minimum  = existing min_order_size
exit child maximum   = min(remaining signed gap magnitude,
                           existing lot_qty,
                           locally delivered executable touch size)
exit child minimum   = explicit unwind_min_order_size
```

`unwind_min_order_size=0` is a named policy meaning “use the exchange's
positive-unit minimum”; it does **not** mean price unavailable, no quantity,
or an unlimited order. The existing lot cap, current local touch price, IOC
time-in-force, 20-ms request latency, ordinary taker fee, normal margin
admission, and two-second decision cadence remain unchanged. Thus every child
is capped by observed displayed depth at one price level and cannot sweep a
deeper hidden ladder.

An omitted field preserves P3 v2's legacy behavior by using
`min_order_size` for unwinds. A nonnil field is a new P3 v3 evidence policy:
each decision records its effective unwind minimum explicitly, including a
present zero. This avoids an `omitempty`/zero ambiguity and lets an independent
replay distinguish legacy and P3d decisions.

No passive liquidity, spread, funding rate, latency, capital, clock, maker,
population, or demand parameter changes in P3d. The policy does not use global
depth, a future snapshot, a simulator termination time, forced fills, a fee
subsidy, or a market-price fallback.

## Independent measurement and adversarial checks

The analyzer must receive the persisted policy, decisions, local receipt
frontier, scalar gateway request, venue outcome, canonical fills, and final
accounts. For every submitted unwind child it independently predicts:

```text
side, price, qty = min(remaining gap, lot cap, delivered touch size)
```

using the versioned exit minimum. It must reject an order that:

- uses a future, missing, wrong-side, or wrong-symbol touch;
- exceeds current touch depth or the lot/gap cap;
- applies the exit exception to entry;
- turns a present small touch into price absence;
- creates/drops/duplicates an unwind fill or close; or
- reports a flat terminal balance while a canonical position remains.

Direct fixtures must prove both directions before a market cell:

1. legacy/no override still defers a 50-unit close below a 75-unit entry
   floor;
2. explicit zero exit minimum submits that 50-unit close at the same local
   touch, while the same 50-unit *entry* still defers;
3. partial unwind fills retain the remaining gap and submit the next bounded
   child rather than calling `TERM_CLOSED` early; and
4. a forged v3 effective minimum or oversized child makes the independent
   replay fail.

Evidence instrumentation remains observer-only: no scheduler event, random
draw, goroutine, gateway visibility, or actor decision is added solely for
recording. Existing fresh-process evidence-on/off and GOMAXPROCS tests remain
required; any new P3d evidence field must be included in the versioned
observer/replay contract.

## Preregistered causal screen after implementation

Only after code, unit tests, independent-replay mutation tests, and
instrumentation-neutrality pass, create immutable full-evidence P3d cells:

| arm | sole participant-policy delta |
| --- | --- |
| A — legacy exit floor | `unwind_min_order_size=100000` |
| B — exchange-unit exit floor | `unwind_min_order_size=0` |

Both arms use the P3c seed, all P3c market/financial/latency parameters, the
same explicit finite mandate, and the same 98-hour horizon. The P3d v3
versioned decision schema is common to both arms. This recreates the actual
control policy under the new auditable schema rather than treating historical
P3c as a silently incomparable implementation baseline.

Primary prediction for `B - A`: B emits at least one ordinary, receipt-bounded
unwind request after term end; A remains size-deferred. Completion is assessed
separately: every active term must close exactly once, terminal spot/perp
ownership must be flat, no funding transfer may occur after its term window,
and all mechanical evidence gates must pass. A lack of close in B is a
falsification of this proposed policy, not permission to alter its floor,
lot cap, or horizon after seeing the result.

## Kill criterion and limitations

P3d is killed if B cannot submit a legal small child from a present local touch,
if replay cannot independently predict it, if residual exposure survives the
registered close window, or if the policy causes an accounting/information
boundary failure. Even a P3d pass establishes only that this actor can reduce
its current finite term under this development ecology. It does not validate
its funding-persistence belief, prove positive net carry after all exits,
establish funding anchoring, or demonstrate a realistic optimal execution
strategy. A later execution/participation model would require a new
mechanism-level experiment, not an in-place P3d parameter adjustment.
