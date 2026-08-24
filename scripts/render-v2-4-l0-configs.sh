#!/usr/bin/env bash
# Render immutable V2-4 L0 delivery-liability-hedger cells from the completed
# V2-3 P2-B parent. The only A/B economic delta is submit permission.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
parent_dir="$root_dir/research/configs/v2-3-p2"
output_dir="$root_dir/research/configs/v2-4-l0"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-4 L0 configs" >&2
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
		echo "missing P2-B parent: $parent" >&2
		exit 1
	fi

	jq \
		--arg arm "$arm" \
		--argjson enabled "$enabled" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-4-l0-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-4-L0" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Five-minute CDF/USD finite-capital delivery-liability activation screen; V2-3 P2-B remains fixed." |
		.seed = $seed |
		.record_liability_hedger_decisions = true |
		.market_data_receipt_roles = (
			(.market_data_receipt_roles // []) |
			if index("liability_hedger") then . else . + ["liability_hedger"] end
		) |
		.latency_profiles.liability_hedger = {
			model: "constant",
			delay: 20000000,
			market_data_scale: 2
		} |
		.cdf_liability_hedger = {
			enabled: $enabled,
			symbol: "CDF/USD",
			decision_interval: 2000000000,
			obligation_interval: 10000000000,
			obligation_step_qty: 200000000,
			max_abs_obligation_qty: 2000000000,
			max_request_qty: 100000000
		}
		' "$parent" >"$output"

	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .record_liability_hedger_decisions, .market_data_receipt_roles, .latency_profiles.liability_hedger, .cdf_liability_hedger)' "$parent") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .record_liability_hedger_decisions, .market_data_receipt_roles, .latency_profiles.liability_hedger, .cdf_liability_hedger)' "$output"); then
		echo "L0 config contains an undeclared change: $output" >&2
		exit 1
	fi
}

for seed in 101 103; do
	render A false "$seed"
	render B true "$seed"
done

for seed in 101 103; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.enabled)' "$output_dir/A-$seed.json") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.enabled)' "$output_dir/B-$seed.json"); then
		echo "L0 A/B configs differ outside declared enabled treatment: seed $seed" >&2
		exit 1
	fi
done
