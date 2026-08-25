#!/usr/bin/env bash
# Render the V2-3 P3 R1 scorecard from already-extracted, retained evidence.
# It neither reruns worlds nor authorizes pruning.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-3-p3-r1"
output="$artifact_dir/p3-r1-summary.json"

require_file() {
	if [[ ! -s $1 ]]; then
		echo "missing required P3 R1 artifact: $1" >&2
		exit 1
	fi
}

row_file=$(mktemp "$artifact_dir/.p3-r1-row-XXXXXX.json")
summary_file=$(mktemp "${output}.tmp-XXXXXX")
trap 'rm -f "$row_file" "$summary_file"' EXIT

rows='[]'
for arm in A B; do
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		for file in run-metadata.json evidenceartifacthash.json perpreplenishment.json observationreceipts.json orderlifecycle.json viability.json; do
			require_file "$cell/$file"
		done

		jq -e '
		  .result.valid == true and .result.decisions > 0 and
		  (.result.invalid_decision_records // 0) == 0 and
		  (.result.invalid_lifecycle_records // 0) == 0 and
		  (.result.lifecycle_mismatches // 0) == 0 and
		  (.result.threshold_mismatches // 0) == 0 and
		  (.result.missing_outcomes // 0) == 0 and
		  (.result.duplicate_outcomes // 0) == 0 and
		  (.result.outcome_field_mismatches // 0) == 0
		' "$cell/perpreplenishment.json" >/dev/null
		jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
		jq -e '.result.events > 0 and (.result.digest | length) == 64' "$cell/evidenceartifacthash.json" >/dev/null
		jq -e '
		  .result.accepted > 0 and
		  (.result.missing_immediate_terminal // 0) == 0 and
		  (.result.unknown_fills // 0) == 0 and
		  (.result.unknown_cancellations // 0) == 0 and
		  (.result.duplicate_acceptances // 0) == 0 and
		  (.result.duplicate_terminals // 0) == 0 and
		  (.result.fills_after_terminal // 0) == 0 and
		  (.result.fill_quantity_mismatches // 0) == 0 and
		  (.result.cancel_quantity_mismatches // 0) == 0 and
		  (.result.client_mismatches // 0) == 0
		' "$cell/orderlifecycle.json" >/dev/null
		jq -e '
		  [.result.book_summaries[] | select(.symbol == "ABC-PERP")] as $books |
		  ($books | length) == 3 and
		  ($books | map(.snapshots) | add) > 0 and
		  ($books | map(.empty_side_snapshots) | add) < ($books | map(.snapshots) | add) and
		  ($books | map(.trades) | add) > 0
		' "$cell/viability.json" >/dev/null

		jq -n \
			--arg arm "$arm" \
			--argjson seed "$seed" \
			--slurpfile run "$cell/run-metadata.json" \
			--slurpfile evidence "$cell/evidenceartifacthash.json" \
			--slurpfile replay "$cell/perpreplenishment.json" \
			--slurpfile receipts "$cell/observationreceipts.json" \
			--slurpfile lifecycle "$cell/orderlifecycle.json" \
			--slurpfile viability "$cell/viability.json" '
			  ($viability[0].result.book_summaries | map(select(.symbol == "ABC-PERP"))) as $books |
			  {
			    arm: $arm,
			    seed: $seed,
			    provenance: {
			      config_sha256: $run[0].config_sha256,
			      binary_sha256: $run[0].binary_sha256,
			      git_revision: $run[0].git_revision,
			      gomaxprocs: $run[0].gomaxprocs,
			      evidence: {
			        domain: $evidence[0].result.domain,
			        ordering: $evidence[0].result.ordering,
			        events: $evidence[0].result.events,
			        digest: $evidence[0].result.digest
			      }
			    },
			    p3_replay: {
			      valid: $replay[0].result.valid,
			      decisions: $replay[0].result.decisions,
			      enabled_decisions: $replay[0].result.enabled_decisions,
			      disabled_decisions: $replay[0].result.disabled_decisions,
			      refresh_due: $replay[0].result.refresh_due,
			      no_refresh: $replay[0].result.no_refresh,
			      action_counts: $replay[0].result.action_counts,
			      lifecycle_rows: $replay[0].result.lifecycle_rows,
			      structural_mismatches: {
			        invalid_decision_records: ($replay[0].result.invalid_decision_records // 0),
			        invalid_lifecycle_records: ($replay[0].result.invalid_lifecycle_records // 0),
			        lifecycle_mismatches: ($replay[0].result.lifecycle_mismatches // 0),
			        threshold_mismatches: ($replay[0].result.threshold_mismatches // 0),
			        missing_outcomes: ($replay[0].result.missing_outcomes // 0),
			        duplicate_outcomes: ($replay[0].result.duplicate_outcomes // 0),
			        outcome_field_mismatches: ($replay[0].result.outcome_field_mismatches // 0)
			      }
			    },
			    information_boundary: {
			      valid: $receipts[0].result.valid,
			      schedules: $receipts[0].result.schedules,
			      receipts: $receipts[0].result.receipts,
			      decisions: $receipts[0].result.decisions
			    },
			    order_lifecycle: {
			      accepted: $lifecycle[0].result.accepted,
			      fill_records: $lifecycle[0].result.fill_records,
			      cancelled: $lifecycle[0].result.cancelled,
			      fully_filled: $lifecycle[0].result.fully_filled,
			      required_immediate_terminal: $lifecycle[0].result.required_immediate_terminal,
			      missing_immediate_terminal: ($lifecycle[0].result.missing_immediate_terminal // 0),
			      unknown_fills: ($lifecycle[0].result.unknown_fills // 0),
			      unknown_cancellations: ($lifecycle[0].result.unknown_cancellations // 0),
			      duplicate_acceptances: ($lifecycle[0].result.duplicate_acceptances // 0),
			      duplicate_terminals: ($lifecycle[0].result.duplicate_terminals // 0),
			      fills_after_terminal: ($lifecycle[0].result.fills_after_terminal // 0),
			      fill_quantity_mismatches: ($lifecycle[0].result.fill_quantity_mismatches // 0),
			      cancel_quantity_mismatches: ($lifecycle[0].result.cancel_quantity_mismatches // 0),
			      client_mismatches: ($lifecycle[0].result.client_mismatches // 0)
			    },
			    abc_perp_viability: {
			      venues: ($books | map(.venue_id)),
			      snapshots: ($books | map(.snapshots) | add),
			      empty_side_snapshots: ($books | map(.empty_side_snapshots) | add),
			      two_sided_share: (1 - (($books | map(.empty_side_snapshots) | add) / ($books | map(.snapshots) | add))),
			      trades: ($books | map(.trades) | add),
			      viable_windows: ($books | map(.viable) | add),
			      windows: ($books | map(.windows) | add)
			    },
			    gates: {
			      p3_replay_valid: true,
			      information_boundary_valid: true,
			      quote_decisions_active: true,
			      quote_order_activity_active: true,
			      perp_snapshots_present: true,
			      perp_two_sided_available: true,
			      perp_trading_active: true
			    }
			  }
			' >"$row_file"
		rows=$(jq --argjson row "$(<"$row_file")" '. + [$row]' <<<"$rows")
	done
done

jq -n --argjson rows "$rows" '
  {
    schema_version: 1,
    experiment: "V2-3-P3-R1",
    comparison_contract: {
      a: "perp_maker_replenish_below_bps = 0; policy disabled",
      b: "perp_maker_replenish_below_bps = 5000; strict below-half confirmed residual",
      primary_activation: "B must have a qualifying confirmed partial fill and refresh exactly at the next otherwise-unchanged decision",
      score_rule: "if a valid B cell has zero qualifying partial fills, the seed is NOT EXERCISED"
    },
    rows: $rows,
    score: (
      if ($rows | length) != 4 or ($rows | any(.gates[]; . == false)) then
        {verdict: "INVALID", reason: "required replay or ordinary viability gate failed"}
      elif ($rows | map(select(.arm == "A") | .p3_replay.refresh_due) | any(. != 0)) then
        {verdict: "INVALID", reason: "policy-disabled control emitted a replenishment trigger"}
      elif ($rows | map(select(.arm == "B") | .p3_replay.refresh_due) | all(. == 0)) then
        {verdict: "NOT EXERCISED", reason: "neither fixed-threshold treatment cell had a qualifying below-half confirmed residual at an audited decision"}
      else
        {verdict: "REQUIRES_MANUAL_SCORE", reason: "at least one treatment cell activated; inspect exact next-decision refresh matching before assigning a causal verdict"}
      end
    ),
    raw_evidence_policy: "retained; this compact scorecard does not authorize pruning"
  }
' >"$summary_file"

mv "$summary_file" "$output"
trap - EXIT
rm -f "$row_file"
printf 'wrote %s\n' "$output"
