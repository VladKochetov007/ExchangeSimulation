#!/usr/bin/env bash
# Verify the immutable V2-5 P4 funding/carry config family. This script reads
# configs only; it does not run a world or inspect an outcome.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-5-p4"
development_seeds=(107 109)
holdout_seeds=(127 131 137)
all_seeds=("${development_seeds[@]}" "${holdout_seeds[@]}")
declare -A expected_sha256=(
	[A-107]=c2e759b7828eeef968d4acfc44f0d4b7312a78cd8039b4bc7f449bc3a046d029
	[B-107]=271825ccd0441c73d18a7f0d60e2dfe5c356a82494765acf6a6ac4e6b187f20b
	[A-109]=a2503e6d963029e5c01533b125014eb163f67b48c4d8a867b8ec97ecde9b90dc
	[B-109]=6b4eb86916d4d4a0b8813058119fa0f5bf932021e7d30ac4bbf5eb1ba0546b9c
	[A-127]=cb20ae27119c1d4a523931ebdf305b79d39964120a3c639be4cc4a268cfe21e4
	[B-127]=83d0f736b5a491ae8067c82772958b213e332065623221f1de071e91def5716e
	[A-131]=31771a138f91aab3d346804bfa9752a7f9ef39fa3c50f1423c03eb65816ea79f
	[B-131]=3b2b45b3f8c14f260a14de794e3f2a390a59507307cad4b19b9d32758d8ded3c
	[A-137]=e485f53aeaf235eb29cdbe9972fe71a1c0e10fcf9b204c96985430b7c8952933
	[B-137]=116110210b3602d87bedf37d78547d169eee9e7d14839534f903f892b9c30677
)

normalize_pair() {
	jq -S 'del(.experiment_id, .description, .funding_max_rate_bps)' "$1"
}

normalize_seed() {
	jq -S 'del(.experiment_id, .description, .seed)' "$1"
}

for seed in "${all_seeds[@]}"; do
	control="$config_dir/A-$seed.json"
	treatment="$config_dir/B-$seed.json"
	test -s "$control"
	test -s "$treatment"
	for arm in A B; do
		cell="$arm-$seed"
		actual_sha256=$(sha256sum "$config_dir/$cell.json" | awk '{print $1}')
		test "$actual_sha256" = "${expected_sha256[$cell]}"
	done
	jq -e --argjson seed "$seed" '
	  .seed == $seed and .funding_max_rate_bps == 1 and
	  .log_mode == "full" and .strict_population_accounting == true and
	  .record_market_data_receipts == true and
	  .record_term_carry_decisions == true and
	  .market_data_receipt_roles == ["term_carry_allocator"] and
	  .funding_interval_seconds == 28800 and
	  .term_carry_allocator.commitment_intervals == 12 and
	  .term_carry_allocator.passive_exit == {
	    slice_qty: 100000,
	    deadline_at_nano: 1736038805000000000
	  }
	' "$control" >/dev/null
	jq -e --argjson seed "$seed" '
	  .seed == $seed and .funding_max_rate_bps == 75 and
	  .log_mode == "full" and .strict_population_accounting == true and
	  .record_market_data_receipts == true and
	  .record_term_carry_decisions == true and
	  .market_data_receipt_roles == ["term_carry_allocator"] and
	  .funding_interval_seconds == 28800 and
	  .term_carry_allocator.commitment_intervals == 12 and
	  .term_carry_allocator.passive_exit == {
	    slice_qty: 100000,
	    deadline_at_nano: 1736038805000000000
	  }
	' "$treatment" >/dev/null
	diff -u <(normalize_pair "$control") <(normalize_pair "$treatment")
done

for arm in A B; do
	base="$config_dir/$arm-107.json"
	for seed in 109 127 131 137; do
		diff -u <(normalize_seed "$base") <(normalize_seed "$config_dir/$arm-$seed.json")
	done
done

printf 'V2-5 P4 configs: exact sole funding-cap intervention verified\n'
