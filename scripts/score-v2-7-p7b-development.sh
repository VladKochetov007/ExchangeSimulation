#!/usr/bin/env bash
# Fail-closed development score for the immutable V2-7 P7b screen.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base=${P7B_OUTPUT_ROOT:-"$root_dir/research/artifacts/v2-7-p7b/full"}
output=${P7B_SCORE_OUTPUT:-"$root_dir/research/artifacts/v2-7-p7b/p7b-development-score.json"}
[[ ! -e "$output" ]] || { echo "refusing to overwrite score: $output" >&2; exit 1; }
cells=(C-337 C-341 L-337 L-341 H-337 H-341)

for id in "${cells[@]}"; do
	cell="$base/$id"
	for f in analysis-metadata.json run-metadata.json run-config.json \
		perpexposurehedger.json observationreceipts.json evidenceartifacthash.json \
		streamhash.json liquidations.json marginchecks.json conservation.json \
		positions.json fillpositions.json orderlifecycle.json settlements.json \
		expiryfills.json derivatives.json ecology.json roleaudit.json; do
		[[ -s "$cell/$f" ]] || { echo "missing P7b score input: $cell/$f" >&2; exit 1; }
	done
	[[ "$(cat "$cell.extract.status" 2>/dev/null || true)" == 0 ]] || {
		echo "P7b extraction did not pass: $id" >&2; exit 1;
	}
done

source_revision=$(jq -er '.git_revision' "$base/C-337/run-metadata.json")
binary_sha256=$(jq -er '.binary_sha256' "$base/C-337/run-metadata.json")
for id in "${cells[@]}"; do
	cell="$base/$id"
	config_sha=$(sha256sum "$cell/run-config.json" | awk '{print $1}')
	jq -e --arg rev "$source_revision" --arg bin "$binary_sha256" --arg cfg "$config_sha" \
		'.git_revision == $rev and .binary_sha256 == $bin and .config_sha256 == $cfg and
		 .preflight == false and .simulated_horizon == "24h" and
		 .completion_sentinels == ["greeks.json", "latency.json"]' \
		"$cell/run-metadata.json" >/dev/null
	jq -e '.analysis_contract == "v2-7-p7b-distress-v1" and
		 .runtime_evidence_artifact.events > 0 and
		 (.runtime_evidence_artifact.digest | length) == 64 and
		 (.required_artifacts | length) == 15' "$cell/analysis-metadata.json" >/dev/null
	runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
	runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
	jq -e --argjson ev "$runtime_events" --arg dg "$runtime_digest" \
		'.result.events == $ev and .result.digest == $dg' "$cell/evidenceartifacthash.json" >/dev/null
done

cell_json=()
for id in "${cells[@]}"; do
	cell="$base/$id"; arm=${id%%-*}; seed=${id##*-}
	liquidation_lines=$(rg --no-filename '"event":"liquidation"' "$cell/venues" || true)
	if [[ -n "$liquidation_lines" ]]; then
		actor_liquidations=$(printf '%s\n' "$liquidation_lines" |
			jq -R -r 'fromjson | select(.event == "liquidation" and .client_id == 59) | 1' |
			wc -l | tr -d ' ')
	else actor_liquidations=0; fi
	margin_lines=$(rg --no-filename '"event":"margin_call"|"event":"margincall"|"event":"margin_check"' "$cell/venues" || true)
	if [[ -n "$margin_lines" ]]; then
		actor_margin_events=$(printf '%s\n' "$margin_lines" |
			jq -R -r 'fromjson | select((.client_id // null) == 59) | 1' |
			wc -l | tr -d ' ')
	else actor_margin_events=0; fi
	cell_json+=("$(jq -n --arg cell "$arm" --argjson seed "$seed" \
		--argjson actor_liquidations "$actor_liquidations" \
		--argjson actor_margin_events "$actor_margin_events" \
		--slurpfile p "$cell/perpexposurehedger.json" --slurpfile o "$cell/observationreceipts.json" \
		--slurpfile m "$cell/marginchecks.json" --slurpfile l "$cell/liquidations.json" \
		--slurpfile e "$cell/evidenceartifacthash.json" '
		{
		 cell:$cell, seed:$seed,
		 activation:{enabled_decisions:$p[0].result.enabled_decisions,
		  disabled_decisions:$p[0].result.disabled_decisions, decisions:$p[0].result.decisions,
		  submitted:$p[0].result.submitted, accepted:$p[0].result.accepted,
		  rejected:$p[0].result.rejected, fills:$p[0].result.fills,
		  filled_qty:$p[0].result.filled_qty,
		  terminal_absolute_gaps:($p[0].result.hedgers|map(.terminal_absolute_gap)),
		  action_counts:$p[0].result.action_counts,
		  receipt_audit_valid:$p[0].result.receipt_audit_valid,
		  future_receipt_use:$p[0].result.future_receipt_use, valid:$p[0].result.valid,
		  control_valid:($cell=="C" and $p[0].result.enabled_decisions==0 and
		   $p[0].result.disabled_decisions==$p[0].result.decisions and
		   $p[0].result.submitted==0 and $p[0].result.fills==0 and $p[0].result.filled_qty==0),
		  active_valid:($cell!="C" and $p[0].result.enabled_decisions==$p[0].result.decisions and
		   $p[0].result.enabled_decisions>0 and $p[0].result.accepted>0 and
		   $p[0].result.fills>0 and $p[0].result.filled_qty==3000000000 and
		   all($p[0].result.hedgers[];.terminal_absolute_gap=="0"))},
		 information_integrity:{observations_valid:$o[0].result.valid,
		  future_decision_use:$o[0].result.future_decision_use,
		  bad_decision_frontier:$o[0].result.bad_decision_frontier,
		  receipt_errors:$p[0].result.receipt_evidence_errors},
		 risk:{active_mark_checks:$m[0].result.active_mark_checks,
		  expected_breaches:$m[0].result.expected_breaches,
		  observed_checks:$m[0].result.observed_checks, missing_checks:$m[0].result.missing_checks,
		  unexpected_checks:$m[0].result.unexpected_checks,
		  actor_margin_events:$actor_margin_events, actor_liquidations:$actor_liquidations,
		  generic_liquidations:$l[0].result.liquidations,
		  generic_liquidation_checks:$l[0].result.liquidation_checks,
		  generic_affected_accounts:$l[0].result.affected_accounts,
		  deficits:$l[0].result.liquidations_with_deficit,
		  total_deficit:$l[0].result.total_deficit,
		  insurance_deficit:$l[0].result.insurance_deficit,
		  invalid_liquidations:$l[0].result.invalid_liquidations,
		  path_failures:$l[0].result.position_path_failures,
		  conservation_failures:$l[0].result.position_conservation_failures},
		 evidence:{events:$e[0].result.events,digest:$e[0].result.digest}
		}')")
done

cells_json=$(printf '%s\n' "${cell_json[@]}" | jq -s '.')
activation=$(jq -e 'all(.[];.cell=="C" or .activation.active_valid)' <<<"$cells_json" >/dev/null && echo true || echo false)
control_valid=$(jq -e 'all(.[];.cell!="C" or .activation.control_valid)' <<<"$cells_json" >/dev/null && echo true || echo false)
integrity=$(jq -e 'all(.[];.activation.valid and .activation.receipt_audit_valid and
	.activation.future_receipt_use==0 and .information_integrity.observations_valid and
	.information_integrity.future_decision_use==0 and .information_integrity.bad_decision_frontier==0 and
	.information_integrity.receipt_errors==0)' <<<"$cells_json" >/dev/null && echo true || echo false)
risk_exercised=$(jq -e 'any(.[];.cell!="C" and (.risk.expected_breaches>0 or
	.risk.actor_margin_events>0 or .risk.actor_liquidations>0))' <<<"$cells_json" >/dev/null && echo true || echo false)
deficit_exercised=$(jq -e 'any(.[];.risk.deficits>0 or .risk.total_deficit>0 or .risk.insurance_deficit>0)' <<<"$cells_json" >/dev/null && echo true || echo false)
if [[ "$integrity" != true || "$control_valid" != true ]]; then classification=INVALID
elif [[ "$activation" != true ]]; then classification='FALSIFIED AT ACTIVATION'
elif [[ "$risk_exercised" != true ]]; then classification='NOT EXERCISED'
else classification='PENDING DEFICIT/CAUSAL REVIEW'; fi

mkdir -p "$(dirname -- "$output")"
jq -n --arg protocol v2-7-p7b-distress-v1 --arg classification "$classification" \
	--arg source_revision "$source_revision" --arg binary_sha256 "$binary_sha256" \
	--argjson activation "$activation" --argjson control_valid "$control_valid" \
	--argjson integrity "$integrity" --argjson risk_exercised "$risk_exercised" \
	--argjson deficit_exercised "$deficit_exercised" --argjson cells "$cells_json" '
	{protocol:$protocol,classification:$classification,development_seeds:[337,341],
	 untouched_holdout_seeds:[347,349,353],holdouts_consumed:false,
	 source_revision:$source_revision,binary_sha256:$binary_sha256,
	 predicates:{control_valid:$control_valid,activation_valid:$activation,
	  evidence_integrity_valid:$integrity,actor_risk_path_exercised:$risk_exercised,
	  deficit_path_exercised:$deficit_exercised},
	 verdicts:{fixed_liability_activation:"SUPPORTED (screening)",
	  participant_margin_call:(if $risk_exercised then "PENDING REVIEW" else "NOT EXERCISED" end),
	  participant_forced_close:(if $risk_exercised then "PENDING REVIEW" else "NOT EXERCISED" end),
	  deficit_insurance_bankruptcy:(if $deficit_exercised then "PENDING REVIEW" else "NOT EXERCISED" end),
	  finite_margin_ladder_effect:"NOT SCORED"},cells:$cells,
	 interpretation:"P7b is scored only for fixed-liability activation, evidence integrity, and participant-specific risk reachability; generic liquidation totals cannot substitute for a participant-built risk path.",
	 next_action:(if $risk_exercised then "Audit participant-specific risk events before any holdout." else "Do not consume holdouts or tune P7b; preregister a new economically reachable distress mechanism." end)}' >"$output"
echo "scored P7b development: $classification"
