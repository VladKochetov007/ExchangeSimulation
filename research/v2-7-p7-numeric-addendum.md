# V2-7 P7a numeric addendum

Status: **immutable before outcomes**.  This addendum fixes the numbers used by
the P7a development cells.  The protocol and this file are the authority for
the generated JSON configs; no result is used to choose or alter a number.

## Classification and rationale

| choice | value | class | ex-ante rationale |
|---|---:|---|---|
| venue roster | north, central, south | A | inherited V2 multi-venue environment |
| runner step | 1 s | A | inherited deterministic runner granularity |
| decision interval | 2 s | A | inherited P2 local-exposure decision clock |
| public-feed delay | 40 ms | A | inherited local information contract |
| request latency | inherited per-role profile | A | no new execution-latency input |
| fixed physical exposure | -1,000,000,000 raw ABC | C | 10 ABC is a finite producer/consumer liability and gives a $500 opening notional at the inherited $50 reference |
| request cap | 250,000,000 raw ABC | C | one quarter of the fixed target; limits each IOC and preserves ordinary partial-leg risk |
| disabled/control margin | 12,000,000 raw USD | C | 4.17x gross leverage at the opening notional; finite buffer and same account roster |
| lower active margin | 12,000,000 raw USD | C | isolates activation at the lower-risk ladder level |
| higher active margin | 6,000,000 raw USD | C | 8.33x gross leverage while remaining above the exchange's approximately 5,000,000 raw initial requirement |
| initial spot USD | 50,000,000 raw USD | C | fixed small operating wallet; spot balance is not used as a perp reserve |
| taker fee | configured 5 bps | A | inherited venue fee; no subsidy |
| max exposure bound | 1,000,000,000 raw ABC | C | equals the declared fixed liability and prevents hidden leverage growth |
| exposure step | 1 raw ABC | C | required schema field but unused by fixed policy; nonzero validation without RNG-driven state changes |
| tick size | 1,000,000 raw quote | A | inherited ABC-PERP grid |
| development seeds | 307, 311 | C | fresh odd screening seeds not used by completed P4/P5/P6 development or holdout cells |
| holdout seeds | 313, 317, 331 | C | fresh untouched seeds reserved before any outcome inspection |
| preflight horizon | 15 min | C | cheap mechanics-only activation check; cannot score risk |
| registered horizon | 4 h | C | allows entry, ordinary market evolution, and repeated risk checks without a 24h campaign before activation is known |
| post-entry observation | 30 min | C | observes residual position and any delayed risk event after target entry |
| evidence | full + receipts/frontiers + strict risk | A | inherited V2 scientific evidence contract |

The exchange's inherited perp rates are 10% initial margin, 5% maintenance,
and 7.5% warning margin.  At an opening price of 50 USD, the 10-ABC target is
approximately 500 USD notional (50,000,000 raw quote), requiring approximately
5,000,000 raw quote initial margin.  The 6,000,000 and 12,000,000 raw deposits
therefore represent an ex-ante finite-collateral ladder; they are not chosen
from a realized mark path.  A positive liquidation/deficit result remains
contingent on ordinary fills and subsequent marks.

## Exact arm serialization

The immutable cell identifiers are:

```text
C-307, C-311  disabled fixed-liability roster control
L-307, L-311  enabled fixed-liability, 12,000,000 raw initial margin
H-307, H-311  enabled fixed-liability,  6,000,000 raw initial margin
```

All fields not listed as a P7 field are byte-for-byte inherited from the
checked base config `research/configs/v005-stress-perp.json`, except that
`cross_asset_spot_graph` is explicitly `false` to remove the known CDF/USD
mark-viability censor from this perp-only mechanism screen.  The generated
configs must retain the full evidence contract and must record the P7 policy
version in their manifest.

No holdout config may be generated until both development levels have passed
the activation/evidence gate.  If an active level is inactive or invalid, all
its registered cells remain in the ledger and no number is changed.

