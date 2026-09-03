#!/usr/bin/env bash
# Run the separately registered, development-only V2-R2-SV1 activation probe.
# This is intentionally not a registered integrated-long-run cell and cannot
# select a holdout seed. It creates a fresh paired treatment/control namespace
# and leaves every artifact in place for independent review.
set -euo pipefail

if [[ $# -gt 2 ]]; then
	echo "usage: $0 [multivenue-binary] [cdf-liquidity-audit-binary]" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
scientific_root=$(realpath -e -- "$root_dir") || {
	echo "could not resolve scientific repository root" >&2
	exit 1
}
source "$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
contract_script=$(v2_r2_select_sv1_contract "$root_dir") || {
	echo "activation probe received an unregistered SV1 contract path" >&2
	exit 1
}
source "$contract_script"
export V2_R2_SV1_CONTRACT_SCRIPT="$contract_script"

go_bin_dir=/usr/local/go/bin
[[ -x "$go_bin_dir/go" ]] || go_bin_dir=$(dirname -- "$(command -v go)")
PATH="$go_bin_dir:$PATH"
export PATH

[[ -z "${EXSIM_BINARY_EVIDENCE:-}" ]] || {
	echo "activation probe refuses prototype EXSIM_BINARY_EVIDENCE overrides" >&2
	exit 1
}
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || {
	echo "activation probe requires a clean scientific worktree" >&2
	exit 1
}

head_revision=$(git -C "$root_dir" rev-parse HEAD)
[[ "$head_revision" =~ ^[0-9a-f]{40}$ ]] || {
	echo "invalid scientific HEAD: $head_revision" >&2
	exit 1
}

activation_seed="$v2_r2_sv1_activation_seed"
treatment_config="$v2_r2_sv1_activation_config"
control_config="$v2_r2_sv1_activation_control_config"
binary=${1:-"$root_dir/bin/multivenue"}
audit_binary=${2:-"$root_dir/bin/cdf-liquidity-audit"}
[[ -x "$binary" && -x "$audit_binary" && -s "$treatment_config" && -s "$control_config" ]] || {
	echo "missing activation configs or executable: $treatment_config $control_config $binary $audit_binary" >&2
	exit 1
}

[[ "$(jq -er '.seed' "$treatment_config")" == "$activation_seed" && "$(jq -er '.seed' "$control_config")" == "$activation_seed" ]] || {
	echo "activation probe seed differs from the registered development activation seed: $activation_seed" >&2
	exit 1
}
[[ "$(jq -er '.evidence_format' "$treatment_config")" == evstream_v3 && "$(jq -er '.evidence_format' "$control_config")" == evstream_v3 ]] || {
	echo "activation probe requires evstream_v3 in both arms" >&2
	exit 1
}
[[ "$(jq -er '.record_market_data_receipts' "$treatment_config")" == true && "$(jq -er '.record_market_data_receipts' "$control_config")" == true ]] || {
	echo "activation probe requires receipt evidence in both arms" >&2
	exit 1
}
jq -e '(.elastic_liquidity_suppliers | type == "array" and length > 0) and
	(.market_data_receipt_roles | index("cdf_elastic_supplier") != null)' "$treatment_config" >/dev/null || {
	echo "treatment does not declare the CDF roster and receipt role" >&2
	exit 1
}
jq -e '(.elastic_liquidity_suppliers == null or .elastic_liquidity_suppliers == []) and
	(.market_data_receipt_roles | index("cdf_elastic_supplier") == null)' "$control_config" >/dev/null || {
	echo "control is not a no-CDF paired population" >&2
	exit 1
}

binary_revision=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
binary_modified=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
binary_trimpath=$(go version -m "$binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
binary_cgo_enabled=$(go version -m "$binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
binary_goos=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOOS=") == 1 {sub("GOOS=", "", $2); print $2; exit}')
binary_goarch=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOARCH=") == 1 {sub("GOARCH=", "", $2); print $2; exit}')
binary_goamd64=$(go version -m "$binary" | awk '$1 == "build" && index($2, "GOAMD64=") == 1 {sub("GOAMD64=", "", $2); print $2; exit}')
binary_go_version=$(v2_r2_binary_go_version "$binary")
audit_revision=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
audit_modified=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
audit_trimpath=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
audit_cgo_enabled=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
audit_goos=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "GOOS=") == 1 {sub("GOOS=", "", $2); print $2; exit}')
audit_goarch=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "GOARCH=") == 1 {sub("GOARCH=", "", $2); print $2; exit}')
audit_goamd64=$(go version -m "$audit_binary" | awk '$1 == "build" && index($2, "GOAMD64=") == 1 {sub("GOAMD64=", "", $2); print $2; exit}')
audit_go_version=$(v2_r2_binary_go_version "$audit_binary")
[[ "$binary_revision" == "$head_revision" && "$binary_modified" == false && "$binary_trimpath" == true && "$binary_cgo_enabled" == 0 ]] || {
	echo "multivenue binary is not a clean reproducible build of HEAD" >&2
	exit 1
}
[[ "$audit_revision" == "$head_revision" && "$audit_modified" == false && "$audit_trimpath" == true && "$audit_cgo_enabled" == 0 ]] || {
	echo "CDF analyzer is not a clean reproducible build of HEAD" >&2
	exit 1
}
[[ "$binary_goos" == linux && "$binary_goarch" == amd64 && "$binary_goamd64" == v1 && "$audit_goos" == linux && "$audit_goarch" == amd64 && "$audit_goamd64" == v1 ]] || {
	echo "activation binaries must attest linux/amd64/v1 (simulator=$binary_goos/$binary_goarch/$binary_goamd64 analyzer=$audit_goos/$audit_goarch/$audit_goamd64)" >&2
	exit 1
}
v2_r2_is_go_127 "$binary_go_version" || { echo "multivenue binary is not Go 1.27: $binary_go_version" >&2; exit 1; }
v2_r2_is_go_127 "$audit_go_version" || { echo "CDF analyzer is not Go 1.27: $audit_go_version" >&2; exit 1; }

review_attestation=$(v2_r2_sv1b_review_attestation_path "$head_revision") || {
	echo "could not resolve the exact-tree SV1B review attestation path" >&2
	exit 1
}
v2_r2_require_sv1b_review_attestation "$review_attestation" "$head_revision" || {
	echo "activation probe requires an accepted independent review of the exact candidate tree" >&2
	exit 1
}
review_attestation_sha256=$(sha256sum -- "$review_attestation" | awk '{print $1}')
activation_gomaxprocs=${v2_r2_sv1_activation_gomaxprocs:-2}
activation_memory_limit_bytes=${v2_r2_sv1_activation_memory_limit_bytes:-$((20 * 1024 * 1024 * 1024))}
activation_gomemlimit_bytes=${v2_r2_sv1_activation_gomemlimit_bytes:-$((18 * 1024 * 1024 * 1024))}
activation_minimum_free_bytes=${v2_r2_sv1_activation_minimum_free_bytes:-$((4 * 1024 * 1024 * 1024))}
[[ "$activation_gomaxprocs" =~ ^[1-9][0-9]*$ && "$activation_memory_limit_bytes" =~ ^[1-9][0-9]*$ &&
	"$activation_gomemlimit_bytes" =~ ^[1-9][0-9]*$ && "$activation_minimum_free_bytes" =~ ^[1-9][0-9]*$ ]] || {
	echo "activation resource policy is not positive and integral" >&2
	exit 1
}
(( activation_gomemlimit_bytes < activation_memory_limit_bytes )) || {
	echo "activation GOMEMLIMIT must remain below the hard address-space ceiling" >&2
	exit 1
}
command -v prlimit >/dev/null 2>&1 || {
	echo "activation probe requires prlimit for the hard address-space ceiling" >&2
	exit 1
}

v2_r2_acquire_namespace_lock || {
	echo "could not acquire the R2 evidence namespace lock" >&2
	exit 1
}

horizon=5m
simulation_start_nano=1735689600000000000
simulation_end_nano=1735689900000000000
output_root=${V2_R2_SV1_ACTIVATION_ROOT:-"/home/vlad/external-scratch/${v2_r2_sv1_activation_output_prefix}-${activation_seed}-${head_revision}"}
[[ "$output_root" == /* && "$output_root" != */ && "$output_root" != *$'\n'* && "$output_root" != *$'\t'* ]] || {
	echo "activation output root must be an absolute, non-empty path" >&2
	exit 1
}
resolved_output_root=$(realpath -m -- "$output_root") || {
	echo "could not resolve activation output root: $output_root" >&2
	exit 1
}
case "$resolved_output_root" in
	"$scientific_root"|"$scientific_root"/*)
		echo "activation output root must remain outside the scientific repository: $output_root" >&2
		exit 1
		;;
esac
[[ ! -e "$output_root" && ! -L "$output_root" ]] || {
	echo "refusing to overwrite activation output root: $output_root" >&2
	exit 1
}
mkdir -p -- "$(dirname -- "$output_root")"
mkdir -- "$output_root"
[[ "$(realpath -e -- "$output_root")" == "$(realpath -m -- "$output_root")" ]] || {
	echo "activation output root canonicalization changed after creation" >&2
	exit 1
}
treatment_dir="$output_root/treatment"
control_dir="$output_root/control"

activation_free_bytes() {
	local path=$1 df_output available_bytes
	df_output=$(df -P -B1 -- "$path") || return 1
	available_bytes=$(awk 'NR == 2 {print $4}' <<<"$df_output")
	[[ "$available_bytes" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$available_bytes"
}

activation_process_rss_bytes() {
	local process_id=$1 rss_kib
	rss_kib=$(awk '$1 == "VmRSS:" {print $2; exit}' "/proc/$process_id/status" 2>/dev/null)
	[[ "$rss_kib" =~ ^[0-9]+$ ]] || return 1
	printf '%s\n' "$((rss_kib * 1024))"
}

simulator_pid=""
terminate_simulator() {
	local child_pid=${simulator_pid:-}
	[[ "$child_pid" =~ ^[1-9][0-9]*$ ]] || return 0
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
	simulator_pid=""
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

prepare_arm() {
	local arm=$1 config=$2
	mkdir -- "$arm"
	"$binary" -config "$config" -logdir "$arm" -log-mode full -evidence-format evstream_v3 -write-effective-config "$arm/run-config.json"
	local config_sha binary_sha experiment hypothesis
	if ! cmp -s -- "$config" "$arm/run-config.json"; then
		echo "effective activation config differs from registered source config: $config" >&2
		return 1
	fi
	config_sha=$(sha256sum -- "$arm/run-config.json" | awk '{print $1}')
	binary_sha=$(sha256sum -- "$binary" | awk '{print $1}')
	experiment=$(jq -er '.experiment_id' "$arm/run-config.json")
	hypothesis=$(jq -er '.hypothesis_id' "$arm/run-config.json")
	jq -n \
		--arg cell "${v2_r2_sv1_activation_output_prefix}-${activation_seed}-$(basename "$arm")" \
		--argjson seed "$activation_seed" \
		--arg horizon "$horizon" \
		--argjson simulation_start_nano "$simulation_start_nano" \
		--argjson simulation_end_nano "$simulation_end_nano" \
		--arg config_sha256 "$config_sha" \
		--arg binary_sha256 "$binary_sha" \
		--arg git_revision "$head_revision" \
		--arg experiment "$experiment" \
		--arg hypothesis "$hypothesis" \
		--arg evidence_format evstream_v3 \
		--arg log_mode full \
			--arg binary_path "$binary" \
			--arg review_attestation_path "$review_attestation" \
			--arg review_attestation_sha256 "$review_attestation_sha256" \
			--arg binary_go_version "$binary_go_version" \
			--arg binary_goos "$binary_goos" --arg binary_goarch "$binary_goarch" --arg binary_goamd64 "$binary_goamd64" \
			--argjson gomaxprocs "$activation_gomaxprocs" \
			--argjson memory_limit_bytes "$activation_memory_limit_bytes" \
			--argjson gomemlimit_bytes "$activation_gomemlimit_bytes" \
			--argjson minimum_free_bytes "$activation_minimum_free_bytes" \
		--argjson venue_ids "$(jq -c '.venue_ids' "$arm/run-config.json")" \
		--arg contract "$v2_r2_sv1_activation_contract" \
		'{schema_version: 1, contract: $contract,
		 cell: $cell, seed: $seed, simulated_horizon: $horizon,
		 simulation_start_nano: $simulation_start_nano, simulation_end_nano: $simulation_end_nano,
		 config_sha256: $config_sha256, binary_sha256: $binary_sha256,
		 git_revision: $git_revision, config_experiment_id: $experiment,
		 hypothesis_id: $hypothesis, evidence_format: $evidence_format, log_mode: $log_mode,
			 venue_ids: $venue_ids, binary_path: $binary_path, binary_go_version: $binary_go_version,
			 binary_goos: $binary_goos, binary_goarch: $binary_goarch, binary_goamd64: $binary_goamd64,
			 review_attestation_path: $review_attestation_path, review_attestation_sha256: $review_attestation_sha256,
			 gomaxprocs: $gomaxprocs, memory_limit_bytes: $memory_limit_bytes,
			 gomemlimit_bytes: $gomemlimit_bytes, minimum_free_bytes: $minimum_free_bytes,
		 command: ["multivenue", "-config", "run-config.json", "-duration", $horizon,
		           "-logdir", ".", "-log-mode", $log_mode, "-evidence-format", $evidence_format]}' \
		>"$arm/run-metadata.json"
}

run_arm() {
	local arm=$1
	local metadata_sha_before status terminal_failure=false outcome_status
	metadata_sha_before=$(sha256sum -- "$arm/run-metadata.json" | awk '{print $1}')
	local stdout_log="$output_root/$(basename "$arm").stdout.log"
	local stderr_log="$output_root/$(basename "$arm").stderr.log"
	simulator_peak_rss_bytes=0
	simulator_peak_rss_at=""
	simulator_initial_free_bytes=$(activation_free_bytes "$output_root") || {
		echo "could not measure activation free space before launching $arm" >&2
		return 1
	}
	simulator_final_free_bytes="$simulator_initial_free_bytes"
	resource_guard_failed=false
	resource_guard_reason=""
	if (( simulator_initial_free_bytes < activation_minimum_free_bytes )); then
		echo "activation free space is below the ${activation_minimum_free_bytes}-byte reserve before launching $arm" >&2
		return 1
	fi
	env GOMAXPROCS="$activation_gomaxprocs" GOMEMLIMIT="${activation_gomemlimit_bytes}B" prlimit --as="$activation_memory_limit_bytes" -- \
		"$binary" -config "$arm/run-config.json" -duration "$horizon" -logdir "$arm" -log-mode full -evidence-format evstream_v3 \
		>"$stdout_log" 2>"$stderr_log" &
	simulator_pid=$!
	while kill -0 "$simulator_pid" 2>/dev/null; do
		if ! current_rss_bytes=$(activation_process_rss_bytes "$simulator_pid"); then
			resource_guard_failed=true
			resource_guard_reason="simulator RSS measurement failed while $arm was running"
			terminate_simulator
			break
		fi
		if (( current_rss_bytes > simulator_peak_rss_bytes )); then
			simulator_peak_rss_bytes=$current_rss_bytes
			simulator_peak_rss_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
		fi
		if (( current_rss_bytes > activation_memory_limit_bytes )); then
			resource_guard_failed=true
			resource_guard_reason="simulator RSS crossed the ${activation_memory_limit_bytes}-byte ceiling: $current_rss_bytes"
			terminate_simulator
			break
		fi
		if ! simulator_final_free_bytes=$(activation_free_bytes "$output_root"); then
			resource_guard_failed=true
			resource_guard_reason="activation free-space measurement failed while $arm was running"
			terminate_simulator
			break
		fi
		if (( simulator_final_free_bytes < activation_minimum_free_bytes )); then
			resource_guard_failed=true
			resource_guard_reason="activation free space crossed the ${activation_minimum_free_bytes}-byte reserve: $simulator_final_free_bytes"
			terminate_simulator
			break
		fi
		sleep 1
	done
	if [[ -n "$simulator_pid" ]]; then
		if wait "$simulator_pid"; then
			status=0
		else
			status=$?
		fi
		simulator_pid=""
	else
		status=125
	fi
	if ! simulator_final_free_bytes=$(activation_free_bytes "$output_root"); then
		resource_guard_failed=true
		resource_guard_reason="final activation free-space measurement failed"
	else
		if (( simulator_final_free_bytes < activation_minimum_free_bytes )); then
			resource_guard_failed=true
			resource_guard_reason="final activation free space is below the ${activation_minimum_free_bytes}-byte reserve: $simulator_final_free_bytes"
		fi
	fi
	mv -- "$stdout_log" "$arm/simulator.stdout.log"
	mv -- "$stderr_log" "$arm/simulator.stderr.log"
	if [[ "$resource_guard_failed" == true ]]; then
		echo "activation resource guard failed for $arm: $resource_guard_reason" >&2
		return 1
	fi
	[[ -s "$arm/terminal-outcome.json" ]] || {
		echo "activation arm did not produce a typed terminal outcome: $arm" >&2
		return 1
	}
	if ! jq -e --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
		-f "$root_dir/scripts/v2-r2-sv1-terminal-outcome.jq" "$arm/terminal-outcome.json" >/dev/null; then
		echo "activation arm produced an invalid typed terminal outcome: $arm" >&2
		return 1
	fi
	outcome_status=$(jq -er '.status' "$arm/terminal-outcome.json") || return 1
	case "$outcome_status:$status" in
		completed:0) ;;
		terminal_failure:0)
			echo "activation terminal failure did not propagate a nonzero status: $arm" >&2
			return 1
			;;
		terminal_failure:*) terminal_failure=true ;;
		*)
			echo "activation exit status $status is inconsistent with typed outcome $outcome_status: $arm" >&2
			return 1
			;;
	esac
	[[ -s "$arm/greeks.json" && -s "$arm/latency.json" && -s "$arm/manifest.json" && -s "$arm/checkpoints.jsonl" && -s "$arm/events.evs" && -s "$arm/binary-evidence-attestation.json" ]] || {
		echo "activation arm did not produce all completion evidence: $arm" >&2
		return 1
	}
	jq -e --arg revision "$head_revision" --argjson seed "$activation_seed" --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		'.build.revision == $revision and .build.modified == false and .build.goos == "linux" and .build.goarch == "amd64" and .build.goamd64 == "v1" and .venue_ids == ["north", "central", "south"] and
		 .config.seed == $seed and .config.log_mode == "full" and .config.evidence_format == "evstream_v3"' \
		"$arm/manifest.json" >/dev/null || return 1
	jq -e --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		--argjson terminal_failure "$terminal_failure" \
		'(.initial_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $simulation_start_nano)) and
		 (if $terminal_failure then
			((.terminal_accounts // []) | type == "array" and length == 0) and
			((.terminal_risk // {}) | type == "object" and length == 0) and
			.report_status == "partial_terminal_failure" and .terminal_valuation_available == false
		 else
			(.terminal_accounts | type == "array" and length > 0 and all(.[]; .account.timestamp == $simulation_end_nano)) and
			.report_status == "complete_terminal_valuation" and .terminal_valuation_available == true
		 end)' "$arm/greeks.json" >/dev/null || return 1
	v2_r2_require_checkpoint_stream "$arm/checkpoints.jsonl" "$simulation_start_nano" "$simulation_end_nano" || return 1
	[[ "$metadata_sha_before" == "$(sha256sum -- "$arm/run-metadata.json" | awk '{print $1}')" ]] || {
		echo "activation metadata changed during simulation: $arm" >&2
		return 1
	}
	v2_r2_write_evidence_manifest "$arm" || return 1
	v2_r2_verify_evidence_manifest "$arm" || return 1
	local run_status_tmp="$arm/run-status.json.tmp-$$"
	local terminal_outcome_sha256 evidence_manifest_sha256
	terminal_outcome_sha256=$(sha256sum -- "$arm/terminal-outcome.json" | awk '{print $1}')
	evidence_manifest_sha256=$(sha256sum -- "$arm/evidence-manifest.json" | awk '{print $1}')
	jq -n --arg arm "$(basename "$arm")" --argjson exit_status "$status" \
		--arg outcome_status "$outcome_status" --argjson terminal_failure "$terminal_failure" \
		--arg terminal_outcome_sha256 "$terminal_outcome_sha256" \
		--arg run_metadata_sha256 "$metadata_sha_before" \
		--arg manifest_sha256 "$(sha256sum -- "$arm/manifest.json" | awk '{print $1}')" \
		--arg greeks_sha256 "$(sha256sum -- "$arm/greeks.json" | awk '{print $1}')" \
		--arg latency_sha256 "$(sha256sum -- "$arm/latency.json" | awk '{print $1}')" \
		--arg checkpoints_sha256 "$(sha256sum -- "$arm/checkpoints.jsonl" | awk '{print $1}')" \
			--arg binary_attestation_sha256 "$(sha256sum -- "$arm/binary-evidence-attestation.json" | awk '{print $1}')" \
			--arg evidence_manifest_sha256 "$evidence_manifest_sha256" \
			--argjson peak_rss_bytes "$simulator_peak_rss_bytes" --arg peak_rss_at "$simulator_peak_rss_at" \
			--argjson initial_free_bytes "$simulator_initial_free_bytes" --argjson final_free_bytes "$simulator_final_free_bytes" \
			--argjson resource_guard_failed "$resource_guard_failed" --arg resource_guard_reason "$resource_guard_reason" \
			'{schema_version: 2, contract: "v2-r2-sv1b-activation-arm-status-v1", arm: $arm,
		 exit_status: $exit_status, completion_verified: ($terminal_failure | not),
		 terminal_failure_verified: $terminal_failure, terminal_outcome_status: $outcome_status,
		 terminal_outcome_sha256: $terminal_outcome_sha256, run_metadata_sha256: $run_metadata_sha256,
		 manifest_sha256: $manifest_sha256, greeks_sha256: $greeks_sha256,
		 latency_sha256: $latency_sha256, checkpoints_sha256: $checkpoints_sha256,
			  binary_attestation_sha256: $binary_attestation_sha256,
			  evidence_manifest_sha256: $evidence_manifest_sha256,
			  peak_rss_bytes: $peak_rss_bytes, peak_rss_observed_at: $peak_rss_at,
			  initial_available_free_bytes: $initial_free_bytes, final_available_free_bytes: $final_free_bytes,
			  resource_guard_failed: $resource_guard_failed, resource_guard_reason: $resource_guard_reason}' >"$run_status_tmp" || return 1
	mv -- "$run_status_tmp" "$arm/run-status.json"
}

prepare_arm "$treatment_dir" "$treatment_config"
prepare_arm "$control_dir" "$control_config"
set +e
run_arm "$treatment_dir"
treatment_run_status=$?
run_arm "$control_dir"
control_run_status=$?
set -e

write_pair_provenance() {
	local pair_status=$1 activation_satisfied=$2 comparison_sha=""
	[[ $# -ge 3 ]] && comparison_sha=$3
	local provenance_tmp="$output_root/activation-provenance.json.tmp-$$"
	local treatment_terminal_status_json=null control_terminal_status_json=null
	local treatment_status_sha256="" control_status_sha256=""
	local treatment_terminal_outcome_sha256="" control_terminal_outcome_sha256=""
	local treatment_artifacts='[]' control_artifacts='[]' candidate_tree_sha256
	local treatment_source_config_path control_source_config_path
	local treatment_source_config_sha256 control_source_config_sha256
	candidate_tree_sha256=$(v2_r2_sv1b_git_tree_sha256 "$head_revision") || return 1
	treatment_source_config_path=$(realpath -e -- "$treatment_config") || return 1
	control_source_config_path=$(realpath -e -- "$control_config") || return 1
	treatment_source_config_sha256=$(sha256sum -- "$treatment_source_config_path" | awk '{print $1}')
	control_source_config_sha256=$(sha256sum -- "$control_source_config_path" | awk '{print $1}')
	treatment_artifacts=$(v2_r2_sv1b_artifact_records "$treatment_dir" 2>/dev/null || printf '[]\n')
	control_artifacts=$(v2_r2_sv1b_artifact_records "$control_dir" 2>/dev/null || printf '[]\n')
	if [[ -s "$treatment_dir/run-status.json" ]]; then
		treatment_terminal_status_json=$(jq -c '.terminal_outcome_status' "$treatment_dir/run-status.json") || return 1
		treatment_status_sha256=$(sha256sum -- "$treatment_dir/run-status.json" | awk '{print $1}')
		treatment_terminal_outcome_sha256=$(jq -r '.terminal_outcome_sha256' "$treatment_dir/run-status.json") || return 1
	fi
	if [[ -s "$control_dir/run-status.json" ]]; then
		control_terminal_status_json=$(jq -c '.terminal_outcome_status' "$control_dir/run-status.json") || return 1
		control_status_sha256=$(sha256sum -- "$control_dir/run-status.json" | awk '{print $1}')
		control_terminal_outcome_sha256=$(jq -r '.terminal_outcome_sha256' "$control_dir/run-status.json") || return 1
	fi
	jq -n \
		--arg contract "$v2_r2_sv1_activation_pair_contract" \
		--arg candidate "$head_revision" \
		--arg output_root "$output_root" \
		--arg treatment "$treatment_dir" \
		--arg control "$control_dir" \
		--arg treatment_config_sha256 "$(sha256sum -- "$treatment_dir/run-config.json" | awk '{print $1}')" \
		--arg control_config_sha256 "$(sha256sum -- "$control_dir/run-config.json" | awk '{print $1}')" \
		--arg binary_sha256 "$(sha256sum -- "$binary" | awk '{print $1}')" \
		--arg analyzer_sha256 "$(sha256sum -- "$audit_binary" | awk '{print $1}')" \
		--arg comparison_sha256 "$comparison_sha" \
		--argjson seed "$activation_seed" --arg horizon "$horizon" \
		--arg pair_status "$pair_status" --argjson activation_satisfied "$activation_satisfied" \
		--argjson treatment_run_status "$treatment_run_status" --argjson control_run_status "$control_run_status" \
		--argjson treatment_terminal_status "$treatment_terminal_status_json" \
		--argjson control_terminal_status "$control_terminal_status_json" \
		--arg treatment_status_sha256 "$treatment_status_sha256" \
			--arg control_status_sha256 "$control_status_sha256" \
			--arg treatment_terminal_outcome_sha256 "$treatment_terminal_outcome_sha256" \
			--arg control_terminal_outcome_sha256 "$control_terminal_outcome_sha256" \
			--arg candidate_tree_sha256 "$candidate_tree_sha256" \
			--arg review_attestation_path "$review_attestation" \
			--arg review_attestation_sha256 "$review_attestation_sha256" \
			--arg simulator_binary_path "$binary" --arg analyzer_binary_path "$audit_binary" \
			--argjson treatment_artifacts "$treatment_artifacts" --argjson control_artifacts "$control_artifacts" \
			--arg treatment_source_config_path "$treatment_source_config_path" --arg control_source_config_path "$control_source_config_path" \
			--arg treatment_source_config_sha256 "$treatment_source_config_sha256" --arg control_source_config_sha256 "$control_source_config_sha256" \
			'{schema_version: 3, contract: $contract, candidate_revision: $candidate,
		 seed: $seed, simulated_horizon: $horizon, output_root: $output_root,
			 candidate_tree_sha256: $candidate_tree_sha256,
			 treatment_dir: $treatment, control_dir: $control,
			 treatment_source_config_path: $treatment_source_config_path, control_source_config_path: $control_source_config_path,
			 treatment_source_config_sha256: $treatment_source_config_sha256, control_source_config_sha256: $control_source_config_sha256,
			 simulator_binary_path: $simulator_binary_path, analyzer_binary_path: $analyzer_binary_path,
			 review_attestation_path: $review_attestation_path, review_attestation_sha256: $review_attestation_sha256,
			 comparison_path: ($output_root + "/cdf-liquidity-comparison.json"),
		 treatment_config_sha256: $treatment_config_sha256, control_config_sha256: $control_config_sha256,
		 simulator_binary_sha256: $binary_sha256, analyzer_binary_sha256: $analyzer_sha256,
		 comparison_sha256: (if $comparison_sha256 == "" then null else $comparison_sha256 end),
		 status: $pair_status, activation_satisfied: $activation_satisfied,
		 treatment_runner_status: $treatment_run_status, control_runner_status: $control_run_status,
		 treatment_terminal_status: $treatment_terminal_status, control_terminal_status: $control_terminal_status,
		 treatment_run_status_sha256: $treatment_status_sha256,
		 control_run_status_sha256: $control_status_sha256,
		 treatment_terminal_outcome_sha256: $treatment_terminal_outcome_sha256,
			 control_terminal_outcome_sha256: $control_terminal_outcome_sha256,
			 treatment_artifacts: $treatment_artifacts, control_artifacts: $control_artifacts,
			 resource_policy: {gomaxprocs: $activation_gomaxprocs, memory_limit_bytes: $activation_memory_limit_bytes,
			   gomemlimit_bytes: $activation_gomemlimit_bytes, minimum_free_bytes: $activation_minimum_free_bytes},
		 holdouts_consumed: false,
		 scope: "development-only mechanism activation; not a 24-hour survival claim"}' \
		>"$provenance_tmp" || return 1
	mv -- "$provenance_tmp" "$output_root/activation-provenance.json"
}

if [[ "$treatment_run_status" -ne 0 || "$control_run_status" -ne 0 ]]; then
	write_pair_provenance "INVALID_ARM_EVIDENCE" false
	echo "activation probe rejected malformed or generic arm evidence; see $output_root" >&2
	exit 1
fi

if ! treatment_terminal_status=$(jq -er '.terminal_outcome_status' "$treatment_dir/run-status.json") ||
	! control_terminal_status=$(jq -er '.terminal_outcome_status' "$control_dir/run-status.json"); then
	write_pair_provenance "INVALID_ARM_EVIDENCE" false
	echo "activation probe could not read typed terminal arm status; see $output_root" >&2
	exit 1
fi

if [[ "$treatment_terminal_status" != completed || "$control_terminal_status" != completed ]]; then
	comparison_tmp="$output_root/cdf-liquidity-comparison.json.tmp-$$"
	jq -n --arg contract "$v2_r2_sv1_activation_contract" --argjson seed "$activation_seed" \
		--arg treatment_status "$treatment_terminal_status" --arg control_status "$control_terminal_status" \
		'{schema_version: 2, contract: $contract, seed: $seed, status: "UNAVAILABLE_TERMINAL_FAILURE",
		 valid: false, evidence_valid: true, activation_satisfied: false,
		 anti_cheating_satisfied: false, measurement_valid: true,
		 treatment_terminal_status: $treatment_status, control_terminal_status: $control_status,
		 reason: "typed terminal failure prevents a complete paired activation comparison"}' \
		>"$comparison_tmp"
	mv -- "$comparison_tmp" "$output_root/cdf-liquidity-comparison.json"
	comparison_sha=$(sha256sum -- "$output_root/cdf-liquidity-comparison.json" | awk '{print $1}')
	write_pair_provenance "UNAVAILABLE_TERMINAL_FAILURE" false "$comparison_sha"
	echo "activation probe recorded typed terminal failure; no economic activation verdict: $output_root"
	# A typed terminal failure is retained evidence, but it is not a successful
	# activation probe. Propagate failure so callers cannot promote it by exit
	# status alone.
	exit 1
fi

comparison_tmp="$output_root/cdf-liquidity-comparison.json.tmp-$$"
if "$audit_binary" -treatment "$treatment_dir" -control "$control_dir" >"$comparison_tmp"; then
	audit_status=0
else
	audit_status=$?
fi
if [[ "$audit_status" -ne 0 ]] || ! jq -e 'type == "object"' "$comparison_tmp" >/dev/null; then
	mv -- "$comparison_tmp" "$output_root/cdf-liquidity-comparison.json.invalid"
	write_pair_provenance "INVALID_AUDIT_EVIDENCE" false
	echo "activation probe rejected malformed CDF audit evidence; see $output_root" >&2
	exit 1
fi
mv -- "$comparison_tmp" "$output_root/cdf-liquidity-comparison.json"
expected_supplier_count=$(jq -er '(.elastic_liquidity_suppliers | length) * (.venue_ids | length)' "$treatment_config")
activation_satisfied=false
if v2_r2_require_cdf_supplier_comparison "$output_root/cdf-liquidity-comparison.json" "$expected_supplier_count"; then
	activation_satisfied=true
fi

comparison_sha=$(sha256sum -- "$output_root/cdf-liquidity-comparison.json" | awk '{print $1}')
if [[ "$activation_satisfied" == true ]]; then
	write_pair_provenance "ACTIVATION_CONTRACT_SATISFIED" true "$comparison_sha"
	echo "completed V2-R2-SV1 activation probe: $output_root"
	exit 0
fi
write_pair_provenance "ACTIVATION_CONTRACT_NOT_SATISFIED" false "$comparison_sha"
echo "activation contract not satisfied: suppliers did not demonstrate finite bounded activity" >&2
exit 1
