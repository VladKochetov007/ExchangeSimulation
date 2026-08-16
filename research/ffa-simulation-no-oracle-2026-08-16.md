# The FFA simulation under the no-oracle law

Date 2026-08-16. Branch `autoresearch/ffa-ecology-gen0`. This document supersedes
`research/simulation-description-2026-08-16.md`, which describes the same
machinery under an oracle and whose payoff numbers must not be reasoned from.

## The law

No participant may receive information derived from the exogenous fundamental.
Equal rights and equal information for every participant of every market. Any
edge must be earned from speed, modelling, order-flow inference or inventory
management, never granted by the environment.

A lagged and noisy fundamental is still a fundamental: it tells its subscriber
which way the world will move. Degrading the observation blurs the oracle, it
does not remove it. Both are now refused.

The venue is a separate matter. An exchange may compute its own marks and index
for margin and liquidation, exactly as real venues do, because that is a venue
function rather than a participant edge. Prices that participants trade on must
be produced by participants.

### Enforcement

`NewSim` rejects, unless `debug_oracle_mode` is set:

- `maker_anchor: "fundamental"`, with or without `degraded_index`
- any `value_trader_count` above zero, because `ValueTrader` reads the
  fundamental directly (`valuetrader.go:14`)

`debug_oracle_mode` exists so a quoting, risk or liquidation path can be
validated under known-value conditions. It may never support a claim about
strategy performance. Guarded by
`TestExogenousFundamentalReachesNoParticipantWithoutDebugOptIn`.

Legal anchors for a scientific run are `own_mid` (each maker quotes around its
own book) and `consensus` (a cross-venue average of venue midpoints). Both are
endogenous: they are computed from what participants did, not from what is true.

## Venues, assets and markets

Three independent venues — `north` (price-time matching), `central` (pro-rata),
`south` (pro-rata). Each has its own accounts, its own margin and borrowing, its
own insurance fund, and its own copy of every market. There is no shared wallet
and no cross-venue collateral; a configured router holds one local account per
venue and models neither transfer nor atomic legs.

Assets: `ABC` (base), `CDF` (second base, optional via `cross_asset_spot_graph`),
`USD` (quote).

Markets on each venue:

| market | symbol | notes |
|---|---|---|
| spot | `ABC/USD` | the primary book, spot-margin borrowing enabled |
| spot | `CDF/USD` | second asset against quote |
| spot | `ABC/CDF` | cross-asset pair, no USD leg |
| perpetual | `ABC-PERP` | funding at a configurable interval and cap |
| dated futures | `ABC-FUT-<expiry>` | two live tenors, cash-settled to spot |
| options | `ABC-<expiry>-<strike>-<C\|P>` | European, cash-settled |

So a three-venue run with the cross-asset graph carries nine spot books, three
perpetuals, six dated futures and sixty option contracts at the default board.

## Derivative schedules

- **Dated futures**: two tenors, short (default 8h) and long (default 72h),
  listed by `DatedFuturesLister` and rolled as they expire.
- **Options**: two tenors, short (default 6h) and long (default 48h), with a
  strike ladder of `strikes_per_side` either side of the prevailing level at
  `strike_step_usd` spacing, capped by `option_max_strikes_per_expiry`. Default
  produces twenty contracts per venue; a 41-strike board costs about six times
  the wall clock for the same simulated span.
- Expiry runs through the exchange's own expiry engine, which settles options to
  intrinsic value and futures to the spot mark, then relists the next tenor.

## Participants

Every count is configurable; none is hardcoded. Counts below are per venue.

**Liquidity providers**
- `spot_maker` — Avellaneda-Stoikov makers on `ABC/USD`, in relative units
  (log-variance per second, relative risk aversion, relative fill decay), with an
  inventory limit, inventory skew, and an optional perpetual hedge. Quote
  replacement is either cancel-first or `spot_maker_submit_before_cancel`.
- `cdf_spot_maker`, `abc_cdf_spot_maker` — the same engine on the other books.
- `perp_maker` — Stoikov on `ABC-PERP`.
- `futures_maker` — fixed-spread maker on the dated ladder;
  `futures_maker_self_anchored` lets each dated book price from its own trades
  rather than pinning its mid to spot, which is what gives the ladder a basis at
  all.
- `option_dealer` — quotes the whole chain from a configured implied volatility
  with a per-lot skew, and hedges delta into spot or perpetual. Not yet a
  Black-Scholes surface dealer; this is the largest remaining derivative gap.
- `bootstrap_depth` — a passive ladder that rests levels and never reprices,
  refilling only what is consumed, with an optional withdrawal time.

**Takers and arbitrage**
- `noise_flow` — independent random takers using market orders.
- `option_flow` — random option and futures takers.
- `carry_arb` — spot against perpetual basis, delta-neutral, tick-aligned IOC.
- `dated_carry_arb` — spot against dated futures, with a slippage bound.
- `parity_arb` — put-call parity desk.
- `metaorder_trader` — parent orders drawn from a Pareto size distribution, split
  into children paced against external volume.
- `round_trip` — enters and unwinds on a hold timer.
- `elastic_supplier` — supplies size as a function of price movement.
- `latent_liquidity` — reveals hidden intentions near the touch.

**Removed from scientific runs by the law**
- `value_trader` — it reads the exogenous fundamental and trades the book
  against it. Available only under `debug_oracle_mode`.

## What the removal of the oracle changes, and what is now open

The exogenous fundamental process still exists in the runtime, but with the index
anchor and the value traders barred it reaches no participant. Prices are
therefore produced entirely by participants: makers quote around their own or a
consensus midpoint, and takers trade against them.

This reopens a question the oracle had been suppressing. Earlier in this campaign
a purely self-referential market drifted without bound, and the fix at the time
was to anchor makers to the fundamental and add value traders — that is, to hand
out an oracle. That fix is no longer permitted. Whether an endogenous market
stays bounded, and what population is required to make it stay bounded, is now
the central open question rather than a solved one.

Candidate endogenous anchors that respect the law, none yet tested:

- the `consensus` anchor, which couples venues to each other without telling
  anyone what is true
- cross-market arbitrage between spot, perpetual and dated futures, which ties
  the term structure together without an external reference
- inventory limits and the cost of carrying inventory, which bound how far a
  maker can be pushed
- funding on the perpetual, which is a market-generated force toward the spot
  price rather than toward the fundamental

## Results that survive the correction

Mechanical and accounting findings do not depend on who knew what:

- The intra-step liquidity hole: cancel-then-replace quoting left both sides of
  the book empty in 99.5% of steps, and takers scheduled in that instant missed
  it every step. Fill rates ran from 0.008% to 44.9% by class; submit-before-
  cancel takes them to about 100%.
- Two distinct execution failure modes: market orders need depth to exist, IOC
  limits need depth at the price they targeted. A passive ladder fixes the first
  and not the second.
- Conservation: population sum equals insurance-fund payout minus fees. Asset
  units conserve exactly against fees collected in each asset.
- The maker share of volume is a quote-size artifact: maker-versus-maker is
  9.5–19.8% of trades against 89.8–95.4% of volume.

## Results re-validated without the oracle

- **Presence monopoly (E-144)**: confirmed and stronger. Against continuously
  quoting makers the never-repricing ladder inverts from +17,643,176 to
  **−35,366,067** per member at high fundamental volatility.
- **Dealer competition (E-145)**: the qualitative law holds. Competition
  transfers rent to the flow, per-taker loss falling from −118,780 to −63,123
  with flow held fixed, and dealer income tracks the flow-to-dealer ratio. The
  quantitative fit is weaker than under the oracle: dealers capture 70–84% of the
  pure-transfer prediction rather than about 97%.

## Results falsified by the correction

- **Volatility carrying capacity (E-144)**: "every maker class dies at high
  fundamental volatility" is false. The perpetual maker earns **+8,015,689** per
  member where the oracle regime showed −57,293,586. With an oracle its
  counterparties saw true value and picked it off; blind them and it earns its
  spread. Restated: there is a solvency boundary for spot market making and for
  passive non-repricing depth, and whether a maker survives depends on whether
  its counterparties can see value better than it can — which is a statement
  about relative information, and is testable by varying that directly.
- Every ladder profitability figure from the oracle era, and every population
  payoff table from it.
