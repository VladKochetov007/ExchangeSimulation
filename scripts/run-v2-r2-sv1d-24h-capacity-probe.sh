#!/usr/bin/env bash
# Measure the registered SV1D treatment's 24-hour evstream_v3 footprint. This
# is a capacity prerequisite, not a scientific result or activation run.
set -euo pipefail

if [[ $# -gt 2 ]]; then
	echo "usage: $0 [multivenue-binary] [checkpointvalidate-binary]" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
scientific_root=$(realpath -e -- "$root_dir") || exit 1
export V2_R2_SV1_CONTRACT_SCRIPT="$root_dir/scripts/v2-r2-sv1d-activation-contract.sh"
source "$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
contract_script=$(v2_r2_select_sv1_contract "$root_dir") || exit 1
[[ "$contract_script" == "$V2_R2_SV1_CONTRACT_SCRIPT" ]] || exit 1
source "$contract_script"

v2_r2_require_known_candidate || {
	echo "SV1D capacity probe received an unknown candidate" >&2
	exit 1
}
[[ -z "$(printenv EXSIM_BINARY_EVIDENCE 2>/dev/null || true)" ]] || {
	echo "SV1D capacity probe refuses prototype evidence overrides" >&2
	exit 1
}
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || {
	echo "SV1D capacity probe requires a clean scientific worktree" >&2
	exit 1
}

go_bin_dir=/usr/local/go/bin
[[ -x "$go_bin_dir/go" ]] || go_bin_dir=$(dirname -- "$(command -v go)")
PATH="$go_bin_dir:$PATH"
export PATH
head_revision=$(git -C "$root_dir" rev-parse HEAD) || exit 1
[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || exit 1

binary=${1:-"$root_dir/bin/multivenue"}
checkpoint_validator=${2:-"$root_dir/bin/checkpointvalidate"}
for executable in "$binary" "$checkpoint_validator"; do
	[[ "$executable" == /* && "$executable" != */ && "$executable" != *$'\n'* && "$executable" != *$'\t'* &&
		-x "$executable" && ! -L "$executable" && "$(realpath -e -- "$executable")" == "$executable" ]] || {
		echo "missing or non-canonical capacity executable: $executable" >&2
		exit 1
	}
done

config="$v2_r2_sv1d_capacity_config"
config_relative="research/configs/v2-r2-sv1d-activation/activation-659-treatment.json"
[[ "$(realpath -e -- "$config")" == "$root_dir/$config_relative" && -s "$config" && ! -L "$config" ]] || exit 1
v2_r2_sv1d_require_activation_capacity "$config" || {
	echo "registered SV1D treatment capital cannot bind within the activation horizon" >&2
	exit 1
}

binary_sha256=$(v2_r2_sv1d_sha256_file "$binary") || exit 1
checkpoint_validator_sha256=$(v2_r2_sv1d_sha256_file "$checkpoint_validator") || exit 1
v2_r2_sv1d_require_pinned_binary "$binary" "$head_revision" "$binary_sha256" "exchange_sim/cmd/multivenue" || exit 1
v2_r2_sv1d_require_pinned_binary "$checkpoint_validator" "$head_revision" "$checkpoint_validator_sha256" \
	"exchange_sim/cmd/checkpointvalidate" || exit 1
v2_r2_register_checkpoint_validator "$checkpoint_validator" "$head_revision" "$checkpoint_validator_sha256" || exit 1

review_path=$(v2_r2_sv1d_review_attestation_path "$head_revision") || exit 1
v2_r2_require_sv1b_review_attestation "$review_path" "$head_revision" || {
	echo "SV1D capacity requires accepted exact-tree independent review: $review_path" >&2
	exit 1
}
review_sha256=$(v2_r2_sv1d_sha256_file "$review_path") || exit 1

command -v taskset >/dev/null 2>&1 || exit 1
command -v setsid >/dev/null 2>&1 || exit 1
command -v prlimit >/dev/null 2>&1 || exit 1
IFS=$'\t' read -r host_cpu_count allowed_cpu_count cpu_affinity < <(v2_r2_sv1d_cpu_policy) || exit 1
[[ "$host_cpu_count" =~ ^[1-9][0-9]*$ && "$allowed_cpu_count" =~ ^[1-9][0-9]*$ && "$cpu_affinity" != *$'\n'* ]] || exit 1
export GOMAXPROCS="$v2_r2_sv1d_capacity_gomaxprocs"
gomemlimit_bytes=$v2_r2_sv1d_capacity_gomemlimit_bytes
memory_limit_bytes=$v2_r2_sv1d_capacity_memory_limit_bytes
minimum_free_bytes=$v2_r2_sv1d_capacity_minimum_free_bytes
safety_margin_bytes=$v2_r2_sv1d_capacity_safety_margin_bytes
max_wall_seconds=$v2_r2_sv1d_capacity_max_wall_seconds
host_memory_total_bytes=$(awk '$1 == "MemTotal:" {printf "%.0f\n", $2 * 1024; exit}' /proc/meminfo) || exit 1
minimum_memory_available_bytes=$(v2_r2_sv1d_required_memory_available_bytes "$host_memory_total_bytes") || exit 1

memory_available_bytes() {
	local available
	available=$(awk '$1 == "MemAvailable:" {printf "%.0f\n", $2 * 1024; exit}' /proc/meminfo) || return 1
	[[ "$available" =~ ^[1-9][0-9]*$ ]] || return 1
	printf '%s\n' "$available"
}
directory_bytes() {
	local bytes
	bytes=$(du -sb -- "$1" | awk 'NR == 1 {print $1}') || return 1
	[[ "$bytes" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$bytes"
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

v2_r2_acquire_namespace_lock || {
	echo "could not acquire the SV1D evidence namespace lock" >&2
	exit 1
}
probe_root=$(v2_r2_sv1d_capacity_probe_root "$head_revision") || exit 1
attestation=$(v2_r2_sv1d_capacity_attestation_path "$head_revision") || exit 1
[[ "$probe_root" == /* && "$probe_root" != "$scientific_root" && "$probe_root" != "$scientific_root"/* &&
	"$attestation" == /* && "$attestation" != "$scientific_root"/* &&
	! -e "$probe_root" && ! -L "$probe_root" && ! -e "$attestation" && ! -L "$attestation" ]] || {
	echo "SV1D capacity output or attestation already exists" >&2
	exit 1
}
mkdir -p -- "$(dirname -- "$probe_root")"
mkdir -- "$probe_root"
probe_cell="$probe_root/$(v2_r2_sv1d_capacity_probe_cell)"
mkdir -- "$probe_cell"

initial_available_free_bytes=$(v2_r2_sv1d_capacity_free_bytes "$probe_root") || exit 1
initial_memory_available_bytes=$(memory_available_bytes) || exit 1
(( initial_available_free_bytes >= minimum_free_bytes )) || exit 1
(( initial_memory_available_bytes >= minimum_memory_available_bytes )) || exit 1

"$binary" -config "$config" -logdir "$probe_cell" -log-mode full -evidence-format evstream_v3 \
	-write-effective-config "$probe_cell/run-config.json" >/dev/null 2>"$probe_root/config.stderr.log" || {
	echo "SV1D capacity config normalization failed; output retained at $probe_root" >&2
	exit 1
}
cmp -s -- "$config" "$probe_cell/run-config.json" || exit 1
config_sha256=$(v2_r2_sv1d_sha256_file "$probe_cell/run-config.json") || exit 1
[[ "$config_sha256" == "$(v2_r2_sv1d_sha256_file "$config")" ]] || exit 1
jq -e '.seed == 659 and .log_mode == "full" and .evidence_format == "evstream_v3" and
	.record_market_data_receipts == true and .strict_risk_contract == true' "$probe_cell/run-config.json" >/dev/null || exit 1

jq -n --arg contract "v2-r2-sv1d-capacity-runner-v1" --arg cell "$(basename -- "$probe_cell")" \
	--arg revision "$head_revision" --arg config_path "$config_relative" --arg config_sha256 "$config_sha256" \
	--arg binary_path "$binary" --arg binary_sha256 "$binary_sha256" --arg review_path "$review_path" --arg review_sha256 "$review_sha256" \
	--arg checkpoint_path "$checkpoint_validator" --arg checkpoint_sha256 "$checkpoint_validator_sha256" \
	--argjson seed "$v2_r2_sv1d_capacity_seed" --arg horizon "$v2_r2_sv1d_capacity_horizon" \
	--argjson start "$v2_r2_sv1d_capacity_simulation_start_nano" --argjson end "$v2_r2_sv1d_capacity_simulation_end_nano" \
	--argjson gomaxprocs "$v2_r2_sv1d_capacity_gomaxprocs" --argjson memory_limit "$memory_limit_bytes" \
	--argjson gomemlimit "$gomemlimit_bytes" --argjson host_memory "$host_memory_total_bytes" \
	--argjson minimum_memory "$minimum_memory_available_bytes" --argjson minimum_free "$minimum_free_bytes" \
	--argjson host_cpu "$host_cpu_count" --argjson allowed_cpu "$allowed_cpu_count" --argjson cpu_limit "$v2_r2_sv1_cpu_limit_percent" \
	--arg affinity "$cpu_affinity" --arg output_dir "$probe_cell" --arg attestation_path "$attestation" \
	'{schema_version: 1, contract: $contract, cell: $cell, git_revision: $revision, seed: $seed,
	 simulated_horizon: $horizon, simulation_start_nano: $start, simulation_end_nano: $end,
	 capacity_only: true, calibration_only: false, holdouts_consumed: false,
	 config_path: $config_path, config_sha256: $config_sha256, binary_path: $binary_path,
	 binary_sha256: $binary_sha256, review_attestation_path: $review_path, review_attestation_sha256: $review_sha256,
	 checkpoint_validator_path: $checkpoint_path, checkpoint_validator_sha256: $checkpoint_sha256,
	 evidence_format: "evstream_v3", log_mode: "full", output_dir: $output_dir,
	 attestation_path: $attestation_path,
	 resource_policy: {gomaxprocs: $gomaxprocs, memory_limit_bytes: $memory_limit,
	   gomemlimit_bytes: $gomemlimit, host_memory_total_bytes: $host_memory,
	   minimum_memory_available_bytes: $minimum_memory, host_cpu_count: $host_cpu,
	   allowed_cpu_count: $allowed_cpu, cpu_limit_percent: $cpu_limit,
	   cpu_affinity: $affinity, minimum_free_bytes: $minimum_free},
	 command: ["multivenue", "-config", "run-config.json", "-duration", $horizon,
	   "-logdir", ".", "-log-mode", "full", "-evidence-format", "evstream_v3"]}' \
	>"$probe_cell/run-metadata.json"
run_metadata_sha256=$(v2_r2_sv1d_sha256_file "$probe_cell/run-metadata.json") || exit 1

simulator_pid=""
terminate_process_group() {
	local process_id=${1:-} process_group_id
	[[ "$process_id" =~ ^[1-9][0-9]*$ ]] || return 0
	process_group_id=$(ps -o pgid= -p "$process_id" 2>/dev/null | tr -d ' ' || true)
	if kill -0 "$process_id" 2>/dev/null; then
		if [[ "$process_group_id" == "$process_id" ]]; then
			kill -TERM -- "-$process_group_id" 2>/dev/null || true
		else
			kill -TERM "$process_id" 2>/dev/null || true
		fi
		for _ in {1..15}; do
			kill -0 "$process_id" 2>/dev/null || break
			sleep 1
		done
		if kill -0 "$process_id" 2>/dev/null; then
			if [[ "$process_group_id" == "$process_id" ]]; then
				kill -KILL -- "-$process_group_id" 2>/dev/null || true
			else
				kill -KILL "$process_id" 2>/dev/null || true
			fi
		fi
	fi
	wait "$process_id" 2>/dev/null || true
}
cleanup_capacity_probe() {
	local exit_status=$?
	trap - EXIT INT TERM HUP
	terminate_process_group "$simulator_pid"
	exit "$exit_status"
}
trap cleanup_capacity_probe EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

stdout_tmp="$probe_root/simulator.stdout.tmp-$$"
stderr_tmp="$probe_root/simulator.stderr.tmp-$$"
start_epoch=$(date +%s)
setsid --wait taskset --cpu-list "$cpu_affinity" env GOMAXPROCS="$GOMAXPROCS" \
	GOMEMLIMIT="${gomemlimit_bytes}B" prlimit --as="$memory_limit_bytes" -- \
	"$binary" -config "$probe_cell/run-config.json" -duration "$v2_r2_sv1d_capacity_horizon" \
	-logdir "$probe_cell" -log-mode full -evidence-format evstream_v3 \
	>"$stdout_tmp" 2>"$stderr_tmp" &
simulator_pid=$!
peak_output_bytes=0
peak_rss_bytes=0
peak_output_at=""
peak_rss_at=""
resource_guard_reason=""
while kill -0 "$simulator_pid" 2>/dev/null; do
	wall_seconds=$(( $(date +%s) - start_epoch ))
	if (( wall_seconds > max_wall_seconds )); then
		resource_guard_reason="capacity simulator exceeded the ${max_wall_seconds}-second wall-clock limit"
		terminate_process_group "$simulator_pid"
		break
	fi
	if ! current_output_bytes=$(directory_bytes "$probe_root"); then
		resource_guard_reason="capacity output-size measurement failed"
		terminate_process_group "$simulator_pid"
		break
	fi
	if (( current_output_bytes > peak_output_bytes )); then
		peak_output_bytes=$current_output_bytes
		peak_output_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	fi
	if ! current_rss_bytes=$(process_group_rss_bytes "$simulator_pid"); then
		resource_guard_reason="capacity simulator RSS measurement failed"
		terminate_process_group "$simulator_pid"
		break
	fi
	if (( current_rss_bytes <= 0 )); then
		resource_guard_reason="capacity simulator process-group RSS was unavailable"
		terminate_process_group "$simulator_pid"
		break
	fi
	if (( current_rss_bytes > peak_rss_bytes )); then
		peak_rss_bytes=$current_rss_bytes
		peak_rss_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	fi
	if (( current_rss_bytes > memory_limit_bytes )); then
		resource_guard_reason="capacity simulator RSS crossed the hard memory limit"
		terminate_process_group "$simulator_pid"
		break
	fi
	if ! current_memory_available_bytes=$(memory_available_bytes); then
		resource_guard_reason="available-memory measurement failed"
		terminate_process_group "$simulator_pid"
		break
	fi
	if (( current_memory_available_bytes < minimum_memory_available_bytes )); then
		resource_guard_reason="available memory crossed the registered reserve"
		terminate_process_group "$simulator_pid"
		break
	fi
	if ! current_available_free_bytes=$(v2_r2_sv1d_capacity_free_bytes "$probe_root"); then
		resource_guard_reason="free-space measurement failed"
		terminate_process_group "$simulator_pid"
		break
	fi
	if (( current_available_free_bytes < minimum_free_bytes )); then
		resource_guard_reason="free disk crossed the registered reserve"
		terminate_process_group "$simulator_pid"
		break
	fi
	sleep 2
done
set +e
wait "$simulator_pid"
simulator_status=$?
set -e
simulator_pid=""
wall_clock_seconds=$(( $(date +%s) - start_epoch ))
mv -- "$stdout_tmp" "$probe_cell/simulator.stdout.log"
mv -- "$stderr_tmp" "$probe_cell/simulator.stderr.log"
[[ "$simulator_status" -eq 0 && -z "$resource_guard_reason" ]] || {
	echo "SV1D capacity probe failed (status=$simulator_status reason=${resource_guard_reason:-simulator failure}); output retained at $probe_root" >&2
	exit 1
}
[[ "$run_metadata_sha256" == "$(v2_r2_sv1d_sha256_file "$probe_cell/run-metadata.json")" ]] || exit 1

jq -e --arg revision "$head_revision" --argjson seed "$v2_r2_sv1d_capacity_seed" \
	'.build.revision == $revision and .build.modified == false and .config.seed == $seed and
	 .config.log_mode == "full" and .config.evidence_format == "evstream_v3"' \
	"$probe_cell/manifest.json" >/dev/null || exit 1
jq -e --argjson start "$v2_r2_sv1d_capacity_simulation_start_nano" --argjson end "$v2_r2_sv1d_capacity_simulation_end_nano" \
	'(.initial_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $start)) and
	 (.terminal_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $end))' \
	"$probe_cell/greeks.json" >/dev/null || exit 1
jq -e --argjson start "$v2_r2_sv1d_capacity_simulation_start_nano" --argjson end "$v2_r2_sv1d_capacity_simulation_end_nano" \
	-f "$root_dir/scripts/v2-r2-sv1-terminal-outcome.jq" "$probe_cell/terminal-outcome.json" >/dev/null || exit 1
v2_r2_terminal_completed_outcome_present "$probe_cell" || exit 1
v2_r2_require_checkpoint_stream "$probe_cell/checkpoints.jsonl" \
	"$v2_r2_sv1d_capacity_simulation_start_nano" "$v2_r2_sv1d_capacity_simulation_end_nano" evstream_v3 || exit 1
v2_r2_require_binary_checkpoint_stream_exact "$probe_cell/checkpoints.jsonl" \
	"$v2_r2_sv1d_capacity_simulation_start_nano" "$v2_r2_sv1d_capacity_simulation_end_nano" \
	"$probe_cell/binary-evidence-attestation.json" || exit 1
v2_r2_write_evidence_manifest "$probe_cell" || exit 1
v2_r2_verify_evidence_manifest "$probe_cell" || exit 1

final_available_free_bytes=$(v2_r2_sv1d_capacity_free_bytes "$probe_root") || exit 1
final_memory_available_bytes=$(memory_available_bytes) || exit 1
retained_output_bytes=$(directory_bytes "$probe_root") || exit 1
(( retained_output_bytes > peak_output_bytes )) && peak_output_bytes=$retained_output_bytes
(( final_available_free_bytes >= minimum_free_bytes )) || exit 1
(( final_memory_available_bytes >= minimum_memory_available_bytes )) || exit 1
required_free_bytes=$((peak_output_bytes + safety_margin_bytes))
(( final_available_free_bytes >= required_free_bytes )) || {
	echo "SV1D capacity floor failed: available=$final_available_free_bytes required=$required_free_bytes; output retained at $probe_root" >&2
	exit 1
}
(( peak_rss_bytes > 0 )) || exit 1

source_tree_sha256=$(v2_r2_sv1d_git_tree_sha256 "$head_revision") || exit 1
evidence_manifest_sha256=$(v2_r2_sv1d_sha256_file "$probe_cell/evidence-manifest.json") || exit 1
stdout_sha256=$(v2_r2_sv1d_sha256_file "$probe_cell/simulator.stdout.log") || exit 1
stderr_sha256=$(v2_r2_sv1d_sha256_file "$probe_cell/simulator.stderr.log") || exit 1
attestation_tmp="$attestation.tmp-$$"
[[ ! -e "$attestation_tmp" && ! -L "$attestation_tmp" ]] || exit 1
jq -n --arg contract "$v2_r2_sv1d_capacity_attestation_contract" --arg revision "$head_revision" \
	--arg tree "$source_tree_sha256" --arg binary_sha256 "$binary_sha256" --arg config_path "$config_relative" \
	--arg config_sha256 "$config_sha256" --arg review_path "$review_path" --arg review_sha256 "$review_sha256" \
	--arg probe_root "$probe_root" --arg probe_cell "$(basename -- "$probe_cell")" \
	--arg validator_path "$checkpoint_validator" --arg validator_revision "$head_revision" --arg validator_sha256 "$checkpoint_validator_sha256" \
	--arg evidence_manifest_sha256 "$evidence_manifest_sha256" --arg stdout_sha256 "$stdout_sha256" --arg stderr_sha256 "$stderr_sha256" \
	--arg peak_output_at "$peak_output_at" --arg peak_rss_at "$peak_rss_at" \
	--argjson seed "$v2_r2_sv1d_capacity_seed" --arg horizon "$v2_r2_sv1d_capacity_horizon" \
	--argjson start "$v2_r2_sv1d_capacity_simulation_start_nano" --argjson end "$v2_r2_sv1d_capacity_simulation_end_nano" \
	--argjson peak_output "$peak_output_bytes" --argjson safety_margin "$safety_margin_bytes" --argjson required_free "$required_free_bytes" \
	--argjson initial_free "$initial_available_free_bytes" --argjson available_free "$final_available_free_bytes" \
	--argjson peak_rss "$peak_rss_bytes" --argjson initial_memory "$initial_memory_available_bytes" --argjson final_memory "$final_memory_available_bytes" \
	--argjson gomaxprocs "$v2_r2_sv1d_capacity_gomaxprocs" --argjson memory_limit "$memory_limit_bytes" --argjson gomemlimit "$gomemlimit_bytes" \
	--argjson host_memory "$host_memory_total_bytes" --argjson minimum_memory "$minimum_memory_available_bytes" \
	--argjson host_cpu "$host_cpu_count" --argjson allowed_cpu "$allowed_cpu_count" --argjson cpu_limit "$v2_r2_sv1_cpu_limit_percent" \
	--arg affinity "$cpu_affinity" --argjson minimum_free "$minimum_free_bytes" --argjson max_wall "$max_wall_seconds" --argjson wall_clock "$wall_clock_seconds" \
	'{schema_version: 1, contract: $contract, measurement: "full_24h_binary_evidence_capacity_probe",
	 evidence_format: "evstream_v3", log_mode: "full", source_revision: $revision,
	 source_tree_sha256: $tree, binary_sha256: $binary_sha256, measurement_seed: $seed, source_config_seed: $seed,
	 measurement_config_path: $config_path, measurement_config_sha256: $config_sha256,
	 launch_config_path: $config_path, launch_config_sha256: $config_sha256, config_sha256: $config_sha256,
	 capacity_only: true, calibration_only: false, simulated_horizon: $horizon,
	 simulation_start_nano: $start, simulation_end_nano: $end, holdouts_consumed: false,
	 review: {path: $review_path, sha256: $review_sha256},
	 checkpoint_validator: {path: $validator_path, revision: $validator_revision, sha256: $validator_sha256},
	 probe_root: $probe_root, probe_cell: $probe_cell, evidence_manifest_sha256: $evidence_manifest_sha256,
	 simulator_stdout_sha256: $stdout_sha256, simulator_stderr_sha256: $stderr_sha256,
	 peak_output_bytes: $peak_output, safety_margin_bytes: $safety_margin, required_free_bytes: $required_free,
	 initial_available_free_bytes: $initial_free, available_free_bytes: $available_free,
	 initial_memory_available_bytes: $initial_memory, final_memory_available_bytes: $final_memory,
	 wall_clock_seconds: $wall_clock,
	 peak_observed_at: $peak_output_at, peak_rss_bytes: $peak_rss, peak_rss_observed_at: $peak_rss_at,
	 resource_policy: {gomaxprocs: $gomaxprocs, memory_limit_bytes: $memory_limit, gomemlimit_bytes: $gomemlimit,
	   host_memory_total_bytes: $host_memory, minimum_memory_available_bytes: $minimum_memory,
	   host_cpu_count: $host_cpu, allowed_cpu_count: $allowed_cpu, cpu_limit_percent: $cpu_limit,
	   cpu_affinity: $affinity, minimum_free_bytes: $minimum_free, max_wall_seconds: $max_wall}}' \
	>"$attestation_tmp"
v2_r2_sv1d_require_capacity_attestation "$attestation_tmp" "$head_revision" "$binary_sha256" "$config_sha256" \
	"$review_path" "$review_sha256" || {
	echo "generated SV1D capacity attestation failed validation; output retained at $probe_root" >&2
	exit 1
}
mv -- "$attestation_tmp" "$attestation"
v2_r2_sv1d_require_capacity_attestation "$attestation" "$head_revision" "$binary_sha256" "$config_sha256" \
	"$review_path" "$review_sha256" || {
	echo "published SV1D capacity attestation failed validation; output retained at $probe_root" >&2
	exit 1
}
echo "completed SV1D 24-hour binary capacity probe: root=$probe_root peak_bytes=$peak_output_bytes required_free_bytes=$required_free_bytes"
