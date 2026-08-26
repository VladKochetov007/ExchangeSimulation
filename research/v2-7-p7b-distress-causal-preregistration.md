# V2-7 P7b — unit-corrected leveraged liability

Status: **preregistered design; no P7b outcome or preflight world has been
inspected**.

P7a established that the fixed-liability actor can enter and hold a declared
perpetual hedge, but its participant risk path was not exercised. A post-
outcome raw-unit audit found that the P7a prose labels overstated leverage by a
factor of ten; the immutable P7a cells remain historical and are not rerun.
P7b is a new protocol that corrects that unit interpretation and asks a
narrower question:

> With the same persistent physical liability and ordinary venue execution,
> does an explicitly capitalized, near-initial-margin perpetual hedge produce
> an independently observable participant margin/forced-close event during a
> one-day market horizon?

This is a mechanism-identification experiment, not a crisis-realism or price-
path target. No price shock, threshold override, synthetic fill, atomic leg,
funding read, or direct price anchor is added.

## Fixed information and execution contract

The participant is the audited `v2_7_fixed_liability_v1` local-feed actor. It
has a fixed off-exchange physical exposure of `-1,000,000,000` raw ABC (10
ABC), so its declared perpetual target is `+1,000,000,000` raw ABC. It submits
ordinary IOC orders at the last locally delivered executable touch, capped at
`250,000,000` raw ABC per request. Once the target is filled it holds it and
does not reopen after an exchange-side close. All P7a receipt/frontier,
position, fill, lifecycle, conservation and risk evidence requirements remain
in force.

## Development design

| cell | participant | initial perp margin | gross leverage at the inherited $50,000 reference | purpose |
|---|---|---:|---:|---|
| C | installed, disabled | 10,000,000,000 raw ($100,000) | 5.0x if active | roster/evidence control |
| L | enabled fixed liability | 10,000,000,000 raw ($100,000) | 5.0x | lower-risk active level |
| H | enabled fixed liability | 5,500,000,000 raw ($55,000) | 9.1x | near-initial-margin active level |

The target's opening notional is approximately $500,000 and the inherited 10%
initial-margin rule therefore requires approximately $50,000 (plus the
configured five-basis-point taker fee). H is deliberately only a small,
ex-ante buffer above that requirement; L supplies a wider finite-capital
comparison. These values are derived from the contract requirement and stated
capital policy, not from a realized price path. With the inherited 5%
maintenance rate, the approximate long liquidation levels are $42,100 (L) and
$46,800 (H); these are diagnostics, not targets or guarantees.

All other fields inherit `research/configs/v005-stress-perp.json` exactly as in
P7a: three venues, cross-asset spot graph disabled, projected maker rosters,
ordinary fees/borrowing/funding, local 40-ms public-feed delay, 2-second actor
decisions, one-second runner step, full evidence and strict risk checks. The
P7a physical exposure, request cap, tick, quote wallet and policy are unchanged.
The registered horizon is 24 simulated hours to cover a one-day market-risk
window; a mechanics-only preflight may use at most 15 simulated minutes and
cannot score distress. No P7a clock, liquidity or population is tuned.

Development seeds are **337 and 341**. Untouched holdouts are **347, 349 and
353**, reserved before any P7b run. Holdouts are not permitted unless both
active levels pass activation/evidence gates and the participant risk path is
observable in at least one development cell.

## Separate endpoints and promotion rule

The analyzer must report independently:

1. participant decisions, local receipt frontier and ordinary IOC outcomes;
2. target entry, admitted/fill-linked quantity and terminal hedge gap;
3. contemporaneous participant margin checks and expected breaches;
4. participant-specific margin-call/forced-close events and position reduction;
5. residual position/collateral after any close;
6. deficit, insurance transfer and bankruptcy, each with exact balance and
   conservation reconstruction;
7. generic liquidations for other accounts, explicitly not substituted for
   participant distress.

The development classification is `SUPPORTED (screening)` only for a valid
fixed-liability activation, `FALSIFIED AT ACTIVATION` or `FALSIFIED AT
EXECUTION` for invalid entry evidence, `NOT EXERCISED` when participant risk
or deficit triggers have zero count, and `MIXED` only when registered active
levels disagree after valid activation. P7b is promoted to holdout only if
both active levels have valid activation and at least one active cell has a
participant-built, independently reconstructed risk event. Generic
liquidations alone do not satisfy this rule.

No basis, funding-anchoring, profitability, market-stability or realism claim
is licensed by P7b. If risk is inactive, the result is retained and the next
distress design must change the economic exposure source or horizon through a
new preregistration rather than lowering a threshold post hoc.

## Adversarial and provenance requirements

Before scoring, the fail-closed extractor must verify the immutable config
delta, full evidence, receipt/frontier causality, ordinary order admission,
position/fill links, lifecycle, settlement, expiry, conservation and all risk
arithmetic. It must compare runtime and offline evidence-artifact digests and
record simulator revision, binary, config hash, analysis revision and raw-log
retention. Mutations cover reversed liability sign, dropped/duplicated
decisions or fills, future/delayed/duplicated/reordered receipts, wrong local
touch, request-cap violations, dropped/duplicated liquidation evidence, stale
marks/collateral and synthetic balance resets. Zero trigger count is
`NOT TESTED/NOT EXERCISED`, never a pass.

