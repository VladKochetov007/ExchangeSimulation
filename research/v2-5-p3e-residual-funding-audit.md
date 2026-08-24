# V2-5 P3e — residual-funding replay contract

Status: **pre-run analyzer-contract correction.** This document is written
before a completed P3e P0 cell is inspected or scored. It changes neither the
simulator nor the P0 economic policy.

## Defect found by static contract review

`TermCarryAudit` originally classified every `funding_settlement` after a
finite term's `term_end` as `OutsideTermFunding`. That is correct for the
legacy P3 finite policy, but it contradicts the declared P3e lifecycle:

```text
post-only exit deadline
  -> cancel/stop replacing the passive child
  -> retain any actual residual position
  -> retain its ordinary funding and price risk until an actual close
```

The existing P3e design explicitly says that deadline expiry does not fabricate
a close or stop funding. Therefore a real, auditable residual must not be
mislabelled as unowned funding merely because the planned carry interval ended.

## Replay classification

The independent analyzer must use only persisted term decisions, actor
outcomes, canonical balance changes, and the manifest policy. For each
funding settlement it classifies exactly one of:

| class | necessary evidence |
| --- | --- |
| `active_term_funding_settlements` | one active owned term, within `[active_at, term_end]`, before any close |
| `residual_exit_funding_settlements` | one P4 owned/active term; settlement strictly after `term_end` and before the passive deadline; a prior persisted post-term decision attests the same open term and a nonzero perpetual residual |
| `expired_residual_funding_settlements` | the same proof, at or after the declared deadline; it is a risk-state observation, not a successful close |
| `outside_term_funding_settlements` | every other settlement: legacy policy, no owned/active term, no prior post-term residual attestation, zero perpetual residual, a closed term, or ambiguous ownership |

The replay uses the latest **strictly earlier** local term state, never
cross-file same-timestamp ordering. A fill/cancel/close at the same timestamp
as funding is therefore not used to manufacture an attribution.

`residual_*` counters are diagnostic risk observations, not lifecycle success
or funding-anchor evidence. `outside_*` remains a mechanical failure. An
unfilled/expired residual must remain visible in positions and conservation;
neither counter can mask a synthetic flat position, duplicated close, missing
actor outcome, overlap, or funding after an unproven residual.

## Required adversarial tests

- valid P4 passive residual before deadline is counted as residual funding;
- valid P4 residual at/after deadline is counted separately and remains open;
- a legacy residual, a closed/flat P4 term, or a settlement without a prior
  post-term local attestation remains `outside` and invalid;
- changing a valid residual's perpetual position to zero is caught;
- same-timestamp funding never borrows a future decision/outcome for proof.

This corrects an erroneous sentence in the previous restart handoff that said
post-deadline funding must be rejected. It must instead remain an explicitly
classified economic risk until a real close. P3c is unchanged: it is legacy
P3 and its post-term funding remains `outside`/invalid for that finite-term
lifecycle claim.
