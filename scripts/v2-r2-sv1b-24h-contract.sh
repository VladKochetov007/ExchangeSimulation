#!/usr/bin/env bash
# SV1B namespace contract. It reuses the accepted SV1 evidence primitives
# while giving the fresh CDF successor its own identities and storage roots.
set -euo pipefail

source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"

v2_r2_output_root="/home/vlad/v2-r2-sv1b-24h-development-20260902-v1"
v2_r2_attestation_root="/home/vlad/v2-r2-sv1b-24h-development-20260902-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-r2-sv1b-24h-development.lock"
v2_r2_sv1_candidate_id="V2-R2-SV1B-24H-CDF-LIQUIDITY"
v2_r2_sv1_require_candidate_metadata=true
v2_r2_sv1_require_generator_metadata=true
v2_r2_sv1_candidate_contract_version="v2-r2-sv1b-24h-candidate-v3"
v2_r2_sv1_scorer_contract="v2-r2-sv1b-24h-development-scorer-v3"
v2_r2_sv1_survival_contract="v2-r2-sv1b-24h-survival-side-availability-v2"
v2_r2_sv1_paired_effect_contract="v2-r2-sv1b-24h-paired-survival-effect-v1"
v2_r2_sv1_parity_contract="v2-r2-sv1b-24h-parity-v1"
v2_r2_sv1_predecessor_id="V2-R2-SV1"
v2_r2_sv1_runner_contract="v2-r2-sv1b-24h-runner-v3"
v2_r2_sv1_require_terminal_outcome=true
v2_r2_sv1_completion_sentinels='["greeks.json", "latency.json", "terminal-outcome.json"]'
v2_r2_sv1_require_no_replacement_withdrawal=true
v2_r2_sv1_experiment_prefix="v2-r2-sv1b-24h"
v2_r2_sv1_config_provenance_contract="v2-r2-sv1b-24h-config-provenance-v3"
v2_r2_sv1_config_dir="$root_dir/research/configs/v2-r2-sv1b-24h"
v2_r2_sv1_config_provenance_manifest="$root_dir/research/v2-r2-sv1b-24h-config-provenance.json"
v2_r2_sv1_seeds=(643 647 653)
v2_r2_sv1_parity_seed=643
v2_r2_sv1_source_config_names=(dev-607.json dev-607-none.json)
v2_r2_sv1_activation_config="$root_dir/research/configs/v2-r2-sv1b/activation-643.json"
v2_r2_sv1_activation_control_config="$root_dir/research/configs/v2-r2-sv1b/activation-643-control.json"
v2_r2_sv1_activation_seed=643
v2_r2_sv1_run_hypothesis_id="V2-R2-SV1B-24H-CDF-LIQUIDITY"
v2_r2_sv1_activation_hypothesis_prefix="V2-R2-SV1B-CDF-LIQUIDITY"
v2_r2_sv1_activation_contract="v2-r2-sv1b-activation-provenance-v2"
v2_r2_sv1_activation_pair_contract="v2-r2-sv1b-activation-pair-v2"
v2_r2_sv1_activation_output_prefix="v2-r2-sv1b-activation"
v2_r2_sv1_capacity_attestation="/home/vlad/v2-r2-sv1b-24h-binary-capacity-20260902-v2-treatment-643-g4.json"
v2_r2_sv1_capacity_probe_prefix="v2-r2-sv1b-24h-capacity"
v2_r2_sv1_capacity_attestation_contract="v2-r2-sv1b-24h-binary-capacity-v2"
v2_r2_sv1_capacity_probe_contract="v2-r2-sv1b-24h-capacity-probe-v2"
v2_r2_sv1_capacity_measurement_config="$root_dir/research/configs/v2-r2-sv1b-24h/treatment-643.json"
v2_r2_sv1_capacity_measurement_seed=643
v2_r2_sv1_capacity_launch_config="$root_dir/research/configs/v2-r2-sv1b-24h/treatment-643.json"
v2_r2_capacity_probe_cell="capacity-treatment-643-g4"

v2_r2_sv1_capacity_registered_config_name() {
	[[ $# -eq 1 ]] || return 1
	local config_path=$1 config_name
	if [[ "$config_path" == /* ]]; then
		config_name=$(basename -- "$config_path")
	else
		config_name=$(basename -- "$config_path")
	fi
	case "$config_name" in
		control-643-none.json|control-643.json|control-647.json|control-653.json|treatment-643.json|treatment-647.json|treatment-653.json)
			printf '%s\n' "$config_name"
			;;
		*) return 1 ;;
	esac
}

v2_r2_capacity_attestation_path_for_config() {
	[[ $# -eq 2 ]] || return 1
	local config_path=$1 gomaxprocs=$2 config_name
	config_name=$(v2_r2_sv1_capacity_registered_config_name "$config_path") || return 1
	[[ "$gomaxprocs" =~ ^[0-9]+$ ]] || return 1
	local resolved_config_path="$config_path"
	[[ "$resolved_config_path" == /* ]] || resolved_config_path="$root_dir/$resolved_config_path"
	[[ "$(realpath -m -- "$resolved_config_path")" == "$v2_r2_sv1_config_dir/$config_name" ]] || return 1
	case "$gomaxprocs:$config_name" in
		4:control-643-none.json|4:control-643.json|4:control-647.json|4:control-653.json|4:treatment-643.json|4:treatment-647.json|4:treatment-653.json|8:treatment-643.json)
			printf '/home/vlad/v2-r2-sv1b-24h-binary-capacity-20260902-v2-%s-g%s.json\n' "${config_name%.json}" "$gomaxprocs"
			;;
		*) return 1 ;;
	esac
}

v2_r2_capacity_probe_cell_for_config() {
	[[ $# -eq 2 ]] || return 1
	local config_path=$1 gomaxprocs=$2 config_name
	config_name=$(v2_r2_sv1_capacity_registered_config_name "$config_path") || return 1
	v2_r2_capacity_attestation_path_for_config "$config_path" "$gomaxprocs" >/dev/null || return 1
	printf 'capacity-%s-g%s\n' "${config_name%.json}" "$gomaxprocs"
}

# Keep SV1B's exact Go toolchain requirement and all fail-closed activity
# predicates from the accepted SV1 contract.
