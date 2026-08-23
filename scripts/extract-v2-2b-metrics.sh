#!/usr/bin/env bash
# Recompute every V2-2b derived metric from retained evidence. Completion is
# established only by final greeks.json and latency.json; this script never
# interprets a process name or a partial output directory as a completed run.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
artifact_dir="$root_dir/research/artifacts/v2-2b"
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

for arm in I0R0 I1R0 I0R1 I1R1; do
	for seed in 101 103; do
		cell="$artifact_dir/$arm/seed-$seed"
		if [[ ! -s "$cell/greeks.json" || ! -s "$cell/latency.json" ]]; then
			echo "incomplete V2-2b cell (needs final greeks.json + latency.json): $cell" >&2
			exit 1
		fi
		if [[ ! -s "$cell/run-config.json" || ! -s "$cell/run-metadata.json" ]]; then
			echo "missing V2-2b provenance input: $cell" >&2
			exit 1
		fi
		fee_bps=$(jq -er '.taker_fee_bps | numbers' "$cell/run-config.json")

		write_metric "$cell/observationreceipts.json" "$analyzer" -metric observationreceipts -json "$cell"
		write_metric "$cell/frontiervectors.json" "$analyzer" -metric frontiervectors -json "$cell"
		write_metric "$cell/evidenceartifacthash.json" "$analyzer" -metric evidenceartifacthash -json "$cell"
		write_metric "$cell/crossvenue.json" "$analyzer" -metric crossvenue -json -arb-staleness 2 \
			-cross-venue-symbol ABC-USD -cross-venue-min-venues 3 -cross-venue-positive-times "$cell"
		write_metric "$cell/arbitrage.json" "$analyzer" -metric arbitrage -json -arb-fee-bps "$fee_bps" -arb-staleness 2 "$cell"

		temp=$(mktemp "$cell/activation.json.tmp-XXXXXX")
		jq '{
		  metaorders: [.metaorders[] | {
		    venue_id, trader_id, side, parent_qty, filled_qty,
		    start_timestamp, end_timestamp, vwap, child_count, completed,
		    signed_impact, market_volume_during_execution
		  }],
		  routers: [(.router_reports // [])[] | {
		    tier, router_id, executable_signals, submitted_groups, completed_groups,
		    failed_groups, pending_groups, buy_filled_qty, sell_filled_qty,
		    buy_notional, sell_notional, quote_fees, unpriced_fee_count,
		    completed_quote_cashflow, completed_cashflow_groups, residual_base_qty,
		    computed: {
		      submitted_groups: (.groups | length),
		      completed_groups: ([.groups[] | select(.complete)] | length),
		      failed_groups: ([.groups[] | select(.failed)] | length),
		      pending_groups: ([.groups[] | select((.complete or .failed) | not)] | length),
		      buy_filled_qty: ([.groups[].buy.filled_qty] | add // 0),
		      sell_filled_qty: ([.groups[].sell.filled_qty] | add // 0),
		      buy_notional: ([.groups[].buy.notional] | add // 0),
		      sell_notional: ([.groups[].sell.notional] | add // 0),
		      quote_fees: ([.groups[].buy.quote_fees, .groups[].sell.quote_fees] | add // 0),
		      unpriced_fee_count: ([.groups[].buy.unpriced_fee_count, .groups[].sell.unpriced_fee_count] | add // 0),
		      completed_quote_cashflow: ([.groups[] | select(.complete and .quote_cashflow_valid) | .quote_cashflow] | add // 0),
		      completed_cashflow_groups: ([.groups[] | select(.complete and .quote_cashflow_valid)] | length),
		      residual_base_qty: (([.groups[].buy.filled_qty] | add // 0) - ([.groups[].sell.filled_qty] | add // 0))
		    }
		  }]
		}' "$cell/greeks.json" >"$temp"
		jq -e '.routers | all(.[];
		  .submitted_groups == .computed.submitted_groups and
		  .completed_groups == .computed.completed_groups and
		  .failed_groups == .computed.failed_groups and
		  .pending_groups == .computed.pending_groups and
		  .buy_filled_qty == .computed.buy_filled_qty and
		  .sell_filled_qty == .computed.sell_filled_qty and
		  .buy_notional == .computed.buy_notional and
		  .sell_notional == .computed.sell_notional and
		  .quote_fees == .computed.quote_fees and
		  .unpriced_fee_count == .computed.unpriced_fee_count and
		  .completed_quote_cashflow == .computed.completed_quote_cashflow and
		  .completed_cashflow_groups == .computed.completed_cashflow_groups and
		  .residual_base_qty == .computed.residual_base_qty
		)' "$temp" >/dev/null
		mv "$temp" "$cell/activation.json"

		temp=$(mktemp "$cell/analysis-metadata.json.tmp-XXXXXX")
		jq -n \
			--arg analysis_revision "$(git -C "$root_dir" rev-parse HEAD)" \
			--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
			--argjson fee_bps "$fee_bps" \
			'{
			  analysis_revision: $analysis_revision,
			  analyzer_sha256: $analyzer_sha256,
			  crossvenue_staleness_seconds: 2,
			  crossvenue_min_venues: 3,
			  executable_edge_taker_fee_bps: $fee_bps,
			  completion_sentinels: ["greeks.json", "latency.json"]
			}' >"$temp"
		mv "$temp" "$cell/analysis-metadata.json"
		echo "extracted $arm/$seed"
	done
done
