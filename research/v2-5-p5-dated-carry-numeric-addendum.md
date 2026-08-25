# V2-5 P5 — immutable dated-carry numeric addendum

Status: **preregistered before P5 implementation, preflight, or outcome
inspection.** It completes the numeric gate in
[`v2-5-p5-dated-carry-causal-preregistration.md`](v2-5-p5-dated-carry-causal-preregistration.md).

The source revision is `ea0eb66`. The exact configs are under
`research/configs/v2-5-p5/`. Unknown P5 fields are intentionally committed
before their implementation; running these configs before the implementation
and structural preflight is prohibited.

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
| A-117 | `83fa1d0431825e3926c416cc9a81ae8b7d6df278315bd6abd9df1372ea844f12` |
| B-117 | `3d7e9629d2387ab531138250ff72ac034115596fcaf34985365b7bd6988739be` |
| A-119 | `c40d0a324d8b827a09db7bc2594df584eee87377c513675fff3dc83ee24bcad4` |
| B-119 | `b74a090f871274ad8c8a4b4260179649e7899f36a712ac1507b8e44ead489403` |
| A-139 | `5f225a67355a60294d4a3fee20d53db4af4d420359e4a670d564e052a7516da1` |
| B-139 | `9b0a9f3f0047045c23dd1d388e4063e7633a094a518062006dfce15993c18fdb` |
| A-149 | `4c3d7fe31401a157534a308ea98209c8c6b0dd5590d50165d627e6650b0b02f1` |
| B-149 | `fdebc14de7ce3ee12be5a04a55ed07460d3fd2f82a3699995006e61fd861edf4` |
| A-151 | `090169094c3116f710512af7f222ded7ea9328d6c0474fbd955ceb98e3d5705c` |
| B-151 | `77fa8e726dd56e41515d8ac9947a742227b9dbaa883b87da984bece59814b4a3` |

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
