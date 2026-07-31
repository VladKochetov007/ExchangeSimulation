# Lab notes: making latency races real (and the bugs found on the way)

Working notes for the experiment that turns the gen-6 negative result — four
arbitrageurs with a twentyfold speed spread capturing identical profits —
into a measurable race. Kept as a running log of what was tried, what the
data said, and which detours turned out to be bugs, because the negative
results and the wrong turns are the parts that are expensive to rediscover.

## The hypothesis

Gen-6 concluded that polling actors cannot express a latency race: a 100 ms
decision timer swamps any network latency difference, so reaction time is
dominated by ticker phase rather than by delivery time. The prediction that
follows is sharp — if decisions are moved from the timer into the event
handler, reaction time becomes delivery latency, and profit should order by
speed.

Two arms, identical in every other respect:

- **control (polling)** — `RaceArbReactive: false`, decisions on the 100 ms ticker
- **treatment (reactive)** — `RaceArbReactive: true`, decisions inside `HandleEvent`
  on every trade print and snapshot

Four entrants per arm at latency multipliers 1.0, 0.5, 0.2, 0.05 against the
same spot/perp pair, so relative capture measures race concentration.

## Result 1: the control replicates gen-6 exactly

Final positions across the 20× speed spread: 171, 177, 170, 174. No ordering,
no advantage to the fast entrant. The mechanism claim from gen-6 is confirmed
rather than assumed — worth stating, because it is the baseline the treatment
has to beat, and because a control that failed to reproduce would have
invalidated the whole comparison.

## Result 2: the treatment exposed a bug, not a result

Every reactive entrant reported a final position of exactly 5000 — the
configured `MaxPosition` — while actually holding around 50 ABC, roughly a
tenth of that. The counter was incremented on **submission**:

```go
a.SubmitOrder(...)   // perp leg
a.SubmitOrder(...)   // spot leg
a.position++         // regardless of what actually filled
```

A market order into a thin book fills partially or not at all, so believed
inventory drifts from real inventory without bound and `MaxPosition` stops
being a risk limit. Polling hid this: at 100 ms cadence there is time for
fills to land between decisions, so intent and reality stay close. Reactive
firing is exactly the regime that breaks it.

This is the same bug class as the randomwalk postmortem's ghost orders —
believing a request is a fact. Fixed by deriving position from perp-leg fills
and adding an in-flight counter, so a burst of decisions inside one latency
window cannot stack orders past the cap before the first fill reports back.

Blocked on a library gap: `actor.OrderFillEvent` dropped the `Symbol` the
exchange already sends, so no multi-leg strategy could tell which leg filled.
Order-ID lookup does not substitute — market-order fills arrive *before* the
accept response, so the ID map is not yet populated when the first fill
lands. Symbol added to the event.

## Detour: a conservation "violation" that was my own measurement error

Checking whether the reactive arm's much larger flow broke accounting, a
whole-ecology test appeared to show USD being created — about 2.7 million
units in the reactive arm, thirteen times the polling arm. Chasing it
produced a genuine find and a genuine mistake, in that order.

**The mistake.** Total cash is not the conservation invariant while positions
are open. Probing trade by trade showed the exact arithmetic: on one partial
close the system gained 1,064,158 units, which decomposes as a +5,064,158
realized profit for the closer against its averaged entry minus a −4,000,000
realized loss for the counterparty against theirs. That is correct
accounting. The offsetting loss sits *unrealized* in the other
counterparties' still-open positions and only becomes cash when they close.
The invariant is

```
cash − Σ (size × entry / precision)
```

and it must be evaluated with **each instrument's own base precision** — this
ecology mixes 1e8 and 1e6 assets, and a shared constant mis-scales every
position on the smaller-precision book. That single error accounted for most
of the apparent leak (616,429 units → 460 once corrected). Under the correct
invariant the ecology conserves every asset to a few hundred units of
rounding dust across tens of thousands of fills. The engine was fine.

**The genuine finds**, both of which the detour was worth:

- Entry-price averaging ran through `float64`. Its 53-bit mantissa cannot
  hold `size × price`, which reaches ~5e17 at these precisions, so the basis
  was quantized to roughly 64 units — and basis error is money. Replaced with
  exact 128-bit arithmetic.
- `MidPriceOracle.Price` read a book returned by `GetBook` *after* that lock
  was released, racing every order that mutates it. The new ecology test is
  the first to run full feesim automation under the race detector, which is
  why it had never surfaced. Fixed with an optional `MidPriceProvider`
  upgrade so the public `BookProvider` interface is unchanged.

**And a test-harness defect** that had been silently corrupting results:
`InjectMarketOrder` hard-coded request ID 1000000 and returned on the first
successful response of any kind. Once responses became asynchronous, it could
match a *previous* order's message and return before its own order was
processed — so a test could read exchange state one trade stale. Both
injection helpers now use unique IDs and match strictly. Any market-order
test written between the async-outbox change and this fix should be treated
as suspect.

## Correction: the capture metric was measuring the wrong thing

Everything in Results 3 and 4 below was computed from end-of-run wallet
wealth, and that measure is wrong for this strategy. A basis arb holds long
spot against short perp. The spot leg is a *balance*; the perp leg is a
*position*. A wallet-only view counts the first and not the second, so pure
inventory accumulation reads as profit — and since faster entrants accumulate
faster, the metric manufactured exactly the correlation it was supposed to
test.

Recomputing from per-client fill and position events, with the perp leg
valued at its unrealized mark, gives three separate answers:

| metric | what it asks | reactive, 5 seeds |
|---|---|---|
| volume (filled qty) | who won the races | **+0.80** |
| net (realized − fees) | economics of closed round trips | **−0.68** |
| equity (cash + spot + perp unrealized) | total economic P&L | **+0.00** |

**The mechanism claim survives and is strengthened.** Fill capture orders by
speed with rank correlation +0.80 across five seeds under reactive decisions,
against noise under polling. Reaction time became delivery latency, and the
fast entrants win more races. That is the result the experiment was built to
test, and it holds on the metric that measures it directly.

**The profit claim does not survive.** Total economic P&L has no relationship
to speed at all — mean rank correlation 0.00, individual seeds scattered from
−0.80 to +0.80. The "1.25× capture" in the table below is spot-leg inventory,
which is volume wearing a dollar sign.

**And the reason is economically interesting.** Realized P&L on completed
round trips is small and negative (about −$400) while fees are one to two
orders of magnitude larger (about $5,000–$10,000), and fees scale with
volume. So the faster entrant wins more races, pays proportionally more in
taker fees on both legs, and ends up no better off. Speed buys volume, not
profit, at these fee levels.

That reframes the saturation in Result 4 as well: there was never a
saturation of profit to explain, because there was never profit ordered by
speed. On volume there is a mild flattening between the 0.2× and 0.05× tiers;
on equity there is no signal to saturate.

The obvious follow-up, and a much better experiment than the inventory sweep:
**sweep the fee level and find where speed starts paying.** If the marginal
race won is break-even at 8bps spot plus 5bps perp, there is some fee level
below which speed becomes profitable, and the shape of that boundary is
exactly the market-design question — it connects directly to why real venues
compete on fee schedules and why the Budish–Cramton–Shim argument is about
rents rather than volumes.

`scripts/race_capture.py` computes all three metrics; `race_summary.py` is
kept only because it reproduces the flawed wealth view that the tables below
were built on.

### The fee sweep does not rescue it, and leg risk is why

Sweeping the taker fee on both legs, three seeds each, equity metric:

| fee (spot/perp) | speed → profit rank-corr |
|---|---|
| 0 bps | +0.20 |
| 2 bps | +0.27 |
| 8/5 bps (baseline) | +0.00 |

No level shows a speed effect, and all three are within noise of each other.
The reason is not fees at all. Measuring each entrant's actual net delta —
spot inventory against perp position — shows the strategy is **not
delta-neutral in practice**:

```
client 12: spotABC=  54.96   perpPos= -45.56   net_delta=   9.40   (lot=0.10)
client 13: spotABC=  50.16   perpPos= -58.57   net_delta=  -8.41
client 14: spotABC=  61.94   perpPos= -66.00   net_delta=  -4.06
```

Between 17 and 94 lots of unhedged directional exposure, roughly a tenth of
gross position. At $50,000 an ABC that is on the order of half a million
dollars of naked delta, so a one percent move swings P&L by about $4,700 —
the same magnitude as the entire equity figure being measured. The arb edge
is buried under leg risk an order of magnitude larger.

The cause is structural, not a bug: the strategy fires two market orders and
assumes both fill completely. Into a thin book they fill partially and by
different amounts, and nothing ever reconciles the residual. That is a
faithful reproduction of a real problem — leg risk is exactly why production
basis arbs hedge the residual, cap size to available depth, or quote one leg
rather than crossing both — but it means this strategy cannot answer a
question about profit until the residual is managed.

So the honest chain is: reactive decisions make speed win races (robust,
+0.80 on volume, five of five seeds); speed does not translate into profit
here, because fees scale with the volume won and because leg-risk noise
dominates what is left; and no profit claim of any kind is measurable from
this strategy until it hedges its residual delta. That last item is the real
prerequisite, and it is a more valuable piece of work than the fee sweep —
a `residual delta` hedge turns this into a strategy whose P&L is actually
about basis rather than about direction.

## Result 5: hedging the residual partly uncovers the profit signal

`HedgeResidual` flattens leftover delta on the perp leg, which needs only
margin rather than spot inventory. The first implementation barely helped,
for a reason worth recording: it did not account for its own orders in
flight, so every subsequent fill re-measured a residual that was already
being corrected and fired another hedge. The strategy overshot and the
exposure flipped sign rather than closing. Tracking pending hedge quantity
and netting it into the residual fixed that.

Even corrected, post-hoc hedging is only partly effective:

| arm | mean abs net delta | volume corr | equity corr |
|---|---|---|---|
| unhedged | 4.43 ABC | +0.80 | +0.00 |
| hedged | 3.00 ABC | +0.84 | +0.44 |

A 32% reduction in naked exposure, and the speed-to-profit correlation moves
from nothing to +0.44 while the volume correlation is unchanged. That is
consistent with the leg-risk explanation — as the directional noise falls,
the profit signal starts to show through — but it is five seeds with equity
still ranging from −$2,500 to +$70,000 across them, so it is directional
evidence rather than a result. Three ABC of residual is still about $150,000
of naked delta, and a one percent move is still $1,500.

**Why post-hoc hedging cannot finish the job.** The residual is regenerated
continuously: every pair of market orders fills asymmetrically, and the hedge
is itself a market order that also fills asymmetrically. It is chasing its
own tail, which is why a third of the exposure is the best it manages.

The structural fix is **sequential legging** — send one leg, wait for the
actual fill, then send the second leg sized to exactly that fill. Residual
then arises only from the second leg's own partial fills rather than from the
mismatch between two independent ones. It is also how production arbs work,
and it is more interesting for a latency race rather than less: the race
becomes about who completes *both* legs first, and the second leg's latency
becomes part of the edge. That is the next experiment, and it should be
judged by whether the equity correlation converges toward the volume
correlation as residual approaches zero.

## Result 6: sequential legging made it worse, not better

Implemented and measured, five seeds each:

| execution style | mean abs net delta | residual as share of gross |
|---|---|---|
| simultaneous | 4.43 ABC | ~3% |
| simultaneous + hedge | 3.00 ABC | ~3% |
| sequential + hedge | 22.42 ABC | ~27% |

The prediction was wrong by a factor of seven. The mirror leg does fire — the
spot position moves in the right direction — but it systematically fills
about a quarter short of the perp leg it is mirroring, and the spot-side
hedges that are supposed to clean that up under-fill for the same reason.

Cutting the lot size tenfold changed almost nothing (27% → 25%), which rules
out per-order depth: with reactive decisions the strategy simply makes ten
times as many decisions, so aggregate flow is set by the opportunity rather
than by how it is sliced.

So the honest reading is that sequential legging trades one risk for another.
Firing both legs at once risks the two fills mismatching. Legging in
sequentially risks the second leg *chasing* — it is a market order sent after
a latency delay into a book that has already moved, and repeated one-sided
pressure depletes the side it needs. Neither is delta-neutral; they fail
differently.

**A confound worth stating rather than explaining away.** The two arms ended
up trading in opposite directions — the simultaneous arm was net short spot
and long perp, the sequential arm the reverse. Changing the execution
sequence changes the trade path, which changes prices, which lands the
strategy in a different basis regime. So part of the difference between 3%
and 27% may be a book-side asymmetry rather than the execution style, and the
comparison is not as clean as the table makes it look. Controlling for
direction is a prerequisite before treating this as a settled result.

**The design this points at.** Real desks do not solve legging risk by
crossing the second leg faster; they post it. The second leg should be a
*limit* order at a price that still preserves the edge, with a deadline —
and if it does not fill, the first leg gets unwound rather than paid up for.
That converts an uncontrolled directional residual into a bounded, decided
cost, and it makes the latency race sharper rather than softer: the fast
entrant wins because its resting second leg is already in the queue when the
opportunity appears.

## Result 3: reactive decisions produce a real race (5 seeds)

With position tracking fixed, both arms rerun across five seeds. Capture is
each entrant's wealth change (cash plus inventory marked at its nominal
level) relative to the slowest entrant; the headline statistic is the rank
correlation between speed and capture, because strict monotonicity across
four tiers is too brittle a test on five seeds.

| arm | 1.0× | 0.5× | 0.2× | 0.05× | mean rank-corr |
|---|---|---|---|---|---|
| polling | 1.00 | 1.06 | 1.02 | 0.96 | **+0.12** |
| reactive | 1.00 | 1.05 | 1.25 | 1.20 | **+0.72** |

The per-seed correlations are what settle it. Polling: −0.80, −0.60, +0.80,
+0.80, +0.40 — the sign flips, which is what a coin looks like. Reactive:
+0.80, +0.80, +0.60, +0.60, +0.80 — positive in **five seeds out of five**.

So the gen-6 conclusion holds and is now mechanistic rather than
observational: the tie was caused by the decision cadence, not by anything in
the latency model or the exchange. Move the decision into the event handler
and the same latency spread, the same agents, and the same market produce a
speed-ordered outcome.

## Result 4: the advantage saturates, and that is the interesting part

Capture does not keep rising with speed. Mean capture peaks at the 0.2× tier
(1.25) and *falls back* at 0.05× (1.20), and the gap between 1.0× and 0.5× is
almost nothing (1.05). The effect switches on somewhere between 0.5× and
0.2× and then flattens — being twenty times faster is worth no more than
being five times faster.

Leading hypothesis: the winner throttles itself. The accumulate threshold
scales with `|position|/MaxPosition`, so the entrant that wins the early
races builds inventory, raises its own bar for the next trade, and hands the
marginal opportunity back to slower entrants. If that is the mechanism, the
saturation point is an inventory-risk artifact, not a latency one — and
raising `MaxPosition` should let the fastest tier pull away.

That is a cheap, decisive test (`RaceArbMaxPosition`, now configurable), and
it is worth running before drawing any conclusion about diminishing returns
to speed, because the alternative explanations — market-data granularity
binding before the network does, or the opportunity being exhausted within
one latency window regardless of who takes it — imply very different things
about what a latency race in this simulator actually measures.

### The inventory hypothesis is not supported

Ten times the inventory cap, seven seeds:

| cap | 1.0× | 0.5× | 0.2× | 0.05× | mean rank-corr |
|---|---|---|---|---|---|
| 5 000 (baseline) | 1.00 | 1.05 | 1.25 | 1.20 | +0.72 (5/5 seeds positive) |
| 50 000 (10×) | 1.00 | 0.92 | 1.03 | 1.06 | +0.40 (5/7 positive) |

If self-throttling were the mechanism, relaxing the cap should have let the
fastest entrant pull away. It did the opposite: the speed signal got *weaker*
and much noisier, with two seeds going negative and one entrant landing at
0.39 — a worse fit than the baseline in every respect except that the
nominal peak moved to the fastest tier.

An interim three-seed read of this same sweep looked like confirmation
(peak on the fastest tier, two of three monotonic). It did not survive four
more seeds. Recording that explicitly because the premature read is the
error worth remembering: three seeds of a four-point ranking is not enough
to distinguish a mechanism from a coin, and the sweep was cheap enough that
there was no reason to read it early.

The likely reason the high-cap arm is noisy also undermines it as a test: at
ten times the cap, entrants carry far larger inventory, so the wealth measure
stops being dominated by arb capture and starts being dominated by
mark-to-market on a directional position. The 0.39 seed is an entrant sitting
on a large losing position, not one that lost a race. Any rerun of this test
needs capture measured from round-trip arb P&L rather than end-of-run wealth.

So the saturation stands unexplained. The remaining candidates — market-data
granularity binding before the network does, or the opportunity being fully
consumed within one latency window regardless of who wins it — are both
testable, and both need a cleaner capture metric first.

## Method note

The trade-by-trade probe was the turning point. Asserting conservation at the
end of a run says only that something is wrong; instrumenting after every
individual trade and printing the deltas of both counterparties named the
mechanism in one run. Worth reaching for earlier next time an aggregate
invariant fails.

## Open

- Per-tier profit attribution for the reactive arm, which is the actual race
  verdict. Final position is not the measure — capture is. Trade events carry
  `client_id = 0` because the logger is symbol-scoped, so attribution has to
  go through per-client fill events.
- Whether reactive entrants ordered by speed at all, and if so how
  concentrated the capture is (the Aquilina–Budish–O'Neill comparison).
