# Derivative Population and Contract-Availability Edge Cases

Date: 2026-08-16. Branch `autoresearch/ffa-ecology-gen0`. All runs 12h simulated,
seed 91, three venues, `research/ffa-ecology-population-2026-08-16.json` as the base
configuration. Payoffs are active result against a passive benchmark, per member, USD.

## 1. Making the derivative population many-vs-many

Until this change the derivative participants were hardcoded at one option dealer,
one futures maker and two option takers per venue. That is one-vs-many: the dealer
faced flow but no competitor, so its result was a monopoly rent rather than a price.

`option_dealer_count` and `futures_maker_count` are now configuration fields
(`simulations/multivenue/sim.go`), defaulting to 1 so existing configurations are
unchanged. The venue exposes `OptionDealers` and `FuturesMakers` slices; the
original singular fields remain and point at the first member.

### Result (E-125, FFA-55)

| Arm | dealers/venue | option flow/venue | dealer per member | dealer class total | option flow per member |
|---|---|---|---|---|---|
| A | 1 | 2 | 231,543 | 694,628 | -119,468 |
| B | 3 | 2 | 39,494 | 355,443 | -63,218 |
| C | 3 | 6 | 236,175 | 2,125,575 | -121,697 |

Two findings.

**Dealer income is set by the flow-to-dealer ratio, not the dealer count.**
Predicting each dealer's result as (flow per venue / dealers per venue) x the
observed per-flow loss gives 238,937 vs 231,543 measured in A, 42,145 vs 39,494 in B,
and 243,394 vs 236,175 in C. Arm C restores the monopoly-level per-dealer income at
three dealers purely by tripling the flow.

**Competition is price-forming, not merely dilutive.** With flow held fixed (A vs B),
tripling the dealers cuts what each option taker loses by 47%, and the dealer class
total falls 49%. If competition only split a fixed rent, the class total would be
unchanged and the flow would lose the same amount. It does not: the rent is
transferred out of the dealer class and into the flow, through tighter quotes.

**Anomaly: the dated futures ladder is inert.** Futures makers earn between -1 and
+60 USD per member in every arm. The dated contracts are listed, scheduled and
quoted, but no participant class takes dated-futures risk against them, so there is
no flow to trade against. Options are now many-vs-many; dated futures are not. This
is the next gap to close, and it needs a new participant class, not a count.

## 2. Contract-availability edge cases

How do strategies behave when the contracts they were designed around are absent,
or too numerous to handle? Three arms against the same baseline.

| Arm | option contracts seen | option_dealer | carry_arb | spot_maker |
|---|---|---|---|---|
| baseline | 30 | 231,543 | 35,373 | -51,974 |
| no options (`strikes_per_side=0`, `option_max_strikes_per_expiry=1`) | 6 | 1,157,545 | 23,321 | -34,786 |
| no cross listing (`cross_asset_spot_graph=false`) | 30 | 231,391 | 37,254 | -63,435 |
| huge board (`strikes_per_side=20`, `option_max_strikes_per_expiry=41`, `strike_step_usd=250`) | 164/venue | see below | | |

### No options

Nothing crashes and no strategy stalls. The counter-intuitive result is that the
dealer earns *five times more* (1,157,545 vs 231,543) on 6 contracts than on 30.
With a single strike per expiry all option flow is concentrated into one book, so
every taker order crosses the dealer's quote at that one strike. With 30 contracts
the same flow is spread thin and much of it never trades at all. The dealer's income
is bounded by flow it actually faces, not by how many contracts it quotes — the same
ratio law as section 1, seen from the contract axis instead of the dealer axis.

Carry arbitrage falls (35,373 -> 23,321) and spot makers lose less (-51,974 ->
-34,786): with fewer option books, dealer hedging into spot and perp is smaller, so
there is less flow for the carry participants to intermediate and less adverse
selection against the spot makers.

### No cross listing

Removing the ABC/CDF cross-asset graph is a non-event for the derivative
participants: option dealer 231,391 vs 231,543 baseline, statistically the same run.
Strategies that need the missing pair do not error, retry or spin — they simply never
find a quote and stay flat. Carry arbitrage rises slightly (35,373 -> 37,254) and
spot makers lose more (-51,974 -> -63,435), because without the cross-asset venue the
same directional flow concentrates on the direct ABC/USD books.

The important negative result: no strategy has a hard dependency on a contract's
existence. Missing instruments degrade participation rather than breaking the actor.

### Too many options at once

A 41-strike-per-expiry board (164 option contracts per venue, ~8x the baseline's 20)
does not fail — it becomes too slow to run within the campaign's run window.
Measured cost:

| board | 1h sim | 3h sim |
|---|---|---|
| big (41 strikes/expiry) | 60.67s | 182.42s |
| baseline (5 strikes/expiry) | — | 29.12s |

Cost is ~6.3x for 8.2x the contracts, i.e. close to linear in contract count and
linear in simulated time. The 12h big-board run exceeds the 10-minute execution
window and was abandoned, not crashed. This is a compute-scaling limit of the
harness, not a strategy-handling failure: the dealer quotes all 164 books each
interval because its quoting loop has no per-cycle contract budget.

The realistic finding for the ecology question is that a dealer with no attention
constraint spreads a fixed quoting effort over an unbounded board. Section 2's
no-options result shows the economic consequence directly: thinner coverage per
contract means less flow captured per contract. A dealer that selected which strikes
to quote would dominate one that quotes everything — that is a strategy axis the
simulation currently does not model.
