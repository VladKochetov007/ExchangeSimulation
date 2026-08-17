# What request budgets do to a market

Findings from wiring published exchange request limits into the simulator and
measuring what they change. Grounded in what exchanges publish
(`research/exchange-mechanics-reference-2026-08-16.md`); run on the profiled
reference configuration (`research/reference-configuration-2026-08-17.md`).

## The result that survived

**A request budget costs a strategy trades in proportion to its refusal rate
when its intent expires, and costs it nothing when its intent survives to be
retried.**

Measured on fills, across three seeds, on the dated carry desk:

| seed | refused | fills lost |
|---|---|---|
| 91 | 37.4% | 20.5% |
| 92 | 38.9% | 42.5% |
| 93 | 35.3% | 35.9% |

Against metaorder execution, whose intent persists: no fill loss at 5.4%
refusals, and none at 78.9% in a separate configuration. A parent order still
needs filling a second later, so a refusal costs it an attempt. A carry desk
refused while a dislocation exists cannot retry into the same opportunity, so a
refusal costs it the trade.

Direction is consistent across three seeds. Magnitude is not: fills lost run
between 0.55 and 1.09 times the refusal rate.

## Why it had to be measured on fills

Payoff cannot answer this question. Both candidate classes lose money
unthrottled, and refusing a request from a negative-edge strategy prevents a
losing trade: a 35.3% refusal rate *improved* the dated carry desk's result by
26.2%. Payoff therefore measures the sign of a strategy's edge rather than the
effect of the limit.

Two earlier experiments in this line were withdrawn for exactly this. A third
was withdrawn because tier assignment depended on Go's map iteration order, so
budgets bound on some seeds and not others for no reason at all.

## What budgets do to makers

Nothing, once makers quote sensibly. A maker with a requote threshold sees zero
refusals under budgets that refuse takers 20 to 79 percent, because the
threshold cuts its request rate from 43,340 to between 76 and 222 over two
hours.

Without a threshold, no budget is compatible with market making: a requote is a
cancel and two placements, so any budget tight enough to bind stops a maker
restoring one side of its quote and the book empties. That is the same
synchronised-quoting failure recorded as FFA-83, arriving from a third
direction.

## Two practical findings about limit design

**A budget below the cost of a market-data subscription blinds a participant
rather than throttling it.** A per-minute weight budget of one against a
subscription costing two left a desk with twelve requests for an entire run, all
refused as impossible. It never subscribed, so it never saw a book. The symptom
is an inactive strategy, which reads as a population property rather than a
misconfiguration.

**Cancels must be free against order-count budgets.** A throttled maker that
cannot withdraw its quotes is trapped behind them. This is why the two-lane
admission queue exists and why placements are metered while cancels are not.

## What was built

- `ratelimit`: composable cost models, fixed-window and token-bucket limiters,
  a two-lane admission queue that keeps risk-reducing requests in the lane that
  saturates last, and per-connection stream limits. Seventeen defects were found
  across three adversarial reviews, five of them introduced while fixing
  earlier ones.
- `exchange.RequestPolicy`: the seam where a venue decides whether to accept a
  request, with `RATE_LIMITED` and `OVERLOADED` as distinct reasons because they
  tell a client different things.
- `actor.RequestBackoff`: honours the venue's retry advice, and only for
  refusals the venue asked us to wait on.
- Per-class budget tiers in the scenario, and per-class admission telemetry
  without which a payoff table cannot distinguish a class that declined to trade
  from one that was refused.

## What remains open

The magnitude of the fill loss, which spans a factor of two across seeds. A
second expiring-intent class, since one desk currently stands for the whole
category. And whether budgets change *which strategies survive* rather than how
much they trade, which needs a profitable expiring-intent participant the
population does not yet contain.
