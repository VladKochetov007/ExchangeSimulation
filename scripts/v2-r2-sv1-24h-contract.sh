#!/usr/bin/env bash
# SV1-specific namespace wrapper around the accepted R2 evidence primitives.
# The economic and calendar checks remain shared; only the output namespace and
# registered cell identities differ from historical R2.
set -euo pipefail

source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"

v2_r2_output_root="/home/vlad/v2-r2-sv1-24h-development-20260901-v1"
v2_r2_attestation_root="/home/vlad/v2-r2-sv1-24h-development-20260901-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-r2-sv1-24h-development.lock"

v2_r2_capacity_attestation_path() {
	printf '%s\n' '/home/vlad/v2-r2-sv1-24h-binary-capacity-20260901-v1.json'
}

# SV1 promotion is tied to the installed Go 1.27.0 toolchain, not to an
# unbounded future 1.27 patch. Historical R2 callers retain the shared prefix
# predicate because their archived identities predate this successor contract.
v2_r2_is_go_127() {
	[[ "$1" == "go1.27.0" ]]
}

v2_r2_require_attestation_path() {
	local cell=$1
	case "$cell" in
		treatment-607|treatment-613|treatment-617|control-607|control-613|control-617|treatment-607-g8|control-607-none) ;;
		*) return 1 ;;
	esac
	[[ ! -L "$v2_r2_attestation_root" ]] || return 1
	[[ ! -L "$v2_r2_attestation_root/$cell.json" ]] || return 1
	[[ "$(realpath -m -- "$v2_r2_attestation_root/$cell.json")" == "$v2_r2_attestation_root/$cell.json" ]]
}

# The SV1 supplier is funded by its registered endowment. Collateral borrowing
# is therefore an anti-cheating failure, not an activation requirement. This
# predicate is deliberately derived from the typed CDF audit rather than from
# a raw-event grep so activation and accounting use the same evidence path.
v2_r2_require_cdf_supplier_activation() {
	local audit_path=$1 expected_supplier_count=$2
	[[ -s "$audit_path" ]] || return 1
	[[ "$expected_supplier_count" =~ ^[1-9][0-9]*$ ]] || return 1
	jq -e --argjson expected_supplier_count "$expected_supplier_count" '
		type == "object" and (.result | type) == "object" and
		.result.valid == true and
		(.result.supplier_count | type) == "number" and
		.result.supplier_count == $expected_supplier_count and
		.result.decision_count > 0 and .result.fill_count > 0 and
		.result.trading_supplier_count == .result.supplier_count and
		.result.pnl_changing_supplier_count == .result.supplier_count and
		.result.inventory_responsive_decision_count > 0 and
		(.result.cancel_count + .result.withdraw_count) > 0 and
		.result.max_borrowed == 0 and
		.result.supplier_volume_share <= 0.75 and
		.result.supplier_depth_over_75_share <= 0.5 and
		(.result.venues | type) == "array" and (.result.venues | length) == 3 and
		all(.result.venues[]; .supplier_depth_over_75_fraction <= 0.5) and
		(.result.suppliers | type) == "array" and (.result.suppliers | length) == $expected_supplier_count and
		all(.result.suppliers[];
			.valid == true and .fill_count > 0 and .pnl != 0 and
			.min_position != .max_position and .inventory_responsive_decision_count > 0 and
			.max_observation_age_ns > 0 and .max_borrowed == 0 and .borrow_event_count == 0 and
			.max_position <= .configured_max_position and
			.min_position >= (-.configured_max_position) and
			.max_quote_qty <= .configured_max_quote_qty)' "$audit_path" >/dev/null
}

# A paired control intentionally has no CDF supplier roster. Its typed audit
# must still be valid and prove that no supplier activity was smuggled into the
# control through role discovery or stale event routing. This is a population
# contract, not an activation predicate: a valid control reports
# cdf_liquidity_activation_observed=false in activation.json.
v2_r2_require_cdf_supplier_control() {
	local audit_path=$1
	[[ -s "$audit_path" ]] || return 1
	jq -e '
		type == "object" and (.result | type) == "object" and
		.result.valid == true and .result.supplier_count == 0 and
		.result.decision_count == 0 and .result.fill_count == 0 and
		.result.trading_supplier_count == 0 and
		.result.pnl_changing_supplier_count == 0 and
		.result.inventory_responsive_decision_count == 0 and
		.result.cancel_count == 0 and .result.withdraw_count == 0 and
		.result.max_borrowed == 0 and
		(.result.checks | type) == "array" and (.result.checks | length) == 0 and
		(.result.venues | type) == "array" and (.result.venues | length) == 3 and
		(.result.suppliers | type) == "array" and (.result.suppliers | length) == 0
	' "$audit_path" >/dev/null
}
