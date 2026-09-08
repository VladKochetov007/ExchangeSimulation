#!/usr/bin/env bash
# Materialize the immutable SV1C development namespace from one accepted R2
# base configuration. The seed transformation is deterministic and does not
# alter the historical R2 or SV1 config directories.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_dir="$root_dir/research/configs/v2-r2-sv1c-24h"
source_dir="$root_dir/research/configs/v2-integrated-longrun-r2"
activation_config="$root_dir/research/configs/v2-r2-sv1c/activation-643.json"
activation_control_config="$root_dir/research/configs/v2-r2-sv1c/activation-643-control.json"
source "$root_dir/scripts/v2-r2-sv1c-24h-contract.sh"
normalizer_input=${V2_R2_SV1C_CONFIG_NORMALIZER_BIN:-"$root_dir/$v2_r2_sv1_config_normalizer_path"}
normalizer=$(realpath -e -- "$normalizer_input") || {
	echo "could not resolve config normalizer: $normalizer_input" >&2
	exit 1
}
[[ "$normalizer" == "$root_dir/$v2_r2_sv1_config_normalizer_path" ]] || {
	echo "config normalizer must be the registered path: $root_dir/$v2_r2_sv1_config_normalizer_path" >&2
	exit 1
}
normalizer_sha256=$(sha256sum -- "$normalizer" | awk '{print $1}')
normalizer_metadata=$(go version -m -- "$normalizer") || {
	echo "could not read config normalizer build metadata" >&2
	exit 1
}
normalizer_revision=$(v2_r2_sv1c_binary_metadata_value "$normalizer_metadata" "vcs.revision") || {
	echo "config normalizer does not expose exactly one VCS revision" >&2
	exit 1
}
v2_r2_sv1c_require_normalizer_source_revision "$root_dir" "$normalizer_revision" || {
	echo "config normalizer source revision is not an unchanged registered Go input tree" >&2
	exit 1
}
v2_r2_sv1c_require_pinned_binary "$normalizer" "$normalizer_revision" "$normalizer_sha256" "$v2_r2_sv1_config_normalizer_package" || {
	echo "config normalizer is not a pinned Go 1.27 build of its registered source tree" >&2
	exit 1
}
normalizer_go_version=$(go version -m -- "$normalizer" | sed -n '1s/.*: //p')
candidate="V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK"
control_hypothesis="V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK-CONTROL"
date="2026-09-08"
seeds=(643 647 653)

[[ -d "$config_dir" ]] || mkdir -p -- "$config_dir"
[[ -s "$activation_config" && -s "$activation_control_config" ]] || {
	echo "missing SV1C activation pair: $activation_config $activation_control_config" >&2
	exit 1
}
[[ -s "$source_dir/dev-607.json" && -s "$source_dir/dev-607-none.json" ]] || {
	echo "missing accepted R2 source configs" >&2
	exit 1
}
[[ -x "$normalizer" ]] || { echo "missing config normalizer: $normalizer" >&2; exit 1; }
jq -e '.elastic_liquidity_suppliers | type == "array" and length == 4' "$activation_config" >/dev/null || {
	echo "SV1C activation roster must contain exactly four suppliers" >&2
	exit 1
}
for activation_pair_config in "$activation_config" "$activation_control_config"; do
	jq -e '
		.strict_risk_contract == true and
		.auto_borrow_spot == false and
		.cross_asset_spot_graph == true and
		.cross_asset_collateral_marks == false and
		(.perp_exposure_hedger == null or .perp_exposure_hedger.auto_borrow_perp != true)
	' "$activation_pair_config" >/dev/null || {
		echo "SV1C activation pair is not explicit strict-risk configuration: $activation_pair_config" >&2
		exit 1
	}
done
cdf_roster=$(jq -c '.elastic_liquidity_suppliers' "$activation_config")

write_config() {
	local output=$1 source=$2 mode=$3 seed=$4 experiment=$5 hypothesis=$6
	[[ ! -e "$output" && ! -L "$output" ]] || {
		echo "refusing to overwrite registered config: $output" >&2
		exit 1
	}
	local temporary normalized_dir normalized
	temporary=$(mktemp "$output.tmp-XXXXXX")
	if [[ "$mode" == treatment ]]; then
		jq --arg experiment "$experiment" --arg hypothesis "$hypothesis" --arg description "V2-R2-SV1C-24H finite heterogeneous CDF/USD liquidity treatment under the strict-risk amendment; development-only successor" --arg date "$date" --argjson seed "$seed" --argjson roster "$cdf_roster" \
			'.seed = $seed |
			 .experiment_id = $experiment |
			 .hypothesis_id = $hypothesis |
			 .description = $description |
			 .date = $date |
			 .status = "registered-development" |
			 .strict_risk_contract = true |
			 .auto_borrow_spot = false |
			 .cross_asset_spot_graph = true |
			 .cross_asset_collateral_marks = false |
			 .elastic_liquidity_suppliers = $roster |
			 .record_elastic_liquidity_supplier_decisions = true |
			 .market_data_receipt_roles = ((.market_data_receipt_roles + ["cdf_elastic_supplier"]) | unique)' "$source" >"$temporary"
	else
		jq --arg experiment "$experiment" --arg hypothesis "$hypothesis" --arg description "V2-R2-SV1C-24H matched no-CDF strict-risk control; development-only successor" --arg date "$date" --argjson seed "$seed" \
			'.seed = $seed |
			 .experiment_id = $experiment |
			 .hypothesis_id = $hypothesis |
			 .description = $description |
			 .date = $date |
			 .status = "registered-development" |
			 .strict_risk_contract = true |
			 .auto_borrow_spot = false |
			 .cross_asset_spot_graph = true |
			 .cross_asset_collateral_marks = false |
			 del(.elastic_liquidity_suppliers) |
			 del(.record_elastic_liquidity_supplier_decisions)' "$source" >"$temporary"
	fi
	normalized_dir=$(mktemp -d)
	normalized="$normalized_dir/run-config.json"
	"$normalizer" -config "$temporary" -logdir "$normalized_dir" -write-effective-config "$normalized" >/dev/null 2>&1
	[[ -s "$normalized" ]] || { echo "config normalizer produced no effective config: $output" >&2; exit 1; }
	mv -- "$normalized" "$output"
	rmdir -- "$normalized_dir" 2>/dev/null || true
	rm -- "$temporary"
}

for seed in "${seeds[@]}"; do
	write_config "$config_dir/treatment-$seed.json" "$source_dir/dev-607.json" treatment "$seed" \
		"v2-r2-sv1c-24h-treatment-$seed" "$candidate"
	write_config "$config_dir/control-$seed.json" "$source_dir/dev-607.json" control "$seed" \
		"v2-r2-sv1c-24h-control-$seed" "$control_hypothesis"
done

# The parity no-log arm is a fresh seed-643 copy of the accepted R2 no-log
# control. It intentionally has no CDF roster because supplier decisions are
# required to be persisted in the full-log treatment.
write_config "$config_dir/control-643-none.json" "$source_dir/dev-607-none.json" control 643 \
	"v2-r2-sv1c-24h-control-643-none" "$control_hypothesis"

provenance="$root_dir/research/v2-r2-sv1c-24h-config-provenance.json"
[[ ! -e "$provenance" && ! -L "$provenance" ]] || {
	echo "refusing to overwrite config provenance manifest: $provenance" >&2
	exit 1
}
withdrawal_measurement_path="$v2_r2_sv1_withdrawal_measurement_path"
withdrawal_measurement_file="$root_dir/$withdrawal_measurement_path"
[[ -s "$withdrawal_measurement_file" && ! -L "$withdrawal_measurement_file" ]] || {
	echo "missing withdrawal measurement amendment: $withdrawal_measurement_file" >&2
	exit 1
}
source_hash=$(sha256sum "$source_dir/dev-607.json" | awk '{print $1}')
no_log_source_hash=$(sha256sum "$source_dir/dev-607-none.json" | awk '{print $1}')
roster_hash=$(sha256sum "$activation_config" | awk '{print $1}')
activation_control_hash=$(sha256sum "$activation_control_config" | awk '{print $1}')
generator_hash=$(sha256sum "$root_dir/scripts/render-v2-r2-sv1c-24h-configs.sh" | awk '{print $1}')
contract_definition_path="$v2_r2_sv1_contract_path"
contract_definition_file="$root_dir/$contract_definition_path"
[[ -s "$contract_definition_file" && ! -L "$contract_definition_file" ]] || {
	echo "missing SV1C contract definition: $contract_definition_file" >&2
	exit 1
}
contract_definition_hash=$(sha256sum "$contract_definition_file" | awk '{print $1}')
contract_loader_path="$v2_r2_sv1_contract_loader_path"
contract_loader_file="$root_dir/$contract_loader_path"
[[ -s "$contract_loader_file" && ! -L "$contract_loader_file" ]] || {
	echo "missing SV1 contract loader: $contract_loader_file" >&2
	exit 1
}
contract_loader_hash=$(sha256sum "$contract_loader_file" | awk '{print $1}')
contract_dependencies='[]'
for dependency_path in "${v2_r2_sv1_contract_dependency_paths[@]}"; do
	dependency_file="$root_dir/$dependency_path"
	[[ -s "$dependency_file" && ! -L "$dependency_file" ]] || {
		echo "missing SV1C contract dependency: $dependency_file" >&2
		exit 1
	}
	dependency_hash=$(sha256sum "$dependency_file" | awk '{print $1}')
	contract_dependencies=$(jq -cn --argjson dependencies "$contract_dependencies" --arg path "$dependency_path" --arg sha256 "$dependency_hash" \
		'$dependencies + [{path: $path, sha256: $sha256}]')
done
withdrawal_measurement_hash=$(sha256sum "$withdrawal_measurement_file" | awk '{print $1}')
preregistration_path="$v2_r2_sv1_preregistration_path"
preregistration_file="$root_dir/$preregistration_path"
[[ -s "$preregistration_file" && ! -L "$preregistration_file" ]] || {
	echo "missing SV1C preregistration: $preregistration_file" >&2
	exit 1
}
preregistration_hash=$(sha256sum "$preregistration_file" | awk '{print $1}')
registered_configs=$(for path in "$config_dir"/*.json; do printf '%s\t%s\n' "$(basename -- "$path")" "$(sha256sum -- "$path" | awk '{print $1}')"; done | jq -Rn '[inputs | split("\t") | {(.[0]): .[1]}] | add')
activation_diagnostics_path="$v2_r2_sv1_activation_diagnostics_path"
activation_diagnostics_file="$root_dir/$activation_diagnostics_path"
[[ -s "$activation_diagnostics_file" && ! -L "$activation_diagnostics_file" ]] || {
	echo "missing activation diagnostics amendment: $activation_diagnostics_file" >&2
	exit 1
}
activation_diagnostics_hash=$(sha256sum "$activation_diagnostics_file" | awk '{print $1}')
capacity_order_path="$v2_r2_sv1_capacity_order_path"
capacity_order_file="$root_dir/$capacity_order_path"
[[ -s "$capacity_order_file" && ! -L "$capacity_order_file" ]] || {
	echo "missing activation/capacity ordering amendment: $capacity_order_file" >&2
	exit 1
}
capacity_order_hash=$(sha256sum "$capacity_order_file" | awk '{print $1}')
authorized_launch_config_hashes=$(for config_name in "${v2_r2_sv1_capacity_authorized_launch_config_names[@]}"; do
	sha256sum -- "$config_dir/$config_name" | awk '{print $1}'
done | jq -Rsc 'split("\n") | map(select(length > 0))')
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
	attestation_path=$(v2_r2_capacity_attestation_path_for_config "$capacity_config" "$gomaxprocs")
	probe_cell=$(v2_r2_capacity_probe_cell_for_config "$capacity_config" "$gomaxprocs")
	capacity_case=$(jq -cn --arg config_path "$capacity_config_path" --arg measurement_config_sha256 "$capacity_measurement_sha" \
		--arg attestation_path "$attestation_path" --arg probe_cell "$probe_cell" --arg role "$capacity_role" --argjson gomaxprocs "$gomaxprocs" \
		'{config_path:$config_path,measurement_config_sha256:$measurement_config_sha256,measurement_seed:659,source_seed:643,gomaxprocs:$gomaxprocs,attestation_path:$attestation_path,probe_cell:$probe_cell,role:$role}')
	capacity_cases=$(jq -c --argjson case "$capacity_case" '. + [$case]' <<<"$capacity_cases")
done
jq -n \
	--arg candidate "$candidate" \
	--arg source_hash "$source_hash" \
	--arg no_log_source_hash "$no_log_source_hash" \
	--arg roster_path "research/configs/v2-r2-sv1c/activation-643.json" \
	--arg roster_hash "$roster_hash" \
	--arg activation_control_path "research/configs/v2-r2-sv1c/activation-643-control.json" \
	--arg activation_control_hash "$activation_control_hash" \
	--arg generator_path "scripts/render-v2-r2-sv1c-24h-configs.sh" \
	--arg generator_hash "$generator_hash" \
	--arg contract_definition_path "$contract_definition_path" \
	--arg contract_definition_hash "$contract_definition_hash" \
	--arg contract_loader_path "$contract_loader_path" \
	--arg contract_loader_hash "$contract_loader_hash" \
	--argjson contract_dependencies "$contract_dependencies" \
	--arg normalizer_path "$v2_r2_sv1_config_normalizer_path" \
	--arg normalizer_sha256 "$normalizer_sha256" \
	--arg normalizer_revision "$normalizer_revision" \
	--arg normalizer_go_version "$normalizer_go_version" \
	--arg normalizer_package "$v2_r2_sv1_config_normalizer_package" \
	--arg withdrawal_measurement_path "$withdrawal_measurement_path" \
	--arg withdrawal_measurement_hash "$withdrawal_measurement_hash" \
	--arg activation_diagnostics_path "$activation_diagnostics_path" \
	--arg activation_diagnostics_hash "$activation_diagnostics_hash" \
	--arg preregistration_path "$preregistration_path" \
	--arg preregistration_hash "$preregistration_hash" \
	--arg capacity_order_path "$capacity_order_path" \
	--arg capacity_order_hash "$capacity_order_hash" \
	--argjson registered_configs "$registered_configs" \
	--argjson capacity_cases "$capacity_cases" \
	--argjson authorized_launch_config_hashes "$authorized_launch_config_hashes" \
	'{schema_version: 1,
		 contract: "v2-r2-sv1c-24h-config-provenance-v2",
	 candidate: $candidate,
	 predecessor: "V2-R2-SV1B-24H-CDF-LIQUIDITY",
	 source_configs: {"dev-607.json": $source_hash, "dev-607-none.json": $no_log_source_hash},
	 activation_roster: {path: $roster_path, sha256: $roster_hash},
	 activation_control: {path: $activation_control_path, sha256: $activation_control_hash},
	 registered_configs: $registered_configs,
	 fresh_development_seeds: [643, 647, 653],
	 activation_seed: 643,
	 reserved_holdout_seeds: [619, 631, 641],
	 transform: {
		full_log_source: "research/configs/v2-integrated-longrun-r2/dev-607.json",
		no_log_source: "research/configs/v2-integrated-longrun-r2/dev-607-none.json",
		seed_rule: "set source seed to the first three unused odd primes greater than reserved holdout maximum 641",
		treatment: "inject the immutable SV1C activation roster and persist supplier decisions/receipts",
		control: "remove only the SV1C roster and supplier-decision recording; preserve the paired base configuration"
	 },
		 generator: {path: $generator_path, sha256: $generator_hash},
		 contract_definition: {path: $contract_definition_path, sha256: $contract_definition_hash},
		 contract_loader: {path: $contract_loader_path, sha256: $contract_loader_hash},
		 contract_dependencies: $contract_dependencies,
		 normalizer: {path: $normalizer_path, sha256: $normalizer_sha256, revision: $normalizer_revision, go_version: $normalizer_go_version, package: $normalizer_package},
	 withdrawal_measurement: {path: $withdrawal_measurement_path, sha256: $withdrawal_measurement_hash},
	 activation_diagnostics: {path: $activation_diagnostics_path, sha256: $activation_diagnostics_hash},
	 preregistration: {path: $preregistration_path, sha256: $preregistration_hash},
	 capacity_ordering: {path: $capacity_order_path, sha256: $capacity_order_hash},
		 capacity_calibration: {
					contract: "v2-r2-sv1c-24h-binary-capacity-v1",
				 mode: "production_capacity_measurement",
				 calibration_only: true,
				 cpu_limit_percent: 90,
				 cpu_affinity_policy: "first_floor(0.9*host_cpus)",
				 capacity_only_seed: 659,
			 source_config_seed: 643,
			 minimum_free_bytes: 4294967296,
			 safety_margin_bytes: 4294967296,
			 memory_limit_bytes: 21474836480,
			 gomemlimit_bytes: 19327352832,
			 measurement_config_path: "research/configs/v2-r2-sv1c-24h/treatment-643.json",
		 launch_config_path: "research/configs/v2-r2-sv1c-24h/treatment-643.json",
		 authorized_launch_config_sha256: $authorized_launch_config_hashes,
		 measurement_cases: $capacity_cases
	 },
	 holdout_policy: "development generator and checker never read or create holdout 619/631/641"
	}' >"$provenance"

echo "rendered SV1C 24-hour configs in $config_dir"
sha256sum "$config_dir"/*.json "$provenance"
