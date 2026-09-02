#!/usr/bin/env bash
# Verify the registered SV1 treatment G8 and control no-log parity
# pairs. Treatment and control are different economic populations and are
# never compared as if their trajectories should be identical.
set -euo pipefail

verify_existing=false
if [[ "$#" -gt 0 && "$1" == "--verify-existing" ]]; then
	verify_existing=true
	shift
fi
if [[ "$#" -gt 1 ]]; then
	printf 'usage: %s [--verify-existing] [OUTPUT_ROOT]\n' "$0" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1-contract-loader.sh"
contract_script=$(v2_r2_select_sv1_contract "$root_dir") || {
	echo "SV1 parity checker received an unregistered contract path" >&2
	exit 1
}
source "$contract_script"
export V2_R2_SV1_CONTRACT_SCRIPT="$contract_script"
output_root="$v2_r2_output_root"
if [[ "$#" -eq 1 ]]; then
	output_root=$1
fi
attestation="$output_root/parity-attestation.json"
analyzer="$root_dir/bin/mvanalyze"
if [[ -n "$(printenv MVANALYZE_BIN 2>/dev/null || true)" ]]; then
	analyzer=$(printenv MVANALYZE_BIN)
fi
parity_seed="${v2_r2_sv1_parity_seed}"
treatment_cell="$output_root/treatment-$parity_seed"
treatment_g8_cell="$output_root/treatment-$parity_seed-g8"
control_cell="$output_root/control-$parity_seed"
control_none_cell="$output_root/control-$parity_seed-none"

fail() {
	printf 'SV1 parity failure: %s\n' "$*" >&2
	exit 1
}
require_file() {
	[[ -s "$1" ]] || fail "missing required parity file: $1"
}
require_object() {
	jq -e 'type == "object"' "$1" >/dev/null || fail "malformed parity JSON: $1"
}

v2_r2_acquire_namespace_lock || fail "could not acquire the SV1 namespace lock"
v2_r2_require_output_root "$output_root" || fail "parity root is not the canonical SV1 evidence root"
if [[ "$verify_existing" == true ]]; then
	require_file "$attestation"
else
	[[ ! -e "$attestation" && ! -L "$attestation" ]] || fail "refusing to overwrite parity attestation: $attestation"
fi
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "parity requires a clean source worktree"
[[ -x "$analyzer" ]] || fail "missing analyzer: $analyzer"
head_revision=$(git -C "$root_dir" rev-parse HEAD)
analyzer_revision=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
analyzer_modified=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
analyzer_trimpath=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
analyzer_cgo_enabled=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
analyzer_go_version=$(v2_r2_binary_go_version "$analyzer")
[[ "$analyzer_revision" == "$head_revision" && "$analyzer_modified" == false &&
	"$analyzer_trimpath" == true && "$analyzer_cgo_enabled" == 0 ]] ||
	fail "parity analyzer is not a clean reproducible build of current HEAD"
v2_r2_is_go_127 "$analyzer_go_version" || fail "parity analyzer is not built with Go 1.27: $analyzer_go_version"

raw_stage_cells=()
cleanup_raw_stage() {
	local cell
	for cell in "${raw_stage_cells[@]}"; do
		v2_r2_cleanup_staged_raw_evidence "$cell" ||
			printf 'SV1 parity cleanup failure: %s\n' "$cell" >&2
	done
}
trap cleanup_raw_stage EXIT

for cell in "$treatment_cell" "$treatment_g8_cell" "$control_cell" "$control_none_cell"; do
	v2_r2_require_cell_path "$cell" || fail "parity cell is outside the canonical SV1 root or is symlinked: $cell"
	raw_stage_cells+=("$cell")
	v2_r2_stage_raw_evidence "$cell" || fail "raw evidence is not retained for $(basename "$cell")"
done

cmp -s "$v2_r2_sv1_config_dir/treatment-$parity_seed.json" "$treatment_cell/run-config.json" || fail "treatment config differs from registry"
cmp -s "$treatment_cell/run-config.json" "$treatment_g8_cell/run-config.json" || fail "treatment G4/G8 config differs"
cmp -s "$v2_r2_sv1_config_dir/control-$parity_seed.json" "$control_cell/run-config.json" || fail "control config differs from registry"
cmp -s "$v2_r2_sv1_config_dir/control-$parity_seed-none.json" "$control_none_cell/run-config.json" || fail "control no-log config differs from registry"

validate_cell() {
	local cell=$1 expected_name=$2 expected_seed=$3 expected_log_mode=$4 expected_gomaxprocs=$5 expected_hypothesis=$6
	for file in run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json checkpoints.jsonl events.evs binary-evidence-attestation.json evidence-manifest.json; do
		require_file "$cell/$file"
	done
	v2_r2_verify_evidence_manifest "$cell" || fail "evidence manifest mismatch: $(basename "$cell")"
	v2_r2_verify_attestation "$cell" || fail "external attestation mismatch: $(basename "$cell")"
	jq -e --arg cell "$expected_name" --argjson seed "$expected_seed" --arg log_mode "$expected_log_mode" \
		--arg hypothesis "$expected_hypothesis" --arg runner_contract "$v2_r2_sv1_runner_contract" --argjson gomaxprocs "$expected_gomaxprocs" \
		'.schema_version == 6 and .runner_contract == $runner_contract and .cell == $cell and
		 .seed == $seed and .holdout == false and .simulated_horizon == "24h" and
		 .log_mode == $log_mode and .evidence_format == "evstream_v3" and .gomaxprocs == $gomaxprocs and
		 .hypothesis_id == $hypothesis and .binary_vcs_modified == false and .binary_trimpath == true and
		 .binary_cgo_enabled == "0" and (.git_revision | test("^[0-9a-f]{40}$")) and
		 (.binary_sha256 | test("^[0-9a-f]{64}$")) and (.config_sha256 | test("^[0-9a-f]{64}$")) and
		 (.prunegate_sha256 | test("^[0-9a-f]{64}$"))' "$cell/run-metadata.json" >/dev/null ||
		fail "invalid run metadata: $(basename "$cell")"
	jq -e --arg name "$expected_name" \
		'.exit_status == 0 and .completion_verified == true and .cell == $name and
		 .simulated_horizon == "24h" and .completion_sentinels == ["greeks.json", "latency.json"] and
		 (.run_metadata_sha256 | test("^[0-9a-f]{64}$")) and (.evidence_manifest_sha256 | test("^[0-9a-f]{64}$"))' \
		"$cell/run-status.json" >/dev/null || fail "invalid run status: $(basename "$cell")"
	jq -e --arg revision "$head_revision" --argjson seed "$expected_seed" --arg log_mode "$expected_log_mode" \
		'.build.revision == $revision and .build.modified == false and .config.seed == $seed and
		 .config.log_mode == $log_mode and .config.evidence_format == "evstream_v3"' "$cell/manifest.json" >/dev/null ||
		fail "invalid manifest identity: $(basename "$cell")"
}

validate_cell "$treatment_cell" "treatment-$parity_seed" "$parity_seed" full 4 "$v2_r2_sv1_run_hypothesis_id"
validate_cell "$treatment_g8_cell" "treatment-$parity_seed-g8" "$parity_seed" full 8 "$v2_r2_sv1_run_hypothesis_id"
validate_cell "$control_cell" "control-$parity_seed" "$parity_seed" full 4 "${v2_r2_sv1_run_hypothesis_id}-CONTROL"
validate_cell "$control_none_cell" "control-$parity_seed-none" "$parity_seed" none 4 "${v2_r2_sv1_run_hypothesis_id}-CONTROL"

compare_exact_pair() {
	local left=$1 right=$2 label=$3
	for file in checkpoints.jsonl greeks.json latency.json events.evs binary-evidence-attestation.json; do
		cmp -s "$left/$file" "$right/$file" || fail "$label differs: $file"
	done
	v2_r2_compare_ordered_raw_manifests "$left" "$right" || fail "$label ordered raw manifest differs"
}
compare_exact_pair "$treatment_cell" "$treatment_g8_cell" "treatment G4/G8"

# The no-log control suppresses persistence-only sidecars. Its raw frame route
# sequence and canonical raw-frame hash may therefore differ, while the
# normalized execution projection must remain identical. Comparing the whole
# binary file here would reject the evidence-neutrality contract itself.
for file in checkpoints.jsonl greeks.json latency.json; do
	cmp -s "$control_cell/$file" "$control_none_cell/$file" || fail "control full/no-log differs: $file"
done
jq -e -S '
	{domain, ordering, hashing, event_frames, stream_frames, execution_stream_hash,
		unencodable_payloads: (.unencodable_payloads // 0)}' "$control_cell/binary-evidence-attestation.json" \
	| cmp -s - <(jq -e -S '
	{domain, ordering, hashing, event_frames, stream_frames, execution_stream_hash,
		unencodable_payloads: (.unencodable_payloads // 0)}' "$control_none_cell/binary-evidence-attestation.json") \
	|| fail "control full/no-log normalized execution attestation differs"
for cell in "$control_cell" "$control_none_cell"; do
	jq -e '.canonical_execution_stream_hash | type == "string" and test("^[0-9a-f]{64}$")' \
		"$cell/binary-evidence-attestation.json" >/dev/null || fail "control canonical reconstruction identity is missing: $(basename "$cell")"
done

[[ -d "$control_none_cell/venues" ]] || fail "control no-log is missing the venue root"
if find "$control_none_cell/venues" -type f -name '*.jsonl' -print -quit | grep -q .; then
	fail "control no-log contains venue JSONL evidence"
fi
[[ ! -e "$control_none_cell/evidence-artifact-hash.json" ]] || fail "control no-log contains legacy evidence hash"
[[ ! -e "$control_none_cell/evidence-only-artifact-hash.json" ]] || fail "control no-log contains evidence-only hash"

full_sidecar_events() {
	local cell=$1
	"$analyzer" -metric evidenceartifacthash -json "$cell" | jq -er '.result.events'
}
full_sidecar_digest() {
	local cell=$1
	"$analyzer" -metric evidenceartifacthash -json "$cell" | jq -er '.result.digest'
}
treatment_events=$(full_sidecar_events "$treatment_cell") || fail "could not recompute treatment evidence-only events"
treatment_g8_events=$(full_sidecar_events "$treatment_g8_cell") || fail "could not recompute treatment G8 evidence-only events"
treatment_digest=$(full_sidecar_digest "$treatment_cell") || fail "could not recompute treatment evidence-only digest"
treatment_g8_digest=$(full_sidecar_digest "$treatment_g8_cell") || fail "could not recompute treatment G8 evidence-only digest"
control_events=$(full_sidecar_events "$control_cell") || fail "could not recompute control evidence-only events"
control_digest=$(full_sidecar_digest "$control_cell") || fail "could not recompute control evidence-only digest"
[[ "$treatment_events" == "$treatment_g8_events" && "$treatment_digest" == "$treatment_g8_digest" ]] || fail "treatment G4/G8 evidence-only reconstruction differs"

source_revision=$(jq -er '.git_revision' "$treatment_cell/run-metadata.json")
binary_sha256=$(jq -er '.binary_sha256' "$treatment_cell/run-metadata.json")
prunegate_sha256=$(jq -er '.prunegate_sha256' "$treatment_cell/run-metadata.json")
for cell in "${raw_stage_cells[@]}"; do
	jq -e --arg revision "$source_revision" --arg binary_sha256 "$binary_sha256" --arg prunegate_sha256 "$prunegate_sha256" \
		'.git_revision == $revision and .binary_vcs_modified == false and .binary_vcs_revision == $revision and
		 .binary_sha256 == $binary_sha256 and .prunegate_sha256 == $prunegate_sha256 and
		 .prunegate_vcs_revision == $revision and .prunegate_vcs_modified == false and
		 .prunegate_trimpath == true and .prunegate_cgo_enabled == "0"' "$cell/run-metadata.json" >/dev/null ||
		fail "source/build identity differs: $(basename "$cell")"
done

tmp_attestation=$(mktemp "$attestation.tmp-XXXXXX")
cleanup_attestation() {
	rm -f -- "$tmp_attestation"
}
trap 'cleanup_attestation; cleanup_raw_stage' EXIT
jq -n --arg source_revision "$source_revision" --arg analyzer_revision "$analyzer_revision" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" --arg analyzer_go_version "$analyzer_go_version" \
	--arg simulator_binary_sha256 "$binary_sha256" --arg prunegate_sha256 "$prunegate_sha256" \
	--arg treatment_stream_sha256 "$(sha256sum "$treatment_cell/events.evs" | awk '{print $1}')" \
	--arg control_stream_sha256 "$(sha256sum "$control_cell/events.evs" | awk '{print $1}')" \
	--argjson treatment_events "$treatment_events" --arg treatment_digest "$treatment_digest" \
	--argjson control_events "$control_events" --arg control_digest "$control_digest" \
	--arg parity_contract "$v2_r2_sv1_parity_contract" --argjson parity_seed "$parity_seed" \
	--arg treatment_name "$(basename "$treatment_cell")" --arg treatment_g8_name "$(basename "$treatment_g8_cell")" \
	--arg control_name "$(basename "$control_cell")" --arg control_none_name "$(basename "$control_none_cell")" \
	'{
		schema_version: 1, contract: $parity_contract, evidence_format: "evstream_v3", seed: $parity_seed, horizon: "24h",
		source_revision: $source_revision, analyzer_revision: $analyzer_revision, analyzer_sha256: $analyzer_sha256,
		analyzer_go_version: $analyzer_go_version, simulator_binary_sha256: $simulator_binary_sha256,
		prunegate_sha256: $prunegate_sha256,
		pairs: {treatment_g4_g8: [$treatment_name, $treatment_g8_name], control_full_no_log: [$control_name, $control_none_name]},
		exact_equal_domains: ["checkpoints.jsonl", "greeks.json", "latency.json", "events.evs", "binary-evidence-attestation.json", "ordered_raw_manifest"],
		control_normalized_equal_domains: ["checkpoints.jsonl", "greeks.json", "latency.json", "execution_event_frames", "execution_stream_frames", "execution_stream_hash"],
		no_log_absence_contract: ["evidence-artifact-hash.json", "evidence-only-artifact-hash.json", "venues/*.jsonl"],
	streams: {treatment: $treatment_stream_sha256, control: $control_stream_sha256},
		treatment_evidence_only: {events: $treatment_events, digest: $treatment_digest},
		control_evidence_only: {events: $control_events, digest: $control_digest},
		control_no_log_normalized_execution: {
			event_frames: true, stream_frames: true, execution_stream_hash: true,
			checkpoints: true, terminal_sidecars: true
		},
		predicates: {treatment_g4_g8_equal: true, control_full_no_log_normalized_equal: true,
			no_log_evidence_absent: true, ordered_raw_evidence_equal: true,
			evidence_only_reconstruction_equal: true, source_and_build_identity_equal: true}
	}' >"$tmp_attestation" || fail "could not construct parity attestation"
if [[ "$verify_existing" == true ]]; then
	cmp -s <(jq -S -c . "$attestation") <(jq -S -c . "$tmp_attestation") || fail "existing parity attestation differs from fresh recomputation"
else
	mkdir -p -- "$output_root"
	mv -- "$tmp_attestation" "$attestation"
fi
require_object "$attestation"
jq -e --arg contract "$v2_r2_sv1_parity_contract" '.schema_version == 1 and .contract == $contract and .evidence_format == "evstream_v3" and
		.exact_equal_domains == ["checkpoints.jsonl", "greeks.json", "latency.json", "events.evs", "binary-evidence-attestation.json", "ordered_raw_manifest"] and
		.control_normalized_equal_domains == ["checkpoints.jsonl", "greeks.json", "latency.json", "execution_event_frames", "execution_stream_frames", "execution_stream_hash"] and
		 (.predicates | keys) == ["control_full_no_log_normalized_equal", "evidence_only_reconstruction_equal", "no_log_evidence_absent", "ordered_raw_evidence_equal", "source_and_build_identity_equal", "treatment_g4_g8_equal"] and
	 (.control_no_log_normalized_execution | all(to_entries[]; .value == true)) and
	 all(.predicates | to_entries[]; .value == true)' "$attestation" >/dev/null || fail "parity attestation self-check failed"
printf 'SV1 parity verified: %s\n' "$attestation"
