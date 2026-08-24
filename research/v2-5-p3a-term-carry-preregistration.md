# V2-5 P3a — term-carry allocator integrity preregistration

Status: **preregistered before any P3a market world or P3a raw-evidence
inspection.** This is an actor-integrity screen for the new finite term-carry
allocator. It is not a test of funding anchoring, carry profitability, basis
convergence, price stability, or market realism.

## Question and A/B design

The historical `FundingCarryArbitrageur` has no realized ownership or unwind
term (C-001). P3 is a new participant: it can hold an offsetting spot/perp
position only after a locally delivered public book and funding observation
passes a declared, exact 12-funding-interval economic policy. It sends its
spot and perpetual legs as ordinary, bounded, non-atomic IOC orders and
retains any partial exposure for repair or eventual unwind.

```text
A: allocator installed; its local public feed, timer, account, policy,
   recorder, and evidence are active; submission policy is disabled.
B: identically installed allocator; submission policy is enabled.
seed: 107 (development); horizon: five simulated minutes; full retained logs.
```

The sole policy delta is `term_carry_allocator.enabled`. The participant has
no declared mandate deadline (`mandate_end_at_nano = 0`), so it never learns
the simulator end time. If B opens a matched pair, the five-minute run will
end before the 96-hour term completes; that is explicitly reported as an open,
terminal-censored term. No realized carry or basis outcome may be scored from
this screen.

## Fixed participant contract

Each venue has one independently funded treasury allocator with an ordinary
spot account and USD perpetual margin. It consumes only its own delayed public
`ABC/USD`, `ABC-PERP`, and `ABC-PERP` funding messages. Its public-feed delay
is 40 ms and its order-request delay is 20 ms (constant 20-ms link with
`market_data_scale = 2`). It does not read a global mark, index, funding value,
world stop time, PnL, or another venue.

The policy is fixed before execution at a 12 × 8-hour commitment, a
100,000,000 raw-ABC maximum position, 10,000,000 raw-ABC leg cap, 100,000
minimum order, 5-bps exchange taker fee on every leg, 500-bps annual long-spot
cash financing / short-spot borrow, and one basis point each for named
balance-sheet, margin-risk, leg-risk, and minimum-net-carry charges. A present
but insufficient expected rate defers as `NET_CARRY_BELOW_MINIMUM`; no fee,
funding-rate, clock, latency, maker, demand, capital, or population parameter
may be adjusted after this screen.

## Required evidence and hard gates

Each cell must retain non-empty final `greeks.json` and `latency.json` (the
only completion sentinels), raw venue JSONL, manifest, checkpoints, public-feed
receipt sidecars, and these independent artifacts:

```text
mvanalyze -metric termcarry -json
mvanalyze -metric observationreceipts -json
mvanalyze -metric streamhash -json
mvanalyze -metric evidenceartifacthash -json
mvanalyze -metric conservation -json
mvanalyze -metric positions -json
mvanalyze -metric orderlifecycle -json
```

The term-carry replay, receipt audit, conservation, positions, and order
lifecycle audit must parse and have zero contract failures. `latency.json`
must contain non-empty allocator rows with the declared 40-ms delivered market
data and 20-ms delivered request latency. Raw evidence is not prunable.

The disabled control must emit local policy evaluations but no submitted,
accepted, rejected, filled, or cancelled allocator order. The enabled arm must
show at least one ordinary accepted allocator order and at least one
fill-qualified leg. The activation target is a fully matched
`spot_position == -perp_position != 0` pair evidenced by a `TERM_ACTIVE`
transition, with no source, frontier, gateway, venue, arithmetic, lifecycle,
or position-continuity error. No matched pair is `NOT EXERCISED`; an invalid
or unmatched evidence chain is a failed actor-integrity screen. The expected
absence of an eight-hour funding settlement in five minutes is neither a pass
nor a failure.

## Interpretation fence

`B - A` can support only the statement that this declared finite treasury
policy reaches a locally informed, ordinarily admitted, independently
replayable matched spot/perp entry path in this development cell. It cannot
support that funding is a general price anchor, that the 12-interval
persistence belief is empirically valid, that carry is realized, or that basis
convergence emerged. P3b is prohibited unless P3a passes; P3b must observe one
actual funding settlement while the independently reconstructed term remains
active. P3c must then show exactly one close, zero terminal positions, and
conservation.
