# V2-5 P2 design — finite physical-exposure perp hedger

Status: **P2a passed its narrow actor-activation screen.** The result is
recorded in [`v2-5-p2-perp-exposure-results.md`](v2-5-p2-perp-exposure-results.md).
The actor, local receipt-bound decision evidence, independent replay, evidence
mutations, and evidence-on/off fresh-process neutrality are committed through
`6013808`; P2a's implementation, analyzer, configuration, and verdict follow
in later commits. It follows the valid-but-unexercised P1a result; it does not
revise P0/P1a, change their costs, inject a funding rate, or authorize a P1b
market score.

## Observed constraint

P1a retained P0's local evidence contract but priced actual exchange fees. In
its fresh 30-minute seed-107 cell, expected funding was only 1–3 bps while an
ordinary four-leg entry/exit estimate was 20 bps before balance-sheet,
margin-risk, leg-risk, and minimum-return terms. Every potential action
explicitly deferred; zero carry inventory and orders mean an A/B basis screen
would be not identified.

The current retained population has a random multi-symbol taker but no
separately identifiable, economically motivated participant whose *perpetual*
exposure creates a durable price/funding signal. Making P1 trade by lowering
fees or directly increasing the funding rate would encode the desired result
and is prohibited.

## Hypothesis card

| field | P2 proposal |
| --- | --- |
| representation change | Replace anonymous directional perpetual activity with an externally auditable physical-exposure state and its contract hedge. |
| local mechanism | A finite-capital producer/consumer has a bounded net future ABC exposure. Its target perpetual position is the negative of that physical exposure. It observes only a delayed public `ABC-PERP` book and sends ordinary capped IOC hedges at its own local executable touch. |
| causal path | exogenous exposure change → signed desired perp inventory → locally priced request → actual fill/remaining hedge gap → book pressure → public mark/premium → public funding update. |
| forbidden shortcut | No actor reads a global mark/index/funding object; no actor writes or predicts a funding rate; no fair-price target, forced fill, fee subsidy, cross-venue transfer, or synthetic opposite leg is introduced. |
| predicted observable | In the enabled arm, each venue has independently replayable exposure changes and at least one accepted/fill-qualified perp hedge that reduces its own `abs(target_perp - filled_perp)`. Mark/funding may change, but is a secondary activation observable—not a success metric. |
| cheapest discriminating test | A 5-minute A/B × seed-{101,103} actor-integrity activation screen, with the actor installed, feed/timer/deposit path fixed, and only submit permission changing. |
| primary falsifier | Missing evidence, future/missing local book, side that increases hedge-gap magnitude, a non-ordinary order, hidden rate access, or no accepted hedge in an enabled paired seed. No accepted action is `NOT EXERCISED`, not a price claim. |
| likely breakpoint | Makers absorb every finite hedge and the funding mark stays near the index. That falsifies signal activation in this population; it does not justify increasing a funding rate post hoc. |

## Proposed actor contract

The implementation must be a new `PerpExposureHedger`, not a silent extension
of the CDF/USD L0 actor or a new mode in its historical analyzer. This keeps
P0–V2-4 trajectories and their exact replay contracts intact.

The actor has one venue-local USD-margin account and one named external
`physical_exposure` state in raw ABC units. Its state follows an explicitly
bounded reflected random walk derived from a declared per-venue deterministic
seed. A positive value represents future physical long exposure (producer
inventory), hence a negative target perpetual position; a negative value
represents future physical short exposure (consumer liability), hence a
positive target perpetual position. The physical ledger is a motive only: it
is not an internal cash transfer and does not count as a perpetual fill.

At every separately configured decision interval it computes, with checked
signed arithmetic:

```text
target_perp = -physical_exposure
hedge_gap   = target_perp - filled_perp_position
request_qty = min(abs(hedge_gap), declared request cap)
```

It submits an IOC at its last **locally delivered** required `ABC-PERP` touch:
BUY requires a local ask, SELL a local bid. The account pays ordinary venue
taker fees and uses ordinary margin admission. A rejected, partial, or
cancelled IOC stays a named, observable hedge gap. There is no spot leg: this
actor hedges a genuine physical exposure, whereas the later funding-carry
participant deliberately holds offsetting spot/perp legs.

The initial activation configuration must state before implementation:

| field | declared initial value | reason |
| --- | ---: | --- |
| decision interval | 2 s | matches the P0 desk observation clock without changing it |
| exposure-update interval | 10 s | exposes state transitions while remaining distinct from decision cadence |
| feed / request latency | 40 ms / 20 ms | same separated public/execution relation as V2-4 L0; no direct link |
| target step | 10,000,000 raw ABC | one-half of the retained P0 maker quote size; stated before an outcome run |
| absolute target bound | 100,000,000 raw ABC | finite five-step balance sheet |
| request cap | 10,000,000 raw ABC | one state increment and no forced sweep |
| account | explicitly prefunded USD margin, recorded in config | normal derivative risk admission, not a reserve exemption |
| fee | existing configured 5 bps | must remain equal to exchange admission fee |

The step is deliberately smaller than the retained 500,000,000 raw ABC spot
maker quote and is not a funding-rate target. If it produces no executable
hedges, that is activation information, not permission to increase it within
the registered screen.

## Evidence, independent replay, and mutations

The actor must persist append-only, evidence-only `perp_exposure_hedger_decision`
and fill records. Every decision includes named policy/action, venue/client,
symbols, exposure before/after/step/bound, filled position, target, signed
gap, required touch availability and values, local book source identity,
decision frontier, declared fee, request fields, and terminal-censor reason.
`side` must be a named string, never a zero-valued enum omitted by JSON.

The independent analyzer must be separately written against raw decisions,
V2 receipt sidecars, scalar gateway decisions, venue outcomes, fill events,
and immutable config. It must replay the reflected physical-state stream,
target/gap/side/cap/touch, local delivered receipt, ordinary fee, request
chain, and actor-local fill transition. It must not import the actor package.

Required adversarial fixtures before a market cell:

1. reverse the target sign and prove the replay rejects the side;
2. drop or duplicate a physical exposure update;
3. inject a future/delayed/duplicate/reordered book observation;
4. convert a missing required touch to numeric zero;
5. exceed the request cap or use an off-touch price;
6. drop an IOC cancellation or forge a self/free fill; and
7. turn evidence on/off across fresh processes and `GOMAXPROCS=1/4`, requiring
   the same execution hash.

## Registered next experiments, contingent on implementation checks

### P2a — actor-integrity screen

```text
A: actor installed, state/feed/evidence active, submission disabled
B: same actor, state/feed/evidence active, submission enabled
seeds: 101, 103; horizon: 5 minutes; full raw evidence
```

The sole economic delta is submit permission. P2a scores evidence integrity,
state activation, ordinary accepted/fill-qualified hedges, and individual gap
reduction. It explicitly does not score a basis, funding benefit, stability,
or price realism.

### P2b — only after P2a passes and signal readiness is measured

Before a P2b market screen, retain and independently extract P2a's public
mark/funding observations under the separate
[`P2 signal-readiness preregistration`](v2-5-p2-signal-readiness-preregistration.md).
If those observations show no usable variation, P2b is halted as `NOT
IDENTIFIED`; a funding-carry desk cannot respond to a signal that is absent
from its public feed. Passing that readiness screen establishes only that the
public signal is present. It does not revive P1a: P1a's costed four-leg policy
still has zero inventory and orders. A separately designed viable carry
participant must first pass its own local-economic activation gate. Only then
may a preregistered 2×2 compare physical-exposure hedgers off/on with that
active policy off/on. Score in order: actor activation; local expected-rate →
net-carry → inventory → order chain; then paired signed-basis and
funding-response metrics. A price/basis difference without the desk's actual
inventory/order change is not a funding attribution.

## Explicit limitations

P2 does not repair the whole-bps carry calculation. P1a shows that the
annual-borrow term rounds to zero at an eight-hour horizon, but fractional
resolution alone cannot overcome the observed 20-bps execution-fee hurdle.
Any change to money/carry units must therefore be a separate representation
slice with arithmetic fixtures and positive-world equivalence, not a hidden
parameter adjustment in P2.
