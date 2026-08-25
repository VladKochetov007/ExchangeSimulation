# V2-5 P4 — immutable funding/carry numeric addendum

Status: **preregistered before P4 implementation, preflight, or outcome
inspection.** This addendum completes the numeric gate required by
[`v2-5-p4-funding-carry-causal-preregistration.md`](v2-5-p4-funding-carry-causal-preregistration.md).
It does not report a simulated result.

The economic simulator baseline is `d1555e191549be8679aed6b027aa054e5d64025e`.
That commit changes only a result-rendering scratch path and historical note;
`go test ./...` passes. Any later simulator-semantic change before P4 execution
invalidates these cells and requires a new preregistration. Analyzer-only code
may be added if its neutrality is proved and its revision is recorded.

## Sole intervention and why no ladder is used

P4 uses a paired A/B design. The only economic config difference is the
per-interval cap applied by the existing `SimpleFundingCalc` to its endogenous
public funding rate:

| arm | exact serialization | interpretation | class |
| --- | --- | --- | --- |
| A | `"funding_max_rate_bps": 1` | weak-funding control; the model's one-basis-point base component is also its smallest valid positive cap | C |
| B | `"funding_max_rate_bps": 75` | inherited V2/P3e funding cap | A |

Here, A/B/C mean inherited fixed V2 input, economically/exogenously motivated
input, and free experimental-design choice. The treatment value is inherited,
not selected to cross the carry threshold. The control is a declared weak
funding regime, not numeric zero and not “funding unavailable.” The calculator
still observes the ordinary positive-domain mark/index premium and can emit
`-1`, `0`, or `+1` within the cap.

No intervention ladder is necessary. Even before financing, the control's
maximum 12-interval funding magnitude is 12 bps, below the inherited four-leg
taker-fee budget (20 bps), fixed balance-sheet/margin/leg-risk charges (3 bps),
and required return (1 bp). B remains falsifiable: its actual delivered rate
is generated from its observed mark/index premium and need not make exact net
carry eligible.

The intervention changes neither mark, index, book, quote, target price,
participant capital, actor policy, nor order admission. Funding can affect a
market only through a delivered observation, the participant calculation,
target inventory, and ordinary orders/fills.

## Frozen numeric contract

| input or endpoint | exact value/rule | class and ex-ante rationale |
| --- | --- | --- |
| development paired seeds | 107, 109 | A: the completed P3e development pairs; this is a mechanism screen, not holdout validation |
| untouched holdout seeds | 127, 131, 137 | C: the first three unused prime seeds after the prior 113 timing holdout; fixed now and prohibited for debugging |
| run horizon | 98 simulated hours | A: retains the proven P3e entry, 96-hour term, exit, and post-deadline observation horizon |
| global funding interval | 28,800 s (8 h) | A |
| commitment | 12 funding intervals, at most one owned term per venue under the mandate | A |
| participant decision period | 2 s | A |
| maximum funding age | 10 s | A |
| mandate end | `1736035205000000000` ns | A |
| primary analysis cutoff | `1736038805000000000` ns | A: the proven P3e passive deadline; observations after it cannot enter the primary basis endpoint |
| terminal censoring | `1736042400000000000` ns | A: one hour after the primary cutoff; used only for lifecycle, residual, accounting, and evidence checks |
| initial spot capital per venue | 2,000 ABC (`200000000000` raw units) | A: fixed participant-construction balance |
| initial quote capital per venue | USD 200,000,000 (`20000000000000` raw units) | A |
| initial perpetual margin ledger | USD 100,000,000 (`10000000000000` raw units) | A |
| maximum target magnitude | 1 ABC (`100000000` raw units) per leg | A |
| child lot | 0.1 ABC (`10000000` raw units) | A |
| venue/actor minimum | 0.001 ABC (`100000` raw units) | A |
| taker fee assumption | 5 bps per leg; four expected entry/exit legs = 20 bps | A |
| long-spot cash financing | 500 bps/year over exact decision-to-term-end nanoseconds | A: inherited symmetric conservative financing assumption |
| short-spot asset borrow | 500 bps/year over the same exact horizon | A |
| balance-sheet charge | 1 bp per term | A |
| margin/liquidation-risk charge | 1 bp per term | A |
| latency/non-atomic-leg-risk charge | 1 bp per term | A |
| registered premium-realization income | 0 bps | A: premium selects direction but is not double-counted as certain profit |
| required net return | at least 1 bp using exact rational comparison | A |
| target rule | target spot is `+100000000` and perp its negative for independently positive long-spot/short-perp carry; signs mirror for negative premium/funding; otherwise target remains zero | A |
| P3e passive exit | slice `100000`, deadline `1736038805000000000` ns | A: unchanged validated execution contract |
| evidence | full JSON evidence, 30 s execution checkpoints, term-carry decisions, and participant-local receipt sidecars | A |
| request / market-data latency | 20 ms request; 40 ms delivered market data (`delay=20ms`, scale=2) | A |

All remaining population, maker, taker, liquidity, spread, tick, latency,
clock, option, dated-future, and router values are rendered explicitly in each
cell config from the completed P3e normalized manifest. No default is left as
an unrecorded experimental degree of freedom. In particular: three venues,
two spot makers per venue, one noise trader, one option-flow participant,
maker quote quantity `20000000`, ten-USD spot/perp tick (`1000000` quote raw
units), own-mid maker anchor, one-second step/snapshot/automation/quote clocks,
two-second noise clock, and no legacy carry, dated-carry, router, elastic,
fixed-distance, imbalance, triangle, or latent-liquidity population.

## Ordered activation and execution gates

The independent analyzer recomputes the exact break-even rate at each decision
from the delivered next-funding timestamp and rational holding duration. A
paired seed is activated only when:

1. both arms have valid delayed local spot, perp, and funding frontiers;
2. B's delivered funding differs from A in the registered direction;
3. A's exact net carry is below the 1 bp hurdle while B's exact net carry is
   at or above it; and
4. B changes target inventory in the corresponding signed direction while A
   remains at zero.

Failure of step 4 after steps 1–3 is `FALSIFIED AT ACTIVATION`. No fixed
funding magnitude is assumed to produce this crossing.

Execution requires, in each activated development seed, canonical admission
and at least one fill on both the spot and perpetual entry legs, followed by
an independently reconstructed matched exposure of at least one declared lot
(`10000000` raw units). A target, request, acceptance, one-sided fill, partial
or orphan position, or rejected order cannot satisfy this gate. Failure is
`FALSIFIED AT EXECUTION`.

## Frozen basis event study

For each treatment venue that completes the execution gate, event time `t0`
is the first independently verified B decision that changes the carry target
from zero under the exact net-carry crossing. The same venue and simulated
timestamp are applied to paired A; A cannot supply or move the clock.

At each whole simulated second, the analyzer reconstructs the latest canonical
two-sided spot and perp midpoints. Both observations must be no older than two
seconds and the positive-index ratio domain must be valid. Define:

```text
premium_bps = 10000 * (perp_mid - spot_mid) / spot_mid
direction   = sign(B target spot)
oriented_premium_bps = direction * premium_bps
```

The pre window is `[t0-30s, t0)` and the sole post window is
`[t0, t0+300s)`. Thirty seconds supplies 15 pre-decision periods without using
the simulation's initial feed bootstrap; five minutes supplies 150 actor
decisions and 300 quote updates while remaining far inside one eight-hour
funding interval. These are C choices fixed before outcomes.

Each arm/venue window is measurable only with at least 24 of 30 valid pre
samples and 240 of 300 valid post samples. An observed zero basis remains a
valid numeric result. Missing, stale, one-sided, zero-denominator, or
out-of-domain prices never become zero observations. Events lacking the full
window before the primary cutoff are censored and cannot be replaced by a
shorter window.

For each venue:

```text
delta_arm = mean(post oriented premium) - mean(pre oriented premium)
paired_convergence = delta_A - delta_B
```

The seed statistic is the equal-weight mean `paired_convergence` across its
execution-qualified measurable venues. This is the sole primary basis
statistic. Positive values have the registered convergence sign; zero is not
support. There is no post-hoc magnitude filter and no alternative horizon.
Every qualifying venue, including adverse effects, remains in the aggregate.

`SUPPORTED (screening)` requires the complete six-link chain and positive
primary paired convergence in both development seeds. Opposite seed signs are
`MIXED`; complete execution with no positive seed is `FALSIFIED`; missing
links or insufficient measurable basis follow the already registered `NOT
IDENTIFIED` rules. With two development pairs, no robustness or p-value claim
is permitted.

## Immutable cells and raw-evidence policy

All ten configs are fully rendered. Development uses A/B 107 and 109 only.
Holdout A/B 127, 131, and 137 may run only if development yields a complete,
identifiable mechanism eligible for promotion; no holdout cell may debug or
revise this protocol.

| cell | SHA-256 |
| --- | --- |
| A-107 | `c2e759b7828eeef968d4acfc44f0d4b7312a78cd8039b4bc7f449bc3a046d029` |
| B-107 | `271825ccd0441c73d18a7f0d60e2dfe5c356a82494765acf6a6ac4e6b187f20b` |
| A-109 | `a2503e6d963029e5c01533b125014eb163f67b48c4d8a867b8ec97ecde9b90dc` |
| B-109 | `6b4eb86916d4d4a0b8813058119fa0f5bf932021e7d30ac4bbf5eb1ba0546b9c` |
| A-127 | `cb20ae27119c1d4a523931ebdf305b79d39964120a3c639be4cc4a268cfe21e4` |
| B-127 | `83d0f736b5a491ae8067c82772958b213e332065623221f1de071e91def5716e` |
| A-131 | `31771a138f91aab3d346804bfa9752a7f9ef39fa3c50f1423c03eb65816ea79f` |
| B-131 | `3b2b45b3f8c14f260a14de794e3f2a390a59507307cad4b19b9d32758d8ded3c` |
| A-137 | `e485f53aeaf235eb29cdbe9972fe71a1c0e10fcf9b204c96985430b7c8952933` |
| B-137 | `116110210b3602d87bedf37d78547d169eee9e7d14839534f903f892b9c30677` |

`scripts/check-v2-5-p4-configs.sh` proves that each A/B pair differs
economically only in `funding_max_rate_bps`, and that same-arm configs differ
only by seed and provenance metadata. Completion requires final nonempty
`greeks.json` and `latency.json`; host process names are never sentinels.

Before scoring, every cell must pass receipt/frontier replay, exact P4
arithmetic, target/order/fill reconstruction, P3e lifecycle, generic order and
position replay, funding direction/duplication, conservation, ordered
execution hash, and runtime/offline exact evidence-artifact identity. Raw logs
and compact sidecars remain retained. No historical ae13f9a prune contract has
authority over P4.
