# Executable Latency-Ecology Null Result, 2026-08-15

## Question

Does the fee simulation's many-agent spot/perpetual ecology give a lower
market-data/request-latency basis arbitrageur more **completed, positive,
all-in** conversions than a slower counterpart?

This is deliberately a stronger question than whether a lower-latency actor
can win a constructed scarce conversion. `latencylab` already establishes the
latter causal arrival mechanism. This experiment asks whether the current
fee-simulation ecology produces the preconditions for that mechanism without
manufacturing an opportunity.

## Defects found before the experiment

The previous `FeeAwareBasisArb` was not admissible for this question.

1. It decided from spot and perpetual **midpoints**, then crossed both
   spreads and paid both taker fees. A midpoint gap could therefore be a
   guaranteed loss at executable prices.
2. In reactive mode it replaced an old snapshot's mid with each trade price
   while retaining the old half-spread. That invented a new bid/ask pair; a
   trade does not prove either displayed touch remains available.
3. It tracked submissions/intent but did not expose a fill-ledger or a
   terminal account valuation.
4. Raw terminal account changes included the value movement of the common
   initial ABC and Q inventories, even when the arb had made no trade. That
   is passive beta, not strategy PnL.

The implementation now reconstructs each public book from its snapshot plus
subsequent visible-depth deltas. The entry test for a one-lot pair is:

```text
sell-perp / buy-spot: perp_bid * lot - perp_fee > spot_ask * lot + spot_fee
buy-perp / sell-spot: spot_bid * lot - spot_fee > perp_ask * lot + perp_fee
```

All operations use fixed-point checked arithmetic. Each arb reports observed
spot/perp fills, notionals, quote fees, unpriced fees, residual base quantity,
and open perp lots. Its strategy account change is terminal marked equity
minus the terminal value of the same static starting ABC/Q/USD inventory.

## Controlled matrix

- Simulator: deterministic-phase `feesim`; two linear market makers; eight
  independently seeded random-flow participants across spot, perp, and cross
  books.
- Simulation duration: 30 simulated seconds per world.
- Fees: default 8 bp spot taker and 5 bp perp taker.
- Latency tiers: `0.2` and `1.0` times the same deterministic latency model.
- Seeds: `42` through `49`.
- Client-order control: each seed was run with tier order `[0.2, 1.0]` and
  the reversed `[1.0, 0.2]`.

The command and the full result matrix are retained in
[feesim-executable-latency-null-2026-08-15.json](feesim-executable-latency-null-2026-08-15.json).

## Result

All 16 worlds, and all 32 tier observations, had:

```text
executable signals = 0
submitted pairs    = 0
observed leg fills = 0
strategy PnL       = not estimated from passive inventory; zero when inactive
```

Reversing client/tier order did not change this result. A post-fix seed-43
terminal valuation had a large raw ABC/Q inventory move, but both inactive
tiers had identical passive terminal equity and exactly zero
`strategy_equity_change_usd` after subtraction.

## Interpretation

The old midpoint signal was not evidence of a latency-arbitrage ecology. With
the stated spreads and taker fees, this particular one-venue configuration
does not generate an executable spot/perp conversion at the configured lot.
It cannot support a claim that faster agents are profitable, nor a claim that
the slower tier loses, because neither participates.

This does **not** falsify the deterministic arrival mechanism: E-031 remains
a separate scarce-conversion proof with exact two-leg cashflows and
label-swapping controls. It falsifies the broader inference that the existing
fee-simulation midpoint dynamics instantiate that mechanism economically.

## Admissible next step

Do not relax the price test or remove fees to make this study active. A future
ecology experiment needs an explicit source of temporary, executable
dislocation: for example, venue-qualified reference-price propagation and
independently delayed market-maker repricing, coupled with a routed
cross-venue execution group that records both legs, fills, fees, inventory,
and transfer policy. That is a new model with its own controls, not a repair
to this null result.
