# V2-7 P7d numeric addendum

Status: **immutable before P7d outcomes**.  The accompanying preregistration
and hash-pinned configs are the authority; no P7d outcome or preflight world
may be used to choose a number.

The full table and rationale are in
[`v2-7-p7d-directional-distress-causal-preregistration.md`](v2-7-p7d-directional-distress-causal-preregistration.md).
The compact signed values below make the target and capital arithmetic
auditable without relying on prose labels.

| field | value |
|---|---:|
| target long | `+2,000,000,000` raw ABC |
| target short | `-2,000,000,000` raw ABC |
| maximum request | `500,000,000` raw ABC |
| own perp margin | `5,500,000,000` raw USD |
| spot operating wallet | `50,000,000` raw USD |
| maximum quote borrow | `5,500,000,000` raw USD |
| horizon | `4h` |
| preflight ceiling | `15m` |
| development seeds | `431, 433` |
| untouched holdouts | `439, 443, 449` |

Classifications are fixed as follows:

* **A inherited:** venue roster, cross-asset graph projection, tick, fees,
  delayed local-feed link, decision/risk clocks, population, liquidity,
  P3e settings, evidence schema, and P7c own margin/wallet.
* **B economically motivated:** quote-margin borrowing is the ordinary finite
  balance-sheet action required to carry a target whose initial requirement
  exceeds the desk's own collateral.
* **C experimental design:** target size/orientations, child cap, borrow cap,
  four-hour horizon, preflight ceiling, and fresh seed sets.  These values are
  fixed to register both signs and a bounded two-day-risk follow-up without
  selecting a realized price path.

No P7d holdout config may be rendered until the development promotion rule in
the preregistration is met.
