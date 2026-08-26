#!/usr/bin/env bash
# Render the immutable P4b funding screen from the completed P4 family.
# This script only transforms preregistered inputs; it never reads outcomes.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source_dir="$root_dir/research/configs/v2-5-p4"
output_dir="$root_dir/research/configs/v2-5-p4b"
mkdir -p "$output_dir"

render() {
	local arm=$1 seed=$2 cap=$3 source=$4
	local description
	if [[ "$arm" == A ]]; then
		description="P4b conditional funding control: validated independent physical-exposure perpetual flow is present; the sole A/B funding intervention is funding_max_rate_bps=1."
	else
		description="P4b conditional funding treatment: validated independent physical-exposure perpetual flow is present; the sole A/B funding intervention is funding_max_rate_bps=75."
	fi
	jq \
		--arg experiment_id "v2-5-p4b-independent-perp-flow-${arm}-seed-${seed}" \
		--arg description "$description" \
		--argjson seed "$seed" \
		--argjson cap "$cap" \
		'
		 .experiment_id = $experiment_id
		| .hypothesis_id = "V2-5-P4B-INDEPENDENT-PERP-FLOW"
		| .date = "2026-08-26"
		| .status = "preregistered"
		| .description = $description
		| .seed = $seed
		| .funding_max_rate_bps = $cap
		| .record_market_data_receipts = true
		| .record_decision_frontier_vectors = false
		| .record_perp_exposure_hedger_decisions = true
		| .record_term_carry_decisions = true
		| .market_data_receipt_roles = ["term_carry_allocator", "perp_exposure_hedger"]
		| .latency_profiles.perp_exposure_hedger = {
		    model: "constant",
		    delay: 20000000,
		    market_data_scale: 2
		  }
		| .perp_exposure_hedger = {
		    enabled: true,
		    symbol: "ABC-PERP",
		    decision_interval: 2000000000,
		    exposure_interval: 10000000000,
		    exposure_step_qty: 10000000,
		    max_abs_exposure: 100000000,
		    max_request_qty: 10000000,
		    tick_size: 1000000,
		    initial_quote_balance: 20000000000000,
		    initial_margin: 10000000000000
		  }
		' "$source" >"$output_dir/${arm}-${seed}.json"
}

for seed in 401 409; do
	render A "$seed" 1 "$source_dir/A-107.json"
	render B "$seed" 75 "$source_dir/B-107.json"
done
for seed in 419 421 431; do
	render A "$seed" 1 "$source_dir/A-107.json"
	render B "$seed" 75 "$source_dir/B-107.json"
done

printf 'rendered immutable P4b configs in %s\n' "$output_dir"
