#!/usr/bin/env bash
# SV1C namespace contract. It reuses the accepted SV1 evidence primitives
# while giving the fresh CDF successor its own identities and storage roots.
set -euo pipefail

source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"

v2_r2_output_root="/home/vlad/v2-r2-sv1c-24h-development-20260908-v1"
v2_r2_attestation_root="/home/vlad/v2-r2-sv1c-24h-development-20260908-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-r2-sv1c-24h-development-20260908.lock"
v2_r2_sv1_candidate_id="V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK"
v2_r2_sv1_require_candidate_metadata=true
v2_r2_sv1_require_generator_metadata=true
v2_r2_sv1_candidate_contract_version="v2-r2-sv1c-24h-candidate-v1"
v2_r2_sv1_generator_path="scripts/render-v2-r2-sv1c-24h-configs.sh"
v2_r2_sv1_contract_path="scripts/v2-r2-sv1c-24h-contract.sh"
v2_r2_sv1_contract_loader_path="scripts/v2-r2-sv1-contract-loader.sh"
v2_r2_sv1_config_normalizer_path="bin/multivenue"
v2_r2_sv1_config_normalizer_package="exchange_sim/cmd/multivenue"
v2_r2_sv1_contract_dependency_paths=(
	scripts/v2-r2-sv1-24h-contract.sh
	scripts/v2-integrated-longrun-r2-contract.sh
)
v2_r2_sv1_withdrawal_measurement_path="research/v2-r2-sv1c-withdrawal-measurement-amendment-2026-09-08.md"
v2_r2_sv1_activation_diagnostics_path="research/v2-r2-sv1c-activation-diagnostics-amendment-2026-09-08.md"
v2_r2_sv1_preregistration_path="research/v2-r2-sv1c-strict-risk-amendment-2026-09-08.md"
v2_r2_sv1_capacity_order_path="research/v2-r2-sv1c-activation-capacity-order-amendment-2026-09-08.md"
v2_r2_sv1_scorer_contract="v2-r2-sv1c-24h-development-scorer-v1"
v2_r2_sv1_survival_contract="v2-r2-sv1c-24h-survival-side-availability-v1"
v2_r2_sv1_paired_effect_contract="v2-r2-sv1c-24h-paired-survival-effect-v1"
v2_r2_sv1_parity_contract="v2-r2-sv1c-24h-parity-v1"
v2_r2_sv1_predecessor_id="V2-R2-SV1B-24H-CDF-LIQUIDITY"
v2_r2_sv1_runner_contract="v2-r2-sv1c-24h-runner-v1"
v2_r2_sv1_require_terminal_outcome=true
v2_r2_sv1_completion_sentinels='["greeks.json", "latency.json", "terminal-outcome.json"]'
v2_r2_sv1_require_positive_loss_budget=true
v2_r2_sv1_require_no_replacement_withdrawal=true
v2_r2_sv1_experiment_prefix="v2-r2-sv1c-24h"
v2_r2_sv1_config_provenance_contract="v2-r2-sv1c-24h-config-provenance-v3"
v2_r2_sv1_config_dir="$root_dir/research/configs/v2-r2-sv1c-24h"
v2_r2_sv1_config_provenance_manifest="$root_dir/research/v2-r2-sv1c-24h-config-provenance.json"
v2_r2_sv1_seeds=(643 647 653)
v2_r2_sv1_parity_seed=643
v2_r2_sv1_source_config_names=(dev-607.json dev-607-none.json)
v2_r2_sv1_activation_config="$root_dir/research/configs/v2-r2-sv1c/activation-643.json"
v2_r2_sv1_activation_control_config="$root_dir/research/configs/v2-r2-sv1c/activation-643-control.json"
v2_r2_sv1_activation_seed=643
v2_r2_sv1_run_hypothesis_id="V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK"
v2_r2_sv1_activation_hypothesis_prefix="V2-R2-SV1C-STRICT-RISK-CDF-LIQUIDITY"
v2_r2_sv1_activation_contract="v2-r2-sv1c-activation-provenance-v1"
v2_r2_sv1_activation_pair_contract="v2-r2-sv1c-activation-pair-v1"
v2_r2_sv1_activation_arm_status_contract="v2-r2-sv1c-activation-arm-status-v1"
v2_r2_sv1_activation_horizon="5m"
v2_r2_sv1_activation_simulation_start_nano=1735689600000000000
v2_r2_sv1_activation_simulation_end_nano=1735689900000000000
v2_r2_sv1_activation_evidence_format="evstream_v3"
v2_r2_sv1_activation_log_mode="full"
v2_r2_sv1_activation_output_prefix="v2-r2-sv1c-activation"
v2_r2_sv1_review_contract="v2-r2-sv1c-independent-review-v1"
v2_r2_sv1_activation_review_contract="v2-r2-sv1c-activation-review-v1"
v2_r2_sv1_cpu_limit_percent=90
v2_r2_sv1_review_scope='["r2_calendar", "correctness_hardening", "binary_evidence", "cdf_supplier", "strict_risk_contract", "account_scoped_liquidation", "activation_protocol", "capacity_protocol", "parity_controls", "historical_boundary"]'
v2_r2_sv1_activation_review_scope='["activation_evidence", "cdf_activation", "strict_risk_contract", "account_scoped_liquidation", "binary_evidence", "resource_guards", "provenance_binding", "historical_boundary"]'
v2_r2_sv1_capacity_attestation="/home/vlad/v2-r2-sv1c-24h-binary-capacity-20260908-v1-capacity-seed-659-treatment-g4.json"
v2_r2_sv1_capacity_probe_prefix="v2-r2-sv1c-24h-capacity"
v2_r2_sv1_capacity_attestation_contract="v2-r2-sv1c-24h-binary-capacity-v1"
v2_r2_sv1_capacity_probe_contract="v2-r2-sv1c-24h-capacity-probe-v1"
v2_r2_sv1_capacity_measurement_config="$root_dir/research/configs/v2-r2-sv1c-24h/treatment-643.json"
v2_r2_sv1_capacity_measurement_seed=659
v2_r2_sv1_capacity_launch_config="$root_dir/research/configs/v2-r2-sv1c-24h/treatment-643.json"
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
	printf '%s\n' "${V2_R2_SV1C_ACTIVATION_PROVENANCE:-/home/vlad/external-scratch/v2-r2-sv1c-activation-${v2_r2_sv1_activation_seed}-${head_revision}/activation-provenance.json}"
}

v2_r2_sv1c_review_attestation_path() {
	local head_revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '%s\n' "${V2_R2_SV1C_REVIEW_ATTESTATION:-/home/vlad/external-scratch/v2-r2-sv1c-review-${head_revision}/review-attestation.json}"
}

v2_r2_sv1c_activation_review_attestation_path() {
	local head_revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '%s\n' "${V2_R2_SV1C_ACTIVATION_REVIEW_ATTESTATION:-/home/vlad/external-scratch/v2-r2-sv1c-activation-review-${v2_r2_sv1_activation_seed}-${head_revision}/review-attestation.json}"
}

v2_r2_sv1c_cpu_policy() {
	local host_cpu_count allowed_cpu_count
	command -v nproc >/dev/null 2>&1 || return 1
	host_cpu_count=$(nproc --all) || return 1
	[[ "$host_cpu_count" =~ ^[1-9][0-9]*$ ]] || return 1
	allowed_cpu_count=$((host_cpu_count * v2_r2_sv1_cpu_limit_percent / 100))
	(( allowed_cpu_count > 0 )) || allowed_cpu_count=1
	printf '%s\t%s\t0-%s\n' "$host_cpu_count" "$allowed_cpu_count" "$((allowed_cpu_count - 1))"
}

# A representative capacity run measures one immutable launch configuration,
# while the attestation may authorize a finite set of contract variants. The
# selected cell must be a member of that attested set; it is not the measured
# representative configuration unless the hashes happen to be identical.
v2_r2_sv1c_require_authorized_capacity_config() {
	[[ $# -eq 2 ]] || return 1
	local attestation_path=$1 selected_config_sha256=$2
	[[ "$attestation_path" == /* && "$attestation_path" != */ && "$attestation_path" != *$'\n'* && "$attestation_path" != *$'\t'* ]] || return 1
	[[ "$selected_config_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -f "$attestation_path" && ! -L "$attestation_path" ]] || return 1
	[[ "$(realpath -e -- "$attestation_path")" == "$attestation_path" ]] || return 1
	jq -e --arg selected_config_sha256 "$selected_config_sha256" '
		type == "object" and
		(.authorized_launch_config_sha256 | type) == "array" and
		(.authorized_launch_config_sha256 | length) > 0 and
		all(.authorized_launch_config_sha256[]; type == "string" and test("^[0-9a-f]{64}$")) and
		(.authorized_launch_config_sha256 | index($selected_config_sha256)) != null' \
		"$attestation_path" >/dev/null
}

v2_r2_sv1c_binary_metadata_value() {
	[[ $# -eq 2 ]] || return 1
	local metadata=$1 key=$2
	awk -v key="$key" '
		BEGIN { prefix = key "="; count = 0; value = "" }
		$1 == "build" && index($2, prefix) == 1 {
			count++
			value = substr($2, length(prefix) + 1)
		}
		END {
			if (count != 1 || value == "") {
				exit 1
			}
			print value
		}' <<<"$metadata"
}

v2_r2_sv1c_require_normalizer_source_revision() {
	[[ $# -eq 2 ]] || return 1
	local root_dir=$1 normalizer_revision=$2 current_revision
	[[ -e "$root_dir/.git" && "$normalizer_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	current_revision=$(git -C "$root_dir" rev-parse HEAD) || return 1
	[[ "$current_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	git -C "$root_dir" cat-file -e "$normalizer_revision^{commit}" || return 1
	git -C "$root_dir" merge-base --is-ancestor "$normalizer_revision" "$current_revision" || return 1
	# A normalizer built from an ancestor remains valid only when no Go or
	# module input changed between that build and the manifest's current tree.
	git -C "$root_dir" diff --quiet "$normalizer_revision" "$current_revision" -- '*.go' 'go.mod' 'go.sum' || return 1
	git -C "$root_dir" diff --quiet "$current_revision" -- '*.go' 'go.mod' 'go.sum' || return 1
	git -C "$root_dir" diff --cached --quiet -- '*.go' 'go.mod' 'go.sum' || return 1
	[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all -- '*.go' 'go.mod' 'go.sum')" ]] || return 1
}

v2_r2_sv1c_require_pinned_binary() {
	[[ $# -eq 4 ]] || return 1
	local binary=$1 expected_revision=$2 expected_sha256=$3 expected_package=$4
	local metadata go_version package_path module_path
	local buildmode compiler trimpath cgo_enabled goos goarch goamd64 vcs vcs_revision vcs_modified
	[[ "$binary" == /* && "$binary" != */ && "$binary" != *$'\n'* && "$binary" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -x "$binary" && ! -L "$binary" && "$(realpath -e -- "$binary")" == "$binary" ]] || return 1
	metadata=$(go version -m -- "$binary") || return 1
	go_version=$(sed -n '1s/.*: //p' <<<"$metadata") || return 1
	package_path=$(awk '$1 == "path" {count++; value=$2} END {if (count != 1 || value == "") exit 1; print value}' <<<"$metadata") || return 1
	module_path=$(awk '$1 == "mod" {count++; value=$2} END {if (count != 1 || value == "") exit 1; print value}' <<<"$metadata") || return 1
	buildmode=$(v2_r2_sv1c_binary_metadata_value "$metadata" "-buildmode") || return 1
	compiler=$(v2_r2_sv1c_binary_metadata_value "$metadata" "-compiler") || return 1
	trimpath=$(v2_r2_sv1c_binary_metadata_value "$metadata" "-trimpath") || return 1
	cgo_enabled=$(v2_r2_sv1c_binary_metadata_value "$metadata" "CGO_ENABLED") || return 1
	goos=$(v2_r2_sv1c_binary_metadata_value "$metadata" "GOOS") || return 1
	goarch=$(v2_r2_sv1c_binary_metadata_value "$metadata" "GOARCH") || return 1
	goamd64=$(v2_r2_sv1c_binary_metadata_value "$metadata" "GOAMD64") || return 1
	vcs=$(v2_r2_sv1c_binary_metadata_value "$metadata" "vcs") || return 1
	vcs_revision=$(v2_r2_sv1c_binary_metadata_value "$metadata" "vcs.revision") || return 1
	vcs_modified=$(v2_r2_sv1c_binary_metadata_value "$metadata" "vcs.modified") || return 1
	[[ "$go_version" == "go1.27.0" && "$package_path" == "$expected_package" && "$module_path" == "exchange_sim" &&
		"$buildmode" == "exe" && "$compiler" == "gc" && "$trimpath" == "true" && "$cgo_enabled" == "0" &&
		"$goos" == "linux" && "$goarch" == "amd64" && "$goamd64" == "v1" && "$vcs" == "git" &&
		"$vcs_revision" == "$expected_revision" && "$vcs_modified" == "false" ]] || return 1
	[[ "$(sha256sum -- "$binary" | awk '{print $1}')" == "$expected_sha256" ]]
}

v2_r2_sv1c_require_checkpoint_validator_attestation_binding() {
	[[ $# -eq 1 ]] || return 1
	local attestation=$1
	[[ -s "$attestation" && ! -L "$attestation" ]] || return 1
	v2_r2_require_registered_checkpoint_validator || return 1
	jq -e --arg path "$v2_r2_checkpoint_validator_path" \
		--arg revision "$v2_r2_checkpoint_validator_revision" \
		--arg sha256 "$v2_r2_checkpoint_validator_sha256" '
		type == "object" and
		.checkpoint_validator_path == $path and
		.checkpoint_validator_revision == $revision and
		.checkpoint_validator_sha256 == $sha256' \
		"$attestation" >/dev/null
}

v2_r2_sv1c_git_tree_sha256() {
	[[ $# -eq 1 ]] || return 1
	local revision=$1
	[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	git -C "$root_dir" ls-tree -r --full-tree "$revision" | sha256sum | awk '{print $1}'
}

v2_r2_require_sv1c_review_attestation() {
	[[ $# -eq 2 ]] || return 1
	local review_path=$1 expected_revision=$2 report_path report_sha256 actual_report_sha256 reviewed_tree_sha256 expected_tree_sha256
	[[ "$review_path" == /* && "$review_path" != */ && "$review_path" != *$'\n'* && "$review_path" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	[[ -f "$review_path" && ! -L "$review_path" ]] || return 1
	[[ "$(realpath -e -- "$review_path")" == "$review_path" ]] || return 1
	expected_tree_sha256=$(v2_r2_sv1c_git_tree_sha256 "$expected_revision") || return 1
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

v2_r2_require_sv1c_activation_review_attestation() {
	[[ $# -eq 3 ]] || return 1
	local review_path=$1 expected_revision=$2 activation_provenance_path=$3
	local report_path report_sha256 actual_report_sha256 expected_tree_sha256 reviewed_tree_sha256 activation_provenance_sha256
	[[ "$review_path" == /* && "$review_path" != */ && "$review_path" != *$'\n'* && "$review_path" != *$'\t'* ]] || return 1
	[[ "$activation_provenance_path" == /* && "$activation_provenance_path" != */ && "$activation_provenance_path" != *$'\n'* && "$activation_provenance_path" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	[[ -f "$review_path" && ! -L "$review_path" && "$(realpath -e -- "$review_path")" == "$review_path" ]] || return 1
	[[ -f "$activation_provenance_path" && ! -L "$activation_provenance_path" && "$(realpath -e -- "$activation_provenance_path")" == "$activation_provenance_path" ]] || return 1
	expected_tree_sha256=$(v2_r2_sv1c_git_tree_sha256 "$expected_revision") || return 1
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

v2_r2_sv1c_artifact_records() {
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

v2_r2_sv1c_verify_artifact_records() {
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

v2_r2_sv1c_require_activation_arm_artifacts() {
	[[ $# -eq 5 || $# -eq 6 ]] || return 1
	local arm_dir=$1 arm_name=$2 expected_revision=$3 expected_config_sha256=$4 expected_binary_sha256=$5
	local expected_outcome=${6:-completed}
	local expected_config expected_venue_ids expected_experiment expected_hypothesis
	local expected_root_files actual_root_files required_file status_field file_path expected_hash actual_hash
	[[ "$expected_outcome" == completed || "$expected_outcome" == terminal_failure ]] || return 1
	case "$arm_name" in
		treatment) expected_config="$v2_r2_sv1_activation_config" ;;
		control) expected_config="$v2_r2_sv1_activation_control_config" ;;
		*) return 1 ;;
	esac
	[[ "$arm_dir" == /* && "$arm_dir" != */ && "$arm_dir" != *$'\n'* && "$arm_dir" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_config_sha256" =~ ^[0-9a-f]{64}$ && "$expected_binary_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -d "$arm_dir" && ! -L "$arm_dir" && "$(realpath -e -- "$arm_dir")" == "$arm_dir" ]] || return 1
	if find "$arm_dir" -type l -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	expected_config=$(realpath -e -- "$expected_config") || return 1
	[[ -f "$expected_config" && ! -L "$expected_config" ]] || return 1
	expected_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0 and all(.[]; type == "string" and length > 0))' "$expected_config") || return 1
	expected_experiment=$(jq -er '.experiment_id | select(type == "string" and length > 0)' "$expected_config") || return 1
	expected_hypothesis=$(jq -er '.hypothesis_id | select(type == "string" and length > 0)' "$expected_config") || return 1
	[[ -f "$arm_dir/run-config.json" && ! -L "$arm_dir/run-config.json" && "$(sha256sum -- "$arm_dir/run-config.json" | awk '{print $1}')" == "$expected_config_sha256" ]] || return 1
	expected_root_files=$(printf '%s\n' \
		run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json checkpoints.jsonl \
		events.evs binary-evidence-attestation.json evidence-manifest.json evidence-only-artifact-hash.json terminal-outcome.json \
		market-data-evidence-v2.json market-data-schedules-v2.bin market-data-receipts-v2.bin \
		market-data-decisions-v2.bin market-data-actions-v2.bin simulator.stdout.log simulator.stderr.log | LC_ALL=C sort)
	actual_root_files=$(find "$arm_dir" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)
	[[ "$actual_root_files" == "$expected_root_files" ]] || return 1
	for required_file in run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json checkpoints.jsonl \
		events.evs binary-evidence-attestation.json evidence-manifest.json evidence-only-artifact-hash.json terminal-outcome.json \
		market-data-evidence-v2.json market-data-schedules-v2.bin market-data-receipts-v2.bin market-data-decisions-v2.bin market-data-actions-v2.bin \
		simulator.stdout.log simulator.stderr.log; do
		[[ -f "$arm_dir/$required_file" && ! -L "$arm_dir/$required_file" ]] || return 1
	done
	for required_file in run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json checkpoints.jsonl \
		events.evs binary-evidence-attestation.json evidence-manifest.json evidence-only-artifact-hash.json terminal-outcome.json market-data-evidence-v2.json; do
		[[ -s "$arm_dir/$required_file" ]] || return 1
	done
	v2_r2_verify_evidence_manifest "$arm_dir" || return 1
	jq -e --arg revision "$expected_revision" --argjson seed "$v2_r2_sv1_activation_seed" \
		--argjson venue_ids "$expected_venue_ids" --arg experiment "$expected_experiment" --arg hypothesis "$expected_hypothesis" \
		--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		--arg expected_config_sha256 "$expected_config_sha256" --arg expected_binary_sha256 "$expected_binary_sha256" \
		--arg arm_name "$arm_name" --arg expected_cell "${v2_r2_sv1_activation_output_prefix}-${v2_r2_sv1_activation_seed}-${arm_name}" \
		--argjson start_nano "$v2_r2_sv1_activation_simulation_start_nano" --argjson end_nano "$v2_r2_sv1_activation_simulation_end_nano" \
		--arg checkpoint_validator_path "$v2_r2_checkpoint_validator_path" \
		--arg checkpoint_validator_revision "$v2_r2_checkpoint_validator_revision" \
		--arg checkpoint_validator_sha256 "$v2_r2_checkpoint_validator_sha256" '
		type == "object" and .schema_version == 1 and .cell == $expected_cell and .seed == $seed and
		.simulated_horizon == "5m" and .simulation_start_nano == $start_nano and .simulation_end_nano == $end_nano and
		.config_sha256 == $expected_config_sha256 and .binary_sha256 == $expected_binary_sha256 and
		.git_revision == $revision and .config_experiment_id == $experiment and .hypothesis_id == $hypothesis and
		.evidence_format == $evidence_format and .log_mode == $log_mode and .venue_ids == $venue_ids and
		.binary_go_version == "go1.27.0" and .binary_goos == "linux" and .binary_goarch == "amd64" and .binary_goamd64 == "v1" and
		.gomaxprocs == 2 and .memory_limit_bytes == 21474836480 and .gomemlimit_bytes == 19327352832 and
		.minimum_free_bytes == 4294967296 and .cpu_limit_percent == 90 and
		.checkpoint_validator_path == $checkpoint_validator_path and
		.checkpoint_validator_revision == $checkpoint_validator_revision and
		.checkpoint_validator_sha256 == $checkpoint_validator_sha256' \
		"$arm_dir/run-metadata.json" >/dev/null || return 1
	jq -e --arg revision "$expected_revision" --argjson seed "$v2_r2_sv1_activation_seed" \
		--argjson venue_ids "$expected_venue_ids" --arg evidence_format "$v2_r2_sv1_activation_evidence_format" \
		--arg log_mode "$v2_r2_sv1_activation_log_mode" '
		type == "object" and .schema_version == 2 and
		.build.revision == $revision and .build.modified == false and .build.goos == "linux" and
		.build.goarch == "amd64" and .build.goamd64 == "v1" and .venue_ids == $venue_ids and
		.config.seed == $seed and .config.log_mode == $log_mode and .config.evidence_format == $evidence_format and
		.config.record_market_data_receipts == true and
		.config.strict_risk_contract == true and .config.auto_borrow_spot == false and
		.config.cross_asset_spot_graph == true and .config.cross_asset_collateral_marks == false and
		(.config.perp_exposure_hedger == null or .config.perp_exposure_hedger.auto_borrow_perp != true)' \
		"$arm_dir/manifest.json" >/dev/null || return 1
	if [[ "$expected_outcome" == completed ]]; then
		jq -e --arg arm_name "$arm_name" --arg arm_status_contract "$v2_r2_sv1_activation_arm_status_contract" '
			type == "object" and .schema_version == 2 and .contract == $arm_status_contract and
			.arm == $arm_name and .exit_status == 0 and .completion_verified == true and
			.terminal_failure_verified == false and .terminal_outcome_status == "completed" and
			.resource_guard_failed == false and
			all([.terminal_outcome_sha256, .run_metadata_sha256, .manifest_sha256, .greeks_sha256,
				.checkpoints_sha256, .binary_attestation_sha256, .evidence_manifest_sha256][];
				type == "string" and test("^[0-9a-f]{64}$"))' "$arm_dir/run-status.json" >/dev/null || return 1
	else
		jq -e --arg arm_name "$arm_name" --arg arm_status_contract "$v2_r2_sv1_activation_arm_status_contract" '
			type == "object" and .schema_version == 2 and .contract == $arm_status_contract and
			.arm == $arm_name and (.exit_status | type == "number" and floor == . and . > 0 and . <= 255) and
			.completion_verified == false and .terminal_failure_verified == true and
			.terminal_outcome_status == "terminal_failure" and .resource_guard_failed == false and
			all([.terminal_outcome_sha256, .run_metadata_sha256, .manifest_sha256, .greeks_sha256,
				.checkpoints_sha256, .binary_attestation_sha256, .evidence_manifest_sha256][];
				type == "string" and test("^[0-9a-f]{64}$"))' "$arm_dir/run-status.json" >/dev/null || return 1
	fi
	while IFS=$'\t' read -r status_field file_path; do
		expected_hash=$(jq -er --arg field "$status_field" '.[$field] | select(type == "string" and test("^[0-9a-f]{64}$"))' "$arm_dir/run-status.json") || return 1
		actual_hash=$(sha256sum -- "$arm_dir/$file_path" | awk '{print $1}') || return 1
		[[ "$actual_hash" == "$expected_hash" ]] || return 1
	done < <(printf '%s\n' \
		$'terminal_outcome_sha256\tterminal-outcome.json' \
		$'run_metadata_sha256\trun-metadata.json' \
		$'manifest_sha256\tmanifest.json' \
		$'greeks_sha256\tgreeks.json' \
		$'latency_sha256\tlatency.json' \
		$'checkpoints_sha256\tcheckpoints.jsonl' \
		$'binary_attestation_sha256\tbinary-evidence-attestation.json' \
		$'evidence_manifest_sha256\tevidence-manifest.json')
	if [[ "$expected_outcome" == completed ]]; then
		v2_r2_terminal_completed_outcome_present "$arm_dir" || return 1
	else
		v2_r2_terminal_failure_outcome_present "$arm_dir" || return 1
	fi
	v2_r2_require_checkpoint_stream "$arm_dir/checkpoints.jsonl" \
		"$v2_r2_sv1_activation_simulation_start_nano" "$v2_r2_sv1_activation_simulation_end_nano" evstream_v3 || return 1
	v2_r2_sv1c_require_checkpoint_attestation_binding "$arm_dir/checkpoints.jsonl" "$arm_dir/binary-evidence-attestation.json" || return 1
	jq -e '
		type == "object" and .domain == "canonical_binary_execution_frames" and .ordering == "ordered_stream" and
		.hashing == "route_sequence_neutral_v1" and (.event_frames | type) == "number" and (.event_frames | floor) == .event_frames and .event_frames > 0 and
		(.stream_frames | type) == "number" and (.stream_frames | floor) == .stream_frames and .stream_frames >= .event_frames and
		(.execution_stream_hash | type) == "string" and (.execution_stream_hash | test("^[0-9a-f]{64}$")) and
		(.canonical_execution_stream_hash | type) == "string" and (.canonical_execution_stream_hash | test("^[0-9a-f]{64}$")) and
		((.unencodable_payloads // 0) | type) == "number" and (((.unencodable_payloads // 0) | floor) == (.unencodable_payloads // 0)) and ((.unencodable_payloads // 0) | . == 0)' \
		"$arm_dir/binary-evidence-attestation.json" >/dev/null || return 1
}

v2_r2_sv1c_require_checkpoint_attestation_binding() {
	[[ $# -eq 2 ]] || return 1
	local checkpoints=$1 attestation=$2
	[[ -s "$checkpoints" && ! -L "$checkpoints" && -s "$attestation" && ! -L "$attestation" ]] || return 1
	v2_r2_require_binary_checkpoint_stream_exact \
		"$checkpoints" "$v2_r2_sv1_activation_simulation_start_nano" \
		"$v2_r2_sv1_activation_simulation_end_nano" "$attestation"
}

v2_r2_sv1c_require_terminal_arm_reconstruction() {
	[[ $# -eq 2 ]] || return 1
	local renderer_path=$1 arm_dir=$2 render_root render_dir report_path renderer_status
	[[ "$renderer_path" == /* && "$renderer_path" != */ && "$renderer_path" != *$'\n'* && "$renderer_path" != *$'\t'* ]] || return 1
	[[ "$arm_dir" == /* && "$arm_dir" != */ && "$arm_dir" != *$'\n'* && "$arm_dir" != *$'\t'* ]] || return 1
	render_root=$(mktemp -d) || return 1
	render_dir="$render_root/rendered"
	report_path="$render_root/report.json"
	if "$renderer_path" -dir "$arm_dir" -out "$render_dir" -route-compression none >"$report_path" 2>"$render_root/renderer.stderr"; then
		renderer_status=0
	else
		renderer_status=$?
	fi
	if [[ "$renderer_status" -ne 0 ]]; then
		rm -rf -- "$render_root"
		return 1
	fi
	if ! jq -e --slurpfile attestation "$arm_dir/binary-evidence-attestation.json" '
		type == "object" and (.event_frames | type) == "number" and .event_frames > 0 and
		.event_frames == $attestation[0].event_frames and
		(.dictionary_frames | type) == "number" and
		(.dictionary_frames + .event_frames) == $attestation[0].stream_frames and
		(.routes | type) == "number" and .routes > 0 and
		.route_compression == "none" and
		.execution_stream_hash == $attestation[0].execution_stream_hash and
		.canonical_execution_stream_hash == $attestation[0].canonical_execution_stream_hash' \
		"$report_path" >/dev/null; then
		rm -rf -- "$render_root"
		return 1
	fi
	if ! find "$render_dir/venues" -type f -name '*.jsonl' -size +0c -print -quit 2>/dev/null | grep -q .; then
		rm -rf -- "$render_root"
		return 1
	fi
	rm -rf -- "$render_root"
}

v2_r2_sv1c_require_terminal_failure_pair_provenance() {
	[[ $# -eq 3 ]] || return 1
	local provenance_path=$1 expected_revision=$2 expected_binary_sha256=$3
	local output_root treatment_dir control_dir comparison_path review_path simulator_path analyzer_path renderer_path checkpoint_validator_path path
	local treatment_status control_status arm arm_status expected_outcome arm_dir arm_config_sha256 artifacts
	local actual_sha256 expected_sha256 expected_tree_sha256 analyzer_sha256 renderer_sha256 checkpoint_validator_revision checkpoint_validator_sha256
	local treatment_config_path control_config_path expected_treatment_config expected_control_config
	local treatment_source_config_sha256 control_source_config_sha256 treatment_venue_ids control_venue_ids
	local treatment_experiment control_experiment treatment_hypothesis control_hypothesis
	local expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity
	[[ "$provenance_path" == /* && "$provenance_path" != */ && "$provenance_path" != *$'\n'* && "$provenance_path" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_binary_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -f "$provenance_path" && ! -L "$provenance_path" && "$(realpath -e -- "$provenance_path")" == "$provenance_path" ]] || return 1
	expected_tree_sha256=$(v2_r2_sv1c_git_tree_sha256 "$expected_revision") || return 1
	IFS=$'\t' read -r expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity < <(v2_r2_sv1c_cpu_policy) || return 1
	output_root=$(jq -er '.output_root | select(type == "string")' "$provenance_path") || return 1
	treatment_dir=$(jq -er '.treatment_dir | select(type == "string")' "$provenance_path") || return 1
	control_dir=$(jq -er '.control_dir | select(type == "string")' "$provenance_path") || return 1
	comparison_path=$(jq -er '.comparison_path | select(type == "string")' "$provenance_path") || return 1
	review_path=$(jq -er '.review_attestation_path | select(type == "string")' "$provenance_path") || return 1
	simulator_path=$(jq -er '.simulator_binary_path | select(type == "string")' "$provenance_path") || return 1
	analyzer_path=$(jq -er '.analyzer_binary_path | select(type == "string")' "$provenance_path") || return 1
	renderer_path=$(jq -er '.renderer_binary_path | select(type == "string")' "$provenance_path") || return 1
	checkpoint_validator_path=$(jq -er '.checkpoint_validator_path | select(type == "string")' "$provenance_path") || return 1
	checkpoint_validator_revision=$(jq -er '.checkpoint_validator_revision | select(type == "string" and test("^[0-9a-f]{40}$"))' "$provenance_path") || return 1
	checkpoint_validator_sha256=$(jq -er '.checkpoint_validator_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	for path in "$output_root" "$treatment_dir" "$control_dir" "$comparison_path" "$review_path" "$simulator_path" "$analyzer_path" "$renderer_path" "$checkpoint_validator_path"; do
		[[ "$path" == /* && "$path" != */ && "$path" != *$'\n'* && "$path" != *$'\t'* ]] || return 1
	done
	[[ -d "$output_root" && ! -L "$output_root" && "$(realpath -e -- "$output_root")" == "$output_root" ]] || return 1
	[[ -d "$treatment_dir" && ! -L "$treatment_dir" && "$(realpath -e -- "$treatment_dir")" == "$treatment_dir" ]] || return 1
	[[ -d "$control_dir" && ! -L "$control_dir" && "$(realpath -e -- "$control_dir")" == "$control_dir" ]] || return 1
	[[ "$treatment_dir" == "$output_root/treatment" && "$control_dir" == "$output_root/control" ]] || return 1
	[[ "$comparison_path" == "$output_root/cdf-liquidity-comparison.json" && -f "$comparison_path" && ! -L "$comparison_path" && "$(realpath -e -- "$comparison_path")" == "$comparison_path" ]] || return 1
	[[ "$review_path" != "$output_root"/* && "$review_path" != "$root_dir"/* ]] || return 1
	expected_treatment_config=$(realpath -e -- "$v2_r2_sv1_activation_config") || return 1
	expected_control_config=$(realpath -e -- "$v2_r2_sv1_activation_control_config") || return 1
	treatment_config_path=$(jq -er '.treatment_source_config_path | select(type == "string")' "$provenance_path") || return 1
	control_config_path=$(jq -er '.control_source_config_path | select(type == "string")' "$provenance_path") || return 1
	[[ "$treatment_config_path" == "$expected_treatment_config" && "$control_config_path" == "$expected_control_config" ]] || return 1
	[[ -s "$treatment_config_path" && ! -L "$treatment_config_path" && "$(realpath -e -- "$treatment_config_path")" == "$treatment_config_path" ]] || return 1
	[[ -s "$control_config_path" && ! -L "$control_config_path" && "$(realpath -e -- "$control_config_path")" == "$control_config_path" ]] || return 1
	treatment_source_config_sha256=$(sha256sum -- "$treatment_config_path" | awk '{print $1}') || return 1
	control_source_config_sha256=$(sha256sum -- "$control_config_path" | awk '{print $1}') || return 1
	treatment_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0 and all(.[]; type == "string" and length > 0))' "$treatment_config_path") || return 1
	control_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0 and all(.[]; type == "string" and length > 0))' "$control_config_path") || return 1
	treatment_experiment=$(jq -er '.experiment_id | select(type == "string" and length > 0)' "$treatment_config_path") || return 1
	control_experiment=$(jq -er '.experiment_id | select(type == "string" and length > 0)' "$control_config_path") || return 1
	treatment_hypothesis=$(jq -er '.hypothesis_id | select(type == "string" and length > 0)' "$treatment_config_path") || return 1
	control_hypothesis=$(jq -er '.hypothesis_id | select(type == "string" and length > 0)' "$control_config_path") || return 1
	[[ "$treatment_venue_ids" == "$control_venue_ids" ]] || return 1
	jq -e --argjson seed "$v2_r2_sv1_activation_seed" --arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		'.seed == $seed and .evidence_format == $evidence_format and .log_mode == $log_mode' "$treatment_config_path" >/dev/null || return 1
	jq -e --argjson seed "$v2_r2_sv1_activation_seed" --arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		'.seed == $seed and .evidence_format == $evidence_format and .log_mode == $log_mode' "$control_config_path" >/dev/null || return 1
	analyzer_sha256=$(jq -er '.analyzer_binary_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	renderer_sha256=$(jq -er '.renderer_binary_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	v2_r2_sv1c_require_pinned_binary "$simulator_path" "$expected_revision" "$expected_binary_sha256" "exchange_sim/cmd/multivenue" || return 1
	v2_r2_sv1c_require_pinned_binary "$analyzer_path" "$expected_revision" "$analyzer_sha256" "exchange_sim/cmd/cdf-liquidity-audit" || return 1
	v2_r2_sv1c_require_pinned_binary "$renderer_path" "$expected_revision" "$renderer_sha256" "exchange_sim/cmd/evsrender" || return 1
	[[ "$checkpoint_validator_path" == "$v2_r2_checkpoint_validator_path" &&
		"$checkpoint_validator_revision" == "$expected_revision" &&
		"$checkpoint_validator_sha256" == "$v2_r2_checkpoint_validator_sha256" ]] || return 1
	v2_r2_require_registered_checkpoint_validator || return 1
	v2_r2_require_sv1c_review_attestation "$review_path" "$expected_revision" || return 1
	actual_sha256=$(sha256sum -- "$review_path" | awk '{print $1}') || return 1
	[[ "$actual_sha256" == "$(jq -er '.review_attestation_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path")" ]] || return 1
	jq -e --arg contract "$v2_r2_sv1_activation_pair_contract" --arg revision "$expected_revision" \
		--arg tree_sha256 "$expected_tree_sha256" --argjson seed "$v2_r2_sv1_activation_seed" \
		--argjson start_nano "$v2_r2_sv1_activation_simulation_start_nano" --argjson end_nano "$v2_r2_sv1_activation_simulation_end_nano" \
		--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		--argjson venue_ids "$treatment_venue_ids" --arg treatment_experiment "$treatment_experiment" --arg control_experiment "$control_experiment" \
		--arg treatment_hypothesis "$treatment_hypothesis" --arg control_hypothesis "$control_hypothesis" \
			--arg binary_sha256 "$expected_binary_sha256" --arg analyzer_sha256 "$analyzer_sha256" --arg renderer_sha256 "$renderer_sha256" \
			--arg checkpoint_validator_path "$checkpoint_validator_path" --arg checkpoint_validator_revision "$checkpoint_validator_revision" --arg checkpoint_validator_sha256 "$checkpoint_validator_sha256" \
		--arg treatment_config_sha256 "$treatment_source_config_sha256" --arg control_config_sha256 "$control_source_config_sha256" \
		--arg expected_host_cpu_count "$expected_host_cpu_count" --arg expected_allowed_cpu_count "$expected_allowed_cpu_count" \
		--arg expected_cpu_affinity "$expected_cpu_affinity" --argjson expected_gomaxprocs "$v2_r2_sv1_activation_gomaxprocs" \
		--argjson expected_memory_limit_bytes "$v2_r2_sv1_activation_memory_limit_bytes" --argjson expected_gomemlimit_bytes "$v2_r2_sv1_activation_gomemlimit_bytes" \
		--argjson expected_minimum_free_bytes "$v2_r2_sv1_activation_minimum_free_bytes" --argjson expected_cpu_limit_percent "$v2_r2_sv1_cpu_limit_percent" '
		type == "object" and .schema_version == 3 and .contract == $contract and
		.candidate_revision == $revision and .candidate_tree_sha256 == $tree_sha256 and .seed == $seed and
		.simulated_horizon == "5m" and .simulation_start_nano == $start_nano and .simulation_end_nano == $end_nano and
		.evidence_format == $evidence_format and .log_mode == $log_mode and .venue_ids == $venue_ids and
		.treatment_experiment_id == $treatment_experiment and .control_experiment_id == $control_experiment and
		.treatment_hypothesis_id == $treatment_hypothesis and .control_hypothesis_id == $control_hypothesis and
		.status == "UNAVAILABLE_TERMINAL_FAILURE" and .activation_satisfied == false and
		.holdouts_consumed == false and .treatment_runner_status == 0 and .control_runner_status == 0 and
		(.treatment_terminal_status == "completed" or .treatment_terminal_status == "terminal_failure") and
		(.control_terminal_status == "completed" or .control_terminal_status == "terminal_failure") and
		(.treatment_terminal_status == "terminal_failure" or .control_terminal_status == "terminal_failure") and
		.simulator_binary_sha256 == $binary_sha256 and .analyzer_binary_sha256 == $analyzer_sha256 and
		.renderer_binary_sha256 == $renderer_sha256 and
		.checkpoint_validator_path == $checkpoint_validator_path and .checkpoint_validator_revision == $checkpoint_validator_revision and
		.checkpoint_validator_sha256 == $checkpoint_validator_sha256 and
		.treatment_source_config_sha256 == $treatment_config_sha256 and .control_source_config_sha256 == $control_config_sha256 and
		.treatment_config_sha256 == $treatment_config_sha256 and .control_config_sha256 == $control_config_sha256 and
		all([.treatment_run_status_sha256, .control_run_status_sha256,
			.treatment_terminal_outcome_sha256, .control_terminal_outcome_sha256][];
			type == "string" and test("^[0-9a-f]{64}$")) and
		(.review_attestation_sha256 | type) == "string" and (.review_attestation_sha256 | test("^[0-9a-f]{64}$")) and
		(.comparison_sha256 | type) == "string" and (.comparison_sha256 | test("^[0-9a-f]{64}$")) and
		(.resource_policy | type) == "object" and
		.resource_policy.gomaxprocs == $expected_gomaxprocs and
		.resource_policy.memory_limit_bytes == $expected_memory_limit_bytes and
		.resource_policy.gomemlimit_bytes == $expected_gomemlimit_bytes and
		.resource_policy.minimum_free_bytes == $expected_minimum_free_bytes and
		.resource_policy.host_cpu_count == ($expected_host_cpu_count | tonumber) and
		.resource_policy.allowed_cpu_count == ($expected_allowed_cpu_count | tonumber) and
		.resource_policy.cpu_limit_percent == $expected_cpu_limit_percent and
		.resource_policy.cpu_affinity == $expected_cpu_affinity' "$provenance_path" >/dev/null || return 1
	treatment_status=$(jq -er '.treatment_terminal_status' "$provenance_path") || return 1
	control_status=$(jq -er '.control_terminal_status' "$provenance_path") || return 1
	jq -e --arg contract "$v2_r2_sv1_activation_contract" --arg treatment_status "$treatment_status" \
		--arg control_status "$control_status" '
		type == "object" and .schema_version == 2 and .contract == $contract and
		.status == "UNAVAILABLE_TERMINAL_FAILURE" and .valid == false and .evidence_valid == true and
		.activation_satisfied == false and .anti_cheating_satisfied == false and .measurement_valid == true and
		.treatment_terminal_status == $treatment_status and .control_terminal_status == $control_status' \
		"$comparison_path" >/dev/null || return 1
	actual_sha256=$(sha256sum -- "$comparison_path" | awk '{print $1}') || return 1
	expected_sha256=$(jq -er '.comparison_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	[[ "$actual_sha256" == "$expected_sha256" ]] || return 1
	for arm in treatment control; do
		arm_dir=$([[ "$arm" == treatment ]] && printf '%s' "$treatment_dir" || printf '%s' "$control_dir")
		arm_config=$([[ "$arm" == treatment ]] && printf '%s' "$v2_r2_sv1_activation_config" || printf '%s' "$v2_r2_sv1_activation_control_config")
		arm_config_sha256=$(sha256sum -- "$arm_config" | awk '{print $1}') || return 1
		arm_status=$(jq -er --arg arm "$arm" '.[$arm + "_terminal_status"]' "$provenance_path") || return 1
		expected_outcome=completed
		if [[ "$arm_status" == terminal_failure ]]; then
			expected_outcome=terminal_failure
		fi
		v2_r2_sv1c_require_activation_arm_artifacts "$arm_dir" "$arm" "$expected_revision" "$arm_config_sha256" "$expected_binary_sha256" "$expected_outcome" || return 1
		actual_sha256=$(sha256sum -- "$arm_dir/run-status.json" | awk '{print $1}') || return 1
		expected_sha256=$(jq -er --arg arm "$arm" '.[$arm + "_run_status_sha256"] | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
		[[ "$actual_sha256" == "$expected_sha256" ]] || return 1
		actual_sha256=$(sha256sum -- "$arm_dir/terminal-outcome.json" | awk '{print $1}') || return 1
		expected_sha256=$(jq -er --arg arm "$arm" '.[$arm + "_terminal_outcome_sha256"] | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
		[[ "$actual_sha256" == "$expected_sha256" ]] || return 1
		artifacts=$(jq -c --arg arm "$arm" '.[$arm + "_artifacts"]' "$provenance_path") || return 1
		v2_r2_sv1c_verify_artifact_records "$arm_dir" "$artifacts" || return 1
		v2_r2_sv1c_require_terminal_arm_reconstruction "$renderer_path" "$arm_dir" || return 1
	done
}

v2_r2_sv1c_require_invalid_audit_diagnostic() {
	[[ $# -eq 4 ]] || return 1
	local provenance_path=$1 expected_output_root=$2 expected_audit_status=$3 expected_object_valid=$4 comparison_path actual_object_valid=false actual_sha256 expected_sha256
	[[ "$provenance_path" == /* && "$provenance_path" != */ && "$provenance_path" != *$'\n'* && "$provenance_path" != *$'\t'* ]] || return 1
	[[ "$expected_output_root" == /* && "$expected_output_root" != */ && "$expected_output_root" != *$'\n'* && "$expected_output_root" != *$'\t'* ]] || return 1
	[[ ("$expected_object_valid" == true && "$expected_audit_status" =~ ^[1-9][0-9]*$) ||
		("$expected_object_valid" == false && "$expected_audit_status" =~ ^[0-9]+$) ]] || return 1
	v2_r2_require_single_json_object "$provenance_path" || return 1
	[[ -f "$provenance_path" && ! -L "$provenance_path" ]] || return 1
	comparison_path=$(jq -er '.comparison_path | select(type == "string")' "$provenance_path") || return 1
	[[ "$comparison_path" == "$expected_output_root/cdf-liquidity-comparison.json.invalid" &&
		-f "$comparison_path" && ! -L "$comparison_path" && "$(realpath -e -- "$comparison_path")" == "$comparison_path" ]] || return 1
	if v2_r2_require_single_json_object "$comparison_path"; then
		actual_object_valid=true
	fi
	[[ "$actual_object_valid" == "$expected_object_valid" ]] || return 1
	jq -e --arg expected_status "INVALID_AUDIT_EVIDENCE" --argjson expected_audit_status "$expected_audit_status" --argjson expected_object_valid "$expected_object_valid" '
		type == "object" and .status == $expected_status and .activation_satisfied == false and
		.holdouts_consumed == false and .analyzer_exit_status == $expected_audit_status and .comparison_object_valid == $expected_object_valid and
		(.comparison_sha256 | type) == "string" and (.comparison_sha256 | test("^[0-9a-f]{64}$"))' \
		"$provenance_path" >/dev/null || return 1
	actual_sha256=$(sha256sum -- "$comparison_path" | awk '{print $1}') || return 1
	expected_sha256=$(jq -er '.comparison_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	[[ "$actual_sha256" == "$expected_sha256" ]]
}

v2_r2_sv1c_require_invalid_audit_pair_provenance() {
	[[ $# -eq 5 ]] || return 1
	local provenance_path=$1 expected_revision=$2 expected_binary_sha256=$3 expected_audit_status=$4 expected_object_valid=$5
	local output_root treatment_dir control_dir comparison_path review_path simulator_path analyzer_path renderer_path checkpoint_validator_path path
	local actual_sha256 expected_sha256 expected_tree_sha256 analyzer_sha256 renderer_sha256 checkpoint_validator_revision checkpoint_validator_sha256
	local treatment_config_path control_config_path expected_treatment_config expected_control_config
	local treatment_source_config_sha256 control_source_config_sha256 treatment_venue_ids control_venue_ids
	local treatment_experiment control_experiment treatment_hypothesis control_hypothesis
	local expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity
	local treatment_artifacts control_artifacts arm arm_dir arm_config_sha256 artifacts
	local replay_dir replay_path replay_status
	[[ "$provenance_path" == /* && "$provenance_path" != */ && "$provenance_path" != *$'\n'* && "$provenance_path" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_binary_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -f "$provenance_path" && ! -L "$provenance_path" && "$(realpath -e -- "$provenance_path")" == "$provenance_path" ]] || return 1
	expected_tree_sha256=$(v2_r2_sv1c_git_tree_sha256 "$expected_revision") || return 1
	IFS=$'\t' read -r expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity < <(v2_r2_sv1c_cpu_policy) || return 1
	output_root=$(jq -er '.output_root | select(type == "string")' "$provenance_path") || return 1
	treatment_dir=$(jq -er '.treatment_dir | select(type == "string")' "$provenance_path") || return 1
	control_dir=$(jq -er '.control_dir | select(type == "string")' "$provenance_path") || return 1
	comparison_path=$(jq -er '.comparison_path | select(type == "string")' "$provenance_path") || return 1
	v2_r2_sv1c_require_invalid_audit_diagnostic "$provenance_path" "$output_root" "$expected_audit_status" "$expected_object_valid" || return 1
	review_path=$(jq -er '.review_attestation_path | select(type == "string")' "$provenance_path") || return 1
	simulator_path=$(jq -er '.simulator_binary_path | select(type == "string")' "$provenance_path") || return 1
	analyzer_path=$(jq -er '.analyzer_binary_path | select(type == "string")' "$provenance_path") || return 1
	renderer_path=$(jq -er '.renderer_binary_path | select(type == "string")' "$provenance_path") || return 1
	checkpoint_validator_path=$(jq -er '.checkpoint_validator_path | select(type == "string")' "$provenance_path") || return 1
	checkpoint_validator_revision=$(jq -er '.checkpoint_validator_revision | select(type == "string" and test("^[0-9a-f]{40}$"))' "$provenance_path") || return 1
	checkpoint_validator_sha256=$(jq -er '.checkpoint_validator_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	for path in "$output_root" "$treatment_dir" "$control_dir" "$comparison_path" "$review_path" "$simulator_path" "$analyzer_path" "$renderer_path" "$checkpoint_validator_path"; do
		[[ "$path" == /* && "$path" != */ && "$path" != *$'\n'* && "$path" != *$'\t'* ]] || return 1
	done
	[[ -d "$output_root" && ! -L "$output_root" && "$(realpath -e -- "$output_root")" == "$output_root" ]] || return 1
	[[ -d "$treatment_dir" && ! -L "$treatment_dir" && "$(realpath -e -- "$treatment_dir")" == "$treatment_dir" ]] || return 1
	[[ -d "$control_dir" && ! -L "$control_dir" && "$(realpath -e -- "$control_dir")" == "$control_dir" ]] || return 1
	[[ "$treatment_dir" == "$output_root/treatment" && "$control_dir" == "$output_root/control" && "$treatment_dir" != "$control_dir" ]] || return 1
	[[ "$comparison_path" == "$output_root/cdf-liquidity-comparison.json.invalid" && -f "$comparison_path" && ! -L "$comparison_path" && "$(realpath -e -- "$comparison_path")" == "$comparison_path" ]] || return 1
	expected_treatment_config=$(realpath -e -- "$v2_r2_sv1_activation_config") || return 1
	expected_control_config=$(realpath -e -- "$v2_r2_sv1_activation_control_config") || return 1
	treatment_config_path=$(jq -er '.treatment_source_config_path | select(type == "string")' "$provenance_path") || return 1
	control_config_path=$(jq -er '.control_source_config_path | select(type == "string")' "$provenance_path") || return 1
	[[ "$treatment_config_path" == "$expected_treatment_config" && "$control_config_path" == "$expected_control_config" ]] || return 1
	[[ -s "$treatment_config_path" && ! -L "$treatment_config_path" && -s "$control_config_path" && ! -L "$control_config_path" ]] || return 1
	treatment_source_config_sha256=$(sha256sum -- "$treatment_config_path" | awk '{print $1}') || return 1
	control_source_config_sha256=$(sha256sum -- "$control_config_path" | awk '{print $1}') || return 1
	treatment_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0 and all(.[]; type == "string" and length > 0))' "$treatment_config_path") || return 1
	control_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0 and all(.[]; type == "string" and length > 0))' "$control_config_path") || return 1
	treatment_experiment=$(jq -er '.experiment_id | select(type == "string" and length > 0)' "$treatment_config_path") || return 1
	control_experiment=$(jq -er '.experiment_id | select(type == "string" and length > 0)' "$control_config_path") || return 1
	treatment_hypothesis=$(jq -er '.hypothesis_id | select(type == "string" and length > 0)' "$treatment_config_path") || return 1
	control_hypothesis=$(jq -er '.hypothesis_id | select(type == "string" and length > 0)' "$control_config_path") || return 1
	[[ "$treatment_venue_ids" == "$control_venue_ids" ]] || return 1
	jq -e --argjson seed "$v2_r2_sv1_activation_seed" --arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		'.seed == $seed and .evidence_format == $evidence_format and .log_mode == $log_mode' "$treatment_config_path" >/dev/null || return 1
	jq -e --argjson seed "$v2_r2_sv1_activation_seed" --arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		'.seed == $seed and .evidence_format == $evidence_format and .log_mode == $log_mode' "$control_config_path" >/dev/null || return 1
	analyzer_sha256=$(jq -er '.analyzer_binary_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	renderer_sha256=$(jq -er '.renderer_binary_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	v2_r2_sv1c_require_pinned_binary "$simulator_path" "$expected_revision" "$expected_binary_sha256" "exchange_sim/cmd/multivenue" || return 1
	v2_r2_sv1c_require_pinned_binary "$analyzer_path" "$expected_revision" "$analyzer_sha256" "exchange_sim/cmd/cdf-liquidity-audit" || return 1
	v2_r2_sv1c_require_pinned_binary "$renderer_path" "$expected_revision" "$renderer_sha256" "exchange_sim/cmd/evsrender" || return 1
	[[ "$checkpoint_validator_path" == "$v2_r2_checkpoint_validator_path" &&
		"$checkpoint_validator_revision" == "$expected_revision" &&
		"$checkpoint_validator_sha256" == "$v2_r2_checkpoint_validator_sha256" ]] || return 1
	v2_r2_require_registered_checkpoint_validator || return 1
	v2_r2_require_sv1c_review_attestation "$review_path" "$expected_revision" || return 1
	[[ "$review_path" != "$output_root"/* && "$review_path" != "$root_dir"/* ]] || return 1
	actual_sha256=$(sha256sum -- "$review_path" | awk '{print $1}') || return 1
	[[ "$actual_sha256" == "$(jq -er '.review_attestation_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path")" ]] || return 1
	actual_sha256=$(sha256sum -- "$comparison_path" | awk '{print $1}') || return 1
	[[ "$actual_sha256" == "$(jq -er '.comparison_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path")" ]] || return 1
	jq -e --arg contract "$v2_r2_sv1_activation_pair_contract" --arg revision "$expected_revision" \
		--arg tree_sha256 "$expected_tree_sha256" --argjson seed "$v2_r2_sv1_activation_seed" \
		--argjson start_nano "$v2_r2_sv1_activation_simulation_start_nano" --argjson end_nano "$v2_r2_sv1_activation_simulation_end_nano" \
		--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		--argjson venue_ids "$treatment_venue_ids" --arg treatment_experiment "$treatment_experiment" --arg control_experiment "$control_experiment" \
		--arg treatment_hypothesis "$treatment_hypothesis" --arg control_hypothesis "$control_hypothesis" \
		--arg binary_sha256 "$expected_binary_sha256" --arg analyzer_sha256 "$analyzer_sha256" --arg renderer_sha256 "$renderer_sha256" \
		--arg checkpoint_validator_path "$checkpoint_validator_path" --arg checkpoint_validator_revision "$checkpoint_validator_revision" --arg checkpoint_validator_sha256 "$checkpoint_validator_sha256" \
		--arg treatment_config_sha256 "$treatment_source_config_sha256" --arg control_config_sha256 "$control_source_config_sha256" \
		--arg expected_host_cpu_count "$expected_host_cpu_count" --arg expected_allowed_cpu_count "$expected_allowed_cpu_count" \
		--arg expected_cpu_affinity "$expected_cpu_affinity" --argjson expected_gomaxprocs "$v2_r2_sv1_activation_gomaxprocs" \
		--argjson expected_memory_limit_bytes "$v2_r2_sv1_activation_memory_limit_bytes" --argjson expected_gomemlimit_bytes "$v2_r2_sv1_activation_gomemlimit_bytes" \
		--argjson expected_minimum_free_bytes "$v2_r2_sv1_activation_minimum_free_bytes" --argjson expected_cpu_limit_percent "$v2_r2_sv1_cpu_limit_percent" \
		--argjson expected_audit_status "$expected_audit_status" --argjson expected_object_valid "$expected_object_valid" '
		type == "object" and .schema_version == 3 and .contract == $contract and
		.candidate_revision == $revision and .candidate_tree_sha256 == $tree_sha256 and .seed == $seed and
		.simulated_horizon == "5m" and .simulation_start_nano == $start_nano and .simulation_end_nano == $end_nano and
		.evidence_format == $evidence_format and .log_mode == $log_mode and .venue_ids == $venue_ids and
		.treatment_experiment_id == $treatment_experiment and .control_experiment_id == $control_experiment and
		.treatment_hypothesis_id == $treatment_hypothesis and .control_hypothesis_id == $control_hypothesis and
		.status == "INVALID_AUDIT_EVIDENCE" and .activation_satisfied == false and .holdouts_consumed == false and
		.treatment_runner_status == 0 and .control_runner_status == 0 and
		.treatment_terminal_status == "completed" and .control_terminal_status == "completed" and
		.analyzer_exit_status == $expected_audit_status and .comparison_object_valid == $expected_object_valid and
		.simulator_binary_sha256 == $binary_sha256 and .analyzer_binary_sha256 == $analyzer_sha256 and
		.renderer_binary_sha256 == $renderer_sha256 and
		.checkpoint_validator_path == $checkpoint_validator_path and .checkpoint_validator_revision == $checkpoint_validator_revision and
		.checkpoint_validator_sha256 == $checkpoint_validator_sha256 and
		.treatment_source_config_sha256 == $treatment_config_sha256 and .control_source_config_sha256 == $control_config_sha256 and
		.treatment_config_sha256 == $treatment_config_sha256 and .control_config_sha256 == $control_config_sha256 and
		all([.treatment_run_status_sha256, .control_run_status_sha256, .treatment_terminal_outcome_sha256, .control_terminal_outcome_sha256][];
			type == "string" and test("^[0-9a-f]{64}$")) and
		(.review_attestation_sha256 | type) == "string" and (.review_attestation_sha256 | test("^[0-9a-f]{64}$")) and
		(.comparison_sha256 | type) == "string" and (.comparison_sha256 | test("^[0-9a-f]{64}$")) and
		(.treatment_artifacts | type) == "array" and (.treatment_artifacts | length) > 0 and
		(.control_artifacts | type) == "array" and (.control_artifacts | length) > 0 and
		(.resource_policy | type) == "object" and
		.resource_policy.gomaxprocs == $expected_gomaxprocs and
		.resource_policy.memory_limit_bytes == $expected_memory_limit_bytes and
		.resource_policy.gomemlimit_bytes == $expected_gomemlimit_bytes and
		.resource_policy.minimum_free_bytes == $expected_minimum_free_bytes and
		.resource_policy.host_cpu_count == ($expected_host_cpu_count | tonumber) and
		.resource_policy.allowed_cpu_count == ($expected_allowed_cpu_count | tonumber) and
		.resource_policy.cpu_limit_percent == $expected_cpu_limit_percent and
		.resource_policy.cpu_affinity == $expected_cpu_affinity' "$provenance_path" >/dev/null || return 1
	for arm in treatment control; do
		arm_dir=$([[ "$arm" == treatment ]] && printf '%s' "$treatment_dir" || printf '%s' "$control_dir")
		arm_config_sha256=$([[ "$arm" == treatment ]] && printf '%s' "$treatment_source_config_sha256" || printf '%s' "$control_source_config_sha256")
		v2_r2_sv1c_require_activation_arm_artifacts "$arm_dir" "$arm" "$expected_revision" "$arm_config_sha256" "$expected_binary_sha256" completed || return 1
		actual_sha256=$(sha256sum -- "$arm_dir/run-status.json" | awk '{print $1}') || return 1
		expected_sha256=$(jq -er --arg arm "$arm" '.[$arm + "_run_status_sha256"] | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
		[[ "$actual_sha256" == "$expected_sha256" ]] || return 1
		actual_sha256=$(sha256sum -- "$arm_dir/terminal-outcome.json" | awk '{print $1}') || return 1
		expected_sha256=$(jq -er --arg arm "$arm" '.[$arm + "_terminal_outcome_sha256"] | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
		[[ "$actual_sha256" == "$expected_sha256" ]] || return 1
		artifacts=$(jq -c --arg arm "$arm" '.[$arm + "_artifacts"]' "$provenance_path") || return 1
		v2_r2_sv1c_verify_artifact_records "$arm_dir" "$artifacts" || return 1
	done
	# Re-run the exact analyzer and require the retained invalid diagnostic to be
	# byte-identical. A nonzero analyzer result is expected here; it is still
	# provenance evidence rather than an activation verdict.
	replay_dir=$(mktemp -d) || return 1
	replay_path="$replay_dir/cdf-liquidity-comparison.json"
	if GOMAXPROCS="$v2_r2_sv1_activation_gomaxprocs" "$analyzer_path" -treatment "$treatment_dir" -control "$control_dir" >"$replay_path"; then
		replay_status=0
	else
		replay_status=$?
	fi
	if [[ "$replay_status" != "$expected_audit_status" ]] || ! cmp -s -- "$comparison_path" "$replay_path"; then
		rm -rf -- "$replay_dir"
		return 1
	fi
	rm -rf -- "$replay_dir"
}

v2_r2_sv1c_require_produced_activation_comparison() {
	[[ $# -eq 4 ]] || return 1
	local analyzer_path=$1 treatment_dir=$2 control_dir=$3 comparison_path=$4 replay_dir replay_path
	[[ "$analyzer_path" == /* && "$treatment_dir" == /* && "$control_dir" == /* && "$comparison_path" == /* ]] || return 1
	replay_dir=$(mktemp -d) || return 1
	replay_path="$replay_dir/cdf-liquidity-comparison.json"
	if ! GOMAXPROCS="${v2_r2_sv1_activation_gomaxprocs:-2}" "$analyzer_path" -treatment "$treatment_dir" -control "$control_dir" >"$replay_path"; then
		rm -rf -- "$replay_dir"
		return 1
	fi
	if ! cmp -s -- "$comparison_path" "$replay_path"; then
		rm -rf -- "$replay_dir"
		return 1
	fi
	rm -rf -- "$replay_dir"
}

v2_r2_sv1c_require_activation_comparison_identity() {
	[[ $# -eq 6 || $# -eq 7 ]] || return 1
	local comparison_path=$1 provenance_path=$2 expected_revision=$3 expected_binary_sha256=$4 expected_analyzer_sha256=$5 expected_supplier_count=$6
	local expected_activation=true
	[[ $# -eq 7 ]] && expected_activation=$7
	local treatment_config control_config treatment_config_sha256 control_config_sha256
	local treatment_experiment treatment_hypothesis control_experiment control_hypothesis
	local treatment_venue_ids control_venue_ids activation_venue_ids activation_seed activation_horizon
	local expected_supplier_pairs
	local activation_start_nano activation_end_nano activation_evidence_format activation_log_mode
	local activation_treatment_experiment activation_control_experiment
	local activation_treatment_hypothesis activation_control_hypothesis
	[[ "$expected_activation" == true || "$expected_activation" == false ]] || return 1
	[[ -s "$comparison_path" && ! -L "$comparison_path" ]] || return 1
	[[ -s "$provenance_path" && ! -L "$provenance_path" ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_binary_sha256" =~ ^[0-9a-f]{64}$ && "$expected_analyzer_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ "$expected_supplier_count" =~ ^[1-9][0-9]*$ ]] || return 1
	treatment_config=$(realpath -e -- "$v2_r2_sv1_activation_config") || return 1
	control_config=$(realpath -e -- "$v2_r2_sv1_activation_control_config") || return 1
	[[ -f "$treatment_config" && ! -L "$treatment_config" && -f "$control_config" && ! -L "$control_config" ]] || return 1
	treatment_config_sha256=$(sha256sum -- "$treatment_config" | awk '{print $1}') || return 1
	control_config_sha256=$(sha256sum -- "$control_config" | awk '{print $1}') || return 1
	treatment_experiment=$(jq -er '.experiment_id | select(type == "string" and length > 0)' "$treatment_config") || return 1
	treatment_hypothesis=$(jq -er '.hypothesis_id | select(type == "string" and length > 0)' "$treatment_config") || return 1
	control_experiment=$(jq -er '.experiment_id | select(type == "string" and length > 0)' "$control_config") || return 1
	control_hypothesis=$(jq -er '.hypothesis_id | select(type == "string" and length > 0)' "$control_config") || return 1
	treatment_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0 and all(.[]; type == "string" and length > 0))' "$treatment_config") || return 1
	control_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0 and all(.[]; type == "string" and length > 0))' "$control_config") || return 1
	[[ "$treatment_venue_ids" == "$control_venue_ids" ]] || return 1
	expected_supplier_pairs=$(jq -ce '
		if ((.venue_ids | type) == "array" and (.venue_ids | length) > 0 and
			all(.venue_ids[]; type == "string" and length > 0) and
			((.venue_ids | unique | length) == (.venue_ids | length)) and
			(.elastic_liquidity_suppliers | type) == "array" and
			(.elastic_liquidity_suppliers | length) > 0 and
			all(.elastic_liquidity_suppliers[]; (.role | type) == "string" and (.role | length) > 0) and
			(([.elastic_liquidity_suppliers[].role] | unique | length) == (.elastic_liquidity_suppliers | length))) then
			[.venue_ids[] as $venue | .elastic_liquidity_suppliers[].role as $role | {venue_id: $venue, role: $role}]
			| sort_by(.venue_id, .role)
		else error("invalid activation supplier roster") end' "$treatment_config") || return 1
	activation_seed=$(jq -er '.seed | select(type == "number" and floor == .)' "$provenance_path") || return 1
	activation_horizon=$(jq -er '.simulated_horizon | select(type == "string" and length > 0)' "$provenance_path") || return 1
	activation_start_nano=$(jq -er '.simulation_start_nano | select(type == "number" and floor == .)' "$provenance_path") || return 1
	activation_end_nano=$(jq -er '.simulation_end_nano | select(type == "number" and floor == .)' "$provenance_path") || return 1
	activation_evidence_format=$(jq -er '.evidence_format | select(type == "string" and length > 0)' "$provenance_path") || return 1
	activation_log_mode=$(jq -er '.log_mode | select(type == "string" and length > 0)' "$provenance_path") || return 1
	activation_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0 and all(.[]; type == "string" and length > 0))' "$provenance_path") || return 1
	activation_treatment_experiment=$(jq -er '.treatment_experiment_id | select(type == "string" and length > 0)' "$provenance_path") || return 1
	activation_control_experiment=$(jq -er '.control_experiment_id | select(type == "string" and length > 0)' "$provenance_path") || return 1
	activation_treatment_hypothesis=$(jq -er '.treatment_hypothesis_id | select(type == "string" and length > 0)' "$provenance_path") || return 1
	activation_control_hypothesis=$(jq -er '.control_hypothesis_id | select(type == "string" and length > 0)' "$provenance_path") || return 1
	[[ "$activation_seed" == "$v2_r2_sv1_activation_seed" && "$activation_horizon" == "$v2_r2_sv1_activation_horizon" &&
		"$activation_start_nano" == "$v2_r2_sv1_activation_simulation_start_nano" &&
		"$activation_end_nano" == "$v2_r2_sv1_activation_simulation_end_nano" &&
		"$activation_evidence_format" == "$v2_r2_sv1_activation_evidence_format" &&
		"$activation_log_mode" == "$v2_r2_sv1_activation_log_mode" &&
		"$activation_venue_ids" == "$treatment_venue_ids" &&
		"$activation_treatment_experiment" == "$treatment_experiment" &&
		"$activation_control_experiment" == "$control_experiment" &&
		"$activation_treatment_hypothesis" == "$treatment_hypothesis" &&
		"$activation_control_hypothesis" == "$control_hypothesis" ]] || return 1
	jq -e \
		--arg revision "$expected_revision" \
		--arg binary_sha256 "$expected_binary_sha256" \
		--arg analyzer_sha256 "$expected_analyzer_sha256" \
		--arg treatment_config_sha256 "$treatment_config_sha256" \
		--arg control_config_sha256 "$control_config_sha256" \
		--arg treatment_experiment "$treatment_experiment" --arg treatment_hypothesis "$treatment_hypothesis" \
		--arg control_experiment "$control_experiment" --arg control_hypothesis "$control_hypothesis" \
		--argjson seed "$activation_seed" --arg horizon "$activation_horizon" \
		--argjson start_nano "$activation_start_nano" --argjson end_nano "$activation_end_nano" \
			--argjson venue_ids "$treatment_venue_ids" --arg evidence_format "$activation_evidence_format" --arg log_mode "$activation_log_mode" \
			--argjson expected_supplier_pairs "$expected_supplier_pairs" \
			--argjson expected_activation "$expected_activation" \
		'
			type == "object" and
			(.valid | type) == "boolean" and .valid == true and
			(.evidence_valid | type) == "boolean" and .evidence_valid == true and
				(.activation_satisfied | type) == "boolean" and .activation_satisfied == $expected_activation and
			(.anti_cheating_satisfied | type) == "boolean" and .anti_cheating_satisfied == true and
			(.provenance | type) == "object" and .provenance.valid == true and
			(.provenance.treatment | type) == "object" and (.provenance.control | type) == "object" and
			(.provenance.analyzer_sha256 | type) == "string" and .provenance.analyzer_sha256 == $analyzer_sha256 and
			(.provenance.analyzer_source_revision | type) == "string" and .provenance.analyzer_source_revision == $revision and
			(.provenance.analyzer_source_modified | type) == "boolean" and .provenance.analyzer_source_modified == false and
			all([.provenance.treatment, .provenance.control][];
				.valid == true and .source_revision == $revision and .source_modified == false and
				.binary_sha256 == $binary_sha256 and .binary_goos == "linux" and .binary_goarch == "amd64" and .binary_goamd64 == "v1" and
				.seed == $seed and .horizon == $horizon and .simulation_start_nano == $start_nano and .simulation_end_nano == $end_nano and
				.venue_ids == $venue_ids and .evidence_format == $evidence_format and .log_mode == $log_mode) and
			.provenance.treatment.config_sha256 == $treatment_config_sha256 and
			.provenance.control.config_sha256 == $control_config_sha256 and
			.provenance.treatment.experiment_id == $treatment_experiment and
				.provenance.treatment.hypothesis_id == $treatment_hypothesis and
				.provenance.control.experiment_id == $control_experiment and
				.provenance.control.hypothesis_id == $control_hypothesis and
				(.treatment.venues | type) == "array" and
				all(.treatment.venues[]; (.venue_id | type) == "string") and
				([.treatment.venues[].venue_id] | sort) == ($venue_ids | sort) and
				(.control.venues | type) == "array" and
				all(.control.venues[]; (.venue_id | type) == "string") and
				([.control.venues[].venue_id] | sort) == ($venue_ids | sort) and
				(.treatment.suppliers | type) == "array" and
				all(.treatment.suppliers[]; (.venue_id | type) == "string" and (.role | type) == "string" and
					(.client_id | type) == "number" and .client_id == (.client_id | floor) and .client_id > 0) and
				([.treatment.suppliers[] | {venue_id, role}] | sort_by(.venue_id, .role)) == $expected_supplier_pairs and
				([.treatment.suppliers[] | {venue_id, client_id}] | unique_by([.venue_id, .client_id]) | length) == (.treatment.suppliers | length) and
				(.control.suppliers | type) == "array" and (.control.suppliers | length) == 0' "$comparison_path" >/dev/null || return 1
	if [[ "$expected_activation" == true ]]; then
		v2_r2_require_cdf_supplier_comparison "$comparison_path" "$expected_supplier_count"
	else
		v2_r2_require_cdf_supplier_comparison_measurement "$comparison_path" "$expected_supplier_count"
	fi
}

v2_r2_require_sv1c_activation_provenance_mode() {
	[[ $# -eq 5 ]] || return 1
	local provenance_path=$1 expected_revision=$2 expected_binary_sha256=$3 expected_status=$4 expected_activation=$5
	local output_root treatment_dir control_dir comparison_path review_path analyzer_path simulator_path renderer_path checkpoint_validator_path
	local expected_tree_sha256 actual_sha256 treatment_artifacts control_artifacts
	local expected_treatment_config expected_control_config treatment_config_path control_config_path
	local treatment_source_config_sha256 control_source_config_sha256
	local analyzer_sha256 renderer_sha256 checkpoint_validator_revision checkpoint_validator_sha256 expected_supplier_count arm_config_sha256
	local expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity
	[[ "$expected_status" == ACTIVATION_CONTRACT_SATISFIED || "$expected_status" == ACTIVATION_CONTRACT_NOT_SATISFIED ]] || return 1
	[[ "$expected_activation" == true || "$expected_activation" == false ]] || return 1
	[[ "$provenance_path" == /* && "$provenance_path" != */ && "$provenance_path" != *$'\n'* && "$provenance_path" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_binary_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -f "$provenance_path" && ! -L "$provenance_path" ]] || return 1
	[[ "$(realpath -e -- "$provenance_path")" == "$provenance_path" ]] || return 1
	v2_r2_require_single_json_object "$provenance_path" || return 1
	expected_tree_sha256=$(v2_r2_sv1c_git_tree_sha256 "$expected_revision") || return 1
	IFS=$'\t' read -r expected_host_cpu_count expected_allowed_cpu_count expected_cpu_affinity < <(v2_r2_sv1c_cpu_policy) || return 1
	output_root=$(jq -er '.output_root | select(type == "string")' "$provenance_path") || return 1
	treatment_dir=$(jq -er '.treatment_dir | select(type == "string")' "$provenance_path") || return 1
	control_dir=$(jq -er '.control_dir | select(type == "string")' "$provenance_path") || return 1
	comparison_path=$(jq -er '.comparison_path | select(type == "string")' "$provenance_path") || return 1
	review_path=$(jq -er '.review_attestation_path | select(type == "string")' "$provenance_path") || return 1
	analyzer_path=$(jq -er '.analyzer_binary_path | select(type == "string")' "$provenance_path") || return 1
	simulator_path=$(jq -er '.simulator_binary_path | select(type == "string")' "$provenance_path") || return 1
	renderer_path=$(jq -er '.renderer_binary_path | select(type == "string")' "$provenance_path") || return 1
	checkpoint_validator_path=$(jq -er '.checkpoint_validator_path | select(type == "string")' "$provenance_path") || return 1
	checkpoint_validator_revision=$(jq -er '.checkpoint_validator_revision | select(type == "string" and test("^[0-9a-f]{40}$"))' "$provenance_path") || return 1
	checkpoint_validator_sha256=$(jq -er '.checkpoint_validator_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	for path in "$output_root" "$treatment_dir" "$control_dir" "$comparison_path" "$review_path" "$analyzer_path" "$simulator_path" "$renderer_path" "$checkpoint_validator_path"; do
		[[ "$path" == /* && "$path" != */ && "$path" != *$'\n'* && "$path" != *$'\t'* ]] || return 1
	done
	[[ -d "$output_root" && ! -L "$output_root" && "$(realpath -e -- "$output_root")" == "$output_root" ]] || return 1
	[[ -d "$treatment_dir" && ! -L "$treatment_dir" && "$(realpath -e -- "$treatment_dir")" == "$treatment_dir" ]] || return 1
	[[ -d "$control_dir" && ! -L "$control_dir" && "$(realpath -e -- "$control_dir")" == "$control_dir" ]] || return 1
	[[ "$treatment_dir" == "$output_root/treatment" && "$control_dir" == "$output_root/control" ]] || return 1
	[[ "$treatment_dir" != "$control_dir" ]] || return 1
	[[ "$comparison_path" == "$output_root/cdf-liquidity-comparison.json" ]] || return 1
	[[ -f "$comparison_path" && ! -L "$comparison_path" && "$(realpath -e -- "$comparison_path")" == "$comparison_path" ]] || return 1
	analyzer_sha256=$(jq -er '.analyzer_binary_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	renderer_sha256=$(jq -er '.renderer_binary_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || return 1
	v2_r2_sv1c_require_pinned_binary "$simulator_path" "$expected_revision" "$expected_binary_sha256" "exchange_sim/cmd/multivenue" || return 1
	v2_r2_sv1c_require_pinned_binary "$analyzer_path" "$expected_revision" "$analyzer_sha256" "exchange_sim/cmd/cdf-liquidity-audit" || return 1
	v2_r2_sv1c_require_pinned_binary "$renderer_path" "$expected_revision" "$renderer_sha256" "exchange_sim/cmd/evsrender" || return 1
	[[ "$checkpoint_validator_path" == "$v2_r2_checkpoint_validator_path" &&
		"$checkpoint_validator_revision" == "$expected_revision" &&
		"$checkpoint_validator_sha256" == "$v2_r2_checkpoint_validator_sha256" ]] || return 1
	v2_r2_require_registered_checkpoint_validator || return 1
	v2_r2_require_sv1c_review_attestation "$review_path" "$expected_revision" || return 1
	[[ "$review_path" != "$output_root"/* && "$review_path" != "$root_dir"/* ]] || return 1
	actual_sha256=$(sha256sum -- "$review_path" | awk '{print $1}') || return 1
	[[ "$actual_sha256" == "$(jq -er '.review_attestation_sha256' "$provenance_path")" ]] || return 1
	actual_sha256=$(sha256sum -- "$comparison_path" | awk '{print $1}') || return 1
	[[ "$actual_sha256" == "$(jq -er '.comparison_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path")" ]] || return 1
	jq -e --arg contract "$v2_r2_sv1_activation_pair_contract" --arg revision "$expected_revision" \
		--arg tree_sha256 "$expected_tree_sha256" --argjson seed "$v2_r2_sv1_activation_seed" \
		--argjson expected_host_cpu_count "$expected_host_cpu_count" --argjson expected_allowed_cpu_count "$expected_allowed_cpu_count" \
		--argjson expected_cpu_limit_percent "$v2_r2_sv1_cpu_limit_percent" --arg expected_cpu_affinity "$expected_cpu_affinity" \
		--argjson expected_activation_gomaxprocs "$v2_r2_sv1_activation_gomaxprocs" \
		--argjson expected_memory_limit_bytes "$v2_r2_sv1_activation_memory_limit_bytes" \
		--argjson expected_gomemlimit_bytes "$v2_r2_sv1_activation_gomemlimit_bytes" \
		--argjson expected_minimum_free_bytes "$v2_r2_sv1_activation_minimum_free_bytes" \
			--arg binary_sha256 "$expected_binary_sha256" \
			--arg checkpoint_validator_path "$checkpoint_validator_path" --arg checkpoint_validator_revision "$checkpoint_validator_revision" --arg checkpoint_validator_sha256 "$checkpoint_validator_sha256" \
		--arg renderer_path "$renderer_path" --arg renderer_sha256 "$renderer_sha256" \
		--arg expected_horizon "$v2_r2_sv1_activation_horizon" \
		--arg expected_evidence_format "$v2_r2_sv1_activation_evidence_format" \
		--arg expected_log_mode "$v2_r2_sv1_activation_log_mode" \
			--argjson expected_start_nano "$v2_r2_sv1_activation_simulation_start_nano" \
			--argjson expected_end_nano "$v2_r2_sv1_activation_simulation_end_nano" \
			--arg expected_status "$expected_status" --argjson expected_activation "$expected_activation" '
		type == "object" and .schema_version == 3 and .contract == $contract and
		.candidate_revision == $revision and .candidate_tree_sha256 == $tree_sha256 and .seed == $seed and
		.simulated_horizon == $expected_horizon and .simulation_start_nano == $expected_start_nano and
		.simulation_end_nano == $expected_end_nano and .evidence_format == $expected_evidence_format and
		.log_mode == $expected_log_mode and
		(.venue_ids | type) == "array" and (.treatment_experiment_id | type) == "string" and
		(.control_experiment_id | type) == "string" and (.treatment_hypothesis_id | type) == "string" and
		(.control_hypothesis_id | type) == "string" and
			.status == $expected_status and .activation_satisfied == $expected_activation and
		.holdouts_consumed == false and .treatment_runner_status == 0 and .control_runner_status == 0 and
		.treatment_terminal_status == "completed" and .control_terminal_status == "completed" and
		.simulator_binary_sha256 == $binary_sha256 and
		.checkpoint_validator_path == $checkpoint_validator_path and .checkpoint_validator_revision == $checkpoint_validator_revision and
		.checkpoint_validator_sha256 == $checkpoint_validator_sha256 and
		(.analyzer_binary_sha256 | type) == "string" and (.analyzer_binary_sha256 | test("^[0-9a-f]{64}$")) and
		.renderer_binary_path == $renderer_path and .renderer_binary_sha256 == $renderer_sha256 and
		(.review_attestation_sha256 | type) == "string" and (.review_attestation_sha256 | test("^[0-9a-f]{64}$")) and
		(.comparison_sha256 | type) == "string" and (.comparison_sha256 | test("^[0-9a-f]{64}$")) and
		(.treatment_config_sha256 | type) == "string" and (.treatment_config_sha256 | test("^[0-9a-f]{64}$")) and
		(.control_config_sha256 | type) == "string" and (.control_config_sha256 | test("^[0-9a-f]{64}$")) and
		(.treatment_run_status_sha256 | type) == "string" and (.treatment_run_status_sha256 | test("^[0-9a-f]{64}$")) and
		(.control_run_status_sha256 | type) == "string" and (.control_run_status_sha256 | test("^[0-9a-f]{64}$")) and
		(.treatment_terminal_outcome_sha256 | type) == "string" and (.treatment_terminal_outcome_sha256 | test("^[0-9a-f]{64}$")) and
		(.control_terminal_outcome_sha256 | type) == "string" and (.control_terminal_outcome_sha256 | test("^[0-9a-f]{64}$")) and
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
	jq -e --argjson seed "$v2_r2_sv1_activation_seed" --arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		'.seed == $seed and .evidence_format == $evidence_format and .log_mode == $log_mode' "$treatment_config_path" >/dev/null || return 1
	jq -e --argjson seed "$v2_r2_sv1_activation_seed" --arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		'.seed == $seed and .evidence_format == $evidence_format and .log_mode == $log_mode' "$control_config_path" >/dev/null || return 1
	jq -e --arg treatment_config_sha256 "$treatment_source_config_sha256" \
		--arg control_config_sha256 "$control_source_config_sha256" \
		'.treatment_source_config_sha256 == $treatment_config_sha256 and .control_source_config_sha256 == $control_config_sha256' \
		"$provenance_path" >/dev/null || return 1
	expected_supplier_count=$(jq -er 'select((.elastic_liquidity_suppliers | type) == "array" and (.elastic_liquidity_suppliers | length) > 0 and (.venue_ids | type) == "array" and (.venue_ids | length) > 0) | (.elastic_liquidity_suppliers | length) * (.venue_ids | length)' "$treatment_config_path") || return 1
	v2_r2_sv1c_require_activation_comparison_identity "$comparison_path" "$provenance_path" "$expected_revision" "$expected_binary_sha256" "$analyzer_sha256" "$expected_supplier_count" "$expected_activation" || return 1
	for arm in treatment control; do
		arm_dir=$([[ "$arm" == treatment ]] && printf '%s' "$treatment_dir" || printf '%s' "$control_dir")
		arm_config_sha256=$([[ "$arm" == treatment ]] && printf '%s' "$treatment_source_config_sha256" || printf '%s' "$control_source_config_sha256")
		v2_r2_sv1c_require_activation_arm_artifacts "$arm_dir" "$arm" "$expected_revision" "$arm_config_sha256" "$expected_binary_sha256" || return 1
	done
	v2_r2_sv1c_require_produced_activation_comparison "$analyzer_path" "$treatment_dir" "$control_dir" "$comparison_path" || return 1
	treatment_artifacts=$(jq -c '.treatment_artifacts' "$provenance_path") || return 1
	control_artifacts=$(jq -c '.control_artifacts' "$provenance_path") || return 1
	v2_r2_sv1c_verify_artifact_records "$treatment_dir" "$treatment_artifacts" || return 1
	v2_r2_sv1c_verify_artifact_records "$control_dir" "$control_artifacts" || return 1
	[[ "$(sha256sum -- "$treatment_dir/run-config.json" | awk '{print $1}')" == "$treatment_source_config_sha256" ]] || return 1
	[[ "$(sha256sum -- "$control_dir/run-config.json" | awk '{print $1}')" == "$control_source_config_sha256" ]] || return 1
	[[ "$(sha256sum -- "$treatment_dir/run-config.json" | awk '{print $1}')" == "$(jq -er '.treatment_config_sha256' "$provenance_path")" ]] || return 1
	[[ "$(sha256sum -- "$control_dir/run-config.json" | awk '{print $1}')" == "$(jq -er '.control_config_sha256' "$provenance_path")" ]] || return 1
	[[ "$(sha256sum -- "$treatment_dir/run-status.json" | awk '{print $1}')" == "$(jq -er '.treatment_run_status_sha256' "$provenance_path")" ]] || return 1
	[[ "$(sha256sum -- "$control_dir/run-status.json" | awk '{print $1}')" == "$(jq -er '.control_run_status_sha256' "$provenance_path")" ]] || return 1
	[[ "$(sha256sum -- "$treatment_dir/terminal-outcome.json" | awk '{print $1}')" == "$(jq -er '.treatment_terminal_outcome_sha256' "$provenance_path")" ]] || return 1
	[[ "$(sha256sum -- "$control_dir/terminal-outcome.json" | awk '{print $1}')" == "$(jq -er '.control_terminal_outcome_sha256' "$provenance_path")" ]] || return 1
}

v2_r2_require_sv1c_activation_provenance() {
	[[ $# -eq 3 ]] || return 1
	v2_r2_require_sv1c_activation_provenance_mode "$1" "$2" "$3" "ACTIVATION_CONTRACT_SATISFIED" true
}

v2_r2_require_sv1c_activation_nonactivation_provenance() {
	[[ $# -eq 3 ]] || return 1
	v2_r2_require_sv1c_activation_provenance_mode "$1" "$2" "$3" "ACTIVATION_CONTRACT_NOT_SATISFIED" false
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
			printf '/home/vlad/v2-r2-sv1c-24h-binary-capacity-20260903-v4-capacity-seed-659-control-g4.json\n'
			;;
		4:treatment-643.json|4:treatment-647.json|4:treatment-653.json)
			printf '/home/vlad/v2-r2-sv1c-24h-binary-capacity-20260903-v4-capacity-seed-659-treatment-g4.json\n'
			;;
		8:treatment-643.json)
			printf '/home/vlad/v2-r2-sv1c-24h-binary-capacity-20260903-v4-capacity-seed-659-treatment-g8.json\n'
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

# Keep SV1C's exact Go toolchain requirement and all fail-closed activity
# predicates from the accepted SV1 contract.

# The shared SV1 runners still call the historical successor helper names.
# These adapters retain the runner interface while every identity, path, and
# validation implementation remains bound to the SV1C namespace above.
v2_r2_sv1b_cpu_policy() { v2_r2_sv1c_cpu_policy "$@"; }
v2_r2_sv1b_require_authorized_capacity_config() { v2_r2_sv1c_require_authorized_capacity_config "$@"; }
v2_r2_sv1b_require_pinned_binary() { v2_r2_sv1c_require_pinned_binary "$@"; }
v2_r2_sv1b_require_checkpoint_validator_attestation_binding() { v2_r2_sv1c_require_checkpoint_validator_attestation_binding "$@"; }
v2_r2_sv1b_git_tree_sha256() { v2_r2_sv1c_git_tree_sha256 "$@"; }
v2_r2_sv1b_review_attestation_path() { v2_r2_sv1c_review_attestation_path "$@"; }
v2_r2_require_sv1b_review_attestation() { v2_r2_require_sv1c_review_attestation "$@"; }
v2_r2_require_sv1b_activation_review_attestation() { v2_r2_require_sv1c_activation_review_attestation "$@"; }
v2_r2_require_sv1b_activation_provenance_mode() { v2_r2_require_sv1c_activation_provenance_mode "$@"; }
v2_r2_sv1b_activation_review_attestation_path() { v2_r2_sv1c_activation_review_attestation_path "$@"; }
v2_r2_sv1b_artifact_records() { v2_r2_sv1c_artifact_records "$@"; }
v2_r2_sv1b_verify_artifact_records() { v2_r2_sv1c_verify_artifact_records "$@"; }
v2_r2_sv1b_require_activation_arm_artifacts() { v2_r2_sv1c_require_activation_arm_artifacts "$@"; }
v2_r2_sv1b_require_activation_comparison_identity() { v2_r2_sv1c_require_activation_comparison_identity "$@"; }
v2_r2_sv1b_require_checkpoint_attestation_binding() { v2_r2_sv1c_require_checkpoint_attestation_binding "$@"; }
v2_r2_sv1b_require_invalid_audit_diagnostic() { v2_r2_sv1c_require_invalid_audit_diagnostic "$@"; }
v2_r2_sv1b_require_invalid_audit_pair_provenance() { v2_r2_sv1c_require_invalid_audit_pair_provenance "$@"; }
v2_r2_sv1b_require_terminal_failure_pair_provenance() { v2_r2_sv1c_require_terminal_failure_pair_provenance "$@"; }
v2_r2_sv1b_require_produced_activation_comparison() { v2_r2_sv1c_require_produced_activation_comparison "$@"; }
v2_r2_sv1b_require_terminal_arm_reconstruction() { v2_r2_sv1c_require_terminal_arm_reconstruction "$@"; }
v2_r2_require_sv1b_activation_provenance() { v2_r2_require_sv1c_activation_provenance "$@"; }
v2_r2_require_sv1b_activation_nonactivation_provenance() { v2_r2_require_sv1c_activation_nonactivation_provenance "$@"; }
