# V2-6 P6 — staged options ecology causal preregistration

Status: **frozen before P6 implementation, preflight, or outcome inspection**.

P6 asks how much option-market structure is present before explicit smile
beliefs are introduced, and whether an economically motivated end user plus
dealer hedging creates measurable underlying transmission. It is a staged
mechanism screen, not a realism claim. A surface observed in O3/O4 is inherited
from those participant priors unless the corresponding removal/restoration
comparison says otherwise.

## Stages

The stages are cumulative. Every non-option input is identical across stages;
only the declared option population delta is allowed.

| stage | sole option delta from the previous stage | scientific purpose |
|---|---|---|
| O0 | one flat-vol dealer; no dealer hedge, value taker, liability user, or VV desk | establish that listing, quoting, settlement, and market-price IV inversion work without a smile prior |
| O1 | three realized-vol dealers with different half-lives/premiums; no hedge or explicit smile prior | test whether heterogeneous *strike-flat* beliefs alone create cross-strike dispersion |
| O2 | O1 plus heterogeneous dealer delta-hedge policies and one fixed downside-liability user; no SABR/VV prior | test end-user demand, inventory transfer, and hedge feedback before smile beliefs |
| O3 | O2 plus one SABR-view value taker | measure the incremental inherited SABR contribution |
| O4 | O3 plus one Vanna–Volga risk-transfer desk | measure the incremental effect of explicit second-order relative-value trading |

The random option flow participant remains present and identical in every
stage as an inherited liquidity-control population. It has no volatility model
and is not evidence of an emergent smile. The new O2 liability user is the
only stage-specific end-user population and has no option-pricing model.

## Primary observables and gates

Every stage must first pass evidence integrity: successful run completion,
receipt/frontier reconstruction, runtime/offline evidence digest agreement,
conservation, position/fill reconstruction, lifecycle settlement, and exact
determinism. A stage with an inactive intended participant is **NOT EXERCISED**
and cannot support a surface interpretation.

Independent market-price analysis infers Black–76 IV only from persisted
trades/quotes. Model volatility values from dealers, value takers, or VV desks
are never used as market IV. Black–76 observations with unavailable index,
non-positive forward/strike, non-invertible premium, or out-of-bound premium
are reported with explicit skip reasons rather than coerced values.

Required stage activity:

* O0/O1: at least two listed short-option generations per venue, two-sided
  dealer quotes, and non-empty market-price IV observations;
* O2: liability decisions, at least one canonical admitted/fill-linked put
  order per venue, active dealer option inventory, and non-zero hedge-tagged
  underlying flow where a dealer takes option risk;
* O3: value-taker decisions and at least one canonical fill-linked option
  order, while the chain remains active;
* O4: VV decisions and at least one canonical fill-linked option order, while
  the chain remains active.

The activity gates identify the mechanism only. They do not imply that a
participant was profitable or that a market-level effect is realistic.

## Registered comparisons

Development uses paired seeds 211 and 213. Untouched holdout seeds are 223,
227, and 229 and may be run only if development produces a complete,
identifiable stage screen. The horizon is 8 hours, with the inherited 2-hour
short-option tenor and 6-hour long tenor; the first and last 30 minutes are
retained for feed warm-up and terminal censoring, while surface samples use
only observations whose source and delivery frontiers are complete.

The registered descriptive contrasts are:

* O1 − O0: ATM IV, cross-strike slope, curvature, term structure, and
  cross-strike dispersion;
* O2 − O1: dealer option inventory, hedge-tagged underlying volume, dealer
  net delta, and the same independently inferred surface statistics;
* O3 − O2 and O4 − O3: incremental surface and exposure changes attributable
  to the newly activated explicit-prior/risk-transfer participant.

The primary causal question is activation plus direction of option-to-underlying
transmission in O2. A paired screen is **SUPPORTED (screening)** only when
both seeds pass all gates and show the registered sign for the predeclared
transmission statistic. Opposite signs are **MIXED**; complete activation with
no registered directional effect is **FALSIFIED** for that component; missing
activation is **NOT EXERCISED**. No stage can be called “fully emergent” while
an explicit SABR or VV prior remains in the population.

## Information and accounting boundary

The liability user receives only its declared delayed local instrument and book
feeds. It chooses a listed put nearest a fixed 95% strike target based on its
latest delivered underlying snapshot, buys only at an observed executable ask,
and stops at its finite target quantity/budget. It does not read a global
index, dealer state, model IV, or opponent inventory. Every decision records
its observation frontier; every order/fill is reconstructed from venue events.

All option prices remain in the signed-price representation, but the current
Black–76 model is explicitly positive-forward/positive-strike only. No signed
underlying option claim is made by P6.

## Invalidations and mutations

The screen is invalid if a stage changes an undeclared non-option field, if
instrumentation changes the execution hash, or if a decision uses a future or
unpublished observation. Analyzer mutations must catch: dropped liability
decisions, dropped fills, duplicated fills, future-injected option snapshots,
stale asks, model-IV substitution for market IV, and a zero-trigger stage
silently treated as a pass.

