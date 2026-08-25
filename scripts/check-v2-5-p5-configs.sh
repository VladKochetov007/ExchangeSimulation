#!/usr/bin/env bash
# Verify the immutable P5 config family before any implementation or run.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-5-p5"

declare -A expected=(
  [A-117]=ef212666c0f7126c4ece902ea6f51af6860e59617bd5c31204257b6f8092f834
  [B-117]=f43f5fc0a368d01aed7e748a8a9e71b41cb1a70f0dc07893b1ee2b25fc92e096
  [A-119]=848029742d0337cdb846d175c6e622558f6387c6ac25c5f408be266f7f6ff97c
  [B-119]=726c42f7934108197d26459f5bc91cb1264cabc7c4b2a42889a8aae01d3f0e5b
  [A-139]=7d7e97f3a466123afec64e6156a37e5341acc99ef1234fc7ba2100e691034a1b
  [B-139]=6f90d563a26a975376341fb14f8b2730c5f2d57e6d362d1a634c54ad512a4fe1
  [A-149]=647aab3668ed8bd6a3789b4e1c9f095057c1348ea5f01b149680ea71b7e0da41
  [B-149]=5c2ce805ac2ab0a3660c449516975ae35fd71fdacc24253e235b845f9d8c6399
  [A-151]=e3793b4ac22c1c5284f3d19cb06b13c3b2aba6d2e999e01b942a5437bcc7e75b
  [B-151]=eb4558ebc87e9ed2c0e83db32420d78bd65fd65851c37d670bd4d3e49132b7c6
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
    (.latency_profiles | keys) == ["dated_execution_mandate", "dated_term_carry_allocator"] and
    .latency_profiles.dated_execution_mandate.delay == 20000000 and
    .latency_profiles.dated_term_carry_allocator.delay == 20000000 and
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
