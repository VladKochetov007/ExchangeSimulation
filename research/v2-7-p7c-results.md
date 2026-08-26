# V2-7 P7c development results

Status: **complete development screen; `NOT EXERCISED` for
participant-specific distress**.

The preregistration in
[`v2-7-p7c-distress-causal-preregistration.md`](v2-7-p7c-distress-causal-preregistration.md)
and numeric addendum are unchanged.  This document records the completed
development cells only.  It does not consume the reserved holdouts or make a
funding, basis, profitability, liquidation-realism, or market-realism claim.

## Contract and provenance

All four development cells are final 48-hour, full-evidence worlds.  The
simulator source revision is `4f674d36faf909e602f13f8975215c29a5ab8e0a`; the
`multivenue` binary SHA-256 is
`5f1f411f2a4a0cfab14a016155a9b1d8dbb2fbb7ccc530d2b652a277da80480d`.  The
registered completion sentinels (`greeks.json` and `latency.json`) are
present in every cell, and the fail-closed extractor returned status `0` with
all 15 registered metric artifacts plus complete analysis metadata.

The analysis contract is `v2-7-p7c-distress-v1`.  Runtime and offline
persisted-evidence artifact hashes agree exactly in every cell.  The raw
worlds remain retained under `research/artifacts/v2-7-p7c/full/`; this
protocol has no prune authority.

| cell | seed | config SHA-256 | evidence events | evidence digest |
|---|---:|---|---:|---|
| C | 367 | `a2b225ed060099ebc87aeec00a95501d6e5296de095897f4c44eb5003d4ef1b9` | 167,680,423 | `f6a1feefc786a6238acdd7de0015b3f20333ea1106cdcc8827ed7af1befa71f6` |
| C | 371 | `7627709f7fceb3fd9f8d9c40e7020752639f316a9eb12f268c79acc4af5e4488` | 166,839,082 | `0859682fb504982d8e568e8ba30779af7b8e85a8c34f31f5b970439223bb4401` |
| T | 367 | `0a6fc74c0f89e2b0e2a7fffa8d017705021afea7645cb46be0ffaf0cd179651a` | 167,222,940 | `234431d553f21f713a9103bcb979902f38a6859b5aa3d5b6f87a07babc741ae7` |
| T | 371 | `6a3d8c6cb33ab4587eee662a8a7ace5888583e19c02d83e4279a45e4d26cd1b1` | 167,193,301 | `f757a81afca9a161aae7143388420021e0f9ddcf4c48d93bb1d3781e67ded18c` |

The first C-367 extraction attempt was terminated for memory safety while two
large analyzers were concurrent; the raw evidence was preserved and the
cell was subsequently extracted sequentially to status `0`.  This operational
retry did not change simulator output or analysis semantics.

## Activation and ordinary execution

The controls remained disabled: 259,200 decisions, zero submissions and zero
fills in each seed.  Both enabled treatments ran the fixed-liability actor for
the full 48-hour horizon, admitted all submitted IOC orders, and reached the
declared `3,000,000,000` raw-quantity target across the three venues with zero
terminal target gap.  All receipts were delivered before decisions, and the
independent order, position, fill, lifecycle, settlement, expiry and
conservation checks passed.

| cell | seed | enabled / decisions | accepted | fills | filled quantity | terminal gaps (N/C/S) |
|---|---:|---:|---:|---:|---:|---|
| C | 367 | 0 / 259,200 | 0 | 0 | 0 | 1e9 / 1e9 / 1e9 |
| C | 371 | 0 / 259,200 | 0 | 0 | 0 | 1e9 / 1e9 / 1e9 |
| T | 367 | 259,200 / 259,200 | 95 | 23 | 3e9 | 0 / 0 / 0 |
| T | 371 | 259,200 / 259,200 | 110 | 26 | 3e9 | 0 / 0 / 0 |

The treatment fills were all reducing fills.  Per-venue fill/submission counts
were 13/34, 6/48 and 4/13 for T-367 (central/north/south), and 6/23, 7/52
and 13/35 for T-371.

## Risk reachability

The treatment performed 518,332 and 518,312 participant mark checks,
respectively.  Neither cell produced an expected margin breach, participant
margin event, participant liquidation/forced close, deficit, insurance
transfer, or bankruptcy.  The independent raw venue scan found zero
participant-specific client-59 margin or liquidation events in both treatment
cells.  Risk arithmetic, stale-mark/collateral, missing/unexpected-check and
conservation counters were all zero.

| cell | seed | active mark checks | expected breaches | participant risk events | generic liquidations | affected accounts | deficits | insurance residual |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| C | 367 | 0 | 0 | 0 | 32,493 | 26 | 0 | 0 |
| C | 371 | 0 | 0 | 0 | 37,545 | 26 | 0 | 0 |
| T | 367 | 518,332 | 0 | 0 | 32,422 | 22 | 0 | 0 |
| T | 371 | 518,312 | 0 | 0 | 34,392 | 22 | 0 | 0 |

Generic liquidation events are for other accounts and cannot substitute for
the registered participant risk path.  Their presence shows that venue risk
machinery executes for the wider population, not that this fixed-liability
actor reached distress.

## Registered classification

| predicate | result |
|---|---|
| control validity | `true` |
| fixed-liability activation | `SUPPORTED (screening)` |
| evidence/information integrity | `true` |
| participant margin-call path | `NOT EXERCISED` |
| participant forced-close path | `NOT EXERCISED` |
| deficit/insurance/bankruptcy path | `NOT EXERCISED` |
| development classification | `NOT EXERCISED` |
| holdouts 373/379/383 | not consumed |

The machine-readable score is
[`p7c-development-score.json`](artifacts/v2-7-p7c/p7c-development-score.json).

## Interpretation and next gate

P7c supports, at screening level, that the persistent fixed-liability actor
can execute and hold the declared three-venue hedge over a two-day horizon
with a complete independently checked evidence path.  It does **not** show
that the two-day horizon makes participant distress reachable: both treatment
seeds had zero expected breaches and zero participant-specific risk events.
The registered distress predicates therefore remain `NOT EXERCISED`, not
falsified and not validated by the generic liquidation counts.

Do not consume the untouched holdouts or tune the P7c capital, thresholds,
liquidity, clocks, or population.  A subsequent distress protocol must use a
new economically motivated exposure source or risk horizon, with its own
pre-registration and activation predicate, rather than lowering a trigger
after this result.  The V2-5 funding/carry work remains a separate causal
program.

