#!/usr/bin/env bash

# Write the atomic per-arm completion status shared by the activation runner
# and its contract fixtures. The selected contract supplies the status-contract
# identity; the writer never embeds a namespace-specific literal.
v2_r2_activation_status_file_sha256() {
	[[ $# -eq 1 ]] || return 1
	local file=$1 checksum digest
	[[ -f "$file" && ! -L "$file" ]] || return 1
	checksum=$(sha256sum -- "$file") || return 1
	digest=${checksum%% *}
	[[ "$digest" =~ ^[0-9a-f]{64}$ ]] || return 1
	printf '%s\n' "$digest"
}

v2_r2_write_activation_arm_status() {
	[[ $# -eq 11 || $# -eq 12 ]] || return 1
	local arm=$1 status=$2 outcome_status=$3 terminal_failure=$4 run_metadata_sha256=$5
	local peak_rss_bytes=$6 peak_rss_at=$7 initial_free_bytes=$8 final_free_bytes=$9
	local resource_guard_failed=${10} resource_guard_reason=${11}
	local elapsed_wall_seconds=${12:-0}
	local run_status_tmp="$arm/run-status.json.tmp-$$"
	local terminal_outcome_sha256 evidence_manifest_sha256 manifest_sha256 greeks_sha256
	local latency_sha256 checkpoints_sha256 binary_attestation_sha256 stdout_log_sha256 stderr_log_sha256 arm_status_contract
	arm_status_contract=${v2_r2_sv1_activation_arm_status_contract:-v2-r2-sv1b-activation-arm-status-v1}
	[[ -d "$arm" && ! -L "$arm" && -s "$arm/terminal-outcome.json" && -s "$arm/evidence-manifest.json" ]] || return 1
	[[ "$status" =~ ^[0-9]+$ && "$peak_rss_bytes" =~ ^[0-9]+$ && "$initial_free_bytes" =~ ^[0-9]+$ && "$final_free_bytes" =~ ^[0-9]+$ ]] || return 1
	[[ "$terminal_failure" == true || "$terminal_failure" == false ]] || return 1
	[[ "$resource_guard_failed" == true || "$resource_guard_failed" == false ]] || return 1
	[[ "$elapsed_wall_seconds" =~ ^[0-9]+$ ]] || return 1
	[[ "$run_metadata_sha256" =~ ^[0-9a-f]{64}$ ]] || return 1
	terminal_outcome_sha256=$(v2_r2_activation_status_file_sha256 "$arm/terminal-outcome.json") || return 1
	evidence_manifest_sha256=$(v2_r2_activation_status_file_sha256 "$arm/evidence-manifest.json") || return 1
	manifest_sha256=$(v2_r2_activation_status_file_sha256 "$arm/manifest.json") || return 1
	greeks_sha256=$(v2_r2_activation_status_file_sha256 "$arm/greeks.json") || return 1
	latency_sha256=$(v2_r2_activation_status_file_sha256 "$arm/latency.json") || return 1
	checkpoints_sha256=$(v2_r2_activation_status_file_sha256 "$arm/checkpoints.jsonl") || return 1
	binary_attestation_sha256=$(v2_r2_activation_status_file_sha256 "$arm/binary-evidence-attestation.json") || return 1
	stdout_log_sha256=null
	stderr_log_sha256=null
	if [[ -f "$arm/simulator.stdout.log" && ! -L "$arm/simulator.stdout.log" ]]; then
		stdout_log_sha256=$(v2_r2_activation_status_file_sha256 "$arm/simulator.stdout.log") || return 1
	fi
	if [[ -f "$arm/simulator.stderr.log" && ! -L "$arm/simulator.stderr.log" ]]; then
		stderr_log_sha256=$(v2_r2_activation_status_file_sha256 "$arm/simulator.stderr.log") || return 1
	fi
	jq -n --arg arm "$(basename "$arm")" --argjson exit_status "$status" \
		--arg outcome_status "$outcome_status" --argjson terminal_failure "$terminal_failure" \
		--arg terminal_outcome_sha256 "$terminal_outcome_sha256" \
		--arg run_metadata_sha256 "$run_metadata_sha256" \
		--arg manifest_sha256 "$manifest_sha256" \
		--arg greeks_sha256 "$greeks_sha256" \
		--arg latency_sha256 "$latency_sha256" \
		--arg checkpoints_sha256 "$checkpoints_sha256" \
		--arg binary_attestation_sha256 "$binary_attestation_sha256" \
		--arg evidence_manifest_sha256 "$evidence_manifest_sha256" \
		--argjson stdout_log_sha256 "$stdout_log_sha256" \
		--argjson stderr_log_sha256 "$stderr_log_sha256" \
		--argjson peak_rss_bytes "$peak_rss_bytes" --arg peak_rss_at "$peak_rss_at" \
		--argjson initial_free_bytes "$initial_free_bytes" --argjson final_free_bytes "$final_free_bytes" \
		--argjson resource_guard_failed "$resource_guard_failed" --arg resource_guard_reason "$resource_guard_reason" \
		--argjson elapsed_wall_seconds "$elapsed_wall_seconds" \
		--arg arm_status_contract "$arm_status_contract" \
		'{schema_version: 2, contract: $arm_status_contract, arm: $arm,
		 exit_status: $exit_status, completion_verified: ($terminal_failure | not),
		 terminal_failure_verified: $terminal_failure, terminal_outcome_status: $outcome_status,
		 terminal_outcome_sha256: $terminal_outcome_sha256, run_metadata_sha256: $run_metadata_sha256,
		 manifest_sha256: $manifest_sha256, greeks_sha256: $greeks_sha256,
		 latency_sha256: $latency_sha256, checkpoints_sha256: $checkpoints_sha256,
		 binary_attestation_sha256: $binary_attestation_sha256,
		 evidence_manifest_sha256: $evidence_manifest_sha256,
		 simulator_stdout_sha256: $stdout_log_sha256,
		 simulator_stderr_sha256: $stderr_log_sha256,
		 peak_rss_bytes: $peak_rss_bytes, peak_rss_observed_at: $peak_rss_at,
		 wall_clock_seconds: $elapsed_wall_seconds,
		 initial_available_free_bytes: $initial_free_bytes, final_available_free_bytes: $final_free_bytes,
			 resource_guard_failed: $resource_guard_failed, resource_guard_reason: $resource_guard_reason}' >"$run_status_tmp" || return 1
	mv -- "$run_status_tmp" "$arm/run-status.json" || return 1
}
