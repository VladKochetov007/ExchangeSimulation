#!/usr/bin/env bash
# Validate the R2 successor configuration namespace without touching the
# historical rolling-ladder registration.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-integrated-longrun-r2"
source_full="$root_dir/research/configs/v2-integrated-longrun/dev-607.json"
source_none="$root_dir/research/configs/v2-integrated-longrun/dev-607-none.json"

fail() {
	echo "integrated long-run R2 config failure: $*" >&2
	exit 1
}

expected_files=(dev-607-none.json dev-607.json dev-613.json dev-617.json holdout-619.json holdout-631.json holdout-641.json)
mapfile -t actual_files < <(find "$config_dir" -maxdepth 1 -type f -name '*.json' -printf '%f\n' | sort)
if ! printf '%s\n' "${actual_files[@]}" | cmp -s - <(printf '%s\n' "${expected_files[@]}" | sort); then
	fail "unregistered or missing config files: ${actual_files[*]}"
fi

calendar='[{"name":"short","listing_interval_nano":3600000000000,"time_to_expiry_nano":7200000000000},{"name":"medium","listing_interval_nano":10800000000000,"time_to_expiry_nano":21600000000000},{"name":"long","listing_interval_nano":21600000000000,"time_to_expiry_nano":43200000000000}]'
evidence_format='evstream_v3'

for cell in dev-607 dev-613 dev-617 holdout-619 holdout-631 holdout-641; do
	seed=${cell#*-}
	file="$config_dir/$cell.json"
	[[ -s "$file" ]] || fail "missing $file"
	jq -e --arg cell "$cell" --argjson seed "$seed" --argjson calendar "$calendar" --arg evidence_format "$evidence_format" \
		'.seed == $seed and
		 .experiment_id == ("v2-integrated-longrun-r2-" + $cell) and
		 .hypothesis_id == "V2-INTEGRATED-LONG-R2-CANDIDATE" and
		 .log_mode == "full" and .evidence_format == $evidence_format and .record_market_data_receipts == true and
		 .record_decision_frontier_vectors == true and
		 .cross_asset_spot_graph == true and .cross_asset_collateral_marks == true and
		 .spot_passive_maker_post_only == true and .spot_passive_maker_cancel_before_replace == true and
		 .checkpoint_interval_seconds == 60 and
		 .short_future_tenor == 7200000000000 and .long_future_tenor == 43200000000000 and
		 .short_option_tenor == 7200000000000 and .long_option_tenor == 43200000000000 and
		 .r2_expiry_calendar.schedules == $calendar' "$file" >/dev/null ||
		fail "invalid R2 full candidate fields in $file"
	if [[ "$(jq -s '.[0] == .[1]' \
		<(jq 'del(.seed, .experiment_id, .hypothesis_id, .description, .dated_future_delivery_fee_policy, .short_future_tenor, .long_future_tenor, .short_option_tenor, .long_option_tenor, .r2_expiry_calendar, .evidence_format)' "$source_full") \
		<(jq 'del(.seed, .experiment_id, .hypothesis_id, .description, .dated_future_delivery_fee_policy, .short_future_tenor, .long_future_tenor, .short_option_tenor, .long_option_tenor, .r2_expiry_calendar, .evidence_format)' "$file"))" != true ]]; then
		fail "non-calendar economic/config drift in $file"
	fi
done

none="$config_dir/dev-607-none.json"
jq -e --argjson calendar "$calendar" --arg evidence_format "$evidence_format" \
	'.seed == 607 and .experiment_id == "v2-integrated-longrun-r2-dev-607-none" and
	 .hypothesis_id == "V2-INTEGRATED-LONG-R2-CANDIDATE-PARITY" and .log_mode == "none" and .evidence_format == $evidence_format and
	 .record_market_data_receipts == false and .record_decision_frontier_vectors == false and
	 .record_maker_quote_size_decisions == false and .record_maker_inventory_rebalance_decisions == false and
	 .record_liability_hedger_decisions == false and .record_noise_flow_phase_decisions == false and
	 .record_option_liability_user_decisions == false and
	 .short_future_tenor == 7200000000000 and .long_future_tenor == 43200000000000 and
	 .short_option_tenor == 7200000000000 and .long_option_tenor == 43200000000000 and
	 .r2_expiry_calendar.schedules == $calendar' "$none" >/dev/null || fail "invalid R2 parity fields in $none"
if [[ "$(jq -s '.[0] == .[1]' \
	<(jq 'del(.seed, .experiment_id, .hypothesis_id, .description, .dated_future_delivery_fee_policy, .short_future_tenor, .long_future_tenor, .short_option_tenor, .long_option_tenor, .r2_expiry_calendar, .evidence_format)' "$source_none") \
	<(jq 'del(.seed, .experiment_id, .hypothesis_id, .description, .dated_future_delivery_fee_policy, .short_future_tenor, .long_future_tenor, .short_option_tenor, .long_option_tenor, .r2_expiry_calendar, .evidence_format)' "$none"))" != true ]]; then
	fail "non-calendar economic/config drift in $none"
fi

echo "integrated long-run R2 configs: all development/reserved identities and calendar fields valid"
