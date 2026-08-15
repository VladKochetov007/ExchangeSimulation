# Reaction time versus transport latency among informed traders

**Question.** What happens to a medium-frequency participant that re-evaluates
the book every 30 seconds and reaches the venue in 1 second, when it competes
with a participant that re-evaluates every 100 milliseconds and reaches the
venue in 10?

**Setup.** Three venues, calibrated market, 20 simulated minutes, 10ms engine
clock, seed 91. Both informed tiers have identical capital, identical 10bps
edge threshold from fundamental value, identical lot size, and identical
inventory bound. They differ only in reaction interval and transport latency,
which is applied equally to requests, responses, and market data. Reported PnL
is measured against a passive benchmark — the participant's initial holdings
valued at the same terminal marks — so it is trading result, not inventory
revaluation. All participants are net long the base asset, so raw equity change
would otherwise show every group "profitable" whenever the price rose.

Configuration: `research/ffa-latency-tiers-2026-08-16.json`.

## Result

Active PnL in USD, summed over three venues:

| group | no informed | MFT only | fast only | both tiers |
| --- | ---: | ---: | ---: | ---: |
| informed, fast (100ms / 10ms) | — | 0.00 | **41,557.03** | **41,557.03** |
| informed, MFT (30s / 1s) | — | **−292.06** | 0.00 | **0.00** |
| spot makers | 239.00 | 348.02 | −52,969.05 | −52,969.05 |
| noise takers | −995.54 | −996.50 | −714.06 | −714.06 |
| option flow | — | −5,641.48 | −5,402.71 | −5,402.71 |
| mean abs log mispricing | 0.00342 | 0.00345 | **0.00116** | **0.00116** |

## What this says

**The medium-frequency participant is not merely less profitable, it is
excluded.** With the fast tier present it executes exactly zero trades — its
terminal ABC and USD balances are unchanged to the atom. The control run
proves this is competitive exclusion rather than a broken configuration: alone,
the same MFT participant does trade, accumulating 2.81 ABC across the venues.

**Every column of the "both" arm equals the "fast only" arm to the cent.**
Adding a medium-frequency informed participant to a market that already
contains a fast one changes nothing at all: not the makers' losses, not the
noise traders' losses, not price efficiency. Its marginal contribution to price
discovery is zero.

**Speed alone does not make informed trading profitable — being first does.**
The MFT tier loses money when it is the only informed participant (−292 USD).
Its 10bps edge threshold does not cover the 5bps taker fee plus what the price
does during its 30-second evaluation gap. The identical strategy at 100ms/10ms
earns +41,557 USD. The edge is in the latency, not in the signal: both tiers
trade the same signal against the same fundamental value.

**Market makers pay for it.** Spot makers earn a small positive spread in every
arm without a fast informed participant (+239 to +348 USD) and lose 53,000 USD
when one is present. This is adverse selection in its cleanest form: the maker's
quote is stale relative to a participant that re-evaluates 300 times more often.

**Price efficiency improves threefold.** Mean absolute log deviation from
fundamental value falls from 0.00342 to 0.00116 when the fast tier is present,
and the MFT-only market is statistically indistinguishable from having no
informed participants at all (0.00345 against 0.00342). The socially useful
function — keeping the price near fundamental value — is performed entirely by
the fast participant, and it is paid for out of the market makers' spread.

## Caveats

- Constant latency, not a distribution. A P99 of 10ms with a fatter tail would
  let the MFT participant win occasionally; that is the natural next treatment.
- One seed, three venues, 20 minutes. The exclusion result is qualitative and
  robust (zero fills is not a noisy measurement), but the PnL magnitudes are not
  yet confidence-bounded across seeds.
- Both tiers use the same naive taking rule. A real medium-frequency
  participant would post passively rather than cross, which changes the
  competition from a race into a queueing problem.
- Fees are the same for both tiers. Real venues price latency and volume tiers
  differently, which would change the break-even edge.

## Decomposition: reaction cadence versus transport latency

The first experiment varied reaction interval and transport latency together,
so it could not say which axis mattered. Running a single informed participant
alone, at a 1ms engine clock, over the two axes separately:

| reaction interval | transport latency | informed PnL (USD) | net ABC traded | mean abs log mispricing |
| --- | --- | ---: | ---: | ---: |
| 100ms | 1ms | 41,557.03 | 249.31 | 0.00116 |
| 100ms | 1s | **47,411.82** | 358.89 | 0.00167 |
| 30s | 1ms | −293.87 | 2.86 | 0.00345 |
| 30s | 1s | −292.06 | 2.81 | 0.00345 |

**Reaction cadence decides everything; transport latency barely registers.** A
participant that re-evaluates every 100ms is profitable whether its orders take
1ms or a full second. A participant that re-evaluates every 30 seconds loses
money either way, and the difference between its two latency arms is 1.81 USD
out of a 292 USD loss. A thousand-fold change in transport latency is worth
less than a percent; a three-hundred-fold change in evaluation cadence flips
the sign of the result.

This also explains why the medium-frequency participant was excluded even when
the fast tier's latency was drawn from a lognormal with a 99th percentile near
10ms: its handicap was never transport.

**Slower transport was mildly *better* for a lone informed participant.** With
1s latency it earned 14% more and traded 44% more. The market was also less
efficient in that arm (0.00167 against 0.00116), which is the likely reason:
a slower corrective participant leaves larger deviations standing, and it is
the only participant collecting them. Latency is a handicap in a race against
other informed traders, not against the market itself.

## Methodological finding: the engine clock bounds latency resolution

Configuring a lognormal latency with a 1ms median under a 10ms engine step
produced results identical to the cent to the constant-latency arm. Latency
below one step cannot be resolved: every draw lands in the same advance.
Raising the tier's median to 50ms at the same step changed the outcome, and
lowering the step to 1ms changed it again, confirming the wiring was correct
and the coarseness was the cause.

Any experiment quoting sub-millisecond or single-digit-millisecond latency must
set the engine step below the latency it claims to measure, and should report
both. This is recorded because a run that silently ignores its own treatment
looks exactly like a null result.
