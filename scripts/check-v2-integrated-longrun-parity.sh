#!/usr/bin/env bash
# Verify the registered seed-607 full/no-log/g8 parity controls. This script
# never reads holdout directories and refuses to overwrite its attestation.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-integrated-longrun-r4-contract.sh"
output_root=${1:-"$v2_r4_output_root"}
attestation="$output_root/parity-attestation.json"

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
v2_r4_require_output_root "$output_root" || fail "parity root is not the canonical r4 evidence root"
[[ ! -e "$attestation" ]] || fail "refusing to overwrite parity attestation: $attestation"
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "parity requires a clean gate worktree"

"$root_dir/scripts/check-v2-integrated-longrun-configs.sh" >/dev/null
for cell in dev-607 dev-607-none dev-607-g8; do
	cell_dir="$output_root/$cell"
	v2_r4_require_cell_path "$cell_dir" || fail "parity cell is outside the canonical r4 root or is symlinked: $cell"
done
cmp -s "$root_dir/research/configs/v2-integrated-longrun/dev-607.json" \
	"$output_root/dev-607/run-config.json" || fail "seed-607 full config differs from registry"
cmp -s "$root_dir/research/configs/v2-integrated-longrun/dev-607-none.json" \
	"$output_root/dev-607-none/run-config.json" || fail "seed-607 no-log config differs from registry"
cmp -s "$root_dir/research/configs/v2-integrated-longrun/dev-607.json" \
	"$output_root/dev-607-g8/run-config.json" || fail "seed-607 g8 config differs from registry"
for cell in dev-607 dev-607-none dev-607-g8; do
	cell_dir="$output_root/$cell"
	[[ -d "$cell_dir" ]] || fail "missing parity cell: $cell"
	for file in run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json checkpoints.jsonl; do
		require_file "$cell_dir/$file"
	done
	for json_file in "$cell_dir"/*.json; do
		[[ -f "$json_file" ]] || continue
		require_object "$json_file"
	done
done

jq -e '.schema_version == 4 and .runner_contract == "v2-integrated-longrun-runner-v4" and
	.cell == "dev-607" and .seed == 607 and .holdout == false and
	.log_mode == "full" and .gomaxprocs == 4 and
	.hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE" and
	.binary_vcs_modified == false and .binary_trimpath == true and
	.binary_cgo_enabled == "0" and
	(.binary_go_version | startswith("go1.27")) and
	(.git_revision | test("^[0-9a-f]{40}$")) and
	(.config_sha256 | test("^[0-9a-f]{64}$")) and
	(.binary_sha256 | test("^[0-9a-f]{64}$"))' \
	"$output_root/dev-607/run-metadata.json" >/dev/null || fail "invalid seed-607 g4 metadata"
jq -e '.schema_version == 4 and .runner_contract == "v2-integrated-longrun-runner-v4" and
	.cell == "dev-607-none" and .seed == 607 and .holdout == false and
	.log_mode == "none" and .gomaxprocs == 4 and
	.hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE-PARITY" and
	.binary_vcs_modified == false and .binary_trimpath == true and
	.binary_cgo_enabled == "0" and
	(.binary_go_version | startswith("go1.27")) and
	(.git_revision | test("^[0-9a-f]{40}$")) and
	(.config_sha256 | test("^[0-9a-f]{64}$")) and
	(.binary_sha256 | test("^[0-9a-f]{64}$"))' \
	"$output_root/dev-607-none/run-metadata.json" >/dev/null || fail "invalid seed-607 no-log metadata"
jq -e '.schema_version == 4 and .runner_contract == "v2-integrated-longrun-runner-v4" and
	.cell == "dev-607-g8" and .seed == 607 and .holdout == false and
	.log_mode == "full" and .gomaxprocs == 8 and
	.hypothesis_id == "V2-INTEGRATED-LONG-CANDIDATE" and
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
		.completion_sentinels == ["greeks.json", "latency.json"]' \
		"$output_root/$cell/run-status.json" >/dev/null || fail "incomplete parity cell: $cell"
	for file in run-metadata.json manifest.json greeks.json latency.json; do
		actual_sha256=$(sha256sum "$output_root/$cell/$file" | awk '{print $1}')
		case "$file" in
			run-metadata.json) declared_sha256=$(jq -er '.run_metadata_sha256' "$output_root/$cell/run-status.json") ;;
			manifest.json) declared_sha256=$(jq -er '.manifest_sha256' "$output_root/$cell/run-status.json") ;;
			greeks.json) declared_sha256=$(jq -er '.greeks_sha256' "$output_root/$cell/run-status.json") ;;
			latency.json) declared_sha256=$(jq -er '.latency_sha256' "$output_root/$cell/run-status.json") ;;
		esac
		[[ "$actual_sha256" == "$declared_sha256" ]] || fail "run-status hash mismatch: $cell/$file"
	done
done

for file in checkpoints.jsonl greeks.json latency.json; do
	cmp -s "$output_root/dev-607/$file" "$output_root/dev-607-none/$file" || fail "$file differs between full and no-log"
	cmp -s "$output_root/dev-607/$file" "$output_root/dev-607-g8/$file" || fail "$file differs between g4 and g8"
done
cmp -s "$output_root/dev-607/run-config.json" "$output_root/dev-607-g8/run-config.json" || fail "full g4/g8 config differs"
cmp -s "$output_root/dev-607/evidence-artifact-hash.json" "$output_root/dev-607-g8/evidence-artifact-hash.json" || fail "full g4/g8 persisted evidence hash differs"

[[ ! -e "$output_root/dev-607-none/evidence-artifact-hash.json" ]] || fail "no-log cell contains runtime evidence hash"
[[ -d "$output_root/dev-607-none/venues" ]] || fail "no-log cell is missing simulator venue root"
if find "$output_root/dev-607-none/venues" -type f -name '*.jsonl' -print -quit | grep -q .; then
	fail "no-log cell contains venue JSONL evidence"
fi

full_runtime_events=$(jq -er '.events' "$output_root/dev-607/evidence-artifact-hash.json")
full_runtime_digest=$(jq -er '.digest' "$output_root/dev-607/evidence-artifact-hash.json")
g8_runtime_events=$(jq -er '.events' "$output_root/dev-607-g8/evidence-artifact-hash.json")
g8_runtime_digest=$(jq -er '.digest' "$output_root/dev-607-g8/evidence-artifact-hash.json")
[[ "$full_runtime_events" == "$g8_runtime_events" && "$full_runtime_digest" == "$g8_runtime_digest" ]] || fail "full runtime evidence hashes are not equal"

source_revision=$(jq -er '.git_revision' "$output_root/dev-607/run-metadata.json")
[[ "$source_revision" =~ ^[0-9a-f]{40}$ ]] || fail "parity simulator source revision is invalid"
binary_sha256=$(jq -er '.binary_sha256' "$output_root/dev-607/run-metadata.json")
binary_go_version=$(jq -er '.binary_go_version' "$output_root/dev-607/run-metadata.json")
prunegate_sha256=$(jq -er '.prunegate_sha256' "$output_root/dev-607/run-metadata.json")
prunegate_go_version=$(jq -er '.prunegate_go_version' "$output_root/dev-607/run-metadata.json")
prunegate_revision=$(jq -er '.prunegate_vcs_revision' "$output_root/dev-607/run-metadata.json")
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
	--arg contract "v2-integrated-longrun-parity-v2" \
	--arg source_revision "$source_revision" \
	--arg simulator_binary_sha256 "$binary_sha256" \
	--arg simulator_binary_go_version "$binary_go_version" \
	--arg prunegate_sha256 "$prunegate_sha256" \
	--arg prunegate_go_version "$prunegate_go_version" \
	--arg prunegate_revision "$prunegate_revision" \
	--arg full_g4_checkpoints "$(sha256sum "$output_root/dev-607/checkpoints.jsonl" | awk '{print $1}')" \
	--arg none_g4_checkpoints "$(sha256sum "$output_root/dev-607-none/checkpoints.jsonl" | awk '{print $1}')" \
	--arg full_g8_checkpoints "$(sha256sum "$output_root/dev-607-g8/checkpoints.jsonl" | awk '{print $1}')" \
	--arg greeks_sha256 "$(sha256sum "$output_root/dev-607/greeks.json" | awk '{print $1}')" \
	--arg latency_sha256 "$(sha256sum "$output_root/dev-607/latency.json" | awk '{print $1}')" \
	--arg evidence_hash_sha256 "$(sha256sum "$output_root/dev-607/evidence-artifact-hash.json" | awk '{print $1}')" \
	--argjson evidence_events "$full_runtime_events" \
	--arg evidence_digest "$full_runtime_digest" \
	'{
		schema_version: 1, contract: $contract, seed: 607, horizon: "24h",
		source_revision: $source_revision, simulator_revision: $source_revision,
		simulator_binary_sha256: $simulator_binary_sha256,
		simulator_binary_go_version: $simulator_binary_go_version,
		prunegate_sha256: $prunegate_sha256, prunegate_go_version: $prunegate_go_version,
		prunegate_revision: $prunegate_revision, controls: [
			{cell: "dev-607", log_mode: "full", gomaxprocs: 4},
			{cell: "dev-607-none", log_mode: "none", gomaxprocs: 4},
			{cell: "dev-607-g8", log_mode: "full", gomaxprocs: 8}
		],
		exact_equal_domains: ["checkpoints.jsonl", "greeks.json", "latency.json"],
		full_evidence_equal_domains: ["evidence-artifact-hash.json"],
		no_log_absence_contract: ["evidence-artifact-hash.json", "venues/*.jsonl"],
		hashes: {
			full_g4_checkpoints: $full_g4_checkpoints,
			none_g4_checkpoints: $none_g4_checkpoints,
			full_g8_checkpoints: $full_g8_checkpoints,
			greeks: $greeks_sha256, latency: $latency_sha256,
			full_evidence_artifact_hash: $evidence_hash_sha256
		},
		full_runtime_evidence: {events: $evidence_events, digest: $evidence_digest},
		predicates: {
			ordered_checkpoints_equal: true,
			deterministic_sidecars_equal: true,
			full_evidence_equal: true,
			no_log_evidence_absent: true,
			source_and_build_identity_equal: true
		}
	}' >"$tmp"
mv "$tmp" "$attestation"
require_object "$attestation"
jq -e '(.simulator_binary_sha256 | test("^[0-9a-f]{64}$")) and
	(.simulator_binary_go_version | startswith("go1.27")) and
	(.prunegate_sha256 | test("^[0-9a-f]{64}$")) and
	(.prunegate_go_version | startswith("go1.27")) and
	(.prunegate_revision | test("^[0-9a-f]{40}$")) and
	(.predicates | keys) == ["deterministic_sidecars_equal", "full_evidence_equal", "no_log_evidence_absent", "ordered_checkpoints_equal", "source_and_build_identity_equal"] and
	all(.predicates | to_entries[]; .value == true)' "$attestation" >/dev/null || fail "parity attestation self-check failed"
printf 'integrated long-run parity verified: %s\n' "$attestation"
