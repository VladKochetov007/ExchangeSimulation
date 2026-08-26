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
  [O0-223]=c03f6f67e1778c709793a504229a046bd248d18cfaf493c68ac3e93eab01c637
  [O0-227]=6260f57a237a95f94824ed7a45896b621524fcb3a3810e35f13a8b67bbea7e49
  [O0-229]=db68eb8d2535b4e7a37487f110c8bfbdc131e340d4d0930f0295d364be2fb0f3
  [O1-223]=08c12ceab666775f3a4dcfa6a8afd2ccc2977e94d431dbe0d80c3d04c2fc98ae
  [O1-227]=75f958fdd629f96d629341f71509451d4e44a196e806414a16935ed72ac6d8ed
  [O1-229]=35b4fe5d79265361674e49f565c9277eee1969d03d4ae5ab9ddc4e9db3f89a74
  [O2-223]=d4863b18791ecb45cd9caeeee979cb07e0b14c6624c8ea4cf77b4e0e70cb6de2
  [O2-227]=b78abf1206f4cebd3f62b62b48d0538fb311aab96e1a2d1bd5b905b65b0fca62
  [O2-229]=44970b418e7044a71b5b25e81fefc8f52864a8665bcc547440026092cdf8e463
  [O3-223]=dc48ff6b058eba3c08278ad2cb3667ef2d48b61b8ba8d3362effe7c9120376d7
  [O3-227]=9bedf971b69e362513081a24ea2e0a0cb6cab8fd5b7ce5b60992be213baa4a4d
  [O3-229]=efe156e690e56b94f9da4436ee1271a94e68754a94ca7acbc9b47901b0f64a3b
  [O4-223]=2c69f6f01f14c9180079a1dc154c84e15dba4c1c12b7860a0392fcc30da9b9aa
  [O4-227]=4a89d33cd91b2f51aa71c40cf16e5ee123fd4f52986ddeaa7d6f7b4981cd4fe6
  [O4-229]=8767a6b400c5cb45103a49076b9ae35414a753ea987393014ae3ec583c2ad554
)

normalise() {
	jq -S '
    del(.experiment_id,.hypothesis_id,.description,.date,.status,.seed,
        .cross_asset_collateral_marks)
  '
}

for cell in "${!expected[@]}"; do
	file="$config_dir/$cell.json"
	stage=${cell%%-*}
	seed=${cell##*-}
	if [[ "$seed" == 211 || "$seed" == 213 ]]; then
		source="$source_dir/$cell.json"
	else
		source="$config_dir/$stage-211.json"
	fi
	[[ -s "$file" && -s "$source" ]] || { echo "missing P6-R1 config: $cell" >&2; exit 1; }
	actual=$(sha256sum "$file" | awk '{print $1}')
	[[ "$actual" == "${expected[$cell]}" ]] || { echo "P6-R1 config hash mismatch: $cell" >&2; exit 1; }
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
	if [[ "$seed" == 223 || "$seed" == 227 || "$seed" == 229 ]]; then
		jq -e '.description | contains("untouched holdout")' "$file" >/dev/null
	fi
	if ! diff -u <(normalise <"$source") <(normalise <"$file") >/dev/null; then
		echo "P6-R1 $cell contains an undeclared delta" >&2
		exit 1
	fi
done

echo "V2-6 P6-R1 configs: explicit collateral mark delta verified"
