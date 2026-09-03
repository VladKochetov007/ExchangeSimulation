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
v2_r2_sv1_require_positive_loss_budget=true
v2_r2_sv1_require_no_replacement_withdrawal=true
v2_r2_sv1_experiment_prefix="v2-r2-sv1b-24h"
v2_r2_sv1_config_provenance_contract="v2-r2-sv1b-24h-config-provenance-v4"
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
v2_r2_sv1_activation_pair_contract="v2-r2-sv1b-activation-pair-v3"
v2_r2_sv1_activation_output_prefix="v2-r2-sv1b-activation"
v2_r2_sv1_review_contract="v2-r2-sv1b-independent-review-v2"
v2_r2_sv1_activation_review_contract="v2-r2-sv1b-activation-review-v1"
v2_r2_sv1_cpu_limit_percent=90
v2_r2_sv1_review_scope='["r2_calendar", "correctness_hardening", "binary_evidence", "cdf_supplier", "activation_protocol", "capacity_protocol", "parity_controls", "historical_boundary"]'
v2_r2_sv1_activation_review_scope='["activation_evidence", "cdf_activation", "binary_evidence", "resource_guards", "provenance_binding", "historical_boundary"]'
v2_r2_sv1_capacity_attestation="/home/vlad/v2-r2-sv1b-24h-binary-capacity-20260903-v4-capacity-seed-659-treatment-g4.json"
v2_r2_sv1_capacity_probe_prefix="v2-r2-sv1b-24h-capacity"
v2_r2_sv1_capacity_attestation_contract="v2-r2-sv1b-24h-binary-capacity-v4"
v2_r2_sv1_capacity_probe_contract="v2-r2-sv1b-24h-capacity-probe-v4"
v2_r2_sv1_capacity_measurement_config="$root_dir/research/configs/v2-r2-sv1b-24h/treatment-643.json"
v2_r2_sv1_capacity_measurement_seed=659
v2_r2_sv1_capacity_launch_config="$root_dir/research/configs/v2-r2-sv1b-24h/treatment-643.json"
v2_r2_sv1_capacity_memory_limit_bytes=$((20 * 1024 * 1024 * 1024))
v2_r2_sv1_activation_gomaxprocs=2
v2_r2_sv1_activation_memory_limit_bytes=$((20 * 1024 * 1024 * 1024))
v2_r2_sv1_activation_gomemlimit_bytes=$((18 * 1024 * 1024 * 1024))
v2_r2_sv1_activation_minimum_free_bytes=$((4 * 1024 * 1024 * 1024))
v2_r2_capacity_probe_cell="capacity-treatment-643-g4"
v2_r2_sv1_capacity_authorized_launch_config_names=(
	control-643-none.json control-643.json control-647.json control-653.json
	treatment-643.json treatment-647.json treatment-653.json
)

v2_r2_sv1_activation_provenance_path() {
	local head_revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '%s\n' "${V2_R2_SV1B_ACTIVATION_PROVENANCE:-/home/vlad/external-scratch/v2-r2-sv1b-activation-${v2_r2_sv1_activation_seed}-${head_revision}/activation-provenance.json}"
}

v2_r2_sv1b_review_attestation_path() {
	local head_revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '%s\n' "${V2_R2_SV1B_REVIEW_ATTESTATION:-/home/vlad/external-scratch/v2-r2-sv1b-review-${head_revision}/review-attestation.json}"
}

v2_r2_sv1b_activation_review_attestation_path() {
	local head_revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '%s\n' "${V2_R2_SV1B_ACTIVATION_REVIEW_ATTESTATION:-/home/vlad/external-scratch/v2-r2-sv1b-activation-review-${v2_r2_sv1_activation_seed}-${head_revision}/review-attestation.json}"
}

v2_r2_sv1b_cpu_policy() {
	local host_cpu_count allowed_cpu_count
	command -v nproc >/dev/null 2>&1 || return 1
	host_cpu_count=$(nproc --all) || return 1
	[[ "$host_cpu_count" =~ ^[1-9][0-9]*$ ]] || return 1
	allowed_cpu_count=$((host_cpu_count * v2_r2_sv1_cpu_limit_percent / 100))
	(( allowed_cpu_count > 0 )) || allowed_cpu_count=1
	printf '%s\t%s\t0-%s\n' "$host_cpu_count" "$allowed_cpu_count" "$((allowed_cpu_count - 1))"
}

v2_r2_sv1b_git_tree_sha256() {
	[[ $# -eq 1 ]] || return 1
	local revision=$1
	[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	git -C "$root_dir" ls-tree -r --full-tree "$revision" | sha256sum | awk '{print $1}'
}

v2_r2_require_sv1b_review_attestation() {
	[[ $# -eq 2 ]] || return 1
	local review_path=$1 expected_revision=$2 report_path report_sha256 actual_report_sha256 reviewed_tree_sha256 expected_tree_sha256
	[[ "$review_path" == /* && "$review_path" != */ && "$review_path" != *$'\n'* && "$review_path" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	[[ -f "$review_path" && ! -L "$review_path" ]] || return 1
	[[ "$(realpath -e -- "$review_path")" == "$review_path" ]] || return 1
	expected_tree_sha256=$(v2_r2_sv1b_git_tree_sha256 "$expected_revision") || return 1
	reviewed_tree_sha256=$(jq -er '.reviewed_tree_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$review_path") || return 1
	[[ "$reviewed_tree_sha256" == "$expected_tree_sha256" ]] || return 1
	report_path=$(jq -er '.review_report_path | select(type == "string")' "$review_path") || return 1
	report_sha256=$(jq -er '.review_report_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$review_path") || return 1
	[[ "$report_path" == /* && "$report_path" != */ && "$report_path" != *$'\n'* && "$report_path" != *$'\t'* ]] || return 1
	[[ -f "$report_path" && ! -L "$report_path" && "$(realpath -e -- "$report_path")" == "$report_path" ]] || return 1
	actual_report_sha256=$(sha256sum -- "$report_path" | awk '{print $1}') || return 1
	[[ "$actual_report_sha256" == "$report_sha256" ]] || return 1
	jq -e --arg contract "$v2_r2_sv1_review_contract" --arg revision "$expected_revision" --arg tree_sha256 "$expected_tree_sha256" \
		--arg report_sha256 "$report_sha256" --argjson required_scope "$v2_r2_sv1_review_scope" '
		type == "object" and .schema_version == 1 and .contract == $contract and
		.reviewed_revision == $revision and .reviewed_tree_sha256 == $tree_sha256 and
		.review_type == "independent_sol_xhigh" and .verdict == "ACCEPTED_FOR_ACTIVATION" and
		.reviewed_worktree_clean == true and .holdouts_consumed == false and
		(.reviewer | type == "string" and length > 0) and
		(.reviewed_scope | type == "array" and length > 0) and
		(($required_scope - .reviewed_scope) | length == 0) and
		.review_report_sha256 == $report_sha256' "$review_path" >/dev/null
}

v2_r2_require_sv1b_activation_review_attestation() {
	[[ $# -eq 3 ]] || return 1
	local review_path=$1 expected_revision=$2 activation_provenance_path=$3
	local report_path report_sha256 actual_report_sha256 expected_tree_sha256 reviewed_tree_sha256 activation_provenance_sha256
	[[ "$review_path" == /* && "$review_path" != */ && "$review_path" != *$'\n'* && "$review_path" != *$'\t'* ]] || return 1
	[[ "$activation_provenance_path" == /* && "$activation_provenance_path" != */ && "$activation_provenance_path" != *$'\n'* && "$activation_provenance_path" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	[[ -f "$review_path" && ! -L "$review_path" && "$(realpath -e -- "$review_path")" == "$review_path" ]] || return 1
	[[ -f "$activation_provenance_path" && ! -L "$activation_provenance_path" && "$(realpath -e -- "$activation_provenance_path")" == "$activation_provenance_path" ]] || return 1
	expected_tree_sha256=$(v2_r2_sv1b_git_tree_sha256 "$expected_revision") || return 1
	activation_provenance_sha256=$(sha256sum -- "$activation_provenance_path" | awk '{print $1}') || return 1
	jq -e '
		type == "object" and .schema_version == 3 and
		.status == "ACTIVATION_CONTRACT_SATISFIED" and .activation_satisfied == true and
		.holdouts_consumed == false and .treatment_runner_status == 0 and .control_runner_status == 0 and
		.treatment_terminal_status == "completed" and .control_terminal_status == "completed"' \
		"$activation_provenance_path" >/dev/null || return 1
	reviewed_tree_sha256=$(jq -er '.reviewed_tree_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$review_path") || return 1
	[[ "$reviewed_tree_sha256" == "$expected_tree_sha256" ]] || return 1
	report_path=$(jq -er '.review_report_path | select(type == "string")' "$review_path") || return 1
	report_sha256=$(jq -er '.review_report_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$review_path") || return 1
	[[ "$report_path" == /* && "$report_path" != */ && "$report_path" != *$'\n'* && "$report_path" != *$'\t'* ]] || return 1
	[[ "$review_path" != "$root_dir"/* && "$report_path" != "$root_dir"/* ]] || return 1
	[[ -f "$report_path" && ! -L "$report_path" && "$(realpath -e -- "$report_path")" == "$report_path" ]] || return 1
	actual_report_sha256=$(sha256sum -- "$report_path" | awk '{print $1}') || return 1
	[[ "$actual_report_sha256" == "$report_sha256" ]] || return 1
	jq -e --arg contract "$v2_r2_sv1_activation_review_contract" --arg revision "$expected_revision" \
		--arg tree_sha256 "$expected_tree_sha256" --arg activation_path "$activation_provenance_path" \
		--arg activation_sha256 "$activation_provenance_sha256" --arg report_sha256 "$report_sha256" \
		--argjson required_scope "$v2_r2_sv1_activation_review_scope" '
		type == "object" and .schema_version == 1 and .contract == $contract and
		.reviewed_revision == $revision and .reviewed_tree_sha256 == $tree_sha256 and
		.review_type == "independent_sol_xhigh" and .verdict == "ACCEPTED_FOR_CAPACITY" and
		.reviewed_worktree_clean == true and .holdouts_consumed == false and
		(.reviewer | type == "string" and length > 0) and
		(.reviewed_scope | type == "array" and length > 0) and
		(($required_scope - .reviewed_scope) | length == 0) and
		.activation_provenance_path == $activation_path and
		.activation_provenance_sha256 == $activation_sha256 and
		.review_report_sha256 == $report_sha256' "$review_path" >/dev/null
}

v2_r2_sv1b_artifact_records() {
	[[ $# -eq 1 ]] || return 1
	local artifact_root=$1 relative bytes digest records='[]'
	[[ -d "$artifact_root" && ! -L "$artifact_root" ]] || return 1
	if find "$artifact_root" -type l -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	while IFS= read -r -d '' relative; do
		[[ "$relative" != /* && "$relative" != *$'\n'* && "$relative" != *$'\t'* ]] || return 1
		bytes=$(stat -c '%s' -- "$artifact_root/$relative") || return 1
		digest=$(sha256sum -- "$artifact_root/$relative" | awk '{print $1}') || return 1
		records=$(jq -c --arg path "$relative" --arg digest "$digest" --argjson bytes "$bytes" \
			'. + [{path:$path,bytes:$bytes,sha256:$digest}]' <<<"$records") || return 1
	done < <(find "$artifact_root" -type f -printf '%P\0' | LC_ALL=C sort -z)
	printf '%s\n' "$records"
}

v2_r2_sv1b_verify_artifact_records() {
	[[ $# -eq 2 ]] || return 1
	local artifact_root=$1 records=$2 relative expected_bytes expected_digest actual_bytes actual_digest listed actual
	[[ -d "$artifact_root" && ! -L "$artifact_root" ]] || return 1
	if find "$artifact_root" -type l -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	[[ "$(jq -r '.[].path' <<<"$records" | LC_ALL=C sort)" == "$(find "$artifact_root" -type f -printf '%P\n' | LC_ALL=C sort)" ]] || return 1
	while IFS=$'\t' read -r relative expected_bytes expected_digest; do
		[[ "$relative" != /* && "$relative" != *$'\n'* && "$relative" != *$'\t'* ]] || return 1
		[[ "$expected_bytes" =~ ^[0-9]+$ && "$expected_digest" =~ ^[0-9a-f]{64}$ ]] || return 1
		[[ -f "$artifact_root/$relative" && ! -L "$artifact_root/$relative" ]] || return 1
		[[ "$(realpath -e -- "$artifact_root/$relative")" == "$artifact_root/$relative" ]] || return 1
		actual_bytes=$(stat -c '%s' -- "$artifact_root/$relative") || return 1
		actual_digest=$(sha256sum -- "$artifact_root/$relative" | awk '{print $1}') || return 1
		[[ "$actual_bytes" == "$expected_bytes" && "$actual_digest" == "$expected_digest" ]] || return 1
	done < <(jq -r '.[] | [.path, (.bytes | tostring), .sha256] | @tsv' <<<"$records")
}

v2_r2_require_sv1b_activation_provenance() {
	[[ $# -eq 3 ]] || return 1
	local provenance_path=$1 expected_revision=$2 expected_binary_sha256=$3
	local output_root treatment_dir control_dir comparison_path review_path analyzer_path simulator_path
	local expected_tree_sha256 actual_sha256 treatment_artifacts control_artifacts
	local expected_treatment_config expected_control_config treatment_config_path control_config_path
	local treatment_source_config_sha256 control_source_config_sha256
	local expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity
	[[ "$provenance_path" == /* && "$provenance_path" != */ && "$provenance_path" != *$'\n'* && "$provenance_path" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_binary_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -f "$provenance_path" && ! -L "$provenance_path" ]] || return 1
	[[ "$(realpath -e -- "$provenance_path")" == "$provenance_path" ]] || return 1
	expected_tree_sha256=$(v2_r2_sv1b_git_tree_sha256 "$expected_revision") || return 1
	IFS=$'\t' read -r expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity < <(v2_r2_sv1b_cpu_policy) || return 1
	output_root=$(jq -er '.output_root | select(type == "string")' "$provenance_path") || return 1
	treatment_dir=$(jq -er '.treatment_dir | select(type == "string")' "$provenance_path") || return 1
	control_dir=$(jq -er '.control_dir | select(type == "string")' "$provenance_path") || return 1
	comparison_path=$(jq -er '.comparison_path | select(type == "string")' "$provenance_path") || return 1
	review_path=$(jq -er '.review_attestation_path | select(type == "string")' "$provenance_path") || return 1
	analyzer_path=$(jq -er '.analyzer_binary_path | select(type == "string")' "$provenance_path") || return 1
	simulator_path=$(jq -er '.simulator_binary_path | select(type == "string")' "$provenance_path") || return 1
	for path in "$output_root" "$treatment_dir" "$control_dir" "$comparison_path" "$review_path" "$analyzer_path" "$simulator_path"; do
		[[ "$path" == /* && "$path" != */ && "$path" != *$'\n'* && "$path" != *$'\t'* ]] || return 1
	done
	[[ -d "$output_root" && ! -L "$output_root" && "$(realpath -e -- "$output_root")" == "$output_root" ]] || return 1
	[[ -d "$treatment_dir" && ! -L "$treatment_dir" && "$(realpath -e -- "$treatment_dir")" == "$treatment_dir" ]] || return 1
	[[ -d "$control_dir" && ! -L "$control_dir" && "$(realpath -e -- "$control_dir")" == "$control_dir" ]] || return 1
	[[ "$treatment_dir" == "$output_root/treatment" && "$control_dir" == "$output_root/control" ]] || return 1
	[[ "$treatment_dir" != "$control_dir" ]] || return 1
	[[ -f "$comparison_path" && ! -L "$comparison_path" && "$(realpath -e -- "$comparison_path")" == "$comparison_path" ]] || return 1
	[[ -x "$simulator_path" && ! -L "$simulator_path" && "$(realpath -e -- "$simulator_path")" == "$simulator_path" ]] || return 1
	[[ -x "$analyzer_path" && ! -L "$analyzer_path" && "$(realpath -e -- "$analyzer_path")" == "$analyzer_path" ]] || return 1
	[[ "$(sha256sum -- "$simulator_path" | awk '{print $1}')" == "$expected_binary_sha256" ]] || return 1
	actual_sha256=$(jq -er '.analyzer_binary_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	[[ "$(sha256sum -- "$analyzer_path" | awk '{print $1}')" == "$actual_sha256" ]] || return 1
	v2_r2_require_sv1b_review_attestation "$review_path" "$expected_revision" || return 1
	[[ "$review_path" != "$output_root"/* && "$review_path" != "$root_dir"/* ]] || return 1
	actual_sha256=$(sha256sum -- "$review_path" | awk '{print $1}') || return 1
	[[ "$actual_sha256" == "$(jq -er '.review_attestation_sha256' "$provenance_path")" ]] || return 1
	actual_sha256=$(sha256sum -- "$comparison_path" | awk '{print $1}') || return 1
	[[ "$actual_sha256" == "$(jq -er '.comparison_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path")" ]] || return 1
	jq -e 'type == "object" and .valid == true and .evidence_valid == true and .activation_satisfied == true and .anti_cheating_satisfied == true' "$comparison_path" >/dev/null || return 1
	jq -e --arg contract "$v2_r2_sv1_activation_pair_contract" --arg revision "$expected_revision" \
		--arg tree_sha256 "$expected_tree_sha256" --argjson seed "$v2_r2_sv1_activation_seed" \
		--argjson expected_host_cpu_count "$expected_host_cpu_count" --argjson expected_allowed_cpu_count "$expected_allowed_cpu_count" \
		--argjson expected_cpu_limit_percent "$v2_r2_sv1_cpu_limit_percent" --arg expected_cpu_affinity "$expected_cpu_affinity" \
		--argjson expected_activation_gomaxprocs "$v2_r2_sv1_activation_gomaxprocs" \
		--argjson expected_memory_limit_bytes "$v2_r2_sv1_activation_memory_limit_bytes" \
		--argjson expected_gomemlimit_bytes "$v2_r2_sv1_activation_gomemlimit_bytes" \
		--argjson expected_minimum_free_bytes "$v2_r2_sv1_activation_minimum_free_bytes" \
		--arg binary_sha256 "$expected_binary_sha256" '
		type == "object" and .schema_version == 3 and .contract == $contract and
		.candidate_revision == $revision and .candidate_tree_sha256 == $tree_sha256 and .seed == $seed and
		.status == "ACTIVATION_CONTRACT_SATISFIED" and .activation_satisfied == true and
		.holdouts_consumed == false and .treatment_runner_status == 0 and .control_runner_status == 0 and
		.treatment_terminal_status == "completed" and .control_terminal_status == "completed" and
		.simulator_binary_sha256 == $binary_sha256 and
		(.analyzer_binary_sha256 | type) == "string" and (.analyzer_binary_sha256 | test("^[0-9a-f]{64}$")) and
		(.review_attestation_sha256 | type) == "string" and (.review_attestation_sha256 | test("^[0-9a-f]{64}$")) and
		(.comparison_sha256 | type) == "string" and (.comparison_sha256 | test("^[0-9a-f]{64}$")) and
		(.treatment_config_sha256 | type) == "string" and (.treatment_config_sha256 | test("^[0-9a-f]{64}$")) and
		(.control_config_sha256 | type) == "string" and (.control_config_sha256 | test("^[0-9a-f]{64}$")) and
		(.treatment_run_status_sha256 | type) == "string" and (.treatment_run_status_sha256 | test("^[0-9a-f]{64}$")) and
		(.control_run_status_sha256 | type) == "string" and (.control_run_status_sha256 | test("^[0-9a-f]{64}$")) and
		(.treatment_artifacts | type) == "array" and (.treatment_artifacts | length) > 0 and
		(.control_artifacts | type) == "array" and (.control_artifacts | length) > 0 and
		(.resource_policy | type) == "object" and
		.resource_policy.gomaxprocs == $expected_activation_gomaxprocs and
		.resource_policy.memory_limit_bytes == $expected_memory_limit_bytes and
		.resource_policy.gomemlimit_bytes == $expected_gomemlimit_bytes and
		.resource_policy.minimum_free_bytes == $expected_minimum_free_bytes and
		.resource_policy.host_cpu_count == $expected_host_cpu_count and
		.resource_policy.allowed_cpu_count == $expected_allowed_cpu_count and
		.resource_policy.cpu_limit_percent == $expected_cpu_limit_percent and
		.resource_policy.cpu_affinity == $expected_cpu_affinity' "$provenance_path" >/dev/null || return 1
	expected_treatment_config=$(realpath -e -- "$v2_r2_sv1_activation_config") || return 1
	expected_control_config=$(realpath -e -- "$v2_r2_sv1_activation_control_config") || return 1
	treatment_config_path=$(jq -er '.treatment_source_config_path | select(type == "string")' "$provenance_path") || return 1
	control_config_path=$(jq -er '.control_source_config_path | select(type == "string")' "$provenance_path") || return 1
	[[ "$treatment_config_path" == "$expected_treatment_config" && "$control_config_path" == "$expected_control_config" ]] || return 1
	[[ -s "$treatment_config_path" && ! -L "$treatment_config_path" && -s "$control_config_path" && ! -L "$control_config_path" ]] || return 1
	treatment_source_config_sha256=$(sha256sum -- "$treatment_config_path" | awk '{print $1}') || return 1
	control_source_config_sha256=$(sha256sum -- "$control_config_path" | awk '{print $1}') || return 1
	jq -e --argjson seed "$v2_r2_sv1_activation_seed" \
		'.seed == $seed and .evidence_format == "evstream_v3" and .log_mode == "full"' "$treatment_config_path" >/dev/null || return 1
	jq -e --argjson seed "$v2_r2_sv1_activation_seed" \
		'.seed == $seed and .evidence_format == "evstream_v3" and .log_mode == "full"' "$control_config_path" >/dev/null || return 1
	jq -e --arg treatment_config_sha256 "$treatment_source_config_sha256" \
		--arg control_config_sha256 "$control_source_config_sha256" \
		'.treatment_source_config_sha256 == $treatment_config_sha256 and .control_source_config_sha256 == $control_config_sha256' \
		"$provenance_path" >/dev/null || return 1
	for arm in treatment control; do
		arm_dir=$([[ "$arm" == treatment ]] && printf '%s' "$treatment_dir" || printf '%s' "$control_dir")
		jq -e --arg arm "$arm" '
			type == "object" and .arm == $arm and .exit_status == 0 and
			.completion_verified == true and .terminal_failure_verified == false and
			.terminal_outcome_status == "completed" and (.resource_guard_failed // false) == false' \
			"$arm_dir/run-status.json" >/dev/null || return 1
	done
	treatment_artifacts=$(jq -c '.treatment_artifacts' "$provenance_path") || return 1
	control_artifacts=$(jq -c '.control_artifacts' "$provenance_path") || return 1
	v2_r2_sv1b_verify_artifact_records "$treatment_dir" "$treatment_artifacts" || return 1
	v2_r2_sv1b_verify_artifact_records "$control_dir" "$control_artifacts" || return 1
	[[ "$(sha256sum -- "$treatment_dir/run-config.json" | awk '{print $1}')" == "$treatment_source_config_sha256" ]] || return 1
	[[ "$(sha256sum -- "$control_dir/run-config.json" | awk '{print $1}')" == "$control_source_config_sha256" ]] || return 1
	[[ "$(sha256sum -- "$treatment_dir/run-config.json" | awk '{print $1}')" == "$(jq -er '.treatment_config_sha256' "$provenance_path")" ]] || return 1
	[[ "$(sha256sum -- "$control_dir/run-config.json" | awk '{print $1}')" == "$(jq -er '.control_config_sha256' "$provenance_path")" ]] || return 1
	[[ "$(sha256sum -- "$treatment_dir/run-status.json" | awk '{print $1}')" == "$(jq -er '.treatment_run_status_sha256' "$provenance_path")" ]] || return 1
	[[ "$(sha256sum -- "$control_dir/run-status.json" | awk '{print $1}')" == "$(jq -er '.control_run_status_sha256' "$provenance_path")" ]] || return 1
}

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
	[[ "$gomaxprocs" =~ ^[0-9]+$ ]] || return 1
	local resolved_config_path="$config_path"
	[[ "$resolved_config_path" == /* ]] || resolved_config_path="$root_dir/$resolved_config_path"
	resolved_config_path=$(realpath -m -- "$resolved_config_path") || return 1
	config_name=$(v2_r2_sv1_capacity_registered_config_name "$config_path") || return 1
	[[ "$resolved_config_path" == "$v2_r2_sv1_config_dir/$config_name" ]] || return 1
	case "$gomaxprocs:$config_name" in
		4:control-643-none.json|4:control-643.json|4:control-647.json|4:control-653.json)
			printf '/home/vlad/v2-r2-sv1b-24h-binary-capacity-20260903-v4-capacity-seed-659-control-g4.json\n'
			;;
		4:treatment-643.json|4:treatment-647.json|4:treatment-653.json)
			printf '/home/vlad/v2-r2-sv1b-24h-binary-capacity-20260903-v4-capacity-seed-659-treatment-g4.json\n'
			;;
		8:treatment-643.json)
			printf '/home/vlad/v2-r2-sv1b-24h-binary-capacity-20260903-v4-capacity-seed-659-treatment-g8.json\n'
			;;
		*) return 1 ;;
	esac
}

v2_r2_capacity_probe_cell_for_config() {
	[[ $# -eq 2 ]] || return 1
	local config_path=$1 gomaxprocs=$2 config_name
	v2_r2_capacity_attestation_path_for_config "$config_path" "$gomaxprocs" >/dev/null || return 1
	local resolved_config_path="$config_path"
	[[ "$resolved_config_path" == /* ]] || resolved_config_path="$root_dir/$resolved_config_path"
	resolved_config_path=$(realpath -m -- "$resolved_config_path") || return 1
	config_name=$(v2_r2_sv1_capacity_registered_config_name "$config_path") || return 1
	[[ "$resolved_config_path" == "$v2_r2_sv1_config_dir/$config_name" ]] || return 1
	case "$gomaxprocs:$config_name" in
		4:control-643-none.json|4:control-643.json|4:control-647.json|4:control-653.json)
			printf 'capacity-seed-659-control-g4\n'
			;;
		4:treatment-643.json|4:treatment-647.json|4:treatment-653.json)
			printf 'capacity-seed-659-treatment-g4\n'
			;;
		8:treatment-643.json)
			printf 'capacity-seed-659-treatment-g8\n'
			;;
		*) return 1 ;;
	esac
}

# Keep SV1B's exact Go toolchain requirement and all fail-closed activity
# predicates from the accepted SV1 contract.
