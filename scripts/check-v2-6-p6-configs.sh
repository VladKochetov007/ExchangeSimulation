#!/usr/bin/env bash
# Verify the immutable V2-6 staged-options config family.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-6-p6"
base="$root_dir/research/configs/v2-4-l1p3/A-107.json"

declare -A expected=(
  [O0-211]=f8421827eb23314a9988678443f9da2da3dcb6dabf2c98121f562804389559b8
  [O0-213]=d55dc184495dbadea930be0f269f5a5ca931fa0660e450e193422fb543294ccb
  [O1-211]=7e519ee2a55010d5189bc4234d3e6385e3bcb1398a9dd02ff6950a3fbab79106
  [O1-213]=5036d62929e5fd76bf920ee371826d73c2506c8a65c2329c181947637becad80
  [O2-211]=d859f1c9c6ab319175ae3ee008285f97becd2e77e08f203cc863e947860b1b7d
  [O2-213]=ee1e9ef750392ff0328200e9c9b75940e5dc8df2798fd57fd05bedd2722f0f24
  [O3-211]=fed98f1fa982354e43604acaf761b3577cc00e96cc53c6070ae086526ea9753c
  [O3-213]=0082c7274e6b51fc44d677230bcfd73154bac454ec1babfb22932fcc5fdba7cc
  [O4-211]=bde2697027e108619b5881ea6446388c62446875eebf8ff5b6ac188c5287804a
  [O4-213]=399dbaef87c888c856b42359b3b62d579163568c08fa2a0388db4435edd41e01
)

for cell in "${!expected[@]}"; do
	file="$config_dir/$cell.json"
	[[ -s "$file" ]] || { echo "missing P6 config: $cell" >&2; exit 1; }
	actual=$(sha256sum "$file" | awk '{print $1}')
	[[ "$actual" == "${expected[$cell]}" ]] || { echo "P6 config hash mismatch: $cell" >&2; exit 1; }
	stage=${cell%%-*}
	seed=${cell##*-}
	jq -e --arg stage "$stage" --argjson seed "$seed" '
    .seed == $seed and .status == "preregistered" and
    .hypothesis_id == "V2-6-P6-OPTIONS" and .log_mode == "full" and
    .latency_profiles.option_liability_user.model == "constant" and
    .latency_profiles.option_liability_user.delay == 20000000 and
    .latency_profiles.option_liability_user.market_data_scale == 2 and
    (if $stage == "O0" then
       .option_dealer_count == 1 and .option_dealer_vol.model == "flat" and
       .dealer_hedge_mode == "off" and .option_dealer_hedge_policies == ["none"] and
       .option_value_taker_count == 0 and .vanna_volga_desk_count == 0 and
       .option_liability_user == null and .record_option_liability_user_decisions == false
     elif $stage == "O1" then
       .option_dealer_count == 3 and .option_dealer_vol.model == "realized" and
       .option_dealer_vol.half_life_seconds == [300,1800,7200] and
       .dealer_hedge_mode == "off" and .option_dealer_hedge_policies == ["none"] and
       .option_value_taker_count == 0 and .vanna_volga_desk_count == 0 and
       .option_liability_user == null and .record_option_liability_user_decisions == false
     elif $stage == "O2" then
       .option_dealer_count == 3 and .option_dealer_vol.model == "realized" and
       .dealer_hedge_mode == "on" and .option_dealer_hedge_policies == ["banded","static","timed"] and
       .option_value_taker_count == 0 and .vanna_volga_desk_count == 0 and
       .record_option_liability_user_decisions == true and .option_liability_user.target_qty == 100000000 and
       .option_liability_user.lot_qty == 10000000 and .option_liability_user.target_strike_bps == 9500 and
       .option_liability_user.max_premium == 10000000000 and .option_liability_user.interval == 5000000000
     elif $stage == "O3" then
       .option_dealer_count == 3 and .dealer_hedge_mode == "on" and
       .option_value_taker_count == 1 and .option_value_taker_vol.model == "sabr" and
       .vanna_volga_desk_count == 0 and .record_option_liability_user_decisions == true and
       .option_liability_user != null
     else
       .option_dealer_count == 3 and .dealer_hedge_mode == "on" and
       .option_value_taker_count == 1 and .option_value_taker_vol.model == "sabr" and
       .vanna_volga_desk_count == 1 and .record_option_liability_user_decisions == true and
       .option_liability_user != null
     end) and
    ((.market_data_receipt_roles | index("option_liability_user")) != null) == (.option_liability_user != null)
  ' "$file" >/dev/null
done

normalise() {
	jq -S '
    del(.experiment_id,.hypothesis_id,.description,.date,.status,.seed,
        .option_dealer_count,.option_dealer_vol,.dealer_hedge_mode,
        .option_dealer_hedge_policies,.option_value_taker_count,
        .option_value_taker_vol,.vanna_volga_desk_count,
        .vanna_volga_vega_tolerance,.vanna_volga_vanna_tolerance,
        .vanna_volga_volga_tolerance,.vanna_volga_lot_qty,
        .vanna_volga_max_contracts,.vanna_volga_interval,.vanna_volga_vol,
        .record_option_liability_user_decisions,.option_liability_user,
        .latency_profiles.option_liability_user)
    | .market_data_receipt_roles |= (map(select(. != "option_liability_user")) | sort)
  '
}

for stage in O0 O1 O2 O3 O4; do
	for seed in 211 213; do
		if ! diff -u <(normalise <"$base") <(normalise <"$config_dir/$stage-$seed.json") >/dev/null; then
			echo "P6 $stage-$seed contains an undeclared non-option delta" >&2
			exit 1
		fi
	done
done

echo "V2-6 P6 configs: staged option deltas and inherited environment verified"
