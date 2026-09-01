#!/usr/bin/env bash
# Measure actual 24-hour evstream_v3 storage for the SV1 treatment before any
# registered development cell. The probe is not a scientific result and is
# retained so its capacity claim can be independently audited.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"
head_revision=$(git -C "$root_dir" rev-parse HEAD)

binary=${1:-"$root_dir/bin/multivenue"}
config="$root_dir/research/configs/v2-r2-sv1-24h/treatment-607.json"
probe_root=${V2_R2_SV1_CAPACITY_ROOT:-"/home/vlad/external-scratch/v2-r2-sv1-24h-capacity-$head_revision"}
attestation=$(v2_r2_capacity_attestation_path)
probe_dir="$probe_root/treatment-607"
horizon=24h
simulation_start_nano=1735689600000000000
simulation_end_nano=1735776000000000000
safety_margin_bytes=$((4 * 1024 * 1024 * 1024))
minimum_free_bytes=$((4 * 1024 * 1024 * 1024))

fail() {
	echo "SV1 capacity probe failure: $*" >&2
	exit 1
}

[[ -x "$binary" && -s "$config" ]] || fail "missing binary or treatment config"
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "scientific worktree is dirty"
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
[[ ! -e "$probe_root" && ! -L "$probe_root" ]] || fail "capacity probe root already exists: $probe_root"
[[ ! -e "$attestation" && ! -L "$attestation" ]] || fail "capacity attestation already exists: $attestation"
[[ "$probe_root" == /* && "$probe_root" != "$root_dir" && "$probe_root" != "$root_dir"/* ]] || fail "capacity root is inside the scientific repository"
mkdir -p -- "$probe_root"
mkdir -- "$probe_dir"

"$binary" -config "$config" -logdir "$probe_dir" -write-effective-config "$probe_dir/run-config.json" >/dev/null 2>"$probe_root/config.stderr.log" || fail "effective-config validation failed"
cmp -s "$config" "$probe_dir/run-config.json" || fail "registered config is not normalized"
config_sha256=$(sha256sum "$probe_dir/run-config.json" | awk '{print $1}')
binary_sha256=$(sha256sum "$binary" | awk '{print $1}')
jq -n --arg git_revision "$head_revision" --arg binary_sha256 "$binary_sha256" --arg config_sha256 "$config_sha256" \
	--arg binary_go_version "$binary_go_version" --argjson seed 607 --arg horizon "$horizon" \
	'{schema_version:1,contract:"v2-r2-sv1-24h-capacity-probe-v1",git_revision:$git_revision,binary_sha256:$binary_sha256,config_sha256:$config_sha256,binary_go_version:$binary_go_version,seed:$seed,simulated_horizon:$horizon,evidence_format:"evstream_v3"}' >"$probe_dir/run-metadata.json"

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
while kill -0 "$simulator_pid" 2>/dev/null; do
	current_bytes=$(du -sb -- "$probe_root" | awk '{print $1}')
	if (( current_bytes > peak_output_bytes )); then
		peak_output_bytes=$current_bytes
		peak_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	fi
	available_bytes=$(df -P -B1 -- "$probe_root" | awk 'NR == 2 {print $4}')
	if [[ "$available_bytes" =~ ^[0-9]+$ ]] && (( available_bytes < minimum_free_bytes )); then
		capacity_abort=true
		kill -TERM "$simulator_pid" 2>/dev/null || true
		break
	fi
	sleep 2
done
set +e
wait "$simulator_pid"
simulator_status=$?
set -e
final_output_bytes=$(du -sb -- "$probe_root" | awk '{print $1}')
(( final_output_bytes > peak_output_bytes )) && peak_output_bytes=$final_output_bytes
[[ "$capacity_abort" == false && "$simulator_status" == 0 ]] || fail "probe did not complete (status=$simulator_status capacity_abort=$capacity_abort); output retained at $probe_root"

for required in run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json evidence-only-artifact-hash.json; do
	[[ -s "$probe_dir/$required" ]] || fail "completed probe is missing $required"
done
jq -e --arg revision "$head_revision" --argjson seed 607 \
	'.build.revision == $revision and .build.modified == false and .config.seed == $seed and .config.evidence_format == "evstream_v3"' "$probe_dir/manifest.json" >/dev/null || fail "probe manifest provenance mismatch"
jq -e --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
	'(.initial_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $start)) and (.terminal_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $end))' "$probe_dir/greeks.json" >/dev/null || fail "probe greeks do not attest the 24-hour horizon"
v2_r2_require_checkpoint_stream "$probe_dir/checkpoints.jsonl" "$simulation_start_nano" "$simulation_end_nano" || fail "probe checkpoints do not attest the 24-hour horizon"
v2_r2_write_evidence_manifest "$probe_dir" || fail "probe evidence manifest generation failed"
v2_r2_verify_evidence_manifest "$probe_dir" || fail "probe evidence manifest verification failed"

available_free_bytes=$(df -P -B1 -- "$probe_root" | awk 'NR == 2 {print $4}')
required_free_bytes=$((peak_output_bytes + safety_margin_bytes))
attestation_tmp="$attestation.tmp-$$"
mkdir -p -- "$(dirname -- "$attestation")"
jq -n --arg source_revision "$head_revision" --arg binary_sha256 "$binary_sha256" \
	--arg config_sha256 "$config_sha256" --arg probe_root "$probe_root" --arg peak_at "$peak_at" \
	--argjson peak_output_bytes "$peak_output_bytes" --argjson safety_margin_bytes "$safety_margin_bytes" \
	--argjson required_free_bytes "$required_free_bytes" --argjson available_free_bytes "$available_free_bytes" \
	'{schema_version:1,contract:"v2-integrated-longrun-r2-binary-capacity-v1",measurement:"full_24h_binary_evidence_capacity_probe",evidence_format:"evstream_v3",source_revision:$source_revision,binary_sha256:$binary_sha256,config_sha256:$config_sha256,probe_root:$probe_root,peak_output_bytes:$peak_output_bytes,safety_margin_bytes:$safety_margin_bytes,required_free_bytes:$required_free_bytes,available_free_bytes:$available_free_bytes,peak_observed_at:$peak_at}' >"$attestation_tmp"
mv -- "$attestation_tmp" "$attestation"
echo "completed SV1 24-hour binary capacity probe: root=$probe_root peak_bytes=$peak_output_bytes required_free_bytes=$required_free_bytes"
