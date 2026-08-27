# V2-6 option causal disposition

Status: **NOT IDENTIFIED for an emergent option-surface or hedge-response
claim; explicit limitation accepted.**

This disposition follows the completed P6-R1 viability and untouched-stage
replication.  It does not reopen P6/P6-R1, consume a seed, change an option
configuration, or rewrite a historical result.  The stage evidence remains
valid for what it actually measured: executable option ecology and participant
activation.

## Evidence reviewed

The review used the immutable P6 and P6-R1 preregistrations and numeric
addenda, the original incomplete P6 result, the P6-R1 development and holdout
results, all machine summaries, and independently inferred market-price IV,
parity, dealer-exposure and hedge artifacts.  P6-R1 development used O0--O4 ×
211/213; its untouched replication used O0--O4 × 223/227/229.  All fifteen
holdout cells passed their viability/evidence contracts.  These seeds are
consumed under their own P6-R1 policy and are not available for a new option
causal experiment.

The P6-R1 machine summaries are:

- `research/artifacts/v2-6-p6r1/development-summary.json`;
- `research/artifacts/v2-6-p6r1/holdout-summary.json`.

The detailed stage reports are
`v2-6-p6r1-viability-results.md` and `v2-6-p6r1-holdout-results.md`.
Market IV is inferred from persisted market prices; dealer/value-taker model
volatility is not substituted for market IV.  The current Black--76 inversion
continues to report explicit positive-domain skips rather than coercing
non-positive inputs.

## Stage-to-question mapping

| stage/contrast | declared delta | what the evidence establishes | causal status |
|---|---|---|---|
| O0 | one flat-vol dealer; no hedge, liability, SABR or VV participant | listing, quoting, settlement and market-price IV inversion are executable | `SUPPORTED (screening)` activation only |
| O1−O0 | three heterogeneous strike-flat dealers replace O0 dealer | non-flat descriptive surfaces and active heterogeneous dealers occur | no registered effect-size/corridor; not a causal emergence verdict |
| O2−O1 | adds a liability user **and** heterogeneous dealer delta hedging | liability/hedge decisions, option inventory and hedge-tagged underlying flow are active | bundled intervention; O2 transmission sign/corridor was not fixed, so `NOT IDENTIFIED` |
| O3−O2 | adds one SABR-view value taker | SABR activity and option fills survive on development and holdouts | clean activation contrast, but any surface change is explicitly inherited prior structure; no emergent claim |
| O4−O3 | adds one Vanna--Volga risk-transfer desk | VV activity and fills survive on development and holdouts | clean activation contrast, but any surface change is explicitly inherited prior structure; no emergent claim |

The original P6 O3/O4 failures are historical incomplete worlds.  P6-R1's
explicit collateral mark/cap repaired viability and did not expose a hidden
fair-value oracle.  That repair is an accounting/authorization mechanism, not
a smile prior and not evidence of market realism.

## Why the causal question is not identified

The target question is not simply whether a smile-shaped table can be printed;
it is how much option structure and option-to-underlying feedback exists before
explicit smile priors, and what changes when those priors are introduced.
The retained evidence cannot identify that question under the fixed rules:

1. **The pre-prior O2 intervention is bundled.**  O1→O2 changes both
   end-user liability demand and dealer hedge policy.  A surface or underlying
   response cannot be attributed to liability, delta hedging, their interaction,
   or the inherited heterogeneous flat-vol dealer roster.
2. **The primary O2 directional endpoint is incomplete.**  The preregistration
   required a registered transmission sign, but fixed neither sign nor an
   effect-size/corridor.  Positive observed correlations are therefore
   descriptive and cannot be promoted after the fact.
3. **O3 and O4 are prior additions.**  O3 adds an explicit SABR belief and O4
   an explicit Vanna--Volga risk-transfer belief.  Differences from O2/O3 can
   identify that those participants were active, but by construction they are
   inherited structure, not emergence.  Their activity and liquidity also
   change the participant roster, so an unregistered market-level difference
   has no fixed causal classifier here.
4. **No removal/restoration or belief-permutation was preregistered.**  The
   existing staged contrasts do not hold participant count, capital, order
   volume and execution opportunity fixed while removing only a belief prior.
   Retrospectively choosing a threshold, sign, aggregation rule or a convenient
   stage pair would be post hoc.
5. **The valid holdouts answer viability, not causality.**  Seeds 223/227/229
   reproduce all O0--O4 activation gates, but no registered surface or hedge
   causal corridor was scored on them.  Calling those observations an
   out-of-sample emergence result would overstate the evidence.

## Disposition

The option-emergence and option-to-underlying causal claims are recorded as
`NOT IDENTIFIED` and remain explicit V2 limitations.  The following narrower
claims are retained:

- the P6-R1 O0--O4 option stages are executable and their declared
  participants activate at screening level in development and untouched
  seeds;
- market-price IV, parity, dealer exposures and hedge-tagged flow are
  independently measurable in those valid cells;
- O3/SABR and O4/Vanna--Volga surface structure, if observed, is inherited
  from explicit participant priors and must not be called emergent;
- the O2 liability and delta-hedge paths are active, but their directional
  transmission effect is unresolved;
- the repaired CDF collateral mark is an authorization/accounting aid and is
  not participant information.

No additional P6 simulation, holdout, parameter sweep or option economic
change is licensed by this disposition.  A future causal study would require a
new preregistration with a matched prior-on/prior-off (or belief-permutation)
contrast, fixed activation/liquidity controls, an ex-ante surface endpoint,
and a separately signed hedge-response endpoint.  It must not be backfilled
from the consumed P6-R1 cells.

## Adversarial review

Configured Sol-xhigh reviewer agents were unavailable because of the current
model-usage limit.  A fresh primary-agent red-team pass is recorded in
`research/reviews/v2-6-option-causal-disposition-review.md`; it checks stage
delta identity, seed accounting, IV independence, prior inheritance, the O2
endpoint omission and the prohibition on post-hoc thresholds.  The review is
explicitly not an independent Sol review.
