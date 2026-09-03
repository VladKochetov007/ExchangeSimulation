#!/usr/bin/env bash
# Validate the immutable SV1 24-hour treatment/control configuration set.
# Historical R2 configs are read-only inputs and are never modified here.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
contract_script=$(v2_r2_select_sv1_contract "$root_dir") || {
	echo "SV1 config checker received an unregistered contract path" >&2
	exit 1
}
source "$contract_script"
export V2_R2_SV1_CONTRACT_SCRIPT="$contract_script"
config_dir="$v2_r2_sv1_config_dir"
activation_config="$v2_r2_sv1_activation_config"
provenance_manifest="$v2_r2_sv1_config_provenance_manifest"

fail() {
	echo "SV1 24-hour config failure: $*" >&2
	exit 1
}

expected_files=("control-${v2_r2_sv1_parity_seed}-none.json")
for seed in "${v2_r2_sv1_seeds[@]}"; do
	expected_files+=("control-$seed.json" "treatment-$seed.json")
done
mapfile -t expected_files < <(printf '%s\n' "${expected_files[@]}" | sort)
mapfile -t actual_files < <(find "$config_dir" -maxdepth 1 -type f -name '*.json' -printf '%f\n' | sort)
[[ "${actual_files[*]}" == "${expected_files[*]}" ]] || fail "unregistered or missing config files: ${actual_files[*]}"
[[ -s "$provenance_manifest" && ! -L "$provenance_manifest" ]] || fail "missing config provenance manifest"
expected_files_json=$(printf '%s\n' "${expected_files[@]}" | jq -Rsc 'split("\n") | map(select(length > 0)) | sort')
expected_sources_json=$(printf '%s\n' "${v2_r2_sv1_source_config_names[@]}" | jq -Rsc 'split("\n") | map(select(length > 0)) | sort')
activation_path="${activation_config#"$root_dir/"}"
jq -e --argjson expected_configs "$expected_files_json" \
	--argjson expected_sources "$expected_sources_json" \
	--argjson require_candidate "$v2_r2_sv1_require_candidate_metadata" \
	--argjson require_generator "$v2_r2_sv1_require_generator_metadata" \
	--arg provenance_contract "$v2_r2_sv1_config_provenance_contract" \
	--arg activation_path "$activation_path" \
	--arg candidate "$v2_r2_sv1_candidate_id" \
	'.schema_version == 1 and .contract == $provenance_contract and
	 ($require_candidate == false or .candidate == $candidate) and
		 (.source_configs | keys | sort) == $expected_sources and
		 .activation_roster.path == $activation_path and
		 (.registered_configs | keys | sort) == $expected_configs and
		 ($require_generator == false or ((.generator.path | type) == "string" and (.generator.sha256 | test("^[0-9a-f]{64}$"))))' "$provenance_manifest" >/dev/null || fail "invalid config provenance manifest"
if [[ "$v2_r2_sv1_require_generator_metadata" == true ]]; then
	generator_path=$(jq -er '.generator.path | select(type == "string")' "$provenance_manifest") || fail "config provenance omits generator path"
	[[ "$generator_path" == "scripts/render-v2-r2-sv1b-24h-configs.sh" ]] || fail "config provenance names an unexpected generator"
	generator_file="$root_dir/$generator_path"
	[[ -f "$generator_file" && ! -L "$generator_file" ]] || fail "config generator is missing or symlinked"
	generator_sha=$(sha256sum "$generator_file" | awk '{print $1}')
	[[ "$generator_sha" == "$(jq -er '.generator.sha256' "$provenance_manifest")" ]] || fail "config generator hash mismatch"
fi
if [[ "$v2_r2_sv1_candidate_id" == V2-R2-SV1B-* ]]; then
	withdrawal_measurement_path=$(jq -er '.withdrawal_measurement.path' "$provenance_manifest") || fail "SV1B provenance omits withdrawal measurement amendment"
	[[ "$withdrawal_measurement_path" == "research/v2-r2-sv1b-withdrawal-measurement-amendment-2026-09-02.md" ]] || fail "SV1B provenance names an unexpected withdrawal measurement amendment"
	withdrawal_measurement_file="$root_dir/$withdrawal_measurement_path"
	[[ -s "$withdrawal_measurement_file" && ! -L "$withdrawal_measurement_file" ]] || fail "missing withdrawal measurement amendment"
	withdrawal_measurement_sha=$(sha256sum "$withdrawal_measurement_file" | awk '{print $1}')
	[[ "$withdrawal_measurement_sha" == "$(jq -er '.withdrawal_measurement.sha256' "$provenance_manifest")" ]] || fail "withdrawal measurement amendment hash mismatch"
	diagnostics_path=$(jq -er '.activation_diagnostics.path' "$provenance_manifest") || fail "SV1B provenance omits activation diagnostic amendment"
	[[ "$diagnostics_path" == "research/v2-r2-sv1b-activation-diagnostics-amendment-2026-09-02.md" ]] || fail "SV1B provenance names an unexpected activation diagnostic amendment"
	diagnostics_file="$root_dir/$diagnostics_path"
	[[ -s "$diagnostics_file" && ! -L "$diagnostics_file" ]] || fail "missing activation diagnostic amendment"
	diagnostics_sha=$(sha256sum "$diagnostics_file" | awk '{print $1}')
	[[ "$diagnostics_sha" == "$(jq -er '.activation_diagnostics.sha256' "$provenance_manifest")" ]] || fail "activation diagnostic amendment hash mismatch"
	preregistration_path=$(jq -er '.preregistration.path | select(type == "string")' "$provenance_manifest") || fail "SV1B provenance omits preregistration"
	[[ "$preregistration_path" == "research/v2-r2-sv1b-cdf-successor-preregistration-2026-09-02.md" ]] || fail "SV1B provenance names an unexpected preregistration"
	preregistration_file="$root_dir/$preregistration_path"
	[[ -s "$preregistration_file" && ! -L "$preregistration_file" ]] || fail "missing SV1B preregistration"
	preregistration_sha=$(sha256sum "$preregistration_file" | awk '{print $1}')
	[[ "$preregistration_sha" == "$(jq -er '.preregistration.sha256' "$provenance_manifest")" ]] || fail "SV1B preregistration hash mismatch"
	capacity_order_path=$(jq -er '.capacity_ordering.path | select(type == "string")' "$provenance_manifest") || fail "SV1B provenance omits capacity-order amendment"
	[[ "$capacity_order_path" == "research/v2-r2-sv1b-activation-capacity-order-amendment-2026-09-03.md" ]] || fail "SV1B provenance names an unexpected capacity-order amendment"
	capacity_order_file="$root_dir/$capacity_order_path"
	[[ -s "$capacity_order_file" && ! -L "$capacity_order_file" ]] || fail "missing capacity-order amendment"
	capacity_order_sha=$(sha256sum "$capacity_order_file" | awk '{print $1}')
	[[ "$capacity_order_sha" == "$(jq -er '.capacity_ordering.sha256' "$provenance_manifest")" ]] || fail "capacity-order amendment hash mismatch"
	capacity_cases='[]'
	capacity_case_specs=(
		"$config_dir/treatment-643.json|4|capacity_only_seed_659_treatment_g4"
		"$config_dir/control-643.json|4|capacity_only_seed_659_control_g4"
		"$config_dir/treatment-643.json|8|capacity_only_seed_659_treatment_g8"
	)
	for capacity_case_spec in "${capacity_case_specs[@]}"; do
		IFS='|' read -r capacity_config gomaxprocs capacity_role <<<"$capacity_case_spec"
		capacity_config_path="${capacity_config#"$root_dir/"}"
		capacity_measurement_sha=$(sha256sum "$capacity_config" | awk '{print $1}')
		capacity_attestation_path=$(v2_r2_capacity_attestation_path_for_config "$capacity_config" "$gomaxprocs") || fail "production capacity case has no attestation identity: $capacity_config G$gomaxprocs"
		capacity_probe_cell=$(v2_r2_capacity_probe_cell_for_config "$capacity_config" "$gomaxprocs") || fail "production capacity case has no probe identity: $capacity_config G$gomaxprocs"
		capacity_case=$(jq -cn --arg config_path "$capacity_config_path" --arg measurement_config_sha256 "$capacity_measurement_sha" \
			--arg attestation_path "$capacity_attestation_path" --arg probe_cell "$capacity_probe_cell" --arg role "$capacity_role" --argjson gomaxprocs "$gomaxprocs" \
			'{config_path:$config_path,measurement_config_sha256:$measurement_config_sha256,measurement_seed:659,source_seed:643,gomaxprocs:$gomaxprocs,attestation_path:$attestation_path,probe_cell:$probe_cell,role:$role}')
		capacity_cases=$(jq -c --argjson case "$capacity_case" '. + [$case]' <<<"$capacity_cases")
	done
	launch_config_path="${v2_r2_sv1_capacity_launch_config#"$root_dir/"}"
	authorized_launch_config_hashes=$(for config_name in "${v2_r2_sv1_capacity_authorized_launch_config_names[@]}"; do
		sha256sum -- "$config_dir/$config_name" | awk '{print $1}'
	done | jq -Rsc 'split("\n") | map(select(length > 0))')
	jq -e --arg contract "$v2_r2_sv1_capacity_attestation_contract" --arg mode "production_capacity_measurement" \
		--arg measurement_config_path "${v2_r2_sv1_capacity_measurement_config#"$root_dir/"}" --arg launch_config_path "$launch_config_path" \
		--argjson authorized "$authorized_launch_config_hashes" --argjson cases "$capacity_cases" \
		'.capacity_calibration.contract == $contract and .capacity_calibration.mode == $mode and
			 .capacity_calibration.calibration_only == true and .capacity_calibration.minimum_free_bytes == 4294967296 and
			 .capacity_calibration.safety_margin_bytes == 4294967296 and .capacity_calibration.memory_limit_bytes == 21474836480 and
			 .capacity_calibration.gomemlimit_bytes == 19327352832 and .capacity_calibration.capacity_only_seed == 659 and
			 .capacity_calibration.source_config_seed == 643 and
			 .capacity_calibration.measurement_config_path == $measurement_config_path and
		 .capacity_calibration.launch_config_path == $launch_config_path and
		 .capacity_calibration.authorized_launch_config_sha256 == $authorized and
		 .capacity_calibration.measurement_cases == $cases' "$provenance_manifest" >/dev/null ||
		fail "SV1B capacity provenance does not bind activation calibration to the registered launch set"
fi
source_dir="$root_dir/research/configs/v2-integrated-longrun-r2"
while IFS=$'\t' read -r relative expected_sha; do
	[[ "$relative" != *[/:]* && "$relative" == *.json ]] || fail "unsafe source config identity: $relative"
	path="$source_dir/$relative"
	[[ -s "$path" && "$(sha256sum "$path" | awk '{print $1}')" == "$expected_sha" ]] || fail "source config hash mismatch: $relative"
done < <(jq -r '.source_configs | to_entries[] | [.key,.value] | @tsv' "$provenance_manifest")
activation_sha=$(sha256sum "$activation_config" | awk '{print $1}')
[[ "$activation_sha" == "$(jq -er '.activation_roster.sha256' "$provenance_manifest")" ]] || fail "activation roster hash mismatch"
activation_control_path=$(jq -er '.activation_control.path | select(type == "string")' "$provenance_manifest") || fail "SV1B provenance omits activation control"
[[ "$activation_control_path" == "research/configs/v2-r2-sv1b/activation-643-control.json" ]] || fail "SV1B provenance names an unexpected activation control"
activation_control_file="$root_dir/$activation_control_path"
[[ -s "$activation_control_file" && ! -L "$activation_control_file" ]] || fail "missing activation control"
activation_control_sha=$(sha256sum "$activation_control_file" | awk '{print $1}')
[[ "$activation_control_sha" == "$(jq -er '.activation_control.sha256' "$provenance_manifest")" ]] || fail "activation control hash mismatch"
while IFS=$'\t' read -r relative expected_sha; do
	[[ "$relative" != */* && "$relative" == *.json ]] || fail "unsafe registered config identity: $relative"
	path="$config_dir/$relative"
	[[ -s "$path" && "$(sha256sum "$path" | awk '{print $1}')" == "$expected_sha" ]] || fail "registered config hash mismatch: $relative"
done < <(jq -r '.registered_configs | to_entries[] | [.key,.value] | @tsv' "$provenance_manifest")

calendar='[{"name":"short","listing_interval_nano":3600000000000,"time_to_expiry_nano":7200000000000},{"name":"medium","listing_interval_nano":10800000000000,"time_to_expiry_nano":21600000000000},{"name":"long","listing_interval_nano":21600000000000,"time_to_expiry_nano":43200000000000}]'
jq -e '.elastic_liquidity_suppliers | type == "array" and length == 4' "$activation_config" >/dev/null || fail "activation roster must contain four suppliers"
for activation_pair_config in "$activation_config" "$activation_control_file"; do
	jq -e '.seed == 643 and .log_mode == "full" and .evidence_format == "evstream_v3" and .record_market_data_receipts == true' "$activation_pair_config" >/dev/null ||
		fail "activation pair config is not explicit full-log evstream evidence: $activation_pair_config"
done
if [[ "$v2_r2_sv1_candidate_id" == V2-R2-SV1B-* ]]; then
	jq -e '
		all(.elastic_liquidity_suppliers[];
			(.max_loss_quote | type) == "number" and .max_loss_quote > 0 and
			.max_loss_quote == ((.initial_quote_balance +
				((.initial_base_balance * .reference_price) / .base_precision)) / 10) and
			(.minimum_executable_qty | type) == "number" and
			.minimum_executable_qty == (.base_precision / 1000) and
			.minimum_executable_qty <= .max_quote_qty)
	' "$activation_config" >/dev/null || fail "SV1B roster must register a 10 percent marked-equity loss budget and venue minimum executable depth"
fi
roster=$(jq -c '.elastic_liquidity_suppliers' "$activation_config")

for seed in "${v2_r2_sv1_seeds[@]}"; do
	treatment="$config_dir/treatment-$seed.json"
	control="$config_dir/control-$seed.json"
	jq -e --argjson seed "$seed" --argjson calendar "$calendar" --argjson roster "$roster" \
		'.seed == $seed and .log_mode == "full" and .evidence_format == "evstream_v3" and
		 .record_market_data_receipts == true and .record_elastic_liquidity_supplier_decisions == true and
		 (.market_data_receipt_roles | index("cdf_elastic_supplier") != null) and
		 .r2_expiry_calendar.schedules == $calendar and
		 .elastic_liquidity_suppliers == $roster' "$treatment" >/dev/null || fail "invalid treatment $seed"
	jq -e --argjson seed "$seed" --argjson calendar "$calendar" \
		'.seed == $seed and .log_mode == "full" and .evidence_format == "evstream_v3" and
		 .record_market_data_receipts == true and
		 (.record_elastic_liquidity_supplier_decisions == null or .record_elastic_liquidity_supplier_decisions == false) and
		 (.elastic_liquidity_suppliers == null or .elastic_liquidity_suppliers == []) and
		 (.market_data_receipt_roles | index("cdf_elastic_supplier") == null) and
		 .r2_expiry_calendar.schedules == $calendar' "$control" >/dev/null || fail "invalid control $seed"
	if [[ "$(jq -S 'del(.experiment_id,.hypothesis_id,.description,.date,.status,.elastic_liquidity_suppliers,.record_elastic_liquidity_supplier_decisions,.market_data_receipt_roles)' "$treatment")" != \
		"$(jq -S 'del(.experiment_id,.hypothesis_id,.description,.date,.status,.elastic_liquidity_suppliers,.record_elastic_liquidity_supplier_decisions,.market_data_receipt_roles)' "$control")" ]]; then
		fail "treatment/control economic drift for seed $seed"
	fi
done

none="$config_dir/control-${v2_r2_sv1_parity_seed}-none.json"
jq -e --argjson seed "$v2_r2_sv1_parity_seed" '.seed == $seed and .log_mode == "none" and .evidence_format == "evstream_v3" and
	.record_market_data_receipts == false and
	(.elastic_liquidity_suppliers == null or .elastic_liquidity_suppliers == []) and
	(.market_data_receipt_roles | index("cdf_elastic_supplier") == null)' "$none" >/dev/null || fail "invalid no-log control"

echo "SV1 24-hour configs: valid pairs; source and roster provenance verified (activation=$activation_sha)"
