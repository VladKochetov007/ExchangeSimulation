#!/usr/bin/env bash
# Fail-closed extraction for one completed immutable V2-5 P4b cell.  This
# script reconstructs the registered evidence contract; it never scores or
# prunes a funding world.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	echo "usage: $0 CELL_DIR" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cell=$(CDPATH= cd -- "$1" && pwd)
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
deadline=1736038805000000000

if [[ ! -x "$analyzer" ]]; then
	echo "missing executable analyzer: $analyzer" >&2
	exit 1
fi
for input in greeks.json latency.json manifest.json evidence-artifact-hash.json run-config.json run-metadata.json checkpoints.jsonl; do
	if [[ ! -s "$cell/$input" ]]; then
		echo "missing P4b completion/provenance input $input: $cell" >&2
		exit 1
	fi
done
if [[ ! -d "$cell/venues" ]] || ! find "$cell/venues" -type f -name '*.jsonl' -print -quit | grep -q .; then
	echo "missing persisted venue evidence under $cell/venues" >&2
	exit 1
fi

arm=$(jq -er '.arm' "$cell/run-metadata.json")
seed=$(jq -er '.seed' "$cell/run-metadata.json")
case "$arm" in A|B) ;; *) echo "invalid P4b arm: $arm" >&2; exit 1 ;; esac
case "$seed" in 401|409|419|421|431) ;; *) echo "invalid P4b seed: $seed" >&2; exit 1 ;; esac
jq -e --arg arm "$arm" --argjson seed "$seed" '
  .hypothesis_id == "V2-5-P4B-INDEPENDENT-PERP-FLOW" and
  .arm == $arm and .seed == $seed and
  .simulated_horizon == "98h0m0s" and
  .completion_sentinels == ["greeks.json", "latency.json"]
' "$cell/run-metadata.json" >/dev/null
jq -e --argjson seed "$seed" --arg arm "$arm" '
  .schema_version == 2 and .config.seed == $seed and
  .config.hypothesis_id == "V2-5-P4B-INDEPENDENT-PERP-FLOW" and
  .config.log_mode == "full" and
  .config.record_market_data_receipts == true and
  .config.record_perp_exposure_hedger_decisions == true and
  .config.record_term_carry_decisions == true and
  .config.market_data_receipt_roles == ["term_carry_allocator", "perp_exposure_hedger"] and
  .config.perp_exposure_hedger.enabled == true and
  .config.perp_exposure_hedger.exposure_mode == "random" and
  .config.funding_max_rate_bps == (if $arm == "A" then 1 else 75 end)
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
	termcarryp4chain termcarry perpexposurehedger observationreceipts
	derivatives conservation positions orderlifecycle lifecycle streamhash
	evidenceartifacthash basis perpsignals
)
for metric in "${metrics[@]}"; do
	write_metric "$cell/$metric.json" "$analyzer" -metric "$metric" -json "$cell"
done
write_metric "$cell/termcarrylifecycle.json" "$analyzer" \
	-metric termcarrylifecycle -term-carry-lifecycle-deadline "$deadline" -json "$cell"

# P4's exact-cost chain and the independently audited P2 exposure chain are
# activation/evidence gates.  A missing link is not a funding result.
jq -e '
  .result.valid == true and .result.base_audit.valid == true and
  .result.exact_cost_decisions_evaluated > 0 and
  (.result.checks | length) == 0
' "$cell/termcarryp4chain.json" >/dev/null
jq -e '
  .result.valid == true and .result.receipt_audit_valid == true and
  .result.decisions > 0 and .result.submitted > 0 and .result.fills > 0 and
  .result.non_reducing_fills == 0 and .result.receipt_evidence_errors == 0 and
  .result.future_receipt_use == 0 and .result.decision_mismatches == 0 and
  .result.outcome_mismatches == 0 and .result.missing_outcomes == 0 and
  .result.duplicate_outcomes == 0 and .result.missing_ioc_terminals == 0 and
  .result.duplicate_ioc_terminals == 0 and
  .result.fill_quantity_mismatches == 0 and .result.fill_evidence_mismatches == 0 and
  .result.fee_mismatches == 0
' "$cell/perpexposurehedger.json" >/dev/null
jq -e '
  .result.valid == true and .result.future_decision_use == 0 and
  .result.bad_global_event_order == 0 and .result.bad_receipt_ordinal == 0 and
  .result.bad_schedule_ordinal == 0 and .result.duplicate_source_identity == 0 and
  .result.receipt_without_schedule == 0 and .result.schedule_receipt_mismatch == 0 and
  .result.missing_due_receipt == 0 and .result.bad_decision_frontier == 0
' "$cell/observationreceipts.json" >/dev/null
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
jq -e '.result.disagreement == 0 and .result.unrepresentable_open_values == 0' \
	"$cell/positions.json" >/dev/null
jq -e '
  .result.unknown_fills == 0 and .result.unknown_cancellations == 0 and
  .result.duplicate_acceptances == 0 and .result.duplicate_terminals == 0 and
  .result.missing_immediate_terminal == 0 and .result.fills_after_terminal == 0 and
  .result.fill_quantity_mismatches == 0 and .result.cancel_quantity_mismatches == 0 and
  .result.client_mismatches == 0
' "$cell/orderlifecycle.json" >/dev/null
jq -e '
  .result.integrity_valid == true and
  .result.analysis_deadline_at_nano == 1736038805000000000 and
  .result.observation_end_at_nano >= .result.analysis_deadline_at_nano and
  (.result.integrity_failures | length) == 0
' "$cell/termcarrylifecycle.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' \
	"$cell/streamhash.json" >/dev/null
jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) == 64' \
	"$cell/evidenceartifacthash.json" >/dev/null
jq -e '.result.valid == true' "$cell/basis.json" >/dev/null
jq -e '.result.valid == true' "$cell/perpsignals.json" >/dev/null

runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
offline_events=$(jq -er '.result.events' "$cell/evidenceartifacthash.json")
offline_digest=$(jq -er '.result.digest' "$cell/evidenceartifacthash.json")
if [[ "$runtime_events" != "$offline_events" || "$runtime_digest" != "$offline_digest" ]]; then
	echo "runtime/offline P4b evidence artifact digest mismatch: $cell" >&2
	exit 1
fi

metadata_tmp=$(mktemp "$cell/analysis-metadata.json.tmp-XXXXXX")
jq -n \
	--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
	--arg arm "$arm" \
	--argjson seed "$seed" \
	--arg runtime_evidence_digest "$runtime_digest" \
	--argjson runtime_evidence_events "$runtime_events" \
	--argjson analysis_deadline_at_nano "$deadline" \
	' {
	  analysis_revision: $analysis_revision,
	  analyzer_sha256: $analyzer_sha256,
	  analysis_contract: "v2-5-p4b-independent-perp-flow-v1",
	  arm: $arm,
	  seed: $seed,
	  analysis_deadline_at_nano: $analysis_deadline_at_nano,
	  completion_sentinels: ["greeks.json", "latency.json"],
	  required_artifacts: [
	    "termcarryp4chain.json", "termcarry.json", "termcarrylifecycle.json",
	    "perpexposurehedger.json", "observationreceipts.json", "derivatives.json",
	    "conservation.json", "positions.json", "orderlifecycle.json", "lifecycle.json",
	    "streamhash.json", "evidenceartifacthash.json", "basis.json", "perpsignals.json"
	  ],
	  runtime_evidence_artifact: {events: $runtime_evidence_events, digest: $runtime_evidence_digest},
	  raw_log_policy: "retained; this extractor has no prune authority"
	}' >"$metadata_tmp"
mv "$metadata_tmp" "$cell/analysis-metadata.json"

echo "extracted V2-5 P4b evidence: $cell"
