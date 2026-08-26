# V2-7 P7a development result

Status: **NOT EXERCISED** for participant distress, with **SUPPORTED
(screening)** activation of the fixed-liability execution contract. This is a
development result under the immutable P7a protocol; no holdout seeds were
consumed and no market-level realism claim is licensed.

## Provenance and evidence gate

All six registered cells completed at the simulator revision recorded by the
run metadata (`8d36f74a401ada2218f169fba8c6d2e2ffcd0d12`) with binary
SHA-256
`8c5420573b1d38a7d32cde1ad7798a3c8aff22c9ac8eef80bf65210edc6ed549`,
4 simulated hours, full evidence, and the required `greeks.json` and
`latency.json` completion sentinels. The corrected analyzer was
`0b672601afffea49a6c683f84115230914d0f5c6` with SHA-256
`db78fdf3084458ed89b6854147c80667592e388a9b6860b149d6498f15a9470c`.

Each cell passed the fail-closed `v2-7-p7a-distress-v1` contract. All 15
required metric artifacts were present; receipt/frontier checks, conservation,
positions, fill-position links, lifecycle, expiry, settlement, risk arithmetic
and generic liquidation accounting were valid. Runtime and offline
persisted-evidence artifact digests agreed exactly. Raw evidence remains
retained; this experiment has no prune authority.

## Registered cells

`filled_qty` and terminal gaps are in raw ABC units. `generic liquidations` are
all exchange liquidation events and are not attributed to the P7 hedger unless
their top-level event `client_id` is 59. `actor liquidations` is an independent
scan of the persisted venue evidence.

| cell | seed | decisions | enabled | admitted | fills | filled qty | terminal gaps (central/north/south) | active mark checks | expected breaches | actor liquidations | generic liquidations | deficits | evidence events | evidence digest |
|---|---:|---:|---:|---:|---:|---:|---|---:|---:|---:|---:|---:|---:|---|
| C | 307 | 21,600 | 0 | 0 | 0 | 0 | 1e9/1e9/1e9 | 0 | 0 | 0 | 557 | 0 | 14,241,527 | `c37aeb57de2e1f793c454634369fdbc9e62a9ca180b87f0cd4a94226bb318530` |
| C | 311 | 21,600 | 0 | 0 | 0 | 0 | 1e9/1e9/1e9 | 0 | 0 | 0 | 195 | 0 | 14,236,505 | `fbec628015a8e310fc386e2e9040be0f7143aa486f17f32568c5cf5976b2bdd0` |
| L | 307 | 21,600 | 21,600 | 109 | 21 | 3e9 | 0/0/0 | 43,177 | 0 | 0 | 282 | 0 | 14,320,521 | `afc1bae7ffa823d53d237632f29d356c0fa77ef69394ca18d02fb1627a508e59` |
| L | 311 | 21,600 | 21,600 | 163 | 18 | 3e9 | 0/0/0 | 43,170 | 0 | 0 | 274 | 0 | 14,300,444 | `acd25908ad89af61f7b7d781a42f27b65ad5fd15b71f24bcf805b36f6fc44bec` |
| H | 307 | 21,600 | 21,600 | 109 | 21 | 3e9 | 0/0/0 | 43,177 | 0 | 0 | 282 | 0 | 14,320,521 | `1a634437b5b661f762701f9c2ca6f047988584474a17f2aeb1b017bf5d4f842e` |
| H | 311 | 21,600 | 21,600 | 163 | 18 | 3e9 | 0/0/0 | 43,170 | 0 | 0 | 274 | 0 | 14,300,444 | `835398f7d69aae225340aea01804b8b8cf33543339e1baa82462af0fcbb41952` |

The exact machine-readable score is
`research/artifacts/v2-7-p7a/p7a-development-score.json`; it is reproducible
with `scripts/score-v2-7-p7a-development.sh` and the six immutable configs
verified by `scripts/check-v2-7-p7-configs.sh`.

## Preregistered predicates and interpretation

* **Control validity:** both C cells remained disabled with no orders or fills.
* **Fixed-liability activation:** both L and both H cells independently made
  ordinary IOC requests, filled the complete declared 3e9 target in both seeds,
  and ended with zero target gap in every venue. Receipt/frontier and policy
  replay checks passed. This supports only the participant activation/execution
  contract at screening level.
* **Participant risk activation:** the active actor had 43,170--43,177
  independent mark checks but zero expected breaches and zero observed margin
  calls. No persisted venue liquidation event named client 59 in any cell. The
  participant risk path is therefore **NOT EXERCISED**.
* **Forced close:** generic exchange liquidations occurred (195--557 per
  cell), and every one passed independent path/conservation checks, but they
  affected other accounts. They do not activate the fixed-liability actor's
  forced-close mechanism. Participant forced close is **NOT EXERCISED**.
* **Deficit/insurance/bankruptcy:** all cells had zero deficits, insurance
  debits and deficit residuals. This path is **NOT EXERCISED**.
* **Margin ladder:** L and H have different registered initial margins, but no
  participant breach occurred and the observed activation/fill endpoints were
  identical within each seed. A capital-dependent distress effect is **NOT
  EXERCISED**, not evidence that margin is generally unimportant.

The protocol's promotion rule is not met because the participant-built risk
path is not observable. Registered holdouts 313, 317 and 331 are therefore not
licensed. P7a numbers, liquidity, clocks and population are not changed in
response. The result enters the redesign ledger: the fixed liability was large
enough to activate ordinary hedging but the finite collateral ladder did not
put this participant near a contemporaneous margin breach under the registered
market path. Any P7b must be a new preregistration with an ex-ante economic
leverage/distress mechanism; it must not retrofit P7a or use post-outcome shock
tuning.
