#!/usr/bin/env bash
# Seal a completed full-evidence cell into a lossless raw archive. The raw
# event bytes remain addressable through evidence-manifest.json; --prune-after-
# verify removes only the verified uncompressed copies, never the archive.
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
	printf 'usage: %s CELL_DIR [--prune-after-verify]\n' "$0" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-integrated-longrun-r5-contract.sh"
cell=$1
prune=false
if [[ ${2:-} == --prune-after-verify ]]; then
	prune=true
elif [[ $# -eq 2 ]]; then
	printf 'unknown archive option: %s\n' "$2" >&2
	exit 2
fi

fail() {
	printf 'integrated long-run raw archive failure: %s\n' "$*" >&2
	exit 1
}

v2_r5_require_output_root "$v2_r5_output_root" || fail "r5 output root is not canonical"
v2_r5_require_cell_path "$cell" || fail "cell is outside the canonical r5 evidence root or is symlinked: $cell"
cell=$(realpath -e -- "$cell")
cell_name=$(basename "$cell")
case "$cell_name" in
	dev-607|dev-613|dev-617|dev-607-g8) ;;
	*) fail "raw archiving is not registered for cell $cell_name" ;;
esac

[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] || fail "raw archiving requires a clean gate worktree"
required_inputs=(run-config.json run-metadata.json run-status.json manifest.json greeks.json latency.json checkpoints.jsonl evidence-manifest.json integrity.json)
if [[ "$cell_name" != dev-607-g8 ]]; then
	required_inputs+=(analysis-metadata.json)
fi
for input in "${required_inputs[@]}"; do
	[[ -s "$cell/$input" ]] || fail "missing completed-cell input: $cell/$input"
done
[[ "$(jq -er '.log_mode' "$cell/run-config.json")" == full ]] || fail "raw archive requires full log mode"
[[ ! -e "$(v2_r5_raw_archive_path "$cell")" ]] || fail "raw archive already exists"
[[ ! -e "$(v2_r5_raw_archive_descriptor_path "$cell")" ]] || fail "raw archive descriptor already exists"
v2_r5_require_raw_archive_attestation_path "$cell_name" || fail "raw archive attestation namespace is not canonical"
[[ ! -e "$(v2_r5_raw_archive_attestation_path "$cell_name")" ]] || fail "raw archive attestation already exists"
[[ ! -e "$cell/.raw-evidence-staged.$$" ]] || fail "staging marker collides with archive process"

jq -e '.exit_status == 0 and .completion_verified == true' "$cell/run-status.json" >/dev/null || fail "cell is not a completed run"
jq -e '.contract == "v2-integrated-longrun-candidate-v5" and
		(.predicates | type) == "object" and
		all(.predicates | to_entries[]; .value == true)' "$cell/integrity.json" >/dev/null ||
	fail "cell measurement contract has not passed"
v2_r5_verify_evidence_manifest "$cell" || fail "source evidence manifest does not verify"
v2_r5_verify_attestation "$cell" || fail "external evidence attestation does not verify"

if find "$cell/venues" -type l -print -quit 2>/dev/null | grep -q .; then
	fail "venue evidence contains a symlink"
fi
if find "$cell/venues" -type f ! -name '*.jsonl' -print -quit 2>/dev/null | grep -q .; then
	fail "venue evidence contains a non-JSONL regular file"
fi

archive=$(v2_r5_raw_archive_path "$cell")
archive_tmp="$archive.tmp-$$"
descriptor=$(v2_r5_raw_archive_descriptor_path "$cell")
descriptor_tmp="$descriptor.tmp-$$"
archive_attestation=$(v2_r5_raw_archive_attestation_path "$cell_name")
archive_attestation_tmp="$archive_attestation.tmp-$$"
trap 'rm -f -- "$archive_tmp" "$descriptor_tmp" "$archive_attestation_tmp"' EXIT
tar --format=posix --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
	--no-acls --no-xattrs --no-selinux --use-compress-program='zstd -q -T1' \
	-cf "$archive_tmp" -C "$cell" venues || fail "could not create raw archive"
[[ -s "$archive_tmp" ]] || fail "raw archive is empty"
zstd -q -t "$archive_tmp" || fail "raw archive compression test failed"
mv -- "$archive_tmp" "$archive"

archive_sha256=$(sha256sum -- "$archive" | awk '{print $1}')
archive_bytes=$(stat -c '%s' -- "$archive")
evidence_manifest_sha256=$(sha256sum -- "$cell/evidence-manifest.json" | awk '{print $1}')
run_status_sha256=$(sha256sum -- "$cell/run-status.json" | awk '{print $1}')
source_revision=$(jq -er '.git_revision' "$cell/run-metadata.json")
log_mode=$(jq -er '.log_mode' "$cell/run-config.json")
jq -n --arg cell "$cell_name" --arg source_revision "$source_revision" \
	--arg log_mode "$log_mode" \
	--arg archive_sha256 "$archive_sha256" --argjson archive_bytes "$archive_bytes" \
	--arg evidence_manifest_sha256 "$evidence_manifest_sha256" \
	--arg run_status_sha256 "$run_status_sha256" \
	--slurpfile evidence_manifest "$cell/evidence-manifest.json" \
	'{schema_version: 1, contract: "v2-integrated-longrun-raw-archive-v1", cell: $cell,
	 log_mode: $log_mode, source_revision: $source_revision, archive: "raw-evidence.tar.zst",
	 archive_sha256: $archive_sha256, archive_bytes: $archive_bytes,
	 evidence_manifest_sha256: $evidence_manifest_sha256,
	 run_status_sha256: $run_status_sha256,
	 raw_files: $evidence_manifest[0].raw_files,
	 raw_jsonl_files: $evidence_manifest[0].raw_jsonl_files,
	 raw_jsonl_bytes: $evidence_manifest[0].raw_jsonl_bytes}' >"$descriptor_tmp" || fail "could not write raw archive descriptor"
mv -- "$descriptor_tmp" "$descriptor"
descriptor_sha256=$(sha256sum -- "$descriptor" | awk '{print $1}')
mkdir -p -- "$v2_r5_attestation_root"
jq -n --arg cell "$cell_name" --arg source_revision "$source_revision" \
	--arg descriptor_sha256 "$descriptor_sha256" --arg archive_sha256 "$archive_sha256" \
	--arg evidence_manifest_sha256 "$evidence_manifest_sha256" --arg run_status_sha256 "$run_status_sha256" \
	'{schema_version: 1, contract: "v2-integrated-longrun-raw-archive-attestation-v1", cell: $cell,
	 source_revision: $source_revision, descriptor_sha256: $descriptor_sha256,
	 archive_sha256: $archive_sha256, evidence_manifest_sha256: $evidence_manifest_sha256,
	 run_status_sha256: $run_status_sha256,
	 attestation_scope: "immutable archive descriptor and raw payload binding"}' >"$archive_attestation_tmp" ||
	fail "could not write raw archive attestation"
mv -- "$archive_attestation_tmp" "$archive_attestation"
v2_r5_verify_raw_evidence_archive "$cell" || fail "raw archive descriptor or attestation does not verify"

if [[ "$prune" == true ]]; then
	relative=''
	while IFS= read -r relative; do
		v2_r5_validate_raw_path "$relative" || fail "manifest contains an unsafe raw path"
		local_path="$cell/$relative"
		[[ -f "$local_path" && ! -L "$local_path" ]] || fail "raw path disappeared before archive prune: $local_path"
		rm -- "$local_path"
	done < <(jq -r '.raw_files[].path' "$cell/evidence-manifest.json")
	v2_r5_verify_evidence_manifest "$cell" || fail "archived evidence manifest failed after raw prune"
fi

trap - EXIT
rm -f -- "$archive_tmp" "$descriptor_tmp" "$archive_attestation_tmp"
if [[ "$prune" == true ]]; then
	printf 'sealed and losslessly archived raw evidence: %s\n' "$cell"
else
	printf 'created lossless raw evidence archive: %s\n' "$cell"
fi
