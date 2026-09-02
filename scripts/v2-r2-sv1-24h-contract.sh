#!/usr/bin/env bash
# SV1-specific namespace wrapper around the accepted R2 evidence primitives.
# The economic and calendar checks remain shared; only the output namespace and
# registered cell identities differ from historical R2.
set -euo pipefail

source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"

v2_r2_output_root="/home/vlad/v2-r2-sv1-24h-development-20260901-v1"
v2_r2_attestation_root="/home/vlad/v2-r2-sv1-24h-development-20260901-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-r2-sv1-24h-development.lock"
v2_r2_sv1_candidate_id="V2-R2-SV1"
v2_r2_sv1_require_candidate_metadata=false
v2_r2_sv1_require_generator_metadata=false
v2_r2_sv1_candidate_contract_version="v2-r2-sv1-24h-candidate-v3"
v2_r2_sv1_scorer_contract="v2-r2-sv1-24h-development-scorer-v5"
v2_r2_sv1_survival_contract="v2-r2-sv1-24h-survival-side-availability-v2"
v2_r2_sv1_paired_effect_contract="v2-r2-sv1-24h-paired-survival-effect-v1"
v2_r2_sv1_parity_contract="v2-r2-sv1-24h-parity-v1"
v2_r2_sv1_predecessor_id="R2"
v2_r2_sv1_runner_contract="v2-r2-sv1-24h-runner-v1"
v2_r2_sv1_require_terminal_outcome=false
v2_r2_sv1_completion_sentinels='["greeks.json", "latency.json"]'
v2_r2_sv1_require_no_replacement_withdrawal=false
v2_r2_sv1_experiment_prefix="v2-r2-sv1-24h"
v2_r2_sv1_config_provenance_contract="v2-r2-sv1-24h-config-provenance-v1"
v2_r2_sv1_config_dir="$root_dir/research/configs/v2-r2-sv1-24h"
v2_r2_sv1_config_provenance_manifest="$root_dir/research/v2-r2-sv1-24h-config-provenance.json"
v2_r2_sv1_seeds=(607 613 617)
v2_r2_sv1_parity_seed=607
v2_r2_sv1_source_config_names=(dev-607-none.json dev-607.json dev-613.json dev-617.json)
v2_r2_sv1_activation_config="$root_dir/research/configs/v2-r2-sv1/activation-607.json"
v2_r2_sv1_activation_control_config="$root_dir/research/configs/v2-r2-sv1/activation-607-control.json"
v2_r2_sv1_activation_seed=607
v2_r2_sv1_run_hypothesis_id="V2-R2-SV1-CDF-LIQUIDITY-24H"
v2_r2_sv1_activation_hypothesis_prefix="V2-R2-SV1-CDF-LIQUIDITY"
v2_r2_sv1_activation_contract="v2-r2-sv1-activation-provenance-v1"
v2_r2_sv1_activation_pair_contract="v2-r2-sv1-activation-pair-v1"
v2_r2_sv1_activation_output_prefix="v2-r2-sv1-activation"
v2_r2_sv1_capacity_attestation="/home/vlad/v2-r2-sv1-24h-binary-capacity-20260901-v1.json"
v2_r2_sv1_capacity_probe_prefix="v2-r2-sv1-24h-capacity"
v2_r2_sv1_capacity_probe_contract="v2-r2-sv1-24h-capacity-probe-v1"
v2_r2_capacity_probe_cell="treatment-607"

v2_r2_capacity_attestation_path() {
	printf '%s\n' "$v2_r2_sv1_capacity_attestation"
}

# SV1 promotion is tied to the installed Go 1.27.0 toolchain, not to an
# unbounded future 1.27 patch. Historical R2 callers retain the shared prefix
# predicate because their archived identities predate this successor contract.
v2_r2_is_go_127() {
	[[ "$1" == "go1.27.0" ]]
}

v2_r2_sv1_is_registered_seed() {
	local candidate=$1 registered_seed
	for registered_seed in "${v2_r2_sv1_seeds[@]}"; do
		[[ "$candidate" == "$registered_seed" ]] && return 0
	done
	return 1
}

v2_r2_require_attestation_path() {
	local cell=$1 cell_suffix seed
	case "$cell" in
		treatment-*|control-*) ;;
		*) return 1 ;;
	esac
	cell_suffix=${cell#*-}
	case "$cell_suffix" in
		*-g8)
			seed=${cell_suffix%-g8}
			[[ "$cell" == "treatment-${v2_r2_sv1_parity_seed}-g8" ]] || return 1
			;;
		*-none)
			seed=${cell_suffix%-none}
			[[ "$cell" == "control-${v2_r2_sv1_parity_seed}-none" ]] || return 1
			;;
		*) seed=$cell_suffix ;;
	esac
	v2_r2_sv1_is_registered_seed "$seed" || return 1
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
	if ! jq -e --argjson expected_supplier_count "$expected_supplier_count" --argjson require_no_replacement "$v2_r2_sv1_require_no_replacement_withdrawal" '
		type == "object" and (.result | type) == "object" and
		.result.valid == true and
		(.result.supplier_count | type) == "number" and
		.result.supplier_count == $expected_supplier_count and
		.result.decision_count > 0 and .result.fill_count > 0 and
		.result.trading_supplier_count == .result.supplier_count and
		.result.pnl_changing_supplier_count == .result.supplier_count and
		.result.inventory_responsive_decision_count > 0 and
		(.result.cancel_count + .result.withdraw_count) > 0 and
		((any(.result.suppliers[]; (.configured_max_loss_quote // 0) > 0) | not) or
			(.result.risk_state_decision_count | type) == "number" and
			.result.risk_state_decision_count > 0 and
			all(.result.suppliers[]; (.configured_max_loss_quote // 0) > 0 and .risk_state_decision_count > 0)) and
			.result.max_borrowed == 0 and
			(($require_no_replacement | not) or
				(.result.withdrawal_without_replacement_count | type) == "number" and
				.result.withdrawal_without_replacement_count > 0) and
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
			.max_quote_qty <= .configured_max_quote_qty)' "$audit_path" >/dev/null; then
		return 1
	fi
	if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
		jq -e '
			.result.supplier_removal_counterfactual_valid == true and
			.result.supplier_removal_snapshot_count == .result.snapshot_count and
			(.result.supplier_removal_bid_absence_fraction | type) == "number" and
			(.result.supplier_removal_ask_absence_fraction | type) == "number" and
			(.result.supplier_time_weighted_resting_depth_share | type) == "number" and
			.result.supplier_time_weighted_resting_depth_share <= 0.75 and
			all(.result.venues[];
				.supplier_removal_counterfactual_valid == true and
				.supplier_removal_snapshot_count == .snapshot_count and
				(.supplier_removal_bid_absence_fraction | type) == "number" and
				(.supplier_removal_ask_absence_fraction | type) == "number")
		' "$audit_path" >/dev/null
	fi
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

v2_r2_require_cdf_supplier_comparison() {
	local comparison_path=$1 expected_supplier_count=$2
	[[ -s "$comparison_path" ]] || return 1
	[[ "$expected_supplier_count" =~ ^[1-9][0-9]*$ ]] || return 1
	jq -e --argjson expected_supplier_count "$expected_supplier_count" --argjson require_no_replacement "$v2_r2_sv1_require_no_replacement_withdrawal" '
		type == "object" and .valid == true and
		(.provenance.valid // false) == true and
		(.treatment | type) == "object" and (.control | type) == "object" and
		.treatment.valid == true and .control.valid == true and
		.treatment.supplier_count == $expected_supplier_count and .control.supplier_count == 0 and
		.control.decision_count == 0 and .control.fill_count == 0 and
		.control.trading_supplier_count == 0 and .control.pnl_changing_supplier_count == 0 and
		.control.inventory_responsive_decision_count == 0 and .control.cancel_count == 0 and
		.control.withdraw_count == 0 and .control.max_borrowed == 0 and
		.treatment.trading_supplier_count == .treatment.supplier_count and
		.treatment.pnl_changing_supplier_count == .treatment.supplier_count and
		.treatment.inventory_responsive_decision_count > 0 and
		(.treatment.cancel_count + .treatment.withdraw_count) > 0 and
		((any(.treatment.suppliers[]; (.configured_max_loss_quote // 0) > 0) | not) or
			(.treatment.risk_state_decision_count | type) == "number" and
			.treatment.risk_state_decision_count > 0 and
			all(.treatment.suppliers[]; (.configured_max_loss_quote // 0) > 0 and .risk_state_decision_count > 0)) and
		.treatment.max_borrowed == 0 and
		(($require_no_replacement | not) or
			(.treatment.withdrawal_without_replacement_count | type) == "number" and
			.treatment.withdrawal_without_replacement_count > 0) and
		(.treatment.suppliers | length) == $expected_supplier_count and
		all(.treatment.suppliers[];
			.valid == true and .fill_count > 0 and .pnl != 0 and
			.inventory_responsive_decision_count > 0 and
			.max_position <= .configured_max_position and
			.min_position >= (-.configured_max_position) and
			.max_gross_base_balance <= .configured_max_inventory and
			.max_quote_qty <= .configured_max_quote_qty)' "$comparison_path" >/dev/null
}
