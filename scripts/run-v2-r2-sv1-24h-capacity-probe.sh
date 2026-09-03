#!/usr/bin/env bash
# Measure actual 24-hour evstream_v3 storage for one registered production
# configuration. The probe is not a scientific result and is retained so its
# capacity claim can be independently audited. Successor namespaces bind each
# production configuration and process width to a separate attestation.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
contract_script=$(v2_r2_select_sv1_contract "$root_dir") || {
	echo "SV1 capacity probe received an unregistered contract path" >&2
	exit 1
}
source "$contract_script"
export V2_R2_SV1_CONTRACT_SCRIPT="$contract_script"
head_revision=$(git -C "$root_dir" rev-parse HEAD)
scientific_root=$(realpath -e -- "$root_dir")

binary=${1:-"$root_dir/bin/multivenue"}
primary_seed="${v2_r2_sv1_seeds[0]}"
config="${V2_R2_SV1_CAPACITY_CONFIG:-${v2_r2_sv1_capacity_measurement_config:-$v2_r2_sv1_config_dir/treatment-$primary_seed.json}}"
launch_config="${V2_R2_SV1_CAPACITY_LAUNCH_CONFIG:-${v2_r2_sv1_capacity_launch_config:-$config}}"
if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* && -z "${V2_R2_SV1_CAPACITY_LAUNCH_CONFIG:-}" ]]; then
	case "$(basename -- "$config")" in
		control-*) launch_config="$v2_r2_sv1_config_dir/control-643.json" ;;
		treatment-*) launch_config="$v2_r2_sv1_config_dir/treatment-643.json" ;;
		*) : ;;
	esac
fi
configured_measurement_seed="${v2_r2_sv1_capacity_measurement_seed:-$primary_seed}"
horizon=24h
simulation_start_nano=1735689600000000000
simulation_end_nano=1735776000000000000
safety_margin_bytes=$((4 * 1024 * 1024 * 1024))
minimum_free_bytes=$((4 * 1024 * 1024 * 1024))
expected_gomaxprocs=${V2_R2_SV1_CAPACITY_GOMAXPROCS:-${GOMAXPROCS:-4}}
memory_limit_bytes=${v2_r2_sv1_capacity_memory_limit_bytes:-0}
gomemlimit_bytes=0
memory_validation_arg=""
capacity_config_name=$(basename -- "$config")
capacity_role=${capacity_config_name%.json}
probe_root_requested=${V2_R2_SV1_CAPACITY_ROOT:-"/home/vlad/external-scratch/${v2_r2_sv1_capacity_probe_prefix}-${head_revision}-seed-${configured_measurement_seed}-${capacity_role}-g${expected_gomaxprocs}"}
host_cpu_count=0
allowed_cpu_count=0
cpu_affinity=""
cpu_launch_prefix=()

fail() {
	echo "SV1 capacity probe failure: $*" >&2
	exit 1
}

if [[ "$memory_limit_bytes" =~ ^[1-9][0-9]*$ ]]; then
	gomemlimit_bytes=$((memory_limit_bytes - 2 * 1024 * 1024 * 1024))
	(( gomemlimit_bytes > 0 )) || fail "configured capacity memory limit leaves no Go heap reserve"
	memory_validation_arg="$memory_limit_bytes"
	command -v prlimit >/dev/null 2>&1 || fail "SV1B capacity probe requires prlimit for the hard address-space ceiling"
fi

capacity_free_bytes() {
	local path=$1 df_output available_bytes
	df_output=$(df -P -B1 -- "$path") || return 1
	available_bytes=$(awk 'NR == 2 {print $4}' <<<"$df_output")
	[[ "$available_bytes" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$available_bytes"
}

capacity_directory_bytes() {
	local path=$1 du_output output_bytes
	du_output=$(du -sb -- "$path") || return 1
	output_bytes=$(awk 'NR == 1 {print $1}' <<<"$du_output")
	[[ "$output_bytes" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$output_bytes"
}

capacity_process_rss_bytes() {
	local process_id=$1 rss_kib
	[[ "$process_id" =~ ^[1-9][0-9]*$ ]] || return 1
	rss_kib=$(awk '$1 == "VmRSS:" {print $2; exit}' "/proc/$process_id/status" 2>/dev/null)
	[[ "$rss_kib" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$((rss_kib * 1024))"
}

simulator_pid=""
terminate_simulator() {
	local child_pid=${simulator_pid:-}
	[[ -n "$child_pid" ]] || return 0
	if kill -0 "$child_pid" 2>/dev/null; then
		kill -TERM "$child_pid" 2>/dev/null || true
		for _ in {1..15}; do
			kill -0 "$child_pid" 2>/dev/null || break
			sleep 1
		done
		if kill -0 "$child_pid" 2>/dev/null; then
			kill -KILL "$child_pid" 2>/dev/null || true
		fi
	fi
	wait "$child_pid" 2>/dev/null || true
}

cleanup_simulator() {
	local exit_status=$?
	trap - EXIT INT TERM HUP
	terminate_simulator
	exit "$exit_status"
}

trap cleanup_simulator EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

v2_r2_acquire_namespace_lock || fail "could not acquire the SV1 capacity namespace lock"

[[ -x "$binary" && -s "$config" && -s "$launch_config" ]] || fail "missing binary or capacity configuration"
if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
	[[ "$(realpath -e -- "$config")" == "$v2_r2_sv1_config_dir/$(basename -- "$config")" ]] ||
		fail "SV1B capacity measurement must use a registered production configuration"
	[[ "$(realpath -e -- "$launch_config")" == "$(realpath -e -- "$config")" ]] ||
		fail "SV1B capacity launch binding must equal the measured production configuration"
	[[ "$memory_limit_bytes" =~ ^[1-9][0-9]*$ ]] || fail "SV1B capacity probe has no positive RSS safety limit"
fi
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "scientific worktree is dirty"
[[ "${GOMAXPROCS:-}" == "$expected_gomaxprocs" ]] || fail "capacity probe requires GOMAXPROCS=$expected_gomaxprocs"
[[ "$expected_gomaxprocs" =~ ^[1-9][0-9]*$ ]] || fail "capacity probe process width is not integral"
if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
	[[ "$expected_gomaxprocs" == 4 || "$expected_gomaxprocs" == 8 ]] || fail "SV1B capacity probe width must be 4 or 8"
	IFS=$'\t' read -r host_cpu_count allowed_cpu_count cpu_affinity < <(v2_r2_sv1b_cpu_policy) ||
		fail "could not establish the registered CPU affinity policy"
	command -v taskset >/dev/null 2>&1 || fail "SV1B capacity probe requires taskset for the CPU ceiling"
	cpu_launch_prefix=(taskset --cpu-list "$cpu_affinity")
fi
[[ "$probe_root_requested" == /* && "$probe_root_requested" != */ && "$probe_root_requested" != *$'\n'* && "$probe_root_requested" != *$'\t'* ]] || fail "capacity root must be an absolute, non-empty path"
probe_root=$(realpath -m -- "$probe_root_requested") || fail "could not resolve capacity probe root"
case "$probe_root" in
	"$scientific_root"|"$scientific_root"/*) fail "capacity root resolves inside the scientific repository" ;;
esac
[[ ! -e "$probe_root_requested" && ! -L "$probe_root_requested" ]] || fail "capacity probe root already exists: $probe_root_requested"
binary_revision=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
binary_modified=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
binary_trimpath=$(go version -m "$binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
binary_cgo_enabled=$(go version -m "$binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
binary_goos=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOOS=") == 1 {sub("GOOS=", "", $2); print $2; exit}')
binary_goarch=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOARCH=") == 1 {sub("GOARCH=", "", $2); print $2; exit}')
binary_goamd64=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOAMD64=") == 1 {sub("GOAMD64=", "", $2); print $2; exit}')
binary_go_version=$(v2_r2_binary_go_version "$binary")
[[ "$binary_revision" == "$head_revision" && "$binary_modified" == false && "$binary_trimpath" == true && "$binary_cgo_enabled" == 0 ]] || fail "binary is not a clean build of current HEAD"
[[ "$binary_goos" == linux && "$binary_goarch" == amd64 && "$binary_goamd64" == v1 ]] || fail "binary target is not linux/amd64/v1"
v2_r2_is_go_127 "$binary_go_version" || fail "binary is not Go 1.27: $binary_go_version"
"$root_dir/scripts/check-v2-r2-sv1-24h-configs.sh" >/dev/null
capacity_mount_path="$probe_root"
while [[ ! -e "$capacity_mount_path" ]]; do
	parent_path=$(dirname -- "$capacity_mount_path")
	[[ "$parent_path" != "$capacity_mount_path" ]] || fail "capacity probe root has no existing filesystem ancestor"
	capacity_mount_path=$parent_path
done
[[ -d "$capacity_mount_path" ]] || fail "capacity probe filesystem ancestor is not a directory: $capacity_mount_path"
initial_available_free_bytes=$(capacity_free_bytes "$capacity_mount_path") || fail "could not measure initial free space"
(( initial_available_free_bytes >= minimum_free_bytes )) ||
	fail "initial free space is below the ${minimum_free_bytes}-byte reserve: $initial_available_free_bytes"
if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
	log_mode=full
	evidence_format=evstream_v3
else
	log_mode=$(jq -er '.log_mode | select(. == "full" or . == "none")' "$config") || fail "registered capacity config has no supported log mode"
	evidence_format=$(jq -er '.evidence_format' "$config") || fail "registered capacity config has no evidence format"
fi
[[ "$evidence_format" == evstream_v3 ]] || fail "capacity probe requires evstream_v3"
source_config_seed=$(jq -er '.seed' "$config") || fail "registered capacity config has no seed"
measurement_seed="$source_config_seed"
capacity_only=false
if [[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]]; then
	measurement_seed="${V2_R2_SV1_CAPACITY_SEED:-$configured_measurement_seed}"
	[[ "$measurement_seed" == "$configured_measurement_seed" ]] || fail "capacity-only measurement seed does not match the registered capacity contract"
	[[ "$source_config_seed" == 643 && "$measurement_seed" == 659 && "$measurement_seed" != "$source_config_seed" ]] ||
		fail "SV1B capacity measurement must use the preregistered seed-659 override over source seed 643"
	capacity_only=true
else
	[[ "$measurement_seed" == "$configured_measurement_seed" ]] || fail "capacity measurement seed does not match the registered configuration"
fi
seed_override_args=()
if [[ "$measurement_seed" != "$source_config_seed" ]]; then
	seed_override_args=(-seed "$measurement_seed")
fi
attestation=$(v2_r2_capacity_attestation_path_for_config "$config" "$expected_gomaxprocs") ||
	fail "registered capacity configuration has no capacity attestation identity"
probe_cell_name=$(v2_r2_capacity_probe_cell_for_config "$config" "$expected_gomaxprocs") ||
	fail "registered capacity configuration has no probe-cell identity"
[[ ! -e "$attestation" && ! -L "$attestation" ]] || fail "capacity attestation already exists: $attestation"
mkdir -p -- "$probe_root"
[[ "$(realpath -e -- "$probe_root")" == "$probe_root" ]] || fail "capacity probe root canonicalization changed after creation"
probe_dir="$probe_root/$probe_cell_name"
mkdir -- "$probe_dir"

	"$binary" -config "$config" "${seed_override_args[@]}" -logdir "$probe_dir" -log-mode "$log_mode" -evidence-format "$evidence_format" \
	-write-effective-config "$probe_dir/run-config.json" >/dev/null 2>"$probe_root/config.stderr.log" || fail "effective-config validation failed"
jq -e --arg log_mode "$log_mode" --arg evidence_format "$evidence_format" \
	--argjson measurement_seed "$measurement_seed" \
	'.log_mode == $log_mode and .evidence_format == $evidence_format and .seed == $measurement_seed' \
	"$probe_dir/run-config.json" >/dev/null || fail "effective capacity configuration is incomplete"
config_sha256=$(sha256sum "$probe_dir/run-config.json" | awk '{print $1}')
measurement_config_sha256=$(sha256sum "$config" | awk '{print $1}')
launch_config_sha256=$(sha256sum "$launch_config" | awk '{print $1}')
measurement_config_path=${config#"$root_dir/"}
launch_config_path=${launch_config#"$root_dir/"}
calibration_only=false
[[ "${v2_r2_sv1_candidate_id:-}" == V2-R2-SV1B-* ]] && calibration_only=true
authorized_launch_config_sha256='[]'
activation_provenance_path=""
activation_review_attestation_path=""
activation_review_attestation_sha256=""
if [[ "$calibration_only" == true ]]; then
	for authorized_name in "${v2_r2_sv1_capacity_authorized_launch_config_names[@]}"; do
		[[ "$authorized_name" != */* && "$authorized_name" == *.json ]] || fail "unsafe authorized launch config name: $authorized_name"
		authorized_path="$v2_r2_sv1_config_dir/$authorized_name"
		[[ -s "$authorized_path" && ! -L "$authorized_path" ]] || fail "authorized launch config is missing: $authorized_name"
	done
	authorized_launch_config_sha256=$(for authorized_name in "${v2_r2_sv1_capacity_authorized_launch_config_names[@]}"; do
		sha256sum -- "$v2_r2_sv1_config_dir/$authorized_name" | awk '{print $1}'
	done | jq -Rsc 'split("\n") | map(select(length > 0))')
fi
binary_sha256=$(sha256sum "$binary" | awk '{print $1}')
if [[ "$calibration_only" == true ]]; then
	activation_provenance=$(v2_r2_sv1_activation_provenance_path "$head_revision") || fail "could not resolve accepted SV1B activation provenance path"
	v2_r2_require_sv1b_activation_provenance "$activation_provenance" "$head_revision" "$binary_sha256" ||
		fail "refusing capacity calibration before accepted SV1B activation provenance"
	activation_provenance_path="$activation_provenance"
	activation_provenance_sha256=$(sha256sum -- "$activation_provenance" | awk '{print $1}')
	activation_review_attestation_path=$(v2_r2_sv1b_activation_review_attestation_path "$head_revision") ||
		fail "could not resolve the post-activation review attestation path"
	v2_r2_require_sv1b_activation_review_attestation "$activation_review_attestation_path" "$head_revision" "$activation_provenance" ||
		fail "refusing capacity calibration before independent review of the activation evidence"
	activation_review_attestation_sha256=$(sha256sum -- "$activation_review_attestation_path" | awk '{print $1}')
else
	activation_provenance_sha256=""
fi
jq -n --arg git_revision "$head_revision" --arg binary_sha256 "$binary_sha256" --arg config_sha256 "$config_sha256" \
	--arg launch_config_sha256 "$launch_config_sha256" --arg measurement_config_path "$measurement_config_path" \
	--arg launch_config_path "$launch_config_path" --arg binary_go_version "$binary_go_version" --arg log_mode "$log_mode" \
	--arg evidence_format "$evidence_format" --argjson seed "$measurement_seed" --arg horizon "$horizon" \
	--argjson gomaxprocs "$expected_gomaxprocs" --argjson minimum_free_bytes "$minimum_free_bytes" \
	--argjson initial_available_free_bytes "$initial_available_free_bytes" \
	--arg contract "$v2_r2_sv1_capacity_probe_contract" --argjson calibration_only "$calibration_only" \
	--argjson authorized_launch_config_sha256 "$authorized_launch_config_sha256" \
	--arg activation_provenance_path "$activation_provenance_path" --arg activation_provenance_sha256 "$activation_provenance_sha256" \
	--arg activation_review_attestation_path "$activation_review_attestation_path" --arg activation_review_attestation_sha256 "$activation_review_attestation_sha256" \
	--argjson memory_limit_bytes "$memory_limit_bytes" \
	--argjson gomemlimit_bytes "$gomemlimit_bytes" \
	--argjson host_cpu_count "$host_cpu_count" --argjson allowed_cpu_count "$allowed_cpu_count" --argjson cpu_limit_percent "${v2_r2_sv1_cpu_limit_percent:-0}" --arg cpu_affinity "$cpu_affinity" \
	--argjson capacity_only "$capacity_only" --argjson source_config_seed "$source_config_seed" \
	--arg probe_cell "$probe_cell_name" \
	'{schema_version:2,contract:$contract,git_revision:$git_revision,binary_sha256:$binary_sha256,config_sha256:$config_sha256,measurement_config_sha256:$measurement_config_sha256,measurement_config_path:$measurement_config_path,primary_launch_config_sha256:$launch_config_sha256,launch_config_sha256:$launch_config_sha256,launch_config_path:$launch_config_path,authorized_launch_config_sha256:$authorized_launch_config_sha256,calibration_only:$calibration_only,capacity_only:$capacity_only,source_config_seed:$source_config_seed,activation_provenance_path:(if $activation_provenance_path == "" then null else $activation_provenance_path end),activation_provenance_sha256:(if $activation_provenance_sha256 == "" then null else $activation_provenance_sha256 end),activation_review_attestation_path:(if $activation_review_attestation_path == "" then null else $activation_review_attestation_path end),activation_review_attestation_sha256:(if $activation_review_attestation_sha256 == "" then null else $activation_review_attestation_sha256 end),memory_limit_bytes:$memory_limit_bytes,gomemlimit_bytes:$gomemlimit_bytes,binary_go_version:$binary_go_version,gomaxprocs:$gomaxprocs,host_cpu_count:$host_cpu_count,allowed_cpu_count:$allowed_cpu_count,cpu_limit_percent:$cpu_limit_percent,cpu_affinity:$cpu_affinity,minimum_free_bytes:$minimum_free_bytes,initial_available_free_bytes:$initial_available_free_bytes,seed:$seed,simulated_horizon:$horizon,log_mode:$log_mode,evidence_format:$evidence_format,simulation_start_nano:1735689600000000000,simulation_end_nano:1735776000000000000,probe_cell:$probe_cell}' >"$probe_dir/run-metadata.json"

stdout_log="$probe_root/simulator.stdout.log"
stderr_log="$probe_root/simulator.stderr.log"
run_simulator() {
	if [[ "$memory_limit_bytes" =~ ^[1-9][0-9]*$ ]]; then
			"${cpu_launch_prefix[@]}" env GOMAXPROCS="$expected_gomaxprocs" GOMEMLIMIT="${gomemlimit_bytes}B" prlimit --as="$memory_limit_bytes" -- \
				"$binary" -config "$probe_dir/run-config.json" "${seed_override_args[@]}" -duration "$horizon" -logdir "$probe_dir" -log-mode "$log_mode" -evidence-format "$evidence_format" \
			>"$stdout_log" 2>"$stderr_log" &
	else
			"${cpu_launch_prefix[@]}" "$binary" -config "$probe_dir/run-config.json" "${seed_override_args[@]}" -duration "$horizon" -logdir "$probe_dir" -log-mode "$log_mode" -evidence-format "$evidence_format" \
			>"$stdout_log" 2>"$stderr_log" &
	fi
	simulator_pid=$!
}
set +e
run_simulator
set -e
peak_output_bytes=0
peak_at=""
peak_rss_bytes=0
peak_rss_at=""
capacity_abort=false
capacity_abort_reason=""
while kill -0 "$simulator_pid" 2>/dev/null; do
	if ! current_bytes=$(capacity_directory_bytes "$probe_root"); then
		capacity_abort=true
		capacity_abort_reason="output-size measurement failed during probe"
		terminate_simulator
		break
	fi
	if (( current_bytes > peak_output_bytes )); then
		peak_output_bytes=$current_bytes
		peak_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	fi
	if [[ "$memory_limit_bytes" =~ ^[1-9][0-9]*$ ]]; then
		if ! current_rss_bytes=$(capacity_process_rss_bytes "$simulator_pid"); then
			capacity_abort=true
			capacity_abort_reason="simulator RSS measurement failed during probe"
			terminate_simulator
			break
		fi
		if (( current_rss_bytes > peak_rss_bytes )); then
			peak_rss_bytes=$current_rss_bytes
			peak_rss_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
		fi
		if (( current_rss_bytes > memory_limit_bytes )); then
			capacity_abort=true
			capacity_abort_reason="simulator RSS crossed the ${memory_limit_bytes}-byte reserve: $current_rss_bytes"
			terminate_simulator
			break
		fi
	fi
	if ! available_bytes=$(capacity_free_bytes "$probe_root"); then
		capacity_abort=true
		capacity_abort_reason="free-space measurement failed during probe"
		terminate_simulator
		break
	elif (( available_bytes < minimum_free_bytes )); then
		capacity_abort=true
		capacity_abort_reason="free space crossed the ${minimum_free_bytes}-byte reserve during probe: $available_bytes"
		terminate_simulator
		break
	fi
	sleep 2
done
set +e
wait "$simulator_pid"
simulator_status=$?
set -e
if ! final_output_bytes=$(capacity_directory_bytes "$probe_root"); then
	capacity_abort=true
	capacity_abort_reason="final output-size measurement failed"
else
	(( final_output_bytes > peak_output_bytes )) && peak_output_bytes=$final_output_bytes
fi
if ! final_available_free_bytes=$(capacity_free_bytes "$probe_root"); then
	capacity_abort=true
	capacity_abort_reason="final free-space measurement failed"
elif (( final_available_free_bytes < minimum_free_bytes )); then
	capacity_abort=true
	capacity_abort_reason="final free space is below the ${minimum_free_bytes}-byte reserve: $final_available_free_bytes"
fi
[[ "$capacity_abort" == false && "$simulator_status" == 0 ]] ||
	fail "probe did not complete (status=$simulator_status capacity_abort=$capacity_abort reason=${capacity_abort_reason:-simulator failure}); output retained at $probe_root"
if [[ "$memory_limit_bytes" =~ ^[1-9][0-9]*$ ]]; then
	(( peak_rss_bytes > 0 )) || fail "probe completed without a simulator RSS sample; output retained at $probe_root"
fi

required_files=(run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json)
if [[ "$log_mode" == full ]]; then
	required_files+=(evidence-only-artifact-hash.json)
else
	[[ ! -e "$probe_dir/evidence-only-artifact-hash.json" && ! -L "$probe_dir/evidence-only-artifact-hash.json" ]] || fail "no-log capacity probe wrote an evidence-only hash"
fi
if [[ "${v2_r2_sv1_require_terminal_outcome:-false}" == true ]]; then
	required_files+=(terminal-outcome.json)
fi
for required in "${required_files[@]}"; do
	[[ -s "$probe_dir/$required" ]] || fail "completed probe is missing $required"
done
jq -e --arg revision "$head_revision" --argjson seed "$measurement_seed" \
	--arg log_mode "$log_mode" --arg evidence_format "$evidence_format" \
	'.build.revision == $revision and .build.modified == false and .config.seed == $seed and
	 .config.log_mode == $log_mode and .config.evidence_format == $evidence_format' \
	"$probe_dir/manifest.json" >/dev/null || fail "probe manifest provenance mismatch"
jq -e --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
	'(.initial_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $start)) and (.terminal_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $end))' "$probe_dir/greeks.json" >/dev/null || fail "probe greeks do not attest the 24-hour horizon"
v2_r2_require_checkpoint_stream "$probe_dir/checkpoints.jsonl" "$simulation_start_nano" "$simulation_end_nano" || fail "probe checkpoints do not attest the 24-hour horizon"
v2_r2_write_evidence_manifest "$probe_dir" || fail "probe evidence manifest generation failed"
v2_r2_verify_evidence_manifest "$probe_dir" || fail "probe evidence manifest verification failed"

if ! retained_output_bytes=$(capacity_directory_bytes "$probe_root"); then
	fail "final retained-output size measurement failed; output retained at $probe_root"
fi
(( retained_output_bytes > peak_output_bytes )) && peak_output_bytes=$retained_output_bytes
if ! final_available_free_bytes=$(capacity_free_bytes "$probe_root"); then
	fail "final retained-output free-space measurement failed; output retained at $probe_root"
fi
(( final_available_free_bytes >= minimum_free_bytes )) ||
	fail "final retained-output free space is below the ${minimum_free_bytes}-byte reserve: $final_available_free_bytes"
required_free_bytes=$((peak_output_bytes + safety_margin_bytes))
(( final_available_free_bytes >= required_free_bytes )) ||
	fail "final retained-output free space is below the measured capacity floor: available=$final_available_free_bytes required=$required_free_bytes"
attestation_tmp="$attestation.tmp-$$"
mkdir -p -- "$(dirname -- "$attestation")"
evidence_manifest_sha256=$(sha256sum -- "$probe_dir/evidence-manifest.json" | awk '{print $1}')
jq -n --arg source_revision "$head_revision" --arg binary_sha256 "$binary_sha256" \
	--arg config_sha256 "$config_sha256" --arg measurement_config_sha256 "$measurement_config_sha256" \
	--arg launch_config_sha256 "$launch_config_sha256" --arg measurement_config_path "$measurement_config_path" --arg launch_config_path "$launch_config_path" \
	--arg probe_root "$probe_root" --arg peak_at "$peak_at" --arg evidence_format "$evidence_format" \
	--arg evidence_manifest_sha256 "$evidence_manifest_sha256" \
	--argjson peak_output_bytes "$peak_output_bytes" --argjson safety_margin_bytes "$safety_margin_bytes" \
	--argjson required_free_bytes "$required_free_bytes" --argjson available_free_bytes "$final_available_free_bytes" \
	--argjson gomaxprocs "$expected_gomaxprocs" --argjson minimum_free_bytes "$minimum_free_bytes" \
	--argjson initial_available_free_bytes "$initial_available_free_bytes" --argjson measurement_seed "$measurement_seed" \
	--argjson calibration_only "$calibration_only" --argjson authorized_launch_config_sha256 "$authorized_launch_config_sha256" \
	--arg activation_provenance_path "$activation_provenance_path" --arg activation_provenance_sha256 "$activation_provenance_sha256" \
	--arg activation_review_attestation_path "$activation_review_attestation_path" --arg activation_review_attestation_sha256 "$activation_review_attestation_sha256" \
	--argjson memory_limit_bytes "$memory_limit_bytes" \
	--argjson gomemlimit_bytes "$gomemlimit_bytes" \
	--argjson capacity_only "$capacity_only" --argjson source_config_seed "$source_config_seed" \
	--argjson peak_rss_bytes "$peak_rss_bytes" --arg peak_rss_at "$peak_rss_at" --arg log_mode "$log_mode" --arg probe_cell "$probe_cell_name" \
	--argjson host_cpu_count "$host_cpu_count" --argjson allowed_cpu_count "$allowed_cpu_count" --argjson cpu_limit_percent "${v2_r2_sv1_cpu_limit_percent:-0}" --arg cpu_affinity "$cpu_affinity" \
	--arg contract "${v2_r2_sv1_capacity_attestation_contract:-v2-integrated-longrun-r2-binary-capacity-v1}" \
	--argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
	'{schema_version:1,contract:$contract,measurement:"full_24h_binary_evidence_capacity_probe",evidence_format:$evidence_format,log_mode:$log_mode,source_revision:$source_revision,binary_sha256:$binary_sha256,config_sha256:$config_sha256,measurement_config_sha256:$measurement_config_sha256,measurement_config_path:$measurement_config_path,measurement_seed:$measurement_seed,source_config_seed:$source_config_seed,capacity_only:$capacity_only,primary_launch_config_sha256:$launch_config_sha256,launch_config_sha256:$launch_config_sha256,launch_config_path:$launch_config_path,authorized_launch_config_sha256:$authorized_launch_config_sha256,calibration_only:$calibration_only,activation_provenance_path:(if $activation_provenance_path == "" then null else $activation_provenance_path end),activation_provenance_sha256:(if $activation_provenance_sha256 == "" then null else $activation_provenance_sha256 end),activation_review_attestation_path:(if $activation_review_attestation_path == "" then null else $activation_review_attestation_path end),activation_review_attestation_sha256:(if $activation_review_attestation_sha256 == "" then null else $activation_review_attestation_sha256 end),gomaxprocs:$gomaxprocs,host_cpu_count:$host_cpu_count,allowed_cpu_count:$allowed_cpu_count,cpu_limit_percent:$cpu_limit_percent,cpu_affinity:$cpu_affinity,minimum_free_bytes:$minimum_free_bytes,initial_available_free_bytes:$initial_available_free_bytes,memory_limit_bytes:$memory_limit_bytes,gomemlimit_bytes:$gomemlimit_bytes,peak_rss_bytes:$peak_rss_bytes,peak_rss_observed_at:$peak_rss_at,probe_root:$probe_root,probe_cell:$probe_cell,evidence_manifest_sha256:$evidence_manifest_sha256,peak_output_bytes:$peak_output_bytes,safety_margin_bytes:$safety_margin_bytes,required_free_bytes:$required_free_bytes,available_free_bytes:$final_available_free_bytes,peak_observed_at:$peak_at,simulation_start_nano:$simulation_start_nano,simulation_end_nano:$simulation_end_nano}' >"$attestation_tmp"
	v2_r2_require_binary_capacity_attestation "$binary" "$head_revision" "$attestation_tmp" "" "$expected_gomaxprocs" "$minimum_free_bytes" true "$launch_config_sha256" "$memory_validation_arg" "$measurement_config_sha256" "$activation_provenance_sha256" "$activation_review_attestation_sha256" ||
	fail "generated capacity attestation failed retained-evidence validation; output retained at $probe_root"
mv -- "$attestation_tmp" "$attestation"
v2_r2_require_binary_capacity_attestation "$binary" "$head_revision" "$attestation" "" "$expected_gomaxprocs" "$minimum_free_bytes" true "$launch_config_sha256" "$memory_validation_arg" "$measurement_config_sha256" "$activation_provenance_sha256" "$activation_review_attestation_sha256" ||
	fail "published capacity attestation failed final validation; output retained at $probe_root"
echo "completed SV1 24-hour binary capacity probe: root=$probe_root peak_bytes=$peak_output_bytes required_free_bytes=$required_free_bytes"
