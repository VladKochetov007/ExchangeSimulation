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
