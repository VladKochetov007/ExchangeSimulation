#!/usr/bin/env bash
# Run the isolated SV1D five-minute activation probe. The three arms are
# treatment, same-roster mode-off, and no-roster; no holdout seed can be
# selected through this entry point.
set -euo pipefail

if [[ $# -gt 4 ]]; then
	echo "usage: $0 [multivenue-binary] [cdf-liquidity-audit-binary] [evsrender-binary] [checkpointvalidate-binary]" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
scientific_root=$(realpath -e -- "$root_dir") || exit 1
export V2_R2_SV1_CONTRACT_SCRIPT="$root_dir/scripts/v2-r2-sv1d-activation-contract.sh"
source "$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
contract_script=$(v2_r2_select_sv1_contract "$root_dir") || exit 1
source "$contract_script"
source "$root_dir/scripts/v2-r2-sv1-activation-status.sh"

v2_r2_require_known_candidate || exit 1
[[ -z "$(printenv EXSIM_BINARY_EVIDENCE 2>/dev/null || true)" ]] || {
	echo "activation probe refuses prototype EXSIM_BINARY_EVIDENCE overrides" >&2
	exit 1
}
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || {
	echo "activation probe requires a clean scientific worktree" >&2
	exit 1
}

go_bin_dir=/usr/local/go/bin
[[ -x "$go_bin_dir/go" ]] || go_bin_dir=$(dirname -- "$(command -v go)")
PATH="$go_bin_dir:$PATH"
export PATH
head_revision=$(git -C "$root_dir" rev-parse HEAD)
[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || exit 1

binary="$root_dir/bin/multivenue"
audit_binary="$root_dir/bin/cdf-liquidity-audit"
renderer="$root_dir/bin/evsrender"
checkpoint_validator="$root_dir/bin/checkpointvalidate"
if [[ $# -ge 1 ]]; then binary=$1; fi
if [[ $# -ge 2 ]]; then audit_binary=$2; fi
if [[ $# -ge 3 ]]; then renderer=$3; fi
if [[ $# -ge 4 ]]; then checkpoint_validator=$4; fi
for executable in "$binary" "$audit_binary" "$renderer" "$checkpoint_validator"; do
	[[ "$executable" == /* && "$executable" != */ && "$executable" != *$'\n'* && "$executable" != *$'\t'* &&
		-x "$executable" && ! -L "$executable" && "$(realpath -e -- "$executable")" == "$executable" ]] || {
		echo "missing or non-canonical activation executable: $executable" >&2
		exit 1
	}
done

config_checker="$root_dir/scripts/check-v2-r2-sv1d-activation-configs.sh"
[[ -x "$config_checker" ]] || exit 1
"$config_checker" >/dev/null || {
	echo "SV1D activation configs failed their immutable contract" >&2
	exit 1
}

binary_sha256=$(v2_r2_sv1d_sha256_file "$binary")
audit_sha256=$(v2_r2_sv1d_sha256_file "$audit_binary")
renderer_sha256=$(v2_r2_sv1d_sha256_file "$renderer")
checkpoint_validator_sha256=$(v2_r2_sv1d_sha256_file "$checkpoint_validator")
v2_r2_sv1d_require_pinned_binary "$binary" "$head_revision" "$binary_sha256" "exchange_sim/cmd/multivenue" || exit 1
v2_r2_sv1d_require_pinned_binary "$audit_binary" "$head_revision" "$audit_sha256" "exchange_sim/cmd/cdf-liquidity-audit" || exit 1
v2_r2_sv1d_require_pinned_binary "$renderer" "$head_revision" "$renderer_sha256" "exchange_sim/cmd/evsrender" || exit 1
v2_r2_register_checkpoint_validator "$checkpoint_validator" "$head_revision" "$checkpoint_validator_sha256" || exit 1

review_attestation=$(v2_r2_sv1d_review_attestation_path "$head_revision") || exit 1
v2_r2_require_sv1b_review_attestation "$review_attestation" "$head_revision" || {
	echo "SV1D activation requires accepted exact-tree independent review: $review_attestation" >&2
	exit 1
}
review_attestation_sha256=$(v2_r2_sv1d_sha256_file "$review_attestation")
capacity_attestation=$(v2_r2_sv1d_capacity_attestation_path "$head_revision") || exit 1
capacity_config_sha256=$(v2_r2_sv1d_sha256_file "$v2_r2_sv1d_capacity_config") || exit 1
v2_r2_sv1d_require_capacity_attestation "$capacity_attestation" "$head_revision" "$binary_sha256" \
	"$capacity_config_sha256" "$review_attestation" "$review_attestation_sha256" || {
	echo "SV1D activation requires a valid binary-evidence capacity attestation: $capacity_attestation" >&2
	exit 1
}
capacity_attestation_sha256=$(v2_r2_sv1d_sha256_file "$capacity_attestation") || exit 1

IFS=$'\t' read -r host_cpu_count allowed_cpu_count cpu_affinity < <(v2_r2_sv1d_cpu_policy) || exit 1
command -v taskset >/dev/null 2>&1 || exit 1
command -v prlimit >/dev/null 2>&1 || exit 1
activation_gomaxprocs=$v2_r2_sv1_activation_gomaxprocs
activation_memory_limit_bytes=$v2_r2_sv1_activation_memory_limit_bytes
activation_gomemlimit_bytes=$v2_r2_sv1_activation_gomemlimit_bytes
activation_minimum_free_bytes=$v2_r2_sv1_activation_minimum_free_bytes
activation_host_memory_total_bytes=$(v2_r2_sv1d_host_memory_total_bytes) || exit 1
activation_minimum_memory_available_bytes=$(v2_r2_sv1d_required_memory_available_bytes "$activation_host_memory_total_bytes") || exit 1
activation_max_wall_seconds=$v2_r2_sv1_activation_max_wall_seconds
activation_analyzer_max_wall_seconds=$v2_r2_sv1_activation_analyzer_max_wall_seconds
[[ "$activation_gomaxprocs" =~ ^[1-9][0-9]*$ && "$activation_memory_limit_bytes" =~ ^[1-9][0-9]*$ &&
	"$activation_gomemlimit_bytes" =~ ^[1-9][0-9]*$ && "$activation_minimum_free_bytes" =~ ^[1-9][0-9]*$ &&
	"$activation_host_memory_total_bytes" =~ ^[1-9][0-9]*$ &&
	"$activation_minimum_memory_available_bytes" =~ ^[1-9][0-9]*$ && "$activation_max_wall_seconds" =~ ^[1-9][0-9]*$ &&
	"$activation_analyzer_max_wall_seconds" =~ ^[1-9][0-9]*$ ]] || exit 1
(( activation_gomemlimit_bytes < activation_memory_limit_bytes )) || exit 1

v2_r2_acquire_namespace_lock || exit 1
root_override=$(printenv V2_R2_SV1D_ACTIVATION_ROOT 2>/dev/null || true)
if [[ -n "$root_override" ]]; then
	output_root=$root_override
else
	output_root="/home/vlad/external-scratch/$v2_r2_sv1_activation_output_prefix-$v2_r2_sv1_activation_seed-$head_revision"
fi
[[ "$output_root" == /* && "$output_root" != */ && "$output_root" != *$'\n'* && "$output_root" != *$'\t'* ]] || exit 1
resolved_output_root=$(realpath -m -- "$output_root") || exit 1
case "$resolved_output_root" in
	"$scientific_root"|"$scientific_root"/*)
		echo "activation output must remain outside the scientific repository" >&2
		exit 1
		;;
esac
[[ ! -e "$output_root" && ! -L "$output_root" ]] || {
	echo "refusing to overwrite activation output root: $output_root" >&2
	exit 1
}
mkdir -p -- "$(dirname -- "$output_root")"
mkdir -- "$output_root"
[[ "$(realpath -e -- "$output_root")" == "$resolved_output_root" ]] || exit 1

simulation_start_nano=$v2_r2_sv1_activation_simulation_start_nano
simulation_end_nano=$v2_r2_sv1_activation_simulation_end_nano
horizon=$v2_r2_sv1_activation_horizon

arm_exit_statuses='{}'
arm_outcomes='{}'
arm_valid='{}'
set_arm_status() {
	local arm=$1 status=$2
	arm_exit_statuses=$(jq -c --arg arm "$arm" --argjson status "$status" '. + {($arm): $status}' <<<"$arm_exit_statuses")
}
set_arm_outcome() {
	local arm=$1 outcome=$2
	arm_outcomes=$(jq -c --arg arm "$arm" --arg outcome "$outcome" '. + {($arm): $outcome}' <<<"$arm_outcomes")
}
set_arm_valid() {
	local arm=$1 valid=$2
	arm_valid=$(jq -c --arg arm "$arm" --argjson valid "$valid" '. + {($arm): $valid}' <<<"$arm_valid")
}
for arm in treatment mode-off no-roster; do
	set_arm_status "$arm" 125
	set_arm_outcome "$arm" unknown
	set_arm_valid "$arm" false
done

activation_free_bytes() {
	local path=$1 available
	available=$(df -P -B1 -- "$path" | awk 'NR == 2 {print $4}') || return 1
	[[ "$available" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$available"
}
process_rss_bytes() {
	local process_id=$1 rss_kib
	rss_kib=$(awk '$1 == "VmRSS:" {print $2; exit}' "/proc/$process_id/status" 2>/dev/null)
	[[ "$rss_kib" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$((rss_kib * 1024))"
}

process_group_rss_bytes() {
	local process_id=$1 process_group_id process_group_rss=0 child_id child_group_id child_rss_kib
	process_group_id=$(ps -o pgid= -p "$process_id" 2>/dev/null | tr -d ' ') || return 1
	[[ "$process_group_id" =~ ^[1-9][0-9]*$ ]] || return 1
	while read -r child_id child_group_id; do
		[[ "$child_id" =~ ^[1-9][0-9]*$ && "$child_group_id" == "$process_group_id" ]] || continue
		child_rss_kib=$(awk '$1 == "VmRSS:" {print $2; exit}' "/proc/$child_id/status" 2>/dev/null || true)
		[[ "$child_rss_kib" =~ ^[0-9]+$ ]] || continue
		process_group_rss=$((process_group_rss + child_rss_kib * 1024))
	done < <(ps -e -o pid=,pgid= 2>/dev/null)
	printf '%s\n' "$process_group_rss"
}

available_memory_bytes() {
	local available_kib
	available_kib=$(awk '$1 == "MemAvailable:" {print $2; exit}' /proc/meminfo 2>/dev/null)
	[[ "$available_kib" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$((available_kib * 1024))"
}

simulator_pid=""
analyzer_pid=""
terminate_process_group() {
	local child_pid=$1
	[[ "$child_pid" =~ ^[1-9][0-9]*$ ]] || return 0
	local process_group_id
	process_group_id=$(ps -o pgid= -p "$child_pid" 2>/dev/null | tr -d ' ' || true)
	if kill -0 "$child_pid" 2>/dev/null; then
		if [[ "$process_group_id" =~ ^[1-9][0-9]*$ && "$process_group_id" == "$child_pid" ]]; then
			kill -TERM -- "-$process_group_id" 2>/dev/null || true
		else
			kill -TERM "$child_pid" 2>/dev/null || true
		fi
		for _ in {1..15}; do
			kill -0 "$child_pid" 2>/dev/null || break
			sleep 1
		done
		if kill -0 "$child_pid" 2>/dev/null; then
			if [[ "$process_group_id" =~ ^[1-9][0-9]*$ && "$process_group_id" == "$child_pid" ]]; then
				kill -KILL -- "-$process_group_id" 2>/dev/null || true
			else
				kill -KILL "$child_pid" 2>/dev/null || true
			fi
		fi
	fi
	wait "$child_pid" 2>/dev/null || true
}
terminate_simulator() {
	local child_pid=$simulator_pid
	terminate_process_group "$child_pid"
	simulator_pid=""
}
terminate_analyzer() {
	local child_pid=$analyzer_pid
	terminate_process_group "$child_pid"
	analyzer_pid=""
}
cleanup_simulator() {
	local exit_status=$?
	trap - EXIT INT TERM HUP
	terminate_simulator
	terminate_analyzer
	exit "$exit_status"
}
trap cleanup_simulator EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

write_arm_metadata() {
	local arm_dir=$1 arm_name=${1##*/} config_sha256 venue_ids experiment_id hypothesis_id
	config_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/run-config.json") || return 1
	venue_ids=$(jq -ce '.venue_ids' "$arm_dir/run-config.json") || return 1
	experiment_id=$(jq -er '.experiment_id' "$arm_dir/run-config.json") || return 1
	hypothesis_id=$(jq -er '.hypothesis_id' "$arm_dir/run-config.json") || return 1
	jq -n --arg contract "$v2_r2_sv1_activation_contract" \
		--arg cell "$v2_r2_sv1_activation_output_prefix-$v2_r2_sv1_activation_seed-$arm_name" \
		--arg arm "$arm_name" --arg mode "$(v2_r2_sv1d_arm_mode "$arm_name")" --argjson seed "$v2_r2_sv1_activation_seed" \
		--arg horizon "$horizon" --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
		--arg config_sha256 "$config_sha256" --arg binary_sha256 "$binary_sha256" --arg revision "$head_revision" \
		--arg experiment_id "$experiment_id" --arg hypothesis_id "$hypothesis_id" \
		--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		--argjson venue_ids "$venue_ids" --arg binary_path "$binary" \
		--arg binary_go_version "$(v2_r2_binary_go_version "$binary")" \
		--arg binary_goos "$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOOS=") == 1 {sub("GOOS=", "", $2); print $2; exit}')" \
		--arg binary_goarch "$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOARCH=") == 1 {sub("GOARCH=", "", $2); print $2; exit}')" \
		--arg binary_goamd64 "$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOAMD64=") == 1 {sub("GOAMD64=", "", $2); print $2; exit}')" \
		--arg checkpoint_validator_path "$checkpoint_validator" --arg checkpoint_validator_revision "$head_revision" \
		--arg checkpoint_validator_sha256 "$checkpoint_validator_sha256" \
		--arg review_path "$review_attestation" --arg review_sha256 "$review_attestation_sha256" \
		--argjson gomaxprocs "$activation_gomaxprocs" --argjson memory_limit_bytes "$activation_memory_limit_bytes" \
		--argjson gomemlimit_bytes "$activation_gomemlimit_bytes" --argjson host_cpu_count "$host_cpu_count" \
		--argjson allowed_cpu_count "$allowed_cpu_count" --argjson cpu_limit_percent "$v2_r2_sv1_cpu_limit_percent" \
		--arg cpu_affinity "$cpu_affinity" --argjson minimum_free_bytes "$activation_minimum_free_bytes" \
		--argjson host_memory_total_bytes "$activation_host_memory_total_bytes" \
		--argjson minimum_memory_available_bytes "$activation_minimum_memory_available_bytes" \
		--argjson max_wall_seconds "$activation_max_wall_seconds" --argjson analyzer_max_wall_seconds "$activation_analyzer_max_wall_seconds" \
		'{schema_version: 1, contract: $contract, arm: $arm, mode: $mode, cell: $cell,
		 seed: $seed, simulated_horizon: $horizon, simulation_start_nano: $start,
		 simulation_end_nano: $end, config_sha256: $config_sha256, binary_sha256: $binary_sha256,
		 git_revision: $revision, config_experiment_id: $experiment_id, hypothesis_id: $hypothesis_id,
		 evidence_format: $evidence_format, log_mode: $log_mode, venue_ids: $venue_ids,
		 binary_path: $binary_path, binary_go_version: $binary_go_version, binary_goos: $binary_goos,
		 binary_goarch: $binary_goarch, binary_goamd64: $binary_goamd64,
		 checkpoint_validator_path: $checkpoint_validator_path,
		 checkpoint_validator_revision: $checkpoint_validator_revision,
		 checkpoint_validator_sha256: $checkpoint_validator_sha256,
		 review_attestation_path: $review_path, review_attestation_sha256: $review_sha256,
		 resource_policy: {gomaxprocs: $gomaxprocs, memory_limit_bytes: $memory_limit_bytes,
		   gomemlimit_bytes: $gomemlimit_bytes, host_cpu_count: $host_cpu_count,
		   allowed_cpu_count: $allowed_cpu_count, cpu_limit_percent: $cpu_limit_percent,
		   cpu_affinity: $cpu_affinity, minimum_free_bytes: $minimum_free_bytes,
		   host_memory_total_bytes: $host_memory_total_bytes,
		   minimum_memory_available_bytes: $minimum_memory_available_bytes,
		   max_wall_seconds: $max_wall_seconds, analyzer_max_wall_seconds: $analyzer_max_wall_seconds},
		 command: ["multivenue", "-config", "run-config.json", "-duration", $horizon,
		           "-logdir", ".", "-log-mode", $log_mode, "-evidence-format", $evidence_format]}' \
		>"$arm_dir/run-metadata.json"
}

prepare_arm() {
	local arm_dir=$1 config=$2
	mkdir -- "$arm_dir"
	"$binary" -config "$config" -logdir "$arm_dir" -log-mode "$v2_r2_sv1_activation_log_mode" \
		-evidence-format "$v2_r2_sv1_activation_evidence_format" -write-effective-config "$arm_dir/run-config.json" >/dev/null
	cmp -s -- "$config" "$arm_dir/run-config.json" || {
		echo "effective config differs from registered $config" >&2
		return 1
	}
	write_arm_metadata "$arm_dir"
}

run_arm() {
	local arm_dir=$1 arm_name=${1##*/}
	local initial_free final_free current_rss peak_rss=0 peak_at="" resource_failed=false resource_reason=""
	local initial_memory_available final_memory_available arm_started_epoch wall_clock_seconds=0
	local command_status outcome_status terminal_failure=false metadata_sha256
	metadata_sha256=$(v2_r2_sv1d_sha256_file "$arm_dir/run-metadata.json") || return 1
	initial_free=$(activation_free_bytes "$output_root") || return 1
	initial_memory_available=$(available_memory_bytes) || return 1
	final_free=$initial_free
	(( initial_free >= activation_minimum_free_bytes )) || {
		echo "free disk reserve is too small before $arm_name" >&2
		return 1
	}
	(( initial_memory_available >= activation_minimum_memory_available_bytes )) || {
		echo "available-memory reserve is too small before $arm_name" >&2
		return 1
	}
	arm_started_epoch=$(date +%s)
	local stdout_tmp stderr_tmp
	stdout_tmp=$(mktemp "$output_root/$arm_name.stdout.XXXXXX")
	stderr_tmp=$(mktemp "$output_root/$arm_name.stderr.XXXXXX")
	setsid --wait taskset --cpu-list "$cpu_affinity" env GOMAXPROCS="$activation_gomaxprocs" \
		GOMEMLIMIT="$activation_gomemlimit_bytes"B prlimit --as="$activation_memory_limit_bytes" -- \
		"$binary" -config "$arm_dir/run-config.json" -duration "$horizon" -logdir "$arm_dir" \
		-log-mode "$v2_r2_sv1_activation_log_mode" -evidence-format "$v2_r2_sv1_activation_evidence_format" \
		>"$stdout_tmp" 2>"$stderr_tmp" &
	simulator_pid=$!
	while kill -0 "$simulator_pid" 2>/dev/null; do
		wall_clock_seconds=$(( $(date +%s) - arm_started_epoch ))
		if (( wall_clock_seconds > activation_max_wall_seconds )); then
			resource_failed=true
			resource_reason="simulator exceeded registered wall-clock deadline"
			terminate_simulator
			break
		fi
		if ! current_rss=$(process_group_rss_bytes "$simulator_pid"); then
			resource_failed=true
			resource_reason="simulator process-group RSS became unmeasurable"
			terminate_simulator
			break
		fi
		if (( current_rss > peak_rss )); then
			peak_rss=$current_rss
			peak_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
		fi
		if (( current_rss > activation_memory_limit_bytes )); then
			resource_failed=true
			resource_reason="simulator process-group RSS exceeded hard memory limit"
			terminate_simulator
			break
		fi
		if ! final_memory_available=$(available_memory_bytes); then
			resource_failed=true
			resource_reason="available-memory measurement failed"
			terminate_simulator
			break
		fi
		if (( final_memory_available < activation_minimum_memory_available_bytes )); then
			resource_failed=true
			resource_reason="available-memory reserve crossed registered floor"
			terminate_simulator
			break
		fi
		if ! final_free=$(activation_free_bytes "$output_root"); then
			resource_failed=true
			resource_reason="free disk measurement failed"
			terminate_simulator
			break
		fi
		if (( final_free < activation_minimum_free_bytes )); then
			resource_failed=true
			resource_reason="free disk crossed registered reserve"
			terminate_simulator
			break
		fi
		sleep 1
	done
	if [[ -n "$simulator_pid" ]]; then
		if wait "$simulator_pid"; then command_status=0; else command_status=$?; fi
		simulator_pid=""
	else
		command_status=125
	fi
	wall_clock_seconds=$(( $(date +%s) - arm_started_epoch ))
	final_memory_available=$(available_memory_bytes) || {
		resource_failed=true
		resource_reason="final available-memory measurement failed"
		final_memory_available=0
	}
	if (( final_memory_available < activation_minimum_memory_available_bytes )); then
		resource_failed=true
		resource_reason="final available-memory reserve is below registered floor"
	fi
	if (( wall_clock_seconds > activation_max_wall_seconds )); then
		resource_failed=true
		resource_reason="final simulator wall-clock duration exceeded registered deadline"
	fi
	final_free=$(activation_free_bytes "$output_root") || {
		resource_failed=true
		resource_reason="final free disk measurement failed"
		final_free=0
	}
	if (( final_free < activation_minimum_free_bytes )); then
		resource_failed=true
		resource_reason="final free disk is below registered reserve"
	fi
	mv -- "$stdout_tmp" "$arm_dir/simulator.stdout.log"
	mv -- "$stderr_tmp" "$arm_dir/simulator.stderr.log"
	set_arm_status "$arm_name" "$command_status"
	if [[ "$resource_failed" == true ]]; then
		set_arm_outcome "$arm_name" resource_failure
		return 1
	fi
	[[ -s "$arm_dir/terminal-outcome.json" ]] || {
		set_arm_outcome "$arm_name" missing_terminal_outcome
		return 1
	}
	jq -e --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
		-f "$root_dir/scripts/v2-r2-sv1-terminal-outcome.jq" "$arm_dir/terminal-outcome.json" >/dev/null || {
		set_arm_outcome "$arm_name" malformed_terminal_outcome
		return 1
	}
	outcome_status=$(jq -er '.status' "$arm_dir/terminal-outcome.json") || return 1
	case "$outcome_status:$command_status" in
		completed:0) ;;
		terminal_failure:0)
			echo "terminal failure returned zero status for $arm_name" >&2
			return 1
			;;
		terminal_failure:*) terminal_failure=true ;;
		*) echo "terminal outcome/status mismatch for $arm_name" >&2; return 1 ;;
	esac
	set_arm_outcome "$arm_name" "$outcome_status"
	[[ -s "$arm_dir/manifest.json" && -s "$arm_dir/greeks.json" && -s "$arm_dir/latency.json" &&
		-s "$arm_dir/checkpoints.jsonl" && -s "$arm_dir/events.evs" && -s "$arm_dir/binary-evidence-attestation.json" ]] || return 1
	[[ "$metadata_sha256" == "$(v2_r2_sv1d_sha256_file "$arm_dir/run-metadata.json")" ]] || return 1
	v2_r2_require_checkpoint_stream "$arm_dir/checkpoints.jsonl" "$simulation_start_nano" "$simulation_end_nano" evstream_v3 || return 1
	v2_r2_write_evidence_manifest "$arm_dir" || return 1
	v2_r2_verify_evidence_manifest "$arm_dir" || return 1
	v2_r2_write_activation_arm_status "$arm_dir" "$command_status" "$outcome_status" "$terminal_failure" \
		"$metadata_sha256" "$peak_rss" "$peak_at" "$initial_free" "$final_free" "$resource_failed" "$resource_reason" "$wall_clock_seconds" || return 1
	v2_r2_sv1d_require_activation_arm_artifacts "$arm_dir" "$arm_name" "$head_revision" \
		"$(v2_r2_sv1d_sha256_file "$arm_dir/run-config.json")" "$binary_sha256" || return 1
	set_arm_valid "$arm_name" true
}

write_invalid_provenance() {
	local status=$1 reason=$2 path="$output_root/activation-provenance.json" tmp
	[[ ! -e "$path" && ! -L "$path" ]] || return 1
	tmp="$path.tmp-$$"
	jq -n --arg contract "$v2_r2_sv1_activation_contract" --arg candidate "$v2_r2_sv1_candidate_id" \
		--arg revision "$head_revision" --arg status "$status" --arg reason "$reason" --arg output_root "$output_root" \
		--argjson seed "$v2_r2_sv1_activation_seed" --arg horizon "$horizon" \
			--argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
			--argjson arm_exit_statuses "$arm_exit_statuses" --argjson arm_outcomes "$arm_outcomes" --argjson arm_valid "$arm_valid" \
			--arg capacity_path "$capacity_attestation" --arg capacity_sha256 "$capacity_attestation_sha256" \
			--arg capacity_config_sha256 "$capacity_config_sha256" --arg capacity_contract "$v2_r2_sv1d_capacity_attestation_contract" \
			'{schema_version: 1, contract: $contract, candidate: $candidate, candidate_revision: $revision,
		 candidate_tree_sha256: null, seed: $seed, simulated_horizon: $horizon,
		 simulation_start_nano: $start, simulation_end_nano: $end, output_root: $output_root,
		 status: $status, activation_satisfied: false, holdouts_consumed: false, reason: $reason,
			 arm_exit_statuses: $arm_exit_statuses, arm_outcomes: $arm_outcomes, arm_valid: $arm_valid,
			 capacity: {path: $capacity_path, sha256: $capacity_sha256, config_sha256: $capacity_config_sha256, contract: $capacity_contract}}' \
		>"$tmp"
	mv -- "$tmp" "$path"
}

for arm in treatment mode-off no-roster; do
	arm_dir="$output_root/$arm"
	if prepare_arm "$arm_dir" "$(v2_r2_sv1d_config_for_arm "$arm")"; then
		:
	else
		set_arm_outcome "$arm" prepare_failure
	fi
done
for arm in treatment mode-off no-roster; do
	if [[ "$(jq -r --arg arm "$arm" '.[$arm]' <<<"$arm_outcomes")" == unknown ]]; then
		if run_arm "$output_root/$arm"; then
			:
		else
			set_arm_valid "$arm" false
		fi
	fi
done
if ! jq -e 'all(to_entries[]; .value == true)' <<<"$arm_valid" >/dev/null; then
	write_invalid_provenance "INVALID_ARM_EVIDENCE" "one or more activation arms failed the producer contract"
	echo "SV1D activation arms failed; retained diagnostic at $output_root" >&2
	exit 1
fi

no_roster_dir="$output_root/no-roster"
no_roster_diagnostic_path="$output_root/no-roster-diagnostic.json"
no_roster_config_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_dir/run-config.json") || exit 1
no_roster_terminal_status=$(jq -er '.status' "$no_roster_dir/terminal-outcome.json") || exit 1
jq -e '[.initial_accounts // [] | .[] | select((.role // "") | test("^cdf_elastic_supplier_[0-9]+$"))] | length == 0' \
	"$no_roster_dir/greeks.json" >/dev/null || {
	echo "no-roster runtime topology contains a CDF successor supplier" >&2
	exit 1
}
no_roster_greeks_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_dir/greeks.json") || exit 1
no_roster_runtime_cdf_supplier_count=$(v2_r2_sv1d_runtime_cdf_supplier_count "$no_roster_dir") || exit 1
no_roster_runtime_cdf_decision_count=$(v2_r2_sv1d_runtime_cdf_event_count "$no_roster_dir" '"event":"elastic_liquidity_supplier_decision"') || exit 1
no_roster_runtime_cdf_fill_count=$(v2_r2_sv1d_runtime_cdf_event_count "$no_roster_dir" '"event":"elastic_liquidity_supplier_fill"') || exit 1
jq -n --arg contract "v2-r2-sv1d-no-roster-diagnostic-v1" --arg terminal_status "$no_roster_terminal_status" \
	--arg config_sha256 "$no_roster_config_sha256" \
	--arg run_status_sha256 "$(v2_r2_sv1d_sha256_file "$no_roster_dir/run-status.json")" \
	--arg terminal_outcome_sha256 "$(v2_r2_sv1d_sha256_file "$no_roster_dir/terminal-outcome.json")" \
	--arg run_metadata_sha256 "$(v2_r2_sv1d_sha256_file "$no_roster_dir/run-metadata.json")" \
	--arg manifest_sha256 "$(v2_r2_sv1d_sha256_file "$no_roster_dir/manifest.json")" \
	--arg greeks_sha256 "$no_roster_greeks_sha256" \
	--arg binary_attestation_sha256 "$(v2_r2_sv1d_sha256_file "$no_roster_dir/binary-evidence-attestation.json")" \
	--arg evidence_manifest_sha256 "$(v2_r2_sv1d_sha256_file "$no_roster_dir/evidence-manifest.json")" \
	--arg simulator_stdout_sha256 "$(v2_r2_sv1d_sha256_file "$no_roster_dir/simulator.stdout.log")" \
	--arg simulator_stderr_sha256 "$(v2_r2_sv1d_sha256_file "$no_roster_dir/simulator.stderr.log")" \
	--argjson runtime_cdf_supplier_count "$no_roster_runtime_cdf_supplier_count" \
	--argjson runtime_cdf_decision_count "$no_roster_runtime_cdf_decision_count" \
	--argjson runtime_cdf_fill_count "$no_roster_runtime_cdf_fill_count" \
	'{schema_version: 1, contract: $contract, arm: "no-roster", config_sha256: $config_sha256,
	 run_status_sha256: $run_status_sha256, terminal_outcome_sha256: $terminal_outcome_sha256,
	 run_metadata_sha256: $run_metadata_sha256, manifest_sha256: $manifest_sha256,
	 greeks_sha256: $greeks_sha256, runtime_topology_sha256: $greeks_sha256,
	 binary_attestation_sha256: $binary_attestation_sha256, evidence_manifest_sha256: $evidence_manifest_sha256,
	 simulator_stdout_sha256: $simulator_stdout_sha256, simulator_stderr_sha256: $simulator_stderr_sha256,
	 terminal_status: $terminal_status, strict_population_accounting: true, cdf_roster: false,
	 runtime_topology_valid: true, runtime_cdf_supplier_count: $runtime_cdf_supplier_count,
	 runtime_cdf_decision_count: $runtime_cdf_decision_count, runtime_cdf_fill_count: $runtime_cdf_fill_count,
	 cdf_metrics: "out_of_scope", status: "VALID_TOPOLOGY_CONTROL", holdouts_consumed: false}' \
	>"$no_roster_diagnostic_path"
v2_r2_sv1d_require_no_roster_diagnostic "$no_roster_diagnostic_path" "$no_roster_dir" "$no_roster_config_sha256" || exit 1
no_roster_diagnostic_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_diagnostic_path") || exit 1

treatment_dir="$output_root/treatment"
mode_off_dir="$output_root/mode-off"
comparison_path="$output_root/cdf-liquidity-comparison.json"
comparison_record_path="$comparison_path"
comparison_status=125
comparison_object_valid=false
comparison_sha256=""
comparison_resource_executed=false
comparison_resource_guard_failed=false
comparison_resource_reason=""
comparison_initial_free=0
comparison_final_free=0
comparison_initial_memory=0
comparison_final_memory=0
comparison_analyzer_peak_rss=0
comparison_analyzer_wall_seconds=0
comparison_analyzer_exit_status=125
comparison_analyzer_stderr_path=""
comparison_analyzer_stderr_sha256=""
if [[ "$(jq -r '.status' "$treatment_dir/terminal-outcome.json")" == completed &&
	"$(jq -r '.status' "$mode_off_dir/terminal-outcome.json")" == completed ]]; then
	comparison_tmp="$comparison_path.tmp-$$"
	comparison_initial_free=$(activation_free_bytes "$output_root") || exit 1
	comparison_initial_memory=$(available_memory_bytes) || exit 1
	(( comparison_initial_free >= activation_minimum_free_bytes && comparison_initial_memory >= activation_minimum_memory_available_bytes )) || exit 1
	comparison_tmpdir="$output_root/analyzer-tmp"
	mkdir -- "$comparison_tmpdir"
	comparison_analyzer_stderr_tmp=$(mktemp "$output_root/analyzer.stderr.XXXXXX")
	comparison_analyzer_started_epoch=$(date +%s)
	comparison_resource_executed=true
	setsid --wait taskset --cpu-list "$cpu_affinity" env TMPDIR="$comparison_tmpdir" GOMAXPROCS="$activation_gomaxprocs" \
		GOMEMLIMIT="$activation_gomemlimit_bytes"B prlimit --as="$activation_memory_limit_bytes" -- \
		"$audit_binary" -treatment "$treatment_dir" -control "$mode_off_dir" >"$comparison_tmp" 2>"$comparison_analyzer_stderr_tmp" &
	analyzer_pid=$!
	while kill -0 "$analyzer_pid" 2>/dev/null; do
		comparison_analyzer_wall_seconds=$(( $(date +%s) - comparison_analyzer_started_epoch ))
		if (( comparison_analyzer_wall_seconds > activation_analyzer_max_wall_seconds )); then
			comparison_resource_guard_failed=true
			comparison_resource_reason="analyzer exceeded registered wall-clock deadline"
			terminate_analyzer
			break
		fi
		if ! comparison_current_rss=$(process_group_rss_bytes "$analyzer_pid"); then
			comparison_resource_guard_failed=true
			comparison_resource_reason="analyzer process-group RSS became unmeasurable"
			terminate_analyzer
			break
		fi
		if (( comparison_current_rss > comparison_analyzer_peak_rss )); then
			comparison_analyzer_peak_rss=$comparison_current_rss
		fi
		if (( comparison_current_rss > activation_memory_limit_bytes )); then
			comparison_resource_guard_failed=true
			comparison_resource_reason="analyzer process-group RSS exceeded hard memory limit"
			terminate_analyzer
			break
		fi
		if ! comparison_final_memory=$(available_memory_bytes); then
			comparison_resource_guard_failed=true
			comparison_resource_reason="analyzer available-memory measurement failed"
			terminate_analyzer
			break
		fi
		if (( comparison_final_memory < activation_minimum_memory_available_bytes )); then
			comparison_resource_guard_failed=true
			comparison_resource_reason="analyzer available-memory reserve crossed registered floor"
			terminate_analyzer
			break
		fi
		if ! comparison_final_free=$(activation_free_bytes "$output_root"); then
			comparison_resource_guard_failed=true
			comparison_resource_reason="analyzer free-disk measurement failed"
			terminate_analyzer
			break
		fi
		if (( comparison_final_free < activation_minimum_free_bytes )); then
			comparison_resource_guard_failed=true
			comparison_resource_reason="analyzer free-disk reserve crossed registered floor"
			terminate_analyzer
			break
		fi
		sleep 1
	done
	if [[ -n "$analyzer_pid" ]]; then
		if wait "$analyzer_pid"; then comparison_analyzer_exit_status=0; else comparison_analyzer_exit_status=$?; fi
		comparison_status=$comparison_analyzer_exit_status
		analyzer_pid=""
	else
		comparison_analyzer_exit_status=125
		comparison_status=125
	fi
	comparison_analyzer_wall_seconds=$(( $(date +%s) - comparison_analyzer_started_epoch ))
	comparison_final_free=$(activation_free_bytes "$output_root") || exit 1
	comparison_final_memory=$(available_memory_bytes) || exit 1
	if (( comparison_final_free < activation_minimum_free_bytes || comparison_final_memory < activation_minimum_memory_available_bytes )); then
		comparison_resource_guard_failed=true
		comparison_resource_reason="analyzer final resource reserve crossed registered floor"
		comparison_status=125
	fi
	if (( comparison_analyzer_wall_seconds > activation_analyzer_max_wall_seconds )); then
		comparison_resource_guard_failed=true
		comparison_resource_reason="final analyzer wall-clock duration exceeded registered deadline"
		comparison_status=125
	fi
	comparison_analyzer_stderr_path="$output_root/analyzer.stderr.log"
	mv -- "$comparison_analyzer_stderr_tmp" "$comparison_analyzer_stderr_path"
	comparison_analyzer_stderr_sha256=$(v2_r2_sv1d_sha256_file "$comparison_analyzer_stderr_path") || exit 1
	if [[ "$comparison_resource_guard_failed" == false ]] && v2_r2_require_single_json_object "$comparison_tmp"; then
		comparison_enriched_tmp="$comparison_tmp.enriched"
		jq --argjson analyzer_exit_status "$comparison_analyzer_exit_status" --argjson peak_rss_bytes "$comparison_analyzer_peak_rss" \
			--argjson wall_clock_seconds "$comparison_analyzer_wall_seconds" --argjson initial_free_bytes "$comparison_initial_free" \
			--argjson final_free_bytes "$comparison_final_free" --argjson initial_memory_available_bytes "$comparison_initial_memory" \
			--argjson final_memory_available_bytes "$comparison_final_memory" --argjson host_memory_total_bytes "$activation_host_memory_total_bytes" \
			--argjson minimum_memory_available_bytes "$activation_minimum_memory_available_bytes" \
			--argjson resource_guard_failed "$comparison_resource_guard_failed" --arg resource_guard_reason "$comparison_resource_reason" \
			'.provenance.resource = {executed: true, exit_status: $analyzer_exit_status, peak_rss_bytes: $peak_rss_bytes,
				wall_clock_seconds: $wall_clock_seconds, initial_available_free_bytes: $initial_free_bytes,
				final_available_free_bytes: $final_free_bytes, initial_memory_available_bytes: $initial_memory_available_bytes,
				final_memory_available_bytes: $final_memory_available_bytes, host_memory_total_bytes: $host_memory_total_bytes,
				minimum_memory_available_bytes: $minimum_memory_available_bytes, resource_guard_failed: $resource_guard_failed,
				resource_guard_reason: $resource_guard_reason}' "$comparison_tmp" >"$comparison_enriched_tmp" &&
			mv -- "$comparison_enriched_tmp" "$comparison_tmp"
	else
		comparison_object_valid=false
	fi
	if [[ "$comparison_resource_guard_failed" == false ]] && v2_r2_require_single_json_object "$comparison_tmp"; then
		comparison_object_valid=true
	else
		comparison_object_valid=false
	fi
	if [[ "$comparison_status" -eq 0 || "$comparison_status" -eq 1 ]] && [[ "$comparison_object_valid" == true ]] &&
		jq -e '(.evidence_valid == true) and (.provenance == null or .provenance.valid == true)' "$comparison_tmp" >/dev/null 2>&1 &&
		v2_r2_sv1d_require_comparison_provenance "$comparison_tmp" "$audit_sha256" "$head_revision" "$treatment_dir" "$mode_off_dir"; then
		# cdf-liquidity-audit returns exit status 1 for a valid, reconstructible
		# negative outcome whose economic predicate is false. The evidence
		# validity, not that scientific outcome status, controls publication.
		comparison_status=0
		mv -- "$comparison_tmp" "$comparison_path"
	else
		comparison_record_path="$comparison_path.invalid"
		mv -- "$comparison_tmp" "$comparison_record_path"
	fi
else
	jq -n --arg contract "$v2_r2_sv1_activation_contract" --argjson seed "$v2_r2_sv1_activation_seed" \
		--arg horizon "$horizon" --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
		--arg treatment_status "$(jq -r '.status' "$treatment_dir/terminal-outcome.json")" \
		--arg control_status "$(jq -r '.status' "$mode_off_dir/terminal-outcome.json")" \
		--arg treatment_status_sha256 "$(v2_r2_sv1d_sha256_file "$treatment_dir/run-status.json")" \
		--arg control_status_sha256 "$(v2_r2_sv1d_sha256_file "$mode_off_dir/run-status.json")" \
		--arg treatment_terminal_sha256 "$(v2_r2_sv1d_sha256_file "$treatment_dir/terminal-outcome.json")" \
		--arg control_terminal_sha256 "$(v2_r2_sv1d_sha256_file "$mode_off_dir/terminal-outcome.json")" \
		'{schema_version: 1, contract: $contract, seed: $seed, status: "UNAVAILABLE_TERMINAL_FAILURE",
		 valid: false, evidence_valid: true, activation_satisfied: false, anti_cheating_satisfied: false,
		 treatment_terminal_status: $treatment_status, control_terminal_status: $control_status,
		 simulated_horizon: $horizon, simulation_start_nano: $start, simulation_end_nano: $end,
		 provenance: null, arm_artifacts_valid: true, holdouts_consumed: false,
		 treatment_run_status_sha256: $treatment_status_sha256, control_run_status_sha256: $control_status_sha256,
		 treatment_terminal_outcome_sha256: $treatment_terminal_sha256, control_terminal_outcome_sha256: $control_terminal_sha256,
		 reason: "treatment/mode-off pair lacks two completed terminal valuations"}' \
		>"$comparison_path"
	comparison_status=0
	comparison_object_valid=true
	v2_r2_sv1d_require_comparison_provenance "$comparison_path" "$audit_sha256" "$head_revision" "$treatment_dir" "$mode_off_dir" || exit 1
fi

if [[ -d "$output_root/analyzer-tmp" ]]; then
	rmdir -- "$output_root/analyzer-tmp" 2>/dev/null || true
fi
comparison_sha256=$(v2_r2_sv1d_sha256_file "$comparison_record_path")
comparison_valid=false
comparison_activation=false
comparison_anticheating=false
if [[ "$comparison_object_valid" == true ]]; then
	comparison_valid=$(jq -r '(.valid // false) | if . then "true" else "false" end' "$comparison_record_path") || comparison_valid=false
	comparison_activation=$(jq -r '(.activation_satisfied // false) | if . then "true" else "false" end' "$comparison_record_path") || comparison_activation=false
	comparison_anticheating=$(jq -r '(.anti_cheating_satisfied // false) | if . then "true" else "false" end' "$comparison_record_path") || comparison_anticheating=false
fi
expected_supplier_count=$(jq -er '(.elastic_liquidity_suppliers | length) * (.venue_ids | length)' "$v2_r2_sv1_activation_config")
pair_contract_satisfied=false
if [[ "$comparison_status" -eq 0 && "$comparison_object_valid" == true ]] &&
	v2_r2_sv1d_require_mode_pair_comparison "$comparison_record_path" "$expected_supplier_count"; then
	pair_contract_satisfied=true
fi

arm_records='{}'
for arm in treatment mode-off no-roster; do
	actual_dir="$output_root/$arm"
	config_path="$(v2_r2_sv1d_config_for_arm "$arm")"
	metadata_sha256=""
	manifest_sha256=""
	attestation_sha256=""
	evidence_manifest_sha256=""
	stdout_sha256=""
	stderr_sha256=""
	if [[ -s "$actual_dir/run-config.json" ]]; then
		config_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/run-config.json")
	else
		config_sha256=""
	fi
	if [[ -s "$actual_dir/run-status.json" ]]; then
		status_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/run-status.json")
	else
		status_sha256=""
	fi
	if [[ -s "$actual_dir/terminal-outcome.json" ]]; then
		terminal_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/terminal-outcome.json")
		terminal_status=$(jq -r '.status' "$actual_dir/terminal-outcome.json")
	else
		terminal_sha256=""
		terminal_status=unknown
	fi
	if [[ -s "$actual_dir/run-metadata.json" ]]; then metadata_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/run-metadata.json"); fi
	if [[ -s "$actual_dir/manifest.json" ]]; then manifest_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/manifest.json"); fi
	if [[ -s "$actual_dir/binary-evidence-attestation.json" ]]; then attestation_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/binary-evidence-attestation.json"); fi
	if [[ -s "$actual_dir/evidence-manifest.json" ]]; then evidence_manifest_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/evidence-manifest.json"); fi
	if [[ -f "$actual_dir/simulator.stdout.log" ]]; then stdout_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/simulator.stdout.log"); fi
	if [[ -f "$actual_dir/simulator.stderr.log" ]]; then stderr_sha256=$(v2_r2_sv1d_sha256_file "$actual_dir/simulator.stderr.log"); fi
	arm_record=$(jq -n --arg path "$actual_dir" --arg config_path "$config_path" \
		--arg mode "$(v2_r2_sv1d_arm_mode "$arm")" --arg config_sha256 "$config_sha256" \
		--arg terminal_status "$terminal_status" --arg status_sha256 "$status_sha256" \
		--arg terminal_sha256 "$terminal_sha256" --arg metadata_sha256 "$metadata_sha256" \
		--arg manifest_sha256 "$manifest_sha256" --arg attestation_sha256 "$attestation_sha256" \
		--arg evidence_manifest_sha256 "$evidence_manifest_sha256" --arg stdout_sha256 "$stdout_sha256" --arg stderr_sha256 "$stderr_sha256" \
		--argjson exit_status "$(jq -r --arg arm "$arm" '.[$arm]' <<<"$arm_exit_statuses")" \
		--argjson valid "$(jq -r --arg arm "$arm" '.[$arm]' <<<"$arm_valid")" \
		'{path: $path, config_path: $config_path, mode: $mode, config_sha256: $config_sha256,
		 exit_status: $exit_status, terminal_status: $terminal_status, valid: $valid,
		 run_status_sha256: $status_sha256, terminal_outcome_sha256: $terminal_sha256,
		 run_metadata_sha256: $metadata_sha256, manifest_sha256: $manifest_sha256,
		 binary_attestation_sha256: $attestation_sha256, evidence_manifest_sha256: $evidence_manifest_sha256,
		 simulator_stdout_sha256: $stdout_sha256, simulator_stderr_sha256: $stderr_sha256}')
	arm_records=$(jq -c --arg arm "$arm" --argjson record "$arm_record" '. + {($arm): $record}' <<<"$arm_records")
done

provenance_path="$output_root/activation-provenance.json"
[[ ! -e "$provenance_path" && ! -L "$provenance_path" ]] || exit 1
candidate_tree_sha256=$(v2_r2_sv1d_git_tree_sha256 "$head_revision")
config_manifest_sha256=$(v2_r2_sv1d_sha256_file "$v2_r2_sv1_config_provenance_manifest")
final_status=ACTIVATION_CONTRACT_NOT_SATISFIED
if [[ "$pair_contract_satisfied" == true ]]; then
	final_status=ACTIVATION_CONTRACT_SATISFIED
	comparison_provenance_valid=true
else
	comparison_provenance_valid=false
	if [[ -s "$comparison_record_path" ]] &&
		jq -e 'type == "object" and .evidence_valid == true and (.provenance.valid // true) == true' "$comparison_record_path" >/dev/null 2>&1; then
		comparison_provenance_valid=true
	fi
	if [[ "$comparison_status" -ne 0 || "$comparison_object_valid" != true || "$comparison_provenance_valid" != true ]]; then
	final_status=INVALID_AUDIT_EVIDENCE
	fi
fi
provenance_tmp="$provenance_path.tmp-$$"
jq -n --arg contract "$v2_r2_sv1_activation_pair_contract" --arg candidate "$v2_r2_sv1_candidate_id" \
	--arg revision "$head_revision" --arg tree_sha256 "$candidate_tree_sha256" --arg output_root "$output_root" \
	--argjson seed "$v2_r2_sv1_activation_seed" --arg horizon "$horizon" \
	--argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
	--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
	--argjson venue_ids "$(jq -ce '.venue_ids' "$treatment_dir/run-config.json")" \
	--argjson expected_supplier_count "$expected_supplier_count" \
	--argjson calendar "$v2_r2_sv1d_calendar" --arg status "$final_status" \
	--argjson activation_satisfied "$pair_contract_satisfied" --argjson comparison_valid "$comparison_valid" \
	--argjson comparison_object_valid "$comparison_object_valid" --argjson comparison_status "$comparison_status" \
	--arg comparison_path "$comparison_path" --arg recorded_path "$comparison_record_path" --arg comparison_sha256 "$comparison_sha256" \
	--argjson comparison_activation "$comparison_activation" --argjson comparison_anticheating "$comparison_anticheating" \
	--arg analyzer_stderr_path "$comparison_analyzer_stderr_path" --arg analyzer_stderr_sha256 "$comparison_analyzer_stderr_sha256" \
	--argjson comparison_resource_executed "$comparison_resource_executed" \
	--argjson comparison_resource_guard_failed "$comparison_resource_guard_failed" \
	--arg comparison_resource_reason "$comparison_resource_reason" --argjson comparison_analyzer_exit_status "$comparison_analyzer_exit_status" \
	--argjson comparison_analyzer_peak_rss "$comparison_analyzer_peak_rss" --argjson comparison_analyzer_wall_seconds "$comparison_analyzer_wall_seconds" \
	--argjson comparison_initial_free "$comparison_initial_free" --argjson comparison_final_free "$comparison_final_free" \
	--argjson comparison_initial_memory "$comparison_initial_memory" --argjson comparison_final_memory "$comparison_final_memory" \
	--argjson comparison_host_memory_total "$activation_host_memory_total_bytes" --argjson comparison_minimum_memory "$activation_minimum_memory_available_bytes" \
	--arg no_roster_diagnostic_path "$no_roster_diagnostic_path" --arg no_roster_diagnostic_sha256 "$no_roster_diagnostic_sha256" \
	--arg binary_path "$binary" --arg binary_sha256 "$binary_sha256" --arg audit_path "$audit_binary" --arg audit_sha256 "$audit_sha256" \
	--arg renderer_path "$renderer" --arg renderer_sha256 "$renderer_sha256" \
	--arg checkpoint_path "$checkpoint_validator" --arg checkpoint_sha256 "$checkpoint_validator_sha256" \
		--arg review_path "$review_attestation" --arg review_sha256 "$review_attestation_sha256" \
		--arg capacity_path "$capacity_attestation" --arg capacity_sha256 "$capacity_attestation_sha256" \
		--arg capacity_config_sha256 "$capacity_config_sha256" --arg capacity_contract "$v2_r2_sv1d_capacity_attestation_contract" \
		--arg config_manifest_path "$v2_r2_sv1_config_provenance_manifest" --arg config_manifest_sha256 "$config_manifest_sha256" \
	--argjson arms "$arm_records" \
	--argjson gomaxprocs "$activation_gomaxprocs" --argjson memory_limit_bytes "$activation_memory_limit_bytes" \
	--argjson gomemlimit_bytes "$activation_gomemlimit_bytes" --argjson host_cpu_count "$host_cpu_count" \
	--argjson allowed_cpu_count "$allowed_cpu_count" --argjson cpu_limit_percent "$v2_r2_sv1_cpu_limit_percent" \
	--arg cpu_affinity "$cpu_affinity" --argjson minimum_free_bytes "$activation_minimum_free_bytes" \
	--argjson minimum_memory_available_bytes "$activation_minimum_memory_available_bytes" \
	--argjson max_wall_seconds "$activation_max_wall_seconds" --argjson analyzer_max_wall_seconds "$activation_analyzer_max_wall_seconds" \
	'{schema_version: 1, contract: $contract, candidate: $candidate, candidate_revision: $revision,
	 candidate_tree_sha256: $tree_sha256, seed: $seed, simulated_horizon: $horizon,
	 simulation_start_nano: $start, simulation_end_nano: $end, evidence_format: $evidence_format,
	 log_mode: $log_mode, venue_ids: $venue_ids, expiry_calendar: $calendar, output_root: $output_root,
	 status: $status, activation_satisfied: $activation_satisfied, holdouts_consumed: false,
	 expected_supplier_count: $expected_supplier_count, config_provenance_manifest_path: $config_manifest_path,
	 config_provenance_manifest_sha256: $config_manifest_sha256,
	 binaries: {simulator: {path: $binary_path, sha256: $binary_sha256},
	   analyzer: {path: $audit_path, sha256: $audit_sha256},
	   renderer: {path: $renderer_path, sha256: $renderer_sha256},
	   checkpoint_validator: {path: $checkpoint_path, sha256: $checkpoint_sha256}},
		 review: {path: $review_path, sha256: $review_sha256},
		 capacity: {path: $capacity_path, sha256: $capacity_sha256, config_sha256: $capacity_config_sha256, contract: $capacity_contract},
	 comparison: {path: $comparison_path, recorded_path: $recorded_path, sha256: $comparison_sha256,
	   exit_status: $comparison_status, object_valid: $comparison_object_valid, valid: $comparison_valid,
	   activation_satisfied: $comparison_activation, anti_cheating_satisfied: $comparison_anticheating,
	   analyzer_stderr_path: (if $analyzer_stderr_path == "" then null else $analyzer_stderr_path end),
	   analyzer_stderr_sha256: (if $analyzer_stderr_sha256 == "" then null else $analyzer_stderr_sha256 end),
	   resource: {executed: $comparison_resource_executed, exit_status: $comparison_analyzer_exit_status,
	     peak_rss_bytes: $comparison_analyzer_peak_rss, wall_clock_seconds: $comparison_analyzer_wall_seconds,
	     initial_available_free_bytes: $comparison_initial_free, final_available_free_bytes: $comparison_final_free,
	     initial_memory_available_bytes: $comparison_initial_memory, final_memory_available_bytes: $comparison_final_memory,
	     host_memory_total_bytes: $comparison_host_memory_total, minimum_memory_available_bytes: $comparison_minimum_memory,
	     resource_guard_failed: $comparison_resource_guard_failed, resource_guard_reason: $comparison_resource_reason}},
	 no_roster_diagnostic: {path: $no_roster_diagnostic_path, sha256: $no_roster_diagnostic_sha256,
	   status: "VALID_TOPOLOGY_CONTROL", cdf_metrics: "out_of_scope"},
	 arms: $arms,
	 resource_policy: {gomaxprocs: $gomaxprocs, memory_limit_bytes: $memory_limit_bytes,
	   gomemlimit_bytes: $gomemlimit_bytes, host_cpu_count: $host_cpu_count,
	   allowed_cpu_count: $allowed_cpu_count, cpu_limit_percent: $cpu_limit_percent,
	   cpu_affinity: $cpu_affinity, minimum_free_bytes: $minimum_free_bytes,
	   host_memory_total_bytes: $comparison_host_memory_total,
	   minimum_memory_available_bytes: $minimum_memory_available_bytes,
	   max_wall_seconds: $max_wall_seconds, analyzer_max_wall_seconds: $analyzer_max_wall_seconds},
	 scope: "development-only five-minute mechanism activation; not a 24-hour survival claim"}' \
	>"$provenance_tmp"
mv -- "$provenance_tmp" "$provenance_path"

if [[ "$pair_contract_satisfied" == true ]]; then
	echo "SV1D activation contract satisfied: $output_root"
	exit 0
fi
echo "SV1D activation contract not satisfied; retained evidence: $output_root" >&2
exit 1
