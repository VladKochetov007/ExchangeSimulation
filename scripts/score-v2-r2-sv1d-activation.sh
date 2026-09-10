#!/usr/bin/env bash
# Score the retained SV1D tri-arm activation probe. This consumer never edits
# simulator evidence; it writes one append-only score after validating the
# producer, comparison, and no-roster control contracts.
set -euo pipefail

if [[ $# -gt 1 ]]; then
	echo "usage: $0 [activation-output-root]" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
scientific_root=$(realpath -e -- "$root_dir") || exit 1
export V2_R2_SV1_CONTRACT_SCRIPT="$root_dir/scripts/v2-r2-sv1d-activation-contract.sh"
source "$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
contract_script=$(v2_r2_select_sv1_contract "$root_dir") || exit 1
source "$contract_script"

v2_r2_require_known_candidate || exit 1
head_revision=$(git -C "$root_dir" rev-parse HEAD) || exit 1
head_tree_sha256=$(v2_r2_sv1d_git_tree_sha256 "$head_revision") || exit 1
output_root=${1:-"/home/vlad/external-scratch/$v2_r2_sv1_activation_output_prefix-$v2_r2_sv1_activation_seed-$head_revision"}
[[ "$output_root" == /* && "$output_root" != */ && "$output_root" != *$'\n'* && "$output_root" != *$'\t'* ]] || exit 1
output_root=$(realpath -e -- "$output_root") || exit 1
case "$output_root" in
	"$scientific_root"|"$scientific_root"/*)
		echo "activation score must read evidence outside the scientific repository" >&2
		exit 1
		;;
esac

provenance_path="$output_root/activation-provenance.json"
score_path="$output_root/activation-score.json"
[[ -s "$provenance_path" && ! -L "$provenance_path" ]] || exit 1
[[ ! -e "$score_path" && ! -L "$score_path" ]] || {
	echo "refusing to overwrite an existing SV1D activation score" >&2
	exit 1
}
v2_r2_require_single_json_object "$provenance_path" || exit 1

jq -e --arg contract "$v2_r2_sv1_activation_pair_contract" --arg candidate "$v2_r2_sv1_candidate_id" \
	--arg revision "$head_revision" --arg tree_sha256 "$head_tree_sha256" \
	--argjson seed "$v2_r2_sv1_activation_seed" --arg horizon "$v2_r2_sv1_activation_horizon" \
	--arg evidence_format "$v2_r2_sv1_activation_evidence_format" --arg log_mode "$v2_r2_sv1_activation_log_mode" '
		type == "object" and .schema_version == 1 and .contract == $contract and .candidate == $candidate and
		.candidate_revision == $revision and .candidate_tree_sha256 == $tree_sha256 and .seed == $seed and
		.simulated_horizon == $horizon and .evidence_format == $evidence_format and .log_mode == $log_mode and
		.holdouts_consumed == false and (.arms | type) == "object" and
		(.arms | keys | sort) == ["mode-off", "no-roster", "treatment"] and
		(.resource_policy | type) == "object" and .resource_policy.gomaxprocs == 2 and
		.resource_policy.memory_limit_bytes == (20 * 1024 * 1024 * 1024) and
		.resource_policy.gomemlimit_bytes == (18 * 1024 * 1024 * 1024) and
		.resource_policy.minimum_free_bytes == (4 * 1024 * 1024 * 1024) and
		(.resource_policy.host_memory_total_bytes | type) == "number" and .resource_policy.host_memory_total_bytes > 0 and
		(.resource_policy.minimum_memory_available_bytes | type) == "number" and
		.resource_policy.minimum_memory_available_bytes == (if ((.resource_policy.host_memory_total_bytes + 4) / 5) < (4 * 1024 * 1024 * 1024) then (4 * 1024 * 1024 * 1024) else ((.resource_policy.host_memory_total_bytes + 4) / 5 | floor) end) and
		.resource_policy.max_wall_seconds == 900 and .resource_policy.analyzer_max_wall_seconds == 300 and
		(.capacity | type) == "object" and .capacity.contract == "v2-r2-sv1d-24h-binary-capacity-v1"' "$provenance_path" >/dev/null || exit 1

config_checker="$root_dir/$v2_r2_sv1_config_checker_path"
[[ -x "$config_checker" ]] || exit 1
"$config_checker" >/dev/null || {
	echo "SV1D configs failed the consumer-side provenance checker" >&2
	exit 1
}

binary_path=$(jq -er '.binaries.simulator.path | select(type == "string")' "$provenance_path") || exit 1
binary_sha256=$(jq -er '.binaries.simulator.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
audit_path=$(jq -er '.binaries.analyzer.path | select(type == "string")' "$provenance_path") || exit 1
audit_sha256=$(jq -er '.binaries.analyzer.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
renderer_path=$(jq -er '.binaries.renderer.path | select(type == "string")' "$provenance_path") || exit 1
renderer_sha256=$(jq -er '.binaries.renderer.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
checkpoint_path=$(jq -er '.binaries.checkpoint_validator.path | select(type == "string")' "$provenance_path") || exit 1
checkpoint_sha256=$(jq -er '.binaries.checkpoint_validator.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
[[ "$(v2_r2_sv1d_sha256_file "$binary_path")" == "$binary_sha256" ]] || exit 1
[[ "$(v2_r2_sv1d_sha256_file "$audit_path")" == "$audit_sha256" ]] || exit 1
[[ "$(v2_r2_sv1d_sha256_file "$renderer_path")" == "$renderer_sha256" ]] || exit 1
[[ "$(v2_r2_sv1d_sha256_file "$checkpoint_path")" == "$checkpoint_sha256" ]] || exit 1
v2_r2_sv1d_require_pinned_binary "$binary_path" "$head_revision" "$binary_sha256" "exchange_sim/cmd/multivenue" || exit 1
v2_r2_sv1d_require_pinned_binary "$audit_path" "$head_revision" "$audit_sha256" "exchange_sim/cmd/cdf-liquidity-audit" || exit 1
v2_r2_sv1d_require_pinned_binary "$renderer_path" "$head_revision" "$renderer_sha256" "exchange_sim/cmd/evsrender" || exit 1
v2_r2_register_checkpoint_validator "$checkpoint_path" "$head_revision" "$checkpoint_sha256" || exit 1

review_path=$(jq -er '.review.path | select(type == "string")' "$provenance_path") || exit 1
review_sha256=$(jq -er '.review.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
[[ "$(v2_r2_sv1d_sha256_file "$review_path")" == "$review_sha256" ]] || exit 1
v2_r2_require_sv1b_review_attestation "$review_path" "$head_revision" || exit 1

capacity_path=$(jq -er '.capacity.path | select(type == "string")' "$provenance_path") || exit 1
capacity_sha256=$(jq -er '.capacity.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
capacity_config_sha256=$(jq -er '.capacity.config_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
[[ "$capacity_path" == "$(v2_r2_sv1d_capacity_attestation_path "$head_revision")" ]] || exit 1
[[ "$(v2_r2_sv1d_sha256_file "$capacity_path")" == "$capacity_sha256" ]] || exit 1
[[ "$capacity_config_sha256" == "$(v2_r2_sv1d_sha256_file "$v2_r2_sv1d_capacity_config")" ]] || exit 1
v2_r2_sv1d_require_capacity_attestation "$capacity_path" "$head_revision" "$binary_sha256" \
	"$capacity_config_sha256" "$review_path" "$review_sha256" || exit 1

config_manifest_path=$(jq -er '.config_provenance_manifest_path | select(type == "string")' "$provenance_path") || exit 1
config_manifest_sha256=$(jq -er '.config_provenance_manifest_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
[[ -f "$config_manifest_path" && ! -L "$config_manifest_path" ]] || exit 1
[[ "$(v2_r2_sv1d_sha256_file "$config_manifest_path")" == "$config_manifest_sha256" ]] || exit 1

for arm in "${v2_r2_sv1d_arm_names[@]}"; do
	arm_dir="$output_root/$arm"
	arm_config=$(v2_r2_sv1d_config_for_arm "$arm")
	arm_config_sha256=$(v2_r2_sv1d_sha256_file "$arm_config") || exit 1
	arm_record=$(jq -ce --arg arm "$arm" '.arms[$arm]' "$provenance_path") || exit 1
	jq -e --arg arm "$arm" --arg path "$arm_dir" --arg mode "$(v2_r2_sv1d_arm_mode "$arm")" \
		--arg config_sha256 "$arm_config_sha256" '
			.path == $path and .mode == $mode and .config_sha256 == $config_sha256 and .valid == true and
			(.exit_status | type) == "number" and (.terminal_status | type) == "string" and
			(.run_status_sha256 | type) == "string" and (.run_status_sha256 | test("^[0-9a-f]{64}$")) and
			(.terminal_outcome_sha256 | type) == "string" and (.terminal_outcome_sha256 | test("^[0-9a-f]{64}$")) and
			all([.run_metadata_sha256, .manifest_sha256, .binary_attestation_sha256, .evidence_manifest_sha256,
				.simulator_stdout_sha256, .simulator_stderr_sha256][]; type == "string" and test("^[0-9a-f]{64}$"))' \
		<<<"$arm_record" >/dev/null || exit 1
	v2_r2_sv1d_require_activation_arm_artifacts "$arm_dir" "$arm" "$head_revision" "$arm_config_sha256" "$binary_sha256" || exit 1
	v2_r2_sv1d_require_arm_record_matches "$provenance_path" "$arm" "$arm_dir" || exit 1
	jq -e --arg binary_path "$binary_path" --arg checkpoint_path "$checkpoint_path" \
		--arg review_path "$review_path" --arg review_sha256 "$review_sha256" \
		--arg revision "$head_revision" --arg checkpoint_sha256 "$checkpoint_sha256" \
		'.binary_path == $binary_path and .checkpoint_validator_path == $checkpoint_path and
		 .checkpoint_validator_revision == $revision and .checkpoint_validator_sha256 == $checkpoint_sha256 and
		 .review_attestation_path == $review_path and .review_attestation_sha256 == $review_sha256' \
		"$arm_dir/run-metadata.json" >/dev/null || exit 1
done

treatment_dir="$output_root/treatment"
mode_off_dir="$output_root/mode-off"
no_roster_dir="$output_root/no-roster"
no_roster_terminal_status=$(jq -er '.status' "$no_roster_dir/terminal-outcome.json") || exit 1
jq -e '
	.elastic_liquidity_suppliers == null and .record_elastic_liquidity_supplier_decisions == null and
	.market_data_receipt_roles == ["liability_hedger"] and .record_market_data_receipts == true and
	.elastic_supplier_count == 8 and .elastic_supplier_symbols == null' \
	"$v2_r2_sv1_activation_no_roster_config" >/dev/null || exit 1
jq -e --arg expected "$v2_r2_sv1_activation_no_roster_config" '
	.config_path == $expected and .mode == "no_roster" and .valid == true and
	(.terminal_status == "completed" or .terminal_status == "terminal_failure")' \
	<(jq -ce '.arms["no-roster"]' "$provenance_path") >/dev/null || exit 1

no_roster_diagnostic_path=$(jq -er '.no_roster_diagnostic.path | select(type == "string")' "$provenance_path") || exit 1
no_roster_diagnostic_sha256=$(jq -er '.no_roster_diagnostic.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
[[ -f "$no_roster_diagnostic_path" && ! -L "$no_roster_diagnostic_path" &&
	"$(v2_r2_sv1d_sha256_file "$no_roster_diagnostic_path")" == "$no_roster_diagnostic_sha256" ]] || exit 1
v2_r2_sv1d_require_no_roster_diagnostic "$no_roster_diagnostic_path" "$no_roster_dir" \
	"$(v2_r2_sv1d_sha256_file "$no_roster_dir/run-config.json")" || exit 1

comparison_path=$(jq -er '.comparison.recorded_path | select(type == "string")' "$provenance_path") || exit 1
comparison_sha256=$(jq -er '.comparison.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$provenance_path") || exit 1
[[ -f "$comparison_path" && ! -L "$comparison_path" && "$(v2_r2_sv1d_sha256_file "$comparison_path")" == "$comparison_sha256" ]] || exit 1
v2_r2_require_single_json_object "$comparison_path" || exit 1
v2_r2_sv1d_require_comparison_provenance "$comparison_path" "$audit_sha256" "$head_revision" "$treatment_dir" "$mode_off_dir" || exit 1

comparison_valid=$(jq -er '(.valid | type) == "boolean" and .valid' "$comparison_path" 2>/dev/null || true)
comparison_evidence_valid=$(jq -er '(.evidence_valid | type) == "boolean" and .evidence_valid' "$comparison_path" 2>/dev/null || true)
comparison_anticheating=$(jq -er '(.anti_cheating_satisfied | type) == "boolean" and .anti_cheating_satisfied' "$comparison_path" 2>/dev/null || true)
comparison_provenance_valid=$(jq -er '(.provenance == null or .provenance.valid == true)' "$comparison_path" 2>/dev/null || true)
comparison_terminal_negative=$(jq -er '.status == "UNAVAILABLE_TERMINAL_FAILURE"' "$comparison_path" 2>/dev/null || true)
score_status="SV1D_ACTIVATION_INVALID_EVIDENCE"
score_reason="comparison was not a valid reconstructed same-roster pair"
if [[ "$comparison_valid" == true && "$comparison_evidence_valid" == true && "$comparison_provenance_valid" == true ]]; then
	if [[ "$comparison_terminal_negative" == true ]]; then
		score_status="SV1D_ACTIVATION_NOT_SATISFIED_TERMINAL_FAILURE"
		score_reason="the registered treatment/control pair reached a valid terminal valuation failure; no activation claim is made"
	elif [[ "$comparison_anticheating" != true ]]; then
		score_status="SV1D_ACTIVATION_REJECTED_ANTI_CHEATING"
		score_reason="reconstructed evidence was valid but a preregistered anti-cheating or concentration predicate failed"
	elif v2_r2_sv1d_require_mode_pair_comparison "$comparison_path" "$(jq -er '.expected_supplier_count' "$provenance_path")"; then
		score_status="SV1D_ACTIVATION_ACCEPTED"
		score_reason="all supplier instances, one-sided restoration, paired survival effect, and anti-cheating predicates passed"
	else
		score_status="SV1D_ACTIVATION_NOT_SATISFIED"
		score_reason="reconstructed evidence is valid but the preregistered activation or survival predicate did not pass"
	fi
fi

score_tmp="$score_path.tmp-$$"
activation_provenance_sha256=$(v2_r2_sv1d_sha256_file "$provenance_path") || exit 1
jq -n --arg contract "$v2_r2_sv1_scorer_contract" --arg candidate "$v2_r2_sv1_candidate_id" \
	--arg revision "$head_revision" --arg tree_sha256 "$head_tree_sha256" --arg status "$score_status" \
	--arg reason "$score_reason" --arg output_root "$output_root" --arg comparison_path "$comparison_path" \
	--arg comparison_sha256 "$comparison_sha256" --argjson seed "$v2_r2_sv1_activation_seed" \
	--argjson holdouts_consumed false --argjson arm_count "${#v2_r2_sv1d_arm_names[@]}" \
	--arg no_roster_terminal_status "$no_roster_terminal_status" \
	--argjson comparison_valid "$(jq -r '(.valid // false)' "$comparison_path")" \
	--argjson comparison_evidence_valid "$(jq -r '(.evidence_valid // false)' "$comparison_path")" \
	--argjson comparison_anticheating "$(jq -r '(.anti_cheating_satisfied // false)' "$comparison_path")" \
	--arg comparison_provenance_valid "$comparison_provenance_valid" --arg comparison_status "$comparison_terminal_negative" \
	--arg activation_provenance_sha256 "$activation_provenance_sha256" \
	'{schema_version: 1, contract: $contract, candidate: $candidate, candidate_revision: $revision,
	 candidate_tree_sha256: $tree_sha256, seed: $seed, output_root: $output_root, status: $status,
	 reason: $reason, holdouts_consumed: $holdouts_consumed, arm_count: $arm_count,
	 activation_provenance_sha256: $activation_provenance_sha256,
	 comparison: {path: $comparison_path, sha256: $comparison_sha256, valid: $comparison_valid,
	   evidence_valid: $comparison_evidence_valid, anti_cheating_satisfied: $comparison_anticheating,
	   provenance_valid: ($comparison_provenance_valid == "true"), terminal_negative: ($comparison_status == "true")},
	 no_roster_control: {config_path: "research/configs/v2-r2-sv1d-activation/activation-659-no-roster.json",
	   terminal_status: $no_roster_terminal_status, role_contract: "liability_hedger_only"},
	 scope: "development-only five-minute activation classification; no 24-hour or holdout claim"}' \
	>"$score_tmp"
mv -- "$score_tmp" "$score_path"
printf 'SV1D activation score: %s (%s)\n' "$score_status" "$output_root"
if [[ "$score_status" == SV1D_ACTIVATION_INVALID_EVIDENCE ]]; then
	exit 1
fi
