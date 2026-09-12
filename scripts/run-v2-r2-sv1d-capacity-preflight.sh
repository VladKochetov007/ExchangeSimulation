#!/usr/bin/env bash
# Execute the outcome-ineligible, measured five-minute SV1D capacity preflight.
# The normal entrypoint verifies the exact external review and then runs three
# capacity-only arms. The private internal entrypoint is used only as the
# command sampled by sv1dresource, so the simulator and renderer share one
# measured process tree.
set -euo pipefail

root_dir=${SV1D_CAPACITY_ROOT_DIR:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
capacity_script=${SV1D_CAPACITY_SCRIPT_PATH:-"$root_dir/scripts/run-v2-r2-sv1d-capacity-preflight.sh"}
config_dir="$root_dir/research/configs/v2-r2-sv1d-activation"
policy_path="$root_dir/research/configs/v2-r2-sv1d-capacity/resource-policy-v1.json"
parent_registration_path="$root_dir/research/v2-r2-sv1d-one-sided-elastic-successor-preregistration-2026-09-10.md"
amendment_path="$root_dir/research/v2-r2-sv1d-activation-contract-amendment-2026-09-11.md"

capacity_seed=977
capacity_horizon=5m
simulation_start_nano=1735689600000000000
simulation_end_nano=1735689900000000000
capacity_hypothesis="V2-R2-SV1D-CAPACITY-ONLY"
capacity_contract="v2-r2-sv1d-capacity-runner-v1"
capacity_probe_id="v2-r2-sv1d-activation-659"
capacity_lock_path=${SV1D_CAPACITY_LOCK_PATH:-"/home/vlad/v2-r2-sv1d-capacity-977.lock"}
output_root=${SV1D_CAPACITY_OUTPUT_ROOT:-"/home/vlad/v2-r2-sv1d-capacity-977-v1"}

fail() {
	echo "SV1D capacity preflight: $*" >&2
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
	IFS=/ read -r -a components <<<"$path_without_root"
	for component in "${components[@]}"; do
		[[ -n "$component" ]] || continue
		current="${current%/}/$component"
		[[ ! -L "$current" ]] || return 1
	done
}

require_regular_file() {
	local name=$1 path=$2
	[[ -f "$path" && ! -L "$path" ]] || fail "missing regular $name: $path"
	require_no_symlink_components "$path" || fail "$name path contains a symlink: $path"
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
	[[ "$revision" == "$expected_revision" ]] || fail "$name revision does not match HEAD"
	[[ "$modified" == false ]] || fail "$name has vcs.modified=$modified"
	[[ "$trimpath" == true && "$cgo_enabled" == 0 ]] || fail "$name is not trimpath/CGO-disabled"
	[[ "$goos" == linux && "$goarch" == amd64 && "$goamd64" == v1 ]] || fail "$name is not linux/amd64/v1"
	[[ "$go_version" == go1.27* ]] || fail "$name is not built with Go 1.27: $go_version"
}

copy_immutable_binary() {
	local name=$1 source=$2 destination=$3
	require_binary "$name" "$source"
	[[ ! -e "$destination" && ! -L "$destination" ]] || fail "refusing to overwrite staged $name"
	cp -- "$source" "$destination"
	chmod 0555 -- "$destination"
	[[ "$(sha256sum -- "$source" | awk '{print $1}')" == "$(sha256sum -- "$destination" | awk '{print $1}')" ]] || fail "$name changed during immutable copy"
}

copy_immutable_file() {
	local name=$1 source=$2 destination=$3
	require_regular_file "$name" "$source"
	[[ ! -e "$destination" && ! -L "$destination" ]] || fail "refusing to overwrite staged $name"
	cp -- "$source" "$destination"
	cmp -s "$source" "$destination" || fail "$name changed during immutable copy"
}

config_sha256() {
	sha256sum -- "$1" | awk '{print $1}'
}

hash_file() {
	sha256sum -- "$1" | awk '{print $1}'
}

run_capacity_arm() {
	local arm=$1 config=$2 arm_dir=$3 rendered_dir=$4 simulator=$5 analyzer=$6 renderer=$7 source_revision=$8 experiment_id=$9 stdout_log=${10} stderr_log=${11}
	local config_digest hypothesis_id log_mode evidence_format run_metadata_sha256_before status

	[[ ! -e "$arm_dir" && ! -L "$arm_dir" ]] || return 91
	[[ ! -e "$rendered_dir" && ! -L "$rendered_dir" ]] || return 92
	mkdir -p "$arm_dir"
	cp -- "$config" "$arm_dir/run-config.json"
	cmp -s "$config" "$arm_dir/run-config.json" || return 93
	config_digest=$(config_sha256 "$config")
	hypothesis_id=$(jq -er '.hypothesis_id' "$config")
	log_mode=$(jq -er '.log_mode' "$config")
	evidence_format=$(jq -er '.evidence_format' "$config")
	jq -n \
		--arg arm "$arm" --arg experiment_id "$experiment_id" --arg hypothesis_id "$hypothesis_id" \
		--argjson seed "$capacity_seed" --arg horizon "$capacity_horizon" \
		--argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		--arg config_sha256 "$config_digest" --arg binary_sha256 "$(hash_file "$simulator")" \
		--arg git_revision "$source_revision" --arg binary_path "$simulator" \
		--arg binary_go_version "$(binary_go_version "$simulator")" --arg analyzer_sha256 "$(hash_file "$analyzer")" \
		--arg renderer_sha256 "$(hash_file "$renderer")" --arg log_mode "$log_mode" --arg evidence_format "$evidence_format" \
		--arg output_dir "$arm_dir" --argjson gomaxprocs 2 --arg runner_contract "$capacity_contract" --arg probe_id "$capacity_probe_id" \
		'{schema_version: 1, runner_contract: $runner_contract, probe_id: $probe_id, capacity_only: true, scientific_result_eligible: false,
		  arm: $arm, experiment_id: $experiment_id, config_experiment_id: $experiment_id, hypothesis_id: $hypothesis_id,
		  seed: $seed, simulated_horizon: $horizon, simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		  config_sha256: $config_sha256, binary_sha256: $binary_sha256, git_revision: $git_revision, binary_path: $binary_path,
		  binary_go_version: $binary_go_version, binary_goos: "linux", binary_goarch: "amd64", binary_goamd64: "v1",
		  analyzer_sha256: $analyzer_sha256, renderer_sha256: $renderer_sha256, log_mode: $log_mode, evidence_format: $evidence_format,
		  gomaxprocs: $gomaxprocs, output_dir: $output_dir, holdout: false,
		  command: ["multivenue", "-config", "run-config.json", "-duration", "5m", "-log-mode", "full", "-evidence-format", "evstream_v3"],
		  raw_log_policy: "retain until the capacity attestation and successor promotion gates pass"}' \
		>"$arm_dir/run-metadata.json" || return 94
	run_metadata_sha256_before=$(hash_file "$arm_dir/run-metadata.json")

	set +e
	GOMAXPROCS=2 GOMEMLIMIT=4GiB "$simulator" -config "$arm_dir/run-config.json" -duration "$capacity_horizon" \
		-logdir "$arm_dir" -log-mode full -evidence-format evstream_v3 >"$stdout_log" 2>"$stderr_log"
	status=$?
	set -e
	[[ "$status" -eq 0 ]] || return "$status"
	[[ -s "$arm_dir/greeks.json" && -s "$arm_dir/latency.json" ]] || return 95
	[[ "$run_metadata_sha256_before" == "$(hash_file "$arm_dir/run-metadata.json")" ]] || return 96
	jq -e --arg revision "$source_revision" --arg experiment "$experiment_id" \
		'.schema_version == 2 and .build.revision == $revision and .build.modified == false and
		 .build.goos == "linux" and .build.goarch == "amd64" and .build.goamd64 == "v1" and
		 .config.seed == 977 and .config.experiment_id == $experiment and .config.log_mode == "full" and
		 .config.evidence_format == "evstream_v3" and .config.evidence_contract_version == 2' \
		"$arm_dir/manifest.json" >/dev/null || return 97
	jq -e --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
		'all(.initial_accounts[]; .account.timestamp == $start) and all(.terminal_accounts[]; .account.timestamp == $end)' \
		"$arm_dir/greeks.json" >/dev/null || return 98
	jq -e -s --argjson end "$simulation_end_nano" \
		--argjson event_frames "$(jq -er '.event_frames' "$arm_dir/binary-evidence-attestation.json")" \
		--arg execution_hash "$(jq -er '.execution_stream_hash' "$arm_dir/binary-evidence-attestation.json")" \
		'. as $checkpoints | ($checkpoints | length) > 0 and
		 all($checkpoints[]; .domain == "execution_observations" and .ordering == "ordered_stream" and
			(.sim_time | type) == "number" and (.event_count | type) == "number" and .event_count > 0 and
			(.execution_stream_hash | test("^[0-9a-f]{64}$")) and .representation == "evstream_v3" and (.unencodable_payloads // 0) == 0) and
		 all(range(1; ($checkpoints | length)); $checkpoints[. - 1].sim_time < $checkpoints[.].sim_time and $checkpoints[. - 1].event_count < $checkpoints[.].event_count) and
		 $checkpoints[-1].sim_time == $end and $checkpoints[-1].event_count == $event_frames and
		 $checkpoints[-1].execution_stream_hash == $execution_hash' \
		"$arm_dir/checkpoints.jsonl" >/dev/null || return 99

	v2_r2_write_evidence_manifest "$arm_dir" || return 100
	v2_r2_verify_evidence_manifest "$arm_dir" || return 101
	local status_tmp="$arm_dir/run-status.json.tmp-$$"
	jq -n \
		--argjson exit_status "$status" --arg arm "$arm" --arg experiment_id "$experiment_id" --arg hypothesis_id "$hypothesis_id" \
		--arg horizon "$capacity_horizon" --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		--arg run_metadata_sha256 "$run_metadata_sha256_before" --arg manifest_sha256 "$(hash_file "$arm_dir/manifest.json")" \
		--arg greeks_sha256 "$(hash_file "$arm_dir/greeks.json")" --arg latency_sha256 "$(hash_file "$arm_dir/latency.json")" \
		--arg checkpoints_sha256 "$(hash_file "$arm_dir/checkpoints.jsonl")" --arg evidence_manifest_sha256 "$(hash_file "$arm_dir/evidence-manifest.json")" \
		--arg binary_attestation_sha256 "$(hash_file "$arm_dir/binary-evidence-attestation.json")" \
		--arg market_data_evidence_sha256 "$(hash_file "$arm_dir/market-data-evidence-v2.json")" \
		--arg market_data_schedules_sha256 "$(hash_file "$arm_dir/market-data-schedules-v2.bin")" \
		--arg market_data_receipts_sha256 "$(hash_file "$arm_dir/market-data-receipts-v2.bin")" \
		--arg market_data_decisions_sha256 "$(hash_file "$arm_dir/market-data-decisions-v2.bin")" \
		'{schema_version: 1, contract: "v2-r2-sv1d-capacity-arm-status-v1", capacity_only: true, scientific_result_eligible: false,
		  cell: $arm, experiment_id: $experiment_id, config_experiment_id: $experiment_id, hypothesis_id: $hypothesis_id,
		  exit_status: $exit_status, completion_verified: true, simulated_horizon: $horizon,
		  simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		  completion_sentinels: ["greeks.json", "latency.json"], run_metadata_sha256: $run_metadata_sha256,
		  manifest_sha256: $manifest_sha256, greeks_sha256: $greeks_sha256, latency_sha256: $latency_sha256,
		  checkpoints_sha256: $checkpoints_sha256, evidence_manifest_sha256: $evidence_manifest_sha256,
		  binary_evidence_attestation_sha256: $binary_attestation_sha256,
		  market_data_evidence_sha256: $market_data_evidence_sha256, market_data_schedules_sha256: $market_data_schedules_sha256,
		  market_data_receipts_sha256: $market_data_receipts_sha256, market_data_decisions_sha256: $market_data_decisions_sha256}' \
		>"$status_tmp" || return 102
	mv -- "$status_tmp" "$arm_dir/run-status.json" || return 103

	mkdir -p "$(dirname -- "$rendered_dir")"
	"$renderer" -dir "$arm_dir" -out "$rendered_dir" >"$arm_dir/renderer-report.json" || return 104
	jq -e --argjson event_frames "$(jq -er '.event_frames' "$arm_dir/binary-evidence-attestation.json")" \
		--argjson stream_frames "$(jq -er '.stream_frames' "$arm_dir/binary-evidence-attestation.json")" \
		--arg execution_hash "$(jq -er '.execution_stream_hash' "$arm_dir/binary-evidence-attestation.json")" \
		'.event_frames == $event_frames and .dictionary_frames + .event_frames == $stream_frames and
		 .execution_stream_hash == $execution_hash and .routes > 0 and (.rendered_digest | test("^[0-9a-f]{64}$"))' \
		"$arm_dir/renderer-report.json" >/dev/null || return 105
	return 0
}

# Internal arm invocation is deliberately after all helper definitions but
# before normal argument parsing. The resource wrapper executes this exact
# checked-in file so simulator and renderer remain within one sampled tree.
# FD3 remains the namespace lock, inherited through sv1dresource. The private
# arm also rechecks the signed exact-tree review before it can create output;
# the public resource adapter is not itself an authorization capability.
if [[ "${1:-}" == "--internal-arm" ]]; then
	shift
	[[ "${SV1D_LOCK_HELD:-0}" == 1 ]] || exit 1
	[[ $# -eq 11 ]] || exit 2
	[[ "$output_root" == /* && "$output_root" != "/" && "$(realpath -m -- "$output_root")" == "$output_root" ]] || exit 7
	[[ -d "$root_dir/.git" ]] || exit 7
	[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || exit 7
	current_source_revision=$(git -C "$root_dir" rev-parse HEAD 2>/dev/null) || exit 7
	current_tree_revision=$(git -C "$root_dir" rev-parse HEAD^{tree} 2>/dev/null) || exit 7
	arm=$1
	config=$2
	arm_dir=$3
	rendered_dir=$4
	simulator=$5
	analyzer=$6
	renderer=$7
	source_revision=$8
	experiment_id=$9
	stdout_log=${10}
	stderr_log=${11}
	case "$arm" in
		treatment|mode-off|no-roster)
			expected_experiment_id="v2-r2-sv1d-capacity-977-$arm"
			case "$arm" in
				treatment) target_config=${SV1D_CAPACITY_INTERNAL_TREATMENT_CONFIG:-} ;;
				mode-off) target_config=${SV1D_CAPACITY_INTERNAL_MODE_OFF_CONFIG:-} ;;
				no-roster) target_config=${SV1D_CAPACITY_INTERNAL_NO_ROSTER_CONFIG:-} ;;
			esac
			case "$arm" in
				treatment) expected_target_config_sha256=${SV1D_CAPACITY_INTERNAL_TREATMENT_CONFIG_SHA256:-} ;;
				mode-off) expected_target_config_sha256=${SV1D_CAPACITY_INTERNAL_MODE_OFF_CONFIG_SHA256:-} ;;
				no-roster) expected_target_config_sha256=${SV1D_CAPACITY_INTERNAL_NO_ROSTER_CONFIG_SHA256:-} ;;
			esac
			;;
		*) exit 8 ;;
	esac
	[[ "$source_revision" =~ ^[0-9a-f]{40}$ && "$source_revision" == "$current_source_revision" ]] || exit 8
	[[ "${SV1D_CAPACITY_INTERNAL_TREE_REVISION:-}" == "$current_tree_revision" ]] || exit 8
	[[ "$experiment_id" == "$expected_experiment_id" ]] || exit 8
	[[ "$config" == "$output_root/configs/capacity-$arm.json" ]] || exit 8
	[[ "$arm_dir" == "$output_root/arms/$arm" ]] || exit 8
	[[ "$rendered_dir" == "$output_root/rendered/$arm" ]] || exit 8
	[[ "$stdout_log" == "$output_root/logs/$arm.simulator.stdout.log" ]] || exit 8
	[[ "$stderr_log" == "$output_root/logs/$arm.simulator.stderr.log" ]] || exit 8
	[[ ! -e "$arm_dir" && ! -L "$arm_dir" && ! -e "$rendered_dir" && ! -L "$rendered_dir" ]] || exit 8
	[[ "$(readlink "/proc/$$/fd/3" 2>/dev/null)" == "$capacity_lock_path" ]] || exit 3
	flock -n 3 || exit 4
	resource_parent_exe=$(readlink "/proc/$PPID/exe" 2>/dev/null) || exit 4
	resource_parent_name=${resource_parent_exe##*/}
	[[ "$resource_parent_name" == sv1dresource || "$resource_parent_name" == sv1dresource-* ]] || exit 4
	[[ "${SV1D_CAPACITY_INTERNAL_RESOURCE_SHA256:-}" =~ ^[0-9a-f]{64}$ ]] || exit 10
	[[ "$(hash_file "$resource_parent_exe")" == "$SV1D_CAPACITY_INTERNAL_RESOURCE_SHA256" ]] || exit 10
	internal_review_attestation=${SV1D_CAPACITY_INTERNAL_REVIEW_ATTESTATION:-}
	internal_review_report=${SV1D_CAPACITY_INTERNAL_REVIEW_REPORT:-}
	internal_trusted_key=${SV1D_CAPACITY_INTERNAL_TRUSTED_KEY:-}
	internal_review_plan=${SV1D_CAPACITY_INTERNAL_REVIEW_PLAN:-}
	internal_parent_registration=${SV1D_CAPACITY_INTERNAL_PARENT_REGISTRATION:-}
	internal_amendment=${SV1D_CAPACITY_INTERNAL_AMENDMENT:-}
	for review_file in "$internal_review_attestation" "$internal_review_report" "$internal_trusted_key" "$internal_review_plan" "$internal_parent_registration" "$internal_amendment" "$target_config" "$config" "$simulator" "$analyzer" "$renderer" "$0"; do
		[[ -n "$review_file" ]] || exit 9
	done
	for review_file in "$internal_review_attestation" "$internal_review_report" "$internal_trusted_key" "$internal_review_plan" "$internal_parent_registration" "$internal_amendment" "$target_config" "$config"; do
		require_regular_file internal-review-input "$review_file"
	done
	for binary in "$simulator" "$analyzer" "$renderer" "$0"; do
		[[ -f "$binary" && ! -L "$binary" && -x "$binary" ]] || exit 9
		require_no_symlink_components "$binary" || exit 9
	done
	[[ "$(hash_file "$internal_review_attestation")" == "${SV1D_CAPACITY_INTERNAL_REVIEW_ATTESTATION_SHA256:-}" ]] || exit 9
	[[ "$(hash_file "$internal_review_report")" == "${SV1D_CAPACITY_INTERNAL_REVIEW_REPORT_SHA256:-}" ]] || exit 9
	[[ "$(hash_file "$internal_trusted_key")" == "${SV1D_CAPACITY_INTERNAL_TRUSTED_KEY_SHA256:-}" ]] || exit 9
	[[ "$(hash_file "$internal_review_plan")" == "${SV1D_CAPACITY_INTERNAL_REVIEW_PLAN_SHA256:-}" ]] || exit 9
	[[ "$(hash_file "$internal_parent_registration")" == "${SV1D_CAPACITY_INTERNAL_PARENT_REGISTRATION_SHA256:-}" ]] || exit 9
	[[ "$(hash_file "$internal_amendment")" == "${SV1D_CAPACITY_INTERNAL_AMENDMENT_SHA256:-}" ]] || exit 9
	[[ "$expected_target_config_sha256" =~ ^[0-9a-f]{64}$ ]] || exit 9
	[[ "$(hash_file "$target_config")" == "$expected_target_config_sha256" ]] || exit 9
	[[ "${SV1D_CAPACITY_INTERNAL_ANALYZER_SHA256:-}" =~ ^[0-9a-f]{64}$ ]] || exit 9
	[[ "${SV1D_CAPACITY_INTERNAL_SIMULATOR_SHA256:-}" =~ ^[0-9a-f]{64}$ ]] || exit 9
	[[ "${SV1D_CAPACITY_INTERNAL_RENDERER_SHA256:-}" =~ ^[0-9a-f]{64}$ ]] || exit 9
	[[ "${SV1D_CAPACITY_INTERNAL_RUNNER_SHA256:-}" =~ ^[0-9a-f]{64}$ ]] || exit 9
	[[ "$(hash_file "$simulator")" == "$SV1D_CAPACITY_INTERNAL_SIMULATOR_SHA256" ]] || exit 10
	[[ "$(hash_file "$analyzer")" == "$SV1D_CAPACITY_INTERNAL_ANALYZER_SHA256" ]] || exit 10
	[[ "$(hash_file "$renderer")" == "$SV1D_CAPACITY_INTERNAL_RENDERER_SHA256" ]] || exit 10
	[[ "$(hash_file "$0")" == "${SV1D_CAPACITY_INTERNAL_RUNNER_SHA256:-}" ]] || exit 10
	require_clean_pinned_binary multivenue "$simulator" "$source_revision"
	require_clean_pinned_binary sv1dprobe "$analyzer" "$source_revision"
	require_clean_pinned_binary evsrender "$renderer" "$source_revision"
	"$analyzer" -mode verify-review \
		-review-attestation "$internal_review_attestation" -review-report "$internal_review_report" -trusted-review-key "$internal_trusted_key" \
		-source-revision "$source_revision" -tree-revision "$current_tree_revision" -plan-sha256 "$SV1D_CAPACITY_INTERNAL_REVIEW_PLAN_SHA256" \
		-parent-registration-sha256 "$SV1D_CAPACITY_INTERNAL_PARENT_REGISTRATION_SHA256" -amendment-sha256 "$SV1D_CAPACITY_INTERNAL_AMENDMENT_SHA256" \
		-treatment-config "${SV1D_CAPACITY_INTERNAL_TREATMENT_CONFIG:?}" -mode-off-config "${SV1D_CAPACITY_INTERNAL_MODE_OFF_CONFIG:?}" \
		-no-roster-config "${SV1D_CAPACITY_INTERNAL_NO_ROSTER_CONFIG:?}" -binary-sha256 "$SV1D_CAPACITY_INTERNAL_SIMULATOR_SHA256" \
		-analyzer-sha256 "$SV1D_CAPACITY_INTERNAL_ANALYZER_SHA256" -renderer-sha256 "$SV1D_CAPACITY_INTERNAL_RENDERER_SHA256" || exit 11
	identity_filter='del(.seed,.experiment_id,.hypothesis_id,.status,.description)'
	[[ "$(jq -S "$identity_filter" "$target_config")" == "$(jq -S "$identity_filter" "$config")" ]] || exit 12
	source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"
	run_capacity_arm "$@"
	exit $?
fi

if [[ $# -gt 5 ]]; then
	echo "usage: $0 [multivenue-binary] [sv1dprobe-binary] [evsrender-binary] [sv1dresource-binary] [sv1dlock-binary]" >&2
	exit 2
fi

root_dir=$(normalize_input_path "$root_dir") || fail "could not normalize the repository root"
capacity_script=$(normalize_input_path "$capacity_script") || fail "could not normalize the capacity runner"
require_no_symlink_components "$root_dir" || fail "repository root contains a symlink"

multivenue_binary=${1:-"$root_dir/bin/multivenue"}
sv1dprobe_binary=${2:-"$root_dir/bin/sv1dprobe"}
evsrender_binary=${3:-"$root_dir/bin/evsrender"}
sv1dresource_binary=${4:-"$root_dir/bin/sv1dresource"}
lock_binary=${5:-"$root_dir/bin/sv1dlock"}
capacity_attestation=${SV1D_CAPACITY_PREFLIGHT_ATTESTATION:-"/home/vlad/v2-r2-sv1d-capacity-977-v1.attestation.json"}
review_attestation=${SV1D_REVIEW_ATTESTATION:-}
review_report=${SV1D_REVIEW_REPORT:-}
trusted_review_key=${SV1D_TRUSTED_REVIEW_KEY:-}
multivenue_binary=$(normalize_input_path "$multivenue_binary") || fail "could not normalize multivenue binary"
sv1dprobe_binary=$(normalize_input_path "$sv1dprobe_binary") || fail "could not normalize sv1dprobe binary"
evsrender_binary=$(normalize_input_path "$evsrender_binary") || fail "could not normalize evsrender binary"
sv1dresource_binary=$(normalize_input_path "$sv1dresource_binary") || fail "could not normalize sv1dresource binary"
lock_binary=$(normalize_input_path "$lock_binary") || fail "could not normalize sv1dlock binary"
review_attestation=$(normalize_input_path "$review_attestation") || fail "could not normalize review attestation"
review_report=$(normalize_input_path "$review_report") || fail "could not normalize review report"
trusted_review_key=$(normalize_input_path "$trusted_review_key") || fail "could not normalize trusted review key"

[[ "${SV1D_CAPACITY_AUTHORIZED:-0}" == 1 ]] || fail "set SV1D_CAPACITY_AUTHORIZED=1 at the explicit capacity-preflight boundary"
[[ -d "$root_dir/.git" ]] || fail "repository root is not a Git worktree"
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "source worktree must be clean"
source_revision=$(git -C "$root_dir" rev-parse HEAD)
tree_revision=$(git -C "$root_dir" rev-parse HEAD^{tree})
[[ "$source_revision" =~ ^[0-9a-f]{40}$ && "$tree_revision" =~ ^[0-9a-f]{40}$ ]] || fail "invalid Git identity"

[[ -d "$config_dir" && ! -L "$config_dir" ]] || fail "missing or symlinked activation config directory"
"$root_dir/scripts/check-v2-r2-sv1d-activation-configs.sh" >/dev/null || fail "registered target config contract failed"
require_regular_file resource-policy "$policy_path"
require_regular_file parent-registration "$parent_registration_path"
require_regular_file amendment "$amendment_path"
require_regular_file review-attestation "$review_attestation"
require_regular_file review-report "$review_report"
require_regular_file trusted-review-key "$trusted_review_key"

require_binary multivenue "$multivenue_binary"
require_binary sv1dprobe "$sv1dprobe_binary"
require_binary evsrender "$evsrender_binary"
require_binary sv1dresource "$sv1dresource_binary"
require_binary sv1dlock "$lock_binary"
multivenue_binary=$(realpath -e -- "$multivenue_binary")
sv1dprobe_binary=$(realpath -e -- "$sv1dprobe_binary")
evsrender_binary=$(realpath -e -- "$evsrender_binary")
sv1dresource_binary=$(realpath -e -- "$sv1dresource_binary")
lock_binary=$(realpath -e -- "$lock_binary")
require_clean_pinned_binary multivenue "$multivenue_binary" "$source_revision"
require_clean_pinned_binary sv1dprobe "$sv1dprobe_binary" "$source_revision"
require_clean_pinned_binary evsrender "$evsrender_binary" "$source_revision"
require_clean_pinned_binary sv1dresource "$sv1dresource_binary" "$source_revision"
require_clean_pinned_binary sv1dlock "$lock_binary" "$source_revision"

if [[ "${SV1D_LOCK_HELD:-0}" != 1 ]]; then
	exec "$lock_binary" -path "$capacity_lock_path" -- env SV1D_LOCK_HELD=1 SV1D_LOCK_FD=3 "$0" "$@"
fi
[[ "$(readlink "/proc/$$/fd/3" 2>/dev/null)" == "$capacity_lock_path" ]] || fail "capacity lock was not opened by the trusted lock adapter"
flock -n 3 || fail "capacity lock descriptor is not exclusively held"

[[ "$output_root" == /* && "$output_root" != "/" && "$(realpath -m -- "$output_root")" == "$output_root" ]] || fail "capacity output root must be clean absolute path"
[[ "$capacity_attestation" == /* && "$capacity_attestation" != "/" && "$(realpath -m -- "$capacity_attestation")" == "$capacity_attestation" ]] || fail "capacity attestation path must be clean absolute path"
[[ "$capacity_attestation" != "$output_root" && "$capacity_attestation" != "$output_root/"* ]] || fail "capacity attestation must remain outside the measured output root"
require_no_symlink_components "$output_root" || fail "capacity output root contains a symlink"
require_no_symlink_components "$(dirname -- "$capacity_attestation")" || fail "capacity attestation parent contains a symlink"
[[ ! -e "$output_root" && ! -L "$output_root" ]] || fail "refusing to overwrite capacity output root: $output_root"
[[ ! -e "$capacity_attestation" && ! -L "$capacity_attestation" ]] || fail "refusing to overwrite capacity attestation: $capacity_attestation"

staging_root=$(mktemp -d)
[[ -d "$staging_root" && ! -L "$staging_root" ]] || fail "invalid staging directory"
trap 'rm -rf -- "$staging_root"' EXIT
mkdir -p "$staging_root/tools" "$staging_root/configs" "$staging_root/review"

staged_capacity_script="$staging_root/tools/capacity-runner.sh"
copy_immutable_file capacity-runner "$capacity_script" "$staged_capacity_script"
chmod 0555 -- "$staged_capacity_script"

staged_multivenue="$staging_root/tools/multivenue-$(hash_file "$multivenue_binary")"
staged_sv1dprobe="$staging_root/tools/sv1dprobe-$(hash_file "$sv1dprobe_binary")"
staged_evsrender="$staging_root/tools/evsrender-$(hash_file "$evsrender_binary")"
staged_sv1dresource="$staging_root/tools/sv1dresource-$(hash_file "$sv1dresource_binary")"
staged_sv1dlock="$staging_root/tools/sv1dlock-$(hash_file "$lock_binary")"
copy_immutable_binary multivenue "$multivenue_binary" "$staged_multivenue"
copy_immutable_binary sv1dprobe "$sv1dprobe_binary" "$staged_sv1dprobe"
copy_immutable_binary evsrender "$evsrender_binary" "$staged_evsrender"
copy_immutable_binary sv1dresource "$sv1dresource_binary" "$staged_sv1dresource"
copy_immutable_binary sv1dlock "$lock_binary" "$staged_sv1dlock"

staged_review_attestation="$staging_root/review/attestation.json"
staged_review_report="$staging_root/review/report.md"
staged_trusted_key="$staging_root/review/trusted-key.raw"
staged_parent_registration="$staging_root/review/parent-registration.md"
staged_amendment="$staging_root/review/amendment.md"
copy_immutable_file review-attestation "$review_attestation" "$staged_review_attestation"
copy_immutable_file review-report "$review_report" "$staged_review_report"
copy_immutable_file trusted-review-key "$trusted_review_key" "$staged_trusted_key"
copy_immutable_file parent-registration "$parent_registration_path" "$staged_parent_registration"
copy_immutable_file amendment "$amendment_path" "$staged_amendment"
for staged_file in "$staged_review_attestation" "$staged_review_report" "$staged_trusted_key" "$staged_parent_registration" "$staged_amendment"; do
	require_regular_file staged-review-input "$staged_file"
done

declare -A target_config_for=(
	[treatment]="$config_dir/activation-659-treatment.json"
	[mode-off]="$config_dir/activation-659-mode-off.json"
	[no-roster]="$config_dir/activation-659-no-roster.json"
)
declare -A staged_target_for capacity_config_for capacity_experiment_for capacity_record_for
for arm in treatment mode-off no-roster; do
	target_path=${target_config_for[$arm]}
	require_regular_file "target-$arm-config" "$target_path"
	staged_target="$staging_root/configs/target-$arm.json"
	copy_immutable_file "target-$arm-config" "$target_path" "$staged_target"
	staged_target_for[$arm]="$staged_target"
done

target_treatment_sha256=$(config_sha256 "${staged_target_for[treatment]}")
target_mode_off_sha256=$(config_sha256 "${staged_target_for[mode-off]}")
target_no_roster_sha256=$(config_sha256 "${staged_target_for[no-roster]}")
multivenue_sha256=$(hash_file "$staged_multivenue")
sv1dprobe_sha256=$(hash_file "$staged_sv1dprobe")
evsrender_sha256=$(hash_file "$staged_evsrender")
sv1dresource_sha256=$(hash_file "$staged_sv1dresource")
lock_binary_sha256=$(hash_file "$staged_sv1dlock")
parent_registration_sha256=$(hash_file "$staged_parent_registration")
amendment_sha256=$(hash_file "$staged_amendment")
policy_sha256=$(hash_file "$policy_path")
runner_sha256=$(hash_file "$staged_capacity_script")
review_attestation_sha256=$(hash_file "$staged_review_attestation")
review_report_sha256=$(hash_file "$staged_review_report")
trusted_review_key_sha256=$(hash_file "$staged_trusted_key")
expected_trusted_review_key_sha256=${SV1D_TRUSTED_REVIEW_KEY_SHA256:-}
[[ "$expected_trusted_review_key_sha256" =~ ^[0-9a-f]{64}$ ]] || fail "SV1D_TRUSTED_REVIEW_KEY_SHA256 must pin the trusted review key"
[[ "$trusted_review_key_sha256" == "$expected_trusted_review_key_sha256" ]] || fail "trusted review key does not match the pinned digest"

review_plan="$staging_root/probe-plan.json"
"$staged_sv1dprobe" -mode plan -out "$review_plan" \
	-treatment-config "${staged_target_for[treatment]}" -mode-off-config "${staged_target_for[mode-off]}" \
	-no-roster-config "${staged_target_for[no-roster]}" -source-revision "$source_revision" \
	-binary-sha256 "$multivenue_sha256" -analyzer-sha256 "$sv1dprobe_sha256" -renderer-sha256 "$evsrender_sha256" ||
	fail "could not derive capacity review-bound plan"
review_plan_sha256=$(jq -er '.plan_sha256 | select(test("^[0-9a-f]{64}$"))' "$review_plan") || fail "review-bound plan has no canonical digest"
"$staged_sv1dprobe" -mode verify-review \
	-review-attestation "$staged_review_attestation" -review-report "$staged_review_report" -trusted-review-key "$staged_trusted_key" \
	-source-revision "$source_revision" -tree-revision "$tree_revision" -plan-sha256 "$review_plan_sha256" \
	-parent-registration-sha256 "$parent_registration_sha256" -amendment-sha256 "$amendment_sha256" \
	-treatment-config "${staged_target_for[treatment]}" -mode-off-config "${staged_target_for[mode-off]}" -no-roster-config "${staged_target_for[no-roster]}" \
	-binary-sha256 "$multivenue_sha256" -analyzer-sha256 "$sv1dprobe_sha256" -renderer-sha256 "$evsrender_sha256" ||
	fail "external exact-tree review was not accepted"

export SV1D_CAPACITY_OUTPUT_ROOT="$output_root"
export SV1D_CAPACITY_LOCK_PATH="$capacity_lock_path"
export SV1D_CAPACITY_INTERNAL_REVIEW_ATTESTATION="$staged_review_attestation"
export SV1D_CAPACITY_INTERNAL_REVIEW_REPORT="$staged_review_report"
export SV1D_CAPACITY_INTERNAL_TRUSTED_KEY="$staged_trusted_key"
export SV1D_CAPACITY_INTERNAL_REVIEW_PLAN="$review_plan"
export SV1D_CAPACITY_INTERNAL_PARENT_REGISTRATION="$staged_parent_registration"
export SV1D_CAPACITY_INTERNAL_AMENDMENT="$staged_amendment"
export SV1D_CAPACITY_INTERNAL_TREE_REVISION="$tree_revision"
export SV1D_CAPACITY_INTERNAL_REVIEW_ATTESTATION_SHA256="$review_attestation_sha256"
export SV1D_CAPACITY_INTERNAL_REVIEW_REPORT_SHA256="$review_report_sha256"
export SV1D_CAPACITY_INTERNAL_TRUSTED_KEY_SHA256="$trusted_review_key_sha256"
export SV1D_CAPACITY_INTERNAL_REVIEW_PLAN_SHA256="$review_plan_sha256"
export SV1D_CAPACITY_INTERNAL_PARENT_REGISTRATION_SHA256="$parent_registration_sha256"
export SV1D_CAPACITY_INTERNAL_AMENDMENT_SHA256="$amendment_sha256"
export SV1D_CAPACITY_INTERNAL_TREATMENT_CONFIG="${staged_target_for[treatment]}"
export SV1D_CAPACITY_INTERNAL_MODE_OFF_CONFIG="${staged_target_for[mode-off]}"
export SV1D_CAPACITY_INTERNAL_NO_ROSTER_CONFIG="${staged_target_for[no-roster]}"
export SV1D_CAPACITY_INTERNAL_TREATMENT_CONFIG_SHA256="$target_treatment_sha256"
export SV1D_CAPACITY_INTERNAL_MODE_OFF_CONFIG_SHA256="$target_mode_off_sha256"
export SV1D_CAPACITY_INTERNAL_NO_ROSTER_CONFIG_SHA256="$target_no_roster_sha256"
export SV1D_CAPACITY_INTERNAL_SIMULATOR_SHA256="$multivenue_sha256"
export SV1D_CAPACITY_INTERNAL_ANALYZER_SHA256="$sv1dprobe_sha256"
export SV1D_CAPACITY_INTERNAL_RENDERER_SHA256="$evsrender_sha256"
export SV1D_CAPACITY_INTERNAL_RUNNER_SHA256="$runner_sha256"
export SV1D_CAPACITY_INTERNAL_RESOURCE_SHA256="$sv1dresource_sha256"

output_parent=$(dirname -- "$output_root")
mkdir -p "$output_root"
require_no_symlink_components "$output_root" || fail "created capacity output root contains a symlink"
mkdir -p "$output_root/configs" "$output_root/review" "$output_root/tools" "$output_root/arms" "$output_root/rendered" "$output_root/logs"
copy_immutable_file retained-multivenue "$staged_multivenue" "$output_root/tools/multivenue-$multivenue_sha256"
copy_immutable_file retained-sv1dprobe "$staged_sv1dprobe" "$output_root/tools/sv1dprobe-$sv1dprobe_sha256"
copy_immutable_file retained-evsrender "$staged_evsrender" "$output_root/tools/evsrender-$evsrender_sha256"
copy_immutable_file retained-sv1dresource "$staged_sv1dresource" "$output_root/tools/sv1dresource-$sv1dresource_sha256"
copy_immutable_file retained-sv1dlock "$staged_sv1dlock" "$output_root/tools/sv1dlock-$(hash_file "$lock_binary")"
retained_capacity_runner="$output_root/tools/capacity-runner-$runner_sha256.sh"
copy_immutable_file retained-capacity-runner "$staged_capacity_script" "$retained_capacity_runner"
chmod 0555 -- "$output_root"/tools/*
copy_immutable_file retained-resource-policy "$policy_path" "$output_root/resource-policy-v1.json"
copy_immutable_file retained-review-attestation "$staged_review_attestation" "$output_root/review/attestation.json"
copy_immutable_file retained-review-report "$staged_review_report" "$output_root/review/report.md"
copy_immutable_file retained-trusted-review-key "$staged_trusted_key" "$output_root/review/trusted-key.raw"
copy_immutable_file retained-parent-registration "$staged_parent_registration" "$output_root/review/parent-registration.md"
copy_immutable_file retained-amendment "$staged_amendment" "$output_root/review/amendment.md"
for arm in treatment mode-off no-roster; do
	copy_immutable_file "retained-target-$arm-config" "${staged_target_for[$arm]}" "$output_root/configs/target-$arm.json"
done
for immutable_file in "$output_root"/tools/* "$output_root"/review/* "$output_root/resource-policy-v1.json"; do
	[[ -e "$immutable_file" && ! -L "$immutable_file" ]] || fail "capacity retention copy missing: $immutable_file"
done

measurement_records_root="${output_root}.measurements"
[[ ! -e "$measurement_records_root" && ! -L "$measurement_records_root" ]] || fail "refusing to overwrite capacity measurement records"
mkdir -p "$measurement_records_root"
require_no_symlink_components "$measurement_records_root" || fail "capacity measurement records contain a symlink"

identity_json=$("$staged_sv1dresource" -inspect-filesystem "$output_parent") || fail "could not inspect output-parent filesystem"
output_parent_device=$(jq -er '.device' <<<"$identity_json")
output_parent_id=$(jq -er '.id' <<<"$identity_json")
output_parent_type=$(jq -er '.type' <<<"$identity_json")
output_parent_mount_id=$(jq -er '.mount_id' <<<"$identity_json")
output_parent_uuid=$(jq -er '.uuid' <<<"$identity_json")
measurement_identity_json=$("$staged_sv1dresource" -inspect-filesystem "$output_root") || fail "could not inspect measurement-root filesystem"
[[ "$(jq -S -c . <<<"$identity_json")" == "$(jq -S -c . <<<"$measurement_identity_json")" ]] || fail "capacity output and measurement roots are on different filesystems"
measurement_records_identity_json=$("$staged_sv1dresource" -inspect-filesystem "$measurement_records_root") || fail "could not inspect measurement-records filesystem"
[[ "$(jq -S -c . <<<"$identity_json")" == "$(jq -S -c . <<<"$measurement_records_identity_json")" ]] || fail "capacity measurements are on a different filesystem"

identity_filter='del(.seed,.experiment_id,.hypothesis_id,.status,.description)'
delta_records='[]'
for arm in treatment mode-off no-roster; do
	target_path=${staged_target_for[$arm]}
	capacity_experiment="v2-r2-sv1d-capacity-977-$arm"
	capacity_path="$output_root/configs/capacity-$arm.json"
	jq --argjson seed "$capacity_seed" --arg experiment_id "$capacity_experiment" --arg hypothesis_id "$capacity_hypothesis" \
		'.seed = $seed | .experiment_id = $experiment_id | .hypothesis_id = $hypothesis_id |
		 .status = "capacity-preflight-only" | .description = "Outcome-ineligible binary-evidence capacity preflight arm"' \
		"$target_path" >"$capacity_path.tmp-$$"
	mv -- "$capacity_path.tmp-$$" "$capacity_path"
	[[ "$(jq -S "$identity_filter" "$target_path")" == "$(jq -S "$identity_filter" "$capacity_path")" ]] || fail "capacity config changes scientific fields: $arm"
	capacity_hash=$(config_sha256 "$capacity_path")
	delta_records=$(jq -c --arg arm "$arm" --arg target_sha256 "$(config_sha256 "$target_path")" --arg capacity_sha256 "$capacity_hash" \
		--arg experiment_id "$capacity_experiment" \
		'. + [{arm: $arm, target_config_sha256: $target_sha256, capacity_config_sha256: $capacity_sha256,
			changed_fields: ["seed", "experiment_id", "hypothesis_id", "status", "description"],
			capacity_experiment_id: $experiment_id, capacity_hypothesis_id: "V2-R2-SV1D-CAPACITY-ONLY", capacity_seed: 977}]' <<<"$delta_records")
	capacity_config_for[$arm]="$capacity_path"
	capacity_experiment_for[$arm]="$capacity_experiment"
done
capacity_delta_path="$output_root/configs/capacity-config-delta.json"
jq -S -n --arg contract "v2-r2-sv1d-capacity-config-delta-v1" --argjson arms "$delta_records" \
	'{schema_version: 1, contract: $contract, scientific_fields_unchanged: true,
	 allowed_changed_fields: ["seed", "experiment_id", "hypothesis_id", "status", "description"], arms: $arms}' \
	>"$capacity_delta_path.tmp-$$"
mv -- "$capacity_delta_path.tmp-$$" "$capacity_delta_path"
capacity_delta_sha256=$(hash_file "$capacity_delta_path")
capacity_treatment_sha256=$(config_sha256 "${capacity_config_for[treatment]}")
capacity_mode_off_sha256=$(config_sha256 "${capacity_config_for[mode-off]}")
capacity_no_roster_sha256=$(config_sha256 "${capacity_config_for[no-roster]}")

jq -S -n \
	--arg contract "v2-r2-sv1d-capacity-run-metadata-v1" --arg source_revision "$source_revision" --arg tree_revision "$tree_revision" \
	--arg probe_id "$capacity_probe_id" --arg plan_sha256 "$review_plan_sha256" --arg review_attestation_sha256 "$review_attestation_sha256" \
	--arg review_report_sha256 "$review_report_sha256" --arg trusted_review_key_sha256 "$trusted_review_key_sha256" \
	--arg runner_sha256 "$runner_sha256" --arg measurer_sha256 "$sv1dresource_sha256" \
	--arg lock_binary_sha256 "$lock_binary_sha256" \
	--arg resource_policy_sha256 "$policy_sha256" --arg evidence_format "evstream_v3" --arg log_mode "full" \
	--arg output_root "$output_root" --arg output_parent "$output_parent" \
	'{schema_version: 1, contract: $contract, scientific_result_eligible: false, capacity_seed: 977, horizon: "5m",
	 source_revision: $source_revision, tree_revision: $tree_revision, probe_id: $probe_id, plan_sha256: $plan_sha256,
	 review_attestation_sha256: $review_attestation_sha256, review_report_sha256: $review_report_sha256, trusted_review_key_sha256: $trusted_review_key_sha256,
	 runner_sha256: $runner_sha256, measurer_sha256: $measurer_sha256, resource_policy_sha256: $resource_policy_sha256,
	 lock_binary_sha256: $lock_binary_sha256,
	 evidence_format: $evidence_format, log_mode: $log_mode, output_root: $output_root, output_parent: $output_parent,
	 arms: ["treatment", "mode-off", "no-roster"], holdouts_consumed: [], outcome_metrics_recorded: false}' \
	>"$output_root/capacity-run-metadata.json.tmp-$$"
mv -- "$output_root/capacity-run-metadata.json.tmp-$$" "$output_root/capacity-run-metadata.json"

declare -A measurement_for arm_record_for
arm_failure=0
for arm in treatment mode-off no-roster; do
	arm_dir="$output_root/arms/$arm"
	rendered_dir="$output_root/rendered/$arm"
	stdout_log="$output_root/logs/$arm.simulator.stdout.log"
	stderr_log="$output_root/logs/$arm.simulator.stderr.log"
	measurement_path="$measurement_records_root/$arm-resource-measurement.json"
	arm_result="$measurement_records_root/$arm-record.json"
	set +e
	GOMAXPROCS=2 GOMEMLIMIT=4GiB SV1D_CAPACITY_ROOT_DIR="$root_dir" SV1D_CAPACITY_SCRIPT_PATH="$retained_capacity_runner" "$staged_sv1dresource" -out "$measurement_path" -output-parent "$output_parent" -measurement-root "$output_root" \
		-sample-interval 250ms -require-finite-cgroup -inherit-fd 3 -- \
		"$retained_capacity_runner" --internal-arm "$arm" "${capacity_config_for[$arm]}" "$arm_dir" "$rendered_dir" "$staged_multivenue" "$staged_sv1dprobe" "$staged_evsrender" "$source_revision" \
		"${capacity_experiment_for[$arm]}" "$stdout_log" "$stderr_log"
	resource_status=$?
	set -e
	[[ -s "$measurement_path" ]] || fail "resource measurer produced no retained record for $arm"
	measurement_for[$arm]="$measurement_path"
	if [[ "$resource_status" -ne 0 ]] || ! jq -e '.complete == true and .exit_status == 0 and .cgroup_memory_limit_bytes > 0' "$measurement_path" >/dev/null; then
		echo "capacity arm did not complete a finite measured run: $arm" >&2
		arm_failure=1
		continue
	fi
	for required_file in run-config.json run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl evidence-manifest.json binary-evidence-attestation.json run-status.json renderer-report.json; do
		[[ -s "$arm_dir/$required_file" && ! -L "$arm_dir/$required_file" ]] || { echo "capacity arm missing $arm/$required_file" >&2; arm_failure=1; continue 2; }
	done
	[[ -d "$rendered_dir" && ! -L "$rendered_dir" && -s "$rendered_dir/renderer-attestation.json" && -s "$rendered_dir/rendered-binary-evidence-attestation.json" ]] || { echo "capacity renderer incomplete: $arm" >&2; arm_failure=1; continue; }
	event_frames=$(jq -er '.event_frames' "$arm_dir/binary-evidence-attestation.json")
	stream_frames=$(jq -er '.stream_frames' "$arm_dir/binary-evidence-attestation.json")
	execution_hash=$(jq -er '.execution_stream_hash | select(test("^[0-9a-f]{64}$"))' "$arm_dir/binary-evidence-attestation.json")
	evidence_epoch=$(jq -er '.schema_epoch' "$arm_dir/binary-evidence-attestation.json")
	[[ "$evidence_epoch" == 4 && "$event_frames" -gt 0 && "$stream_frames" -ge "$event_frames" ]] || { echo "capacity binary evidence is incomplete: $arm" >&2; arm_failure=1; continue; }
	rendered_digest=$(jq -er '.rendered_digest | select(test("^[0-9a-f]{64}$"))' "$arm_dir/renderer-report.json")
	jq -n \
		--arg name "$arm" --arg capacity_experiment_id "${capacity_experiment_for[$arm]}" --arg capacity_hypothesis_id "$capacity_hypothesis" \
		--arg capacity_config_sha256 "$(config_sha256 "${capacity_config_for[$arm]}")" --arg run_metadata_sha256 "$(hash_file "$arm_dir/run-metadata.json")" \
		--arg manifest_sha256 "$(hash_file "$arm_dir/manifest.json")" --arg run_status_sha256 "$(hash_file "$arm_dir/run-status.json")" \
		--arg evidence_manifest_sha256 "$(hash_file "$arm_dir/evidence-manifest.json")" --arg binary_evidence_attestation_sha256 "$(hash_file "$arm_dir/binary-evidence-attestation.json")" \
		--arg events_sha256 "$(hash_file "$arm_dir/events.evs")" --arg execution_stream_hash "$execution_hash" \
		--arg renderer_report_sha256 "$(hash_file "$arm_dir/renderer-report.json")" --arg renderer_attestation_sha256 "$(hash_file "$rendered_dir/renderer-attestation.json")" \
		--arg resource_measurement_sha256 "$(hash_file "$measurement_path")" \
		--arg rendered_tree_digest "$rendered_digest" --argjson event_frames "$event_frames" --argjson stream_frames "$stream_frames" \
		--argjson peak_apparent_bytes "$(jq -er '.peak_apparent_bytes' "$measurement_path")" --argjson peak_allocated_bytes "$(jq -er '.peak_allocated_bytes' "$measurement_path")" \
		--argjson peak_process_tree_rss_bytes "$(jq -er '.peak_process_tree_rss_bytes' "$measurement_path")" \
		'{name: $name, capacity_experiment_id: $capacity_experiment_id, capacity_hypothesis_id: $capacity_hypothesis_id,
		 capacity_config_sha256: $capacity_config_sha256, complete: true, exit_status: 0,
		 simulation_start_nano: 1735689600000000000, simulation_end_nano: 1735689900000000000,
		 run_metadata_sha256: $run_metadata_sha256, manifest_sha256: $manifest_sha256, run_status_sha256: $run_status_sha256,
		 evidence_manifest_sha256: $evidence_manifest_sha256, binary_evidence_attestation_sha256: $binary_evidence_attestation_sha256,
		 events_sha256: $events_sha256, execution_stream_hash: $execution_stream_hash, renderer_report_sha256: $renderer_report_sha256,
			 renderer_attestation_sha256: $renderer_attestation_sha256, rendered_tree_digest: $rendered_tree_digest,
			 event_frames: $event_frames, stream_frames: $stream_frames, peak_apparent_bytes: $peak_apparent_bytes,
			 peak_allocated_bytes: $peak_allocated_bytes, peak_process_tree_rss_bytes: $peak_process_tree_rss_bytes,
			 resource_measurement_sha256: $resource_measurement_sha256}' \
		>"$arm_result"
	arm_record_for[$arm]="$arm_result"
done
[[ "$arm_failure" -eq 0 ]] || fail "one or more capacity arms failed; no capacity attestation was issued"

sample_records=("${measurement_for[treatment]}" "${measurement_for[mode-off]}" "${measurement_for[no-roster]}")
sample_aggregate="$measurement_records_root/all-samples.json"
jq -S -c -s '[.[] as $record | $record.samples[] | {arm: $record.arm, sample: .}]' \
	<(jq --arg arm treatment '{arm: $arm, samples: .samples}' "${measurement_for[treatment]}") \
	<(jq --arg arm mode-off '{arm: $arm, samples: .samples}' "${measurement_for[mode-off]}") \
	<(jq --arg arm no-roster '{arm: $arm, samples: .samples}' "${measurement_for[no-roster]}") \
	>"$sample_aggregate"
samples_sha256=$(hash_file "$sample_aggregate")
measurement_manifest_records='[]'
for measurement_record in treatment-resource-measurement.json mode-off-resource-measurement.json no-roster-resource-measurement.json treatment-record.json mode-off-record.json no-roster-record.json all-samples.json; do
	measurement_manifest_records=$(jq -c --arg path "$measurement_record" --arg sha256 "$(hash_file "$measurement_records_root/$measurement_record")" --argjson bytes "$(stat -c '%s' -- "$measurement_records_root/$measurement_record")" \
		'. + [{path: $path, bytes: $bytes, sha256: $sha256}]' <<<"$measurement_manifest_records")
done
measurement_records_manifest="$measurement_records_root/measurement-records-manifest.json"
jq -S -n --arg contract "v2-r2-sv1d-capacity-measurement-records-v1" --arg root "$output_root" --arg sample_path "all-samples.json" --arg sample_sha256 "$samples_sha256" --argjson sample_bytes "$(stat -c '%s' -- "$sample_aggregate")" --argjson files "$measurement_manifest_records" \
	'{schema_version: 1, contract: $contract, measurement_root: $root, sample_aggregate: {path: $sample_path, bytes: $sample_bytes, sha256: $sample_sha256}, files: $files}' \
	>"$measurement_records_manifest.tmp-$$"
mv -- "$measurement_records_manifest.tmp-$$" "$measurement_records_manifest"
measurement_records_sha256=$(hash_file "$measurement_records_manifest")
initial_available_bytes=$(jq -er '.initial_available_bytes' "${measurement_for[treatment]}")
minimum_available_bytes=$(jq -s -er 'map(.minimum_available_bytes) | min' "${sample_records[@]}")
final_available_bytes=$(jq -er '.final_available_bytes' "${measurement_for[no-roster]}")
peak_apparent_bytes=$(jq -s -er 'map(.peak_apparent_bytes) | max' "${sample_records[@]}")
peak_allocated_bytes=$(jq -s -er 'map(.peak_allocated_bytes) | max' "${sample_records[@]}")
peak_process_tree_rss_bytes=$(jq -s -er 'map(.peak_process_tree_rss_bytes) | max' "${sample_records[@]}")
peak_cgroup_memory_bytes=$(jq -s -er 'map(.peak_cgroup_memory_bytes) | max' "${sample_records[@]}")
cgroup_memory_limit_bytes=$(jq -s -er 'map(.cgroup_memory_limit_bytes) | min' "${sample_records[@]}")
minimum_host_mem_available_bytes=$(jq -s -er 'map(.minimum_host_mem_available_bytes) | min' "${sample_records[@]}")
swap_used_bytes=$(jq -s -er 'map(.maximum_swap_used_bytes) | max' "${sample_records[@]}")
maximum_sample_gap_nano=$(jq -s -er 'map(.maximum_sample_gap_nano) | max' "${sample_records[@]}")
sample_count=$(jq -s -er 'map(.sample_count) | add' "${sample_records[@]}")
oom_events_delta=$(jq -s -er 'map(.cgroup_oom_events_delta) | add' "${sample_records[@]}")
oom_kill_events_delta=$(jq -s -er 'map(.cgroup_oom_kill_events_delta) | add' "${sample_records[@]}")
cgroup_oom_events_delta=$(jq -s -er 'map(.cgroup_local_oom_events_delta) | add' "${sample_records[@]}")
cgroup_oom_kill_events_delta=$(jq -s -er 'map(.cgroup_local_oom_kill_events_delta) | add' "${sample_records[@]}")
[[ "$minimum_available_bytes" -le "$initial_available_bytes" ]] || fail "filesystem free space increased during capacity measurement; no conservative floor can be derived"
peak_filesystem_consumption_bytes=$((initial_available_bytes - minimum_available_bytes))
measured_peak_bytes=$peak_apparent_bytes
(( peak_allocated_bytes > measured_peak_bytes )) && measured_peak_bytes=$peak_allocated_bytes
(( peak_filesystem_consumption_bytes > measured_peak_bytes )) && measured_peak_bytes=$peak_filesystem_consumption_bytes
safety_reserve_bytes=$((2 * 1024 * 1024 * 1024))
required_free_bytes=$((measured_peak_bytes * 2))
(( measured_peak_bytes + safety_reserve_bytes > required_free_bytes )) && required_free_bytes=$((measured_peak_bytes + safety_reserve_bytes))
required_available_memory_bytes=$((peak_process_tree_rss_bytes + 1024 * 1024 * 1024))
ceil_one_point_five=$(((3 * peak_process_tree_rss_bytes + 1) / 2))
(( ceil_one_point_five > required_available_memory_bytes )) && required_available_memory_bytes=$ceil_one_point_five

arms_json=$(jq -s \
	--arg treatment_record_sha256 "$(hash_file "${arm_record_for[treatment]}")" \
	--arg mode_off_record_sha256 "$(hash_file "${arm_record_for[mode-off]}")" \
	--arg no_roster_record_sha256 "$(hash_file "${arm_record_for[no-roster]}")" \
	'map(if .name == "treatment" then . + {capacity_arm_record_sha256: $treatment_record_sha256}
	      elif .name == "mode-off" then . + {capacity_arm_record_sha256: $mode_off_record_sha256}
	      elif .name == "no-roster" then . + {capacity_arm_record_sha256: $no_roster_record_sha256}
	      else . end)' \
	"${arm_record_for[treatment]}" "${arm_record_for[mode-off]}" "${arm_record_for[no-roster]}")
jq -S -n \
	--arg contract "v2-r2-sv1d-binary-capacity-preflight-v1" --arg purpose "five_minute_sv1d_binary_evidence_capacity_preflight" \
	--arg probe_id "$capacity_probe_id" --arg source_revision "$source_revision" --arg tree_revision "$tree_revision" \
	--arg review_attestation_sha256 "$review_attestation_sha256" --arg review_report_sha256 "$review_report_sha256" --arg trusted_review_key_sha256 "$trusted_review_key_sha256" --arg plan_sha256 "$review_plan_sha256" \
	--arg target_treatment_config_sha256 "$target_treatment_sha256" --arg target_mode_off_config_sha256 "$target_mode_off_sha256" --arg target_no_roster_config_sha256 "$target_no_roster_sha256" \
	--arg capacity_treatment_config_sha256 "$capacity_treatment_sha256" --arg capacity_mode_off_config_sha256 "$capacity_mode_off_sha256" --arg capacity_no_roster_config_sha256 "$capacity_no_roster_sha256" \
	--arg capacity_config_delta_sha256 "$capacity_delta_sha256" --arg binary_sha256 "$multivenue_sha256" --arg analyzer_sha256 "$sv1dprobe_sha256" \
	--arg renderer_sha256 "$evsrender_sha256" --arg runner_sha256 "$runner_sha256" --arg measurer_sha256 "$sv1dresource_sha256" --arg resource_policy_sha256 "$policy_sha256" \
	--arg output_parent "$output_parent" --arg measurement_root "$output_root" --arg filesystem_device "$output_parent_device" --arg filesystem_id "$output_parent_id" \
	--arg measurement_records_root "$measurement_records_root" --arg measurement_records_sha256 "$measurement_records_sha256" \
	--arg filesystem_type "$output_parent_type" --arg filesystem_mount_id "$output_parent_mount_id" --arg filesystem_uuid "$output_parent_uuid" \
	--arg samples_sha256 "$samples_sha256" --argjson arms "$arms_json" --argjson initial_available_bytes "$initial_available_bytes" \
	--argjson minimum_available_bytes "$minimum_available_bytes" --argjson final_available_bytes "$final_available_bytes" \
	--argjson peak_apparent_bytes "$peak_apparent_bytes" --argjson peak_allocated_bytes "$peak_allocated_bytes" \
	--argjson peak_filesystem_consumption_bytes "$peak_filesystem_consumption_bytes" --argjson measured_peak_bytes "$measured_peak_bytes" \
	--argjson safety_reserve_bytes "$safety_reserve_bytes" --argjson required_free_bytes "$required_free_bytes" \
	--argjson peak_process_tree_rss_bytes "$peak_process_tree_rss_bytes" --argjson required_available_memory_bytes "$required_available_memory_bytes" \
	--argjson peak_cgroup_memory_bytes "$peak_cgroup_memory_bytes" --argjson cgroup_memory_limit_bytes "$cgroup_memory_limit_bytes" \
	--argjson minimum_host_mem_available_bytes "$minimum_host_mem_available_bytes" --argjson swap_used_bytes "$swap_used_bytes" \
	--argjson maximum_sample_gap_nano "$maximum_sample_gap_nano" --argjson sample_count "$sample_count" \
	--argjson oom_events_delta "$oom_events_delta" --argjson oom_kill_events_delta "$oom_kill_events_delta" \
	--argjson cgroup_oom_events_delta "$cgroup_oom_events_delta" --argjson cgroup_oom_kill_events_delta "$cgroup_oom_kill_events_delta" \
	'{schema_version: 1, contract: $contract, scientific_result_eligible: false, purpose: $purpose, probe_id: $probe_id,
	 capacity_seed: 977, horizon: "5m", duration_nano: 300000000000, simulation_start_nano: 1735689600000000000,
	 simulation_end_nano: 1735689900000000000, source_revision: $source_revision, tree_revision: $tree_revision,
	 review_attestation_sha256: $review_attestation_sha256, review_report_sha256: $review_report_sha256, trusted_review_key_sha256: $trusted_review_key_sha256, plan_sha256: $plan_sha256,
	 target_treatment_config_sha256: $target_treatment_config_sha256, target_mode_off_config_sha256: $target_mode_off_config_sha256,
	 target_no_roster_config_sha256: $target_no_roster_config_sha256, capacity_treatment_config_sha256: $capacity_treatment_config_sha256,
	 capacity_mode_off_config_sha256: $capacity_mode_off_config_sha256, capacity_no_roster_config_sha256: $capacity_no_roster_config_sha256,
	 capacity_config_delta_sha256: $capacity_config_delta_sha256, binary_sha256: $binary_sha256, analyzer_sha256: $analyzer_sha256,
	 renderer_sha256: $renderer_sha256, runner_sha256: $runner_sha256, measurer_sha256: $measurer_sha256, resource_policy_sha256: $resource_policy_sha256,
	 evidence_format: "evstream_v3", evidence_schema_epoch: 4, log_mode: "full", gomaxprocs: 2, gomemlimit: "4GiB",
	 output_parent: $output_parent, measurement_root: $measurement_root, measurement_records_root: $measurement_records_root,
	 measurement_records_sha256: $measurement_records_sha256, filesystem_device: $filesystem_device, filesystem_id: $filesystem_id,
	 filesystem_type: $filesystem_type, filesystem_mount_id: $filesystem_mount_id, filesystem_uuid: $filesystem_uuid, same_filesystem: true,
	 initial_available_bytes: $initial_available_bytes, minimum_available_bytes: $minimum_available_bytes, final_available_bytes: $final_available_bytes,
	 peak_apparent_bytes: $peak_apparent_bytes, peak_allocated_bytes: $peak_allocated_bytes,
	 peak_filesystem_consumption_bytes: $peak_filesystem_consumption_bytes, measured_peak_bytes: $measured_peak_bytes,
	 safety_reserve_bytes: $safety_reserve_bytes, required_free_bytes: $required_free_bytes,
	 peak_process_tree_rss_bytes: $peak_process_tree_rss_bytes, required_available_memory_bytes: $required_available_memory_bytes,
	 peak_cgroup_memory_bytes: $peak_cgroup_memory_bytes, cgroup_memory_limit_bytes: $cgroup_memory_limit_bytes,
	 minimum_host_mem_available_bytes: $minimum_host_mem_available_bytes, swap_used_bytes: $swap_used_bytes,
	 sample_interval_nano: 250000000, maximum_sample_gap_nano: $maximum_sample_gap_nano, sample_count: $sample_count,
	 samples_sha256: $samples_sha256, oom_events_delta: $oom_events_delta, oom_kill_events_delta: $oom_kill_events_delta,
	 cgroup_oom_events_delta: $cgroup_oom_events_delta, cgroup_oom_kill_events_delta: $cgroup_oom_kill_events_delta, arms: $arms}' \
	>"$capacity_attestation.tmp-$$"
mv -- "$capacity_attestation.tmp-$$" "$capacity_attestation"

"$staged_sv1dprobe" -mode verify-capacity -capacity-attestation "$capacity_attestation" \
	-source-revision "$source_revision" -tree-revision "$tree_revision" -plan-sha256 "$review_plan_sha256" \
	-review-attestation "$review_attestation_sha256" -review-report "$review_report_sha256" -trusted-review-key-sha256 "$trusted_review_key_sha256" \
	-treatment-config "${staged_target_for[treatment]}" -mode-off-config "${staged_target_for[mode-off]}" -no-roster-config "${staged_target_for[no-roster]}" \
	-capacity-treatment-config "${capacity_config_for[treatment]}" -capacity-mode-off-config "${capacity_config_for[mode-off]}" -capacity-no-roster-config "${capacity_config_for[no-roster]}" \
	-capacity-config-delta-sha256 "$capacity_delta_sha256" -binary-sha256 "$multivenue_sha256" -analyzer-sha256 "$sv1dprobe_sha256" \
	-renderer-sha256 "$evsrender_sha256" -runner-sha256 "$runner_sha256" -measurer-sha256 "$sv1dresource_sha256" -resource-policy-sha256 "$policy_sha256" \
	-output-parent "$output_parent" -measurement-root "$output_root" -filesystem-device "$output_parent_device" -filesystem-id "$output_parent_id" \
	-filesystem-type "$output_parent_type" -filesystem-mount-id "$output_parent_mount_id" -filesystem-uuid "$output_parent_uuid" \
	-measurement-records-root "$measurement_records_root" -measurement-records-sha256 "$measurement_records_sha256" ||
	fail "generated capacity attestation did not pass the independent typed verifier"

available_kb=$(df -Pk -- "$output_parent" | awk 'NR == 2 {print $4}')
[[ "$available_kb" =~ ^[0-9]+$ ]] || fail "could not measure current output-parent free space"
[[ $((available_kb * 1024)) -ge "$required_free_bytes" ]] || fail "current output-parent free space is below measured required floor"
host_available_bytes=$(awk '$1 == "MemAvailable:" {print $2 * 1024; exit}' /proc/meminfo)
[[ "$host_available_bytes" =~ ^[0-9]+$ && "$host_available_bytes" -ge "$required_available_memory_bytes" ]] || fail "current host available memory is below measured required floor"

echo "completed outcome-ineligible SV1D binary-capacity preflight: $capacity_attestation"
