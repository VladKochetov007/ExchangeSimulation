#!/usr/bin/env bash
# Extract the full L1-P3 holdout evidence contract. It never prunes raw logs.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-4-l1p3"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

if [[ ! -x "$analyzer" ]]; then
	echo "missing executable analyzer: $analyzer" >&2
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

for arm in A B C D; do
	case "$arm" in
		A) liability_phase=0; noise_phase=0 ;;
		B) liability_phase=1000000000; noise_phase=0 ;;
		C) liability_phase=0; noise_phase=1000000000 ;;
		D) liability_phase=1000000000; noise_phase=1000000000 ;;
	esac
	for seed in 107 109 113; do
		cell="$artifact_dir/$arm/seed-$seed"
		if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
			echo "incomplete V2-4 L1-P3 cell (needs final greeks.json + latency.json): $cell" >&2
			exit 1
		fi
		if [[ ! -s "$cell/run-config.json" || ! -s "$cell/run-metadata.json" ]]; then
			echo "missing L1-P3 provenance input: $cell" >&2
			exit 1
		fi

		expected_actors=$(jq '.noise_trader_count * (.venue_ids | length)' "$cell/run-config.json")
		write_metric "$cell/observationreceipts.json" "$analyzer" -metric observationreceipts -json "$cell"
		write_metric "$cell/evidenceartifacthash.json" "$analyzer" -metric evidenceartifacthash -json "$cell"
		write_metric "$cell/liabilityhedger.json" "$analyzer" -metric liabilityhedger -json "$cell"
		write_metric "$cell/noiseflowphase.json" "$analyzer" -metric noiseflowphase -json "$cell"
		write_metric "$cell/viability.json" "$analyzer" -metric viability -json -viability-window 60 "$cell"
		write_metric "$cell/viability-after-warmup.json" "$analyzer" -metric viability -json -viability-window 60 -viability-start 10 "$cell"

		jq -e --argjson phase "$liability_phase" '
			.result.valid == true and .result.policy_mode == "delivery_liability" and
			.result.phase_configured == true and .result.decision_phase_offset_nanos == $phase and
			.result.decisions > 0 and .result.state_updates > 0 and (.result.hedgers | length) == 3 and
			all(.result.hedgers[]; .state_updates >= 120 and .accepted >= 1) and .result.non_reducing_fills == 0
		' "$cell/liabilityhedger.json" >/dev/null
		jq -e --argjson phase "$noise_phase" --argjson actors "$expected_actors" '
			.result.valid == true and .result.decision_phase_offset_nanos == $phase and
			.result.expected_participants == $actors and .result.expected_ticks_per_participant > 0 and
			.result.decisions == (.result.expected_participants * .result.expected_ticks_per_participant) and
			.result.subscribe_decisions == .result.expected_participants and
			.result.evaluate_decisions == (.result.decisions - .result.expected_participants) and
			.result.missing_ticks == 0 and .result.duplicate_ticks == 0 and .result.off_phase_ticks == 0 and
			.result.phase_mismatches == 0 and .result.action_mismatches == 0 and .result.extra_ticks == 0
		' "$cell/noiseflowphase.json" >/dev/null
		jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
		jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) > 0' "$cell/evidenceartifacthash.json" >/dev/null

		jq -n \
			--arg arm "$arm" --argjson seed "$seed" --argjson liability_phase "$liability_phase" --argjson noise_phase "$noise_phase" \
			--slurpfile full "$cell/viability.json" --slurpfile warm "$cell/viability-after-warmup.json" '
			def cdf($report; $venue): [$report.result.book_summaries[] | select(.symbol == "CDF/USD" and .venue_id == $venue)][0];
			[["central", "north", "south"][] as $venue |
			 (cdf($full[0]; $venue)) as $whole | (cdf($warm[0]; $venue)) as $after |
			 {venue_id:$venue, trades:($whole.trades // 0), taker_roles:($whole.taker_roles // 0), maker_roles:($whole.maker_roles // 0),
			  snapshots_after_warmup:($after.snapshots // 0), empty_side_snapshots_after_warmup:($after.empty_side_snapshots // 0),
			  two_sided_snapshot_share_after_warmup:(if ($after.snapshots // 0)>0 then (($after.snapshots-$after.empty_side_snapshots)/$after.snapshots) else null end)}] as $venues |
			 {schema_version:1, experiment_id:"V2-4-L1-P3", arm:$arm, seed:$seed,
			  liability_decision_phase_offset_nanos:$liability_phase, noise_flow_decision_phase_offset_nanos:$noise_phase,
			  measurement_contract:"minimum non-collapse floor only; not an ecology-viability result", warmup_excluded_seconds:10, venues:$venues,
			  passes_noncollapse_floor:(all($venues[]; .trades>=150 and .taker_roles>=2 and .maker_roles>=1 and .snapshots_after_warmup>0 and (.empty_side_snapshots_after_warmup*100 <= .snapshots_after_warmup*5)))}
		' >"$cell/l1p3-activity-gate.json"

		jq -n --arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" --arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" '
			{analysis_revision:$analysis_revision, analyzer_sha256:$analyzer_sha256,
			 completion_sentinels:["greeks.json","latency.json"],
			 required_artifacts:["observationreceipts.json","evidenceartifacthash.json","liabilityhedger.json","noiseflowphase.json","viability.json","viability-after-warmup.json","l1p3-activity-gate.json"],
			 raw_log_policy:"retained; no L1-P3 raw evidence is prunable from this script"}
		' >"$cell/analysis-metadata.json"
		echo "extracted L1-P3 $arm/$seed"
	done
done
