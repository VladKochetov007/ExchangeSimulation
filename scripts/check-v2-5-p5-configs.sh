#!/usr/bin/env bash
# Verify the immutable P5 config family before any implementation or run.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-5-p5"

declare -A expected=(
  [A-117]=83fa1d0431825e3926c416cc9a81ae8b7d6df278315bd6abd9df1372ea844f12
  [B-117]=3d7e9629d2387ab531138250ff72ac034115596fcaf34985365b7bd6988739be
  [A-119]=c40d0a324d8b827a09db7bc2594df584eee87377c513675fff3dc83ee24bcad4
  [B-119]=b74a090f871274ad8c8a4b4260179649e7899f36a712ac1507b8e44ead489403
  [A-139]=5f225a67355a60294d4a3fee20d53db4af4d420359e4a670d564e052a7516da1
  [B-139]=9b0a9f3f0047045c23dd1d388e4063e7633a094a518062006dfce15993c18fdb
  [A-149]=4c3d7fe31401a157534a308ea98209c8c6b0dd5590d50165d627e6650b0b02f1
  [B-149]=fdebc14de7ce3ee12be5a04a55ed07460d3fd2f82a3699995006e61fd861edf4
  [A-151]=090169094c3116f710512af7f222ded7ea9328d6c0474fbd955ceb98e3d5705c
  [B-151]=77fa8e726dd56e41515d8ac9947a742227b9dbaa883b87da984bece59814b4a3
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
