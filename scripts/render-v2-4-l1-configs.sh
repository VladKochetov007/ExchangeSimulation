#!/usr/bin/env bash
# Render immutable V2-4 L1 matched CDF/USD motive-control cells. Both arms use
# the completed L0-B slot; policy_mode is the sole economic delta.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
parent_dir="$root_dir/research/configs/v2-4-l0"
output_dir="$root_dir/research/configs/v2-4-l1"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-4 L1 configs" >&2
	exit 1
fi

mkdir -p "$output_dir"

render() {
	local arm=$1
	local mode=$2
	local seed=$3
	local parent="$parent_dir/B-$seed.json"
	local output="$output_dir/$arm-$seed.json"

	if [[ ! -f "$parent" ]]; then
		echo "missing L0-B parent: $parent" >&2
		exit 1
	fi

	jq \
		--arg arm "$arm" \
		--arg mode "$mode" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-4-l1-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-4-L1" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Thirty-minute matched CDF/USD motive-control screen; policy_mode is the sole economic A/B delta from V2-4 L0-B." |
		.seed = $seed |
		.cdf_liability_hedger.enabled = true |
		.cdf_liability_hedger.policy_mode = $mode
		' "$parent" >"$output"

	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.policy_mode)' "$parent") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.policy_mode)' "$output"); then
		echo "L1 config contains an undeclared change: $output" >&2
		exit 1
	fi
}

for seed in 101 103; do
	render A random_side_control "$seed"
	render B delivery_liability "$seed"
done

for seed in 101 103; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.policy_mode)' "$output_dir/A-$seed.json") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.policy_mode)' "$output_dir/B-$seed.json"); then
		echo "L1 A/B configs differ outside declared policy mode: seed $seed" >&2
		exit 1
	fi
done
