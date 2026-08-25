#!/usr/bin/env bash
# Verify the immutable P5 config family before any implementation or run.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-5-p5"

declare -A expected=(
  [A-117]=c1ca7a6401ac0723a9226c826ccdb2002232a0743ef88847778b41c8b1487685
  [B-117]=55bfedd029d3ad91d8ff0b99983ba8692dc95f3b91dc30b6f953d3c3d8ea0544
  [A-119]=9085c514316ba988a288f51eb4d2da6987fa3ef368f79bfa28193c2e8fc576ae
  [B-119]=630684b1d68fb4e0cb42644d8dea92c66e13cc4fb70a9f01af66bed413d8fe77
  [A-139]=ec832268ffbb6a0b9584868af062af8782498223cb1be6feb27b8b208f42ceb3
  [B-139]=37d3d94b3bc2ddb1ca6985aef8b09d1a18547babfb8b7f56b436d35f5660ea64
  [A-149]=c2e930e468ab9415f0c16c45b7c6199edbdcb6188afc47621350ccb29b20454f
  [B-149]=da9d96804a76ca695e78099169d3724b1f4c2bb8b581990d96babcb4cbb62df4
  [A-151]=545d60feb5bf6b536e59bdff88ebc03b2e80958e4bab01e69625d1c86fdac2fc
  [B-151]=0bce13c8f202a20330a35a3e87bca2438b7faf4a6a381b45786cbed994e62f7c
)

for cell in "${!expected[@]}"; do
	actual=$(sha256sum "$config_dir/$cell.json" | awk '{print $1}')
	if [[ "$actual" != "${expected[$cell]}" ]]; then
		echo "P5 config hash mismatch: $cell" >&2
		exit 1
	fi

	arm=${cell%%-*}
	seed=${cell##*-}
	trade=false
	[[ "$arm" == B ]] && trade=true
	jq -e --argjson seed "$seed" --argjson trade "$trade" '
    .seed == $seed and .log_mode == "full" and
    .market_data_receipt_roles == ["dated_execution_mandate", "dated_term_carry_allocator"] and
    .term_carry_allocator == null and .record_term_carry_decisions == false and
    .option_flow_include_futures == false and
    .record_dated_execution_mandate_decisions == true and
    .record_dated_term_carry_decisions == true and
    .dated_future_execution_mandate.enabled == true and
    .dated_future_execution_mandate.target_tenor_nanos == 28800000000000 and
    .dated_future_execution_mandate.parent_qty == 200000000 and
    .dated_future_execution_mandate.child_qty == 10000000 and
    .dated_term_carry_allocator.enabled == true and
    .dated_term_carry_allocator.trade_enabled == $trade and
    .dated_term_carry_allocator.min_time_to_expiry_nanos == 600000000000 and
    .dated_term_carry_allocator.max_position == 100000000 and
    .dated_term_carry_allocator.lot_qty == 10000000 and
    .dated_term_carry_allocator.settlement_mismatch_bps == 2 and
    .dated_term_carry_allocator.post_settlement_exit_bps == 2
  ' "$config_dir/$cell.json" >/dev/null
done

for seed in 117 119 139 149 151; do
	if ! diff -u \
		<(jq -S 'del(.experiment_id,.description,.dated_term_carry_allocator.trade_enabled)' "$config_dir/A-$seed.json") \
		<(jq -S 'del(.experiment_id,.description,.dated_term_carry_allocator.trade_enabled)' "$config_dir/B-$seed.json") >/dev/null; then
		echo "P5 pair $seed contains an undeclared delta" >&2
		exit 1
	fi
done

for arm in A B; do
	for seed in 119 139 149 151; do
		if ! diff -u \
			<(jq -S 'del(.experiment_id,.description,.seed)' "$config_dir/$arm-117.json") \
			<(jq -S 'del(.experiment_id,.description,.seed)' "$config_dir/$arm-$seed.json") >/dev/null; then
			echo "P5 $arm seed $seed contains an undeclared non-seed delta" >&2
			exit 1
		fi
	done
done

echo "V2-5 P5 configs: exact shadow/active trade-permission intervention verified"
