#!/usr/bin/env bash
# Fail-closed extraction for one completed V2-7 P7a development cell.  This
# script has no scoring or prune authority: it only reconstructs the
# preregistered evidence contract and records immutable analysis metadata.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 CELL_DIR" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cell=$(CDPATH= cd -- "$1" && pwd)
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

if [[ ! -x "$analyzer" ]]; then
	echo "missing executable analyzer: $analyzer" >&2
	exit 1
fi
for input in greeks.json latency.json manifest.json evidence-artifact-hash.json run-config.json run-metadata.json checkpoints.jsonl; do
	if [[ ! -s "$cell/$input" ]]; then
		echo "missing P7a completion/provenance input $input: $cell" >&2
		exit 1
	fi
done
if [[ ! -d "$cell/venues" ]] || ! find "$cell/venues" -type f -name '*.jsonl' -print -quit | grep -q .; then
	echo "missing persisted venue evidence under $cell/venues" >&2
	exit 1
fi

preflight=$(jq -r '.preflight' "$cell/run-metadata.json")
[[ "$preflight" == false ]] || {
	echo "refusing to score a P7a preflight directory: $cell" >&2
	exit 1
}
horizon=$(jq -er '.simulated_horizon' "$cell/run-metadata.json")
[[ "$horizon" == 4h ]] || {
	echo "unexpected registered P7a horizon $horizon: $cell" >&2
	exit 1
}
cell_id=$(jq -er '.cell' "$cell/run-metadata.json")
seed=$(jq -er '.seed' "$cell/run-metadata.json")
case "$cell_id" in C|L|H) ;; *) echo "invalid P7a cell $cell_id" >&2; exit 1 ;; esac
case "$seed" in 307|311) ;; *) echo "invalid P7a development seed $seed" >&2; exit 1 ;; esac

# The run directory is itself part of the provenance contract.  Refuse a
# copied cell whose metadata/config no longer identify the same registered
# experiment.
jq -e --arg cell "$cell_id" --argjson seed "$seed" '
  .hypothesis_id == "V2-7-P7A-DISTRESS" and
  .cell == $cell and .seed == $seed and
  (.experiment_id | startswith("v2-7-p7a-distress-")) and
  (.completion_sentinels == ["greeks.json", "latency.json"])
' "$cell/run-metadata.json" >/dev/null
jq -e --argjson seed "$seed" --arg cell "$cell_id" '
  .schema_version == 2 and .config.seed == $seed and
  .config.hypothesis_id == "V2-7-P7A-DISTRESS" and
  .config.log_mode == "full" and
  .config.record_market_data_receipts == true and
  .config.record_perp_exposure_hedger_decisions == true and
  .config.perp_exposure_hedger.exposure_mode == "fixed_liability" and
  .config.perp_exposure_hedger.initial_physical_exposure == -1000000000 and
  .config.perp_exposure_hedger.enabled == ($cell != "C")
' "$cell/manifest.json" >/dev/null
config_sha=$(sha256sum "$cell/run-config.json" | awk '{print $1}')
jq -e --arg config_sha "$config_sha" '.config_sha256 == $config_sha' "$cell/run-metadata.json" >/dev/null

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

metrics=(
	perpexposurehedger observationreceipts evidenceartifacthash streamhash
	liquidations marginchecks conservation positions fillpositions orderlifecycle
	settlements expiryfills derivatives ecology roleaudit
)
for metric in "${metrics[@]}"; do
	if [[ "$metric" == marginchecks ]]; then
		write_metric "$cell/$metric.json" "$analyzer" -metric "$metric" -margin-role perp_exposure_hedger -json "$cell"
	else
		write_metric "$cell/$metric.json" "$analyzer" -metric "$metric" -json "$cell"
	fi
done

# Participant information and execution evidence must be independently
# reconstructible even when a particular risk endpoint is inactive.
jq -e '.result.decisions > 0 and .result.valid == true and
  .result.receipt_audit_valid == true and .result.future_receipt_use == 0 and
  .result.decision_mismatches == 0 and .result.outcome_mismatches == 0 and
  .result.missing_outcomes == 0 and .result.duplicate_outcomes == 0 and
  .result.missing_ioc_terminals == 0 and .result.duplicate_ioc_terminals == 0 and
  .result.fill_quantity_mismatches == 0 and .result.fill_evidence_mismatches == 0 and
  .result.non_reducing_fills == 0 and .result.fee_mismatches == 0' \
	"$cell/perpexposurehedger.json" >/dev/null
jq -e '.result.valid == true and .result.future_decision_use == 0 and
  .result.bad_global_event_order == 0 and .result.bad_receipt_ordinal == 0 and
  .result.bad_schedule_ordinal == 0 and .result.duplicate_source_identity == 0 and
  .result.receipt_without_schedule == 0 and .result.schedule_receipt_mismatch == 0 and
  .result.missing_due_receipt == 0 and .result.bad_decision_frontier == 0' \
	"$cell/observationreceipts.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and
  (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and
  (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null

# Mechanical reconstruction checks are fail-closed.  Risk events may be zero,
# but any emitted event must still satisfy the independent accounting checks.
jq -e '.result.delta_consistency.mismatched == 0 and
  .result.delta_consistency.chain_broken == 0 and
  .result.delta_consistency.decode_failures == 0' "$cell/conservation.json" >/dev/null
jq -e '.result.disagreement == 0 and .result.unrepresentable_open_values == 0' \
	"$cell/positions.json" >/dev/null
jq -e '.result.missing_position_update == 0 and
  .result.unexpected_position_update == 0 and
  .result.position_chain_failures == 0' "$cell/fillpositions.json" >/dev/null
jq -e '.result.unknown_fills == 0 and .result.unknown_cancellations == 0 and
  .result.duplicate_acceptances == 0 and .result.duplicate_terminals == 0 and
  .result.missing_immediate_terminal == 0 and .result.fills_after_terminal == 0 and
  .result.fill_quantity_mismatches == 0 and .result.cancel_quantity_mismatches == 0 and
  .result.client_mismatches == 0' "$cell/orderlifecycle.json" >/dev/null
jq -e '.result.mismatched == 0 and .result.unpaid == 0 and
  .result.total_trades_after_expiry == 0' "$cell/settlements.json" >/dev/null
jq -e '.result.fills_after_expiry == 0 and .result.missing_expiry_metadata == 0 and
  .result.settlement_without_listing == 0 and .result.metadata_mismatches == 0' \
	"$cell/expiryfills.json" >/dev/null
jq -e '.result.invalid_liquidations == 0 and
  .result.deficit_mismatch_instants == 0 and
  .result.position_path_failures == 0 and
  .result.position_conservation_failures == 0 and
  .result.deficit_insurance_residual == 0 and
  .result.deficit_balance_residual == 0' "$cell/liquidations.json" >/dev/null
jq -e '.result.excluded_candidates == 0 and .result.arithmetic_failures == 0 and
  .result.balance_chain_failures == 0 and .result.position_chain_failures == 0 and
  .result.mark_mismatches == 0 and .result.balance_mismatches == 0 and
  .result.equity_mismatches == 0 and .result.notional_mismatches == 0 and
  .result.maintenance_mismatches == 0' "$cell/marginchecks.json" >/dev/null

runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
offline_events=$(jq -er '.result.events' "$cell/evidenceartifacthash.json")
offline_digest=$(jq -er '.result.digest' "$cell/evidenceartifacthash.json")
if [[ "$runtime_events" != "$offline_events" || "$runtime_digest" != "$offline_digest" ]]; then
	echo "runtime/offline P7a evidence artifact digest mismatch: $cell" >&2
	exit 1
fi

metadata_tmp=$(mktemp "$cell/analysis-metadata.json.tmp-XXXXXX")
jq -n \
	--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
	--arg cell "$cell_id" \
	--argjson seed "$seed" \
	--arg runtime_evidence_digest "$runtime_digest" \
	--argjson runtime_evidence_events "$runtime_events" \
	'{
	  analysis_revision: $analysis_revision,
	  analyzer_sha256: $analyzer_sha256,
	  analysis_contract: "v2-7-p7a-distress-v1",
	  cell: $cell,
	  seed: $seed,
	  completion_sentinels: ["greeks.json", "latency.json"],
	  required_artifacts: [
	    "perpexposurehedger.json", "observationreceipts.json",
	    "evidenceartifacthash.json", "streamhash.json", "liquidations.json",
	    "marginchecks.json", "conservation.json", "positions.json",
	    "fillpositions.json", "orderlifecycle.json", "settlements.json",
	    "expiryfills.json", "derivatives.json", "ecology.json", "roleaudit.json"
	  ],
	  runtime_evidence_artifact: {events: $runtime_evidence_events, digest: $runtime_evidence_digest},
	  raw_log_policy: "retained; this extractor has no prune authority"
}' >"$metadata_tmp"
mv "$metadata_tmp" "$cell/analysis-metadata.json"

echo "extracted V2-7 P7a evidence: $cell"
