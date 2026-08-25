#!/usr/bin/env bash
# Fail-closed extraction for one completed immutable V2-5 P4 cell. This script
# never scores the funding treatment and never prunes raw evidence.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 CELL_DIR" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cell=$1
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
deadline=1736038805000000000

if [[ ! -x "$analyzer" ]]; then
	echo "missing executable analyzer: $analyzer" >&2
	exit 1
fi
if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
	echo "incomplete P4 cell (requires final greeks.json + latency.json): $cell" >&2
	exit 1
fi
for input in manifest.json evidence-artifact-hash.json run-config.json run-metadata.json; do
	if [[ ! -s "$cell/$input" ]]; then
		echo "missing P4 provenance input $input: $cell" >&2
		exit 1
	fi
done

write_metric() {
	local output=$1
	shift
	local temporary
	temporary=$(mktemp "${output}.tmp-XXXXXX")
	"$@" >"$temporary"
	mv "$temporary" "$output"
}

metrics=(termcarryp4chain termcarry observationreceipts derivatives conservation positions orderlifecycle lifecycle streamhash evidenceartifacthash)
for metric in "${metrics[@]}"; do
	write_metric "$cell/$metric.json" "$analyzer" -metric "$metric" -json "$cell"
done
write_metric "$cell/termcarrylifecycle.json" "$analyzer" -metric termcarrylifecycle -term-carry-lifecycle-deadline "$deadline" -json "$cell"

jq -e '
  .result.valid == true and .result.base_audit.valid == true and
  .result.exact_cost_decisions_evaluated > 0 and
  (.result.checks | length) == 0
' "$cell/termcarryp4chain.json" >/dev/null
jq -e '.result.valid == true and .result.receipt_audit_valid == true' "$cell/termcarry.json" >/dev/null
jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
jq -e '
  .result.funding_broken == 0 and .result.funding_sign_wrong == 0 and
  .result.funding_misdirected == 0 and .result.funding_undirected == 0 and
  .result.funding_duplicate_payments == 0 and .result.exercise_broken == 0 and
  .result.holders_mispaid == 0 and .result.worthless_paid == 0 and
  .result.exercise_arithmetic_failures == 0
' "$cell/derivatives.json" >/dev/null
jq -e '
  .result.delta_consistency.mismatched == 0 and
  .result.delta_consistency.chain_broken == 0 and
  .result.delta_consistency.decode_failures == 0
' "$cell/conservation.json" >/dev/null
jq -e '.result.disagreement == 0 and .result.unrepresentable_open_values == 0' "$cell/positions.json" >/dev/null
jq -e '
  .result.unknown_fills == 0 and .result.unknown_cancellations == 0 and
  .result.duplicate_acceptances == 0 and .result.duplicate_terminals == 0 and
  .result.missing_immediate_terminal == 0 and .result.fills_after_terminal == 0 and
  .result.fill_quantity_mismatches == 0 and .result.cancel_quantity_mismatches == 0 and
  .result.client_mismatches == 0
' "$cell/orderlifecycle.json" >/dev/null
jq -e '
  .result.integrity_valid == true and .result.analysis_deadline_at_nano == 1736038805000000000 and
  .result.observation_end_at_nano >= .result.analysis_deadline_at_nano and
  (.result.integrity_failures | length) == 0
' "$cell/termcarrylifecycle.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null

runtime_events=$(jq -r '.events' "$cell/evidence-artifact-hash.json")
runtime_digest=$(jq -r '.digest' "$cell/evidence-artifact-hash.json")
offline_events=$(jq -r '.result.events' "$cell/evidenceartifacthash.json")
offline_digest=$(jq -r '.result.digest' "$cell/evidenceartifacthash.json")
if [[ "$runtime_events" != "$offline_events" || "$runtime_digest" != "$offline_digest" ]]; then
	echo "runtime/offline P4 evidence artifact digest mismatch: $cell" >&2
	exit 1
fi

jq -n \
	--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
	--arg runtime_evidence_digest "$runtime_digest" \
	--argjson runtime_evidence_events "$runtime_events" \
	--argjson analysis_deadline_at_nano "$deadline" \
	'{
	  analysis_revision: $analysis_revision,
	  analyzer_sha256: $analyzer_sha256,
	  analysis_contract: "v2-5-p4-funding-carry-v1",
	  analysis_deadline_at_nano: $analysis_deadline_at_nano,
	  completion_sentinels: ["greeks.json", "latency.json"],
	  required_artifacts: [
	    "termcarryp4chain.json", "termcarry.json", "termcarrylifecycle.json",
	    "observationreceipts.json", "derivatives.json", "conservation.json",
	    "positions.json", "orderlifecycle.json", "lifecycle.json",
	    "streamhash.json", "evidenceartifacthash.json"
	  ],
	  runtime_evidence_artifact: {events: $runtime_evidence_events, digest: $runtime_evidence_digest},
	  raw_log_policy: "retained; this extractor has no prune authority"
	}' >"$cell/analysis-metadata.json"

echo "extracted V2-5 P4 evidence: $cell"
