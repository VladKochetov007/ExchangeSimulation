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
manifest_sha256=$(sha256sum "$cell/evidence-manifest.json" | awk '{print $1}')
status_sha256=$(sha256sum "$cell/run-status.json" | awk '{print $1}')
jq -n --arg archive_sha256 "$archive_sha256" --arg manifest_sha256 "$manifest_sha256" \
	--arg status_sha256 "$status_sha256" --arg raw_sha256 "$raw_sha256" --argjson raw_bytes "$raw_bytes" \
	'{schema_version: 1, contract: "v2-integrated-longrun-raw-archive-v1", cell: "dev-607",
	 log_mode: "full", source_revision: "0123456789abcdef0123456789abcdef01234567",
	 archive: "raw-evidence.tar.zst", archive_sha256: $archive_sha256,
	 evidence_manifest_sha256: $manifest_sha256, run_status_sha256: $status_sha256,
	 raw_files: [{path: "venues/north/events.jsonl", bytes: $raw_bytes, sha256: $raw_sha256}]}' \
	>"$(v2_r5_raw_archive_descriptor_path "$cell")" || fail "could not write descriptor"

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

printf 'integrated long-run archive tests: pass\n'
