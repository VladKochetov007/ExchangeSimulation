#!/usr/bin/env bash
# Reconcile the historical term-carry audit with the independent P3e lifecycle
# phases without changing the historical audit's semantics.
set -euo pipefail

cell=${1:?usage: check-v2-5-p3e-lifecycle-termcarry.sh CELL}

jq -e '
  .result.receipt_audit_valid == true and
  .result.receipt_evidence_errors == 0 and
  .result.source_mismatches == 0 and .result.future_source_use == 0 and
  .result.invalid_decision_records == 0 and .result.decision_field_mismatches == 0 and
  .result.arithmetic_mismatches == 0 and .result.missing_gateway_decisions == 0 and
  .result.gateway_decision_mismatches == 0 and .result.missing_venue_outcomes == 0 and
  .result.duplicate_venue_outcomes == 0 and .result.missing_actor_outcomes == 0 and
  .result.actor_outcome_mismatches == 0 and .result.lifecycle_violations == 0 and
  .result.position_continuity_errors == 0 and .result.terminal_perp_mismatches == 0 and
  .result.terminal_spot_mismatches == 0 and .result.first_exposure_mismatches == 0 and
  .result.missing_passive_exit_cancellations == 0 and
  .result.passive_exit_cancellation_mismatches == 0
' "$cell/termcarry.json" >/dev/null

jq -e --slurpfile lifecycle "$cell/termcarrylifecycle.json" '
  .result as $termcarry |
  $lifecycle[0].result as $lifecycle |
  ($termcarry.active_terms == $lifecycle.aggregates.activated_terms) and
  ($termcarry.open_terms + $termcarry.closed_terms == $lifecycle.aggregates.owned_terms) and
  (if $lifecycle.arm == "A" then
    $termcarry.residual_exit_funding_settlements == 0 and
    $termcarry.expired_residual_funding_settlements == 0 and
    $termcarry.outside_term_funding_settlements == $lifecycle.aggregates.residual_funding_settlements and
    ($termcarry.valid == ($termcarry.outside_term_funding_settlements == 0)) and
    ($termcarry.checks | length) == $termcarry.outside_term_funding_settlements and
    ($termcarry.checks | all(.failure == "funding_settlement_outside_active_term"))
  elif $lifecycle.arm == "B" then
    $termcarry.valid == true and
    $termcarry.outside_term_funding_settlements == 0 and
    ($termcarry.checks | length) == 0 and
    ($termcarry.residual_exit_funding_settlements + $termcarry.expired_residual_funding_settlements ==
      $lifecycle.aggregates.residual_funding_settlements)
  else
    false
  end)
' "$cell/termcarry.json" >/dev/null
