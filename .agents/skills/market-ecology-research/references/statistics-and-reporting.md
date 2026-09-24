# Statistics, records and comparable reports

## Prospective inference

Lock primary endpoint, contrasts, causal DAG, opportunity/risk-set denominator,
units, censoring and missingness before outcomes. State nulls and practical
effect/equivalence margins if used; otherwise do not claim equivalence.
Separate development, confirmation, empirical validation and exploratory discovery.
Same seed means matched randomness only if stream consumption/namespaces align.
Use independent worlds or justified blocks, not correlated event rows, as samples.
Three development seeds support a screen, not tail-risk or high-powered claims.

Report all assigned cells. If opportunity/valuation is affected by treatment,
shortfall or PnL among survivors is a conditional estimand. Declare when paired
contrasts are undefined and never replace them with a convenient complete-case
aggregate. Economic failure can still be valid evidence.
No-trade may be optimal; inspect delivered information, constraints and no-action
decisions as required by the declared objective.

For empirical claims choose accessible data with compatible units, sampling,
costs and regime. New seeds validate replication inside the model only.
A stylized fact can match under multiple hidden populations.
For impact, define intervention Q, horizon, control and normalization. Compare
alternative exponents; treatment-altered volume/volatility can bias normalization.

## Records, version 1

The validator and templates implement a small typed metadata contract, not an
experiment runner. Paths are repository-relative, except opaque external
evidence locations in result evidence entries; those are never opened by validation.
Registry fields: schema_version, baseline_commit, studies, policies.
A study has id/title/claim_type/stage/idea/protocol/report/result, dependencies,
policy_ids, authorization and next_action. Absent later-stage paths are null.
Authorization contains prepare and run booleans plus basis (owner instruction
or explicit absence); a boolean alone is not sufficient execution authority.
Policies have id/title/source_status/sources/catalogue_anchor.
Source status: IMPLEMENTED, PARTIAL, NOT_IMPLEMENTED or UNVERIFIED; cite scope.

Results contain schema_version, study_id, stage, process_status, evidence_validity,
opportunity_presence, policy_activity, registered_activation, scientific_verdict,
causal_verdict,
empirical_comparison, reasons, identities, authorization, review, claims and evidence.
An optional top-level `observed` object may retain study-specific numeric
summaries. Its values are not semantically certified by this metadata
validator; the bound analyzer and evidence review own those checks.
Use the supplied result template for exact field names and initial values.
A status record is never an authorization grant; consult the owner instruction.

| Dimension | Allowed values |
|---|---|
| stage | INTAKE, PLAN, PREPARE, DEVELOPMENT, CONFIRMATION, REPORT, CLOSED |
| process_status | NOT_RUN, COMPLETED, FAILED, INCOMPLETE |
| evidence_validity | NOT_ASSESSED, VALID, INVALID, INCOMPLETE |
| opportunity_presence | NOT_ASSESSED, PRESENT, ABSENT, UNKNOWN, NOT_APPLICABLE, SAMPLED_LOCAL_PROXY_PRESENT_AT_DECISION |
| policy_activity | NOT_ASSESSED, ACTIVE, INACTIVE, UNKNOWN, NOT_APPLICABLE |
| registered_activation | NOT_ASSESSED, SATISFIED, NOT_SATISFIED, UNKNOWN, NOT_APPLICABLE |
| scientific_verdict | NOT_ISSUED, SUPPORTED, NOT_SUPPORTED, INCONCLUSIVE, IDENTIFICATION_LIMITATION, MECHANICAL_ONLY |
| causal_verdict | NOT_ASSESSED, SUPPORTED, NOT_SUPPORTED, INCONCLUSIVE, NOT_IDENTIFIED, NOT_APPLICABLE, BOUNDED_RESPONSE_OBSERVED |
| empirical_comparison | NOT_PERFORMED, COMPATIBLE, MISMATCH, INCONCLUSIVE, NOT_APPLICABLE |
| review execution | COMPLETED, UNAVAILABLE, FAILED, NOT_REQUESTED |
| review verdict | ACCEPT, ACCEPT_WITH_REQUIRED_CHANGES, REJECT, NOT_ISSUED |

A result with missing/invalid evidence cannot issue an economic verdict.
An issued scientific verdict requires evidence-linked claims and complete identities.
Assess causal effects separately even when mechanical or descriptive evidence is valid.
MECHANICAL_ONLY limits the entire result to mechanics: causal_verdict must be
NOT_ASSESSED or NOT_APPLICABLE, not a substantive causal finding (including a null).
An accepted review requires COMPLETED execution and a substantive scope/reference.
Every claim has id, type, text, evidence_ids and limitations.
Every evidence entry has id, kind, location and identity.
Claims may use only recorded evidence IDs.
A no-run template has no claims and NOT_ISSUED verdict.
Use reasons for unknown/N/A values; null identities require explicit explanation.
Historic vocabulary is retained in linked original records, not silently translated
into a stronger current schema verdict.
`SAMPLED_LOCAL_PROXY_PRESENT_AT_DECISION` names a sampled delivered quote,
not at-arrival executable depth. `BOUNDED_RESPONSE_OBSERVED` identifies an
observed registered response in the stated development scope, not a general
causal effect or confirmation.

## Reporting and synthesis

Use the twelve-part report template and publish compact artifacts in Git.
Keep old attempts/reports immutable; append a named successor/amendment with why.
Pin source/config/run/analyzer/protocol/skill independently. Reports may be
committed after candidate C without pretending C changed.
Describe actual review scope and unperformed checks. A service failure is
UNAVAILABLE / NOT_ISSUED. Use one permitted fallback, then stop with an external
package if substantive review is still required. Do not shop around a rejection.

Synthesis begins with a comparability table: population, resources, policies,
deployment, venue, horizon, endpoint, source/config/analyzer and evidence validity.
Trace each statement to actual completed evidence including contrary results.
Do not pool incompatible studies as replications, average mismatched endpoints,
or call a planned mechanism an observed result. End with one bounded next question.
