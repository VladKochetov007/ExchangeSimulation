#!/usr/bin/env bash
# Run the registered development-only SV1D tri-arm activation probe.
#
# This adapter is deliberately narrower than the 24-hour campaign runner:
# treatment, same-roster mode-off, and no-roster are the only accepted arms;
# every arm gets a fresh namespace; and the command refuses to start until an
# independent review and a matching measured binary-capacity attestation have
# been supplied explicitly.
set -euo pipefail

if [[ $# -gt 3 ]]; then
	echo "usage: $0 [multivenue-binary] [sv1dprobe-binary] [evsrender-binary]" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-r2-sv1d-activation"
multivenue_binary=${1:-"$root_dir/bin/multivenue"}
sv1dprobe_binary=${2:-"$root_dir/bin/sv1dprobe"}
evsrender_binary=${3:-"$root_dir/bin/evsrender"}
output_root=${SV1D_OUTPUT_ROOT:-"/home/vlad/v2-r2-sv1d-activation-659-v1"}
capacity_attestation=${SV1D_CAPACITY_ATTESTATION:-"/home/vlad/v2-integrated-longrun-r2-binary-capacity-v1.json"}
lock_path="/home/vlad/v2-r2-sv1d-activation-659.lock"

fail() {
	echo "SV1D activation runner: $*" >&2
	exit 1
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

[[ "${SV1D_REVIEW_ACCEPTED:-0}" == 1 ]] || fail "set SV1D_REVIEW_ACCEPTED=1 only after fresh exact-tree review acceptance"
[[ "${SV1D_PROBE_AUTHORIZED:-0}" == 1 ]] || fail "set SV1D_PROBE_AUTHORIZED=1 at the explicit development-probe boundary"

[[ -d "$root_dir/.git" ]] || fail "repository root is not a Git worktree"
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "source worktree must be clean"
source_revision=$(git -C "$root_dir" rev-parse HEAD)
[[ "$source_revision" =~ ^[0-9a-f]{40}$ ]] || fail "invalid source revision: $source_revision"

[[ -d "$config_dir" && ! -L "$config_dir" ]] || fail "missing or symlinked SV1D config directory"
"$root_dir/scripts/check-v2-r2-sv1d-activation-configs.sh" >/dev/null || fail "registered SV1D config contract failed"

require_binary multivenue "$multivenue_binary"
require_binary sv1dprobe "$sv1dprobe_binary"
require_binary evsrender "$evsrender_binary"
multivenue_binary=$(realpath -e -- "$multivenue_binary")
sv1dprobe_binary=$(realpath -e -- "$sv1dprobe_binary")
evsrender_binary=$(realpath -e -- "$evsrender_binary")
require_clean_pinned_binary multivenue "$multivenue_binary" "$source_revision"
require_clean_pinned_binary sv1dprobe "$sv1dprobe_binary" "$source_revision"
require_clean_pinned_binary evsrender "$evsrender_binary" "$source_revision"

source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"
v2_r2_require_binary_capacity_attestation "$multivenue_binary" "$source_revision" "$capacity_attestation" ||
	fail "matching measured binary-evidence capacity attestation is missing or disk headroom is unsafe"

[[ "$output_root" == /* ]] || fail "SV1D output root must be absolute"
require_no_symlink_components "$output_root" || fail "SV1D output root contains a symlink"
[[ ! -e "$output_root" && ! -L "$output_root" ]] || fail "refusing to overwrite SV1D evidence root: $output_root"
[[ ! -L "$lock_path" ]] || fail "SV1D namespace lock is symlinked"
exec {lock_fd}>"$lock_path" || fail "cannot open SV1D namespace lock"
flock -n "$lock_fd" || fail "another SV1D activation run holds the namespace lock"

export GOMAXPROCS=2
export GOMEMLIMIT=4GiB

treatment_config="$config_dir/activation-659-treatment.json"
mode_off_config="$config_dir/activation-659-mode-off.json"
no_roster_config="$config_dir/activation-659-no-roster.json"
mkdir -p "$output_root"
config_sha256() { sha256sum -- "$1" | awk '{print $1}'; }
multivenue_sha256=$(sha256sum -- "$multivenue_binary" | awk '{print $1}')
sv1dprobe_sha256=$(sha256sum -- "$sv1dprobe_binary" | awk '{print $1}')
evsrender_sha256=$(sha256sum -- "$evsrender_binary" | awk '{print $1}')

plan_path="$output_root/probe-plan.json"
"$sv1dprobe_binary" -mode plan -out "$plan_path" \
	-treatment-config "$treatment_config" -mode-off-config "$mode_off_config" \
	-no-roster-config "$no_roster_config" -source-revision "$source_revision" \
	-binary-sha256 "$multivenue_sha256" -analyzer-sha256 "$sv1dprobe_sha256" \
	-renderer-sha256 "$evsrender_sha256" || fail "could not publish immutable SV1D probe plan"
[[ -f "$plan_path" && ! -L "$plan_path" ]] || fail "probe plan was not published"

simulation_start_nano=1735689600000000000
simulation_end_nano=1735689900000000000
probe_horizon=5m
mkdir -p "$output_root/arms"

declare -A config_for=(
	[treatment]="$treatment_config"
	[mode-off]="$mode_off_config"
	[no-roster]="$no_roster_config"
)

for arm in treatment mode-off no-roster; do
	config=${config_for[$arm]}
	arm_dir="$output_root/arms/$arm"
	rendered_dir="$output_root/rendered/$arm"
	stdout_log="$output_root/$arm.simulator.stdout.log"
	stderr_log="$output_root/$arm.simulator.stderr.log"
	result_path="$output_root/results/$arm.json"
	[[ ! -e "$arm_dir" && ! -L "$arm_dir" ]] || fail "refusing to overwrite arm directory: $arm_dir"
	[[ ! -e "$rendered_dir" && ! -L "$rendered_dir" ]] || fail "refusing to overwrite rendered directory: $rendered_dir"
	[[ ! -e "$result_path" && ! -L "$result_path" ]] || fail "refusing to overwrite arm result: $result_path"
	mkdir -p "$arm_dir"
	cp -- "$config" "$arm_dir/run-config.json"
	cmp -s "$config" "$arm_dir/run-config.json" || fail "config copy changed for $arm"

	config_digest=$(config_sha256 "$config")
	experiment_id=$(jq -er '.experiment_id' "$config")
	hypothesis_id=$(jq -er '.hypothesis_id' "$config")
	log_mode=$(jq -er '.log_mode' "$config")
	evidence_format=$(jq -er '.evidence_format' "$config")
	jq -n \
		--arg arm "$arm" --arg experiment_id "$experiment_id" --arg hypothesis_id "$hypothesis_id" \
		--argjson seed 659 --arg horizon "$probe_horizon" \
		--argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		--arg config_sha256 "$config_digest" --arg binary_sha256 "$multivenue_sha256" \
		--arg git_revision "$source_revision" --arg binary_path "$multivenue_binary" \
		--arg binary_go_version "$(binary_go_version "$multivenue_binary")" \
		--arg analyzer_sha256 "$sv1dprobe_sha256" --arg renderer_sha256 "$evsrender_sha256" \
		--arg log_mode "$log_mode" --arg evidence_format "$evidence_format" \
		--arg output_dir "$arm_dir" --argjson gomaxprocs 2 \
		'{schema_version: 1, runner_contract: "v2-r2-sv1d-activation-runner-v1", probe_id: "v2-r2-sv1d-activation-659", arm: $arm,
		  experiment_id: $experiment_id, hypothesis_id: $hypothesis_id, seed: $seed, simulated_horizon: $horizon,
		  simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		  config_sha256: $config_sha256, binary_sha256: $binary_sha256, git_revision: $git_revision,
		  binary_path: $binary_path, binary_go_version: $binary_go_version, binary_goos: "linux", binary_goarch: "amd64", binary_goamd64: "v1",
		  analyzer_sha256: $analyzer_sha256, renderer_sha256: $renderer_sha256, log_mode: $log_mode, evidence_format: $evidence_format,
		  gomaxprocs: $gomaxprocs, output_dir: $output_dir, holdout: false,
		  command: ["multivenue", "-config", "run-config.json", "-duration", "5m", "-log-mode", "full", "-evidence-format", "evstream_v3"],
		  raw_log_policy: "retain until the complete SV1D arm and tri-arm score have passed independent review"}' \
		>"$arm_dir/run-metadata.json"
	run_metadata_sha256_before=$(sha256sum -- "$arm_dir/run-metadata.json" | awk '{print $1}')

	set +e
	GOMAXPROCS=2 GOMEMLIMIT=4GiB "$multivenue_binary" -config "$arm_dir/run-config.json" -duration "$probe_horizon" \
		-logdir "$arm_dir" -log-mode full -evidence-format evstream_v3 >"$stdout_log" 2>"$stderr_log"
	status=$?
	set -e
	[[ "$status" -eq 0 ]] || fail "$arm simulator failed with status $status; retain $arm_dir"
	[[ -s "$arm_dir/greeks.json" && -s "$arm_dir/latency.json" ]] || fail "$arm completion sentinels are missing"
	[[ "$run_metadata_sha256_before" == "$(sha256sum -- "$arm_dir/run-metadata.json" | awk '{print $1}')" ]] || fail "$arm metadata changed during execution"
	jq -e --arg revision "$source_revision" --arg experiment "$experiment_id" \
		'.schema_version == 2 and .build.revision == $revision and .build.modified == false and
		 .build.goos == "linux" and .build.goarch == "amd64" and .build.goamd64 == "v1" and
		 .config.seed == 659 and .config.experiment_id == $experiment and .config.log_mode == "full" and .config.evidence_format == "evstream_v3"' \
		"$arm_dir/manifest.json" >/dev/null || fail "$arm manifest provenance/config identity mismatch"
	jq -e --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
		'all(.initial_accounts[]; .account.timestamp == $start) and all(.terminal_accounts[]; .account.timestamp == $end)' \
		"$arm_dir/greeks.json" >/dev/null || fail "$arm terminal valuation does not attest the registered 5m horizon"

	v2_r2_write_evidence_manifest "$arm_dir" || fail "could not write $arm evidence manifest"
	v2_r2_verify_evidence_manifest "$arm_dir" || fail "$arm evidence manifest did not verify"
	status_tmp="$arm_dir/run-status.json.tmp-$$"
	jq -n \
		--argjson exit_status "$status" --arg arm "$arm" --arg horizon "$probe_horizon" \
		--argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		--arg run_metadata_sha256 "$run_metadata_sha256_before" \
		--arg manifest_sha256 "$(sha256sum -- "$arm_dir/manifest.json" | awk '{print $1}')" \
		--arg greeks_sha256 "$(sha256sum -- "$arm_dir/greeks.json" | awk '{print $1}')" \
		--arg latency_sha256 "$(sha256sum -- "$arm_dir/latency.json" | awk '{print $1}')" \
		--arg checkpoints_sha256 "$(sha256sum -- "$arm_dir/checkpoints.jsonl" | awk '{print $1}')" \
		--arg evidence_manifest_sha256 "$(sha256sum -- "$arm_dir/evidence-manifest.json" | awk '{print $1}')" \
		--arg binary_attestation_sha256 "$(sha256sum -- "$arm_dir/binary-evidence-attestation.json" | awk '{print $1}')" \
		--arg market_data_evidence_sha256 "$(sha256sum -- "$arm_dir/market-data-evidence-v2.json" | awk '{print $1}')" \
		--arg market_data_schedules_sha256 "$(sha256sum -- "$arm_dir/market-data-schedules-v2.bin" | awk '{print $1}')" \
		--arg market_data_receipts_sha256 "$(sha256sum -- "$arm_dir/market-data-receipts-v2.bin" | awk '{print $1}')" \
		--arg market_data_decisions_sha256 "$(sha256sum -- "$arm_dir/market-data-decisions-v2.bin" | awk '{print $1}')" \
		'{schema_version: 1, cell: $arm, exit_status: $exit_status, completion_verified: true, simulated_horizon: $horizon,
		  simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		  completion_sentinels: ["greeks.json", "latency.json"], run_metadata_sha256: $run_metadata_sha256,
		  manifest_sha256: $manifest_sha256, greeks_sha256: $greeks_sha256, latency_sha256: $latency_sha256,
		  checkpoints_sha256: $checkpoints_sha256, evidence_manifest_sha256: $evidence_manifest_sha256,
		  binary_evidence_attestation_sha256: $binary_attestation_sha256,
		  market_data_evidence_sha256: $market_data_evidence_sha256, market_data_schedules_sha256: $market_data_schedules_sha256,
		  market_data_receipts_sha256: $market_data_receipts_sha256, market_data_decisions_sha256: $market_data_decisions_sha256}' \
		>"$status_tmp"
	mv -- "$status_tmp" "$arm_dir/run-status.json"

	mkdir -p "$output_root/rendered"
	"$evsrender_binary" -dir "$arm_dir" -out "$rendered_dir" >"$output_root/$arm.renderer-report.json" || fail "$arm renderer failed"
	jq -e --argjson event_frames "$(jq -er '.event_frames' "$arm_dir/binary-evidence-attestation.json")" \
		--argjson stream_frames "$(jq -er '.stream_frames' "$arm_dir/binary-evidence-attestation.json")" \
		--arg execution_hash "$(jq -er '.execution_stream_hash' "$arm_dir/binary-evidence-attestation.json")" \
		'.event_frames == $event_frames and .dictionary_frames + .event_frames == $stream_frames and .execution_stream_hash == $execution_hash and .routes > 0 and (.rendered_digest | test("^[0-9a-f]{64}$"))' \
		"$output_root/$arm.renderer-report.json" >/dev/null || fail "$arm renderer report is not bound to the source stream"

	"$sv1dprobe_binary" -mode audit -out "$result_path" -plan "$plan_path" -arm "$arm" \
		-run-dir "$arm_dir" -rendered-dir "$rendered_dir" || fail "$arm strict audit failed; retain all evidence"
	[[ -s "$result_path" && ! -L "$result_path" ]] || fail "$arm audit result was not published"
done

score_path="$output_root/score.json"
"$sv1dprobe_binary" -mode score -out "$score_path" -plan "$plan_path" \
	-treatment-result "$output_root/results/treatment.json" \
	-mode-off-result "$output_root/results/mode-off.json" \
	-no-roster-result "$output_root/results/no-roster.json" || fail "could not publish tri-arm score"
jq -e 'type == "object" and .contract == "v2-r2-sv1d-score-v1" and .probe_id == "v2-r2-sv1d-activation-659" and (.score.status | type == "string")' \
	"$score_path" >/dev/null || fail "tri-arm score is malformed"
echo "completed development-only SV1D activation probe: $output_root"
