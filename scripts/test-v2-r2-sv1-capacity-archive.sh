#!/usr/bin/env bash
# Exercise the superseded-capacity retention protocol with a real Go binary,
# a valid binary-evidence manifest, and exact archive publication checks.
set -euo pipefail

root_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
archiver="$root_dir/scripts/archive-v2-r2-sv1-capacity-probe.sh"
tmp_root=$(mktemp -d)
probe_root="/home/vlad/external-scratch/v2-r2-sv1-24h-capacity-contract-${BASHPID}"
attestation="/home/vlad/v2-r2-sv1-24h-binary-capacity-contract-${BASHPID}.json"
archive="/home/vlad/v2-r2-sv1-capacity-contract-${BASHPID}.tar.zst"
collision_archive="/home/vlad/v2-r2-sv1-capacity-contract-collision-${BASHPID}.tar.zst"
old_worktree="$tmp_root/old-source"
validator="$tmp_root/evsrender"
probe_cell="$probe_root/treatment-607"
probe_real="$probe_root.real"

fail() {
	echo "SV1 capacity archive fixture failure: $*" >&2
	exit 1
}

cleanup() {
	git -C "$root_dir" worktree remove --force "$old_worktree" >/dev/null 2>&1 || true
	rm -rf -- "$old_worktree" "$tmp_root" "$probe_root" "$probe_real"
	rm -f -- "$attestation" "$archive" "$archive.members" "$archive.sha256" \
		"$archive.compare.log" "$archive.retention.json" \
		"$collision_archive" "$collision_archive.members" "$collision_archive.sha256" \
		"$collision_archive.compare.log" "$collision_archive.retention.json"
}
trap cleanup EXIT

if [[ -n "$(git -C "$root_dir" status --porcelain --untracked-files=all)" ]]; then
	echo "SV1 capacity archive fixture: skipped because the scientific worktree is dirty"
	exit 0
fi

[[ -d /home/vlad/external-scratch && ! -L /home/vlad/external-scratch ]] ||
	fail "external scratch namespace is unavailable"
for path in "$probe_root" "$attestation" "$archive" "$collision_archive"; do
	[[ ! -e "$path" && ! -L "$path" ]] || fail "fixture path already exists: $path"
done

source "$root_dir/scripts/v2-integrated-longrun-r2-contract.sh"
source "$root_dir/scripts/v2-r2-evidence-input-contract.sh"

source_revision=$(git -C "$root_dir" rev-parse HEAD^)
git -C "$root_dir" worktree add --detach "$old_worktree" "$source_revision" >/dev/null
(
	cd "$old_worktree"
	CGO_ENABLED=0 GOAMD64=v1 GOTOOLCHAIN=local go build -trimpath -buildvcs=true \
		-o "$validator" ./cmd/evsrender
)
[[ -x "$validator" ]] || fail "parent-revision validator binary was not built"
binary_sha256=$(sha256sum -- "$validator" | awk '{print $1}')
binary_go_version=$(v2_r2_binary_go_version "$validator")
config="$root_dir/research/configs/v2-r2-sv1-24h/treatment-607.json"
config_sha256=$(sha256sum -- "$config" | awk '{print $1}')
minimum_free_bytes=$((4 * 1024 * 1024 * 1024))
safety_margin_bytes=$((2 * 1024 * 1024 * 1024))
required_free_bytes=$((1 + safety_margin_bytes))

mkdir -p -- "$probe_cell/venues/north"
cp -- "$config" "$probe_cell/run-config.json"
jq -n \
	--arg source_revision "$source_revision" \
	--arg binary_sha256 "$binary_sha256" \
	--arg config_sha256 "$config_sha256" \
	--arg binary_go_version "$binary_go_version" \
	--argjson minimum_free_bytes "$minimum_free_bytes" \
	'{schema_version: 1, contract: "v2-r2-sv1-24h-capacity-probe-v1",
	 git_revision: $source_revision, source_revision: $source_revision,
	 binary_sha256: $binary_sha256, config_sha256: $config_sha256,
	 binary_go_version: $binary_go_version, gomaxprocs: 4,
	 minimum_free_bytes: $minimum_free_bytes, seed: 607,
	 simulated_horizon: "24h", evidence_format: "evstream_v3"}' \
	>"$probe_cell/run-metadata.json"
jq -n \
	--arg source_revision "$source_revision" \
	'{schema_version: 2, build: {revision: $source_revision, modified: false},
	 config: {seed: 607, log_mode: "full", evidence_format: "evstream_v3",
	 experiment_id: "v2-r2-sv1-24h-treatment-607"}}' \
	>"$probe_cell/manifest.json"
for retained_json in greeks.json latency.json binary-evidence-attestation.json evidence-only-artifact-hash.json; do
	printf '{}\n' >"$probe_cell/$retained_json"
done
printf '{}\n' >"$probe_cell/checkpoints.jsonl"
printf '\000synthetic-binary-event-stream\n' >"$probe_cell/events.evs"
printf '%s\n' '{}' >"$probe_cell/venues/north/general.jsonl"
printf '%s\n' '{}' >"$probe_cell/market-data-evidence-v2.json"
for receipt_binary in market-data-schedules-v2.bin market-data-receipts-v2.bin market-data-decisions-v2.bin market-data-actions-v2.bin; do
	printf 'synthetic-receipt\n' >"$probe_cell/$receipt_binary"
done
v2_r2_require_evidence_input_file binary "$probe_cell/events.evs" || fail "binary fixture was rejected"
v2_r2_write_evidence_manifest "$probe_cell" || fail "synthetic evidence manifest generation failed"
v2_r2_verify_evidence_manifest "$probe_cell" || fail "synthetic evidence manifest verification failed"

jq -n \
	--arg source_revision "$source_revision" \
	--arg binary_sha256 "$binary_sha256" \
	--arg config_sha256 "$config_sha256" \
	--arg probe_root "$probe_root" \
	--argjson minimum_free_bytes "$minimum_free_bytes" \
	--argjson required_free_bytes "$required_free_bytes" \
	--argjson safety_margin_bytes "$safety_margin_bytes" \
	'{schema_version: 1, contract: "v2-integrated-longrun-r2-binary-capacity-v1",
	 measurement: "full_24h_binary_evidence_capacity_probe", evidence_format: "evstream_v3",
	 source_revision: $source_revision, binary_sha256: $binary_sha256,
	 config_sha256: $config_sha256, gomaxprocs: 4,
	 minimum_free_bytes: $minimum_free_bytes,
	 initial_available_free_bytes: 50000000000, available_free_bytes: 50000000000,
	 probe_root: $probe_root,
	 evidence_manifest_sha256: "PLACEHOLDER",
	 peak_output_bytes: 1, safety_margin_bytes: $safety_margin_bytes,
	 required_free_bytes: $required_free_bytes}' \
	>"$attestation"
manifest_sha256=$(sha256sum -- "$probe_cell/evidence-manifest.json" | awk '{print $1}')
jq --arg manifest_sha256 "$manifest_sha256" '.evidence_manifest_sha256 = $manifest_sha256' \
	"$attestation" >"$tmp_root/attestation-with-manifest.json"
mv -- "$tmp_root/attestation-with-manifest.json" "$attestation"
v2_r2_require_binary_capacity_attestation "$validator" "$source_revision" "$attestation" \
	"$config_sha256" 4 "$minimum_free_bytes" || fail "valid synthetic capacity attestation was rejected"
cp -- "$attestation" "$tmp_root/valid-attestation.json"

expect_failure() {
	if "$@" >"$tmp_root/command.stdout" 2>"$tmp_root/command.stderr"; then
		fail "command unexpectedly succeeded: $*"
	fi
}
require_fixture_intact() {
	[[ -d "$probe_root" && ! -L "$probe_root" ]] || fail "failed boundary removed the probe root"
	[[ -s "$attestation" && ! -L "$attestation" ]] || fail "failed boundary removed the attestation"
}
restore_attestation() {
	cp -- "$tmp_root/valid-attestation.json" "$attestation"
}

jq --arg current_revision "$(git -C "$root_dir" rev-parse HEAD)" \
	'.source_revision = $current_revision' "$attestation" >"$tmp_root/current-revision.json"
mv -- "$tmp_root/current-revision.json" "$attestation"
expect_failure "$archiver" "$attestation" "$validator" "$archive"
require_fixture_intact
restore_attestation

jq --arg wrong_config "$(printf '0%.0s' {1..64})" \
	'.config_sha256 = $wrong_config' "$attestation" >"$tmp_root/wrong-config.json"
mv -- "$tmp_root/wrong-config.json" "$attestation"
expect_failure "$archiver" "$attestation" "$validator" "$archive"
require_fixture_intact
restore_attestation

mv -- "$probe_root" "$probe_real"
ln -s -- "$probe_real" "$probe_root"
expect_failure "$archiver" "$attestation" "$validator" "$archive"
[[ -L "$probe_root" ]] || fail "symlink boundary was not preserved for inspection"
unlink -- "$probe_root"
mv -- "$probe_real" "$probe_root"
require_fixture_intact

cp -- "$probe_cell/events.evs" "$tmp_root/events.evs.valid"
printf '\000tampered-event-stream\n' >>"$probe_cell/events.evs"
expect_failure "$archiver" "$attestation" "$validator" "$archive"
require_fixture_intact
mv -- "$tmp_root/events.evs.valid" "$probe_cell/events.evs"
v2_r2_verify_evidence_manifest "$probe_cell" || fail "fixture was not restored after corruption test"

touch -- "$collision_archive"
expect_failure "$archiver" "$attestation" "$validator" "$collision_archive"
require_fixture_intact
unlink -- "$collision_archive"
touch -- "$archive.members"
expect_failure "$archiver" "$attestation" "$validator" "$archive"
require_fixture_intact
unlink -- "$archive.members"

"$archiver" "$attestation" "$validator" "$archive" >"$tmp_root/archive.stdout"
[[ ! -e "$probe_root" && ! -L "$probe_root" ]] || fail "successful archive retained the source probe root"
[[ ! -e "$attestation" && ! -L "$attestation" ]] || fail "successful archive retained the source attestation"
for published in "$archive" "$archive.members" "$archive.sha256" "$archive.retention.json"; do
	[[ -f "$published" && ! -L "$published" && -s "$published" ]] ||
		fail "published archive artifact is missing or empty: $published"
done
[[ -f "$archive.compare.log" && ! -L "$archive.compare.log" ]] ||
	fail "published archive comparison log is missing or symlinked"
zstd -t -- "$archive"
[[ ! -s "$archive.compare.log" ]] || fail "published archive comparison log is not empty"
archive_sha256=$(sha256sum -- "$archive" | awk '{print $1}')
read -r published_sha256 published_path <"$archive.sha256"
[[ "$published_sha256" == "$archive_sha256" && "$published_path" == "$archive" ]] ||
	fail "published archive checksum sidecar does not bind the archive"
grep -Fx "$(basename -- "$attestation")" "$archive.members" >/dev/null ||
	fail "archive member list omitted the attestation"
grep -F "external-scratch/$(basename -- "$probe_root")/treatment-607/events.evs" \
	"$archive.members" >/dev/null || fail "archive member list omitted binary evidence"
jq -e --arg archive "$archive" --arg archive_sha256 "$archive_sha256" \
	--arg source_revision "$source_revision" \
	'.schema_version == 1 and .contract == "v2-r2-sv1-capacity-archive-retention-v1" and
	 .source_revision == $source_revision and .archive == $archive and
	 .archive_sha256 == $archive_sha256 and .comparison == "tar_compare_clean" and
	 .disposition == "superseded_capacity_probe_compacted_before_new_revision_bound_measurement"' \
	"$archive.retention.json" >/dev/null || fail "retention receipt is not bound to the archive"

echo "SV1 capacity archive fixture: pass"
