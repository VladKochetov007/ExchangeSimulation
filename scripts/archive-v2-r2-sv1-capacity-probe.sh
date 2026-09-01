#!/usr/bin/env bash
# Compact a superseded, already-validated capacity probe without losing its
# raw evidence. The active capacity attestation is never compacted in place.
set -euo pipefail

if [[ $# -ne 3 ]]; then
	echo "usage: $0 CAPACITY_ATTESTATION VALIDATING_BINARY ARCHIVE_PATH" >&2
	exit 2
fi

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source "$root_dir/scripts/v2-r2-sv1-24h-contract.sh"

attestation=$1
binary=$2
archive=$3
current_revision=$(git -C "$root_dir" rev-parse HEAD)
capacity_parent=/home/vlad/external-scratch

fail() {
	echo "SV1 capacity archive failure: $*" >&2
	exit 1
}

v2_r2_acquire_namespace_lock || fail "could not acquire the SV1 capacity namespace lock"
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] ||
	fail "scientific worktree must be clean"
[[ -f "$attestation" && ! -L "$attestation" && -s "$attestation" ]] ||
	fail "capacity attestation is missing or symlinked: $attestation"
[[ -x "$binary" && ! -L "$binary" ]] || fail "validating binary is missing or symlinked: $binary"
[[ "$archive" == /* && "$archive" != */ && "$archive" != *$'\n'* && "$archive" != *$'\t'* ]] ||
	fail "archive path must be an absolute, non-empty path"
archive=$(realpath -m -- "$archive") || fail "could not resolve archive path"
archive_parent=$(dirname -- "$archive")
[[ -d "$archive_parent" && ! -L "$archive_parent" ]] || fail "archive parent is not a directory: $archive_parent"
[[ ! -e "$archive" && ! -L "$archive" ]] || fail "archive already exists: $archive"
for sidecar in "$archive.members" "$archive.sha256" "$archive.compare.log" "$archive.retention.json"; do
	[[ ! -e "$sidecar" && ! -L "$sidecar" ]] || fail "archive sidecar already exists: $sidecar"
done

source_revision=$(jq -er '.source_revision | select(type == "string" and test("^[0-9a-f]{40}$"))' "$attestation") ||
	fail "capacity attestation has no valid source revision"
[[ "$source_revision" != "$current_revision" ]] ||
	fail "refusing to compact a capacity probe for the current source revision"
config_sha256=$(jq -er '.config_sha256 | select(type == "string")' "$attestation") ||
	fail "capacity attestation has no configuration hash"
gomaxprocs=$(jq -er '.gomaxprocs | select(type == "number")' "$attestation") ||
	fail "capacity attestation has no process width"
minimum_free_bytes=$(jq -er '.minimum_free_bytes | select(type == "number")' "$attestation") ||
	fail "capacity attestation has no minimum-free reserve"
probe_root=$(jq -er '.probe_root | select(type == "string")' "$attestation") ||
	fail "capacity attestation has no probe root"
[[ -d "$probe_root" && ! -L "$probe_root" ]] || fail "capacity probe root is missing or symlinked"
probe_root=$(realpath -e -- "$probe_root") || fail "capacity probe root is not present: $probe_root"
case "$probe_root" in
	"$capacity_parent"/v2-r2-sv1-24h-capacity-*) ;;
	*) fail "capacity probe root is outside the dedicated SV1 capacity namespace: $probe_root" ;;
esac
[[ -d "$probe_root" && ! -L "$probe_root" ]] || fail "capacity probe root is not a directory"
case "$archive" in
	"$root_dir"|"$root_dir"/*) fail "capacity probe archive cannot be inside the scientific repository" ;;
	"$probe_root"|"$probe_root"/*) fail "capacity probe archive cannot be inside the source probe root" ;;
esac
probe_cell="$probe_root/treatment-607"
[[ -d "$probe_cell" && ! -L "$probe_cell" ]] || fail "capacity probe cell is missing"
v2_r2_require_binary_capacity_attestation "$binary" "$source_revision" "$attestation" \
	"$config_sha256" "$gomaxprocs" "$minimum_free_bytes" ||
	fail "capacity attestation or retained probe evidence did not validate"

attestation_real=$(realpath -e -- "$attestation") || fail "could not resolve attestation"
case "$attestation_real" in
	/home/vlad/v2-r2-sv1-24h-binary-capacity-*.json) ;;
	*) fail "capacity attestation is outside the dedicated SV1 attestation namespace" ;;
esac
probe_relative=${probe_root#/home/vlad/}
attestation_relative=${attestation_real#/home/vlad/}
archive_tmp="$archive.tmp-$$"
[[ ! -e "$archive_tmp" && ! -L "$archive_tmp" ]] || fail "archive temporary path already exists"
cleanup_archive_tmp() {
	if [[ -e "$archive_tmp" || -L "$archive_tmp" ]]; then
		unlink -- "$archive_tmp" 2>/dev/null || true
	fi
}
trap cleanup_archive_tmp EXIT

tar --sort=name --use-compress-program='zstd -q -T1' -cf "$archive_tmp" \
	-C /home/vlad "$probe_relative" "$attestation_relative" ||
	fail "capacity probe archive creation failed"
mv -- "$archive_tmp" "$archive"
zstd -t -- "$archive" || fail "capacity probe archive failed zstd integrity test"
tar --use-compress-program='zstd -q' -tf "$archive" | sort >"$archive.members" ||
	fail "capacity probe archive member enumeration failed"
tar --use-compress-program='zstd -q' --compare --file="$archive" --directory=/home/vlad \
	>"$archive.compare.log" 2>&1 || fail "capacity probe archive comparison failed"
[[ ! -s "$archive.compare.log" ]] || fail "capacity probe archive comparison reported differences"
archive_sha256=$(sha256sum -- "$archive" | awk '{print $1}')
attestation_sha256=$(sha256sum -- "$attestation_real" | awk '{print $1}')
probe_bytes=$(du -sb -- "$probe_root" | awk '{print $1}')
member_count=$(grep -vc '/$' "$archive.members")
jq -n \
	--arg contract "v2-r2-sv1-capacity-archive-retention-v1" \
	--arg source_revision "$source_revision" \
	--arg protocol_revision "$current_revision" \
	--arg probe_root "$probe_root" \
	--arg attestation "$attestation_real" \
	--arg attestation_sha256 "$attestation_sha256" \
	--arg validating_binary "$binary" \
	--arg archive "$archive" \
	--arg archive_sha256 "$archive_sha256" \
	--arg members "$archive.members" \
	--arg compare_log "$archive.compare.log" \
	--argjson probe_bytes "$probe_bytes" \
	--argjson member_count "$member_count" \
	'{schema_version: 1, contract: $contract, source_revision: $source_revision,
	 protocol_revision: $protocol_revision, probe_root: $probe_root,
	 attestation: $attestation, attestation_sha256: $attestation_sha256,
	 validating_binary: $validating_binary, archive: $archive,
	 archive_sha256: $archive_sha256, archive_members: $members,
	 archive_compare_log: $compare_log, probe_bytes: $probe_bytes,
	 member_count: $member_count, comparison: "tar_compare_clean",
	 disposition: "superseded_capacity_probe_compacted_before_new_revision_bound_measurement"}' \
	>"$archive.retention.json.tmp"
mv -- "$archive.retention.json.tmp" "$archive.retention.json"
printf '%s  %s\n' "$archive_sha256" "$archive" >"$archive.sha256"

zstd -t -- "$archive" || fail "capacity probe archive failed final integrity test"
find "$probe_root" -depth -mindepth 1 -delete
rmdir -- "$probe_root"
unlink -- "$attestation_real"
[[ ! -e "$probe_root" && ! -e "$attestation_real" ]] || fail "capacity probe compaction was incomplete"
echo "compacted superseded SV1 capacity probe: archive=$archive bytes=$probe_bytes sha256=$archive_sha256"
