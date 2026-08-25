# V2-7 P7a numeric addendum

Status: **immutable before outcomes**.  This addendum fixes the numbers used by
the P7a development cells.  The protocol and this file are the authority for
the generated JSON configs; no result is used to choose or alter a number.

## Pre-outcome unit erratum

The first draft of this addendum used `12,000,000` and `6,000,000` raw USD for
the two margins.  The repository's `USD_PRECISION` is `100,000`, while the
inherited ABC bootstrap is `$50,000`; the target's 10% initial margin is
therefore `5,000,000,000` raw USD, not `5,000,000`.  Before any P7 world was
run or inspected, those draft values were superseded by `120,000,000,000` and
`60,000,000,000` raw USD below.  The superseded values are retained only for
provenance and must not be used to generate a config.

## Classification and rationale

| choice | value | class | ex-ante rationale |
|---|---:|---|---|
| venue roster | north, central, south | A | inherited V2 multi-venue environment |
| runner step | 1 s | A | inherited deterministic runner granularity |
| decision interval | 2 s | A | inherited P2 local-exposure decision clock |
| public-feed delay | 40 ms | A | inherited local information contract |
| request latency | inherited per-role profile | A | no new execution-latency input |
| fixed physical exposure | -1,000,000,000 raw ABC | C | 10 ABC is a finite producer/consumer liability and gives a $500,000 opening notional at the inherited $50,000 reference |
| request cap | 250,000,000 raw ABC | C | one quarter of the fixed target; limits each IOC and preserves ordinary partial-leg risk |
| disabled/control margin | 120,000,000,000 raw USD | C | 4.17x gross leverage at the opening notional; finite buffer and same account roster |
| lower active margin | 120,000,000,000 raw USD | C | isolates activation at the lower-risk ladder level |
| higher active margin | 60,000,000,000 raw USD | C | 8.33x gross leverage while remaining above the exchange's approximately 5,000,000,000 raw initial requirement |
| initial spot USD | 50,000,000 raw USD | C | fixed small operating wallet; spot balance is not used as a perp reserve |
| taker fee | configured 5 bps | A | inherited venue fee; no subsidy |
| max exposure bound | 1,000,000,000 raw ABC | C | equals the declared fixed liability and prevents hidden leverage growth |
| exposure step | 1 raw ABC | C | required schema field but unused by fixed policy; nonzero validation without RNG-driven state changes |
| tick size | 10,000 raw quote | A | inherited `SpotTickQuoteUnits` ABC-PERP grid in the checked base config |
| development seeds | 307, 311 | C | fresh odd screening seeds not used by completed P4/P5/P6 development or holdout cells |
| holdout seeds | 313, 317, 331 | C | fresh untouched seeds reserved before any outcome inspection |
| preflight horizon | 15 min | C | cheap mechanics-only activation check; cannot score risk |
| registered horizon | 4 h | C | allows entry, ordinary market evolution, and repeated risk checks without a 24h campaign before activation is known |
| post-entry observation | 30 min | C | observes residual position and any delayed risk event after target entry |
| evidence | full + receipts/frontiers + strict risk | A | inherited V2 scientific evidence contract |

The exchange's inherited perp rates are 10% initial margin, 5% maintenance,
and 7.5% warning margin.  At the inherited opening price of $50,000, the
10-ABC target is approximately $500,000 notional (`50,000,000,000` raw quote),
requiring approximately `5,000,000,000` raw quote initial margin.  The
`60,000,000,000` and `120,000,000,000` raw deposits therefore represent an
ex-ante finite-collateral ladder; they are not chosen from a realized mark path.
A positive liquidation/deficit result remains contingent on ordinary fills and
subsequent marks.

## Exact arm serialization

The immutable cell identifiers are:

```text
C-307, C-311  disabled fixed-liability roster control
L-307, L-311  enabled fixed-liability, 120,000,000,000 raw initial margin
H-307, H-311  enabled fixed-liability,  60,000,000,000 raw initial margin
```

All fields not listed as a P7 field are byte-for-byte inherited from the
checked base config `research/configs/v005-stress-perp.json`, except that
`cross_asset_spot_graph` is explicitly `false` to remove the known CDF/USD
mark-viability censor from this perp-only mechanism screen.  Because that
graph removes CDF/USD and ABC/CDF books, the generated configs also restrict
the inherited fixed-distance/imbalance maker symbol rosters to
`["ABC/USD", "ABC-PERP"]` and remove the two unavailable CDF noise-target
entries; these are validity-preserving projections, not economic treatments.
The generated configs must retain the full evidence contract and must record
the P7 policy version in their manifest.

The checked renderer is `scripts/render-v2-7-p7-configs.sh`; the exact config
hashes are verified by `scripts/check-v2-7-p7-configs.sh`:

| cell | SHA-256 |
|---|---|
| C-307 | `633750a02818b8204e174e81126ec2e506ec75b1d6cbba48d22f1feba60aca82` |
| C-311 | `e604150c2d23528fa9e684311ca6a141e3a4ddbe996453252a0241c37a9d5c85` |
| L-307 | `45083d2bb6527a3c61b53a638c95afe42815c0e9186c0c0dfc68180f28d9fd79` |
| L-311 | `c1ef3301735b059ddca8a07af30f69244af7828d769b0375a3bc67c5f5b5fdd8` |
| H-307 | `b2606c9757a7d8106d1dc1a94c52c23d0a204c60f184aa93b4ef2e55f7e7fbdc` |
| H-311 | `0f97b833c03df7901c6b990b56fa6547a2e1652c310db146fa42a945adfedda1` |

No holdout config may be generated until both development levels have passed
the activation/evidence gate.  If an active level is inactive or invalid, all
its registered cells remain in the ledger and no number is changed.
