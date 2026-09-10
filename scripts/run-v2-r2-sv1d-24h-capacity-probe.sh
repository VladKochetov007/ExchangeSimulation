#!/usr/bin/env bash
# Measure the registered binary evidence footprint with a synthetic workload.
# This path is outcome-neutral: it never starts the market simulator.
set -euo pipefail

if [[ $# -gt 1 ]]; then
	echo "usage: $0 [evscapacity-binary]" >&2
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
	echo "SV1D synthetic capacity received an unknown candidate" >&2
	exit 1
}
[[ -z "$(printenv EXSIM_BINARY_EVIDENCE 2>/dev/null || true)" ]] || {
	echo "SV1D synthetic capacity refuses prototype evidence overrides" >&2
	exit 1
}
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || {
	echo "SV1D synthetic capacity requires a clean scientific worktree" >&2
	exit 1
}

go_bin_dir=/usr/local/go/bin
[[ -x "$go_bin_dir/go" ]] || go_bin_dir=$(dirname -- "$(command -v go)")
PATH="$go_bin_dir:$PATH"
export PATH
head_revision=$(git -C "$root_dir" rev-parse HEAD) || exit 1
[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || exit 1

capacity_binary="$v2_r2_sv1d_capacity_binary"
if [[ $# -eq 1 ]]; then
	capacity_binary="$1"
fi
[[ "$capacity_binary" == /* && "$capacity_binary" != */ && "$capacity_binary" != *$'\n'* &&
	"$capacity_binary" != *$'\t'* && -x "$capacity_binary" && ! -L "$capacity_binary" &&
	"$(realpath -e -- "$capacity_binary")" == "$capacity_binary" ]] || {
	echo "missing or non-canonical synthetic capacity binary: $capacity_binary" >&2
	exit 1
}
capacity_binary_sha256=$(v2_r2_sv1d_sha256_file "$capacity_binary") || exit 1
v2_r2_sv1d_require_pinned_binary "$capacity_binary" "$head_revision" "$capacity_binary_sha256" \
	"$v2_r2_sv1d_capacity_binary_package" || exit 1

target_config="$v2_r2_sv1d_capacity_config"
target_config_relative="research/configs/v2-r2-sv1d-activation/activation-659-treatment.json"
[[ "$(realpath -e -- "$target_config")" == "$root_dir/$target_config_relative" && -s "$target_config" && ! -L "$target_config" ]] || exit 1
target_config_sha256=$(v2_r2_sv1d_sha256_file "$target_config") || exit 1

review_path=$(v2_r2_sv1d_review_attestation_path "$head_revision") || exit 1
v2_r2_require_sv1b_review_attestation "$review_path" "$head_revision" || {
	echo "SV1D synthetic capacity requires accepted exact-tree independent review: $review_path" >&2
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
	awk '$1 == "MemAvailable:" {printf "%.0f\n", $2 * 1024; exit}' /proc/meminfo
}

directory_bytes() {
	du -sb -- "$1" | awk 'NR == 1 {print $1}'
}

process_group_rss_bytes() {
	local process_id=$1 process_group_id child_id child_group_id child_rss_kib total=0
	process_group_id=$(ps -o pgid= -p "$process_id" 2>/dev/null | tr -d ' ') || return 1
	[[ "$process_group_id" =~ ^[1-9][0-9]*$ ]] || return 1
	while read -r child_id child_group_id; do
		[[ "$child_id" =~ ^[1-9][0-9]*$ && "$child_group_id" == "$process_group_id" ]] || continue
		child_rss_kib=$(awk '$1 == "VmRSS:" {print $2; exit}' "/proc/$child_id/status" 2>/dev/null || true)
		[[ "$child_rss_kib" =~ ^[0-9]+$ ]] || continue
		total=$((total + child_rss_kib * 1024))
	done < <(ps -e -o pid=,pgid= 2>/dev/null)
	printf '%s\n' "$total"
}

workload_pid=""
terminate_workload() {
	local process_id=$1 process_group_id
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

cleanup_capacity() {
	local status=$?
	trap - EXIT INT TERM HUP
	if [[ -n "$workload_pid" ]]; then
		terminate_workload "$workload_pid"
	fi
	exit "$status"
}
trap cleanup_capacity EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP

v2_r2_acquire_namespace_lock || {
	echo "could not acquire the SV1D evidence namespace lock" >&2
	exit 1
}
probe_root=$(v2_r2_sv1d_capacity_probe_root "$head_revision") || exit 1
attestation=$(v2_r2_sv1d_capacity_attestation_path "$head_revision") || exit 1
[[ "$probe_root" == /* && "$probe_root" != "$scientific_root" && "$probe_root" != "$scientific_root"/* &&
	"$attestation" == /* && "$attestation" != "$scientific_root"/* &&
	! -e "$probe_root" && ! -L "$probe_root" && ! -e "$attestation" && ! -L "$attestation" ]] || {
	echo "SV1D synthetic capacity output already exists or is inside the repository" >&2
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

start_epoch=$(date +%s)
setsid --wait taskset --cpu-list "$cpu_affinity" env GOMAXPROCS="$GOMAXPROCS" \
	GOMEMLIMIT="$gomemlimit_bytes"B prlimit --as="$memory_limit_bytes" -- \
	"$capacity_binary" \
	-profile "$v2_r2_sv1d_capacity_workload_profile" \
	-seed "$v2_r2_sv1d_capacity_workload_seed" \
	-event-count "$v2_r2_sv1d_capacity_event_count" \
	-start-nano "$v2_r2_sv1d_capacity_workload_start_nano" \
	-end-nano "$v2_r2_sv1d_capacity_workload_end_nano" \
	-out "$probe_cell/events.evs" \
	-report "$probe_cell/synthetic-capacity-report.json" \
	-attestation "$probe_cell/binary-evidence-attestation.json" \
	-profile-out "$probe_cell/workload-profile.json" \
	>"$probe_cell/capacity.stdout.log" 2>"$probe_cell/capacity.stderr.log" &
workload_pid=$!
peak_output_bytes=0
peak_rss_bytes=0
peak_output_at=""
peak_rss_at=""
resource_guard_reason=""
while kill -0 "$workload_pid" 2>/dev/null; do
	wall_seconds=$(( $(date +%s) - start_epoch ))
	if (( wall_seconds > max_wall_seconds )); then
		resource_guard_reason="synthetic capacity workload exceeded the $max_wall_seconds-second wall-clock limit"
		terminate_workload "$workload_pid"
		break
	fi
	current_output_bytes=$(directory_bytes "$probe_root") || {
		resource_guard_reason="synthetic capacity output-size measurement failed"
		terminate_workload "$workload_pid"
		break
	}
	if (( current_output_bytes > peak_output_bytes )); then
		peak_output_bytes=$current_output_bytes
		peak_output_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	fi
	current_rss_bytes=$(process_group_rss_bytes "$workload_pid") || {
		resource_guard_reason="synthetic capacity RSS measurement failed"
		terminate_workload "$workload_pid"
		break
	}
	if (( current_rss_bytes <= 0 )); then
		resource_guard_reason="synthetic capacity process-group RSS was unavailable"
		terminate_workload "$workload_pid"
		break
	fi
	if (( current_rss_bytes > peak_rss_bytes )); then
		peak_rss_bytes=$current_rss_bytes
		peak_rss_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	fi
	if (( current_rss_bytes > memory_limit_bytes )); then
		resource_guard_reason="synthetic capacity RSS crossed the hard memory limit"
		terminate_workload "$workload_pid"
		break
	fi
	current_memory_available_bytes=$(memory_available_bytes) || {
		resource_guard_reason="available-memory measurement failed"
		terminate_workload "$workload_pid"
		break
	}
	if (( current_memory_available_bytes < minimum_memory_available_bytes )); then
		resource_guard_reason="available RAM crossed the registered safety floor"
		terminate_workload "$workload_pid"
		break
	fi
	sleep 1
done
set +e
wait "$workload_pid"
command_status=$?
set -e
workload_pid=""
if (( command_status != 0 )) || [[ -n "$resource_guard_reason" ]]; then
	if [[ -n "$resource_guard_reason" ]]; then
		echo "synthetic capacity failed; output retained at $probe_root: $resource_guard_reason" >&2
	else
		echo "synthetic capacity failed; output retained at $probe_root: command status $command_status" >&2
	fi
	exit 1
fi

final_available_free_bytes=$(v2_r2_sv1d_capacity_free_bytes "$probe_root") || exit 1
final_memory_available_bytes=$(memory_available_bytes) || exit 1
(( peak_output_bytes > 0 && peak_rss_bytes > 0 )) || exit 1
required_free_bytes=$((peak_output_bytes + safety_margin_bytes))
(( final_available_free_bytes >= required_free_bytes )) || exit 1
(( final_memory_available_bytes >= minimum_memory_available_bytes )) || exit 1

report_path="$probe_cell/synthetic-capacity-report.json"
profile_path="$probe_cell/workload-profile.json"
binary_attestation_path="$probe_cell/binary-evidence-attestation.json"
report_sha256=$(v2_r2_sv1d_sha256_file "$report_path") || exit 1
profile_sha256=$(v2_r2_sv1d_sha256_file "$profile_path") || exit 1
binary_attestation_sha256=$(v2_r2_sv1d_sha256_file "$binary_attestation_path") || exit 1
stream_sha256=$(v2_r2_sv1d_sha256_file "$probe_cell/events.evs") || exit 1
stream_bytes=$(stat -c '%s' -- "$probe_cell/events.evs") || exit 1

jq -n --arg revision "$head_revision" --arg target_config_path "$target_config_relative" \
	--arg target_config_sha256 "$target_config_sha256" --arg profile "$v2_r2_sv1d_capacity_workload_profile" \
	--argjson seed "$v2_r2_sv1d_capacity_workload_seed" --argjson event_count "$v2_r2_sv1d_capacity_event_count" \
	--argjson start "$v2_r2_sv1d_capacity_workload_start_nano" --argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
	--argjson stream_bytes "$stream_bytes" --arg stream_sha256 "$stream_sha256" \
	'{schema_version: 1, contract: "v2-r2-sv1d-synthetic-capacity-run-v1", source_revision: $revision,
	 target_config_path: $target_config_path, target_config_sha256: $target_config_sha256,
	 workload_profile: $profile, workload_seed: $seed, event_count: $event_count,
	 workload_start_nano: $start, workload_end_nano: $end, workload_horizon: "24h",
	 capacity_only: true, outcome_neutral: true, simulator_invoked: false, terminal_outcome_present: false,
	 holdouts_consumed: false, evidence_format: "evstream_v3", stream_bytes: $stream_bytes,
	 stream_sha256: $stream_sha256,
	 command: ["evscapacity", "-profile", $profile, "-seed", ($seed|tostring), "-event-count", ($event_count|tostring)]}' \
	>"$probe_cell/run-metadata.json"

manifest_records='[]'
while IFS= read -r relative; do
	path="$probe_cell/$relative"
	bytes=$(stat -c '%s' -- "$path") || exit 1
	digest=$(v2_r2_sv1d_sha256_file "$path") || exit 1
	manifest_records=$(jq -c --arg path "$relative" --arg digest "$digest" --argjson bytes "$bytes" \
		'. + [{path: $path, bytes: $bytes, sha256: $digest}]' <<<"$manifest_records") || exit 1
done < <(v2_r2_sv1d_capacity_expected_cell_files | grep -v '^evidence-manifest.json$')
jq -n --arg cell "$(basename -- "$probe_cell")" --argjson files "$manifest_records" \
	'{schema_version: 1, contract: "v2-r2-sv1d-synthetic-capacity-evidence-manifest-v1", cell: $cell,
	 outcome_neutral: true, simulator_invoked: false, holdouts_consumed: false, files: $files}' \
	>"$probe_cell/evidence-manifest.json"
v2_r2_sv1d_capacity_verify_manifest "$probe_cell" || exit 1
evidence_manifest_sha256=$(v2_r2_sv1d_sha256_file "$probe_cell/evidence-manifest.json") || exit 1

source_tree_sha256=$(v2_r2_sv1d_git_tree_sha256 "$head_revision") || exit 1
attestation_tmp="$attestation.tmp-$$"
jq -n --arg contract "$v2_r2_sv1d_capacity_attestation_contract" \
	--arg revision "$head_revision" --arg tree "$source_tree_sha256" \
	--arg binary_path "$capacity_binary" --arg binary_sha256 "$capacity_binary_sha256" \
	--arg target_config_path "$target_config_relative" --arg target_config_sha256 "$target_config_sha256" \
	--arg review_path "$review_path" --arg review_sha256 "$review_sha256" \
	--arg probe_root "$probe_root" --arg probe_cell "$(basename -- "$probe_cell")" \
	--arg attestation_path "$attestation" --arg profile "$v2_r2_sv1d_capacity_workload_profile" \
	--argjson seed "$v2_r2_sv1d_capacity_workload_seed" --argjson event_count "$v2_r2_sv1d_capacity_event_count" \
	--argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" --argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" \
	--argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" \
	--argjson start "$v2_r2_sv1d_capacity_workload_start_nano" --argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
	--argjson peak_output "$peak_output_bytes" --arg peak_output_at "$peak_output_at" \
	--argjson peak_rss "$peak_rss_bytes" --arg peak_rss_at "$peak_rss_at" \
	--argjson stream_bytes "$stream_bytes" --arg report_sha256 "$report_sha256" --arg profile_sha256 "$profile_sha256" \
	--arg binary_attestation_sha256 "$binary_attestation_sha256" --arg stream_sha256 "$stream_sha256" \
	--arg manifest_sha256 "$evidence_manifest_sha256" --argjson initial_free "$initial_available_free_bytes" \
	--argjson final_free "$final_available_free_bytes" --argjson required_free "$required_free_bytes" \
	--argjson initial_memory "$initial_memory_available_bytes" --argjson final_memory "$final_memory_available_bytes" \
	--argjson host_memory "$host_memory_total_bytes" --argjson minimum_memory "$minimum_memory_available_bytes" \
	--argjson host_cpu "$host_cpu_count" --argjson allowed_cpu "$allowed_cpu_count" --arg affinity "$cpu_affinity" \
	--argjson gomaxprocs "$v2_r2_sv1d_capacity_gomaxprocs" --argjson memory_limit "$memory_limit_bytes" \
	--argjson gomemlimit "$gomemlimit_bytes" --argjson minimum_free "$minimum_free_bytes" \
	--argjson safety_margin "$safety_margin_bytes" --argjson max_wall "$max_wall_seconds" \
	--argjson wall_seconds "$(( $(date +%s) - start_epoch ))" --argjson cpu_limit "$v2_r2_sv1_cpu_limit_percent" \
	--arg stdout_sha256 "$(v2_r2_sv1d_sha256_file "$probe_cell/capacity.stdout.log")" \
	--arg stderr_sha256 "$(v2_r2_sv1d_sha256_file "$probe_cell/capacity.stderr.log")" \
	'{schema_version: 1, contract: $contract, measurement: "synthetic_24h_binary_evidence_capacity_probe",
	 source_revision: $revision, source_tree_sha256: $tree, capacity_only: true, calibration_only: false,
	 outcome_neutral: true, simulator_invoked: false, terminal_outcome_present: false, holdouts_consumed: false,
	 capacity_binary: {path: $binary_path, sha256: $binary_sha256},
	 target_config: {path: $target_config_path, sha256: $target_config_sha256},
	 review: {path: $review_path, sha256: $review_sha256}, probe_root: $probe_root, probe_cell: $probe_cell,
	 attestation_path: $attestation_path,
	 workload: {profile: $profile, seed: $seed, event_count: $event_count,
	   book_delta_events: $book_events, balance_change_events: $balance_events,
	   opaque_scientific_events: $opaque_events, start_nano: $start, end_nano: $end, horizon: "24h"},
	 evidence_format: "evstream_v3", hashing: "route_and_global_sequence_neutral_v2", ordering: "ordered_stream",
	 stream_bytes: $stream_bytes, stream_sha256: $stream_sha256, report_sha256: $report_sha256,
	 profile_sha256: $profile_sha256, binary_attestation_sha256: $binary_attestation_sha256,
	 evidence_manifest_sha256: $manifest_sha256, peak_output_bytes: $peak_output,
	 peak_output_at: $peak_output_at, peak_rss_bytes: $peak_rss, peak_rss_at: $peak_rss_at,
	 initial_available_free_bytes: $initial_free, final_available_free_bytes: $final_free,
	 available_free_bytes: $final_free, required_free_bytes: $required_free,
	 minimum_free_bytes: $minimum_free, safety_margin_bytes: $safety_margin,
	 initial_memory_available_bytes: $initial_memory, final_memory_available_bytes: $final_memory,
	 wall_clock_seconds: $wall_seconds, stdout_sha256: $stdout_sha256, stderr_sha256: $stderr_sha256,
	 resource_policy: {gomaxprocs: $gomaxprocs, memory_limit_bytes: $memory_limit,
	   gomemlimit_bytes: $gomemlimit, cpu_limit_percent: $cpu_limit,
	   minimum_free_bytes: $minimum_free, minimum_memory_available_bytes: $minimum_memory,
	   host_memory_total_bytes: $host_memory, host_cpu_count: $host_cpu,
	   allowed_cpu_count: $allowed_cpu, cpu_affinity: $affinity, max_wall_seconds: $max_wall},
	 command: ["evscapacity", "-profile", $profile, "-seed", ($seed|tostring), "-event-count", ($event_count|tostring)]}' \
	>"$attestation_tmp"

v2_r2_sv1d_require_capacity_attestation "$attestation_tmp" "$head_revision" "$capacity_binary_sha256" \
	"$target_config_sha256" "$review_path" "$review_sha256" || {
	echo "synthetic capacity attestation failed closed; output retained at $probe_root" >&2
	exit 1
}
mv -- "$attestation_tmp" "$attestation"
echo "SV1D synthetic binary capacity measured: $attestation"
