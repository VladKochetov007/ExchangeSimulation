# FFA Ecology Plan

## Answer to the starting question

**Partly, but not yet.** The repository already has a sound controlled
three-venue derivatives baseline: `ABC/USD` is cross-listed across three
independently funded venues, each venue has spot, perpetuals, rolling dated
futures, European options, local spot-margin borrowing, makers, noise flow,
option flow, and a non-atomic cross-venue spot router. The accepted controls
are deterministic and retain fills, residuals, and strict USD valuation.

It is **not** yet the requested FFA research world. It has one underlying,
one cross-listed spot symbol, one common venue matcher per world, hand-coded
static policies, limited agent rosters, no cross-currency graph, no survival
or mutation rule, no opponent-tournament matrix, and no evidence for a
non-transitive equilibrium. Calling it an organism today would overstate it.

## Gen-0 evaluator before scale

The first accepted FFA scenario will have three venues and the smallest graph
that can distinguish mechanisms:

| Object | Gen-0 requirement |
| --- | --- |
| Assets | `ABC`, `CDF`, `USD`; fixed precision and USD reporting numeraire |
| Spot graph | `ABC/USD`, `CDF/USD`, `ABC/CDF`; each pair must have a named valuation path |
| Cross listings | At least `ABC/USD` and `CDF/USD` on two or more venues, with independent local books |
| Derivatives | Perpetual, short and long dated futures, and short and long European option tenors for one underlying first |
| Venues | FIFO, pro-rata, and a deterministic explicit hybrid only after its allocation policy is implemented and tested |
| Participants | Noise flow, liquidity providers, triangular/cross-venue arbs, basis/funding arbs, and an option dealer; every actor independently funded |
| Information | Delayed public book/trade/instrument events plus own responses only; no exchange query in a strategy loop |
| End state | Complete USD marked accounts, residual positions, Greeks, funding, fees, borrow interest, liquidations, and invalid-mark reasons |

## Hypothesis queue

| ID | Family | Mechanism | Cheap discriminant | Falsifier |
| --- | --- | --- | --- | --- |
| FFA-01 | venue allocation | FIFO versus pro-rata changes adverse-selection and inventory exposure at equal displayed depth. | One-book identical-flow allocation test. | Any difference is explained by queue/rounding bug or missing quantity conservation. |
| FFA-02 | cross-asset graph | Triangular arbitrage removes executable cycle errors only if delayed three-leg fills cover cost and residual risk. | `ABC/CDF` triangle with all-in FOK/null cases. | Edge remains after executable depth/fees are removed, or PnL ignores a residual. |
| FFA-03 | fragmentation | A faster router has an advantage only when it observes/arrives earlier at a scarce cross-venue executable opportunity. | Existing label-swap latency control generalized to two symbols. | Label/ID permutation retains the result, or equal latency preserves it. |
| FFA-04 | liquidity ecology | A-S skew improves inventory-tail control without merely withdrawing liquidity. | Common-random-number A-S versus linear-skew population. | Lower inventory tail accompanies lower quote survival/fill and no cost-adjusted wealth benefit. |
| FFA-05 | derivative ecology | Option flow plus delayed hedging changes portfolio Greek tails and terminal wealth conditional on dynamic IV/forward regime. | Static-IV risk null first, then a controlled IV shock. | Greeks do not align with exchange positions/marks or wealth is incomplete. |
| FFA-06 | non-transitivity | Strategy advantage is frequency dependent and may form a directed invasion cycle. | Leave-one-out two-strategy invasion matrix before three-way cycle. | Sign changes under held-out seeds, label permutation, or a reasonable population mixture. |

## Prerequisite experiment FFA-00

`OptionBuyProbability=0` was silently converted to the default buy bias, so an
all-sell option-flow treatment could not be expressed. The experiment
hypothesis is mechanical: nullable configuration preserves omission as the
historical default while retaining explicit zero. It is accepted only if JSON
configuration with zero normalizes to all-sell flow and omission normalizes to
the documented default; otherwise no one-sided gamma/ecology arm may run.

## Promotion ladder

1. **Mechanics:** `VenueProfile`, 3-asset spot graph, and exact per-venue
   matching. Write property tests before a scenario runs.
2. **Measurement:** immutable manifest; account, risk, fill, and information
   boundary logs. Make incomplete valuation a run failure.
3. **Null controls:** noise plus market makers, then no-arb and no-options
   controls. Validate stylized internal signals only; do not calibrate claims.
4. **Mechanism arms:** introduce one strategy family at a time using common
   random numbers and held-out seeds.
5. **Ecology:** fixed capital shares and repeated population mixtures.
6. **Evolution:** only after the fixed-mixture payoff matrix is valid; freeze
   recapitalization, bankruptcy, mutation, and re-entry rules in a new
   manifest version.

## Gen-0 acceptance tests

- Same manifest produces identical digest at `GOMAXPROCS=1` and `14`; agent
  registration and ID permutations leave economic aggregate results unchanged
  unless the scenario explicitly creates a simulated-time tie.
- Each venue's matcher conserves execution quantity and obeys its published
  allocation rule. No fill may exceed displayed/resting quantity.
- A full cross-currency marked account either resolves a declared FX path or
  is invalid. It never silently omits `ABC` or `CDF` exposure.
- A scenario seeking population fitness enables `strict_population_accounting`.
  It records every independently funded venue account at initial and terminal
  lifecycle points and fails on a missing terminal conversion mark.
- All agents pass an information-boundary test that fails if they query engine
  state rather than react to their gateway events.
- Invasion matrix rows report means, paired bootstrap interval, completion,
  invalid-run count, liquidation rate, inventory/Greek tails, and wealth
  share. No scalar "winner" can mask these outcomes.
