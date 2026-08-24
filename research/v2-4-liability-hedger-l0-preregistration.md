# V2-4 L0 — stateful CDF/USD delivery-liability hedger

Status: **preregistered before implementation, configuration rendering, or
simulation.** L0 is an activation and evidence screen for one economically
motivated participant. It is not a price-stability, demand-elasticity,
replacement, or long-horizon ecology claim.

## Question and diagnosis

The retained V2-3 population still uses `noise_flow` as an unconditional
random side/size source. It may create trades, but neither a fill nor an order
answers *why* that participant needed the asset. The V2-4 requirement is to
replace such activity generators one family at a time with finite-capital
participants whose state and objective are independently auditable.

L0 introduces one CDF/USD delivery-liability hedger on each venue. It receives
a bounded, mean-zero sequence of external delivery obligations. Its only
objective is to carry a CDF position equal to the current signed obligation;
it does not observe an index, midpoint, private fair value, or simulator
internal book. A positive obligation means it must acquire CDF relative to its
opening inventory, and a negative obligation means it must reduce that relative
CDF inventory. An ordinary filled hedge may be costly or fail; the obligation
does not force a fill or set a price.

The first screen deliberately **adds** this actor while retaining the P2-B
population. It must be proved stateful before a later L1 replacement removes a
random-flow participant. Treating an unvalidated new actor as an immediate
replacement would confound motive activation with a change in aggregate flow.

## Local mechanism and fixed policy

L0 starts from the final V2-3 P2-B configuration, including P0-C passive
refresh, P1-B inventory-asymmetric quote size, and the enabled P2 rebalance.
It introduces exactly one `liability_hedger_1` on `CDF/USD` per venue. All
P0/P1/P2, maker, supplier, noise, latency, router, fee-rate, derivative, and
clock parameters are held fixed.

The actor has finite venue-local inventory and cash, receives its ordinary
delayed public CDF/USD snapshot through role `liability_hedger`, and evaluates
every 2 seconds after subscription. Its obligation updates every 10 seconds
from a per-venue, seed-derived private state stream:

```text
obligation[t+10s] = clamp(obligation[t] + {-200,000,000, +200,000,000},
                           -2,000,000,000, +2,000,000,000)
hedge gap          = obligation - filled position relative to opening inventory
```

At a boundary the step is reflected inward, rather than clipped to zero, so an
eligible update always changes the obligation. The exogenous liability stream
is mean-zero by construction; the actor's market side is **not** sampled. It
is implied by the signed hedge gap. The treatment may trade at most
`100,000,000` base units per decision using an IOC limit at its latest locally
received executable ask (buy) or bid (sell). There is no price concession,
guaranteed fill, reserve exemption, zero-price fallback, post-only exemption,
or direct counterparty. The actor pays the ordinary configured 5-bps quote
taker fee on a fill.

The execution contract is intentionally one-sided and explicitly named:
buying needs a local executable **ask**, selling needs a local executable
**bid**. A two-sided midpoint is neither required nor constructed. Missing the
needed executable side is an observable `LOCAL_EXECUTABLE_PRICE_UNAVAILABLE`
defer, never numeric price zero.

The actor starts with finite CDF and USD deposits sufficient for both sides of
the bounded five-minute L0 screen; exchange admission, balance reservations,
and ordinary fee posting still determine whether a request can execute.
External obligations are an off-exchange motive ledger, not a hidden transfer
inside venue conservation accounting.

## A/B causal screen

Both arms construct the same actor, timer, seed-derived liability path,
delayed-feed role, deposits, and evidence recorder. Their immutable rendered
configs may differ only in `liability_hedger.enabled` plus declared
provenance.

| arm | seeds | submit permission | meaning |
| --- | ---: | --- | --- |
| L0-A | 101, 103 | false | state/evidence control: liability evolves, but all market actions are explicitly disabled |
| L0-B | 101, 103 | true | stateful local hedger may submit capped IOC orders |

Each cell is five simulated minutes, uses full persisted evidence, and is
complete only when final `greeks.json` and `latency.json` sidecars exist. Raw
evidence stays retained; no L0 extractor may prune it.

## Required evidence and independent replay

Every two-second evaluation emits an evidence-only
`liability_hedger_decision` row with:

```text
venue_id, hedger, client_id, symbol, decision_time, enabled, subscribed,
action, obligation_before, obligation_after, obligation_step, obligation_limit,
position_before, hedge_gap, decision_interval, update_interval,
last_book_source_time, last_book_received_time, snapshot_sequence,
bid_price, bid_visible_qty, ask_price, ask_visible_qty,
side, limit_price, requested_qty, request_id, taker_fee_bps
```

`side` is a named string when a side is selected. Thus the valid internal BUY
zero enum cannot disappear under JSON `omitempty`; the completed V2 zero-enum
wire audit is a required preflight. Every exchange-confirmed L0 fill additionally
emits `liability_hedger_fill` with order/trade/fee fields and actor-local
pre/post position.

The generic V2-0 receipt sidecar covers the role's delayed local trading link.
For each submitted request, the independent auditor must join the exact local
snapshot receipt, show `delivered_at <= decision_time`, and recompute the
required side, executable touch, requested cap, and initial request fields.
It must join accepted/rejected outcomes and, if filled, the exchange fill,
different-client counterparty, positive ordinary taker fee, and actor-local
position change. It must not use actor state or a simulator callback as its
source of truth.

Deferred actions remain observable. In particular, `POLICY_DISABLED`,
`NOT_SUBSCRIBED`, `REQUEST_PENDING`, `IN_BAND`,
`LOCAL_EXECUTABLE_PRICE_UNAVAILABLE`, `INVALID_LIMIT_PRICE`,
`ZERO_REQUEST_QUANTITY`, and terminal-horizon censoring are distinct states.

## Activation gates and falsifiers

Before any market interpretation, all cells must pass:

1. the V2-0 receipt/decision audit and exact persisted-evidence artifact hash;
2. an independent L0 replay with no missing/duplicate decision, impossible
   obligation transition, future/missing/ambiguous receipt, side/touch/cap
   mismatch, unmatched outcome, non-IOC terminal, self fill, fee mismatch, or
   non-reducing local fill;
3. each hedger has at least 20 nonzero obligation transitions in both arms;
4. L0-A records only `POLICY_DISABLED` after subscription and submits zero L0
   requests; and
5. L0-B submits eligible requests whose named side always reduces the signed
   hedge gap. At least one accepted L0 request across each paired seed is
   required to call the action path exercised. If L0-B has no accepted request,
   the mechanism is **NOT EXERCISED**, not a successful deferral policy.

The mutation suite must independently catch: a dropped/duplicated decision,
an obligation update stripped or made constant, a reversed BUY/SELL side, a
future-injected or delayed snapshot, an absent executable touch treated as
zero, a cap violation, a dropped terminal IOC cancellation, a fake/self fill,
and a fee-free fill. Recorder on/off must retain the execution hash across
fresh processes. A new scheduler event, RNG draw, actor-visible state path, or
request-order change introduced solely by evidence recording invalidates L0.

## Scoring and limits

The only primary L0 claim is:

> A finite-capital participant with an evolving, independently observable
> delivery obligation can submit locally informed, capped, ordinary-cost
> CDF/USD hedges whose direction reduces its own explicit hedge gap.

`SUPPORTED (screening)` requires all activation gates and exercised actions in
both paired seeds. A failed evidence contract is **INVALID**; an untriggered
action is **NOT EXERCISED**; seed disagreement is reported without averaging
away the difference.

No result may claim that L0 stabilizes CDF/USD, improves liquidity, supplies
price elasticity, replaces random flow, produces a realistic wealth ecology,
or repairs the frozen runaway. Those are later L1 / longer-horizon questions
with separate controls, viability gates, phase sweeps, and holdout seeds.
