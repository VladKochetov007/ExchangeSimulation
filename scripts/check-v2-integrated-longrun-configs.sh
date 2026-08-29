#!/usr/bin/env bash
# Validate the pre-registered integrated long-run configuration set without
# running a simulator. Economic fields must match the registered integrated
# reference after provenance identities and seed are removed. This checker is
# deliberately stricter than the simulator's decoder: a valid JSON file is
# not sufficient evidence of registered identity.
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

jq -e 'type == "object" and (.seed | type) == "number" and
  (.experiment_id | type) == "string" and (.hypothesis_id | type) == "string" and
  (.log_mode | type) == "string"' "$source_full" >/dev/null ||
	fail "malformed full integrated reference"
jq -e 'type == "object" and (.seed | type) == "number" and
  (.experiment_id | type) == "string" and (.hypothesis_id | type) == "string" and
  (.log_mode | type) == "string"' "$source_none" >/dev/null ||
	fail "malformed no-log integrated reference"

expected_files=(
	dev-607-none.json dev-607.json dev-613.json dev-617.json
	holdout-619.json holdout-631.json holdout-641.json
)
mapfile -t actual_files < <(find "$config_dir" -maxdepth 1 -type f -name '*.json' -printf '%f\n' | sort)
if ! printf '%s\n' "${actual_files[@]}" | cmp -s - <(printf '%s\n' "${expected_files[@]}" | sort); then
	fail "unregistered or missing config files: ${actual_files[*]}"
fi

expected_full='607 613 617 619 631 641'
for seed in $expected_full; do
	if [[ "$seed" == 619 || "$seed" == 631 || "$seed" == 641 ]]; then
		file="$config_dir/holdout-$seed.json"
	else
		file="$config_dir/dev-$seed.json"
	fi
	[[ -s "$file" ]] || fail "missing $file"
	jq -e --argjson seed "$seed" \
		'type == "object" and (.seed | type) == "number" and .seed == $seed and
		 .experiment_id == ("v2-integrated-longrun-" + (if $seed == 619 or $seed == 631 or $seed == 641 then "holdout-" else "dev-" end) + ($seed | tostring)) and
		 .hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE" and
			.log_mode == "full" and .record_market_data_receipts == true and
			.record_decision_frontier_vectors == true and
			.cross_asset_spot_graph == true and .cross_asset_collateral_marks == true and
			.spot_passive_maker_post_only == true and
			.spot_passive_maker_cancel_before_replace == true and
			(.record_maker_quote_size_decisions | type) == "boolean" and
			(.record_maker_inventory_rebalance_decisions | type) == "boolean" and
			(.record_liability_hedger_decisions | type) == "boolean" and
			(.checkpoint_interval_seconds | type) == "number" and
			.checkpoint_interval_seconds == 60' "$file" >/dev/null ||
		fail "invalid full candidate fields in $file"
	# jq -S gives a semantic JSON comparison, avoiding irrelevant exponent or
	# whitespace changes introduced by deterministic config generation.
	if ! cmp -s \
		<(jq -cS 'del(.seed, .experiment_id, .hypothesis_id, .description, .dated_future_delivery_fee_policy)' "$source_full") \
		<(jq -cS 'del(.seed, .experiment_id, .hypothesis_id, .description, .dated_future_delivery_fee_policy)' "$file"); then
		fail "economic/config drift in $file"
	fi
done

for seed in 607 613 617; do
	jq -e '.dated_future_delivery_fee_policy == "zero"' "$config_dir/dev-$seed.json" >/dev/null ||
		fail "development config lacks the pinned delivery fee policy: dev-$seed"
done

none="$config_dir/dev-607-none.json"
[[ -s "$none" ]] || fail "missing $none"
jq -e 'type == "object" and (.seed | type) == "number" and .seed == 607 and
	.experiment_id == "v2-integrated-longrun-dev-607-none" and
	.hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE-PARITY" and
	.log_mode == "none" and .record_market_data_receipts == false and
	.record_decision_frontier_vectors == false and
	.record_maker_quote_size_decisions == false and
	.record_maker_inventory_rebalance_decisions == false and
	.record_liability_hedger_decisions == false and
	.record_noise_flow_phase_decisions == false and
	.record_option_liability_user_decisions == false' "$none" >/dev/null ||
	fail "invalid none parity fields in $none"

jq -e '.dated_future_delivery_fee_policy == "zero"' "$none" >/dev/null ||
	fail "none parity config lacks the pinned delivery fee policy"
if ! cmp -s \
	<(jq -cS 'del(.seed, .experiment_id, .hypothesis_id, .description, .dated_future_delivery_fee_policy)' "$source_none") \
	<(jq -cS 'del(.seed, .experiment_id, .hypothesis_id, .description, .dated_future_delivery_fee_policy)' "$none"); then
	fail "economic/config drift in $none"
fi

echo "integrated long-run configs: all registered identities and economic fields valid"
