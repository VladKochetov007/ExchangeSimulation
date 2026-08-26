#!/usr/bin/env bash
# Verify the immutable, preregistered P7d holdout configurations.
# This is read-only and requires the development configuration contract first.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-7-p7d"
base="$root_dir/research/configs/v005-stress-perp.json"

"$root_dir/scripts/check-v2-7-p7d-configs.sh" >/dev/null

declare -A expected=(
  [C-439]=0ff72d0aae2db4c1bfb22d59742df8be3aa58af01a6268f1a6bb418fdd14b21b
  [C-443]=0709d24ebce257cd0dda441cda4c0745986a51758e6cd655223b6589f7819cea
  [C-449]=93cdebfde9e1949637cc61896e889dddf9efef81d42d05ad0d4dc975b3ccc4a5
  [L-439]=1089fe48fbf3745d2b39ba939c2531f7ba7d3a2da15ff1a97b271f0e7d66ec5a
  [L-443]=d58274248399e00cadc73d6c815170e2e0503a7dd3b3752068c80107d3839d83
  [L-449]=6bb033b6b71a1c4e8d5e1302a7069b748d272af25d01ad217fbbb736a573d9d6
  [S-439]=9a2b6dbdd9b1d65525276367d243f221a3ffb18db2f73dca8fe07e016a564c9c
  [S-443]=00660dd63a588145b164e3affa7ca32336638e943517b8de6eb1adf93c99fb45
  [S-449]=ccba09c2345c04930f3914e27d4e05a8d1a1d6bfb6ddf036f1763b60ef90fe26
)

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

for id in C-439 C-443 C-449 L-439 L-443 L-449 S-439 S-443 S-449; do
  file="$config_dir/$id.json"
  [[ -s "$file" ]] || { echo "missing P7d holdout config: $id" >&2; exit 1; }
  actual=$(sha256sum "$file" | awk '{print $1}')
  [[ "$actual" == "${expected[$id]}" ]] || {
    echo "P7d holdout config hash mismatch: $id" >&2
    exit 1
  }
  cell=${id%%-*}; seed=${id##*-}
  case "$cell" in
    C) enabled=false; target=0 ;;
    L) enabled=true; target=2000000000 ;;
    S) enabled=true; target=-2000000000 ;;
    *) echo "invalid P7d holdout cell: $id" >&2; exit 1 ;;
  esac
  jq -e --argjson seed "$seed" --argjson enabled "$enabled" --argjson target "$target" --arg cell "$cell" '
    .seed == $seed and .status == "preregistered" and
    .hypothesis_id == "V2-7-P7D-DIRECTIONAL-DISTRESS" and
    (.experiment_id == ("v2-7-p7d-directional-distress-" + $cell + "-seed-" + ($seed|tostring))) and
    (.description | contains("untouched holdout")) and .log_mode == "full" and
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
  if ! diff -u <(normalise_base <"$base") <(normalise_base <"$file") >/dev/null; then
    echo "P7d holdout $id contains an undeclared environment delta" >&2
    exit 1
  fi
  dev_source="$config_dir/$cell-431.json"
  if ! diff -u \
    <(jq -S 'del(.experiment_id,.description,.seed)' "$dev_source") \
    <(jq -S 'del(.experiment_id,.description,.seed)' "$file") >/dev/null; then
    echo "P7d holdout $id differs from its development policy beyond seed/provenance" >&2
    exit 1
  fi
done

for cell in C L S; do
  for seed in 439 443 449; do
    jq -e --argjson seed "$seed" --arg cell "$cell" \
      '.seed == $seed and (.experiment_id | endswith(($cell + "-seed-" + ($seed|tostring))))' \
      "$config_dir/$cell-$seed.json" >/dev/null
  done
done

echo "V2-7 P7d holdout configs: reserved seeds and policy deltas verified"
