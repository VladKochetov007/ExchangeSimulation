#!/usr/bin/env bash
# Fail-closed extraction for one completed integrated V2 long-run development
# cell. This script adds derived evidence but has no prune authority.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	printf 'usage: %s CELL_DIR\n' "$0" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-integrated-longrun-r5-contract.sh"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
contract_version="v2-integrated-longrun-candidate-v5"
conservation_tolerance_fixed_units=1000

fail() {
	printf 'integrated long-run extraction failure: %s\n' "$*" >&2
	exit 1
}
require_file() {
	local path=$1
	[[ -s "$path" ]] || fail "missing required file: $path"
}
require_json_object() {
	local path=$1
	jq -e 'type == "object"' "$path" >/dev/null || fail "malformed JSON object: $path"
}

v2_r5_require_output_root "$v2_r5_output_root" || fail "r5 output root is not canonical"
v2_r5_require_cell_path "$1" || fail "cell is outside the canonical r5 evidence root or is symlinked: $1"
cell=$(realpath -e -- "$1")
cell_name=$(basename "$cell")
analysis_dir=${V2_ANALYSIS_OUTPUT_DIR:-$cell}
if [[ "$analysis_dir" != "$cell" ]]; then
	[[ -d "$analysis_dir" && ! -L "$analysis_dir" ]] || fail "analysis output directory must be an existing non-symlink directory: $analysis_dir"
	analysis_dir=$(realpath -e -- "$analysis_dir")
	[[ "$analysis_dir" != "$cell" ]] || fail "analysis output directory must be separate from raw evidence cell"
fi
[[ -x "$analyzer" ]] || fail "missing analyzer: $analyzer"
analyzer_go_version=$(v2_r5_binary_go_version "$analyzer")
v2_r5_is_go_127 "$analyzer_go_version" || fail "analyzer is not built with the pinned Go 1.27 toolchain: $analyzer_go_version"
case "$cell_name" in
	dev-607|dev-613|dev-617) ;;
	*) fail "extractor accepts only registered full development cells, got $cell_name" ;;
esac
for sentinel in greeks.json latency.json; do
	require_file "$cell/$sentinel"
	require_json_object "$cell/$sentinel"
done
for input in manifest.json evidence-artifact-hash.json evidence-manifest.json run-config.json run-metadata.json run-status.json; do
	require_file "$cell/$input"
	require_json_object "$cell/$input"
done

expected_config="$root_dir/research/configs/v2-integrated-longrun/$cell_name.json"
require_file "$expected_config"
cmp -s "$expected_config" "$cell/run-config.json" || fail "run config is not byte-identical to registered $cell_name"

seed=$(jq -er '.seed' "$cell/run-metadata.json")
config_seed=$(jq -er '.seed' "$cell/run-config.json")
config_hypothesis=$(jq -er '.hypothesis_id' "$cell/run-config.json")
config_experiment=$(jq -er '.experiment_id' "$cell/run-config.json")
log_mode=$(jq -er '.log_mode' "$cell/run-config.json")
delivery_fee_policy=$(jq -er '.dated_future_delivery_fee_policy' "$cell/run-config.json")
simulation_start_nano=$(jq -er '.simulation_start_nano' "$cell/run-metadata.json")
simulation_end_nano=$(jq -er '.simulation_end_nano' "$cell/run-metadata.json")
[[ "$seed" == "$config_seed" ]] || fail "metadata/config seed mismatch"
[[ "$log_mode" == full ]] || fail "development extraction requires full log mode"
jq -e --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
	'($simulation_start_nano == 1735689600000000000 and $simulation_end_nano == 1735776000000000000 and
	 ($simulation_end_nano - $simulation_start_nano) == 86400000000000)' \
	<<<'null' >/dev/null || fail "run metadata does not use the registered 24-hour horizon"
jq -e --arg cell "$cell_name" --argjson seed "$seed" --arg experiment "$config_experiment" \
	'.schema_version == 5 and .runner_contract == "v2-integrated-longrun-runner-v5" and
	 .cell == $cell and .seed == $seed and .holdout == false and
	 .simulated_horizon == "24h" and .log_mode == "full" and
	 .simulation_start_nano == 1735689600000000000 and .simulation_end_nano == 1735776000000000000 and
	 (.gomaxprocs | type) == "number" and .gomaxprocs == 4 and
	 .hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE" and
	 .config_experiment_id == $experiment and
	 (.config_sha256 | test("^[0-9a-f]{64}$")) and
	 (.binary_sha256 | test("^[0-9a-f]{64}$")) and
	 .binary_trimpath == true and .binary_cgo_enabled == "0" and
	 (.git_revision | test("^[0-9a-f]{40}$")) and
	 .completion_sentinels == ["greeks.json", "latency.json"]' \
	"$cell/run-metadata.json" >/dev/null || fail "invalid run metadata contract"
jq -e --arg cell "$cell_name" \
	'.cell == $cell and .exit_status == 0 and .completion_verified == true and
	 .simulated_horizon == "24h" and .completion_sentinels == ["greeks.json", "latency.json"] and
	 (.run_metadata_sha256 | test("^[0-9a-f]{64}$")) and
	 (.manifest_sha256 | test("^[0-9a-f]{64}$")) and
	 (.greeks_sha256 | test("^[0-9a-f]{64}$")) and
	(.latency_sha256 | test("^[0-9a-f]{64}$")) and
	(.checkpoints_sha256 | test("^[0-9a-f]{64}$")) and
	(.simulation_start_nano | type) == "number" and (.simulation_end_nano | type) == "number" and
	.simulation_start_nano == 1735689600000000000 and .simulation_end_nano == 1735776000000000000 and
	(.evidence_manifest_sha256 | test("^[0-9a-f]{64}$"))' \
	"$cell/run-status.json" >/dev/null || fail "invalid run status contract"

head_revision=$(git -C "$root_dir" rev-parse HEAD)
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "source worktree is dirty"
metadata_revision=$(jq -er '.git_revision' "$cell/run-metadata.json")
# The simulator and analyzer are one frozen measurement implementation. An
# analyzer-only change after a development cell ran would make the derived
# result post hoc, even if its output is internally self-consistent.
[[ "$metadata_revision" == "$head_revision" ]] || fail "run source revision is not the current analysis revision"

simulator_binary=$(jq -er '.binary_path' "$cell/run-metadata.json")
require_file "$simulator_binary"
[[ -x "$simulator_binary" ]] || fail "recorded simulator binary is not executable"
simulator_sha256=$(sha256sum "$simulator_binary" | awk '{print $1}')
[[ "$simulator_sha256" == "$(jq -er '.binary_sha256' "$cell/run-metadata.json")" ]] || fail "simulator binary hash changed"
binary_revision=$(go version -m "$simulator_binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
binary_modified=$(go version -m "$simulator_binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
binary_trimpath=$(go version -m "$simulator_binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
binary_cgo_enabled=$(go version -m "$simulator_binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
simulator_go_version=$(v2_r5_binary_go_version "$simulator_binary")
v2_r5_is_go_127 "$simulator_go_version" || fail "simulator is not built with the pinned Go 1.27 toolchain: $simulator_go_version"
declared_simulator_go_version=$(jq -er '.binary_go_version' "$cell/run-metadata.json")
[[ "$binary_revision" == "$metadata_revision" && "$binary_modified" == false && "$binary_trimpath" == true && "$binary_cgo_enabled" == 0 && "$declared_simulator_go_version" == "$simulator_go_version" ]] || fail "simulator binary provenance is not clean/current/reproducible"
prunegate_binary=$(jq -er '.prunegate_path' "$cell/run-metadata.json")
require_file "$prunegate_binary"
[[ -x "$prunegate_binary" ]] || fail "recorded prunegate binary is not executable"
prunegate_sha256=$(sha256sum "$prunegate_binary" | awk '{print $1}')
prunegate_revision=$(go version -m "$prunegate_binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
prunegate_modified=$(go version -m "$prunegate_binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
prunegate_trimpath=$(go version -m "$prunegate_binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
prunegate_cgo_enabled=$(go version -m "$prunegate_binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
prunegate_go_version=$(v2_r5_binary_go_version "$prunegate_binary")
v2_r5_is_go_127 "$prunegate_go_version" || fail "prunegate is not built with the pinned Go 1.27 toolchain: $prunegate_go_version"
[[ "$prunegate_sha256" == "$(jq -er '.prunegate_sha256' "$cell/run-metadata.json")" &&
	"$prunegate_revision" == "$metadata_revision" && "$prunegate_modified" == false &&
	"$prunegate_trimpath" == true && "$prunegate_cgo_enabled" == 0 ]] || fail "prunegate provenance is not clean/current/reproducible"

config_sha256=$(sha256sum "$cell/run-config.json" | awk '{print $1}')
[[ "$config_sha256" == "$(jq -er '.config_sha256' "$cell/run-metadata.json")" ]] || fail "run config hash changed"
manifest_revision=$(jq -er '.build.revision' "$cell/manifest.json")
# jq -e returns failure for a valid false boolean; this is a provenance value,
# not a predicate whose false result should abort before the explicit check.
manifest_modified=$(jq -r '.build.modified' "$cell/manifest.json")
jq -e --arg revision "$metadata_revision" --argjson seed "$seed" --arg log_mode "$log_mode" \
	--arg experiment "$config_experiment" \
	'type == "object" and .schema_version == 2 and .build.revision == $revision and
	 .build.modified == false and .config.seed == $seed and .config.log_mode == $log_mode and
	 .config.experiment_id == $experiment' "$cell/manifest.json" >/dev/null || fail "manifest provenance/config mismatch"
[[ "$manifest_revision" == "$metadata_revision" && "$manifest_modified" == false ]] || fail "manifest build identity mismatch"
[[ "$(sha256sum "$cell/run-metadata.json" | awk '{print $1}')" == "$(jq -er '.run_metadata_sha256' "$cell/run-status.json")" ]] || fail "run metadata status hash mismatch"
[[ "$(sha256sum "$cell/manifest.json" | awk '{print $1}')" == "$(jq -er '.manifest_sha256' "$cell/run-status.json")" ]] || fail "manifest status hash mismatch"
[[ "$(sha256sum "$cell/greeks.json" | awk '{print $1}')" == "$(jq -er '.greeks_sha256' "$cell/run-status.json")" ]] || fail "greeks status hash mismatch"
[[ "$(sha256sum "$cell/latency.json" | awk '{print $1}')" == "$(jq -er '.latency_sha256' "$cell/run-status.json")" ]] || fail "latency status hash mismatch"
[[ "$(sha256sum "$cell/checkpoints.jsonl" | awk '{print $1}')" == "$(jq -er '.checkpoints_sha256' "$cell/run-status.json")" ]] || fail "checkpoint status hash mismatch"
[[ "$(sha256sum "$cell/evidence-manifest.json" | awk '{print $1}')" == "$(jq -er '.evidence_manifest_sha256' "$cell/run-status.json")" ]] || fail "evidence manifest status hash mismatch"
v2_r5_verify_evidence_manifest "$cell" || fail "runner evidence manifest does not match retained raw evidence: $cell"
v2_r5_verify_attestation "$cell" || fail "external evidence attestation is missing or stale: $cell"

jq -e --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
	'(.initial_accounts | type == "array" and length > 0 and
	 all(.[]; .account.timestamp == $simulation_start_nano)) and
	(.terminal_accounts | type == "array" and length > 0 and
	 all(.[]; .account.timestamp == $simulation_end_nano))' \
	"$cell/greeks.json" >/dev/null || fail "greeks report does not attest the registered 24-hour horizon"
jq -e -s --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
	'. as $checkpoints |
	 ($checkpoints | length) > 0 and
	 all($checkpoints[]; .domain == "execution_observations" and .ordering == "ordered_stream" and
		(.sim_time | type) == "number" and (.event_count | type) == "number" and
		.sim_time >= $simulation_start_nano and .sim_time <= $simulation_end_nano and .event_count >= 0) and
	 all(range(1; ($checkpoints | length)); $checkpoints[. - 1].sim_time < $checkpoints[.].sim_time and
		$checkpoints[. - 1].event_count < $checkpoints[.].event_count) and
	 $checkpoints[-1].sim_time == $simulation_end_nano' \
	"$cell/checkpoints.jsonl" >/dev/null || fail "checkpoint stream does not attest the registered 24-hour horizon"

for json_file in "$cell"/*.json; do
	[[ -f "$json_file" ]] || continue
	require_json_object "$json_file"
done
derived_artifacts=(
	observationreceipts.json frontiervectors.json mechanical.json conservation.json positions.json
	fillpositions.json orderlifecycle.json lifecycle.json settlements.json expiryfills.json
	evidenceartifacthash.json streamhash.json arbitrage.json crossvenue.json roleaudit.json ecology.json
	derivatives.json liquidations.json marginchecks.json optionsurface.json optionliabilityp6.json
	optionvaluetakerp6.json vannavolgap6.json exposure.json hedging.json makerrefresh.json
	makerquotesize.json makerrebalance.json postonly.json liabilityhedger.json perpsignals.json
	datedmandatep5.json fundingcarry.json termcarry.json datedcarryp5.json perpreplenishment.json
	activation.json integrity.json analysis-metadata.json
)
for artifact in "${derived_artifacts[@]}"; do
	[[ ! -e "$analysis_dir/$artifact" && ! -e "$analysis_dir/$artifact.err" ]] || fail "refusing to overwrite existing derived evidence: $analysis_dir/$artifact"
done
if find "$cell" -maxdepth 1 -type f -name '*.json.tmp-*' -print -quit | grep -q .; then
	fail "refusing extraction with stale temporary derived evidence in $cell"
fi
jq -e '.domain == "persisted_json_records" and .ordering == "unordered_multiset" and
	 (.events | type) == "number" and .events > 0 and (.digest | test("^[0-9a-f]{64}$"))' \
	"$cell/evidence-artifact-hash.json" >/dev/null || fail "invalid runtime evidence artifact hash"

write_metric() {
	local output=$1
	shift
	local temporary
	temporary=$(mktemp "$output.tmp-XXXXXX")
	if ! "$@" >"$temporary" 2>"$output.err"; then
		rm -f "$temporary"
		return 1
	fi
	mv "$temporary" "$output"
	require_json_object "$output"
	jq -e 'has("result") and (.result | type) == "object"' "$output" >/dev/null ||
		fail "analyzer output lacks an object result: $output"
}
metrics=(
	observationreceipts frontiervectors mechanical conservation positions
	fillpositions orderlifecycle lifecycle settlements expiryfills
	evidenceartifacthash streamhash arbitrage crossvenue roleaudit ecology
	derivatives liquidations marginchecks optionsurface optionliabilityp6
	optionvaluetakerp6 vannavolgap6 exposure hedging makerrefresh makerquotesize
	makerrebalance postonly liabilityhedger perpsignals
)
for metric in "${metrics[@]}"; do
	if [[ "$metric" == positions || "$metric" == settlements ]]; then
		if [[ "$metric" == settlements ]]; then
			write_metric "$analysis_dir/$metric.json" "$analyzer" -metric "$metric" -json "$cell" -require-exact-replay -delivery-fee-policy "$delivery_fee_policy" ||
				fail "analyzer metric failed: $metric"
		else
			write_metric "$analysis_dir/$metric.json" "$analyzer" -metric "$metric" -json "$cell" -require-exact-replay ||
				fail "analyzer metric failed: $metric"
		fi
	else
		write_metric "$analysis_dir/$metric.json" "$analyzer" -metric "$metric" -json "$cell" ||
			fail "analyzer metric failed: $metric"
	fi
done

write_inactive() {
	local metric=$1 field=$2 reason=$3
	local temporary
	temporary=$(mktemp "$analysis_dir/$metric.json.tmp-XXXXXX")
	jq -n --arg metric "$metric" --arg field "$field" --arg reason "$reason" \
		'{schema_version: 1, result: {status: "OUT_OF_SCOPE", classification: "RECORDER_NOT_ENABLED",
		metric: $metric, config_field: $field, reason: $reason, observations: 0}}' >"$temporary"
	mv "$temporary" "$analysis_dir/$metric.json"
	require_json_object "$analysis_dir/$metric.json"
}
if [[ "$(jq -r '.record_funding_carry_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive fundingcarry record_funding_carry_decisions "registered integrated composition does not enable P4 actor decision receipts"
else
	write_metric "$analysis_dir/fundingcarry.json" "$analyzer" -metric fundingcarry -json "$cell" || fail "fundingcarry failed"
fi
if [[ "$(jq -r '.record_term_carry_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive termcarry record_term_carry_decisions "registered integrated composition does not enable P5 actor decision receipts"
else
	write_metric "$analysis_dir/termcarry.json" "$analyzer" -metric termcarry -json "$cell" || fail "termcarry failed"
fi
if [[ "$(jq -r '.record_dated_term_carry_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive datedcarryp5 record_dated_term_carry_decisions "registered integrated composition does not enable P5 dated-carry decision receipts"
else
	write_metric "$analysis_dir/datedcarryp5.json" "$analyzer" -metric datedcarryp5 -json "$cell" || fail "datedcarryp5 failed"
fi
if [[ "$(jq -r '.record_dated_execution_mandate_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive datedmandatep5 record_dated_execution_mandate_decisions "registered integrated composition does not enable P5 dated-execution mandate receipts"
else
	write_metric "$analysis_dir/datedmandatep5.json" "$analyzer" -metric datedmandatep5 -json "$cell" || fail "datedmandatep5 failed"
fi
if [[ "$(jq -r '.record_perp_maker_replenishment_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive perpreplenishment record_perp_maker_replenishment_decisions "registered integrated composition does not enable P3 replenishment receipts"
else
	write_metric "$analysis_dir/perpreplenishment.json" "$analyzer" -metric perpreplenishment -json "$cell" || fail "perpreplenishment failed"
fi

raw_count=0
cdf_borrow_events=0
price_unavailable_rejections=0
[[ -d "$cell/venues" ]] || fail "missing raw venue evidence directory"
while IFS= read -r -d '' raw_file; do
	raw_count=$((raw_count + 1))
	count=$(jq -c '
		def payload: (.data.payload // .data // {});
		def number_value: if type == "number" then . elif type == "string" then (tonumber? // 0) else 0 end;
		select(.event == "borrow" and (payload.asset // "") == "CDF" and
			((payload.amount // 0) | number_value) > 0 and
			((payload.collateral_used // 0) | number_value) > 0) | 1' "$raw_file" | wc -l)
	cdf_borrow_events=$((cdf_borrow_events + count))
	count=$(jq -c '
		def payload: (.data.payload // .data // {});
		select(.event == "OrderRejected" and
			((payload.error // payload.payload.error // .data.error // .error // "") == "PRICE_UNAVAILABLE")) | 1' "$raw_file" | wc -l)
	price_unavailable_rejections=$((price_unavailable_rejections + count))
done < <(find "$cell/venues" -type f -name '*.jsonl' -print0 | sort -z)
[[ "$raw_count" -gt 0 ]] || fail "no raw JSONL evidence files found"

activation_tmp=$(mktemp "$analysis_dir/activation.json.tmp-XXXXXX")
jq -n --argjson cdf_borrow_events "$cdf_borrow_events" \
	--argjson price_unavailable_rejections "$price_unavailable_rejections" \
	--argjson enabled_cross_asset_spot_graph "$(jq -er '.cross_asset_spot_graph' "$cell/run-config.json")" \
	--argjson enabled_cross_asset_collateral_marks "$(jq -er '.cross_asset_collateral_marks' "$cell/run-config.json")" \
	--arg contract "$contract_version" \
	'{schema_version: 1, result: {contract: $contract,
		cdf_collateral_borrowing: {events: $cdf_borrow_events,
			enabled_cross_asset_spot_graph: $enabled_cross_asset_spot_graph,
			enabled_cross_asset_collateral_marks: $enabled_cross_asset_collateral_marks},
		price_unavailable_order_rejections: $price_unavailable_rejections,
		predicates: {cdf_collateral_borrowing_observed: ($cdf_borrow_events > 0 and
			$enabled_cross_asset_spot_graph and $enabled_cross_asset_collateral_marks),
			zero_price_unavailable_order_rejections: ($price_unavailable_rejections == 0)}}}' >"$activation_tmp"
mv "$activation_tmp" "$analysis_dir/activation.json"
require_json_object "$analysis_dir/activation.json"
jq -e '(.result.predicates | length) == 2 and
	(.result.predicates | keys) == ["cdf_collateral_borrowing_observed", "zero_price_unavailable_order_rejections"] and
	all(.result.predicates | to_entries[]; .value == true)' "$analysis_dir/activation.json" >/dev/null ||
	fail "candidate activation contract not satisfied"

integrity_tmp=$(mktemp "$analysis_dir/integrity.json.tmp-XXXXXX")
jq -n --argjson tolerance "$conservation_tolerance_fixed_units" \
	--slurpfile receipts "$analysis_dir/observationreceipts.json" \
	--slurpfile frontiers "$analysis_dir/frontiervectors.json" \
	--slurpfile mechanical "$analysis_dir/mechanical.json" \
	--slurpfile conservation "$analysis_dir/conservation.json" \
	--slurpfile positions "$analysis_dir/positions.json" \
	--slurpfile fillpositions "$analysis_dir/fillpositions.json" \
	--slurpfile orderlifecycle "$analysis_dir/orderlifecycle.json" \
	--slurpfile lifecycle "$analysis_dir/lifecycle.json" \
	--slurpfile settlements "$analysis_dir/settlements.json" \
	--slurpfile expiryfills "$analysis_dir/expiryfills.json" \
	--slurpfile derivatives "$analysis_dir/derivatives.json" \
	--slurpfile liquidations "$analysis_dir/liquidations.json" \
	--slurpfile marginchecks "$analysis_dir/marginchecks.json" \
	--slurpfile optionliability "$analysis_dir/optionliabilityp6.json" \
	--slurpfile optionvaluetaker "$analysis_dir/optionvaluetakerp6.json" \
	--slurpfile vannavolga "$analysis_dir/vannavolgap6.json" \
	--slurpfile optionsurface "$analysis_dir/optionsurface.json" \
	--slurpfile exposure "$analysis_dir/exposure.json" \
	--slurpfile hedging "$analysis_dir/hedging.json" \
	--slurpfile makerrefresh "$analysis_dir/makerrefresh.json" \
	--slurpfile makerquotesize "$analysis_dir/makerquotesize.json" \
	--slurpfile makerrebalance "$analysis_dir/makerrebalance.json" \
	--slurpfile postonly "$analysis_dir/postonly.json" \
	--slurpfile liabilityhedger "$analysis_dir/liabilityhedger.json" \
	--slurpfile activation "$analysis_dir/activation.json" \
	--arg contract "$contract_version" \
	'def r($x): $x[0].result;
	 def field($x; $name): (r($x) | getpath($name | split(".")));
	 def count($x; $name): (field($x; $name) // 0) as $value | if ($value | type) == "array" then ($value | length) elif ($value | type) == "number" then $value else 0 end;
	 def zero($x; $name): ((field($x; $name) | type) == "number" and field($x; $name) == 0);
	 def absolute($x): if ($x | type) == "number" then (if $x < 0 then -$x else $x end) else 999999999999999999 end;
	 def residuals_within($items): all(($items // [])[]; absolute(.residual // 0) <= $tolerance);
	 def field_zeroes($x; $names): all($names[]; zero($x; .));
	 {schema_version: 1, contract: $contract,
		tolerances: {max_abs_identity_residual_fixed_units: $tolerance},
		predicates: {
			observation_receipts: (r($receipts).valid == true and count($receipts; "schedules") > 0 and count($receipts; "receipts") > 0 and count($receipts; "decisions") > 0 and field_zeroes($receipts; ["unknown_link_id", "unknown_symbol_id", "unknown_type", "nonzero_reserved", "scheduled_before_publication", "delivered_before_scheduled", "bad_schedule_ordinal", "bad_receipt_ordinal", "duplicate_source_identity", "receipt_without_schedule", "schedule_receipt_mismatch", "missing_due_receipt", "bad_global_event_order", "decision_without_link", "bad_decision_frontier", "future_decision_use"])),
			frontier_vectors: (r($frontiers).valid == true and r($frontiers).base_evidence_valid == true and r($frontiers).base_manifest_digest_matches == true and r($frontiers).decision_digest_matches == true and r($frontiers).component_digest_matches == true and count($frontiers; "decisions") > 0 and count($frontiers; "components") > 0 and field_zeroes($frontiers; ["bad_decision_id", "bad_decision_fields", "missing_scalar_decision", "missing_vector_decision", "duplicate_vector_decision", "unknown_component_link", "bad_component_ordinal", "duplicate_component", "bad_component_frontier", "future_component_use", "missing_decision_components", "extra_decision_components", "nonzero_reserved"])),
			mechanical: (count($mechanical; "orders") > 0 and zero($mechanical; "drift.mismatches")),
			conservation: (count($conservation; "flows") > 0 and count($conservation; "identities") > 0 and count($conservation; "venue_identities") > 0 and count($conservation; "delta_consistency.checked") > 0 and zero($conservation; "delta_consistency.mismatched") and zero($conservation; "delta_consistency.chain_broken") and zero($conservation; "delta_consistency.decode_failures") and residuals_within(r($conservation).identities) and residuals_within(r($conservation).venue_identities)),
			position_rounding: (r($conservation).position_rounding.valid == true and count($conservation; "position_rounding.events") > 0 and field_zeroes($conservation; ["position_rounding.invalid", "position_rounding.remainder_out_of_range", "position_rounding.duplicate_terminal_keys", "position_rounding.balance_link_failures", "position_rounding.venue_link_failures", "position_rounding.asset_wallet_failures", "position_rounding.venue_bucket_failures"])),
			positions: (count($positions; "contracts") > 0 and count($positions; "exact_replay_checks") > 0 and count($positions; "realized_pnl_checks") > 0 and field_zeroes($positions; ["non_zero_net_contracts", "disagreement", "unrepresentable_open_values", "exact_replay_failures", "realized_pnl_failures", "evidence_failures", "missing_marks", "mark_identity_failures", "missing_terminal_positions", "unexpected_terminal_positions", "terminal_position_mismatches", "terminal_timestamp_failures", "post_terminal_position_updates"])),
			fill_positions: (zero($fillpositions; "missing_position_update") and zero($fillpositions; "unexpected_position_update") and zero($fillpositions; "position_chain_failures")),
			order_lifecycle: field_zeroes($orderlifecycle; ["unknown_fills", "unknown_cancellations", "duplicate_acceptances", "duplicate_terminals", "fills_after_terminal", "fill_quantity_mismatches", "cancel_quantity_mismatches", "client_mismatches", "unlinked_fills", "missing_immediate_terminal"]),
			settlement: (count($settlements; "checks") > 0 and count($settlements; "exact_replay_checks") > 0 and field_zeroes($settlements; ["mismatched", "unpaid", "total_trades_after_expiry", "total_position_updates_after_expiry", "arithmetic_failures", "explicit_unavailable_announcements", "exact_replay_failures", "settlement_event_mismatches", "evidence_failures", "descriptor_conflicts", "settlement_timing_failures", "delivery_fee_mismatches"])),
			expiry: (count($expiryfills; "expired_contracts") > 0 and count($expiryfills; "settled_contracts") > 0 and zero($expiryfills; "expired_unsettled_contracts") and field_zeroes($expiryfills; ["fills_before_listing", "fills_after_expiry", "malformed_fill_records", "fill_identity_failures", "missing_expiry_metadata", "settlement_without_listing", "metadata_mismatches", "snapshot_records_before_listing", "snapshot_records_after_expiry", "nonempty_snapshots_after_expiry"])),
			derivatives: (count($derivatives; "funding") > 0 and field_zeroes($derivatives; ["funding_broken", "funding_sign_wrong", "funding_misdirected", "funding_undirected", "funding_duplicate_payments", "exercise_broken", "exercise_timing_failures", "holders_mispaid", "worthless_paid", "exercise_arithmetic_failures"])),
			liquidations: (zero($liquidations; "invalid_liquidations") and zero($liquidations; "position_path_missing") and zero($liquidations; "position_path_failures") and zero($liquidations; "position_conservation_missing") and zero($liquidations; "position_conservation_failures") and zero($liquidations; "deficit_mismatch_instants")),
			margin: field_zeroes($marginchecks; ["missing_checks", "unexpected_checks", "duplicate_checks", "field_mismatches", "mark_mismatches", "balance_mismatches", "contribution_mismatches", "equity_mismatches", "notional_mismatches", "maintenance_mismatches", "position_chain_failures", "balance_chain_failures", "arithmetic_failures", "unsupported_mark_domain", "ambiguous_mark_timestamp_collisions"]),
			option_liability: (r($optionliability).valid == true and count($optionliability; "decisions") > 0 and field_zeroes($optionliability; ["decode_errors", "future_observation_use", "invalid_decisions", "missing_outcomes", "duplicate_outcomes", "orphan_outcomes", "outcome_mismatches"])),
			maker_refresh: (r($makerrefresh).valid == true),
			maker_quote_size: (count($makerquotesize; "decisions") > 0 and field_zeroes($makerquotesize; ["missing_outcomes", "duplicate_outcomes", "duplicate_decision_sides", "decision_field_mismatches", "outcome_field_mismatches", "invalid_decision_records", "invalid_censor_records", "wrong_direction_size_skew", "censored_outcome_deliveries"])),
			maker_rebalance: (r($makerrebalance).valid == true),
			post_only: (count($postonly; "accepted_post_only") > 0 and zero($postonly; "unmatched_fill_orders")),
			liability_hedger: (r($liabilityhedger).valid == true),
			option_value_taker: (count($optionvaluetaker; "decisions") > 0),
			vanna_volga: (count($vannavolga; "decisions") > 0),
			option_surface: (count($optionsurface; "points") > 0),
			exposure: (count($exposure; "risk_samples") > 0),
			hedging: (count($hedging; "profiles") > 0),
			activation: (r($activation).predicates.cdf_collateral_borrowing_observed == true and r($activation).predicates.zero_price_unavailable_order_rejections == true),
			late_path: (count($lifecycle; "funding") > 0 and count($lifecycle; "settlement_rounds") > 0 and count($settlements; "checks") > 0 and count($expiryfills; "expired_contracts") > 0)
		},
		observed: {
			conservation_max_abs_identity_residual: ([((r($conservation).identities // [])[] | absolute(.residual // 0)), ((r($conservation).venue_identities // [])[] | absolute(.residual // 0))] | max // 0),
			funding_instants: count($conservation; "funding_instants"),
			expiry_instants: count($conservation; "expiry_instants"),
			option_expiry_instants: count($conservation; "option_expiry_instants")
		}}' >"$integrity_tmp"
mv "$integrity_tmp" "$analysis_dir/integrity.json"
require_json_object "$analysis_dir/integrity.json"
jq -e '(.predicates | keys) == ["activation", "conservation", "derivatives", "expiry", "exposure", "fill_positions", "frontier_vectors", "hedging", "late_path", "liability_hedger", "liquidations", "maker_quote_size", "maker_rebalance", "maker_refresh", "margin", "mechanical", "observation_receipts", "option_liability", "option_surface", "option_value_taker", "order_lifecycle", "position_rounding", "positions", "post_only", "settlement", "vanna_volga"] and
	all(.predicates | to_entries[]; .value == true)' "$analysis_dir/integrity.json" >/dev/null ||
	fail "one or more fail-closed integrity predicates failed"

required=(
	observationreceipts.json frontiervectors.json mechanical.json conservation.json
	positions.json fillpositions.json orderlifecycle.json lifecycle.json settlements.json
	expiryfills.json evidenceartifacthash.json streamhash.json arbitrage.json crossvenue.json
	roleaudit.json ecology.json derivatives.json liquidations.json marginchecks.json
	optionsurface.json optionliabilityp6.json optionvaluetakerp6.json vannavolgap6.json
	exposure.json hedging.json makerrefresh.json makerquotesize.json makerrebalance.json
	postonly.json liabilityhedger.json perpsignals.json datedmandatep5.json fundingcarry.json
	termcarry.json datedcarryp5.json perpreplenishment.json activation.json integrity.json
)
for artifact in "${required[@]}"; do
	require_file "$analysis_dir/$artifact"
	require_json_object "$analysis_dir/$artifact"
done

runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
offline_events=$(jq -er '.result.events' "$analysis_dir/evidenceartifacthash.json")
offline_digest=$(jq -er '.result.digest' "$analysis_dir/evidenceartifacthash.json")
[[ "$runtime_events" == "$offline_events" && "$runtime_digest" == "$offline_digest" ]] || fail "runtime/offline evidence digest mismatch"
jq -e '(.result.domain // "") == "persisted_json_records" and (.result.ordering // "") == "unordered_multiset"' \
	"$analysis_dir/evidenceartifacthash.json" >/dev/null || fail "offline evidence hash domain mismatch"
jq -e '.result.domain == "persisted_evidence" and .result.ordering == "unordered_multiset"' \
	"$analysis_dir/streamhash.json" >/dev/null || fail "stream hash domain mismatch"

analyzer_revision=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
analyzer_modified=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
analyzer_sha256=$(sha256sum "$analyzer" | awk '{print $1}')
analyzer_trimpath=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
analyzer_cgo_enabled=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
[[ "$analyzer_revision" == "$head_revision" && "$analyzer_modified" == false && "$analyzer_trimpath" == true && "$analyzer_cgo_enabled" == 0 ]] || fail "analyzer is not a clean reproducible build of current HEAD"

required_json=$(printf '%s\n' "${required[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')
artifact_sha256=$(for artifact in "${required[@]}"; do
	printf '%s\t%s\n' "$artifact" "$(sha256sum "$analysis_dir/$artifact" | awk '{print $1}')"
done | jq -Rn 'reduce inputs as $line ({}; ($line | split("\t")) as $parts | .[$parts[0]] = $parts[1])')
metadata_tmp=$(mktemp "$analysis_dir/analysis-metadata.json.tmp-XXXXXX")
analyzer_modified_json=false
[[ "$analyzer_modified" == true ]] && analyzer_modified_json=true
jq -n \
	--arg analysis_revision "$head_revision" \
	--arg analyzer_revision "$analyzer_revision" \
	--arg analyzer_sha256 "$analyzer_sha256" \
	--argjson analyzer_trimpath true \
	--arg analyzer_cgo_enabled "$analyzer_cgo_enabled" \
	--arg analyzer_go_version "$analyzer_go_version" \
	--argjson analyzer_modified "$analyzer_modified_json" \
	--argjson required_artifacts "$required_json" \
	--argjson artifact_sha256 "$artifact_sha256" \
	--argjson runtime_evidence_events "$runtime_events" \
	--arg runtime_evidence_digest "$runtime_digest" \
	--arg contract "$contract_version" \
	--arg cell "$cell_name" \
	--argjson seed "$seed" \
	--arg simulator_revision "$metadata_revision" \
	--arg simulator_sha256 "$simulator_sha256" \
	--argjson simulator_trimpath true \
	--arg simulator_cgo_enabled "$binary_cgo_enabled" \
	--arg simulator_go_version "$simulator_go_version" \
	--arg prunegate_revision "$prunegate_revision" \
	--arg prunegate_sha256 "$prunegate_sha256" \
	--argjson prunegate_trimpath true \
	--arg prunegate_cgo_enabled "$prunegate_cgo_enabled" \
	--arg prunegate_go_version "$prunegate_go_version" \
	--arg config_sha256 "$config_sha256" \
	'{schema_version: 2, cell: $cell, seed: $seed,
		analysis_revision: $analysis_revision, analyzer_revision: $analyzer_revision,
		analyzer_sha256: $analyzer_sha256, analyzer_vcs_modified: $analyzer_modified,
		analyzer_trimpath: $analyzer_trimpath, analyzer_cgo_enabled: $analyzer_cgo_enabled,
		analyzer_go_version: $analyzer_go_version,
		require_exact_replay: true,
		simulator_revision: $simulator_revision, simulator_sha256: $simulator_sha256,
		simulator_trimpath: $simulator_trimpath, simulator_cgo_enabled: $simulator_cgo_enabled,
		simulator_go_version: $simulator_go_version,
		prunegate_revision: $prunegate_revision, prunegate_sha256: $prunegate_sha256,
		prunegate_trimpath: $prunegate_trimpath, prunegate_cgo_enabled: $prunegate_cgo_enabled,
		prunegate_go_version: $prunegate_go_version,
		config_sha256: $config_sha256, analysis_contract: $contract,
		integrity_contract: $contract, activation_contract: $contract,
		completion_sentinels: ["greeks.json", "latency.json"], required_artifacts: $required_artifacts,
		artifact_sha256: $artifact_sha256,
		runtime_evidence_artifact: {events: $runtime_evidence_events, digest: $runtime_evidence_digest},
		inactive_contracts: ["fundingcarry", "termcarry", "datedcarryp5", "datedmandatep5", "perpreplenishment"],
		raw_log_policy: "retained; this extractor has no prune authority"}' >"$metadata_tmp"
mv "$metadata_tmp" "$analysis_dir/analysis-metadata.json"
require_json_object "$analysis_dir/analysis-metadata.json"
jq -e --arg revision "$head_revision" --arg analyzer_revision "$analyzer_revision" \
	--arg contract "$contract_version" --argjson required_artifacts "$required_json" \
	'.schema_version == 2 and .analysis_revision == $revision and
	 .analyzer_revision == $analyzer_revision and .analyzer_vcs_modified == false and
	 .require_exact_replay == true and
	 .analysis_contract == $contract and .required_artifacts == $required_artifacts and
	 (.artifact_sha256 | keys) == ($required_artifacts | sort) and
	 all(.artifact_sha256 | to_entries[]; (.value | test("^[0-9a-f]{64}$"))) and
	 (.raw_log_policy | type) == "string"' "$analysis_dir/analysis-metadata.json" >/dev/null ||
	fail "analysis metadata self-check failed"

printf 'extracted integrated long-run evidence: %s\n' "$cell"
