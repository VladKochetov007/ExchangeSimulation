# V2-5 P1a — fee-aware funding/carry feasibility preregistration

Status: **preregistered before rendering, execution, or inspection of a P1a
world.** P1a is a single-cell feasibility gate for the planned paired P1
causal market screen. It is not a funding-anchor, basis-convergence,
profitability, or realism experiment.

## Parent and question

P0 established that the opt-in desk consumes only delayed public funding and
locally delivered books, and that its decision/outcome evidence can be
independently replayed. Its activation configuration intentionally set every
economic cost to zero; it was therefore unsuitable for a market-level carry
claim. The P0 seed-101 calibration record saw public funding rates of 1, 2,
and 3 bps during its five-minute horizon. It also established from actual
funding-carry fills that the exchange charges the configured 5-bps taker fee
per aggressive leg.

P1a asks the cheap falsification question:

> With the exchange's actual five-bps taker fee priced for all four entry and
> eventual exit legs, plus small separately named balance-sheet, margin-risk,
> leg-risk, and minimum-return terms, does the existing public funding process
> produce even one locally justified funding-carry action in a fresh
> 30-minute development cell?

This is deliberately a feasibility test before spending four long paired
market worlds. A negative result means this particular costed policy is not
exercised in this population; it does **not** mean funding is economically
unimportant in general.

## Fixed policy and population contract

The P1a cell retains the P0 r1 population, symbols, clocks, 10-ms
participant-local public-feed delay, full receipt/decision evidence, one
funding interval horizon, request size, capital, non-atomic IOC legs, and
terminal censoring. It changes only the named carry-economics fields below and
makes the exchange fee explicit at its existing default value:

| field | P0 activation | P1a fee-aware feasibility | rationale |
| --- | ---: | ---: | --- |
| exchange `taker_fee_bps` | implicit default 5 | explicit 5 | actual exchange fee recorded in P0 fills |
| policy `taker_fee_bps` | 0 | 5 | four aggressive entry/exit legs cost 20 bps in this policy |
| `borrow_annual_bps` | 0 | 500 | declared annual financing estimate; exact bps rounding is retained in evidence |
| `balance_sheet_bps` | 0 | 1 | minimum discrete balance-sheet charge |
| `margin_risk_bps` | 0 | 1 | minimum discrete margin/liquidation-risk charge |
| `leg_risk_bps` | 0 | 1 | minimum discrete non-atomic leg-risk charge |
| `min_net_carry_bps` | 0 | 1 | minimum post-cost return |

No price, funding rate, mark, index, demand, maker, latency, spread, timer,
population, legacy carry desk, or capital parameter is adjusted. In
particular, P1a does not inject a funding rate, modify a premium, subsidize a
fee, force a fill, or use an exchange-global price.

At the policy's integer-bps granularity, its fixed immediate hurdle is 24 bps
(20 fee + 1 balance sheet + 1 margin risk + 1 leg risk + 1 minimum net
carry), plus the exact annual-borrow term. The annual-borrow term may round to
zero over one eight-hour interval; that is an explicit limitation to be
reported from the persisted `borrow_cost_bps` field, not silently treated as a
free loan or changed after the run.

## Immutable cell

| field | value |
| --- | --- |
| config | `configs/v2-5-p1a/fee-aware-107.json` |
| seed / horizon | 107 / 30 simulated minutes |
| logs | full persisted evidence; retained |
| completion sentinel | final non-empty `greeks.json` **and** `latency.json` only |
| evaluation role | development feasibility; not a paired causal seed |

Seed 107 is deliberately distinct from P0's seed 101 and reserved P1 paired
seeds 101/103. It is not an untouched holdout for V2.

## Evidence and independent checks

Before any interpretation, retain and independently extract:

1. V2 receipt/frontier audit and persisted-evidence artifact digest;
2. funding-carry policy/outcome replay, including each declared cost and every
   public funding/book source in the decision frontier;
3. rate, income, each cost, net-carry, action/defer, requested-leg, accepted,
   fill, cancel, and orphan-position counts by venue; and
4. terminal conservation and generic funding semantics.

No decision may treat rate zero, price zero, a missing book, or a missing
funding update as a numeric economic input. A failed costed action must be an
explicit defer such as `NET_CARRY_BELOW_MINIMUM`, not a missing record.

## Activation gate and falsifiers

P1a is **FEASIBLE** only if all evidence audits are valid and all three venues
each produce at least one fresh-funding, positive-net-carry submitted first
spot leg with a gateway/venue/actor-outcome chain. Its actual rate, named
costs, net carry, position change, and non-atomic outcome must replay exactly.

P1a is **NOT EXERCISED** if no venue produces such a first leg, even if the
receipt and arithmetic audits pass. That outcome halts the planned P1 paired
market screen at these declared values; it does not authorize lowering fees,
changing clocks, widening funding, or inventing a price intervention. The
next permissible work would be a separately designed V2-5 policy-resolution
or population-economics slice.

The cell is **INVALID** if any actor uses an undelivered/future source, if a
recorded fee/risk component does not recompute, if an actual order lacks a
decision/evidence chain, if the exchange fee differs from its explicitly
configured policy estimate without being reported, or if instrumentation
changes the ordered execution contract.

## Conditional next gate

Only a FEASIBLE P1a permits a separately preregistered P1b:

```text
A: same installed desk, policy disabled
B: same installed desk, fee-aware policy enabled
paired seeds: 101, 103
```

P1b would score desk inventory/orders first, then paired perp-basis width,
signed basis persistence, and funding-response observations. It would not
claim that funding is an anchor if expected funding changes without an
independently reconstructed inventory/order response.
