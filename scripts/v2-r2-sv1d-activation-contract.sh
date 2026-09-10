#!/usr/bin/env bash
# SV1D is an activation-only successor namespace. It reuses historical
# evidence primitives but owns its candidate, configs, review identity, and
# output root so a failed successor cannot rewrite an earlier result.
set -euo pipefail

source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"

v2_r2_output_root="/home/vlad/v2-r2-sv1d-activation-development-v1"
v2_r2_attestation_root="/home/vlad/v2-r2-sv1d-activation-development-v1-attestations"
v2_r2_namespace_lock_path="/home/vlad/v2-r2-sv1d-activation-development-v1.lock"
v2_r2_sv1_candidate_id="V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY"
v2_r2_sv1_require_candidate_metadata=true
v2_r2_sv1_require_generator_metadata=true
v2_r2_sv1_candidate_contract_version="v2-r2-sv1d-activation-candidate-v1"
v2_r2_sv1_generator_path="scripts/render-v2-r2-sv1d-activation-configs.sh"
v2_r2_sv1_contract_path="scripts/v2-r2-sv1d-activation-contract.sh"
v2_r2_sv1_contract_loader_path="scripts/v2-r2-sv1-contract-loader.sh"
v2_r2_sv1_config_normalizer_path="bin/multivenue"
v2_r2_sv1_config_normalizer_package="exchange_sim/cmd/multivenue"
v2_r2_sv1_config_normalizer_registration_path=""
v2_r2_sv1_contract_dependency_paths=(
	scripts/v2-r2-sv1-24h-contract.sh
	scripts/v2-integrated-longrun-r2-contract.sh
	scripts/v2-r2-sv1-terminal-outcome.jq
	scripts/v2-r2-sv1-activation-status.sh
)
v2_r2_sv1_preregistration_path="research/v2-r2-sv1d-one-sided-elastic-successor-preregistration-2026-09-09.md"
v2_r2_sv1_implementation_path="research/v2-r2-sv1d-implementation-2026-09-09.md"
v2_r2_sv1_scorer_contract="v2-r2-sv1d-activation-scorer-v1"
v2_r2_sv1_survival_contract="v2-r2-sv1d-activation-survival-v1"
v2_r2_sv1_paired_effect_contract="v2-r2-sv1d-activation-paired-effect-v1"
v2_r2_sv1_parity_contract="v2-r2-sv1d-activation-parity-v1"
v2_r2_sv1_predecessor_id="V2-R2-SV1C-STRICT-RISK-CDF-LIQUIDITY"
v2_r2_sv1_runner_contract="v2-r2-sv1d-activation-runner-v1"
v2_r2_sv1_require_terminal_outcome=true
v2_r2_sv1_completion_sentinels='["greeks.json", "latency.json", "terminal-outcome.json"]'
v2_r2_sv1_require_positive_loss_budget=true
v2_r2_sv1_require_no_replacement_withdrawal=false
v2_r2_sv1_experiment_prefix="v2-r2-sv1d-activation"
v2_r2_sv1_config_provenance_contract="v2-r2-sv1d-activation-config-provenance-v1"
v2_r2_sv1_config_dir="$root_dir/research/configs/v2-r2-sv1d-activation"
v2_r2_sv1_config_provenance_manifest="$root_dir/research/v2-r2-sv1d-activation-config-provenance.json"
v2_r2_sv1_seeds=(659)
v2_r2_sv1_parity_seed=659
v2_r2_sv1_source_config_names=(activation-643.json activation-643-control.json)
v2_r2_sv1_activation_config="$root_dir/research/configs/v2-r2-sv1d-activation/activation-659-treatment.json"
v2_r2_sv1_activation_control_config="$root_dir/research/configs/v2-r2-sv1d-activation/activation-659-mode-off.json"
v2_r2_sv1_activation_no_roster_config="$root_dir/research/configs/v2-r2-sv1d-activation/activation-659-no-roster.json"
v2_r2_sv1_activation_seed=659
v2_r2_sv1_run_hypothesis_id="V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY"
v2_r2_sv1_activation_hypothesis_prefix="V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY"
v2_r2_sv1_activation_contract="v2-r2-sv1d-activation-provenance-v1"
v2_r2_sv1_activation_pair_contract="v2-r2-sv1d-activation-tri-arm-v1"
v2_r2_sv1_activation_arm_status_contract="v2-r2-sv1d-activation-arm-status-v1"
v2_r2_sv1_activation_horizon="5m"
v2_r2_sv1_activation_simulation_start_nano=1735689600000000000
v2_r2_sv1_activation_simulation_end_nano=1735689900000000000
v2_r2_sv1_activation_evidence_format="evstream_v3"
v2_r2_sv1_activation_log_mode="full"
v2_r2_sv1_activation_output_prefix="v2-r2-sv1d-activation"
v2_r2_sv1_review_contract="v2-r2-sv1d-independent-review-v1"
v2_r2_sv1_activation_review_contract="v2-r2-sv1d-activation-review-v1"
v2_r2_sv1_config_checker_path="scripts/check-v2-r2-sv1d-activation-configs.sh"
v2_r2_sv1_config_contract_test_path="scripts/test-v2-r2-sv1d-activation-config-contract.sh"
v2_r2_sv1_activation_runner_path="scripts/run-v2-r2-sv1d-activation-probe.sh"
v2_r2_sv1d_capacity_runner_path="scripts/run-v2-r2-sv1d-24h-capacity-probe.sh"
v2_r2_sv1_activation_scorer_path="scripts/score-v2-r2-sv1d-activation.sh"
v2_r2_sv1_terminal_outcome_path="scripts/v2-r2-sv1-terminal-outcome.jq"
v2_r2_sv1_cpu_limit_percent=90
v2_r2_sv1_review_scope='["r2_calendar", "correctness_hardening", "lifecycle", "strict_risk", "account_scoped_liquidation", "binary_evidence", "cdf_supplier", "one_sided_local_book", "activation_protocol", "capacity_protocol", "resource_guards", "tri_arm_controls", "provenance_binding", "historical_boundary"]'
v2_r2_sv1_activation_review_scope='["activation_evidence", "cdf_activation", "one_sided_local_book", "tri_arm_controls", "binary_evidence", "lifecycle", "strict_risk", "account_scoped_liquidation", "resource_guards", "provenance_binding", "historical_boundary"]'
v2_r2_sv1_activation_gomaxprocs=2
v2_r2_sv1_activation_memory_limit_bytes=$((20 * 1024 * 1024 * 1024))
v2_r2_sv1_activation_gomemlimit_bytes=$((18 * 1024 * 1024 * 1024))
v2_r2_sv1_activation_minimum_free_bytes=$((4 * 1024 * 1024 * 1024))
v2_r2_sv1_activation_minimum_memory_available_bytes=$((4 * 1024 * 1024 * 1024))
v2_r2_sv1_activation_max_wall_seconds=900
v2_r2_sv1_activation_analyzer_max_wall_seconds=300
v2_r2_sv1d_activation_decision_interval_nano=2000000000

v2_r2_sv1d_capacity_attestation_contract="v2-r2-sv1d-synthetic-capacity-v2"
v2_r2_sv1d_capacity_workload_contract="v2-r2-sv1d-synthetic-capacity-workload-v1"
v2_r2_sv1d_capacity_workload_profile="sv1d-production-mix-v1"
v2_r2_sv1d_capacity_workload_seed=2026091001
v2_r2_sv1d_capacity_event_count=26100000
v2_r2_sv1d_capacity_book_delta_events=20880000
v2_r2_sv1d_capacity_balance_change_events=2610000
v2_r2_sv1d_capacity_opaque_events=2610000
v2_r2_sv1d_capacity_workload_start_nano=1735689600000000000
v2_r2_sv1d_capacity_workload_end_nano=1735776000000000000
v2_r2_sv1d_capacity_horizon="24h"
v2_r2_sv1d_capacity_config="$root_dir/research/configs/v2-r2-sv1d-activation/activation-659-treatment.json"
v2_r2_sv1d_capacity_binary="$root_dir/bin/evscapacity"
v2_r2_sv1d_capacity_binary_package="exchange_sim/cmd/evscapacity"
v2_r2_sv1d_capacity_gomaxprocs=2
v2_r2_sv1d_capacity_memory_limit_bytes=$((20 * 1024 * 1024 * 1024))
v2_r2_sv1d_capacity_gomemlimit_bytes=$((18 * 1024 * 1024 * 1024))
v2_r2_sv1d_capacity_minimum_free_bytes=$((4 * 1024 * 1024 * 1024))
v2_r2_sv1d_capacity_safety_margin_bytes=$((4 * 1024 * 1024 * 1024))
v2_r2_sv1d_capacity_max_wall_seconds=3600

v2_r2_sv1d_capacity_attestation_path() {
	local revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '/home/vlad/external-scratch/v2-r2-sv1d-synthetic-capacity-%s-attestation.json\n' "$revision"
}

v2_r2_sv1d_capacity_probe_root() {
	local revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '/home/vlad/external-scratch/v2-r2-sv1d-synthetic-capacity-%s\n' "$revision"
}

v2_r2_sv1d_capacity_probe_cell() {
	[[ $# -eq 0 ]] || return 1
	printf 'capacity-synthetic-production-mix-v1\n'
}

v2_r2_sv1d_capacity_free_bytes() {
	[[ $# -eq 1 && -d "$1" && ! -L "$1" ]] || return 1
	local available_bytes
	available_bytes=$(df -P -B1 -- "$1" | awk 'NR == 2 {print $4}') || return 1
	[[ "$available_bytes" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$available_bytes"
}

v2_r2_sv1d_capacity_memory_available_bytes() {
	local available_bytes
	available_bytes=$(awk '$1 == "MemAvailable:" {printf "%.0f\n", $2 * 1024; exit}' /proc/meminfo) || return 1
	[[ "$available_bytes" =~ ^[1-9][0-9]*$ ]] || return 1
	printf '%s\n' "$available_bytes"
}

v2_r2_sv1d_capacity_expected_cell_files() {
	printf '%s\n' \
		binary-evidence-attestation.json capacity.stderr.log capacity.stdout.log events.evs \
		evidence-manifest.json run-metadata.json synthetic-capacity-report.json workload-profile.json | LC_ALL=C sort
}

v2_r2_sv1d_capacity_require_root() {
	[[ $# -eq 2 ]] || return 1
	local probe_root=$1 probe_cell=$2 actual_directories actual_files
	[[ "$probe_root" == /* && "$probe_root" != */ && "$probe_root" != *$'\n'* && "$probe_root" != *$'\t'* ]] || return 1
	[[ "$probe_cell" != /* && "$probe_cell" != */ && "$probe_cell" != *$'\n'* && "$probe_cell" != *$'\t'* ]] || return 1
	[[ -d "$probe_root" && ! -L "$probe_root" && "$(realpath -e -- "$probe_root")" == "$probe_root" ]] || return 1
	if find "$probe_root" -mindepth 1 -maxdepth 1 \( -type l -o -type p -o -type s -o -type b -o -type c \) -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	actual_directories=$(find "$probe_root" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | LC_ALL=C sort)
	[[ "$actual_directories" == "$probe_cell" ]] || return 1
	actual_files=$(find "$probe_root" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)
	[[ -z "$actual_files" ]] || return 1
	[[ -d "$probe_root/$probe_cell" && ! -L "$probe_root/$probe_cell" &&
		"$(realpath -e -- "$probe_root/$probe_cell")" == "$probe_root/$probe_cell" ]] || return 1
}

v2_r2_sv1d_capacity_require_cell() {
	[[ $# -eq 1 ]] || return 1
	local cell=$1 expected_files actual_files actual_directories file_path
	[[ "$cell" == /* && "$cell" != */ && "$cell" != *$'\n'* && "$cell" != *$'\t'* ]] || return 1
	[[ -d "$cell" && ! -L "$cell" && "$(realpath -e -- "$cell")" == "$cell" ]] || return 1
	if find "$cell" -type l -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	if find "$cell" \( -type p -o -type s -o -type b -o -type c \) -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	actual_directories=$(find "$cell" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | LC_ALL=C sort)
	[[ -z "$actual_directories" ]] || return 1
	expected_files=$(v2_r2_sv1d_capacity_expected_cell_files)
	actual_files=$(find "$cell" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)
	[[ "$actual_files" == "$expected_files" ]] || return 1
	while IFS= read -r file_path; do
		[[ -f "$cell/$file_path" && ! -L "$cell/$file_path" ]] || return 1
		if [[ "$file_path" != capacity.stderr.log && "$file_path" != capacity.stdout.log ]]; then
			[[ -s "$cell/$file_path" ]] || return 1
		fi
	done < <(v2_r2_sv1d_capacity_expected_cell_files)
}

v2_r2_sv1d_capacity_verify_manifest() {
	[[ $# -eq 1 ]] || return 1
	local cell=$1 manifest="$1/evidence-manifest.json" expected listed relative expected_bytes expected_sha actual_bytes actual_sha
	[[ -s "$manifest" && ! -L "$manifest" ]] || return 1
	expected=$(v2_r2_sv1d_capacity_expected_cell_files | grep -v '^evidence-manifest.json$')
	listed=$(jq -r '.files[] | .path' "$manifest" | LC_ALL=C sort) || return 1
	[[ "$listed" == "$expected" ]] || return 1
	jq -e --arg cell "$(basename "$cell")" \
		'.schema_version == 1 and .contract == "v2-r2-sv1d-synthetic-capacity-evidence-manifest-v1" and
		 .cell == $cell and .outcome_neutral == true and .simulator_invoked == false and
		 .holdouts_consumed == false and (.files | type == "array") and
		 all(.files[]; (.path | type == "string") and (.bytes | type == "number" and floor == . and . >= 0) and
		   (.sha256 | type == "string" and test("^[0-9a-f]{64}$")))' "$manifest" >/dev/null || return 1
	while IFS=$'\t' read -r relative expected_bytes expected_sha; do
		[[ "$relative" != /* && "$relative" != *$'\n'* && "$relative" != *$'\t'* ]] || return 1
		actual_bytes=$(stat -c '%s' -- "$cell/$relative") || return 1
		actual_sha=$(sha256sum -- "$cell/$relative" | awk '{print $1}') || return 1
		[[ "$actual_bytes" == "$expected_bytes" && "$actual_sha" == "$expected_sha" ]] || return 1
	done < <(jq -r '.files[] | [.path, .bytes, .sha256] | @tsv' "$manifest")
}

v2_r2_sv1d_require_capacity_attestation_shape() {
	[[ $# -eq 1 ]] || return 1
	local attestation=$1 expected_host_memory_total expected_minimum_memory_available
	[[ -s "$attestation" && ! -L "$attestation" ]] || return 1
	expected_host_memory_total=$(jq -er '.resource_policy.host_memory_total_bytes | select(type == "number" and floor == . and . > 0)' "$attestation") || return 1
	expected_minimum_memory_available=$(v2_r2_sv1d_required_memory_available_bytes "$expected_host_memory_total") || return 1
	jq -e --arg contract "$v2_r2_sv1d_capacity_attestation_contract" \
		--arg profile "$v2_r2_sv1d_capacity_workload_profile" --argjson workload_seed "$v2_r2_sv1d_capacity_workload_seed" \
		--argjson event_count "$v2_r2_sv1d_capacity_event_count" --argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" \
		--argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" --argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" \
		--argjson start "$v2_r2_sv1d_capacity_workload_start_nano" --argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
		--argjson minimum_free "$v2_r2_sv1d_capacity_minimum_free_bytes" --argjson safety_margin "$v2_r2_sv1d_capacity_safety_margin_bytes" \
		--argjson minimum_memory_available "$expected_minimum_memory_available" --argjson gomaxprocs "$v2_r2_sv1d_capacity_gomaxprocs" \
		--argjson memory_limit "$v2_r2_sv1d_capacity_memory_limit_bytes" --argjson gomemlimit "$v2_r2_sv1d_capacity_gomemlimit_bytes" \
		--argjson cpu_limit "$v2_r2_sv1_cpu_limit_percent" --argjson max_wall "$v2_r2_sv1d_capacity_max_wall_seconds" '
		type == "object" and .schema_version == 1 and .contract == $contract and
		.measurement == "synthetic_24h_binary_evidence_capacity_probe" and .capacity_only == true and
		.outcome_neutral == true and .simulator_invoked == false and .terminal_outcome_present == false and
		.holdouts_consumed == false and .evidence_format == "evstream_v3" and
		.hashing == "route_and_global_sequence_neutral_v2" and .ordering == "ordered_stream" and
		(.source_revision | type == "string" and test("^[0-9a-f]{40}$")) and
		(.source_tree_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
		(.capacity_binary | type == "object" and (.path | type == "string" and startswith("/")) and
			(.sha256 | type == "string" and test("^[0-9a-f]{64}$"))) and
		(.target_config | type == "object" and (.path | type == "string" and length > 0) and
			(.sha256 | type == "string" and test("^[0-9a-f]{64}$"))) and
		(.review | type == "object" and (.path | type == "string" and startswith("/")) and
			(.sha256 | type == "string" and test("^[0-9a-f]{64}$"))) and
		.workload.profile == $profile and .workload.seed == $workload_seed and
		.workload.event_count == $event_count and .workload.book_delta_events == $book_events and
		.workload.balance_change_events == $balance_events and .workload.opaque_scientific_events == $opaque_events and
		.workload.start_nano == $start and .workload.end_nano == $end and .workload.horizon == "24h" and
		(.report_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
		(.profile_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
		(.evidence_manifest_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
		(.stream_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
		(.stream_bytes | type == "number" and floor == . and . > 0) and
		(.peak_output_bytes | type == "number" and floor == . and . > 0) and
		(.peak_rss_bytes | type == "number" and floor == . and . > 0) and
		(.safety_margin_bytes | type) == "number" and .safety_margin_bytes == $safety_margin and
		(.required_free_bytes | type) == "number" and .required_free_bytes == (.peak_output_bytes + .safety_margin_bytes) and
		(.available_free_bytes | type) == "number" and .available_free_bytes >= .required_free_bytes and
		(.initial_available_free_bytes | type) == "number" and .initial_available_free_bytes >= $minimum_free and
		(.minimum_free_bytes | type) == "number" and .minimum_free_bytes == $minimum_free and
		(.initial_memory_available_bytes | type) == "number" and .initial_memory_available_bytes >= $minimum_memory_available and
		(.final_memory_available_bytes | type) == "number" and .final_memory_available_bytes >= $minimum_memory_available and
		(.wall_clock_seconds | type == "number" and . >= 0 and . <= $max_wall) and
		(.resource_policy | type == "object" and .gomaxprocs == $gomaxprocs and
			.memory_limit_bytes == $memory_limit and .gomemlimit_bytes == $gomemlimit and
			.cpu_limit_percent == $cpu_limit and .minimum_free_bytes == $minimum_free and
			.minimum_memory_available_bytes == $minimum_memory_available and
			(.host_memory_total_bytes | type == "number" and floor == . and . > 0) and
			(.host_cpu_count | type == "number" and floor == . and . > 0) and
			(.allowed_cpu_count | type == "number" and floor == . and . > 0) and
			(.cpu_affinity | type == "string" and length > 0) and .max_wall_seconds == $max_wall)' \
		"$attestation" >/dev/null
}

v2_r2_sv1d_require_capacity_attestation() {
	[[ $# -eq 6 ]] || return 1
	local attestation=$1 expected_revision=$2 expected_capacity_binary_sha256=$3 expected_config_sha256=$4
	local expected_review_path=$5 expected_review_sha256=$6
	local expected_tree expected_config_path expected_config expected_review_actual_sha256
	local probe_root probe_cell capacity_binary_path report_sha profile_sha evidence_manifest_sha
	local actual_stream_sha actual_stream_bytes actual_stdout_sha actual_stderr_sha
	local expected_host_memory_total expected_minimum_memory_available
	local current_available_free_bytes current_memory_available_bytes
	local current_host_cpu_count current_allowed_cpu_count current_cpu_affinity
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_capacity_binary_sha256" =~ ^[0-9a-f]{64}$ &&
		"$expected_config_sha256" =~ ^[0-9a-f]{64}$ && "$expected_review_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -s "$attestation" && ! -L "$attestation" ]] || return 1
	v2_r2_require_single_json_object "$attestation" || return 1
	v2_r2_sv1d_require_capacity_attestation_shape "$attestation" || return 1
	expected_tree=$(v2_r2_sv1d_git_tree_sha256 "$expected_revision") || return 1
	expected_config_path="research/configs/v2-r2-sv1d-activation/activation-659-treatment.json"
	expected_config="$root_dir/$expected_config_path"
	[[ -f "$expected_config" && ! -L "$expected_config" && "$(realpath -e -- "$expected_config")" == "$expected_config" ]] || return 1
	[[ "$(v2_r2_sv1d_sha256_file "$expected_config")" == "$expected_config_sha256" ]] || return 1
	[[ "$expected_review_path" == /* && "$expected_review_path" != "$root_dir"/* && "$expected_review_path" != */ &&
		-f "$expected_review_path" && ! -L "$expected_review_path" && "$(realpath -e -- "$expected_review_path")" == "$expected_review_path" ]] || return 1
	expected_review_actual_sha256=$(v2_r2_sv1d_sha256_file "$expected_review_path") || return 1
	[[ "$expected_review_actual_sha256" == "$expected_review_sha256" ]] || return 1
	v2_r2_require_sv1b_review_attestation "$expected_review_path" "$expected_revision" || return 1

	capacity_binary_path=$(jq -er '.capacity_binary.path | select(type == "string" and startswith("/"))' "$attestation") || return 1
	[[ "$capacity_binary_path" == "$v2_r2_sv1d_capacity_binary" ]] || return 1
	v2_r2_sv1d_require_pinned_binary "$capacity_binary_path" "$expected_revision" \
		"$expected_capacity_binary_sha256" "$v2_r2_sv1d_capacity_binary_package" || return 1
	probe_root=$(jq -er '.probe_root | select(type == "string" and startswith("/"))' "$attestation") || return 1
	[[ "$probe_root" != "$root_dir" && "$probe_root" != "$root_dir"/* && "$probe_root" != *$'\n'* && "$probe_root" != *$'\t'* &&
		-d "$probe_root" && ! -L "$probe_root" && "$(realpath -e -- "$probe_root")" == "$probe_root" ]] || return 1
	[[ "$probe_root" == "$(v2_r2_sv1d_capacity_probe_root "$expected_revision")" ]] || return 1
	probe_cell=$(jq -er '.probe_cell | select(type == "string" and length > 0)' "$attestation") || return 1
	[[ "$probe_cell" == "$(v2_r2_sv1d_capacity_probe_cell)" && "$probe_cell" != */* && "$probe_cell" != *$'\n'* && "$probe_cell" != *$'\t'* ]] || return 1
	v2_r2_sv1d_capacity_require_root "$probe_root" "$probe_cell" || return 1
	v2_r2_sv1d_capacity_require_cell "$probe_root/$probe_cell" || return 1
	v2_r2_sv1d_capacity_verify_manifest "$probe_root/$probe_cell" || return 1

	report_sha=$(v2_r2_sv1d_sha256_file "$probe_root/$probe_cell/synthetic-capacity-report.json") || return 1
	profile_sha=$(v2_r2_sv1d_sha256_file "$probe_root/$probe_cell/workload-profile.json") || return 1
	evidence_manifest_sha=$(v2_r2_sv1d_sha256_file "$probe_root/$probe_cell/evidence-manifest.json") || return 1
	actual_stream_sha=$(v2_r2_sv1d_sha256_file "$probe_root/$probe_cell/events.evs") || return 1
	actual_stream_bytes=$(stat -c '%s' -- "$probe_root/$probe_cell/events.evs") || return 1
	actual_stdout_sha=$(v2_r2_sv1d_sha256_file "$probe_root/$probe_cell/capacity.stdout.log") || return 1
	actual_stderr_sha=$(v2_r2_sv1d_sha256_file "$probe_root/$probe_cell/capacity.stderr.log") || return 1
	[[ "$(jq -er '.report_sha256' "$attestation")" == "$report_sha" &&
		"$(jq -er '.profile_sha256' "$attestation")" == "$profile_sha" &&
		"$(jq -er '.evidence_manifest_sha256' "$attestation")" == "$evidence_manifest_sha" &&
		"$(jq -er '.stream_sha256' "$attestation")" == "$actual_stream_sha" &&
		"$(jq -er '.stream_bytes' "$attestation")" == "$actual_stream_bytes" &&
		"$(jq -er '.stdout_sha256' "$attestation")" == "$actual_stdout_sha" &&
		"$(jq -er '.stderr_sha256' "$attestation")" == "$actual_stderr_sha" ]] || return 1

	expected_host_memory_total=$(jq -er '.resource_policy.host_memory_total_bytes | select(type == "number" and floor == . and . > 0)' "$attestation") || return 1
	expected_minimum_memory_available=$(v2_r2_sv1d_required_memory_available_bytes "$expected_host_memory_total") || return 1
	IFS=$'\t' read -r current_host_cpu_count current_allowed_cpu_count current_cpu_affinity < <(v2_r2_sv1d_cpu_policy) || return 1
	current_available_free_bytes=$(v2_r2_sv1d_capacity_free_bytes "$probe_root") || return 1
	current_memory_available_bytes=$(v2_r2_sv1d_capacity_memory_available_bytes) || return 1
	jq -e --arg contract "$v2_r2_sv1d_capacity_attestation_contract" --arg revision "$expected_revision" \
		--arg tree "$expected_tree" --arg capacity_binary_sha256 "$expected_capacity_binary_sha256" \
		--arg capacity_binary_path "$v2_r2_sv1d_capacity_binary" \
		--arg config_path "$expected_config_path" --arg config_sha256 "$expected_config_sha256" \
		--arg review_path "$expected_review_path" --arg review_sha256 "$expected_review_sha256" \
		--arg profile "$v2_r2_sv1d_capacity_workload_profile" --argjson workload_seed "$v2_r2_sv1d_capacity_workload_seed" \
		--argjson event_count "$v2_r2_sv1d_capacity_event_count" \
		--argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" \
		--argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" \
		--argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" \
		--argjson start "$v2_r2_sv1d_capacity_workload_start_nano" --argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
		--argjson minimum_free "$v2_r2_sv1d_capacity_minimum_free_bytes" \
		--argjson safety_margin "$v2_r2_sv1d_capacity_safety_margin_bytes" \
		--argjson minimum_memory_available "$expected_minimum_memory_available" \
		--argjson gomaxprocs "$v2_r2_sv1d_capacity_gomaxprocs" \
		--argjson memory_limit "$v2_r2_sv1d_capacity_memory_limit_bytes" \
		--argjson gomemlimit "$v2_r2_sv1d_capacity_gomemlimit_bytes" \
		--argjson cpu_limit "$v2_r2_sv1_cpu_limit_percent" \
		--argjson expected_host_memory_total "$expected_host_memory_total" \
		--argjson host_cpu "$current_host_cpu_count" --argjson allowed_cpu "$current_allowed_cpu_count" --arg affinity "$current_cpu_affinity" \
		--argjson max_wall "$v2_r2_sv1d_capacity_max_wall_seconds" \
		--arg report_sha "$report_sha" --arg profile_sha "$profile_sha" --arg manifest_sha "$evidence_manifest_sha" \
		--argjson stream_bytes "$actual_stream_bytes" --arg stream_sha256 "$actual_stream_sha" \
		--argjson current_free "$current_available_free_bytes" --argjson current_memory "$current_memory_available_bytes" '
		type == "object" and .schema_version == 1 and .contract == $contract and
		.measurement == "synthetic_24h_binary_evidence_capacity_probe" and
		.capacity_only == true and .outcome_neutral == true and .simulator_invoked == false and
		.terminal_outcome_present == false and .holdouts_consumed == false and
		.source_revision == $revision and .source_tree_sha256 == $tree and
		.capacity_binary.path == $capacity_binary_path and .capacity_binary.sha256 == $capacity_binary_sha256 and
		.target_config.path == $config_path and .target_config.sha256 == $config_sha256 and
		.workload.profile == $profile and .workload.seed == $workload_seed and
		.workload.event_count == $event_count and .workload.book_delta_events == $book_events and
		.workload.balance_change_events == $balance_events and .workload.opaque_scientific_events == $opaque_events and
		.workload.start_nano == $start and .workload.end_nano == $end and .workload.horizon == "24h" and
		.review.path == $review_path and .review.sha256 == $review_sha256 and
		.report_sha256 == $report_sha and .profile_sha256 == $profile_sha and
		.evidence_manifest_sha256 == $manifest_sha and
		.stream_bytes == $stream_bytes and .stream_sha256 == $stream_sha256 and
		(.peak_output_bytes | type) == "number" and (.peak_output_bytes | floor) == .peak_output_bytes and .peak_output_bytes > 0 and
		(.stream_bytes | type) == "number" and (.stream_bytes | floor) == .stream_bytes and .stream_bytes > 0 and
		(.required_free_bytes | type) == "number" and .required_free_bytes == (.peak_output_bytes + .safety_margin_bytes) and
		(.safety_margin_bytes | type) == "number" and .safety_margin_bytes == $safety_margin and
		(.available_free_bytes | type) == "number" and .available_free_bytes >= .required_free_bytes and
		(.initial_available_free_bytes | type) == "number" and .initial_available_free_bytes >= $minimum_free and
		(.minimum_free_bytes | type) == "number" and .minimum_free_bytes == $minimum_free and
		(.initial_memory_available_bytes | type) == "number" and .initial_memory_available_bytes >= $minimum_memory_available and
		(.final_memory_available_bytes | type) == "number" and .final_memory_available_bytes >= $minimum_memory_available and
		(.resource_policy | type) == "object" and .resource_policy.gomaxprocs == $gomaxprocs and
		.resource_policy.memory_limit_bytes == $memory_limit and .resource_policy.gomemlimit_bytes == $gomemlimit and
		.resource_policy.cpu_limit_percent == $cpu_limit and .resource_policy.minimum_free_bytes == $minimum_free and
		.resource_policy.minimum_memory_available_bytes == $minimum_memory_available and
		.resource_policy.host_memory_total_bytes == $expected_host_memory_total and
		.resource_policy.host_cpu_count == $host_cpu and .resource_policy.allowed_cpu_count == $allowed_cpu and
		.resource_policy.cpu_affinity == $affinity and .resource_policy.max_wall_seconds == $max_wall and
		(.peak_rss_bytes | type) == "number" and .peak_rss_bytes > 0 and .peak_rss_bytes <= $memory_limit and
		.wall_clock_seconds >= 0 and .wall_clock_seconds <= $max_wall and
		$current_free >= .required_free_bytes and $current_memory >= .resource_policy.minimum_memory_available_bytes' \
		"$attestation" >/dev/null || return 1

	jq -e --arg contract "$v2_r2_sv1d_capacity_workload_contract" --arg profile "$v2_r2_sv1d_capacity_workload_profile" \
		--argjson seed "$v2_r2_sv1d_capacity_workload_seed" --argjson event_count "$v2_r2_sv1d_capacity_event_count" \
		--argjson start "$v2_r2_sv1d_capacity_workload_start_nano" --argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
		--argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" \
		--argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" \
		--argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" '
		.schema_version == 1 and .contract == $contract and .profile.name == $profile and
		.profile.workload_seed == $seed and .profile.event_count == $event_count and
		.profile.book_delta_events == $book_events and .profile.balance_change_events == $balance_events and
		.profile.opaque_scientific_events == $opaque_events and
		.profile.start_nano == $start and .profile.end_nano == $end' \
		"$probe_root/$probe_cell/workload-profile.json" >/dev/null || return 1

	jq -e --arg revision "$expected_revision" --arg config_path "$expected_config_path" --arg config_sha256 "$expected_config_sha256" \
		--arg profile "$v2_r2_sv1d_capacity_workload_profile" --argjson seed "$v2_r2_sv1d_capacity_workload_seed" \
		--argjson event_count "$v2_r2_sv1d_capacity_event_count" --argjson start "$v2_r2_sv1d_capacity_workload_start_nano" \
		--argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
		'.schema_version == 1 and .contract == "v2-r2-sv1d-synthetic-capacity-run-v1" and
		 .source_revision == $revision and .target_config_path == $config_path and
		 .target_config_sha256 == $config_sha256 and .workload_profile == $profile and
		 .workload_seed == $seed and .event_count == $event_count and
		 .workload_start_nano == $start and .workload_end_nano == $end and
		 .workload_horizon == "24h" and .capacity_only == true and .outcome_neutral == true and
		 .simulator_invoked == false and .terminal_outcome_present == false and .holdouts_consumed == false and
		 .evidence_format == "evstream_v3" and (.command | type == "array") and .command[0] == "evscapacity"' \
		"$probe_root/$probe_cell/run-metadata.json" >/dev/null || return 1

	jq -e --arg profile "$v2_r2_sv1d_capacity_workload_profile" --argjson seed "$v2_r2_sv1d_capacity_workload_seed" \
		--argjson event_count "$v2_r2_sv1d_capacity_event_count" --argjson start "$v2_r2_sv1d_capacity_workload_start_nano" \
		--argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
		--argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" \
		--argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" \
		--argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" '
		.schema_version == 1 and .contract == "v2-r2-sv1d-synthetic-capacity-workload-v1" and
		.profile.name == $profile and .profile.workload_seed == $seed and
		.profile.event_count == $event_count and .profile.book_delta_events == $book_events and
		.profile.balance_change_events == $balance_events and .profile.opaque_scientific_events == $opaque_events and
		.profile.start_nano == $start and .profile.end_nano == $end and
		.evidence_format == "evstream_v3" and .hashing == "route_and_global_sequence_neutral_v2" and
		.ordering == "ordered_stream" and .event_frames == $event_count and
		.family_counts == [$book_events, $balance_events, $opaque_events] and
		.unencodable_payloads == 0 and .readback_verified == true' \
		"$probe_root/$probe_cell/synthetic-capacity-report.json" >/dev/null || return 1

	jq -e '.schema_version == 1 and .contract == "v2-r2-sv1d-synthetic-capacity-evidence-v1" and
		.domain == "canonical_binary_execution_frames" and .ordering == "ordered_stream" and
		.hashing == "route_and_global_sequence_neutral_v2" and .outcome_neutral == true and
		.simulator_invoked == false and .holdouts_consumed == false and .readback_verified == true and
		.unencodable_payloads == 0' "$probe_root/$probe_cell/binary-evidence-attestation.json" >/dev/null || return 1
}
v2_r2_sv1d_calendar='[{"name":"short","listing_interval_nano":3600000000000,"time_to_expiry_nano":7200000000000},{"name":"medium","listing_interval_nano":10800000000000,"time_to_expiry_nano":21600000000000},{"name":"long","listing_interval_nano":21600000000000,"time_to_expiry_nano":43200000000000}]'
v2_r2_sv1d_arm_names=(treatment mode-off no-roster)

v2_r2_sv1d_host_memory_total_bytes() {
	awk '$1 == "MemTotal:" { printf "%.0f\n", $2 * 1024; exit }' /proc/meminfo
}

v2_r2_sv1d_required_memory_available_bytes() {
	[[ $# -eq 1 && "$1" =~ ^[1-9][0-9]*$ ]] || return 1
	local host_memory_total_bytes=$1 twenty_percent_bytes
	twenty_percent_bytes=$(( (host_memory_total_bytes + 4) / 5 ))
	if (( twenty_percent_bytes < 4 * 1024 * 1024 * 1024 )); then
		twenty_percent_bytes=$((4 * 1024 * 1024 * 1024))
	fi
	printf '%s\n' "$twenty_percent_bytes"
}

v2_r2_sv1d_activation_decision_budget() {
	local duration_nano=$((v2_r2_sv1_activation_simulation_end_nano - v2_r2_sv1_activation_simulation_start_nano))
	(( duration_nano > 0 )) || return 1
	printf '%s\n' "$(( (duration_nano + v2_r2_sv1d_activation_decision_interval_nano - 1) / v2_r2_sv1d_activation_decision_interval_nano ))"
}

v2_r2_sv1d_require_activation_capacity() {
	[[ $# -eq 1 && -s "$1" && ! -L "$1" ]] || return 1
	local config_path=$1 decision_budget
	decision_budget=$(v2_r2_sv1d_activation_decision_budget) || return 1
	jq -e --argjson decision_budget "$decision_budget" '
		(.elastic_liquidity_suppliers | type) == "array" and
		all(.elastic_liquidity_suppliers[];
			(.initial_base_balance | type) == "number" and (.initial_base_balance | floor) == .initial_base_balance and .initial_base_balance > 0 and
			(.initial_quote_balance | type) == "number" and (.initial_quote_balance | floor) == .initial_quote_balance and .initial_quote_balance > 0 and
			(.max_position | type) == "number" and (.max_position | floor) == .max_position and .max_position > 0 and
			(.max_inventory | type) == "number" and (.max_inventory | floor) == .max_inventory and .max_inventory >= .initial_base_balance and
			(.max_quote_qty | type) == "number" and (.max_quote_qty | floor) == .max_quote_qty and .max_quote_qty > 0 and
			(.max_loss_quote | type) == "number" and (.max_loss_quote | floor) == .max_loss_quote and .max_loss_quote > 0 and
			.max_position <= (.max_quote_qty * $decision_budget) and
			(.max_inventory - .initial_base_balance) <= (.max_quote_qty * $decision_budget))' \
		"$config_path" >/dev/null
}

v2_r2_is_successor_candidate() {
	[[ "${v2_r2_sv1_candidate_id:-}" == "V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY" ]]
}

v2_r2_require_known_candidate() {
	v2_r2_is_successor_candidate
}

v2_r2_is_go_127() {
	[[ "$1" == "go1.27.0" ]]
}

v2_r2_sv1d_config_for_arm() {
	[[ $# -eq 1 ]] || return 1
	case "$1" in
		treatment) printf '%s\n' "$v2_r2_sv1_activation_config" ;;
		mode-off) printf '%s\n' "$v2_r2_sv1_activation_control_config" ;;
		no-roster) printf '%s\n' "$v2_r2_sv1_activation_no_roster_config" ;;
		*) return 1 ;;
	esac
}

v2_r2_sv1d_arm_mode() {
	case "$1" in
		treatment) printf 'one_sided\n' ;;
		mode-off) printf 'two_sided_only\n' ;;
		no-roster) printf 'no_roster\n' ;;
		*) return 1 ;;
	esac
}

v2_r2_sv1d_review_attestation_path() {
	local revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '%s\n' "${V2_R2_SV1D_REVIEW_ATTESTATION:-/home/vlad/external-scratch/v2-r2-sv1d-review-${revision}/review-attestation.json}"
}

v2_r2_sv1d_activation_review_attestation_path() {
	local revision=${1:-$(git -C "$root_dir" rev-parse HEAD)}
	[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	printf '%s\n' "${V2_R2_SV1D_ACTIVATION_REVIEW_ATTESTATION:-/home/vlad/external-scratch/v2-r2-sv1d-activation-review-${v2_r2_sv1_activation_seed}-${revision}/review-attestation.json}"
}

v2_r2_sv1d_cpu_policy() {
	local host_cpu_count allowed_cpu_count
	host_cpu_count=$(nproc --all) || return 1
	[[ "$host_cpu_count" =~ ^[1-9][0-9]*$ ]] || return 1
	allowed_cpu_count=$((host_cpu_count * v2_r2_sv1_cpu_limit_percent / 100))
	(( allowed_cpu_count > 0 )) || allowed_cpu_count=1
	printf '%s\t%s\t0-%s\n' "$host_cpu_count" "$allowed_cpu_count" "$((allowed_cpu_count - 1))"
}

v2_r2_sv1d_sha256_file() {
	[[ $# -eq 1 && -f "$1" && ! -L "$1" ]] || return 1
	sha256sum -- "$1" | awk '$1 ~ /^[0-9a-f]{64}$/ {print $1; found=1} END {if (!found) exit 1}'
}

v2_r2_sv1d_binary_metadata_value() {
	[[ $# -eq 2 ]] || return 1
	local metadata=$1 key=$2
	awk -v key="$key" '
		BEGIN { prefix = key "="; count = 0; value = "" }
		$1 == "build" && index($2, prefix) == 1 { count++; value = substr($2, length(prefix) + 1) }
		END { if (count != 1 || value == "") exit 1; print value }' <<<"$metadata"
}

v2_r2_sv1d_require_pinned_binary() {
	[[ $# -eq 4 ]] || return 1
	local binary=$1 expected_revision=$2 expected_sha256=$3 expected_package=$4 metadata
	[[ "$binary" == /* && "$binary" != */ && "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -x "$binary" && ! -L "$binary" && "$(realpath -e -- "$binary")" == "$binary" ]] || return 1
	metadata=$(go version -m -- "$binary") || return 1
	local go_version package_path module_path buildmode compiler trimpath cgo_enabled goos goarch goamd64 vcs vcs_revision vcs_modified
	go_version=$(sed -n '1s/.*: //p' <<<"$metadata") || return 1
	package_path=$(awk '$1 == "path" {count++; value=$2} END {if (count != 1 || value == "") exit 1; print value}' <<<"$metadata") || return 1
	module_path=$(awk '$1 == "mod" {count++; value=$2} END {if (count != 1 || value == "") exit 1; print value}' <<<"$metadata") || return 1
	buildmode=$(v2_r2_sv1d_binary_metadata_value "$metadata" "-buildmode") || return 1
	compiler=$(v2_r2_sv1d_binary_metadata_value "$metadata" "-compiler") || return 1
	trimpath=$(v2_r2_sv1d_binary_metadata_value "$metadata" "-trimpath") || return 1
	cgo_enabled=$(v2_r2_sv1d_binary_metadata_value "$metadata" "CGO_ENABLED") || return 1
	goos=$(v2_r2_sv1d_binary_metadata_value "$metadata" "GOOS") || return 1
	goarch=$(v2_r2_sv1d_binary_metadata_value "$metadata" "GOARCH") || return 1
	goamd64=$(v2_r2_sv1d_binary_metadata_value "$metadata" "GOAMD64") || return 1
	vcs=$(v2_r2_sv1d_binary_metadata_value "$metadata" "vcs") || return 1
	vcs_revision=$(v2_r2_sv1d_binary_metadata_value "$metadata" "vcs.revision") || return 1
	vcs_modified=$(v2_r2_sv1d_binary_metadata_value "$metadata" "vcs.modified") || return 1
	[[ "$go_version" == "go1.27.0" && "$package_path" == "$expected_package" && "$module_path" == "exchange_sim" &&
		"$buildmode" == "exe" && "$compiler" == "gc" && "$trimpath" == "true" && "$cgo_enabled" == "0" &&
		"$goos" == "linux" && "$goarch" == "amd64" && "$goamd64" == "v1" && "$vcs" == "git" &&
		"$vcs_revision" == "$expected_revision" && "$vcs_modified" == "false" ]] || return 1
	[[ "$(v2_r2_sv1d_sha256_file "$binary")" == "$expected_sha256" ]]
}

v2_r2_sv1d_git_tree_sha256() {
	[[ $# -eq 1 && "$1" =~ ^[0-9a-f]{40}$ ]] || return 1
	git -C "$root_dir" ls-tree -r --full-tree "$1" | sha256sum | awk '{print $1}'
}

v2_r2_sv1b_cpu_policy() { v2_r2_sv1d_cpu_policy "$@"; }
v2_r2_sv1b_require_pinned_binary() { v2_r2_sv1d_require_pinned_binary "$@"; }
v2_r2_sv1b_git_tree_sha256() { v2_r2_sv1d_git_tree_sha256 "$@"; }
v2_r2_sv1b_review_attestation_path() { v2_r2_sv1d_review_attestation_path "$@"; }

v2_r2_require_sv1b_review_attestation() {
	[[ $# -eq 2 ]] || return 1
	local review_path=$1 expected_revision=$2 expected_tree report_path report_sha actual_report_sha
	[[ -f "$review_path" && ! -L "$review_path" && "$expected_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	v2_r2_require_single_json_object "$review_path" || return 1
	expected_tree=$(v2_r2_sv1d_git_tree_sha256 "$expected_revision") || return 1
	[[ "$(jq -er '.reviewed_tree_sha256' "$review_path")" == "$expected_tree" ]] || return 1
	report_path=$(jq -er '.review_report_path | select(type == "string" and length > 0)' "$review_path") || return 1
	report_sha=$(jq -er '.review_report_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$review_path") || return 1
	[[ "$report_path" == /* && "$report_path" != "$root_dir"/* && -f "$report_path" && ! -L "$report_path" ]] || return 1
	actual_report_sha=$(v2_r2_sv1d_sha256_file "$report_path") || return 1
	[[ "$actual_report_sha" == "$report_sha" ]] || return 1
	jq -e --arg contract "$v2_r2_sv1_review_contract" --arg revision "$expected_revision" --arg tree "$expected_tree" --arg report_sha "$report_sha" --argjson required_scope "$v2_r2_sv1_review_scope" '
		type == "object" and .schema_version == 1 and .contract == $contract and
		.reviewed_revision == $revision and .reviewed_tree_sha256 == $tree and
		.review_type == "independent_sol_xhigh" and .verdict == "ACCEPTED_FOR_ACTIVATION" and
		.reviewed_worktree_clean == true and .holdouts_consumed == false and
		(.reviewer | type == "string" and length > 0) and (.reviewed_scope | type == "array") and
		(($required_scope - .reviewed_scope) | length == 0) and .review_report_sha256 == $report_sha' "$review_path" >/dev/null
}

# The activation runner and scorer use the same closed artifact set. Keeping
# this predicate in the candidate contract prevents a producer and consumer
# from silently drifting on terminal-failure or receipt-file semantics.
v2_r2_sv1d_expected_arm_root_files() {
	printf '%s\n' \
		run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json checkpoints.jsonl \
		events.evs binary-evidence-attestation.json evidence-manifest.json evidence-only-artifact-hash.json terminal-outcome.json \
		market-data-evidence-v2.json market-data-schedules-v2.bin market-data-receipts-v2.bin \
		market-data-decisions-v2.bin market-data-actions-v2.bin simulator.stdout.log simulator.stderr.log | LC_ALL=C sort
}

v2_r2_sv1d_require_json_objects() {
	[[ $# -ge 1 ]] || return 1
	local arm_dir=$1 relative
	shift
	for relative in "$@"; do
		v2_r2_require_single_json_object "$arm_dir/$relative" || return 1
	done
}

v2_r2_sv1d_require_activation_arm_artifacts() {
	[[ $# -eq 5 ]] || return 1
	local arm_dir=$1 arm_name=$2 expected_revision=$3 expected_config_sha256=$4 expected_binary_sha256=$5
	local expected_config expected_venue_ids expected_experiment expected_hypothesis expected_root_files actual_root_files
	local expected_outcome terminal_outcome_status status_field file_path expected_hash actual_hash resource_policy_host_memory_total_bytes
	[[ "$arm_name" == treatment || "$arm_name" == mode-off || "$arm_name" == no-roster ]] || return 1
	[[ "$arm_dir" == /* && "$arm_dir" != */ && "$arm_dir" != *$'\n'* && "$arm_dir" != *$'\t'* ]] || return 1
	[[ "$expected_revision" =~ ^[0-9a-f]{40}$ && "$expected_config_sha256" =~ ^[0-9a-f]{64}$ && "$expected_binary_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	[[ -d "$arm_dir" && ! -L "$arm_dir" && "$(realpath -e -- "$arm_dir")" == "$arm_dir" ]] || return 1
	if find "$arm_dir" -type l -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	if find "$arm_dir" \( -type p -o -type s -o -type b -o -type c \) -print -quit 2>/dev/null | grep -q .; then
		return 1
	fi
	local actual_root_directories
	actual_root_directories=$(find "$arm_dir" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | LC_ALL=C sort)
	[[ "$actual_root_directories" == venues ]] || return 1
	expected_config=$(realpath -e -- "$(v2_r2_sv1d_config_for_arm "$arm_name")") || return 1
	expected_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length > 0)' "$expected_config") || return 1
	expected_experiment=$(jq -er '.experiment_id | select(type == "string" and length > 0)' "$expected_config") || return 1
	expected_hypothesis=$(jq -er '.hypothesis_id | select(type == "string" and length > 0)' "$expected_config") || return 1
	resource_policy_host_memory_total_bytes=$(jq -er '.resource_policy.host_memory_total_bytes | select(type == "number" and floor == . and . > 0)' "$arm_dir/run-metadata.json" 2>/dev/null || printf '0')
	[[ -f "$arm_dir/run-config.json" && ! -L "$arm_dir/run-config.json" &&
		"$(v2_r2_sv1d_sha256_file "$arm_dir/run-config.json")" == "$expected_config_sha256" ]] || return 1
	v2_r2_sv1d_require_json_objects "$arm_dir" \
		run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json \
		binary-evidence-attestation.json evidence-manifest.json evidence-only-artifact-hash.json terminal-outcome.json market-data-evidence-v2.json || return 1
	expected_root_files=$(v2_r2_sv1d_expected_arm_root_files)
	actual_root_files=$(find "$arm_dir" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)
	[[ "$actual_root_files" == "$expected_root_files" ]] || return 1
	while IFS= read -r file_path; do
		[[ -f "$arm_dir/$file_path" && ! -L "$arm_dir/$file_path" ]] || return 1
		if [[ "$file_path" != market-data-*.bin && "$file_path" != simulator.stdout.log && "$file_path" != simulator.stderr.log ]]; then
			[[ -s "$arm_dir/$file_path" ]] || return 1
		fi
	done < <(v2_r2_sv1d_expected_arm_root_files)
	v2_r2_verify_evidence_manifest "$arm_dir" || return 1
	jq -e --arg revision "$expected_revision" --argjson seed "$v2_r2_sv1_activation_seed" \
		--argjson venue_ids "$expected_venue_ids" --arg experiment "$expected_experiment" --arg hypothesis "$expected_hypothesis" \
		--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		--arg contract "$v2_r2_sv1_activation_contract" --arg arm "$arm_name" --arg mode "$(v2_r2_sv1d_arm_mode "$arm_name")" \
		--arg cell "${v2_r2_sv1_activation_output_prefix}-${v2_r2_sv1_activation_seed}-${arm_name}" \
		--argjson start "$v2_r2_sv1_activation_simulation_start_nano" --argjson end "$v2_r2_sv1_activation_simulation_end_nano" \
		--arg expected_config_sha256 "$expected_config_sha256" --arg expected_binary_sha256 "$expected_binary_sha256" \
		--argjson resource_policy_host_memory_total_bytes "$resource_policy_host_memory_total_bytes" '
		type == "object" and .schema_version == 1 and .contract == $contract and .arm == $arm and .mode == $mode and
			.cell == $cell and .seed == $seed and .simulated_horizon == "5m" and
			.simulation_start_nano == $start and .simulation_end_nano == $end and
			.config_sha256 == $expected_config_sha256 and .binary_sha256 == $expected_binary_sha256 and
			.git_revision == $revision and .config_experiment_id == $experiment and .hypothesis_id == $hypothesis and
			.evidence_format == $evidence_format and .log_mode == $log_mode and .venue_ids == $venue_ids and
			.binary_go_version == "go1.27.0" and .binary_goos == "linux" and .binary_goarch == "amd64" and .binary_goamd64 == "v1" and
			(.binary_path | type == "string" and startswith("/")) and
			(.checkpoint_validator_path | type == "string" and startswith("/")) and
			(.checkpoint_validator_revision == $revision) and
			(.checkpoint_validator_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
			(.review_attestation_path | type == "string" and startswith("/")) and
			(.review_attestation_sha256 | type == "string" and test("^[0-9a-f]{64}$")) and
			.resource_policy.gomaxprocs == 2 and .resource_policy.memory_limit_bytes == (20 * 1024 * 1024 * 1024) and
			.resource_policy.gomemlimit_bytes == (18 * 1024 * 1024 * 1024) and .resource_policy.cpu_limit_percent == 90 and
			.resource_policy.minimum_free_bytes == (4 * 1024 * 1024 * 1024) and
			(.resource_policy.host_memory_total_bytes | type) == "number" and .resource_policy.host_memory_total_bytes > 0 and
			(.resource_policy.minimum_memory_available_bytes | type) == "number" and
			.resource_policy.minimum_memory_available_bytes == (if (($resource_policy_host_memory_total_bytes + 4) / 5) < (4 * 1024 * 1024 * 1024) then (4 * 1024 * 1024 * 1024) else (($resource_policy_host_memory_total_bytes + 4) / 5 | floor) end) and
			.resource_policy.max_wall_seconds == 900 and .resource_policy.analyzer_max_wall_seconds == 300 and
			(.command == ["multivenue", "-config", "run-config.json", "-duration", "5m", "-logdir", ".", "-log-mode", $log_mode, "-evidence-format", $evidence_format])' \
		"$arm_dir/run-metadata.json" >/dev/null || return 1
	jq -e --arg revision "$expected_revision" --argjson seed "$v2_r2_sv1_activation_seed" \
		--argjson venue_ids "$expected_venue_ids" --arg experiment "$expected_experiment" --arg hypothesis "$expected_hypothesis" \
		--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		'.schema_version == 2 and .build.revision == $revision and .build.modified == false and
		 .build.goos == "linux" and .build.goarch == "amd64" and .build.goamd64 == "v1" and
		 .venue_ids == $venue_ids and .config.seed == $seed and .config.experiment_id == $experiment and
		 .config.hypothesis_id == $hypothesis and .config.log_mode == $log_mode and
		 .config.evidence_format == $evidence_format and .config.record_market_data_receipts == true and
		 .config.strict_risk_contract == true and .config.auto_borrow_spot == false and
		 .config.cross_asset_spot_graph == true and .config.cross_asset_collateral_marks == false' \
		"$arm_dir/manifest.json" >/dev/null || return 1
	[[ "$(jq -cS '.' "$arm_dir/run-config.json")" == "$(jq -cS '.config' "$arm_dir/manifest.json")" ]] || return 1
	jq -e --argjson start "$v2_r2_sv1_activation_simulation_start_nano" --argjson end "$v2_r2_sv1_activation_simulation_end_nano" \
		-f "$root_dir/scripts/v2-r2-sv1-terminal-outcome.jq" "$arm_dir/terminal-outcome.json" >/dev/null || return 1
	terminal_outcome_status=$(jq -er '.status' "$arm_dir/terminal-outcome.json") || return 1
	case "$terminal_outcome_status" in
		completed) expected_outcome=completed ;;
		terminal_failure) expected_outcome=terminal_failure ;;
		*) return 1 ;;
	esac
	if [[ "$expected_outcome" == completed ]]; then
		jq -e --arg arm "$arm_name" --arg contract "$v2_r2_sv1_activation_arm_status_contract" '
			.schema_version == 2 and .contract == $contract and .arm == $arm and .exit_status == 0 and
			.completion_verified == true and .terminal_failure_verified == false and
			.terminal_outcome_status == "completed" and .resource_guard_failed == false and
			(.wall_clock_seconds | type == "number" and floor == . and . <= 900)' \
			"$arm_dir/run-status.json" >/dev/null || return 1
		v2_r2_terminal_completed_outcome_present "$arm_dir" || return 1
	else
		jq -e --arg arm "$arm_name" --arg contract "$v2_r2_sv1_activation_arm_status_contract" '
			.schema_version == 2 and .contract == $contract and .arm == $arm and
			(.exit_status | type == "number" and floor == . and . > 0 and . <= 255) and
			.completion_verified == false and .terminal_failure_verified == true and
			.terminal_outcome_status == "terminal_failure" and .resource_guard_failed == false and
			(.wall_clock_seconds | type == "number" and floor == . and . <= 900)' \
			"$arm_dir/run-status.json" >/dev/null || return 1
		v2_r2_terminal_failure_outcome_present "$arm_dir" || return 1
	fi
	while IFS=$'\t' read -r status_field file_path; do
		expected_hash=$(jq -er --arg field "$status_field" '.[$field] | select(type == "string" and test("^[0-9a-f]{64}$"))' "$arm_dir/run-status.json") || return 1
		actual_hash=$(v2_r2_sv1d_sha256_file "$arm_dir/$file_path") || return 1
		[[ "$actual_hash" == "$expected_hash" ]] || return 1
	done < <(printf '%s\n' \
		$'terminal_outcome_sha256\tterminal-outcome.json' \
		$'run_metadata_sha256\trun-metadata.json' \
		$'manifest_sha256\tmanifest.json' \
		$'greeks_sha256\tgreeks.json' \
		$'latency_sha256\tlatency.json' \
		$'checkpoints_sha256\tcheckpoints.jsonl' \
		$'binary_attestation_sha256\tbinary-evidence-attestation.json' \
		$'evidence_manifest_sha256\tevidence-manifest.json' \
		$'simulator_stdout_sha256\tsimulator.stdout.log' \
		$'simulator_stderr_sha256\tsimulator.stderr.log')
	v2_r2_require_binary_checkpoint_stream_exact "$arm_dir/checkpoints.jsonl" \
		"$v2_r2_sv1_activation_simulation_start_nano" "$v2_r2_sv1_activation_simulation_end_nano" \
		"$arm_dir/binary-evidence-attestation.json" || return 1
	jq -e '
		type == "object" and .domain == "canonical_binary_execution_frames" and .ordering == "ordered_stream" and
		.hashing == "route_and_global_sequence_neutral_v2" and (.event_frames | type) == "number" and
		(.event_frames | floor) == .event_frames and .event_frames > 0 and (.stream_frames | type) == "number" and
		(.stream_frames | floor) == .stream_frames and .stream_frames >= .event_frames and
		(.execution_stream_hash | type) == "string" and (.execution_stream_hash | test("^[0-9a-f]{64}$")) and
		(.canonical_execution_stream_hash | type) == "string" and (.canonical_execution_stream_hash | test("^[0-9a-f]{64}$")) and
		(.persisted_event_records | type) == "number" and (.persisted_event_records | floor) == .persisted_event_records and
		.persisted_event_records >= 0 and (.final_global_sequence | type) == "number" and
		(.final_global_sequence | floor) == .final_global_sequence and .final_global_sequence >= 0 and
		.final_global_sequence == (.event_frames + .persisted_event_records) and
		((.unencodable_payloads // 0) | type) == "number" and (((.unencodable_payloads // 0) | floor) == (.unencodable_payloads // 0)) and
		((.unencodable_payloads // 0) == 0)' "$arm_dir/binary-evidence-attestation.json" >/dev/null || return 1
}

v2_r2_sv1d_require_arm_record_matches() {
	[[ $# -eq 3 ]] || return 1
	local provenance_path=$1 arm=$2 arm_dir=$3 arm_record
	[[ -s "$provenance_path" && ! -L "$provenance_path" && -d "$arm_dir" && ! -L "$arm_dir" ]] || return 1
	arm_record=$(jq -ce --arg arm "$arm" '.arms[$arm] | select(type == "object")' "$provenance_path") || return 1
	local config_path config_sha256 status_sha256 terminal_sha256 metadata_sha256 manifest_sha256 attestation_sha256 evidence_manifest_sha256 stdout_sha256 stderr_sha256
	config_path=$(v2_r2_sv1d_config_for_arm "$arm") || return 1
	config_sha256=$(v2_r2_sv1d_sha256_file "$config_path") || return 1
	status_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/run-status.json") || return 1
	terminal_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/terminal-outcome.json") || return 1
	metadata_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/run-metadata.json") || return 1
	manifest_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/manifest.json") || return 1
	attestation_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/binary-evidence-attestation.json") || return 1
	evidence_manifest_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/evidence-manifest.json") || return 1
	stdout_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/simulator.stdout.log") || return 1
	stderr_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/simulator.stderr.log") || return 1
	local exit_status terminal_status
	exit_status=$(jq -er '.exit_status | select(type == "number" and floor == .)' "$arm_dir/run-status.json") || return 1
	terminal_status=$(jq -er '.status | select(type == "string")' "$arm_dir/terminal-outcome.json") || return 1
	jq -e --arg path "$arm_dir" --arg config_path "$config_path" --arg mode "$(v2_r2_sv1d_arm_mode "$arm")" \
		--arg config_sha256 "$config_sha256" --argjson exit_status "$exit_status" --arg terminal_status "$terminal_status" \
		--arg status_sha256 "$status_sha256" --arg terminal_sha256 "$terminal_sha256" --arg metadata_sha256 "$metadata_sha256" \
		--arg manifest_sha256 "$manifest_sha256" --arg attestation_sha256 "$attestation_sha256" \
		--arg evidence_manifest_sha256 "$evidence_manifest_sha256" --arg stdout_sha256 "$stdout_sha256" --arg stderr_sha256 "$stderr_sha256" \
		'.path == $path and .config_path == $config_path and .mode == $mode and .config_sha256 == $config_sha256 and
			.exit_status == $exit_status and .terminal_status == $terminal_status and .valid == true and
			.run_status_sha256 == $status_sha256 and .terminal_outcome_sha256 == $terminal_sha256 and
			.run_metadata_sha256 == $metadata_sha256 and .manifest_sha256 == $manifest_sha256 and
			.binary_attestation_sha256 == $attestation_sha256 and .evidence_manifest_sha256 == $evidence_manifest_sha256 and
			.simulator_stdout_sha256 == $stdout_sha256 and .simulator_stderr_sha256 == $stderr_sha256' <<<"$arm_record" >/dev/null
}

v2_r2_sv1d_require_no_roster_diagnostic() {
	[[ $# -eq 3 ]] || return 1
	local diagnostic_path=$1 arm_dir=$2 expected_config_sha256=$3
	[[ "$diagnostic_path" == /* && "$diagnostic_path" != */ && ! -L "$diagnostic_path" && -f "$diagnostic_path" ]] || return 1
	v2_r2_require_single_json_object "$diagnostic_path" || return 1
	local status_sha256 terminal_sha256 metadata_sha256 manifest_sha256 attestation_sha256 evidence_manifest_sha256 stdout_sha256 stderr_sha256 greeks_sha256
	status_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/run-status.json") || return 1
	terminal_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/terminal-outcome.json") || return 1
	metadata_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/run-metadata.json") || return 1
	manifest_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/manifest.json") || return 1
	greeks_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/greeks.json") || return 1
	attestation_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/binary-evidence-attestation.json") || return 1
	evidence_manifest_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/evidence-manifest.json") || return 1
	stdout_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/simulator.stdout.log") || return 1
	stderr_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/simulator.stderr.log") || return 1
	local runtime_cdf_supplier_count runtime_cdf_decision_count runtime_cdf_fill_count
	runtime_cdf_supplier_count=$(v2_r2_sv1d_runtime_cdf_supplier_count "$arm_dir") || return 1
	runtime_cdf_decision_count=$(v2_r2_sv1d_runtime_cdf_event_count "$arm_dir" '"event":"elastic_liquidity_supplier_decision"') || return 1
	runtime_cdf_fill_count=$(v2_r2_sv1d_runtime_cdf_event_count "$arm_dir" '"event":"elastic_liquidity_supplier_fill"') || return 1
	jq -e --arg contract "v2-r2-sv1d-no-roster-diagnostic-v1" --arg config_sha256 "$expected_config_sha256" \
		--arg status_sha256 "$status_sha256" --arg terminal_sha256 "$terminal_sha256" --arg metadata_sha256 "$metadata_sha256" \
		--arg greeks_sha256 "$greeks_sha256" --arg manifest_sha256 "$manifest_sha256" --arg attestation_sha256 "$attestation_sha256" \
		--arg evidence_manifest_sha256 "$evidence_manifest_sha256" --arg stdout_sha256 "$stdout_sha256" --arg stderr_sha256 "$stderr_sha256" \
		--arg terminal_status "$(jq -er '.status' "$arm_dir/terminal-outcome.json")" \
		--argjson runtime_cdf_supplier_count "$runtime_cdf_supplier_count" --argjson runtime_cdf_decision_count "$runtime_cdf_decision_count" \
		--argjson runtime_cdf_fill_count "$runtime_cdf_fill_count" \
		' type == "object" and .schema_version == 1 and .contract == $contract and .arm == "no-roster" and
			.config_sha256 == $config_sha256 and .run_status_sha256 == $status_sha256 and
			.terminal_outcome_sha256 == $terminal_sha256 and .run_metadata_sha256 == $metadata_sha256 and
			.greeks_sha256 == $greeks_sha256 and .manifest_sha256 == $manifest_sha256 and .binary_attestation_sha256 == $attestation_sha256 and
			.evidence_manifest_sha256 == $evidence_manifest_sha256 and .simulator_stdout_sha256 == $stdout_sha256 and
			.simulator_stderr_sha256 == $stderr_sha256 and .terminal_status == $terminal_status and
			($terminal_status == "completed" or $terminal_status == "terminal_failure") and
			.strict_population_accounting == true and .cdf_roster == false and .cdf_metrics == "out_of_scope" and
			.runtime_topology_sha256 == $greeks_sha256 and .runtime_topology_valid == true and
			.runtime_cdf_supplier_count == $runtime_cdf_supplier_count and .runtime_cdf_supplier_count == 0 and
			.runtime_cdf_decision_count == $runtime_cdf_decision_count and .runtime_cdf_decision_count == 0 and
			.runtime_cdf_fill_count == $runtime_cdf_fill_count and .runtime_cdf_fill_count == 0 and
			.status == "VALID_TOPOLOGY_CONTROL" and .holdouts_consumed == false' "$diagnostic_path" >/dev/null
}

v2_r2_sv1d_runtime_cdf_supplier_count() {
	[[ $# -eq 1 && -d "$1" && ! -L "$1" && -f "$1/greeks.json" && ! -L "$1/greeks.json" ]] || return 1
	jq -er '[.initial_accounts // [] | .[] | select((.role // "") | test("^cdf_elastic_supplier_[0-9]+$"))] | length' "$1/greeks.json"
}

v2_r2_sv1d_runtime_cdf_event_count() {
	[[ $# -eq 2 && -d "$1" && ! -L "$1" ]] || return 1
	local arm_dir=$1 event_pattern=$2 path match_count total=0
	while IFS= read -r -d '' path; do
		match_count=$( { rg --text --only-matching -- "$event_pattern" "$path" || true; } | wc -l )
		total=$((total + match_count))
	done < <(find "$arm_dir/venues" -type f -name '*.jsonl' -print0 2>/dev/null)
	printf '%s\n' "$total"
}

v2_r2_sv1d_require_comparison_provenance() {
	[[ $# -eq 5 ]] || return 1
	local comparison_path=$1 expected_analyzer_sha256=$2 expected_revision=$3 treatment_dir=$4 control_dir=$5
	[[ "$expected_analyzer_sha256" =~ ^[0-9a-f]{64}$ && "$expected_revision" =~ ^[0-9a-f]{40}$ ]] || return 1
	v2_r2_require_single_json_object "$comparison_path" || return 1
	if jq -e '.status == "UNAVAILABLE_TERMINAL_FAILURE"' "$comparison_path" >/dev/null 2>&1; then
		local treatment_terminal_status control_terminal_status treatment_status_sha256 control_status_sha256
		local treatment_terminal_sha256 control_terminal_sha256
		treatment_terminal_status=$(jq -er '.status' "$treatment_dir/terminal-outcome.json") || return 1
		control_terminal_status=$(jq -er '.status' "$control_dir/terminal-outcome.json") || return 1
		treatment_status_sha256=$(v2_r2_sv1d_sha256_file "$treatment_dir/run-status.json") || return 1
		control_status_sha256=$(v2_r2_sv1d_sha256_file "$control_dir/run-status.json") || return 1
		treatment_terminal_sha256=$(v2_r2_sv1d_sha256_file "$treatment_dir/terminal-outcome.json") || return 1
		control_terminal_sha256=$(v2_r2_sv1d_sha256_file "$control_dir/terminal-outcome.json") || return 1
		jq -e --arg contract "$v2_r2_sv1_activation_contract" --argjson seed "$v2_r2_sv1_activation_seed" \
			--arg horizon "$v2_r2_sv1_activation_horizon" --argjson start "$v2_r2_sv1_activation_simulation_start_nano" \
			--argjson end "$v2_r2_sv1_activation_simulation_end_nano" --arg treatment_status "$treatment_terminal_status" \
			--arg control_status "$control_terminal_status" --arg treatment_status_sha256 "$treatment_status_sha256" \
			--arg control_status_sha256 "$control_status_sha256" --arg treatment_terminal_sha256 "$treatment_terminal_sha256" \
			--arg control_terminal_sha256 "$control_terminal_sha256" '
			type == "object" and .schema_version == 1 and .contract == $contract and .seed == $seed and
			.simulated_horizon == $horizon and .simulation_start_nano == $start and .simulation_end_nano == $end and
			.status == "UNAVAILABLE_TERMINAL_FAILURE" and .valid == false and .evidence_valid == true and
			.activation_satisfied == false and .anti_cheating_satisfied == false and .provenance == null and
			.holdouts_consumed == false and .arm_artifacts_valid == true and
			(.treatment_terminal_status == $treatment_status) and (.control_terminal_status == $control_status) and
			(.treatment_run_status_sha256 == $treatment_status_sha256) and
			(.control_run_status_sha256 == $control_status_sha256) and
			(.treatment_terminal_outcome_sha256 == $treatment_terminal_sha256) and
			(.control_terminal_outcome_sha256 == $control_terminal_sha256) and
			(($treatment_status == "terminal_failure") or ($control_status == "terminal_failure")) and
			(($treatment_status == "completed") or ($treatment_status == "terminal_failure")) and
			(($control_status == "completed") or ($control_status == "terminal_failure"))' "$comparison_path" >/dev/null
		return $?
	fi
	jq -e --arg analyzer_sha256 "$expected_analyzer_sha256" --arg revision "$expected_revision" '
		type == "object" and
		(.provenance | type) == "object" and .provenance.valid == true and
			.provenance.analyzer_sha256 == $analyzer_sha256 and
			.provenance.analyzer_source_revision == $revision and
			.provenance.analyzer_source_modified == false and
			.provenance.source_revision_mode == "pinned_live" and
			(.provenance.treatment.source_revision == $revision) and
			(.provenance.control.source_revision == $revision) and
			(.provenance.resource | type) == "object" and .provenance.resource.executed == true and
			.provenance.resource.resource_guard_failed == false and
			(.provenance.resource.exit_status == 0 or .provenance.resource.exit_status == 1) and
			(.provenance.resource.wall_clock_seconds | type) == "number" and
			.provenance.resource.wall_clock_seconds >= 0 and .provenance.resource.wall_clock_seconds <= 300 and
			(.provenance.resource.peak_rss_bytes | type) == "number" and
			.provenance.resource.peak_rss_bytes > 0 and .provenance.resource.peak_rss_bytes <= (20 * 1024 * 1024 * 1024) and
			(.provenance.resource.host_memory_total_bytes | type) == "number" and .provenance.resource.host_memory_total_bytes > 0 and
			(.provenance.resource.minimum_memory_available_bytes | type) == "number" and
			.provenance.resource.minimum_memory_available_bytes == (if ((.provenance.resource.host_memory_total_bytes + 4) / 5) < (4 * 1024 * 1024 * 1024) then (4 * 1024 * 1024 * 1024) else ((.provenance.resource.host_memory_total_bytes + 4) / 5 | floor) end) and
			(.provenance.resource.initial_available_free_bytes | type) == "number" and
			(.provenance.resource.final_available_free_bytes | type) == "number" and
			(.provenance.resource.initial_memory_available_bytes | type) == "number" and
			(.provenance.resource.final_memory_available_bytes | type) == "number" and
			.provenance.resource.initial_available_free_bytes >= (4 * 1024 * 1024 * 1024) and
			.provenance.resource.final_available_free_bytes >= (4 * 1024 * 1024 * 1024) and
			.provenance.resource.initial_memory_available_bytes >= .provenance.resource.minimum_memory_available_bytes and
			.provenance.resource.final_memory_available_bytes >= .provenance.resource.minimum_memory_available_bytes' "$comparison_path" >/dev/null
}

# SV1D pairs a one-sided treatment with a same-roster mode-off control. The
# historical comparison predicate intentionally requires a zero-roster
# control, so using it here would silently reject the registered scientific
# design. Keep this stricter mode-pair predicate beside the successor contract
# and require both arms to be reconstructible before accepting an activation.
v2_r2_sv1d_require_mode_pair_comparison() {
	[[ $# -eq 2 ]] || return 1
	local comparison_path=$1 expected_supplier_count=$2
	v2_r2_require_single_json_object "$comparison_path" || return 1
	[[ "$expected_supplier_count" =~ ^[1-9][0-9]*$ ]] || return 1
	jq -e --argjson expected_supplier_count "$expected_supplier_count" '
		def bounded_fraction:
			type == "number" and isfinite and . >= 0 and . <= 1;
		def supplier_contract:
			.valid == true and .evidence_valid == true and .anti_cheating_satisfied == true and
			.configured_max_position > 0 and .max_position <= .configured_max_position and
			.min_position >= (-.configured_max_position) and
			.configured_max_inventory > 0 and .max_gross_base_balance <= .configured_max_inventory and
			(.max_inventory_utilization | bounded_fraction) and
			.configured_max_quote_qty > 0 and .max_quote_qty <= .configured_max_quote_qty and
			.max_borrowed == 0;
		def evidence_run:
			.valid == true and .evidence_valid == true and .anti_cheating_satisfied == true and
			.supplier_count == $expected_supplier_count and
			(.suppliers | type) == "array" and (.suppliers | length) == $expected_supplier_count and
			(.venues | type) == "array" and (.venues | length) == 3 and
			.supplier_removal_counterfactual_valid == true and
			.supplier_removal_time_weighted_counterfactual_valid == true and
			(.supplier_removal_snapshot_count | type) == "number" and
			.supplier_removal_snapshot_count == .snapshot_count and
			(.supplier_removal_observed_duration_ns | type) == "number" and .supplier_removal_observed_duration_ns > 0 and
			(.supplier_volume_share | bounded_fraction) and .supplier_volume_share <= 0.75 and
			(.supplier_depth_over_75_active_time_fraction | bounded_fraction) and .supplier_depth_over_75_active_time_fraction <= 0.5 and
			(.supplier_bid_depth_over_75_active_time_fraction | bounded_fraction) and .supplier_bid_depth_over_75_active_time_fraction <= 0.5 and
			(.supplier_ask_depth_over_75_active_time_fraction | bounded_fraction) and .supplier_ask_depth_over_75_active_time_fraction <= 0.5 and
			(.supplier_bid_time_weighted_resting_depth_share | bounded_fraction) and .supplier_bid_time_weighted_resting_depth_share <= 0.75 and
			(.supplier_ask_time_weighted_resting_depth_share | bounded_fraction) and .supplier_ask_time_weighted_resting_depth_share <= 0.75 and
			(.supplier_only_bid_time_weighted_fraction | bounded_fraction) and .supplier_only_bid_time_weighted_fraction <= 0.5 and
			(.supplier_only_ask_time_weighted_fraction | bounded_fraction) and .supplier_only_ask_time_weighted_fraction <= 0.5 and
			(.supplier_removal_qualified_bid_absence_active_time_fraction | bounded_fraction) and .supplier_removal_qualified_bid_absence_active_time_fraction <= 0.5 and
			(.supplier_removal_qualified_ask_absence_active_time_fraction | bounded_fraction) and .supplier_removal_qualified_ask_absence_active_time_fraction <= 0.5 and
			all(.venues[];
				.supplier_removal_counterfactual_valid == true and
				.supplier_removal_time_weighted_counterfactual_valid == true and
				(.supplier_removal_snapshot_count | type) == "number" and .supplier_removal_snapshot_count == .snapshot_count and
				(.supplier_removal_observed_duration_ns | type) == "number" and .supplier_removal_observed_duration_ns > 0 and
				(.supplier_depth_over_75_active_time_fraction | bounded_fraction) and .supplier_depth_over_75_active_time_fraction <= 0.5 and
				(.supplier_bid_depth_over_75_active_time_fraction | bounded_fraction) and .supplier_bid_depth_over_75_active_time_fraction <= 0.5 and
				(.supplier_ask_depth_over_75_active_time_fraction | bounded_fraction) and .supplier_ask_depth_over_75_active_time_fraction <= 0.5 and
				(.supplier_bid_time_weighted_resting_depth_share | bounded_fraction) and .supplier_bid_time_weighted_resting_depth_share <= 0.75 and
				(.supplier_ask_time_weighted_resting_depth_share | bounded_fraction) and .supplier_ask_time_weighted_resting_depth_share <= 0.75 and
				(.supplier_only_bid_time_weighted_fraction | bounded_fraction) and .supplier_only_bid_time_weighted_fraction <= 0.5 and
				(.supplier_only_ask_time_weighted_fraction | bounded_fraction) and .supplier_only_ask_time_weighted_fraction <= 0.5 and
				(.supplier_removal_qualified_bid_absence_active_time_fraction | bounded_fraction) and .supplier_removal_qualified_bid_absence_active_time_fraction <= 0.5 and
				(.supplier_removal_qualified_ask_absence_active_time_fraction | bounded_fraction) and .supplier_removal_qualified_ask_absence_active_time_fraction <= 0.5) and
			all(.suppliers[]; supplier_contract);
		def activated_run:
			evidence_run and .activation_satisfied == true and
			all(.suppliers[];
				 supplier_contract and
				.configured_minimum_qualifying_qty > 0 and .filled_qty >= .configured_minimum_qualifying_qty and
				.fill_caused_risk_transition == true and .fill_count > 0 and .trading_pnl != 0 and
				.inventory_responsive_decision_count > 0 and
				.max_inventory_utilization > 0);
			(.provenance | type) == "object" and .provenance.valid == true and
			.valid == true and .evidence_valid == true and .liquidation_evidence_valid == true and
			(.treatment | activated_run) and
			(.control | evidence_run) and
			.treatment.one_sided_decision_count > 0 and
			.treatment.one_sided_missing_side_accepted_count > 0 and
			.treatment.one_sided_restoration_count > 0 and
			.control.one_sided_decision_count == 0 and
			.survival_effect_satisfied == true and .activation_satisfied == true and
			.anti_cheating_satisfied == true
	' "$comparison_path" >/dev/null
}

v2_r2_sv1d_require_scoring_comparison_claims() {
	[[ $# -eq 11 ]] || return 1
	local provenance_path=$1 output_root=$2 comparison_path=$3 comparison_sha256=$4 expected_status=$5
	local expected_activation=$6 comparison_valid=$7 comparison_evidence_valid=$8 comparison_anticheating=$9
	local comparison_activation=${10} comparison_terminal_negative=${11}
	[[ "$comparison_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	for boolean_value in "$expected_activation" "$comparison_valid" "$comparison_evidence_valid" "$comparison_anticheating" "$comparison_activation" "$comparison_terminal_negative"; do
		[[ "$boolean_value" == true || "$boolean_value" == false ]] || return 1
	done
	[[ -f "$comparison_path" && ! -L "$comparison_path" && "$(v2_r2_sv1d_sha256_file "$comparison_path")" == "$comparison_sha256" ]] || return 1
	v2_r2_require_single_json_object "$provenance_path" || return 1
	jq -e --arg output_root "$output_root" --arg comparison_path "$comparison_path" --arg comparison_sha256 "$comparison_sha256" \
		--arg expected_status "$expected_status" --argjson expected_activation "$expected_activation" \
		--argjson comparison_valid "$comparison_valid" --argjson comparison_evidence_valid "$comparison_evidence_valid" \
		--argjson comparison_anticheating "$comparison_anticheating" --argjson comparison_activation "$comparison_activation" \
		--argjson comparison_terminal_negative "$comparison_terminal_negative" '
		type == "object" and .output_root == $output_root and .status == $expected_status and
			.activation_satisfied == $expected_activation and .comparison.path == $comparison_path and
			.comparison.recorded_path == $comparison_path and .comparison.sha256 == $comparison_sha256 and
			.comparison.exit_status == 0 and .comparison.object_valid == true and
			.comparison.valid == $comparison_valid and .comparison.evidence_valid == $comparison_evidence_valid and
			.comparison.anti_cheating_satisfied == $comparison_anticheating and
			.comparison.activation_satisfied == $comparison_activation and
			.comparison.terminal_negative == $comparison_terminal_negative
	' "$provenance_path" >/dev/null
}

v2_r2_sv1d_classify_comparison() {
	[[ $# -eq 2 ]] || return 1
	local comparison_path=$1 expected_supplier_count=$2
	v2_r2_require_single_json_object "$comparison_path" || return 1
	local comparison_valid comparison_evidence_valid comparison_anticheating comparison_provenance_valid comparison_terminal_negative
	comparison_valid=$(jq -r 'if ((.valid | type) == "boolean" and .valid) then "true" else "false" end' "$comparison_path") || return 1
	comparison_evidence_valid=$(jq -r 'if ((.evidence_valid | type) == "boolean" and .evidence_valid) then "true" else "false" end' "$comparison_path") || return 1
	comparison_anticheating=$(jq -r 'if ((.anti_cheating_satisfied | type) == "boolean" and .anti_cheating_satisfied) then "true" else "false" end' "$comparison_path") || return 1
	comparison_provenance_valid=$(jq -r 'if (.provenance == null or .provenance.valid == true) then "true" else "false" end' "$comparison_path") || return 1
	comparison_terminal_negative=$(jq -r 'if .status == "UNAVAILABLE_TERMINAL_FAILURE" then "true" else "false" end' "$comparison_path") || return 1
	local score_status="SV1D_ACTIVATION_INVALID_EVIDENCE"
	local score_reason="comparison was not a valid reconstructed same-roster pair"
	# A terminal valuation failure is valid negative evidence even though it
	# deliberately carries valid=false and makes no activation claim.
	if [[ "$comparison_evidence_valid" == true && "$comparison_provenance_valid" == true &&
		( "$comparison_valid" == true || "$comparison_terminal_negative" == true ) ]]; then
		if [[ "$comparison_terminal_negative" == true ]]; then
			score_status="SV1D_ACTIVATION_NOT_SATISFIED_TERMINAL_FAILURE"
			score_reason="the registered treatment/control pair reached a valid terminal valuation failure; no activation claim is made"
		elif [[ "$comparison_anticheating" != true ]]; then
			score_status="SV1D_ACTIVATION_REJECTED_ANTI_CHEATING"
			score_reason="reconstructed evidence was valid but a preregistered anti-cheating or concentration predicate failed"
		elif v2_r2_sv1d_require_mode_pair_comparison "$comparison_path" "$expected_supplier_count"; then
			score_status="SV1D_ACTIVATION_ACCEPTED"
			score_reason="all supplier instances, one-sided restoration, paired survival effect, and anti-cheating predicates passed"
		else
			score_status="SV1D_ACTIVATION_NOT_SATISFIED"
			score_reason="reconstructed evidence is valid but the preregistered activation or survival predicate did not pass"
		fi
	fi
	printf '%s\t%s\n' "$score_status" "$score_reason"
}
