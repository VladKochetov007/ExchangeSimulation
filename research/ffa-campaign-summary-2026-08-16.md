# FFA ecology campaign: what was found

Sixty-seven hypothesis records and fifty experiments, on branch
`autoresearch/ffa-ecology-gen0`. This is the consolidated reading; the ledgers
`ffa-ecology-hypotheses-2026-08-15.jsonl` and
`ffa-ecology-experiments-2026-08-15.jsonl` remain the record of what was
actually run, including the results that were withdrawn.

## The question and the answer

The campaign asked whether a many-strategy market behaves as an organism or
degrades to a single winner, and whether strategies stand in a non-transitive
relation. The answer this simulator gives, at twelve simulated hours:

- **No rock-paper-scissors in opponent space.** Carry arbitrage dominates
  execution, which dominates informed trading, in every opponent mixture
  tested. No strategy's sign reverses against any opponent (FFA-48).
- **But dominance does reverse in time.** Market makers beat carry
  arbitrageurs at three, six and seven hours and lose to them at nine and
  twelve, in the same population with the same seed. The cause is the venue's
  eight-hourly funding settlement rather than any gradual convergence: with
  funding suppressed the reversal disappears and carry loses at every horizon
  (FFA-47, corrected). Halving the funding interval halves the crossover, which
  moves to between three and five hours (FFA-60), so which strategy looks
  dominant is a choice the venue makes when it sets its settlement frequency.
  No ranking in this campaign is meaningful without its horizon.
- **The ecology is stabilised by congestion.** Per-participant carry returns
  fall roughly as the inverse of density, and total extraction is hump shaped:
  it peaks near 288 participants at about 341,000 US dollars per twelve hours
  and collapses to 211,000 at 576. Beyond the peak, crowding dissipates the
  resource rather than transferring more of it from the makers (FFA-49,
  FFA-50).

So: a hierarchy in opponent space, a reversal in horizon, and a carrying
capacity. Coexistence comes from negative frequency dependence, not from a
cycle.

## What the population looks like when it works

Reaching a market that was simultaneously accurate, stable, risk-bounded and
capable of impact took most of the campaign, and each property turned out to
depend on a specific participant or mechanism:

| requirement | what supplies it | evidence |
| --- | --- | --- |
| price stays near value | an exogenous or consensus reference the makers cannot move | FFA-22, FFA-23 |
| price can be moved at all | maker reference partly weighted on its own book | FFA-35, FFA-36 |
| maker inventory stays bounded | displayed depth large enough for a delta-neutral participant to absorb | FFA-33 |
| a basis exists | makers pricing their inventory into their quotes | FFA-31, FFA-32 |
| risk leaves the makers | hedging into a second instrument, and someone delta neutral to take it | FFA-29, FFA-30 |

The counterintuitive ones are worth stating directly. Anchoring makers to the
*true* value is worse than anchoring them to a consensus of their own
midpoints, because pinning the price removes the excursions through which
makers shed inventory (FFA-24). And hedging does not remove risk from a closed
population; it concentrates it in whoever does not hedge (FFA-30).

## A correction worth reading before the tables

The cross-asset markets were unanchored for most of the campaign: the venue
published an index only for the main pair and the perpetual, so the CDF and
cross makers quoted around their own midpoint and drifted (FFA-51). Every
cross-pair maker figure reported before that fix is void, including the
140,431 US dollars per member that briefly made it the best strategy here; it
was a drifting book, not an edge. After publishing an index per symbol the same
class loses money on all three seeds.

The headline results were re-run on the fixed engine and all of them hold:
option dealers around 240,000 per member, carry arbitrage around 31,000, option
flow around minus 105,000, spot makers around minus 50,000, and the horizon
reversal unchanged at 9,318 against minus 5,035 at three hours and minus 44,687
against 30,744 at twelve (E-113).

## The transfer this market runs on

The largest flows in the ecology are one chain, measured at every link:

**perpetual maker's inventory skew -> funding rate -> carry participants ->
extraction from spot makers**

- The perpetual maker is persistently short, because it absorbs one-sided
  derivative flow, and prices that risk with a skew. The resulting premium is
  `skew * min(|inventory|/limit, 1) / indexWeight`, verified at four anchor
  weights within half a basis point and in both the saturated and proportional
  inventory regimes (FFA-55).
- That premium sets the funding rate. Carry participants collect it: 44,054 US
  dollars per member over twelve hours at the default settings.
- Their net result is that funding minus a persistent execution and mark loss.
  Suppressing funding leaves them with exactly that loss, minus 12,746 per
  member against minus 12,821 predicted, while trading the same basis and the
  same number of fills (FFA-59).
- The spot makers pay. They lose 61,805 per member while carry is profitable
  and earn 27,251 when funding is suppressed, or 2,850 when only the perpetual
  maker's risk budget is loosened (FFA-57).

So "market making is unprofitable here" is not a property of market making. It
is the other end of this transfer, and it reverses when the transfer is turned
off. Carry arbitrage is likewise not an arbitrage: it is a funding carry
financed by an execution loss.

## Literature validation

| regularity | result |
| --- | --- |
| long-memory order flow with diffusive prices | reproduced: sign autocorrelation 0.33 to 0.05 over three decades of lag, variance ratio within 5% of one (FFA-34) |
| square-root impact | supply curve gives V(x) ~ x^2.04, implying an exponent of 0.48; metaorders at the right scale give 0.52 to 0.61 (FFA-40) |
| spread proportional to volatility per trade | falsified, with an exact analytic account: the spread is the constant fill-decay term of the control law (FFA-16) |
| latent liquidity as the source of the square root | falsified here: the exponent survives removing the latent population entirely, and its prefactor prediction fails (FFA-41, FFA-42, FFA-43) |
| impact independent of participation rate | untestable at present power: the estimator's seed spread is an order of magnitude larger than the effect (FFA-44, FFA-45) |

The most important of these is the fourth. The exponent near one half is
produced here by makers mean-reverting toward a reference, not by latent
liquidity, so reproducing it is weak evidence for the mechanism it is usually
attributed to.

## Engine defects found by experiment

These were all found because a measurement looked wrong, not by reading code:

- Spot market orders could not use enabled margin, silently disabling every
  option-dealer hedge (FFA-05).
- `Subscribe` replaced a client's feed list instead of adding to it, so a maker
  asking for trades lost its snapshot feed and froze its forward. This
  invalidated a calibration and a falsification, both of which were re-run
  (FFA-17).
- Hedge orders were rejected for prices off the tick grid; 1,218 attempts
  produced zero fills and no error anyone would notice, because a rejection is
  not a fill (FFA-28).
- Maker inventory entered the control per unit rather than as a fraction of a
  risk budget, so a large position produced an arbitrarily large skew.

## Methodological rules the campaign had to learn

Each of these cost at least one wrong result:

1. **Measure the market's own scale before choosing experiment parameters.**
   Metaorders of one unit in a market where one basis point costs forty-five
   units produced first "no impact" and then "superlinear impact". Both looked
   like clean measurements (FFA-38, corrected FFA-37).
2. **A swept parameter that changes nothing is a bug report.** Participation
   rate, informed-trader count and value-trader exits each turned out to be
   capped by something else. Three separate occasions (FFA-20, FFA-44).
3. **Latency below one engine step is invisible.** A treatment smaller than the
   simulation step reproduces the control exactly (FFA-15).
4. **Prefer the estimator with the smaller seed spread.** Metaorder binning
   varies by 1.22 across seeds where the supply curve varies by 0.10; an
   earlier three-seed agreement was partly luck (FFA-45).
5. **Subtract the passive benchmark before comparing participants.** Every
   participant is net long, so raw equity ranks them by inventory held rather
   than by skill (FFA-46).

## Reproduction

```bash
# healthy population, payoff table
go run ./cmd/multivenue -config=research/ffa-ecology-population-2026-08-16.json -duration=12h -seed=91 -logdir=logs/research/run
python tools/population_payoff.py --report logs/research/run/greeks.json

# impact mechanism, independent of metaorders
python tools/analyze_supply_curve.py --log logs/research/run/venues/north/spot/ABC-USD.jsonl

# order-flow persistence and response
python tools/analyze_response.py --log logs/research/run/venues/north/spot/ABC-USD.jsonl
```

## What is open

- Carrying capacity is measured for carry arbitrage only. The same curve for
  market making and for execution would say whether the optimum population is a
  property of the strategy or of the market.
- The strong form of the square-root law needs either far more metaorders per
  arm or a supply-curve estimator conditioned on participation.
- Latent liquidity is implemented and does not behave as the theory describes.
  Closing that needs the displayed book to be a thin veneer over latent
  liquidity with no continuously quoting maker, which is a different market
  from the one this campaign built.
