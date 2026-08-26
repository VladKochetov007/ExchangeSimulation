#!/usr/bin/env bash
# Fail-closed extraction for one completed V2-6 P6-R1 cross-asset-mark cell. This
# records every registered option/evidence metric and has no prune authority.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 CELL_DIR" >&2
	exit 2
fi
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cell=$1
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

if [[ ! -x "$analyzer" || ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
	echo "incomplete P6-R1 cell or analyzer: $cell" >&2
	exit 1
fi
for input in manifest.json evidence-artifact-hash.json run-config.json run-metadata.json; do
	if [[ ! -s "$cell/$input" ]]; then
		echo "missing P6-R1 provenance input $input: $cell" >&2
		exit 1
	fi
done

stage=$(jq -er '.stage' "$cell/run-metadata.json")
case "$stage" in O0|O1|O2|O3|O4) ;; *) echo "invalid P6-R1 stage in run metadata: $stage" >&2; exit 1 ;; esac
jq -e '.cross_asset_collateral_marks == true and .cross_asset_spot_graph == true' "$cell/run-config.json" >/dev/null
jq -e '.hypothesis_id == "V2-6-P6R1-CROSS-ASSET-MARK"' "$cell/run-metadata.json" >/dev/null

write_metric() {
	local output=$1
	shift
	local temporary
	temporary=$(mktemp "${output}.tmp-XXXXXX")
	"$@" >"$temporary" 2>"$output.err"
	mv "$temporary" "$output"
}

metrics=(
	optionliabilityp6 optionvaluetakerp6 vannavolgap6 optionsurface exposure hedging
	roleaudit observationreceipts derivatives conservation positions fillpositions
	orderlifecycle lifecycle settlements expiryfills streamhash evidenceartifacthash
)
for metric in "${metrics[@]}"; do
	write_metric "$cell/$metric.json" "$analyzer" -metric "$metric" -json "$cell"
done

# Cross-asset viability is checked from the persisted venue evidence rather
# than inferred from a terminal account alone.  A CDF borrow proves the new
# oracle path was exercised; a CDF/USD PRICE_UNAVAILABLE order rejection is
# the exact historical failure and invalidates this cell's viability contract.
count_events() {
	local filter=$1
	local total=0 file count
	while IFS= read -r -d '' file; do
		count=$(jq -r "$filter" "$file" | wc -l)
		total=$((total + count))
	done < <(find "$cell/venues" -type f -name '*.jsonl' -print0)
	printf '%d' "$total"
}
cdf_borrow_events=$(count_events 'select(.event == "borrow" and .data.payload.asset == "CDF") | 1')
cdf_price_unavailable_rejections=$(count_events 'select(.event == "OrderRejected" and .data.payload.symbol == "CDF/USD" and .data.payload.error == "PRICE_UNAVAILABLE") | 1')
jq -n \
	--argjson cdf_borrow_events "$cdf_borrow_events" \
	--argjson cdf_price_unavailable_rejections "$cdf_price_unavailable_rejections" \
	'{schema_version: 1, result: {
	  cdf_borrow_events: $cdf_borrow_events,
	  cdf_price_unavailable_rejections: $cdf_price_unavailable_rejections,
	  collateral_mark_contract: "explicit CDF/USD bootstrap mark plus finite borrow cap"
	}}' >"$cell/crossassetmark.json"
if [[ "$cdf_price_unavailable_rejections" != 0 ]]; then
	echo "P6-R1 CDF collateral viability failed: $cdf_price_unavailable_rejections PRICE_UNAVAILABLE CDF/USD rejections" >&2
	exit 1
fi

# Evidence integrity is independent of whether a registered participant or
# option surface was active. The latter is scored as NOT EXERCISED.
jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
jq -e '.result.delta_consistency.mismatched == 0 and .result.delta_consistency.chain_broken == 0 and .result.delta_consistency.decode_failures == 0' "$cell/conservation.json" >/dev/null
jq -e '.result.disagreement == 0 and .result.unrepresentable_open_values == 0' "$cell/positions.json" >/dev/null
jq -e '.result.missing_position_update == 0 and .result.unexpected_position_update == 0 and .result.position_chain_failures == 0' "$cell/fillpositions.json" >/dev/null
jq -e '.result.unknown_fills == 0 and .result.unknown_cancellations == 0 and .result.duplicate_acceptances == 0 and .result.duplicate_terminals == 0 and .result.missing_immediate_terminal == 0 and .result.fills_after_terminal == 0 and .result.fill_quantity_mismatches == 0 and .result.cancel_quantity_mismatches == 0 and .result.client_mismatches == 0' "$cell/orderlifecycle.json" >/dev/null
jq -e '.result.mismatched == 0 and .result.unpaid == 0 and .result.total_trades_after_expiry == 0' "$cell/settlements.json" >/dev/null
jq -e '.result.fills_after_expiry == 0 and .result.missing_expiry_metadata == 0 and .result.settlement_without_listing == 0 and .result.metadata_mismatches == 0' "$cell/expiryfills.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null
jq -e '.result.trades > 0 and .result.priced > 0' "$cell/optionsurface.json" >/dev/null

# If the liability actor emitted rows, every row must pass its independent
# join/frontier audit. A zero-row stage remains a registered inactive outcome.
if [[ "$(jq -r '.result.decisions' "$cell/optionliabilityp6.json")" -gt 0 ]]; then
	jq -e '.result.valid == true' "$cell/optionliabilityp6.json" >/dev/null
fi

runtime_events=$(jq -r '.events' "$cell/evidence-artifact-hash.json")
runtime_digest=$(jq -r '.digest' "$cell/evidence-artifact-hash.json")
offline_events=$(jq -r '.result.events' "$cell/evidenceartifacthash.json")
offline_digest=$(jq -r '.result.digest' "$cell/evidenceartifacthash.json")
if [[ "$runtime_events" != "$offline_events" || "$runtime_digest" != "$offline_digest" ]]; then
	echo "runtime/offline P6-R1 evidence artifact digest mismatch: $cell" >&2
	exit 1
fi

jq -n \
	--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
	--arg stage "$stage" \
	--arg runtime_evidence_digest "$runtime_digest" \
	--argjson runtime_evidence_events "$runtime_events" \
	'{
	  analysis_revision: $analysis_revision,
	  analyzer_sha256: $analyzer_sha256,
	  analysis_contract: "v2-6-p6r1-cross-asset-mark-v1",
	  stage: $stage,
	  completion_sentinels: ["greeks.json", "latency.json"],
	  required_artifacts: [
	"optionliabilityp6.json", "optionvaluetakerp6.json", "vannavolgap6.json",
	"crossassetmark.json",
	    "optionsurface.json", "exposure.json", "hedging.json", "roleaudit.json",
	    "observationreceipts.json", "derivatives.json", "conservation.json",
	    "positions.json", "fillpositions.json", "orderlifecycle.json", "lifecycle.json",
	    "settlements.json", "expiryfills.json", "streamhash.json", "evidenceartifacthash.json"
	  ],
	  runtime_evidence_artifact: {events: $runtime_evidence_events, digest: $runtime_evidence_digest},
	  raw_log_policy: "retained; this extractor has no prune authority"
	}' >"$cell/analysis-metadata.json"

echo "extracted V2-6 P6-R1 evidence: $cell"
