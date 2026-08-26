# V2-7 P7b development results

Status: **complete development screen; `NOT EXERCISED` for participant
distress**.

The preregistration in
[`v2-7-p7b-distress-causal-preregistration.md`](v2-7-p7b-distress-causal-preregistration.md)
is unchanged.  This document records outcomes only; it does not promote any
holdout seed or make a market, funding, profitability, or realism claim.

## Contract and provenance

All six development cells are final 24-hour, full-evidence worlds.  The
simulator source revision is `e1a6d80c515c91d21c0a91296b6ad16c19cb2a6b`; the
`multivenue` binary SHA-256 is
`e3f95a47237437afd850c0fc9d38a724de584a961d7181d9552eb72b904293ea`.  Every
cell uses the registered completion sentinels `greeks.json` and `latency.json`,
and every extractor returned status `0` with the 15 required artifacts.

The fail-closed analysis contract is `v2-7-p7b-distress-v1`.  Runtime and
offline persisted-evidence artifact hashes agree exactly in every cell.  The
first five cells were extracted under analysis revision `ab78e4e`; L-341 was
extracted under `e021a34` after a scorer-only nested-field check was corrected.
The analyzer SHA-256 is identical in all cells:
`2dc0b8b3dd99f47d403d5a720438d527a0628193794fb99569ca879ce827dcb7`.

| cell | seed | config SHA-256 | evidence events | evidence digest |
|---|---:|---|---:|---|
| C | 337 | `d9bd9f0928d80d3cb289037110f5c22a1af2ed6de1e417ece9b94e16f78539bf` | 82,046,895 | `f0672a9634936e131c64fef857e53eb49ff50e7033ebd4a07bf876cd4c79daf3` |
| C | 341 | `c18a9df7d70b36964dae40d8bccb9fcb64b14e38c8f072384af2a7fea8be36b7` | 82,553,464 | `704f25d1082104474541214507b6caf83584ac8a58e78d06f1a0d9017cbe48b8` |
| L | 337 | `eadae97f8363e25d73584d9699194f2e1d7673f01f0888071b04211a8f94d468` | 81,679,891 | `8a9dc8faa32ebb9d6ea5f98d2d659d72d1e881601b6d0fc5d0c5f0a11231b215` |
| L | 341 | `c5c037dc5bc270dfeb40cd38ddbbc847aabf69c25069ff6a4d016b705f6f7c39` | 82,511,453 | `4fea4b51739e37f27958c6fa393949e457a784fdbef23293d6c9bd25b20eefd8` |
| H | 337 | `eee3960cea6f62d38125bbf2c151b18242ea9d15825109bb5e75c5df6ce4749b` | 81,679,891 | `f1af5a4f3eb322b20f229cf4e87f1b54ef0b92b06f4e7f5b95efeb284fbcfeb9` |
| H | 341 | `7f4a44a981b5ec13938856cc87dc9458a04ea3038984af4f1afeca74f35f6277` | 82,511,453 | `4b6e43e6c4e3cf919f820d80cf87d2277f13bcd952640ba22f0d530be799f600` |

The raw worlds and all evidence remain retained under
`research/artifacts/v2-7-p7b/full/`; P7b has no prune authority.  An earlier
analyzer-only interruption for C-341 produced no scored evidence and remains
historical only.

## Activation and execution

The control cells are correctly disabled: 129,600 decisions, zero enabled
decisions, zero submissions and zero fills.  Each active cell has 129,600
enabled decisions, all submissions accepted, 28 or 34 ordinary IOC fills, and
exactly `3,000,000,000` raw quantity filled.  The fixed-liability target is
therefore reached on all three venues in all active cells; each venue's
terminal absolute hedge gap is `0`.

| cell | seed | decisions | enabled | accepted | fills | filled quantity | terminal gaps |
|---|---:|---:|---:|---:|---:|---:|---|
| C | 337 | 129,600 | 0 | 0 | 0 | 0 | 1e9 / 1e9 / 1e9 |
| C | 341 | 129,600 | 0 | 0 | 0 | 0 | 1e9 / 1e9 / 1e9 |
| L | 337 | 129,600 | 129,600 | 162 | 34 | 3e9 | 0 / 0 / 0 |
| L | 341 | 129,600 | 129,600 | 178 | 28 | 3e9 | 0 / 0 / 0 |
| H | 337 | 129,600 | 129,600 | 162 | 34 | 3e9 | 0 / 0 / 0 |
| H | 341 | 129,600 | 129,600 | 178 | 28 | 3e9 | 0 / 0 / 0 |

Receipt/frontier, order-outcome, position, fill, lifecycle, settlement,
expiry, conservation, and risk arithmetic checks all pass.  The independent
raw venue scan found zero `liquidation` and zero margin events for client 59 in
each active cell.  The active margin replay recorded 259,042--259,156 mark
checks per cell, with zero expected breaches, zero observed checks, and zero
missing/unexpected checks.

## Risk and accounting outcome

No participant-specific margin call, forced close, deficit, insurance transfer,
or bankruptcy occurred.  The generic liquidation auditor did observe other
accounts, but these are not substitutes for the registered participant path:

| cell | seed | generic liquidations | affected accounts | deficits | insurance residual | invalid/path/conservation failures |
|---|---:|---:|---:|---:|---:|---:|
| C | 337 | 13,656 | 14 | 0 | 0 | 0 / 0 / 0 |
| C | 341 | 13,942 | 14 | 0 | 0 | 0 / 0 / 0 |
| L | 337 | 13,516 | 14 | 0 | 0 | 0 / 0 / 0 |
| L | 341 | 13,063 | 14 | 0 | 0 | 0 / 0 / 0 |
| H | 337 | 13,516 | 14 | 0 | 0 | 0 / 0 / 0 |
| H | 341 | 13,063 | 14 | 0 | 0 | 0 / 0 / 0 |

These generic events are retained as a diagnostic that the venue liquidation
machinery executes for the wider population, not as evidence that the fixed-
liability actor reached distress.

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
| holdout seeds 347/349/353 | not consumed |

The development score is the machine-readable
[`p7b-development-score.json`](artifacts/v2-7-p7b/p7b-development-score.json).
The scorer correction in commit `e021a34` fixed only a nested receipt-field
selection in the analysis predicate; it did not alter simulator code,
evidence, RNG, scheduling, or the already-complete extraction outputs.

## Interpretation and next gate

P7b identifies a valid, ordinary fixed-liability hedge and complete evidence
contract, but the two unit-corrected margin levels do not exercise participant
distress in a one-day ordinary market.  This is not evidence that liquidation
semantics are wrong, nor evidence about funding, basis, profitability, or
market realism.  Holdouts are not licensed.  The next distress experiment
must be newly preregistered around a genuinely different economic exposure
source or risk horizon; it must not lower thresholds or tune liquidity after
these outcomes.
