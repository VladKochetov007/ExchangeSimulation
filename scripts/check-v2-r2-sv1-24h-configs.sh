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
source_dir="$root_dir/research/configs/v2-integrated-longrun-r2"
while IFS=$'\t' read -r relative expected_sha; do
	[[ "$relative" != *[/:]* && "$relative" == *.json ]] || fail "unsafe source config identity: $relative"
	path="$source_dir/$relative"
	[[ -s "$path" && "$(sha256sum "$path" | awk '{print $1}')" == "$expected_sha" ]] || fail "source config hash mismatch: $relative"
done < <(jq -r '.source_configs | to_entries[] | [.key,.value] | @tsv' "$provenance_manifest")
activation_sha=$(sha256sum "$activation_config" | awk '{print $1}')
[[ "$activation_sha" == "$(jq -er '.activation_roster.sha256' "$provenance_manifest")" ]] || fail "activation roster hash mismatch"
while IFS=$'\t' read -r relative expected_sha; do
	[[ "$relative" != */* && "$relative" == *.json ]] || fail "unsafe registered config identity: $relative"
	path="$config_dir/$relative"
	[[ -s "$path" && "$(sha256sum "$path" | awk '{print $1}')" == "$expected_sha" ]] || fail "registered config hash mismatch: $relative"
done < <(jq -r '.registered_configs | to_entries[] | [.key,.value] | @tsv' "$provenance_manifest")

calendar='[{"name":"short","listing_interval_nano":3600000000000,"time_to_expiry_nano":7200000000000},{"name":"medium","listing_interval_nano":10800000000000,"time_to_expiry_nano":21600000000000},{"name":"long","listing_interval_nano":21600000000000,"time_to_expiry_nano":43200000000000}]'
jq -e '.elastic_liquidity_suppliers | type == "array" and length == 4' "$activation_config" >/dev/null || fail "activation roster must contain four suppliers"
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
