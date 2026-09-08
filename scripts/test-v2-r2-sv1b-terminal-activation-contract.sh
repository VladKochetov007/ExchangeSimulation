#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
runner="$root_dir/scripts/run-v2-r2-sv1-activation-probe.sh"
source "$root_dir/scripts/v2-r2-sv1b-24h-contract.sh"
fixture_root=$(mktemp -d)
trap 'rm -rf -- "$fixture_root"' EXIT

rg -F 'expected_arm_outcome=terminal_failure' "$runner" >/dev/null || {
	echo "activation runner does not route terminal failures to the diagnostic arm contract" >&2
	exit 1
}
rg -F 'v2_r2_sv1b_require_terminal_failure_pair_provenance "$activation_provenance_pending"' "$runner" >/dev/null || {
	echo "activation runner does not self-validate terminal-failure pair provenance" >&2
	exit 1
}

revision=$(git -C "$root_dir" rev-parse HEAD)
binary_sha256=$(printf 'synthetic activation binary\n' | sha256sum | awk '{print $1}')
zeros=$(printf '%064d' 0)
start=$v2_r2_sv1_activation_simulation_start_nano
end=$v2_r2_sv1_activation_simulation_end_nano

write_terminal_arm() {
	local arm_name=$1 config=$2 arm_dir="$fixture_root/$1"
	local config_sha256 venue_ids experiment hypothesis
	mkdir -p -- "$arm_dir/venues/north"
	cp -- "$config" "$arm_dir/run-config.json"
	config_sha256=$(sha256sum -- "$config" | awk '{print $1}')
	venue_ids=$(jq -c '.venue_ids' "$config")
	experiment=$(jq -er '.experiment_id' "$config")
	hypothesis=$(jq -er '.hypothesis_id' "$config")
	jq -n --arg cell "v2-r2-sv1b-activation-643-$arm_name" --arg revision "$revision" \
		--arg config_sha256 "$config_sha256" --arg binary_sha256 "$binary_sha256" \
		--arg experiment "$experiment" --arg hypothesis "$hypothesis" --argjson seed 643 \
		--argjson start "$start" --argjson end "$end" --argjson venue_ids "$venue_ids" \
		'{schema_version:1,cell:$cell,seed:$seed,simulated_horizon:"5m",
		 simulation_start_nano:$start,simulation_end_nano:$end,config_sha256:$config_sha256,
		 binary_sha256:$binary_sha256,git_revision:$revision,config_experiment_id:$experiment,
		 hypothesis_id:$hypothesis,evidence_format:"evstream_v3",log_mode:"full",venue_ids:$venue_ids,
		 binary_go_version:"go1.27.0",binary_goos:"linux",binary_goarch:"amd64",binary_goamd64:"v1",
		 gomaxprocs:2,memory_limit_bytes:21474836480,gomemlimit_bytes:19327352832,
		 minimum_free_bytes:4294967296,cpu_limit_percent:90}' >"$arm_dir/run-metadata.json"
	jq -n --argjson start "$start" --argjson end "$end" \
		'{schema_version:2,status:"terminal_failure",code:"PRICE_UNAVAILABLE",phase:"terminal_post_mark",
		 simulation_start_nano:$start,simulation_end_nano:$end,strict_population_accounting:true,
		 evidence_format:"evstream_v3",evidence_sealed:true,terminal_risk_captured:false,
		 terminal_population_captured:false,stage:"terminal_risk_capture",failure_at_nano:$end,
		 failure_venue_id:"north",failure_symbol:"CDF/USD",error:"no usable price"}' \
		>"$arm_dir/terminal-outcome.json"
	jq -n --argjson start "$start" --slurpfile outcome "$arm_dir/terminal-outcome.json" \
		'{schema_version:7,report_status:"partial_terminal_failure",terminal_valuation_available:false,
		 initial_accounts:[{account:{timestamp:$start}}],terminal_accounts:[],terminal_risk:{},
		 terminal_outcome:$outcome[0]}' >"$arm_dir/greeks.json"
	printf '{}\n' >"$arm_dir/latency.json"
	jq -nc --argjson time "$end" --arg hash "$zeros" \
		'{domain:"execution_observations",ordering:"ordered_stream",sim_time:$time,event_count:0,execution_stream_hash:$hash}' \
		>"$arm_dir/checkpoints.jsonl"
	jq -nc --argjson time "$end" --arg hash "$zeros" \
		'{domain:"execution_observations",ordering:"ordered_stream",sim_time:$time,event_count:0,execution_stream_hash:$hash,final:true}' \
		>>"$arm_dir/checkpoints.jsonl"
	jq -n --arg revision "$revision" --argjson venue_ids "$venue_ids" \
		'{schema_version:2,build:{revision:$revision,modified:false,goos:"linux",goarch:"amd64",goamd64:"v1"},
		 venue_ids:$venue_ids,config:{seed:643,log_mode:"full",evidence_format:"evstream_v3",
		 record_market_data_receipts:true}}' >"$arm_dir/manifest.json"
	printf '\000synthetic sealed stream\n' >"$arm_dir/events.evs"
	jq -n --arg hash "$zeros" \
		'{domain:"canonical_binary_execution_frames",ordering:"ordered_stream",hashing:"route_sequence_neutral_v1",
		 event_frames:1,stream_frames:1,execution_stream_hash:$hash,canonical_execution_stream_hash:$hash,
		 unencodable_payloads:0}' >"$arm_dir/binary-evidence-attestation.json"
	printf '{}\n' >"$arm_dir/evidence-only-artifact-hash.json"
	printf '{}\n' >"$arm_dir/market-data-evidence-v2.json"
	: >"$arm_dir/market-data-schedules-v2.bin"
	: >"$arm_dir/market-data-receipts-v2.bin"
	: >"$arm_dir/market-data-decisions-v2.bin"
	: >"$arm_dir/market-data-actions-v2.bin"
	printf '{}\n' >"$arm_dir/venues/north/general.jsonl"
	: >"$arm_dir/simulator.stdout.log"
	: >"$arm_dir/simulator.stderr.log"
	v2_r2_write_evidence_manifest "$arm_dir"
	v2_r2_verify_evidence_manifest "$arm_dir"
	jq -n --arg arm "$arm_name" \
		--arg terminal "$(sha256sum -- "$arm_dir/terminal-outcome.json" | awk '{print $1}')" \
		--arg metadata "$(sha256sum -- "$arm_dir/run-metadata.json" | awk '{print $1}')" \
		--arg manifest "$(sha256sum -- "$arm_dir/manifest.json" | awk '{print $1}')" \
		--arg greeks "$(sha256sum -- "$arm_dir/greeks.json" | awk '{print $1}')" \
		--arg latency "$(sha256sum -- "$arm_dir/latency.json" | awk '{print $1}')" \
		--arg checkpoints "$(sha256sum -- "$arm_dir/checkpoints.jsonl" | awk '{print $1}')" \
		--arg binary_attestation "$(sha256sum -- "$arm_dir/binary-evidence-attestation.json" | awk '{print $1}')" \
		--arg evidence_manifest "$(sha256sum -- "$arm_dir/evidence-manifest.json" | awk '{print $1}')" \
		'{schema_version:2,contract:"v2-r2-sv1b-activation-arm-status-v1",arm:$arm,exit_status:1,
		 completion_verified:false,terminal_failure_verified:true,terminal_outcome_status:"terminal_failure",
		 terminal_outcome_sha256:$terminal,run_metadata_sha256:$metadata,manifest_sha256:$manifest,
		 greeks_sha256:$greeks,latency_sha256:$latency,checkpoints_sha256:$checkpoints,
		 binary_attestation_sha256:$binary_attestation,evidence_manifest_sha256:$evidence_manifest,
		 resource_guard_failed:false}' >"$arm_dir/run-status.json"
}

write_terminal_arm treatment "$v2_r2_sv1_activation_config"
write_terminal_arm control "$v2_r2_sv1_activation_control_config"
comparison_path="$fixture_root/cdf-liquidity-comparison.json"
jq -n --arg contract "$v2_r2_sv1_activation_contract" \
	'{schema_version:2,contract:$contract,seed:643,status:"UNAVAILABLE_TERMINAL_FAILURE",valid:false,
	 evidence_valid:true,activation_satisfied:false,anti_cheating_satisfied:false,measurement_valid:true,
	 treatment_terminal_status:"terminal_failure",control_terminal_status:"terminal_failure"}' >"$comparison_path"
comparison_sha256=$(sha256sum -- "$comparison_path" | awk '{print $1}')
treatment_artifacts=$(v2_r2_sv1b_artifact_records "$fixture_root/treatment")
control_artifacts=$(v2_r2_sv1b_artifact_records "$fixture_root/control")
treatment_config_sha256=$(sha256sum -- "$v2_r2_sv1_activation_config" | awk '{print $1}')
control_config_sha256=$(sha256sum -- "$v2_r2_sv1_activation_control_config" | awk '{print $1}')
treatment_status_sha256=$(sha256sum -- "$fixture_root/treatment/run-status.json" | awk '{print $1}')
control_status_sha256=$(sha256sum -- "$fixture_root/control/run-status.json" | awk '{print $1}')
treatment_outcome_sha256=$(sha256sum -- "$fixture_root/treatment/terminal-outcome.json" | awk '{print $1}')
control_outcome_sha256=$(sha256sum -- "$fixture_root/control/terminal-outcome.json" | awk '{print $1}')
provenance_path="$fixture_root/activation-provenance.json"
jq -n --arg contract "$v2_r2_sv1_activation_pair_contract" --arg revision "$revision" \
	--arg binary_sha256 "$binary_sha256" --arg output_root "$fixture_root" \
	--arg treatment_dir "$fixture_root/treatment" --arg control_dir "$fixture_root/control" \
	--arg comparison_path "$comparison_path" --arg comparison_sha256 "$comparison_sha256" \
	--arg treatment_config_sha256 "$treatment_config_sha256" --arg control_config_sha256 "$control_config_sha256" \
	--arg treatment_status_sha256 "$treatment_status_sha256" --arg control_status_sha256 "$control_status_sha256" \
	--arg treatment_outcome_sha256 "$treatment_outcome_sha256" --arg control_outcome_sha256 "$control_outcome_sha256" \
	--argjson treatment_artifacts "$treatment_artifacts" --argjson control_artifacts "$control_artifacts" \
	'{schema_version:3,contract:$contract,candidate_revision:$revision,candidate_tree_sha256:"",
	 seed:643,simulated_horizon:"5m",simulation_start_nano:1735689600000000000,
	 simulation_end_nano:1735689900000000000,evidence_format:"evstream_v3",log_mode:"full",
	 venue_ids:["north","central","south"],treatment_experiment_id:"v2-r2-sv1b-activation-643",
	 control_experiment_id:"v2-r2-sv1b-activation-643-control",
	 treatment_hypothesis_id:"V2-R2-SV1B-CDF-LIQUIDITY",
	 control_hypothesis_id:"V2-R2-SV1B-CDF-LIQUIDITY-CONTROL",output_root:$output_root,
	 treatment_dir:$treatment_dir,control_dir:$control_dir,comparison_path:$comparison_path,
	 treatment_source_config_path:"",control_source_config_path:"",treatment_source_config_sha256:$treatment_config_sha256,
	 control_source_config_sha256:$control_config_sha256,simulator_binary_path:"/bin/true",analyzer_binary_path:"/bin/true",
	 review_attestation_path:"/bin/true",review_attestation_sha256:"",simulator_binary_sha256:$binary_sha256,
	 analyzer_binary_sha256:"0000000000000000000000000000000000000000000000000000000000000000",comparison_sha256:$comparison_sha256,
	 treatment_config_sha256:$treatment_config_sha256,control_config_sha256:$control_config_sha256,
	 treatment_run_status_sha256:$treatment_status_sha256,control_run_status_sha256:$control_status_sha256,
	 treatment_terminal_outcome_sha256:$treatment_outcome_sha256,control_terminal_outcome_sha256:$control_outcome_sha256,
	 status:"UNAVAILABLE_TERMINAL_FAILURE",activation_satisfied:false,holdouts_consumed:false,
	 treatment_runner_status:0,control_runner_status:0,treatment_terminal_status:"terminal_failure",
	 control_terminal_status:"terminal_failure",treatment_artifacts:$treatment_artifacts,control_artifacts:$control_artifacts}' \
	>"$provenance_path"
v2_r2_sv1b_require_terminal_failure_pair_provenance "$provenance_path" "$revision" "$binary_sha256" || {
	echo "valid terminal-failure pair diagnostic was rejected" >&2
	exit 1
}
jq '.treatment_terminal_outcome_sha256 = ("1" * 64)' "$provenance_path" >"$fixture_root/mutated-provenance.json"
if v2_r2_sv1b_require_terminal_failure_pair_provenance "$fixture_root/mutated-provenance.json" "$revision" "$binary_sha256"; then
	echo "terminal-failure pair accepted a mismatched outcome hash" >&2
	exit 1
fi
printf 'V2-R2-SV1B terminal-failure activation contract: pass\n'
