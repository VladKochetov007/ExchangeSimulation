# Integrated V2 long-run v3 development failure

Date: 2026-08-28  
Cell: development 607, full logging  
Protocol: `v2-integrated-longrun-candidate-v3`

## Boundary

This is a retained failed development attempt. It is not a score, freeze
candidate, or holdout result. The raw root
`/home/vlad/v2-integrated-longrun-candidate-20260828-v3/dev-607` remains
immutable. Holdouts 619, 631, and 641 were not read. The earlier r2 roots and
all historical results remain separate.

The clean simulator, analyzer, and prune-gate binaries were built from HEAD
`648d4083648732040bcb9da08a1cc63562f69581` in a detached clean worktree with
Go 1.27.0, `trimpath=true`, `CGO_ENABLED=0`, and clean embedded VCS state.
Their SHA-256 identities were, respectively:

* multivenue: `958f4d5bdd2386b4b56d46fa8d1df9e19ca3dd7d468b262379d268d57b3bcb74`
* mvanalyze: `8eda0585daca4ac53b7a95635c9df82c488af9a015fb883018ecc3a3d3dd1e86`
* prunegate: `5f69c8040acd90f0ea54bf43884b316fdf19abdf7b2c8eca2d467314135fd831`

The runner completed successfully and the extractor failed closed during the
conservation predicate. Activation, delta consistency, and event-chain
predicates passed. The failed conservation identity was:

| asset | internal net | exchange take | open linear value | residual |
|---|---:|---:|---:|---:|
| USD | -5,701,469,660,713 | 1,052,760,985,044 | 4,648,709,341,670 | -6,667,139 |

The three venue USD residuals were `-2,126,484`, `-2,054,423`, and
`-2,486,232`. The configured bound remains `abs(residual) <= 1000`; it was not
relaxed after this observation. Raw evidence is therefore preserved and no
derived candidate was archived or scored.

## Diagnosis

`delta_consistency` checked 21,482,980 movement records with zero mismatches;
720 balance chains were checked with zero breaks; there were zero decode
failures. The activation predicate observed 19,097 collateral-borrow events,
all graph/mark requirements passed, and zero price-unavailable rejections were
recorded.

An independent Go diagnostic reconstructed the derivative position stream
without modifying the run. The current realized PnL is exactly the integer
toward-zero result of the rounded integer entry-price formula. Reconstructing
the weighted cost basis before entry-price truncation attributed the residual
to lifecycle rounding:

* realized basis correction: `+5,236,701` fixed units;
* dated-expiry correction: `+349,457` fixed units;
* terminal open-position correction: `+1,098,169` fixed units;
* corrected identity residual: `+17,188` fixed units.

Nearest rounding of the already-rounded entry-price formula changes realized
PnL by only `-16,825` units, so changing toward-zero to nearest is not the
mechanics correction. The evidence isolates the defect to the repeated loss of
cost-basis precision in weighted-average entry prices and its reuse by trade,
expiry, marked-risk, and liquidation valuation. It does not support an event
omission, analyzer repair, threshold change, or reopening of any historical
line.

## Next gate

The next source amendment keeps the existing average-cost policy and public
`EntryPrice` compatibility. The default position store will maintain a signed,
bounded-width aggregate cost numerator, use deterministic proportional basis
allocation, and derive realized, marked, expiry, and liquidation values from
that same accounting authority. Cash rounding is applied to cumulative
lifecycle totals, with the sub-unit remainder retained across flat/flip/reopen
transitions. An optional all-or-nothing exact position-store interface leaves
legacy custom stores source-compatible; strict scored simulations require the
exact interface before instrument admission.

The amendment must be independently reviewed, fully tested, cleanly rebuilt,
and rerun on fresh development output before cells 613/617, parity controls,
freeze, or any holdout authorization is considered.
