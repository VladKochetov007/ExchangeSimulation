# V2-7 P7d seed-439 scorer independent review

Reviewer: independent Sol xhigh red-team pass.  Scope: the immutable P7d
preregistration, numeric addendum, development promotion artifact, holdout
config policy, scorer implementation, seed-439 score, and the C/L/S-439
evidence metadata and metric contracts.

Verdict: **ACCEPT WITH NARROWER CLAIM**.

## What the scorer may say

The seed-439 machine artifact records a valid, orientation-separated factual
result under the existing activation and participant-specific maintenance-risk
predicate:

| orientation | activation | reconstructed maintenance breaches | participant forced closes | permitted status |
|---|---|---:|---:|---|
| C | disabled control valid | 0 | 0 | matched control |
| L | valid and target reached | 12 | 11 | risk exercised |
| S | valid and target reached | 0 | 0 | not exercised |

Thus the combined L/S description is `MIXED`: the long orientation cannot
compensate for the short orientation.  This is not an out-of-sample replication
claim, an aggregate holdout verdict, or a result about the full ecology.

## Review findings

1. The development scorer already fixed the primary activation threshold,
   `expected_breaches > 0` risk predicate, complete-evidence gates, and
   two-orientation classifier.  The new scorer transcribes those primary
   predicates; no result-fitted threshold, sign selection, or compensating
   orientation rule was found.
2. The exact holdout scorer commit `be717d3` was made after seed-439 metrics
   existed and after the current-state audit recorded their values.  It cannot
   honestly be described as a scorer frozen before seed-439 outcomes, even
   though its primary rule matches the prior development implementation.
3. The preregistration requires separate sign reporting and permits opposite
   signs to be `MIXED`.  The scorer requires both signs for `SUPPORTED
   (screening)`; one exercised sign yields `MIXED`.
4. Participant-specific breach evidence is role/account scoped in
   `perpexposurerisk.json`; generic venue liquidations are not used for the
   primary L/S risk predicate.
5. The ancillary `deficit_insurance_bankruptcy_exercised` field is not a
   supported participant-specific endpoint: it reads ecology-wide
   `liquidations.json`, combines distinct endpoints, and does not reconstruct
   bankruptcy.  It must not be cited as a P7d participant result.
6. Missing files, extraction-status failure, digest mismatch, and the checked
   analyzer integrity counters fail closed.  The scorer does not independently
   verify launcher success, exact required-artifact membership (only its
   length), or bankruptcy evidence.  Its fail-closed characterization is
   therefore limited to the primary checked contract.
7. The protocol reserves seeds 439, 443, and 449 but does **not** define an
   all-seed, majority, or other aggregate replication predicate.  A later
   aggregate decision rule would be post-439 and cannot be presented as
   preregistered.

## Required interpretation and next step

Keep the seed-439 world and its exact source/binary/config/evidence provenance.
Do not rerun it.  Treat its score as an orientation-separated factual replay
only.  The already reserved 443 and 449 cells may be run without changing the
economic protocol, then reported per seed under the same narrow predicate.
No three-seed aggregate `SUPPORTED`/`FALSIFIED` claim is licensed by the
existing preregistration.  Deficit, insurance, bankruptcy, and ecology-wide
liquidation claims require their own participant-scoped reconstruction.
