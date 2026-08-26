#!/usr/bin/env bash
# Fail-closed per-seed replication score for the preregistered V2-7 P7d
# holdout policy.  It mirrors the immutable development activation, evidence,
# and participant-risk predicates; it deliberately does not aggregate seeds or
# license any broader ecology claim.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	printf 'usage: %s 439|443|449\n' "$0" >&2
	exit 2
fi

seed=$1
case "$seed" in
	439|443|449) ;;
	*) printf 'unregistered P7d holdout seed: %s\n' "$seed" >&2; exit 2 ;;
esac

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base=${P7D_HOLDOUT_ROOT:-"$root_dir/research/artifacts/v2-7-p7d/holdout"}
output=${P7D_HOLDOUT_SCORE_OUTPUT:-"$base/seed-$seed-score.json"}
development_score="$root_dir/research/artifacts/v2-7-p7d/p7d-development-score.json"

[[ ! -e "$output" ]] || { printf 'refusing to overwrite score: %s\n' "$output" >&2; exit 1; }
"$root_dir/scripts/check-v2-7-p7d-holdout-configs.sh" >/dev/null
[[ -s "$development_score" ]] || { printf 'missing P7d development score: %s\n' "$development_score" >&2; exit 1; }

# This is the original promotion predicate that licensed every pre-reserved
# holdout configuration.  It is not reselected from the holdout evidence.
jq -e '.classification == "SUPPORTED (screening)" and
  .holdouts_consumed == false and
  .predicates.activation_valid == true and
  .predicates.participant_risk_replay_valid == true and
  .predicates.long_risk_exercised == true and
  .predicates.short_risk_exercised == true' "$development_score" >/dev/null || {
	printf 'P7d development promotion predicate is not satisfied\n' >&2
	exit 1
}

source_revision=$(jq -er '.source_revision' "$development_score")
binary_sha256=$(jq -er '.binary_sha256' "$development_score")
analyzer_sha256=$(jq -er '.analyzer_sha256' "$development_score")
cells=("C-$seed" "L-$seed" "S-$seed")
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
	[[ -d "$cell" ]] || { printf 'missing P7d holdout cell: %s\n' "$cell" >&2; exit 1; }
	for f in "${required[@]}"; do
		[[ -s "$cell/$f" ]] || { printf 'missing P7d holdout score input: %s/%s\n' "$cell" "$f" >&2; exit 1; }
	done
	[[ "$(<"$cell.extract.status")" == 0 ]] || {
		printf 'P7d holdout extraction did not pass: %s\n' "$id" >&2
		exit 1
	}

	cfg_sha=$(sha256sum "$cell/run-config.json" | awk '{print $1}')
	jq -e --arg rev "$source_revision" --arg bin "$binary_sha256" --arg cfg "$cfg_sha" '
		.git_revision == $rev and .binary_sha256 == $bin and .config_sha256 == $cfg and
		.holdout == true and .preflight == false and .simulated_horizon == "4h" and
		.completion_sentinels == ["greeks.json", "latency.json"]' "$cell/run-metadata.json" >/dev/null
	jq -e --arg analyzer "$analyzer_sha256" '
		.analyzer_sha256 == $analyzer and
		.analysis_contract == "v2-7-p7d-directional-distress-v1" and
		(.required_artifacts | length) == 16 and
		.runtime_evidence_artifact.events > 0 and
		(.runtime_evidence_artifact.digest | length) == 64' "$cell/analysis-metadata.json" >/dev/null

	runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
	runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
	jq -e --argjson events "$runtime_events" --arg digest "$runtime_digest" '
		.result.events == $events and .result.digest == $digest and
		(.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null
	jq -e '.result.events > 0 and (.result.digest | length) == 64' "$cell/streamhash.json" >/dev/null

	jq -e '.result.valid == true and .result.receipt_audit_valid == true and
		.result.future_receipt_use == 0 and .result.decision_mismatches == 0 and
		.result.outcome_mismatches == 0 and .result.missing_outcomes == 0 and
		.result.duplicate_outcomes == 0 and .result.missing_ioc_terminals == 0 and
		.result.duplicate_ioc_terminals == 0 and .result.fill_quantity_mismatches == 0 and
		.result.fill_evidence_mismatches == 0 and .result.non_reducing_fills == 0 and
		.result.fee_mismatches == 0 and .result.decisions > 0' "$cell/perpexposurehedger.json" >/dev/null
	jq -e '.result.valid == true and .result.future_decision_use == 0 and
		.result.bad_global_event_order == 0 and .result.bad_receipt_ordinal == 0 and
		.result.bad_schedule_ordinal == 0 and .result.duplicate_source_identity == 0 and
		.result.receipt_without_schedule == 0 and .result.schedule_receipt_mismatch == 0 and
		.result.missing_due_receipt == 0 and .result.bad_decision_frontier == 0' "$cell/observationreceipts.json" >/dev/null
	jq -e '.result.valid == true and .result.candidates > 0 and .result.mark_updates > 0 and
		.result.missing_checks == 0 and .result.unexpected_checks == 0 and
		.result.duplicate_checks == 0 and .result.field_mismatches == 0 and
		.result.position_chain_failures == 0 and .result.balance_chain_failures == 0 and
		.result.borrow_amount_mismatches == 0 and .result.position_path_failures == 0 and
		.result.terminal_state_mismatches == 0 and
		.result.observed_checks == .result.expected_breaches' "$cell/perpexposurerisk.json" >/dev/null
	jq -e '.result.invalid_liquidations == 0 and .result.deficit_mismatch_instants == 0 and
		.result.position_path_failures == 0 and .result.position_conservation_failures == 0 and
		.result.deficit_insurance_residual == 0 and .result.deficit_balance_residual == 0' "$cell/liquidations.json" >/dev/null
	jq -e '.result.delta_consistency.mismatched == 0 and
		.result.delta_consistency.chain_broken == 0 and .result.delta_consistency.decode_failures == 0' "$cell/conservation.json" >/dev/null
done

records=()
for id in "${cells[@]}"; do
	cell="$base/$id"
	arm=${id%%-*}
	records+=("$(jq -n --arg cell "$arm" --argjson seed "$seed" \
		--arg analysis_revision "$(jq -er '.analysis_revision' "$cell/analysis-metadata.json")" \
		--slurpfile hedger "$cell/perpexposurehedger.json" \
		--slurpfile risk "$cell/perpexposurerisk.json" \
		--slurpfile receipts "$cell/observationreceipts.json" \
		--slurpfile liquidations "$cell/liquidations.json" \
		--slurpfile evidence "$cell/evidenceartifacthash.json" '
		{cell:$cell,seed:$seed,analysis_revision:$analysis_revision,
		 activation:{enabled_decisions:$hedger[0].result.enabled_decisions,
		   disabled_decisions:$hedger[0].result.disabled_decisions,
		   decisions:$hedger[0].result.decisions,submitted:$hedger[0].result.submitted,
		   accepted:$hedger[0].result.accepted,fills:$hedger[0].result.fills,
		   filled_qty:$hedger[0].result.filled_qty,
		   terminal_absolute_gaps:($hedger[0].result.hedgers|map(.terminal_absolute_gap)),
		   control_valid:($cell=="C" and $hedger[0].result.enabled_decisions==0 and
		     $hedger[0].result.disabled_decisions==$hedger[0].result.decisions and
		     $hedger[0].result.submitted==0 and $hedger[0].result.accepted==0 and
		     $hedger[0].result.fills==0 and $hedger[0].result.filled_qty==0),
		   active_valid:($cell!="C" and $hedger[0].result.enabled_decisions==$hedger[0].result.decisions and
		     $hedger[0].result.enabled_decisions>0 and $hedger[0].result.accepted>0 and
		     $hedger[0].result.fills>0 and $hedger[0].result.filled_qty==6000000000 and
		     all($hedger[0].result.hedgers[];.terminal_absolute_gap=="0")),
		   action_counts:$hedger[0].result.action_counts},
		 information_integrity:{observations_valid:$receipts[0].result.valid,
		   future_decision_use:$receipts[0].result.future_decision_use,
		   bad_decision_frontier:$receipts[0].result.bad_decision_frontier,
		   receipt_errors:$hedger[0].result.receipt_evidence_errors},
		 risk:{candidates:$risk[0].result.candidates,mark_updates:$risk[0].result.mark_updates,
		   expected_breaches:$risk[0].result.expected_breaches,observed_checks:$risk[0].result.observed_checks,
		   missing_checks:$risk[0].result.missing_checks,unexpected_checks:$risk[0].result.unexpected_checks,
		   duplicate_checks:$risk[0].result.duplicate_checks,field_mismatches:$risk[0].result.field_mismatches,
		   position_path_failures:$risk[0].result.position_path_failures,
		   terminal_state_mismatches:$risk[0].result.terminal_state_mismatches,
		   participant_liquidations:$risk[0].result.participant_liquidations,
		   risk_exercised:($risk[0].result.expected_breaches>0 and $risk[0].result.observed_checks>0),
		   deficits:$liquidations[0].result.liquidations_with_deficit,
		   total_deficit:$liquidations[0].result.total_deficit,
		   insurance_deficit:$liquidations[0].result.insurance_deficit},
		 evidence:{events:$evidence[0].result.events,digest:$evidence[0].result.digest}}')")
done
cells_json=$(printf '%s\n' "${records[@]}" | jq -s '.')

control_valid=$(jq -e 'all(.[]; .cell!="C" or .activation.control_valid)' <<<"$cells_json" >/dev/null && echo true || echo false)
activation_valid=$(jq -e 'all(.[]; .cell=="C" or .activation.active_valid)' <<<"$cells_json" >/dev/null && echo true || echo false)
integrity_valid=$(jq -e 'all(.[]; .information_integrity.observations_valid and .information_integrity.future_decision_use==0 and .information_integrity.bad_decision_frontier==0 and .information_integrity.receipt_errors==0)' <<<"$cells_json" >/dev/null && echo true || echo false)
risk_integrity=$(jq -e 'all(.[]; .risk.missing_checks==0 and .risk.unexpected_checks==0 and .risk.duplicate_checks==0 and .risk.field_mismatches==0 and .risk.position_path_failures==0 and .risk.terminal_state_mismatches==0 and .risk.observed_checks==.risk.expected_breaches)' <<<"$cells_json" >/dev/null && echo true || echo false)
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

remaining=$(jq -cn --argjson seed "$seed" '[439,443,449] - [$seed]')
jq -n --arg protocol v2-7-p7d-directional-distress-v1 --arg classification "$classification" \
	--arg source_revision "$source_revision" --arg binary_sha256 "$binary_sha256" --arg analyzer_sha256 "$analyzer_sha256" \
	--argjson seed "$seed" --argjson control_valid "$control_valid" --argjson activation_valid "$activation_valid" \
	--argjson integrity_valid "$integrity_valid" --argjson risk_integrity "$risk_integrity" \
	--argjson long_risk "$long_risk" --argjson short_risk "$short_risk" --argjson deficit_exercised "$deficit_exercised" \
	--argjson remaining "$remaining" --argjson cells "$cells_json" '
	{protocol:$protocol,scope:"one preregistered untouched seed; no cross-seed aggregate inference",
	 holdout_seed:$seed,classification:$classification,
	 source_revision:$source_revision,binary_sha256:$binary_sha256,analyzer_sha256:$analyzer_sha256,
	 development_reference:{classification:"SUPPORTED (screening)",seeds:[431,433],
	   directional_risk_both_signs:true},
	 predicates:{control_valid:$control_valid,activation_valid:$activation_valid,
	   evidence_integrity_valid:$integrity_valid,participant_risk_replay_valid:$risk_integrity,
	   long_risk_exercised:$long_risk,short_risk_exercised:$short_risk,
	   deficit_insurance_bankruptcy_exercised:$deficit_exercised},
	 verdicts:{directional_activation:(if $activation_valid then "SUPPORTED (screening)" else "FALSIFIED AT ACTIVATION/EXECUTION" end),
	   participant_specific_risk:(if ($long_risk and $short_risk) then "SUPPORTED (screening)" elif ($long_risk or $short_risk) then "MIXED" else "NOT EXERCISED" end),
	   deficit_insurance_bankruptcy:(if $deficit_exercised then "OBSERVED; separate accounting review required" else "NOT EXERCISED" end)},
	 cells:$cells,remaining_preregistered_holdout_seeds:$remaining,
	 interpretation:"This is a fail-closed per-seed replication score of the immutable P7d directional activation, evidence-integrity, and participant-specific maintenance-risk predicate. It does not aggregate the pre-reserved holdout seeds and does not support a claim about bankruptcy, full-ecology liquidation realism, funding, basis, profitability, or market stability.",
	 next_action:"Do not tune. Preserve this raw evidence; complete remaining pre-reserved holdout seeds under their immutable configs before any aggregate out-of-sample interpretation."}' >"$output"
printf 'scored P7d holdout seed %s: %s\n' "$seed" "$classification"
