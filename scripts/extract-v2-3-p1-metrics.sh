#!/usr/bin/env bash
# Extract the complete preregistered P1 evidence contract. This script never
# prunes raw JSONL; completion requires final sidecars, exact evidence identity,
# participant-information replay, decision/request joins, and viability rows.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-3-p1"
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

write_maker_delta() {
	local cell=$1
	local output="$cell/maker-net-delta.json"
	local temp
	temp=$(mktemp "${output}.tmp-XXXXXX")
	local -a logs=()
	while IFS= read -r -d '' file; do logs+=("$file"); done < <(find "$cell/venues" -type f -name '*.jsonl' -print0 | sort -z)
	if [[ ${#logs[@]} -eq 0 ]]; then
		echo "no raw evidence logs for P1 maker-state extraction: $cell" >&2
		exit 1
	fi
	jq -s '
	  def abs: if . < 0 then -. else . end;
	  def mean: if length == 0 then null else add / length end;
	  def lag1:
	    . as $values | ($values | length) as $n |
	    if $n < 2 then null else
	      $values[0:$n-1] as $previous |
	      $values[1:$n] as $next |
	      ($previous | mean) as $previous_mean |
	      ($next | mean) as $next_mean |
	      ([range(0; $n-1) | (($previous[.] - $previous_mean) * ($next[.] - $next_mean))] | add) as $covariance |
	      ([$previous[] | (. - $previous_mean) * (. - $previous_mean)] | add) as $variance |
	      if $variance == 0 then null else $covariance / $variance end
	    end;
	  [ .[] |
	    select(.event == "maker_state") |
	    {
	      venue_id: .data.venue_id,
	      timestamp: .sim_ts,
	      maker: .data.payload.maker,
	      net_delta: .data.payload.net_delta
	    } |
	    select(.venue_id != null and (.maker | test("^(spot|cdf|cross)_[0-9]+$")) and (.net_delta | type) == "number")
	  ] as $rows |
	  ($rows | sort_by(.venue_id, .maker, .timestamp) | group_by([.venue_id, .maker]) |
	    map(. as $series |
	      ($series | map(.net_delta)) as $values |
	      {
	        venue_id: $series[0].venue_id,
	        maker: $series[0].maker,
	        observations: ($values | length),
	        mean_abs_net_delta: ($values | map(abs) | mean),
	        lag1_autocorrelation: ($values | lag1)
	      }
	    )
	  ) as $series |
	  {
	    schema_version: 1,
	    source: "persisted maker_state events; lag-1 is within venue/maker ordered state series",
	    included_maker_labels: "^(spot|cdf|cross)_[0-9]+$",
	    series: $series,
	    aggregate: (
	      ($series | length) as $series_count |
	      ($series | map(.observations) | add) as $observations |
	      ($series | map(.mean_abs_net_delta * .observations) | add) as $weighted_abs_net_delta |
	      ($series | map(select(.lag1_autocorrelation != null) | .lag1_autocorrelation) | mean) as $mean_lag1 |
	      {
	        series: $series_count,
	        observations: $observations,
	        mean_abs_net_delta: ($weighted_abs_net_delta / $observations),
	        mean_lag1_autocorrelation: $mean_lag1
	      }
	    )
	  }
	' "${logs[@]}" >"$temp"
	jq -e '.aggregate.series > 0 and .aggregate.observations > 0' "$temp" >/dev/null
	mv "$temp" "$output"
}

write_spot_trade_price_ratio() {
	local cell=$1
	local output="$cell/spot-trade-price-ratio.json"
	local scratch
	scratch=$(mktemp -d -t v2-3-p1-price-ratio-XXXXXX)
	local file venue filename symbol row
	while IFS= read -r -d '' file; do
		venue=$(basename "$(dirname "$(dirname "$file")")")
		filename=$(basename "$file")
		case "$filename" in
			ABC-USD.jsonl) symbol="ABC/USD" ;;
			CDF-USD.jsonl) symbol="CDF/USD" ;;
			ABC-CDF.jsonl) symbol="ABC/CDF" ;;
			*) continue ;;
		esac
		row="$scratch/${venue}-${filename}.json"
		jq -s --arg venue "$venue" --arg symbol "$symbol" '
		  [ .[] | select(.event == "Trade") | .data.payload.price ] as $prices |
		  if ($prices | length) == 0 then
		    {venue_id: $venue, symbol: $symbol, available: false, reason: "no_executed_spot_trade"}
		  elif $prices[0] == 0 then
		    {venue_id: $venue, symbol: $symbol, available: false, reason: "opening_trade_price_zero_ratio_undefined", trades: ($prices | length), opening_trade_price: $prices[0], terminal_trade_price: $prices[-1]}
		  else
		    {venue_id: $venue, symbol: $symbol, available: true, trades: ($prices | length), opening_trade_price: $prices[0], terminal_trade_price: $prices[-1], terminal_opening_price_ratio: ($prices[-1] / $prices[0])}
		  end
		' "$file" >"$row"
	done < <(find "$cell/venues" -type f -path '*/spot/*.jsonl' -print0 | sort -z)
	if ! compgen -G "$scratch/*.json" >/dev/null; then
		echo "no scoped spot raw evidence for P1 trade-price ratio: $cell" >&2
		exit 1
	fi
	jq -s '{
	  schema_version: 1,
	  source: "first and terminal executed trade per venue/spot book; ratio explicitly unavailable when no trade or opening price is zero",
	  rows: sort_by(.venue_id, .symbol)
	}' "$scratch"/*.json >"$output"
}

for arm in A B; do
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
			echo "incomplete V2-3 P1 cell (needs final greeks.json + latency.json): $cell" >&2
			exit 1
		fi
		if [[ ! -s "$cell/run-config.json" || ! -s "$cell/run-metadata.json" ]]; then
			echo "missing P1 provenance input: $cell" >&2
			exit 1
		fi

		write_metric "$cell/observationreceipts.json" "$analyzer" -metric observationreceipts -json "$cell"
		write_metric "$cell/evidenceartifacthash.json" "$analyzer" -metric evidenceartifacthash -json "$cell"
		write_metric "$cell/makerquotesize.json" "$analyzer" -metric makerquotesize -json "$cell"
		write_metric "$cell/viability.json" "$analyzer" -metric viability -json -viability-window 60 "$cell"
		write_maker_delta "$cell"
		write_spot_trade_price_ratio "$cell"

		temp=$(mktemp "$cell/spot-viability.json.tmp-XXXXXX")
		jq '{
		  windows: [.result.windows[] | select(.symbol == "ABC/USD" or .symbol == "CDF/USD" or .symbol == "ABC/CDF") | . + {
		    two_sided_share: (if .snapshots == 0 then null else 1 - (.empty_side_snapshots / .snapshots) end)
		  }],
		  summaries: [.result.book_summaries[] | select(.symbol == "ABC/USD" or .symbol == "CDF/USD" or .symbol == "ABC/CDF")]
		}' "$cell/viability.json" >"$temp"
		jq -e '.windows | length > 0' "$temp" >/dev/null
		mv "$temp" "$cell/spot-viability.json"

		jq -n \
			--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
			--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
			'{
			  analysis_revision: $analysis_revision,
			  analyzer_sha256: $analyzer_sha256,
			  completion_sentinels: ["greeks.json", "latency.json"],
			  required_artifacts: [
			    "observationreceipts.json", "evidenceartifacthash.json", "makerquotesize.json",
			    "viability.json", "spot-viability.json", "maker-net-delta.json", "spot-trade-price-ratio.json"
			  ],
			  raw_log_policy: "retained; no P1 raw evidence is prunable from this script"
			}' >"$cell/analysis-metadata.json"
		echo "extracted P1 $arm/$seed"
	done
done
