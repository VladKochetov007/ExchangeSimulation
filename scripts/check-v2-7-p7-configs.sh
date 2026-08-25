#!/usr/bin/env bash
# Verify the immutable V2-7 P7a development config family.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-7-p7"
base="$root_dir/research/configs/v005-stress-perp.json"

declare -A expected=(
	[C-307]=633750a02818b8204e174e81126ec2e506ec75b1d6cbba48d22f1feba60aca82
	[C-311]=e604150c2d23528fa9e684311ca6a141e3a4ddbe996453252a0241c37a9d5c85
	[H-307]=b2606c9757a7d8106d1dc1a94c52c23d0a204c60f184aa93b4ef2e55f7e7fbdc
	[H-311]=0f97b833c03df7901c6b990b56fa6547a2e1652c310db146fa42a945adfedda1
	[L-307]=45083d2bb6527a3c61b53a638c95afe42815c0e9186c0c0dfc68180f28d9fd79
	[L-311]=c1ef3301735b059ddca8a07af30f69244af7828d769b0375a3bc67c5f5b5fdd8
)

for cell in C-307 C-311 L-307 L-311 H-307 H-311; do
	file="$config_dir/$cell.json"
	[[ -s "$file" ]] || { echo "missing P7 config: $cell" >&2; exit 1; }
	actual=$(sha256sum "$file" | awk '{print $1}')
	[[ "$actual" == "${expected[$cell]}" ]] || { echo "P7 config hash mismatch: $cell" >&2; exit 1; }
	arm=${cell%%-*}
	seed=${cell##*-}
	case "$arm" in
		C) enabled=false; margin=120000000000 ;;
		L) enabled=true; margin=120000000000 ;;
		H) enabled=true; margin=60000000000 ;;
		*) echo "invalid P7 arm: $cell" >&2; exit 1 ;;
	esac
	jq -e --argjson seed "$seed" --argjson enabled "$enabled" --argjson margin "$margin" '
    .seed == $seed and .status == "preregistered" and
    .hypothesis_id == "V2-7-P7A-DISTRESS" and .log_mode == "full" and
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

for cell in C-307 C-311 L-307 L-311 H-307 H-311; do
	if ! diff -u <(normalise_base <"$base") <(normalise_base <"$config_dir/$cell.json") >/dev/null; then
		echo "P7 $cell contains an undeclared environment delta" >&2
		exit 1
	fi
done

for seed in 307 311; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_margin)' "$config_dir/C-$seed.json") \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_margin)' "$config_dir/L-$seed.json") >/dev/null; then
		echo "P7 control/active pair $seed contains an undeclared delta" >&2
		exit 1
	fi
	if ! diff -u \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_margin)' "$config_dir/L-$seed.json") \
		<(jq -S 'del(.experiment_id,.description,.seed,.perp_exposure_hedger.enabled,.perp_exposure_hedger.initial_margin)' "$config_dir/H-$seed.json") >/dev/null; then
		echo "P7 active levels $seed contain an undeclared delta" >&2
		exit 1
	fi
done

echo "V2-7 P7a configs: immutable environment and capital ladder verified"
