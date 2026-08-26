#!/usr/bin/env bash
# Verify the immutable V2-7 P7c two-day risk-horizon family.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-7-p7c"
base="$root_dir/research/configs/v005-stress-perp.json"

declare -A expected=(
	[C-367]=a2b225ed060099ebc87aeec00a95501d6e5296de095897f4c44eb5003d4ef1b9
	[C-371]=7627709f7fceb3fd9f8d9c40e7020752639f316a9eb12f268c79acc4af5e4488
	[T-367]=0a6fc74c0f89e2b0e2a7fffa8d017705021afea7645cb46be0ffaf0cd179651a
	[T-371]=6a3d8c6cb33ab4587eee662a8a7ace5888583e19c02d83e4279a45e4d26cd1b1
)

for cell in C-367 C-371 T-367 T-371; do
	file="$config_dir/$cell.json"
	[[ -s "$file" ]] || { echo "missing P7c config: $cell" >&2; exit 1; }
	actual=$(sha256sum "$file" | awk '{print $1}')
	[[ "$actual" == "${expected[$cell]}" ]] || {
		echo "P7c config hash mismatch: $cell" >&2
		exit 1
	}
	arm=${cell%%-*}
	seed=${cell##*-}
	case "$arm" in
		C) enabled=false ;;
		T) enabled=true ;;
		*) echo "invalid P7c arm: $cell" >&2; exit 1 ;;
	esac
	jq -e --argjson seed "$seed" --argjson enabled "$enabled" '
    .seed == $seed and .status == "preregistered" and
    .hypothesis_id == "V2-7-P7C-DISTRESS" and .log_mode == "full" and
    .cross_asset_spot_graph == false and .checkpoint_interval_seconds == 30 and
    .fixed_distance_maker_symbols == ["ABC/USD", "ABC-PERP"] and
    .imbalance_maker_symbols == ["ABC/USD", "ABC-PERP"] and
    (.noise_target_qty_by_symbol | keys) == ["ABC-PERP"] and
    .strict_population_accounting == true and
    .record_market_data_receipts == true and
    .record_perp_exposure_hedger_decisions == true and
    .market_data_receipt_roles == ["perp_exposure_hedger"] and
    .latency_profiles.perp_exposure_hedger.model == "constant" and
    .latency_profiles.perp_exposure_hedger.delay == 40000000 and
    .perp_exposure_hedger.enabled == $enabled and
    .perp_exposure_hedger.symbol == "ABC-PERP" and
    .perp_exposure_hedger.exposure_mode == "fixed_liability" and
    .perp_exposure_hedger.initial_physical_exposure == -1000000000 and
    .perp_exposure_hedger.decision_interval == 2000000000 and
    .perp_exposure_hedger.exposure_interval == 10000000000 and
    .perp_exposure_hedger.exposure_step_qty == 1 and
    .perp_exposure_hedger.max_abs_exposure == 1000000000 and
    .perp_exposure_hedger.max_request_qty == 250000000 and
    .perp_exposure_hedger.tick_size == 10000 and
    .perp_exposure_hedger.initial_quote_balance == 50000000 and
    .perp_exposure_hedger.initial_margin == 5500000000
  ' "$file" >/dev/null
done

normalise_base() {
	jq -S '
    del(.experiment_id,.hypothesis_id,.description,.date,.status,.seed,
        .cross_asset_spot_graph,.checkpoint_interval_seconds,
        .fixed_distance_maker_symbols,.imbalance_maker_symbols,
        .noise_target_qty_by_symbol,
        .record_market_data_receipts,.market_data_receipt_roles,
        .record_perp_exposure_hedger_decisions,.latency_profiles.perp_exposure_hedger,
        .perp_exposure_hedger)
  '
}

for cell in C-367 C-371 T-367 T-371; do
	if ! diff -u <(normalise_base <"$base") <(normalise_base <"$config_dir/$cell.json") >/dev/null; then
		echo "P7c $cell contains an undeclared environment delta" >&2
		exit 1
	fi
done

for seed in 367 371; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled)' "$config_dir/C-$seed.json") \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled)' "$config_dir/T-$seed.json") >/dev/null; then
		echo "P7c control/treatment pair $seed contains an undeclared delta" >&2
		exit 1
	fi
done

echo "V2-7 P7c configs: immutable environment and two-day risk-horizon delta verified"
