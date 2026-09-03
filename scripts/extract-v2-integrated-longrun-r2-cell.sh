#!/usr/bin/env bash
# Fail-closed extraction for one completed integrated V2 long-run development
# cell. This script adds derived evidence but has no prune authority.
set -euo pipefail

if [[ $# -ne 1 ]]; then
	printf 'usage: %s CELL_DIR\n' "$0" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
extractor_variant=${V2_R2_EXTRACTOR_VARIANT:-historical}
case "$extractor_variant" in
	historical)
		contract_script="$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"
		contract_version="v2-integrated-longrun-r2-candidate-v2"
		expected_config_dir="$root_dir/research/configs/v2-integrated-longrun-r2"
		expected_runner_contract="v2-integrated-longrun-r2-runner-v2"
		;;
	sv1)
		contract_script=$(v2_r2_select_sv1_contract "$root_dir") || {
			printf 'integrated long-run extraction failure: unregistered SV1 contract path\n' >&2
			exit 1
		}
		;;
	*)
		printf 'integrated long-run extraction failure: unsupported extractor variant: %s\n' "$extractor_variant" >&2
		exit 2
		;;
esac
source "$contract_script"
if [[ "$extractor_variant" == sv1 ]]; then
	export V2_R2_SV1_CONTRACT_SCRIPT="$contract_script"
fi
if [[ "$extractor_variant" == sv1 ]]; then
	contract_version="$v2_r2_sv1_candidate_contract_version"
	expected_config_dir="$v2_r2_sv1_config_dir"
	expected_runner_contract="$v2_r2_sv1_runner_contract"
fi
completion_sentinels='["greeks.json", "latency.json"]'
if [[ "$extractor_variant" == sv1 ]]; then
	completion_sentinels="$v2_r2_sv1_completion_sentinels"
fi
source "$root_dir/scripts/v2-r2-evidence-input-contract.sh"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}
renderer=${EVSRENDER_BIN:-"$root_dir/bin/evsrender"}
terminal_outcome_filter="$root_dir/scripts/v2-r2-sv1-terminal-outcome.jq"
render_route_compression=${V2_R2_RENDER_ROUTE_COMPRESSION:-zstd}
analyzer_only_replay=${V2_R2_ANALYZER_ONLY_REPLAY:-false}
raw_source_revision=${V2_R2_RAW_SOURCE_REVISION:-}
conservation_tolerance_fixed_units=1000
calendar_epoch_nano=1735689600000000000
calendar_hour_nano=3600000000000
expected_calendar_listing_timeline=$(v2_r2_expected_calendar_listing_timeline)
expected_calendar_expiries="["
for hour in $(seq 2 26); do
	expected_calendar_expiries+="$((calendar_epoch_nano + hour * calendar_hour_nano)),"
done
expected_calendar_expiries+="$((calendar_epoch_nano + 27 * calendar_hour_nano)),$((calendar_epoch_nano + 30 * calendar_hour_nano)),$((calendar_epoch_nano + 36 * calendar_hour_nano))]"
expected_calendar_completed_expiries="["
for hour in $(seq 2 24); do
	expected_calendar_completed_expiries+="$((calendar_epoch_nano + hour * calendar_hour_nano)),"
done
expected_calendar_completed_expiries="${expected_calendar_completed_expiries%,}]"

fail() {
	printf 'integrated long-run extraction failure: %s\n' "$*" >&2
	exit 1
}
case "$render_route_compression" in
	none|zstd) ;;
	*) fail "unsupported rendered route compression: $render_route_compression" ;;
esac
case "$analyzer_only_replay" in
	true|false) ;;
	*) fail "V2_R2_ANALYZER_ONLY_REPLAY must be true or false" ;;
esac
v2_r2_acquire_namespace_lock || fail "could not acquire the R2 evidence namespace lock"
require_file() {
	local path=$1
	[[ -s "$path" ]] || fail "missing required file: $path"
}
require_json_object() {
	local path=$1
	jq -e 'type == "object"' "$path" >/dev/null || fail "malformed JSON object: $path"
}
require_evidence_input() {
	local input_kind=$1
	local path=$2
	if ! v2_r2_require_evidence_input_file "$input_kind" "$path"; then
		case "$input_kind" in
			binary) fail "missing or invalid binary evidence stream: $path" ;;
			json) fail "malformed JSON object: $path" ;;
			*) fail "unsupported evidence input kind: $input_kind" ;;
		esac
	fi
}

v2_r2_require_output_root "$v2_r2_output_root" || fail "R2 output root is not canonical"
v2_r2_require_cell_path "$1" || fail "cell is outside the canonical R2 evidence root or is symlinked: $1"
cell=$(realpath -e -- "$1")
cell_name=$(basename "$cell")
analysis_dir=${V2_ANALYSIS_OUTPUT_DIR:-$cell}
if [[ "$analysis_dir" != "$cell" ]]; then
	[[ -d "$analysis_dir" && ! -L "$analysis_dir" ]] || fail "analysis output directory must be an existing non-symlink directory: $analysis_dir"
	analysis_dir=$(realpath -e -- "$analysis_dir")
	[[ "$analysis_dir" != "$cell" ]] || fail "analysis output directory must be separate from raw evidence cell"
fi
[[ -x "$analyzer" ]] || fail "missing analyzer: $analyzer"
analyzer_go_version=$(v2_r2_binary_go_version "$analyzer")
v2_r2_is_go_127 "$analyzer_go_version" || fail "analyzer is not built with the pinned Go 1.27 toolchain: $analyzer_go_version"
case "$extractor_variant:$cell_name" in
	historical:dev-607|historical:dev-613|historical:dev-617) ;;
	sv1:treatment-*|sv1:control-*)
		[[ "$cell_name" =~ ^(treatment|control)-([0-9]+)$ ]] || fail "extractor accepts only primary SV1B cells: $cell_name"
		v2_r2_sv1_is_registered_seed "${BASH_REMATCH[2]}" || fail "unregistered SV1 development seed: $cell_name"
		;;
	*) fail "extractor accepts only registered full development cells, got $cell_name" ;;
esac
terminal_failure=false
require_file "$cell/latency.json"
require_json_object "$cell/latency.json"
log_mode=$(jq -er '.log_mode' "$cell/run-config.json")
evidence_format=$(jq -er '.evidence_format // "jsonl"' "$cell/run-config.json")
case "$evidence_format" in
	jsonl) required_inputs=(manifest.json evidence-artifact-hash.json evidence-manifest.json run-config.json run-metadata.json run-status.json) ;;
	evstream_v3) required_inputs=(manifest.json binary-evidence-attestation.json evidence-manifest.json events.evs run-config.json run-metadata.json run-status.json) ;;
	*) fail "unsupported evidence format: $evidence_format" ;;
esac
if [[ "$evidence_format" == evstream_v3 && "$log_mode" == "full" ]]; then
	required_inputs+=(evidence-only-artifact-hash.json)
fi
for input in "${required_inputs[@]}"; do
	case "$input" in
		events.evs) require_evidence_input binary "$cell/$input" ;;
		*) require_evidence_input json "$cell/$input" ;;
	esac
done

raw_stage_marker="$cell/.raw-evidence-staged.$$"
cleanup_raw_stage() {
	if [[ -e "$raw_stage_marker" ]]; then
		v2_r2_cleanup_staged_raw_evidence "$cell" ||
			printf 'integrated long-run extraction cleanup failure: %s\n' "$cell" >&2
	fi
}
trap cleanup_raw_stage EXIT
v2_r2_stage_raw_evidence "$cell" || fail "raw evidence is neither retained nor covered by a valid archive"

expected_config="$expected_config_dir/$cell_name.json"
require_file "$expected_config"
cmp -s "$expected_config" "$cell/run-config.json" || fail "run config is not byte-identical to registered $cell_name"

seed=$(jq -er '.seed' "$cell/run-metadata.json")
config_seed=$(jq -er '.seed' "$cell/run-config.json")
config_hypothesis=$(jq -er '.hypothesis_id' "$cell/run-config.json")
config_experiment=$(jq -er '.experiment_id' "$cell/run-config.json")
delivery_fee_policy=$(jq -er '.dated_future_delivery_fee_policy' "$cell/run-config.json")
funding_interval_seconds=$(jq -er '.funding_interval_seconds' "$cell/run-config.json")
simulation_start_nano=$(jq -er '.simulation_start_nano' "$cell/run-metadata.json")
simulation_end_nano=$(jq -er '.simulation_end_nano' "$cell/run-metadata.json")
[[ "$seed" == "$config_seed" ]] || fail "metadata/config seed mismatch"
	[[ "$log_mode" == full || "$log_mode" == none ]] || fail "unsupported development log mode: $log_mode"
	[[ "$evidence_format" == "evstream_v3" ]] || fail "successor development extraction requires evstream_v3 evidence"
jq -e --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
	'($simulation_start_nano == 1735689600000000000 and $simulation_end_nano == 1735776000000000000 and
	 ($simulation_end_nano - $simulation_start_nano) == 86400000000000)' \
	<<<'null' >/dev/null || fail "run metadata does not use the registered 24-hour horizon"
if [[ "$extractor_variant" == sv1 && "$v2_r2_sv1_require_terminal_outcome" == true ]]; then
	require_file "$cell/terminal-outcome.json"
	require_json_object "$cell/terminal-outcome.json"
	jq -e --argjson start "$simulation_start_nano" --argjson end "$simulation_end_nano" \
		-f "$terminal_outcome_filter" "$cell/terminal-outcome.json" >/dev/null ||
		fail "invalid typed terminal outcome"
	if [[ "$(jq -er '.status' "$cell/terminal-outcome.json")" == terminal_failure ]]; then
		terminal_failure=true
	fi
fi
	greeks_present=false
	if [[ -s "$cell/greeks.json" ]]; then
		[[ -f "$cell/greeks.json" && ! -L "$cell/greeks.json" ]] || fail "greeks completion sentinel is symlinked"
		require_json_object "$cell/greeks.json"
		greeks_present=true
	elif [[ "$terminal_failure" != true || "$log_mode" != none ]]; then
		fail "missing greeks completion sentinel"
	fi
	if [[ "$terminal_failure" == true ]]; then
		if [[ "$greeks_present" == true ]]; then
		jq -e --slurpfile outcome "$cell/terminal-outcome.json" '
			type == "object" and .schema_version == 7 and
		.report_status == "partial_terminal_failure" and .terminal_valuation_available == false and
		(.initial_accounts | type) == "array" and (.initial_accounts | length) > 0 and
		((.terminal_accounts // []) | type) == "array" and ((.terminal_accounts // []) | length) == 0 and
		((.terminal_risk // {}) | type) == "object" and ((.terminal_risk // {}) | length) == 0 and
		(.terminal_outcome | type) == "object" and .terminal_outcome == $outcome[0]' \
			"$cell/greeks.json" >/dev/null || fail "partial terminal-failure report is not explicitly valuation-incomplete"
		fi
	elif [[ "$greeks_present" == true ]]; then
		jq -e '(.schema_version == 7 and .report_status == "complete_terminal_valuation" and .terminal_valuation_available == true)' \
			"$cell/greeks.json" >/dev/null || fail "greeks report has an unknown valuation completeness state"
	fi

	if [[ "$analysis_dir" != "$cell" && "$v2_r2_sv1_require_terminal_outcome" == true ]]; then
		[[ ! -e "$analysis_dir/terminal-outcome.json" && ! -L "$analysis_dir/terminal-outcome.json" ]] ||
			fail "analysis output already contains terminal outcome metadata"
		cp -- "$cell/terminal-outcome.json" "$analysis_dir/terminal-outcome.json" ||
			fail "could not stage immutable terminal outcome metadata for analysis"
	fi
jq -e --arg cell "$cell_name" --argjson seed "$seed" --arg log_mode "$log_mode" --arg experiment "$config_experiment" \
	--arg runner_contract "$expected_runner_contract" --arg hypothesis_id "$config_hypothesis" \
	--argjson completion_sentinels "$completion_sentinels" \
	'.schema_version == 6 and .runner_contract == $runner_contract and
	 .cell == $cell and .seed == $seed and .holdout == false and
		 .simulated_horizon == "24h" and .log_mode == $log_mode and .evidence_format == "evstream_v3" and
	 .simulation_start_nano == 1735689600000000000 and .simulation_end_nano == 1735776000000000000 and
	 (.gomaxprocs | type) == "number" and .gomaxprocs == 4 and
	 .hypothesis_id == $hypothesis_id and
	 .config_experiment_id == $experiment and
	 (.config_sha256 | test("^[0-9a-f]{64}$")) and
	 (.binary_sha256 | test("^[0-9a-f]{64}$")) and
	 .binary_trimpath == true and .binary_cgo_enabled == "0" and
	 (.git_revision | test("^[0-9a-f]{40}$")) and
	 .completion_sentinels == $completion_sentinels' \
	"$cell/run-metadata.json" >/dev/null || fail "invalid run metadata contract"
	if [[ "$terminal_failure" == true ]]; then
		jq -e --arg cell "$cell_name" --argjson completion_sentinels "$completion_sentinels" \
			--arg code "$(jq -er '.code' "$cell/terminal-outcome.json")" \
			--arg stage "$(jq -er '.stage' "$cell/terminal-outcome.json")" \
			--argjson failure_at_nano "$(jq -er '.failure_at_nano' "$cell/terminal-outcome.json")" \
			--arg failure_venue_id "$(jq -er '.failure_venue_id' "$cell/terminal-outcome.json")" \
			--arg failure_symbol "$(jq -er '.failure_symbol' "$cell/terminal-outcome.json")" \
			'.cell == $cell and (.exit_status | type) == "number" and .exit_status != 0 and
			 .completion_verified == false and .terminal_failure_verified == true and
			 .simulated_horizon == "24h" and .completion_sentinels == $completion_sentinels and
			 (.run_metadata_sha256 | test("^[0-9a-f]{64}$")) and
			 (.manifest_sha256 | test("^[0-9a-f]{64}$")) and
			 (.latency_sha256 | test("^[0-9a-f]{64}$")) and
			 (.checkpoints_sha256 | test("^[0-9a-f]{64}$")) and
			 (.simulation_start_nano | type) == "number" and (.simulation_end_nano | type) == "number" and
			 .simulation_start_nano == 1735689600000000000 and .simulation_end_nano == 1735776000000000000 and
			 (.evidence_manifest_sha256 | test("^[0-9a-f]{64}$")) and
			 (.terminal_outcome_sha256 | test("^[0-9a-f]{64}$")) and
			 .terminal_failure_code == $code and .terminal_failure_stage == $stage and
			 .terminal_failure_at_nano == $failure_at_nano and
			 .terminal_failure_venue_id == $failure_venue_id and .terminal_failure_symbol == $failure_symbol' \
			"$cell/run-status.json" >/dev/null || fail "invalid terminal-failure run status contract"
		else
		jq -e --arg cell "$cell_name" --argjson completion_sentinels "$completion_sentinels" \
		'.cell == $cell and .exit_status == 0 and .completion_verified == true and
		 .simulated_horizon == "24h" and .completion_sentinels == $completion_sentinels and
		 (.run_metadata_sha256 | test("^[0-9a-f]{64}$")) and
		 (.manifest_sha256 | test("^[0-9a-f]{64}$")) and
		 (.greeks_sha256 | test("^[0-9a-f]{64}$")) and
		(.latency_sha256 | test("^[0-9a-f]{64}$")) and
		(.checkpoints_sha256 | test("^[0-9a-f]{64}$")) and
		(.simulation_start_nano | type) == "number" and (.simulation_end_nano | type) == "number" and
		.simulation_start_nano == 1735689600000000000 and .simulation_end_nano == 1735776000000000000 and
		(.evidence_manifest_sha256 | test("^[0-9a-f]{64}$")) and
		(has("terminal_failure_verified") | not)' \
		"$cell/run-status.json" >/dev/null || fail "invalid run status contract"
		fi
		if [[ "$greeks_present" == true ]]; then
			jq -e '(.greeks_sha256 | test("^[0-9a-f]{64}$"))' "$cell/run-status.json" >/dev/null ||
				fail "terminal-failure run status has an invalid greeks hash"
		else
			jq -e '(has("greeks_sha256") | not)' "$cell/run-status.json" >/dev/null ||
				fail "no-log terminal-failure status unexpectedly claims a greeks artifact"
		fi
	if [[ "$extractor_variant" == sv1 && "$v2_r2_sv1_require_terminal_outcome" == true ]]; then
	jq -e '(.terminal_outcome_sha256 | test("^[0-9a-f]{64}$"))' "$cell/run-status.json" >/dev/null ||
		fail "run status lacks terminal outcome hash"
	[[ "$(sha256sum "$cell/terminal-outcome.json" | awk '{print $1}')" == "$(jq -er '.terminal_outcome_sha256' "$cell/run-status.json")" ]] ||
		fail "terminal outcome status hash mismatch"
fi

head_revision=$(git -C "$root_dir" rev-parse HEAD)
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "source worktree is dirty"
metadata_revision=$(jq -er '.git_revision' "$cell/run-metadata.json")
# A retained raw cell may be replayed by a descendant analyzer only when the
# operator declares that no simulator trajectory is being rerun. The old
# simulator and gate binaries remain independently bound to the raw metadata.
source_revision_mode=$(v2_r2_resolve_analysis_source_mode \
	"$root_dir" "$metadata_revision" "$head_revision" "$analyzer_only_replay" "$raw_source_revision") ||
	fail "raw source revision is not valid for this analysis mode"

simulator_binary=$(jq -er '.binary_path' "$cell/run-metadata.json")
require_file "$simulator_binary"
[[ -x "$simulator_binary" ]] || fail "recorded simulator binary is not executable"
simulator_sha256=$(sha256sum "$simulator_binary" | awk '{print $1}')
[[ "$simulator_sha256" == "$(jq -er '.binary_sha256' "$cell/run-metadata.json")" ]] || fail "simulator binary hash changed"
binary_revision=$(go version -m "$simulator_binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
binary_modified=$(go version -m "$simulator_binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
binary_trimpath=$(go version -m "$simulator_binary" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
binary_cgo_enabled=$(go version -m "$simulator_binary" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
simulator_go_version=$(v2_r2_binary_go_version "$simulator_binary")
v2_r2_is_go_127 "$simulator_go_version" || fail "simulator is not built with the pinned Go 1.27 toolchain: $simulator_go_version"
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
prunegate_go_version=$(v2_r2_binary_go_version "$prunegate_binary")
v2_r2_is_go_127 "$prunegate_go_version" || fail "prunegate is not built with the pinned Go 1.27 toolchain: $prunegate_go_version"
[[ "$prunegate_sha256" == "$(jq -er '.prunegate_sha256' "$cell/run-metadata.json")" &&
	"$prunegate_revision" == "$metadata_revision" && "$prunegate_modified" == false &&
	"$prunegate_trimpath" == true && "$prunegate_cgo_enabled" == 0 ]] || fail "prunegate provenance is not clean/current/reproducible"

config_sha256=$(sha256sum "$cell/run-config.json" | awk '{print $1}')
[[ "$config_sha256" == "$(jq -er '.config_sha256' "$cell/run-metadata.json")" ]] || fail "run config hash changed"
manifest_revision=$(jq -er '.build.revision' "$cell/manifest.json")
# jq -e returns failure for a valid false boolean; this is a provenance value,
# not a predicate whose false result should abort before the explicit check.
manifest_modified=$(jq -r '.build.modified' "$cell/manifest.json")
	jq -e --arg revision "$metadata_revision" --argjson seed "$seed" --arg log_mode "$log_mode" --arg evidence_format "$evidence_format" \
	--arg experiment "$config_experiment" \
	'type == "object" and .schema_version == 2 and .build.revision == $revision and
	 .build.modified == false and .config.seed == $seed and .config.log_mode == $log_mode and .config.evidence_format == $evidence_format and
	 .config.experiment_id == $experiment' "$cell/manifest.json" >/dev/null || fail "manifest provenance/config mismatch"
[[ "$manifest_revision" == "$metadata_revision" && "$manifest_modified" == false ]] || fail "manifest build identity mismatch"
[[ "$(sha256sum "$cell/run-metadata.json" | awk '{print $1}')" == "$(jq -er '.run_metadata_sha256' "$cell/run-status.json")" ]] || fail "run metadata status hash mismatch"
[[ "$(sha256sum "$cell/manifest.json" | awk '{print $1}')" == "$(jq -er '.manifest_sha256' "$cell/run-status.json")" ]] || fail "manifest status hash mismatch"
	if [[ "$greeks_present" == true ]]; then
		[[ "$(sha256sum "$cell/greeks.json" | awk '{print $1}')" == "$(jq -er '.greeks_sha256' "$cell/run-status.json")" ]] || fail "greeks status hash mismatch"
	fi
[[ "$(sha256sum "$cell/latency.json" | awk '{print $1}')" == "$(jq -er '.latency_sha256' "$cell/run-status.json")" ]] || fail "latency status hash mismatch"
[[ "$(sha256sum "$cell/checkpoints.jsonl" | awk '{print $1}')" == "$(jq -er '.checkpoints_sha256' "$cell/run-status.json")" ]] || fail "checkpoint status hash mismatch"
[[ "$(sha256sum "$cell/evidence-manifest.json" | awk '{print $1}')" == "$(jq -er '.evidence_manifest_sha256' "$cell/run-status.json")" ]] || fail "evidence manifest status hash mismatch"
v2_r2_verify_evidence_manifest "$cell" || fail "runner evidence manifest does not match retained raw evidence: $cell"
v2_r2_verify_attestation "$cell" || fail "external evidence attestation is missing or stale: $cell"

analysis_input_dir="$cell"
rendered_dir=""
cleanup_rendered_input() {
	if [[ -n "$rendered_dir" && -d "$rendered_dir" ]]; then
		rm -rf -- "$rendered_dir"
	fi
}
trap 'cleanup_raw_stage; cleanup_rendered_input' EXIT
runtime_canonical_digest=""
if [[ "$evidence_format" == "evstream_v3" ]]; then
	[[ -x "$renderer" ]] || fail "missing binary evidence renderer: $renderer"
	renderer_go_version=$(v2_r2_binary_go_version "$renderer")
	v2_r2_is_go_127 "$renderer_go_version" || fail "renderer is not built with the pinned Go 1.27 toolchain: $renderer_go_version"
	renderer_revision=$(go version -m "$renderer" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
	renderer_modified=$(go version -m "$renderer" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
	renderer_trimpath=$(go version -m "$renderer" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
	renderer_cgo_enabled=$(go version -m "$renderer" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
	renderer_sha256=$(sha256sum "$renderer" | awk '{print $1}')
	[[ "$renderer_revision" == "$head_revision" && "$renderer_modified" == false && "$renderer_trimpath" == true && "$renderer_cgo_enabled" == 0 ]] ||
		fail "renderer is not a clean reproducible build of current HEAD"
	rendered_dir=$(mktemp -d)
	render_report=$("$renderer" -dir "$cell" -out "$rendered_dir" -route-compression "$render_route_compression") || fail "binary evidence rendering failed"
	jq -e --argjson event_frames "$(jq -er '.event_frames' "$cell/binary-evidence-attestation.json")" \
		--arg execution_hash "$(jq -er '.execution_stream_hash' "$cell/binary-evidence-attestation.json")" \
		--arg canonical_hash "$(jq -er '.canonical_execution_stream_hash' "$cell/binary-evidence-attestation.json")" \
		--arg route_compression "$render_route_compression" \
		'.event_frames == $event_frames and .execution_stream_hash == $execution_hash and
		 .canonical_execution_stream_hash == $canonical_hash and
		 (.routes | type) == "number" and .route_compression == $route_compression' \
		<<<"$render_report" >/dev/null || fail "renderer report does not match binary attestation"
	# Report-derived metrics also consume the immutable instrumented market-data
	# sidecars. Link them into the rendered input namespace without copying them.
	while IFS= read -r name; do
		ln -s -- "$cell/$name" "$rendered_dir/$name"
	done < <(find "$cell" -maxdepth 1 -type f \( -name '*.json' -o -name '*.jsonl' -o -name '*.bin' \) -printf '%f\n' | sort)
	analysis_input_dir="$rendered_dir"
fi

if [[ "$terminal_failure" != true ]]; then
	jq -e --argjson simulation_start_nano "$simulation_start_nano" --argjson simulation_end_nano "$simulation_end_nano" \
		'(.initial_accounts | type == "array" and length > 0 and
		 all(.[]; .account.timestamp == $simulation_start_nano)) and
		(.terminal_accounts | type == "array" and length > 0 and
		 all(.[]; .account.timestamp == $simulation_end_nano))' \
		"$cell/greeks.json" >/dev/null || fail "greeks report does not attest the registered 24-hour horizon"
fi
v2_r2_require_checkpoint_stream "$cell/checkpoints.jsonl" "$simulation_start_nano" "$simulation_end_nano" ||
	fail "checkpoint stream does not attest the registered 24-hour horizon"

for json_file in "$cell"/*.json; do
	[[ -f "$json_file" ]] || continue
	require_json_object "$json_file"
done
derived_artifacts=(
	observationreceipts.json frontiervectors.json mechanical.json conservation.json positions.json
	fillpositions.json orderlifecycle.json lifecycle.json calendar.json settlements.json expiryfills.json
	evidenceartifacthash.json streamhash.json arbitrage.json crossvenue.json roleaudit.json ecology.json
	derivatives.json liquidations.json marginchecks.json optionsurface.json optionliabilityp6.json
	optionvaluetakerp6.json vannavolgap6.json exposure.json hedging.json makerrefresh.json
	makerquotesize.json makerrebalance.json postonly.json liabilityhedger.json perpsignals.json
	datedmandatep5.json fundingcarry.json termcarry.json datedcarryp5.json perpreplenishment.json
	activation.json integrity.json analysis-metadata.json terminalfailure.json
)
if [[ "$extractor_variant" == sv1 ]]; then
	derived_artifacts+=(cdfliquidity.json priceunavailable.json)
fi
for artifact in "${derived_artifacts[@]}"; do
	[[ ! -e "$analysis_dir/$artifact" && ! -e "$analysis_dir/$artifact.err" ]] || fail "refusing to overwrite existing derived evidence: $analysis_dir/$artifact"
done
if find "$cell" -maxdepth 1 -type f -name '*.json.tmp-*' -print -quit | grep -q .; then
	fail "refusing extraction with stale temporary derived evidence in $cell"
fi

required=(
	observationreceipts.json frontiervectors.json mechanical.json conservation.json
	positions.json fillpositions.json orderlifecycle.json lifecycle.json settlements.json
	expiryfills.json evidenceartifacthash.json streamhash.json arbitrage.json crossvenue.json
	roleaudit.json ecology.json derivatives.json liquidations.json marginchecks.json
	optionsurface.json optionliabilityp6.json optionvaluetakerp6.json vannavolgap6.json
	exposure.json hedging.json makerrefresh.json makerquotesize.json makerrebalance.json
	postonly.json liabilityhedger.json perpsignals.json datedmandatep5.json fundingcarry.json
	termcarry.json datedcarryp5.json perpreplenishment.json activation.json integrity.json calendar.json
)
if [[ "$extractor_variant" == sv1 ]]; then
	required+=(cdfliquidity.json priceunavailable.json terminalfailure.json)
	if [[ "$v2_r2_sv1_require_terminal_outcome" == true ]]; then
		required+=(terminal-outcome.json)
	fi
fi

if [[ "$evidence_format" == "evstream_v3" ]]; then
	jq -e '.domain == "canonical_binary_execution_frames" and .ordering == "ordered_stream" and
		 .hashing == "route_sequence_neutral_v1" and
		 (.event_frames | type) == "number" and .event_frames > 0 and
		 (.stream_frames | type) == "number" and .stream_frames >= .event_frames and
		 (.execution_stream_hash | test("^[0-9a-f]{64}$")) and
		 (.canonical_execution_stream_hash | type) == "string" and
		 (.canonical_execution_stream_hash | test("^[0-9a-f]{64}$")) and
		 (.unencodable_payloads // 0) == 0' \
		"$cell/binary-evidence-attestation.json" >/dev/null || fail "invalid runtime binary evidence attestation"
else
	jq -e '.domain == "persisted_json_records" and .ordering == "unordered_multiset" and
		 (.events | type) == "number" and .events > 0 and (.digest | test("^[0-9a-f]{64}$"))' \
		"$cell/evidence-artifact-hash.json" >/dev/null || fail "invalid runtime evidence artifact hash"
fi

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
	fillpositions orderlifecycle lifecycle calendar settlements expiryfills
	evidenceartifacthash streamhash arbitrage crossvenue roleaudit ecology
	derivatives liquidations marginchecks optionsurface optionliabilityp6
	optionvaluetakerp6 vannavolgap6 exposure hedging makerrefresh makerquotesize
	makerrebalance postonly liabilityhedger perpsignals
)
if [[ "$extractor_variant" == sv1 ]]; then
	metrics+=(cdfliquidity priceunavailable)
fi

write_terminal_unavailable_metric() {
	local metric=$1 temporary
	temporary=$(mktemp "$analysis_dir/$metric.json.tmp-XXXXXX")
	jq -n --arg metric "$metric" \
		'{schema_version: 1, result: {status: "UNAVAILABLE_TERMINAL_FAILURE", metric: $metric,
		 terminal_valuation_available: false,
		 reason: "typed terminal economic failure prevented terminal account valuation",
		 observations: 0}}' >"$temporary"
	mv "$temporary" "$analysis_dir/$metric.json"
	require_json_object "$analysis_dir/$metric.json"
}

write_terminal_failure_artifacts() {
	local outcome_json expected_supplier_count population raw_count=0
	local price_unavailable_rejections=0 raw_file price_metric_tmp temporary
	outcome_json=$(jq -c '.' "$cell/terminal-outcome.json")
	expected_supplier_count=$(jq -er '((.elastic_liquidity_suppliers // []) | length) * (.venue_ids | length)' "$cell/run-config.json")
	if (( expected_supplier_count > 0 )); then
		population=treatment
	else
		population=control
	fi
	if [[ -d "$cell/venues" ]]; then
		while IFS= read -r -d '' raw_file; do
			raw_count=$((raw_count + 1))
		done < <(find "$cell/venues" -type f -name '*.jsonl' -print0 | sort -z)
	fi
	# Terminal diagnostics must use the same rendered evidence namespace as the
	# ordinary metrics. In particular, binary execution frames can contain the
	# only OrderRejected record, while raw JSONL may be absent in no-log mode.
	price_metric_tmp=$(mktemp "$analysis_dir/priceunavailable-source-XXXXXX")
	if ! "$analyzer" -metric priceunavailable -json "$analysis_input_dir" >"$price_metric_tmp" 2>"$price_metric_tmp.err"; then
		mv "$price_metric_tmp" "$analysis_dir/priceunavailable-source.invalid"
		mv "$price_metric_tmp.err" "$analysis_dir/priceunavailable-source.error"
		fail "typed PRICE_UNAVAILABLE diagnostic failed on rendered evidence"
	fi
	if ! jq -e '.result.valid == true and
		(.result.order_rejected_count | type) == "number" and
		(.result.price_unavailable_order_rejections | type) == "number" and
		(.result.malformed_order_rejected_count // 0) == 0' "$price_metric_tmp" >/dev/null; then
		mv "$price_metric_tmp" "$analysis_dir/priceunavailable-source.invalid"
		mv "$price_metric_tmp.err" "$analysis_dir/priceunavailable-source.error"
		fail "typed PRICE_UNAVAILABLE diagnostic is invalid on rendered evidence"
	fi
	price_unavailable_rejections=$(jq -er '.result.price_unavailable_order_rejections' "$price_metric_tmp")
	rm -f -- "$price_metric_tmp" "$price_metric_tmp.err"

	for metric in "${metrics[@]}"; do
		write_terminal_unavailable_metric "$metric"
	done

	temporary=$(mktemp "$analysis_dir/cdfliquidity.json.tmp-XXXXXX")
	jq -n --arg population "$population" --argjson expected "$expected_supplier_count" \
		--argjson outcome "$outcome_json" \
		'{schema_version: 1, result: {status: "UNAVAILABLE_TERMINAL_FAILURE", valid: false,
		 measurement_valid: false, terminal_valuation_available: false,
		 population: $population, expected_supplier_count: $expected, supplier_count: null,
		 decision_count: null, fill_count: null, checks: ["terminal economic failure prevented paired CDF account reconciliation"],
		 terminal_outcome: $outcome}}' >"$temporary"
	mv "$temporary" "$analysis_dir/cdfliquidity.json"

	temporary=$(mktemp "$analysis_dir/priceunavailable.json.tmp-XXXXXX")
	jq -n --argjson rejections "$price_unavailable_rejections" --argjson outcome "$outcome_json" \
		'{schema_version: 1, result: {status: "TERMINAL_FAILURE_DIAGNOSTIC", valid: true,
		 measurement_valid: true, terminal_valuation_available: false,
		 price_unavailable_order_rejections: $rejections, malformed_order_rejected_count: 0,
		 terminal_outcome: $outcome}}' >"$temporary"
	mv "$temporary" "$analysis_dir/priceunavailable.json"

		temporary=$(mktemp "$analysis_dir/evidenceartifacthash.json.tmp-XXXXXX")
		jq -n --arg execution_hash "$(jq -er '.execution_stream_hash' "$cell/binary-evidence-attestation.json")" \
			--arg canonical_hash "$(jq -er '.canonical_execution_stream_hash' "$cell/binary-evidence-attestation.json")" \
			--argjson event_frames "$(jq -er '.event_frames' "$cell/binary-evidence-attestation.json")" \
			'{schema_version: 1, result: {domain: "rendered_binary_json_records",
			 ordering: "venue_sequence_reconstructed", source_execution_stream_hash: $execution_hash,
			 source_canonical_execution_stream_hash: $canonical_hash,
			 source_binary_event_frames: $event_frames, status: "TERMINAL_FAILURE_DIAGNOSTIC"}}' >"$temporary"
	mv "$temporary" "$analysis_dir/evidenceartifacthash.json"

		temporary=$(mktemp "$analysis_dir/streamhash.json.tmp-XXXXXX")
		jq -n --arg execution_hash "$(jq -er '.execution_stream_hash' "$cell/binary-evidence-attestation.json")" \
			--arg canonical_hash "$(jq -er '.canonical_execution_stream_hash' "$cell/binary-evidence-attestation.json")" \
			--argjson event_frames "$(jq -er '.event_frames' "$cell/binary-evidence-attestation.json")" \
			--argjson stream_frames "$(jq -er '.stream_frames' "$cell/binary-evidence-attestation.json")" \
			'{schema_version: 1, result: {domain: "canonical_binary_execution_frames", ordering: "ordered_stream", hashing: "route_sequence_neutral_v1",
			 event_frames: $event_frames, stream_frames: $stream_frames, execution_stream_hash: $execution_hash,
			 canonical_execution_stream_hash: $canonical_hash,
			 status: "TERMINAL_FAILURE_DIAGNOSTIC"}}' >"$temporary"
	mv "$temporary" "$analysis_dir/streamhash.json"

	temporary=$(mktemp "$analysis_dir/activation.json.tmp-XXXXXX")
	jq -n --arg contract "$contract_version" --arg population "$population" \
		--argjson expected "$expected_supplier_count" --argjson rejections "$price_unavailable_rejections" \
		--argjson outcome "$outcome_json" \
		'{schema_version: 1, result: {contract: $contract, status: "terminal_failure_diagnostic",
		 terminal_valuation_available: false, terminal_outcome: $outcome,
		 cdf_liquidity: {valid: false, population: $population, expected_supplier_count: $expected,
		  supplier_count: null, activation_observed: false, measurement_valid: false},
		 observed: {cdf_liquidity_activation_observed: false, price_unavailable_order_rejections: $rejections},
		 predicates: {cdf_liquidity_population_contract: false,
		  zero_price_unavailable_order_rejections: ($rejections == 0), calendar_behavior_attested: false}}}' >"$temporary"
	mv "$temporary" "$analysis_dir/activation.json"

	temporary=$(mktemp "$analysis_dir/integrity.json.tmp-XXXXXX")
	jq -n --arg contract "$contract_version" --argjson outcome "$outcome_json" \
		'{schema_version: 1, contract: $contract, status: "terminal_failure_diagnostic",
		 terminal_valuation_available: false, terminal_measurement_valid: true,
		 terminal_outcome: $outcome, predicates: {terminal_failure_endpoint: true},
		 unavailable_metrics: ["standard_integrity", "calendar", "risk", "settlement", "cdf_liquidity"]}' >"$temporary"
	mv "$temporary" "$analysis_dir/integrity.json"

	temporary=$(mktemp "$analysis_dir/terminalfailure.json.tmp-XXXXXX")
	jq -n --arg contract "$contract_version" --argjson outcome "$outcome_json" \
		--argjson raw_count "$raw_count" --argjson event_frames "$(jq -er '.event_frames' "$cell/binary-evidence-attestation.json")" \
		'{schema_version: 1, result: {contract: $contract, status: "VALID_TERMINAL_ECONOMIC_FAILURE",
		 terminal_valuation_available: false, terminal_outcome: $outcome,
		 raw_evidence_contract_valid: true, evidence_manifest_verified: true,
		 external_attestation_verified: true, checkpoint_contract_valid: true,
		 standard_metrics: "not_run", raw_jsonl_files: $raw_count, event_frames: $event_frames,
		 claim_scope: "typed terminal failure diagnostic; no terminal valuation claim"}}' >"$temporary"
	mv "$temporary" "$analysis_dir/terminalfailure.json"

		runtime_events=$(jq -er '.event_frames' "$cell/binary-evidence-attestation.json")
		runtime_digest=$(jq -er '.execution_stream_hash' "$cell/binary-evidence-attestation.json")
		runtime_canonical_digest=$(jq -er '.canonical_execution_stream_hash' "$cell/binary-evidence-attestation.json")
	analyzer_revision=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
	analyzer_modified=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
	analyzer_sha256=$(sha256sum "$analyzer" | awk '{print $1}')
	analyzer_trimpath=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
	analyzer_cgo_enabled=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
	[[ "$analyzer_revision" == "$head_revision" && "$analyzer_modified" == false && "$analyzer_trimpath" == true && "$analyzer_cgo_enabled" == 0 ]] ||
		fail "analyzer is not a clean reproducible build of current HEAD"
	analyzer_modified_json=false
	[[ "$analyzer_modified" == true ]] && analyzer_modified_json=true
	required_json=$(printf '%s\n' "${required[@]}" | jq -Rsc 'split("\n") | map(select(length > 0))')
	artifact_sha256=$(for artifact in "${required[@]}"; do
		printf '%s\t%s\n' "$artifact" "$(sha256sum "$analysis_dir/$artifact" | awk '{print $1}')"
	done | jq -Rn 'reduce inputs as $line ({}; ($line | split("\t")) as $parts | .[$parts[0]] = $parts[1])')
	temporary=$(mktemp "$analysis_dir/analysis-metadata.json.tmp-XXXXXX")
	jq -n \
		--arg analysis_revision "$head_revision" --arg analyzer_revision "$analyzer_revision" \
		--arg analyzer_sha256 "$analyzer_sha256" --argjson analyzer_trimpath true \
		--arg analyzer_cgo_enabled "$analyzer_cgo_enabled" --arg analyzer_go_version "$analyzer_go_version" \
		--arg evidence_format "$evidence_format" --arg renderer_revision "$renderer_revision" \
		--arg renderer_sha256 "$renderer_sha256" --arg renderer_go_version "$renderer_go_version" \
		--arg renderer_route_compression "$render_route_compression" --arg source_revision_mode "$source_revision_mode" \
		--arg raw_source_revision "$metadata_revision" --argjson analyzer_modified "$analyzer_modified_json" \
		--argjson required_artifacts "$required_json" --argjson artifact_sha256 "$artifact_sha256" \
			--argjson runtime_evidence_events "$runtime_events" --arg runtime_evidence_digest "$runtime_digest" \
			--arg runtime_canonical_digest "$runtime_canonical_digest" \
		--arg contract "$contract_version" --arg cell "$cell_name" --argjson seed "$seed" \
		--arg simulator_revision "$metadata_revision" --arg simulator_sha256 "$simulator_sha256" \
		--argjson simulator_trimpath true --arg simulator_cgo_enabled "$binary_cgo_enabled" \
		--arg simulator_go_version "$simulator_go_version" --arg prunegate_revision "$prunegate_revision" \
		--arg prunegate_sha256 "$prunegate_sha256" --argjson prunegate_trimpath true \
		--arg prunegate_cgo_enabled "$prunegate_cgo_enabled" --arg prunegate_go_version "$prunegate_go_version" \
		--arg config_sha256 "$config_sha256" --argjson completion_sentinels "$completion_sentinels" \
		'{schema_version: 3, cell: $cell, seed: $seed, evidence_format: $evidence_format,
		 analysis_revision: $analysis_revision, analyzer_revision: $analyzer_revision,
		 analyzer_sha256: $analyzer_sha256, analyzer_vcs_modified: $analyzer_modified,
		 analyzer_trimpath: $analyzer_trimpath, analyzer_cgo_enabled: $analyzer_cgo_enabled,
		 analyzer_go_version: $analyzer_go_version, renderer_revision: $renderer_revision,
		 renderer_sha256: $renderer_sha256, renderer_go_version: $renderer_go_version,
		 renderer_route_compression: $renderer_route_compression, source_revision_mode: $source_revision_mode,
		 raw_source_revision: $raw_source_revision, require_exact_replay: true,
		 simulator_revision: $simulator_revision, simulator_sha256: $simulator_sha256,
		 simulator_trimpath: $simulator_trimpath, simulator_cgo_enabled: $simulator_cgo_enabled,
		 simulator_go_version: $simulator_go_version, prunegate_revision: $prunegate_revision,
		 prunegate_sha256: $prunegate_sha256, prunegate_trimpath: $prunegate_trimpath,
		 prunegate_cgo_enabled: $prunegate_cgo_enabled, prunegate_go_version: $prunegate_go_version,
		 config_sha256: $config_sha256, analysis_contract: $contract, integrity_contract: $contract,
		 activation_contract: $contract, completion_sentinels: $completion_sentinels,
		 required_artifacts: $required_artifacts, artifact_sha256: $artifact_sha256,
			runtime_evidence_artifact: {representation: $evidence_format, event_frames: $runtime_evidence_events,
			 execution_stream_hash: $runtime_evidence_digest, canonical_execution_stream_hash: $runtime_canonical_digest}, terminal_failure_diagnostic: true,
		 inactive_contracts: ["fundingcarry", "termcarry", "datedcarryp5", "datedmandatep5", "perpreplenishment"],
		 raw_log_policy: "retained; this extractor has no prune authority"}' >"$temporary"
	mv "$temporary" "$analysis_dir/analysis-metadata.json"
	require_json_object "$analysis_dir/analysis-metadata.json"
	jq -e --arg revision "$head_revision" --arg analyzer_revision "$analyzer_revision" \
		--arg contract "$contract_version" --arg renderer_route_compression "$render_route_compression" \
		--arg source_revision_mode "$source_revision_mode" --arg raw_source_revision "$metadata_revision" \
		--argjson completion_sentinels "$completion_sentinels" --argjson required_artifacts "$required_json" \
		'.schema_version == 3 and .evidence_format == "evstream_v3" and .analysis_revision == $revision and
		 .analyzer_revision == $analyzer_revision and .analyzer_vcs_modified == false and
		 .require_exact_replay == true and .renderer_route_compression == $renderer_route_compression and
		 .source_revision_mode == $source_revision_mode and .raw_source_revision == $raw_source_revision and
		 .analysis_contract == $contract and .required_artifacts == $required_artifacts and
		 .completion_sentinels == $completion_sentinels and
		 (.artifact_sha256 | keys) == ($required_artifacts | sort) and
		 all(.artifact_sha256 | to_entries[]; (.value | test("^[0-9a-f]{64}$"))) and
		 (.raw_log_policy | type) == "string"' "$analysis_dir/analysis-metadata.json" >/dev/null ||
		fail "terminal-failure analysis metadata self-check failed"
	v2_r2_verify_evidence_manifest "$cell" || fail "raw evidence changed during terminal-failure extraction: $cell"
	v2_r2_verify_attestation "$cell" || fail "external attestation changed during terminal-failure extraction: $cell"
	printf 'extracted terminal economic failure diagnostic: %s\n' "$cell"
}

if [[ "$terminal_failure" == true ]]; then
	write_terminal_failure_artifacts
	exit 0
fi

for metric in "${metrics[@]}"; do
	if [[ "$metric" == positions || "$metric" == settlements ]]; then
		if [[ "$metric" == settlements ]]; then
			write_metric "$analysis_dir/$metric.json" "$analyzer" -metric "$metric" -require-exact-replay -delivery-fee-policy "$delivery_fee_policy" -json "$analysis_input_dir" ||
				fail "analyzer metric failed: $metric"
		else
				write_metric "$analysis_dir/$metric.json" "$analyzer" -metric "$metric" -require-exact-replay -json "$analysis_input_dir" ||
					fail "analyzer metric failed: $metric"
		fi
	elif [[ "$metric" == derivatives ]]; then
		write_metric "$analysis_dir/$metric.json" "$analyzer" -metric "$metric" -require-exact-replay -funding-interval-seconds "$funding_interval_seconds" -json "$analysis_input_dir" ||
			fail "analyzer metric failed: $metric"
	else
		write_metric "$analysis_dir/$metric.json" "$analyzer" -metric "$metric" -json "$analysis_input_dir" ||
			fail "analyzer metric failed: $metric"
	fi
done
if [[ "$evidence_format" == "evstream_v3" ]]; then
	rendered_hash=$(jq -er '.execution_stream_hash' "$cell/binary-evidence-attestation.json")
	rendered_canonical_hash=$(jq -er '.canonical_execution_stream_hash' "$cell/binary-evidence-attestation.json")
	rendered_frames=$(jq -er '.event_frames' "$cell/binary-evidence-attestation.json")
	rendered_tmp=$(mktemp "$analysis_dir/evidenceartifacthash.json.tmp-XXXXXX")
	jq --arg execution_hash "$rendered_hash" --arg canonical_hash "$rendered_canonical_hash" --argjson event_frames "$rendered_frames" \
		'.result.domain = "rendered_binary_json_records" |
		 .result.ordering = "venue_sequence_reconstructed" |
		 .result.source_execution_stream_hash = $execution_hash |
		 .result.source_canonical_execution_stream_hash = $canonical_hash |
		 .result.source_binary_event_frames = $event_frames' \
		"$analysis_dir/evidenceartifacthash.json" >"$rendered_tmp"
	mv "$rendered_tmp" "$analysis_dir/evidenceartifacthash.json"
fi

v2_r2_require_calendar_listing_timeline "$analysis_dir/calendar.json" "$expected_calendar_listing_timeline" ||
	fail "calendar first-listing timeline or per-expiry cardinality does not match the registered policy"
v2_r2_require_calendar_venue_set "$analysis_dir/calendar.json" ||
	fail "calendar derivative evidence does not contain exactly the registered central/north/south venues"

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
	write_metric "$analysis_dir/fundingcarry.json" "$analyzer" -metric fundingcarry -json "$analysis_input_dir" || fail "fundingcarry failed"
fi
if [[ "$(jq -r '.record_term_carry_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive termcarry record_term_carry_decisions "registered integrated composition does not enable P5 actor decision receipts"
else
	write_metric "$analysis_dir/termcarry.json" "$analyzer" -metric termcarry -json "$analysis_input_dir" || fail "termcarry failed"
fi
if [[ "$(jq -r '.record_dated_term_carry_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive datedcarryp5 record_dated_term_carry_decisions "registered integrated composition does not enable P5 dated-carry decision receipts"
else
	write_metric "$analysis_dir/datedcarryp5.json" "$analyzer" -metric datedcarryp5 -json "$analysis_input_dir" || fail "datedcarryp5 failed"
fi
if [[ "$(jq -r '.record_dated_execution_mandate_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive datedmandatep5 record_dated_execution_mandate_decisions "registered integrated composition does not enable P5 dated-execution mandate receipts"
else
	write_metric "$analysis_dir/datedmandatep5.json" "$analyzer" -metric datedmandatep5 -json "$analysis_input_dir" || fail "datedmandatep5 failed"
fi
if [[ "$(jq -r '.record_perp_maker_replenishment_decisions' "$cell/run-config.json")" != true ]]; then
	write_inactive perpreplenishment record_perp_maker_replenishment_decisions "registered integrated composition does not enable P3 replenishment receipts"
else
	write_metric "$analysis_dir/perpreplenishment.json" "$analyzer" -metric perpreplenishment -json "$analysis_input_dir" || fail "perpreplenishment failed"
fi

raw_count=0
cdf_borrow_events=0
price_unavailable_rejections=0
cdf_liquidity_activation_observed=false
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
	if [[ "$extractor_variant" != sv1 ]]; then
		count=$(jq -c '
			def payload: (.data.payload // .data // {});
			select(.event == "OrderRejected" and
				((payload.error // payload.payload.error // .data.error // .error // "") == "PRICE_UNAVAILABLE")) | 1' "$raw_file" | wc -l)
		price_unavailable_rejections=$((price_unavailable_rejections + count))
	fi
done < <(find "$cell/venues" -type f -name '*.jsonl' -print0 | sort -z)
[[ "$raw_count" -gt 0 ]] || fail "no raw JSONL evidence files found"
if [[ "$extractor_variant" == sv1 ]]; then
	jq -e '.result.valid == true and
		(.result.price_unavailable_order_rejections | type) == "number" and
		(.result.malformed_order_rejected_count // 0) == 0' \
		"$analysis_dir/priceunavailable.json" >/dev/null ||
		fail "typed PRICE_UNAVAILABLE rejection audit is invalid"
	price_unavailable_rejections=$(jq -er '.result.price_unavailable_order_rejections' "$analysis_dir/priceunavailable.json")
fi

activation_tmp=$(mktemp "$analysis_dir/activation.json.tmp-XXXXXX")
activation_filter="$(v2_r2_calendar_listing_timeline_jq_definition)"
jq_args=(
	-n --argjson cdf_borrow_events "$cdf_borrow_events"
	--argjson price_unavailable_rejections "$price_unavailable_rejections"
	--argjson enabled_cross_asset_spot_graph "$(jq -er '.cross_asset_spot_graph' "$cell/run-config.json")"
	--argjson enabled_cross_asset_collateral_marks "$(jq -er '.cross_asset_collateral_marks' "$cell/run-config.json")"
	--slurpfile calendar "$analysis_dir/calendar.json"
	--argjson expected_calendar_expiries "$expected_calendar_expiries"
	--argjson expected_calendar_completed_expiries "$expected_calendar_completed_expiries"
	--argjson expected_calendar_listing_timeline "$expected_calendar_listing_timeline"
	--argjson max_option_listing_delay_nano "$(v2_r2_calendar_option_listing_max_delay_nano)"
	--argjson expected_calendar_venue_ids "$(v2_r2_expected_calendar_venue_ids)"
	--arg contract "$contract_version"
)
if [[ "$extractor_variant" == sv1 ]]; then
	expected_cdf_supplier_count=$(jq -er '((.elastic_liquidity_suppliers // []) | length) * (.venue_ids | length)' "$cell/run-config.json")
	[[ "$expected_cdf_supplier_count" =~ ^[0-9]+$ ]] || fail "SV1 config has an invalid CDF supplier roster count"
	cdf_liquidity_population="control"
	cdf_liquidity_population_contract=false
	if [[ "$expected_cdf_supplier_count" -gt 0 ]]; then
		v2_r2_require_cdf_supplier_activation "$analysis_dir/cdfliquidity.json" "$expected_cdf_supplier_count" ||
			fail "SV1 treatment CDF liquidity activation contract not satisfied"
		cdf_liquidity_population="treatment"
		cdf_liquidity_population_contract=true
		cdf_liquidity_activation_observed=true
	else
		v2_r2_require_cdf_supplier_control "$analysis_dir/cdfliquidity.json" ||
			fail "SV1 control CDF population contract not satisfied"
		cdf_liquidity_population_contract=true
	fi
	jq_args+=(--slurpfile cdf_liquidity "$analysis_dir/cdfliquidity.json")
	jq_args+=(--arg cdf_liquidity_population "$cdf_liquidity_population")
	jq_args+=(--argjson expected_cdf_supplier_count "$expected_cdf_supplier_count")
	jq_args+=(--argjson cdf_liquidity_population_contract "$cdf_liquidity_population_contract")
	activation_filter+='def r($x): $x[0].result;
	 {schema_version: 1, result: {contract: $contract,
	 cdf_collateral_borrowing: {events: $cdf_borrow_events,
		 enabled_cross_asset_spot_graph: $enabled_cross_asset_spot_graph,
		 enabled_cross_asset_collateral_marks: $enabled_cross_asset_collateral_marks},
		 cdf_liquidity: {valid: r($cdf_liquidity).valid, population: $cdf_liquidity_population,
			expected_supplier_count: $expected_cdf_supplier_count, supplier_count: r($cdf_liquidity).supplier_count,
			 decision_count: r($cdf_liquidity).decision_count, fill_count: r($cdf_liquidity).fill_count,
		 trading_supplier_count: r($cdf_liquidity).trading_supplier_count,
			 pnl_changing_supplier_count: r($cdf_liquidity).pnl_changing_supplier_count,
				 inventory_responsive_decision_count: r($cdf_liquidity).inventory_responsive_decision_count,
				 risk_state_decision_count: r($cdf_liquidity).risk_state_decision_count,
				 fresh_risk_state_decision_count: r($cdf_liquidity).fresh_risk_state_decision_count,
				 risk_limit_triggered_decision_count: r($cdf_liquidity).risk_limit_triggered_decision_count,
				 max_observed_loss_from_initial_quote: r($cdf_liquidity).max_observed_loss_from_initial_quote,
				 max_observed_drawdown_quote: r($cdf_liquidity).max_observed_drawdown_quote,
				 cancel_count: r($cdf_liquidity).cancel_count, withdraw_count: r($cdf_liquidity).withdraw_count,
				 withdrawal_without_replacement_count: r($cdf_liquidity).withdrawal_without_replacement_count,
				 censored_withdrawal_count: r($cdf_liquidity).censored_withdrawal_count,
			 max_borrowed: r($cdf_liquidity).max_borrowed,
			 supplier_volume_share: r($cdf_liquidity).supplier_volume_share,
			 supplier_depth_over_75_share: r($cdf_liquidity).supplier_depth_over_75_share,
			 supplier_time_weighted_resting_depth_share: r($cdf_liquidity).supplier_time_weighted_resting_depth_share,
				 supplier_removal_counterfactual_valid: r($cdf_liquidity).supplier_removal_counterfactual_valid,
				 supplier_removal_time_weighted_counterfactual_valid: r($cdf_liquidity).supplier_removal_time_weighted_counterfactual_valid,
				 supplier_removal_snapshot_count: r($cdf_liquidity).supplier_removal_snapshot_count,
				 supplier_removal_observed_duration_ns: r($cdf_liquidity).supplier_removal_observed_duration_ns,
				 supplier_removal_bid_absence_duration_ns: r($cdf_liquidity).supplier_removal_bid_absence_duration_ns,
				 supplier_removal_ask_absence_duration_ns: r($cdf_liquidity).supplier_removal_ask_absence_duration_ns,
				 supplier_removal_qualified_bid_absence_duration_ns: r($cdf_liquidity).supplier_removal_qualified_bid_absence_duration_ns,
				 supplier_removal_qualified_ask_absence_duration_ns: r($cdf_liquidity).supplier_removal_qualified_ask_absence_duration_ns,
				 supplier_removal_bid_absence_fraction: r($cdf_liquidity).supplier_removal_bid_absence_fraction,
				 supplier_removal_ask_absence_fraction: r($cdf_liquidity).supplier_removal_ask_absence_fraction,
				 supplier_removal_qualified_bid_absence_fraction: r($cdf_liquidity).supplier_removal_qualified_bid_absence_fraction,
				 supplier_removal_qualified_ask_absence_fraction: r($cdf_liquidity).supplier_removal_qualified_ask_absence_fraction,
				 supplier_removal_bid_absence_active_time_fraction: r($cdf_liquidity).supplier_removal_bid_absence_active_time_fraction,
				 supplier_removal_ask_absence_active_time_fraction: r($cdf_liquidity).supplier_removal_ask_absence_active_time_fraction,
				 supplier_removal_qualified_bid_absence_active_time_fraction: r($cdf_liquidity).supplier_removal_qualified_bid_absence_active_time_fraction,
				 supplier_removal_qualified_ask_absence_active_time_fraction: r($cdf_liquidity).supplier_removal_qualified_ask_absence_active_time_fraction,
			 supplier_removal_one_sided_snapshots: r($cdf_liquidity).supplier_removal_one_sided_snapshots,
				 venues: (r($cdf_liquidity).venues | map({venue_id, supplier_volume_share, supplier_depth_over_75_fraction,
				 supplier_time_weighted_resting_depth_share, supplier_removal_counterfactual_valid,
				 supplier_removal_time_weighted_counterfactual_valid, supplier_removal_snapshot_count,
				 supplier_removal_observed_duration_ns, supplier_removal_bid_absence_duration_ns,
				 supplier_removal_ask_absence_duration_ns, supplier_removal_qualified_bid_absence_duration_ns,
				 supplier_removal_qualified_ask_absence_duration_ns, supplier_removal_bid_absence_active_time_fraction,
				 supplier_removal_ask_absence_active_time_fraction, supplier_removal_qualified_bid_absence_active_time_fraction,
				 supplier_removal_qualified_ask_absence_active_time_fraction, supplier_removal_bid_absence_fraction,
				 supplier_removal_ask_absence_fraction, supplier_removal_qualified_bid_absence_fraction,
				 supplier_removal_qualified_ask_absence_fraction, supplier_removal_one_sided_snapshots})),
			 checks: (r($cdf_liquidity).checks // [] | length)},
	 price_unavailable_order_rejections: $price_unavailable_rejections,
	 calendar: (r($calendar)),
		 observed: {cdf_liquidity_activation_observed: $cdf_liquidity_activation_observed},
		 predicates: {cdf_liquidity_population_contract: $cdf_liquidity_population_contract,
			 zero_price_unavailable_order_rejections: ($price_unavailable_rejections == 0),
		 calendar_behavior_attested: (r($calendar).contract == "calendar-audit-v2" and
			 all(r($calendar).venues[]; calendar_listing_timeline_matches(.listing_timeline; $expected_calendar_listing_timeline)) and
			 (r($calendar).venues | map(.venue_id) | sort) == ($expected_calendar_venue_ids | sort) and
			 r($calendar).futures_expiry_nanos == $expected_calendar_expiries and
			 r($calendar).option_expiry_nanos == $expected_calendar_expiries and
			 r($calendar).shared_expiry_nanos == $expected_calendar_expiries and
			 (r($calendar).venues | length) == 3 and
			 all(r($calendar).venues[];
				 .futures_expiry_nanos == $expected_calendar_expiries and
				 .option_expiry_nanos == $expected_calendar_expiries and
				 .shared_expiry_nanos == $expected_calendar_expiries and
				 .futures_listed == 28 and .options_listed == 280 and
				 .futures_settled == 23 and .options_settled == 230 and
				 .future_expiry_cycles == 23 and .option_expiry_cycles == 23 and
				 .duplicate_future_listings == 0 and .duplicate_option_listings == 0 and
				 .duplicate_future_settlements == 0 and .duplicate_option_settlements == 0 and
				 .settlement_without_listing == 0 and .settlement_before_listing == 0 and
				 .malformed_derivative_events == 0 and
				 .max_simultaneous_future_expiries >= 3 and
				 .max_simultaneous_option_expiries >= 3 and
				 .future_expiry_cycles == ($expected_calendar_completed_expiries | length) and
				 .option_expiry_cycles == ($expected_calendar_completed_expiries | length)))}}}'
	jq_args+=(--argjson cdf_liquidity_activation_observed "$cdf_liquidity_activation_observed")
else
	activation_filter+='def r($x): $x[0].result;
	 {schema_version: 1, result: {contract: $contract,
	 cdf_collateral_borrowing: {events: $cdf_borrow_events,
		 enabled_cross_asset_spot_graph: $enabled_cross_asset_spot_graph,
		 enabled_cross_asset_collateral_marks: $enabled_cross_asset_collateral_marks},
	 price_unavailable_order_rejections: $price_unavailable_rejections,
	 calendar: (r($calendar)),
	 predicates: {cdf_collateral_borrowing_observed: ($cdf_borrow_events > 0 and
		 $enabled_cross_asset_spot_graph and $enabled_cross_asset_collateral_marks),
		 zero_price_unavailable_order_rejections: ($price_unavailable_rejections == 0),
		 calendar_behavior_attested: (r($calendar).contract == "calendar-audit-v2" and
			 all(r($calendar).venues[]; calendar_listing_timeline_matches(.listing_timeline; $expected_calendar_listing_timeline)) and
			 (r($calendar).venues | map(.venue_id) | sort) == ($expected_calendar_venue_ids | sort) and
			 r($calendar).futures_expiry_nanos == $expected_calendar_expiries and
			 r($calendar).option_expiry_nanos == $expected_calendar_expiries and
			 r($calendar).shared_expiry_nanos == $expected_calendar_expiries and
			 (r($calendar).venues | length) == 3 and
			 all(r($calendar).venues[];
				 .futures_expiry_nanos == $expected_calendar_expiries and
				 .option_expiry_nanos == $expected_calendar_expiries and
				 .shared_expiry_nanos == $expected_calendar_expiries and
				 .futures_listed == 28 and .options_listed == 280 and
				 .futures_settled == 23 and .options_settled == 230 and
				 .future_expiry_cycles == 23 and .option_expiry_cycles == 23 and
				 .duplicate_future_listings == 0 and .duplicate_option_listings == 0 and
				 .duplicate_future_settlements == 0 and .duplicate_option_settlements == 0 and
				 .settlement_without_listing == 0 and .settlement_before_listing == 0 and
				 .malformed_derivative_events == 0 and
				 .max_simultaneous_future_expiries >= 3 and
				 .max_simultaneous_option_expiries >= 3 and
				 .future_expiry_cycles == ($expected_calendar_completed_expiries | length) and
				 .option_expiry_cycles == ($expected_calendar_completed_expiries | length)))}}}'
fi
jq "${jq_args[@]}" "$activation_filter" >"$activation_tmp"
mv "$activation_tmp" "$analysis_dir/activation.json"
require_json_object "$analysis_dir/activation.json"
if [[ "$extractor_variant" == sv1 ]]; then
	jq -e '(.result.predicates | length) == 3 and
		(.result.predicates | keys) == ["calendar_behavior_attested", "cdf_liquidity_population_contract", "zero_price_unavailable_order_rejections"] and
		(.result.observed.cdf_liquidity_activation_observed | type) == "boolean" and
		all(.result.predicates | to_entries[]; .value == true)' "$analysis_dir/activation.json" >/dev/null ||
		fail "SV1 population activation contract not satisfied"
	else
	jq -e '(.result.predicates | length) == 3 and
		(.result.predicates | keys) == ["calendar_behavior_attested", "cdf_collateral_borrowing_observed", "zero_price_unavailable_order_rejections"] and
		all(.result.predicates | to_entries[]; .value == true)' "$analysis_dir/activation.json" >/dev/null ||
		fail "candidate activation contract not satisfied"
fi

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
	--slurpfile calendar "$analysis_dir/calendar.json" \
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
	--argjson expected_calendar_listing_timeline "$expected_calendar_listing_timeline" \
		--argjson max_option_listing_delay_nano "$(v2_r2_calendar_option_listing_max_delay_nano)" \
		--argjson expected_calendar_venue_ids "$(v2_r2_expected_calendar_venue_ids)" \
	--arg contract "$contract_version" \
	--arg extractor_variant "$extractor_variant" \
	"$(v2_r2_calendar_listing_timeline_jq_definition)"'def r($x): $x[0].result;
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
			mechanical: (count($mechanical; "orders") > 0 and zero($mechanical; "drift.mismatches") and zero($mechanical; "drift.malformed_records") and zero($mechanical; "drift.malformed_deltas") and zero($mechanical; "drift.malformed_trades") and zero($mechanical; "drift.malformed_accepts") and zero($mechanical; "drift.malformed_snapshots")),
			conservation: (count($conservation; "flows") > 0 and count($conservation; "identities") > 0 and count($conservation; "venue_identities") > 0 and count($conservation; "delta_consistency.checked") > 0 and count($conservation; "delta_consistency.trading_fee_events") > 0 and count($conservation; "delta_consistency.margin_interest_events") > 0 and count($conservation; "delta_consistency.funding_remainder_records") > 0 and zero($conservation; "delta_consistency.mismatched") and zero($conservation; "delta_consistency.chain_broken") and zero($conservation; "delta_consistency.decode_failures") and zero($conservation; "delta_consistency.malformed_venue_records") and zero($conservation; "delta_consistency.malformed_fee_records") and zero($conservation; "delta_consistency.venue_balance_mismatches") and zero($conservation; "delta_consistency.fee_revenue_mismatches") and zero($conservation; "delta_consistency.trading_fee_mismatches") and zero($conservation; "delta_consistency.margin_interest_mismatches") and zero($conservation; "delta_consistency.funding_remainder_mismatches") and zero($conservation; "delta_consistency.funding_wallet_mismatches") and zero($conservation; "delta_consistency.unsupported_revenue_records") and zero($conservation; "delta_consistency.malformed_interest_records") and zero($conservation; "delta_consistency.margin_interest_failures") and zero($conservation; "delta_consistency.duplicate_fee_identities") and zero($conservation; "delta_consistency.duplicate_fee_movements") and zero($conservation; "delta_consistency.malformed_venue_ledgers") and zero($conservation; "delta_consistency.venue_terminal_sequence_missing") and zero($conservation; "delta_consistency.venue_order_mismatches") and zero($conservation; "delta_consistency.venue_sequence_mismatches") and zero($conservation; "delta_consistency.venue_chain_mismatches") and zero($conservation; "delta_consistency.arithmetic_failures") and residuals_within(r($conservation).identities) and residuals_within(r($conservation).venue_identities)),
			position_rounding: (r($conservation).position_rounding.valid == true and count($conservation; "position_rounding.events") > 0 and field_zeroes($conservation; ["position_rounding.invalid", "position_rounding.remainder_out_of_range", "position_rounding.duplicate_terminal_keys", "position_rounding.balance_link_failures", "position_rounding.venue_link_failures", "position_rounding.asset_wallet_failures", "position_rounding.venue_bucket_failures"])),
			positions: (count($positions; "contracts") > 0 and count($positions; "exact_replay_checks") > 0 and count($positions; "realized_pnl_checks") > 0 and field_zeroes($positions; ["non_zero_net_contracts", "disagreement", "unrepresentable_open_values", "exact_replay_failures", "realized_pnl_failures", "evidence_failures", "missing_marks", "mark_identity_failures", "missing_terminal_positions", "unexpected_terminal_positions", "terminal_position_mismatches", "terminal_timestamp_failures", "post_terminal_position_updates"])),
			fill_positions: (zero($fillpositions; "missing_position_update") and zero($fillpositions; "unexpected_position_update") and zero($fillpositions; "price_mismatches") and zero($fillpositions; "malformed_fill_records") and zero($fillpositions; "malformed_position_updates") and zero($fillpositions; "position_chain_failures")),
			order_lifecycle: field_zeroes($orderlifecycle; ["unknown_fills", "unknown_cancellations", "duplicate_acceptances", "duplicate_terminals", "fills_after_terminal", "fill_quantity_mismatches", "cancel_quantity_mismatches", "client_mismatches", "unlinked_fills", "missing_immediate_terminal", "malformed_accepted_records", "malformed_fill_records", "malformed_cancel_records", "malformed_liquidation_records"]),
			calendar: (r($calendar).contract == "calendar-audit-v2" and count($calendar; "venues") > 0 and zero($calendar; "duplicate_listings") and zero($calendar; "duplicate_settlements") and zero($calendar; "malformed_derivative_events") and (r($calendar).venues | map(.venue_id) | sort) == ($expected_calendar_venue_ids | sort) and all(r($calendar).venues[]; calendar_listing_timeline_matches(.listing_timeline; $expected_calendar_listing_timeline))),
			settlement: (count($settlements; "checks") > 0 and count($settlements; "exact_replay_checks") > 0 and field_zeroes($settlements; ["mismatched", "unpaid", "total_trades_after_expiry", "total_position_updates_after_expiry", "arithmetic_failures", "explicit_unavailable_announcements", "exact_replay_failures", "settlement_event_mismatches", "evidence_failures", "descriptor_conflicts", "settlement_timing_failures", "delivery_fee_mismatches"])),
			expiry: (count($expiryfills; "expired_contracts") > 0 and count($expiryfills; "settled_contracts") > 0 and zero($expiryfills; "expired_unsettled_contracts") and field_zeroes($expiryfills; ["fills_before_listing", "fills_after_expiry", "malformed_fill_records", "fill_identity_failures", "malformed_lifecycle_records", "malformed_snapshot_records", "settlement_before_listing", "missing_expiry_metadata", "settlement_without_listing", "metadata_mismatches", "snapshot_records_before_listing", "snapshot_records_after_expiry", "nonempty_snapshots_after_expiry"])),
				derivatives: (count($derivatives; "funding") > 0 and field_zeroes($derivatives; ["funding_broken", "funding_sign_wrong", "funding_misdirected", "funding_undirected", "funding_duplicate_payments", "funding_payment_mismatches", "funding_missing_rates", "funding_missing_settlements", "funding_timing_failures", "funding_evidence_failures", "funding_arithmetic_failures", "funding_settlement_failures", "exercise_broken", "exercise_timing_failures", "exercise_evidence_failures", "holders_mispaid", "worthless_paid", "exercise_arithmetic_failures", "exercise_missing_payouts", "exercise_extra_payouts", "exercise_duplicate_payouts", "exercise_unknown_payouts"])),
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
		activation: (if $extractor_variant == "sv1" then r($activation).predicates.cdf_liquidity_population_contract == true and r($activation).predicates.zero_price_unavailable_order_rejections == true else r($activation).predicates.cdf_collateral_borrowing_observed == true and r($activation).predicates.zero_price_unavailable_order_rejections == true end),
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
jq -e '(.predicates | keys) == ["activation", "calendar", "conservation", "derivatives", "expiry", "exposure", "fill_positions", "frontier_vectors", "hedging", "late_path", "liability_hedger", "liquidations", "maker_quote_size", "maker_rebalance", "maker_refresh", "margin", "mechanical", "observation_receipts", "option_liability", "option_surface", "option_value_taker", "order_lifecycle", "position_rounding", "positions", "post_only", "settlement", "vanna_volga"] and
   all(.predicates | to_entries[]; .value == true)' "$analysis_dir/integrity.json" >/dev/null ||
	fail "one or more fail-closed integrity predicates failed"

terminalfailure_tmp=$(mktemp "$analysis_dir/terminalfailure.json.tmp-XXXXXX")
if [[ "$extractor_variant" == sv1 && "$v2_r2_sv1_require_terminal_outcome" == true ]]; then
	jq -n --arg contract "$contract_version" --slurpfile outcome "$analysis_dir/terminal-outcome.json" \
		'{schema_version: 1, result: {contract: $contract, status: "NO_TERMINAL_FAILURE",
		 terminal_valuation_available: true, terminal_outcome: $outcome[0],
		 raw_evidence_contract_valid: true, evidence_manifest_verified: true,
		 external_attestation_verified: true, checkpoint_contract_valid: true,
		 standard_metrics: "complete", claim_scope: "complete terminal valuation"}}' >"$terminalfailure_tmp"
else
	jq -n --arg contract "$contract_version" \
		'{schema_version: 1, result: {contract: $contract, status: "NO_TERMINAL_FAILURE",
		 terminal_valuation_available: true, terminal_outcome: null,
		 raw_evidence_contract_valid: true, evidence_manifest_verified: true,
		 external_attestation_verified: true, checkpoint_contract_valid: true,
		 standard_metrics: "complete", claim_scope: "complete terminal valuation"}}' >"$terminalfailure_tmp"
fi
mv "$terminalfailure_tmp" "$analysis_dir/terminalfailure.json"
require_json_object "$analysis_dir/terminalfailure.json"

required=(
	observationreceipts.json frontiervectors.json mechanical.json conservation.json
	positions.json fillpositions.json orderlifecycle.json lifecycle.json settlements.json
	expiryfills.json evidenceartifacthash.json streamhash.json arbitrage.json crossvenue.json
	roleaudit.json ecology.json derivatives.json liquidations.json marginchecks.json
	optionsurface.json optionliabilityp6.json optionvaluetakerp6.json vannavolgap6.json
	exposure.json hedging.json makerrefresh.json makerquotesize.json makerrebalance.json
	postonly.json liabilityhedger.json perpsignals.json datedmandatep5.json fundingcarry.json
	termcarry.json datedcarryp5.json perpreplenishment.json activation.json integrity.json calendar.json
)
if [[ "$extractor_variant" == sv1 ]]; then
	required+=(cdfliquidity.json priceunavailable.json terminalfailure.json)
	if [[ "$v2_r2_sv1_require_terminal_outcome" == true ]]; then
		required+=(terminal-outcome.json)
	fi
fi
for artifact in "${required[@]}"; do
	require_file "$analysis_dir/$artifact"
	require_json_object "$analysis_dir/$artifact"
done

if [[ "$evidence_format" == "evstream_v3" ]]; then
	runtime_events=$(jq -er '.event_frames' "$cell/binary-evidence-attestation.json")
	runtime_digest=$(jq -er '.execution_stream_hash' "$cell/binary-evidence-attestation.json")
	runtime_canonical_digest=$(jq -er '.canonical_execution_stream_hash' "$cell/binary-evidence-attestation.json")
	jq -e --arg execution_hash "$runtime_digest" --argjson event_frames "$runtime_events" \
		'.result.domain == "rendered_binary_json_records" and .result.ordering == "venue_sequence_reconstructed" and
		 .result.source_execution_stream_hash == $execution_hash and .result.source_binary_event_frames == $event_frames' \
		"$analysis_dir/evidenceartifacthash.json" >/dev/null || fail "rendered binary evidence hash domain mismatch"
	jq -e --arg canonical_hash "$runtime_canonical_digest" \
		'.result.source_canonical_execution_stream_hash == $canonical_hash' \
		"$analysis_dir/evidenceartifacthash.json" >/dev/null || fail "rendered binary canonical hash domain mismatch"
else
	runtime_events=$(jq -er '.events' "$cell/evidence-artifact-hash.json")
	runtime_digest=$(jq -er '.digest' "$cell/evidence-artifact-hash.json")
	offline_events=$(jq -er '.result.events' "$analysis_dir/evidenceartifacthash.json")
	offline_digest=$(jq -er '.result.digest' "$analysis_dir/evidenceartifacthash.json")
	[[ "$runtime_events" == "$offline_events" && "$runtime_digest" == "$offline_digest" ]] || fail "runtime/offline evidence digest mismatch"
	jq -e '(.result.domain // "") == "persisted_json_records" and (.result.ordering // "") == "unordered_multiset"' \
		"$analysis_dir/evidenceartifacthash.json" >/dev/null || fail "offline evidence hash domain mismatch"
fi
	if [[ "$terminal_failure" == true ]]; then
		jq -e '.result.domain == "canonical_binary_execution_frames" and .result.ordering == "ordered_stream" and
			.result.hashing == "route_sequence_neutral_v1" and
			(.result.event_frames | type) == "number" and (.result.stream_frames | type) == "number" and
			(.result.execution_stream_hash | test("^[0-9a-f]{64}$")) and
			(.result.canonical_execution_stream_hash | test("^[0-9a-f]{64}$"))' \
			"$analysis_dir/streamhash.json" >/dev/null || fail "terminal stream hash domain mismatch"
	else
		jq -e '.result.domain == "persisted_evidence" and .result.ordering == "unordered_multiset"' \
			"$analysis_dir/streamhash.json" >/dev/null || fail "stream hash domain mismatch"
	fi

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
	--arg evidence_format "$evidence_format" \
	--arg renderer_revision "$renderer_revision" \
	--arg renderer_sha256 "$renderer_sha256" \
	--arg renderer_go_version "$renderer_go_version" \
	--arg renderer_route_compression "$render_route_compression" \
	--arg source_revision_mode "$source_revision_mode" \
	--arg raw_source_revision "$metadata_revision" \
	--argjson analyzer_modified "$analyzer_modified_json" \
	--argjson required_artifacts "$required_json" \
	--argjson artifact_sha256 "$artifact_sha256" \
	--argjson runtime_evidence_events "$runtime_events" \
	--arg runtime_evidence_digest "$runtime_digest" \
	--arg runtime_canonical_digest "$runtime_canonical_digest" \
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
	--argjson completion_sentinels "$completion_sentinels" \
	'{schema_version: 3, cell: $cell, seed: $seed, evidence_format: $evidence_format,
		analysis_revision: $analysis_revision, analyzer_revision: $analyzer_revision,
		analyzer_sha256: $analyzer_sha256, analyzer_vcs_modified: $analyzer_modified,
		analyzer_trimpath: $analyzer_trimpath, analyzer_cgo_enabled: $analyzer_cgo_enabled,
		analyzer_go_version: $analyzer_go_version,
		renderer_revision: $renderer_revision, renderer_sha256: $renderer_sha256,
		renderer_go_version: $renderer_go_version, renderer_route_compression: $renderer_route_compression,
		source_revision_mode: $source_revision_mode, raw_source_revision: $raw_source_revision,
		require_exact_replay: true,
		simulator_revision: $simulator_revision, simulator_sha256: $simulator_sha256,
		simulator_trimpath: $simulator_trimpath, simulator_cgo_enabled: $simulator_cgo_enabled,
		simulator_go_version: $simulator_go_version,
		prunegate_revision: $prunegate_revision, prunegate_sha256: $prunegate_sha256,
		prunegate_trimpath: $prunegate_trimpath, prunegate_cgo_enabled: $prunegate_cgo_enabled,
		prunegate_go_version: $prunegate_go_version,
		config_sha256: $config_sha256, analysis_contract: $contract,
		integrity_contract: $contract, activation_contract: $contract,
			completion_sentinels: $completion_sentinels, required_artifacts: $required_artifacts,
		artifact_sha256: $artifact_sha256,
		runtime_evidence_artifact: ({representation: $evidence_format, event_frames: $runtime_evidence_events, execution_stream_hash: $runtime_evidence_digest} +
			(if $runtime_canonical_digest == "" then {} else {canonical_execution_stream_hash: $runtime_canonical_digest} end)),
		inactive_contracts: ["fundingcarry", "termcarry", "datedcarryp5", "datedmandatep5", "perpreplenishment"],
		raw_log_policy: "retained; this extractor has no prune authority"}' >"$metadata_tmp"
mv "$metadata_tmp" "$analysis_dir/analysis-metadata.json"
require_json_object "$analysis_dir/analysis-metadata.json"
jq -e --arg revision "$head_revision" --arg analyzer_revision "$analyzer_revision" \
	--arg contract "$contract_version" --arg renderer_route_compression "$render_route_compression" \
	--arg source_revision_mode "$source_revision_mode" --arg raw_source_revision "$metadata_revision" \
	--argjson completion_sentinels "$completion_sentinels" \
	--argjson required_artifacts "$required_json" \
	'.schema_version == 3 and .evidence_format == "evstream_v3" and .analysis_revision == $revision and
	 .analyzer_revision == $analyzer_revision and .analyzer_vcs_modified == false and
	 .require_exact_replay == true and
	 .renderer_route_compression == $renderer_route_compression and
		 .source_revision_mode == $source_revision_mode and .raw_source_revision == $raw_source_revision and
		 .analysis_contract == $contract and .required_artifacts == $required_artifacts and
		 .completion_sentinels == $completion_sentinels and
		 (.artifact_sha256 | keys) == ($required_artifacts | sort) and
	all(.artifact_sha256 | to_entries[]; (.value | test("^[0-9a-f]{64}$"))) and
	(.raw_log_policy | type) == "string"' "$analysis_dir/analysis-metadata.json" >/dev/null ||
	fail "analysis metadata self-check failed"

# The analyzer has no prune authority, but extraction is still a long-running
# read of the raw stream. Revalidate the runner seal and sibling attestation
# after all derived artifacts are produced so a concurrent raw mutation cannot
# be mistaken for the measured result.
v2_r2_verify_evidence_manifest "$cell" || fail "raw evidence changed during extraction: $cell"
v2_r2_verify_attestation "$cell" || fail "external attestation changed during extraction: $cell"

printf 'extracted integrated long-run evidence: %s\n' "$cell"
