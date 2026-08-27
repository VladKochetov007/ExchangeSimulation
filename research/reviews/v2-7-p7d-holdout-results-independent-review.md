# P7d holdout results — adversarial second-pass review

Review status: **fallback second pass; not an independent Sol-xhigh review**.

The configured Sol-xhigh reviewer agents were unavailable because the model
usage limit was exhausted.  I therefore performed a fresh red-team pass over
the preregistration, the numeric addendum, the development promotion score,
the scorer-review record, the three retained per-seed scores, all nine cell
metadata/metric contracts, and the aggregate machine artifact.  This document
does not represent an independent reviewer; that limitation is itself part of
the provenance.

## Verdict

**ACCEPT WITH NARROWER CLAIM.**

The three holdout evidence packages are complete enough for the registered
orientation-separated activation and participant-specific maintenance-risk
replay to be reported per seed.  The package does not support a three-seed
aggregate verdict, participant-specific deficit/insurance/bankruptcy claims,
or any broader distress, funding, basis, profitability, or realism claim.

## Checks performed

1. **Protocol and config immutability.**  The P7d preregistration and pinned
   holdout config checker identify only C/L/S enablement and the registered
   seed as varying.  The nine `run-metadata.json` files carry source revision
   `8b1d013a33129beeb61a578b6f59aefa59cce0c2`, the same simulator SHA, the
   four-hour horizon, `holdout=true`, `preflight=false`, and the declared
   `greeks.json`/`latency.json` completion sentinels.  The 443/449 launcher
   statuses and all extraction statuses are zero.  Seed 439 predates launcher
   status files; its final sentinels and completed fail-closed extraction are
   present, so its claim is retained with that explicit provenance limitation.

2. **Evidence completeness.**  Every cell has the sixteen registered P7d
   metric artifacts, complete analysis metadata, non-empty files, and raw
   evidence.  Runtime and offline evidence-artifact event counts/digests are
   equal for all nine cells.  Receipt replay has zero future decision use,
   invalid frontiers and receipt errors; order/fill/position and conservation
   checks pass.

3. **Activation rule.**  Controls have zero enabled decisions, submissions,
   fills and quantity.  All six active orientations have enabled decisions,
   admitted/fill-linked IOC orders, the declared 6,000,000,000 raw-ABC target,
   and zero terminal target gap.  A zero-fill control is not being counted as
   a treatment success, and activation is not being conflated with risk.

4. **Risk rule and sign separation.**  The scorer uses the role/account-scoped
   `perpexposurerisk.json` replay.  It requires observed checks to equal
   independently expected breaches and does not use generic ecology-wide
   liquidations for the primary endpoint.  Long and short signs are reported
   separately: seed 439 is MIXED; 443 and 449 have both signs exercised.
   There is no path by which one sign compensates for the other within a
   seed.

5. **Post-outcome scorer provenance.**  The per-seed scorer was introduced
   after seed-439 metrics existed.  The prior scorer review correctly narrows
   the claim: its primary predicates transcribe the development predicate,
   but it was not frozen before the 439 outcome.  The same scorer was applied
   unchanged to 443/449.  The scorer checks required-artifact count rather
   than exact set membership and does not check launcher status or
   participant-scoped bankruptcy; these are limitations, not evidence for a
   broader claim.  The retained metadata was independently checked here for
   exact registered membership and non-empty files.

6. **Aggregate rule.**  The P7d preregistration reserves 439, 443 and 449 but
   defines no all-seed, majority, or other aggregate replication criterion.
   The machine artifact now contains exactly one record for each seed and
   explicitly reports aggregate `NOT IDENTIFIED`.  Declaring “two of three”
   or “all three” after seeing the results would be post hoc.

7. **Endpoint boundaries.**  The ancillary `liquidations.json` values are
   ecology-wide and cannot establish participant-specific deficit, insurance,
   or bankruptcy.  Those endpoints remain NOT SCORED/NOT EXERCISED.  The
   package makes no claim about funding, basis, profitability, market
   stability, full-ecology liquidation realism or stylized facts.

8. **Seed contamination and invalid attempts.**  The valid 443/449 runs were
   launched only after the scorer review and use the pinned source, binary,
   configs and evidence semantics.  Earlier pre-review attempts are archived
   with status 143 and no final sentinels/raw evidence/score; they are not
   mixed into the valid package.  No raw valid holdout evidence was pruned.

## Required disposition

Keep the nine valid worlds and three per-seed scores.  Promote only the narrow
claim in `research/v2-7-p7d-holdout-results.md`.  Do not rerun, retune or
replace a seed, and do not retrofit an aggregate rule.  If participant
deficit/insurance/bankruptcy is scientifically required later, preregister a
new participant-scoped reconstruction rather than reusing the ecology-wide
ancillary field.

The package is sufficient to close P7d holdout execution as a per-seed
screening result, but the missing aggregate rule and lack of an independent
Sol-xhigh final review prevent a stronger replication claim.  The next
licensed scientific gate is the V2-6 causal option disposition; P4/P4b/P5 and
the mixed timing line remain explicit limitations.
