#!/usr/bin/env bash
# Verify the immutable V2-7 P7b unit-corrected leverage family.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-7-p7b"
base="$root_dir/research/configs/v005-stress-perp.json"

declare -A expected=(
	[C-337]=d9bd9f0928d80d3cb289037110f5c22a1af2ed6de1e417ece9b94e16f78539bf
	[C-341]=c18a9df7d70b36964dae40d8bccb9fcb64b14e38c8f072384af2a7fea8be36b7
	[H-337]=eee3960cea6f62d38125bbf2c151b18242ea9d15825109bb5e75c5df6ce4749b
	[H-341]=7f4a44a981b5ec13938856cc87dc9458a04ea3038984af4f1afeca74f35f6277
	[L-337]=eadae97f8363e25d73584d9699194f2e1d7673f01f0888071b04211a8f94d468
	[L-341]=c5c037dc5bc270dfeb40cd38ddbbc847aabf69c25069ff6a4d016b705f6f7c39
)

for cell in C-337 C-341 L-337 L-341 H-337 H-341; do
	file="$config_dir/$cell.json"
	[[ -s "$file" ]] || { echo "missing P7b config: $cell" >&2; exit 1; }
	actual=$(sha256sum "$file" | awk '{print $1}')
	[[ "$actual" == "${expected[$cell]}" ]] || { echo "P7b config hash mismatch: $cell" >&2; exit 1; }
	arm=${cell%%-*}
	seed=${cell##*-}
	case "$arm" in
		C) enabled=false; margin=10000000000 ;;
		L) enabled=true; margin=10000000000 ;;
		H) enabled=true; margin=5500000000 ;;
		*) echo "invalid P7b arm: $cell" >&2; exit 1 ;;
	esac
	jq -e --argjson seed "$seed" --argjson enabled "$enabled" --argjson margin "$margin" '
    .seed == $seed and .status == "preregistered" and
    .hypothesis_id == "V2-7-P7B-DISTRESS" and .log_mode == "full" and
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
    .perp_exposure_hedger.initial_margin == $margin
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

for cell in C-337 C-341 L-337 L-341 H-337 H-341; do
	if ! diff -u <(normalise_base <"$base") <(normalise_base <"$config_dir/$cell.json") >/dev/null; then
		echo "P7b $cell contains an undeclared environment delta" >&2
		exit 1
	fi
done

for seed in 337 341; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_margin)' "$config_dir/C-$seed.json") \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_margin)' "$config_dir/L-$seed.json") >/dev/null; then
		echo "P7b control/active pair $seed contains an undeclared delta" >&2
		exit 1
	fi
	if ! diff -u \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_margin)' "$config_dir/L-$seed.json") \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_margin)' "$config_dir/H-$seed.json") >/dev/null; then
		echo "P7b active levels $seed contain an undeclared delta" >&2
		exit 1
	fi
done

echo "V2-7 P7b configs: immutable environment and unit-corrected capital ladder verified"
