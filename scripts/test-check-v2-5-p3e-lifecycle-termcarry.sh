#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
checker="$root_dir/scripts/check-v2-5-p3e-lifecycle-termcarry.sh"
fixture=$(mktemp -d)
trap 'rm -rf -- "$fixture"' EXIT

write_fixture() {
	local arm=$1
	local outside=$2
	local residual=$3
	local valid=$4
	local checks='[]'
	if ((outside > 0)); then
		checks='[{"failure":"funding_settlement_outside_active_term"}]'
	fi
	jq -n --argjson outside "$outside" --argjson residual "$residual" --argjson valid "$valid" --argjson checks "$checks" '{
	  result: {
	    valid: $valid, receipt_audit_valid: true, receipt_evidence_errors: 0,
	    source_mismatches: 0, future_source_use: 0, invalid_decision_records: 0,
	    decision_field_mismatches: 0, arithmetic_mismatches: 0, missing_gateway_decisions: 0,
	    gateway_decision_mismatches: 0, missing_venue_outcomes: 0, duplicate_venue_outcomes: 0,
	    missing_actor_outcomes: 0, actor_outcome_mismatches: 0, lifecycle_violations: 0,
	    position_continuity_errors: 0, terminal_perp_mismatches: 0, terminal_spot_mismatches: 0,
	    first_exposure_mismatches: 0, missing_passive_exit_cancellations: 0,
	    passive_exit_cancellation_mismatches: 0, active_terms: 2, open_terms: 2,
	    closed_terms: 0, residual_exit_funding_settlements: $residual,
	    expired_residual_funding_settlements: 0, outside_term_funding_settlements: $outside,
	    checks: $checks
	  }
	}' >"$fixture/termcarry.json"
	jq -n --arg arm "$arm" --argjson residual "$((outside + residual))" '{
	  result: {arm: $arm, aggregates: {
	    activated_terms: 2, owned_terms: 2, residual_funding_settlements: $residual
	  }}
	}' >"$fixture/termcarrylifecycle.json"
}

write_fixture A 1 0 false
"$checker" "$fixture"

jq '.result.outside_term_funding_settlements = 2' "$fixture/termcarry.json" >"$fixture/termcarry-mutated.json"
mv "$fixture/termcarry-mutated.json" "$fixture/termcarry.json"
if "$checker" "$fixture" 2>/dev/null; then
	echo "accepted an unreconciled A residual" >&2
	exit 1
fi

write_fixture A 1 0 false
jq '.result.checks[0].failure = "forged_failure"' "$fixture/termcarry.json" >"$fixture/termcarry-mutated.json"
mv "$fixture/termcarry-mutated.json" "$fixture/termcarry.json"
if "$checker" "$fixture" 2>/dev/null; then
	echo "accepted an unrelated A integrity failure" >&2
	exit 1
fi

write_fixture B 0 1 true
"$checker" "$fixture"

jq '.result.outside_term_funding_settlements = 1 | .result.valid = false | .result.checks = [{"failure":"funding_settlement_outside_active_term"}]' "$fixture/termcarry.json" >"$fixture/termcarry-mutated.json"
mv "$fixture/termcarry-mutated.json" "$fixture/termcarry.json"
if "$checker" "$fixture" 2>/dev/null; then
	echo "accepted outside funding in B" >&2
	exit 1
fi
