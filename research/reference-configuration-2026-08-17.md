# Reference configuration and its profile

`research/configs/ffa-2026-08-16/reference_base.json`, 8 simulated hours, seeds
91, 92 and 93. Every comparison should be checked against this before it is
designed, and the profile cited with the result.

## What it fixes

Four experiments in this campaign were undermined by a property of the base
configuration rather than by their hypothesis. Each fix below was forced by one
of them.

| setting | why |
|---|---|
| `short_future_tenor` 2h, `long_future_tenor` 6h | An 8h contract is never listed inside a shorter run, so dated futures and their carry desks recorded 12 requests and could not be measured. |
| `spot_maker_submit_before_cancel` | Cancel-then-replace quoting empties the book, and a population of such makers has no two-sided market at all. |
| `spot_maker_requote_bps_tiers` 0, 10, 25, 40 | One shared threshold makes every book go stale at the same instant, which removes the dislocations cross-market arbitrage trades. A mix leaves them drifting apart. |
| generous `rate_limit_tiers` | Request counts are only reported for metered participants, so a budget far above any real usage is set purely to obtain them. |

## Profile

All seventeen classes are active. Requests are per venue over 8h, seed 91.

| class | requests | per member (seed 91) | sign across seeds |
|---|---|---|---|
| option_dealer | 4,369,498 | 109,007 | stable, and stable in magnitude to 0.8% |
| spot_maker | 1,125,033 | −1,436,345 | stable |
| noise_flow | 518,400 | −266,590 | **unstable** |
| perp_maker | 343,921 | −541,825 | stable |
| option_flow | 172,800 | −55,118 | stable |
| carry_arb | 80,857 | 4,750,764 | stable in sign, 2.7M to 19.8M in magnitude |
| elastic_supplier | 78,739 | −93,063 | stable |
| futures_maker | 47,862 | 8,986 | stable, 8,160 to 9,017 |
| metaorder_trader | 38,479 | −2,340 | stable |
| triangle_arb | 19,056 | 44 | **unstable** |
| dated_carry_arb | 2,292 | −9,833 | stable, −9,786 to −10,206 |
| parity_arb | 2,118 | −3,219 | not yet measured across seeds |
| fixed_distance_maker | 6,932 | 2,805 | not yet measured across seeds |
| imbalance_maker | 6,294 | 4,414 | not yet measured across seeds |

## What this configuration can and cannot measure

**Can carry a directional claim.** Option dealers, futures makers and dated
carry are stable in both sign and magnitude, so a change of a few percent in
them is meaningful. Spot makers, perpetual makers, metaorder traders and
elastic suppliers are sign-stable, so a directional claim is safe while a
magnitude claim is not.

**Cannot carry a claim at this horizon.** Triangular arbitrage returns 44, −135
and 14 per member across the three seeds, and noise flow returns −266,590,
80,785 and −592,876. Both change sign. No comparison involving either is
interpretable from three eight-hour seeds, and both were classes I previously
attempted to measure.

**Carry arbitrage needs care.** Its sign is stable but its magnitude spans 2.7
to 19.8 million per member, so it can support "was it harmed" and cannot support
"by how much".

## Remaining imbalance

Makers lose 1.4 million per member here. A shared requote threshold instead
leaves them slightly profitable but silences the arbitrage desks. The
configuration therefore buys a fully active population at the cost of maker
solvency, and any result about maker profitability must state which side of that
trade its configuration sits on.
