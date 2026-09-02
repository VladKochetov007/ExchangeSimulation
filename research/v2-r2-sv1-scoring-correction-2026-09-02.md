# V2-R2-SV1 scoring correction — measurement validity boundary

Date: 2026-09-02
Candidate: separately named CDF-liquidity successor `V2-R2-SV1`
Predecessor: archived negative `R2`
Status: pre-campaign contract amendment; no SV1 24-hour cell has been accepted

## Finding

An independent Sol-xhigh specification critic identified a fail-closed defect
in the pre-campaign scorer. A failed or malformed survival derivation was
recorded as a false treatment outcome, allowing it to become
`NON-VIABLE_AT_24H_MARKET_SURVIVAL_GATE`. The same failure on a control could be
ignored while the treatment was reported viable. The scorer also evaluated the
strict terminal predicate only for treatment, so a malformed control terminal
record was not required to fail the evidence contract.

This was a scorer-contract defect, not a simulator trajectory finding. The
critic's report was obtained before the formal exact-tree promotion review and
is not itself an acceptance decision.

## Intended invariant

Every registered primary treatment and control cell must produce a valid
terminal measurement and a valid survival measurement. A valid measurement may
produce a false economic predicate. In particular, a no-CDF control may
validly fail the strict terminal or survival endpoint as its counterfactual
diagnostic; that failure must not gate treatment viability. A missing,
malformed, or structurally invalid measurement is instead
`INVALID_DEVELOPMENT_EVIDENCE` for the whole development score.

The distinction is therefore:

| state | treatment | control |
|---|---|---|
| valid measurement, endpoint true | qualifies that endpoint | diagnostic pass |
| valid measurement, endpoint false | valid economic negative | diagnostic counterfactual failure |
| invalid/missing measurement | invalid evidence | invalid evidence |

No treatment-minus-control effect estimate is introduced by this correction;
that estimand remains unregistered.

## Mechanical correction

The pre-campaign scorer is now
`v2-r2-sv1-24h-development-scorer-v3`.

* `terminal_measurement_valid` checks nonempty terminal records, lifecycle phase,
  exact horizon timestamp, positive numeric CDF/USD marks, and a typed mark
  source. `terminal_mark_valid` then separately checks the strict two-sided
  source predicate.
* Survival derivation success is tracked independently from the truth of its
  predicates. A valid summary whose predicates are false is an endpoint result;
  a missing analyzer result or malformed summary is a measurement failure.
* Treatment and control terminal/survival measurement flags are aggregated into
  `measurement_contract`. That aggregate is required for both viable and
  evidence-valid classifications.
* Control endpoint flags are retained as diagnostics and do not determine
  treatment viability.
* Status classification is isolated in
  `scripts/v2-r2-sv1-score-classification.jq` and exercised by
  `scripts/test-v2-r2-sv1-score-contract.sh` for viable, valid-negative, invalid
  control-measurement, invalid treatment-measurement, mechanical-invalid, and
  audit-invalid cases.

The correction does not change the simulator, calendar, CDF supplier roster,
information boundary, evidence format, development seeds, predecessor result,
or holdout policy. It runs before any matched SV1 treatment/control campaign.

## Verification and disposition

At this checkpoint:

* shell syntax, score-classification matrix, and `git diff --check` pass;
* the existing treatment-only raw trajectory remains an analyzer-only
  diagnostic from source `3f73f3043c2cb04f01c9acfa86eed132cfadff11`;
* no paired current-revision SV1 result exists;
* no holdout seed was consumed or read;
* the formal fresh exact-tree Sol-xhigh promotion review remains outstanding.

This amendment must be included in that review and in the provenance-pinned
successor build. Historical JSON and predecessor verdicts remain immutable.
