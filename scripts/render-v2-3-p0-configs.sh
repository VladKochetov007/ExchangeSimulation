#!/usr/bin/env bash
# Render immutable V2-3 P0 passive-refresh inputs from the retained frozen
# population. Only the declared A/B/C policy fields, provenance labels, and
# V2 information-boundary evidence switches differ from the source config.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base_config="$root_dir/research/configs/frozen-baseline-2026-08-22.json"
output_dir="$root_dir/research/configs/v2-3-p0"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-3 P0 configs" >&2
	exit 1
fi

mkdir -p "$output_dir"

render() {
	local arm=$1
	local post_only=$2
	local cancel_before_replace=$3
	local seed=$4

	jq \
		--arg arm "$arm" \
		--argjson post_only "$post_only" \
		--argjson cancel_before_replace "$cancel_before_replace" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-3-p0-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-3-P0" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Five-minute passive refresh A/B/C mechanism screen; no price-stability claim." |
		.seed = $seed |
		.log_mode = "full" |
		.record_market_data_receipts = true |
		.record_decision_frontier_vectors = false |
		.market_data_receipt_roles = ["spot_maker", "fixed_distance_maker", "imbalance_maker"] |
		.spot_passive_maker_post_only = $post_only |
		.spot_passive_maker_cancel_before_replace = $cancel_before_replace
		' "$base_config" >"$output_dir/$arm-$seed.json"
}

for seed in 101 103; do
	render A false false "$seed"
	render B true false "$seed"
	render C true true "$seed"
done
