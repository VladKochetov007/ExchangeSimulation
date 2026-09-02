# SV1B withdrawal measurement amendment

Date: 2026-09-02
Candidate: `V2-R2-SV1B-24H-CDF-LIQUIDITY`
Status: preregistered development-contract clarification; no cell or holdout authorization

## Scientific purpose

The SV1B activation contract requires evidence that a supplier can reduce or
withdraw liquidity. An ordinary cancel/requote does not establish that
behavior: it can leave the book continuously supplied by the same participant.
This amendment defines the measurable event used by the activation and scoring
contracts. It does not change the supplier, exchange, scheduler, matching,
calendar, or economic model.

## Qualified withdrawal

For a supplier decision with `action = withdraw`, the audit records a qualified
withdrawal only when all of the following are true in the reconstructed event
stream:

1. the decision names an accepted live order and its cancellation request;
2. the exact request later produces an actual `OrderCancelled` outcome for that
   order after the withdrawal decision; and
3. no later supplier `submit` decision for that participant occurs at or before
   `cancellation_time + configured_interval`.

The interval is the supplier's registered decision interval, represented in
nanoseconds in the configuration and decision evidence. A submit exactly at
the interval boundary is treated as a replacement, so the qualified event
must remain absent through that boundary. The follow-up clock starts at the
confirmed cancellation, rather than at the withdrawal request, so a delayed
exchange response cannot be mistaken for realized liquidity absence. This
conservative boundary rule is fixed before activation measurement.

A cancellation request that loses a fill race, a missing cancellation outcome,
an unknown order, or a malformed action is not a qualified withdrawal. Such an
event cannot satisfy the activation gate. Existing evidence-validity checks
remain responsible for rejecting malformed or unreconciled order lifecycles.

## Censoring

When the terminal measurement time is earlier than the full follow-up deadline
from confirmed cancellation, the withdrawal is counted as
`censored_withdrawal_count`, not as a qualified withdrawal. The terminal time
comes from the registered market-data receipt terminal marker when receipts
are enabled; otherwise the audit uses the last observed event boundary as its
existing diagnostic fallback. The SV1B run contract supplies receipts and
therefore uses the explicit terminal boundary.

## Required evidence and anti-cheating use

The aggregate and per-supplier audits retain both qualified and censored
counts. SV1B requires a positive aggregate qualified count in treatment. The
count is diagnostic evidence of a real, finite-horizon withdrawal episode; it
is not a survival target and does not require the supplier to withdraw on a
fixed schedule. The existing concentration, finite-capital, PnL, inventory,
delayed-observation, and no-borrowing predicates remain in force.

The focused regression covers replacement before the boundary, an actual
cancellation with no replacement, and terminal censoring. A fill-race or
uncancelled order must not be allowed to satisfy the metric.
