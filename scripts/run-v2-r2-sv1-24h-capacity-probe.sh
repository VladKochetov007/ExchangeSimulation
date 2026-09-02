#!/usr/bin/env bash
# Measure actual 24-hour evstream_v3 storage in a dedicated calibration world.
# The probe is not a scientific result and is retained so its capacity claim
# can be independently audited. SV1B deliberately does not use a registered
# treatment trajectory as the capacity workload.
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
launch_config="${v2_r2_sv1_capacity_launch_config:-$v2_r2_sv1_config_dir/treatment-$primary_seed.json}"
measurement_seed="${v2_r2_sv1_capacity_measurement_seed:-$primary_seed}"
config="${v2_r2_sv1_capacity_measurement_config:-$v2_r2_sv1_config_dir/treatment-$primary_seed.json}"
probe_root_requested=${V2_R2_SV1_CAPACITY_ROOT:-"/home/vlad/external-scratch/${v2_r2_sv1_capacity_probe_prefix}-$head_revision"}
attestation=$(v2_r2_capacity_attestation_path)
horizon=24h
simulation_start_nano=1735689600000000000
simulation_end_nano=1735776000000000000
safety_margin_bytes=$((4 * 1024 * 1024 * 1024))
minimum_free_bytes=$((4 * 1024 * 1024 * 1024))
expected_gomaxprocs=4

fail() {
	echo "SV1 capacity probe failure: $*" >&2
	exit 1
}

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
	[[ "$config" != "$launch_config" ]] || fail "SV1B capacity measurement must use a dedicated calibration config"
fi
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "scientific worktree is dirty"
[[ "${GOMAXPROCS:-}" == "$expected_gomaxprocs" ]] || fail "capacity probe requires GOMAXPROCS=$expected_gomaxprocs"
[[ "$probe_root_requested" == /* && "$probe_root_requested" != */ && "$probe_root_requested" != *$'\n'* && "$probe_root_requested" != *$'\t'* ]] || fail "capacity root must be an absolute, non-empty path"
probe_root=$(realpath -m -- "$probe_root_requested") || fail "could not resolve capacity probe root"
case "$probe_root" in
	"$scientific_root"|"$scientific_root"/*) fail "capacity root resolves inside the scientific repository" ;;
esac
[[ ! -e "$probe_root_requested" && ! -L "$probe_root_requested" ]] || fail "capacity probe root already exists: $probe_root_requested"
[[ ! -e "$attestation" && ! -L "$attestation" ]] || fail "capacity attestation already exists: $attestation"
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
mkdir -p -- "$probe_root"
[[ "$(realpath -e -- "$probe_root")" == "$probe_root" ]] || fail "capacity probe root canonicalization changed after creation"
probe_dir="$probe_root/$v2_r2_capacity_probe_cell"
mkdir -- "$probe_dir"

"$binary" -config "$config" -logdir "$probe_dir" -write-effective-config "$probe_dir/run-config.json" >/dev/null 2>"$probe_root/config.stderr.log" || fail "effective-config validation failed"
cmp -s "$config" "$probe_dir/run-config.json" || fail "registered config is not normalized"
config_sha256=$(sha256sum "$probe_dir/run-config.json" | awk '{print $1}')
launch_config_sha256=$(sha256sum "$launch_config" | awk '{print $1}')
measurement_config_path=${config#"$root_dir/"}
launch_config_path=${launch_config#"$root_dir/"}
calibration_only=false
[[ "$config" != "$launch_config" ]] && calibration_only=true
binary_sha256=$(sha256sum "$binary" | awk '{print $1}')
jq -n --arg git_revision "$head_revision" --arg binary_sha256 "$binary_sha256" --arg config_sha256 "$config_sha256" \
	--arg launch_config_sha256 "$launch_config_sha256" --arg measurement_config_path "$measurement_config_path" \
	--arg launch_config_path "$launch_config_path" --arg binary_go_version "$binary_go_version" --argjson seed "$measurement_seed" --arg horizon "$horizon" \
	--argjson gomaxprocs "$expected_gomaxprocs" --argjson minimum_free_bytes "$minimum_free_bytes" \
	--argjson initial_available_free_bytes "$initial_available_free_bytes" \
	--arg contract "$v2_r2_sv1_capacity_probe_contract" --argjson calibration_only "$calibration_only" \
	'{schema_version:1,contract:$contract,git_revision:$git_revision,binary_sha256:$binary_sha256,config_sha256:$config_sha256,measurement_config_path:$measurement_config_path,launch_config_sha256:$launch_config_sha256,launch_config_path:$launch_config_path,calibration_only:$calibration_only,binary_go_version:$binary_go_version,gomaxprocs:$gomaxprocs,minimum_free_bytes:$minimum_free_bytes,initial_available_free_bytes:$initial_available_free_bytes,seed:$seed,simulated_horizon:$horizon,evidence_format:"evstream_v3"}' >"$probe_dir/run-metadata.json"

stdout_log="$probe_root/simulator.stdout.log"
stderr_log="$probe_root/simulator.stderr.log"
set +e
"$binary" -config "$probe_dir/run-config.json" -duration "$horizon" -logdir "$probe_dir" -log-mode full -evidence-format evstream_v3 \
	>"$stdout_log" 2>"$stderr_log" &
simulator_pid=$!
set -e
peak_output_bytes=0
peak_at=""
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

for required in run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json evidence-only-artifact-hash.json; do
	[[ -s "$probe_dir/$required" ]] || fail "completed probe is missing $required"
done
jq -e --arg revision "$head_revision" --argjson seed "$measurement_seed" \
	'.build.revision == $revision and .build.modified == false and .config.seed == $seed and .config.evidence_format == "evstream_v3"' "$probe_dir/manifest.json" >/dev/null || fail "probe manifest provenance mismatch"
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
	--arg config_sha256 "$config_sha256" --arg launch_config_sha256 "$launch_config_sha256" \
	--arg measurement_config_path "$measurement_config_path" --arg launch_config_path "$launch_config_path" \
	--arg probe_root "$probe_root" --arg peak_at "$peak_at" \
	--arg evidence_manifest_sha256 "$evidence_manifest_sha256" \
	--argjson peak_output_bytes "$peak_output_bytes" --argjson safety_margin_bytes "$safety_margin_bytes" \
	--argjson required_free_bytes "$required_free_bytes" --argjson available_free_bytes "$final_available_free_bytes" \
	--argjson gomaxprocs "$expected_gomaxprocs" --argjson minimum_free_bytes "$minimum_free_bytes" \
	--argjson initial_available_free_bytes "$initial_available_free_bytes" --argjson measurement_seed "$measurement_seed" \
	--argjson calibration_only "$calibration_only" \
	'{schema_version:1,contract:"v2-integrated-longrun-r2-binary-capacity-v1",measurement:"full_24h_binary_evidence_capacity_probe",evidence_format:"evstream_v3",source_revision:$source_revision,binary_sha256:$binary_sha256,config_sha256:$config_sha256,measurement_config_path:$measurement_config_path,measurement_seed:$measurement_seed,launch_config_sha256:$launch_config_sha256,launch_config_path:$launch_config_path,calibration_only:$calibration_only,gomaxprocs:$gomaxprocs,minimum_free_bytes:$minimum_free_bytes,initial_available_free_bytes:$initial_available_free_bytes,probe_root:$probe_root,evidence_manifest_sha256:$evidence_manifest_sha256,peak_output_bytes:$peak_output_bytes,safety_margin_bytes:$safety_margin_bytes,required_free_bytes:$required_free_bytes,available_free_bytes:$available_free_bytes,peak_observed_at:$peak_at}' >"$attestation_tmp"
v2_r2_require_binary_capacity_attestation "$binary" "$head_revision" "$attestation_tmp" "$config_sha256" "$expected_gomaxprocs" "$minimum_free_bytes" true "$launch_config_sha256" ||
	fail "generated capacity attestation failed retained-evidence validation; output retained at $probe_root"
mv -- "$attestation_tmp" "$attestation"
v2_r2_require_binary_capacity_attestation "$binary" "$head_revision" "$attestation" "$config_sha256" "$expected_gomaxprocs" "$minimum_free_bytes" true "$launch_config_sha256" ||
	fail "published capacity attestation failed final validation; output retained at $probe_root"
echo "completed SV1 24-hour binary capacity probe: root=$probe_root peak_bytes=$peak_output_bytes required_free_bytes=$required_free_bytes"
