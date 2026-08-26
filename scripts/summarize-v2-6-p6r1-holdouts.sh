#!/usr/bin/env bash
# Render the immutable P6-R1 untouched-holdout ledger from validated artifacts.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
outroot=${P6R1_OUTROOT:-"$root_dir/research/artifacts/v2-6-p6r1"}
output=${P6R1_SUMMARY_OUT:-"$outroot/holdout-summary.json"}
cells=(
	O0-223 O0-227 O0-229
	O1-223 O1-227 O1-229
	O2-223 O2-227 O2-229
	O3-223 O3-227 O3-229
	O4-223 O4-227 O4-229
)

"$root_dir/scripts/check-v2-6-p6r1-holdouts.sh" >/dev/null

first=${cells[0]}
simulator_revision=$(jq -er '.git_revision' "$outroot/$first/run-metadata.json")
binary_sha256=$(jq -er '.binary_sha256' "$outroot/$first/run-metadata.json")
analysis_revision=$(jq -er '.analysis_revision' "$outroot/$first/analysis-metadata.json")
analyzer_sha256=$(jq -er '.analyzer_sha256' "$outroot/$first/analysis-metadata.json")
for cell in "${cells[@]}"; do
	[[ $(jq -er '.git_revision' "$outroot/$cell/run-metadata.json") == "$simulator_revision" ]] || {
		echo "P6-R1 holdout source revision mismatch: $cell" >&2
		exit 1
	}
	[[ $(jq -er '.binary_sha256' "$outroot/$cell/run-metadata.json") == "$binary_sha256" ]] || {
		echo "P6-R1 holdout binary mismatch: $cell" >&2
		exit 1
	}
	[[ $(jq -er '.analysis_revision' "$outroot/$cell/analysis-metadata.json") == "$analysis_revision" ]] || {
		echo "P6-R1 holdout analysis revision mismatch: $cell" >&2
		exit 1
	}
	[[ $(jq -er '.analyzer_sha256' "$outroot/$cell/analysis-metadata.json") == "$analyzer_sha256" ]] || {
		echo "P6-R1 holdout analyzer mismatch: $cell" >&2
		exit 1
	}
done

cells_json=$(mktemp "$output.cells-XXXXXX")
trap 'rm -f "$cells_json"' EXIT
for cell in "${cells[@]}"; do
	stage=$(jq -er '.stage' "$outroot/$cell/run-metadata.json")
	seed=$(jq -er '.seed' "$outroot/$cell/run-metadata.json")
	config_sha256=$(jq -er '.config_sha256' "$outroot/$cell/run-metadata.json")
	runtime_events=$(jq -er '.events' "$outroot/$cell/evidence-artifact-hash.json")
	runtime_digest=$(jq -er '.digest' "$outroot/$cell/evidence-artifact-hash.json")
	offline_events=$(jq -er '.result.events' "$outroot/$cell/evidenceartifacthash.json")
	offline_digest=$(jq -er '.result.digest' "$outroot/$cell/evidenceartifacthash.json")
	cdf=$(jq -c '.result' "$outroot/$cell/crossassetmark.json")
	surface=$(jq -c '.result | {trades,priced,points:(.points|length)}' "$outroot/$cell/optionsurface.json")
	lifecycle=$(jq -c '.result | {listings,settlements}' "$outroot/$cell/lifecycle.json")
	liability=$(jq -c '.result | {participants,decisions,submit_decisions,deferred_decisions,accepted,rejected,canonical_fills,filled_qty,target_reached,valid}' "$outroot/$cell/optionliabilityp6.json")
	value_taker=$(jq -c '.result | {participants,decisions,accepted,rejected,fills,filled_qty}' "$outroot/$cell/optionvaluetakerp6.json")
	vanna_volga=$(jq -c '.result | {participants,decisions,accepted,rejected,fills,filled_qty}' "$outroot/$cell/vannavolgap6.json")
	exposure=$(jq -c '.result | {pooled_hedge_ratio,pooled_mean_abs_net_delta,pooled_max_abs_net_delta,pooled_mean_abs_vega,pooled_mean_abs_vanna,pooled_mean_abs_volga,pooled_correlation,transmission,hedge_flows}' "$outroot/$cell/exposure.json")
	jq -n \
		--arg cell "$cell" --arg stage "$stage" --argjson seed "$seed" \
		--arg config_sha256 "$config_sha256" --argjson evidence_events "$runtime_events" \
		--arg evidence_digest "$runtime_digest" --argjson runtime_events "$runtime_events" \
		--arg runtime_digest "$runtime_digest" --argjson offline_events "$offline_events" \
		--arg offline_digest "$offline_digest" --argjson cdf "$cdf" \
		--argjson surface "$surface" --argjson lifecycle "$lifecycle" \
		--argjson liability "$liability" --argjson value_taker "$value_taker" \
		--argjson vanna_volga "$vanna_volga" --argjson exposure "$exposure" \
		'{cell:$cell,stage:$stage,seed:$seed,config_sha256:$config_sha256,
		 evidence_events:$evidence_events,evidence_digest:$evidence_digest,
		 runtime_offline_equal:($runtime_events==$offline_events and $runtime_digest==$offline_digest),
		 cdf_collateral:$cdf,surface:$surface,lifecycle:$lifecycle,
		 liability:$liability,value_taker:$value_taker,vanna_volga:$vanna_volga,
		 exposure:$exposure}' >>"$cells_json"
done

temporary=$(mktemp "${output}.tmp-XXXXXX")
jq -n \
	--arg simulator_revision "$simulator_revision" --arg binary_sha256 "$binary_sha256" \
	--arg analysis_revision "$analysis_revision" --arg analyzer_sha256 "$analyzer_sha256" \
	--slurpfile cells "$cells_json" \
	'{schema_version:1,
	 experiment_id:"v2-6-p6r1-cross-asset-mark",
	 hypothesis_id:"V2-6-P6R1-CROSS-ASSET-MARK",
	 contract:"v2-6-p6r1-cross-asset-mark-v1",
	 classification:"SUPPORTED_SCREENING",
	 scope:"untouched_holdout_viability_and_stage_activation_only",
	 simulator_revision:$simulator_revision,binary_sha256:$binary_sha256,
	 analysis_revision:$analysis_revision,analyzer_sha256:$analyzer_sha256,
	 horizon:"8h",development_seeds:[211,213],holdout_seeds:[223,227,229],
	 config_hashes_pinned:true,all_required_artifacts_valid:true,
	 runtime_offline_evidence_digest_equal:true,
	 cdf_collateral_mark:{enabled:true,bootstrap_price:3000,borrow_cap_units:20000,
	   participant_visible:false,purpose:"collateral_authorization_and_accounting_only"},
	 raw_log_policy:"retained; no P6-R1 prune authority",
	 cells:$cells}' "$cells_json" >"$temporary"
mv "$temporary" "$output"
echo "rendered P6-R1 holdout summary: $output"
