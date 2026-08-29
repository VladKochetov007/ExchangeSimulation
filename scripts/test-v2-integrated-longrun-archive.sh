#!/usr/bin/env bash
# Cheap round-trip and tamper tests for archived raw evidence.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp_root=$(mktemp -d)
trap 'rm -rf "$tmp_root"' EXIT

fail() {
	printf 'integrated long-run archive test failure: %s\n' "$*" >&2
	exit 1
}

expect_failure() {
	if "$@" >/dev/null 2>&1; then
		fail "command unexpectedly succeeded: $*"
	fi
}

source "$root_dir/scripts/v2-integrated-longrun-r5-contract.sh"
v2_r5_attestation_root="$tmp_root/attestations"
mkdir -p "$tmp_root/attestation-target"
ln -s "$tmp_root/attestation-target" "$tmp_root/attestation-link"
v2_r5_attestation_root="$tmp_root/attestation-link"
expect_failure v2_r5_require_raw_archive_attestation_path dev-607
v2_r5_attestation_root="$tmp_root/attestations"
cell="$tmp_root/dev-607"
mkdir -p "$cell/venues/north"
printf '%s\n' '{"sequence":1,"event":"trade"}' >"$cell/venues/north/events.jsonl"
printf '%s\n' '{"log_mode":"full"}' >"$cell/run-config.json"
printf '%s\n' '{"git_revision":"0123456789abcdef0123456789abcdef01234567"}' >"$cell/run-metadata.json"
printf '%s\n' '{"exit_status":0}' >"$cell/run-status.json"

raw_bytes=$(stat -c '%s' "$cell/venues/north/events.jsonl")
raw_sha256=$(sha256sum "$cell/venues/north/events.jsonl" | awk '{print $1}')
cat_manifest=$(jq -n --arg sha256 "$raw_sha256" --argjson bytes "$raw_bytes" \
	'{schema_version: 1, contract: "v2-integrated-longrun-evidence-manifest-v1", cell: "dev-607",
	 log_mode: "full", source_revision: "0123456789abcdef0123456789abcdef01234567",
	 fixed_files: [], raw_jsonl_files: 1, raw_jsonl_bytes: $bytes,
	 raw_files: [{path: "venues/north/events.jsonl", bytes: $bytes, sha256: $sha256}]}' )
printf '%s\n' "$cat_manifest" >"$cell/evidence-manifest.json"

archive=$(v2_r5_raw_archive_path "$cell")
tar --format=posix --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
	--no-acls --no-xattrs --no-selinux --use-compress-program='zstd -q -T1' \
	-cf "$archive" -C "$cell" venues
archive_sha256=$(sha256sum "$archive" | awk '{print $1}')
archive_bytes=$(stat -c '%s' "$archive")
manifest_sha256=$(sha256sum "$cell/evidence-manifest.json" | awk '{print $1}')
status_sha256=$(sha256sum "$cell/run-status.json" | awk '{print $1}')
jq -n --arg archive_sha256 "$archive_sha256" --arg manifest_sha256 "$manifest_sha256" \
	--arg status_sha256 "$status_sha256" --arg raw_sha256 "$raw_sha256" --argjson raw_bytes "$raw_bytes" \
	--argjson archive_bytes "$archive_bytes" \
	'{schema_version: 1, contract: "v2-integrated-longrun-raw-archive-v1", cell: "dev-607",
	 log_mode: "full", source_revision: "0123456789abcdef0123456789abcdef01234567",
	 archive: "raw-evidence.tar.zst", archive_sha256: $archive_sha256, archive_bytes: $archive_bytes,
	 evidence_manifest_sha256: $manifest_sha256, run_status_sha256: $status_sha256,
	 raw_jsonl_files: 1, raw_jsonl_bytes: $raw_bytes,
	 raw_files: [{path: "venues/north/events.jsonl", bytes: $raw_bytes, sha256: $raw_sha256}]}' \
	>"$(v2_r5_raw_archive_descriptor_path "$cell")" || fail "could not write descriptor"
	descriptor_sha256=$(sha256sum "$(v2_r5_raw_archive_descriptor_path "$cell")" | awk '{print $1}')
mkdir -p "$v2_r5_attestation_root"
jq -n --arg descriptor_sha256 "$descriptor_sha256" --arg archive_sha256 "$archive_sha256" \
	--arg manifest_sha256 "$manifest_sha256" --arg status_sha256 "$status_sha256" \
	'{schema_version: 1, contract: "v2-integrated-longrun-raw-archive-attestation-v1", cell: "dev-607",
	 source_revision: "0123456789abcdef0123456789abcdef01234567", descriptor_sha256: $descriptor_sha256,
	 archive_sha256: $archive_sha256, evidence_manifest_sha256: $manifest_sha256,
	 run_status_sha256: $status_sha256}' \
	>"$(v2_r5_raw_archive_attestation_path dev-607)"

v2_r5_verify_raw_evidence_archive "$cell" || fail "valid archive was rejected"
rm -- "$cell/venues/north/events.jsonl"
v2_r5_stage_raw_evidence "$cell" || fail "valid archive did not stage"
[[ -e "$cell/.raw-evidence-staged.$$" ]] || fail "staging marker was not created"
cmp -s <(printf '%s\n' '{"sequence":1,"event":"trade"}') "$cell/venues/north/events.jsonl" ||
	fail "staged raw evidence differs from source"
v2_r5_cleanup_staged_raw_evidence "$cell" || fail "staged raw evidence did not clean up"
[[ ! -e "$cell/venues/north/events.jsonl" && ! -e "$cell/.raw-evidence-staged.$$" ]] ||
	fail "cleanup left staged evidence behind"

mv "$archive" "$tmp_root/archive-original.tar.zst"
cp "$tmp_root/archive-original.tar.zst" "$archive"
printf 'tamper' >>"$archive"
expect_failure v2_r5_verify_raw_evidence_archive "$cell"

cp "$tmp_root/archive-original.tar.zst" "$archive"

content_cell="$tmp_root/dev-613"
cp -a "$cell" "$content_cell"
jq '.cell = "dev-613"' "$content_cell/evidence-manifest.json" >"$tmp_root/content-manifest.json"
mv "$tmp_root/content-manifest.json" "$content_cell/evidence-manifest.json"
content_manifest_sha256=$(sha256sum "$content_cell/evidence-manifest.json" | awk '{print $1}')
mkdir -p "$tmp_root/tampered/venues/north"
printf '%s\n' '{"sequence":2,"event":"tampered"}' >"$tmp_root/tampered/venues/north/events.jsonl"
tar --format=posix --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
	--no-acls --no-xattrs --no-selinux --use-compress-program='zstd -q -T1' \
	-cf "$content_cell/raw-evidence.tar.zst" -C "$tmp_root/tampered" venues
tampered_archive_sha256=$(sha256sum "$content_cell/raw-evidence.tar.zst" | awk '{print $1}')
tampered_archive_bytes=$(stat -c '%s' "$content_cell/raw-evidence.tar.zst")
jq --arg cell "dev-613" --arg archive_sha256 "$tampered_archive_sha256" \
	--arg manifest_sha256 "$content_manifest_sha256" --argjson archive_bytes "$tampered_archive_bytes" \
	'.cell = $cell | .archive_sha256 = $archive_sha256 |
	 .archive_bytes = $archive_bytes | .evidence_manifest_sha256 = $manifest_sha256' \
	"$(v2_r5_raw_archive_descriptor_path "$content_cell")" >"$tmp_root/content-descriptor.json"
mv "$tmp_root/content-descriptor.json" "$(v2_r5_raw_archive_descriptor_path "$content_cell")"
content_descriptor_sha256=$(sha256sum "$(v2_r5_raw_archive_descriptor_path "$content_cell")" | awk '{print $1}')
jq -n --arg descriptor_sha256 "$content_descriptor_sha256" --arg archive_sha256 "$tampered_archive_sha256" \
	--arg manifest_sha256 "$content_manifest_sha256" --arg status_sha256 "$status_sha256" \
	'{schema_version: 1, contract: "v2-integrated-longrun-raw-archive-attestation-v1", cell: "dev-613",
	 source_revision: "0123456789abcdef0123456789abcdef01234567", descriptor_sha256: $descriptor_sha256,
	 archive_sha256: $archive_sha256, evidence_manifest_sha256: $manifest_sha256,
	 run_status_sha256: $status_sha256}' \
	>"$(v2_r5_raw_archive_attestation_path dev-613)"
expect_failure v2_r5_verify_raw_evidence_archive "$content_cell"

rm -rf "$tmp_root/tampered"
mkdir -p "$tmp_root/symlinked/venues/north"
ln -s /dev/null "$tmp_root/symlinked/venues/north/events.jsonl"
tar --format=posix --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
	--no-acls --no-xattrs --no-selinux --use-compress-program='zstd -q -T1' \
	-cf "$content_cell/raw-evidence.tar.zst" -C "$tmp_root/symlinked" venues
symlink_archive_sha256=$(sha256sum "$content_cell/raw-evidence.tar.zst" | awk '{print $1}')
symlink_archive_bytes=$(stat -c '%s' "$content_cell/raw-evidence.tar.zst")
jq --arg archive_sha256 "$symlink_archive_sha256" --argjson archive_bytes "$symlink_archive_bytes" \
	'.archive_sha256 = $archive_sha256 | .archive_bytes = $archive_bytes' \
	"$(v2_r5_raw_archive_descriptor_path "$content_cell")" >"$tmp_root/symlink-descriptor.json"
mv "$tmp_root/symlink-descriptor.json" "$(v2_r5_raw_archive_descriptor_path "$content_cell")"
symlink_descriptor_sha256=$(sha256sum "$(v2_r5_raw_archive_descriptor_path "$content_cell")" | awk '{print $1}')
jq --arg descriptor_sha256 "$symlink_descriptor_sha256" --arg archive_sha256 "$symlink_archive_sha256" \
	'.descriptor_sha256 = $descriptor_sha256 | .archive_sha256 = $archive_sha256' \
	"$(v2_r5_raw_archive_attestation_path dev-613)" >"$tmp_root/symlink-attestation.json"
mv "$tmp_root/symlink-attestation.json" "$(v2_r5_raw_archive_attestation_path dev-613)"
expect_failure v2_r5_verify_raw_evidence_archive "$content_cell"

printf 'integrated long-run archive tests: pass\n'
