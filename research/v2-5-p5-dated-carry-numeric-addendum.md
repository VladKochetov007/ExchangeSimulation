# V2-5 P5 — immutable dated-carry numeric addendum

Status: **preregistered before P5 implementation, preflight, or outcome
inspection.** It completes the numeric gate in
[`v2-5-p5-dated-carry-causal-preregistration.md`](v2-5-p5-dated-carry-causal-preregistration.md).

The source revision is `ea0eb66`. The exact configs are under
`research/configs/v2-5-p5/`. Unknown P5 fields are intentionally committed
before their implementation; running these configs before the implementation
and structural preflight is prohibited.

Pre-implementation evidence correction: commit `47ce6a0` mistakenly retained
the obsolete P4 receipt role `term_carry_allocator`. Before either new actor
existed and before any P5 run or outcome inspection, the role contract was
replaced by the required `dated_execution_mandate` and
`dated_term_carry_allocator` roles. The superseded hashes from `47ce6a0` are
provenance only and are not runnable P5 cells; the hashes below are canonical.

## Environment changes and sole intervention

P5 derives from the fully rendered P4 V2 environment, with three declared
same-in-both-arms changes:

1. the completed perpetual `term_carry_allocator` is absent, so P5 cannot be
   attributed to simultaneous perp carry;
2. the legacy random option-flow participant no longer includes futures; and
3. one explicit long-future execution mandate per venue replaces that
   activity-generator channel for the short dated contract.

These are free experimental-design choices (C), frozen before implementation.
They isolate dated economics and give future demand an inspectable parent
objective. They are not calibrated from a P5 basis outcome.

The sole A/B delta is:

```text
A  dated_term_carry_allocator.trade_enabled = false
B  dated_term_carry_allocator.trade_enabled = true
```

Every other JSON value must be identical within a paired seed. A computes and
evidences the same candidates as B but cannot change target or submit.

## Frozen cells

| cell | SHA-256 |
| --- | --- |
| A-117 | `c1ca7a6401ac0723a9226c826ccdb2002232a0743ef88847778b41c8b1487685` |
| B-117 | `55bfedd029d3ad91d8ff0b99983ba8692dc95f3b91dc30b6f953d3c3d8ea0544` |
| A-119 | `9085c514316ba988a288f51eb4d2da6987fa3ef368f79bfa28193c2e8fc576ae` |
| B-119 | `630684b1d68fb4e0cb42644d8dea92c66e13cc4fb70a9f01af66bed413d8fe77` |
| A-139 | `ec832268ffbb6a0b9584868af062af8782498223cb1be6feb27b8b208f42ceb3` |
| B-139 | `37d3d94b3bc2ddb1ca6985aef8b09d1a18547babfb8b7f56b436d35f5660ea64` |
| A-149 | `c2e930e468ab9415f0c16c45b7c6199edbdcb6188afc47621350ccb29b20454f` |
| B-149 | `da9d96804a76ca695e78099169d3724b1f4c2bb8b581990d96babcb4cbb62df4` |
| A-151 | `545d60feb5bf6b536e59bdff88ebc03b2e80958e4bab01e69625d1c86fdac2fc` |
| B-151 | `0bce13c8f202a20330a35a3e87bca2438b7faf4a6a381b45786cbed994e62f7c` |

Development paired seeds are 117 and 119, the first unused primes after the
prior 113 timing holdout. Untouched holdouts are 139, 149, and 151, the first
unused primes after the frozen P4 holdout boundary. Holdouts may not be used for
debugging and run only if development earns promotion.

## Contract, horizon, and execution demand

| input | exact value | class and rationale |
| --- | ---: | --- |
| simulated horizon | 26 h | C: three complete rolling 8 h short futures plus two hours for final closure evidence |
| eligible contract | delivered FUTURE announcement with original tenor exactly 8 h | A: inherited short-future contract |
| long future | inherited 72 h, never selected by the P5 actors | A |
| settlement | inherited cash settlement to canonical spot observation-window result | A |
| carry minimum TTE | 10 min | C: leaves deterministic time for two ordinary legs and excludes expiry-boundary admission |
| carry decision interval / phase | 2 s / 0 | A: inherited P4 participant cadence |
| maximum delivered book age | 10 s | A |
| request / delivered market-data latency | 20 ms / 40 ms | A |
| evidence checkpoints | 30 s | A |

Each venue has one declared long-future execution mandate representing a
consumer/portfolio hedge order. For every newly delivered 8 h contract it must
buy a parent 2 ABC (`200000000` raw) using 0.1 ABC (`10000000`) ordinary IOC
children. It begins 10 minutes after listing, decides every 5 minutes at phase
zero, and has a two-hour execution horizon. Each child is half the inherited
0.2-ABC maker touch, while the parent is ten touches. This C choice creates a
gradual, finite execution objective rather than a forced price or random
activity stream. It is fixed without observing P5.

The mandate uses a 15 bps tick-aligned IOC slippage bound, 10-second maximum
book age, and the inherited one-million quote-unit tick. Submission, rejection,
partial fill, completion, and residual parent quantity are separate evidence.
The parent target is an objective, never an executed position or a basis claim.

## Carry capital, size, and exact costs

The policy receives the inherited P4 participant capitalization per venue:
2,000 ABC, USD 200,000,000 spot quote balance, and USD 100,000,000 derivative
margin ledger. These values are A. No P5 outcome may revise them.

| input | exact value | class and rationale |
| --- | ---: | --- |
| maximum matched target | 1 ABC per contract | A: inherited P4 cap |
| child lot | 0.1 ABC | A |
| venue/actor minimum | 0.001 ABC | A |
| taker fee | 5 bps per leg; four expected legs = 20 bps | A |
| long-spot quote financing | 500 bps/year over exact decision-to-expiry ns | A |
| short-spot asset borrow | 500 bps/year over exact remaining ns | A |
| balance-sheet charge | 1 bp/term | A |
| margin/liquidation-risk charge | 1 bp/term | A |
| latency/non-atomic-leg charge | 1 bp/term | A |
| settlement/TWAP mismatch charge | 2 bps/term | C: nonzero explicit allowance for cash-settlement versus executable spot exit |
| post-settlement spot-exit charge | 2 bps/term | C: nonzero explicit residual-liquidity allowance distinct from the actual exit fee |
| minimum net return | 1 bp exact rational | A |
| leg order | spot first, then dated future | A: matches the independently validated P3/P4 non-atomic workflow |
| passive exit slice | 0.001 ABC | A: inherited venue minimum |
| final exit deadline | one hour after settlement | C: observable inside the 26 h horizon for the third generation |

The fixed non-fee risk budget is seven bps. It is not scaled by observed basis.
Financing/borrow alone declines with exact remaining time. The analyzer must
recompute gross locked spread from executable touches using wide arithmetic,
then subtract every component. The treatment is not selected because it is
known to cross this hurdle; activation remains falsifiable.

## Activation and execution gates

For each paired contract/venue candidate:

1. both arms have the same valid delayed announcement and book frontiers;
2. exact time to expiry and every cost component match;
3. both arms classify the same exact net carry as eligible;
4. A emits `SHADOW_ELIGIBLE`, remains target-flat, and sends no order;
5. B changes the corresponding matched spot/future target; and
6. B receives canonical admission and fills on both entry legs, reaching at
   least one independently reconstructed 0.1-ABC matched lot.

Failure before candidate eligibility is `NOT EXERCISED`; target failure is
`FALSIFIED AT ACTIVATION`; missing matched ordinary fills are `FALSIFIED AT
EXECUTION`. A one-leg/orphan position cannot pass.

At least two complete 8 h generations per venue (six contract-venue terms per
seed) must pass links 1–5 for the primary market endpoint. Every additional
eligible treatment term is retained. Fewer cannot be replaced by selecting a
healthy venue or long maturity.

## Primary pre-settlement basis statistic

For every execution-qualified B term, `t0` is its first independently verified
target change. The identical contract, venue, and timestamps are applied to A.
At absolute whole-minute samples from the first minute at or after `t0` through
`expiry - 5 minutes` (exclusive), reconstruct the latest canonical two-sided
spot and future midpoints no older than two seconds. The positive spot
denominator is required; a signed future price remains a valid numerator.

Define direction from B's target spot and:

```text
oriented_basis_bps = direction * 10000 * (future_mid - spot_mid) / spot_mid
contract_compression = mean(A oriented basis) - mean(B oriented basis)
```

All arithmetic and the final sign are exact rationals. Each arm/contract window
needs at least 90% of its expected minute samples. The terminal five minutes and
the settlement print are excluded, so mechanical cash settlement cannot pass
the test.

The seed statistic is the equal-weight mean contract compression across every
qualifying contract and venue. Positive has the registered sign; exact zero is
no support. `SUPPORTED (screening)` requires positive seed statistics for both
117 and 119. Opposite signs are `MIXED`; complete interventions with neither
positive are `FALSIFIED`. There is no magnitude filter or alternate window.

Secondary diagnostics, unable to upgrade the verdict, are: first-to-last
quartile within-contract change, time-to-expiry slope, executable locked spread,
parent mandate completion, carry residuals, settlement/exit latency, realized
fees and PnL after proven closure, and quote/trade activity.

## Frozen population and evidence

All other fully rendered inputs remain those of P4 B-107 with seed substitution:
three venues; two own-mid spot makers and one futures maker per venue; inherited
maker sizes/spreads/ticks; one spot noise participant; one option flow
participant restricted to options; no legacy carry, dated carry, router,
triangle, parity, value-taker, VV, latent-liquidity, or future-random-flow
population. Clocks, phase, latency, funding, options, listing, settlement, risk,
and venue rules are identical within pairs.

Full JSON evidence, compact participant receipt/frontier sidecars, mandate and
carry decisions, 30-second execution checkpoints, final Greek/account and
latency reports are required. Final `greeks.json` plus `latency.json` are the
only completion sentinels. Runtime/offline exact artifact digest equality is
mandatory. Raw evidence is retained; no earlier prune gate has authority.
