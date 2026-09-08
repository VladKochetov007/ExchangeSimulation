#!/usr/bin/env bash
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
loader="$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
contract="$root_dir/scripts/v2-r2-sv1c-24h-contract.sh"
checker="$root_dir/scripts/check-v2-r2-sv1c-24h-configs.sh"

source "$loader"
default_contract=$(v2_r2_select_sv1_contract "$root_dir")
[[ "$default_contract" == "$root_dir/scripts/v2-r2-sv1-24h-contract.sh" ]] || {
	echo "SV1 loader default changed from the historical contract" >&2
	exit 1
}
if V2_R2_SV1_CONTRACT_SCRIPT="$root_dir/scripts/not-registered.sh" v2_r2_select_sv1_contract "$root_dir"; then
	echo "SV1 loader accepted an unregistered contract path" >&2
	exit 1
fi
export V2_R2_SV1_CONTRACT_SCRIPT="$contract"
selected_contract=$(v2_r2_select_sv1_contract "$root_dir")
[[ "$selected_contract" == "$contract" ]] || {
	echo "SV1 loader did not select the registered SV1C contract" >&2
	exit 1
}
source "$selected_contract"

[[ "$v2_r2_sv1_candidate_id" == "V2-R2-SV1C-24H-CDF-LIQUIDITY-STRICT-RISK" ]] || exit 1
[[ "$v2_r2_sv1_predecessor_id" == "V2-R2-SV1B-24H-CDF-LIQUIDITY" ]] || exit 1
[[ "$v2_r2_sv1_generator_path" == "scripts/render-v2-r2-sv1c-24h-configs.sh" ]] || exit 1
[[ "$v2_r2_sv1_config_dir" == "$root_dir/research/configs/v2-r2-sv1c-24h" ]] || exit 1
[[ "$v2_r2_sv1_config_provenance_manifest" == "$root_dir/research/v2-r2-sv1c-24h-config-provenance.json" ]] || exit 1
[[ "$v2_r2_sv1_activation_config" == "$root_dir/research/configs/v2-r2-sv1c/activation-643.json" ]] || exit 1
[[ "$v2_r2_sv1_config_normalizer_registration_path" == "research/v2-r2-sv1c-24h-config-normalizer-registration.json" ]] || exit 1
[[ "$v2_r2_sv1_config_normalizer_registration_contract" == "v2-r2-sv1c-24h-config-normalizer-registration-v1" ]] || exit 1
[[ "$v2_r2_sv1_activation_arm_status_contract" == "v2-r2-sv1c-activation-arm-status-v1" ]] || exit 1
[[ "$v2_r2_sv1_review_contract" == "v2-r2-sv1c-independent-review-v1" ]] || exit 1
[[ "$v2_r2_sv1_predecessor_id" != "V2-R2-SV1" ]] || exit 1
declare -F v2_r2_sv1c_require_normalizer_source_revision >/dev/null || {
	echo "SV1C normalizer source-revision helper is missing" >&2
	exit 1
}
v2_r2_sv1c_require_normalizer_source_revision "$root_dir" "$(git -C "$root_dir" rev-parse HEAD)" || {
	echo "SV1C current Go input tree was not accepted as a normalizer source" >&2
	exit 1
}
jq -e --arg contract_path "$v2_r2_sv1_contract_path" --arg loader_path "$v2_r2_sv1_contract_loader_path" '
	(.contract_definition.path == $contract_path and
	 (.contract_definition.sha256 | test("^[0-9a-f]{64}$"))) and
	(.contract_loader.path == $loader_path and
	 (.contract_loader.sha256 | test("^[0-9a-f]{64}$"))) and
	(.contract_dependencies | map(.path) == ["scripts/v2-r2-sv1-24h-contract.sh", "scripts/v2-integrated-longrun-r2-contract.sh", "scripts/v2-r2-sv1-terminal-outcome.jq"]) and
	(.contract_dependencies | all(.sha256 | test("^[0-9a-f]{64}$")))
' "$v2_r2_sv1_config_provenance_manifest" >/dev/null || {
	echo "SV1C provenance does not bind its complete contract graph" >&2
	exit 1
}
normalizer_registration_file="$root_dir/$v2_r2_sv1_config_normalizer_registration_path"
v2_r2_sv1c_require_normalizer_registration "$root_dir" "$normalizer_registration_file" || {
	echo "SV1C normalizer registration is invalid" >&2
	exit 1
}
(
	registration_backup=$(mktemp)
	registration_mutation=$(mktemp)
	trap 'cp -- "$registration_backup" "$normalizer_registration_file"; rm -f -- "$registration_backup" "$registration_mutation"' EXIT
	cp -- "$normalizer_registration_file" "$registration_backup"
	for mutation in '.sha256 = ("0" * 64)' '.build.compiler = "tampered"'; do
		jq "$mutation" "$registration_backup" >"$registration_mutation"
		cp -- "$registration_mutation" "$normalizer_registration_file"
		if v2_r2_sv1c_require_normalizer_registration "$root_dir" "$normalizer_registration_file"; then
			echo "SV1C accepted a mutable normalizer registration: $mutation" >&2
			exit 1
		fi
		cp -- "$registration_backup" "$normalizer_registration_file"
	done
)
jq -e --arg path "$v2_r2_sv1_config_normalizer_registration_path" \
	--arg sha256 "$(sha256sum -- "$normalizer_registration_file" | awk '{print $1}')" \
	'.normalizer_registration.path == $path and .normalizer_registration.sha256 == $sha256' \
	"$v2_r2_sv1_config_provenance_manifest" >/dev/null || {
	echo "SV1C provenance does not bind its normalizer registration" >&2
	exit 1
}

registered_candidate="$v2_r2_sv1_candidate_id"
for unknown_candidate in "" "V2-R2-SV1C-UNREGISTERED" "V2-R2-SV1B-24H-CDF-LIQUIDITY-UNREGISTERED"; do
	v2_r2_sv1_candidate_id="$unknown_candidate"
	if v2_r2_require_known_candidate; then
		echo "SV1C accepted unknown candidate identity: $unknown_candidate" >&2
		exit 1
	fi
done
v2_r2_sv1_candidate_id="$registered_candidate"

"$checker"

manifest_backup=$(mktemp)
manifest_mutation_fixture=$(mktemp)
cp -- "$v2_r2_sv1_config_provenance_manifest" "$manifest_backup"
restore_manifest() {
	cp -- "$manifest_backup" "$v2_r2_sv1_config_provenance_manifest"
	rm -f -- "$manifest_backup" "$manifest_mutation_fixture"
}
trap restore_manifest EXIT
for mutation in '.contract_dependencies[2].sha256 = ("0" * 64)' '.normalizer.go_version = "tampered"' '.normalizer.package = "tampered"' '.normalizer_registration.sha256 = ("0" * 64)'; do
	jq "$mutation" "$manifest_backup" >"$manifest_mutation_fixture"
	cp -- "$manifest_mutation_fixture" "$v2_r2_sv1_config_provenance_manifest"
	if V2_R2_SV1_CONTRACT_SCRIPT="$contract" "$checker"; then
		echo "SV1C accepted mutated normalizer provenance: $mutation" >&2
		exit 1
	fi
	cp -- "$manifest_backup" "$v2_r2_sv1_config_provenance_manifest"
done
rm -f -- "$manifest_backup" "$manifest_mutation_fixture"
trap - EXIT

strict_config_valid() {
	jq -e '
		.strict_risk_contract == true and
		.auto_borrow_spot == false and
		.cross_asset_spot_graph == true and
		.cross_asset_collateral_marks == false and
		(.perp_exposure_hedger == null or .perp_exposure_hedger.auto_borrow_perp != true)
	' "$1" >/dev/null
}

for config_path in "$v2_r2_sv1_activation_config" "$v2_r2_sv1_activation_control_config" "$v2_r2_sv1_config_dir"/*.json; do
	strict_config_valid "$config_path" || {
		echo "registered SV1C config is not strict-risk explicit: $config_path" >&2
		exit 1
	}
done

fixture=$(mktemp)
trap 'rm -f -- "$fixture"' EXIT
for mutation in \
	'.strict_risk_contract = false' \
	'del(.strict_risk_contract)' \
	'.auto_borrow_spot = true' \
	'.auto_borrow_spot = null' \
	'.cross_asset_spot_graph = false' \
	'.cross_asset_collateral_marks = true' \
	'.perp_exposure_hedger = {auto_borrow_perp: true}'; do
	jq "$mutation" "$v2_r2_sv1_activation_config" >"$fixture"
	if strict_config_valid "$fixture"; then
		echo "SV1C strict-risk mutation was accepted: $mutation" >&2
		exit 1
	fi
done

for helper in \
	v2_r2_sv1b_require_pinned_binary \
	v2_r2_sv1b_require_authorized_capacity_config \
	v2_r2_sv1b_require_checkpoint_validator_attestation_binding \
	v2_r2_require_sv1b_review_attestation \
	v2_r2_require_sv1b_activation_provenance \
	v2_r2_sv1b_require_invalid_audit_pair_provenance; do
	declare -F "$helper" >/dev/null || {
		echo "SV1C shared-runner adapter is missing: $helper" >&2
		exit 1
	}
done

echo "SV1C contract: loader, namespace, strict-risk mutations, and config checker passed"
