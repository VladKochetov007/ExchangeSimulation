#!/usr/bin/env bash
# Render immutable V2-4 L1-P3 holdout cells from the completed L1-P2 parent.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
parent="$root_dir/research/configs/v2-4-l1p2/A-101.json"
output_dir="$root_dir/research/configs/v2-4-l1p3"

if ! command -v jq >/dev/null; then
	echo "jq is required to render V2-4 L1-P3 configs" >&2
	exit 1
fi
if [[ ! -f "$parent" ]]; then
	echo "missing L1-P2 parent: $parent" >&2
	exit 1
fi
mkdir -p "$output_dir"

render() {
	local arm=$1
	local liability_phase_nanos=$2
	local noise_phase_nanos=$3
	local seed=$4
	local output="$output_dir/$arm-$seed.json"

	jq \
		--arg arm "$arm" \
		--argjson liability_phase_nanos "$liability_phase_nanos" \
		--argjson noise_phase_nanos "$noise_phase_nanos" \
		--argjson seed "$seed" \
		'
		.experiment_id = ("v2-4-l1p3-" + $arm + "-seed-" + ($seed|tostring)) |
		.hypothesis_id = "V2-4-L1-P3" |
		.date = "2026-08-24" |
		.status = "preregistered" |
		.description = "Thirty-minute untouched-seed replication of V2-4 L1-P2; seed and declared liability/noise-flow first-tick phases are the only behavioral deltas from the L1-P2 parent." |
		.seed = $seed |
		.cdf_liability_hedger.decision_phase_offset = $liability_phase_nanos |
		.noise_flow_decision_phase_offset = $noise_phase_nanos
		' "$parent" >"$output"

	if ! diff -u \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.decision_phase_offset, .noise_flow_decision_phase_offset)' "$parent") \
		<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.decision_phase_offset, .noise_flow_decision_phase_offset)' "$output"); then
		echo "L1-P3 config contains an undeclared change: $output" >&2
		exit 1
	fi
	jq -e --argjson liability_phase "$liability_phase_nanos" --argjson noise_phase "$noise_phase_nanos" --argjson seed "$seed" '
		.seed == $seed and .log_mode == "full" and .record_liability_hedger_decisions == true and
		.record_market_data_receipts == true and .record_noise_flow_phase_decisions == true and
		(.market_data_receipt_roles | index("liability_hedger") != null) and
		.noise_interval == 2000000000 and .cdf_liability_hedger.decision_interval == 2000000000 and
		.cdf_liability_hedger.decision_phase_offset == $liability_phase and
		.noise_flow_decision_phase_offset == $noise_phase
	' "$output" >/dev/null
}

for seed in 107 109 113; do
	render A 0 0 "$seed"
	render B 1000000000 0 "$seed"
	render C 0 1000000000 "$seed"
	render D 1000000000 1000000000 "$seed"
done

for seed in 107 109 113; do
	for arm in B C D; do
		if ! diff -u \
			<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.decision_phase_offset, .noise_flow_decision_phase_offset)' "$output_dir/A-$seed.json") \
			<(jq -S 'del(.experiment_id, .hypothesis_id, .date, .status, .description, .seed, .cdf_liability_hedger.decision_phase_offset, .noise_flow_decision_phase_offset)' "$output_dir/$arm-$seed.json"); then
			echo "L1-P3 arms differ outside declared fields: seed $seed A/$arm" >&2
			exit 1
		fi
	done
done
