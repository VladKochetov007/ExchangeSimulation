#!/usr/bin/env bash
# Extract P2's complete preregistered evidence contract. This script does not
# prune raw JSONL: no result is safe to prune after this command alone.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-3-p2"
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

write_cdf_inventory() {
	local cell=$1
	local output="$cell/cdf-maker-inventory.json"
	local temp
	temp=$(mktemp "${output}.tmp-XXXXXX")
	local -a logs=()
	while IFS= read -r -d '' file; do logs+=("$file"); done < <(find "$cell/venues" -type f -name 'general.jsonl' -print0 | sort -z)
	if [[ ${#logs[@]} -eq 0 ]]; then
		echo "no general evidence logs for P2 inventory extraction: $cell" >&2
		exit 1
	fi
	jq -s '
	  def abs: if . < 0 then -. else . end;
	  def mean: if length == 0 then null else add / length end;
	  [ .[] |
	    select(.event == "maker_state") |
	    select((.data.payload.maker? | type) == "string") |
	    select(.data.payload.maker | test("^cdf_[0-9]+$")) |
	    {venue_id: .data.venue_id, timestamp: .sim_ts, maker: .data.payload.maker,
	     inventory: .data.payload.inventory, net_delta: .data.payload.net_delta} |
	    select(.venue_id != null and (.inventory | type) == "number")
	  ] as $rows |
	  ($rows | sort_by(.venue_id, .maker, .timestamp) | group_by([.venue_id, .maker]) |
	    map(. as $series |
	      ($series | map(.inventory)) as $inventory |
	      {venue_id: $series[0].venue_id, maker: $series[0].maker,
	       observations: ($series | length),
	       mean_abs_inventory: ($inventory | map(abs) | mean),
	       max_abs_inventory: ($inventory | map(abs) | max),
	       terminal_inventory: $inventory[-1]}
	    )
	  ) as $series |
	  {
	    schema_version: 1,
	    source: "persisted maker_state inventory; CDF maker labels are stable actor-state labels, not a substitute for P2 fill evidence",
	    series: $series,
	    aggregate: {
	      series: ($series | length),
	      observations: ($series | map(.observations) | add // 0),
	      mean_abs_inventory: (if ($series | length) == 0 then null else ($series | map(.mean_abs_inventory * .observations) | add) / ($series | map(.observations) | add) end),
	      max_abs_inventory: (if ($series | length) == 0 then null else ($series | map(.max_abs_inventory) | max) end),
	      terminal_inventory_sum: ($series | map(.terminal_inventory) | add // 0)
	    }
	  }
	' "${logs[@]}" >"$temp"
	jq -e '.aggregate.series == 6 and .aggregate.observations > 0' "$temp" >/dev/null
	mv "$temp" "$output"
}

write_cdf_trade_ratio() {
	local cell=$1
	local output="$cell/cdf-trade-price-ratio.json"
	local temp
	temp=$(mktemp "${output}.tmp-XXXXXX")
	local -a logs=()
	while IFS= read -r -d '' file; do logs+=("$file"); done < <(find "$cell/venues" -type f -path '*/spot/CDF-USD.jsonl' -print0 | sort -z)
	if [[ ${#logs[@]} -eq 0 ]]; then
		echo "no CDF/USD spot raw evidence for P2 price ratio: $cell" >&2
		exit 1
	fi
	jq -s '
	  [ .[] | select(.event == "Trade") | .data.payload.price ] as $prices |
	  if ($prices | length) == 0 then
	    {schema_version: 1, available: false, reason: "no_executed_cdf_spot_trade", trades: 0}
	  elif $prices[0] == 0 then
	    {schema_version: 1, available: false, reason: "opening_trade_price_zero_ratio_undefined", trades: ($prices | length), opening_trade_price: $prices[0], terminal_trade_price: $prices[-1]}
	  else
	    {schema_version: 1, available: true, trades: ($prices | length), opening_trade_price: $prices[0], terminal_trade_price: $prices[-1], terminal_opening_price_ratio: ($prices[-1] / $prices[0])}
	  end
	' "${logs[@]}" >"$temp"
	mv "$temp" "$output"
}

for arm in A B; do
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
			echo "incomplete V2-3 P2 cell (needs final greeks.json + latency.json): $cell" >&2
			exit 1
		fi
		if [[ ! -s "$cell/run-config.json" || ! -s "$cell/run-metadata.json" ]]; then
			echo "missing P2 provenance input: $cell" >&2
			exit 1
		fi

		write_metric "$cell/observationreceipts.json" "$analyzer" -metric observationreceipts -json "$cell"
		write_metric "$cell/evidenceartifacthash.json" "$analyzer" -metric evidenceartifacthash -json "$cell"
		write_metric "$cell/makerrebalance.json" "$analyzer" -metric makerrebalance -json "$cell"
		write_metric "$cell/viability.json" "$analyzer" -metric viability -json -viability-window 60 "$cell"
		write_cdf_inventory "$cell"
		write_cdf_trade_ratio "$cell"

		temp=$(mktemp "$cell/cdf-viability.json.tmp-XXXXXX")
		jq '{
		  windows: [.result.windows[] | select(.symbol == "CDF/USD") | . + {
		    two_sided_share: (if .snapshots == 0 then null else 1 - (.empty_side_snapshots / .snapshots) end)
		  }],
		  summaries: [.result.book_summaries[] | select(.symbol == "CDF/USD")]
		}' "$cell/viability.json" >"$temp"
		jq -e '.windows | length > 0' "$temp" >/dev/null
		mv "$temp" "$cell/cdf-viability.json"

		jq -n \
			--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
			--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
			'{
			  analysis_revision: $analysis_revision,
			  analyzer_sha256: $analyzer_sha256,
			  completion_sentinels: ["greeks.json", "latency.json"],
			  required_artifacts: [
			    "observationreceipts.json", "evidenceartifacthash.json", "makerrebalance.json",
			    "viability.json", "cdf-viability.json", "cdf-maker-inventory.json", "cdf-trade-price-ratio.json"
			  ],
			  raw_log_policy: "retained; no P2 raw evidence is prunable from this script"
			}' >"$cell/analysis-metadata.json"
		echo "extracted P2 $arm/$seed"
	done
done
