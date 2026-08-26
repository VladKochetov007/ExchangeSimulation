#!/usr/bin/env bash
# Verify the immutable V2-6 P6-R1 explicit cross-asset mark configs.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-6-p6r1"
source_dir="$root_dir/research/configs/v2-6-p6"

declare -A expected=(
  [O0-211]=5948f6f009df73a3230e646b5804c38beae8e6892a79f98d41a957ffd9d23b6c
  [O0-213]=229707d82180e53f348814164bfa2aa0078293f8362ebd4e4a656d75cd0f4e68
  [O1-211]=848326866157f8067c26052bbb035e2119d5b433fbe692cb341f7776f477784f
  [O1-213]=c58f338eb196c878d48bf7d7efc2b0f12e36ae0a2c0dc60ad935d76bea26ba52
  [O2-211]=837141bb7a665d87a7681450b790ad3231bbfdce6198867bde6a7d528f271a34
  [O2-213]=a195cf64c10467d47dac0a708d01a1e268eee02f0dde4b0355753c67c032b135
  [O3-211]=2ed17e3e68d424a667ffe075e9c8a464f73eaa0a5d1ec113c9db4d189f07384a
  [O3-213]=8d39677d0e28eae0d132e6a7376f796c88f873ccf74d8aa848f3fbed232c9508
  [O4-211]=b89e6a52d107c4aa9d395786a4ea72a74a48f5746b1b6d13934ac144507af557
  [O4-213]=e13281501ddd7f28af4e2fc8602354cded844c2b883f07b52ddfc11c75d8fc12
)

normalise() {
	jq -S '
    del(.experiment_id,.hypothesis_id,.description,.date,.status,.seed,
        .cross_asset_collateral_marks)
  '
}

for cell in "${!expected[@]}"; do
	file="$config_dir/$cell.json"
	source="$source_dir/$cell.json"
	[[ -s "$file" && -s "$source" ]] || { echo "missing P6-R1 config: $cell" >&2; exit 1; }
	actual=$(sha256sum "$file" | awk '{print $1}')
	[[ "$actual" == "${expected[$cell]}" ]] || { echo "P6-R1 config hash mismatch: $cell" >&2; exit 1; }
	stage=${cell%%-*}
	seed=${cell##*-}
	jq -e --arg stage "$stage" --argjson seed "$seed" '
    .seed == $seed and .status == "preregistered" and
    .hypothesis_id == "V2-6-P6R1-CROSS-ASSET-MARK" and
    .cross_asset_collateral_marks == true and .log_mode == "full" and
    (.description | contains("no other stage or ecology delta")) and
    (if $stage == "O0" then
       .option_dealer_count == 1 and .option_dealer_vol.model == "flat" and
       .dealer_hedge_mode == "off" and .option_value_taker_count == 0 and
       .vanna_volga_desk_count == 0 and .option_liability_user == null
     elif $stage == "O1" then
       .option_dealer_count == 3 and .option_dealer_vol.model == "realized" and
       .dealer_hedge_mode == "off" and .option_value_taker_count == 0 and
       .vanna_volga_desk_count == 0 and .option_liability_user == null
     elif $stage == "O2" then
       .option_dealer_count == 3 and .dealer_hedge_mode == "on" and
       .option_value_taker_count == 0 and .vanna_volga_desk_count == 0 and
       .option_liability_user != null
     elif $stage == "O3" then
       .option_dealer_count == 3 and .dealer_hedge_mode == "on" and
       .option_value_taker_count == 1 and .option_value_taker_vol.model == "sabr" and
       .vanna_volga_desk_count == 0 and .option_liability_user != null
     else
       .option_dealer_count == 3 and .dealer_hedge_mode == "on" and
       .option_value_taker_count == 1 and .option_value_taker_vol.model == "sabr" and
       .vanna_volga_desk_count == 1 and .option_liability_user != null
     end)
  ' "$file" >/dev/null
	if ! diff -u <(normalise <"$source") <(normalise <"$file") >/dev/null; then
		echo "P6-R1 $cell contains an undeclared delta" >&2
		exit 1
	fi
done

echo "V2-6 P6-R1 configs: explicit collateral mark delta verified"
