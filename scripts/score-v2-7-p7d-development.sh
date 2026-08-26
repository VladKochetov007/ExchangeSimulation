#!/usr/bin/env bash
# Fail-closed score for the immutable V2-7 P7d development screen.
# This scores only the registered activation/integrity/risk predicates. It
# has no holdout or prune authority.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base=${P7D_OUTPUT_ROOT:-"$root_dir/research/artifacts/v2-7-p7d/full"}
output=${P7D_SCORE_OUTPUT:-"$root_dir/research/artifacts/v2-7-p7d/p7d-development-score.json"}
[[ ! -e "$output" ]] || { echo "refusing to overwrite score: $output" >&2; exit 1; }
cells=(C-431 C-433 L-431 L-433 S-431 S-433)
required=(
  analysis-metadata.json run-metadata.json run-config.json manifest.json
  greeks.json latency.json evidence-artifact-hash.json checkpoints.jsonl
  perpexposurehedger.json perpexposurerisk.json observationreceipts.json
  evidenceartifacthash.json streamhash.json liquidations.json marginchecks.json
  conservation.json positions.json fillpositions.json orderlifecycle.json
  settlements.json expiryfills.json derivatives.json ecology.json roleaudit.json
)

for id in "${cells[@]}"; do
  cell="$base/$id"
  [[ -d "$cell" ]] || { echo "missing P7d cell: $cell" >&2; exit 1; }
  for f in "${required[@]}"; do
    [[ -s "$cell/$f" ]] || { echo "missing P7d score input: $cell/$f" >&2; exit 1; }
  done
  [[ "$(cat "$cell.extract.status" 2>/dev/null || true)" == 0 ]] || {
    echo "P7d extraction did not pass: $id" >&2
    exit 1
  }
done

source_revision=$(jq -er '.git_revision' "$base/C-431/run-metadata.json")
binary_sha256=$(jq -er '.binary_sha256' "$base/C-431/run-metadata.json")
analyzer_sha256=$(jq -er '.analyzer_sha256' "$base/C-431/analysis-metadata.json")

for id in "${cells[@]}"; do
  cell="$base/$id"
  cfg_sha=$(sha256sum "$cell/run-config.json" | awk '{print $1}')
  jq -e --arg rev "$source_revision" --arg bin "$binary_sha256" \
    --arg cfg "$cfg_sha" \
    '.git_revision == $rev and .binary_sha256 == $bin and .config_sha256 == $cfg and
     .preflight == false and .simulated_horizon == "4h" and
     .completion_sentinels == ["greeks.json", "latency.json"]' \
    "$cell/run-metadata.json" >/dev/null
  jq -e --arg bin "$analyzer_sha256" \
    '.analyzer_sha256 == $bin and
     .analysis_contract == "v2-7-p7d-directional-distress-v1" and
     (.required_artifacts | length) == 16 and
     .runtime_evidence_artifact.events > 0 and
     (.runtime_evidence_artifact.digest | length) == 64' \
    "$cell/analysis-metadata.json" >/dev/null
  runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
  runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
  jq -e --argjson ev "$runtime_events" --arg dg "$runtime_digest" \
    '.result.events == $ev and .result.digest == $dg and (.result.digest | length) == 64' \
    "$cell/evidenceartifacthash.json" >/dev/null
  jq -e '.result.events > 0 and (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null
  jq -e '.result.valid == true and .result.receipt_audit_valid == true and
    .result.future_receipt_use == 0 and .result.decision_mismatches == 0 and
    .result.outcome_mismatches == 0 and .result.missing_outcomes == 0 and
    .result.duplicate_outcomes == 0 and .result.missing_ioc_terminals == 0 and
    .result.duplicate_ioc_terminals == 0 and .result.fill_quantity_mismatches == 0 and
    .result.fill_evidence_mismatches == 0 and .result.non_reducing_fills == 0 and
    .result.fee_mismatches == 0 and .result.decisions > 0' \
    "$cell/perpexposurehedger.json" >/dev/null
  jq -e '.result.valid == true and .result.future_decision_use == 0 and
    .result.bad_global_event_order == 0 and .result.bad_receipt_ordinal == 0 and
    .result.bad_schedule_ordinal == 0 and .result.duplicate_source_identity == 0 and
    .result.receipt_without_schedule == 0 and .result.schedule_receipt_mismatch == 0 and
    .result.missing_due_receipt == 0 and .result.bad_decision_frontier == 0' \
    "$cell/observationreceipts.json" >/dev/null
  jq -e '.result.valid == true and .result.candidates > 0 and
    .result.mark_updates > 0 and .result.missing_checks == 0 and
    .result.unexpected_checks == 0 and .result.duplicate_checks == 0 and
    .result.field_mismatches == 0 and .result.position_chain_failures == 0 and
    .result.balance_chain_failures == 0 and .result.borrow_amount_mismatches == 0 and
    .result.position_path_failures == 0 and .result.terminal_state_mismatches == 0 and
    .result.observed_checks == .result.expected_breaches' \
    "$cell/perpexposurerisk.json" >/dev/null
  jq -e '.result.invalid_liquidations == 0 and
    .result.deficit_mismatch_instants == 0 and .result.position_path_failures == 0 and
    .result.position_conservation_failures == 0 and .result.deficit_insurance_residual == 0 and
    .result.deficit_balance_residual == 0' "$cell/liquidations.json" >/dev/null
  jq -e '.result.delta_consistency.mismatched == 0 and
    .result.delta_consistency.chain_broken == 0 and
    .result.delta_consistency.decode_failures == 0' "$cell/conservation.json" >/dev/null
done

cell_records=()
for id in "${cells[@]}"; do
  cell="$base/$id"
  arm=${id%%-*}
  seed=${id##*-}
  cell_analysis_revision=$(jq -er '.analysis_revision' "$cell/analysis-metadata.json")
  cell_records+=("$(jq -n --arg cell "$arm" --argjson seed "$seed" \
    --arg analysis_revision "$cell_analysis_revision" \
    --slurpfile p "$cell/perpexposurehedger.json" --slurpfile r "$cell/perpexposurerisk.json" \
    --slurpfile o "$cell/observationreceipts.json" --slurpfile l "$cell/liquidations.json" \
    --slurpfile e "$cell/evidenceartifacthash.json" '
    {cell:$cell,seed:$seed,analysis_revision:$analysis_revision,
     activation:{enabled_decisions:$p[0].result.enabled_decisions,
       disabled_decisions:$p[0].result.disabled_decisions,decisions:$p[0].result.decisions,
       submitted:$p[0].result.submitted,accepted:$p[0].result.accepted,
       fills:$p[0].result.fills,filled_qty:$p[0].result.filled_qty,
       terminal_absolute_gaps:($p[0].result.hedgers|map(.terminal_absolute_gap)),
       target_reached:all($p[0].result.hedgers[];.terminal_absolute_gap=="0"),
       control_valid:($cell=="C" and $p[0].result.enabled_decisions==0 and
         $p[0].result.disabled_decisions==$p[0].result.decisions and
         $p[0].result.submitted==0 and $p[0].result.accepted==0 and
         $p[0].result.fills==0 and $p[0].result.filled_qty==0),
       active_valid:($cell!="C" and
         $p[0].result.enabled_decisions==$p[0].result.decisions and
         $p[0].result.enabled_decisions>0 and $p[0].result.accepted>0 and
         $p[0].result.fills>0 and $p[0].result.filled_qty==6000000000 and
         all($p[0].result.hedgers[];.terminal_absolute_gap=="0")),
       action_counts:$p[0].result.action_counts},
     information_integrity:{observations_valid:$o[0].result.valid,
       future_decision_use:$o[0].result.future_decision_use,
       bad_decision_frontier:$o[0].result.bad_decision_frontier,
       receipt_errors:$p[0].result.receipt_evidence_errors},
     risk:{candidates:$r[0].result.candidates,mark_updates:$r[0].result.mark_updates,
       expected_breaches:$r[0].result.expected_breaches,
       observed_checks:$r[0].result.observed_checks,
       arithmetic_failures:$r[0].result.arithmetic_failures,
       mark_mismatches:$r[0].result.mark_mismatches,
       balance_mismatches:$r[0].result.balance_mismatches,
       contribution_mismatches:$r[0].result.contribution_mismatches,
       equity_mismatches:$r[0].result.equity_mismatches,
       notional_mismatches:$r[0].result.notional_mismatches,
       maintenance_mismatches:$r[0].result.maintenance_mismatches,
       mark_domain_failures:$r[0].result.mark_domain_failures,
       cross_file_ambiguities:$r[0].result.cross_file_ambiguities,
       malformed_records:$r[0].result.malformed_records,
       missing_checks:$r[0].result.missing_checks,unexpected_checks:$r[0].result.unexpected_checks,
       duplicate_checks:$r[0].result.duplicate_checks,field_mismatches:$r[0].result.field_mismatches,
       position_path_failures:$r[0].result.position_path_failures,
       terminal_state_mismatches:$r[0].result.terminal_state_mismatches,
       participant_liquidations:$r[0].result.participant_liquidations,
       deficits:$l[0].result.liquidations_with_deficit,
       total_deficit:$l[0].result.total_deficit,insurance_deficit:$l[0].result.insurance_deficit,
       risk_exercised:($r[0].result.expected_breaches>0 and $r[0].result.observed_checks>0)},
     evidence:{events:$e[0].result.events,digest:$e[0].result.digest}}')")
done
cells_json=$(printf '%s\n' "${cell_records[@]}" | jq -s '.')

control_valid=$(jq -e 'all(.[]; .cell!="C" or .activation.control_valid)' <<<"$cells_json" >/dev/null && echo true || echo false)
activation_valid=$(jq -e 'all(.[]; .cell=="C" or .activation.active_valid)' <<<"$cells_json" >/dev/null && echo true || echo false)
integrity_valid=$(jq -e 'all(.[];
  .information_integrity.observations_valid and .information_integrity.future_decision_use==0 and
  .information_integrity.bad_decision_frontier==0 and .information_integrity.receipt_errors==0)' <<<"$cells_json" >/dev/null && echo true || echo false)
risk_integrity=$(jq -e 'all(.[];
  .risk.missing_checks==0 and .risk.unexpected_checks==0 and .risk.duplicate_checks==0 and
  .risk.field_mismatches==0 and .risk.position_path_failures==0 and
  .risk.terminal_state_mismatches==0 and .risk.observed_checks==.risk.expected_breaches)' <<<"$cells_json" >/dev/null && echo true || echo false)
long_risk=$(jq -e 'any(.[]; .cell=="L" and .risk.risk_exercised)' <<<"$cells_json" >/dev/null && echo true || echo false)
short_risk=$(jq -e 'any(.[]; .cell=="S" and .risk.risk_exercised)' <<<"$cells_json" >/dev/null && echo true || echo false)
deficit_exercised=$(jq -e 'any(.[]; .risk.deficits>0 or .risk.total_deficit>0 or .risk.insurance_deficit>0)' <<<"$cells_json" >/dev/null && echo true || echo false)

if [[ "$control_valid" != true || "$integrity_valid" != true || "$risk_integrity" != true ]]; then
  classification=INVALID
elif [[ "$activation_valid" != true ]]; then
  classification='FALSIFIED AT ACTIVATION/EXECUTION'
elif [[ "$long_risk" != true && "$short_risk" != true ]]; then
  classification='NOT EXERCISED'
elif [[ "$long_risk" == true && "$short_risk" == true ]]; then
  classification='SUPPORTED (screening)'
else
  classification=MIXED
fi

mkdir -p "$(dirname -- "$output")"
jq -n --arg protocol v2-7-p7d-directional-distress-v1 --arg classification "$classification" \
  --arg source_revision "$source_revision" --arg binary_sha256 "$binary_sha256" \
  --arg analyzer_sha256 "$analyzer_sha256" \
  --argjson control_valid "$control_valid" --argjson activation_valid "$activation_valid" \
  --argjson integrity_valid "$integrity_valid" --argjson risk_integrity "$risk_integrity" \
  --argjson long_risk "$long_risk" --argjson short_risk "$short_risk" \
  --argjson deficit_exercised "$deficit_exercised" --argjson cells "$cells_json" '
  {protocol:$protocol,classification:$classification,development_seeds:[431,433],
   untouched_holdout_seeds:[439,443,449],holdouts_consumed:false,
   source_revision:$source_revision,binary_sha256:$binary_sha256,
   analyzer_sha256:$analyzer_sha256,
   predicates:{control_valid:$control_valid,activation_valid:$activation_valid,
     evidence_integrity_valid:$integrity_valid,participant_risk_replay_valid:$risk_integrity,
     long_risk_exercised:$long_risk,short_risk_exercised:$short_risk,
     deficit_insurance_bankruptcy_exercised:$deficit_exercised},
   verdicts:{
     directional_activation:(if $activation_valid then "SUPPORTED (screening)" else "FALSIFIED AT ACTIVATION/EXECUTION" end),
     participant_specific_risk:(if ($long_risk and $short_risk) then "SUPPORTED (screening)" elif ($long_risk or $short_risk) then "MIXED" else "NOT EXERCISED" end),
     deficit_insurance_bankruptcy:(if $deficit_exercised then "OBSERVED; separate accounting review required" else "NOT EXERCISED" end)},
   cells:$cells,
   interpretation:"P7d scores only the preregistered finite-capital directional activation, evidence integrity, and participant-specific maintenance-breach replay. Risk events, deficits, insurance, bankruptcy, funding, basis, profitability, and realism are separate endpoints.",
   next_action:(if $classification=="SUPPORTED (screening)" then "Review risk-event accounting and authorize reserved holdout policy; do not tune development cells." elif $classification=="MIXED" then "Retain both orientations and treat as screening evidence; no holdout without protocol review." elif $classification=="NOT EXERCISED" then "Do not consume holdouts; design a new exposure source/horizon without tuning P7d." else "Stop causal interpretation and investigate the failed contract." end)}' >"$output"
echo "scored P7d development: $classification"
