# Three-Venue Routed Latency Race, 2026-08-15

## Question

Can a lower-latency participant obtain better **observed, all-in, completed
cross-venue cashflow** when it routes between three independently funded spot
books with non-atomic legs?

This is the first ecology-level successor to `latencylab`. It does not assume
shared collateral, instantaneous asset transfer, atomic execution, or that a
quoted edge remains available at venue arrival.

## Route contract

Each tier owns one prefunded ABC/USD account on `north`, `central`, and
`south`. It receives its own phase-ordered request, response, and market-data
delays. A route observes each local public book from its initial snapshot plus
visible-depth deltas and submits two independent **FOK market** legs only if:

```text
sell venue net proceeds > buy venue cost
```

where both side-specific displayed top-level quantities cover the entire lot
and both 5 bp taker fees are included. The router logs each request, order,
fill, fee, rejection, completion status, actual cashflow, and global base
residual. A route that fills only one venue is failed and remains an explicit
location-specific inventory residual. There is no automatic flattening,
transfer, or hidden shared wallet.

This top-of-book quantity check was added after an initial 0.4-ABC test used a
positive price pair whose displayed best level could not fill the full FOK
lot. The old signal was therefore invalid. The retained study starts only
after that correction.

## Design

- Three venues share a deterministic clock but have independent accounts,
  market makers, and seeded spot/perp/option flow.
- Each venue has two Stoikov spot makers, one Stoikov perp maker, dated
  futures, short and long option tenors, local spot-margin support, four
  random spot/perp traders, and four option-flow traders.
- The option dealer hedge is disabled solely for this router treatment. This
  avoids a local option-dealer ABC terminal-mark failure being mistaken for a
  router outcome; option lifecycle and local books remain live.
- Router lot: 0.4 ABC. At the two 0.2-ABC maker levels, this makes a route
  sensitive to genuine displayed-depth depletion rather than allowing both
  tiers to consume abundant depth.
- Latency: each request, response, and market-data channel has a fast `1 s`
  or slow `2 s` delay, represented as tier multipliers `0.5` and `1.0` on a
  2-second base. The 1-second simulator step is no larger than the fast
  per-channel delay.
- One observed route attempt per tier per world. Seeds `42..51` were fixed
  before reading outcomes.
- Client/actor-order control: run every seed with `[0.5, 1.0]` and the
  reversed `[1.0, 0.5]` tier ordering.

The complete result table is
[multivenue-crossvenue-latency-race-2026-08-15.json](multivenue-crossvenue-latency-race-2026-08-15.json).

## Result

Across ten forward-order worlds:

| Measure | Fast 1 s | Slow 2 s |
| --- | ---: | ---: |
| Submitted attempts | 10 | 10 |
| Completed two-leg FOK groups | 7 | 8 |
| Failed groups | 3 | 2 |
| Nonzero base-residual reports | 2 | 1 |
| Sum completed actual cashflow | +$82.825 | -$69.061 |

Seven seeds completed for both tiers. On all seven, fast cashflow exceeded
slow cashflow; the mean paired difference was **+$20.854** per 0.4-ABC routed
attempt. Fast completed cashflow was positive in every completed fast case.
Slow cashflow was negative in seven of eight completed cases; its only
positive completed case was still below the fast counterpart.

The full ledger matters: seed 49 is a counterexample to a simple completion
claim. The fast router failed with a -0.4 ABC residual while the slow router
completed at -$5.909. Seed 46 left +0.4 ABC residual for both tiers; seed 51
had zero-fill FOK failure for both. These are preserved in the JSON, not
dropped from the raw execution result or replaced by a zero cost.

## Causality controls

1. Reversing tier assignment to client and actor-registration positions gave
   the same report for each physical tier on all ten seeds after masking only
   IDs that necessarily follow the label. The advantage does not follow a
   lower client ID or actor position.
2. Seed 42's full `greeks.json` was byte-identical at `GOMAXPROCS=1` and
   `GOMAXPROCS=14`:
   `e5f5866608dc0139ec4b7f10bdfb373cccfda8867d66249d4ce956ff8863ab00`.
3. A retained regression runs the label-swap and thread-count control through
   the phase-owned delayed courier. The router cannot silently fall back to
   host-scheduled delivery.

## Interpretation and boundary

H-016 is supported **only** as a conditional routed-execution result: on the
seven jointly completed seeds in this explicit configuration, earlier market
data and request arrival captured better realized all-in cashflow. It is not
a universal low-latency PnL claim. The slower tier completed more often over
the full ten-seed ledger, and non-atomic FOK legs can create residual local
inventory even after a correctly funded quote admission.

The cashflow is the sum of actual sell proceeds minus buy cost and quote fees
across independently funded venue accounts. It is not terminal marked PnL,
does not include transfer/funding/custody cost, and does not establish an
equilibrium. A next experiment must add an explicit transfer/rebalance policy
and report the resulting inventory carrying, latency, rejection, and cost
instead of treating the location-neutral base sum as automatically usable
capital.
