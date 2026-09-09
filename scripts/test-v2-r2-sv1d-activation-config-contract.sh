#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
contract="$root_dir/scripts/v2-r2-sv1d-activation-contract.sh"
checker="$root_dir/scripts/check-v2-r2-sv1d-activation-configs.sh"
generator="$root_dir/scripts/render-v2-r2-sv1d-activation-configs.sh"
status_writer="$root_dir/scripts/v2-r2-sv1-activation-status.sh"
source "$contract"
temp_root=$(mktemp -d)
trap 'rm -rf -- "$temp_root"' EXIT

runner="$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh"
scorer="$root_dir/scripts/score-v2-r2-sv1d-activation.sh"
for file in "$contract" "$checker" "$generator" "$runner" "$scorer"; do
	[[ -f "$file" && -x "$file" ]] || { echo "SV1D activation contract script is not executable: $file" >&2; exit 1; }
done
[[ -f "$status_writer" && ! -L "$status_writer" ]] || { echo "SV1D activation status writer is missing or symlinked" >&2; exit 1; }

"$checker"

rg -F 'activation-659-treatment.json' "$contract" "$checker" "$generator" >/dev/null
rg -F 'activation-659-mode-off.json' "$contract" "$checker" "$generator" >/dev/null
rg -F 'activation-659-no-roster.json' "$contract" "$checker" "$generator" >/dev/null
rg -F 'quote_on_one_sided_local_book' "$generator" "$checker" >/dev/null
rg -F 'minimum_qualifying_qty' "$generator" "$checker" >/dev/null
rg -F 'registered_minimum_executable_qty' "$generator" "$checker" >/dev/null
rg -F 'holdouts_consumed: false' "$generator" >/dev/null
rg -F 'elastic_supplier_count == 8' "$checker" >/dev/null
rg -F 'treatment_mode_filter' "$checker" >/dev/null
rg -F 'v2_r2_sv1d_require_mode_pair_comparison' "$contract" "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" >/dev/null
rg -F 'v2_r2_sv1d_require_no_roster_diagnostic' "$contract" "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'process_group_rss_bytes' "$runner" >/dev/null
rg -F 'activation_analyzer_max_wall_seconds' "$contract" "$runner" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'simulator_stdout_sha256' "$status_writer" "$contract" "$runner" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'max_inventory_utilization' "$contract" "$root_dir/analysis/cdf_liquidity.go" >/dev/null
rg -F 'filled_qty >= .configured_minimum_qualifying_qty' "$contract" >/dev/null
rg -F 'v2_r2_sv1d_require_activation_capacity' "$contract" "$checker" "$generator" >/dev/null
rg -F 'host_memory_total_bytes' "$contract" "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'v2_r2_sv1d_require_arm_record_matches' "$contract" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'arm_artifacts_valid' "$contract" "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" >/dev/null
rg -F 'mode-off' "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'status --porcelain --untracked-files=all' "$generator" "$checker" >/dev/null
rg -F 'v2-r2-sv1-activation-status.sh' "$contract" "$generator" "$checker" >/dev/null
rg -F 'ask-only/positive-inventory' "$root_dir/research/v2-r2-sv1d-one-sided-elastic-successor-preregistration-2026-09-09.md" >/dev/null

if rg -F 'v2_r2_require_cdf_supplier_comparison "$comparison_record_path"' "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" >/dev/null; then
	echo "SV1D runner still uses the historical zero-roster comparison predicate" >&2
	exit 1
fi

if rg -F 'holdout-619' "$generator" "$checker" "$contract" >/dev/null; then
	echo "SV1D activation package names a holdout" >&2
	exit 1
fi
if rg -F 'capacity' "$generator" >/dev/null; then
	echo "SV1D activation config generator must not silently perform capacity work" >&2
	exit 1
fi

[[ "$(v2_r2_sv1d_required_memory_available_bytes 100)" == "$((4 * 1024 * 1024 * 1024))" ]] || {
	echo "SV1D memory floor did not preserve the four-GiB minimum" >&2
	exit 1
}
capacity_fixture="$temp_root/capacity.json"
jq -n '{elastic_liquidity_suppliers:[{initial_base_balance:100,initial_quote_balance:1000,max_position:150,max_inventory:250,max_quote_qty:1,max_loss_quote:10}]}' >"$capacity_fixture"
v2_r2_sv1d_require_activation_capacity "$capacity_fixture" || {
	echo "horizon-relative finite-capital fixture was rejected" >&2
	exit 1
}
jq '.elastic_liquidity_suppliers[0].max_position = 151' "$capacity_fixture" >"$temp_root/over-capacity.json"
if v2_r2_sv1d_require_activation_capacity "$temp_root/over-capacity.json"; then
	echo "horizon-relative finite-capital overflow fixture was accepted" >&2
	exit 1
fi

no_roster_fixture="$temp_root/no-roster"
mkdir -p -- "$no_roster_fixture/venues/north"
printf '%s\n' '{"initial_accounts":[{"role":"liability_hedger"}]}' >"$no_roster_fixture/greeks.json"
printf '%s\n' '{}' >"$no_roster_fixture/venues/north/general.jsonl"
printf '%s\n' '{"status":"completed"}' >"$no_roster_fixture/terminal-outcome.json"
for required_file in run-status.json run-metadata.json manifest.json binary-evidence-attestation.json evidence-manifest.json simulator.stdout.log simulator.stderr.log; do
	printf '%s\n' '{}' >"$no_roster_fixture/$required_file"
done
no_roster_greeks_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/greeks.json")
no_roster_terminal_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/terminal-outcome.json")
no_roster_status_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/run-status.json")
no_roster_metadata_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/run-metadata.json")
no_roster_manifest_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/manifest.json")
no_roster_attestation_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/binary-evidence-attestation.json")
no_roster_evidence_manifest_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/evidence-manifest.json")
no_roster_stdout_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/simulator.stdout.log")
no_roster_stderr_sha256=$(v2_r2_sv1d_sha256_file "$no_roster_fixture/simulator.stderr.log")
no_roster_diagnostic="$temp_root/no-roster-diagnostic.json"
jq -n --arg config_sha256 "$(printf '%064d' 0)" --arg status_sha256 "$no_roster_status_sha256" \
	--arg terminal_sha256 "$no_roster_terminal_sha256" --arg metadata_sha256 "$no_roster_metadata_sha256" \
	--arg manifest_sha256 "$no_roster_manifest_sha256" --arg greeks_sha256 "$no_roster_greeks_sha256" \
	--arg attestation_sha256 "$no_roster_attestation_sha256" --arg evidence_manifest_sha256 "$no_roster_evidence_manifest_sha256" \
	--arg stdout_sha256 "$no_roster_stdout_sha256" --arg stderr_sha256 "$no_roster_stderr_sha256" \
	'{schema_version:1,contract:"v2-r2-sv1d-no-roster-diagnostic-v1",arm:"no-roster",config_sha256:$config_sha256,
	 run_status_sha256:$status_sha256,terminal_outcome_sha256:$terminal_sha256,run_metadata_sha256:$metadata_sha256,
	 manifest_sha256:$manifest_sha256,greeks_sha256:$greeks_sha256,binary_attestation_sha256:$attestation_sha256,
	 evidence_manifest_sha256:$evidence_manifest_sha256,simulator_stdout_sha256:$stdout_sha256,simulator_stderr_sha256:$stderr_sha256,
	 terminal_status:"completed",strict_population_accounting:true,cdf_roster:false,cdf_metrics:"out_of_scope",
	 runtime_topology_sha256:$greeks_sha256,runtime_topology_valid:true,runtime_cdf_supplier_count:0,
	 runtime_cdf_decision_count:0,runtime_cdf_fill_count:0,status:"VALID_TOPOLOGY_CONTROL",holdouts_consumed:false}' \
	>"$no_roster_diagnostic"
v2_r2_sv1d_require_no_roster_diagnostic "$no_roster_diagnostic" "$no_roster_fixture" "$(printf '%064d' 0)" || {
	echo "valid runtime no-roster topology fixture was rejected" >&2
	exit 1
}
jq '.runtime_topology_sha256 = ("0" * 64)' "$no_roster_diagnostic" >"$temp_root/no-roster-bad-topology.json"
if v2_r2_sv1d_require_no_roster_diagnostic "$temp_root/no-roster-bad-topology.json" "$no_roster_fixture" "$(printf '%064d' 0)"; then
	echo "runtime no-roster topology hash mutation was accepted" >&2
	exit 1
fi

terminal_treatment="$temp_root/terminal-treatment"
terminal_control="$temp_root/terminal-control"
mkdir -- "$terminal_treatment" "$terminal_control"
printf '%s\n' '{"status":"terminal_failure"}' >"$terminal_treatment/terminal-outcome.json"
printf '%s\n' '{"status":"completed"}' >"$terminal_control/terminal-outcome.json"
printf '%s\n' '{}' >"$terminal_treatment/run-status.json"
printf '%s\n' '{}' >"$terminal_control/run-status.json"
treatment_status_sha256=$(v2_r2_sv1d_sha256_file "$terminal_treatment/run-status.json")
control_status_sha256=$(v2_r2_sv1d_sha256_file "$terminal_control/run-status.json")
treatment_terminal_sha256=$(v2_r2_sv1d_sha256_file "$terminal_treatment/terminal-outcome.json")
control_terminal_sha256=$(v2_r2_sv1d_sha256_file "$terminal_control/terminal-outcome.json")
terminal_comparison="$temp_root/terminal-comparison.json"
jq -n --arg contract "$v2_r2_sv1_activation_contract" --argjson seed "$v2_r2_sv1_activation_seed" \
	--arg horizon "$v2_r2_sv1_activation_horizon" --argjson start "$v2_r2_sv1_activation_simulation_start_nano" \
	--argjson end "$v2_r2_sv1_activation_simulation_end_nano" --arg treatment_status terminal_failure --arg control_status completed \
	--arg treatment_status_sha256 "$treatment_status_sha256" --arg control_status_sha256 "$control_status_sha256" \
	--arg treatment_terminal_sha256 "$treatment_terminal_sha256" --arg control_terminal_sha256 "$control_terminal_sha256" \
	'{schema_version:1,contract:$contract,seed:$seed,simulated_horizon:$horizon,simulation_start_nano:$start,simulation_end_nano:$end,
	 status:"UNAVAILABLE_TERMINAL_FAILURE",valid:false,evidence_valid:true,activation_satisfied:false,anti_cheating_satisfied:false,
	 provenance:null,arm_artifacts_valid:true,holdouts_consumed:false,treatment_terminal_status:$treatment_status,control_terminal_status:$control_status,
	 treatment_run_status_sha256:$treatment_status_sha256,control_run_status_sha256:$control_status_sha256,
	 treatment_terminal_outcome_sha256:$treatment_terminal_sha256,control_terminal_outcome_sha256:$control_terminal_sha256}' \
	>"$terminal_comparison"
v2_r2_sv1d_require_comparison_provenance "$terminal_comparison" "$(printf '%064d' 0)" "$(printf '%040d' 0)" "$terminal_treatment" "$terminal_control" || {
	echo "valid terminal comparison fixture was rejected" >&2
	exit 1
}
jq '.control_run_status_sha256 = ("0" * 64)' "$terminal_comparison" >"$temp_root/terminal-comparison-bad-hash.json"
if v2_r2_sv1d_require_comparison_provenance "$temp_root/terminal-comparison-bad-hash.json" "$(printf '%064d' 0)" "$(printf '%040d' 0)" "$terminal_treatment" "$terminal_control"; then
	echo "terminal comparison hash mutation was accepted" >&2
	exit 1
fi

echo "SV1D activation config contract: PASS"
