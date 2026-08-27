#!/usr/bin/env bash
# Validate the pre-registered integrated long-run configuration set without
# running a simulator. Economic fields must match the registered integrated
# reference after provenance identities and seed are removed.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-integrated-longrun"
source_full="$root_dir/research/configs/v2-integrated/reference-dev-601.json"
source_none="$root_dir/research/configs/v2-integrated/reference-dev-601-none.json"

fail() {
	echo "integrated long-run config failure: $*" >&2
	exit 1
}

[[ -s "$source_full" && -s "$source_none" ]] || fail "missing integrated reference source"

expected_full='607 613 617 619 631 641'
for seed in $expected_full; do
	if [[ "$seed" == 619 || "$seed" == 631 || "$seed" == 641 ]]; then
		file="$config_dir/holdout-$seed.json"
	else
		file="$config_dir/dev-$seed.json"
	fi
	[[ -s "$file" ]] || fail "missing $file"
	jq -e --argjson seed "$seed" \
		'.seed == $seed and .hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE" and
		 .log_mode == "full" and .record_market_data_receipts == true and
		 .record_decision_frontier_vectors == true and
		 .cross_asset_spot_graph == true and .cross_asset_collateral_marks == true and
		 .spot_passive_maker_post_only == true and
		 .spot_passive_maker_cancel_before_replace == true' "$file" >/dev/null ||
		fail "invalid full candidate fields in $file"
	# jq -S gives a semantic JSON comparison, avoiding irrelevant exponent or
	# whitespace changes introduced by deterministic config generation.
	if ! cmp -s \
		<(jq -cS 'del(.seed, .experiment_id, .hypothesis_id, .description)' "$source_full") \
		<(jq -cS 'del(.seed, .experiment_id, .hypothesis_id, .description)' "$file"); then
		fail "economic/config drift in $file"
	fi
done

none="$config_dir/dev-607-none.json"
[[ -s "$none" ]] || fail "missing $none"
jq -e '.seed == 607 and .hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE-PARITY" and
	.log_mode == "none" and .record_market_data_receipts == false and
	.record_decision_frontier_vectors == false and
	.record_maker_quote_size_decisions == false and
	.record_maker_inventory_rebalance_decisions == false and
	.record_liability_hedger_decisions == false and
	.record_noise_flow_phase_decisions == false and
	.record_option_liability_user_decisions == false' "$none" >/dev/null ||
	fail "invalid none parity fields in $none"
if ! cmp -s \
	<(jq -cS 'del(.seed, .experiment_id, .hypothesis_id, .description)' "$source_none") \
	<(jq -cS 'del(.seed, .experiment_id, .hypothesis_id, .description)' "$none"); then
	fail "economic/config drift in $none"
fi

echo "integrated long-run configs: all registered identities and economic fields valid"
