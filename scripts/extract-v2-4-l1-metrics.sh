#!/usr/bin/env bash
# Extract V2-4 L1's complete preregistered evidence contract. It never prunes
# raw evidence and records an activity-floor failure as an outcome, not as a
# reason to discard or overwrite the cell.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-4-l1"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

if [[ ! -x "$analyzer" ]]; then
	echo "missing executable analyzer: $analyzer" >&2
	exit 1
fi

write_metric() {
	local output=$1
	shift
	local temp
	temp=$(mktemp "${output}.tmp-XXXXXX")
	"$@" >"$temp"
	mv "$temp" "$output"
}

for arm in A B; do
	case "$arm" in
		A) expected_mode=random_side_control ;;
		B) expected_mode=delivery_liability ;;
	esac
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
			echo "incomplete V2-4 L1 cell (needs final greeks.json + latency.json): $cell" >&2
			exit 1
		fi
		if [[ ! -s "$cell/run-config.json" || ! -s "$cell/run-metadata.json" ]]; then
			echo "missing L1 provenance input: $cell" >&2
			exit 1
		fi

		write_metric "$cell/observationreceipts.json" "$analyzer" -metric observationreceipts -json "$cell"
		write_metric "$cell/evidenceartifacthash.json" "$analyzer" -metric evidenceartifacthash -json "$cell"
		write_metric "$cell/liabilityhedger.json" "$analyzer" -metric liabilityhedger -json "$cell"
		write_metric "$cell/viability.json" "$analyzer" -metric viability -json -viability-window 60 "$cell"
		write_metric "$cell/viability-after-warmup.json" "$analyzer" -metric viability -json -viability-window 60 -viability-start 10 "$cell"

		# Evidence integrity is a hard extraction condition. Activity is instead
		# recorded below as a scientific outcome; it is never silently filtered.
		jq -e --arg mode "$expected_mode" '
			.result.valid == true and .result.policy_mode == $mode and
			.result.decisions > 0 and .result.state_updates > 0 and
			(.result.hedgers | length) == 3 and
			all(.result.hedgers[]; .state_updates >= 120 and .accepted >= 1) and
			(if $mode == "delivery_liability" then .result.non_reducing_fills == 0 else true end)
		' "$cell/liabilityhedger.json" >/dev/null
		jq -e '.result.valid == true' "$cell/observationreceipts.json" >/dev/null
		jq -e '.result.events > 0 and (.result.digest | type) == "string" and (.result.digest | length) > 0' "$cell/evidenceartifacthash.json" >/dev/null

		jq -n \
			--arg arm "$arm" \
			--argjson seed "$seed" \
			--slurpfile full "$cell/viability.json" \
			--slurpfile warm "$cell/viability-after-warmup.json" \
			'
			def cdf($report; $venue):
			  [$report.result.book_summaries[] | select(.symbol == "CDF/USD" and .venue_id == $venue)][0];
			[
			  ["central", "north", "south"][] as $venue |
			  (cdf($full[0]; $venue)) as $whole |
			  (cdf($warm[0]; $venue)) as $after_warmup |
			  {
			    venue_id: $venue,
			    trades: ($whole.trades // 0),
			    taker_roles: ($whole.taker_roles // 0),
			    maker_roles: ($whole.maker_roles // 0),
			    snapshots_after_warmup: ($after_warmup.snapshots // 0),
			    empty_side_snapshots_after_warmup: ($after_warmup.empty_side_snapshots // 0),
			    two_sided_snapshot_share_after_warmup:
			      (if ($after_warmup.snapshots // 0) > 0
			       then (($after_warmup.snapshots - $after_warmup.empty_side_snapshots) / $after_warmup.snapshots)
			       else null end)
			  }
			] as $venues |
			{
			  schema_version: 1,
			  experiment_id: "V2-4-L1",
			  arm: $arm,
			  seed: $seed,
			  measurement_contract: "minimum non-collapse floor only; not an ecology-viability result",
			  warmup_excluded_seconds: 10,
			  venues: $venues,
			  passes_noncollapse_floor:
			    (all($venues[];
			      .trades >= 150 and .taker_roles >= 2 and .maker_roles >= 1 and
			      .snapshots_after_warmup > 0 and
			      (.empty_side_snapshots_after_warmup * 100 <= .snapshots_after_warmup * 5)))
			}' >"$cell/l1-activity-gate.json"

		jq -n \
			--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
			--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
			'{
			  analysis_revision: $analysis_revision,
			  analyzer_sha256: $analyzer_sha256,
			  completion_sentinels: ["greeks.json", "latency.json"],
			  required_artifacts: [
			    "observationreceipts.json", "evidenceartifacthash.json", "liabilityhedger.json",
			    "viability.json", "viability-after-warmup.json", "l1-activity-gate.json"
			  ],
			  raw_log_policy: "retained; no L1 raw evidence is prunable from this script"
			}' >"$cell/analysis-metadata.json"
		echo "extracted L1 $arm/$seed"
	done
done
