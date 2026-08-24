#!/usr/bin/env bash
# Extract the V2-5 P3e passive-exit evidence contract for one completed cell.
# It never prunes raw evidence. Activation and causal scoring are deliberately
# separate from this mechanical extraction gate.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cell=${P3E_CELL:-"$root_dir/research/artifacts/v2-5-p3e/p0-B-107"}
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

if [[ ! -x "$analyzer" ]]; then
	echo "missing executable analyzer: $analyzer" >&2
	exit 1
fi
if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
	echo "incomplete V2-5 P3e cell (needs final greeks.json + latency.json): $cell" >&2
	exit 1
fi
if [[ ! -s "$cell/manifest.json" || ! -s "$cell/evidence-artifact-hash.json" ]]; then
	echo "missing V2-5 P3e provenance input: $cell" >&2
	exit 1
fi

write_metric() {
	local output=$1
	shift
	local temporary
	temporary=$(mktemp "${output}.tmp-XXXXXX")
	"$@" >"$temporary"
	mv "$temporary" "$output"
}

for metric in termcarry observationreceipts derivatives conservation positions orderlifecycle lifecycle streamhash evidenceartifacthash; do
	write_metric "$cell/${metric}.json" "$analyzer" -metric "$metric" -json "$cell"
done

# A P3e P0 term can intentionally still be economically open at the horizon,
# so this checks only independently reconstructible mechanical evidence. The
# registered activation predicate and any closure claim are scored elsewhere.
jq -e '
  .result.valid == true and .result.receipt_audit_valid == true and
  .result.receipt_evidence_errors == 0 and
  .result.source_mismatches == 0 and .result.future_source_use == 0 and
  .result.invalid_decision_records == 0 and .result.decision_field_mismatches == 0 and
  .result.arithmetic_mismatches == 0 and .result.missing_gateway_decisions == 0 and
  .result.gateway_decision_mismatches == 0 and .result.missing_venue_outcomes == 0 and
  .result.duplicate_venue_outcomes == 0 and .result.missing_actor_outcomes == 0 and
  .result.actor_outcome_mismatches == 0 and .result.lifecycle_violations == 0 and
  .result.position_continuity_errors == 0 and .result.terminal_perp_mismatches == 0 and
  .result.terminal_spot_mismatches == 0 and .result.first_exposure_mismatches == 0 and
  .result.outside_term_funding_settlements == 0 and
  .result.missing_passive_exit_cancellations == 0 and
  .result.passive_exit_cancellation_mismatches == 0
' "$cell/termcarry.json" >/dev/null
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
  .result.fills_after_terminal == 0 and .result.fill_quantity_mismatches == 0 and
  .result.cancel_quantity_mismatches == 0 and .result.client_mismatches == 0
' "$cell/orderlifecycle.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null

runtime_events=$(jq -r '.events' "$cell/evidence-artifact-hash.json")
runtime_digest=$(jq -r '.digest' "$cell/evidence-artifact-hash.json")
offline_events=$(jq -r '.result.events' "$cell/evidenceartifacthash.json")
offline_digest=$(jq -r '.result.digest' "$cell/evidenceartifacthash.json")
if [[ "$runtime_events" != "$offline_events" || "$runtime_digest" != "$offline_digest" ]]; then
	echo "runtime/offline evidence artifact digest mismatch: $cell" >&2
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
	  completion_sentinels: ["greeks.json", "latency.json"],
	  required_artifacts: [
	    "termcarry.json", "observationreceipts.json", "derivatives.json",
	    "conservation.json", "positions.json", "orderlifecycle.json",
	    "lifecycle.json", "streamhash.json", "evidenceartifacthash.json"
	  ],
	  runtime_evidence_artifact: {
	    events: $runtime_evidence_events,
	    digest: $runtime_evidence_digest
	  },
	  raw_log_policy: "retained; P3e evidence is never pruned by this extractor"
	}' >"$cell/analysis-metadata.json"

echo "extracted V2-5 P3e evidence: $cell"
