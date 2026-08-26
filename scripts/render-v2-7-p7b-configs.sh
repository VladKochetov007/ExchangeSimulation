#!/usr/bin/env bash
# Render the immutable V2-7 P7b unit-corrected leverage cells.
# Numeric values are frozen in research/v2-7-p7b-numeric-addendum.md.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base="$root_dir/research/configs/v005-stress-perp.json"
output_dir="$root_dir/research/configs/v2-7-p7b"

[[ -s "$base" ]] || { echo "missing P7b base config: $base" >&2; exit 1; }
mkdir -p "$output_dir"

for seed in 337 341; do
	for cell in C L H; do
		enabled=false
		margin=10000000000
		case "$cell" in
			L) enabled=true ;;
			H) enabled=true; margin=5500000000 ;;
		esac
		output="$output_dir/$cell-$seed.json"
		if [[ -e "$output" ]]; then
			echo "refusing to overwrite immutable P7b config: $output" >&2
			exit 1
		fi
		tmp=$(mktemp "$output.tmp-XXXXXX")
		jq \
			--arg cell "$cell" \
			--argjson seed "$seed" \
			--argjson enabled "$enabled" \
			--argjson margin "$margin" \
			'
			 .experiment_id = ("v2-7-p7b-distress-" + $cell + "-seed-" + ($seed|tostring))
			 | .hypothesis_id = "V2-7-P7B-DISTRESS"
			 | .date = "2026-08-26"
			 | .status = "preregistered"
			 | .description = ("P7b unit-corrected fixed-liability distress screen " + $cell + "; only declared participant enablement and finite perp margin differ across cells")
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
			     initial_margin: $margin
			   }
			' "$base" >"$tmp"
		mv "$tmp" "$output"
	done
done

echo "rendered V2-7 P7b configs in $output_dir"
