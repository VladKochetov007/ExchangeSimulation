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
capacity_runner="$root_dir/scripts/run-v2-r2-sv1d-24h-capacity-probe.sh"
scorer="$root_dir/scripts/score-v2-r2-sv1d-activation.sh"
for file in "$contract" "$checker" "$generator" "$runner" "$capacity_runner" "$scorer"; do
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
rg -F 'v2_r2_sv1d_require_capacity_attestation' "$contract" "$runner" "$capacity_runner" "$scorer" >/dev/null
rg -F 'run-v2-r2-sv1d-24h-capacity-probe.sh' "$contract" "$checker" "$generator" >/dev/null
rg -F 'capacity_protocol' "$contract" >/dev/null
rg -F 'capacity: {path:' "$runner" >/dev/null
rg -F '.capacity.path' "$scorer" >/dev/null
rg -F 'host_memory_total_bytes' "$contract" "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'host_memory_total_bytes: $comparison_host_memory_total' "$root_dir/scripts/run-v2-r2-sv1d-activation-probe.sh" >/dev/null
rg -F 'comparison_classification=$(v2_r2_sv1d_classify_comparison "$comparison_path" "$expected_supplier_count")' "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'v2_r2_sv1d_require_scoring_comparison_claims' "$root_dir/scripts/score-v2-r2-sv1d-activation.sh" >/dev/null
rg -F 'evidence_valid: $comparison_evidence_valid' "$runner" >/dev/null
rg -F 'terminal_negative: $comparison_terminal_negative' "$runner" >/dev/null
rg -F 'outcome_neutral' "$contract" "$capacity_runner" >/dev/null
rg -F 'v2_r2_sv1d_capacity_binary' "$contract" "$capacity_runner" >/dev/null
rg -F 'v2_r2_sv1d_require_capacity_attestation_shape' "$contract" "$capacity_runner" >/dev/null
rg -F 'required_free_during_run' "$capacity_runner" >/dev/null
rg -F '.capacity.contract == $capacity_contract' "$scorer" >/dev/null
if rg -F 'v2-r2-sv1d-24h-binary-capacity-v1' "$scorer" >/dev/null; then
	echo "SV1D scorer retains the obsolete capacity contract" >&2
	exit 1
fi
[[ "$v2_r2_sv1d_capacity_workload_seed" == 2026091001 &&
	"$v2_r2_sv1d_capacity_event_count" == 26100000 &&
	"$v2_r2_sv1d_capacity_book_delta_events" == 20880000 &&
	"$v2_r2_sv1d_capacity_balance_change_events" == 2610000 &&
	"$v2_r2_sv1d_capacity_opaque_events" == 2610000 ]] || {
	echo "synthetic capacity profile constants changed without a contract amendment" >&2
	exit 1
}
if rg -F -- '-seed 659' "$capacity_runner" >/dev/null; then
	echo "synthetic capacity runner attempts to consume activation seed 659" >&2
	exit 1
fi
if rg -F 'multivenue' "$capacity_runner" >/dev/null; then
	echo "synthetic capacity runner must not mention or invoke the market simulator" >&2
	exit 1
fi
if rg -F -- '-config' "$capacity_runner" >/dev/null || rg -F -- '-duration' "$capacity_runner" >/dev/null; then
	echo "synthetic capacity runner contains simulator launch flags" >&2
	exit 1
fi
if rg -F 'terminal-outcome' "$capacity_runner" >/dev/null || rg -F 'greeks.json' "$capacity_runner" >/dev/null; then
	echo "synthetic capacity runner contains terminal-world artifacts" >&2
	exit 1
fi
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
if rg -F 'run-v2-r2-sv1d-24h-capacity-probe.sh ' "$generator" >/dev/null; then
	echo "SV1D activation config generator must not silently perform capacity work" >&2
	exit 1
fi

[[ "$(v2_r2_sv1d_required_memory_available_bytes 100)" == "$((4 * 1024 * 1024 * 1024))" ]] || {
	echo "SV1D memory floor did not preserve the four-GiB minimum" >&2
	exit 1
}
activation_expected_files=$(v2_r2_sv1d_expected_arm_root_files)
capacity_expected_files=$(v2_r2_sv1d_capacity_expected_cell_files)
grep -Fxq run-status.json <<<"$activation_expected_files" || {
	echo "activation artifact contract unexpectedly lost run-status.json" >&2
	exit 1
}
if grep -Fxq run-status.json <<<"$capacity_expected_files"; then
	echo "capacity artifact contract must not require activation run-status.json" >&2
	exit 1
fi
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
printf '%s\n' '{}' >"$temp_root/malformed-capacity-attestation.json"
if v2_r2_sv1d_require_capacity_attestation "$temp_root/malformed-capacity-attestation.json" \
	"$(git -C "$root_dir" rev-parse HEAD)" "$(printf '%064d' 0)" "$(printf '%064d' 0)" \
	"$temp_root/missing-review.json" "$(printf '%064d' 0)"; then
	echo "malformed capacity attestation was accepted" >&2
	exit 1
fi

capacity_shape_fixture="$temp_root/valid-capacity-attestation-shape.json"
capacity_shape_hash=$(printf '%064d' 0)
capacity_shape_revision=$(printf '%040d' 0)
jq -n --arg contract "$v2_r2_sv1d_capacity_attestation_contract" --arg revision "$capacity_shape_revision" \
	--arg hash "$capacity_shape_hash" --arg profile "$v2_r2_sv1d_capacity_workload_profile" \
	--argjson seed "$v2_r2_sv1d_capacity_workload_seed" --argjson event_count "$v2_r2_sv1d_capacity_event_count" \
	--argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" --argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" \
	--argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" --argjson start "$v2_r2_sv1d_capacity_workload_start_nano" \
	--argjson end "$v2_r2_sv1d_capacity_workload_end_nano" --argjson minimum_free "$v2_r2_sv1d_capacity_minimum_free_bytes" \
	--argjson safety_margin "$v2_r2_sv1d_capacity_safety_margin_bytes" --argjson gomaxprocs "$v2_r2_sv1d_capacity_gomaxprocs" \
	--argjson memory_limit "$v2_r2_sv1d_capacity_memory_limit_bytes" --argjson gomemlimit "$v2_r2_sv1d_capacity_gomemlimit_bytes" \
	--argjson host_memory_total "$((5 * 1024 * 1024 * 1024))" --argjson minimum_memory "$((4 * 1024 * 1024 * 1024))" \
	--argjson peak_output 1024 --argjson peak_rss 1048576 --argjson stream_bytes 4096 \
	--argjson initial_free "$((8 * 1024 * 1024 * 1024))" --argjson final_memory "$((4 * 1024 * 1024 * 1024))" \
	--argjson required_free "$((4 * 1024 * 1024 * 1024 + 1024))" --argjson cpu_limit "$v2_r2_sv1_cpu_limit_percent" \
	--argjson max_wall "$v2_r2_sv1d_capacity_max_wall_seconds" \
	'{schema_version:1,contract:$contract,measurement:"synthetic_24h_binary_evidence_capacity_probe",capacity_only:true,
	 outcome_neutral:true,simulator_invoked:false,terminal_outcome_present:false,holdouts_consumed:false,
	 source_revision:$revision,source_tree_sha256:$hash,evidence_format:"evstream_v3",hashing:"route_and_global_sequence_neutral_v2",ordering:"ordered_stream",
	 capacity_binary:{path:"/opt/evscapacity",sha256:$hash},target_config:{path:"research/configs/v2-r2-sv1d-activation/activation-659-treatment.json",sha256:$hash},
	 review:{path:"/opt/review.json",sha256:$hash},workload:{profile:$profile,seed:$seed,event_count:$event_count,book_delta_events:$book_events,balance_change_events:$balance_events,opaque_scientific_events:$opaque_events,start_nano:$start,end_nano:$end,horizon:"24h"},
	 report_sha256:$hash,profile_sha256:$hash,evidence_manifest_sha256:$hash,stream_sha256:$hash,stream_bytes:$stream_bytes,
	 peak_output_bytes:$peak_output,peak_rss_bytes:$peak_rss,safety_margin_bytes:$safety_margin,required_free_bytes:$required_free,
	 available_free_bytes:$initial_free,initial_available_free_bytes:$initial_free,minimum_free_bytes:$minimum_free,
	 initial_memory_available_bytes:$final_memory,final_memory_available_bytes:$final_memory,wall_clock_seconds:1,
	 resource_policy:{gomaxprocs:$gomaxprocs,memory_limit_bytes:$memory_limit,gomemlimit_bytes:$gomemlimit,cpu_limit_percent:$cpu_limit,minimum_free_bytes:$minimum_free,minimum_memory_available_bytes:$minimum_memory,host_memory_total_bytes:$host_memory_total,host_cpu_count:1,allowed_cpu_count:1,cpu_affinity:"0",max_wall_seconds:$max_wall}}' \
	>"$capacity_shape_fixture"
v2_r2_sv1d_require_capacity_attestation_shape "$capacity_shape_fixture" || {
	echo "valid producer-shaped capacity attestation fixture was rejected" >&2
	exit 1
}
jq '.outcome_neutral = false' "$capacity_shape_fixture" >"$temp_root/invalid-capacity-shape.json"
if v2_r2_sv1d_require_capacity_attestation_shape "$temp_root/invalid-capacity-shape.json"; then
	echo "capacity outcome-bearing shape fixture was accepted" >&2
	exit 1
fi

(
	set -euo pipefail
	full_capacity_revision=$(git -C "$root_dir" rev-parse HEAD)
	full_capacity_tree=$(v2_r2_sv1d_git_tree_sha256 "$full_capacity_revision")
	full_capacity_root="$temp_root/full-capacity-root"
	full_capacity_cell="$full_capacity_root/$(v2_r2_sv1d_capacity_probe_cell)"
	mkdir -p -- "$full_capacity_cell"
	full_capacity_binary="$temp_root/evscapacity"
	printf '%s\n' '#!/bin/sh' 'exit 0' >"$full_capacity_binary"
	chmod 755 "$full_capacity_binary"
	full_capacity_binary_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_binary")
	full_capacity_config="$root_dir/research/configs/v2-r2-sv1d-activation/activation-659-treatment.json"
	full_capacity_config_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_config")
	full_capacity_review_report="$temp_root/full-capacity-review-report.json"
	printf '%s\n' '{}' >"$full_capacity_review_report"
	full_capacity_review_report_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_review_report")
	full_capacity_review="$temp_root/full-capacity-review.json"
	full_capacity_review_scope=$(jq -c '.' <<<"$v2_r2_sv1_review_scope")
	jq -n --arg revision "$full_capacity_revision" --arg tree "$full_capacity_tree" \
		--arg report_path "$full_capacity_review_report" --arg report_sha "$full_capacity_review_report_sha" \
		--argjson scope "$full_capacity_review_scope" \
		'{schema_version:1,contract:"v2-r2-sv1d-independent-review-v1",reviewed_revision:$revision,
		 reviewed_tree_sha256:$tree,review_type:"independent_sol_xhigh",verdict:"ACCEPTED_FOR_ACTIVATION",
		 reviewed_worktree_clean:true,holdouts_consumed:false,reviewer:"full-validator-fixture",
		 reviewed_scope:$scope,review_report_path:$report_path,review_report_sha256:$report_sha}' \
		>"$full_capacity_review"
	full_capacity_review_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_review")

	printf 'evs-fixture\n' >"$full_capacity_cell/events.evs"
	printf 'stdout-fixture\n' >"$full_capacity_cell/capacity.stdout.log"
	printf 'stderr-fixture\n' >"$full_capacity_cell/capacity.stderr.log"
	full_capacity_host_memory_total=$(v2_r2_sv1d_host_memory_total_bytes)
	full_capacity_minimum_memory=$(v2_r2_sv1d_required_memory_available_bytes "$full_capacity_host_memory_total")
	full_capacity_memory_available=$(v2_r2_sv1d_capacity_memory_available_bytes)
	IFS=$'\t' read -r full_capacity_host_cpu full_capacity_allowed_cpu full_capacity_affinity < <(v2_r2_sv1d_cpu_policy)
	full_capacity_free=$(v2_r2_sv1d_capacity_free_bytes "$full_capacity_root")
	full_capacity_minimum_free=$v2_r2_sv1d_capacity_minimum_free_bytes
	full_capacity_safety_margin=$v2_r2_sv1d_capacity_safety_margin_bytes
	full_capacity_peak_output=1024
	full_capacity_required_free=$((full_capacity_peak_output + full_capacity_safety_margin))
	full_capacity_stream_bytes=$(stat -c '%s' -- "$full_capacity_cell/events.evs")
	full_capacity_stream_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_cell/events.evs")
	full_capacity_stdout_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_cell/capacity.stdout.log")
	full_capacity_stderr_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_cell/capacity.stderr.log")

	jq -n --arg profile "$v2_r2_sv1d_capacity_workload_profile" \
		--argjson seed "$v2_r2_sv1d_capacity_workload_seed" --argjson event_count "$v2_r2_sv1d_capacity_event_count" \
		--argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" --argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" \
		--argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" --argjson start "$v2_r2_sv1d_capacity_workload_start_nano" \
		--argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
		'{schema_version:1,contract:"v2-r2-sv1d-synthetic-capacity-workload-v1",profile:{name:$profile,
		 workload_seed:$seed,event_count:$event_count,book_delta_events:$book_events,balance_change_events:$balance_events,
		 opaque_scientific_events:$opaque_events,start_nano:$start,end_nano:$end}}' \
		>"$full_capacity_cell/workload-profile.json"
	jq -n --arg profile "$v2_r2_sv1d_capacity_workload_profile" \
		--argjson seed "$v2_r2_sv1d_capacity_workload_seed" --argjson event_count "$v2_r2_sv1d_capacity_event_count" \
		--argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" --argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" \
		--argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" \
		--argjson start "$v2_r2_sv1d_capacity_workload_start_nano" --argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
		'{schema_version:1,contract:"v2-r2-sv1d-synthetic-capacity-workload-v1",profile:{name:$profile,
		 workload_seed:$seed,event_count:$event_count,book_delta_events:$book_events,balance_change_events:$balance_events,
		 opaque_scientific_events:$opaque_events,start_nano:$start,end_nano:$end},evidence_format:"evstream_v3",
		 hashing:"route_and_global_sequence_neutral_v2",ordering:"ordered_stream",event_frames:$event_count,
		 family_counts:[$book_events,$balance_events,$opaque_events],unencodable_payloads:0,readback_verified:true}' \
		>"$full_capacity_cell/synthetic-capacity-report.json"
	jq -n '{schema_version:1,contract:"v2-r2-sv1d-synthetic-capacity-evidence-v1",
		domain:"canonical_binary_execution_frames",ordering:"ordered_stream",hashing:"route_and_global_sequence_neutral_v2",
		outcome_neutral:true,simulator_invoked:false,holdouts_consumed:false,readback_verified:true,unencodable_payloads:0}' \
		>"$full_capacity_cell/binary-evidence-attestation.json"
	jq -n --arg revision "$full_capacity_revision" \
		--arg config_sha "$full_capacity_config_sha" --arg profile "$v2_r2_sv1d_capacity_workload_profile" \
		--argjson seed "$v2_r2_sv1d_capacity_workload_seed" --argjson event_count "$v2_r2_sv1d_capacity_event_count" \
		--argjson start "$v2_r2_sv1d_capacity_workload_start_nano" --argjson end "$v2_r2_sv1d_capacity_workload_end_nano" \
		'{schema_version:1,contract:"v2-r2-sv1d-synthetic-capacity-run-v1",source_revision:$revision,
		 target_config_path:"research/configs/v2-r2-sv1d-activation/activation-659-treatment.json",
		 target_config_sha256:$config_sha,workload_profile:$profile,workload_seed:$seed,event_count:$event_count,
		 workload_start_nano:$start,workload_end_nano:$end,workload_horizon:"24h",capacity_only:true,
		 outcome_neutral:true,simulator_invoked:false,terminal_outcome_present:false,holdouts_consumed:false,
		 evidence_format:"evstream_v3",command:["evscapacity"]}' \
		>"$full_capacity_cell/run-metadata.json"

	full_capacity_manifest=$(jq -cn --arg cell "$(basename "$full_capacity_cell")" \
		'{schema_version:1,contract:"v2-r2-sv1d-synthetic-capacity-evidence-manifest-v1",cell:$cell,
		 outcome_neutral:true,simulator_invoked:false,holdouts_consumed:false,files:[]}')
	while IFS= read -r full_capacity_relative; do
		full_capacity_bytes=$(stat -c '%s' -- "$full_capacity_cell/$full_capacity_relative")
		full_capacity_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_cell/$full_capacity_relative")
		full_capacity_manifest=$(jq -c --arg path "$full_capacity_relative" --argjson bytes "$full_capacity_bytes" \
			--arg sha "$full_capacity_sha" '.files += [{path:$path,bytes:$bytes,sha256:$sha}]' <<<"$full_capacity_manifest")
	done < <(v2_r2_sv1d_capacity_expected_cell_files | grep -v '^evidence-manifest.json$')
	printf '%s\n' "$full_capacity_manifest" >"$full_capacity_cell/evidence-manifest.json"
	full_capacity_report_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_cell/synthetic-capacity-report.json")
	full_capacity_profile_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_cell/workload-profile.json")
	full_capacity_manifest_sha=$(v2_r2_sv1d_sha256_file "$full_capacity_cell/evidence-manifest.json")
	full_capacity_cell_name=$(basename "$full_capacity_cell")
	full_capacity_attestation="$temp_root/full-capacity-attestation.json"
	jq -n --arg contract "$v2_r2_sv1d_capacity_attestation_contract" --arg revision "$full_capacity_revision" \
		--arg tree "$full_capacity_tree" --arg binary_path "$full_capacity_binary" --arg binary_sha "$full_capacity_binary_sha" \
		--arg config_path "research/configs/v2-r2-sv1d-activation/activation-659-treatment.json" --arg config_sha "$full_capacity_config_sha" \
		--arg profile "$v2_r2_sv1d_capacity_workload_profile" --arg review_path "$full_capacity_review" --arg review_sha "$full_capacity_review_sha" \
		--arg probe_root "$full_capacity_root" --arg probe_cell "$full_capacity_cell_name" \
		--arg report_sha "$full_capacity_report_sha" --arg profile_sha "$full_capacity_profile_sha" --arg manifest_sha "$full_capacity_manifest_sha" \
		--arg stream_sha "$full_capacity_stream_sha" --arg stdout_sha "$full_capacity_stdout_sha" --arg stderr_sha "$full_capacity_stderr_sha" \
		--argjson seed "$v2_r2_sv1d_capacity_workload_seed" --argjson event_count "$v2_r2_sv1d_capacity_event_count" \
		--argjson book_events "$v2_r2_sv1d_capacity_book_delta_events" --argjson balance_events "$v2_r2_sv1d_capacity_balance_change_events" \
		--argjson opaque_events "$v2_r2_sv1d_capacity_opaque_events" --argjson start "$v2_r2_sv1d_capacity_workload_start_nano" \
		--argjson end "$v2_r2_sv1d_capacity_workload_end_nano" --argjson stream_bytes "$full_capacity_stream_bytes" \
		--argjson peak_output "$full_capacity_peak_output" --argjson peak_rss 1048576 --argjson safety_margin "$full_capacity_safety_margin" \
		--argjson required_free "$full_capacity_required_free" --argjson available_free "$full_capacity_free" \
		--argjson minimum_free "$full_capacity_minimum_free" --argjson memory_available "$full_capacity_memory_available" \
		--argjson minimum_memory "$full_capacity_minimum_memory" --argjson host_memory_total "$full_capacity_host_memory_total" \
		--argjson host_cpu "$full_capacity_host_cpu" --argjson allowed_cpu "$full_capacity_allowed_cpu" --arg affinity "$full_capacity_affinity" \
		--argjson gomaxprocs "$v2_r2_sv1d_capacity_gomaxprocs" --argjson memory_limit "$v2_r2_sv1d_capacity_memory_limit_bytes" \
		--argjson gomemlimit "$v2_r2_sv1d_capacity_gomemlimit_bytes" --argjson cpu_limit "$v2_r2_sv1_cpu_limit_percent" \
		--argjson max_wall "$v2_r2_sv1d_capacity_max_wall_seconds" \
		'{schema_version:1,contract:$contract,measurement:"synthetic_24h_binary_evidence_capacity_probe",capacity_only:true,
		 outcome_neutral:true,simulator_invoked:false,terminal_outcome_present:false,holdouts_consumed:false,
		 source_revision:$revision,source_tree_sha256:$tree,evidence_format:"evstream_v3",hashing:"route_and_global_sequence_neutral_v2",ordering:"ordered_stream",
		 capacity_binary:{path:$binary_path,sha256:$binary_sha},target_config:{path:$config_path,sha256:$config_sha},
		 review:{path:$review_path,sha256:$review_sha},probe_root:$probe_root,probe_cell:$probe_cell,
		 workload:{profile:$profile,seed:$seed,event_count:$event_count,book_delta_events:$book_events,balance_change_events:$balance_events,
		 opaque_scientific_events:$opaque_events,start_nano:$start,end_nano:$end,horizon:"24h"},
		 report_sha256:$report_sha,profile_sha256:$profile_sha,evidence_manifest_sha256:$manifest_sha,stream_sha256:$stream_sha,stream_bytes:$stream_bytes,
		 stdout_sha256:$stdout_sha,stderr_sha256:$stderr_sha,peak_output_bytes:$peak_output,peak_rss_bytes:$peak_rss,
		 safety_margin_bytes:$safety_margin,required_free_bytes:$required_free,available_free_bytes:$available_free,
		 initial_available_free_bytes:$available_free,minimum_free_bytes:$minimum_free,initial_memory_available_bytes:$memory_available,
		 final_memory_available_bytes:$memory_available,wall_clock_seconds:1,
		 resource_policy:{gomaxprocs:$gomaxprocs,memory_limit_bytes:$memory_limit,gomemlimit_bytes:$gomemlimit,cpu_limit_percent:$cpu_limit,
		 minimum_free_bytes:$minimum_free,minimum_memory_available_bytes:$minimum_memory,host_memory_total_bytes:$host_memory_total,
		 host_cpu_count:$host_cpu,allowed_cpu_count:$allowed_cpu,cpu_affinity:$affinity,max_wall_seconds:$max_wall}}' \
		>"$full_capacity_attestation"

	v2_r2_sv1d_capacity_binary="$full_capacity_binary"
	v2_r2_sv1d_require_pinned_binary() { return 0; }
	v2_r2_sv1d_capacity_probe_root() { printf '%s\n' "$full_capacity_root"; }
	v2_r2_sv1d_require_capacity_attestation "$full_capacity_attestation" "$full_capacity_revision" \
		"$full_capacity_binary_sha" "$full_capacity_config_sha" "$full_capacity_review" "$full_capacity_review_sha" || {
		echo "valid full capacity attestation fixture was rejected" >&2
		exit 1
	}
	jq '.stream_bytes += 1' "$full_capacity_attestation" >"$temp_root/invalid-full-capacity-attestation.json"
	if v2_r2_sv1d_require_capacity_attestation "$temp_root/invalid-full-capacity-attestation.json" "$full_capacity_revision" \
		"$full_capacity_binary_sha" "$full_capacity_config_sha" "$full_capacity_review" "$full_capacity_review_sha"; then
		echo "mutated full capacity attestation was accepted" >&2
		exit 1
	fi
)

capacity_root_fixture="$temp_root/capacity-root"
mkdir -p -- "$capacity_root_fixture/$(v2_r2_sv1d_capacity_probe_cell)"
v2_r2_sv1d_capacity_require_root "$capacity_root_fixture" "$(v2_r2_sv1d_capacity_probe_cell)" || {
	echo "closed capacity root fixture was rejected" >&2
	exit 1
}
printf '%s\n' 'unexpected' >"$capacity_root_fixture/unexpected.log"
if v2_r2_sv1d_capacity_require_root "$capacity_root_fixture" "$(v2_r2_sv1d_capacity_probe_cell)"; then
	echo "capacity root sibling mutation was accepted" >&2
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
terminal_classification=$(v2_r2_sv1d_classify_comparison "$terminal_comparison" 1)
IFS=$'\t' read -r terminal_score_status terminal_score_reason <<<"$terminal_classification"
[[ "$terminal_score_status" == SV1D_ACTIVATION_NOT_SATISFIED_TERMINAL_FAILURE &&
	"$terminal_score_reason" == "the registered treatment/control pair reached a valid terminal valuation failure; no activation claim is made" ]] || {
	echo "valid terminal comparison was not classified as a valid negative outcome" >&2
	exit 1
}
jq '.control_run_status_sha256 = ("0" * 64)' "$terminal_comparison" >"$temp_root/terminal-comparison-bad-hash.json"
if v2_r2_sv1d_require_comparison_provenance "$temp_root/terminal-comparison-bad-hash.json" "$(printf '%064d' 0)" "$(printf '%040d' 0)" "$terminal_treatment" "$terminal_control"; then
	echo "terminal comparison hash mutation was accepted" >&2
	exit 1
fi

write_scoring_provenance_fixture() {
	[[ $# -eq 4 ]] || return 1
	local case_name=$1 comparison_path=$2 expected_status=$3 expected_activation=$4
	local provenance_path="$temp_root/$case_name-provenance.json" output_root="$temp_root/$case_name-output"
	local comparison_sha256 comparison_valid comparison_evidence_valid comparison_anticheating comparison_activation comparison_terminal_negative
	comparison_sha256=$(v2_r2_sv1d_sha256_file "$comparison_path") || return 1
	comparison_valid=$(jq -r 'if ((.valid | type) == "boolean" and .valid) then "true" else "false" end' "$comparison_path") || return 1
	comparison_evidence_valid=$(jq -r 'if ((.evidence_valid | type) == "boolean" and .evidence_valid) then "true" else "false" end' "$comparison_path") || return 1
	comparison_anticheating=$(jq -r 'if ((.anti_cheating_satisfied | type) == "boolean" and .anti_cheating_satisfied) then "true" else "false" end' "$comparison_path") || return 1
	comparison_activation=$(jq -r 'if ((.activation_satisfied | type) == "boolean" and .activation_satisfied) then "true" else "false" end' "$comparison_path") || return 1
	comparison_terminal_negative=$(jq -r 'if .status == "UNAVAILABLE_TERMINAL_FAILURE" then "true" else "false" end' "$comparison_path") || return 1
	jq -n --arg output_root "$output_root" --arg status "$expected_status" --arg comparison_path "$comparison_path" \
		--arg comparison_sha256 "$comparison_sha256" --argjson activation "$expected_activation" \
		--argjson valid "$comparison_valid" --argjson evidence_valid "$comparison_evidence_valid" \
		--argjson anti_cheating "$comparison_anticheating" --argjson comparison_activation "$comparison_activation" \
		--argjson terminal_negative "$comparison_terminal_negative" \
		'{schema_version:1,output_root:$output_root,status:$status,activation_satisfied:$activation,
		 comparison:{path:$comparison_path,recorded_path:$comparison_path,sha256:$comparison_sha256,
		   exit_status:0,object_valid:true,valid:$valid,evidence_valid:$evidence_valid,
		   anti_cheating_satisfied:$anti_cheating,activation_satisfied:$comparison_activation,
		   terminal_negative:$terminal_negative}}' >"$provenance_path"
	printf '%s\n' "$provenance_path"
}

accepted_comparison="$temp_root/accepted-comparison.json"
jq -n '
	def supplier($active):
		{valid:true,evidence_valid:true,anti_cheating_satisfied:true,
		 configured_max_position:100,max_position:50,min_position:-50,
		 configured_max_inventory:100,max_gross_base_balance:50,max_inventory_utilization:0.5,
		 configured_max_quote_qty:100,max_quote_qty:50,max_borrowed:0,
		 configured_minimum_qualifying_qty:10,filled_qty:(if $active then 10 else 0 end),
		 fill_caused_risk_transition:$active,fill_count:(if $active then 1 else 0 end),
		 trading_pnl:(if $active then 1 else 0 end),
		 inventory_responsive_decision_count:(if $active then 1 else 0 end)};
	def venue:
		{supplier_removal_counterfactual_valid:true,
		 supplier_removal_time_weighted_counterfactual_valid:true,snapshot_count:1,
		 supplier_removal_snapshot_count:1,supplier_removal_observed_duration_ns:1,
		 supplier_depth_over_75_active_time_fraction:0.1,
		 supplier_bid_depth_over_75_active_time_fraction:0.1,
		 supplier_ask_depth_over_75_active_time_fraction:0.1,
		 supplier_bid_time_weighted_resting_depth_share:0.1,
		 supplier_ask_time_weighted_resting_depth_share:0.1,
		 supplier_only_bid_time_weighted_fraction:0.1,
		 supplier_only_ask_time_weighted_fraction:0.1,
		 supplier_removal_qualified_bid_absence_active_time_fraction:0.1,
		 supplier_removal_qualified_ask_absence_active_time_fraction:0.1};
	def run($active):
		{valid:true,evidence_valid:true,anti_cheating_satisfied:true,supplier_count:1,snapshot_count:1,
		 supplier_removal_counterfactual_valid:true,
		 supplier_removal_time_weighted_counterfactual_valid:true,supplier_removal_snapshot_count:1,
		 supplier_removal_observed_duration_ns:1,
		 supplier_volume_share:0.1,
		 supplier_depth_over_75_active_time_fraction:0.1,
		 supplier_bid_depth_over_75_active_time_fraction:0.1,
		 supplier_ask_depth_over_75_active_time_fraction:0.1,
		 supplier_bid_time_weighted_resting_depth_share:0.1,
		 supplier_ask_time_weighted_resting_depth_share:0.1,
		 supplier_only_bid_time_weighted_fraction:0.1,
		 supplier_only_ask_time_weighted_fraction:0.1,
		 supplier_removal_qualified_bid_absence_active_time_fraction:0.1,
		 supplier_removal_qualified_ask_absence_active_time_fraction:0.1,
		 suppliers:[supplier($active)],venues:[venue,venue,venue],activation_satisfied:$active};
	{schema_version:1,status:"COMPLETED",valid:true,evidence_valid:true,activation_satisfied:true,
	 anti_cheating_satisfied:true,liquidation_evidence_valid:true,provenance:{valid:true},
	 survival_effect_satisfied:true,
	 treatment:(run(true)+{one_sided_decision_count:1,one_sided_missing_side_accepted_count:1,
		 one_sided_restoration_count:1}),control:(run(false)+{one_sided_decision_count:0})}' \
	>"$accepted_comparison"

assert_scoring_fixture() {
	[[ $# -eq 6 ]] || return 1
	local case_name=$1 comparison_path=$2 expected_classification=$3 expected_status=$4 expected_activation=$5 expected_supplier_count=$6
	local provenance_path classification score_status score_reason
	provenance_path=$(write_scoring_provenance_fixture "$case_name" "$comparison_path" "$expected_status" "$expected_activation") || return 1
	v2_r2_sv1d_require_scoring_comparison_claims "$provenance_path" "$temp_root/$case_name-output" "$comparison_path" \
		"$(v2_r2_sv1d_sha256_file "$comparison_path")" "$expected_status" "$expected_activation" \
		"$(jq -r 'if ((.valid | type) == "boolean" and .valid) then "true" else "false" end' "$comparison_path")" \
		"$(jq -r 'if ((.evidence_valid | type) == "boolean" and .evidence_valid) then "true" else "false" end' "$comparison_path")" \
		"$(jq -r 'if ((.anti_cheating_satisfied | type) == "boolean" and .anti_cheating_satisfied) then "true" else "false" end' "$comparison_path")" \
		"$(jq -r 'if ((.activation_satisfied | type) == "boolean" and .activation_satisfied) then "true" else "false" end' "$comparison_path")" \
		"$(jq -r 'if .status == "UNAVAILABLE_TERMINAL_FAILURE" then "true" else "false" end' "$comparison_path")" || return 1
	classification=$(v2_r2_sv1d_classify_comparison "$comparison_path" "$expected_supplier_count") || return 1
	IFS=$'\t' read -r score_status score_reason <<<"$classification"
	[[ "$score_status" == "$expected_classification" ]] || return 1
	printf '✓ runner/scorer provenance boundary: %s\n' "$case_name"
}

assert_scoring_fixture accepted "$accepted_comparison" SV1D_ACTIVATION_ACCEPTED ACTIVATION_CONTRACT_SATISFIED true 1
jq '.activation_satisfied=false | .survival_effect_satisfied=false' "$accepted_comparison" >"$temp_root/ordinary-negative-comparison.json"
assert_scoring_fixture ordinary-negative "$temp_root/ordinary-negative-comparison.json" \
	SV1D_ACTIVATION_NOT_SATISFIED ACTIVATION_CONTRACT_NOT_SATISFIED false 1
jq '.anti_cheating_satisfied=false' "$accepted_comparison" >"$temp_root/anti-cheating-comparison.json"
assert_scoring_fixture anti-cheating-negative "$temp_root/anti-cheating-comparison.json" \
	SV1D_ACTIVATION_REJECTED_ANTI_CHEATING ACTIVATION_CONTRACT_NOT_SATISFIED false 1
assert_scoring_fixture terminal-negative "$terminal_comparison" \
	SV1D_ACTIVATION_NOT_SATISFIED_TERMINAL_FAILURE ACTIVATION_CONTRACT_NOT_SATISFIED false 1

accepted_provenance=$(write_scoring_provenance_fixture accepted-mutation "$accepted_comparison" ACTIVATION_CONTRACT_SATISFIED true)
jq '.comparison.evidence_valid=false' "$accepted_provenance" >"$temp_root/accepted-provenance-bad-evidence.json"
if v2_r2_sv1d_require_scoring_comparison_claims "$temp_root/accepted-provenance-bad-evidence.json" "$temp_root/accepted-mutation-output" \
	"$accepted_comparison" "$(v2_r2_sv1d_sha256_file "$accepted_comparison")" ACTIVATION_CONTRACT_SATISFIED true true true true true false; then
	echo "scoring provenance evidence mutation was accepted" >&2
	exit 1
fi
jq '.status="ACTIVATION_CONTRACT_NOT_SATISFIED"' "$accepted_provenance" >"$temp_root/accepted-provenance-bad-status.json"
if v2_r2_sv1d_require_scoring_comparison_claims "$temp_root/accepted-provenance-bad-status.json" "$temp_root/accepted-mutation-output" \
	"$accepted_comparison" "$(v2_r2_sv1d_sha256_file "$accepted_comparison")" ACTIVATION_CONTRACT_SATISFIED true true true true true true false; then
	echo "scoring provenance status mutation was accepted" >&2
	exit 1
fi

echo "SV1D activation config contract: PASS"
