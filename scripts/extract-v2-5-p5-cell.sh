#!/usr/bin/env bash
# Fail-closed extraction for one completed P5 cell. This script scores no
# economic endpoint and has no prune authority.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 CELL_DIR" >&2
	exit 2
fi
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cell=$1
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

if [[ ! -x "$analyzer" || ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
	echo "incomplete P5 cell or analyzer: $cell" >&2
	exit 1
fi
for input in manifest.json evidence-artifact-hash.json run-config.json run-metadata.json; do
	if [[ ! -s "$cell/$input" ]]; then
		echo "missing P5 provenance input $input: $cell" >&2
		exit 1
	fi
done

write_metric() {
	local output=$1
	shift
	local temporary
	temporary=$(mktemp "${output}.tmp-XXXXXX")
	"$@" >"$temporary" 2>"$output.err"
	mv "$temporary" "$output"
}

metrics=(datedcarryp5 datedmandatep5 observationreceipts derivatives conservation positions fillpositions orderlifecycle lifecycle settlements expiryfills streamhash evidenceartifacthash)
for metric in "${metrics[@]}"; do
	write_metric "$cell/$metric.json" "$analyzer" -metric "$metric" -json "$cell"
done

jq -e '.result.valid == true and .result.receipt_audit_valid == true and .result.role_links_active == true' "$cell/datedcarryp5.json" >/dev/null
jq -e '.result.valid == true and .result.receipt_audit_valid == true and .result.role_links_active == true' "$cell/datedmandatep5.json" >/dev/null
jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
jq -e '.result.delta_consistency.mismatched == 0 and .result.delta_consistency.chain_broken == 0 and .result.delta_consistency.decode_failures == 0' "$cell/conservation.json" >/dev/null
jq -e '.result.disagreement == 0 and .result.unrepresentable_open_values == 0' "$cell/positions.json" >/dev/null
jq -e '.result.missing_position_update == 0 and .result.unexpected_position_update == 0 and .result.position_chain_failures == 0' "$cell/fillpositions.json" >/dev/null
jq -e '.result.unknown_fills == 0 and .result.unknown_cancellations == 0 and .result.duplicate_acceptances == 0 and .result.duplicate_terminals == 0 and .result.missing_immediate_terminal == 0 and .result.fills_after_terminal == 0 and .result.fill_quantity_mismatches == 0 and .result.cancel_quantity_mismatches == 0 and .result.client_mismatches == 0' "$cell/orderlifecycle.json" >/dev/null
jq -e '.result.mismatched == 0 and .result.unpaid == 0 and .result.total_trades_after_expiry == 0' "$cell/settlements.json" >/dev/null
jq -e '.result.fills_after_expiry == 0 and .result.missing_expiry_metadata == 0 and .result.settlement_without_listing == 0 and .result.metadata_mismatches == 0' "$cell/expiryfills.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null

runtime_events=$(jq -r '.events' "$cell/evidence-artifact-hash.json")
runtime_digest=$(jq -r '.digest' "$cell/evidence-artifact-hash.json")
offline_events=$(jq -r '.result.events' "$cell/evidenceartifacthash.json")
offline_digest=$(jq -r '.result.digest' "$cell/evidenceartifacthash.json")
if [[ "$runtime_events" != "$offline_events" || "$runtime_digest" != "$offline_digest" ]]; then
	echo "runtime/offline P5 evidence artifact digest mismatch: $cell" >&2
	exit 1
fi

jq -n \
	--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
	--arg runtime_evidence_digest "$runtime_digest" \
	--argjson runtime_evidence_events "$runtime_events" \
	'{
	  analysis_revision: $analysis_revision,
	  analyzer_sha256: $analyzer_sha256,
	  analysis_contract: "v2-5-p5-dated-carry-v1",
	  completion_sentinels: ["greeks.json", "latency.json"],
	  required_artifacts: [
	    "datedcarryp5.json", "datedmandatep5.json", "observationreceipts.json",
	    "derivatives.json", "conservation.json", "positions.json", "fillpositions.json",
	    "orderlifecycle.json", "lifecycle.json", "settlements.json", "expiryfills.json",
	    "streamhash.json", "evidenceartifacthash.json"
	  ],
	  runtime_evidence_artifact: {events: $runtime_evidence_events, digest: $runtime_evidence_digest},
	  raw_log_policy: "retained; this extractor has no prune authority"
	}' >"$cell/analysis-metadata.json"
echo "extracted P5 evidence: $cell"
