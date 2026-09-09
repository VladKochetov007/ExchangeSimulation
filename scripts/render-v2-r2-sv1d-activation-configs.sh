#!/usr/bin/env bash
# Materialize the immutable SV1D three-arm activation package. This script
# only derives configs from retained SV1C inputs; it never edits those inputs.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1d-activation-contract.sh"

[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || {
	echo "SV1D config generation requires a clean scientific worktree" >&2
	exit 1
}

require_bound_file() {
	local relative_path=$1 absolute_path="$root_dir/$1"
	[[ -f "$absolute_path" && ! -L "$absolute_path" ]] || {
		echo "SV1D provenance input is missing or symlinked: $relative_path" >&2
		exit 1
	}
}

require_bound_file "$v2_r2_sv1_preregistration_path"
require_bound_file "$v2_r2_sv1_implementation_path"
require_bound_file "$v2_r2_sv1_activation_runner_path"
require_bound_file "$v2_r2_sv1_activation_scorer_path"
require_bound_file "$v2_r2_sv1_config_checker_path"
require_bound_file "$v2_r2_sv1_config_contract_test_path"
require_bound_file "$v2_r2_sv1_terminal_outcome_path"
for dependency_path in "${v2_r2_sv1_contract_dependency_paths[@]}"; do
	require_bound_file "$dependency_path"
done

normalizer=${V2_R2_SV1D_CONFIG_NORMALIZER_BIN:-"$root_dir/bin/multivenue"}
normalizer=$(realpath -e -- "$normalizer") || { echo "missing config normalizer" >&2; exit 1; }
[[ "$normalizer" == "$root_dir/bin/multivenue" && -x "$normalizer" && ! -L "$normalizer" ]] || {
	echo "SV1D normalizer must be the registered bin/multivenue executable" >&2
	exit 1
}
normalizer_revision=$(git -C "$root_dir" rev-parse HEAD)
normalizer_sha256=$(v2_r2_sv1d_sha256_file "$normalizer")
normalizer_version=$(go version -m "$normalizer" | sed -n '1s/.*: //p')
[[ "$normalizer_version" == go1.27.0 ]] || { echo "SV1D normalizer must use Go 1.27.0" >&2; exit 1; }
v2_r2_sv1d_require_pinned_binary "$normalizer" "$normalizer_revision" "$normalizer_sha256" "$v2_r2_sv1_config_normalizer_package" || {
	echo "SV1D normalizer is not a clean pinned build of HEAD" >&2
	exit 1
}

source_treatment="$root_dir/research/configs/v2-r2-sv1c/activation-643.json"
source_control="$root_dir/research/configs/v2-r2-sv1c/activation-643-control.json"
[[ -s "$source_treatment" && -s "$source_control" && ! -L "$source_treatment" && ! -L "$source_control" ]] || {
	echo "retained SV1C activation sources are missing" >&2
	exit 1
}
jq -e '(.elastic_supplier_count == 8) and (.elastic_supplier_symbols == null) and (.elastic_liquidity_suppliers | length == 4)' "$source_treatment" >/dev/null || {
	echo "SV1C source roster is not the expected eight-plus-four population" >&2
	exit 1
}

config_dir="$v2_r2_sv1_config_dir"
mkdir -p -- "$config_dir"
for arm in "${v2_r2_sv1d_arm_names[@]}"; do
	path=$(v2_r2_sv1d_config_for_arm "$arm")
	[[ ! -e "$path" && ! -L "$path" ]] || { echo "refusing to overwrite $path" >&2; exit 1; }
done
[[ ! -e "$v2_r2_sv1_config_provenance_manifest" && ! -L "$v2_r2_sv1_config_provenance_manifest" ]] || {
	echo "refusing to overwrite SV1D config provenance" >&2
	exit 1
}

jq -e '
	all(.elastic_liquidity_suppliers[];
		.initial_base_balance % 10 == 0 and .initial_quote_balance % 10 == 0 and
		.max_position % 10 == 0 and .max_inventory % 10 == 0 and .max_loss_quote % 10 == 0)' \
	"$source_treatment" >/dev/null || {
	echo "SV1D source capital values are not exactly divisible by the registered activation scale" >&2
	exit 1
}
roster=$(jq -c '.elastic_liquidity_suppliers | map(. + {
	tick_size: 100000,
	minimum_qualifying_qty: 1000000,
	registered_minimum_executable_qty: 100000,
	quote_on_one_sided_local_book: true
} |
	.initial_base_balance = (.initial_base_balance / 10 | floor) |
	.initial_quote_balance = (.initial_quote_balance / 10 | floor) |
	.max_position = (.max_position / 10 | floor) |
	.max_inventory = (.max_inventory / 10 | floor) |
	.max_loss_quote = (.max_loss_quote / 10 | floor))' "$source_treatment")
mode_off_roster=$(jq -cn --argjson roster "$roster" '$roster | map(.quote_on_one_sided_local_book = false)')

write_normalized() {
	local raw_config=$1 output=$2
	local scratch normalized
	scratch=$(mktemp -d)
	normalized="$scratch/run-config.json"
	"$normalizer" -config "$raw_config" -logdir "$scratch" -write-effective-config "$normalized" >/dev/null 2>&1 || {
		echo "config normalizer rejected $output" >&2
		exit 1
	}
	[[ -s "$normalized" ]] || { echo "normalizer produced no config for $output" >&2; exit 1; }
	mv -- "$normalized" "$output"
	rmdir -- "$scratch" 2>/dev/null || true
}

make_raw() {
	local source=$1 output=$2 description=$3 hypothesis=$4 mode=$5
	local raw="${output}.raw"
	if [[ "$mode" == treatment ]]; then
		jq --arg description "$description" --arg hypothesis "$hypothesis" --argjson roster "$roster" \
			'.seed = 659 | .experiment_id = "v2-r2-sv1d-activation-659-treatment" |
			 .hypothesis_id = $hypothesis | .description = $description | .date = "2026-09-09" |
			 .status = "registered-development-activation-only" | .strict_risk_contract = true |
			 .auto_borrow_spot = false | .cross_asset_spot_graph = true | .cross_asset_collateral_marks = false |
			 .elastic_liquidity_suppliers = $roster | .record_elastic_liquidity_supplier_decisions = true |
			 .record_market_data_receipts = true |
			 .market_data_receipt_roles = ((.market_data_receipt_roles + ["cdf_elastic_supplier"]) | unique)' "$source" >"$raw"
	elif [[ "$mode" == mode-off ]]; then
		jq --arg description "$description" --arg hypothesis "$hypothesis" --argjson roster "$mode_off_roster" \
			'.seed = 659 | .experiment_id = "v2-r2-sv1d-activation-659-mode-off" |
			 .hypothesis_id = $hypothesis | .description = $description | .date = "2026-09-09" |
			 .status = "registered-development-activation-only" | .strict_risk_contract = true |
			 .auto_borrow_spot = false | .cross_asset_spot_graph = true | .cross_asset_collateral_marks = false |
			 .elastic_liquidity_suppliers = $roster | .record_elastic_liquidity_supplier_decisions = true |
			 .record_market_data_receipts = true |
			 .market_data_receipt_roles = ((.market_data_receipt_roles + ["cdf_elastic_supplier"]) | unique)' "$source" >"$raw"
	else
		jq --arg description "$description" --arg hypothesis "$hypothesis" \
			'.seed = 659 | .experiment_id = "v2-r2-sv1d-activation-659-no-roster" |
			 .hypothesis_id = $hypothesis | .description = $description | .date = "2026-09-09" |
			 .status = "registered-development-activation-only" | .strict_risk_contract = true |
			 .auto_borrow_spot = false | .cross_asset_spot_graph = true | .cross_asset_collateral_marks = false |
			 del(.elastic_liquidity_suppliers) | del(.record_elastic_liquidity_supplier_decisions) |
			 .record_market_data_receipts = true |
			 .market_data_receipt_roles = ((.market_data_receipt_roles - ["cdf_elastic_supplier"]) | unique)' "$source" >"$raw"
	fi
	write_normalized "$raw" "$output"
	rm -- "$raw"
}

make_raw "$source_treatment" "$v2_r2_sv1_activation_config" \
	"V2-R2-SV1D finite CDF/USD one-sided local-book treatment; development-only activation probe" \
	"$v2_r2_sv1_run_hypothesis_id" treatment
make_raw "$source_treatment" "$v2_r2_sv1_activation_control_config" \
	"V2-R2-SV1D same-roster two-sided-only mode-off control; development-only activation probe" \
	"V2-R2-SV1D-ONE-SIDED-ELASTIC-LIQUIDITY-MODE-OFF" mode-off
make_raw "$source_control" "$v2_r2_sv1_activation_no_roster_config" \
	"V2-R2-SV1D no-successor-roster control; development-only activation probe" \
	"V2-R2-SV1D-NO-ROSTER-CONTROL" no-roster

source_treatment_sha256=$(v2_r2_sv1d_sha256_file "$source_treatment")
source_control_sha256=$(v2_r2_sv1d_sha256_file "$source_control")
generator_path="scripts/render-v2-r2-sv1d-activation-configs.sh"
generator_sha256=$(v2_r2_sv1d_sha256_file "$root_dir/$generator_path")
contract_sha256=$(v2_r2_sv1d_sha256_file "$root_dir/$v2_r2_sv1_contract_path")
loader_sha256=$(v2_r2_sv1d_sha256_file "$root_dir/$v2_r2_sv1_contract_loader_path")
source_tree_sha256=$(v2_r2_sv1d_git_tree_sha256 "$normalizer_revision")

bound_files='{}'
for bound_path in \
	"$v2_r2_sv1_preregistration_path" \
	"$v2_r2_sv1_implementation_path" \
	"$v2_r2_sv1_activation_runner_path" \
	"$v2_r2_sv1_activation_scorer_path" \
	"$v2_r2_sv1_config_checker_path" \
	"$v2_r2_sv1_config_contract_test_path" \
	"$v2_r2_sv1_terminal_outcome_path"; do
	bound_sha256=$(v2_r2_sv1d_sha256_file "$root_dir/$bound_path")
	bound_files=$(jq -c --arg path "$bound_path" --arg sha256 "$bound_sha256" \
		'. + {($path): {path: $path, sha256: $sha256}}' <<<"$bound_files")
done
dependencies='{}'
for dependency_path in "${v2_r2_sv1_contract_dependency_paths[@]}"; do
	dependency_sha256=$(v2_r2_sv1d_sha256_file "$root_dir/$dependency_path")
	dependencies=$(jq -c --arg path "$dependency_path" --arg sha256 "$dependency_sha256" \
		'. + {($path): {path: $path, sha256: $sha256}}' <<<"$dependencies")
done
registered_configs='{}'
for arm in "${v2_r2_sv1d_arm_names[@]}"; do
	path=$(v2_r2_sv1d_config_for_arm "$arm")
	filename=${path##*/}
	digest=$(v2_r2_sv1d_sha256_file "$path")
	registered_configs=$(jq -c --arg name "$filename" --arg digest "$digest" '. + {($name): $digest}' <<<"$registered_configs")
done

jq -n \
	--arg contract "$v2_r2_sv1_config_provenance_contract" \
	--arg candidate "$v2_r2_sv1_candidate_id" \
	--arg candidate_contract "$v2_r2_sv1_candidate_contract_version" \
	--arg generator_path "$generator_path" --arg generator_sha256 "$generator_sha256" \
	--arg contract_path "$v2_r2_sv1_contract_path" --arg contract_sha256 "$contract_sha256" \
	--arg loader_path "$v2_r2_sv1_contract_loader_path" --arg loader_sha256 "$loader_sha256" \
	--arg source_treatment_path "research/configs/v2-r2-sv1c/activation-643.json" --arg source_treatment_sha256 "$source_treatment_sha256" \
	--arg source_control_path "research/configs/v2-r2-sv1c/activation-643-control.json" --arg source_control_sha256 "$source_control_sha256" \
	--arg normalizer_path "bin/multivenue" --arg normalizer_revision "$normalizer_revision" --arg normalizer_sha256 "$normalizer_sha256" --arg normalizer_go_version "$normalizer_version" \
	--arg normalizer_package "$v2_r2_sv1_config_normalizer_package" --argjson registered_configs "$registered_configs" \
	--arg source_tree_sha256 "$source_tree_sha256" --argjson bound_files "$bound_files" --argjson dependencies "$dependencies" \
	--arg treatment_path "research/configs/v2-r2-sv1d-activation/activation-659-treatment.json" \
	--arg mode_off_path "research/configs/v2-r2-sv1d-activation/activation-659-mode-off.json" \
	--arg no_roster_path "research/configs/v2-r2-sv1d-activation/activation-659-no-roster.json" \
	--argjson calendar "$v2_r2_sv1d_calendar" \
	'{schema_version: 1, contract: $contract, candidate: $candidate,
	 candidate_contract: $candidate_contract, seed: 659, source_revision: $normalizer_revision,
	 source_tree_sha256: $source_tree_sha256, generated_from_clean_tree: true,
	 source_configs: {
	   "research/configs/v2-r2-sv1c/activation-643.json": $source_treatment_sha256,
	   "research/configs/v2-r2-sv1c/activation-643-control.json": $source_control_sha256},
	 generator: {path: $generator_path, sha256: $generator_sha256},
	 contract_definition: {path: $contract_path, sha256: $contract_sha256},
	 contract_loader: {path: $loader_path, sha256: $loader_sha256},
	 normalizer: {path: $normalizer_path, revision: $normalizer_revision, sha256: $normalizer_sha256,
	   go_version: $normalizer_go_version, package: $normalizer_package},
	 bound_files: $bound_files, contract_dependencies: $dependencies,
	 arms: {
	   treatment: {path: $treatment_path, mode: "one_sided", cdf_roster: true},
	   "mode-off": {path: $mode_off_path, mode: "two_sided_only", cdf_roster: true},
	   "no-roster": {path: $no_roster_path, mode: "no_roster", cdf_roster: false}},
	 registered_configs: $registered_configs, expiry_calendar: $calendar,
	 activation_contract: {seed: 659, horizon: "5m", evidence_format: "evstream_v3", log_mode: "full", holdouts_consumed: false},
	 holdouts_consumed: false}' \
	>"$v2_r2_sv1_config_provenance_manifest"

echo "SV1D activation configs rendered and provenance-bound"
