# V2-3 P2 — explicit CDF/USD inventory rebalance

Status: **preregistered before implementation, configuration rendering, or
simulation.** P2 is a mechanism-identification screen, not a stability,
calibration, or aggregate-risk claim.

## Boundary and hypothesis

P0 made passive refreshes rest-only and P1 established the declared
inventory-asymmetric displayed-size response. P1-B nevertheless showed large
persistent individual CDF/USD exposures: its six CDF-maker venue/state series
had mean absolute net delta from `8.93e9` to `2.88e10` raw base units against
the `1.00e10` configured spot-maker inventory limit. CDF/USD has no configured
hedge symbol; the existing ABC/USD-to-perp hedge does not apply to it.

P2 tests only this local hypothesis: an out-of-band CDF/USD maker can submit a
separate, locally informed, rate-limited IOC risk-transfer order. The order
pays a normal taker fee and bounded price concession, has no priority or
guaranteed fill, and does not repair a price.

It can reduce one maker's risk only by trading against another account. It may
transfer risk to another maker, fail, or worsen book conditions; it cannot
establish aggregate CDF supply/demand balance or solve the runaway claim.

## Scope and fixed policy

P2 applies only to six `cdf_spot_maker_{1,2}` instances (two per venue) on
`CDF/USD`. It starts from P1-B: 5,000-bps size skew and P0-R1 C
post-only/cancel-before-replace are fixed. ABC/USD, ABC/CDF, derivative,
fixed-distance, imbalance, router, spread, reference, latency, population,
price-skew, and existing clock parameters remain unchanged.

Both P2 arms schedule the same new rebalance evaluation timer. Control records
an explicit `POLICY_DISABLED` decision but submits no order. Treatment values:

| field | value | contract |
| --- | ---: | --- |
| risk band | 10,000,000,000 CDF | eligible at or beyond absolute raw inventory |
| target band | 5,000,000,000 CDF | never request below this residual band |
| maximum request | 500,000,000 CDF | hard action cap |
| contra-touch participation | 1,000 bps | request ≤ 10% of last locally received visible contra-touch quantity |
| price concession | 50 bps | sell below local bid / buy above local ask, outward tick alignment |
| evaluation interval | 10 s | independent policy cadence |
| cooldown after submission | 30 s | applies regardless of fill or rejection |
| order | limit IOC, not post-only | actual non-guaranteed aggressive request |
| fee | existing 5-bps taker quote fee | CDF maker accounts use maker=0/taker=5 percentage fees in both arms |

For inventory `q`, P2 sells if `q > 0`, buys if `q < 0`, and starts from
`max(0, abs(q)-target_band)`. Requested quantity is the checked positive
minimum of desired reduction, maximum request, and participation cap.
Missing/stale local book, in-band risk, pending request, cooldown, zero or
overflowed cap, and invalid tick-aligned price are explicit deferred reasons;
none becomes a zero-price or zero-quantity order. “Stale” is not a new free
parameter: a local snapshot is stale when its actor-visible source-publication
age exceeds the already declared 10-s evaluation interval. Receipt delivery
time is retained solely as independent evidence; turning its recorder off for
the neutrality regression must not alter whether the policy acts.

An own resting touch may appear in the local public snapshot. Standard
same-client self-trade prevention skips it; only another client can fill the
IOC. A same-client fill, forced repair, non-IOC remainder, or uncapped request
invalidates P2.

## Evidence and independent audit

Every evaluation persists a `maker_inventory_rebalance_decision` in the
evidence-only domain, including:

```text
venue_id, maker, client_id, symbol, decision_time, enabled, action_or_defer_reason,
inventory, risk_band, target_band, last_book_source_time, last_book_received_time,
bid_price, bid_visible_qty, ask_price, ask_visible_qty, side, desired_reduction,
participation_cap, max_request_qty, slippage_bps, limit_price, requested_qty,
taker_fee_bps, request_id, cooldown_until
```

For submissions, `request_id` must exactly join accepted/rejected evidence;
accepted order IDs must join IOC cancellation/fill records. Every fill must
show the requested symbol/side, a different counterparty client, and a positive
quote fee equal to the existing integer percentage-fee formula. Deferred rows
have no request ID but remain observable.

The recorder adds no scheduler event, RNG draw, actor-visible state, callback,
or request-order change. Fresh-process recorder on/off must preserve execution
hash. The independent analyzer recomputes side, desired reduction,
participation cap, tick-aligned limit, requested quantity, cooldown, joins,
IOC terminal state, fills, and fees. It must catch side reversal, missing cap,
free/fake fill, duplicate request, dropped cancellation, and delayed/future
snapshot mutations. Zero fills means **NOT EXERCISED**, never success.

## Screen and score

After implementation/mutations, render four full-evidence five-minute cells
from a frozen P1-B parent:

| arm | seeds | enabled | meaning |
| --- | ---: | --- | --- |
| P2-A | 101, 103 | false | same timer/evidence; disabled control |
| P2-B | 101, 103 | true | explicit CDF/USD IOC risk-transfer policy |

The rendered differ permits only P2 provenance and declared P2 fields; A/B may
differ only in `enabled`. It rejects P0/P1, price/quote, feed, latency,
population, fee-rate, existing-hedge, or unrelated-clock changes.

Primary score, in this order:

1. policy/quantity/price/request/fee evidence integrity;
2. P2-B has eligible submissions and accepted IOC orders, while P2-A submits
   zero aggressive rebalance orders; and
3. if fills occur, every fill is different-client, fee-positive, and reduces
   that submitting maker's absolute raw inventory.

A control submission, self fill, free fill, non-IOC remainder, absent local
book evidence, or cap breach is **INVALID**, not falsified. Price metrics,
terminal ratios, or lower pooled inventory never rescue the primary score.
Secondary descriptive rows are defer reasons, submissions/accepts/rejects/
fills/cancels, fees, filled/unfilled quantity, individual and pooled CDF maker
inventory, book viability, and executed-trade ratio by seed.

## Interpretation limit

An activated P2 establishes only an auditable, costly, capped,
local-information IOC risk-transfer action. It does not establish realistic
price elasticity, aggregate risk absorption, runaway control, or viable
long-run ecology. Those require later value/liability counterparties and
phase-robust longer experiments.
