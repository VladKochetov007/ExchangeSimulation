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
