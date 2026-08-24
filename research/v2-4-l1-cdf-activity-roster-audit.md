# V2-4 L1 design input — CDF/USD activity roster

Status: **completed design-input audit; no simulation semantics changed.** This
is the evidence basis for choosing the first V2-4 comparison. It is not a
causal result and does not license a population change by itself.

## Question

L0 showed that a finite-capital delivery-liability participant can hedge an
auditable local gap. The next question is not whether another actor can be
removed from the configuration by count. It is whether a state-motivated,
CDF/USD-only participant can be compared with an otherwise matched
activity-generator control without silently changing activity in unrelated
markets.

## Current roster

The retained V2-3 P2-B / V2-4 L0 parent has six `noise_flow` participants per
venue. Each is a single four-symbol random-flow actor:

```text
ABC/USD, ABC-PERP, CDF/USD, ABC/CDF
```

It chooses a symbol and side at its two-second tick, has a 500,000,000 raw
base-unit CDF/USD target size, uses a heavy-tailed size draw, and submits a
market order bounded by its locally observed contra depth. It is therefore a
multi-market activity generator, not a CDF-only participant. The relevant
construction is `simulations/multivenue/sim.go` and
`simulations/feesim/taker.go`.

The L0 delivery-liability actor is one CDF/USD-only actor per venue. It has a
20 ms request/response and 40 ms market-data contract, an independent bounded
obligation stream, a two-second decision cadence, 100,000,000 raw-unit IOC
cap, and no midpoint/index/global-book fallback.

## Descriptive retained-evidence check

The following is a role-level flow reconstruction from retained final L0-B
evidence. Values are `mvanalyze -metric flow` base units; role aggregation
cannot identify individual noise actors, so these rows are not used as a
causal estimate.

| seed | venue | `noise_flow` gross | `liability_hedger` gross | ratio L0 / noise |
| ---: | --- | ---: | ---: | ---: |
| 101 | north | 2,614 | 60 | 2.3% |
| 101 | central | 1,718 | 50 | 2.9% |
| 101 | south | 1,744 | 47 | 2.7% |
| 103 | north | 2,800 | 57 | 2.0% |
| 103 | central | 1,957 | 46 | 2.4% |
| 103 | south | 1,923 | 53 | 2.8% |

The L0 actor is deliberately much smaller than the aggregate legacy random
flow. That is not a defect in L0's narrow hedge-gap result. It means that an
uncontrolled decrement from six to five multi-symbol `noise_flow` actors would
not constitute a clean CDF/USD replacement test.

The common five-minute viability report also shows the parent CDF/USD books
already fail many windows for `concentrated_flow`; L0 neither licenses nor
defeats a viability claim. A later comparison must predeclare an appropriate
CDF-specific corridor rather than reinterpret this descriptive output after
seeing an outcome.

## Rejected shortcut

The following intervention is rejected:

```text
noise_trader_count: 6 -> 5
add or enable one CDF/USD liability hedger
```

It removes ABC/USD, ABC-PERP, and ABC/CDF random orders as well as CDF/USD
orders. Because ABC/CDF connects the graph, a resulting CDF/USD change could
be caused by the missing cross-pair activity. It also changes a global actor
count rather than substituting an economic motive at a named CDF boundary.

Neither an A/B difference nor a viability change from that design can identify
the liability motive. It will not be used for L1.

## Selected L1 comparison boundary

L1 will add one named CDF/USD-only *activity slot* per venue in both arms and
leave the six legacy multi-symbol `noise_flow` actors unchanged. The slot will
have identical finite deposits, delayed local CDF/USD feed, decision cadence,
IOC touch execution, request cap, ordinary taker fee, terminal censor rule,
and evidence surface in both arms. Its sole treatment difference will be the
side-selection policy:

```text
control:   independent random-side activity policy
treatment: direction implied by the slot's signed delivery-liability gap
```

Both slots retain the same bounded liability-state path and request-size rule.
The control deliberately ignores the gap's sign when selecting its side. Thus
the comparison isolates motive-directed side selection from access, capital,
latency, sizing cap, fee schedule, IOC mechanics, and clock.

This is a **matched motive-control screen**, not yet a claim that the legacy
multi-symbol noise roster has been replaced. If it activates and passes its
predeclared CDF viability gate, a subsequent L2 deployment experiment may
demote a specifically isolated CDF allocation. That later experiment must
first split the current multi-symbol noise roster without changing its other
symbol paths.

## Consequence for the V2 ledger

The chosen L1 design makes two possible outcomes informative:

- If liability direction lowers its own signed-gap exposure while the matched
  random-side slot does not, this validates a local objective distinction but
  does not establish price stability or empirical realism.
- If the liability arm cannot meet the declared CDF activity/viability gate,
  it is not a viable replacement candidate at this scale. The result will be
  reported as such rather than repaired by increasing size, shortening clocks,
  or changing demand after observation.

The complete fixed conditions, activation contract, scoring order, and kill
criteria are frozen separately in the L1 preregistration before code,
configuration, or simulation.
