#!/usr/bin/env bash
# Verify immutable V2-7 P7d development cells.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-7-p7d"
base="$root_dir/research/configs/v005-stress-perp.json"

declare -A expected=(
  [C-431]=5bdf0bb6c12353afe3ca0ad263e03318e7b4af89b65210d9d39ee89e0e0a8fe4
  [C-433]=a76c031440643acad94544a181e1367f3ac0513dc5c5a158938c380ce80497f8
  [L-431]=de58790c8afdd20da25d5b9cea467389998cde753aea475874a69eae1dd24e66
  [L-433]=f74dea1ede816890a7fa57dcae4ed7e2dd2eaf7bd7304f64a0aa2937eaf32108
  [S-431]=3cf7b81ab90d6b8cbd5ca46e5cf2f88c0283f944f8d4b41a0af5311458a00ec9
  [S-433]=be933e99848605e6f99dabe5593c3be84f921f86a0249fc047d4c5a946f8db17
)

for id in C-431 C-433 L-431 L-433 S-431 S-433; do
  file="$config_dir/$id.json"
  [[ -s "$file" ]] || { echo "missing P7d config: $id" >&2; exit 1; }
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "${expected[$id]}" ]] || { echo "P7d config hash mismatch: $id" >&2; exit 1; }
  cell=${id%%-*}; seed=${id##*-}
  case "$cell" in
    C) enabled=false; target=0 ;;
    L) enabled=true; target=2000000000 ;;
    S) enabled=true; target=-2000000000 ;;
    *) echo "invalid P7d cell: $id" >&2; exit 1 ;;
  esac
  jq -e --argjson seed "$seed" --argjson enabled "$enabled" --argjson target "$target" '
    .seed == $seed and .status == "preregistered" and
    .hypothesis_id == "V2-7-P7D-DIRECTIONAL-DISTRESS" and .log_mode == "full" and
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
    .perp_exposure_hedger.exposure_mode == "fixed_directional" and
    .perp_exposure_hedger.initial_target_perp_position == $target and
    .perp_exposure_hedger.auto_borrow_perp == true and
    .perp_exposure_hedger.initial_physical_exposure == null and
    .perp_exposure_hedger.decision_interval == 2000000000 and
    .perp_exposure_hedger.exposure_interval == 10000000000 and
    .perp_exposure_hedger.exposure_step_qty == 1 and
    .perp_exposure_hedger.max_abs_exposure == 2000000000 and
    .perp_exposure_hedger.max_request_qty == 500000000 and
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

for id in C-431 C-433 L-431 L-433 S-431 S-433; do
  if ! diff -u <(normalise_base <"$base") <(normalise_base <"$config_dir/$id.json") >/dev/null; then
    echo "P7d $id contains an undeclared environment delta" >&2
    exit 1
  fi
done

for seed in 431 433; do
  if ! diff -u \
    <(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_target_perp_position)' "$config_dir/C-$seed.json") \
    <(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_target_perp_position)' "$config_dir/L-$seed.json") >/dev/null; then
    echo "P7d control/long pair $seed contains an undeclared delta" >&2
    exit 1
  fi
  if ! diff -u \
    <(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_target_perp_position)' "$config_dir/L-$seed.json") \
    <(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_target_perp_position)' "$config_dir/S-$seed.json") >/dev/null; then
    echo "P7d long/short pair $seed contains an undeclared delta" >&2
    exit 1
  fi
done

echo "V2-7 P7d configs: immutable environment and directional deltas verified"
