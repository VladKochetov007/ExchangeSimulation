#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
runner="$root_dir/scripts/run-v2-r2-sv1-activation-probe.sh"
cell_runner="$root_dir/scripts/run-v2-r2-sv1-24h-cell.sh"
source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"
temp_root=$(mktemp -d)
trap 'rm -rf -- "$temp_root"' EXIT

rg -F 'cmp -s -- "$config" "$arm/run-config.json"' "$runner" >/dev/null || {
	echo "activation runner does not enforce byte-identical registered config" >&2
	exit 1
}
assert_rejected() {
	local output_root=$1
	local stdout_log="$temp_root/runner.stdout" stderr_log="$temp_root/runner.stderr"
	if V2_R2_SV1_ACTIVATION_ROOT="$output_root" "$runner" >"$stdout_log" 2>"$stderr_log"; then
		echo "activation runner accepted forbidden output root: $output_root" >&2
		cat "$stderr_log" >&2
		exit 1
	fi
}

assert_rejected "$root_dir/research/probe"
ln -s -- "$root_dir" "$temp_root/repository-link"
assert_rejected "$temp_root/repository-link/probe"

cdf_audit_fixture="$temp_root/cdfliquidity.json"
jq -n ' {
		run: "contract-fixture",
		result: {
			valid: true, evidence_valid: true, activation_satisfied: true, anti_cheating_satisfied: true,
			supplier_count: 2, decision_count: 4, fill_count: 2,
			trading_supplier_count: 2, pnl_changing_supplier_count: 2,
			inventory_responsive_decision_count: 4, cancel_count: 1, withdraw_count: 1,
			withdrawal_without_replacement_count: 2,
			max_borrowed: 0, snapshot_count: 1, supplier_volume_share: 0.2, supplier_depth_over_75_share: 0.1,
			supplier_depth_over_75_active_time_fraction: 0.1,
			supplier_bid_depth_over_75_active_time_fraction: 0.1,
			supplier_ask_depth_over_75_active_time_fraction: 0.1,
			risk_state_decision_count: 2,
			fresh_risk_state_decision_count: 2,
			supplier_time_weighted_resting_depth_share: 0.2,
			supplier_bid_time_weighted_resting_depth_share: 0.2,
			supplier_ask_time_weighted_resting_depth_share: 0.2,
			supplier_only_bid_time_weighted_fraction: 0.1,
			supplier_only_ask_time_weighted_fraction: 0.1,
			supplier_removal_counterfactual_valid: true, supplier_removal_time_weighted_counterfactual_valid: true, supplier_removal_snapshot_count: 1,
			supplier_removal_observed_duration_ns: 1, supplier_removal_bid_absence_duration_ns: 0, supplier_removal_ask_absence_duration_ns: 0,
			supplier_removal_qualified_bid_absence_duration_ns: 0, supplier_removal_qualified_ask_absence_duration_ns: 0,
			supplier_removal_bid_absence_fraction: 0, supplier_removal_ask_absence_fraction: 0,
			supplier_removal_bid_absence_active_time_fraction: 0, supplier_removal_ask_absence_active_time_fraction: 0,
			supplier_removal_qualified_bid_absence_active_time_fraction: 0, supplier_removal_qualified_ask_absence_active_time_fraction: 0,
			venues: [
				{snapshot_count: 1, supplier_depth_over_75_fraction: 0.1,
					supplier_depth_over_75_active_time_fraction: 0.1,
					supplier_bid_depth_over_75_active_time_fraction: 0.1,
					supplier_ask_depth_over_75_active_time_fraction: 0.1,
					supplier_bid_depth_over_75_fraction: 0.1, supplier_ask_depth_over_75_fraction: 0.1,
					supplier_bid_time_weighted_resting_depth_share: 0.2, supplier_ask_time_weighted_resting_depth_share: 0.2,
					supplier_only_bid_time_weighted_fraction: 0.1, supplier_only_ask_time_weighted_fraction: 0.1,
					supplier_removal_counterfactual_valid: true, supplier_removal_time_weighted_counterfactual_valid: true, supplier_removal_snapshot_count: 1,
					supplier_removal_observed_duration_ns: 1, supplier_removal_bid_absence_duration_ns: 0, supplier_removal_ask_absence_duration_ns: 0,
					supplier_removal_qualified_bid_absence_duration_ns: 0, supplier_removal_qualified_ask_absence_duration_ns: 0,
					supplier_removal_bid_absence_fraction: 0, supplier_removal_ask_absence_fraction: 0},
				{snapshot_count: 1, supplier_depth_over_75_fraction: 0.1,
					supplier_depth_over_75_active_time_fraction: 0.1,
					supplier_bid_depth_over_75_active_time_fraction: 0.1,
					supplier_ask_depth_over_75_active_time_fraction: 0.1,
					supplier_bid_depth_over_75_fraction: 0.1, supplier_ask_depth_over_75_fraction: 0.1,
					supplier_bid_time_weighted_resting_depth_share: 0.2, supplier_ask_time_weighted_resting_depth_share: 0.2,
					supplier_only_bid_time_weighted_fraction: 0.1, supplier_only_ask_time_weighted_fraction: 0.1,
					supplier_removal_counterfactual_valid: true, supplier_removal_time_weighted_counterfactual_valid: true, supplier_removal_snapshot_count: 1,
					supplier_removal_observed_duration_ns: 1, supplier_removal_bid_absence_duration_ns: 0, supplier_removal_ask_absence_duration_ns: 0,
					supplier_removal_qualified_bid_absence_duration_ns: 0, supplier_removal_qualified_ask_absence_duration_ns: 0,
					supplier_removal_bid_absence_fraction: 0, supplier_removal_ask_absence_fraction: 0},
				{snapshot_count: 1, supplier_depth_over_75_fraction: 0.1,
					supplier_depth_over_75_active_time_fraction: 0.1,
					supplier_bid_depth_over_75_active_time_fraction: 0.1,
					supplier_ask_depth_over_75_active_time_fraction: 0.1,
					supplier_bid_depth_over_75_fraction: 0.1, supplier_ask_depth_over_75_fraction: 0.1,
					supplier_bid_time_weighted_resting_depth_share: 0.2, supplier_ask_time_weighted_resting_depth_share: 0.2,
					supplier_only_bid_time_weighted_fraction: 0.1, supplier_only_ask_time_weighted_fraction: 0.1,
					supplier_removal_counterfactual_valid: true, supplier_removal_time_weighted_counterfactual_valid: true, supplier_removal_snapshot_count: 1,
					supplier_removal_observed_duration_ns: 1, supplier_removal_bid_absence_duration_ns: 0, supplier_removal_ask_absence_duration_ns: 0,
					supplier_removal_qualified_bid_absence_duration_ns: 0, supplier_removal_qualified_ask_absence_duration_ns: 0,
					supplier_removal_bid_absence_fraction: 0, supplier_removal_ask_absence_fraction: 0}
			],
			suppliers: [
				{valid: true, evidence_valid: true, activation_satisfied: true, anti_cheating_satisfied: true,
				 fill_caused_risk_transition: true, trading_pnl: 1, fill_count: 1, pnl: 1, min_position: 0, max_position: 1,
					inventory_responsive_decision_count: 2, max_observation_age_ns: 1,
					 configured_max_loss_quote: 10, risk_state_decision_count: 1, fresh_risk_state_decision_count: 1,
				 max_borrowed: 0, borrow_event_count: 0, max_position: 1,
				 configured_max_position: 2, max_quote_qty: 1, configured_max_quote_qty: 2},
				{valid: true, evidence_valid: true, activation_satisfied: true, anti_cheating_satisfied: true,
				 fill_caused_risk_transition: true, trading_pnl: -1, fill_count: 1, pnl: -1, min_position: -1, max_position: 0,
					inventory_responsive_decision_count: 2, max_observation_age_ns: 1,
					 configured_max_loss_quote: 10, risk_state_decision_count: 1, fresh_risk_state_decision_count: 1,
				 max_borrowed: 0, borrow_event_count: 0, max_position: 1,
				 configured_max_position: 2, max_quote_qty: 1, configured_max_quote_qty: 2}
			]
		}
	}' >"$cdf_audit_fixture"
jq '
	.result.venues |= map(. + {
		supplier_removal_bid_absence_active_time_fraction: 0,
		supplier_removal_ask_absence_active_time_fraction: 0,
		supplier_removal_qualified_bid_absence_active_time_fraction: 0,
		supplier_removal_qualified_ask_absence_active_time_fraction: 0
	})' "$cdf_audit_fixture" >"$temp_root/cdfliquidity-with-time-weighted-removal.json"
mv -- "$temp_root/cdfliquidity-with-time-weighted-removal.json" "$cdf_audit_fixture"
v2_r2_require_cdf_supplier_activation "$cdf_audit_fixture" 2 || {
	echo "valid CDF activity without borrowing was rejected" >&2
	exit 1
}
jq '.result.suppliers[0].min_position = -3' "$cdf_audit_fixture" >"$temp_root/negative-inventory.json"
if v2_r2_require_cdf_supplier_activation "$temp_root/negative-inventory.json" 2; then
	echo "supplier below its configured lower inventory bound was accepted" >&2
	exit 1
fi
if jq '.result.suppliers[0].borrow_event_count = 1' "$cdf_audit_fixture" >"$temp_root/borrowed.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/borrowed.json" 2; then
	echo "unregistered CDF borrowing was accepted" >&2
	exit 1
fi
if jq '.result.pnl_changing_supplier_count = 0' "$cdf_audit_fixture" >"$temp_root/no-pnl.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/no-pnl.json" 2; then
	echo "non-PnL CDF activity was accepted" >&2
	exit 1
fi
if jq '.result.pnl_changing_supplier_count = 2 |
	.result.suppliers |= map(.pnl = 100 | .trading_pnl = 0)' "$cdf_audit_fixture" >"$temp_root/endowment-revaluation-only.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/endowment-revaluation-only.json" 2; then
	echo "endowment-only CDF revaluation was accepted as trading activation" >&2
	exit 1
fi
for field in evidence_valid activation_satisfied anti_cheating_satisfied; do
	if jq --arg field "$field" '.result[$field] = false' "$cdf_audit_fixture" >"$temp_root/missing-$field.json" &&
		v2_r2_require_cdf_supplier_activation "$temp_root/missing-$field.json" 2; then
		echo "CDF activity with $field=false was accepted" >&2
		exit 1
	fi
done

jq '.result.risk_state_decision_count = 2 |
	.result.suppliers |= map(. + {configured_max_loss_quote: 10, risk_state_decision_count: 1})' \
	"$cdf_audit_fixture" >"$temp_root/marked-risk.json"
v2_r2_require_cdf_supplier_activation "$temp_root/marked-risk.json" 2 || {
	echo "complete marked-risk CDF activity was rejected" >&2
	exit 1
}
if jq '.result.risk_state_decision_count = 0' "$temp_root/marked-risk.json" >"$temp_root/missing-marked-risk.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/missing-marked-risk.json" 2; then
	echo "positive-budget CDF activity without aggregate marked-risk state was accepted" >&2
	exit 1
fi

source "$root_dir/scripts/v2-r2-sv1b-24h-contract.sh"
for required_guard in 'terminate_simulator()' 'kill -KILL' 'final free-space measurement failed after the simulator exited'; do
	rg -F "$required_guard" "$cell_runner" >/dev/null || {
		echo "SV1 cell runner is missing required resource guard: $required_guard" >&2
		exit 1
	}
done
IFS=$'\t' read -r test_host_cpu_count test_allowed_cpu_count test_cpu_affinity < <(v2_r2_sv1b_cpu_policy)
[[ "$test_host_cpu_count" =~ ^[1-9][0-9]*$ && "$test_allowed_cpu_count" =~ ^[1-9][0-9]*$ && "$test_cpu_affinity" =~ ^0-[0-9]+$ ]] || {
	echo "SV1B CPU policy is not a bounded affinity range" >&2
	exit 1
}
(( test_allowed_cpu_count * 100 <= test_host_cpu_count * v2_r2_sv1_cpu_limit_percent )) || {
	echo "SV1B CPU affinity exceeds its registered CPU ceiling" >&2
	exit 1
}
command -v taskset >/dev/null 2>&1 || {
	echo "taskset is required by the SV1B resource contract" >&2
	exit 1
}
v2_r2_require_cdf_supplier_activation "$cdf_audit_fixture" 2 || {
	echo "SV1B activity with a qualified withdrawal was rejected" >&2
	exit 1
}
if jq '.result.suppliers |= map(del(.configured_max_loss_quote, .risk_state_decision_count))' "$cdf_audit_fixture" >"$temp_root/missing-loss-budget.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/missing-loss-budget.json" 2; then
	echo "SV1B activity without a positive loss budget was accepted" >&2
	exit 1
fi
if jq '.result.supplier_removal_counterfactual_valid = false' "$cdf_audit_fixture" >"$temp_root/missing-removal-diagnostic.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/missing-removal-diagnostic.json" 2; then
	echo "SV1B activity without a valid supplier-removal diagnostic was accepted" >&2
	exit 1
fi
if jq '.result.supplier_time_weighted_resting_depth_share = 0.8' "$cdf_audit_fixture" >"$temp_root/dominant-depth.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/dominant-depth.json" 2; then
	echo "SV1B activity with dominant time-weighted supplier depth was accepted" >&2
	exit 1
fi
if jq 'del(.result.withdrawal_without_replacement_count)' "$cdf_audit_fixture" >"$temp_root/missing-withdrawal.json" &&
	v2_r2_require_cdf_supplier_activation "$temp_root/missing-withdrawal.json" 2; then
	echo "SV1B activity without a qualified withdrawal was accepted" >&2
	exit 1
fi

review_revision=$(git -C "$root_dir" rev-parse HEAD)
review_tree_sha256=$(v2_r2_sv1b_git_tree_sha256 "$review_revision")
review_report="$temp_root/tree-review.md"
printf '%s\n' 'independent exact-tree review fixture' >"$review_report"
review_report_sha256=$(sha256sum -- "$review_report" | awk '{print $1}')
review_attestation="$temp_root/tree-review-attestation.json"
jq -n --arg revision "$review_revision" --arg tree_sha256 "$review_tree_sha256" \
	--arg report_path "$review_report" --arg report_sha256 "$review_report_sha256" \
	--arg contract "$v2_r2_sv1_review_contract" --argjson reviewed_scope "$v2_r2_sv1_review_scope" \
	'{schema_version:1,contract:$contract,reviewed_revision:$revision,reviewed_tree_sha256:$tree_sha256,
	 review_type:"independent_sol_xhigh",verdict:"ACCEPTED_FOR_ACTIVATION",reviewed_worktree_clean:true,
	 holdouts_consumed:false,reviewer:"fixture-reviewer",reviewed_scope:$reviewed_scope,
	 review_report_path:$report_path,review_report_sha256:$report_sha256}' >"$review_attestation"
v2_r2_require_sv1b_review_attestation "$review_attestation" "$review_revision" || {
	echo "complete exact-tree review attestation fixture was rejected" >&2
	exit 1
}
jq '.reviewed_scope |= map(select(. != "cdf_supplier"))' "$review_attestation" >"$temp_root/tree-review-missing-scope.json"
if v2_r2_require_sv1b_review_attestation "$temp_root/tree-review-missing-scope.json" "$review_revision"; then
	echo "review attestation missing CDF scope was accepted" >&2
	exit 1
fi

activation_provenance_fixture="$temp_root/activation-provenance.json"
jq -n '{schema_version:3,status:"ACTIVATION_CONTRACT_SATISFIED",activation_satisfied:true,
	 holdouts_consumed:false,treatment_runner_status:0,control_runner_status:0,
	 treatment_terminal_status:"completed",control_terminal_status:"completed"}' >"$activation_provenance_fixture"
activation_review_report="$temp_root/activation-review.md"
printf '%s\n' 'independent activation-evidence review fixture' >"$activation_review_report"
activation_review_report_sha256=$(sha256sum -- "$activation_review_report" | awk '{print $1}')
activation_review_attestation="$temp_root/activation-review-attestation.json"
jq -n --arg revision "$review_revision" --arg tree_sha256 "$review_tree_sha256" \
	--arg activation_path "$activation_provenance_fixture" \
	--arg activation_sha256 "$(sha256sum -- "$activation_provenance_fixture" | awk '{print $1}')" \
	--arg report_path "$activation_review_report" --arg report_sha256 "$activation_review_report_sha256" \
	--arg contract "$v2_r2_sv1_activation_review_contract" --argjson reviewed_scope "$v2_r2_sv1_activation_review_scope" \
	'{schema_version:1,contract:$contract,reviewed_revision:$revision,reviewed_tree_sha256:$tree_sha256,
	 review_type:"independent_sol_xhigh",verdict:"ACCEPTED_FOR_CAPACITY",reviewed_worktree_clean:true,
	 holdouts_consumed:false,reviewer:"fixture-activation-reviewer",reviewed_scope:$reviewed_scope,
	 activation_provenance_path:$activation_path,activation_provenance_sha256:$activation_sha256,
	 review_report_path:$report_path,review_report_sha256:$report_sha256}' >"$activation_review_attestation"
v2_r2_require_sv1b_activation_review_attestation "$activation_review_attestation" "$review_revision" "$activation_provenance_fixture" || {
	echo "complete post-activation review attestation fixture was rejected" >&2
	exit 1
}
jq '.verdict = "ACCEPTED_FOR_ACTIVATION"' "$activation_review_attestation" >"$temp_root/activation-review-wrong-verdict.json"
if v2_r2_require_sv1b_activation_review_attestation "$temp_root/activation-review-wrong-verdict.json" "$review_revision" "$activation_provenance_fixture"; then
	echo "post-activation review with the pre-activation verdict was accepted" >&2
	exit 1
fi

jq -n ' {
		run: "control-fixture",
		result: {
			valid: true, evidence_valid: true, anti_cheating_satisfied: true, supplier_count: 0, decision_count: 0, fill_count: 0,
			trading_supplier_count: 0, pnl_changing_supplier_count: 0,
			inventory_responsive_decision_count: 0, cancel_count: 0, withdraw_count: 0,
			max_borrowed: 0, checks: [], venues: [
				{supplier_depth_over_75_fraction: 0},
				{supplier_depth_over_75_fraction: 0},
				{supplier_depth_over_75_fraction: 0}
			], suppliers: []
		}
	}' >"$temp_root/control-cdfliquidity.json"
v2_r2_require_cdf_supplier_control "$temp_root/control-cdfliquidity.json" || {
	echo "valid no-CDF control audit was rejected" >&2
	exit 1
}
if jq '.result.supplier_count = 1' "$temp_root/control-cdfliquidity.json" >"$temp_root/control-with-supplier.json" &&
	v2_r2_require_cdf_supplier_control "$temp_root/control-with-supplier.json"; then
	echo "CDF activity was accepted in a no-CDF control" >&2
	exit 1
fi

jq -n '{
	analysis_revision: "revision", raw_source_revision: "revision", analysis_contract: "contract",
	evidence_format: "evstream_v3", analyzer_revision: "revision", analyzer_sha256: "a",
	analyzer_vcs_modified: false, analyzer_trimpath: true, analyzer_cgo_enabled: "0", analyzer_go_version: "go1.27.0",
	renderer_revision: "revision", renderer_sha256: "b", renderer_go_version: "go1.27.0", renderer_route_compression: "none",
	simulator_revision: "revision", simulator_sha256: "c", simulator_trimpath: true, simulator_cgo_enabled: "0", simulator_go_version: "go1.27.0",
	prunegate_revision: "revision", prunegate_sha256: "d", prunegate_trimpath: true, prunegate_cgo_enabled: "0", prunegate_go_version: "go1.27.0"
}' >"$temp_root/provenance-a.json"
cp -- "$temp_root/provenance-a.json" "$temp_root/provenance-b.json"
v2_r2_require_campaign_provenance_match "$temp_root/provenance-a.json" "$temp_root/provenance-b.json" || {
	echo "identical campaign provenance was rejected" >&2
	exit 1
}
if jq '.simulator_sha256 = "different"' "$temp_root/provenance-b.json" >"$temp_root/mixed-provenance.json" &&
	v2_r2_require_campaign_provenance_match "$temp_root/provenance-a.json" "$temp_root/mixed-provenance.json"; then
	echo "mixed simulator provenance was accepted" >&2
	exit 1
fi

jq -n \
	--argjson treatment "$(jq '.result' "$cdf_audit_fixture")" \
	--argjson control "$(jq '.result' "$temp_root/control-cdfliquidity.json")" \
	'{valid: true, evidence_valid: true, activation_satisfied: true, anti_cheating_satisfied: true,
	 provenance: {valid: true}, treatment: $treatment, control: $control}' \
	>"$temp_root/comparison-cdfliquidity.json"
v2_r2_require_cdf_supplier_comparison "$temp_root/comparison-cdfliquidity.json" 2 || {
	echo "valid top-level treatment/control CDF comparison was rejected" >&2
	exit 1
}
if jq '.treatment.pnl_changing_supplier_count = 0' "$temp_root/comparison-cdfliquidity.json" >"$temp_root/comparison-no-pnl.json" &&
	v2_r2_require_cdf_supplier_comparison "$temp_root/comparison-no-pnl.json" 2; then
	echo "top-level comparison accepted non-PnL treatment activity" >&2
	exit 1
fi
if jq '.treatment.venues[0].supplier_bid_depth_over_75_active_time_fraction = 1' "$temp_root/comparison-cdfliquidity.json" >"$temp_root/comparison-one-venue-dominance.json" &&
	v2_r2_require_cdf_supplier_comparison "$temp_root/comparison-one-venue-dominance.json" 2; then
	echo "comparison accepted one-venue supplier dominance hidden by aggregate metrics" >&2
	exit 1
fi
if jq '.control.supplier_count = 1' "$temp_root/comparison-cdfliquidity.json" >"$temp_root/comparison-control-activity.json" &&
	v2_r2_require_cdf_supplier_comparison "$temp_root/comparison-control-activity.json" 2; then
	echo "top-level comparison accepted CDF control activity" >&2
	exit 1
fi

# Generate the comparison with the real analysis producer, then pass that
# serialized output through the complete SV1B activation-provenance validator.
# The reduced fixture above protects the comparison predicate; this fixture
# protects the producer-to-provenance boundary that consumes its JSON schema.
serialized_comparison="$temp_root/serialized-cdf-comparison.json"
EXSIM_CDF_COMPARISON_OUTPUT="$serialized_comparison" GOMAXPROCS=2 \
	go test -count=1 ./analysis -run '^TestCDFLiquidityComparisonSerializesContractFixture$' >/dev/null || {
	echo "analysis could not produce the serialized CDF comparison fixture" >&2
	exit 1
}
jq -e 'type == "object" and .valid == true and .evidence_valid == true and .activation_satisfied == true and .anti_cheating_satisfied == true' \
	"$serialized_comparison" >/dev/null || {
	echo "analysis serialized CDF comparison is missing an accepted pair predicate" >&2
	exit 1
}
jq -e --argjson expected_seed 607 -f "$root_dir/scripts/v2-r2-sv1-cdf-comparison-identity.jq" \
	"$serialized_comparison" >/dev/null || {
	echo "development scorer seed identity rejected the real serialized CDF comparison" >&2
	exit 1
}
if jq '.provenance.treatment.seed = 608' "$serialized_comparison" >"$temp_root/comparison-wrong-paired-seed.json" &&
	jq -e --argjson expected_seed 607 -f "$root_dir/scripts/v2-r2-sv1-cdf-comparison-identity.jq" \
		"$temp_root/comparison-wrong-paired-seed.json" >/dev/null; then
	echo "development scorer seed identity accepted a mismatched treatment seed" >&2
	exit 1
fi

full_output_root="$temp_root/full-activation-output"
mkdir -p -- "$full_output_root/treatment" "$full_output_root/control"
cp -- "$v2_r2_sv1_activation_config" "$full_output_root/treatment/run-config.json"
cp -- "$v2_r2_sv1_activation_control_config" "$full_output_root/control/run-config.json"
for arm in treatment control; do
	jq -n --arg arm "$arm" '{arm:$arm,exit_status:0,completion_verified:true,
		terminal_failure_verified:false,terminal_outcome_status:"completed",
		resource_guard_failed:false}' >"$full_output_root/$arm/run-status.json"
done
cp -- "$serialized_comparison" "$full_output_root/cdf-liquidity-comparison.json"
fixture_true_binary=$(type -P true) || {
	echo "could not locate an executable true binary for the provenance fixture" >&2
	exit 1
}
cp -- "$fixture_true_binary" "$temp_root/sv1b-simulator"
cp -- "$fixture_true_binary" "$temp_root/sv1b-analyzer"
chmod 0755 -- "$temp_root/sv1b-simulator" "$temp_root/sv1b-analyzer"
fixture_simulator_sha256=$(sha256sum -- "$temp_root/sv1b-simulator" | awk '{print $1}')
fixture_analyzer_sha256=$(sha256sum -- "$temp_root/sv1b-analyzer" | awk '{print $1}')
fixture_treatment_config_sha256=$(sha256sum -- "$v2_r2_sv1_activation_config" | awk '{print $1}')
fixture_control_config_sha256=$(sha256sum -- "$v2_r2_sv1_activation_control_config" | awk '{print $1}')
fixture_treatment_status_sha256=$(sha256sum -- "$full_output_root/treatment/run-status.json" | awk '{print $1}')
fixture_control_status_sha256=$(sha256sum -- "$full_output_root/control/run-status.json" | awk '{print $1}')
fixture_comparison_sha256=$(sha256sum -- "$full_output_root/cdf-liquidity-comparison.json" | awk '{print $1}')
fixture_review_report="$temp_root/full-review.md"
printf '%s\n' 'independent exact-tree review fixture for serialized comparison' >"$fixture_review_report"
fixture_review_report_sha256=$(sha256sum -- "$fixture_review_report" | awk '{print $1}')
fixture_review="$temp_root/full-review-attestation.json"
jq -n --arg revision "$review_revision" --arg tree_sha256 "$review_tree_sha256" \
	--arg report_path "$fixture_review_report" --arg report_sha256 "$fixture_review_report_sha256" \
	--arg contract "$v2_r2_sv1_review_contract" --argjson reviewed_scope "$v2_r2_sv1_review_scope" \
	'{schema_version:1,contract:$contract,reviewed_revision:$revision,reviewed_tree_sha256:$tree_sha256,
	 review_type:"independent_sol_xhigh",verdict:"ACCEPTED_FOR_ACTIVATION",reviewed_worktree_clean:true,
	 holdouts_consumed:false,reviewer:"fixture-serialized-comparison-reviewer",reviewed_scope:$reviewed_scope,
	 review_report_path:$report_path,review_report_sha256:$report_sha256}' >"$fixture_review"
IFS=$'\t' read -r fixture_host_cpu_count fixture_allowed_cpu_count fixture_cpu_affinity < <(v2_r2_sv1b_cpu_policy)
fixture_treatment_artifacts=$(v2_r2_sv1b_artifact_records "$full_output_root/treatment")
fixture_control_artifacts=$(v2_r2_sv1b_artifact_records "$full_output_root/control")
fixture_activation_provenance="$temp_root/full-activation-provenance.json"
jq -n --arg contract "$v2_r2_sv1_activation_pair_contract" --arg revision "$review_revision" \
	--arg tree_sha256 "$review_tree_sha256" --arg output_root "$full_output_root" \
	--arg treatment_dir "$full_output_root/treatment" --arg control_dir "$full_output_root/control" \
	--arg comparison_path "$full_output_root/cdf-liquidity-comparison.json" \
	--arg review_path "$fixture_review" --arg review_sha256 "$(sha256sum -- "$fixture_review" | awk '{print $1}')" \
	--arg simulator_path "$temp_root/sv1b-simulator" --arg analyzer_path "$temp_root/sv1b-analyzer" \
	--arg simulator_sha256 "$fixture_simulator_sha256" --arg analyzer_sha256 "$fixture_analyzer_sha256" \
	--arg comparison_sha256 "$fixture_comparison_sha256" --arg treatment_source_config_path "$(realpath -e -- "$v2_r2_sv1_activation_config")" \
	--arg control_source_config_path "$(realpath -e -- "$v2_r2_sv1_activation_control_config")" \
	--arg treatment_source_config_sha256 "$fixture_treatment_config_sha256" --arg control_source_config_sha256 "$fixture_control_config_sha256" \
	--arg treatment_config_sha256 "$fixture_treatment_config_sha256" --arg control_config_sha256 "$fixture_control_config_sha256" \
	--arg treatment_status_sha256 "$fixture_treatment_status_sha256" --arg control_status_sha256 "$fixture_control_status_sha256" \
	--argjson treatment_artifacts "$fixture_treatment_artifacts" --argjson control_artifacts "$fixture_control_artifacts" \
	--argjson host_cpu_count "$fixture_host_cpu_count" --argjson allowed_cpu_count "$fixture_allowed_cpu_count" \
	--arg cpu_affinity "$fixture_cpu_affinity" \
	'{schema_version:3,contract:$contract,candidate_revision:$revision,candidate_tree_sha256:$tree_sha256,
	 seed:643,simulated_horizon:"fixture",output_root:$output_root,treatment_dir:$treatment_dir,control_dir:$control_dir,
	 treatment_source_config_path:$treatment_source_config_path,control_source_config_path:$control_source_config_path,
	 treatment_source_config_sha256:$treatment_source_config_sha256,control_source_config_sha256:$control_source_config_sha256,
	 simulator_binary_path:$simulator_path,analyzer_binary_path:$analyzer_path,
	 review_attestation_path:$review_path,review_attestation_sha256:$review_sha256,comparison_path:$comparison_path,
	 treatment_config_sha256:$treatment_config_sha256,control_config_sha256:$control_config_sha256,
	 simulator_binary_sha256:$simulator_sha256,analyzer_binary_sha256:$analyzer_sha256,comparison_sha256:$comparison_sha256,
	 status:"ACTIVATION_CONTRACT_SATISFIED",activation_satisfied:true,holdouts_consumed:false,
	 treatment_runner_status:0,control_runner_status:0,treatment_terminal_status:"completed",control_terminal_status:"completed",
	 treatment_run_status_sha256:$treatment_status_sha256,control_run_status_sha256:$control_status_sha256,
	 treatment_terminal_outcome_sha256:"",control_terminal_outcome_sha256:"",
	 treatment_artifacts:$treatment_artifacts,control_artifacts:$control_artifacts,
	 resource_policy:{gomaxprocs:2,memory_limit_bytes:21474836480,gomemlimit_bytes:19327352832,
		minimum_free_bytes:4294967296,host_cpu_count:$host_cpu_count,allowed_cpu_count:$allowed_cpu_count,
		cpu_limit_percent:90,cpu_affinity:$cpu_affinity}}' >"$fixture_activation_provenance"
v2_r2_require_sv1b_activation_provenance "$fixture_activation_provenance" "$review_revision" "$fixture_simulator_sha256" || {
	echo "full activation provenance rejected the real serialized CDF comparison" >&2
	exit 1
}
jq 'del(.activation_satisfied)' "$full_output_root/cdf-liquidity-comparison.json" >"$temp_root/comparison-missing-pair-field.json"
mv -- "$temp_root/comparison-missing-pair-field.json" "$full_output_root/cdf-liquidity-comparison.json"
fixture_comparison_sha256=$(sha256sum -- "$full_output_root/cdf-liquidity-comparison.json" | awk '{print $1}')
jq --arg comparison_sha256 "$fixture_comparison_sha256" '.comparison_sha256 = $comparison_sha256' \
	"$fixture_activation_provenance" >"$temp_root/full-activation-provenance-missing-pair-field.json"
if v2_r2_require_sv1b_activation_provenance "$temp_root/full-activation-provenance-missing-pair-field.json" "$review_revision" "$fixture_simulator_sha256"; then
	echo "full activation provenance accepted a serialized comparison missing activation_satisfied" >&2
	exit 1
fi
cp -- "$serialized_comparison" "$full_output_root/cdf-liquidity-comparison.json"
fixture_comparison_sha256=$(sha256sum -- "$full_output_root/cdf-liquidity-comparison.json" | awk '{print $1}')
jq --arg comparison_sha256 "$fixture_comparison_sha256" '.comparison_sha256 = $comparison_sha256' \
	"$fixture_activation_provenance" >"$temp_root/full-activation-provenance-restored.json"
jq '.activation_satisfied = false' "$temp_root/full-activation-provenance-restored.json" >"$temp_root/full-activation-provenance-false-pair-field.json"
if v2_r2_require_sv1b_activation_provenance "$temp_root/full-activation-provenance-false-pair-field.json" "$review_revision" "$fixture_simulator_sha256"; then
	echo "full activation provenance accepted a serialized comparison with activation_satisfied=false" >&2
	exit 1
fi

echo "V2-R2-SV1 activation output boundary contract: pass"
