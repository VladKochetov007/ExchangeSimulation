#!/usr/bin/env bash
# Render immutable V2-4 L1-P phase cells from the completed L1-B parent.
# The explicit phase field is the sole behavioral delta.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
parent_dir="$root_dir/research/configs/v2-4-l1"
output_dir="$root_dir/research/configs/v2-4-l1p"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-4 L1-P configs" >&2
	exit 1
fi

mkdir -p "$output_dir"

render() {
	local arm=$1
	local phase_nanos=$2
	local seed=$3
	local parent="$parent_dir/B-$seed.json"
	local output="$output_dir/$arm-$seed.json"

	if [[ ! -f "$parent" ]]; then
		echo "missing L1-B parent: $parent" >&2
		exit 1
	fi

	jq \
		--arg arm "$arm" \
		--argjson phase_nanos "$phase_nanos" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-4-l1p-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-4-L1-P" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Thirty-minute matched CDF/USD liability-hedger phase screen; decision_phase_offset is the sole behavioral delta from V2-4 L1-B." |
		.cdf_liability_hedger.decision_phase_offset = $phase_nanos
		' "$parent" >"$output"

	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .cdf_liability_hedger.decision_phase_offset)' "$parent") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .cdf_liability_hedger.decision_phase_offset)' "$output"); then
		echo "L1-P config contains an undeclared change: $output" >&2
		exit 1
	fi
}

for seed in 101 103; do
	render P0 0 "$seed"
	render P1 1000000000 "$seed"
done

for seed in 101 103; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .cdf_liability_hedger.decision_phase_offset)' "$output_dir/P0-$seed.json") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .cdf_liability_hedger.decision_phase_offset)' "$output_dir/P1-$seed.json"); then
		echo "L1-P P0/P1 configs differ outside declared phase: seed $seed" >&2
		exit 1
	fi
done
