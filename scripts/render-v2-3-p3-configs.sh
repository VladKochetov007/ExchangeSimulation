#!/usr/bin/env bash
# Render immutable V2-3 P3 passive-replenishment cells from the completed P1-B
# parent. The control/treatment differ only in the declared residual threshold.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
parent_dir="$root_dir/research/configs/v2-3-p1"
output_dir="$root_dir/research/configs/v2-3-p3"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-3 P3 configs" >&2
	exit 1
fi

mkdir -p "$output_dir"

render() {
	local arm=$1
	local threshold=$2
	local seed=$3
	local parent="$parent_dir/B-$seed.json"
	local output="$output_dir/$arm-$seed.json"

	if [[ ! -f "$parent" ]]; then
		echo "missing P1-B parent: $parent" >&2
		exit 1
	fi

	jq \
		--arg arm "$arm" \
		--argjson threshold "$threshold" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-3-p3-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-3-P3" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Five-minute ABC-PERP confirmed passive-quote replenishment screen; P0-C and P1-B fixed." |
		.seed = $seed |
		.record_perp_maker_replenishment_decisions = true |
		.perp_maker_replenish_below_bps = $threshold
		' "$parent" >"$output"

	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .record_perp_maker_replenishment_decisions, .perp_maker_replenish_below_bps)' "$parent") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .record_perp_maker_replenishment_decisions, .perp_maker_replenish_below_bps)' "$output"); then
		echo "P3 config contains an undeclared change: $output" >&2
		exit 1
	fi
}

for seed in 101 103; do
	render A 0 "$seed"
	render B 5000 "$seed"
done

for seed in 101 103; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .perp_maker_replenish_below_bps)' "$output_dir/A-$seed.json") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .perp_maker_replenish_below_bps)' "$output_dir/B-$seed.json"); then
		echo "P3 A/B configs differ outside declared replenishment threshold: seed $seed" >&2
		exit 1
	fi
done
