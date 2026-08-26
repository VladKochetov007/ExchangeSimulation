#!/usr/bin/env bash
# Render immutable V2-7 P7d development cells.
# Numeric values are frozen in the accompanying research addendum.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
base="$root_dir/research/configs/v005-stress-perp.json"
output_dir="$root_dir/research/configs/v2-7-p7d"

[[ -s "$base" ]] || { echo "missing P7d base config: $base" >&2; exit 1; }
mkdir -p "$output_dir"

for seed in 431 433; do
	for cell in C L S; do
		enabled=false
	case "$cell" in
	L|S) enabled=true ;;
	esac
	target=0
	case "$cell" in
	L) target=2000000000 ;;
	S) target=-2000000000 ;;
	esac
	output="$output_dir/$cell-$seed.json"
	if [[ -e "$output" ]]; then
		echo "refusing to overwrite immutable P7d config: $output" >&2
		exit 1
	fi
	tmp=$(mktemp "$output.tmp-XXXXXX")
	jq \
		--arg cell "$cell" \
		--argjson seed "$seed" \
		--argjson enabled "$enabled" \
		--argjson target "$target" \
		'
		 .experiment_id = ("v2-7-p7d-directional-distress-" + $cell + "-seed-" + ($seed|tostring))
		 | .hypothesis_id = "V2-7-P7D-DIRECTIONAL-DISTRESS"
		 | .date = "2026-08-26"
		 | .status = "preregistered"
		 | .description = ("P7d finite-capital fixed-directional distress screen " + $cell + "; only declared enablement/target differs across cells")
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
		     exposure_mode: "fixed_directional",
		     initial_target_perp_position: $target,
		     auto_borrow_perp: true,
		     decision_interval: 2000000000,
		     exposure_interval: 10000000000,
		     exposure_step_qty: 1,
		     max_abs_exposure: 2000000000,
		     max_request_qty: 500000000,
		     tick_size: 10000,
		     initial_quote_balance: 50000000,
		     initial_margin: 5500000000
		   }
		' "$base" >"$tmp"
	mv "$tmp" "$output"
	done
done

echo "rendered V2-7 P7d configs in $output_dir"
