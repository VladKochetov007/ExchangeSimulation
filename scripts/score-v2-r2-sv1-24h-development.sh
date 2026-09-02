#!/usr/bin/env bash
# Apply the precommitted V2-R2-SV1 development scoring contract. This scorer
# consumes only the six registered full treatment/control cells plus the
# registered seed-607 parity controls. It never opens a holdout directory.
set -euo pipefail

if [[ $# -gt 1 ]]; then
	printf 'usage: %s [SV1_OUTPUT_ROOT]\n' "$0" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"
output_root=${1:-"$v2_r2_output_root"}
score="$output_root/development-score.json"
parity="$output_root/parity-attestation.json"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
cdf_audit=${CDF_LIQUIDITY_AUDIT_BIN:-"$root_dir/bin/cdf-liquidity-audit"}
contract_version="v2-r2-sv1-24h-development-scorer-v3"
survival_contract="v2-r2-sv1-24h-survival-side-availability-v2"
simulation_start_nano=1735689600000000000
simulation_end_nano=1735776000000000000
survival_start_nano=1735693200000000000
survival_start_seconds=1735693200
survival_window_nano=3600000000000
survival_expected_windows=23
max_empty_side_share=0.02
survival_summary_filter="$root_dir/scripts/v2-r2-sv1-survival-summary.jq"
score_classification_filter="$root_dir/scripts/v2-r2-sv1-score-classification.jq"

fail() {
	printf 'SV1 development scorer failure: %s\n' "$*" >&2
	exit 1
}
require_file() {
	[[ -s "$1" ]] || fail "missing scorer input: $1"
}
require_object() {
	jq -e 'type == "object"' "$1" >/dev/null || fail "malformed scorer JSON: $1"
}
binary_field() {
	local binary=$1 field=$2
	go version -m "$binary" | awk -v wanted="$field" '$1 == "build" && index($2, wanted) == 1 {sub(wanted, "", $2); print $2; exit}'
}
require_clean_binary() {
	local binary=$1 label=$2
	[[ -x "$binary" ]] || fail "missing $label binary: $binary"
	local revision modified trimpath cgo go_version
	revision=$(binary_field "$binary" "vcs.revision=")
	modified=$(binary_field "$binary" "vcs.modified=")
	trimpath=$(binary_field "$binary" "-trimpath=")
	cgo=$(binary_field "$binary" "CGO_ENABLED=")
	go_version=$(v2_r2_binary_go_version "$binary")
	v2_r2_is_go_127 "$go_version" || fail "$label is not built with Go 1.27: $go_version"
	[[ "$revision" == "$head_revision" && "$modified" == false && "$trimpath" == true && "$cgo" == 0 ]] ||
		fail "$label is not a clean reproducible build of current HEAD"
}

v2_r2_acquire_namespace_lock || fail "could not acquire the SV1 namespace lock"
v2_r2_require_output_root "$output_root" || fail "scorer root is not the canonical SV1 evidence root"
[[ -r "$survival_summary_filter" ]] || fail "missing survival summary contract: $survival_summary_filter"
[[ -r "$score_classification_filter" ]] || fail "missing score classification contract: $score_classification_filter"
[[ ! -e "$score" && ! -L "$score" ]] || fail "refusing to overwrite precommitted score: $score"
for holdout in holdout-619 holdout-631 holdout-641; do
	[[ ! -e "$output_root/$holdout" && ! -L "$output_root/$holdout" ]] ||
		fail "reserved holdout output exists before freeze authorization: $holdout"
done
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] ||
	fail "scoring requires a clean source worktree"
head_revision=$(git -C "$root_dir" rev-parse HEAD)
"$root_dir/scripts/check-v2-r2-sv1-24h-configs.sh" >/dev/null
require_clean_binary "$analyzer" analyzer
require_clean_binary "$cdf_audit" cdf-liquidity-audit

required_artifacts=(
	observationreceipts.json frontiervectors.json mechanical.json conservation.json
	positions.json fillpositions.json orderlifecycle.json lifecycle.json settlements.json
	expiryfills.json evidenceartifacthash.json streamhash.json arbitrage.json crossvenue.json
	roleaudit.json ecology.json derivatives.json liquidations.json marginchecks.json
	optionsurface.json optionliabilityp6.json optionvaluetakerp6.json vannavolgap6.json
	exposure.json hedging.json makerrefresh.json makerquotesize.json makerrebalance.json
	postonly.json liabilityhedger.json perpsignals.json datedmandatep5.json fundingcarry.json
	termcarry.json datedcarryp5.json perpreplenishment.json activation.json integrity.json calendar.json cdfliquidity.json priceunavailable.json
)
required_json=$(printf '%s\n' "${required_artifacts[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')

if [[ -e "$parity" || -L "$parity" ]]; then
	"$root_dir/scripts/check-v2-r2-sv1-24h-parity.sh" --verify-existing "$output_root" >/dev/null ||
		fail "existing seed-607 parity attestation did not recompute exactly"
else
	"$root_dir/scripts/check-v2-r2-sv1-24h-parity.sh" "$output_root" >/dev/null ||
		fail "seed-607 parity controls are not valid"
fi
require_file "$parity"
require_object "$parity"
jq -e '.schema_version == 1 and .contract == "v2-r2-sv1-24h-parity-v1" and
	.evidence_format == "evstream_v3" and
	.exact_equal_domains == ["checkpoints.jsonl", "greeks.json", "latency.json", "events.evs", "binary-evidence-attestation.json", "ordered_raw_manifest"] and
	.control_normalized_equal_domains == ["checkpoints.jsonl", "greeks.json", "latency.json", "execution_event_frames", "execution_stream_frames", "execution_stream_hash"] and
	(.predicates | keys) == ["control_full_no_log_normalized_equal", "evidence_only_reconstruction_equal", "no_log_evidence_absent", "ordered_raw_evidence_equal", "source_and_build_identity_equal", "treatment_g4_g8_equal"] and
	(.control_no_log_normalized_execution | all(to_entries[]; .value == true)) and
	all(.predicates | to_entries[]; .value == true)' "$parity" >/dev/null ||
	fail "invalid SV1 parity attestation"

registered_roster_count=$(jq -er '(.elastic_liquidity_suppliers | length) * (.venue_ids | length)' \
	"$root_dir/research/configs/v2-r2-sv1-24h/treatment-607.json")
[[ "$registered_roster_count" =~ ^[0-9]+$ && "$registered_roster_count" -gt 0 ]] || fail "invalid registered CDF roster count"

terminal_measurement_valid() {
	local cell=$1
	if jq -e --argjson end "$simulation_end_nano" '
		(.terminal_accounts | type) == "array" and (.terminal_accounts | length) > 0 and
		all(.terminal_accounts[];
			.phase == "terminal_post_mark" and
			(.account | type) == "object" and
			.account.timestamp == $end and
			(.mark_source | type) == "string" and
			(.marks | type) == "object" and
			(.marks.CDF | type) == "number" and .marks.CDF > 0 and
			(.marks.USD | type) == "number" and .marks.USD > 0)' "$cell/greeks.json" >/dev/null; then
		return 0
	fi
	return 1
}

terminal_mark_valid() {
	local cell=$1
	terminal_measurement_valid "$cell" || return 1
	if jq -e '
		all(.terminal_accounts[];
			.mark_source == "two_sided_ABC_USD_and_CDF_USD_mid")' "$cell/greeks.json" >/dev/null; then
		return 0
	fi
	return 1
}

write_survival_summary() {
	local cell=$1 seed=$2 output=$3 raw=$4
	local temporary
	temporary=$(mktemp "$output.tmp-XXXXXX")
	if jq -e --arg cell "$(basename "$cell")" --argjson seed "$seed" \
		--arg contract "$survival_contract" --argjson start_nano "$survival_start_nano" \
		--argjson window_nano "$survival_window_nano" --argjson expected_windows "$survival_expected_windows" \
		--argjson max_empty "$max_empty_side_share" \
		-f "$survival_summary_filter" "$raw" >"$temporary" &&
		jq -e '.schema_version == 1 and .contract == $contract and
			(.venues | length) == 3 and (.predicates | keys) == ["aggregate_two_sided_98pct", "cdf_books_present", "exact_post_warmup_window_coverage", "no_persistent_one_sided_window", "post_warmup_snapshots_present"] and
			(.venues | all(.[]; (.snapshots | type) == "number" and (.empty_side_snapshots | type) == "number"))' \
			--arg contract "$survival_contract" "$temporary" >/dev/null; then
		mv "$temporary" "$output"
		return 0
	fi
	# A failed derived measurement is retained for diagnosis; it is never
	# silently removed or treated as a zero-survival observation.
	mv "$temporary" "$output.invalid"
	return 1
}

run_survival_metric() {
	local cell=$1 seed=$2 output="$output_root/survival-$(basename "$cell").json"
	[[ ! -e "$output" && ! -L "$output" && ! -e "$output.invalid" && ! -L "$output.invalid" ]] ||
		fail "refusing to overwrite survival measurement: $output"
	local raw status
	raw=$(mktemp "$output.raw-XXXXXX")
	set +e
	"$analyzer" -metric viability -json -viability-window 3600 -viability-start "$survival_start_seconds" -viability-judge-life-edges "$cell" >"$raw"
	status=$?
	set -e
	if [[ "$status" -ne 0 || ! -s "$raw" ]] || ! write_survival_summary "$cell" "$seed" "$output" "$raw"; then
		mv "$raw" "$output.raw.invalid"
		return 1
	fi
	rm -f -- "$raw"
}

pair_records='[]'
all_treatment_terminal_valid=true
all_treatment_terminal_measurement_valid=true
all_control_terminal_measurement_valid=true
all_control_terminal_valid=true
all_treatment_survival_valid=true
all_treatment_survival_measurement_valid=true
all_control_survival_valid=true
all_control_survival_measurement_valid=true
all_cdf_contract_valid=true
all_anticheating_valid=true
all_cells_valid=true

for seed in 607 613 617; do
	for population in treatment control; do
		cell="$output_root/$population-$seed"
		expected_cdf_supplier_count=$(jq -er '((.elastic_liquidity_suppliers // []) | length) * (.venue_ids | length)' "$cell/run-config.json")
		[[ "$expected_cdf_supplier_count" =~ ^[0-9]+$ ]] || fail "invalid CDF roster count: $population-$seed"
		v2_r2_require_cell_path "$cell" || fail "cell is outside the canonical SV1 root: $cell"
		V2_R2_EXTRACTOR_VARIANT=sv1 MVANALYZE_BIN="$analyzer" \
			"$root_dir/scripts/verify-v2-r2-sv1-24h-cell.sh" "$cell" >/dev/null ||
			fail "fresh extraction failed: $population-$seed"
		for file in "${required_artifacts[@]}"; do
			require_file "$cell/$file"
		done
		require_file "$cell/analysis-metadata.json"
		require_file "$cell/integrity.json"
		require_file "$cell/activation.json"
		jq -e --arg cell "$population-$seed" --arg population "$population" --argjson seed "$seed" --arg contract "v2-r2-sv1-24h-candidate-v3" \
			--argjson required "$required_json" \
			'.schema_version == 3 and .cell == $cell and .seed == $seed and
			 .evidence_format == "evstream_v3" and .analysis_contract == $contract and
			 .integrity_contract == $contract and .activation_contract == $contract and
			 .analysis_revision == $head_revision and .analyzer_revision == $head_revision and
			 .analyzer_vcs_modified == false and .require_exact_replay == true and
			 .required_artifacts == $required and (.artifact_sha256 | keys) == ($required | sort) and
			 all(.artifact_sha256 | to_entries[]; .value | test("^[0-9a-f]{64}$"))' \
			--arg head_revision "$head_revision" "$cell/analysis-metadata.json" >/dev/null ||
			fail "invalid analysis metadata: $population-$seed"
		for file in "${required_artifacts[@]}"; do
			actual=$(sha256sum "$cell/$file" | awk '{print $1}')
			declared=$(jq -er --arg file "$file" '.artifact_sha256[$file]' "$cell/analysis-metadata.json")
			[[ "$actual" == "$declared" ]] || fail "artifact hash mismatch: $population-$seed/$file"
		done
		if ! jq -e 'all(.predicates | to_entries[]; .value == true)' "$cell/integrity.json" >/dev/null ||
			! jq -e --arg population "$population" --argjson expected "$expected_cdf_supplier_count" '
				(.result.predicates | keys) == ["calendar_behavior_attested", "cdf_liquidity_population_contract", "zero_price_unavailable_order_rejections"] and
				.result.predicates.cdf_liquidity_population_contract == true and
				.result.cdf_liquidity.valid == true and
				.result.cdf_liquidity.population == $population and
				.result.cdf_liquidity.expected_supplier_count == $expected and
				.result.cdf_liquidity.supplier_count == $expected and
				(.result.observed.cdf_liquidity_activation_observed | type) == "boolean" and
				.result.observed.cdf_liquidity_activation_observed == ($population == "treatment") and
				all(.result.predicates | to_entries[]; .value == true)' "$cell/activation.json" >/dev/null ||
			! jq -e '.result.valid == true and (.result.malformed_order_rejected_count // 0) == 0 and
				(.result.price_unavailable_order_rejections | type) == "number"' "$cell/priceunavailable.json" >/dev/null; then
			all_cells_valid=false
		fi
		if ! terminal_measurement_valid "$cell"; then
			if [[ "$population" == treatment ]]; then
				all_treatment_terminal_measurement_valid=false
			else
				all_control_terminal_measurement_valid=false
			fi
		fi
		if [[ "$population" == treatment ]] && ! terminal_mark_valid "$cell"; then
			all_treatment_terminal_valid=false
		elif [[ "$population" == control ]] && ! terminal_mark_valid "$cell"; then
			all_control_terminal_valid=false
		fi
		if ! run_survival_metric "$cell" "$seed"; then
			if [[ "$population" == treatment ]]; then
				all_treatment_survival_measurement_valid=false
				all_treatment_survival_valid=false
			else
				all_control_survival_measurement_valid=false
				all_control_survival_valid=false
			fi
		else
			if ! jq -e '.predicates | all(to_entries[]; .value == true)' "$output_root/survival-$(basename "$cell").json" >/dev/null; then
				if [[ "$population" == treatment ]]; then
					all_treatment_survival_valid=false
				else
					all_control_survival_valid=false
				fi
			fi
		fi
		cell_record=$(jq -n --arg cell "$population-$seed" --arg population "$population" --argjson seed "$seed" \
			--argjson integrity "$(jq '.predicates' "$cell/integrity.json")" \
			--argjson activation "$(jq '.result.predicates' "$cell/activation.json")" \
			--argjson observed_activation "$(jq '.result.observed.cdf_liquidity_activation_observed' "$cell/activation.json")" \
			--argjson terminal_measurement "$(if terminal_measurement_valid "$cell"; then printf true; else printf false; fi)" \
			--argjson terminal "$(if terminal_mark_valid "$cell"; then printf true; else printf false; fi)" \
			--argjson survival_measurement "$(if [[ -s "$output_root/survival-$(basename "$cell").json" ]]; then printf true; else printf false; fi)" \
			--argjson survival "$(if [[ -s "$output_root/survival-$(basename "$cell").json" ]]; then jq '.predicates' "$output_root/survival-$(basename "$cell").json"; else printf '{}'; fi)" \
			'{cell: $cell, population: $population, seed: $seed, integrity: $integrity, activation: $activation, cdf_liquidity_activation_observed: $observed_activation, terminal_measurement_valid: $terminal_measurement, terminal_strict_valuation: $terminal, survival_measurement_valid: $survival_measurement, survival: $survival}')
		pair_records=$(jq -c --argjson record "$cell_record" '. + [$record]' <<<"$pair_records")
	done

	treatment="$output_root/treatment-$seed"
	control="$output_root/control-$seed"
	audit_path="$output_root/cdf-liquidity-$seed.json"
	[[ ! -e "$audit_path" && ! -L "$audit_path" ]] || fail "refusing to overwrite CDF audit: $audit_path"
	audit_tmp=$(mktemp "$audit_path.tmp-XXXXXX")
	set +e
	"$cdf_audit" -treatment "$treatment" -control "$control" >"$audit_tmp"
	audit_status=$?
	set -e
	if ! jq -e 'type == "object"' "$audit_tmp" >/dev/null; then
		mv "$audit_tmp" "$audit_path.invalid"
		fail "CDF audit did not produce a JSON comparison for seed $seed (status $audit_status)"
	fi
	mv "$audit_tmp" "$audit_path"
	audit_contract_valid=false
	audit_anticheating_valid=false
	if jq -e --argjson expected "$registered_roster_count" '
		.valid == true and .provenance.valid == true and
		.treatment.valid == true and .control.valid == true and
		.treatment.supplier_count == $expected and .control.supplier_count == 0 and
		.treatment.trading_supplier_count == .treatment.supplier_count and
		.treatment.pnl_changing_supplier_count == .treatment.supplier_count and
		.treatment.inventory_responsive_decision_count > 0 and
		(.treatment.cancel_count + .treatment.withdraw_count) > 0 and
		.treatment.max_borrowed == 0 and
		(.treatment.supplier_volume_share <= 0.75) and
		(.treatment.supplier_depth_over_75_share <= 0.5) and
		(.treatment.venues | length) == 3 and
		all(.treatment.venues[]; .supplier_depth_over_75_fraction <= 0.5) and
		(.treatment.suppliers | length) == $expected and
		all(.treatment.suppliers[];
			.valid == true and .fill_count > 0 and .pnl != 0 and
			.inventory_responsive_decision_count > 0 and
			.max_position <= .configured_max_position and
			.min_position >= -.configured_max_position and
			.max_quote_qty <= .configured_max_quote_qty and
			.max_borrowed == 0)' "$audit_path" >/dev/null; then
		audit_contract_valid=true
		audit_anticheating_valid=true
	fi
	[[ "$audit_contract_valid" == true ]] || all_cdf_contract_valid=false
	[[ "$audit_anticheating_valid" == true ]] || all_anticheating_valid=false

	if ! jq -e --arg seed "$seed" \
		'.seed == ($seed | tonumber) and .valid == true and
		 .treatment.supplier_count > 0 and .control.supplier_count == 0' "$audit_path" >/dev/null; then
		all_cells_valid=false
	fi
done

all_measurements_valid=true
if [[ "$all_treatment_terminal_measurement_valid" != true ||
	"$all_control_terminal_measurement_valid" != true ||
	"$all_treatment_survival_measurement_valid" != true ||
	"$all_control_survival_measurement_valid" != true ]]; then
	all_measurements_valid=false
fi

parity_sha256=$(sha256sum "$parity" | awk '{print $1}')
raw_source_revision=$(jq -er '.raw_source_revision' "$output_root/treatment-607/analysis-metadata.json")
analysis_revision=$(jq -er '.analysis_revision' "$output_root/treatment-607/analysis-metadata.json")
analyzer_revision=$(jq -er '.analyzer_revision' "$output_root/treatment-607/analysis-metadata.json")
simulator_revision=$(jq -er '.simulator_revision' "$output_root/treatment-607/analysis-metadata.json")
analyzer_sha256=$(jq -er '.analyzer_sha256' "$output_root/treatment-607/analysis-metadata.json")
simulator_sha256=$(jq -er '.simulator_sha256' "$output_root/treatment-607/analysis-metadata.json")

mkdir -p -- "$output_root"
score_tmp=$(mktemp "$score.tmp-XXXXXX")
classification=$(jq -n \
	--argjson all_cells_valid "$all_cells_valid" \
	--argjson all_measurements_valid "$all_measurements_valid" \
	--argjson all_treatment_terminal_valid "$all_treatment_terminal_valid" \
	--argjson all_treatment_survival_valid "$all_treatment_survival_valid" \
	--argjson all_cdf_contract_valid "$all_cdf_contract_valid" \
	--argjson all_anticheating_valid "$all_anticheating_valid" \
	-f "$score_classification_filter") || fail "could not classify development score"
jq -n --arg contract "$contract_version" --arg candidate "V2-R2-SV1" \
	--arg predecessor "R2" --arg source_revision "$raw_source_revision" \
	--arg raw_source_revision "$raw_source_revision" --arg analysis_revision "$analysis_revision" \
	--arg analyzer_revision "$analyzer_revision" --arg analyzer_sha256 "$analyzer_sha256" \
	--arg simulator_revision "$simulator_revision" --arg simulator_sha256 "$simulator_sha256" \
	--arg parity_sha256 "$parity_sha256" --argjson cells "$pair_records" \
	--argjson all_cells_valid "$all_cells_valid" --argjson all_measurements_valid "$all_measurements_valid" \
	--argjson all_treatment_terminal_valid "$all_treatment_terminal_valid" \
	--argjson all_treatment_terminal_measurement_valid "$all_treatment_terminal_measurement_valid" \
	--argjson all_control_terminal_measurement_valid "$all_control_terminal_measurement_valid" \
	--argjson all_control_terminal_valid "$all_control_terminal_valid" \
	--argjson all_treatment_survival_valid "$all_treatment_survival_valid" \
	--argjson all_treatment_survival_measurement_valid "$all_treatment_survival_measurement_valid" \
	--argjson all_control_survival_valid "$all_control_survival_valid" \
	--argjson all_control_survival_measurement_valid "$all_control_survival_measurement_valid" \
	--argjson all_cdf_contract_valid "$all_cdf_contract_valid" \
	--argjson all_anticheating_valid "$all_anticheating_valid" --argjson expected_roster "$registered_roster_count" \
	--argjson holdouts '[619,631,641]' \
	--argjson classification "$classification" \
	'(
		{
			schema_version: 1, contract: $contract, candidate: $candidate, predecessor: $predecessor,
			status: $classification.status,
			claim_scope: "registered SV1 development qualification and CDF survival screen; no holdout or emergence claim",
			mechanism_hypothesis: "finite delayed-local CDF/USD suppliers may reduce persistent one-sided collapse while bearing finite inventory and PnL risk",
			 source_revision: $source_revision, raw_source_revision: $raw_source_revision,
			 analysis_revision: $analysis_revision, analyzer_revision: $analyzer_revision, analyzer_sha256: $analyzer_sha256,
			simulator_revision: $simulator_revision, simulator_sha256: $simulator_sha256,
			parity_attestation_sha256: $parity_sha256, registered_cdf_supplier_count: $expected_roster,
			development_seeds: [607,613,617], reserved_holdout_seeds: $holdouts,
			holdout_status: "RESERVED_AND_NOT_READ_BY_DEVELOPMENT_SCORER",
			predicates: {
				mechanical_and_artifact_contract: $all_cells_valid,
				measurement_contract: $all_measurements_valid,
				terminal_measurement_treatment: $all_treatment_terminal_measurement_valid,
				terminal_measurement_control: $all_control_terminal_measurement_valid,
				strict_terminal_valuation_treatment: $all_treatment_terminal_valid,
				strict_terminal_valuation_control_diagnostic: $all_control_terminal_valid,
				treatment_survival_measurement_valid: $all_treatment_survival_measurement_valid,
				post_warmup_cdf_side_availability_treatment: $all_treatment_survival_valid,
				control_survival_measurement_valid: $all_control_survival_measurement_valid,
				control_survival_predicate_diagnostic: $all_control_survival_valid,
				cdf_audit_contract: $all_cdf_contract_valid,
				anti_cheating_diagnostics: $all_anticheating_valid,
				parity_attested: true,
				holdout_outputs_absent: true,
				holdout_access_policy_enforced: true
			},
			interpretation: [
				"A viable development candidate is not freeze authorization.",
				"A non-viable status preserves a valid negative successor result and does not rewrite predecessor R2.",
				"An invalid-evidence status is not an economic negative result.",
				"Every primary treatment and control cell must produce valid terminal and survival measurements; a valid false control predicate remains diagnostic rather than gating treatment viability.",
				"Treatment qualification uses treatment terminal valuation and survival only; control population and endpoint outcomes remain diagnostics.",
				"The paired treatment-minus-control survival effect is not identified by this scorer unless a separately registered endpoint supplies an estimand and threshold."
			],
			cells: $cells,
			artifacts: {
				cdf_audits: ["cdf-liquidity-607.json", "cdf-liquidity-613.json", "cdf-liquidity-617.json"],
				survival_summaries: ["survival-treatment-607.json", "survival-control-607.json", "survival-treatment-613.json", "survival-control-613.json", "survival-treatment-617.json", "survival-control-617.json"],
				parity: "parity-attestation.json"
			}
		}
	)' >"$score_tmp" || fail "could not construct development score"
mv -- "$score_tmp" "$score"
require_object "$score"
jq -e --arg contract "$contract_version" --argjson holdouts '[619,631,641]' \
	'.schema_version == 1 and .contract == $contract and
	 .reserved_holdout_seeds == $holdouts and
	 .holdout_status == "RESERVED_AND_NOT_READ_BY_DEVELOPMENT_SCORER" and
		(.predicates | keys) == ["anti_cheating_diagnostics", "cdf_audit_contract", "control_survival_measurement_valid", "control_survival_predicate_diagnostic", "holdout_access_policy_enforced", "holdout_outputs_absent", "mechanical_and_artifact_contract", "measurement_contract", "parity_attested", "post_warmup_cdf_side_availability_treatment", "strict_terminal_valuation_control_diagnostic", "strict_terminal_valuation_treatment", "terminal_measurement_control", "terminal_measurement_treatment", "treatment_survival_measurement_valid"] and
	 .predicates.parity_attested == true and .predicates.holdout_outputs_absent == true and .predicates.holdout_access_policy_enforced == true' "$score" >/dev/null ||
	fail "development score self-check failed"
printf 'scored SV1 development: %s\n' "$(jq -r '.status' "$score")"
