#!/usr/bin/env bash
# Render immutable V2-4 L1-P2 factorial cells from the completed L1-B parent.
# The declared liability/noise phases and evidence-only noise timing recorder
# are the only additions to that parent.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
parent_dir="$root_dir/research/configs/v2-4-l1"
output_dir="$root_dir/research/configs/v2-4-l1p2"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-4 L1-P2 configs" >&2
	exit 1
fi

mkdir -p "$output_dir"

render() {
	local arm=$1
	local liability_phase_nanos=$2
	local noise_phase_nanos=$3
	local seed=$4
	local parent="$parent_dir/B-$seed.json"
	local output="$output_dir/$arm-$seed.json"

	if [[ ! -f "$parent" ]]; then
		echo "missing L1-B parent: $parent" >&2
		exit 1
	fi

	jq \
		--arg arm "$arm" \
		--argjson liability_phase_nanos "$liability_phase_nanos" \
		--argjson noise_phase_nanos "$noise_phase_nanos" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-4-l1p2-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-4-L1-P2" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Thirty-minute matched CDF/USD liability/noise-flow phase-decomposition screen; declared liability and broad noise-flow first-tick phases are the only behavioral deltas from V2-4 L1-B." |
		.cdf_liability_hedger.decision_phase_offset = $liability_phase_nanos |
		.noise_flow_decision_phase_offset = $noise_phase_nanos |
		.record_noise_flow_phase_decisions = true
		' "$parent" >"$output"

	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .cdf_liability_hedger.decision_phase_offset, .noise_flow_decision_phase_offset, .record_noise_flow_phase_decisions)' "$parent") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .cdf_liability_hedger.decision_phase_offset, .noise_flow_decision_phase_offset, .record_noise_flow_phase_decisions)' "$output"); then
		echo "L1-P2 config contains an undeclared change: $output" >&2
		exit 1
	fi
	jq -e --argjson liability_phase "$liability_phase_nanos" --argjson noise_phase "$noise_phase_nanos" '
		.log_mode == "full" and .record_liability_hedger_decisions == true and
		.record_market_data_receipts == true and .record_noise_flow_phase_decisions == true and
		(.market_data_receipt_roles | index("liability_hedger") != null) and
		.noise_interval == 2000000000 and
		.cdf_liability_hedger.decision_interval == 2000000000 and
		.cdf_liability_hedger.decision_phase_offset == $liability_phase and
		.noise_flow_decision_phase_offset == $noise_phase
	' "$output" >/dev/null
}

for seed in 101 103; do
	render A 0 0 "$seed"
	render B 1000000000 0 "$seed"
	render C 0 1000000000 "$seed"
	render D 1000000000 1000000000 "$seed"
done

for seed in 101 103; do
	for arm in B C D; do
		if ! diff -u \
			<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .cdf_liability_hedger.decision_phase_offset, .noise_flow_decision_phase_offset, .record_noise_flow_phase_decisions)' "$output_dir/A-$seed.json") \
			<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .cdf_liability_hedger.decision_phase_offset, .noise_flow_decision_phase_offset, .record_noise_flow_phase_decisions)' "$output_dir/$arm-$seed.json"); then
			echo "L1-P2 arms differ outside declared fields: seed $seed A/$arm" >&2
			exit 1
		fi
	done
done
