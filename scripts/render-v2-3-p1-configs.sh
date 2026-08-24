#!/usr/bin/env bash
# Render the immutable V2-3 P1 inventory-size control/treatment cells from
# the completed P0-R1 C passive-refresh parent. The structural gate permits no
# change outside identity labels and the recorded P1 coefficient/evidence flag.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
parent_dir="$root_dir/research/configs/v2-3-p0-r1"
output_dir="$root_dir/research/configs/v2-3-p1"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-3 P1 configs" >&2
	exit 1
fi

mkdir -p "$output_dir"

render() {
	local arm=$1
	local skew_bps=$2
	local seed=$3
	local parent="$parent_dir/C-$seed.json"
	local output="$output_dir/$arm-$seed.json"

	if [[ ! -f "$parent" ]]; then
		echo "missing P0-R1 C parent: $parent" >&2
		exit 1
	fi

	jq \
		--arg arm "$arm" \
		--argjson skew_bps "$skew_bps" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-3-p1-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-3-P1" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Five-minute inventory-asymmetric spot Stoikov displayed-size screen; P0-R1 C passive refresh fixed." |
		.seed = $seed |
		.record_maker_quote_size_decisions = true |
		.spot_stoikov_inventory_size_skew_bps = $skew_bps
		' "$parent" >"$output"

	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .record_maker_quote_size_decisions, .spot_stoikov_inventory_size_skew_bps)' "$parent") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .record_maker_quote_size_decisions, .spot_stoikov_inventory_size_skew_bps)' "$output"); then
		echo "P1 config contains an undeclared change: $output" >&2
		exit 1
	fi
}

for seed in 101 103; do
	render A 0 "$seed"
	render B 5000 "$seed"
done
