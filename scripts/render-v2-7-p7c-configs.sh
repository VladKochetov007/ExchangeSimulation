#!/usr/bin/env bash
# Render the immutable V2-7 P7c two-day risk-horizon cells.
# Numeric values are frozen in research/v2-7-p7c-numeric-addendum.md.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base="$root_dir/research/configs/v005-stress-perp.json"
output_dir="$root_dir/research/configs/v2-7-p7c"

[[ -s "$base" ]] || { echo "missing P7c base config: $base" >&2; exit 1; }
mkdir -p "$output_dir"

for seed in 367 371; do
	for cell in C T; do
		enabled=false
	if [[ "$cell" == T ]]; then enabled=true; fi
		output="$output_dir/$cell-$seed.json"
		if [[ -e "$output" ]]; then
			echo "refusing to overwrite immutable P7c config: $output" >&2
			exit 1
		fi
		tmp=$(mktemp "$output.tmp-XXXXXX")
		jq \
			--arg cell "$cell" \
			--argjson seed "$seed" \
			--argjson enabled "$enabled" \
			'
			 .experiment_id = ("v2-7-p7c-distress-" + $cell + "-seed-" + ($seed|tostring))
			 | .hypothesis_id = "V2-7-P7C-DISTRESS"
			 | .date = "2026-08-26"
			 | .status = "preregistered"
			 | .description = ("P7c two-day fixed-liability risk-horizon screen " + $cell + "; only declared participant enablement differs across cells")
			 | .seed = $seed
			 | .cross_asset_spot_graph = false
			 | .fixed_distance_maker_symbols = ["ABC/USD", "ABC-PERP"]
			 | .imbalance_maker_symbols = ["ABC/USD", "ABC-PERP"]
			 | .noise_target_qty_by_symbol = {"ABC-PERP": .noise_target_qty_by_symbol["ABC-PERP"]}
			 | .checkpoint_interval_seconds = 30
			 | .record_market_data_receipts = true
			 | .market_data_receipt_roles = ["perp_exposure_hedger"]
			 | .record_perp_exposure_hedger_decisions = true
			 | .latency_profiles = (.latency_profiles + {
			     perp_exposure_hedger: {model: "constant", delay: 40000000}
			   })
			 | .perp_exposure_hedger = {
			     enabled: $enabled,
			     symbol: "ABC-PERP",
			     exposure_mode: "fixed_liability",
			     initial_physical_exposure: -1000000000,
			     decision_interval: 2000000000,
			     exposure_interval: 10000000000,
			     exposure_step_qty: 1,
			     max_abs_exposure: 1000000000,
			     max_request_qty: 250000000,
			     tick_size: 10000,
			     initial_quote_balance: 50000000,
			     initial_margin: 5500000000
			   }
			' "$base" >"$tmp"
		mv "$tmp" "$output"
	done
done

echo "rendered V2-7 P7c configs in $output_dir"
