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
rg -F 'activation-provenance.pending.json' "$runner" >/dev/null || {
	echo "activation runner does not stage provenance before final publication" >&2
	exit 1
}
rg -F 'v2_r2_require_sv1b_activation_provenance "$activation_provenance_pending"' "$runner" >/dev/null || {
	echo "activation runner does not self-validate staged provenance" >&2
	exit 1
}
for required_binding in \
	'--argjson activation_gomaxprocs "$activation_gomaxprocs"' \
	'--argjson activation_memory_limit_bytes "$activation_memory_limit_bytes"' \
	'--argjson activation_gomemlimit_bytes "$activation_gomemlimit_bytes"' \
	'--argjson activation_host_cpu_count "$activation_host_cpu_count"' \
	'--argjson activation_allowed_cpu_count "$activation_allowed_cpu_count"' \
	'--argjson v2_r2_sv1_cpu_limit_percent "$v2_r2_sv1_cpu_limit_percent"' \
	'--arg activation_cpu_affinity "$activation_cpu_affinity"' \
	'--argjson activation_minimum_free_bytes "$activation_minimum_free_bytes"'; do
	rg -F -- "$required_binding" "$runner" >/dev/null || {
		echo "activation pair provenance jq binding is missing: $required_binding" >&2
		exit 1
	}
done
rg -F 'renderer_binary_path' "$root_dir/scripts/v2-r2-sv1b-24h-contract.sh" >/dev/null || {
	echo "SV1B activation contract does not carry renderer identity" >&2
	exit 1
}
rg -F 'v2_r2_sv1b_require_pinned_binary "$renderer_path" "$expected_revision" "$renderer_sha256" "exchange_sim/cmd/evsrender"' \
	"$root_dir/scripts/v2-r2-sv1b-24h-contract.sh" >/dev/null || {
	echo "SV1B activation contract does not validate the recorded renderer identity" >&2
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

capacity_selector_fixture="$temp_root/capacity-selector.json"
authorized_capacity_hashes=$(jq -c '[.registered_configs[]]' "$root_dir/research/v2-r2-sv1b-24h-config-provenance.json")
jq -n --argjson authorized "$authorized_capacity_hashes" \
	'{authorized_launch_config_sha256: $authorized}' >"$capacity_selector_fixture"
authorized_capacity_hash=$(jq -er '.registered_configs["treatment-647.json"]' "$root_dir/research/v2-r2-sv1b-24h-config-provenance.json")
v2_r2_sv1b_require_authorized_capacity_config "$capacity_selector_fixture" "$authorized_capacity_hash" || {
	echo "an authorized non-representative capacity configuration was rejected" >&2
	exit 1
}
if v2_r2_sv1b_require_authorized_capacity_config "$capacity_selector_fixture" "$(printf '0%.0s' {1..64})"; then
	echo "an unregistered capacity configuration was accepted" >&2
	exit 1
fi
rg -F 'v2_r2_sv1b_require_authorized_capacity_config "$capacity_attestation" "$config_sha256"' "$cell_runner" >/dev/null || {
	echo "SV1B cell runner does not bind the selected config to the capacity authorization set" >&2
	exit 1
}
rg -F '"$capacity_launch_config_sha256"' "$cell_runner" >/dev/null || {
	echo "SV1B cell runner does not pass the measured capacity launch identity" >&2
	exit 1
}
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

# Generate the comparison with the real analysis producer. It is deliberately
# used as a negative substitution case below: a valid seed-607 producer result
# must not be accepted as a seed-643, three-venue activation pair merely because
# the surrounding attestation names seed 643.
#
# The reduced fixture above protects the economic comparison predicate. The
# boundary fixture below is intentionally validator-shaped only; it must never
# be accepted as producer evidence without complete arm artifacts and pinned
# executable identities.
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
# Deliberately incomplete arms are a regression fixture for the former false
# positive: a pair-level attestation must not upgrade sparse hand-authored
# files into producer evidence.
cp -- "$v2_r2_sv1_activation_config" "$full_output_root/treatment/run-config.json"
cp -- "$v2_r2_sv1_activation_control_config" "$full_output_root/control/run-config.json"
for arm in treatment control; do
	jq -n --arg arm "$arm" '{arm:$arm,exit_status:0,completion_verified:true,
		terminal_failure_verified:false,terminal_outcome_status:"completed",
		resource_guard_failed:false}' >"$full_output_root/$arm/run-status.json"
done
fixture_true_binary=$(type -P true) || {
	echo "could not locate an executable true binary for the provenance fixture" >&2
	exit 1
}
cp -- "$fixture_true_binary" "$temp_root/sv1b-simulator"
cp -- "$fixture_true_binary" "$temp_root/sv1b-analyzer"
cp -- "$fixture_true_binary" "$temp_root/sv1b-renderer"
chmod 0755 -- "$temp_root/sv1b-simulator" "$temp_root/sv1b-analyzer"
chmod 0755 -- "$temp_root/sv1b-renderer"
fixture_simulator_sha256=$(sha256sum -- "$temp_root/sv1b-simulator" | awk '{print $1}')
fixture_analyzer_sha256=$(sha256sum -- "$temp_root/sv1b-analyzer" | awk '{print $1}')
fixture_renderer_sha256=$(sha256sum -- "$temp_root/sv1b-renderer" | awk '{print $1}')
if v2_r2_sv1b_require_pinned_binary "$temp_root/sv1b-simulator" "$review_revision" "$fixture_simulator_sha256" "exchange_sim/cmd/multivenue"; then
	echo "an arbitrary non-Go executable passed the direct activation binary identity check" >&2
	exit 1
fi
fixture_treatment_config_sha256=$(sha256sum -- "$v2_r2_sv1_activation_config" | awk '{print $1}')
fixture_control_config_sha256=$(sha256sum -- "$v2_r2_sv1_activation_control_config" | awk '{print $1}')
fixture_treatment_status_sha256=$(sha256sum -- "$full_output_root/treatment/run-status.json" | awk '{print $1}')
fixture_control_status_sha256=$(sha256sum -- "$full_output_root/control/run-status.json" | awk '{print $1}')
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

activation_venue_ids=$(jq -ce '.venue_ids | select(type == "array" and length == 3)' "$v2_r2_sv1_activation_config")
activation_supplier_count=$(jq -er '(.elastic_liquidity_suppliers | length) * (.venue_ids | length)' "$v2_r2_sv1_activation_config")
activation_treatment_result="$temp_root/activation-treatment-result.json"
jq --argjson expected_supplier_count "$activation_supplier_count" --argjson venue_ids "$activation_venue_ids" '
	.result as $result |
	$result.suppliers as $supplier_templates |
	$result.venues as $venue_templates |
	.result
	| .supplier_count = $expected_supplier_count
	| .decision_count = ($expected_supplier_count * 2)
	| .fill_count = $expected_supplier_count
	| .trading_supplier_count = $expected_supplier_count
	| .pnl_changing_supplier_count = $expected_supplier_count
	| .inventory_responsive_decision_count = $expected_supplier_count
	| .cancel_count = $expected_supplier_count
	| .withdraw_count = $expected_supplier_count
	| .withdrawal_without_replacement_count = $expected_supplier_count
	| .risk_state_decision_count = $expected_supplier_count
	| .fresh_risk_state_decision_count = $expected_supplier_count
	| .venues = [$venue_ids[] as $venue | ($venue_templates[0] | .venue_id = $venue)]
	| .suppliers = [$venue_ids[] as $venue | range(0; 4) as $index
		| ($supplier_templates[$index % ($supplier_templates | length)]
			| .venue_id = $venue
			| .role = ("cdf_elastic_supplier_" + (($index + 1) | tostring))
			| .client_id = ($index + 1)
			| .max_gross_base_balance = 1
			| .configured_max_inventory = 2)]
' "$cdf_audit_fixture" >"$activation_treatment_result"
jq --argjson venue_ids "$activation_venue_ids" '.result | .venues = [$venue_ids[] as $venue | .venues[0] | .venue_id = $venue]' \
	"$temp_root/control-cdfliquidity.json" >"$temp_root/activation-control-result.json"

write_activation_provenance() {
	local output_path=$1 comparison_path=$2 comparison_sha256=$3
	jq -n \
		--arg contract "$v2_r2_sv1_activation_pair_contract" --arg revision "$review_revision" \
		--arg tree_sha256 "$review_tree_sha256" --arg output_root "$full_output_root" \
		--arg treatment_dir "$full_output_root/treatment" --arg control_dir "$full_output_root/control" \
		--arg comparison_path "$comparison_path" \
		--arg review_path "$fixture_review" --arg review_sha256 "$(sha256sum -- "$fixture_review" | awk '{print $1}')" \
		--arg simulator_path "$temp_root/sv1b-simulator" --arg analyzer_path "$temp_root/sv1b-analyzer" \
		--arg renderer_path "$temp_root/sv1b-renderer" \
		--arg simulator_sha256 "$fixture_simulator_sha256" --arg analyzer_sha256 "$fixture_analyzer_sha256" \
		--arg renderer_sha256 "$fixture_renderer_sha256" \
		--arg comparison_sha256 "$comparison_sha256" \
		--arg treatment_source_config_path "$(realpath -e -- "$v2_r2_sv1_activation_config")" \
		--arg control_source_config_path "$(realpath -e -- "$v2_r2_sv1_activation_control_config")" \
		--arg treatment_source_config_sha256 "$fixture_treatment_config_sha256" --arg control_source_config_sha256 "$fixture_control_config_sha256" \
		--arg treatment_config_sha256 "$fixture_treatment_config_sha256" --arg control_config_sha256 "$fixture_control_config_sha256" \
		--arg treatment_status_sha256 "$fixture_treatment_status_sha256" --arg control_status_sha256 "$fixture_control_status_sha256" \
		--argjson treatment_artifacts "$fixture_treatment_artifacts" --argjson control_artifacts "$fixture_control_artifacts" \
		--argjson host_cpu_count "$fixture_host_cpu_count" --argjson allowed_cpu_count "$fixture_allowed_cpu_count" \
		--arg cpu_affinity "$fixture_cpu_affinity" --argjson venue_ids "$activation_venue_ids" \
		--argjson seed "$v2_r2_sv1_activation_seed" --arg horizon "$v2_r2_sv1_activation_horizon" \
		--argjson start_nano "$v2_r2_sv1_activation_simulation_start_nano" \
		--argjson end_nano "$v2_r2_sv1_activation_simulation_end_nano" \
		--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
		--arg treatment_experiment_id "$(jq -er '.experiment_id' "$v2_r2_sv1_activation_config")" \
		--arg control_experiment_id "$(jq -er '.experiment_id' "$v2_r2_sv1_activation_control_config")" \
		--arg treatment_hypothesis_id "$(jq -er '.hypothesis_id' "$v2_r2_sv1_activation_config")" \
		--arg control_hypothesis_id "$(jq -er '.hypothesis_id' "$v2_r2_sv1_activation_control_config")" \
		'{schema_version:3,contract:$contract,candidate_revision:$revision,candidate_tree_sha256:$tree_sha256,
		 seed:$seed,simulated_horizon:$horizon,
		 simulation_start_nano:$start_nano,simulation_end_nano:$end_nano,
		 evidence_format:$evidence_format,log_mode:$log_mode,
		 venue_ids:$venue_ids,treatment_experiment_id:$treatment_experiment_id,
		 control_experiment_id:$control_experiment_id,treatment_hypothesis_id:$treatment_hypothesis_id,
		 control_hypothesis_id:$control_hypothesis_id,output_root:$output_root,
		 treatment_dir:$treatment_dir,control_dir:$control_dir,
		 treatment_source_config_path:$treatment_source_config_path,control_source_config_path:$control_source_config_path,
		 treatment_source_config_sha256:$treatment_source_config_sha256,control_source_config_sha256:$control_source_config_sha256,
		 simulator_binary_path:$simulator_path,analyzer_binary_path:$analyzer_path,renderer_binary_path:$renderer_path,
		 review_attestation_path:$review_path,review_attestation_sha256:$review_sha256,comparison_path:$comparison_path,
		 treatment_config_sha256:$treatment_config_sha256,control_config_sha256:$control_config_sha256,
		 simulator_binary_sha256:$simulator_sha256,analyzer_binary_sha256:$analyzer_sha256,
		 renderer_binary_sha256:$renderer_sha256,comparison_sha256:$comparison_sha256,
		 status:"ACTIVATION_CONTRACT_SATISFIED",activation_satisfied:true,holdouts_consumed:false,
		 treatment_runner_status:0,control_runner_status:0,treatment_terminal_status:"completed",control_terminal_status:"completed",
		 treatment_run_status_sha256:$treatment_status_sha256,control_run_status_sha256:$control_status_sha256,
		 treatment_terminal_outcome_sha256:"",control_terminal_outcome_sha256:"",
		 treatment_artifacts:$treatment_artifacts,control_artifacts:$control_artifacts,
		 resource_policy:{gomaxprocs:2,memory_limit_bytes:21474836480,gomemlimit_bytes:19327352832,
		 minimum_free_bytes:4294967296,host_cpu_count:$host_cpu_count,allowed_cpu_count:$allowed_cpu_count,
		 cpu_limit_percent:90,cpu_affinity:$cpu_affinity}}' >"$output_path"
}

# A real producer output with seed 607 and the one-venue fixture provenance is
# not a valid activation pair, even when the outer attestation claims seed 643.
cp -- "$serialized_comparison" "$full_output_root/cdf-liquidity-comparison.json"
fixture_comparison_sha256=$(sha256sum -- "$full_output_root/cdf-liquidity-comparison.json" | awk '{print $1}')
write_activation_provenance "$fixture_activation_provenance" "$full_output_root/cdf-liquidity-comparison.json" "$fixture_comparison_sha256"
if v2_r2_require_sv1b_activation_provenance "$fixture_activation_provenance" "$review_revision" "$fixture_simulator_sha256"; then
	echo "full activation provenance accepted a seed-607 producer result as seed-643 activation evidence" >&2
	exit 1
fi

# Build a validator-shaped comparison from a detailed audit fixture while
# binding it to the exact registered 4-supplier x 3-venue activation population.
jq -n \
	--slurpfile treatment "$activation_treatment_result" \
	--slurpfile control "$temp_root/activation-control-result.json" \
	--arg revision "$review_revision" --arg analyzer_sha256 "$fixture_analyzer_sha256" \
	--arg simulator_sha256 "$fixture_simulator_sha256" --argjson seed "$v2_r2_sv1_activation_seed" \
	--arg horizon "$v2_r2_sv1_activation_horizon" \
	--argjson start_nano "$v2_r2_sv1_activation_simulation_start_nano" \
	--argjson end_nano "$v2_r2_sv1_activation_simulation_end_nano" \
	--argjson venue_ids "$activation_venue_ids" \
	--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" \
	--arg treatment_config_sha256 "$fixture_treatment_config_sha256" --arg control_config_sha256 "$fixture_control_config_sha256" \
	--arg treatment_experiment_id "$(jq -er '.experiment_id' "$v2_r2_sv1_activation_config")" \
	--arg control_experiment_id "$(jq -er '.experiment_id' "$v2_r2_sv1_activation_control_config")" \
	--arg treatment_hypothesis_id "$(jq -er '.hypothesis_id' "$v2_r2_sv1_activation_config")" \
	--arg control_hypothesis_id "$(jq -er '.hypothesis_id' "$v2_r2_sv1_activation_control_config")" \
	'
		def run_provenance($config_sha256; $experiment_id; $hypothesis_id):
			{config_sha256:$config_sha256,source_revision:$revision,source_modified:false,
			 binary_sha256:$simulator_sha256,binary_goos:"linux",binary_goarch:"amd64",binary_goamd64:"v1",
			 seed:$seed,horizon:$horizon,simulation_start_nano:$start_nano,simulation_end_nano:$end_nano,
			 venue_ids:$venue_ids,experiment_id:$experiment_id,hypothesis_id:$hypothesis_id,
			 evidence_format:$evidence_format,log_mode:$log_mode,valid:true};
		 {valid:true,evidence_valid:true,activation_satisfied:true,anti_cheating_satisfied:true,
		  provenance:{treatment:run_provenance($treatment_config_sha256; $treatment_experiment_id; $treatment_hypothesis_id),
			 control:run_provenance($control_config_sha256; $control_experiment_id; $control_hypothesis_id),
			 analyzer_sha256:$analyzer_sha256,analyzer_source_revision:$revision,
			 analyzer_source_modified:false,valid:true},
		  treatment:$treatment[0],control:$control[0]}' \
	>"$full_output_root/cdf-liquidity-comparison.json"
fixture_comparison_sha256=$(sha256sum -- "$full_output_root/cdf-liquidity-comparison.json" | awk '{print $1}')
write_activation_provenance "$fixture_activation_provenance" "$full_output_root/cdf-liquidity-comparison.json" "$fixture_comparison_sha256"
v2_r2_sv1b_require_activation_comparison_identity "$full_output_root/cdf-liquidity-comparison.json" "$fixture_activation_provenance" \
	"$review_revision" "$fixture_simulator_sha256" "$fixture_analyzer_sha256" "$activation_supplier_count" || {
	echo "exact registered supplier role and venue population was rejected" >&2
	exit 1
}
if jq '.treatment.suppliers[0].role = "unregistered_role"' "$full_output_root/cdf-liquidity-comparison.json" >"$temp_root/comparison-wrong-role.json" &&
	v2_r2_sv1b_require_activation_comparison_identity "$temp_root/comparison-wrong-role.json" "$fixture_activation_provenance" \
		"$review_revision" "$fixture_simulator_sha256" "$fixture_analyzer_sha256" "$activation_supplier_count"; then
	echo "comparison accepted a supplier role outside the registered roster" >&2
	exit 1
fi
if jq '.treatment.suppliers[0].venue_id = "unregistered_venue"' "$full_output_root/cdf-liquidity-comparison.json" >"$temp_root/comparison-wrong-venue.json" &&
	v2_r2_sv1b_require_activation_comparison_identity "$temp_root/comparison-wrong-venue.json" "$fixture_activation_provenance" \
		"$review_revision" "$fixture_simulator_sha256" "$fixture_analyzer_sha256" "$activation_supplier_count"; then
	echo "comparison accepted a supplier venue outside the registered roster" >&2
	exit 1
fi
if v2_r2_require_sv1b_activation_provenance "$fixture_activation_provenance" "$review_revision" "$fixture_simulator_sha256"; then
	echo "hand-authored activation fixture bypassed complete producer provenance" >&2
	exit 1
fi

expect_comparison_mutation_rejected() {
	local name=$1 filter=$2
	local mutated_path="$full_output_root/mutated-$name.json"
	local mutated_provenance="$temp_root/full-activation-provenance-mutated-$name.json"
	jq "$filter" "$full_output_root/cdf-liquidity-comparison.json" >"$mutated_path"
	local mutated_sha256
	mutated_sha256=$(sha256sum -- "$mutated_path" | awk '{print $1}')
	write_activation_provenance "$mutated_provenance" "$mutated_path" "$mutated_sha256"
	if v2_r2_require_sv1b_activation_provenance "$mutated_provenance" "$review_revision" "$fixture_simulator_sha256"; then
		echo "full activation provenance accepted mutated comparison: $name" >&2
		exit 1
	fi
}

expect_comparison_mutation_rejected wrong-treatment-seed '.provenance.treatment.seed = 642'
expect_comparison_mutation_rejected wrong-source-revision '.provenance.treatment.source_revision = ("a" * 40)'
expect_comparison_mutation_rejected wrong-binary-sha '.provenance.treatment.binary_sha256 = ("0" * 64)'
expect_comparison_mutation_rejected wrong-config-sha '.provenance.treatment.config_sha256 = ("1" * 64)'
expect_comparison_mutation_rejected wrong-evidence-format '.provenance.treatment.evidence_format = "jsonl"'
expect_comparison_mutation_rejected wrong-log-mode '.provenance.treatment.log_mode = "summary"'
expect_comparison_mutation_rejected wrong-venue-set '.provenance.treatment.venue_ids = ["north", "central"]'
expect_comparison_mutation_rejected wrong-analyzer-sha '.provenance.analyzer_sha256 = ("2" * 64)'
expect_comparison_mutation_rejected wrong-analyzer-revision '.provenance.analyzer_source_revision = ("b" * 40)'
expect_comparison_mutation_rejected missing-pair-field 'del(.activation_satisfied)'
expect_comparison_mutation_rejected false-pair-field '.activation_satisfied = false'
expect_comparison_mutation_rejected wrong-supplier-population '.treatment.supplier_count = 11'

expect_activation_provenance_mutation_rejected() {
	local name=$1 filter=$2
	local mutated_path="$temp_root/full-activation-provenance-mutated-$name.json"
	jq "$filter" "$fixture_activation_provenance" >"$mutated_path"
	if v2_r2_require_sv1b_activation_provenance "$mutated_path" "$review_revision" "$fixture_simulator_sha256"; then
		echo "full activation provenance accepted mutated outer identity: $name" >&2
		exit 1
	fi
}

expect_activation_provenance_mutation_rejected wrong-outer-venue-set '.venue_ids = ["north", "central"]'
expect_activation_provenance_mutation_rejected wrong-outer-experiment '.treatment_experiment_id = "unregistered-experiment"'
expect_activation_provenance_mutation_rejected wrong-outer-start '.simulation_start_nano = 1735689600000000001'
expect_activation_provenance_mutation_rejected wrong-outer-evidence-format '.evidence_format = "jsonl"'

echo "V2-R2-SV1 activation output boundary contract: pass"
