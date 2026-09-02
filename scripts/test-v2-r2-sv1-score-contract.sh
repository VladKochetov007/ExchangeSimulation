#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
filter="$root_dir/scripts/v2-r2-sv1-score-classification.jq"

fail() {
	printf 'SV1 score contract test failure: %s\n' "$*" >&2
	exit 1
}

classify() {
	local all_cells_valid=$1 all_measurements_valid=$2
	local all_treatment_terminal_valid=$3 all_treatment_survival_valid=$4
	local all_cdf_contract_valid=$5 all_anticheating_valid=$6
	local all_paired_effect_valid=$7 all_paired_effect_identified=$8
	jq -n \
		--argjson all_cells_valid "$all_cells_valid" \
		--argjson all_measurements_valid "$all_measurements_valid" \
		--argjson all_treatment_terminal_valid "$all_treatment_terminal_valid" \
		--argjson all_treatment_survival_valid "$all_treatment_survival_valid" \
		--argjson all_cdf_contract_valid "$all_cdf_contract_valid" \
		--argjson all_anticheating_valid "$all_anticheating_valid" \
		--argjson all_paired_effect_valid "$all_paired_effect_valid" \
		--argjson all_paired_effect_identified "$all_paired_effect_identified" \
		-f "$filter"
}

expect_status() {
	local name=$1 expected=$2
	shift 2
	local result actual
	result=$(classify "$@")
	actual=$(jq -er '.status' <<<"$result")
	[[ "$actual" == "$expected" ]] || fail "$name status = $actual, want $expected"
	printf '✓ %s\n' "$name"
}

expect_status valid-control-negative VIABLE_DEVELOPMENT_CANDIDATE true true true true true true true true
expect_status valid-treatment-negative NON-VIABLE_AT_24H_MARKET_SURVIVAL_GATE true true false true true true true true
expect_status invalid-control-measurement INVALID_DEVELOPMENT_EVIDENCE true false true true true true true true
expect_status invalid-treatment-measurement INVALID_DEVELOPMENT_EVIDENCE true false false true true true true true
expect_status invalid-mechanical-evidence INVALID_DEVELOPMENT_EVIDENCE false true true true true true true true
expect_status invalid-cdf-audit INVALID_DEVELOPMENT_EVIDENCE true true true true false true true true
expect_status valid-evidence-no-paired-effect NON-VIABLE_AT_24H_MARKET_SURVIVAL_GATE true true true true true true true false
expect_status invalid-paired-effect-measurement INVALID_DEVELOPMENT_EVIDENCE true true true true true false false false
