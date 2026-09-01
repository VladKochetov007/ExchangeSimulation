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
registered_config="$root_dir/research/configs/v2-r2-sv1-24h/treatment-607.json"
expected_gomaxprocs=4
expected_minimum_free_bytes=$((4 * 1024 * 1024 * 1024))
temporary_paths=()

fail() {
	echo "SV1 capacity archive failure: $*" >&2
	exit 1
}

cleanup_temporary_paths() {
	local path
	for path in "${temporary_paths[@]}"; do
		if [[ -e "$path" || -L "$path" ]]; then
			unlink -- "$path" 2>/dev/null || true
		fi
	done
}

new_temporary_file() {
	local template=$1
	temporary_path=$(mktemp --tmpdir="$archive_parent" -- "$template") ||
		fail "could not create a secure temporary file in $archive_parent"
	temporary_paths+=("$temporary_path")
}

publish_new_file() {
	local source_path=$1
	local destination_path=$2
	[[ ! -e "$destination_path" && ! -L "$destination_path" ]] ||
		fail "publication target appeared or already exists: $destination_path"
	ln -- "$source_path" "$destination_path" ||
		fail "could not publish without replacement: $destination_path"
	unlink -- "$source_path" || fail "could not retire temporary file: $source_path"
}

require_same_filesystem() {
	local path device expected_device
	expected_device=$(stat -c '%d' -- "$probe_root") || fail "could not stat probe root"
	while IFS= read -r -d '' path; do
		[[ ! -L "$path" ]] || fail "probe contains a symlink: $path"
		device=$(stat -c '%d' -- "$path") || fail "could not stat probe path: $path"
		[[ "$device" == "$expected_device" ]] || fail "probe contains a nested mount: $path"
	done < <(find -P "$probe_root" -xdev -print0)
}

v2_r2_acquire_namespace_lock || fail "could not acquire the SV1 capacity namespace lock"
trap cleanup_temporary_paths EXIT
[[ -z "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]] ||
	fail "scientific worktree must be clean"
[[ -f "$attestation" && ! -L "$attestation" && -s "$attestation" ]] ||
	fail "capacity attestation is missing or symlinked: $attestation"
[[ -x "$binary" && ! -L "$binary" ]] || fail "validating binary is missing or symlinked: $binary"
[[ -f "$registered_config" && ! -L "$registered_config" && -s "$registered_config" ]] ||
	fail "registered treatment configuration is missing or symlinked"
[[ "$archive" == /* && "$archive" != */ && "$archive" != *$'\n'* && "$archive" != *$'\t'* ]] ||
	fail "archive path must be an absolute, non-empty path"
archive=$(realpath -m -- "$archive") || fail "could not resolve archive path"
archive_parent=$(dirname -- "$archive")
archive_base=$(basename -- "$archive")
[[ -d "$archive_parent" && ! -L "$archive_parent" ]] || fail "archive parent is not a directory: $archive_parent"
[[ ! -e "$archive" && ! -L "$archive" ]] || fail "archive already exists: $archive"
for sidecar in "$archive.members" "$archive.sha256" "$archive.compare.log" "$archive.retention.json"; do
	[[ ! -e "$sidecar" && ! -L "$sidecar" ]] || fail "archive sidecar already exists: $sidecar"
done

source_revision=$(jq -er '.source_revision | select(type == "string" and test("^[0-9a-f]{40}$"))' "$attestation") ||
	fail "capacity attestation has no valid source revision"
[[ "$source_revision" != "$current_revision" ]] ||
	fail "refusing to compact a capacity probe for the current source revision"
git -C "$root_dir" cat-file -e "$source_revision^{commit}" ||
	fail "capacity attestation source revision is not a repository commit"
git -C "$root_dir" merge-base --is-ancestor "$source_revision" "$current_revision" ||
	fail "capacity attestation source revision is not an ancestor of current HEAD"

binary_sha256=$(sha256sum -- "$binary" | awk '{print $1}')
binary_revision=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.revision=") == 1 {sub("vcs.revision=", "", $2); print $2; exit}')
binary_modified=$(go version -m "$binary" | awk '$1 == "build" && index($2, "vcs.modified=") == 1 {sub("vcs.modified=", "", $2); print $2; exit}')
[[ "$binary_revision" == "$source_revision" && "$binary_modified" == false ]] ||
	fail "validating binary provenance does not match the attested source revision"

config_sha256=$(sha256sum -- "$registered_config" | awk '{print $1}')
attested_config_sha256=$(jq -er '.config_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$attestation") ||
	fail "capacity attestation has no valid configuration hash"
[[ "$attested_config_sha256" == "$config_sha256" ]] ||
	fail "capacity attestation configuration hash is not the registered treatment hash"
gomaxprocs=$(jq -er '.gomaxprocs | select(type == "number" and floor == .)' "$attestation") ||
	fail "capacity attestation has no integral process width"
minimum_free_bytes=$(jq -er '.minimum_free_bytes | select(type == "number" and floor == .)' "$attestation") ||
	fail "capacity attestation has no integral minimum-free reserve"
[[ "$gomaxprocs" == "$expected_gomaxprocs" && "$minimum_free_bytes" == "$expected_minimum_free_bytes" ]] ||
	fail "capacity attestation parameters are not the registered capacity parameters"

probe_root_declared=$(jq -er '.probe_root | select(type == "string" and length > 0)' "$attestation") ||
	fail "capacity attestation has no probe root"
[[ -d "$probe_root_declared" && ! -L "$probe_root_declared" ]] ||
	fail "capacity probe root is missing or symlinked"
probe_root=$(realpath -e -- "$probe_root_declared") || fail "capacity probe root is not present"
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
[[ -f "$probe_cell/run-config.json" && ! -L "$probe_cell/run-config.json" ]] || fail "capacity run config is missing or symlinked"
cmp -s "$registered_config" "$probe_cell/run-config.json" || fail "capacity run config is not the registered treatment config"
binary_sha256_from_attestation=$(jq -er '.binary_sha256 | select(type == "string" and test("^[0-9a-f]{64}$"))' "$attestation") ||
	fail "capacity attestation has no valid binary hash"
[[ "$binary_sha256_from_attestation" == "$binary_sha256" ]] || fail "capacity attestation binary hash mismatch"
jq -e --arg revision "$source_revision" --arg binary_sha256 "$binary_sha256" --arg config_sha256 "$config_sha256" \
	--argjson gomaxprocs "$expected_gomaxprocs" --argjson minimum_free_bytes "$expected_minimum_free_bytes" \
	'.schema_version == 1 and .contract == "v2-r2-sv1-24h-capacity-probe-v1" and
	 .git_revision == $revision and .binary_sha256 == $binary_sha256 and .config_sha256 == $config_sha256 and
	 .gomaxprocs == $gomaxprocs and .minimum_free_bytes == $minimum_free_bytes and
	 .seed == 607 and .simulated_horizon == "24h" and .evidence_format == "evstream_v3"' \
	"$probe_cell/run-metadata.json" >/dev/null || fail "capacity run metadata is not cross-bound to its attestation"
jq -e --arg revision "$source_revision" \
	'.schema_version == 2 and .build.revision == $revision and .build.modified == false and
	 .config.seed == 607 and .config.log_mode == "full" and .config.evidence_format == "evstream_v3" and
	 .config.experiment_id == "v2-r2-sv1-24h-treatment-607"' \
	"$probe_cell/manifest.json" >/dev/null || fail "capacity manifest build/config identity is not cross-bound"
require_same_filesystem
v2_r2_require_binary_capacity_attestation "$binary" "$source_revision" "$attestation" \
	"$config_sha256" "$expected_gomaxprocs" "$expected_minimum_free_bytes" ||
	fail "capacity attestation or retained probe evidence did not validate"

attestation_real=$(realpath -e -- "$attestation") || fail "could not resolve attestation"
case "$attestation_real" in
	/home/vlad/v2-r2-sv1-24h-binary-capacity-*.json) ;;
	*) fail "capacity attestation is outside the dedicated SV1 attestation namespace" ;;
esac
probe_relative=${probe_root#/home/vlad/}
attestation_relative=${attestation_real#/home/vlad/}
new_temporary_file ".${archive_base}.archive.XXXXXX"
archive_tmp=$temporary_path
new_temporary_file ".${archive_base}.members.XXXXXX"
members_tmp=$temporary_path
new_temporary_file ".${archive_base}.compare.XXXXXX"
compare_tmp=$temporary_path
new_temporary_file ".${archive_base}.sha256.XXXXXX"
sha_tmp=$temporary_path
new_temporary_file ".${archive_base}.retention.XXXXXX"
receipt_tmp=$temporary_path

tar --sort=name --use-compress-program='zstd -q -T1' -cf "$archive_tmp" \
	-C /home/vlad "$probe_relative" "$attestation_relative" ||
	fail "capacity probe archive creation failed"
publish_new_file "$archive_tmp" "$archive"
zstd -t -- "$archive" || fail "capacity probe archive failed zstd integrity test"
tar --use-compress-program='zstd -q' -tf "$archive" | sort >"$members_tmp" ||
	fail "capacity probe archive member enumeration failed"
publish_new_file "$members_tmp" "$archive.members"
tar --use-compress-program='zstd -q' --compare --file="$archive" --directory=/home/vlad \
	>"$compare_tmp" 2>&1 || fail "capacity probe archive comparison failed"
[[ ! -s "$compare_tmp" ]] || fail "capacity probe archive comparison reported differences"
publish_new_file "$compare_tmp" "$archive.compare.log"
archive_sha256=$(sha256sum -- "$archive" | awk '{print $1}')
printf '%s  %s\n' "$archive_sha256" "$archive" >"$sha_tmp"
publish_new_file "$sha_tmp" "$archive.sha256"
probe_bytes=$(du -sb -- "$probe_root" | awk '{print $1}')
member_count=$(grep -vc '/$' "$archive.members")
jq -n \
	--arg contract "v2-r2-sv1-capacity-archive-retention-v1" \
	--arg source_revision "$source_revision" \
	--arg protocol_revision "$current_revision" \
	--arg probe_root "$probe_root" \
	--arg attestation "$attestation_real" \
	--arg attestation_sha256 "$(sha256sum -- "$attestation_real" | awk '{print $1}')" \
	--arg validating_binary "$binary" \
	--arg validating_binary_sha256 "$binary_sha256" \
	--arg archive "$archive" \
	--arg archive_sha256 "$archive_sha256" \
	--arg members "$archive.members" \
	--arg compare_log "$archive.compare.log" \
	--arg config_sha256 "$config_sha256" \
	--argjson probe_bytes "$probe_bytes" \
	--argjson member_count "$member_count" \
	'{schema_version: 1, contract: $contract, source_revision: $source_revision,
	 protocol_revision: $protocol_revision, probe_root: $probe_root,
	 attestation: $attestation, attestation_sha256: $attestation_sha256,
	 validating_binary: $validating_binary, validating_binary_sha256: $validating_binary_sha256,
	 config_sha256: $config_sha256, archive: $archive,
	 archive_sha256: $archive_sha256, archive_members: $members,
	 archive_compare_log: $compare_log, probe_bytes: $probe_bytes,
	 member_count: $member_count, comparison: "tar_compare_clean",
	 disposition: "superseded_capacity_probe_compacted_before_new_revision_bound_measurement"}' \
	>"$receipt_tmp"
publish_new_file "$receipt_tmp" "$archive.retention.json"

zstd -t -- "$archive" || fail "capacity probe archive failed final integrity test"
new_temporary_file ".${archive_base}.final-members.XXXXXX"
final_members_tmp=$temporary_path
tar --use-compress-program='zstd -q' -tf "$archive" | sort >"$final_members_tmp" || fail "final member enumeration failed"
cmp -s "$archive.members" "$final_members_tmp" || fail "published member list is stale"
unlink -- "$final_members_tmp"
new_temporary_file ".${archive_base}.final-compare.XXXXXX"
final_compare_tmp=$temporary_path
tar --use-compress-program='zstd -q' --compare --file="$archive" --directory=/home/vlad \
	>"$final_compare_tmp" 2>&1 || fail "final capacity probe archive comparison failed"
[[ ! -s "$final_compare_tmp" ]] || fail "final capacity probe archive comparison reported differences"
unlink -- "$final_compare_tmp"
[[ "$archive_sha256" == "$(sha256sum -- "$archive" | awk '{print $1}')" ]] || fail "archive changed after publication"
read -r published_sha256 published_archive <"$archive.sha256"
[[ "$published_sha256" == "$archive_sha256" && "$published_archive" == "$archive" ]] || fail "published archive checksum is stale"
jq -e --arg archive "$archive" --arg archive_sha256 "$archive_sha256" --arg source_revision "$source_revision" \
	--arg config_sha256 "$config_sha256" --argjson probe_bytes "$probe_bytes" \
	'.schema_version == 1 and .contract == "v2-r2-sv1-capacity-archive-retention-v1" and
	 .source_revision == $source_revision and .config_sha256 == $config_sha256 and
	 .archive == $archive and .archive_sha256 == $archive_sha256 and .probe_bytes == $probe_bytes and
	 .comparison == "tar_compare_clean"' "$archive.retention.json" >/dev/null ||
	fail "published retention receipt is not cross-bound"
require_same_filesystem

find -P "$probe_root" -xdev -depth -mindepth 1 -delete
rmdir -- "$probe_root"
unlink -- "$attestation_real"
[[ ! -e "$probe_root" && ! -e "$attestation_real" ]] || fail "capacity probe compaction was incomplete"
echo "compacted superseded SV1 capacity probe: archive=$archive bytes=$probe_bytes sha256=$archive_sha256"
