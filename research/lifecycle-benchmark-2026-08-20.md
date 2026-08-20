# The ten-cycles-alive benchmark

## What is being measured

The guideline (§30) asks for a market that survives repeated lifecycle cycles
rather than one that technically keeps printing trades. This document defines
the benchmark this repository runs against that ask, and records where the
population stands.

**One cycle** is a two-hour interval in simulated time. Within it the listing
scheduler lists a new option expiry and a new dated future, perpetual funding
settles at each venue on its own period, and the previous generation of options
and dated futures expires and settles. A twenty-hour run is ten cycles.

**Funding periods differ by venue and intersect**: north settles every eight
hours, central every hour, south every two. Every eighth hour all three settle
together, every second hour two of them do, and in between only central does.
A shared period would make every venue pay at the same instant, which is the
one schedule under which a desk holding the same exposure at two venues has no
cross-venue funding to trade.

## The corridor

`mvanalyze -metric viability` measures each book in each window and reports the
rules it broke. The rules are supplied by the caller, not built into the
library, because what counts as a living market depends on the instrument: a
chain that trades twenty times an hour is healthy and a spot book that does is
dead. `research/configs/lifecycle-2026-08-20/viability-thresholds.json` holds
the campaign's thresholds, matched by symbol glob.

A window is viable when it passes every rule:

| Rule | What it catches |
|---|---|
| `thin_volume` | the book stopped trading |
| `few_taker_classes` | one participant class is the only source of demand |
| `few_maker_classes` | one class is the only source of liquidity |
| `one_sided_book` | a taker had nothing to trade against for a share of the window |
| `concentrated_flow` | one class holds most of the traded volume |
| `wide_spread`, `thin_depth` | liquidity exists on paper only |

Each book is then rolled up: how many windows it passed, the cycle it first
failed in, and the last cycle it was alive in. A market that traded for seven
cycles and then stopped, and one that was never alive, are different failures.

## Population

Built and wired, each configurable and defaulting to the previous behaviour:

- **Option pricing**: dealers take a `VolatilityModel`. `RealizedVolatility`
  estimates the underlying's own volatility with an exponentially weighted
  variance annualised by observation spacing; a roster assigns consecutive
  dealers different half-lives and risk premiums, so they disagree because they
  forget at different rates rather than because they were told to.
  `InventoryVolatility` marks up the strikes a dealer is short, which is where
  a smile can come from without one being written down.
- **Hedging**: `NoHedge`, `StaticDeltaHedge` (covers each trade once at
  inception and carries the gamma), `BandedDeltaHedge`, `TimedDeltaHedge`.
  Assigned per dealer from a roster.
- **Vanna-volga desks**: take one dealer's book and flatten vega, vanna and
  volga by trading options. Neither vanna nor volga can be hedged in the
  underlying at any size, so without such a desk that risk accumulates on the
  dealers.
- **Option value takers**: price the chain with their own model and trade only
  where a dealer disagrees with them by more than a configured share of the
  contract's own value.
- **Latency**: per-participant-class links. `LognormalLatency` is late as often
  as a measured link, since a normal draw has no tail; `SpikyLatency` is fast
  until it stalls, which no mean latency reproduces.
- **Maker classes on every spot book**: the fixed-distance and imbalance makers
  take a symbol roster instead of being pinned to ABC/USD.

## Results

Recorded in `research/ffa-ecology-experiments-2026-08-15.jsonl` under L-001
onward. Configurations are in `research/configs/lifecycle-2026-08-20/`.

## Known measurement caveats

- A book is only measured in windows where it produced events, so a contract is
  not judged before it lists or after it settles. A window in which it lists or
  expires part-way through is judged as a whole one.
- Spot publications carry no symbol at either nesting level; the book is
  recovered from the file, which holds exactly one book.
- The corridor reads one venue's copy of each book; a book alive at one venue
  and dead at another is two rows, not an average.
