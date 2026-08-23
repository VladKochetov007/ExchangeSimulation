#!/usr/bin/env bash
# Render the immutable V2-2b five-minute factorial inputs from the retained
# ae13f9a baseline population. The generated files are evidence inputs, not
# simulator defaults; do not edit them after a cell has begun.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base_config="$root_dir/research/configs/frozen-baseline-2026-08-22.json"
output_dir="$root_dir/research/configs/v2-2b"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-2b configs" >&2
	exit 1
fi

mkdir -p "$output_dir"

render() {
	local arm=$1
	local informed=$2
	local router=$3
	local seed=$4
	local roles='["spot_maker"]'
	local remote_feeds='[]'
	local tiers='[]'

	if [[ "$informed" == "1" ]]; then
		roles='["spot_maker","v2_remote_feed"]'
		remote_feeds='[
  {"target_venue":"north","target_maker":1,"source_venue":"south","symbol":"ABC/USD","weight":0.50,"confidence":0.80,"max_observation_age":2000000000,"latency":{"model":"constant","delay":10000000}},
  {"target_venue":"central","target_maker":1,"source_venue":"north","symbol":"ABC/USD","weight":0.35,"confidence":0.90,"max_observation_age":4000000000,"latency":{"model":"constant","delay":20000000}},
  {"target_venue":"south","target_maker":1,"source_venue":"central","symbol":"ABC/USD","weight":0.45,"confidence":0.60,"max_observation_age":6000000000,"latency":{"model":"constant","delay":30000000}}
]'
	fi
	if [[ "$router" == "1" ]]; then
		roles=$(jq -cn --argjson current "$roles" '$current + ["cross_venue_router_tier"]')
		tiers='[1]'
	fi

	jq \
		--arg arm "$arm" \
		--argjson seed "$seed" \
		--argjson roles "$roles" \
		--argjson remote_feeds "$remote_feeds" \
		--argjson tiers "$tiers" \
		'
		.experiment_id = ("v2-2b-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-2b" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Five-minute informed-maker x explicit-router feasibility factorial; no market-level claim." |
		.seed = $seed |
		.log_mode = "full" |
		.maker_anchor = "own_mid" |
		.spot_maker_local_reference_cache = true |
		.record_market_data_receipts = true |
		.record_decision_frontier_vectors = true |
		.market_data_receipt_roles = $roles |
		.remote_maker_feeds = $remote_feeds |
		.cross_venue_arb_tiers = $tiers |
		.cross_venue_base_latency = 1000000000 |
		.cross_venue_arb_lot_qty = 1000000 |
		.cross_venue_arb_max_attempts = 100 |
		.latency_profiles.spot_maker = {"model":"constant","delay":10000000} |
		.metaorder_trader_count = 1 |
		.metaorder_traders.min_qty = 10000000000 |
		.metaorder_traders.max_qty = 10000000000 |
		.metaorder_traders.min_child_qty = 500000000 |
		.metaorder_traders.participation_rate = 0 |
		.metaorder_traders.max_slippage_bps = 50 |
		.metaorder_traders.rest_interval = 300000000000
		' "$base_config" >"$output_dir/$arm-$seed.json"
}

for seed in 101 103; do
	render I0R0 0 0 "$seed"
	render I1R0 1 0 "$seed"
	render I0R1 0 1 "$seed"
	render I1R1 1 1 "$seed"
done
