#!/usr/bin/env bash
# Independent, read-only contract check for the P6-R1 untouched holdouts.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
outroot=${P6R1_OUTROOT:-"$root_dir/research/artifacts/v2-6-p6r1"}
cells=(
	O0-223 O0-227 O0-229
	O1-223 O1-227 O1-229
	O2-223 O2-227 O2-229
	O3-223 O3-227 O3-229
	O4-223 O4-227 O4-229
)

fail() {
	echo "P6-R1 holdout contract failure: $*" >&2
	exit 1
}

for cell in "${cells[@]}"; do
	cell_dir="$outroot/$cell"
	[[ -f "$outroot/$cell.extract.status" ]] || fail "$cell missing extraction status"
	[[ $(<"$outroot/$cell.extract.status") == 0 ]] || fail "$cell extraction status is not zero"
	[[ -s "$cell_dir/greeks.json" && -s "$cell_dir/latency.json" ]] || fail "$cell missing completion sentinels"

	for input in manifest.json evidence-artifact-hash.json run-config.json run-metadata.json analysis-metadata.json; do
		[[ -s "$cell_dir/$input" ]] || fail "$cell missing $input"
	done
	stage=$(jq -er '.stage' "$cell_dir/run-metadata.json") || fail "$cell invalid stage"
	seed=$(jq -er '.seed' "$cell_dir/run-metadata.json") || fail "$cell invalid seed"
	case "$stage:$seed" in
		O0:223|O0:227|O0:229|O1:223|O1:227|O1:229|O2:223|O2:227|O2:229|O3:223|O3:227|O3:229|O4:223|O4:227|O4:229) ;;
		*) fail "$cell unexpected stage/seed $stage/$seed" ;;
	esac
	jq -e \
		'.hypothesis_id == "V2-6-P6R1-CROSS-ASSET-MARK" and
		 .simulated_horizon == "8h0m0s" and
		 (.binary_sha256 | length) == 64 and
		 (.git_revision | length) == 40 and
		 (.config_sha256 | length) == 64 and
		 (.completion_sentinels == ["greeks.json", "latency.json"])' \
		"$cell_dir/run-metadata.json" >/dev/null || fail "$cell provenance metadata invalid"
	jq -e \
		'.analysis_contract == "v2-6-p6r1-cross-asset-mark-v1" and
		 (.required_artifacts | length) == 19 and
		 (.runtime_evidence_artifact.events > 0) and
		 (.runtime_evidence_artifact.digest | length) == 64 and
		 (.completion_sentinels == ["greeks.json", "latency.json"])' \
		"$cell_dir/analysis-metadata.json" >/dev/null || fail "$cell analysis metadata invalid"

	for artifact in observationreceipts conservation positions fillpositions orderlifecycle settlements expiryfills streamhash evidenceartifacthash optionsurface crossassetmark; do
		[[ -s "$cell_dir/$artifact.json" ]] || fail "$cell missing $artifact.json"
	done
	jq -e '.result.valid == true' "$cell_dir/observationreceipts.json" >/dev/null || fail "$cell receipt/frontier audit invalid"
	jq -e '.result.delta_consistency.mismatched == 0 and .result.delta_consistency.chain_broken == 0 and .result.delta_consistency.decode_failures == 0' "$cell_dir/conservation.json" >/dev/null || fail "$cell conservation audit invalid"
	jq -e '.result.disagreement == 0 and .result.unrepresentable_open_values == 0' "$cell_dir/positions.json" >/dev/null || fail "$cell position audit invalid"
	jq -e '.result.missing_position_update == 0 and .result.unexpected_position_update == 0 and .result.position_chain_failures == 0' "$cell_dir/fillpositions.json" >/dev/null || fail "$cell fill-position audit invalid"
	jq -e '.result.unknown_fills == 0 and .result.unknown_cancellations == 0 and .result.duplicate_acceptances == 0 and .result.duplicate_terminals == 0 and .result.missing_immediate_terminal == 0 and .result.fills_after_terminal == 0 and .result.fill_quantity_mismatches == 0 and .result.cancel_quantity_mismatches == 0 and .result.client_mismatches == 0' "$cell_dir/orderlifecycle.json" >/dev/null || fail "$cell order lifecycle audit invalid"
	jq -e '.result.mismatched == 0 and .result.unpaid == 0 and .result.total_trades_after_expiry == 0' "$cell_dir/settlements.json" >/dev/null || fail "$cell settlement audit invalid"
	jq -e '.result.fills_after_expiry == 0 and .result.missing_expiry_metadata == 0 and .result.settlement_without_listing == 0 and .result.metadata_mismatches == 0' "$cell_dir/expiryfills.json" >/dev/null || fail "$cell expiry audit invalid"
	jq -e '.result.events > 0 and (.result.digest | length) == 64' "$cell_dir/streamhash.json" >/dev/null || fail "$cell stream hash invalid"
	jq -e '.result.events > 0 and (.result.digest | length) == 64' "$cell_dir/evidenceartifacthash.json" >/dev/null || fail "$cell evidence hash invalid"
	jq -e '.result.trades > 0 and .result.priced > 0 and (.result.points | length) > 0' "$cell_dir/optionsurface.json" >/dev/null || fail "$cell option surface inactive"
	jq -e '.result.cdf_borrow_events > 0 and .result.cdf_price_unavailable_rejections == 0' "$cell_dir/crossassetmark.json" >/dev/null || fail "$cell cross-asset mark path invalid"

	runtime_events=$(jq -er '.events' "$cell_dir/evidence-artifact-hash.json")
	runtime_digest=$(jq -er '.digest' "$cell_dir/evidence-artifact-hash.json")
	offline_events=$(jq -er '.result.events' "$cell_dir/evidenceartifacthash.json")
	offline_digest=$(jq -er '.result.digest' "$cell_dir/evidenceartifacthash.json")
	[[ "$runtime_events" == "$offline_events" && "$runtime_digest" == "$offline_digest" ]] || fail "$cell runtime/offline evidence digest mismatch"

	case "$stage" in
		O0|O1)
			jq -e '.result.decisions == 0 and .result.filled_qty == 0' "$cell_dir/optionliabilityp6.json" >/dev/null || fail "$cell inactive liability population changed"
			jq -e '.result.decisions == 0 and .result.filled_qty == 0' "$cell_dir/optionvaluetakerp6.json" >/dev/null || fail "$cell inactive value-taker population changed"
			jq -e '.result.decisions == 0 and .result.filled_qty == 0' "$cell_dir/vannavolgap6.json" >/dev/null || fail "$cell inactive VV population changed"
			;;
		O2)
			jq -e '.result.decisions > 0 and .result.canonical_fills > 0 and .result.target_reached == true and .result.valid == true' "$cell_dir/optionliabilityp6.json" >/dev/null || fail "$cell liability activation invalid"
			;;
		O3)
			jq -e '.result.decisions > 0 and .result.fills > 0 and .result.filled_qty > 0' "$cell_dir/optionvaluetakerp6.json" >/dev/null || fail "$cell value-taker activation invalid"
			;;
		O4)
			jq -e '.result.decisions > 0 and .result.fills > 0 and .result.filled_qty > 0' "$cell_dir/optionvaluetakerp6.json" >/dev/null || fail "$cell value-taker activation invalid"
			jq -e '.result.decisions > 0 and .result.fills > 0 and .result.filled_qty > 0' "$cell_dir/vannavolgap6.json" >/dev/null || fail "$cell VV activation invalid"
			;;
	esac
	printf '%s OK events=%s cdf_borrow=%s surface=%s/%s/%s\n' \
		"$cell" "$runtime_events" \
		"$(jq -er '.result.cdf_borrow_events' "$cell_dir/crossassetmark.json")" \
		"$(jq -er '.result.trades' "$cell_dir/optionsurface.json")" \
		"$(jq -er '.result.priced' "$cell_dir/optionsurface.json")" \
		"$(jq -er '.result.points | length' "$cell_dir/optionsurface.json")"
done

echo "all ${#cells[@]} P6-R1 holdout contracts passed"
