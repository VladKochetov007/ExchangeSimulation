#!/usr/bin/env bash
# Validate the immutable SV1D activation configs without opening any world.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1d-activation-contract.sh"

fail() { echo "SV1D activation config failure: $*" >&2; exit 1; }
require_file() { [[ -s "$1" && ! -L "$1" ]] || fail "missing or symlinked file: $1"; }
hash_file() { v2_r2_sv1d_sha256_file "$1" || fail "could not hash: $1"; }
require_json_object() { v2_r2_require_single_json_object "$1" || fail "not a single JSON object: $1"; }

[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] ||
	fail "SV1D activation configs require a clean scientific worktree"
current_revision=$(git -C "$root_dir" rev-parse HEAD) || fail "cannot read current revision"
[[ "$current_revision" =~ ^[0-9a-f]{40}$ ]] || fail "invalid current revision"

require_file "$v2_r2_sv1_config_provenance_manifest"
require_json_object "$v2_r2_sv1_config_provenance_manifest"
for arm in "${v2_r2_sv1d_arm_names[@]}"; do
	require_file "$(v2_r2_sv1d_config_for_arm "$arm")"
	require_json_object "$(v2_r2_sv1d_config_for_arm "$arm")"
done
require_file "$root_dir/$v2_r2_sv1_generator_path"
require_file "$root_dir/$v2_r2_sv1_contract_path"
require_file "$root_dir/$v2_r2_sv1_contract_loader_path"
require_file "$root_dir/research/configs/v2-r2-sv1c/activation-643.json"
require_file "$root_dir/research/configs/v2-r2-sv1c/activation-643-control.json"
require_json_object "$root_dir/research/configs/v2-r2-sv1c/activation-643.json"
require_json_object "$root_dir/research/configs/v2-r2-sv1c/activation-643-control.json"
source_treatment="$root_dir/research/configs/v2-r2-sv1c/activation-643.json"
source_control="$root_dir/research/configs/v2-r2-sv1c/activation-643-control.json"

manifest_source_revision=$(jq -er '.source_revision | select(type == "string" and test("^[0-9a-f]{40}$"))' "$v2_r2_sv1_config_provenance_manifest") ||
	fail "manifest source revision is missing"
manifest_source_tree_sha256=$(jq -er '.source_tree_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$v2_r2_sv1_config_provenance_manifest") ||
	fail "manifest source tree hash is missing"
[[ "$manifest_source_revision" != "$current_revision" ]] || fail "generated configs were not committed after their clean source revision"
git -C "$root_dir" merge-base --is-ancestor "$manifest_source_revision" "$current_revision" ||
	fail "manifest source revision is not an ancestor of the candidate"
[[ "$(v2_r2_sv1d_git_tree_sha256 "$manifest_source_revision")" == "$manifest_source_tree_sha256" ]] ||
	fail "manifest source tree hash does not match Git"
jq -e --arg contract "$v2_r2_sv1_config_provenance_contract" --arg candidate "$v2_r2_sv1_candidate_id" \
	--arg candidate_contract "$v2_r2_sv1_candidate_contract_version" --arg source_revision "$manifest_source_revision" \
	--argjson calendar "$v2_r2_sv1d_calendar" '
		type == "object" and .schema_version == 1 and .contract == $contract and .candidate == $candidate and
		.candidate_contract == $candidate_contract and .source_revision == $source_revision and .seed == 659 and
		.holdouts_consumed == false and .expiry_calendar == $calendar and
		(.source_configs | keys | sort) == ["research/configs/v2-r2-sv1c/activation-643-control.json", "research/configs/v2-r2-sv1c/activation-643.json"] and
		(.registered_configs | keys | sort) == ["activation-659-mode-off.json", "activation-659-no-roster.json", "activation-659-treatment.json"] and
			(.arms | keys | sort) == ["mode-off", "no-roster", "treatment"] and
			.generated_from_clean_tree == true and
			.activation_contract == {seed: 659, horizon: "5m", evidence_format: "evstream_v3", log_mode: "full", holdouts_consumed: false} and
			.arms.treatment == {path: "research/configs/v2-r2-sv1d-activation/activation-659-treatment.json", mode: "one_sided", cdf_roster: true} and
			.arms["mode-off"] == {path: "research/configs/v2-r2-sv1d-activation/activation-659-mode-off.json", mode: "two_sided_only", cdf_roster: true} and
			.arms["no-roster"] == {path: "research/configs/v2-r2-sv1d-activation/activation-659-no-roster.json", mode: "no_roster", cdf_roster: false}' "$v2_r2_sv1_config_provenance_manifest" >/dev/null || fail "invalid top-level provenance"

expected_bound_files='["research/v2-r2-sv1d-implementation-2026-09-09.md", "research/v2-r2-sv1d-one-sided-elastic-successor-preregistration-2026-09-09.md", "scripts/check-v2-r2-sv1d-activation-configs.sh", "scripts/run-v2-r2-sv1d-activation-probe.sh", "scripts/score-v2-r2-sv1d-activation.sh", "scripts/test-v2-r2-sv1d-activation-config-contract.sh", "scripts/v2-r2-sv1-terminal-outcome.jq"]'
expected_dependencies='["scripts/v2-integrated-longrun-r2-contract.sh", "scripts/v2-r2-sv1-24h-contract.sh", "scripts/v2-r2-sv1-terminal-outcome.jq", "scripts/v2-r2-sv1-activation-status.sh"]'
jq -e --argjson bound_files "$expected_bound_files" --argjson dependencies "$expected_dependencies" '
			(.bound_files | keys | sort) == ($bound_files | sort) and
			(.contract_dependencies | keys | sort) == ($dependencies | sort)' "$v2_r2_sv1_config_provenance_manifest" >/dev/null || fail "incomplete contract binding"
expected_changed_paths=$(printf '%s\n' \
	research/configs/v2-r2-sv1d-activation/activation-659-mode-off.json \
	research/configs/v2-r2-sv1d-activation/activation-659-no-roster.json \
	research/configs/v2-r2-sv1d-activation/activation-659-treatment.json \
	research/v2-r2-sv1d-activation-config-provenance.json | LC_ALL=C sort)
actual_changed_paths=$(git -C "$root_dir" diff --name-only "$manifest_source_revision" "$current_revision" | LC_ALL=C sort)
[[ "$actual_changed_paths" == "$expected_changed_paths" ]] || fail "candidate contains changes beyond generated activation artifacts"

for entry in \
	"generator|$v2_r2_sv1_generator_path|$root_dir/$v2_r2_sv1_generator_path" \
	"contract_definition|$v2_r2_sv1_contract_path|$root_dir/$v2_r2_sv1_contract_path" \
	"contract_loader|$v2_r2_sv1_contract_loader_path|$root_dir/$v2_r2_sv1_contract_loader_path"; do
	IFS='|' read -r key expected_path absolute_path <<<"$entry"
	actual_path=$(jq -er --arg key "$key" '.[$key].path' "$v2_r2_sv1_config_provenance_manifest") || fail "$key path missing"
	actual_hash=$(hash_file "$absolute_path")
	expected_hash=$(jq -er --arg key "$key" '.[$key].sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$v2_r2_sv1_config_provenance_manifest") || fail "$key hash missing"
	[[ "$actual_path" == "$expected_path" && "$actual_hash" == "$expected_hash" ]] || fail "$key provenance mismatch"
done

while IFS= read -r bound_path; do
	bound_file="$root_dir/$bound_path"
	require_file "$bound_file"
	actual_hash=$(hash_file "$bound_file")
	expected_hash=$(jq -er --arg path "$bound_path" '.bound_files[$path].sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$v2_r2_sv1_config_provenance_manifest") || fail "bound file hash missing: $bound_path"
	[[ "$actual_hash" == "$expected_hash" ]] || fail "bound file changed: $bound_path"
done < <(jq -er '.bound_files | keys[]' "$v2_r2_sv1_config_provenance_manifest")
while IFS= read -r dependency_path; do
	dependency_file="$root_dir/$dependency_path"
	require_file "$dependency_file"
	actual_hash=$(hash_file "$dependency_file")
	expected_hash=$(jq -er --arg path "$dependency_path" '.contract_dependencies[$path].sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$v2_r2_sv1_config_provenance_manifest") || fail "dependency hash missing: $dependency_path"
	[[ "$actual_hash" == "$expected_hash" ]] || fail "contract dependency changed: $dependency_path"
done < <(jq -er '.contract_dependencies | keys[]' "$v2_r2_sv1_config_provenance_manifest")

normalizer_revision=$(jq -er '.normalizer.revision | select(type == "string" and test("^[0-9a-f]{40}$"))' "$v2_r2_sv1_config_provenance_manifest") || fail "normalizer revision missing"
normalizer_sha=$(jq -er '.normalizer.sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$v2_r2_sv1_config_provenance_manifest") || fail "normalizer hash missing"
normalizer_path=$(jq -er '.normalizer.path | select(type == "string")' "$v2_r2_sv1_config_provenance_manifest") || fail "normalizer path missing"
normalizer_package=$(jq -er '.normalizer.package | select(type == "string")' "$v2_r2_sv1_config_provenance_manifest") || fail "normalizer package missing"
normalizer_go_version=$(jq -er '.normalizer.go_version | select(type == "string")' "$v2_r2_sv1_config_provenance_manifest") || fail "normalizer Go version missing"
[[ "$normalizer_path" == "bin/multivenue" && "$normalizer_package" == "$v2_r2_sv1_config_normalizer_package" &&
	"$normalizer_go_version" == "go1.27.0" && "$normalizer_revision" == "$manifest_source_revision" ]] ||
	fail "normalizer provenance identity is inconsistent"

# Config generation records the exact clean parent used by the generator. The
# final candidate necessarily adds the committed generated configs, so the
# current binary is checked against the final candidate and then used for a
# byte-for-byte idempotence pass over every registered config.
normalizer="$root_dir/bin/multivenue"
require_file "$normalizer"
current_normalizer_sha=$(hash_file "$normalizer")
v2_r2_sv1d_require_pinned_binary "$normalizer" "$current_revision" "$current_normalizer_sha" "$v2_r2_sv1_config_normalizer_package" ||
	fail "current normalizer is not the pinned Go 1.27 candidate build"
for arm in "${v2_r2_sv1d_arm_names[@]}"; do
	config=$(v2_r2_sv1d_config_for_arm "$arm")
	normalizer_scratch=$(mktemp -d /tmp/v2-r2-sv1d-normalizer.XXXXXX)
	mkdir -- "$normalizer_scratch/logs"
	"$normalizer" -config "$config" -logdir "$normalizer_scratch/logs" \
		-write-effective-config "$normalizer_scratch/effective.json" >/dev/null 2>&1 ||
		fail "current normalizer rejected registered $arm config"
	cmp -s "$config" "$normalizer_scratch/effective.json" || fail "normalizer is not idempotent for $arm"
	rmdir "$normalizer_scratch/logs" "$normalizer_scratch" 2>/dev/null || true
done

for source_path in research/configs/v2-r2-sv1c/activation-643.json research/configs/v2-r2-sv1c/activation-643-control.json; do
	actual=$(hash_file "$root_dir/$source_path")
	expected=$(jq -er --arg path "$source_path" '.source_configs[$path] | select(type == "string" and test("^[0-9a-f]{64}$"))' "$v2_r2_sv1_config_provenance_manifest") || fail "source hash missing: $source_path"
	[[ "$actual" == "$expected" ]] || fail "source changed: $source_path"
done

for arm in "${v2_r2_sv1d_arm_names[@]}"; do
	config=$(v2_r2_sv1d_config_for_arm "$arm")
	filename=${config##*/}
	expected=$(jq -er --arg filename "$filename" '.registered_configs[$filename] | select(type == "string" and test("^[0-9a-f]{64}$"))' "$v2_r2_sv1_config_provenance_manifest") || fail "registered hash missing: $filename"
	[[ "$(hash_file "$config")" == "$expected" ]] || fail "registered config changed: $filename"
	case "$arm" in
		treatment|mode-off) expected_roles='["cdf_elastic_supplier", "liability_hedger"]' ;;
		no-roster) expected_roles='["liability_hedger"]' ;;
		*) fail "unknown SV1D arm: $arm" ;;
	esac
	cdf_latency='{"model":"constant","delay":10000000,"min":0,"max":0,"std_dev":0,"sigma":0,"cap":0,"spike_delay":0,"spike_probability":0,"response_scale":0,"market_data_scale":0}'
	liability_latency='{"model":"constant","delay":20000000,"min":0,"max":0,"std_dev":0,"sigma":0,"cap":0,"spike_delay":0,"spike_probability":0,"response_scale":0,"market_data_scale":2}'
	jq -e --argjson calendar "$v2_r2_sv1d_calendar" --argjson expected_roles "$expected_roles" \
		--argjson cdf_latency "$cdf_latency" --argjson liability_latency "$liability_latency" '
		.seed == 659 and .log_mode == "full" and .evidence_format == "evstream_v3" and
		.step == 1000000000 and .snapshot_interval == 1000000000 and .automation_interval == 1000000000 and
		.quote_interval == 1000000000 and .noise_interval == 2000000000 and .greek_interval == 60000000000 and
		.checkpoint_interval_seconds == 60 and .market_data_receipt_roles == $expected_roles and
		.record_market_data_receipts == true and
		.strict_risk_contract == true and .auto_borrow_spot == false and .cross_asset_spot_graph == true and
		.cross_asset_collateral_marks == false and .strict_population_accounting == true and
		.venue_ids == ["north", "central", "south"] and .r2_expiry_calendar.schedules == $calendar and
		.elastic_supplier_count == 8 and .elastic_supplier_symbols == null and
		.latency_profiles.cdf_elastic_supplier == $cdf_latency and
		.latency_profiles.liability_hedger == $liability_latency' "$config" >/dev/null || fail "common config contract failed: $arm"
done

treatment=$(v2_r2_sv1d_config_for_arm treatment)
mode_off=$(v2_r2_sv1d_config_for_arm mode-off)
no_roster=$(v2_r2_sv1d_config_for_arm no-roster)
jq -e '
	(.elastic_liquidity_suppliers | type == "array" and length == 4) and
	(.elastic_liquidity_suppliers | map(.role)) == ["cdf_elastic_supplier_1", "cdf_elastic_supplier_2", "cdf_elastic_supplier_3", "cdf_elastic_supplier_4"] and
	(.elastic_liquidity_suppliers | map(.interval)) == [2000000000, 2000000000, 2000000000, 2000000000] and
	(.elastic_liquidity_suppliers | map(.decision_phase_offset)) == [0, 500000000, 1000000000, 1500000000] and
	all(.elastic_liquidity_suppliers[]; .max_observation_age == 60000000000 and .base_precision == 100000000 and .quote_precision == 100000 and .maker_fee_bps == 5) and
	all(.elastic_liquidity_suppliers[]; .symbol == "CDF/USD" and .base_asset == "CDF" and .quote_asset == "USD" and
		.tick_size == 100000 and .minimum_executable_qty == 100000 and .registered_minimum_executable_qty == 100000 and
		.minimum_qualifying_qty == 1000000 and .max_loss_quote > 0 and .max_quote_qty >= .minimum_qualifying_qty and
		.initial_base_balance > 0 and .initial_quote_balance > 0 and .max_position > 0 and .max_inventory >= .initial_base_balance and
		.quote_on_one_sided_local_book == true) and
	(.market_data_receipt_roles | index("cdf_elastic_supplier") != null) and
	.record_elastic_liquidity_supplier_decisions == true' "$treatment" >/dev/null || fail "treatment contract failed"
jq -e --slurpfile source "$source_treatment" '
	(.elastic_liquidity_suppliers | map(del(.tick_size, .minimum_qualifying_qty, .registered_minimum_executable_qty, .quote_on_one_sided_local_book, .decision_phase_offset))) ==
	($source[0].elastic_liquidity_suppliers | map(del(.decision_phase_offset)))' "$treatment" >/dev/null || fail "treatment changed retained CDF economics"
jq -e --slurpfile treatment "$treatment" '
	(.elastic_liquidity_suppliers | type == "array" and length == 4) and
	(.elastic_liquidity_suppliers | map(del(.quote_on_one_sided_local_book))) == ($treatment[0].elastic_liquidity_suppliers | map(del(.quote_on_one_sided_local_book))) and
	(.elastic_liquidity_suppliers | map(.role)) == ["cdf_elastic_supplier_1", "cdf_elastic_supplier_2", "cdf_elastic_supplier_3", "cdf_elastic_supplier_4"] and
	(.elastic_liquidity_suppliers | map(.interval)) == [2000000000, 2000000000, 2000000000, 2000000000] and
	(.elastic_liquidity_suppliers | map(.decision_phase_offset)) == [0, 500000000, 1000000000, 1500000000] and
	all(.elastic_liquidity_suppliers[]; .minimum_executable_qty == 100000 and .registered_minimum_executable_qty == 100000 and .minimum_qualifying_qty == 1000000 and (.quote_on_one_sided_local_book // false) == false) and
	(.market_data_receipt_roles | index("cdf_elastic_supplier") != null) and .record_elastic_liquidity_supplier_decisions == true' "$mode_off" >/dev/null || fail "mode-off roster contract failed"
jq -e '
	(.elastic_liquidity_suppliers == null or .elastic_liquidity_suppliers == []) and
	(.record_elastic_liquidity_supplier_decisions == null or .record_elastic_liquidity_supplier_decisions == false) and
	(.market_data_receipt_roles == ["liability_hedger"]) and
	(.elastic_supplier_count == 8) and (.elastic_supplier_symbols == null)' "$no_roster" >/dev/null || fail "no-roster contract failed"

treatment_mode_filter='del(.seed,.experiment_id,.hypothesis_id,.description,.date,.status,.elastic_liquidity_suppliers,.record_elastic_liquidity_supplier_decisions)'
[[ "$(jq -S "$treatment_mode_filter" "$treatment")" == "$(jq -S "$treatment_mode_filter" "$mode_off")" ]] || fail "treatment/mode-off economic baseline drift"
# The no-roster control deliberately removes the CDF participant receipt role
# along with the participant roster. Keep that topology difference explicit;
# it must not be mistaken for an economic parameter change in the paired
# treatment/mode-off comparison.
mode_no_roster_filter='del(.seed,.experiment_id,.hypothesis_id,.description,.date,.status,.elastic_liquidity_suppliers,.record_elastic_liquidity_supplier_decisions,.market_data_receipt_roles)'
[[ "$(jq -S "$mode_no_roster_filter" "$mode_off")" == "$(jq -S "$mode_no_roster_filter" "$no_roster")" ]] || fail "mode-off/no-roster economic baseline drift"

echo "SV1D activation configs: valid immutable tri-arm provenance and economics"
