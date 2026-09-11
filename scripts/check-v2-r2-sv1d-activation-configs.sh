#!/usr/bin/env bash
# Validate the three development-only SV1D activation arms without opening a
# simulation world. The runner records the resulting file hashes separately;
# this checker owns the semantic shape of the registered arm triad.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-r2-sv1d-activation"
fail() {
	echo "SV1D activation config failure: $*" >&2
	exit 1
}

[[ -d "$config_dir" && ! -L "$config_dir" ]] || fail "config directory is missing or symlinked"
expected_files=$'activation-659-mode-off.json\nactivation-659-no-roster.json\nactivation-659-treatment.json'
actual_files=$(find "$config_dir" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | LC_ALL=C sort)
[[ "$actual_files" == "$expected_files" ]] || fail "registered file set differs"

for config in "$config_dir"/*.json; do
	[[ -s "$config" && ! -L "$config" ]] || fail "missing or symlinked config: $config"
	jq -e -s 'length == 1 and (.[0] | type == "object")' "$config" >/dev/null ||
		fail "config is not one JSON object: $config"
	jq -e '
		.seed == 659 and
		.log_mode == "full" and
		.evidence_format == "evstream_v3" and .evidence_contract_version == 2 and
		.record_market_data_receipts == true and
		.strict_population_accounting == true and .strict_risk_contract == true and
		.auto_borrow_spot == false and .cross_asset_spot_graph == true and
		.cross_asset_collateral_marks == false and
		.venue_ids == ["north", "central", "south"] and
		.venue_rules == {
			central: {funding_interval_seconds: 3600, matching_rule: "pro_rata"},
			north: {funding_interval_seconds: 28800, matching_rule: "price_time"},
			south: {funding_interval_seconds: 7200, matching_rule: "pro_rata"}
		} and
		.step == 1000000000 and .snapshot_interval == 1000000000 and
		.automation_interval == 1000000000 and .quote_interval == 1000000000 and
		.noise_interval == 2000000000 and .greek_interval == 60000000000 and
		.checkpoint_interval_seconds == 60 and
		(.market_data_receipt_roles | index("liability_hedger") != null) and
		.elastic_supplier_count == 8 and
		.r2_expiry_calendar.schedules == [
			{name: "short", listing_interval_nano: 3600000000000, time_to_expiry_nano: 7200000000000},
			{name: "medium", listing_interval_nano: 10800000000000, time_to_expiry_nano: 21600000000000},
			{name: "long", listing_interval_nano: 21600000000000, time_to_expiry_nano: 43200000000000}
		]
	' "$config" >/dev/null || fail "common contract failed: $(basename "$config")"
done

treatment="$config_dir/activation-659-treatment.json"
mode_off="$config_dir/activation-659-mode-off.json"
no_roster="$config_dir/activation-659-no-roster.json"

jq -e '
	(.elastic_liquidity_suppliers | type == "array" and length == 4) and
	(.elastic_liquidity_suppliers | map(.role)) == [
		"cdf_elastic_supplier_1", "cdf_elastic_supplier_2",
		"cdf_elastic_supplier_3", "cdf_elastic_supplier_4"
	] and
	(.elastic_liquidity_suppliers | map(.symbol)) == ["CDF/USD", "CDF/USD", "CDF/USD", "CDF/USD"] and
	(.elastic_liquidity_suppliers | map(.interval)) == [2000000000, 2000000000, 2000000000, 2000000000] and
	(.elastic_liquidity_suppliers | map(.decision_phase_offset // 0)) == [0, 500000000, 1000000000, 1500000000] and
	all(.elastic_liquidity_suppliers[];
		.base_asset == "CDF" and .quote_asset == "USD" and
		.base_precision == 100000000 and .quote_precision == 100000 and
		.initial_base_balance > 0 and .initial_quote_balance > 0 and
		.max_position > 0 and .max_inventory >= .initial_base_balance and
		.max_quote_qty >= .minimum_qualifying_qty and .max_loss_quote > 0 and
		.minimum_executable_qty == 100000 and .minimum_qualifying_qty == 1000000 and
		.registered_minimum_executable_qty == 100000 and .tick_size == 100000 and
		.max_observation_age == 60000000000 and .maker_fee_bps == 5 and
		.quote_on_one_sided_local_book == true) and
	.market_data_receipt_roles == ["cdf_elastic_supplier", "liability_hedger"] and
	.record_elastic_liquidity_supplier_decisions == true
' "$treatment" >/dev/null || fail "treatment arm contract failed"

jq -e '
	(.elastic_liquidity_suppliers | type == "array" and length == 4) and
	(.elastic_liquidity_suppliers | map(.role)) == [
		"cdf_elastic_supplier_1", "cdf_elastic_supplier_2",
		"cdf_elastic_supplier_3", "cdf_elastic_supplier_4"
	] and
	(.elastic_liquidity_suppliers | map(.decision_phase_offset // 0)) == [0, 500000000, 1000000000, 1500000000] and
	all(.elastic_liquidity_suppliers[]; (.quote_on_one_sided_local_book // false) == false) and
	.market_data_receipt_roles == ["cdf_elastic_supplier", "liability_hedger"] and
	.record_elastic_liquidity_supplier_decisions == true
' "$mode_off" >/dev/null || fail "mode-off arm contract failed"

jq -e '
	(.elastic_liquidity_suppliers == null or .elastic_liquidity_suppliers == []) and
	(.record_elastic_liquidity_supplier_decisions == null or .record_elastic_liquidity_supplier_decisions == false) and
	.market_data_receipt_roles == ["liability_hedger"]
' "$no_roster" >/dev/null || fail "no-roster arm contract failed"

identity_filter='del(.experiment_id,.hypothesis_id,.description,.date,.status)'
treatment_mode_filter="$identity_filter | del(.elastic_liquidity_suppliers[].quote_on_one_sided_local_book)"
[[ "$(jq -S "$treatment_mode_filter" "$treatment")" == "$(jq -S "$treatment_mode_filter" "$mode_off")" ]] ||
	fail "treatment/mode-off economic or roster baseline drift"

no_roster_filter="$identity_filter | .elastic_liquidity_suppliers = [] | .record_elastic_liquidity_supplier_decisions = false | .market_data_receipt_roles = [\"liability_hedger\"]"
[[ "$(jq -S "$no_roster_filter" "$mode_off")" == "$(jq -S "$no_roster_filter" "$no_roster")" ]] ||
	fail "mode-off/no-roster economic baseline drift"

echo "SV1D activation configs: valid immutable tri-arm shape"
