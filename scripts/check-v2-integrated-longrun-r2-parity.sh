#!/usr/bin/env bash
# Verify the registered seed-607 full/no-log/g8 parity controls. This script
# never reads holdout directories and refuses to overwrite its attestation.
set -euo pipefail

verify_existing=false
if [[ ${1:-} == --verify-existing ]]; then
	verify_existing=true
	shift
fi
if [[ $# -gt 1 ]]; then
	printf 'usage: %s [--verify-existing] [OUTPUT_ROOT]\n' "$0" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"
output_root=${1:-"$v2_r2_output_root"}
attestation="$output_root/parity-attestation.json"
analyzer=${MVANALYZE_BIN:-"$root_dir/bin/mvanalyze"}

fail() {
	printf 'integrated long-run parity failure: %s\n' "$*" >&2
	exit 1
}
require_file() {
	[[ -s "$1" ]] || fail "missing required parity file: $1"
}
require_object() {
	jq -e 'type == "object"' "$1" >/dev/null || fail "malformed parity JSON: $1"
}
v2_r2_acquire_namespace_lock || fail "could not acquire the R2 evidence namespace lock"
v2_r2_require_output_root "$output_root" || fail "parity root is not the canonical R2 evidence root"
if [[ "$verify_existing" == true ]]; then
	[[ -s "$attestation" ]] || fail "missing existing parity attestation: $attestation"
else
	[[ ! -e "$attestation" ]] || fail "refusing to overwrite parity attestation: $attestation"
fi
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "parity requires a clean gate worktree"
[[ -x "$analyzer" ]] || fail "missing analyzer for independent raw parity recomputation: $analyzer"
analyzer_revision=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
analyzer_modified=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
analyzer_trimpath=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "-trimpath=") == 1 {sub("-trimpath=", "", $2); print $2; exit}')
analyzer_cgo_enabled=$(go version -m "$analyzer" | awk '$1 == "build" && index($2, "CGO_ENABLED=") == 1 {sub("CGO_ENABLED=", "", $2); print $2; exit}')
analyzer_go_version=$(v2_r2_binary_go_version "$analyzer")
head_revision=$(git -C "$root_dir" rev-parse HEAD)
[[ "$analyzer_revision" == "$head_revision" && "$analyzer_modified" == false &&
	"$analyzer_trimpath" == true && "$analyzer_cgo_enabled" == 0 ]] ||
	fail "parity analyzer is not a clean reproducible build of current HEAD"
v2_r2_is_go_127 "$analyzer_go_version" || fail "parity analyzer is not built with the pinned Go 1.27 toolchain: $analyzer_go_version"

raw_stage_cells=()
cleanup_raw_stage() {
	local cell
	for cell in "${raw_stage_cells[@]}"; do
		v2_r2_cleanup_staged_raw_evidence "$cell" ||
			printf 'integrated long-run parity cleanup failure: %s\n' "$cell" >&2
	done
}
trap cleanup_raw_stage EXIT

"$root_dir/scripts/check-v2-integrated-longrun-r2-configs.sh" >/dev/null
for cell in dev-607 dev-607-none dev-607-g8; do
	cell_dir="$output_root/$cell"
	v2_r2_require_cell_path "$cell_dir" || fail "parity cell is outside the canonical R2 root or is symlinked: $cell"
done

for cell in dev-607 dev-607-g8; do
	cell_dir="$output_root/$cell"
	raw_stage_cells+=("$cell_dir")
	v2_r2_stage_raw_evidence "$cell_dir" || fail "raw evidence is neither retained nor covered by a valid archive: $cell"
done
cmp -s "$root_dir/research/configs/v2-integrated-longrun-r2/dev-607.json" \
	"$output_root/dev-607/run-config.json" || fail "seed-607 full config differs from registry"
cmp -s "$root_dir/research/configs/v2-integrated-longrun-r2/dev-607-none.json" \
	"$output_root/dev-607-none/run-config.json" || fail "seed-607 no-log config differs from registry"
cmp -s "$root_dir/research/configs/v2-integrated-longrun-r2/dev-607.json" \
	"$output_root/dev-607-g8/run-config.json" || fail "seed-607 g8 config differs from registry"
evidence_format=$(v2_r2_evidence_format "$output_root/dev-607") || fail "missing parity evidence format"
[[ "$evidence_format" == "evstream_v3" ]] || fail "parity requires the registered binary evidence format"
for cell in dev-607 dev-607-none dev-607-g8; do
	[[ "$(v2_r2_evidence_format "$output_root/$cell")" == "$evidence_format" ]] || fail "parity evidence format differs: $cell"
done
for cell in dev-607 dev-607-none dev-607-g8; do
	cell_dir="$output_root/$cell"
	[[ -d "$cell_dir" ]] || fail "missing parity cell: $cell"
	for file in run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json checkpoints.jsonl; do
		require_file "$cell_dir/$file"
	done
	v2_r2_verify_evidence_manifest "$cell_dir" || fail "raw evidence manifest mismatch: $cell"
	v2_r2_verify_attestation "$cell_dir" || fail "external evidence attestation mismatch: $cell"
	for json_file in "$cell_dir"/*.json; do
		[[ -f "$json_file" ]] || continue
		require_object "$json_file"
	done
done

jq -e '.schema_version == 6 and .runner_contract == "v2-integrated-longrun-r2-runner-v2" and
	.cell == "dev-607" and .seed == 607 and .holdout == false and
	.log_mode == "full" and .evidence_format == "evstream_v3" and .gomaxprocs == 4 and
	.hypothesis_id == "V2-INTEGRATED-LONG-R2-CANDIDATE" and
	.binary_vcs_modified == false and .binary_trimpath == true and
	.binary_cgo_enabled == "0" and
	(.binary_go_version | startswith("go1.27")) and
	(.git_revision | test("^[0-9a-f]{40}$")) and
	(.config_sha256 | test("^[0-9a-f]{64}$")) and
	(.binary_sha256 | test("^[0-9a-f]{64}$"))' \
	"$output_root/dev-607/run-metadata.json" >/dev/null || fail "invalid seed-607 g4 metadata"
jq -e '.schema_version == 6 and .runner_contract == "v2-integrated-longrun-r2-runner-v2" and
	.cell == "dev-607-none" and .seed == 607 and .holdout == false and
	.log_mode == "none" and .evidence_format == "evstream_v3" and .gomaxprocs == 4 and
	.hypothesis_id == "V2-INTEGRATED-LONG-R2-CANDIDATE-PARITY" and
	.binary_vcs_modified == false and .binary_trimpath == true and
	.binary_cgo_enabled == "0" and
	(.binary_go_version | startswith("go1.27")) and
	(.git_revision | test("^[0-9a-f]{40}$")) and
	(.config_sha256 | test("^[0-9a-f]{64}$")) and
	(.binary_sha256 | test("^[0-9a-f]{64}$"))' \
	"$output_root/dev-607-none/run-metadata.json" >/dev/null || fail "invalid seed-607 no-log metadata"
jq -e '.schema_version == 6 and .runner_contract == "v2-integrated-longrun-r2-runner-v2" and
	.cell == "dev-607-g8" and .seed == 607 and .holdout == false and
	.log_mode == "full" and .evidence_format == "evstream_v3" and .gomaxprocs == 8 and
	.hypothesis_id == "V2-INTEGRATED-LONG-R2-CANDIDATE" and
	.binary_vcs_modified == false and .binary_trimpath == true and
	.binary_cgo_enabled == "0" and
	(.binary_go_version | startswith("go1.27")) and
	(.git_revision | test("^[0-9a-f]{40}$")) and
	(.config_sha256 | test("^[0-9a-f]{64}$")) and
	(.binary_sha256 | test("^[0-9a-f]{64}$"))' \
	"$output_root/dev-607-g8/run-metadata.json" >/dev/null || fail "invalid seed-607 g8 metadata"
for cell in dev-607 dev-607-none dev-607-g8; do
	jq -e '(.prunegate_vcs_revision | test("^[0-9a-f]{40}$")) and
		.prunegate_vcs_modified == false and .prunegate_trimpath == true and
		.prunegate_cgo_enabled == "0" and (.prunegate_go_version | startswith("go1.27")) and
		(.prunegate_sha256 | test("^[0-9a-f]{64}$"))' \
		"$output_root/$cell/run-metadata.json" >/dev/null || fail "invalid prunegate metadata: $cell"
done

for cell in dev-607 dev-607-none dev-607-g8; do
	jq -e '.exit_status == 0 and .completion_verified == true and
		.simulated_horizon == "24h" and
		.simulation_start_nano == 1735689600000000000 and
		.simulation_end_nano == 1735776000000000000 and
		(.checkpoints_sha256 | test("^[0-9a-f]{64}$")) and
		.completion_sentinels == ["greeks.json", "latency.json"]' \
		"$output_root/$cell/run-status.json" >/dev/null || fail "incomplete parity cell: $cell"
	for file in run-metadata.json manifest.json greeks.json latency.json checkpoints.jsonl evidence-manifest.json; do
		actual_sha256=$(sha256sum "$output_root/$cell/$file" | awk '{print $1}')
		case "$file" in
			run-metadata.json) declared_sha256=$(jq -er '.run_metadata_sha256' "$output_root/$cell/run-status.json") ;;
			manifest.json) declared_sha256=$(jq -er '.manifest_sha256' "$output_root/$cell/run-status.json") ;;
			greeks.json) declared_sha256=$(jq -er '.greeks_sha256' "$output_root/$cell/run-status.json") ;;
			latency.json) declared_sha256=$(jq -er '.latency_sha256' "$output_root/$cell/run-status.json") ;;
			checkpoints.jsonl) declared_sha256=$(jq -er '.checkpoints_sha256' "$output_root/$cell/run-status.json") ;;
			evidence-manifest.json) declared_sha256=$(jq -er '.evidence_manifest_sha256' "$output_root/$cell/run-status.json") ;;
		esac
		[[ "$actual_sha256" == "$declared_sha256" ]] || fail "run-status hash mismatch: $cell/$file"
	done
done

for file in checkpoints.jsonl greeks.json latency.json; do
	cmp -s "$output_root/dev-607/$file" "$output_root/dev-607-none/$file" || fail "$file differs between full and no-log"
	cmp -s "$output_root/dev-607/$file" "$output_root/dev-607-g8/$file" || fail "$file differs between g4 and g8"
done
cmp -s "$output_root/dev-607/run-config.json" "$output_root/dev-607-g8/run-config.json" || fail "full g4/g8 config differs"
cmp -s "$output_root/dev-607/events.evs" "$output_root/dev-607-none/events.evs" || fail "binary execution stream differs between full and no-log"
cmp -s "$output_root/dev-607/events.evs" "$output_root/dev-607-g8/events.evs" || fail "binary execution stream differs between g4 and g8"
cmp -s "$output_root/dev-607/binary-evidence-attestation.json" "$output_root/dev-607-none/binary-evidence-attestation.json" || fail "binary attestation differs between full and no-log"
cmp -s "$output_root/dev-607/binary-evidence-attestation.json" "$output_root/dev-607-g8/binary-evidence-attestation.json" || fail "binary attestation differs between g4 and g8"
v2_r2_compare_ordered_raw_manifests "$output_root/dev-607" "$output_root/dev-607-g8" ||
	fail "full g4/g8 ordered raw evidence manifest differs"

[[ ! -e "$output_root/dev-607-none/evidence-artifact-hash.json" ]] || fail "no-log cell contains legacy runtime evidence hash"
[[ ! -e "$output_root/dev-607-none/evidence-only-artifact-hash.json" ]] || fail "no-log cell contains evidence-only attestation"
[[ -d "$output_root/dev-607-none/venues" ]] || fail "no-log cell is missing simulator venue root"
if find "$output_root/dev-607-none/venues" -type f -name '*.jsonl' -print -quit | grep -q .; then
	fail "no-log cell contains venue JSONL evidence"
fi

full_runtime_events=$(jq -er '.event_frames' "$output_root/dev-607/binary-evidence-attestation.json")
full_runtime_digest=$(jq -er '.execution_stream_hash' "$output_root/dev-607/binary-evidence-attestation.json")
g8_runtime_events=$(jq -er '.event_frames' "$output_root/dev-607-g8/binary-evidence-attestation.json")
g8_runtime_digest=$(jq -er '.execution_stream_hash' "$output_root/dev-607-g8/binary-evidence-attestation.json")
[[ "$full_runtime_events" == "$g8_runtime_events" && "$full_runtime_digest" == "$g8_runtime_digest" ]] || fail "full runtime evidence hashes are not equal"
full_sidecar_result=$("$analyzer" -metric evidenceartifacthash -json "$output_root/dev-607") || fail "could not recompute g4 evidence-only digest"
g8_sidecar_result=$("$analyzer" -metric evidenceartifacthash -json "$output_root/dev-607-g8") || fail "could not recompute g8 evidence-only digest"
full_sidecar_events=$(jq -er '.result.events' <<<"$full_sidecar_result") || fail "malformed g4 evidence-only digest"
full_sidecar_digest=$(jq -er '.result.digest' <<<"$full_sidecar_result") || fail "malformed g4 evidence-only digest"
g8_sidecar_events=$(jq -er '.result.events' <<<"$g8_sidecar_result") || fail "malformed g8 evidence-only digest"
g8_sidecar_digest=$(jq -er '.result.digest' <<<"$g8_sidecar_result") || fail "malformed g8 evidence-only digest"
[[ "$full_sidecar_events" == "$g8_sidecar_events" && "$full_sidecar_digest" == "$g8_sidecar_digest" ]] || fail "independently recomputed g4/g8 evidence-only hashes are not equal"

source_revision=$(jq -er '.git_revision' "$output_root/dev-607/run-metadata.json")
v2_r2_require_current_source_revision "$source_revision" "$head_revision" "$analyzer_revision" ||
	fail "parity simulator source revision is not the current analyzer/HEAD revision"
binary_sha256=$(jq -er '.binary_sha256' "$output_root/dev-607/run-metadata.json")
binary_go_version=$(jq -er '.binary_go_version' "$output_root/dev-607/run-metadata.json")
prunegate_sha256=$(jq -er '.prunegate_sha256' "$output_root/dev-607/run-metadata.json")
prunegate_go_version=$(jq -er '.prunegate_go_version' "$output_root/dev-607/run-metadata.json")
prunegate_revision=$(jq -er '.prunegate_vcs_revision' "$output_root/dev-607/run-metadata.json")
v2_r2_require_matching_revision "$prunegate_revision" "$source_revision" ||
	fail "parity pruning gate revision is not the current simulator source revision"
for cell in dev-607 dev-607-none dev-607-g8; do
	jq -e --arg revision "$source_revision" \
		--arg binary_sha256 "$binary_sha256" --arg binary_go_version "$binary_go_version" \
		--arg prunegate_sha256 "$prunegate_sha256" --arg prunegate_go_version "$prunegate_go_version" \
		--arg prunegate_revision "$prunegate_revision" \
		'.git_revision == $revision and .binary_vcs_modified == false and .binary_vcs_revision == $revision and
				 .binary_sha256 == $binary_sha256 and .binary_go_version == $binary_go_version and
				 .binary_trimpath == true and .binary_cgo_enabled == "0" and
				 .prunegate_sha256 == $prunegate_sha256 and .prunegate_go_version == $prunegate_go_version and
				 .prunegate_vcs_revision == $prunegate_revision' \
		"$output_root/$cell/run-metadata.json" >/dev/null || fail "parity source identity differs: $cell"
	jq -e --arg revision "$source_revision" \
		'.build.revision == $revision and .build.modified == false' \
		"$output_root/$cell/manifest.json" >/dev/null || fail "parity manifest identity differs: $cell"
done

mkdir -p "$output_root"
tmp=$(mktemp "$attestation.tmp-XXXXXX")
jq -n \
	--arg contract "v2-integrated-longrun-r2-parity-v2" \
	--arg evidence_format "$evidence_format" \
	--arg source_revision "$source_revision" \
	--arg simulator_binary_sha256 "$binary_sha256" \
	--arg simulator_binary_go_version "$binary_go_version" \
	--arg prunegate_sha256 "$prunegate_sha256" \
	--arg prunegate_go_version "$prunegate_go_version" \
	--arg prunegate_revision "$prunegate_revision" \
	--arg analyzer_sha256 "$(sha256sum "$analyzer" | awk '{print $1}')" \
	--arg analyzer_go_version "$analyzer_go_version" \
	--arg analyzer_revision "$analyzer_revision" \
	--arg full_g4_checkpoints "$(sha256sum "$output_root/dev-607/checkpoints.jsonl" | awk '{print $1}')" \
	--arg none_g4_checkpoints "$(sha256sum "$output_root/dev-607-none/checkpoints.jsonl" | awk '{print $1}')" \
	--arg full_g8_checkpoints "$(sha256sum "$output_root/dev-607-g8/checkpoints.jsonl" | awk '{print $1}')" \
	--arg greeks_sha256 "$(sha256sum "$output_root/dev-607/greeks.json" | awk '{print $1}')" \
	--arg latency_sha256 "$(sha256sum "$output_root/dev-607/latency.json" | awk '{print $1}')" \
	--arg binary_attestation_sha256 "$(sha256sum "$output_root/dev-607/binary-evidence-attestation.json" | awk '{print $1}')" \
	--arg binary_stream_sha256 "$(sha256sum "$output_root/dev-607/events.evs" | awk '{print $1}')" \
	--arg full_sidecar_events "$full_sidecar_events" \
	--arg full_sidecar_digest "$full_sidecar_digest" \
	--arg g8_sidecar_events "$g8_sidecar_events" \
	--arg g8_sidecar_digest "$g8_sidecar_digest" \
	--argjson evidence_events "$full_runtime_events" \
	--arg evidence_digest "$full_runtime_digest" \
	'{
		schema_version: 2, contract: $contract, evidence_format: $evidence_format, seed: 607, horizon: "24h",
		source_revision: $source_revision, simulator_revision: $source_revision,
		simulator_binary_sha256: $simulator_binary_sha256,
		simulator_binary_go_version: $simulator_binary_go_version,
		analyzer_sha256: $analyzer_sha256, analyzer_go_version: $analyzer_go_version,
		analyzer_revision: $analyzer_revision,
		prunegate_sha256: $prunegate_sha256, prunegate_go_version: $prunegate_go_version,
		prunegate_revision: $prunegate_revision, controls: [
			{cell: "dev-607", log_mode: "full", gomaxprocs: 4},
			{cell: "dev-607-none", log_mode: "none", gomaxprocs: 4},
			{cell: "dev-607-g8", log_mode: "full", gomaxprocs: 8}
		],
		exact_equal_domains: ["checkpoints.jsonl", "greeks.json", "latency.json", "events.evs", "binary-evidence-attestation.json"],
		full_evidence_equal_domains: ["events.evs", "binary-evidence-attestation.json", "ordered_evidence_only_jsonl", "recomputed_evidence_only_jsonl"],
		no_log_absence_contract: ["evidence-artifact-hash.json", "evidence-only-artifact-hash.json", "venues/*.jsonl"],
		hashes: {
			full_g4_checkpoints: $full_g4_checkpoints,
			none_g4_checkpoints: $none_g4_checkpoints,
			full_g8_checkpoints: $full_g8_checkpoints,
			greeks: $greeks_sha256, latency: $latency_sha256,
			binary_attestation: $binary_attestation_sha256, binary_stream: $binary_stream_sha256
		},
		full_runtime_evidence: {event_frames: ($evidence_events | tonumber), execution_stream_hash: $evidence_digest},
		recomputed_evidence_only: {
			g4: {events: ($full_sidecar_events | tonumber), digest: $full_sidecar_digest},
			g8: {events: ($g8_sidecar_events | tonumber), digest: $g8_sidecar_digest}
		},
		predicates: {
				ordered_checkpoints_equal: true,
				deterministic_sidecars_equal: true,
				full_evidence_equal: true,
				ordered_raw_evidence_equal: true,
				no_log_evidence_absent: true,
			source_and_build_identity_equal: true
		}
	}' >"$tmp"
if [[ "$verify_existing" == true ]]; then
	if ! cmp -s <(jq -S -c . "$attestation") <(jq -S -c . "$tmp"); then
		rm -f -- "$tmp"
		fail "existing parity attestation differs from fresh recomputation"
	fi
	rm -f -- "$tmp"
else
	mv "$tmp" "$attestation"
fi
require_object "$attestation"
jq -e '.schema_version == 2 and .contract == "v2-integrated-longrun-r2-parity-v2" and .evidence_format == "evstream_v3" and
	(.simulator_binary_sha256 | test("^[0-9a-f]{64}$")) and
	(.simulator_binary_go_version | startswith("go1.27")) and
	(.analyzer_sha256 | test("^[0-9a-f]{64}$")) and
	(.analyzer_go_version | startswith("go1.27")) and
	(.analyzer_revision | test("^[0-9a-f]{40}$")) and
	(.prunegate_sha256 | test("^[0-9a-f]{64}$")) and
	(.prunegate_go_version | startswith("go1.27")) and
	(.prunegate_revision | test("^[0-9a-f]{40}$")) and
	(.predicates | keys) == ["deterministic_sidecars_equal", "full_evidence_equal", "no_log_evidence_absent", "ordered_checkpoints_equal", "ordered_raw_evidence_equal", "source_and_build_identity_equal"] and
	all(.predicates | to_entries[]; .value == true)' "$attestation" >/dev/null || fail "parity attestation self-check failed"
printf 'integrated long-run parity verified: %s\n' "$attestation"
