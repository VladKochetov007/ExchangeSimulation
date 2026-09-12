#!/usr/bin/env bash
# Run the registered development-only SV1D tri-arm activation probe.
#
# This adapter is deliberately narrower than the 24-hour campaign runner:
# treatment, same-roster mode-off, and no-roster are the only accepted arms;
# every arm gets a fresh namespace; and the command refuses to start until an
# externally authenticated exact-tree review and a separately measured binary
# capacity preflight have been supplied explicitly.
set -euo pipefail

if [[ $# -gt 4 ]]; then
	echo "usage: $0 [multivenue-binary] [sv1dprobe-binary] [evsrender-binary] [sv1dlock-binary]" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-r2-sv1d-activation"
multivenue_binary=${1:-"$root_dir/bin/multivenue"}
sv1dprobe_binary=${2:-"$root_dir/bin/sv1dprobe"}
evsrender_binary=${3:-"$root_dir/bin/evsrender"}
lock_binary=${4:-"$root_dir/bin/sv1dlock"}
output_root=${SV1D_OUTPUT_ROOT:-"/home/vlad/v2-r2-sv1d-activation-659-v1"}
review_attestation=${SV1D_REVIEW_ATTESTATION:-}
review_report=${SV1D_REVIEW_REPORT:-}
trusted_review_key=${SV1D_TRUSTED_REVIEW_KEY:-}
capacity_attestation=${SV1D_CAPACITY_PREFLIGHT_ATTESTATION:-}
capacity_root=${SV1D_CAPACITY_PREFLIGHT_ROOT:-""}
lock_path="/home/vlad/v2-r2-sv1d-activation-659.lock"

fail() {
	echo "SV1D activation runner: $*" >&2
	exit 1
}

normalize_input_path() {
	local path=$1
	[[ -n "$path" ]] || return 1
	if [[ "$path" != /* ]]; then
		path="$PWD/$path"
	fi
	realpath -ms -- "$path"
}

require_no_symlink_components() {
	local path=$1 current=/ component
	local path_without_root=${path#/}
	IFS=/ read -r -a components <<< "$path_without_root"
	for component in "${components[@]}"; do
		[[ -n "$component" ]] || continue
		current="${current%/}/$component"
		[[ ! -L "$current" ]] || return 1
	done
}

require_binary() {
	local name=$1 path=$2
	[[ -f "$path" && ! -L "$path" && -x "$path" ]] || fail "missing executable $name: $path"
	require_no_symlink_components "$path" || fail "$name path contains a symlink: $path"
}

copy_immutable_file() {
	local name=$1 source=$2 destination=$3
	require_regular_file "$name" "$source"
	[[ ! -e "$destination" && ! -L "$destination" ]] || fail "refusing to overwrite staged $name"
	cp -- "$source" "$destination"
	cmp -s "$source" "$destination" || fail "$name changed during immutable copy"
}

copy_immutable_binary() {
	local name=$1 source=$2 destination=$3
	require_binary "$name" "$source"
	[[ ! -e "$destination" && ! -L "$destination" ]] || fail "refusing to overwrite staged $name"
	cp -- "$source" "$destination"
	chmod 0555 -- "$destination"
	cmp -s "$source" "$destination" || fail "$name changed during immutable copy"
}

require_regular_file() {
	local name=$1 path=$2
	[[ -f "$path" && ! -L "$path" ]] || fail "missing regular file $name: $path"
	require_no_symlink_components "$path" || fail "$name path contains a symlink: $path"
}

build_field() {
	local binary=$1 field=$2
	go version -m "$binary" | awk -v field="$field" '$1 == "build" && index($2, field) == 1 {sub(field, "", $2); print $2; exit}'
}

binary_go_version() {
	go version -m "$1" | awk '$1 == "go" {print $2; exit}'
}

require_clean_pinned_binary() {
	local name=$1 binary=$2 expected_revision=$3
	local revision modified trimpath cgo_enabled goos goarch goamd64 go_version
	revision=$(build_field "$binary" 'vcs.revision=')
	modified=$(build_field "$binary" 'vcs.modified=')
	trimpath=$(build_field "$binary" '-trimpath=')
	cgo_enabled=$(build_field "$binary" 'CGO_ENABLED=')
	goos=$(build_field "$binary" 'GOOS=')
	goarch=$(build_field "$binary" 'GOARCH=')
	goamd64=$(build_field "$binary" 'GOAMD64=')
	go_version=$(binary_go_version "$binary")
	[[ "$revision" == "$expected_revision" ]] || fail "$name VCS revision $revision does not match HEAD $expected_revision"
	[[ "$modified" == false ]] || fail "$name has vcs.modified=$modified"
	[[ "$trimpath" == true && "$cgo_enabled" == 0 ]] || fail "$name is not trimpath/CGO_ENABLED=0"
	[[ "$goos" == linux && "$goarch" == amd64 && "$goamd64" == v1 ]] || fail "$name is not linux/amd64/v1"
	[[ "$go_version" == go1.27* ]] || fail "$name is not built with Go 1.27: $go_version"
}

hash_file() {
	sha256sum -- "$1" | awk '{print $1}'
}

publish_incomplete_arm() {
	local arm=$1 result_path=$2 reason=$3
	[[ ! -e "$result_path" && ! -L "$result_path" ]] || return 1
	"$sv1dprobe_binary" -mode failure -out "$result_path" -plan "$plan_path" -arm "$arm" -failure-reason "$reason" \
		-source-revision "$source_revision" -tree-revision "$tree_revision" -plan-sha256 "$review_plan_sha256" \
		-parent-registration-sha256 "$parent_registration_sha256" -amendment-sha256 "$amendment_sha256" \
		-activation-metadata "$activation_metadata" -review-attestation-sha256 "$review_attestation_sha256" \
		-review-report-sha256 "$review_report_sha256" -capacity-attestation-sha256 "$capacity_attestation_sha256" \
		-capacity-records-sha256 "$capacity_records_sha256" -capacity-runner-sha256 "$capacity_runner_sha256" \
		-activation-runner-sha256 "$activation_runner_sha256" -activation-metadata-sha256 "$activation_metadata_sha256" \
		-trusted-review-key-sha256 "$trusted_review_key_sha256" -evidence-schema-epoch 4 -gomaxprocs 2 -gomemlimit 4GiB
}

activation_cgroup_path=""
activation_baseline_oom_events=""
activation_baseline_oom_kill_events=""
resource_violation_reason=""
stage_resource_path=""
stage_name=""
stage_samples=0
stage_peak_cgroup_memory_bytes=0
stage_min_host_available_bytes=0
stage_min_filesystem_available_bytes=0
stage_max_swap_bytes=0

read_cgroup_event() {
	local event_name=$1
	awk -v event_name="$event_name" '$1 == event_name {print $2; exit}' "$activation_cgroup_path/memory.events"
}

require_live_resource_envelope() {
	local expected_limit=$1
	local cgroup_relative current_limit swap_total
	cgroup_relative=$(awk -F: '$1 == "0" {print $3; exit}' /proc/self/cgroup)
	[[ -n "$cgroup_relative" && "$cgroup_relative" != *$'\n'* ]] || fail "could not resolve the current cgroup"
	activation_cgroup_path="/sys/fs/cgroup$cgroup_relative"
	[[ -f "$activation_cgroup_path/memory.max" && -f "$activation_cgroup_path/memory.current" && -f "$activation_cgroup_path/memory.swap.current" && -f "$activation_cgroup_path/memory.events" ]] || fail "current cgroup lacks the registered memory controls"
	current_limit=$(<"$activation_cgroup_path/memory.max")
	[[ "$current_limit" =~ ^[0-9]+$ && "$current_limit" -gt 0 ]] || fail "activation must run inside a finite memory cgroup"
	[[ "$current_limit" == "$expected_limit" ]] || fail "activation cgroup limit differs from measured capacity envelope"
	swap_total=$(awk '$1 == "SwapTotal:" {print $2; exit}' /proc/meminfo)
	[[ "$swap_total" == 0 ]] || fail "activation host exposes swap although the capacity contract forbids it"
	[[ "$(<"$activation_cgroup_path/memory.swap.current")" == 0 ]] || fail "activation cgroup has non-zero swap usage"
	activation_baseline_oom_events=$(read_cgroup_event oom)
	activation_baseline_oom_kill_events=$(read_cgroup_event oom_kill)
	[[ "$activation_baseline_oom_events" =~ ^[0-9]+$ && "$activation_baseline_oom_kill_events" =~ ^[0-9]+$ ]] || fail "activation cgroup has no readable OOM counters"
}

active_resource_check() {
	local current_memory swap_current host_available_kb available_kb oom_events oom_kill_events
	resource_violation_reason=""
	if [[ -z "$activation_cgroup_path" ]]; then
		resource_violation_reason="activation cgroup is not initialized"
		return 1
	fi
	current_memory=$(<"$activation_cgroup_path/memory.current") || {
		resource_violation_reason="could not read activation cgroup memory.current"
		return 1
	}
	if [[ ! "$current_memory" =~ ^[0-9]+$ ]] || (( current_memory > capacity_memory_limit_bytes )); then
		resource_violation_reason="activation cgroup memory exceeded measured limit"
		return 1
	fi
	stage_samples=$((stage_samples + 1))
	if (( current_memory > stage_peak_cgroup_memory_bytes )); then
		stage_peak_cgroup_memory_bytes=$current_memory
	fi
	swap_current=$(<"$activation_cgroup_path/memory.swap.current") || {
		resource_violation_reason="could not read activation cgroup swap usage"
		return 1
	}
	if [[ "$swap_current" != 0 ]]; then
		resource_violation_reason="activation cgroup swap usage became non-zero"
		return 1
	fi
	if (( swap_current > stage_max_swap_bytes )); then
		stage_max_swap_bytes=$swap_current
	fi
	host_available_kb=$(awk '$1 == "MemAvailable:" {print $2; exit}' /proc/meminfo) || {
		resource_violation_reason="could not read host available memory"
		return 1
	}
	if [[ ! "$host_available_kb" =~ ^[0-9]+$ ]] || (( host_available_kb * 1024 < required_available_memory_bytes )); then
		resource_violation_reason="host available memory fell below measured floor"
		return 1
	fi
	if (( stage_min_host_available_bytes == 0 || host_available_kb * 1024 < stage_min_host_available_bytes )); then
		stage_min_host_available_bytes=$((host_available_kb * 1024))
	fi
	available_kb=$(df -Pk -- "$capacity_output_parent" | awk 'NR == 2 {print $4}') || {
		resource_violation_reason="could not read activation filesystem free space"
		return 1
	}
	if [[ ! "$available_kb" =~ ^[0-9]+$ ]] || (( available_kb * 1024 < required_free_bytes )); then
		resource_violation_reason="activation filesystem free space fell below measured floor"
		return 1
	fi
	if (( stage_min_filesystem_available_bytes == 0 || available_kb * 1024 < stage_min_filesystem_available_bytes )); then
		stage_min_filesystem_available_bytes=$((available_kb * 1024))
	fi
	oom_events=$(read_cgroup_event oom)
	oom_kill_events=$(read_cgroup_event oom_kill)
	if [[ ! "$oom_events" =~ ^[0-9]+$ || ! "$oom_kill_events" =~ ^[0-9]+$ ]] ||
		(( oom_events != activation_baseline_oom_events || oom_kill_events != activation_baseline_oom_kill_events )); then
		resource_violation_reason="activation cgroup OOM counters changed"
		return 1
	fi
	return 0
}

write_resource_record() {
	local path=$1 stage=$2 exit_status=$3 monitor_status=$4 reason=$5
	local oom_events oom_kill_events record_tmp
	[[ -n "$path" && ! -e "$path" && ! -L "$path" ]] || return 1
	oom_events=$(read_cgroup_event oom) || return 1
	oom_kill_events=$(read_cgroup_event oom_kill) || return 1
	[[ "$oom_events" =~ ^[0-9]+$ && "$oom_kill_events" =~ ^[0-9]+$ ]] || return 1
	record_tmp="$path.tmp-$$"
	jq -S -n \
		--arg stage "$stage" --arg path "${path#"$output_root"/}" --arg reason "$reason" \
		--argjson exit_status "$exit_status" --argjson monitor_status "$monitor_status" \
		--argjson sample_count "$stage_samples" --argjson peak_cgroup_memory_bytes "$stage_peak_cgroup_memory_bytes" \
		--argjson cgroup_memory_limit_bytes "$capacity_memory_limit_bytes" \
		--argjson minimum_host_available_bytes "$stage_min_host_available_bytes" \
		--argjson minimum_filesystem_available_bytes "$stage_min_filesystem_available_bytes" \
		--argjson maximum_swap_bytes "$stage_max_swap_bytes" \
		--argjson oom_events_delta "$((oom_events - activation_baseline_oom_events))" \
		--argjson oom_kill_events_delta "$((oom_kill_events - activation_baseline_oom_kill_events))" \
		'{schema_version: 1, contract: "v2-r2-sv1d-activation-resource-stage-v1", stage: $stage, path: $path,
			exit_status: $exit_status, monitor_status: $monitor_status, reason: $reason,
			sample_count: $sample_count, peak_cgroup_memory_bytes: $peak_cgroup_memory_bytes,
			cgroup_memory_limit_bytes: $cgroup_memory_limit_bytes,
			minimum_host_available_bytes: $minimum_host_available_bytes,
			minimum_filesystem_available_bytes: $minimum_filesystem_available_bytes,
			maximum_swap_bytes: $maximum_swap_bytes, oom_events_delta: $oom_events_delta,
			oom_kill_events_delta: $oom_kill_events_delta}' \
		>"$record_tmp" || return 1
	mv -- "$record_tmp" "$path"
}

run_monitored_command() {
	local stdout_path=$1 stderr_path=$2 resource_path=$3 command_stage=$4
	shift 4
	local child_pid child_status monitor_status reason
	stage_resource_path=$resource_path
	stage_name=$command_stage
	stage_samples=0
	stage_peak_cgroup_memory_bytes=0
	stage_min_host_available_bytes=0
	stage_min_filesystem_available_bytes=0
	stage_max_swap_bytes=0
	setsid -- "$@" >"$stdout_path" 2>"$stderr_path" &
	child_pid=$!
	while kill -0 "$child_pid" 2>/dev/null; do
		if ! active_resource_check; then
			reason=$resource_violation_reason
			printf 'activation resource monitor terminated process: %s\n' "$resource_violation_reason" >>"$stderr_path" || true
			kill -TERM -- "-$child_pid" 2>/dev/null || kill -TERM "$child_pid" 2>/dev/null || true
			sleep 0.25
			kill -KILL -- "-$child_pid" 2>/dev/null || kill -KILL "$child_pid" 2>/dev/null || true
			wait "$child_pid" 2>/dev/null || true
			write_resource_record "$stage_resource_path" "$stage_name" 125 125 "$reason" || true
			return 125
		fi
		sleep 0.25
	done
	if wait "$child_pid"; then
		child_status=0
	else
		child_status=$?
	fi
	if ! active_resource_check; then
		reason=$resource_violation_reason
		printf 'activation resource monitor rejected completed process: %s\n' "$resource_violation_reason" >>"$stderr_path" || true
		write_resource_record "$stage_resource_path" "$stage_name" "$child_status" 125 "$reason" || true
		return 125
	fi
	monitor_status=0
	write_resource_record "$stage_resource_path" "$stage_name" "$child_status" "$monitor_status" "" || return 125
	return "$child_status"
}

write_resource_manifest() {
	local manifest_path="$output_root/provenance/resource-usage-manifest.json"
	local manifest_tmp="$manifest_path.tmp-$$"
	local rows='[]' resource_path record digest bytes relative_path
	local resource_paths=()
	local expected_stage index arm stage
	for arm in treatment mode-off no-roster; do
		for stage in simulator renderer audit; do
			resource_paths+=("$output_root/logs/$arm.$stage.resource.json")
		done
	done
	resource_paths+=("$output_root/logs/score.resource.json")
	local -a expected_stages=(simulator renderer audit simulator renderer audit simulator renderer audit score)
	for index in "${!resource_paths[@]}"; do
		resource_path=${resource_paths[$index]}
		require_regular_file activation-resource-stage "$resource_path"
		relative_path=${resource_path#"$output_root"/}
		expected_stage=${expected_stages[$index]}
		record=$(jq -e --argjson limit "$capacity_memory_limit_bytes" --argjson minimum_memory "$required_available_memory_bytes" --argjson minimum_free "$required_free_bytes" \
			--arg expected_path "$relative_path" --arg expected_stage "$expected_stage" \
			'type == "object" and .schema_version == 1 and .contract == "v2-r2-sv1d-activation-resource-stage-v1" and
			 .stage == $expected_stage and .path == $expected_path and
			 .exit_status == 0 and .monitor_status == 0 and .sample_count > 0 and
			 .peak_cgroup_memory_bytes <= $limit and .cgroup_memory_limit_bytes == $limit and
			 .minimum_host_available_bytes >= $minimum_memory and .minimum_filesystem_available_bytes >= $minimum_free and
			 .maximum_swap_bytes == 0 and .oom_events_delta == 0 and .oom_kill_events_delta == 0' \
			"$resource_path") || fail "activation resource stage is incomplete or exceeded its measured envelope: $resource_path"
		digest=$(hash_file "$resource_path")
		bytes=$(stat -c '%s' -- "$resource_path")
		rows=$(jq -S -c --arg path "$relative_path" --arg sha256 "$digest" --argjson bytes "$bytes" --argjson record "$record" --argjson rows "$rows" '$rows + [{path: $path, sha256: $sha256, bytes: $bytes, record: $record}]') || return 1
	done
	local activation_metadata_sha256 score_sha256 corpus_manifest_sha256 resource_policy_sha256
	activation_metadata_sha256=$(hash_file "$activation_metadata")
	score_sha256=$(hash_file "$score_path")
	corpus_manifest_sha256=$(hash_file "$corpus_manifest_path")
	resource_policy_sha256=$(hash_file "$capacity_policy_path")
	[[ ! -e "$manifest_path" && ! -L "$manifest_path" ]] || fail "refusing to overwrite activation resource manifest"
	jq -S -n \
		--arg activation_metadata_sha256 "$activation_metadata_sha256" --arg score_sha256 "$score_sha256" \
		--arg corpus_manifest_sha256 "$corpus_manifest_sha256" --arg resource_policy_sha256 "$resource_policy_sha256" \
		--argjson cgroup_memory_limit_bytes "$capacity_memory_limit_bytes" \
		--argjson required_available_memory_bytes "$required_available_memory_bytes" --argjson required_free_bytes "$required_free_bytes" \
		--argjson stages "$rows" \
		'{schema_version: 1, contract: "v2-r2-sv1d-activation-resource-manifest-v1", scope: "sv1d_activation",
			activation_metadata_sha256: $activation_metadata_sha256, score_sha256: $score_sha256,
			score_corpus_manifest_sha256: $corpus_manifest_sha256, resource_policy_sha256: $resource_policy_sha256,
			cgroup_memory_limit_bytes: $cgroup_memory_limit_bytes, required_available_memory_bytes: $required_available_memory_bytes,
			required_free_bytes: $required_free_bytes, stages: $stages}' \
		>"$manifest_tmp" || return 1
	mv -- "$manifest_tmp" "$manifest_path"
}

multivenue_binary=$(normalize_input_path "$multivenue_binary") || fail "could not normalize multivenue binary"
sv1dprobe_binary=$(normalize_input_path "$sv1dprobe_binary") || fail "could not normalize sv1dprobe binary"
evsrender_binary=$(normalize_input_path "$evsrender_binary") || fail "could not normalize evsrender binary"
lock_binary=$(normalize_input_path "$lock_binary") || fail "could not normalize sv1dlock binary"
review_attestation=$(normalize_input_path "$review_attestation") || fail "could not normalize review attestation"
review_report=$(normalize_input_path "$review_report") || fail "could not normalize review report"
trusted_review_key=$(normalize_input_path "$trusted_review_key") || fail "could not normalize trusted review key"
capacity_attestation=$(normalize_input_path "$capacity_attestation") || fail "could not normalize capacity attestation"
if [[ -n "$capacity_root" ]]; then
	capacity_root=$(normalize_input_path "$capacity_root") || fail "could not normalize capacity root"
fi

[[ "${SV1D_PROBE_AUTHORIZED:-0}" == 1 ]] || fail "set SV1D_PROBE_AUTHORIZED=1 at the explicit development-probe boundary"

[[ -d "$root_dir/.git" ]] || fail "repository root is not a Git worktree"
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "source worktree must be clean"
source_revision=$(git -C "$root_dir" rev-parse HEAD)
[[ "$source_revision" =~ ^[0-9a-f]{40}$ ]] || fail "invalid source revision: $source_revision"
tree_revision=$(git -C "$root_dir" rev-parse HEAD^{tree})
[[ "$tree_revision" =~ ^[0-9a-f]{40}$ ]] || fail "invalid reviewed tree revision: $tree_revision"

[[ -d "$config_dir" && ! -L "$config_dir" ]] || fail "missing or symlinked SV1D config directory"
"$root_dir/scripts/check-v2-r2-sv1d-activation-configs.sh" >/dev/null || fail "registered SV1D config contract failed"

require_binary multivenue "$multivenue_binary"
require_binary sv1dprobe "$sv1dprobe_binary"
require_binary evsrender "$evsrender_binary"
require_binary sv1dlock "$lock_binary"
multivenue_binary=$(realpath -e -- "$multivenue_binary")
sv1dprobe_binary=$(realpath -e -- "$sv1dprobe_binary")
evsrender_binary=$(realpath -e -- "$evsrender_binary")
lock_binary=$(realpath -e -- "$lock_binary")
require_clean_pinned_binary multivenue "$multivenue_binary" "$source_revision"
require_clean_pinned_binary sv1dprobe "$sv1dprobe_binary" "$source_revision"
require_clean_pinned_binary evsrender "$evsrender_binary" "$source_revision"
require_clean_pinned_binary sv1dlock "$lock_binary" "$source_revision"

if [[ "${SV1D_LOCK_HELD:-0}" != 1 ]]; then
	exec "$lock_binary" -path "$lock_path" -- env SV1D_LOCK_HELD=1 SV1D_LOCK_FD=3 "$0" "$@"
fi
[[ "$(readlink "/proc/$$/fd/3" 2>/dev/null)" == "$lock_path" ]] || fail "SV1D lock was not opened by the trusted lock adapter"

require_regular_file sv1d-review-attestation "$review_attestation"
require_regular_file sv1d-review-report "$review_report"
require_regular_file trusted-review-key "$trusted_review_key"
require_regular_file SV1D-parent-registration "$root_dir/research/v2-r2-sv1d-one-sided-elastic-successor-preregistration-2026-09-10.md"
require_regular_file SV1D-amendment "$root_dir/research/v2-r2-sv1d-activation-contract-amendment-2026-09-11.md"

treatment_config="$config_dir/activation-659-treatment.json"
mode_off_config="$config_dir/activation-659-mode-off.json"
no_roster_config="$config_dir/activation-659-no-roster.json"
config_sha256() { hash_file "$1"; }

review_stage=$(mktemp -d)
[[ -d "$review_stage" && ! -L "$review_stage" ]] || fail "invalid SV1D review staging directory"
trap 'rm -rf -- "$review_stage"' EXIT
mkdir -p "$review_stage/tools" "$review_stage/configs" "$review_stage/review"
staged_multivenue_binary="$review_stage/tools/multivenue"
staged_sv1dprobe_binary="$review_stage/tools/sv1dprobe"
staged_evsrender_binary="$review_stage/tools/evsrender"
copy_immutable_binary multivenue "$multivenue_binary" "$staged_multivenue_binary"
copy_immutable_binary sv1dprobe "$sv1dprobe_binary" "$staged_sv1dprobe_binary"
copy_immutable_binary evsrender "$evsrender_binary" "$staged_evsrender_binary"
require_clean_pinned_binary multivenue "$staged_multivenue_binary" "$source_revision"
require_clean_pinned_binary sv1dprobe "$staged_sv1dprobe_binary" "$source_revision"
require_clean_pinned_binary evsrender "$staged_evsrender_binary" "$source_revision"
multivenue_binary="$staged_multivenue_binary"
sv1dprobe_binary="$staged_sv1dprobe_binary"
evsrender_binary="$staged_evsrender_binary"
multivenue_sha256=$(hash_file "$multivenue_binary")
sv1dprobe_sha256=$(hash_file "$sv1dprobe_binary")
evsrender_sha256=$(hash_file "$evsrender_binary")

staged_treatment_config="$review_stage/configs/target-treatment.json"
staged_mode_off_config="$review_stage/configs/target-mode-off.json"
staged_no_roster_config="$review_stage/configs/target-no-roster.json"
copy_immutable_file target-treatment-config "$treatment_config" "$staged_treatment_config"
copy_immutable_file target-mode-off-config "$mode_off_config" "$staged_mode_off_config"
copy_immutable_file target-no-roster-config "$no_roster_config" "$staged_no_roster_config"
staged_review_attestation="$review_stage/review/attestation.json"
staged_review_report="$review_stage/review/report.md"
staged_trusted_review_key="$review_stage/review/trusted-key.raw"
staged_capacity_attestation="$review_stage/review/capacity-attestation.json"
staged_parent_registration="$review_stage/review/parent-registration.md"
staged_amendment="$review_stage/review/amendment.md"
copy_immutable_file review-attestation "$review_attestation" "$staged_review_attestation"
copy_immutable_file review-report "$review_report" "$staged_review_report"
copy_immutable_file trusted-review-key "$trusted_review_key" "$staged_trusted_review_key"
[[ -n "$capacity_attestation" ]] || fail "a measured SV1D binary-capacity preflight is required"
require_regular_file SV1D-capacity-attestation "$capacity_attestation"
copy_immutable_file capacity-attestation "$capacity_attestation" "$staged_capacity_attestation"
capacity_attestation="$staged_capacity_attestation"
copy_immutable_file parent-registration "$root_dir/research/v2-r2-sv1d-one-sided-elastic-successor-preregistration-2026-09-10.md" "$staged_parent_registration"
copy_immutable_file amendment "$root_dir/research/v2-r2-sv1d-activation-contract-amendment-2026-09-11.md" "$staged_amendment"
parent_registration_sha256=$(hash_file "$staged_parent_registration")
amendment_sha256=$(hash_file "$staged_amendment")
review_attestation_sha256=$(hash_file "$staged_review_attestation")
review_report_sha256=$(hash_file "$staged_review_report")
trusted_review_key_sha256=$(hash_file "$staged_trusted_review_key")
expected_trusted_review_key_sha256=${SV1D_TRUSTED_REVIEW_KEY_SHA256:-}
[[ "$expected_trusted_review_key_sha256" =~ ^[0-9a-f]{64}$ ]] || fail "SV1D_TRUSTED_REVIEW_KEY_SHA256 must pin the trusted review key"
[[ "$trusted_review_key_sha256" == "$expected_trusted_review_key_sha256" ]] || fail "trusted review key does not match the pinned digest"

review_plan="$review_stage/probe-plan.json"
"$sv1dprobe_binary" -mode plan -out "$review_plan" \
	-treatment-config "$staged_treatment_config" -mode-off-config "$staged_mode_off_config" \
	-no-roster-config "$staged_no_roster_config" -source-revision "$source_revision" \
	-binary-sha256 "$multivenue_sha256" -analyzer-sha256 "$sv1dprobe_sha256" \
	-renderer-sha256 "$evsrender_sha256" || fail "could not derive the review-bound SV1D plan"
review_plan_sha256=$(jq -er '.plan_sha256 | select(test("^[0-9a-f]{64}$"))' "$review_plan") || fail "review-bound SV1D plan has no canonical digest"
"$sv1dprobe_binary" -mode verify-review \
	-review-attestation "$staged_review_attestation" -review-report "$staged_review_report" -trusted-review-key "$staged_trusted_review_key" \
	-source-revision "$source_revision" -tree-revision "$tree_revision" -plan-sha256 "$review_plan_sha256" \
	-parent-registration-sha256 "$parent_registration_sha256" -amendment-sha256 "$amendment_sha256" \
	-treatment-config "$staged_treatment_config" -mode-off-config "$staged_mode_off_config" -no-roster-config "$staged_no_roster_config" \
	-binary-sha256 "$multivenue_sha256" -analyzer-sha256 "$sv1dprobe_sha256" -renderer-sha256 "$evsrender_sha256" ||
	fail "externally authenticated exact-tree SV1D review was not accepted"

capacity_attestation_sha256=$(hash_file "$capacity_attestation")
attested_capacity_root=$(jq -er '.measurement_root' "$capacity_attestation") || fail "capacity attestation has no measurement root"
if [[ -n "$capacity_root" && "$capacity_root" != "$attested_capacity_root" ]]; then
	fail "configured capacity root differs from the attestation"
fi
capacity_root="$attested_capacity_root"
[[ -d "$capacity_root" && ! -L "$capacity_root" ]] || fail "retained capacity output root is missing"
require_no_symlink_components "$capacity_root" || fail "retained capacity output root contains a symlink"
capacity_treatment_config="$capacity_root/configs/capacity-treatment.json"
capacity_mode_off_config="$capacity_root/configs/capacity-mode-off.json"
capacity_no_roster_config="$capacity_root/configs/capacity-no-roster.json"
capacity_delta_path="$capacity_root/configs/capacity-config-delta.json"
capacity_policy_path="$capacity_root/resource-policy-v1.json"
capacity_measurer_sha256=$(jq -er '.measurer_sha256 | select(test("^[0-9a-f]{64}$"))' "$capacity_attestation") || fail "capacity attestation has no measurer identity"
capacity_measurer="$capacity_root/tools/sv1dresource-$capacity_measurer_sha256"
for capacity_file in "$capacity_treatment_config" "$capacity_mode_off_config" "$capacity_no_roster_config" "$capacity_delta_path" "$capacity_policy_path" "$capacity_measurer"; do
	require_regular_file capacity-retention "$capacity_file"
done
[[ "$(hash_file "$capacity_treatment_config")" == "$(jq -er '.capacity_treatment_config_sha256' "$capacity_attestation")" ]] || fail "capacity treatment config retention changed"
[[ "$(hash_file "$capacity_mode_off_config")" == "$(jq -er '.capacity_mode_off_config_sha256' "$capacity_attestation")" ]] || fail "capacity mode-off config retention changed"
[[ "$(hash_file "$capacity_no_roster_config")" == "$(jq -er '.capacity_no_roster_config_sha256' "$capacity_attestation")" ]] || fail "capacity no-roster config retention changed"
[[ "$(hash_file "$capacity_delta_path")" == "$(jq -er '.capacity_config_delta_sha256' "$capacity_attestation")" ]] || fail "capacity config delta retention changed"
[[ "$(hash_file "$capacity_policy_path")" == "$(jq -er '.resource_policy_sha256' "$capacity_attestation")" ]] || fail "capacity resource policy retention changed"
[[ "$(hash_file "$capacity_measurer")" == "$capacity_measurer_sha256" ]] || fail "capacity measurer retention changed"
capacity_runner_path="$root_dir/scripts/run-v2-r2-sv1d-capacity-preflight.sh"
capacity_runner_sha256=$(hash_file "$capacity_runner_path")
[[ "$capacity_runner_sha256" == "$(jq -er '.runner_sha256' "$capacity_attestation")" ]] || fail "capacity runner differs from the measured preflight"
capacity_output_parent=$(jq -er '.output_parent' "$capacity_attestation") || fail "capacity attestation has no output parent"
activation_output_parent=$(dirname -- "$output_root")
[[ "$activation_output_parent" == "$capacity_output_parent" ]] || fail "activation output parent differs from measured capacity parent"
capacity_records_root=$(jq -er '.measurement_records_root' "$capacity_attestation") || fail "capacity attestation has no measurement-record root"
capacity_records_sha256=$(jq -er '.measurement_records_sha256 | select(test("^[0-9a-f]{64}$"))' "$capacity_attestation") || fail "capacity attestation has no measurement-record identity"
capacity_filesystem_device=$(jq -er '.filesystem_device' "$capacity_attestation")
capacity_filesystem_id=$(jq -er '.filesystem_id' "$capacity_attestation")
capacity_filesystem_type=$(jq -er '.filesystem_type' "$capacity_attestation")
capacity_filesystem_mount_id=$(jq -er '.filesystem_mount_id' "$capacity_attestation")
capacity_filesystem_uuid=$(jq -er '.filesystem_uuid' "$capacity_attestation")
"$sv1dprobe_binary" -mode verify-capacity -capacity-attestation "$capacity_attestation" \
	-source-revision "$source_revision" -tree-revision "$tree_revision" -plan-sha256 "$review_plan_sha256" \
	-review-attestation "$review_attestation_sha256" -review-report "$review_report_sha256" \
		-trusted-review-key-sha256 "$trusted_review_key_sha256" \
	-treatment-config "$staged_treatment_config" -mode-off-config "$staged_mode_off_config" -no-roster-config "$staged_no_roster_config" \
	-capacity-treatment-config "$capacity_treatment_config" -capacity-mode-off-config "$capacity_mode_off_config" -capacity-no-roster-config "$capacity_no_roster_config" \
	-capacity-config-delta-sha256 "$(jq -er '.capacity_config_delta_sha256' "$capacity_attestation")" \
	-binary-sha256 "$multivenue_sha256" -analyzer-sha256 "$sv1dprobe_sha256" -renderer-sha256 "$evsrender_sha256" \
	-runner-sha256 "$capacity_runner_sha256" -measurer-sha256 "$capacity_measurer_sha256" -resource-policy-sha256 "$(jq -er '.resource_policy_sha256' "$capacity_attestation")" \
	-output-parent "$capacity_output_parent" -measurement-root "$(jq -er '.measurement_root' "$capacity_attestation")" \
	-measurement-records-root "$capacity_records_root" -measurement-records-sha256 "$capacity_records_sha256" \
	-filesystem-device "$capacity_filesystem_device" -filesystem-id "$capacity_filesystem_id" -filesystem-type "$capacity_filesystem_type" \
	-filesystem-mount-id "$capacity_filesystem_mount_id" -filesystem-uuid "$capacity_filesystem_uuid" ||
	fail "measured SV1D binary-capacity preflight did not verify against this exact candidate"

source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"

[[ "$output_root" == /* && "$output_root" != "/" && "$(realpath -m -- "$output_root")" == "$output_root" ]] || fail "SV1D output root must be a clean absolute path"
require_no_symlink_components "$output_root" || fail "SV1D output root contains a symlink"
[[ ! -e "$output_root" && ! -L "$output_root" ]] || fail "refusing to overwrite SV1D evidence root: $output_root"

required_free_bytes=$(jq -er '.required_free_bytes' "$capacity_attestation") || fail "capacity attestation has no required free-space floor"
required_available_memory_bytes=$(jq -er '.required_available_memory_bytes' "$capacity_attestation") || fail "capacity attestation has no required memory floor"
capacity_memory_limit_bytes=$(jq -er '.cgroup_memory_limit_bytes' "$capacity_attestation") || fail "capacity attestation has no cgroup memory envelope"
[[ "$required_free_bytes" =~ ^[0-9]+$ && "$required_available_memory_bytes" =~ ^[0-9]+$ && "$capacity_memory_limit_bytes" =~ ^[0-9]+$ && "$capacity_memory_limit_bytes" -gt 0 ]] || fail "capacity resource floors are malformed"
available_kb=$(df -Pk -- "$capacity_output_parent" | awk 'NR == 2 {print $4}') || fail "could not measure activation output-parent free space"
[[ "$available_kb" =~ ^[0-9]+$ && $((available_kb * 1024)) -ge "$required_free_bytes" ]] || fail "activation output-parent free space is below the measured capacity floor"
host_available_bytes=$(awk '$1 == "MemAvailable:" {print $2 * 1024; exit}' /proc/meminfo)
[[ "$host_available_bytes" =~ ^[0-9]+$ && "$host_available_bytes" -ge "$required_available_memory_bytes" ]] || fail "activation host available memory is below the measured capacity floor"
live_filesystem_identity=$("$capacity_measurer" -inspect-filesystem "$capacity_output_parent") || fail "could not inspect the activation output-parent filesystem"
expected_filesystem_identity=$(jq -S -c '{device: .filesystem_device, id: .filesystem_id, type: .filesystem_type, mount_id: .filesystem_mount_id, uuid: .filesystem_uuid}' "$capacity_attestation") || fail "could not derive the measured filesystem identity"
actual_filesystem_identity=$(jq -S -c '{device, id, type, mount_id, uuid}' <<<"$live_filesystem_identity") || fail "capacity measurer returned malformed filesystem identity"
[[ "$actual_filesystem_identity" == "$expected_filesystem_identity" ]] || fail "activation filesystem identity differs from measured capacity"
require_live_resource_envelope "$capacity_memory_limit_bytes"

export GOMAXPROCS=2
export GOMEMLIMIT=4GiB

mkdir -p "$output_root/provenance" "$output_root/provenance/arm-results" "$output_root/tools" "$output_root/configs" "$output_root/arms" "$output_root/rendered" "$output_root/results" "$output_root/logs"
copy_immutable_file retained-multivenue "$multivenue_binary" "$output_root/tools/multivenue-$multivenue_sha256"
copy_immutable_file retained-sv1dprobe "$sv1dprobe_binary" "$output_root/tools/sv1dprobe-$sv1dprobe_sha256"
copy_immutable_file retained-evsrender "$evsrender_binary" "$output_root/tools/evsrender-$evsrender_sha256"
lock_binary_sha256=$(hash_file "$lock_binary")
copy_immutable_file retained-sv1dlock "$lock_binary" "$output_root/tools/sv1dlock-$lock_binary_sha256"
copy_immutable_file retained-review-attestation "$staged_review_attestation" "$output_root/provenance/review-attestation.json"
copy_immutable_file retained-review-report "$staged_review_report" "$output_root/provenance/review-report.md"
copy_immutable_file retained-trusted-review-key "$staged_trusted_review_key" "$output_root/provenance/trusted-review-key.raw"
copy_immutable_file retained-capacity-attestation "$staged_capacity_attestation" "$output_root/provenance/capacity-attestation.json"
activation_runner_sha256=$(hash_file "$root_dir/scripts/run-v2-r2-sv1d-activation.sh")
copy_immutable_file retained-activation-runner "$root_dir/scripts/run-v2-r2-sv1d-activation.sh" "$output_root/provenance/activation-runner.sh"
copy_immutable_file retained-capacity-policy "$capacity_policy_path" "$output_root/provenance/resource-policy-v1.json"
copy_immutable_file retained-parent-registration "$staged_parent_registration" "$output_root/provenance/parent-registration.md"
copy_immutable_file retained-amendment "$staged_amendment" "$output_root/provenance/amendment.md"
copy_immutable_file retained-target-treatment-config "$staged_treatment_config" "$output_root/configs/target-treatment.json"
copy_immutable_file retained-target-mode-off-config "$staged_mode_off_config" "$output_root/configs/target-mode-off.json"
copy_immutable_file retained-target-no-roster-config "$staged_no_roster_config" "$output_root/configs/target-no-roster.json"

plan_path="$output_root/probe-plan.json"
copy_immutable_file probe-plan "$review_plan" "$plan_path"

simulation_start_nano=1735689600000000000
simulation_end_nano=1735689900000000000
probe_horizon=5m
mkdir -p "$output_root/arms"

declare -A config_for=(
	[treatment]="$staged_treatment_config"
	[mode-off]="$staged_mode_off_config"
	[no-roster]="$staged_no_roster_config"
)
declare -A retained_result_for

activation_metadata="$output_root/provenance/activation-run-metadata.json"
jq -S -n \
	--arg contract "v2-r2-sv1d-activation-run-metadata-v2" --arg source_revision "$source_revision" --arg tree_revision "$tree_revision" \
	--arg probe_id "v2-r2-sv1d-activation-659" --arg plan_sha256 "$review_plan_sha256" \
	--arg review_attestation_sha256 "$review_attestation_sha256" --arg review_report_sha256 "$review_report_sha256" \
		--arg capacity_attestation_sha256 "$capacity_attestation_sha256" --arg capacity_records_sha256 "$capacity_records_sha256" \
		--arg trusted_review_key_sha256 "$trusted_review_key_sha256" \
	--arg capacity_root "$capacity_root" --arg capacity_records_root "$capacity_records_root" --arg arm_result_root "$output_root/provenance/arm-results" --arg activation_runner_sha256 "$activation_runner_sha256" --arg capacity_runner_sha256 "$capacity_runner_sha256" \
		--arg simulator_sha256 "$multivenue_sha256" --arg analyzer_sha256 "$sv1dprobe_sha256" --arg renderer_sha256 "$evsrender_sha256" \
		--arg lock_binary_sha256 "$lock_binary_sha256" \
	--arg output_root "$output_root" --arg output_parent "$capacity_output_parent" \
	'{schema_version: 2, contract: $contract, development_only: true, scientific_result_eligible: false,
	 source_revision: $source_revision, tree_revision: $tree_revision, probe_id: $probe_id, plan_sha256: $plan_sha256,
	 review_attestation_sha256: $review_attestation_sha256, review_report_sha256: $review_report_sha256,
		 capacity_attestation_sha256: $capacity_attestation_sha256, capacity_records_sha256: $capacity_records_sha256,
		 trusted_review_key_sha256: $trusted_review_key_sha256,
	 capacity_root: $capacity_root, capacity_records_root: $capacity_records_root, arm_result_root: $arm_result_root, activation_runner_sha256: $activation_runner_sha256, capacity_runner_sha256: $capacity_runner_sha256,
	 simulator_sha256: $simulator_sha256, analyzer_sha256: $analyzer_sha256, renderer_sha256: $renderer_sha256,
	 lock_binary_sha256: $lock_binary_sha256,
	 evidence_format: "evstream_v3", evidence_schema_epoch: 4, log_mode: "full", gomaxprocs: 2, gomemlimit: "4GiB",
	 output_root: $output_root, output_parent: $output_parent, arms: ["treatment", "mode-off", "no-roster"], holdouts_consumed: []}' \
	>"$activation_metadata.tmp-$$"
mv -- "$activation_metadata.tmp-$$" "$activation_metadata"
activation_metadata_sha256=$(hash_file "$activation_metadata")

arm_failure=0

mark_arm_failure() {
	local arm=$1 result_path=$2 reason=$3
	arm_failure=1
	if ! publish_incomplete_arm "$arm" "$result_path" "$reason"; then
		echo "SV1D activation: could not publish incomplete result for $arm ($reason)" >&2
		return 1
	fi
	echo "SV1D activation: retained incomplete arm $arm ($reason)" >&2
}

run_arm() {
	(
	set -euo pipefail
	local arm=$1 config=${config_for[$1]}
	local arm_dir="$output_root/arms/$arm" rendered_dir="$output_root/rendered/$arm"
	local stdout_log="$output_root/logs/$arm.simulator.stdout.log" stderr_log="$output_root/logs/$arm.simulator.stderr.log"
	local renderer_stderr_log="$output_root/logs/$arm.renderer.stderr.log"
	local result_path="$output_root/results/$arm.json" config_digest experiment_id hypothesis_id log_mode evidence_format
	local run_metadata_sha256_before status audit_status renderer_status

	if ! [[ ! -e "$arm_dir" && ! -L "$arm_dir" ]]; then return 1; fi
	if ! [[ ! -e "$rendered_dir" && ! -L "$rendered_dir" ]]; then return 1; fi
	if ! [[ ! -e "$result_path" && ! -L "$result_path" ]]; then return 1; fi
	if ! mkdir -p "$arm_dir"; then return 1; fi
	if ! copy_immutable_file "arm-$arm-config" "$config" "$arm_dir/run-config.json"; then return 1; fi

	config_digest=$(config_sha256 "$config")
	experiment_id=$(jq -er '.experiment_id' "$config")
	hypothesis_id=$(jq -er '.hypothesis_id' "$config")
	log_mode=$(jq -er '.log_mode' "$config")
	evidence_format=$(jq -er '.evidence_format' "$config")
	if ! jq -S -n \
		--arg arm "$arm" --arg experiment_id "$experiment_id" --arg hypothesis_id "$hypothesis_id" \
		--argjson seed 659 --arg horizon "$probe_horizon" \
		--argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		--arg config_sha256 "$config_digest" --arg binary_sha256 "$multivenue_sha256" \
		--arg git_revision "$source_revision" --arg tree_revision "$tree_revision" --arg binary_path "$output_root/tools/multivenue-$multivenue_sha256" \
		--arg binary_go_version "$(binary_go_version "$multivenue_binary")" \
		--arg analyzer_sha256 "$sv1dprobe_sha256" --arg renderer_sha256 "$evsrender_sha256" \
		--arg runner_sha256 "$activation_runner_sha256" --arg review_attestation_sha256 "$review_attestation_sha256" --arg review_report_sha256 "$review_report_sha256" \
			--arg capacity_attestation_sha256 "$capacity_attestation_sha256" --arg log_mode "$log_mode" --arg evidence_format "$evidence_format" \
			--arg capacity_records_sha256 "$capacity_records_sha256" --arg trusted_review_key_sha256 "$trusted_review_key_sha256" \
		--arg output_dir "$arm_dir" --argjson gomaxprocs 2 \
		'{schema_version: 2, runner_contract: "v2-r2-sv1d-activation-runner-v2", probe_id: "v2-r2-sv1d-activation-659", arm: $arm,
		  experiment_id: $experiment_id, config_experiment_id: $experiment_id, hypothesis_id: $hypothesis_id, seed: $seed, simulated_horizon: $horizon,
		  simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		  config_sha256: $config_sha256, binary_sha256: $binary_sha256, git_revision: $git_revision, tree_revision: $tree_revision,
		  binary_path: $binary_path, binary_go_version: $binary_go_version, binary_goos: "linux", binary_goarch: "amd64", binary_goamd64: "v1",
		  analyzer_sha256: $analyzer_sha256, renderer_sha256: $renderer_sha256, runner_sha256: $runner_sha256,
			  review_attestation_sha256: $review_attestation_sha256, review_report_sha256: $review_report_sha256, capacity_attestation_sha256: $capacity_attestation_sha256,
			  capacity_records_sha256: $capacity_records_sha256, trusted_review_key_sha256: $trusted_review_key_sha256,
		  log_mode: $log_mode, evidence_format: $evidence_format, evidence_schema_epoch: 4, gomaxprocs: $gomaxprocs, gomemlimit: "4GiB",
		  output_dir: $output_dir, holdout: false,
		  command: ["multivenue", "-config", "run-config.json", "-duration", "5m", "-log-mode", "full", "-evidence-format", "evstream_v3"],
		  raw_log_policy: "retain until the complete SV1D arm and tri-arm score have passed independent review"}' \
		>"$arm_dir/run-metadata.json"; then
		mark_arm_failure "$arm" "$result_path" "run_metadata_creation_failed" || return 1
		return 0
	fi
	run_metadata_sha256_before=$(hash_file "$arm_dir/run-metadata.json")

	set +e
	run_monitored_command "$stdout_log" "$stderr_log" "$output_root/logs/$arm.simulator.resource.json" "$arm/simulator" env GOMAXPROCS=2 GOMEMLIMIT=4GiB "$multivenue_binary" -config "$arm_dir/run-config.json" -duration "$probe_horizon" \
		-logdir "$arm_dir" -log-mode full -evidence-format evstream_v3
	status=$?
	set -e
	if [[ "$status" -ne 0 ]]; then
		mark_arm_failure "$arm" "$result_path" "simulator_exit_status_$status" || return 1
		return 0
	fi
	if [[ ! -s "$arm_dir/greeks.json" || ! -s "$arm_dir/latency.json" ]]; then
		mark_arm_failure "$arm" "$result_path" "completion_sentinel_missing" || return 1
		return 0
	fi
	if [[ "$run_metadata_sha256_before" != "$(hash_file "$arm_dir/run-metadata.json")" ]]; then
		mark_arm_failure "$arm" "$result_path" "run_metadata_changed_during_execution" || return 1
		return 0
	fi
	if ! jq -e --arg revision "$source_revision" --arg experiment "$experiment_id" \
		'.schema_version == 2 and .build.revision == $revision and .build.modified == false and
		 .build.goos == "linux" and .build.goarch == "amd64" and .build.goamd64 == "v1" and
		 .config.seed == 659 and .config.experiment_id == $experiment and .config.log_mode == "full" and
		 .config.evidence_format == "evstream_v3" and .config.evidence_contract_version == 2' \
		"$arm_dir/manifest.json" >/dev/null; then
		mark_arm_failure "$arm" "$result_path" "manifest_provenance_or_config_mismatch" || return 1
		return 0
	fi
	if ! jq -e --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
		'all(.initial_accounts[]; .account.timestamp == $start) and all(.terminal_accounts[]; .account.timestamp == $end)' \
		"$arm_dir/greeks.json" >/dev/null; then
		mark_arm_failure "$arm" "$result_path" "terminal_valuation_horizon_mismatch" || return 1
		return 0
	fi
	if ! v2_r2_write_evidence_manifest "$arm_dir" || ! v2_r2_verify_evidence_manifest "$arm_dir"; then
		mark_arm_failure "$arm" "$result_path" "evidence_manifest_verification_failed" || return 1
		return 0
	fi
	status_tmp="$arm_dir/run-status.json.tmp-$$"
	if ! jq -S -n \
		--argjson exit_status "$status" --arg arm "$arm" --arg experiment_id "$experiment_id" --arg hypothesis_id "$hypothesis_id" --arg horizon "$probe_horizon" \
		--argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		--arg run_metadata_sha256 "$run_metadata_sha256_before" \
		--arg manifest_sha256 "$(hash_file "$arm_dir/manifest.json")" --arg greeks_sha256 "$(hash_file "$arm_dir/greeks.json")" \
		--arg latency_sha256 "$(hash_file "$arm_dir/latency.json")" --arg checkpoints_sha256 "$(hash_file "$arm_dir/checkpoints.jsonl")" \
		--arg evidence_manifest_sha256 "$(hash_file "$arm_dir/evidence-manifest.json")" --arg binary_attestation_sha256 "$(hash_file "$arm_dir/binary-evidence-attestation.json")" \
		--arg market_data_evidence_sha256 "$(hash_file "$arm_dir/market-data-evidence-v2.json")" --arg market_data_schedules_sha256 "$(hash_file "$arm_dir/market-data-schedules-v2.bin")" \
		--arg market_data_receipts_sha256 "$(hash_file "$arm_dir/market-data-receipts-v2.bin")" --arg market_data_decisions_sha256 "$(hash_file "$arm_dir/market-data-decisions-v2.bin")" \
		'{schema_version: 1, contract: "v2-r2-sv1d-arm-status-v2", cell: $arm, experiment_id: $experiment_id, config_experiment_id: $experiment_id, hypothesis_id: $hypothesis_id,
		  scientific_result_eligible: false, exit_status: $exit_status, completion_verified: true, simulated_horizon: $horizon,
		  simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		  completion_sentinels: ["greeks.json", "latency.json"], run_metadata_sha256: $run_metadata_sha256,
		  manifest_sha256: $manifest_sha256, greeks_sha256: $greeks_sha256, latency_sha256: $latency_sha256,
		  checkpoints_sha256: $checkpoints_sha256, evidence_manifest_sha256: $evidence_manifest_sha256,
		  binary_evidence_attestation_sha256: $binary_attestation_sha256,
		  market_data_evidence_sha256: $market_data_evidence_sha256, market_data_schedules_sha256: $market_data_schedules_sha256,
		  market_data_receipts_sha256: $market_data_receipts_sha256, market_data_decisions_sha256: $market_data_decisions_sha256}' \
		>"$status_tmp"; then
		mark_arm_failure "$arm" "$result_path" "run_status_creation_failed" || return 1
		return 0
	fi
	mv -- "$status_tmp" "$arm_dir/run-status.json"

	set +e
	run_monitored_command "$arm_dir/renderer-report.json" "$renderer_stderr_log" "$output_root/logs/$arm.renderer.resource.json" "$arm/renderer" "$evsrender_binary" -dir "$arm_dir" -out "$rendered_dir"
	renderer_status=$?
	set -e
	if [[ "$renderer_status" -ne 0 ]]; then
		mark_arm_failure "$arm" "$result_path" "renderer_exit_status_$renderer_status" || return 1
		return 0
	fi
	if ! jq -e --argjson event_frames "$(jq -er '.event_frames' "$arm_dir/binary-evidence-attestation.json")" \
		--argjson stream_frames "$(jq -er '.stream_frames' "$arm_dir/binary-evidence-attestation.json")" \
		--arg execution_hash "$(jq -er '.execution_stream_hash' "$arm_dir/binary-evidence-attestation.json")" \
		'.event_frames == $event_frames and .dictionary_frames + .event_frames == $stream_frames and .execution_stream_hash == $execution_hash and .routes > 0 and (.rendered_digest | test("^[0-9a-f]{64}$"))' \
		"$arm_dir/renderer-report.json" >/dev/null; then
		mark_arm_failure "$arm" "$result_path" "renderer_report_binding_failed" || return 1
		return 0
	fi

	set +e
	run_monitored_command "$output_root/logs/$arm.audit.stdout.log" "$output_root/logs/$arm.audit.stderr.log" "$output_root/logs/$arm.audit.resource.json" "$arm/audit" "$sv1dprobe_binary" -mode audit -out "$result_path" -plan "$plan_path" -arm "$arm" \
		-run-dir "$arm_dir" -rendered-dir "$rendered_dir" -source-revision "$source_revision" \
		-tree-revision "$tree_revision" -plan-sha256 "$review_plan_sha256" \
		-parent-registration-sha256 "$parent_registration_sha256" -amendment-sha256 "$amendment_sha256" \
		-activation-metadata "$activation_metadata" -review-attestation-sha256 "$review_attestation_sha256" \
		-review-report-sha256 "$review_report_sha256" -capacity-attestation-sha256 "$capacity_attestation_sha256" \
		-capacity-records-sha256 "$capacity_records_sha256" -capacity-runner-sha256 "$capacity_runner_sha256" \
		-activation-runner-sha256 "$activation_runner_sha256" -activation-metadata-sha256 "$activation_metadata_sha256" \
		-trusted-review-key-sha256 "$trusted_review_key_sha256" -evidence-schema-epoch 4 -gomaxprocs 2 -gomemlimit 4GiB
	audit_status=$?
	set -e
	if [[ "$audit_status" -ne 0 ]]; then
		mark_arm_failure "$arm" "$result_path" "strict_audit_exit_status_$audit_status" || return 1
		return 0
	fi
	[[ -s "$result_path" && ! -L "$result_path" ]] || fail "$arm audit result was not published"
)
}

for arm in treatment mode-off no-roster; do
	set +e
	run_arm "$arm"
	run_arm_status=$?
	set -e
	result_path="$output_root/results/$arm.json"
	if [[ "$run_arm_status" -ne 0 ]]; then
		if [[ ! -s "$result_path" ]]; then
			mark_arm_failure "$arm" "$result_path" "activation_runner_failure_$run_arm_status" || fail "could not retain a typed result for $arm"
		elif ! jq -e '.arm.complete == false' "$result_path" >/dev/null 2>&1; then
			fail "activation runner failed without an incomplete typed result for $arm"
		fi
	fi
	if [[ -s "$result_path" ]] && jq -e '.arm.complete == false' "$result_path" >/dev/null 2>&1; then
		arm_failure=1
	fi
	result_digest=$(hash_file "$result_path")
	retained_result_path="$output_root/provenance/arm-results/$arm-$result_digest.json"
	copy_immutable_file "retained-$arm-arm-result" "$result_path" "$retained_result_path"
	retained_result_for[$arm]="$retained_result_path"
done

score_path="$output_root/score.json"
corpus_manifest_path="$output_root/provenance/score-corpus-manifest.json"
score_resource_path="$output_root/logs/score.resource.json"
set +e
run_monitored_command "$output_root/logs/score.stdout.log" "$output_root/logs/score.stderr.log" "$score_resource_path" "score" "$sv1dprobe_binary" -mode score -out "$score_path" -corpus-manifest "$corpus_manifest_path" -plan "$plan_path" \
	-treatment-result "${retained_result_for[treatment]}" \
	-mode-off-result "${retained_result_for[mode-off]}" \
	-no-roster-result "${retained_result_for[no-roster]}" \
	-treatment-run-dir "$output_root/arms/treatment" -treatment-rendered-dir "$output_root/rendered/treatment" \
	-mode-off-run-dir "$output_root/arms/mode-off" -mode-off-rendered-dir "$output_root/rendered/mode-off" \
	-no-roster-run-dir "$output_root/arms/no-roster" -no-roster-rendered-dir "$output_root/rendered/no-roster" \
	-source-revision "$source_revision" \
	-tree-revision "$tree_revision" -plan-sha256 "$review_plan_sha256" \
	-parent-registration-sha256 "$parent_registration_sha256" -amendment-sha256 "$amendment_sha256" \
	-activation-metadata "$activation_metadata" -review-attestation-sha256 "$review_attestation_sha256" \
	-review-report-sha256 "$review_report_sha256" -capacity-attestation-sha256 "$capacity_attestation_sha256" \
	-capacity-records-sha256 "$capacity_records_sha256" -capacity-runner-sha256 "$capacity_runner_sha256" \
	-activation-runner-sha256 "$activation_runner_sha256" -activation-metadata-sha256 "$activation_metadata_sha256" \
	-trusted-review-key-sha256 "$trusted_review_key_sha256" -evidence-schema-epoch 4 -gomaxprocs 2 -gomemlimit 4GiB
score_status=$?
set -e
[[ -s "$score_path" && ! -L "$score_path" ]] || fail "could not publish tri-arm score"
jq -e 'type == "object" and .contract == "v2-r2-sv1d-score-v1" and .probe_id == "v2-r2-sv1d-activation-659" and (.score.status | type == "string")' \
	"$score_path" >/dev/null || fail "tri-arm score is malformed"
[[ -s "$corpus_manifest_path" && ! -L "$corpus_manifest_path" ]] || fail "could not publish strict score corpus manifest"
jq -e 'type == "object" and .schema_version == 1 and .contract == "v2-r2-sv1d-score-corpus-manifest-v1" and .probe_id == "v2-r2-sv1d-activation-659" and (.arm_corpus | length == 3)' \
	"$corpus_manifest_path" >/dev/null || fail "strict score corpus manifest is malformed"
if [[ "$score_status" -eq 0 && "$arm_failure" -eq 0 ]]; then
	write_resource_manifest
	require_regular_file activation-resource-manifest "$output_root/provenance/resource-usage-manifest.json"
	jq -e --argjson limit "$capacity_memory_limit_bytes" --argjson minimum_memory "$required_available_memory_bytes" --argjson minimum_free "$required_free_bytes" \
		'type == "object" and .schema_version == 1 and .contract == "v2-r2-sv1d-activation-resource-manifest-v1" and
		 .scope == "sv1d_activation" and (.stages | length == 10) and
		 (.stages | map(.path)) == ["logs/treatment.simulator.resource.json", "logs/treatment.renderer.resource.json", "logs/treatment.audit.resource.json",
			"logs/mode-off.simulator.resource.json", "logs/mode-off.renderer.resource.json", "logs/mode-off.audit.resource.json",
			"logs/no-roster.simulator.resource.json", "logs/no-roster.renderer.resource.json", "logs/no-roster.audit.resource.json", "logs/score.resource.json"] and
		 (.stages | map(.record.stage)) == ["simulator", "renderer", "audit", "simulator", "renderer", "audit", "simulator", "renderer", "audit", "score"] and
		 ((.stages | map(.path) | unique | length) == 10) and
		 all(.stages[]; .bytes > 0 and (.sha256 | test("^[0-9a-f]{64}$")) and
		 .record.exit_status == 0 and .record.monitor_status == 0 and
		 .record.peak_cgroup_memory_bytes <= $limit and .record.cgroup_memory_limit_bytes == $limit and
		 .record.minimum_host_available_bytes >= $minimum_memory and .record.minimum_filesystem_available_bytes >= $minimum_free and
		 .record.maximum_swap_bytes == 0 and .record.oom_events_delta == 0 and .record.oom_kill_events_delta == 0)' \
		"$output_root/provenance/resource-usage-manifest.json" >/dev/null || fail "activation resource manifest is malformed or exceeds its measured envelope"
fi
if [[ "$score_status" -ne 0 || "$arm_failure" -ne 0 ]]; then
	echo "SV1D activation probe retained evidence but did not certify an executable tri-arm result" >&2
	exit 1
fi
echo "completed development-only SV1D activation probe: $output_root"
