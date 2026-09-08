#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
runner="$root_dir/scripts/run-v2-r2-sv1-activation-probe.sh"
source "$root_dir/scripts/v2-r2-sv1b-24h-contract.sh"
fixture_root=$(mktemp -d)
review_root=$(mktemp -d)
trap 'rm -rf -- "$fixture_root" "$review_root"' EXIT

rg -F 'expected_arm_outcome=terminal_failure' "$runner" >/dev/null || {
	echo "activation runner does not route terminal failures to the diagnostic arm contract" >&2
	exit 1
}
rg -F 'v2_r2_sv1b_require_terminal_failure_pair_provenance "$activation_provenance_pending"' "$runner" >/dev/null || {
	echo "activation runner does not self-validate terminal-failure pair provenance" >&2
	exit 1
}
rg -F 'renderer=${3:-"$root_dir/bin/evsrender"}' "$runner" >/dev/null || {
	echo "activation runner does not bind the terminal evidence renderer" >&2
	exit 1
}
rg -F 'v2_r2_sv1b_require_terminal_arm_reconstruction "$renderer_path" "$arm_dir"' "$root_dir/scripts/v2-r2-sv1b-24h-contract.sh" >/dev/null || {
	echo "terminal pair contract does not reconstruct each sealed binary arm" >&2
	exit 1
}

revision=$(git -C "$root_dir" rev-parse HEAD)
tree_sha256=$(v2_r2_sv1b_git_tree_sha256 "$revision")
go_command=/usr/local/go/bin/go
[[ -x "$go_command" ]] || go_command=$(command -v go)
[[ -x "$go_command" ]] || { echo "Go toolchain is unavailable for terminal fixture" >&2; exit 1; }
"$go_command" version | grep -F 'go1.27.0' >/dev/null || { echo "terminal fixture requires Go 1.27.0" >&2; exit 1; }

build_pinned_fixture_binary() {
	local output=$1 package_path=$2
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1 "$go_command" build -trimpath -buildvcs=true -buildmode=exe -o "$output" "$package_path"
}

build_pinned_fixture_binary "$fixture_root/multivenue" ./cmd/multivenue
build_pinned_fixture_binary "$fixture_root/cdf-liquidity-audit" ./cmd/cdf-liquidity-audit
build_pinned_fixture_binary "$fixture_root/evsrender" ./cmd/evsrender
build_pinned_fixture_binary "$fixture_root/evsfixture" ./cmd/evsfixture
binary_sha256=$(sha256sum -- "$fixture_root/multivenue" | awk '{print $1}')
analyzer_sha256=$(sha256sum -- "$fixture_root/cdf-liquidity-audit" | awk '{print $1}')
renderer_sha256=$(sha256sum -- "$fixture_root/evsrender" | awk '{print $1}')
zeros=$(printf '%064d' 0)
start=$v2_r2_sv1_activation_simulation_start_nano
end=$v2_r2_sv1_activation_simulation_end_nano
IFS=$'\t' read -r host_cpu_count allowed_cpu_count cpu_affinity < <(v2_r2_sv1b_cpu_policy)

review_report_path="$review_root/review-report.md"
review_attestation_path="$review_root/review-attestation.json"
printf 'Synthetic contract fixture review for exact revision %s.\n' "$revision" >"$review_report_path"
review_report_sha256=$(sha256sum -- "$review_report_path" | awk '{print $1}')
jq -n --arg contract "$v2_r2_sv1_review_contract" --arg revision "$revision" --arg tree "$tree_sha256" \
	--arg report "$review_report_path" --arg report_sha256 "$review_report_sha256" \
	--argjson scope "$v2_r2_sv1_review_scope" \
	'{schema_version:1,contract:$contract,reviewed_revision:$revision,reviewed_tree_sha256:$tree,
	 review_type:"independent_sol_xhigh",verdict:"ACCEPTED_FOR_ACTIVATION",
	 reviewer:"synthetic-contract-fixture",reviewed_worktree_clean:true,holdouts_consumed:false,
	 reviewed_scope:$scope,review_report_path:$report,review_report_sha256:$report_sha256}' \
	>"$review_attestation_path"

write_terminal_arm() {
	local arm_name=$1 config=$2 arm_dir="$fixture_root/$1"
	local config_sha256 venue_ids experiment hypothesis stream_report stream_event_frames stream_frames
	local stream_execution_hash stream_canonical_hash
	mkdir -- "$arm_dir"
	cp -- "$config" "$arm_dir/run-config.json"
	mkdir -p -- "$arm_dir/venues/north"
	: >"$arm_dir/venues/north/general.jsonl"
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
	stream_report="$fixture_root/${arm_name}-stream-report.json"
	"$fixture_root/evsfixture" -out "$arm_dir/events.evs" >"$stream_report"
	stream_event_frames=$(jq -er '.event_frames' "$stream_report")
	stream_frames=$(jq -er '.stream_frames' "$stream_report")
	stream_execution_hash=$(jq -er '.execution_stream_hash' "$stream_report")
	stream_canonical_hash=$(jq -er '.canonical_execution_stream_hash' "$stream_report")
	jq -nc --argjson start "$start" --argjson end "$end" --arg empty_hash "$zeros" \
		--arg hash "$stream_execution_hash" --argjson event_count "$stream_event_frames" \
		'[
		 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$start,event_count:0,
		  execution_stream_hash:$empty_hash,rolling_hash:$empty_hash,representation:"evstream_v3",unencodable_payloads:0},
		 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$end,event_count:$event_count,
		  execution_stream_hash:$hash,rolling_hash:$hash,representation:"evstream_v3",unencodable_payloads:0},
		 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$end,event_count:$event_count,
		  execution_stream_hash:$hash,rolling_hash:$hash,representation:"evstream_v3",unencodable_payloads:0,final:true}
		][]' >"$arm_dir/checkpoints.jsonl"
	jq -n --arg hash "$stream_execution_hash" --arg canonical_hash "$stream_canonical_hash" \
		--argjson event_frames "$stream_event_frames" --argjson stream_frames "$stream_frames" \
		'{domain:"canonical_binary_execution_frames",ordering:"ordered_stream",hashing:"route_sequence_neutral_v1",
		 event_frames:$event_frames,stream_frames:$stream_frames,execution_stream_hash:$hash,
		 canonical_execution_stream_hash:$canonical_hash,unencodable_payloads:0}' \
		>"$arm_dir/binary-evidence-attestation.json"
	jq -n --arg hash "$zeros" \
		'{domain:"persisted_json_log_evidence_only",ordering:"unordered_multiset",events:0,digest:$hash}' \
		>"$arm_dir/evidence-only-artifact-hash.json"
	jq -n --arg revision "$revision" --argjson venue_ids "$venue_ids" \
		'{schema_version:2,build:{revision:$revision,modified:false,goos:"linux",goarch:"amd64",goamd64:"v1"},
		 venue_ids:$venue_ids,config:{seed:643,log_mode:"full",evidence_format:"evstream_v3",
		 record_market_data_receipts:true}}' >"$arm_dir/manifest.json"
	printf '{}\n' >"$arm_dir/market-data-evidence-v2.json"
	: >"$arm_dir/market-data-schedules-v2.bin"
	: >"$arm_dir/market-data-receipts-v2.bin"
	: >"$arm_dir/market-data-decisions-v2.bin"
	: >"$arm_dir/market-data-actions-v2.bin"
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

expect_checkpoint_rejected() {
	local fixture_name=$1
	if v2_r2_require_checkpoint_stream "$fixture_root/$fixture_name" "$start" "$end" evstream_v3; then
		echo "invalid terminal checkpoint stream was accepted: $fixture_name" >&2
		exit 1
	fi
}

expect_checkpoint_rejected_with_bounds() {
	local fixture_name=$1 lower_bound=$2 upper_bound=$3
	if v2_r2_require_checkpoint_stream "$fixture_root/$fixture_name" "$lower_bound" "$upper_bound" evstream_v3; then
		echo "invalid terminal checkpoint stream was accepted: $fixture_name" >&2
		exit 1
	fi
}

v2_r2_require_checkpoint_stream "$fixture_root/treatment/checkpoints.jsonl" "$start" "$end" evstream_v3
v2_r2_sv1b_require_checkpoint_attestation_binding \
	"$fixture_root/treatment/checkpoints.jsonl" "$fixture_root/treatment/binary-evidence-attestation.json"

jq -c 'if .event_count == 0 then .sim_time = 0.5 else .sim_time = 10 end' \
	"$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/fractional-sim-time-checkpoint.jsonl"
expect_checkpoint_rejected_with_bounds fractional-sim-time-checkpoint.jsonl 0 10
jq -c 'if .event_count == 0 then .event_count = 0.5 else . end' \
	"$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/fractional-event-count-checkpoint.jsonl"
expect_checkpoint_rejected fractional-event-count-checkpoint.jsonl
jq -c '.representation = "jsonl"' "$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/wrong-representation-checkpoint.jsonl"
expect_checkpoint_rejected wrong-representation-checkpoint.jsonl
jq -c '.unencodable_payloads = 99' "$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/unencodable-checkpoint.jsonl"
expect_checkpoint_rejected unencodable-checkpoint.jsonl
jq -c '.rolling_hash = ("f" * 64)' "$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/mismatched-rolling-hash-checkpoint.jsonl"
expect_checkpoint_rejected mismatched-rolling-hash-checkpoint.jsonl

jq -c --argjson end "$end" 'if .sim_time == $end then .event_count += 1 else . end' \
	"$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/mismatched-attestation-count-checkpoint.jsonl"
v2_r2_require_checkpoint_stream "$fixture_root/mismatched-attestation-count-checkpoint.jsonl" "$start" "$end" evstream_v3
if v2_r2_sv1b_require_checkpoint_attestation_binding \
	"$fixture_root/mismatched-attestation-count-checkpoint.jsonl" "$fixture_root/treatment/binary-evidence-attestation.json"; then
	echo "checkpoint/binary attestation accepted a mismatched terminal event count" >&2
	exit 1
fi
jq -c --argjson end "$end" \
	'if .sim_time == $end then .execution_stream_hash = ("e" * 64) | .rolling_hash = ("e" * 64) else . end' \
	"$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/mismatched-attestation-hash-checkpoint.jsonl"
v2_r2_require_checkpoint_stream "$fixture_root/mismatched-attestation-hash-checkpoint.jsonl" "$start" "$end" evstream_v3
if v2_r2_sv1b_require_checkpoint_attestation_binding \
	"$fixture_root/mismatched-attestation-hash-checkpoint.jsonl" "$fixture_root/treatment/binary-evidence-attestation.json"; then
	echo "checkpoint/binary attestation accepted a mismatched terminal stream hash" >&2
	exit 1
fi

head -n 2 "$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/missing-terminal-checkpoint.jsonl"
expect_checkpoint_rejected missing-terminal-checkpoint.jsonl
head -c -8 "$fixture_root/treatment/checkpoints.jsonl" >"$fixture_root/truncated-checkpoint.jsonl"
expect_checkpoint_rejected truncated-checkpoint.jsonl
jq -nc --argjson start "$start" --argjson end "$end" --arg empty_hash "$zeros" \
	--arg hash "$(jq -er '.execution_stream_hash' "$fixture_root/treatment/binary-evidence-attestation.json")" \
	--argjson event_count "$(jq -er '.event_frames' "$fixture_root/treatment/binary-evidence-attestation.json")" \
	'[
	 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$end,event_count:0,
	  execution_stream_hash:$empty_hash,rolling_hash:$empty_hash,representation:"evstream_v3",unencodable_payloads:0},
	 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$start,event_count:$event_count,
	  execution_stream_hash:$hash,rolling_hash:$hash,representation:"evstream_v3",unencodable_payloads:0},
	 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$end,event_count:$event_count,
	  execution_stream_hash:$hash,rolling_hash:$hash,representation:"evstream_v3",unencodable_payloads:0,final:true}
	][]' >"$fixture_root/out-of-order-checkpoint.jsonl"
expect_checkpoint_rejected out-of-order-checkpoint.jsonl
jq -nc --argjson start "$start" --argjson end "$end" --arg empty_hash "$zeros" \
	--arg hash "$(jq -er '.execution_stream_hash' "$fixture_root/treatment/binary-evidence-attestation.json")" \
	--argjson event_count "$(jq -er '.event_frames' "$fixture_root/treatment/binary-evidence-attestation.json")" \
	'[
	 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$start,event_count:0,
	  execution_stream_hash:$empty_hash,rolling_hash:$empty_hash,representation:"evstream_v3",unencodable_payloads:0},
	 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$end,event_count:$event_count,
	  execution_stream_hash:$hash,rolling_hash:$hash,representation:"evstream_v3",unencodable_payloads:0},
	 {domain:"execution_observations",ordering:"ordered_stream",sim_time:$end,event_count:($event_count + 1),
	  execution_stream_hash:$hash,rolling_hash:$hash,representation:"evstream_v3",unencodable_payloads:0,final:true}
	][]' >"$fixture_root/non-repeated-terminal-checkpoint.jsonl"
expect_checkpoint_rejected non-repeated-terminal-checkpoint.jsonl

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
treatment_source_config_path=$(realpath -e -- "$v2_r2_sv1_activation_config")
control_source_config_path=$(realpath -e -- "$v2_r2_sv1_activation_control_config")
treatment_experiment_id=$(jq -er '.experiment_id' "$treatment_source_config_path")
control_experiment_id=$(jq -er '.experiment_id' "$control_source_config_path")
treatment_hypothesis_id=$(jq -er '.hypothesis_id' "$treatment_source_config_path")
control_hypothesis_id=$(jq -er '.hypothesis_id' "$control_source_config_path")
provenance_path="$fixture_root/activation-provenance.json"
jq -n --arg contract "$v2_r2_sv1_activation_pair_contract" --arg revision "$revision" \
	--arg tree_sha256 "$tree_sha256" --arg binary_sha256 "$binary_sha256" --arg analyzer_sha256 "$analyzer_sha256" \
	--arg renderer_sha256 "$renderer_sha256" --arg output_root "$fixture_root" \
	--arg treatment_dir "$fixture_root/treatment" --arg control_dir "$fixture_root/control" \
	--arg comparison_path "$comparison_path" --arg comparison_sha256 "$comparison_sha256" \
	--arg treatment_config_sha256 "$treatment_config_sha256" --arg control_config_sha256 "$control_config_sha256" \
	--arg treatment_status_sha256 "$treatment_status_sha256" --arg control_status_sha256 "$control_status_sha256" \
	--arg treatment_outcome_sha256 "$treatment_outcome_sha256" --arg control_outcome_sha256 "$control_outcome_sha256" \
	--arg treatment_source_config_path "$treatment_source_config_path" --arg control_source_config_path "$control_source_config_path" \
	--arg treatment_experiment_id "$treatment_experiment_id" --arg control_experiment_id "$control_experiment_id" \
	--arg treatment_hypothesis_id "$treatment_hypothesis_id" --arg control_hypothesis_id "$control_hypothesis_id" \
	--arg simulator_binary_path "$fixture_root/multivenue" --arg analyzer_binary_path "$fixture_root/cdf-liquidity-audit" \
	--arg renderer_binary_path "$fixture_root/evsrender" --arg review_attestation_path "$review_attestation_path" \
	--arg review_attestation_sha256 "$(sha256sum -- "$review_attestation_path" | awk '{print $1}')" \
	--argjson host_cpu_count "$host_cpu_count" --argjson allowed_cpu_count "$allowed_cpu_count" --arg cpu_affinity "$cpu_affinity" \
	--argjson treatment_artifacts "$treatment_artifacts" --argjson control_artifacts "$control_artifacts" \
	'{schema_version:3,contract:$contract,candidate_revision:$revision,candidate_tree_sha256:$tree_sha256,
	 seed:643,simulated_horizon:"5m",simulation_start_nano:1735689600000000000,
	 simulation_end_nano:1735689900000000000,evidence_format:"evstream_v3",log_mode:"full",
	 venue_ids:["north","central","south"],treatment_experiment_id:$treatment_experiment_id,
	 control_experiment_id:$control_experiment_id,
	 treatment_hypothesis_id:$treatment_hypothesis_id,
	 control_hypothesis_id:$control_hypothesis_id,output_root:$output_root,
	 treatment_dir:$treatment_dir,control_dir:$control_dir,comparison_path:$comparison_path,
	 treatment_source_config_path:$treatment_source_config_path,control_source_config_path:$control_source_config_path,
	 treatment_source_config_sha256:$treatment_config_sha256,control_source_config_sha256:$control_config_sha256,
	 simulator_binary_path:$simulator_binary_path,analyzer_binary_path:$analyzer_binary_path,renderer_binary_path:$renderer_binary_path,
	 review_attestation_path:$review_attestation_path,review_attestation_sha256:$review_attestation_sha256,
	 simulator_binary_sha256:$binary_sha256,analyzer_binary_sha256:$analyzer_sha256,renderer_binary_sha256:$renderer_sha256,
	 comparison_sha256:$comparison_sha256,
	 treatment_config_sha256:$treatment_config_sha256,control_config_sha256:$control_config_sha256,
	 treatment_run_status_sha256:$treatment_status_sha256,control_run_status_sha256:$control_status_sha256,
	 treatment_terminal_outcome_sha256:$treatment_outcome_sha256,control_terminal_outcome_sha256:$control_outcome_sha256,
	 status:"UNAVAILABLE_TERMINAL_FAILURE",activation_satisfied:false,holdouts_consumed:false,
	 treatment_runner_status:0,control_runner_status:0,treatment_terminal_status:"terminal_failure",
	 control_terminal_status:"terminal_failure",treatment_artifacts:$treatment_artifacts,control_artifacts:$control_artifacts,
	 resource_policy:{gomaxprocs:2,memory_limit_bytes:21474836480,gomemlimit_bytes:19327352832,
	 host_cpu_count:$host_cpu_count,allowed_cpu_count:$allowed_cpu_count,cpu_limit_percent:90,
	 cpu_affinity:$cpu_affinity,minimum_free_bytes:4294967296}}' \
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
jq '.comparison_sha256 = ("2" * 64)' "$provenance_path" >"$fixture_root/mutated-comparison-provenance.json"
if v2_r2_sv1b_require_terminal_failure_pair_provenance "$fixture_root/mutated-comparison-provenance.json" "$revision" "$binary_sha256"; then
	echo "terminal-failure pair accepted a mismatched comparison hash" >&2
	exit 1
fi
jq '.candidate_tree_sha256 = ("3" * 64)' "$provenance_path" >"$fixture_root/mutated-tree-provenance.json"
if v2_r2_sv1b_require_terminal_failure_pair_provenance "$fixture_root/mutated-tree-provenance.json" "$revision" "$binary_sha256"; then
	echo "terminal-failure pair accepted a mismatched candidate tree identity" >&2
	exit 1
fi
jq '.renderer_binary_sha256 = ("4" * 64)' "$provenance_path" >"$fixture_root/mutated-renderer-provenance.json"
if v2_r2_sv1b_require_terminal_failure_pair_provenance "$fixture_root/mutated-renderer-provenance.json" "$revision" "$binary_sha256"; then
	echo "terminal-failure pair accepted a mismatched renderer identity" >&2
	exit 1
fi
jq '.resource_policy.cpu_limit_percent = 1' "$provenance_path" >"$fixture_root/mutated-resource-provenance.json"
if v2_r2_sv1b_require_terminal_failure_pair_provenance "$fixture_root/mutated-resource-provenance.json" "$revision" "$binary_sha256"; then
	echo "terminal-failure pair accepted a mismatched resource policy" >&2
	exit 1
fi
cp -a -- "$fixture_root/treatment" "$fixture_root/truncated-treatment"
truncate -s -1 "$fixture_root/truncated-treatment/events.evs"
if "$fixture_root/evsrender" -dir "$fixture_root/truncated-treatment" -out "$fixture_root/truncated-render" -route-compression none >/dev/null 2>&1; then
	echo "renderer accepted a truncated terminal evidence stream" >&2
	exit 1
fi
printf 'V2-R2-SV1B terminal-failure activation contract: pass\n'
