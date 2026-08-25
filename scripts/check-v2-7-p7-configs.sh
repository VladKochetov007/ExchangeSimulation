#!/usr/bin/env bash
# Verify the immutable V2-7 P7a development config family.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-7-p7"
base="$root_dir/research/configs/v005-stress-perp.json"

declare -A expected=(
	[C-307]=25ae4e23510e2fbbdf539c5af0c1c419e1790cea87efa20e904c2a85bbef3173
	[C-311]=e6a6167d1d37b12de41414eb2b2dcef9722419330c13be9b7376651c074317e2
	[H-307]=8b3a3fdde285fa7e4891df1393af2310322f48aa0189e48c1a8cde28fdb1f4a5
	[H-311]=f737817490fdf5561e2d5ef24a35fea7337b81b77f54759f13bee2d532ad0e14
	[L-307]=5280b20934d750a4a1a966ef4db774d71718f0c174447df75e99b61dc948ed49
	[L-311]=28e08418fc5914a6506736772a4aae830b79866f944da95b3f1b2dcd684ac740
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
    .perp_exposure_hedger.tick_size == 1000000 and
    .perp_exposure_hedger.initial_quote_balance == 50000000 and
    .perp_exposure_hedger.initial_margin == $margin
  ' "$file" >/dev/null
done

normalise_base() {
	jq -S '
    del(.experiment_id,.hypothesis_id,.description,.date,.status,.seed,
        .cross_asset_spot_graph,.checkpoint_interval_seconds,
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
