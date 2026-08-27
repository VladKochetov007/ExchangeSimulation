#!/usr/bin/env bash
# Fail-closed extraction for one completed integrated V2 long-run candidate.
# This is an analyzer/evidence operation only; it has no prune authority.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 CELL_DIR" >&2
  exit 2
fi
root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cell=$1
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

[[ -x "$analyzer" ]] || { echo "missing analyzer: $analyzer" >&2; exit 1; }
for sentinel in greeks.json latency.json; do
  [[ -s "$cell/$sentinel" ]] || { echo "missing completion sentinel $sentinel" >&2; exit 1; }
done
for input in manifest.json evidence-artifact-hash.json run-config.json run-metadata.json; do
  [[ -s "$cell/$input" ]] || { echo "missing provenance input $input" >&2; exit 1; }
done

seed=$(jq -er '.seed' "$cell/run-metadata.json")
case "$seed" in 607|613|617) ;; *) echo "unexpected development seed $seed" >&2; exit 1 ;; esac
jq -e '.hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE" and .holdout == false and .simulated_horizon == "24h"' "$cell/run-metadata.json" >/dev/null
jq -e '.hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE" and .log_mode == "full"' "$cell/run-config.json" >/dev/null

write_metric() {
  local output=$1
  shift
  local temporary
  temporary=$(mktemp "${output}.tmp-XXXXXX")
  if ! "$@" >"$temporary" 2>"$output.err"; then
    rm -f "$temporary"
    return 1
  fi
  mv "$temporary" "$output"
}

# Run metrics sequentially to bound analyzer memory and storage contention on
# multi-gigabyte cells.
metrics=(
  observationreceipts frontiervectors mechanical conservation positions
  fillpositions orderlifecycle lifecycle settlements expiryfills
  evidenceartifacthash streamhash arbitrage crossvenue roleaudit ecology
  derivatives liquidations marginchecks optionsurface optionliabilityp6
  optionvaluetakerp6 vannavolgap6 exposure hedging makerrefresh makerquotesize
  makerrebalance postonly liabilityhedger perpsignals
)
for metric in "${metrics[@]}"; do
  write_metric "$cell/$metric.json" "$analyzer" -metric "$metric" -json "$cell"
done

# The integrated reference intentionally does not register P4/P5 actor
# decision receipts or P3 replenishment receipts. Preserve explicit,
# machine-readable NOT_EXERCISED artifacts rather than silently omitting the
# metrics or pretending that a missing policy is a zero economic response.
write_inactive() {
  local metric=$1 field=$2 reason=$3
  local temporary
  temporary=$(mktemp "$cell/$metric.json.tmp-XXXXXX")
  jq -n --arg metric "$metric" --arg field "$field" --arg reason "$reason" \
    '{schema_version: 1, result: {status: "NOT_EXERCISED", metric: $metric, config_field: $field, reason: $reason, observations: 0}}' \
    >"$temporary"
  mv "$temporary" "$cell/$metric.json"
}

if [[ "$(jq -r '.record_funding_carry_decisions' "$cell/run-config.json")" != true ]]; then
  write_inactive fundingcarry record_funding_carry_decisions "registered integrated composition does not enable P4 actor decision receipts"
else
  write_metric "$cell/fundingcarry.json" "$analyzer" -metric fundingcarry -json "$cell"
fi
if [[ "$(jq -r '.record_term_carry_decisions' "$cell/run-config.json")" != true ]]; then
  write_inactive termcarry record_term_carry_decisions "registered integrated composition does not enable P5 actor decision receipts"
else
  write_metric "$cell/termcarry.json" "$analyzer" -metric termcarry -json "$cell"
fi
if [[ "$(jq -r '.record_dated_term_carry_decisions' "$cell/run-config.json")" != true ]]; then
  write_inactive datedcarryp5 record_dated_term_carry_decisions "registered integrated composition does not enable P5 dated-carry decision receipts"
else
  write_metric "$cell/datedcarryp5.json" "$analyzer" -metric datedcarryp5 -json "$cell"
fi
if [[ "$(jq -r '.record_perp_maker_replenishment_decisions' "$cell/run-config.json")" != true ]]; then
  write_inactive perpreplenishment record_perp_maker_replenishment_decisions "registered integrated composition does not enable P3 replenishment receipts"
else
  write_metric "$cell/perpreplenishment.json" "$analyzer" -metric perpreplenishment -json "$cell"
fi

# Independent fail-closed integrity checks. Activity counts are reported, not
# promoted into realism claims.
jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
jq -e '.result.valid == true' "$cell/frontiervectors.json" >/dev/null
jq -e '.result.delta_consistency.mismatched == 0 and .result.delta_consistency.chain_broken == 0 and .result.delta_consistency.decode_failures == 0' "$cell/conservation.json" >/dev/null
jq -e '.result.disagreement == 0 and .result.unrepresentable_open_values == 0' "$cell/positions.json" >/dev/null
jq -e '.result.missing_position_update == 0 and .result.unexpected_position_update == 0 and .result.position_chain_failures == 0' "$cell/fillpositions.json" >/dev/null
jq -e '.result.unknown_fills == 0 and .result.unknown_cancellations == 0 and .result.duplicate_acceptances == 0 and .result.duplicate_terminals == 0 and .result.fills_after_terminal == 0 and .result.fill_quantity_mismatches == 0 and .result.cancel_quantity_mismatches == 0 and .result.client_mismatches == 0' "$cell/orderlifecycle.json" >/dev/null
jq -e '.result.mismatched == 0 and .result.unpaid == 0 and .result.total_trades_after_expiry == 0' "$cell/settlements.json" >/dev/null
jq -e '.result.fills_after_expiry == 0 and .result.missing_expiry_metadata == 0 and .result.settlement_without_listing == 0 and .result.metadata_mismatches == 0' "$cell/expiryfills.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null

runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
offline_events=$(jq -er '.result.events' "$cell/evidenceartifacthash.json")
offline_digest=$(jq -er '.result.digest' "$cell/evidenceartifacthash.json")
[[ "$runtime_events" == "$offline_events" && "$runtime_digest" == "$offline_digest" ]] || {
  echo "runtime/offline evidence digest mismatch: $cell" >&2
  exit 1
}

required=(
  "observationreceipts.json" "frontiervectors.json" "mechanical.json"
  "conservation.json" "positions.json" "fillpositions.json"
  "orderlifecycle.json" "lifecycle.json" "settlements.json" "expiryfills.json"
  "evidenceartifacthash.json" "streamhash.json" "arbitrage.json" "crossvenue.json"
  "roleaudit.json" "ecology.json" "derivatives.json" "liquidations.json"
  "marginchecks.json" "optionsurface.json" "optionliabilityp6.json"
  "optionvaluetakerp6.json" "vannavolgap6.json" "exposure.json" "hedging.json"
  "makerrefresh.json" "makerquotesize.json" "makerrebalance.json" "postonly.json"
  "liabilityhedger.json" "perpsignals.json" "fundingcarry.json" "termcarry.json"
  "datedcarryp5.json" "perpreplenishment.json"
)
for artifact in "${required[@]}"; do
  [[ -s "$cell/$artifact" ]] || { echo "missing required artifact $artifact" >&2; exit 1; }
done

analysis_revision=$(git -C "$root_dir" rev-parse HEAD)
analyzer_sha256=$(sha256sum "$analyzer" | awk '{print $1}')
required_json=$(printf '%s\n' "${required[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')
jq -n \
  --arg analysis_revision "$analysis_revision" \
  --arg analyzer_sha256 "$analyzer_sha256" \
  --argjson required_artifacts "$required_json" \
  --argjson runtime_evidence_events "$runtime_events" \
  --arg runtime_evidence_digest "$runtime_digest" \
  '{analysis_revision: $analysis_revision, analyzer_sha256: $analyzer_sha256,
    analysis_contract: "v2-integrated-longrun-candidate-v1",
    completion_sentinels: ["greeks.json", "latency.json"],
    required_artifacts: $required_artifacts,
    runtime_evidence_artifact: {events: $runtime_evidence_events, digest: $runtime_evidence_digest},
    inactive_contracts: ["fundingcarry", "termcarry", "datedcarryp5", "perpreplenishment"],
    raw_log_policy: "retained; this extractor has no prune authority"}' \
  >"$cell/analysis-metadata.json"

echo "extracted integrated long-run evidence: $cell"
