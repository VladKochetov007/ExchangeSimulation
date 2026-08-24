#!/usr/bin/env bash
# Render immutable V2-3 P2 inventory-rebalance cells from the completed P1-B
# parent. P2 may add only its evidence coverage and declared policy; A/B differ
# economically only by the policy's enabled flag.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
parent_dir="$root_dir/research/configs/v2-3-p1"
output_dir="$root_dir/research/configs/v2-3-p2"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-3 P2 configs" >&2
	exit 1
fi

mkdir -p "$output_dir"

render() {
	local arm=$1
	local enabled=$2
	local seed=$3
	local parent="$parent_dir/B-$seed.json"
	local output="$output_dir/$arm-$seed.json"

	if [[ ! -f "$parent" ]]; then
		echo "missing P1-B parent: $parent" >&2
		exit 1
	fi

	jq \
		--arg arm "$arm" \
		--argjson enabled "$enabled" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-3-p2-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-3-P2" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Five-minute CDF/USD explicit local IOC inventory-rebalance screen; P0-C and P1-B fixed." |
		.seed = $seed |
		.record_maker_inventory_rebalance_decisions = true |
		.market_data_receipt_roles = (
			(.market_data_receipt_roles // []) |
			if index("cdf_spot_maker") then . else . + ["cdf_spot_maker"] end
		) |
		.cdf_inventory_rebalance = {
			enabled: $enabled,
			interval: 10000000000,
			cooldown: 30000000000,
			risk_band_qty: 10000000000,
			target_band_qty: 5000000000,
			max_request_qty: 500000000,
			participation_bps: 1000,
			slippage_bps: 50
		}
		' "$parent" >"$output"

	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .record_maker_inventory_rebalance_decisions, .market_data_receipt_roles, .cdf_inventory_rebalance)' "$parent") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .record_maker_inventory_rebalance_decisions, .market_data_receipt_roles, .cdf_inventory_rebalance)' "$output"); then
		echo "P2 config contains an undeclared change: $output" >&2
		exit 1
	fi
}

for seed in 101 103; do
	render A false "$seed"
	render B true "$seed"
done

for seed in 101 103; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_inventory_rebalance.enabled)' "$output_dir/A-$seed.json") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_inventory_rebalance.enabled)' "$output_dir/B-$seed.json"); then
		echo "P2 A/B configs differ outside the declared enabled treatment: seed $seed" >&2
		exit 1
	fi
done
